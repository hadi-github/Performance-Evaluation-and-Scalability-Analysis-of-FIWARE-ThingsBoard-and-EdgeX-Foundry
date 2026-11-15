#!/usr/bin/env python3
"""Serial, fail-closed driver for the W4 proposed attempts. Offline by default.

W4 is a one-factor replica experiment (N=1 on the same two-core pool 4-5) with
five repetitions per platform. This driver runs the cells the frozen controls
permit, in matrix order, and stops at the first failure instead of working
around it.

Design constraints, all inherited from the campaign rather than invented here:

* **No retry, no resume.** A run ID is consumed the moment it launches. A failed
  attempt is preserved as evidence and the driver stops for review; it never
  re-launches the same ID or substitutes another.
* **One platform at a time.** `isolated_one_platform_at_a_time` holds: each cell
  goes through the launcher's reviewed `switch` before its own stack starts.
* **Frozen controls are read-only.** The matrix, scope and gates are hashed by
  the authorization receipt. This script only reads them.
* **Inventory-blocked cells are skipped, never faked.** EdgeX repetitions 3-5
  cannot fit under the frozen 16,384 discovery limit (see
  research/findings/w4-edgex-inventory-blocks-three-repetitions-20261114.md).
  They are reported as skipped with that reason and left unconsumed.

Usage (from a tmux session on the workstation, so a disconnect does not kill it):

    python3 scripts/run-w4-campaign.py --plan-only
    python3 scripts/run-w4-campaign.py --execute-reviewed --receipt owner-w4-...
"""
import argparse
import importlib.util
import json
import os
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
W_ROOT = REPO / 'workloads/campaigns/fixed-points-w1-w4'
LAUNCHER = REPO / 'scripts/run-workload-campaign.py'
EVIDENCE = REPO / 'results/workload-batch-20261108/evidence'
STATE = REPO / 'results/workload-batch-20261108/w4-campaign-state.json'

# A cell that the frozen discovery limit cannot admit. Recorded here as data so
# the skip is explicit and reviewable, not a silent filter on a magic number.
# Retained inventory is 15,192 after W3 EdgeX plus the N=1 readiness preflight
# cohort; one further 1,000-device repetition reaches 16,192 and fits, the next
# would reach 17,192 and exceed the unchanged 16,384 limit. See
# research/findings/w4-edgex-inventory-blocks-four-repetitions-20261114.md.
INVENTORY_BLOCKED = {
    'w4dp-ex-r02-20261108': 'retained 16,192 + cohort 1,000 exceeds the frozen 16,384 discovery limit',
    'w4dp-ex-r03-20261108': 'retained 16,192 + cohort 1,000 exceeds the frozen 16,384 discovery limit',
    'w4dp-ex-r04-20261108': 'retained 16,192 + cohort 1,000 exceeds the frozen 16,384 discovery limit',
    'w4dp-ex-r05-20261108': 'retained 16,192 + cohort 1,000 exceeds the frozen 16,384 discovery limit',
}

# When the owner amendment is selected, the cap is 50,000 and the readiness cost
# is O(profiles) instead of O(devices), so those four cells stop being blocked.
# The skip list is kept (rather than deleted) so the historical reason stays
# legible and the flag-off behaviour is byte-identical.
AMENDMENT_ENV = 'IOTBENCH_EDGEX_DISCOVERY_AMENDMENT'


def amendment_enabled():
    return os.environ.get(AMENDMENT_ENV, '') == '1'


def load_b0():
    spec = importlib.util.spec_from_file_location(
        'b0launcher', REPO / 'results/feasible-campaign-20261106/run-feasible-campaign.py')
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def w4_rows():
    """Every W4 row, in matrix ordinal order. Reads the frozen matrix as-is."""
    matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
    if matrix.get('status') != 'PROPOSED_NOT_LAUNCH_CLEARED':
        raise SystemExit(f'unexpected matrix status: {matrix.get("status")!r}; refusing to run')
    rows = [r for r in matrix['experimental_runs'] if r['workload_id'] == 'W4']
    return sorted(rows, key=lambda r: r['ordinal'])


def plan(rows):
    """Split the matrix into runnable and skipped cells, with reasons."""
    runnable, skipped = [], []
    amended = amendment_enabled()
    for row in rows:
        rid = row['run_id']
        if rid in INVENTORY_BLOCKED and not amended:
            skipped.append({'run_id': rid, 'ordinal': row['ordinal'],
                            'platform': row['platform'],
                            'reason': INVENTORY_BLOCKED[rid]})
        else:
            runnable.append(row)
    return runnable, skipped


