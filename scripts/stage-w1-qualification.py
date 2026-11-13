#!/usr/bin/env python3
"""Stage W release-qualification plans for a variant. Never launches traffic.

Covers W1 (50/s full-duration) and W2 (2,000-device provisioning). The same code
path is used for both so a qualification cannot drift from the attempt it is
qualifying; only the variant and the canonical attempt plan differ.

W1's gate is throughput/duration. W2's gate is provisioning: 2,000 devices must
be created and every active replica must recognize the entire intended cohort
before traffic. A W2 qualification therefore reproduces W2's exact shape
(2,000 devices, 25/s, 300/600 s, the rate-derived pool, the same timeout) so it
can fail for the same reasons a real W2 attempt would.

The W1 release gate requires, verbatim:

    "Full-duration 50/s, 300/600-second generator/capture validation under
     actual adapter/recording/monitoring: 15000 warm-up and 30000 measurement
     identities."

    "Existing 50/s 30/120-second B0 qualification is not full-duration W1
     clearance."

The only B0 50/s evidence is `tb-dispatch-endpoint-qual-20261106` at 30 s / 120 s
(1,500 + 6,000 identities). That is a fifth of the W1 duration and cannot be
extended by inspection, so W1 needs a qualification run of its own shape.

These plans deliberately reproduce the W1 attempt *exactly* except for identity:
same adapter, same 1,000 devices, same 50/s, same 300 s / 600 s schedule, the
same rate-derived VU pool, the same 5,000 ms timeout and the same
dispatch-integrity policy. That is the point of a qualification: it must be able
to fail for the reasons a real W1 attempt would fail.

The VU pool is the load-bearing choice, and it changed on 2026-11-10. Two v1
qualifications (`w1-qual-tb-20261110c`, `w1-qual-fw-20261110c`) ran at the old
fixed 150 pool and both returned `INVALID_VU_POOL_BOUND`. That was the harness
starving itself, not the platform failing: `rate x request_timeout` bounds
worst-case in-flight at 50 x 5 = 250, so a 150 pool drops by construction. The
owner amendment replaced the fixed pool with a rate-derived one
(`scripts/pool_policy.py`), which gives 300 at 50/s. Those two negatives are
preserved and are NOT retried; a v2 generation is staged under new IDs.

Run IDs are namespaced `w1-qual-*` so that:
  * they never collide with the 60 proposed attempt IDs;
  * device identities (`bench-<run_id>-<NNNNN>`) form a separate cohort;
  * evidence lands in its own root.

Per `scope.json` `qualification_policy`, no qualification counts toward the 60.
"""
import argparse
import hashlib
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import pool_policy  # noqa: E402  (single source of truth for the VU pool)

REPO = Path(__file__).resolve().parents[1]
W_ROOT = REPO / 'workloads/campaigns/fixed-points-20261108'
OUT = W_ROOT / 'qualification'

# Fixed identity for the qualification cohort. Distinct from every
# seed_by_repetition value (202611061..202611065) so the payload stream is its
# own realization and is not mistaken for repetition 1.
QUAL_SEED = 202611060

PREFIX = {'thingsboard': 'tb', 'fiware': 'fw', 'edgex': 'ex'}


def encoded(value):
    return (json.dumps(value, indent=2) + '\n').encode()


def canonical_plan(platform, workload='W1'):
    """The reviewed attempt plan for this platform+workload, used as the shape source.

    Reading the target rather than restating it means the qualification cannot
    silently point somewhere the real attempt would not.
    """
    stem = f'{workload.lower()}dp-{PREFIX[platform]}-r01-20261108.json'
    return json.loads((W_ROOT / 'plans' / stem).read_text())


# Retained under the W1 name: the W1 tests and the shape-source semantics both
# refer to it, and renaming it would silently change what "canonical" means.
def canonical_w1_plan(platform):
    return canonical_plan(platform, 'W1')


