#!/usr/bin/env python3
"""Stage W1-W4 source plans only. Never deploy, run traffic, or edit frozen B0."""
import argparse
import copy
import hashlib
import json
from pathlib import Path
import subprocess

REPO = Path(__file__).resolve().parents[1]
B0 = REPO / 'results/feasible-campaign-20261106'
DEFAULT_OUT = REPO / 'workloads/campaigns/fixed-points-w1-w4'
PLATFORMS = ('thingsboard', 'fiware', 'edgex')
PREFIX = dict(thingsboard='tb', fiware='fw', edgex='ex')
BASELINE = dict(devices=1000, total_messages_per_second=25,
                extra_logical_padding_bytes=0, replicas_per_replicated_service=2)
CHANGES = {'W1': ('total_messages_per_second', 50), 'W2': ('devices', 2000),
           'W3': ('extra_logical_padding_bytes', 1024), 'W4': ('replicas_per_replicated_service', 1)}
SERVICES = {'thingsboard': ['tb-core', 'tb-rule-engine', 'tb-http-transport'],
            'fiware': ['orion', 'iot-agent'], 'edgex': ['core-data', 'device-rest']}
REPETITIONS = 5  # Proposed, not represented as separately launch-approved.

import sys as _sys  # noqa: E402
_sys.path.insert(0, str(Path(__file__).resolve().parent))
import pool_policy  # noqa: E402  (single source of truth for the VU pool)


def encoded(value):
    return (json.dumps(value, indent=2)+'\n').encode()


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def baseline_hashes():
    paths = [B0/'frozen-scope.json', B0/'run-matrix.json']
    paths.extend(sorted((B0/'plans').glob('disp-*.json')))
    return {str(p.relative_to(REPO)): sha(p) for p in paths}