def workload_policy_ok(result, run_id):
    """The mandatory gates, with W's own vocabulary for a platform finding.

    W differs from the B0 ordinary campaign here in exactly one place, and the
    campaign documents why: for a *proposed experimental attempt* a platform
    backlog is a result, not a harness defect. `VALID_PLATFORM_FAILURE_OR_BACKLOG`
    means the harness was clean and the platform was measured — that is the
    finding the attempt was made to obtain. It must never be retried to get a
    nicer number.

    What still stops the driver is anything that makes the *measurement* untrustworthy:
    a failed mandatory gate, a history failure, a retained workload lock, a post-run
    defect, a fetch error, or a run that did not stop its owned services. Those would
    be harness faults worth an owner look, not a platform result.
    """
    if result.get('run_id') != run_id:
        return f'result identity {result.get("run_id")!r} differs from requested {run_id!r}'
    if result.get('run_kind') != 'ordinary':
        return f'run_kind={result.get("run_kind")!r}, not an ordinary workload attempt'
    required = ('measurement_valid', 'capture_valid', 'latency_assessed',
                'generator_resource_qualified', 'sut_resource_qualified',
                'cleanup_verified', 'post_run_check_qualified', 'history_retrieved',
                'reanalysis_with_history', 'analysis_retrieved')
    failed = [k for k in required if result.get(k) is not True]
    if failed:
        return 'mandatory result gates failed: ' + ', '.join(failed)
    if result.get('platform_result') not in ('VALID_PLATFORM_SUCCESS',
                                             'VALID_PLATFORM_FAILURE_OR_BACKLOG'):
        return f'platform_result={result.get("platform_result")!r} is not an assessed platform result'
    if result.get('history_result') != 'VALID_HISTORY_SUCCESS':
        return f'history_result={result.get("history_result")!r}'
    if result.get('workload_lock_retained') is not False:
        return 'workload lock was retained'
    if result.get('post_run_defect') not in (None, False):
        return f'post_run_defect={result.get("post_run_defect")!r}'
    if result.get('fetch_errors') != []:
        return f'fetch_errors={result.get("fetch_errors")!r}'
    if not str(result.get('finish', '')).startswith('stopped owned services; volumes preserved'):
        return f'unexpected finish: {result.get("finish")!r}'
    return None


def write_state(state):
    STATE.parent.mkdir(parents=True, exist_ok=True)
    tmp = STATE.with_suffix('.json.tmp')
    tmp.write_text(json.dumps(state, indent=2) + '\n')
    tmp.replace(STATE)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--plan-only', action='store_true',
                    help='print the runnable/skipped split and exit; touches nothing')
    ap.add_argument('--execute-reviewed', action='store_true',
                    help='actually launch; required for any mutation')
    ap.add_argument('--receipt', help='owner-authorization receipt ID for these run IDs')
    ap.add_argument('--platform', choices=('thingsboard', 'fiware', 'edgex'),
                    help='limit this invocation to one platform (repeat invocations serially)')
    ap.add_argument('--timeout', type=int, default=5400,
                    help='per-run SSH launch timeout passed to the launcher')
    args = ap.parse_args()

    rows = w4_rows()
    runnable, skipped = plan(rows)
    if args.platform:
        runnable = [r for r in runnable if r['platform'] == args.platform]

    if args.plan_only:
        print(json.dumps({'runnable': [{'ordinal': r['ordinal'], 'run_id': r['run_id'],
                                       'platform': r['platform'], 'repetition': r['repetition']}
                                      for r in runnable],
                          'skipped': skipped,
                          'counts': {'runnable': len(runnable), 'skipped': len(skipped)}},
                         indent=2))
        return 0

    if not (args.execute_reviewed and args.receipt):
        raise SystemExit('refusing to launch without --execute-reviewed and --receipt ID')

    state = {'state': 'STARTING', 'receipt': args.receipt,
             'completed': [], 'skipped': skipped, 'failed': None}
    write_state(state)
    try:
        for row in runnable:
            rid = row['run_id']
            prior = EVIDENCE / rid
            if prior.exists() and any(prior.iterdir()):
                raise SystemExit(f'{rid} already has evidence; a consumed ID is never relaunched')
            state.update(state='RUNNING', current=rid, ordinal=row['ordinal'])
            write_state(state)
            print(f'START {row["ordinal"]:2d} {rid} {row["platform"]}', flush=True)

            common = ['--execute-reviewed', '--receipt', args.receipt]
            subprocess.run([sys.executable, str(LAUNCHER), 'switch', *common],
                           cwd=REPO, check=True)
            subprocess.run([sys.executable, str(LAUNCHER), 'launch',
                            '--platform', row['platform'], '--run-id', rid,
                            '--timeout', str(args.timeout), *common],
                           cwd=REPO, check=True)

            result_path = prior / 'run-result.json'
            if not result_path.is_file():
                raise SystemExit(f'{rid}: launch produced no run-result.json')
            result = json.loads(result_path.read_text())
            problem = workload_policy_ok(result, rid)
            if problem:
                raise SystemExit(f'{rid}: {problem}; attempt preserved, never retried')
            state['completed'].append(rid)
            state.update(state='BETWEEN_RUNS', current=None)
            write_state(state)
            print(f'ACCEPTED {rid}', flush=True)

        state.update(state='FINISHED', current=None)
        write_state(state)
        return 0
    except BaseException as exc:
        state.update(state='STOPPED_REQUIRES_REVIEW', error=str(exc))
        write_state(state)
        raise


if __name__ == '__main__':
    sys.exit(main())
