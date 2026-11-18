#!/usr/bin/env python3
"""Serial driver for the authorized 36-row fixed-point W matrix.

Runs on the workstation. For each authorized ordinal, in strict ordinal order:

  1. `switch` (only when the platform changes) -- stops the other owned stacks so
     `isolated_one_platform_at_a_time` holds. Volume-preserving.
  2. `launch --execute-reviewed --receipt ...` -- the reviewed launcher, which
     re-verifies the receipt, plan hash, tree bindings and platform match itself.

It never retries an ID, never reuses a consumed ID, and never edits a plan. A
failed ordinal is recorded and the driver moves to the next one, because the
objective is to fill the matrix: stopping at the first failure would leave later
cells unmeasured for no better reason. Systemic failures still halt (see
HARD_STOP below).

Before a platform's first launch the driver runs a cheap environment repair
(see `repair_detached`). Docker Compose reuses a stopped container, and a
container left behind by an interrupted `up` can carry no network endpoint: the
stack then comes up with a published port that nothing answers on, and the
launcher fails *before* sending traffic. That is a host fault, not a platform
result, and it must not consume an ordinal. Removing such a stopped container is
safe -- Compose recreates it on the next `up` and named volumes are untouched.

Progress is appended to a JSONL log so an interrupted session can be resumed by
re-reading which ordinals already have a terminal record.
"""
import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
RECEIPT = 'owner-w-matrix3-20261117c'
MATRIX = REPO / 'workloads/campaigns/fixed-points-w1-w4/run-matrix.json'
LOG_DEFAULT = REPO / 'results/workload-batch-20261108/driver-log.jsonl'
LAUNCHER = [sys.executable, 'scripts/run-workload-campaign.py']

# The authorized set: every remaining ordinal that fills a cell. Ordinals 1-10 are
# preserved pre-traffic failures and are never retried. Ordinals 4 and 7 completed
# and are accepted as-is, as are 11-13 and 15-19. Ordinal 20 completed validly on
# both servers but the workstation reboot at 2026-11-16T06:40Z destroyed the local
# record before it was written, so its ID is spent and the W4/edgex cell takes its
# pre-registered repetition 4 (39) instead. W2/thingsboard and W3/thingsboard lost
# their only traffic-sending attempts to the pre-traffic CPU-snapshot defect, so
# they take repetition 5 (53, 55).
#
# Ordinal 21 (w4dp-tb-r02) reached traffic but is a harness defect, not a platform
# result: ThingsBoard stored all 22,500 messages (history 22,500 present, 0 missing)
# and every resource gate passed, yet the sink received nothing because the root rule
# chain was missing the `bench sink` TbRestApiCallNode. The launcher installs the
# outbound route for FIWARE and relies on the EdgeX export profile, but never calls
# `outbound.sh thingsboard`. The node is reinstalled and persisted; ordinal 21's ID is
# spent and is never reused. W4/thingsboard therefore takes repetitions 3, 4 and 5
# (29, 37, 60). See authorization/owner-w-matrix3-20261116g.json and
# results/workload-batch-20261108/evidence/w4dp-tb-r02-20261108/HARNESS-DEFECT.md.
#
# A second workstation reboot at 2026-11-16T15:17Z killed the queue at ordinal 25.
# Ordinal 25 (w3dp-fw-r03) had already completed validly on the load host (22,500/
# 22,500 acked, 0 missing) but lost its local record the same way ordinal 20 did, so
# its ID is spent too. W3/fiware is completed by repetitions 2, 4 and 5
# (17, 48, 56). See authorization/owner-w-matrix3-20261116h.json.
#
# The queue then ran to ordinal 45, where it hit a hard stop: ordinal 44
# (w2dp-ex-r04) ran fully valid traffic but leaked /run/iot-bench-workload.lock on
# the load host, and its analysis died on a scenario-start metadata check, so it
# produced no dispatch analysis. W2/edgex therefore looked short and its r05 spare
# (52) was authorized in owner-w-matrix3-20261117a; that spare then failed the
# EdgeX inventory gate, so it was never usable. Ordinal 44's own evidence was later
# recovered on disk (tolerance fix, no new traffic, no ID spent), which makes the
# r05 spare unnecessary. See the completion-batch note below.
#
# Final completion batch. Ordinal 44 was recovered on disk instead of being
# retried (see results/.../w2dp-ex-r04-20261108/harness-defect.json), so W2/edgex
# reached 3/3 at ordinals 13, 36 and 44 without spending its r05 spare. That left
# only the three ThingsBoard cells short, each at 1/3 because pre-traffic host
# faults had consumed every other pre-registered ID for those cells.
#
# The 2026-11-17 completion batch (owner-w-matrix3-20261117b) authorized r06/r07
# for all three cells. What actually happened, recorded rather than assumed:
#
#   W1/thingsboard  r2@o24 valid, then r06@o61 and r07@o62 both valid -> 3/3 DONE
#   W2/thingsboard  r3@o34 valid, then o63 and o64 both lost to host defects:
#                   o63 ran complete, clean traffic (22,500/22,500 records,
#                   15,000/15,000 measurement messages, 0 failures) and was then
#                   recorded NOT_ASSESSED_NO_ANALYSIS because the workstation's
#                   evidence fetch timed out; o64 was burned in 4.3 s by the
#                   capture port o63's own aborted sink still held. o63 was
#                   recovered on disk like ordinal 44 (no ID spent, no new
#                   traffic), leaving this cell at 2/3 and needing one more.
#   W3/thingsboard  r3@o27 valid, r06@o65 and r07@o66 still unspent.
#
# So this batch runs exactly three ordinals: W3's two authorized IDs, plus the one
# new W2 ID (r08@o67) that its two host defects made necessary. Every row is a
# repetition of an already-qualified fixed point: its plan differs from the cell's
# r05/r07 plan only in plan_id and seed. See
# authorization/owner-w-matrix3-20261117c.json.
ORDINALS = (65, 66, 67)

