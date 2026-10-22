#!/usr/bin/env python3
"""Markdown tables for docs/exploratory-capacity.md from results/exploratory/<run>/explore.json.

  scripts/explore-tables.py results/exploratory/EXPL-edgex-k4 [...]

Per run: the step table (verdict, generator health, per-minute ack p99, outstanding work, reasons) and the CPU
table (cores used per container per step, D_i, B_i, pool bounds). Every number is copied from explore.json.
"""
import json
import sys


def f(x, nd=1):
    return "-" if x is None else f"{x:.{nd}f}"


def run_tables(d):
    e = json.load(open(d + "/explore.json"))
    x = e["explore"]
    out = [f"### {x['run_id']}", ""]
    if e.get("run_verdict"):
        out += [f"**Run verdict:** {e['run_verdict']}", ""]
    out += ["| Step | Rate | Verdict | Unsucc. | Send-lag p99 (ms) | Ack p99 min 1/2/3 (ms) | Outstanding min 1/2/3 "
            "| Smallest B_i (msg/s) | Reasons |", "|---|---|---|---|---|---|---|---|---|"]
    for s in x["steps"]:
        mins = "/".join(f(m["ack_p99_ms"], 0) for m in s.get("minutes") or [])
        outs = "/".join(f(o) for o in s.get("outstanding_avg_last10s_by_minute") or [])
        b = f"{f(s.get('utilization_law_bound'), 0)} ({s['bottleneck_container']})" if s.get("bottleneck_container") else "-"
        out.append(f"| {s['name']} | {s['rate']:g} | {s['verdict']} | {s['unsuccessful_ratio'] * 100:.4f}% "
                   f"| {f(s['send_lag_p99_ms'])} | {mins} | {outs} | {b} | {'; '.join(s.get('reasons') or [])} |")
    rec = e.get("history_reconciliation")
    if rec is None:
        hist = "not reconcilable (no history)"
    elif isinstance(rec, str):
        hist = rec
    else:
        hist = (f"stored unique {rec['stored_unique']}, ack_lost {len(rec['ack_lost'] or [])}, "
                f"duplicates {len(rec['duplicates'] or [])}, orphans {len(rec['orphans'] or [])}")
    out += ["", f"Sink: {x['planned_reached_sink']} of {x['planned_messages']} planned reached, {x['sink_duplicates']} "
            f"duplicate receipts. History reconciliation (whole run): {hist}.", ""]

    steps = [s for s in x["steps"] if s.get("cpu")]
    if steps:
        names = sorted({c["container"] for s in steps for c in s["cpu"]})
        dur = {s["name"]: s["attempted"] / s["rate"] for s in steps}
        out += ["CPU per container (cores used = CPU-s / step duration; D_i in ms CPU per message; B_i = allocated "
                "cores / D_i):", "",
                "| Container | " + " | ".join(f"{s['rate']:g}/s" for s in steps) + " |",
                "|---|" + "---|" * len(steps)]
        for n in names:
            cells = []
            for s in steps:
                c = next((c for c in s["cpu"] if c["container"] == n), None)
                if c is None:
                    cells.append("-")
                else:
                    star = "" if c["complete"] else " (incomplete)"
                    cells.append(f"{c['cpu_seconds'] / dur[s['name']]:.2f} c, D {c['D_cpu_s_per_msg'] * 1000:.3f}, "
                                 f"B {c['B_msgs_per_s']:.0f}{star}")
            out.append(f"| {n} | " + " | ".join(cells) + " |")
        out += ["", "| Pool bound (cores / sum D) | " + " | ".join(f"{s['rate']:g}/s" for s in steps) + " |",
                "|---|" + "---|" * len(steps)]
        for pb in steps[0]["pool_bounds"]:
            cells = []
            for s in steps:
                p = next(p for p in s["pool_bounds"] if p["cpuset"] == pb["cpuset"])
                cells.append(f"{p['bound_msgs_per_s']:.0f}")
            out.append(f"| cpuset {pb['cpuset']} ({pb['cores']} c) | " + " | ".join(cells) + " |")
        out.append("")
    return "\n".join(out)


if __name__ == "__main__":
    print("\n".join(run_tables(d.rstrip("/")) for d in sys.argv[1:]))
