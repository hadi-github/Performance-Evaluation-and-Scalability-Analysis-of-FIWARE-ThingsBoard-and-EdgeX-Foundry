#!/usr/bin/env python3
"""Serial, fail-closed supervisor for the exact remaining frozen ordinary matrix.

Run inside an explicitly created tmux session on the SUT, not as a system boot
service. A disconnect does not kill it; a host reboot does. No retry or resume
of attempted IDs is implemented. Each next cell requires the prior result to
pass all launcher gates. A STOP_AFTER_CURRENT file pauses before the next cell.
"""
import argparse
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import time

REPO = Path(__file__).resolve().parents[1]
BASE = REPO / 'results/feasible-campaign-20261106'
LAUNCHER = BASE / 'run-feasible-campaign.py'
REQUIRED = ('measurement_valid', 'capture_valid', 'latency_assessed',
            'generator_resource_qualified', 'sut_resource_qualified',
            'cleanup_verified', 'post_run_check_qualified', 'history_retrieved',
            'reanalysis_with_history', 'analysis_retrieved')


def verify_success(result, cell):
    if result.get('run_id') != cell['run_id'] or result.get('platform') != cell['platform']:
        raise RuntimeError('result identity differs from frozen cell')
    if result.get('run_kind') != 'ordinary' or result.get('workload_policy') != 'dispatch-integrity-v1':
        raise RuntimeError('result is not the frozen ordinary workload policy')
    failed = [key for key in REQUIRED if result.get(key) is not True]
    if failed:
        raise RuntimeError('mandatory result gates failed: ' + ', '.join(failed))
    if (result.get('platform_result') != 'VALID_PLATFORM_SUCCESS' or
            result.get('history_result') != 'VALID_HISTORY_SUCCESS' or
            result.get('workload_lock_retained') is not False or
            result.get('post_run_defect') not in (None, False) or result.get('fetch_errors') != [] or
            not result.get('finish', '').startswith('stopped owned services; volumes preserved')):
        raise RuntimeError('platform/history/ownership/finish gate failed; review, never retry')