def plan_for(platform, variant, scope, canonical, suffix='', workload='W1'):
    shared = scope['shared']
    wid = workload.lower()
    rid = f'{wid}-qual-{PREFIX[platform]}-20261110{suffix}'
    rate = variant['total_messages_per_second']
    # Rate-derived pool (2026-11-10 owner amendment): 300 at 50/s, not the
    # historical fixed 150 that provably starved the open-loop executor. At
    # 25/s the rule returns exactly 150, so W2 stays pool-identical to B0.
    pool = pool_policy.pool_for_rate(rate)
    policy = pool_policy.policy_for_rate(rate)
    gate_label = ('W1 release qualification: full-duration 50/s generator/capture '
                  'validation at the exact W1 shape' if workload == 'W1' else
                  f'{workload} release qualification: {variant["devices"]}-device provisioning and '
                  f'full-duration {rate}/s generator/capture validation at the exact {workload} shape')
    plan = {
        'schema_version': 1,
        'plan_id': f'{wid}-qualification-{rate}s-{platform}-20261110{suffix}',
        'purpose': (gate_label + '. Separate ID; not one of the 60 '
                    'proposed attempts. NOT a fixed-point result and never reported as one.'),
        'pilot_only': True,
        'seed': QUAL_SEED,
        'adapter': platform,
        'payload': {'encoding_id': 'logical-json-ascii-v1',
                    'size_offset_bytes': variant['extra_logical_padding_bytes']},
        'target': canonical['target'],
        'devices': variant['devices'],
        'request_timeout_ms': shared['request_timeout_ms'],
        'connection': {'keep_alive': shared['keep_alive']},
        'arrivals': {'model': 'constant'},
        'vus': {'pre_allocated': pool, 'max': pool},
        'schedule': {'type': 'steps', 'steps': [
            {'phase': 'warmup', 'rate': rate,
             'duration_s': shared['warmup_seconds'], 'vus': pool},
            {'phase': 'measurement', 'rate': rate,
             'duration_s': shared['measurement_seconds'], 'vus': pool},
        ]},
        'health': {'workload_policy': policy,
                   'min_achieved_ratio': 0.99, 'send_lag_p99_ms': 0,
                   'early_tolerance_ms': 2, 'max_core_busy': 0},
        'cleanup': False,
        'test_faults': {},
        'protocol_pilot_parameters': {
            'rate_per_second': variant['total_messages_per_second'],
            'measurement_seconds': shared['measurement_seconds'],
            'warmup_seconds': shared['warmup_seconds'],
            'drain_seconds_after_measurement': shared['maximum_receipt_drain_seconds'],
            'quiet_interval_seconds': shared['quiet_interval_seconds'],
            'late_observation_seconds': shared['late_observation_seconds'],
        },
        'deviations': [
            'Qualification only: separate run ID and seed; never counted as a proposed W attempt.',
            f'Reproduces the {workload} attempt shape exactly (adapter, devices, rate, 300/600 s, '
            f'{pool}/{pool} VU pool, 5000 ms timeout).',
            f'VU pool is rate-derived under the 2026-11-10 owner amendment: {pool} VUs at '
            f'{rate}/s (rate x timeout = {rate * 5}, plus 10% headroom). The pool is generator '
            'concurrency, not a platform capacity claim; v1 qualifications at a fixed 150 are '
            'preserved negatives and are not retried.',
            'Existing deployed state, device cohorts and history are retained; not a cold-start repetition.',
        ],
    }
    return rid, plan


EVIDENCE_ROOT = REPO / 'results/workload-batch-20261108/evidence'


def evidence_status(run_id):
    """Derive a consumed row's status from evidence, or None if it left none.

    A row must never claim an outcome that its own evidence directory does not
    record. Both record shapes are read because a qualification that reached
    traffic writes `qualification-result.json`, while one that stopped before
    traffic writes `recovery-result.json`.
    """
    run_dir = EVIDENCE_ROOT / run_id
    if not run_dir.is_dir():
        return None
    for name in ('qualification-result.json', 'recovery-result.json'):
        record = run_dir / name
        if record.is_file():
            body = json.loads(record.read_text())
            derived = {'execution_status': body.get('outcome', 'CONSUMED_UNSPECIFIED')}
            # Only assert an offered count when the record actually states one;
            # fabricating a null would show up as a permanent diff against the
            # generated row.
            if 'offered_workload_messages' in body:
                derived['offered_workload_messages'] = body['offered_workload_messages']
            return derived
    return {'execution_status': 'CONSUMED_UNCLASSIFIED_SEE_EVIDENCE',
            'offered_workload_messages': None}


