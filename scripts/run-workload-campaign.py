#!/usr/bin/env python3
"""Workload-aware (W1-W4) runner.

B0's launcher (`results/feasible-campaign-20261106/run-feasible-campaign.py`)
hard-rejects W run IDs and validates plans against the frozen B0 `plans/`
directory. This module is the separate W runner the qualification gates require:
it validates against the W scope/matrix/plans and stages evidence under its own
root, so W never contaminates B0 provenance.

It deliberately does NOT reimplement the launch machinery. The B0 launcher's
staging, activation, gate, capture, cleanup and evidence-retrieval functions are
reused as-is; only plan resolution, plan validation and the evidence/remote
roots are bound to W. That keeps a single implementation of the risky parts.

Read-only by default. `--check` validates everything and announces what would
run without touching a server.
"""
import argparse
import importlib.util
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import pool_policy  # noqa: E402  (single source of truth for the VU pool)

REPO = Path(__file__).resolve().parents[1]
W_ROOT = REPO / 'workloads/campaigns/fixed-points-w1-w4'
W_QUAL_ROOT = W_ROOT / 'qualification'
B0_MODULE = REPO / 'results/feasible-campaign-20261106/run-feasible-campaign.py'
# W gets its own remote + local evidence roots. Never share B0's.
W_REMOTE_RESULTS = '/root/iot-qual-load-20261105/feasible-workload-w-20261108'
W_EVIDENCE = REPO / 'results/workload-batch-20261108/evidence'

PLATFORM_PREFIX = {'thingsboard': 'tb', 'fiware': 'fw', 'edgex': 'ex'}
SUPPORTED = ('thingsboard', 'fiware', 'edgex')


def load_b0():
    spec = importlib.util.spec_from_file_location('b0launcher', B0_MODULE)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def load_scope():
    return json.loads((W_ROOT / 'scope.json').read_text())


def load_matrix():
    return json.loads((W_ROOT / 'run-matrix.json').read_text())


def matrix_runs():
    m = load_matrix()
    if m.get('status') != 'PROPOSED_NOT_LAUNCH_CLEARED':
        raise SystemExit(f"unexpected W matrix status: {m.get('status')!r}")
    return m['experimental_runs']


def qualification_index_paths():
    """Every workload's qualification ledger, W1 first.

    W1 keeps the historical root so its ledger is not rewritten. A later
    workload gets its own subdirectory, because one index cannot hold rows that
    ran different shapes under a single `variants[...]` lookup.
    """
    paths = []
    legacy = W_QUAL_ROOT / 'index.json'
    if legacy.is_file():
        paths.append(('W1', legacy))
    for wid in ('W2', 'W3', 'W4'):
        p = W_QUAL_ROOT / wid.lower() / 'index.json'
        if p.is_file():
            paths.append((wid, p))
    return paths


def qual_root_for(row):
    """Directory holding a qualification row's plan."""
    wid = row.get('workload_id', 'W1')
    return W_QUAL_ROOT if wid == 'W1' else W_QUAL_ROOT / wid.lower()


def load_qualification_index():
    paths = qualification_index_paths()
    if not paths:
        raise SystemExit(f'qualification index missing: {W_QUAL_ROOT / "index.json"}; '
                         'run scripts/stage-w1-qualification.py')
    merged = {'schema_version': 1, 'status': None, 'execution_authorized': False, 'runs': []}
    for wid, path in paths:
        idx = json.loads(path.read_text())
        if idx.get('status') != 'NOT_LAUNCH_CLEARED':
            raise SystemExit(f"unexpected qualification status: {idx.get('status')!r}")
        for row in idx['runs']:
            row.setdefault('workload_id', wid)
        merged['runs'].extend(idx['runs'])
    merged['status'] = 'NOT_LAUNCH_CLEARED'
    return merged


def qualification_rows():
    return load_qualification_index()['runs']