def artifacts():
    """Deterministic proposed controls; generator plans remain strict Go schema."""
    original_scope = json.loads((B0/'frozen-scope.json').read_text())
    seeds = original_scope['seed_by_repetition']
    variants = {key: dict(BASELINE, **{field: value}) for key, (field, value) in CHANGES.items()}
    scope = {
        'schema_version': 1, 'batch_id': 'fixed-points-w1-w4-20261108',
        'design_status': 'OWNER_APPROVED_CONFIGURATIONS_REPETITION_COUNT_PROPOSED',
        'execution_authorized': False, 'baseline_workload_id': 'B0',
        'baseline_files_sha256': baseline_hashes(), 'baseline_configuration': BASELINE,
        'variants': variants, 'proposed_attempts_per_workload_platform': REPETITIONS,
        'experimental_attempts_proposed': 60, 'seed_by_repetition': seeds,
        'shared': dict({k:v for k, v in original_scope['shared_configuration'].items()
                        if k not in ('warmup_rate_per_second', 'vus_preallocated_per_scenario',
                                     'vus_max_per_scenario')},
                       application_cpu_pool='4-5', application_cpu_budget_k=2,
                       warmup_seconds=300, measurement_seconds=600,
                       device_selection='round-robin seq modulo devices',
                       same_haproxy_ingress=True, preserve_devices_and_history=True,
                       warmup_rate_rule='same as each workload total_messages_per_second',
                       # Owner amendment 2026-11-10. Replaces the fixed 150/150 with a
                       # rule; at 25/s it still evaluates to 150 so B0 is unchanged.
                       generator_pool_policy='rate-derived: ceil(rate_s * 5000ms/1000 * 1.10 / 50) * 50',
                       generator_pool_headroom_factor=pool_policy.HEADROOM,
                       generator_pool_quantum=pool_policy.QUANTUM,
                       generator_pool_by_rate={str(r): p for r, p in sorted(pool_policy.FROZEN_POOLS.items())},
                       generator_pool_rationale=(
                           'The VU pool is generator-side concurrency, not a platform capacity claim. '
                           'A pool smaller than rate x timeout starves the open-loop executor and is '
                           'reported as dropped_iterations, which is indistinguishable from platform loss. '
                           'The original fixed 150 satisfied this only at 25/s; at 50/s it demanded 250. '
                           '10% headroom is evidence-based: B0 25/s cells peaked at 126 against a theoretical 125.'),
                       generator_pool_comparability=(
                           'At 25/s the rule yields exactly the B0 value (150), so every W2/W3/W4 cell and '
                           'every B0 cell remains pool-identical. Only W1 (50/s, 300) differs from B0, and '
                           'W1 differs from B0 in arrival rate anyway, so no fixed-point comparison is broken.')),
        'endpoint': 'application_dispatch_to_first_complete_body_sink_receipt',
        'latency_method': 'offline per-message first-receipt join; nearest-rank conditional percentiles plus missing denominators',
        'result_grouping': ['workload_id', 'platform'],
        'attempt_policy': 'Five proposed attempts, not five guaranteed successes. Preserve all IDs, failures and in-progress attempts; no automatic retries or replacement.',
        'qualification_policy': 'Separate IDs/evidence for diagnostics and qualifications; no such runs count toward the proposed 60 experimental attempts.',
        'claim_scope': 'Fixed-point comparisons only; no maximum capacity, full scaling curve or cold-start IID claim.',
        'edgex_inventory_budget': {
            'new_experimental_cohort_devices': sum(v['devices']*REPETITIONS for v in variants.values()),
            'projected_retained_after_original_b0': 11192,
            'projected_total_before_new_qualifications': 36192,
            'previous_discovery_limit': 16384,
            'status': 'BLOCKED_PENDING_LIVE_INVENTORY_AND_COMMON_DISCOVERY_CONFIGURATION_REVIEW',
            'note': 'Projection, not a live inventory observation. Include all new diagnostic/qualification cohorts and margin; no deletion. Do not change active B0 or silently raise the limit for W2 alone.'}}
    rows = []
    files = {'scope.json': scope}
    for rep in range(1, REPETITIONS+1):
        workload_order = list(CHANGES)
        workload_order = workload_order[rep-1:] + workload_order[:rep-1] if rep <= 4 else workload_order
        for wid in workload_order:
            variant = variants[wid]
            rotation = (rep-1 + list(CHANGES).index(wid)) % len(PLATFORMS)
            order = PLATFORMS[rotation:] + PLATFORMS[:rotation]
            for platform in order:
                baseline = json.loads((B0/'plans'/f'disp-{PREFIX[platform]}-r{rep:02d}-20261106.json').read_text())
                plan = copy.deepcopy(baseline)
                rid = f'{wid.lower()}dp-{PREFIX[platform]}-r{rep:02d}-20261108'
                plan.update(plan_id=f'fixed-{wid.lower()}-{platform}-r{rep:02d}-20261108',
                            purpose=f'{wid} fixed-point experimental attempt proposal; separate from B0, diagnostics and qualification. NOT launch-cleared.',
                            devices=variant['devices'])
                plan['payload']['size_offset_bytes'] = variant['extra_logical_padding_bytes']
                for step in plan['schedule']['steps']:
                    step['rate'] = variant['total_messages_per_second']
                plan['protocol_pilot_parameters']['rate_per_second'] = variant['total_messages_per_second']
                # Owner amendment 2026-11-10: the generator pool is rate-derived,
                # not a fixed 150. At 25/s this yields 150 (B0-identical); at 50/s
                # it yields 300, which is what W1 needs to avoid starving itself.
                rate = variant['total_messages_per_second']
                pool = pool_policy.pool_for_rate(rate)
                plan['vus'] = {'pre_allocated': pool, 'max': pool}
                for step in plan['schedule']['steps']:
                    step['vus'] = pool
                plan['health']['workload_policy'] = pool_policy.policy_for_rate(rate)
                plan['deviations'] = [
                    f'Workload {wid}: only {CHANGES[wid][0]} changes relative to B0; identities/plan labels differ by design.',
                    'N and application CPU budget are independent: CPU pool 4-5 (k=2) is fixed, including W4 N=1.',
                    'Existing deployed state, device cohorts and history are retained; not cold-start IID repetitions.',
                    'Application dispatch is not wire transmission. Logical padding is not actual wire byte count.',
                    f'Generator VU pool is rate-derived ({pool} at {rate}/s), per the 2026-11-10 owner amendment; '
                    'the pool is generator concurrency, not a platform capacity claim.']
                path = f'plans/{rid}.json'
                files[path] = plan
                deployment = {'replicas_per_replicated_service': variant['replicas_per_replicated_service'],
                              'replicated_services': SERVICES[platform], 'application_cpu_pool': '4-5',
                              'application_cpu_budget_k': 2, 'same_haproxy_ingress': True,
                              'per_replica_memory_and_images': 'unchanged from qualified B0; verify effective values before/after each run',
                              'supporting_tiers': 'unchanged; retain actual resources and reused database state'}
                rows.append({'ordinal': len(rows)+1, 'workload_id': wid, 'platform': platform,
                             'repetition': rep, 'run_id': rid, 'seed': seeds[rep-1],
                             'plan': path, 'plan_sha256': hashlib.sha256(encoded(plan)).hexdigest(),
                             'expected_warmup_messages': variant['total_messages_per_second']*300,
                             'expected_measurement_messages': variant['total_messages_per_second']*600,
                             'deployment': deployment, 'execution_status': 'BLOCKED_PENDING_VARIANT_RELEASE'})
    files['run-matrix.json'] = {'schema_version': 1, 'batch_id': scope['batch_id'],
        'scope_sha256': hashlib.sha256(encoded(scope)).hexdigest(),
        'status': 'PROPOSED_NOT_LAUNCH_CLEARED', 'experimental_runs': rows,
        'pool_policy_revision': {
            'revision': '2026-11-10-generator-pool-v2',
            'decided_by': 'owner instruction to raise the pool when the limit is a framework artifact rather than a platform limit',
            'supersedes': 'fixed 150/150 VUs per scenario (dispatch-integrity-v1)',
            'change': 'generator pool is derived from rate x timeout with 10% headroom, rounded up to 50',
            'scope': 'generator concurrency only; no platform-side resource or deployment setting changes',
            'b0_impact': 'none - the rule reproduces 150 at 25/s, so every B0 cell and every W2/W3/W4 cell is pool-identical',
            'w1_impact': 'W1 50/s moves 150 -> 300; this is the pool W1 always required (rate x timeout = 250)',
            'evidence': [
                'ThingsBoard qualification w1-qual-tb-20261110c recorded implied_in_flight_at_p99 = 243 against a 150 pool',
                'the same run needed ~11 VUs in measurement once warm, and dropped nothing there',
                'FIWARE qualification w1-qual-fw-20261110c dropped only during a 30 s latency stall, not at steady state',
                'B0 peak was 125 (ThingsBoard) and 16 (FIWARE) at 25/s, so the 150 cap never bound any B0 cell'],
            'not_claimed': 'This is not a platform capacity claim and does not authorize a throughput claim above the fixed points.',
        }}
    files['qualification-gates.json'] = {
        'all_workloads': ['Separate workload-aware launcher and aggregation; current B0 launcher/matrix/analysis grouping are not a W1-W4 release.',
                          'Fresh images/resources/clock/capture/history/provenance and workload integrity gates; preserve invalid attempts and valid platform failures.',
                          'Freeze qualification plans and workload-aware verdict policy before variant outcomes. Reuse only matching qualified evidence.',
                          'Reconcile independent stored history and report missing, late, duplicate, malformed/truncated and UNKNOWN deliveries separately.'],
        'W1': ['Full-duration 50/s, 300/600-second generator/capture validation under actual adapter/recording/monitoring: 15000 warm-up and 30000 measurement identities.',
               'Existing 50/s 30/120-second B0 qualification is not full-duration W1 clearance.',
               'Re-qualify under pool policy v2 (300 VUs at 50/s). The two v1 qualifications (w1-qual-tb-20261110c, w1-qual-fw-20261110c) are preserved negatives that measured generator starvation, not platform limits; they are not retried and not reinterpreted. A v2 generation is required.',
                'The dispatch-integrity evaluator has an explicit 100/s profile with unchanged identity, timing, loss, bin, gap and lag-integrity checks. This is source support only: qualify 100/s generator/capture capability under matching HTTP/recording conditions before release; do not edit active B0 gates.'],
        'W2': ['Validate 2000-device provisioning and every active replica recognizing the entire intended cohort before traffic.',
               'For EdgeX, retain numeric runtime inventory and metadata/driver discovery limits with historical cohorts; reserve the entire planned batch plus qualifications/margin. Current 16384 limit is below projected 36192 experimental population.',
               'Any common discovery-setting amendment must be frozen for the new batch and its B0 comparability disclosed; no deletion/reset and no active-B0 change.'],
        'W3': ['Exactly 1024 extra ASCII logical padding bytes, using size_offset_bytes=1024 on every adapter, with live sink/history preservation and independent payload validation.',
               'Current FIWARE sink/history subscriptions select only identity/timing fields, and sink records omit padding. Review and qualify a payload-preserving evidence path with common-batch/comparability treatment; never silently change only W3 or active B0.',
               'Report logical bytes, actual serialized adapter body bytes, scoped measured transport bytes and measurement method separately. Current HTTP-header arithmetic is only an estimate, not actual on-wire capture.',
               'For identical identity fields FIWARE zero padding is encoded as quoted empty text; native body delta can be 1022 although logical padding delta is exactly 1024.'],
        'W4': ['Exactly one active replica of each replicated service, same two-core pool 4-5, unchanged per-replica limits, supporting tiers and HAProxy path.',
               'N=1 must not imply k=1/core4; verify Docker/kernel effective cpusets and one UP backend per tier, including ThingsBoard rule-engine container count.',
               'Report changed aggregate application memory caused by fewer replicas; this is not a fixed-total-memory or CPU-scaling experiment.'],
        'other_configuration_headroom': 'Qualify applicable 50/s headroom for D=2000, padding=1024 or N=1/k=2, or obtain reviewed matching evidence; B0 qualification does not automatically transfer.'}
    return files


