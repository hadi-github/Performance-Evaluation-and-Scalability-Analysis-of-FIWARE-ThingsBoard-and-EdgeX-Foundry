#!/usr/bin/env python3
"""Emit the frozen ordinary-cell plans for the feasible campaign.

One plan per platform x repetition, derived only from frozen-scope.json. The
files are evidence: regenerate only to reproduce the frozen matrix, never to
tune a cell after seeing results.

    python3 results/feasible-campaign-20261106/make-cell-plans.py [--check]
"""
import argparse
import hashlib
import json
import pathlib
import sys

HERE = pathlib.Path(__file__).resolve().parent
SCOPE = json.loads((HERE / "frozen-scope.json").read_text())
OUT = HERE / "plans"
PREFIX = {"thingsboard": "tb", "fiware": "fw", "edgex": "ex"}
TARGETS = {
    "thingsboard": {"base_url": "http://87.107.191.107:8080"},
    "fiware": {"base_url": "http://87.107.191.107:1026",
               "north_url": "http://87.107.191.107:4041",
               "south_url": "http://87.107.191.107:7896"},
    "edgex": {"base_url": "http://87.107.191.107:59986",
              "metadata_url": "http://87.107.191.107:59881",
              "ingress_url": "http://87.107.191.107:59986"},
}
PILOT = {
    "rate_per_second": 25,
    "measurement_seconds": 600,
    "warmup_seconds": 300,
    "drain_seconds_after_measurement": 900,
    "quiet_interval_seconds": 30,
    "late_observation_seconds": 30,
}


def plan_for(cell, rep):
    platform = cell["platform"]
    seed = SCOPE["seed_by_repetition"][rep - 1]
    run_id = f"disp-{PREFIX[platform]}-r{rep:02d}-20261106"
    return {
        "schema_version": 1,
        "plan_id": f"ordinary-{platform}-r{rep:02d}-20261106",
        "purpose": ("Frozen ordinary cell of the feasible-dispatch batch: one "
                    "independent repetition; not a capacity, maximum-throughput or "
                    "scalability measurement."),
        "pilot_only": True,
        "seed": seed,
        "adapter": platform,
        "payload": {"encoding_id": "logical-json-ascii-v1", "size_offset_bytes": 0},
        "target": TARGETS[platform],
        "devices": cell["devices"],
        "request_timeout_ms": 5000,
        "connection": {"keep_alive": True},
        "arrivals": {"model": "constant"},
        "vus": {"pre_allocated": 150, "max": 150},
        "schedule": {"type": "steps", "steps": [
            {"phase": "warmup", "rate": cell["rate_per_second"],
             "duration_s": cell["warmup_seconds"], "vus": 150},
            {"phase": "measurement", "rate": cell["rate_per_second"],
             "duration_s": cell["measurement_seconds"], "vus": 150},
        ]},
        "health": {"workload_policy": SCOPE["workload_policy"],
                   "min_achieved_ratio": 0.99, "send_lag_p99_ms": 0,
                   "early_tolerance_ms": 2, "max_core_busy": 0},
        "cleanup": False,
        "test_faults": {},
        "protocol_pilot_parameters": dict(PILOT),
        "deviations": [
            "Warm-up and measurement both run at the frozen ordinary rate.",
            "Send lag is diagnostic under dispatch-integrity-v1; dispatch-to-complete-body "
            "latency is computed offline per message.",
            "Shared deployed database state is retained and disclosed; not cold-start IID replication.",
        ],
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true", help="verify files match without writing")
    args = ap.parse_args()
    problems = []
    produced = {}
    for cell in SCOPE["ordinary_cells"]:
        for rep in range(1, cell["independent_run_repetitions"] + 1):
            run_id = f"disp-{PREFIX[cell['platform']]}-r{rep:02d}-20261106"
            obj = plan_for(cell, rep)
            path = OUT / f"{run_id}.json"
            produced[path] = json.dumps(obj, indent=2) + "\n"
    # Separate, bounded adapter-specific headroom checks. These are not
    # repetitions of the ordinary campaign and do not change its frozen cells.
    for platform in ('fiware', 'edgex'):
        obj = json.loads((HERE / 'qualification-plan.json').read_text())
        obj['adapter'] = platform
        obj['target'] = TARGETS[platform]
        obj['plan_id'] = f'dispatch-endpoint-headroom-{platform}-20261106'
        obj['purpose'] = (f'Bounded {platform} 50/s adapter-specific headroom check '
                          'for the frozen 25/s ordinary scope; not a campaign repetition '
                          'or maximum-capacity measurement.')
        produced[HERE / f'qualification-{platform}-plan.json'] = json.dumps(obj, indent=2) + '\n'
    matrix = {'schema_version':1,'scope_id':SCOPE['scope_id'],
              'frozen_scope_sha256':hashlib.sha256((HERE/'frozen-scope.json').read_bytes()).hexdigest(),
              'ordinary_runs':[]}
    for rep, order in enumerate(SCOPE['platform_order_by_repetition'],1):
        for platform in order:
            rid=f'disp-{PREFIX[platform]}-r{rep:02d}-20261106'
            matrix['ordinary_runs'].append({'ordinal':len(matrix['ordinary_runs'])+1,
                'repetition':rep,'platform':platform,'run_id':rid,
                'seed':SCOPE['seed_by_repetition'][rep-1],'plan':f'plans/{rid}.json'})
    produced[HERE/'run-matrix.json'] = json.dumps(matrix,indent=2)+'\n'
    for path, text in produced.items():
        if args.check:
            if not path.exists() or path.read_text() != text:
                problems.append(str(path))
        else:
            if path.exists() and path.read_text() != text:
                raise SystemExit(f"refusing to rewrite existing plan with different content: {path}")
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text)
    if args.check and problems:
        raise SystemExit("plans out of date:\n  " + "\n  ".join(problems))
    if not args.check:
        print(f"verified/wrote {len(produced)} frozen plan and matrix artifacts under {HERE}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
