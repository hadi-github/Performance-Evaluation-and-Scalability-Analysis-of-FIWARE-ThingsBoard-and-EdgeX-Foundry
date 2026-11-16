#!/usr/bin/env python3
"""Report the W1-W4 x platform cell census from evidence actually on disk.

The driver log is the source of truth for *what was attempted*; this script
reports what is *usable*, which is a different question. A run counts toward a
cell only if all of the following hold:

  * its evidence directory has a run-result.json,
  * platform_result is VALID_PLATFORM_SUCCESS or VALID_PLATFORM_FAILURE_OR_BACKLOG,
  * capture_valid and measurement_valid are both true,
  * it is not marked a harness defect (harness-defect.json with
    platform_finding false).

The campaign was reduced from five repetitions per cell to three by owner
decision, with run-matrix.json left byte-identical, so three is the target.

Usage: scripts/census-w-matrix.py [--expect 3] [--verbose]
Exit status is 0 when every cell has exactly the expected count.
"""
import argparse
import json
import pathlib
import sys
from collections import defaultdict

REPO = pathlib.Path(__file__).resolve().parents[1]
RESULTS = REPO / 'results/workload-batch-20261108'
EVIDENCE = RESULTS / 'evidence'
MATRIX = REPO / 'workloads/campaigns/fixed-points-w1-w4/run-matrix.json'

VALID = {'VALID_PLATFORM_SUCCESS', 'VALID_PLATFORM_FAILURE_OR_BACKLOG'}


def harness_defect(d):
    p = d / 'harness-defect.json'
    if not p.is_file():
        return None
    try:
        rec = json.loads(p.read_text())
    except (json.JSONDecodeError, OSError):
        # An unreadable sidecar is not evidence of health; treat as excluded.
        return {'platform_finding': False, 'unreadable': True}
    if rec.get('platform_finding') is False:
        return rec
    return None


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--expect', type=int, default=3,
                    help='required valid repetitions per cell (default 3)')
    ap.add_argument('--verbose', action='store_true')
    args = ap.parse_args()

    matrix = json.loads(MATRIX.read_text())
    by_id = {r['run_id']: r for r in matrix['experimental_runs']}

    # Every cell the matrix defines, so a cell with no usable run still shows up
    # as 0/3 instead of silently disappearing from the table.
    cells = defaultdict(list)
    for r in matrix['experimental_runs']:
        cells[(r['workload_id'], r['platform'])]
    excluded = []
    unusable = []

    for d in sorted(EVIDENCE.iterdir()):
        if not d.is_dir():
            continue
        row = by_id.get(d.name)
        rr = d / 'run-result.json'
        if not rr.is_file():
            continue
        try:
            res = json.loads(rr.read_text())
        except (json.JSONDecodeError, OSError):
            unusable.append((d.name, row, 'unreadable run-result.json'))
            continue

        pr = res.get('platform_result')
        defect = harness_defect(d)
        if defect:
            excluded.append((d.name, row, pr, defect))
            continue
        if pr not in VALID:
            unusable.append((d.name, row, pr))
            continue
        if res.get('capture_valid') is not True or res.get('measurement_valid') is not True:
            unusable.append((d.name, row, f'{pr} (capture/measurement invalid)'))
            continue
        if not row:
            # A qualification or diagnostic run: real, but not a matrix cell.
            if args.verbose:
                print(f'  note: {d.name} is not a matrix row; ignored')
            continue
        cells[(row['workload_id'], row['platform'])].append(
            (row['repetition'], row['ordinal'], d.name, pr))

    print(f'\n{"cell":<16} {"n":<3} {"status":<8} repetitions')
    print('-' * 96)
    short = []
    for key in sorted(cells):
        v = sorted(cells[key])
        n = len(v)
        ok = n == args.expect
        if not ok:
            short.append(key)
        reps = ', '.join(
            f"r{rep}@o{ord_}:{'S' if pr == 'VALID_PLATFORM_SUCCESS' else 'F'}"
            for rep, ord_, _, pr in v)
        print(f'  {key[0] + "/" + key[1]:<16} {n:<3} {"OK" if ok else "SHORT":<8} {reps}')

    print(f'\ncells: {len(cells)}  expected: {args.expect} reps each')
    if short:
        print('cells not at target:')
        for key in short:
            have = len(cells[key])
            print(f'  {key[0]}/{key[1]}: {have}/{args.expect}')

    if excluded:
        print(f'\nharness defects excluded from repetition sets: {len(excluded)}')
        for name, row, pr, defect in excluded:
            ord_ = row['ordinal'] if row else '?'
            print(f'  {name} (ordinal {ord_}): has {pr} but was a harness defect')
    if unusable:
        print(f'\nunusable attempts (spent IDs, kept out of every denominator): {len(unusable)}')
        for name, row, why in unusable:
            ord_ = row['ordinal'] if row else 'not in matrix'
            cell = f'{row["workload_id"]}/{row["platform"]}' if row else '-'
            print(f'  {name} (ordinal {ord_}, {cell}): {why}')

    return 0 if not short else 1


if __name__ == '__main__':
    sys.exit(main())