def validate_qualification_shape(row, resolved, scope):
    """Bind a resolved qualification to the gate it is meant to clear.

    A qualification that silently changed shape would 'pass' while proving
    nothing about its workload, so every load-bearing field is checked against
    the scope and against the canonical attempt plan for the same platform and
    workload.
    """
    workload = row.get('workload_id', 'W1')
    variant = scope['variants'][workload]
    shared = scope['shared']
    platform = row['platform']
    canonical = json.loads(w_plan_path(
        find_row(f'{workload.lower()}dp-{PLATFORM_PREFIX[platform]}-r01-20261108')).read_text())
    steps = [(s.get('phase'), s.get('rate'), s.get('duration_s'), s.get('vus'))
             for s in resolved.get('steps', [])]
    rate = variant['total_messages_per_second']
    # Pool is rate-derived (2026-11-10 owner amendment), not the old fixed 150.
    pool = pool_policy.pool_for_rate(rate)
    policy = pool_policy.policy_for_rate(rate)
    checks = {
        'adapter': resolved.get('adapter') == platform,
        'target': resolved.get('target') == canonical.get('target'),
        'devices': resolved.get('devices') == variant['devices'],
        'rate_and_steps': steps == [('warmup', rate, shared['warmup_seconds'], pool),
                                    ('measurement', rate, shared['measurement_seconds'], pool)],
        'timeout': resolved.get('request_timeout_ms') == shared['request_timeout_ms'],
        'keep_alive': (resolved.get('connection') or {}).get('keep_alive') == shared['keep_alive'],
        'vus': (resolved.get('vus') or {}).get('pre_allocated') == pool
               and (resolved.get('vus') or {}).get('max') == pool,
        'padding': resolved.get('payload', {}).get('size_offset_bytes') == variant['extra_logical_padding_bytes'],
        'policy': resolved.get('health', {}).get('workload_policy') == policy,
        'no_faults': not resolved.get('test_faults'),
        'seed': resolved.get('seed') == row['seed'],
        'message_counts': (sum(s.get('ticks', 0) for s in resolved.get('steps', []))
                           == row['expected_warmup_messages'] + row['expected_measurement_messages']),
        'drain': (resolved.get('protocol_pilot_parameters') or {})
                 .get('drain_seconds_after_measurement') == shared['maximum_receipt_drain_seconds'],
        'quiet_late': (resolved.get('protocol_pilot_parameters') or {}).get('quiet_interval_seconds') == shared['quiet_interval_seconds']
                      and (resolved.get('protocol_pilot_parameters') or {}).get('late_observation_seconds') == shared['late_observation_seconds'],
    }
    failed = [k for k, ok in checks.items() if not ok]
    if failed:
        raise SystemExit(f'resolved qualification does not match the {workload} gate shape: ' + ', '.join(failed))
    return {'run_id': row['run_id'], 'kind': 'qualification', 'platform': platform,
            'workload_id': workload,
            'purpose': row['purpose'], 'rate': rate, 'devices': variant['devices'],
            'replicas': variant['replicas_per_replicated_service'],
            'warmup_messages': row['expected_warmup_messages'],
            'measurement_messages': row['expected_measurement_messages'],
            'counts_toward_proposed_attempts': False}


def find_row(run_id, rows=None):
    rows = rows if rows is not None else matrix_runs()
    for row in rows:
        if row['run_id'] == run_id:
            return row
    raise SystemExit(f'run ID is not in the W matrix: {run_id!r}')


def w_plan_path(row):
    return W_ROOT / row['plan']


