#!/usr/bin/env python3
"""Regression-based service demands from explore.json (docs/exploratory-capacity.md, "Service demands").

  scripts/demand-fit.py [--json] results/exploratory/<run> [...]

For every container and every cpuset pool, fit cores_used = a + D x rate by ordinary least squares over the
passing sweep steps (warm-up excluded; for runs with a persistence series the D23 persistence verdict decides
"passing", otherwise the sink verdict). a = fixed cores at zero load, D = marginal CPU per message (reported in
ms). Predicted pool capacity = (pool cores - a_pool) / D_pool, with a_pool, D_pool the fit of the pool's summed
cores (equal to the sums of the container fits). The knee interval is (highest passing rate, first failing rate].
A run is CPU-bound if the smallest pool prediction lies inside the knee interval or within 25 % of it.

Each fit has only as many points as passing steps (2-4 here): R^2 with 2 points is 1 by construction. Method:
the regression approach in Spinner, Casale, Brosig, Kounev, "Evaluating approaches to resource demand
estimation", Performance Evaluation 92 (2015). The utilization-law columns (D_i = CPU-s / messages of one step)
are kept in scripts/explore-tables.py, labelled.
"""
import json
import sys

TOL = 0.25


def ols(xs, ys):
    n = len(xs)
    mx, my = sum(xs) / n, sum(ys) / n
    sxx = sum((x - mx) ** 2 for x in xs)
    if n < 2 or sxx == 0:
        return None
    d = sum((x - mx) * (y - my) for x, y in zip(xs, ys)) / sxx
    a = my - d * mx
    ss_tot = sum((y - my) ** 2 for y in ys)
    ss_res = sum((y - (a + d * x)) ** 2 for x, y in zip(xs, ys))
    r2 = 1 - ss_res / ss_tot if ss_tot > 0 else float("nan")
    return {"a_cores": a, "D_ms": d * 1000, "r2": r2, "n": n}


def cores(cs):
    n = 0
    for part in cs.split(","):
        lo, _, hi = part.partition("-")
        n += int(hi) - int(lo) + 1 if hi else 1
    return n


def pool_use_at(d, x, rate):
    """Measured cores used per cpuset over the step at `rate` (first step at that rate after the warm-up), from the
    CPU snapshot TSV beside the run directory (<run>.cpu.tsv): usage difference of the snapshots nearest to the step
    start and end, per container (same ID and start time at both ends), summed per cpuset."""
    try:
        lines = [l.split("\t") for l in open(d + ".cpu.tsv").read().splitlines()]
    except OSError:
        return None
    summ = json.load(open(d + "/k6-summary.json"))
    res = json.load(open(d + "/resolved.json"))
    idx = next((s["index"] for s in x["steps"] if s["rate"] == rate and s["verdict"] != "warm-up"), None)
    if idx is None:
        return None
    t0 = summ["steps"][idx]["scenario_start_ms"] / 1000
    t1 = t0 + res["steps"][idx]["duration_s"]
    snaps = {}
    for f in lines:
        if len(f) < 6 or f[3] in ("", "NA"):
            continue
        snaps.setdefault(f[1], []).append((float(f[0]), f[2], int(f[3]), f[4], f[5]))
    use = {}
    for c, ss in snaps.items():
        a = min(ss, key=lambda v: abs(v[0] - t0))
        b = min(ss, key=lambda v: abs(v[0] - t1))
        if a[1] != b[1] or a[3] != b[3] or b[0] <= a[0]:
            continue  # restarted or replaced inside the step: incomplete
        use[a[4]] = use.get(a[4], 0) + (b[2] - a[2]) / 1e6 / (b[0] - a[0])
    return use