# Compose projects that publish the shared port 8404, per platform.
PROJECTS = {
    'thingsboard': ['tbq20261105'],
    'fiware': ['fiware', 'fiwhc'],
    'edgex': ['edgex'],
}

# Setup/activation problems that mean *no* row can run, so continuing is waste.
HARD_STOP = (
    'load-host workload lock is present',
    'dedicated capture port 18091 is occupied',
    'authorization receipt not found',
    'receipt has drifted from the tree',
    'refusing to launch without',
    'prebuilt tools do not match',
    'no prebuilt workspace tools',
    'load-host workload lock',
    # A full workstation disk fails the launch while staging the evidence copy,
    # which consumes the ordinal without sending any traffic. That is a host
    # fault, not a platform result: stop the whole run instead of marching on.
    'No space left on device',
    # Transport failures to the load host are host faults too, and they arrive in
    # bursts rather than one at a time. On 2026-11-18 a flaky SSH path to
    # ramonavm1 (measured at ~4-8% of handshakes when opened back-to-back, 0% when
    # paced, and 0/25 to the SUT) failed the staging `ssh` for ordinal 65, the
    # `benchctl resolve` call for 66, and the pre-launch `remote_absent` probe for
    # 67. None of the three sent a single message, yet all three were recorded
    # terminal and every authorized ID for the batch was consumed. These strings
    # stop the run after the first such fault instead.
    'port 2238: Connection timed out',
    'port 2238: No route to host',
    'port 2238 closed',
    'Connection reset by 2.188.205.118 port 2238',
    'kex_exchange_identification',
    'ssh ramonavm1 failed (255)',
    'could not determine whether',
)

# Every evidence directory for a W run is ~90 MB; a fresh run peaks higher while
# the source tarball and the fetched artifacts coexist. Refuse to spend an
# ordinal with less than this much headroom, so a full disk cannot burn one.
MIN_FREE_BYTES = 3 * 1024 ** 3


def disk_guard():
    """Return a problem description if the workstation is too full to run."""
    st = os.statvfs(str(REPO))
    free = st.f_bavail * st.f_frsize
    if free < MIN_FREE_BYTES:
        return (f'workstation has {free / 1024 ** 3:.1f} GiB free, '
                f'need {MIN_FREE_BYTES / 1024 ** 3:.0f} GiB for one run')
    return None