def stage(out, benchctl=None, check=False):
    before = baseline_hashes()
    files = artifacts()
    if check:
        for name, value in files.items():
            if (out/name).read_bytes() != encoded(value):
                raise ValueError('staged proposal differs from generator: '+name)
    else:
        out.mkdir(parents=True, exist_ok=False)
        for name, value in files.items():
            target = out/name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(encoded(value))
    if benchctl:
        if check and not (out/'offline-resolution.json').is_file():
            raise ValueError('offline resolution missing; generate with this binary into a fresh output directory')
        resolved = []
        for cell in files['run-matrix.json']['experimental_runs']:
            p = subprocess.run([str(benchctl), 'resolve', '-plan', str(out/cell['plan']), '-run-id', cell['run_id']],
                               cwd=REPO, check=True, capture_output=True, text=True, timeout=30)
            plan = json.loads(p.stdout)
            assert plan['run_id'] == cell['run_id'] and sum(s['ticks'] for s in plan['steps']) == cell['expected_warmup_messages']+cell['expected_measurement_messages']
            resolved.append({'workload_id':cell['workload_id'], 'platform':cell['platform'],
                             'run_id':cell['run_id'], 'resolved_generator':plan,
                             'intended_deployment_not_live_observation':cell['deployment']})
        validation = {'status':'OFFLINE_RESOLUTION_ONLY_NO_TRAFFIC', 'benchctl_sha256':sha(benchctl),
                      'deployment_verified_live':False, 'execution_authorized':False, 'runs':resolved}
        target = out/'offline-resolution.json'
        if target.exists():
            if target.read_bytes() != encoded(validation):raise ValueError('offline resolution differs; retain original and review')
        else:target.write_bytes(encoded(validation))
    if before != baseline_hashes():raise RuntimeError('B0 changed while staging; investigate, never overwrite it')
    return files


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--out',type=Path,default=DEFAULT_OUT)
    ap.add_argument('--benchctl',type=Path,help='optional existing binary; runs resolve only, no Docker or hosts')
    ap.add_argument('--check',action='store_true',help='strictly read-only verification of existing proposal and optional retained resolution')
    args=ap.parse_args()
    stage(args.out.resolve(),args.benchctl.resolve() if args.benchctl else None,args.check)
    print(str(args.out)+': 60 proposed attempts, five/workload/platform; variants NOT launch-cleared; B0 unchanged')


if __name__=='__main__':main()