def validate_plan_shape(row, resolved, scope):
    """Bind a resolved plan to its W variant and deployment intent.

    Mirrors the B0 equality discipline: every one-factor field, message count,
    seed and deployment intent is checked, so a W run cannot silently differ
    from the reviewed proposal.
    """
    variant = scope['variants'][row['workload_id']]
    platform = row['platform']
    plan = json.loads(w_plan_path(row).read_text())
    steps = [(s.get('phase'), s.get('rate'), s.get('duration_s'), s.get('vus'))
             for s in resolved.get('steps', [])]
    rate = variant['total_messages_per_second']
    pool = pool_policy.pool_for_rate(rate)
    policy = pool_policy.policy_for_rate(rate)
    expected_steps = [('warmup', rate, scope['shared']['warmup_seconds'], pool),
                      ('measurement', rate, scope['shared']['measurement_seconds'], pool)]
    expected_seed = scope['seed_by_repetition'][row['repetition'] - 1]
    drain = scope['shared']['maximum_receipt_drain_seconds']
    quiet = scope['shared']['quiet_interval_seconds']
    late = scope['shared']['late_observation_seconds']
    checks = {
        'adapter': resolved.get('adapter') == platform == plan.get('adapter'),
        'devices': resolved.get('devices') == variant['devices'] == plan.get('devices'),
        'rate_and_steps': steps == expected_steps,
        'seed': resolved.get('seed') == expected_seed == plan.get('seed'),
        'padding': resolved.get('payload', {}).get('size_offset_bytes')
                   == variant['extra_logical_padding_bytes'],
        'policy': resolved.get('health', {}).get('workload_policy') == policy,
        'vus': (resolved.get('vus') or {}).get('pre_allocated') == pool
               and (resolved.get('vus') or {}).get('max') == pool,
        'no_faults': not resolved.get('test_faults'),
        'drain': (resolved.get('protocol_pilot_parameters') or {})
                 .get('drain_seconds_after_measurement') == drain,
        'quiet_late': (resolved.get('protocol_pilot_parameters') or {}).get('quiet_interval_seconds') == quiet
                      and (resolved.get('protocol_pilot_parameters') or {}).get('late_observation_seconds') == late,
        'message_counts': (row['expected_warmup_messages'] == rate * scope['shared']['warmup_seconds']
                           and row['expected_measurement_messages'] == rate * scope['shared']['measurement_seconds']),
    }
    failed = [k for k, ok in checks.items() if not ok]
    if failed:
        raise SystemExit('resolved W plan does not match the reviewed variant: ' + ', '.join(failed))
    return {
        'run_id': row['run_id'], 'workload_id': row['workload_id'], 'platform': platform,
        'repetition': row['repetition'], 'seed': expected_seed,
        'warmup_messages': row['expected_warmup_messages'],
        'measurement_messages': row['expected_measurement_messages'],
        'rate': rate, 'devices': variant['devices'],
        'padding': variant['extra_logical_padding_bytes'],
        'replicas': variant['replicas_per_replicated_service'],
    }