# A launch spends its ordinal the moment the launcher is entered, so a host fault
# that stops the launcher before traffic still costs an ID. Probing the two SSH
# paths a launch depends on is cheap and happens before any ordinal is touched.
#
# The fault this guards against is intermittent, not a hard outage: on 2026-11-18
# the load-host path failed roughly one handshake in fifteen when opened
# back-to-back, but never when paced 3 s apart, and never at all to the SUT. A
# single failed handshake would therefore be a bad reason to stop the campaign,
# while launching straight into one burns a run. So retry a few times and only
# call it a fault when the path stays broken.
PREFLIGHT_ATTEMPTS = 4
PREFLIGHT_BACKOFF_S = 5


def transport_guard():
    """Return a problem description if a launch's SSH paths are unusable."""
    import time
    hosts = (('ramonavm1', 'load host'), ('ramonavm2', 'SUT'))
    for alias, label in hosts:
        last = ''
        for attempt in range(PREFLIGHT_ATTEMPTS):
            p = subprocess.run(
                ['ssh', '-F', str(REPO / '.iotbench-ssh-config'),
                 '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=10', alias, 'true'],
                capture_output=True, text=True, timeout=60)
            if p.returncode == 0:
                break
            lines = (p.stderr or '').strip().splitlines()
            last = lines[-1] if lines else f'exit {p.returncode}'
            if attempt + 1 < PREFLIGHT_ATTEMPTS:
                time.sleep(PREFLIGHT_BACKOFF_S)
        else:
            return f'{label} ({alias}) SSH path is unusable after {PREFLIGHT_ATTEMPTS} tries: {last}'
    return None


def rows():
    data = json.loads(MATRIX.read_text())
    by_ordinal = {r['ordinal']: r for r in data['experimental_runs']}
    missing = [o for o in ORDINALS if o not in by_ordinal]
    if missing:
        raise SystemExit(f'run-matrix.json has no row for authorized ordinal(s) {missing}')
    return [by_ordinal[o] for o in ORDINALS]


def ssh_sut(command, timeout=120):
    """Run a read-only or repair command on the SUT over the configured SSH alias."""
    p = subprocess.run(['ssh', '-F', str(REPO / '.iotbench-ssh-config'), 'ramonavm2', command],
                       capture_output=True, text=True, timeout=timeout)
    return p.returncode, p.stdout, p.stderr


def ssh_load(command, timeout=120):
    """Run a command on the load host over the configured SSH alias."""
    p = subprocess.run(['ssh', '-F', str(REPO / '.iotbench-ssh-config'), 'ramonavm1', command],
                       capture_output=True, text=True, timeout=timeout)
    return p.returncode, p.stdout, p.stderr


# A workload that is killed rather than allowed to exit cleanly leaves
# /run/iot-bench-workload.lock behind on the load host. The next launch then
# refuses with 'load-host workload lock is present', which is a HARD_STOP, so one
# leaked lock silently ends the whole campaign -- exactly what happened at ordinal
# 44 on 2026-11-17. The lock is only meaningful while a workload process is alive,
# so clear it only when every one of these holds:
#   * the lock exists and is an empty directory (no ownership file inside),
#   * no k6 / benchctl / sink process is running on the load host,
#   * no run-capture.sh is still working on a run.
# Anything else is left alone and stays a hard stop for a human to look at.
LOCK_PROCS = ('k6', 'benchctl', 'sink')
LOCK_SCRIPT_MARKER = 'run-capture.sh'


def clear_stale_load_lock():
    """Remove a leaked workload lock, but only when provably nobody owns it."""
    rc, out, err = ssh_load(
        'set -u; '
        'if [ ! -d /run/iot-bench-workload.lock ]; then echo absent; exit 0; fi; '
        'if [ -n "$(ls -A /run/iot-bench-workload.lock 2>/dev/null)" ]; then '
        'echo nonempty; exit 0; fi; '
        'procs=$(ps -eo comm= | grep -cE "^(k6|benchctl|sink)$" || true); '
        'scripts=$(pgrep -fc "' + LOCK_SCRIPT_MARKER + '" || true); '
        'echo "procs=${procs:-0} scripts=${scripts:-0}"; '
        'if [ "${procs:-0}" -eq 0 ] && [ "${scripts:-0}" -eq 0 ]; then '
        'rmdir /run/iot-bench-workload.lock && echo cleared || echo failed; '
        'else echo busy; fi', timeout=120)
    state = (out or '').strip().splitlines()
    verdict = state[-1] if state else (err or '').strip()[-200:]
    return {'exit': rc, 'state': verdict,
            'detail': ' '.join(state[:-1])[-200:] if len(state) > 1 else ''}