def refresh_index(index):
    """Re-derive every row's status from evidence without rewriting any plan."""
    changed = []
    for row in index['runs']:
        derived = evidence_status(row['run_id'])
        if derived is None:
            continue
        for key, value in derived.items():
            if row.get(key) != value:
                changed.append((row['run_id'], key, row.get(key), value))
            row[key] = value
    return changed


def index_out(workload):
    """Where a workload's qualification ledger lives.

    W1 keeps the historical root path so its existing ledger stays byte-identical.
    A later workload gets its own subdirectory: sharing one index would force a
    single `variants[...]` lookup onto rows that ran different shapes.
    """
    return OUT if workload == 'W1' else OUT / workload.lower()


def gate_text(workload, variant):
    """The gate this qualification exists to clear, per variant.

    Each workload tests a different axis, so a shared string would misdescribe
    at least two of them. W1 is duration/throughput; W2 is cohort provisioning;
    W3 is payload bytes; W4 is replica count.
    """
    rate = variant['total_messages_per_second']
    if workload == 'W1':
        return (f'W1: full-duration {rate}/s, 300/600-second generator/capture validation')
    if workload == 'W2':
        return (f'W2: {variant["devices"]}-device provisioning + every replica recognizing '
                f'the full cohort, then {rate}/s for 300/600 s')
    if workload == 'W3':
        return (f'W3: exactly {variant["extra_logical_padding_bytes"]} extra ASCII logical padding '
                f'bytes via size_offset_bytes={variant["extra_logical_padding_bytes"]} on every '
                f'adapter, with live sink/history preservation and independent stored-payload '
                f'validation, then {rate}/s for 300/600 s')
    if workload == 'W4':
        return (f'W4: N={variant["replicas_per_replicated_service"]} replica '
                f'{"(the workload axis)" if variant["replicas_per_replicated_service"] == 1 else ""}'
                f' at {rate}/s for 300/600 s')
    raise ValueError(f'no gate text defined for {workload!r}')