class WBound:
    """Bind the reused B0 machinery to W's plan resolution and evidence roots."""

    def __init__(self, b0):
        self.b0 = b0
        self.scope = load_scope()
        self.rows = matrix_runs()
        self.quals = qualification_rows()

    def _plan_for(self, platform, run_id):
        """Resolve either a proposed attempt (matrix) or a qualification (index).

        Qualification IDs are deliberately resolved from their own index so a W
        attempt ID and a qualification ID can never be substituted for one
        another by a typo.
        """
        for row in self.rows:
            if row['run_id'] == run_id:
                if row['platform'] != platform:
                    raise SystemExit(f'run ID {run_id} belongs to {row["platform"]}, not {platform}')
                return w_plan_path(row), ('attempt', row)
        for row in self.quals:
            if row['run_id'] == run_id:
                if row['platform'] != platform:
                    raise SystemExit(f'run ID {run_id} belongs to {row["platform"]}, not {platform}')
                return qual_root_for(row) / row['plan'], ('qualification', row)
        raise SystemExit(f'run ID is neither in the W matrix nor the W qualification index: {run_id!r}')

    def __enter__(self):
        b0 = self.b0
        self.saved = {
            'REMOTE_RESULTS': b0.REMOTE_RESULTS, 'REMOTE_SOURCE': b0.REMOTE_SOURCE,
            'REMOTE_RUNS': b0.REMOTE_RUNS, 'EVIDENCE': b0.EVIDENCE,
            'frozen_plan_path': b0.frozen_plan_path, 'validate_cell_plan': b0.validate_cell_plan,
            'wait_tb_backends': b0.wait_tb_backends,
        }
        b0.REMOTE_RESULTS = W_REMOTE_RESULTS
        b0.REMOTE_SOURCE = W_REMOTE_RESULTS + '/source'
        b0.REMOTE_RUNS = W_REMOTE_RESULTS + '/runs'
        b0.EVIDENCE = W_EVIDENCE

        def frozen_plan_path(platform, run_id):
            return self._plan_for(platform, run_id)[0]

        # The W batch reaches the load host through an SSH jump, where a single
        # poll can transiently exceed its socket timeout. That is a transport
        # hiccup, not platform unreadiness, and aborting on it wastes an attempt
        # (observed 2026-11-10: TB was healthy with login=200 while the poll
        # raised TimeoutExpired). Retry the poll instead, bounded by the same
        # deadline, so a genuinely unready platform still fails closed.
        def wait_tb_backends(timeout=300):
            import subprocess
            import time as _time
            deadline = _time.monotonic() + timeout
            last = None
            while _time.monotonic() < deadline:
                try:
                    p = b0.ssh(b0.SUT, 'curl -fsS --max-time 5 http://127.0.0.1:8404/metrics',
                               timeout=30, check=False)
                except subprocess.TimeoutExpired:
                    last = 'ssh poll timed out'
                    _time.sleep(2)
                    continue
                if p.returncode == 0 and b0.tb_backends_ready(p.stdout):
                    return p.stdout
                last = 'backends not all UP'
                _time.sleep(2)
            raise RuntimeError('TB pretraffic readiness timed out: both N=2 HAProxy backends '
                               f'must be UP within {timeout}s (last: {last})')

        b0.wait_tb_backends = wait_tb_backends

        def validate_cell_plan(platform, run_id, plan_path, resolved):
            expected, (kind, row) = self._plan_for(platform, run_id)
            if Path(plan_path).resolve() != Path(expected).resolve():
                raise SystemExit(f'launched plan is not the reviewed {kind} plan')
            if kind == 'qualification':
                return validate_qualification_shape(row, resolved, self.scope)
            return validate_plan_shape(row, resolved, self.scope)

        b0.frozen_plan_path = frozen_plan_path
        b0.validate_cell_plan = validate_cell_plan
        return self

    def __exit__(self, *exc):
        for name, value in self.saved.items():
            setattr(self.b0, name, value)
        return False