def repair_detached(platform):
    """Drop stopped containers that Compose would reuse without a network endpoint.

    A container that exists but is not running and has no network attachment was
    left behind by an interrupted `up`. Compose's next `up` starts that same
    container instead of creating a fresh one, so the published port has no
    listener and every request fails before traffic. Removing it forces a clean
    create. Named volumes, images and the running stack are untouched.
    """
    projects = PROJECTS[platform]
    names = ','.join(f"label=com.docker.compose.project={p}" for p in projects)
    cmd = (
        'for c in $(docker ps -a --filter status=exited --filter status=created '
        f'--format "{{{{.Names}}}}" {names}); do '
        'n=$(docker inspect -f "{{len .NetworkSettings.Networks}}" "$c" 2>/dev/null || echo 0); '
        'if [ "${n:-0}" -eq 0 ]; then echo "detached: $c"; docker rm "$c" >/dev/null && '
        'echo "removed: $c"; fi; done'
    )
    rc, out, err = ssh_sut(cmd, timeout=180)
    if rc != 0:
        return {'platform': platform, 'exit': rc, 'stderr': err[-800:]}
    return {'platform': platform, 'removed': [l.split(': ', 1)[1] for l in out.splitlines()
                                              if l.startswith('removed: ')]}


def switch_exporters(platform):
    """Point the SUT's platform-specific exporters at the platform about to run.

    Monitoring only. A failure here costs exporter coverage for that platform but
    cannot invalidate a run, so it is logged and never treated as a hard stop.
    """
    cmd = (f'test -x /root/iot-bench-deploy/monitoring/exporters/exporters-switch.sh && '
           f'cd /root/iot-bench-deploy/monitoring/exporters && '
           f'TB_PROJECT=tbq20261105 ./exporters-switch.sh {platform}')
    rc, out, err = ssh_sut(cmd, timeout=300)
    return {'platform': platform, 'exit': rc,
            'stdout': out.strip()[-400:], 'stderr': err.strip()[-400:]}


def load_log(path):
    done = {}
    if path.is_file():
        for line in path.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            if rec.get('event') == 'terminal':
                done[rec['run_id']] = rec
    return done


def append(path, record):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('a') as f:
        f.write(json.dumps(record, sort_keys=True) + '\n')
        f.flush()