def fit_run(d):
    e = json.load(open(d + "/explore.json"))
    x = e["explore"]
    use_persist = x.get("persistence") is not None
    key = "verdict_persistence" if use_persist else "verdict"
    passing = [s for s in x["steps"] if s.get(key) == "PASS" and s.get("cpu")]
    if use_persist:
        hi_pass, first_fail = x["persistence"]["highest_passing_rate"], x["persistence"]["first_failing_rate"]
    else:
        hi_pass, first_fail = x["highest_passing_rate"], x["first_failing_rate"]
    rates = [s["rate"] for s in passing]
    used = {}  # container -> [(rate, cores used)]
    cpuset = {}
    for s in passing:
        dur = s["attempted"] / s["rate"]
        for c in s["cpu"]:
            if not c["complete"]:
                continue
            used.setdefault(c["container"], []).append((s["rate"], c["cpu_seconds"] / dur))
            cpuset[c["container"]] = c["cpuset"]
    containers = {}
    for n, pts in sorted(used.items()):
        f = ols([p[0] for p in pts], [p[1] for p in pts])
        if f:
            containers[n] = dict(f, cpuset=cpuset[n])
    pools = {}
    for cs in sorted(set(cpuset.values())):
        members = [n for n in used if cpuset[n] == cs and len(used[n]) == len(passing)]
        ys = [sum(dict(used[n])[r] for n in members) for r in rates]
        f = ols(rates, ys)
        if f:
            dcap = f["D_ms"] / 1000
            f["cores"] = cores(cs)
            f["containers"] = sorted(members)
            f["predicted_capacity"] = (f["cores"] - f["a_cores"]) / dcap if dcap > 0 else float("inf")
            pools[cs] = f
    binding = min(pools, key=lambda cs: pools[cs]["predicted_capacity"]) if pools else None
    pred = pools[binding]["predicted_capacity"] if binding else None
    if pred is None or not first_fail:
        cls = "not classifiable (no fit or no failing step)"
    elif hi_pass * (1 - TOL) <= pred <= first_fail * (1 + TOL):
        cls = "CPU-bound"
    else:
        cls = "not CPU-bound"
    at_fail = pool_use_at(d, x, first_fail) if first_fail else None
    return {"run_id": x["run_id"], "passing_basis": key, "pool_cores_used_at_first_fail": at_fail, "passing_rates": rates, "knee": [hi_pass, first_fail],
            "containers": containers, "pools": pools, "binding_pool": binding, "predicted_capacity": pred,
            "classification": cls}


def md(f):
    out = [f"### {f['run_id']}: demand fit over {len(f['passing_rates'])} passing steps "
           f"({', '.join(f'{r:g}' for r in f['passing_rates'])} msg/s; basis `{f['passing_basis']}`)", "",
           "| Container | cpuset | a (cores at 0 msg/s) | D (ms CPU / msg) | R² | n |", "|---|---|---|---|---|---|"]
    for n, c in f["containers"].items():
        out.append(f"| {n} | {c['cpuset']} | {c['a_cores']:.3f} | {c['D_ms']:.3f} | {c['r2']:.3f} | {c['n']} |")
    out += ["", "| Pool (cpuset) | cores | Σa (cores) | ΣD (ms / msg) | R² | n | predicted capacity (msg/s) |",
            "|---|---|---|---|---|---|---|"]
    for cs, p in f["pools"].items():
        mark = " **(binding)**" if cs == f["binding_pool"] else ""
        out.append(f"| {cs}{mark} | {p['cores']} | {p['a_cores']:.3f} | {p['D_ms']:.3f} | {p['r2']:.3f} | {p['n']} "
                   f"| {p['predicted_capacity']:.0f} |")
    lo, hi = f["knee"]
    pred = f["predicted_capacity"]
    out += ["", f"Observed knee: ({lo:g}, {hi:g}] msg/s. Smallest pool prediction: "
            + (f"{pred:.0f} msg/s ({f['binding_pool']})" if pred is not None else "none")
            + f". Classification (±25 %): **{f['classification']}**."]
    if f["pool_cores_used_at_first_fail"]:
        u = f["pool_cores_used_at_first_fail"]
        out.append("Measured pool use during the first failing step (" + f"{hi:g} msg/s): "
                   + ", ".join(f"cpuset {cs} {u[cs]:.2f}/{f['pools'][cs]['cores']} cores" for cs in sorted(u) if cs in f["pools"]) + ".")
    out.append("")
    return "\n".join(out)


if __name__ == "__main__":
    args = sys.argv[1:]
    as_json = "--json" in args
    fits = [fit_run(d.rstrip("/")) for d in args if d != "--json"]
    print(json.dumps(fits, indent=1) if as_json else "\n".join(md(f) for f in fits))