def cmd_check(b0, platform=None, run_id=None):
    """Validate scope/matrix/plans offline. No server contact."""
    scope = load_scope()
    rows = matrix_runs()
    out = {'execution_authorized': scope['execution_authorized'],
           'matrix_status': load_matrix()['status'],
           'rows': len(rows), 'problems': []}
    seen = set()
    for row in rows:
        rid = row['run_id']
        if rid in seen:
            out['problems'].append(f'duplicate run ID: {rid}')
        seen.add(rid)
        plan = w_plan_path(row)
        if not plan.is_file():
            out['problems'].append(f'missing plan: {rid}')
            continue
        import hashlib
        digest = hashlib.sha256(plan.read_bytes()).hexdigest()
        if row.get('plan_sha256') and row['plan_sha256'] != digest:
            out['problems'].append(f'plan hash mismatch: {rid}')
        variant = scope['variants'][row['workload_id']]
        rate = variant['total_messages_per_second']
        if row['expected_warmup_messages'] != rate * scope['shared']['warmup_seconds']:
            out['problems'].append(f'warmup message count: {rid}')
        if row['expected_measurement_messages'] != rate * scope['shared']['measurement_seconds']:
            out['problems'].append(f'measurement message count: {rid}')
        if row['platform'] not in SUPPORTED:
            out['problems'].append(f'unknown platform: {rid}')
        body = json.loads(plan.read_text())
        if body.get('devices') != variant['devices']:
            out['problems'].append(f'plan devices: {rid}')
        if body.get('payload', {}).get('size_offset_bytes') != variant['extra_logical_padding_bytes']:
            out['problems'].append(f'plan padding: {rid}')
        for step in body.get('schedule', {}).get('steps', []):
            if step.get('rate') != rate:
                out['problems'].append(f'plan rate: {rid}')
                break
    # Qualification plans are separate IDs that never count toward the 60, but a
    # broken one is worse than a missing one because it would "clear" W1 falsely.
    quals = qualification_rows() if qualification_index_paths() else []
    out['qualifications'] = len(quals)
    for row in quals:
        rid = row['run_id']
        if rid in seen:
            out['problems'].append(f'run ID collides with the W matrix: {rid}')
        seen.add(rid)
        plan = qual_root_for(row) / row['plan']
        if not plan.is_file():
            out['problems'].append(f'missing qualification plan: {rid}')
            continue
        body = json.loads(plan.read_text())
        steps = body.get('schedule', {}).get('steps', [])
        workload = row.get('workload_id', 'W1')
        variant = scope['variants'][workload]
        rate = variant['total_messages_per_second']
        ticks = sum(s.get('rate', 0) * s.get('duration_s', 0) for s in steps)
        # Preserved generations are retained as history, not as launch candidates.
        # They must still be *readable*, but only a launched-status row is held to
        # the live identity/seed gate; a consumed row is evidence, not a plan.
        live = row.get('execution_status', 'NOT_LAUNCH_CLEARED') == 'NOT_LAUNCH_CLEARED'
        expected_ticks = row.get('expected_warmup_messages', 0) + row.get('expected_measurement_messages', 0)
        if live:
            if not expected_ticks or ticks != expected_ticks:
                out['problems'].append(f'qualification identity count: {rid}')
            if body.get('seed') != row.get('seed'):
                out['problems'].append(f'qualification seed: {rid}')
            if body.get('devices') != variant['devices']:
                out['problems'].append(f'qualification devices: {rid} '
                                       f'(want {variant["devices"]})')
            # A live qualification must use the current rate-derived pool; a
            # consumed generation is evidence and keeps whatever pool it ran at.
            try:
                want_pool = pool_policy.pool_for_rate(rate)
            except ValueError as exc:
                out['problems'].append(f'qualification rate/pool: {rid} ({exc})')
                continue
            if body.get('vus', {}).get('max') != want_pool:
                out['problems'].append(f'qualification VU pool: {rid} (want {want_pool})')
        for step in steps:
            if step.get('rate') != rate:
                out['problems'].append(f'qualification rate: {rid}')
                break
    if platform:
        rows = [r for r in rows if r['platform'] == platform]
        out['by_platform'] = {}
        for wid in ('W1', 'W2', 'W3', 'W4'):
            out['by_platform'][wid] = sorted(r['run_id'] for r in rows if r['workload_id'] == wid)
        out['qualifications_by_platform'] = sorted(r['run_id'] for r in quals if r['platform'] == platform)
    if run_id:
        kind, row = locate(run_id)
        out['row'] = row
        out['run_source'] = kind
        out['plan'] = str(run_plan(kind, row))
    return out