def write_state(path, state):
    state['updated_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    temp = path.with_suffix('.json.tmp')
    temp.write_text(json.dumps(state, indent=2)+'\n')
    temp.replace(path)


def input_binding(matrix, start_ordinal):
    """Hash every frozen plan and staged measurement input before mutations."""
    spec = importlib.util.spec_from_file_location('controller_launcher', LAUNCHER)
    launcher = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(launcher)
    verified = launcher.validate_prebuilt_tools()
    if not verified:
        raise RuntimeError('persistent controller requires verified prebuilt tools')
    prebuilt, provenance = verified
    paths = [REPO / p for p in launcher.STAGED_SCRIPTS]
    paths.extend((LAUNCHER, Path(__file__)))
    paths.extend(p for p in (REPO/'workloads/k6').rglob('*') if p.is_file())
    paths.extend(REPO/p for p in ('scripts/aggregate-runs.py', 'scripts/probe-detached-generator.sh',
                 'deploy/edgex/scripts/provision-then-scale.sh', 'deploy/edgex/compose.feasible-discovery.yaml'))
    sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
    return {'scope_sha256':sha(BASE/'frozen-scope.json'),
            'matrix_sha256':sha(BASE/'run-matrix.json'),
            'remaining_plans':{c['plan']:sha(BASE/c['plan']) for c in matrix[start_ordinal-1:]},
            'measurement_sources':{str(p.relative_to(REPO)):sha(p) for p in sorted(set(paths))},
            'prebuilt_manifest_sha256':sha(prebuilt/'build-provenance.json'),
            'binary_sha256':{name:sha(prebuilt/name) for name in ('benchctl','sink')},
            'go_source_sha256':provenance['go_source_sha256']}


def execute(args):
    matrix_path = BASE / 'run-matrix.json'
    matrix = json.loads(matrix_path.read_text())['ordinary_runs']
    receipt = json.loads(args.review.read_text())
    if (receipt.get('verdict') != 'ACCEPTED_PERSISTENT_ORDINARY_CONTINUATION' or
            receipt.get('authorized_start_ordinal') != args.start_ordinal or
            receipt.get('matrix_sha256') != hashlib.sha256(matrix_path.read_bytes()).hexdigest() or
            receipt.get('controller_source_sha256') != hashlib.sha256(Path(__file__).read_bytes()).hexdigest() or
            receipt.get('launcher_source_sha256') != hashlib.sha256(LAUNCHER.read_bytes()).hexdigest()):
        raise RuntimeError('review does not release this exact continuation')
    if not args.execute_reviewed or not os.environ.get('IOT_BENCH_SSH_CONFIG'):
        raise RuntimeError('explicit execution approval and private SSH configuration required')
    probe = json.loads(args.probe_receipt.read_text())
    if probe.get('verdict') != 'PASS_PERSISTENCE_GENERATOR_PROBE' or probe.get('reviewed_for_continuation') is not True:
        raise RuntimeError('reviewed detached generator probe is required')
    if receipt.get('probe_receipt_sha256') != hashlib.sha256(args.probe_receipt.read_bytes()).hexdigest():
        raise RuntimeError('probe receipt differs from reviewed bytes')
    if not 1 <= args.start_ordinal <= len(matrix):
        raise RuntimeError('start ordinal out of range')
    if receipt.get('input_binding') != input_binding(matrix, args.start_ordinal):
        raise RuntimeError('frozen scope/plans/measurement sources/binaries differ from review')
    args.state_dir.mkdir(parents=True, exist_ok=True)
    state_path = args.state_dir / 'controller-state.json'
    if state_path.exists():
        raise RuntimeError('controller state already exists; reconcile instead of resuming')
    state = {'state': 'STARTING', 'start_ordinal': args.start_ordinal,
             'completed_this_controller': [], 'review': str(args.review),
             'probe_receipt': str(args.probe_receipt), 'pid': os.getpid(),
             'host_boot_id': Path('/proc/sys/kernel/random/boot_id').read_text().strip()}
    write_state(state_path, state)
    try:
        for cell in matrix[args.start_ordinal-1:]:
            if receipt['input_binding'] != input_binding(matrix, args.start_ordinal):
                raise RuntimeError('reviewed inputs changed between attempts')
            if (args.state_dir / 'STOP_AFTER_CURRENT').exists():
                state.update(state='PAUSED_BY_STOP_FILE', next_run=cell['run_id'])
                write_state(state_path, state)
                return 0
            if cell['ordinal'] != matrix.index(cell)+1:
                raise RuntimeError('matrix ordinals are inconsistent')
            evidence = BASE / 'evidence' / cell['run_id']
            if evidence.exists() and any(evidence.iterdir()):
                raise RuntimeError('attempt already has evidence; no automatic reuse: '+cell['run_id'])
            state.update(state='RUNNING', ordinal=cell['ordinal'], current_run=cell['run_id'])
            write_state(state_path, state)
            print('START', cell['ordinal'], cell['run_id'], flush=True)
            # Explicit between-run stack handoff. The launcher refuses an owned
            # workload lock, competing generator or occupied capture listener.
            common = ['--execute-reviewed', '--sol-review', str(args.review)]
            subprocess.run([sys.executable, str(LAUNCHER), 'switch', *common], cwd=REPO, check=True)
            subprocess.run([sys.executable, str(LAUNCHER), 'launch', '--platform', cell['platform'],
                            '--run-id', cell['run_id'], '--plan', str(BASE / cell['plan']),
                            *common], cwd=REPO, check=True)
            result = json.loads((evidence / 'run-result.json').read_text())
            verify_success(result, cell)
            if result.get('plan_sha256') != hashlib.sha256((BASE / cell['plan']).read_bytes()).hexdigest():
                raise RuntimeError('result plan hash differs from frozen plan')
            state['completed_this_controller'].append(cell['run_id'])
            state.update(state='BETWEEN_RUNS', last_run=cell['run_id'])
            write_state(state_path, state)
            subprocess.run([sys.executable, str(REPO / 'scripts/aggregate-runs.py'),
                            str(BASE / 'evidence'), '--out', str(BASE / 'aggregate'), '--force'],
                           cwd=REPO, check=True)
            print('ACCEPTED', cell['run_id'], flush=True)
        state.update(state='FINISHED_ORIGINAL_MATRIX', current_run=None)
        write_state(state_path, state)
        return 0
    except BaseException as error:
        state.update(state='PAUSED_REQUIRES_REVIEW', error=str(error))
        write_state(state_path, state)
        print('PAUSED_REQUIRES_REVIEW:', error, flush=True)
        raise


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--start-ordinal', type=int, required=True)
    ap.add_argument('--review', type=Path, required=True)
    ap.add_argument('--probe-receipt', type=Path, required=True)
    ap.add_argument('--state-dir', type=Path, required=True)
    ap.add_argument('--execute-reviewed', action='store_true')
    args = ap.parse_args()
    # Keep a supervisor lock across the whole serial batch, not just one cell.
    with (BASE / '.persistent-controller.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        return execute(args)


if __name__ == '__main__':
    sys.exit(main())
