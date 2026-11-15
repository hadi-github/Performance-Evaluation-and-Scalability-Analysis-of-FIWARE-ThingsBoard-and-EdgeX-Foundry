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
RECEIPT = 'owner-w-matrix3-20261116e'
MATRIX = REPO / 'workloads/campaigns/fixed-points-w1-w4/run-matrix.json'
LOG_DEFAULT = REPO / 'results/workload-batch-20261108/driver-log.jsonl'
LAUNCHER = [sys.executable, 'scripts/run-workload-campaign.py']

# The authorized set: every remaining ordinal that fills a cell. Ordinals 1-10 are
# preserved pre-traffic failures and are never retried. Ordinals 4 and 7 completed
# and are accepted as-is. The burned cells are completed by a later pre-registered
# repetition (W1 -> 40-42, W2 edgex -> 44, W2 tb -> 45, W3 tb -> 47, W3 fw -> 48,
# W4 tb -> 37). See authorization/owner-w-matrix3-20261116e.json.
ORDINALS = tuple(range(11, 37)) + (37, 40, 41, 42, 44, 45, 47, 48)

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