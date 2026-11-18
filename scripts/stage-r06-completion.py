#!/usr/bin/env python3
"""Extend the W matrix with the repetitions still needed for a balanced 12x3.

Why this exists
---------------
Two thingsboard cells are still short of three valid repetitions, and the frozen
matrix has no unspent IDs left for them:

    W2/thingsboard  2 valid (r3@o34, r6@o63)   needs 1
    W3/thingsboard  1 valid (r3@o27)           needs 2

Every remaining pre-registered ID for those cells is spent. What consumed them
was host faults, not platform results: r2/r5 for W2 and r1/r2/r4/r5 for W3 all
died on pre-traffic gates, o64 was burned in 4.3 s by a capture port its own
aborted predecessor still held, and o65/o66/o67 were burned together on
2026-11-18 when a flaky SSH path to the load host failed three consecutive
launches before any of them sent a message (see POSTPROCESS-FINDINGS item 10).

The owner chose a balanced 12x3 matrix, so this adds exactly three runs:

    w2dp-tb-r09-20261108   (W2/thingsboard, repetition 9)
    w3dp-tb-r08-20261108   (W3/thingsboard, repetition 8)
    w3dp-tb-r09-20261108   (W3/thingsboard, repetition 9)

Each fills one of the missing slots. No cell gains a fourth valid repetition, so
the finished design stays balanced at three per cell.

Why it cannot run mid-campaign
------------------------------
`run-matrix.json` and `scope.json` are both bound into the live receipt via
`bindings_sha256`, and `verify_tree_bindings()` re-checks those hashes on *every*
launch and switch. Editing either file while the queue is running makes every
remaining launch fail closed with 'receipt has drifted from the tree'. So this
script is deliberately two-phase:

    --prepare   write the plans and a proposal sidecar, touch nothing bound
    --apply     extend run-matrix.json and scope.json, then require a new receipt

`--apply` refuses while a driver or launcher is alive, and it refuses if either
bound file has drifted from the receipt it was prepared against.

What it does NOT do
-------------------
It does not authorize anything. After `--apply` the new IDs have no receipt, so
the launcher will refuse them until a new receipt is recorded. That receipt is
the owner's decision and is created separately, which is the intended gate.

Usage:
    scripts/stage-r06-completion.py --prepare
    scripts/stage-r06-completion.py --apply --receipt owner-w-matrix3-20261117c
    scripts/stage-r06-completion.py --check
"""
import argparse
import copy
import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
W_ROOT = REPO / 'workloads/campaigns/fixed-points-w1-w4'
PLANS = W_ROOT / 'plans'
MATRIX = W_ROOT / 'run-matrix.json'
SCOPE = W_ROOT / 'scope.json'
PROPOSAL = W_ROOT / 'r06-completion-proposal.json'

# The remaining gap after the 2026-11-18 re-arm burned ordinals 65-67.
#
# That batch authorized o65/o66/o67 for W3's two remaining IDs and one new W2 ID.
# All three were lost before any message was sent, to an intermittent SSH fault on
# the load host (0 traffic, workload_launch_attempted false). W1/thingsboard had
# already completed via o61 and o62, and W2's o63 was recovered on disk, so what is
# actually left is:
#
#   W2/thingsboard  r3@o34 + r6@o63                     + r09 -> 3
#   W3/thingsboard  r3@o27                              + r08, r09 -> 3
#
# Each new row is a repetition of an already-qualified fixed point, so it fills a
# missing slot rather than adding a fourth.
TARGETS = (
    ('W2', 'thingsboard', 9, 'w2dp-tb-r09-20261108'),
    ('W3', 'thingsboard', 8, 'w3dp-tb-r08-20261108'),
    ('W3', 'thingsboard', 9, 'w3dp-tb-r09-20261108'),
)

