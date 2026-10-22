#!/usr/bin/env python3
"""Write the diagnostics-round-1 plans (workloads/plans/diag-*.json) from the exploratory plans.

Each plan: warm-up 120 s, then one 180 s constant-rate step; VU pool ceil(rate x 1.5 s); 1000 devices round-robin.
Deviations = the exploratory plan's deviations (target, outbound, procedure) + the DIAG labels of the run.
  scripts/diag/make-plans.py
"""
import json
import math

PLANS = "workloads/plans/"
DIAG = "DIAG run (docs/hypotheses.md, diagnostics round 1): not a capacity result."
PGSTAT = ("DIAG ONLY: Postgres statistics on (edgex/diag/compose.diag-pgstat.yaml: pg_stat_statements, "
          "track_io_timing, track_wal_io_timing). Durability unchanged.")
NOSYNC = ("DIAG ONLY, DURABILITY WEAKENED: synchronous_commit=off (edgex/diag/compose.diag-nosync.yaml). Persisted "
          "data may be lost on a crash; never used outside DIAG runs (D25).")
JMX = "DIAG ONLY: Kafka JMX port 9999 (thingsboard/diag/compose.diag-kafka-jmx.yaml) to read request metrics."
NOSINK = {
    "edgex": "DIAG observer run WITHOUT the sink path: app-http-export service stopped; no latency observation.",
    "thingsboard": "DIAG observer run WITHOUT the sink path: 'bench sink' REST node removed from the root rule chain.",
    "fiware": "DIAG observer run WITHOUT the sink path: no Orion subscription.",
}
ORION_WAIT = ("FIWARE: the benchmark waits 65 s after creating the Orion subscription (replicated Orion refreshes its "
              "subscription cache every 60 s, -subCacheIval default; scripts/outbound.sh). Source: fiware-orion 4.5.0 "
              "doc/manuals/admin/cli.md (-subCacheIval) and perf_tuning.md#subscription-cache.")
MONGO_PROFILE = "DIAG: MongoDB profiler (level 2) on for 60 s during the step (adds MongoDB CPU in that minute)."


def plan(platform, k, name, rate, warm_rate, extra):
    base = json.load(open(f"{PLANS}expl-{platform}-k{k}.json"))
    d = dict(base)
    d["plan_id"] = name
    d["purpose"] = f"{DIAG} {name}: warm-up 120 s at {warm_rate}, then 180 s at {rate} msg/s."
    d["schedule"] = {"type": "steps", "steps": [
        {"phase": "warmup", "rate": warm_rate, "duration_s": 120, "vus": max(50, math.ceil(warm_rate * 1.5))},
        {"phase": "sweep", "rate": rate, "duration_s": 180, "vus": max(50, math.ceil(rate * 1.5))}]}
    d["deviations"] = base.get("deviations", []) + [DIAG] + extra
    path = f"{PLANS}{name}.json"
    with open(path, "w") as f:
        f.write(json.dumps(d, indent=2) + "\n")
    print(path)


for k, rate in [(4, 500), (1, 200)]:
    plan("edgex", k, f"diag-edgex-k{k}-r{rate}", rate, 50, [PGSTAT])
    plan("edgex", k, f"diag-edgex-k{k}-r{rate}-nosync", rate, 50, [PGSTAT, NOSYNC])
plan("edgex", 4, "diag-edgex-k4-obs-sink-r200", 200, 50, [])
plan("edgex", 4, "diag-edgex-k4-obs-nosink-r200", 200, 50, [NOSINK["edgex"]])
plan("thingsboard", 4, "diag-tb-obs-sink-r100", 100, 50, [JMX])
plan("thingsboard", 4, "diag-tb-obs-nosink-r100", 100, 50, [JMX, NOSINK["thingsboard"]])
plan("fiware", 4, "diag-fiware-obs-nosink-r50", 50, 50, [NOSINK["fiware"]])
plan("fiware", 4, "diag-fiware-obs-sink-r50", 50, 50, [ORION_WAIT])
plan("fiware", 4, "diag-fiware-k4-r100", 100, 100, [ORION_WAIT, MONGO_PROFILE])
plan("fiware", 4, "diag-fiware-k4-r100-cpuprof", 100, 100, [ORION_WAIT,
     "DIAG: V8 CPU profiler (inspector via SIGUSR1) on fiware-iot-agent-1 for 60 s at steady load "
     "(scripts/diag/node-cpuprof.js); adds CPU to that replica in that minute."])
plan("fiware", 4, "diag-fiware-k4-r100-cpuprof2", 100, 100, [ORION_WAIT,
     "DIAG: V8 CPU profiler (inspector via SIGUSR1 sent from the host) on fiware-iot-agent-1 for 60 s at steady load "
     "(scripts/diag/node-cpuprof.js); retry of DIAG-fiware-k4-r100-cpuprof, whose image has no kill binary."])