def run(cmd, timeout):
    started = time.time()
    try:
        p = subprocess.run(cmd, cwd=REPO, capture_output=True, text=True, timeout=timeout)
        return p.returncode, p.stdout, p.stderr, time.time() - started
    except subprocess.TimeoutExpired as e:
        return 124, (e.stdout or ''), f'timed out after {timeout}s', time.time() - started


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--receipt', default=RECEIPT)
    ap.add_argument('--log', type=Path, default=LOG_DEFAULT)
    ap.add_argument('--start-ordinal', type=int, default=1)
    ap.add_argument('--stop-after', type=int, default=0,
                    help='stop after this many launches this invocation (0 = all)')
    ap.add_argument('--switch-timeout', type=int, default=900)
    ap.add_argument('--launch-timeout', type=int, default=5400)
    ap.add_argument('--dry-run', action='store_true')
    args = ap.parse_args()

    all_rows = rows()
    done = load_log(args.log)
    todo = [r for r in all_rows
            if r['ordinal'] >= args.start_ordinal and r['run_id'] not in done]

    print(f'authorized rows: {len(all_rows)}  already terminal: {len(done)}  to run: {len(todo)}')
    if not todo:
        return 0

    if args.dry_run:
        for r in todo:
            print(f"  would run ordinal {r['ordinal']:>2} {r['run_id']:<26} {r['platform']}")
        return 0

    running = None          # platform whose containers are currently up
    launched = 0
    for row in todo:
        rid, plat, ordinal = row['run_id'], row['platform'], row['ordinal']
        if args.stop_after and launched >= args.stop_after:
            print('stop-after reached; leaving the rest for a later invocation')
            break

        problem = disk_guard()
        if problem:
            print(f'hard stop before ordinal {ordinal}: {problem}')
            append(args.log, {'event': 'hard-stop', 'reason': 'disk-space',
                              'ordinal': ordinal, 'run_id': rid, 'detail': problem})
            return 2

        # Refuse to enter the launcher at all while a launch's SSH paths are
        # broken: the launcher spends this ordinal on its first remote call, so a
        # failed handshake there costs a run without sending any traffic.
        problem = transport_guard()
        if problem:
            print(f'hard stop before ordinal {ordinal}: {problem}')
            append(args.log, {'event': 'hard-stop', 'reason': 'ssh-transport',
                              'ordinal': ordinal, 'run_id': rid, 'detail': problem})
            return 2

        if plat != running:
            print(f'\n=== ordinal {ordinal}: switching to {plat} (stopping other stacks) ===')
            rc, out, err, secs = run(
                LAUNCHER + ['switch', '--execute-reviewed', '--receipt', args.receipt,
                            '--timeout', str(args.switch_timeout)],
                args.switch_timeout + 300)
            append(args.log, {'event': 'switch', 'target_platform': plat,
                              'exit': rc, 'seconds': round(secs, 1),
                              'stdout_tail': out[-800:], 'stderr_tail': err[-800:]})
            if rc != 0:
                print(f'switch failed (exit {rc}): {err.strip()[:400]}')
                append(args.log, {'event': 'terminal', 'run_id': rid, 'ordinal': ordinal,
                                  'platform': plat, 'outcome': 'BLOCKED_SWITCH',
                                  'exit': rc, 'detail': err.strip()[:2000]})
                if any(m in err for m in HARD_STOP):
                    print('hard stop: environment is not in a runnable state')
                    return 2
                continue
            rep = repair_detached(plat)
            append(args.log, {'event': 'repair', **rep})
            if rep.get('removed'):
                print(f"  removed stale detached container(s): {', '.join(rep['removed'])}")
            if rep.get('exit'):
                print(f'  repair could not run: {rep["stderr"].strip()[:300]}')
            running = plat

        # Exporters are per platform (9187 is shared by ThingsBoard and EdgeX), so
        # point them at this platform before the launcher starts the stack. The
        # launcher's own `up` creates the network the exporters join, so this runs
        # again after the first successful start of a platform; a failure here is
        # logged and ignored.
        exp = switch_exporters(plat)
        append(args.log, {'event': 'exporters', **exp})

        # A killed workload leaves its lock behind and the launcher then refuses,
        # which would end the campaign. Clear it only when provably unowned.
        lock = clear_stale_load_lock()
        append(args.log, {'event': 'load-lock', **lock})
        if lock['state'] == 'cleared':
            print('  cleared a leaked load-host workload lock (no workload alive)')
        elif lock['state'] in ('busy', 'nonempty'):
            print(f"  load-host lock is {lock['state']}; leaving it for inspection")

        print(f'\n=== ordinal {ordinal}: launching {rid} ({plat}) ===')
        rc, out, err, secs = run(
            LAUNCHER + ['launch', '--platform', plat, '--run-id', rid,
                        '--execute-reviewed', '--receipt', args.receipt,
                        '--timeout', str(args.launch_timeout)],
            args.launch_timeout + 900)
        launched += 1
        outcome = 'COMPLETED' if rc == 0 else f'FAILED_EXIT_{rc}'
        print(f'  -> {outcome} in {secs / 60:.1f} min')
        append(args.log, {'event': 'terminal', 'run_id': rid, 'ordinal': ordinal,
                          'platform': plat, 'outcome': outcome, 'exit': rc,
                          'seconds': round(secs, 1),
                          'stdout_tail': out[-2000:], 'stderr_tail': err[-4000:]})
        if rc != 0 and any(m in (err + out) for m in HARD_STOP):
            print('hard stop: environment is not in a runnable state')
            return 2

    done = load_log(args.log)
    print(f'\nterminal records: {len(done)} / {len(all_rows)}')
    return 0


if __name__ == '__main__':
    sys.exit(main())