def artifacts(suffix='', platforms=('thingsboard', 'fiware', 'edgex'), workload='W1'):
    scope = json.loads((W_ROOT / 'scope.json').read_text())
    variant = scope['variants'][workload]
    files, rows = {}, []
    for platform in platforms:
        rid, plan = plan_for(platform, variant, scope, canonical_plan(platform, workload),
                             suffix, workload)
        path = f'{rid}.json'
        files[path] = plan
        rows.append({
            'run_id': rid,
            'workload_id': workload,
            'platform': platform,
            'purpose': f'{workload.lower()}-release-qualification',
            'target': plan['target'],
            'gate': gate_text(workload, variant),
            'plan': path,
            'seed': plan['seed'],
            'expected_warmup_messages': plan['schedule']['steps'][0]['rate'] * plan['schedule']['steps'][0]['duration_s'],
            'expected_measurement_messages': plan['schedule']['steps'][1]['rate'] * plan['schedule']['steps'][1]['duration_s'],
            'counts_toward_proposed_attempts': False,
            'execution_status': 'NOT_LAUNCH_CLEARED',
            'invalid_if': 'dropped_iterations > 0 (VU pool binds) or identity/capture mismatch',
        })
    index = {
        'schema_version': 1,
        'batch_id': f'{workload.lower()}-qualification-20261110' + suffix,
        'workload_id': workload,
        'status': 'NOT_LAUNCH_CLEARED',
        'execution_authorized': False,
        'qualification_policy': scope['qualification_policy'],
        'why_needed': (('W1 gate requires full-duration 50/s 300/600 s validation; the only B0 '
                        '50/s evidence is 30/120 s (tb-dispatch-endpoint-qual-20261106) and does not transfer.')
                       if workload == 'W1' else
                       (f'W2 gate requires {variant["devices"]}-device provisioning, with every '
                        f'active replica recognizing the entire intended cohort before traffic, and '
                        f'{variant["total_messages_per_second"]}/s for the full 300/600 s window. No '
                        f'existing evidence covers a {variant["devices"]}-device cohort at this shape.')
                       if workload == 'W2' else
                       (f'W3 gate requires exactly {variant["extra_logical_padding_bytes"]} extra ASCII '
                        f'logical padding bytes on every adapter, with live sink/history preservation and '
                        f'an independent stored-payload check. The sink retains identity/timing only and '
                        f'FIWARE subscriptions select identity fields, so the payload-preserving evidence '
                        f'path must be shown to work rather than assumed.')
                       if workload == 'W3' else
                       (f'W4 gate requires the workload to run at '
                        f'N={variant["replicas_per_replicated_service"]} replica(s), which is a different '
                        f'deployment from B0 and every other W cell; no existing evidence covers it.')),
        'vu_headroom_note': (('The pool is rate-derived under the 2026-11-10 owner amendment. '
                              'At 50/s: 50 x 5000 ms = 250 worst-case in-flight, enlarged by 10% '
                              'headroom and rounded to 300 VUs. Two v1 qualifications at the old '
                              'fixed 150 both returned INVALID_VU_POOL_BOUND; those are preserved '
                              'negatives measuring generator starvation, not platform limits. This '
                              'generation tests the platform at a pool that can actually sustain the rate.')
                             if variant['total_messages_per_second'] != pool_policy.B0_RATE_PER_S else
                             (f'The pool is rate-derived. At {variant["total_messages_per_second"]}/s: '
                              f'{variant["total_messages_per_second"]} x 5000 ms = '
                              f'{variant["total_messages_per_second"] * 5} worst-case in-flight, plus 10% '
                              f'headroom, rounded to {pool_policy.pool_for_rate(variant["total_messages_per_second"])} '
                              'VUs. That is exactly the B0 value, so this cell stays pool-identical to B0.')),
        'runs': rows,
    }
    return files, index


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--out', type=Path, default=OUT)
    ap.add_argument('--check', action='store_true')
    ap.add_argument('--id-suffix', default='',
                    help='suffix for a superseding generation, e.g. "b" after a zero-traffic refusal')
    ap.add_argument('--supersede', action='store_true',
                    help='add a new generation beside an existing index, preserving consumed IDs')
    ap.add_argument('--refresh', action='store_true',
                    help='re-derive consumed statuses from evidence; never touches plan files')
    ap.add_argument('--platform', action='append', choices=('thingsboard','fiware','edgex'),
                    help='limit a superseding generation to these platforms (repeatable). '
                         'Other platforms carry forward untouched: a platform that already '
                         'qualified must not be marked superseded by a generation that does '
                         'not re-run it.')
    ap.add_argument('--workload', choices=('W1', 'W2', 'W3', 'W4'), default='W1',
                    help='variant to stage a qualification for. W1 keeps the historical index '
                         'root; other workloads get their own subdirectory.')
    args = ap.parse_args()
    if args.platform and not args.supersede:
        raise SystemExit('--platform is only meaningful with --supersede')
    platforms = tuple(args.platform) if args.platform else ('thingsboard', 'fiware', 'edgex')
    files, index = artifacts(args.id_suffix, platforms, args.workload)
    out = (args.out / args.workload.lower()).resolve() if args.workload != 'W1' else args.out.resolve()

    if args.refresh:
        index_path = out / 'index.json'
        if not index_path.is_file():
            raise SystemExit(f'cannot refresh without an existing index: {index_path}')
        current = json.loads(index_path.read_text())
        changed = refresh_index(current)
        index_path.write_text(json.dumps(current, indent=2) + '\n')
        if changed:
            for rid, key, old, new in changed:
                print(f'{rid}: {key} {old!r} -> {new!r}')
        else:
            print(f'{out}: no status changes; evidence and index agree')
        (out / 'SHA256SUMS').write_text(''.join(
            f'{hashlib.sha256((out / name).read_bytes()).hexdigest()}  {name}\n'
            for name in sorted(p.name for p in out.iterdir() if p.is_file() and p.name != 'SHA256SUMS')))
        return 0

    prior_rows = []
    if args.supersede:
        if not args.id_suffix:
            raise SystemExit('--supersede requires --id-suffix')
        prior_index_path = out / 'index.json'
        if not prior_index_path.is_file():
            raise SystemExit(f'cannot supersede without an existing index: {prior_index_path}')
        prior = json.loads(prior_index_path.read_text())
        consumed = {r['run_id']: r for r in prior['runs']}
        # Every earlier generation still on disk must stay in the index, even if a
        # middle generation was never recorded there (e.g. after a chained
        # supersede). Earlier plan files are authoritative evidence of existence.
        import re as _re
        for plan_file in sorted(out.glob(f'{args.workload.lower()}-qual-*-20261110*.json')):
            rid = plan_file.stem
            if rid in consumed or rid in {r['run_id'] for r in index['runs']}:
                continue
            platform = {'tb': 'thingsboard', 'fw': 'fiware', 'ex': 'edgex'}[rid.split('-')[2]]
            consumed[rid] = {'run_id': rid, 'platform': platform, 'plan': plan_file.name}
        # Preserve every earlier generation. Only an ID that actually left evidence
        # is "consumed"; an untouched sibling ID keeps its original status, because
        # claiming it was used when it was not would be a false record.
        evidence_root = REPO / 'results' / 'workload-batch-20261108' / 'evidence'
        for rid, row in consumed.items():
            if row['run_id'] in {r['run_id'] for r in index['runs']}:
                continue
            row = dict(row)
            if row.get('platform') not in platforms:
                # This generation does not re-test this platform, so its recorded
                # state is carried forward verbatim. Recomputing it from evidence
                # would relabel a never-run sibling as superseded by a generation
                # that never covered it.
                prior_rows.append(row)
                continue
            run_dir = evidence_root / rid
            # A run that reached traffic writes qualification-result.json; one that
            # stopped before traffic writes recovery-result.json. Both are records of
            # the outcome, so both must be read -- reading only one silently downgrades
            # a measured verdict to an unclassified label.
            recorded = None
            for record_name in ('qualification-result.json', 'recovery-result.json'):
                record = run_dir / record_name
                if record.is_file():
                    body = json.loads(record.read_text())
                    recorded = (body.get('outcome', 'CONSUMED_UNSPECIFIED'),
                                body.get('offered_workload_messages'))
                    break
            if recorded is not None:
                row['execution_status'] = recorded[0]
                if recorded[1] is not None:
                    row['offered_workload_messages'] = recorded[1]
            elif run_dir.exists():
                row['execution_status'] = 'CONSUMED_UNCLASSIFIED_SEE_EVIDENCE'
            else:
                row['execution_status'] = 'NOT_LAUNCH_CLEARED_SUPERSEDED_UNUSED'
            row['superseded_by'] = None  # filled below by platform
            prior_rows.append(row)
        for row in prior_rows:
            match = [r for r in index['runs'] if r['platform'] == row['platform']]
            if match and row['platform'] in platforms:
                row['superseded_by'] = match[0]['run_id']
            else:
                # A platform outside this generation is carried forward, not
                # superseded: claiming a valid result was replaced by a run that
                # never re-tested it would be a false record.
                row['superseded_by'] = None
        index['supersedes'] = sorted(rid for rid, row in consumed.items()
                                     if row.get('platform') in platforms)
        index['supersede_reason'] = (
            'Prior generation is preserved and not retried. The 2026-11-10 owner pool amendment '
            'raised the generator pool to a rate-derived value (300 VUs at 50/s); earlier '
            'generations ran at the old fixed 150 and returned INVALID_VU_POOL_BOUND, which '
            'measured generator starvation rather than a platform limit. Consumed IDs that '
            'produced evidence keep that evidence.'),

    files['index.json'] = index
    # Earlier-generation plan files stay exactly as they were; only the index grows.
    for rid, row in ((r['run_id'], r) for r in prior_rows):
        prior_plan = out / row['plan']
        if not prior_plan.is_file():
            raise SystemExit(f'preserved generation plan missing: {prior_plan}')
        files[row['plan']] = json.loads(prior_plan.read_text())
    index['runs'] = index['runs'] + prior_rows if prior_rows else index['runs']

    if args.check:
        # The index is a growing ledger, not a regenerable artifact: it retains
        # prior generations, so this generator cannot reproduce it byte-for-byte.
        # Check the invariants that actually matter instead.
        problems = []
        on_disk_path = out / 'index.json'
        if not on_disk_path.is_file():
            raise SystemExit(f'missing: {out / "index.json"}')
        on_disk = json.loads(on_disk_path.read_text())
        disk_rows = {r['run_id']: r for r in on_disk['runs']}

        # 1. Every plan the ledger references must still exist. A *live* plan must
        #    also match the canonical attempt shape byte-for-byte; a preserved
        #    generation is historical evidence and keeps the shape it actually ran
        #    at, so it is only required to be readable and self-consistent.
        live_status = 'NOT_LAUNCH_CLEARED'
        scope_body = json.loads((W_ROOT / 'scope.json').read_text())

        def canonical_for(rid_value, platform_value, workload_value):
            """Canonical plan bytes for a live ID, honouring its generation suffix."""
            marker = '-20261110'
            if marker not in rid_value:
                return None
            suffix = rid_value.split(marker, 1)[1]
            try:
                _, plan = plan_for(platform_value, scope_body['variants'][workload_value],
                                   scope_body, canonical_plan(platform_value, workload_value),
                                   suffix, workload_value)
            except Exception:
                return None
            return encoded(plan)

        for rid, row in disk_rows.items():
            workload_value = row.get('workload_id', args.workload)
            plan = out / row.get('plan', '')
            if not row.get('plan') or not plan.is_file():
                problems.append(f'missing plan for {rid}: {row.get("plan")!r}')
                continue
            if row.get('execution_status') == live_status:
                wanted = canonical_for(rid, row.get('platform'), workload_value)
                if wanted is None:
                    problems.append(f'live plan has no canonical generation: {row["plan"]}')
                    continue
                if plan.read_bytes() != wanted:
                    problems.append(f'live plan drifted from canonical {workload_value} shape: {row["plan"]}')
                continue
            # Even a preserved plan must declare the pool and policy its row claims.
            body = json.loads(plan.read_text())
            rate = body.get('schedule', {}).get('steps', [{}])[0].get('rate')
            pool = (body.get('vus') or {}).get('max')
            if not isinstance(rate, int) or not isinstance(pool, int):
                problems.append(f'plan missing rate/pool: {row["plan"]}')
            elif row.get('execution_status') == live_status:
                try:
                    want_pool = pool_policy.pool_for_rate(rate)
                except ValueError as exc:
                    problems.append(f'live plan has unqualified rate: {row["plan"]} ({exc})')
                    continue
                if pool != want_pool:
                    problems.append(f'live plan pool {pool} != rate-derived {want_pool}: {row["plan"]}')

        # 2. The ledger's statuses must agree with the evidence on disk. A row that
        #    has a result record but still claims NOT_LAUNCH_CLEARED would invite a
        #    relaunch of a consumed ID, which is never permitted.
        for rid, row in disk_rows.items():
            derived = evidence_status(rid)
            if derived is None:
                continue
            recorded = derived['execution_status']
            if row.get('execution_status') != recorded:
                problems.append(f'status disagrees with evidence: {rid} '
                                f'(index={row.get("execution_status")}, evidence={recorded})')

        # 3. Any consumed row that produced evidence must keep it. Siblings that
        #    were superseded without ever being used legitimately have none.
        for rid in on_disk.get('supersedes') or []:
            row = disk_rows.get(rid, {})
            if row.get('execution_status') == 'NOT_LAUNCH_CLEARED_SUPERSEDED_UNUSED':
                continue
            if not (EVIDENCE_ROOT / rid).is_dir():
                problems.append(f'consumed row lost its evidence directory: {rid}')
        if problems:
            raise SystemExit('qualification proposal check failed:\n  ' + '\n  '.join(problems))
        print(f'{out}: {len(index["runs"])} qualification plans verified; not launch-cleared')
        return 0
    if out.exists() and any(out.iterdir()) and not args.supersede:
        raise SystemExit(f'refusing to overwrite existing qualification plans: {out}')
    out.mkdir(parents=True, exist_ok=True)
    digests = {}
    for name, body in files.items():
        target = out / name
        if args.supersede and target.is_file() and name != 'index.json':
            continue  # never rewrite a preserved plan
        target.write_bytes(encoded(body))
        digests[name] = hashlib.sha256(encoded(body)).hexdigest()
    for name in files:
        if name not in digests:
            digests[name] = hashlib.sha256((out / name).read_bytes()).hexdigest()
    (out / 'SHA256SUMS').write_text(''.join(f'{v}  {k}\n' for k, v in sorted(digests.items())))
    print(f'{out}: {len(index["runs"])} qualification plans staged; not launch-cleared')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
