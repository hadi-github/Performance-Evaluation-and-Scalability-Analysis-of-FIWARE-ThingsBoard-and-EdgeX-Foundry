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

Progress is appended to a JSONL log so an interrupted session can be resumed by
re-reading which ordinals already have a terminal record.
"""
import argparse
import json
import subprocess
import sys
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
RECEIPT = 'owner-w-matrix3-20261115a'
MATRIX = REPO / 'workloads/campaigns/fixed-points-w1-w4/run-matrix.json'
LOG_DEFAULT = REPO / 'results/workload-batch-20261108/driver-log.jsonl'
LAUNCHER = [sys.executable, 'scripts/run-workload-campaign.py']
MAX_ORDINAL = 36

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
)


def rows():
    data = json.loads(MATRIX.read_text())
    return sorted((r for r in data['experimental_runs'] if r['ordinal'] <= MAX_ORDINAL),
                  key=lambda r: r['ordinal'])


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
            running = plat

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