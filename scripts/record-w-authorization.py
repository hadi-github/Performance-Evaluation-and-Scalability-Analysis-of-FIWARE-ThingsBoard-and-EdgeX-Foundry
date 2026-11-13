#!/usr/bin/env python3
"""Record an owner-authorization receipt for W batch execution. Offline only.

`scope.json` ships with `execution_authorized: false` and the matrix with
`PROPOSED_NOT_LAUNCH_CLEARED`. Those are fail-closed generator outputs. The right
way to open them is not to edit them — that would make the proposal assert an
approval it never received — but to record a separate, hash-bound receipt that
the launcher verifies before any mutation.

This script only writes that receipt. It never launches traffic and cannot grant
itself authority: it requires an explicit `--instruction` quoting the owner's
authorization, and it binds every artefact the launch depends on so the receipt
cannot be replayed against a different tree, plan or binary.

If any bound artefact changes after the receipt is written, the launcher refuses
to start. That is intentional: an approval for artifact set A must not silently
authorize artifact set B.
"""
import argparse
import hashlib
import json
import platform as pyplatform
from pathlib import Path

import sys
sys.path.insert(0, str(Path(__file__).resolve().parent))
import pool_policy  # noqa: E402  (single source of truth for the VU pool)

REPO = Path(__file__).resolve().parents[1]
W_ROOT = REPO / 'workloads' / 'campaigns' / 'fixed-points-20261108'
DEFAULT_OUT = W_ROOT / 'authorization'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def qualification_index_paths():
    """Every workload's qualification ledger, W1 first.

    Mirrors the launcher: W1 keeps the historical root, later workloads get a
    subdirectory. A receipt that could not see the W2 ledger would be unable to
    bind a W2 plan, which would make authorization look like a missing file.
    """
    root = W_ROOT / 'qualification'
    paths = []
    if (root / 'index.json').is_file():
        paths.append(('W1', root / 'index.json'))
    for wid in ('W2', 'W3', 'W4'):
        p = root / wid.lower() / 'index.json'
        if p.is_file():
            paths.append((wid, p))
    return paths


def qual_root_for(row):
    wid = row.get('workload_id', 'W1')
    return W_ROOT / 'qualification' if wid == 'W1' else W_ROOT / 'qualification' / wid.lower()


def bindings():
    """Every artefact whose change would invalidate the approval."""
    files = {
        'scope': W_ROOT / 'scope.json',
        'run_matrix': W_ROOT / 'run-matrix.json',
        'launcher': REPO / 'scripts' / 'run-workload-campaign.py',
        'aggregator': REPO / 'scripts' / 'aggregate-runs.py',
        'qualification_stager': REPO / 'scripts' / 'stage-w1-qualification.py',
        'variant_stager': REPO / 'scripts' / 'stage-fixed-point-variants.py',
        'integrity': REPO / 'scripts' / 'dispatch-integrity.py',
        'pool_policy': REPO / 'scripts' / 'pool_policy.py',
        'b0_launcher': REPO / 'results' / 'feasible-campaign-20261106' / 'run-feasible-campaign.py',
    }
    out = {}
    for name, path in files.items():
        if not path.is_file():
            raise SystemExit(f'refusing to bind a missing artefact: {name} ({path})')
        out[name] = sha(path)
    # Bind each workload's ledger under its own key rather than pretending one
    # file is the whole qualification record.
    for wid, path in qualification_index_paths():
        out[f'qualification_index_{wid.lower()}'] = sha(path)
    return out


def plan_bindings(run_ids):
    """Hash each authorized plan by run ID, resolved exactly as the launcher does."""
    out = {}
    try:
        matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
    except FileNotFoundError as e:
        raise SystemExit(f'missing W control file: {e}')
    by_id = {r['run_id']: W_ROOT / r['plan'] for r in matrix['experimental_runs']}
    for wid, path in qualification_index_paths():
        index = json.loads(path.read_text())
        for r in index['runs']:
            r.setdefault('workload_id', wid)
            by_id[r['run_id']] = qual_root_for(r) / r['plan']
    for rid in run_ids:
        path = by_id.get(rid)
        if path is None:
            raise SystemExit(f'run ID is not in the W matrix or qualification index: {rid!r}')
        if not path.is_file():
            raise SystemExit(f'missing plan for authorized run {rid}: {path}')
        out[rid] = sha(path)
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--instruction',
                    help='verbatim owner authorization this receipt records')
    ap.add_argument('--receipt-id', required=True,
                    help='stable ID for this approval, e.g. owner-w1-qual-20261110')
    ap.add_argument('--authorize', nargs='+',
                    help='run IDs this receipt authorizes (exact, no wildcards)')
    ap.add_argument('--scope', default='w1-qualification',
                    help='human label for what the approval covers')
    ap.add_argument('--out', type=Path, default=DEFAULT_OUT)
    ap.add_argument('--check', action='store_true', help='verify an existing receipt only')
    args = ap.parse_args()
    if not args.check and not (args.instruction and args.authorize):
        raise SystemExit('recording a receipt requires --instruction and --authorize')

    out = args.out.resolve()
    target = out / f'{args.receipt_id}.json'
    if args.check:
        if not target.is_file():
            raise SystemExit(f'receipt not found: {target}')
        stored = json.loads(target.read_text())
        drift = [k for k, v in stored['bindings_sha256'].items()
                 if bindings().get(k) != v]
        plan_drift = [rid for rid, v in stored['authorized_plan_sha256'].items()
                      if plan_bindings([rid]).get(rid) != v]
        if drift or plan_drift:
            raise SystemExit('receipt has drifted from the tree: '
                             + ', '.join(drift + plan_drift))
        print(f'{target}: receipt verified, {len(stored["authorized_run_ids"])} run ID(s), no drift')
        return 0

    payload = {
        'schema_version': 1,
        'receipt_id': args.receipt_id,
        'scope_label': args.scope,
        'recorded_by': 'agent',
        'owner_instruction_verbatim': args.instruction,
        'authorized_run_ids': sorted(set(args.authorize)),
        'counts_toward_proposed_attempts': False,
        'bindings_sha256': bindings(),
        'authorized_plan_sha256': plan_bindings(args.authorize),
        'environment': {'hostname_platform': pyplatform.platform()},
        'limits': [
            'Authorizes exactly the listed run IDs. No wildcard, no substitute ID.',
            'An attempt that fails is preserved; this receipt is not a retry licence.',
            'Does not authorize EdgeX while its readiness blocker is open.',
            'Does not authorize W3 FIWARE, whose padding evidence path does not exist.',
            'The generator VU pool is rate-derived under the separately recorded 2026-11-10 '
            'owner amendment (run-matrix.json pool_policy_revision); this receipt does not '
            f'widen it beyond that rule ({pool_policy.FROZEN_POOLS}).',
        ],
    }
    encoded = (json.dumps(payload, indent=2) + '\n').encode()
    if target.exists():
        raise SystemExit(f'refusing to overwrite an existing receipt: {target}')
    out.mkdir(parents=True, exist_ok=True)
    target.write_bytes(encoded)
    print(f'{target}: receipt recorded for {len(payload["authorized_run_ids"])} run ID(s)')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