# The frozen scope only defines five seeds (`seed_by_repetition`) and the stager
# indexes it as seeds[rep-1], so later repetitions would raise IndexError. Seeds
# are reproducibility metadata, not benchmark parameters, so new values are
# appended rather than re-deriving the existing five, which must stay
# byte-identical to what the 60 authorized runs used.
NEW_SEEDS = {6: 202611066, 7: 202611067, 8: 202611068, 9: 202611069}


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def bounded_receipt(receipt_id):
    path = W_ROOT / 'authorization' / f'{receipt_id}.json'
    if not path.is_file():
        raise SystemExit(f'receipt not found: {path}')
    return json.loads(path.read_text())


def assert_quiet():
    """Refuse to touch bound files while anything could re-verify them.

    The match is on the *executable command line*, not on any string that merely
    mentions the script. A long-lived `tmux` server, for instance, carries the
    original `new-session ... python3 scripts/drive-fixed-point-matrix.py` text in
    its argv forever, even after that session has exited, so a naive `pgrep -f`
    reports a driver that no longer exists. Matching on the leading token of the
    process command line tells the difference.
    """
    p = subprocess.run(['ps', '-eo', 'pid=,args='], capture_output=True, text=True)
    alive = []
    for line in p.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        pid, _, args = line.partition(' ')
        # A live driver/launcher has python3 as the *first* token.
        if not args.startswith('python3 '):
            continue
        if ('drive-fixed-point-matrix.py' in args
                or 'run-workload-campaign.py' in args):
            alive.append(f'{pid} {args[:120]}')
    if alive:
        raise SystemExit('refusing: a driver or launcher is alive:\n  '
                         + '\n  '.join(alive[:5]))


def baseline_plan(workload_id, platform):
    """Reuse the cell's r05 plan as the template for later repetitions."""
    p = PLANS / f'{workload_id.lower()}dp-{"tb" if platform == "thingsboard" else platform[:2]}-r05-20261108.json'
    if not p.is_file():
        raise SystemExit(f'missing template plan: {p}')
    return json.loads(p.read_text())


def build_plan(workload_id, platform, repetition, run_id, seed):
    """Clone the cell's r05 plan and retarget only the identity fields.

    Everything measured -- rates, devices, payload offset, VU pool, schedule,
    health policy -- is copied unchanged, because a later repetition is the same
    fixed point as the other repetitions of that cell. Only labels and seed differ.
    """
    plan = copy.deepcopy(baseline_plan(workload_id, platform))
    plan['plan_id'] = f'fixed-{workload_id.lower()}-{platform}-r{repetition:02d}-20261108'
    plan['seed'] = seed
    plan['purpose'] = (f'{workload_id} fixed-point experimental attempt proposal; separate from B0, '
                       'diagnostics and qualification. NOT launch-cleared.')
    return plan


def encoded(plan):
    return (json.dumps(plan, indent=1, sort_keys=True) + '\n').encode()