def locate(run_id):
    """Find a run ID in the W matrix or the qualification index. Never both.

    A consumed ID is preserved as evidence but is never launchable again: reusing
    it would silently overwrite the record of what happened.
    """
    hits = [('attempt', r) for r in matrix_runs() if r['run_id'] == run_id]
    if qualification_index_paths():
        hits += [('qualification', r) for r in qualification_rows() if r['run_id'] == run_id]
    if not hits:
        raise SystemExit(f'run ID is neither in the W matrix nor the W qualification index: {run_id!r}')
    if len(hits) > 1:
        raise SystemExit(f'run ID appears in more than one W source: {run_id!r}')
    kind, row = hits[0]
    # Only qualifications carry a consumable lifecycle here. Proposed attempts use
    # their own release vocabulary (BLOCKED_PENDING_VARIANT_RELEASE) and are gated
    # by the matrix, not by this check.
    if kind == 'qualification':
        status = row.get('execution_status', 'NOT_LAUNCH_CLEARED')
        if status != 'NOT_LAUNCH_CLEARED':
            successor = row.get('superseded_by')
            guidance = (f'use its successor {successor!r}' if successor
                        else 'it was not superseded (its platform was outside the later '
                             'generation); stage a new generation for this platform')
            raise SystemExit(f'run ID {run_id!r} is already consumed ({status}); {guidance}')
    return kind, row


def run_plan(kind, row):
    return (qual_root_for(row) / row['plan']) if kind == 'qualification' else w_plan_path(row)


def cmd_resolve(b0, run_id):
    """Resolve a plan locally against the prebuilt benchctl, if present."""
    kind, row = locate(run_id)
    plan = run_plan(kind, row)
    prebuilt = b0.validate_prebuilt_tools()
    if not prebuilt:
        raise SystemExit('no prebuilt workspace tools: set IOT_BENCH_PREBUILT_DIR to a directory '
                         'with benchctl, sink and build-provenance.json')
    import subprocess
    source, _ = prebuilt
    resolved = subprocess.run([str(source / 'benchctl'), 'resolve', '-plan', str(plan),
                               '-run-id', run_id], capture_output=True, text=True, check=True)
    body = json.loads(resolved.stdout)
    if kind == 'qualification':
        return validate_qualification_shape(row, body, load_scope())
    return validate_plan_shape(row, body, load_scope())


def verify_receipt(receipt_id, run_id):
    """Fail closed unless a recorded receipt authorizes exactly this run ID.

    Authority is checked against a hash-bound artefact, not against an
    environment variable or a flag the launcher could set for itself.
    """
    path = W_ROOT / 'authorization' / f'{receipt_id}.json'
    if not path.is_file():
        raise SystemExit(f'authorization receipt not found: {path}')
    receipt = json.loads(path.read_text())
    if run_id not in receipt.get('authorized_run_ids', []):
        raise SystemExit(f'receipt {receipt_id} does not authorize {run_id!r}')
    expected = receipt.get('authorized_plan_sha256', {}).get(run_id)
    if not expected:
        raise SystemExit(f'receipt {receipt_id} has no plan binding for {run_id!r}')
    import hashlib
    kind, row = locate(run_id)
    actual = hashlib.sha256(run_plan(kind, row).read_bytes()).hexdigest()
    if actual != expected:
        raise SystemExit(f'plan for {run_id} changed since authorization; obtain a new receipt')
    # Bind the executing tree too: an approval for one launcher is not an approval for another.
    bound = receipt.get('bindings_sha256', {})
    for name, rel in (('launcher', 'scripts/run-workload-campaign.py'),
                      ('integrity', 'scripts/dispatch-integrity.py'),
                      ('scope', 'workloads/campaigns/fixed-points-w1-w4/scope.json')):
        current = hashlib.sha256((REPO / rel).read_bytes()).hexdigest()
        if bound.get(name) != current:
            raise SystemExit(f'{name} changed since authorization ({rel}); obtain a new receipt')
    return receipt


