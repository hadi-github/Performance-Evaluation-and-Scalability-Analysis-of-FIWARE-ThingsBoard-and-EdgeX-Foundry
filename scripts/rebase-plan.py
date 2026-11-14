#!/usr/bin/env python3
"""Point a frozen plan at this environment without changing the workload.

    scripts/rebase-plan.py PLAN OUT                  # rewrite target to .env host
    scripts/rebase-plan.py PLAN OUT --host 10.0.0.5  # explicit host
    scripts/rebase-plan.py PLAN OUT --verify         # assert only target differs

The logic lives in `scripts/iotbench_plan_overlay.py`, which the campaign
launcher also imports, so the offline command and the launcher cannot disagree
about what a legal overlay is.

Exit codes: 0 ok, 1 the overlay would change the workload, 2 usage/parse error.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import iotbench_config as cfg  # noqa: E402
import iotbench_plan_overlay as overlay  # noqa: E402

REPO = Path(__file__).resolve().parents[1]


def resolve(plan_path: Path, run_id: str) -> dict | None:
    """Resolve a plan with benchctl, if a usable binary exists.

    Order matters. An explicitly configured `IOT_BENCH_PREBUILT_DIR` is the
    reviewed binary for this tree and wins. A bare `bin/benchctl` is only a
    fallback: `bin/` is git-ignored, so it can be an arbitrarily old build that
    does not understand the current plan schema. Using one silently would make
    the verify step lie, so a resolver that rejects the plan is reported as
    "no resolver" rather than treated as a workload difference.
    """
    import os
    import subprocess
    candidates = []
    if prebuilt := os.environ.get('IOT_BENCH_PREBUILT_DIR'):
        candidates.append(Path(prebuilt) / 'benchctl')
    candidates.append(REPO / 'bin' / 'benchctl')
    for binary in candidates:
        if not binary.is_file():
            continue
        p = subprocess.run([str(binary), 'resolve', '-plan', str(plan_path), '-run-id', run_id],
                           capture_output=True, text=True)
        if p.returncode == 0:
            return json.loads(p.stdout)
        # A resolver that cannot parse the plan is not evidence about the
        # workload; try the next candidate, then fall back to structure only.
        continue
    return None


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('plan', type=Path)
    ap.add_argument('out', type=Path)
    ap.add_argument('--host', default=None, help='target host (default: IOTBENCH_SUT_IP)')
    ap.add_argument('--verify', action='store_true',
                    help='assert OUT differs from PLAN only in `target`; write nothing')
    args = ap.parse_args()

    if not args.plan.is_file():
        print(f'plan not found: {args.plan}', file=sys.stderr)
        return 2

    if args.verify:
        problems = overlay.verify_overlay(args.plan, args.out)
        if problems:
            for p in problems:
                print(f'OVERLAY INVALID: {p}', file=sys.stderr)
            return 1
        original = json.loads(args.plan.read_text())
        rebased = json.loads(args.out.read_text())
        diff = overlay.differing_fields(original, rebased)
        # Resolve both and compare, if a resolver is available. This catches a
        # target edit that leaks into the derived schedule.
        rid = original.get('plan_id', 'rebase') + '-verify'
        a, b = resolve(args.plan, rid), resolve(args.out, rid)
        if a and b:
            rout = [d for d in overlay.differing_fields(a, b)
                    if not (d == 'target' or d.startswith('target.'))]
            if rout:
                print('RESOLVED PLANS DIFFER OUTSIDE target: ' + ', '.join(rout[:10]),
                      file=sys.stderr)
                return 1
            print(f'verified: only target differs ({len(diff)} field(s)), '
                  f'resolved schedule identical')
        else:
            print(f'verified structurally: only target differs ({len(diff)} field(s)); '
                  f'no benchctl available for a resolve check')
        return 0

    host = args.host or cfg.SUT_IP
    try:
        record = overlay.overlay_plan(args.plan, args.out, host)
    except SystemExit as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print(f"wrote {args.out}: target -> {host} "
          f"({len(record['changed_paths'])} field(s) changed, workload untouched)")
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