def prepare(receipt_id):
    """Write plans + proposal. Touches no bound artefact."""
    receipt = bounded_receipt(receipt_id)
    scope = json.loads(SCOPE.read_text())
    matrix = json.loads(MATRIX.read_text())

    existing = {r['run_id'] for r in matrix['experimental_runs']}
    by_ordinal = {r['ordinal']: r for r in matrix['experimental_runs']}
    next_ordinal = max(by_ordinal) + 1

    # Reuse the deployment block and message counts of the cell's existing rows so
    # the new rows are structurally identical to their siblings.
    rows, files = [], {}
    for workload_id, platform, repetition, run_id in TARGETS:
        if run_id in existing:
            raise SystemExit(f'{run_id} is already in the matrix; nothing to prepare')
        seed = NEW_SEEDS[repetition]
        plan = build_plan(workload_id, platform, repetition, run_id, seed)
        sibling = next(r for r in matrix['experimental_runs']
                       if r['workload_id'] == workload_id and r['platform'] == platform)
        path = f'plans/{run_id}.json'
        files[path] = plan
        rows.append({
            'ordinal': next_ordinal,
            'workload_id': workload_id,
            'platform': platform,
            'repetition': repetition,
            'run_id': run_id,
            'seed': seed,
            'plan': path,
            'plan_sha256': hashlib.sha256(encoded(plan)).hexdigest(),
            'expected_warmup_messages': sibling['expected_warmup_messages'],
            'expected_measurement_messages': sibling['expected_measurement_messages'],
            'deployment': sibling['deployment'],
            'execution_status': 'BLOCKED_PENDING_VARIANT_RELEASE',
        })
        next_ordinal += 1

    # Write plans (not hash-bound: plans are bound per-run via the receipt).
    for path, plan in files.items():
        (W_ROOT / path).write_bytes(encoded(plan))
        print(f'  wrote {path}')

    proposal = {
        'schema_version': 1,
        'purpose': ('Add w2dp-tb-r09-20261108, w3dp-tb-r08-20261108 and '
                    'w3dp-tb-r09-20261108 so W2/thingsboard reaches three valid '
                    'repetitions (r3@o34, r6@o63, r9) and W3/thingsboard reaches three '
                    '(r3@o27, r8, r9), giving a balanced 12x3 matrix. No cell gains a '
                    'fourth valid repetition.'),
        'prepared_against_receipt': receipt_id,
        'prepared_bindings_sha256': {
            'scope': sha(SCOPE),
            'run_matrix': sha(MATRIX),
        },
        'why': ('W2/thingsboard stands at 2 valid reps and W3/thingsboard at 1. Every '
                'pre-registered ID for both cells is spent, and what spent them was host '
                'faults rather than platform results: the early repetitions died on '
                'pre-traffic gates, o64 was burned by a capture port its own aborted '
                'predecessor still held, and on 2026-11-18 a flaky load-host SSH path '
                'consumed o65, o66 and o67 in three consecutive launches that never sent a '
                'single message (workload_launch_attempted false). Those remain burned and '
                'are never retried. These three IDs are new: one closes W2, two close W3. '
                'Each is a repetition of an already-qualified fixed point, so the design '
                'stays balanced at three valid reps per cell.'),
        'adds_rows': rows,
        'appends_seeds': NEW_SEEDS,
        'scope_changes': {
            'seed_by_repetition': 'append ' + ', '.join(str(v) for v in sorted(NEW_SEEDS.values()))
                                  + ' (9th entry)',
            'experimental_attempts_proposed': f'{scope["experimental_attempts_proposed"]} -> '
                                              f'{scope["experimental_attempts_proposed"] + len(rows)}',
        },
        'authorization_required': True,
        'note': ('run-matrix.json and scope.json are hash-bound into the live receipt, and '
                 'verify_tree_bindings() re-checks them on every launch. Apply only while the '
                 'queue is idle, then record a new receipt naming all three new IDs.'),
    }
    PROPOSAL.write_text(json.dumps(proposal, indent=1, sort_keys=True) + '\n')
    print(f'  wrote {PROPOSAL.relative_to(REPO)}')
    print(f'\nprepared {len(rows)} row(s); next ordinals {rows[0]["ordinal"]}-{rows[-1]["ordinal"]}')
    print('nothing bound was modified. Apply with --apply after the queue drains.')
    return 0