def cmd_switch(b0, receipt_id, execute=False, timeout=900):
    """Stop the other owned platform stacks so `isolated_one_platform_at_a_time` holds.

    Volume-preserving: only `compose stop` on the four named owned projects, never
    down/rm/prune. This is the documented reviewed switch, exposed here so the W
    flow does not have to shell out to the B0 launcher directly.
    """
    if not execute:
        raise SystemExit('refusing to mutate stacks without --execute-reviewed')
    if not receipt_id:
        raise SystemExit('refusing to switch without --receipt ID')
    verify_receipt_for_switch(b0, receipt_id)
    b0.owned_workload_check()
    with WBound(b0):
        b0.stop_owned_stacks()
    return {'action': 'switch', 'stopped': ['tbq20261105', 'fiware', 'fiwhc', 'edgex'],
            'volumes_removed': False}


def verify_receipt_for_switch(b0, receipt_id):
    """A switch is a mutation, so it needs the same hash-bound receipt.

    It is not bound to a single run ID (it stops other stacks), so it is checked
    against the receipt's tree bindings only.
    """
    import hashlib
    path = W_ROOT / 'authorization' / f'{receipt_id}.json'
    if not path.is_file():
        raise SystemExit(f'authorization receipt not found: {path}')
    receipt = json.loads(path.read_text())
    bound = receipt.get('bindings_sha256', {})
    for name, rel in (('launcher', 'scripts/run-workload-campaign.py'),
                      ('integrity', 'scripts/dispatch-integrity.py'),
                      ('scope', 'workloads/campaigns/fixed-points-w1-w4/scope.json')):
        current = hashlib.sha256((REPO / rel).read_bytes()).hexdigest()
        if bound.get(name) != current:
            raise SystemExit(f'{name} changed since authorization ({rel}); obtain a new receipt')
    return receipt


def cmd_launch(b0, platform, run_id, sol_review=None, execute=False, timeout=5400):
    kind, row = locate(run_id)
    if row['platform'] != platform:
        raise SystemExit(f'{run_id} is a {row["platform"]} cell, not {platform}')
    if not execute:
        raise SystemExit('refusing to launch without --execute-reviewed and an owner-authorization receipt')
    if not sol_review:
        raise SystemExit('refusing to launch without --receipt ID')
    receipt = verify_receipt(sol_review, run_id)
    out = W_EVIDENCE / run_id
    with WBound(b0):
        rc = b0.launch(platform, run_id, run_plan(kind, row), '', timeout)
    return {'run_id': run_id, 'kind': kind, 'exit': rc, 'evidence': str(out),
            'receipt_id': receipt['receipt_id']}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('action', choices=('check', 'resolve', 'switch', 'launch'))
    ap.add_argument('--platform', choices=SUPPORTED)
    ap.add_argument('--run-id')
    ap.add_argument('--receipt', dest='sol_review', help='owner-authorization receipt ID recorded by scripts/record-w-authorization.py')
    ap.add_argument('--execute-reviewed', action='store_true')
    ap.add_argument('--timeout', type=int, default=5400)
    ap.add_argument('--json', action='store_true')
    args = ap.parse_args()
    b0 = load_b0()
    if args.action == 'check':
        result = cmd_check(b0, args.platform, args.run_id)
        print(json.dumps(result, indent=2) if args.json else
              f"rows={result['rows']} problems={len(result['problems'])} "
              f"authorized={result['execution_authorized']}")
        return 1 if result['problems'] else 0
    if args.action == 'resolve':
        if not args.run_id:
            raise SystemExit('resolve requires --run-id')
        print(json.dumps(cmd_resolve(b0, args.run_id), indent=2))
        return 0
    if args.action == 'switch':
        result = cmd_switch(b0, args.sol_review, args.execute_reviewed, args.timeout)
        print(json.dumps(result, indent=2))
        return 0
    if args.action == 'launch':
        if not (args.platform and args.run_id):
            raise SystemExit('launch requires --platform and --run-id')
        result = cmd_launch(b0, args.platform, args.run_id, args.sol_review,
                            args.execute_reviewed, args.timeout)
        print(json.dumps(result, indent=2))
        return result['exit']
    return 2


if __name__ == '__main__':
    sys.exit(main())
