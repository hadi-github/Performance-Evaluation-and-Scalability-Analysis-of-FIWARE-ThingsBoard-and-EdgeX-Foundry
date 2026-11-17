#!/usr/bin/env python3
"""Extend the W matrix with a sixth repetition so W1/tb and W2/tb reach 3 reps.

Why this exists
---------------
Each thingsboard cell was pre-registered with exactly five IDs. For W1/thingsboard
and W2/thingsboard, three of those five are already spent on pre-traffic host
failures, so those two cells can never reach three valid repetitions from the
frozen matrix:

    W1/thingsboard  usable r2@o24            + unrun r5@o51  -> max 2
    W2/thingsboard  usable r3@o34            + unrun r5@o53  -> max 2

The owner chose a balanced 12x3 matrix, so this adds exactly two runs:

    w1dp-tb-r06-20261108   (W1/thingsboard, repetition 6)
    w2dp-tb-r06-20261108   (W2/thingsboard, repetition 6)

r06 fills the *third* slot in each of those cells. It does not add a fourth
repetition to any cell, so the finished design stays balanced at 3 reps per cell.

Why it cannot run mid-campaign
------------------------------
`run-matrix.json` and `scope.json` are both bound into the live receipt via
`bindings_sha256`, and `verify_tree_bindings()` re-checks those hashes on *every*
launch and switch. Editing either file while the queue is running makes every
remaining launch fail closed with 'receipt has drifted from the tree'. So this
script is deliberately two-phase:

    --prepare   write the two plans and a proposal sidecar, touch nothing bound
    --apply     extend run-matrix.json and scope.json, then require a new receipt

`--apply` refuses while a driver or launcher is alive, and it refuses if either
bound file has drifted from the receipt it was prepared against.

What it does NOT do
-------------------
It does not authorize anything. After `--apply` the two new IDs have no receipt,
so the launcher will refuse them until a new receipt is recorded. That receipt is
the owner's decision and is created separately, which is the intended gate.

Usage:
    scripts/stage-r06-completion.py --prepare
    scripts/stage-r06-completion.py --apply --receipt owner-w-matrix3-20261117a
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

# The two cells that cannot reach three reps from the frozen matrix, and the
# repetition number that fills their third slot.
TARGETS = (
    ('W1', 'thingsboard', 6, 'w1dp-tb-r06-20261108'),
    ('W2', 'thingsboard', 6, 'w2dp-tb-r06-20261108'),
)

# A sixth seed. The frozen scope only defines five (`seed_by_repetition`), and the
# stager indexes it as seeds[rep-1], so repetition 6 would raise IndexError. The
# seed is metadata for reproducibility, not a benchmark parameter, so a new value
# is appended rather than re-deriving the existing five, which must stay
# byte-identical to what the 60 authorized runs used.
NEW_SEED = 202611066


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def bounded_receipt(receipt_id):
    path = W_ROOT / 'authorization' / f'{receipt_id}.json'
    if not path.is_file():
        raise SystemExit(f'receipt not found: {path}')
    return json.loads(path.read_text())


def assert_quiet():
    """Refuse to touch bound files while anything could re-verify them."""
    p = subprocess.run(['pgrep', '-af', 'drive-fixed-point-matrix.py|run-workload-campaign.py'],
                       capture_output=True, text=True)
    alive = [l for l in p.stdout.splitlines()
             if 'pgrep' not in l and 'stage-r06' not in l]
    if alive:
        raise SystemExit('refusing: a driver or launcher is alive:\n  '
                         + '\n  '.join(alive[:5]))


def baseline_plan(workload_id, platform):
    """Reuse the r05 plan for the same cell as the template for r06."""
    p = PLANS / f'{workload_id.lower()}dp-{"tb" if platform == "thingsboard" else platform[:2]}-r05-20261108.json'
    if not p.is_file():
        raise SystemExit(f'missing template plan: {p}')
    return json.loads(p.read_text())


def build_plan(workload_id, platform, repetition, run_id, seed):
    """Clone the cell's r05 plan and retarget only the identity fields.

    Everything measured -- rates, devices, payload offset, VU pool, schedule,
    health policy -- is copied unchanged, because r06 is the same fixed point as
    the other repetitions of that cell. Only the labels and the seed differ.
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
        seed = NEW_SEED
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
        'purpose': ('Add w1dp-tb-r06-20261108 and w2dp-tb-r06-20261108 so '
                    'W1/thingsboard and W2/thingsboard reach three valid repetitions, '
                    'giving a balanced 12x3 matrix.'),
        'prepared_against_receipt': receipt_id,
        'prepared_bindings_sha256': {
            'scope': sha(SCOPE),
            'run_matrix': sha(MATRIX),
        },
        'why': ('Each thingsboard cell had exactly five pre-registered IDs. Three of the '
                'five in W1/thingsboard and in W2/thingsboard were consumed by pre-traffic '
                'host failures, so neither cell can reach three valid repetitions without '
                'a sixth ID. r06 fills the third slot; it adds no fourth repetition '
                'anywhere, so the design stays balanced at three per cell.'),
        'adds_rows': rows,
        'appends_seed': NEW_SEED,
        'scope_changes': {
            'seed_by_repetition': f'append {NEW_SEED} (6th entry)',
            'experimental_attempts_proposed': f'{scope["experimental_attempts_proposed"]} -> '
                                              f'{scope["experimental_attempts_proposed"] + len(rows)}',
        },
        'authorization_required': True,
        'note': ('run-matrix.json and scope.json are hash-bound into the live receipt, and '
                 'verify_tree_bindings() re-checks them on every launch. Apply only after the '
                 'current queue drains, then record a new receipt naming both IDs.'),
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
    if NEW_SEED not in seeds:
        seeds.append(NEW_SEED)
    scope['seed_by_repetition'] = seeds
    scope['experimental_attempts_proposed'] = (scope['experimental_attempts_proposed']
                                               + len(prop['adds_rows']))
    SCOPE.write_text(json.dumps(scope, indent=1, sort_keys=True) + '\n')

    matrix['experimental_runs'].extend(prop['adds_rows'])
    MATRIX.write_text(json.dumps(matrix, indent=1, sort_keys=True) + '\n')

    print(f'extended {MATRIX.relative_to(REPO)} to {len(matrix["experimental_runs"])} rows')
    print(f'extended {SCOPE.relative_to(REPO)} seed_by_repetition to {len(seeds)} entries')
    print('\nBoth files are now drifted from receipt ' + receipt_id + '.')
    print('Record a new receipt naming the two new IDs before launching them:')
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