def check():
    """Report whether the proposal is consistent with the current tree."""
    if not PROPOSAL.is_file():
        print('no proposal; run --prepare first')
        return 1
    prop = json.loads(PROPOSAL.read_text())
    matrix = json.loads(MATRIX.read_text())
    have = {r['run_id'] for r in matrix['experimental_runs']}
    drift = []
    for name, digest in prop['prepared_bindings_sha256'].items():
        path = {'scope': SCOPE, 'run_matrix': MATRIX}[name]
        if sha(path) != digest:
            drift.append(name)
    applied = all(r['run_id'] in have for r in prop['adds_rows'])
    print(f'proposal:            {PROPOSAL.relative_to(REPO)}')
    print(f'adds:                {", ".join(r["run_id"] for r in prop["adds_rows"])}')
    print(f'rows already in matrix: {applied}')
    print(f'bound files drifted since prepare: {", ".join(drift) if drift else "none"}')
    if drift and not applied:
        print('  -> the tree moved; re-run --prepare against the current receipt')
    for r in prop['adds_rows']:
        p = PLANS / f'{r["run_id"]}.json'
        ok = p.is_file() and hashlib.sha256(encoded(json.loads(p.read_text()))).hexdigest() == r['plan_sha256']
        print(f'  plan {r["run_id"]}: {"OK" if ok else "MISSING OR MODIFIED"}'
              f' (ordinal {r["ordinal"]}, seed {r["seed"]})')
    return 0


def apply(receipt_id):
    assert_quiet()
    if not PROPOSAL.is_file():
        raise SystemExit('no proposal; run --prepare first')
    prop = json.loads(PROPOSAL.read_text())

    # The tree must be exactly what the proposal was prepared against, or the
    # hashes baked into the proposal (and the receipt) would describe a different
    # file than the one about to be extended.
    for name, digest in prop['prepared_bindings_sha256'].items():
        path = {'scope': SCOPE, 'run_matrix': MATRIX}[name]
        if sha(path) != digest:
            raise SystemExit(f'{name} drifted since --prepare; re-run --prepare against the '
                             f'current tree, then obtain a fresh receipt')

    matrix = json.loads(MATRIX.read_text())
    have = {r['run_id'] for r in matrix['experimental_runs']}
    for r in prop['adds_rows']:
        if r['run_id'] in have:
            raise SystemExit(f'{r["run_id"]} already present; refusing to duplicate')
        p = PLANS / f'{r["run_id"]}.json'
        if not p.is_file():
            raise SystemExit(f'missing plan {p}; run --prepare first')
        if hashlib.sha256(encoded(json.loads(p.read_text()))).hexdigest() != r['plan_sha256']:
            raise SystemExit(f'plan {p.name} does not match the prepared hash')

    # Extend scope first (matrix hashes nothing but itself).
    scope = json.loads(SCOPE.read_text())
    seeds = list(scope['seed_by_repetition'])
    for value in sorted(prop['appends_seeds'].values()):
        if value not in seeds:
            seeds.append(value)
    scope['seed_by_repetition'] = seeds
    scope['experimental_attempts_proposed'] = (scope['experimental_attempts_proposed']
                                               + len(prop['adds_rows']))
    SCOPE.write_text(json.dumps(scope, indent=1, sort_keys=True) + '\n')

    matrix['experimental_runs'].extend(prop['adds_rows'])
    MATRIX.write_text(json.dumps(matrix, indent=1, sort_keys=True) + '\n')

    print(f'extended {MATRIX.relative_to(REPO)} to {len(matrix["experimental_runs"])} rows')
    print(f'extended {SCOPE.relative_to(REPO)} seed_by_repetition to {len(seeds)} entries')
    print('\nBoth files are now drifted from receipt ' + receipt_id + '.')
    print('Record a new receipt naming the new IDs before launching them:')
    print('  python3 scripts/record-w-authorization.py --receipt-id <new> \\')
    print('      --authorize ' + ' '.join(r['run_id'] for r in prop['adds_rows']))
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument('--prepare', action='store_true',
                   help='write the two plan files and a proposal; touch nothing bound')
    g.add_argument('--apply', action='store_true',
                   help='extend run-matrix.json and scope.json (queue must be idle)')
    g.add_argument('--check', action='store_true', help='report proposal consistency')
    ap.add_argument('--receipt', default='owner-w-matrix3-20261117a',
                    help='receipt to record drift against (default: current live receipt)')
    args = ap.parse_args()
    if args.prepare:
        return prepare(args.receipt)
    if args.apply:
        return apply(args.receipt)
    return check()


if __name__ == '__main__':
    sys.exit(main())
