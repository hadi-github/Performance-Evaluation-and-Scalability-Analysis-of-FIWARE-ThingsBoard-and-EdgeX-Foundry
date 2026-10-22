#!/usr/bin/env python3
"""Generate docs/diagnostics.md (checkpoint report, diagnostics round 1). Every number comes from files under
results/ (explore.json, *.diag.json, sut/ captures) or from scripts/demand-fit.py / scripts/diag/cpuprof-summary.py.

  scripts/diag/report.py > docs/diagnostics.md        (run from the repository root, results/ copied locally)
"""
import json
import re
import subprocess
import sys

EX, DG = "results/exploratory", "results/diag"


def j(p):
    with open(p) as f:
        return json.load(f)


def diag(i):
    return j(f"{DG}/{i}.diag.json")


def f(x, nd=1):
    return "-" if x is None else f"{x:,.{nd}f}"


def pct(x, nd=1):
    return "-" if x is None else f"{100 * x:.{nd}f} %"


def out(s=""):
    print(s)


def table(header, rows):
    out("| " + " | ".join(header) + " |")
    out("|" + "---|" * len(header))
    for r in rows:
        out("| " + " | ".join(str(c) for c in r) + " |")
    out()


def cpu(r, c):
    return r["cpu_per_container"].get(c, {})


# ------------------------------------------------------------------ data
ex = {k: j(f"{EX}/EXPL-edgex-{k}/explore.json") for k in ("k4", "k1")}
fits = json.loads(subprocess.run([sys.executable, "scripts/demand-fit.py", "--json"] +
                                 [f"{EX}/{r}" for r in ("EXPL-edgex-k4", "EXPL-edgex-k1", "EXPL-tb-k4b", "EXPL-fiware-k4")],
                                 capture_output=True, text=True, check=True).stdout)
fit = {x["run_id"]: x for x in fits}
fit_md = subprocess.run([sys.executable, "scripts/demand-fit.py"] +
                        [f"{EX}/{r}" for r in ("EXPL-edgex-k4", "EXPL-edgex-k1", "EXPL-tb-k4b", "EXPL-fiware-k4")],
                        capture_output=True, text=True, check=True).stdout
prof = json.loads(subprocess.run([sys.executable, "scripts/diag/cpuprof-summary.py",
                                  f"{DG}/sut/DIAG-fiware-k4-r100-cpuprof3/iot-agent-1.cpuprofile", "--json"],
                                 capture_output=True, text=True, check=True).stdout)
E = {i: diag(i) for i in ("DIAG-edgex-k4-r500b", "DIAG-edgex-k4-r500-nosync", "DIAG-edgex-k1-r200",
                          "DIAG-edgex-k1-r200-nosync", "DIAG-edgex-k4-obs-sink-r200", "DIAG-edgex-k4-obs-nosink-r200",
                          "DIAG-edgex-k4-r500")}
EXP = {i: j(f"{DG}/{i}/explore.json") for i in E if __import__("os").path.exists(f"{DG}/{i}/explore.json")}
T = {i: diag(i) for i in ("DIAG-tb-obs-sink-r100", "DIAG-tb-obs-nosink-r100")}
TI = {i: diag(i) for i in ("DIAG-tb-idle2", "DIAG-tb-idle")}
F = {i: diag(i) for i in ("DIAG-fiware-obs-nosink-r50", "DIAG-fiware-obs-sink-r50", "DIAG-fiware-k4-r100",
                          "DIAG-fiware-k4-r100-cpuprof", "DIAG-fiware-k4-r100-cpuprof3", "DIAG-fiware-k4-r100-cpuprof2")}
FEXP = {i: j(f"{DG}/{i}/explore.json") for i in F if __import__("os").path.exists(f"{DG}/{i}/explore.json")}
procmodel = open(f"{DG}/sut/fiware-static/DIAG-fiware-procmodel.txt").read()
fidle = dict(re.findall(r"^(\S+)\s+([\d.]+) cores", open(f"{DG}/sut/fiware-static/DIAG-fiware-idle-60s.txt").read(), re.M))
tbjvm = open(f"{DG}/sut/DIAG-tb-idle2/tb-jvm.txt").read()
rc_with = j(f"{DG}/sut/DIAG-tb-obs-sink-r100/tb-rulechain-with-sink.json")
rc_without = j(f"{DG}/sut/DIAG-tb-obs-nosink-r100/tb-rulechain-without-sink.json")
def lines(p):
    with open(p) as fh:
        return sum(1 for _ in fh)


# the k4 run's first history export was taken while persistence was still draining; the -final export is the complete one
hist = {k: lines(next(p for p in (f"{EX}/EXPL-edgex-{k}.history-final.ndjson", f"{EX}/EXPL-edgex-{k}.history.ndjson")
                      if __import__("os").path.exists(p))) for k in ("k4", "k1")}
other_pass = {r: j(f"{EX}/{r}/explore.json")["explore"]["highest_passing_rate"] for r in ("EXPL-tb-k4b", "EXPL-fiware-k4")}
k1_500 = next(s for s in ex["k1"]["explore"]["steps"] if s["rate"] == 500 and s["verdict"] != "warm-up")
k1_drop_outstanding = k1_500["outstanding_persistence_avg_last10s_by_minute"][0]
stored = {i: lines(f"{DG}/{i}.history.ndjson") for i in ("DIAG-edgex-k4-obs-sink-r200", "DIAG-edgex-k4-obs-nosink-r200",
                                                         "DIAG-tb-obs-sink-r100", "DIAG-tb-obs-nosink-r100")}
planned = {i: j(f"{DG}/{i}/resolved.json")["total_ticks"] for i in stored}
devices_per_run = j(f"{DG}/DIAG-fiware-k4-r100/resolved.json").get("devices") or j("workloads/plans/diag-fiware-k4-r100.json")["devices"]
# FIWARE runs in order since the cold starts (each provisions devices_per_run devices)
fiware_order = [["DIAG-fiware-obs-nosink-r50", "DIAG-fiware-obs-sink-r50", "DIAG-fiware-k4-r100", "DIAG-fiware-k4-r100-cpuprof",
                 "DIAG-fiware-k4-r100-cpuprof2"], ["DIAG-fiware-k4-r100-cpuprof3"]]
provisioned = {r: devices_per_run * (n + 1) for grp in fiware_order for n, r in enumerate(grp)}
export_mem = next(int(l.split("\t")[3]) for l in open(f"{DG}/sut/DIAG-edgex-k4-obs-sink-r200/containers.tsv")
                  if l.startswith("edgex-app-http-export"))
git = subprocess.run(["git", "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip()


def persist_first_fail(x):
    p = x["explore"]["persistence"]
    st = next(s for s in x["explore"]["steps"] if s["rate"] == p["first_failing_rate"] and s["verdict"] != "warm-up")
    return p, st


def jvm_flag(container, flag):
    sec = tbjvm.split("== " + container)[1].split("== ")[0]
    m = re.search(rf"-XX:{flag}=(\d+)", sec)
    return int(m.group(1)) if m else None


def jvm_has(container, flag):
    sec = tbjvm.split("== " + container)[1].split("== ")[0]
    return flag in sec


# rule chain wiring: Save Timeseries -> (Success) -> bench sink
def wiring(rc):
    names = [n["name"] for n in rc["nodes"]]
    if "bench sink" not in names:
        return None
    si, bi = names.index("Save Timeseries"), names.index("bench sink")
    return [c["type"] for c in rc["connections"] if c["fromIndex"] == si and c["toIndex"] == bi]


# ------------------------------------------------------------------ derived facts
e4, e1 = persist_first_fail(ex["k4"]), persist_first_fail(ex["k1"])
k4d, k4n = E["DIAG-edgex-k4-r500b"], E["DIAG-edgex-k4-r500-nosync"]
k1d, k1n = E["DIAG-edgex-k1-r200"], E["DIAG-edgex-k1-r200-nosync"]
prof_mongo = prof["inclusive_share_of_busy"].get("mongoose/mongodb/bson", 0)
prof_ulhttp = prof["inclusive_share_of_busy"].get("UL parser / iotagent-ul", 0) + prof["inclusive_share_of_busy"].get("http client to Orion", 0)
domain_self = sum(v for k, v in prof["top_self_functions"] if k.startswith(("init node:domain", "setUncaughtExceptionCaptureCallback",
                                                                              "registerDestroyHook", "emitHook", "emit node:domain")))
fprof = F["DIAG-fiware-k4-r100"]["mongo_profile"]
collscan_dev = [g for g in fprof["groups"] if g["ns"] == "iotagentul.devices" and g["plan"] == "COLLSCAN"]
tbi = TI["DIAG-tb-idle2"]
tbs, tbn = T["DIAG-tb-obs-sink-r100"], T["DIAG-tb-obs-nosink-r100"]
vk = tbs["valkey"]
eos, eon = E["DIAG-edgex-k4-obs-sink-r200"], E["DIAG-edgex-k4-obs-nosink-r200"]
fos, fon = F["DIAG-fiware-obs-sink-r50"], F["DIAG-fiware-obs-nosink-r50"]


def obs_rows(a, b):
    rows, ta, tb = [], 0, 0
    for c in sorted(set(a["cpu_per_container"]) | set(b["cpu_per_container"])):
        x, y = cpu(a, c).get("ms_cpu_per_msg"), cpu(b, c).get("ms_cpu_per_msg")
        if (x or 0) < 0.01 and (y or 0) < 0.01:
            continue
        rows.append([c, f(x, 3), f(y, 3), f"{(x or 0) - (y or 0):+.3f}"])
        ta += x or 0
        tb += y or 0
    rows.append(["**total**", f(ta, 3), f(tb, 3), f"**{ta - tb:+.3f}**"])
    return rows, ta - tb


eobs_rows, eobs = obs_rows(eos, eon)
tobs_rows, tobs = obs_rows(tbs, tbn)
fobs_rows, fobs = obs_rows(fos, fon)
export_ms = cpu(eos, "edgex-app-http-export-1").get("ms_cpu_per_msg")
re_ms = (cpu(tbs, "tb-tb-rule-engine-1").get("ms_cpu_per_msg") or 0) - (cpu(tbn, "tb-tb-rule-engine-1").get("ms_cpu_per_msg") or 0)
orion_obs = sum((cpu(fos, f"fiware-orion-{i}").get("ms_cpu_per_msg") or 0) - (cpu(fon, f"fiware-orion-{i}").get("ms_cpu_per_msg") or 0) for i in range(1, 5))
mongo_obs = (cpu(fos, "fiware-mongo-1").get("ms_cpu_per_msg") or 0) - (cpu(fon, "fiware-mongo-1").get("ms_cpu_per_msg") or 0)

# ------------------------------------------------------------------ document
out("# Diagnostics round 1: causes of the exploratory capacity limits (CHECKPOINT)")
out()
out(f"Status: **exploratory, checkpoint before any tuning (Step 5 waits for approval).** Generated by "
    f"`scripts/diag/report.py` at commit `{git}` from `results/exploratory/` and `results/diag/` (raw files on "
    "ramonavm1 and a workstation copy). Hypotheses were committed before the runs (`docs/hypotheses.md`); source "
    "citations are in `docs/evidence/source-excerpts.md` (generated, pinned tags). Decision numbers: the brief's "
    "D12-D15 are logged as **D23-D26** (D12-D22 already existed).")
out()
out("## Summary")
out()
table(["Platform", "What limits it (evidence)", "Proposed change (category)"], [
    ["EdgeX", f"Persistence = **WAL flush latency**: one goroutine and one transaction per event per replica; each commit waits "
              f"for `fdatasync` ({f(k4d['postgres']['wal_sync_ms_per_sync'], 2)} ms, disk write await "
              f"{f(k4d['disk_write_await_ms'].get('sda'), 2)} ms). k = 4: {f(k4d['persisted_per_s_in_window'])} events/s with the WAL "
              f"sync path busy {pct(k4d['postgres']['wal_sync_time_share_of_window'], 0)}; `synchronous_commit=off` (DIAG) "
              f"keeps up with 500/s ({f(k4n['persisted_per_s_in_window'])}/s). CPU is not the limit.",
     "No Category A fix in EdgeX itself; options are B (async commit) or topology (more core-data replicas per core). See proposals."],
    ["ThingsBoard", f"**CPU-bound** on the stateful pool (Kafka/Valkey/Postgres) and the app pool; Kafka spends "
                    f"{f(tbi['cores_used']['tb-kafka-1']['cores'], 2)} cores idle; Valkey runs {f(vk['total_calls_per_msg'], 1)} "
                    f"commands per message. Rule-engine heap OOM: no `ExitOnOutOfMemoryError` because our stack does not mount "
                    "the official `/config`.",
     "A: mount the official tb-node conf (deployment defect). A (doc-backed, to test): queue poll intervals. Heap: no official guidance found."],
    ["FIWARE", f"**MongoDB COLLSCAN on the IoT Agent device registry**, {f(sum(g['per_msg'] for g in collscan_dev), 2)} per message "
               f"(~{f(collscan_dev[0]['docsExamined_per_op'], 0)} documents examined each): MongoDB CPU per message grows with the "
               f"number of devices. IoT Agent CPU: {pct(prof_mongo, 0)} of its busy time is mongoose/mongodb/bson (per-measure registry "
               "lookups + `oldCtxt` save).",
     "A: the device index upstream added in iotagent-node-lib 4.8.0 (#1576), not yet in any iotagent-ul release."],
])

# ---------------- Step 1
out("## Step 1: exploratory verdicts under D23 (goodput at persistence)")
out()
out("EdgeX outstanding work at persistence = scheduled ≤ t minus stored ≤ t, with stored(t) from the raw scraped samples of "
    "`pg_stat_user_tables_n_tup_ins{relname=\"reading\"}` (`scripts/persist-series.sh`; the per-id store times were lost with the "
    "databases at the next cold start). Stored = counter − its value at the warm-up start, linear between samples; outstanding "
    "work is the exact time-average over the last 10 s of each minute (`benchctl explore -persist`). The unchanged criterion is "
    "applied; `verdict_persistence` is in each run's `explore.json`.")
out()
rows = []
for k, (p, st) in (("k4", e4), ("k1", e1)):
    x = ex[k]["explore"]
    o = st["outstanding_persistence_avg_last10s_by_minute"]
    rows.append([f"EdgeX {k}", f"{x['highest_passing_rate']:g} / {x['first_failing_rate']:g} ({x['first_failing_verdict']})",
                 f"**{p['highest_passing_rate']:g} / {p['first_failing_rate']:g}** ({p['first_failing_verdict']})",
                 " / ".join(f(v) for v in o), f"{f(o[0])} + {st['rate']:g}",
                 f"{p['points']} pts, {f(p['median_sample_spacing_ms'] / 1000, 0)} s", f(p["stored_at_last_point"], 0)])
table(["Run", "Sink verdict: highest pass / first fail", "Persistence verdict (D23)",
       "Persistence outstanding min 1/2/3 at first fail", "Limit (min 1 + rate × 1 s)", "Series resolution", "Stored at last point"], rows)
def growth(step):
    o = step["outstanding_persistence_avg_last10s_by_minute"]
    return (o[2] - o[0]) / (step["rate"] * 1.0)


margins = {}
for k in ("k4", "k1"):
    ev = [s for s in ex[k]["explore"]["steps"] if s.get("verdict_persistence") in ("PASS", "FAIL")]
    margins[k] = (max(growth(s) for s in ev if s["verdict_persistence"] == "PASS"),
                  min(growth(s) for s in ev if s["verdict_persistence"] == "FAIL"))
out(f"Your expectation is confirmed: k = 4 fails between {e4[0]['highest_passing_rate']:g} and {e4[0]['first_failing_rate']:g}; "
    f"k = 1 fails at {e1[0]['first_failing_rate']:g}. Stored at the last point vs the history export: k = 4 "
    f"{f(e4[0]['stored_at_last_point'], 0)} vs {hist['k4']:,} ({e4[0]['stored_at_last_point'] - hist['k4']:+,.0f}), k = 1 "
    f"{f(e1[0]['stored_at_last_point'], 0)} vs {hist['k1']:,} ({e1[0]['stored_at_last_point'] - hist['k1']:+,.0f}): the baseline is "
    "interpolated at the warm-up start, so readings already flowing (k = 4) or gate readings still being written (k = 1) shift it "
    "by a constant, which cancels in the min 3 − min 1 criterion. Resolution: 5 s scrapes plus Postgres's statistics flush "
    "(≤ 1 s for busy backends). Margin, as growth (min 3 − min 1) ÷ slack (rate × 1 s): passing steps at most "
    f"{f(margins['k4'][0], 2)} (k4) / {f(margins['k1'][0], 2)} (k1), failing steps {f(margins['k4'][1], 1)} / {f(margins['k1'][1], 1)}; "
    "no verdict is close to the threshold of 1.")
out()
out("**Decision section of `docs/exploratory-capacity.md`:** the declared-rule result is unchanged, and D23/D24 now apply: no "
    "platform passes 500 msg/s at persistence (EdgeX k = 4 highest pass "
    f"{e4[0]['highest_passing_rate']:g}, ThingsBoard {other_pass['EXPL-tb-k4b']:g}, FIWARE {other_pass['EXPL-fiware-k4']:g}), so D24's reopen condition (tuned capacity > 450) is "
    "not met by the exploratory data. That doc is updated in this commit.")
out()
w = wiring(rc_with)
out("**Do the other sinks observe after persistence?**")
out()
out(f"- ThingsBoard: yes. Rule-chain metadata captured during `DIAG-tb-obs-sink-r100` "
    f"(`results/diag/sut/DIAG-tb-obs-sink-r100/tb-rulechain-with-sink.json`): `Save Timeseries` → `bench sink` connection types "
    f"{w}. In the source (v4.3.1.6), the node's Success fires from the save callback, which runs when the SQL batch insert "
    "returned (`TbMsgTimeseriesNode` → `TelemetryNodeCallback` → `DefaultTelemetrySubscriptionService.saveTimeseriesInternal` → "
    "`JpaSqlTimeseriesDao` → `TbSqlBlockingQueue`, excerpts). Postgres runs with the default `synchronous_commit=on`.")
out("- FIWARE: yes, with a weaker meaning of \"stored\". Orion 4.5.0 `MongoCommonUpdate.cpp` writes the entity (update or "
    "findAndModify), returns without notifying if the write fails, and only then calls `processSubscriptions` (excerpt, lines "
    "3978-4027). Orion's write concern is *acknowledged* = applied in memory by MongoDB, not journaled (perf_tuning.md, Write "
    "concern), and Orion keeps only the latest value (no history; D9).")
out()

# ---------------- Step 2
out("## Step 2: regression-based service demands")
out()
out("Least squares cores_used = a + D × rate over the passing sweep steps (warm-up excluded; EdgeX uses the D23 persistence "
    "verdict). **Each fit has only 2-4 points** (R² with 2 points is 1 by construction). Method: the regression approach in "
    "Spinner, Casale, Brosig, Kounev, *Evaluating approaches to resource demand estimation*, Performance Evaluation 92 (2015). "
    "The utilization-law columns stay in `docs/exploratory-capacity.md` (labelled).")
out()
out(fit_md)
user = {"EXPL-tb-k4b": ("2-3", 270), "EXPL-fiware-k4": ("4-7", 130), "EXPL-edgex-k4": ("4-7", 690)}
rows = []
for r, (pool, est) in user.items():
    p = fit[r]["pools"][pool]["predicted_capacity"]
    rows.append([r, pool, est, f(p, 0), pct(p / est - 1), "yes" if abs(p / est - 1) > 0.2 else "no"])
table(["Run", "Pool", "Your rough figure", "Fit", "Difference", "> 20 %"], rows)
k1 = fit["EXPL-edgex-k1"]
u = k1["pool_cores_used_at_first_fail"]
out(f"No difference exceeds 20 %. **Rule note (reported, not changed):** the ±25 % rule labels EdgeX k = 1 CPU-bound "
    f"(prediction {f(k1['predicted_capacity'], 0)} vs knee ({k1['knee'][0]:g}, {k1['knee'][1]:g}]), but the measured pool use in the failing "
    f"step was {f(u['4'], 2)} of 1 core and the cause is the WAL flush (Step 3): with a 2-point fit and a knee interval of width "
    "100 %, the rule cannot separate CPU-bound from not CPU-bound here.")
out()

# ---------------- Step 3: FIWARE
out("## Step 3: diagnostics")
out()
out("### FIWARE (k = 4: 4 IoT Agent + 4 Orion replicas on cores 4-7, MongoDB on 2-3)")
out()
iot_procs = len(re.findall(r"node /opt/iotaul/bin/iotagent-ul", procmodel))
nproc = sorted(set(re.findall(r"nproc: (\d+)", procmodel)))
ncpu = sorted(set(re.findall(r'"cpus":(\d+)', procmodel)))
orion_flags = sorted(set(re.findall(r"contextBroker (.+)", procmodel)))
out(f"1. **Process model** (`sut/fiware-static/DIAG-fiware-procmodel.txt`): {iot_procs} node processes over 4 IoT Agent "
    f"containers (one each), `IOTA_MULTI_CORE=false`, `IOTA_LOG_LEVEL=WARN`; `nproc` = {', '.join(nproc)} but Node's "
    f"`os.cpus()` = {', '.join(ncpu)} (it sees the host). Orion: `{orion_flags[0] if orion_flags else '?'}`. Idle CPU over 60 s: "
    f"IoT Agent {', '.join(fidle[c] for c in sorted(fidle) if 'iot-agent' in c)} cores, MongoDB {fidle.get('fiware-mongo-1')} cores.")
out(f"2. **MongoDB profiler** (`DIAG-fiware-k4-r100`, 60 s at 100 msg/s, level 2; {f(fprof['messages_in_window'], 0)} messages): "
    f"{f(fprof['ops_per_msg_total'], 2)} operations per message. COLLSCAN on `iotagentul.devices`: "
    f"{f(sum(g['per_msg'] for g in collscan_dev), 2)} per message, ~{f(collscan_dev[0]['docsExamined_per_op'], 0)} documents examined "
    "per query (one per registered device). The profiler itself adds load; this run's latency is not a result.")
out()
table(["namespace", "op", "plan", "per message", "docs examined / op", "returned / op"],
      [[g["ns"], g["op"], (g["plan"] or "")[:48], f(g["per_msg"], 3), f(g["docsExamined_per_op"], 1), f(g["nreturned_per_op"], 1)]
       for g in fprof["groups"] if g["per_msg"] >= 0.5])
idx = fprof["indexes"]
out(f"   Indexes: `iotagentul.devices` {[list(i['key']) for i in idx['iotagentul.devices']]}; `iotagentul.groups` "
    f"{[list(i['key']) for i in idx['iotagentul.groups']]}; `orion-openiot.entities` has every index perf_tuning.md recommends "
    f"({len(idx['orion-openiot.entities'])} incl. `_id`). The IoT Agent's lookups filter on {{id, apikey, service, subservice}} "
    "(iotagent-ul 3.7.0 `retrieveDevice` → `findOrCreate`, and `storeDeviceField` → `getDevice`), which no index serves in 4.7.0. "
    "iotagent-node-lib 4.8.0 added exactly that index (#1576); iotagent-ul 3.7.0 is the latest release and still ships 4.7.0.")
dev_rows = []
for i in ("DIAG-fiware-k4-r100-cpuprof3", "DIAG-fiware-k4-r100", "DIAG-fiware-k4-r100-cpuprof"):
    n = f"{provisioned[i]:,}"
    v = FEXP[i]["explore"]["steps"][1]
    verdict = v["verdict"] if FEXP[i]["explore"]["planned_reached_sink"] else "n/a (no subscription)"
    dev_rows.append([i, n, f(cpu(F[i], "fiware-mongo-1").get("ms_cpu_per_msg"), 2), verdict,
                     "/".join(f(m["ack_p99_ms"], 0) for m in v["minutes"])])
out()
out(f"   MongoDB CPU per message vs devices provisioned since the last cold start (each run provisions {devices_per_run:,}; same rate 100/s):")
out()
table(["Run", "Devices provisioned", "MongoDB ms CPU / msg", "Step verdict", "ack p99 min 1/2/3 (ms)"], dev_rows)
out("   `DIAG-fiware-k4-r100` had the MongoDB profiler on for 60 of its 180 s, which adds MongoDB CPU; the other two rows had no "
    "MongoDB profiler (`-cpuprof*` profiled one IoT Agent only).")
out()
out(f"3. **Registry reads per measure, cache:** iotagent-node-lib 4.7.0 has **no device or group cache** (no `cache` anywhere in "
    "`lib/`). Every measure: group lookup, device lookup, update to Orion, then the device document is re-read and saved with "
    "`oldCtxt` (set on every update, `entities-NGSI-v2.js:659-662`; `storeLastMeasure` is off and not the cause).")
out(f"   **H-F5 (IoT Agent CPU):** V8 profile of `fiware-iot-agent-1`, {f(prof['duration_s'], 0)} s at 25 measures/s per replica "
    f"(`DIAG-fiware-k4-r100-cpuprof3`, after a cold start): busy {pct(prof['busy_share_of_wall'])} of wall; mongoose/mongodb/bson "
    f"frames in {pct(prof_mongo)} of busy samples (inclusive), UL parser + HTTP client to Orion {pct(prof_ulhttp)}. Node's "
    f"`domain`/async-hook machinery (iotagent-node-lib creates a domain per request) is {pct(domain_self)} of busy self time.")
whole = [[i, F[i].get("orion_openiot_incoming_tx_whole_run"), F[i].get("messages_sent_whole_run"),
          F[i].get("orion_tx_minus_sent_whole_run")] for i in F if F[i].get("orion_openiot_incoming_tx_whole_run")]
out(f"4. **Duplicate check (H-F3):** Orion's incoming transactions for service `openiot` (all replicas, `/admin/metrics`, first "
    "sample before k6 setup to last after the drain) vs messages sent:")
out()
table(["Run", "Orion transactions", "Messages sent", "Difference"], whole)
out("   (`DIAG-fiware-obs-nosink-r50` has no Orion metrics: its collector polled `/metrics`, which Orion 4.5.0 does not serve; fixed "
    "to `/admin/metrics` before the next run.)")
out("5. **Orion flags vs perf_tuning.md (4.5.0):** `-reqMutexPolicy none` (recommended for active-active) ✓; `-dbPoolSize 10` "
    "(default) ✓; recommended entity indexes ✓; `-reqPoolSize 0` (a thread per request; the guide recommends a pool ≈ CPU cores "
    "under high load); `-notificationMode` transient (default; threadpool recommended for high load, but its queue can drop "
    "notifications); `-writeConcern 1` (default).")
rss = {i: F[i]["iot_agent_rss_mb_avg"] for i in ("DIAG-fiware-obs-sink-r50", "DIAG-fiware-k4-r100-cpuprof3", "DIAG-fiware-k4-r100-cpuprof2")}
out("6. **IoT Agent memory (H-F4):** one process per container; average RSS per replica (cAdvisor) "
    + "; ".join(f"{i}: {', '.join(f(v, 0) for v in sorted(r.values()))} MB" for i, r in rss.items())
    + ". Memory grows with load and backlog in a single process; the 512 MB OOM at ≥ 500 msg/s (EXPL) was not reproduced.")
out()

# ---------------- Step 3: ThingsBoard
out("### ThingsBoard (k = 4: 1 tb-core, 1 tb-rule-engine, 1 tb-http-transport on cores 4-7; Kafka, ZooKeeper, Postgres, Valkey on 2-3)")
out()
kr = tbi["kafka_requests_per_s"]
out(f"1. **Idle Kafka (`DIAG-tb-idle2`, {f(tbi['window_s'], 0)} s, no load, ≥ 120 s after start-up):** Kafka "
    f"{f(tbi['cores_used']['tb-kafka-1']['cores'], 3)} cores; {f(tbi['kafka_requests_per_s_total'], 0)} requests/s ("
    + ", ".join(f"{k} {f(v, 1)}" for k, v in list(kr.items())[:6]) +
    f"); {tbi['topics']} topics, {tbi['partitions_total']} partitions, {tbi['consumer_groups']} consumer groups "
    f"({tbi['consumer_group_member_rows']} member rows). Other idle cores: "
    + ", ".join(f"{c} {f(v['cores'], 3)}" for c, v in tbi["cores_used"].items() if v.get("cores") and v["cores"] > 0.02 and c != "tb-kafka-1")
    + f". Reference: Kafka with no ThingsBoard clients at all (`DIAG-tb-idle`, invalid for H-T1, see below) used "
      f"{f(TI['DIAG-tb-idle']['cores_used']['tb-kafka-1']['cores'], 3)} cores. Queue settings: all 25 ms poll intervals, core 10 "
      "partitions, `max_poll_records` 8192 (thingsboard.yml v4.3.1.6 defaults, no overrides in our env; `sut/DIAG-tb-idle2/tb-queues.txt`).")
mh = {c: jvm_flag(c, "MaxHeapSize") for c in ("tb-tb-core-1", "tb-tb-rule-engine-1", "tb-tb-http-transport-1")}
out(f"2. **Heap (H-T2):** effective `MaxHeapSize`: tb-core {mh['tb-tb-core-1'] // 2**20} MiB, tb-rule-engine "
    f"{mh['tb-tb-rule-engine-1'] // 2**20} MiB, transport {mh['tb-tb-http-transport-1'] // 2**20} MiB (containers 1536/1536/768 MiB, D11). "
    f"`ExitOnOutOfMemoryError`: **{'set' if jvm_has('tb-tb-rule-engine-1', 'ExitOnOutOfMemoryError') else 'not set'}** — the official tb-node launcher sets it "
    "(`docker/tb-node/conf/thingsboard.conf:11`) and the official compose mounts that conf at `/config`; our stack mounts `/config` "
    "only for the transport, so tb-core/tb-rule-engine run the image's built-in conf without it: after a heap OOM the JVM keeps "
    "running broken (EXPL-tb-k4b) instead of exiting and restarting. GC at 100 msg/s: "
    + "; ".join(f"{c} {g['pauses']} pauses, {f(g['pause_ms'], 0)} ms ({pct(g['pause_share_of_wall'], 2)} of wall), max heap after GC "
                f"{g['heap_after_gc_mb_max']} MB" for c, g in tbs["gc"].items())
    + ". No official heap-sizing guidance for Docker deployments was found (install/docker, cluster docker-compose, config and "
      "troubleshooting pages give host RAM only; the official compose leaves `JAVA_OPTS` empty, i.e. the JVM default fraction).")
out(f"3. **Valkey (H-T3, `DIAG-tb-obs-sink-r100`, {f(vk['window_s'], 0)} s at 100 msg/s):** {f(vk['total_calls_per_msg'], 1)} commands per message:")
out()
table(["command", "calls / msg", "µs / msg"], [[k.replace("|", "\\|"), f(v["calls_per_msg"], 3), f(v["usec_per_msg"], 1)]
                                              for k, v in vk["by_command"].items()])
out(f"   `CLIENT SETINFO` is sent twice per new connection by Valkey/Redis clients, which suggests "
    f"~{f(vk['by_command'].get('client|setinfo', {}).get('calls_per_msg', 0) / 2, 0)} new connections per message "
    "(connection churn, not pooling). Not verified: `total_connections_received` was not collected.")
out("4. **SQL batching / pool:** no `SQL_TS_*`, `SQL_TS_LATEST_*` or `SPRING_DATASOURCE_MAXIMUM_POOL_SIZE` overrides in the "
    "containers' env (`sut/DIAG-tb-idle2/tb-jvm.txt`), so thingsboard.yml defaults apply: ts batch 10,000 / 100 ms / 3 threads, "
    "latest 1,000 / 50 ms / 3 threads, datasource pool 16.")
out()

# ---------------- Step 3: EdgeX
out("### EdgeX (k = 4: 4 device-rest + 4 core-data + export on 4-7; k = 1: one of each on core 4; Postgres, NATS on 2-3)")
out()
out("1. **Source (edgex-go v4.0.2, excerpts):** one subscription and **one goroutine per core-data replica** persisting events one "
    "at a time (`subscriber.go:54-81`); **one transaction per event** (BEGIN, INSERT event, COPY readings, COMMIT; "
    "`postgres/event.go:60-79`, `reading.go:444-460`); no batching or concurrency knob (`PersistData` only switches persistence; "
    "`Database.MaxConns` 4 does not help a single goroutine).")
erow = []
for i in ("DIAG-edgex-k4-r500b", "DIAG-edgex-k4-r500-nosync", "DIAG-edgex-k1-r200", "DIAG-edgex-k1-r200-nosync"):
    r, p = E[i], E[i]["postgres"]
    reps = r["pg_activity"]["by_core_data_replica"]
    walw = [sum(v for k, v in x["busiest_connection_share_by_state"].items() if "WAL" in k) for x in reps.values()]
    stm = {s["query"].split()[0].lower() + (" " + s["query"].split()[1] if s["query"].split()[0].lower() in ("insert", "copy") else ""): s
           for s in p["statements"]}
    pv = EXP[i]["explore"]["steps"][1]["verdict_persistence"]
    erow.append([i, r["pg_settings"].get("synchronous_commit"), f"{r['rate']:g}", f(r["persisted_per_s_in_window"]), pv,
                 f(p["commits_per_s"]), f(p["wal_syncs_per_s"]), f(p["wal_sync_ms_per_sync"], 2), pct(p["wal_sync_time_share_of_window"], 0),
                 f"{pct(min(walw), 0)}-{pct(max(walw), 0)}", f(r["disk_write_await_ms"].get("sda"), 2),
                 f(stm.get("copy \"core_data\".\"reading\"", {}).get("mean_exec_ms"), 3),
                 ", ".join(str(x["connections"]) for x in reps.values())])
out("2-3. **Durable vs `synchronous_commit=off` (DIAG only):**")
out()
table(["Run", "sync commit", "offered/s", "persisted/s", "persistence verdict", "commits/s", "WAL syncs/s", "ms / sync",
       "WAL sync share of time", "WAL waits (busiest conn.)", "disk write await (ms)", "COPY mean (ms)", "connections per replica"], erow)
r1 = k4n["persisted_per_s_in_window"] / k4d["persisted_per_s_in_window"]
r2 = k1n["persisted_per_s_in_window"] / k1d["persisted_per_s_in_window"]
out(f"   With asynchronous commit both configurations keep up with the offered rate (×{f(r1, 2)} at k = 4, ×{f(r2, 2)} at k = 1, "
    "capped by the offered load). **H-E1 confirmed: commit latency (WAL `fdatasync` ≈ the VM disk's write latency) sets the "
    "ceiling.** With 4 replicas, group commit shares a sync among ~"
    f"{f(k4d['postgres']['commits_per_s'] / k4d['postgres']['wal_syncs_per_s'], 1)} commits; a single replica cannot share. "
    "Measured per 1 s `pg_stat_activity` sample; time resolution of the counters 10 s; disk await from node-exporter (5 s steps).")
out(f"4. **NATS and memory limits:** go-mod-messaging v4.0.3 sets no pending limits, so nats.go's defaults apply per subscription: "
    f"500,000 messages / 64 MiB. Event envelopes average {f(k4d['nats']['in_bytes_per_msg'], 0)} B (`nats_varz` in_bytes/in_msgs), "
    f"so the byte limit is reached at ~{f(64 * 2**20 / k4d['nats']['in_bytes_per_msg'] / 1000, 0)}k pending events — H-E2 confirmed "
    f"(EXPL-edgex-k1's 500 msg/s step, where core-data dropped, had {f(k1_drop_outstanding, 0)} events outstanding at persistence "
    f"in minute 1). Export service memory limit: {export_mem // 2**20} MiB (`edgex/compose.yaml`, service `app-http-export`; "
    "`sut/<run>/containers.tsv`).")
out()

# ---------------- Step 4
out("## Step 4: observer overhead (sink path on / off, ~50 % of exploratory capacity)")
out()
for name, rows, note in (
        (f"EdgeX k = 4, 200 msg/s (`DIAG-edgex-k4-obs-*-r200`)", eobs_rows,
         f"Export service {f(export_ms, 3)} ms CPU / msg (H-O expected 1.6 ± 25 %). Persisted/s {f(eos['persisted_per_s_in_window'])} with, "
         f"{f(eon['persisted_per_s_in_window'])} without the export; stored "
         f"{stored['DIAG-edgex-k4-obs-sink-r200']:,} / {stored['DIAG-edgex-k4-obs-nosink-r200']:,} of "
         f"{planned['DIAG-edgex-k4-obs-sink-r200']:,} (history export)."),
        (f"ThingsBoard k = 4, 100 msg/s (`DIAG-tb-obs-*-r100`)", tobs_rows,
         f"REST node: tb-rule-engine {re_ms:+.3f} ms / msg; other containers move by run-to-run noise. Stored "
         f"{stored['DIAG-tb-obs-sink-r100']:,} / {stored['DIAG-tb-obs-nosink-r100']:,} of {planned['DIAG-tb-obs-sink-r100']:,} "
         "(history export). Order: with-sink first (colder JVMs), which can only inflate the difference."),
        (f"FIWARE k = 4, 50 msg/s (`DIAG-fiware-obs-*-r50`)", fobs_rows,
         f"Orion (notifications) {orion_obs:+.3f} ms / msg. MongoDB {mongo_obs:+.3f} ms / msg is **confounded**: the with-sink run "
         f"had {provisioned['DIAG-fiware-obs-sink-r50']:,} provisioned devices against {provisioned['DIAG-fiware-obs-nosink-r50']:,}, "
         "and the per-message COLLSCANs grow with the device count. No history: stored "
         "count not available (Orion keeps the latest value only).")):
    out(f"**{name}** (ms CPU per acknowledged message, measured step):")
    out()
    table(["Container", "with sink", "without", "difference"], rows)
    out(note)
    out()

# ---------------- hypotheses
out("## Hypotheses (committed before the runs)")
out()
hyp = [
    ["H-E1 commit latency", f"durable {f(k4d['persisted_per_s_in_window'])}/s vs async {f(k4n['persisted_per_s_in_window'])}/s (k4); "
                            f"{f(k1d['persisted_per_s_in_window'])} vs {f(k1n['persisted_per_s_in_window'])} (k1); both keep up",
     "**confirmed** (the ≥ 1.5× figure is capped by the offered load: ×" + f(r1, 2) + " / ×" + f(r2, 2) + "; refutation threshold 1.2× not met)"],
    ["H-E2 NATS byte limit", f"{f(k4d['nats']['in_bytes_per_msg'], 0)} B per envelope", "**confirmed**"],
    ["H-T1 idle polling", f"Kafka idle {f(tbi['cores_used']['tb-kafka-1']['cores'], 2)} cores; Fetch {f(kr.get('Fetch', 0) + kr.get('FetchConsumer', 0), 0)}/s; "
                          f"{tbi['consumer_group_member_rows']} members", "**partly**: idle CPU yes (≥ 0.3), fetch rate far below the 1,000/s I predicted (long-poll fetches)"],
    ["H-T2 heap", f"MaxHeapSize 512 MiB; GC share {pct(tbs['gc']['tb-tb-rule-engine-1']['pause_share_of_wall'], 2)} at 100/s",
     "flags confirmed; **GC expectation refuted** at 100 msg/s (< 2 %); the OOM at ≥ 700/s was not reproduced"],
    ["H-T3 Valkey", f"{f(vk['total_calls_per_msg'], 1)} commands / msg", "**confirmed**"],
    ["H-F1 registry reads", f"{f(fprof['ops_per_msg_total'], 1)} DB ops / msg", "**confirmed**"],
    ["H-F2 unindexed query", f"COLLSCAN {f(sum(g['per_msg'] for g in collscan_dev), 2)} / msg on iotagentul.devices", "**confirmed**"],
    ["H-F3 no duplicates", "Orion transactions − sent = " + ", ".join(str(r[3]) for r in whole), "**confirmed**"],
    ["H-F4 one process, RSS", f"{iot_procs} processes / 4 containers; RSS ≤ 512 MB at 100/s", "**confirmed** (OOM not reproduced)"],
    ["H-F5 IoT Agent CPU", f"mongoose/mongodb/bson {pct(prof_mongo)}; UL + HTTP client {pct(prof_ulhttp)}", "**confirmed**"],
    ["H-O observer", f"EdgeX export {f(export_ms, 2)}; TB rule engine {re_ms:+.2f}; FIWARE Orion {orion_obs:+.2f} ms / msg", "**confirmed** (FIWARE MongoDB part confounded)"],
]
table(["Hypothesis", "Measured", "Outcome"], hyp)

# ---------------- proposals
out("## Proposed changes (nothing applied; Step 5 waits for approval)")
out()
table(["Platform", "Change", "Cat.", "Official source", "Expected effect", "Risk to semantics"], [
    ["FIWARE", "Create the IoT Agent device index `{service:1, subservice:1, apikey:1, id:1}` (unique) in `fiware/mongo/init-indexes.js`",
     "A", "iotagent-node-lib 4.8.0 `lib/model/Device.js:64` and change log (#1576); not in any iotagent-ul release yet",
     "Both per-message COLLSCANs become index lookups; MongoDB CPU per message stops growing with the device count; the binding pool "
     f"then stays the IoT Agent app pool (fit: {f(fit['EXPL-fiware-k4']['pools']['4-7']['predicted_capacity'], 0)} msg/s), so capacity "
     "should rise only to the app-pool limit",
     "None (uniqueness the agent already enforces); index maintenance on provisioning"],
    ["FIWARE", "`-reqPoolSize 4` on Orion", "A", "perf_tuning.md, HTTP server tuning (pool ≈ CPU cores under high load)",
     "Small: Orion is ~1 ms CPU / msg", "None"],
    ["FIWARE", "Notification threadpool mode", "B", "perf_tuning.md, Notification modes", "Fewer thread creations per notification",
     "Queue overflow drops notifications (delivery changes); not proposed unless needed"],
    ["ThingsBoard", "Mount the official `docker/tb-node/conf` (v4.3.1.6) at `/config` for tb-core and tb-rule-engine", "A (defect fix)",
     "docker/docker-compose.yml (v4.3.1.6) lines 42/92; `msa/tb-node/docker/start-tb-node.sh`",
     "`ExitOnOutOfMemoryError` + official logback: an OOM restarts the JVM instead of leaving it broken", "None"],
    ["ThingsBoard", "Raise queue poll intervals (e.g. `TB_QUEUE_*_POLL_INTERVAL_MS`)", "A?",
     "thingsboard.yml documents the knobs; no official recommendation found", "Lower idle Kafka/JVM CPU; adds up to the interval to latency",
     "Latency only; needs your call whether 'documented knob' counts as (a)"],
    ["ThingsBoard", "Heap sizing (`-Xmx`)", "?", "No official guidance found",
     f"OOM at ≥ 700/s; at 100/s heap after GC ≤ {max(g['heap_after_gc_mb_max'] for g in tbs['gc'].values())} MB", "—"],
    ["EdgeX", "More core-data replicas than pool cores (e.g. 8 at k = 4) to get more concurrent commits", "A (topology)",
     "None specific; replicas + NATS queue group are EdgeX's own scaling mechanism (D26 records replicas separately)",
     "Group commit shares more commits per sync: higher persisted rate at the same disk", "Changes replicas per tier (reported)"],
    ["EdgeX", "`synchronous_commit=off` / `commit_delay`", "B", "PostgreSQL 16 docs 20.5.1 / 30.4", "Keeps up with 500/s (DIAG)",
     "Async commit can lose acknowledged rows on a crash: weakens durability"],
    ["EdgeX", "JetStream bus", "B", "EdgeX docs (Step 6 ablation)", "Backlog instead of loss under overload", "Delivery semantics (planned as ABL)"],
])
out("## Proposed decision-log entries")
out()
out("- **D27 (proposed):** FIWARE IoT Agent registry index = upstream 4.8.0 fix applied by our init script (Category A), recorded "
    "as a deviation in every later FIWARE run.")
out("- **D28 (proposed):** ThingsBoard tb-core/tb-rule-engine mount the official `docker/tb-node/conf` (deployment defect).")
out("- **D29 (proposed):** EdgeX persistence is bounded by WAL flush latency on the SUT disk; report the disk's write latency with "
    "every EdgeX result. Decide whether core-data replicas per core is a tuning knob (Category A) or a topology change.")
out("- **D30 (proposed):** Observation points per platform (ack / sink / storage), per the README table in Step 5.")
out()
out("## Could not verify")
out()
for x in [
    "ThingsBoard heap sizing: no official guidance found (docs pages checked: install/docker, cluster/docker-compose-setup, install/config, troubleshooting, performance-tools).",
    "ThingsBoard Valkey connection churn: `total_connections_received` was not collected; the ~9 connections per message are inferred from `CLIENT SETINFO`.",
    "The rule-engine OOM (≥ 700 msg/s) and the IoT Agent OOM (≥ 500 msg/s) were not reproduced; only their mechanisms were examined.",
    "FIWARE observer cost in MongoDB: confounded by the growing device registry (repeat after the index fix).",
    f"MongoDB idle CPU ({fidle.get('fiware-mongo-1')} cores after one run's devices): part may be the monitoring exporter's scrapes; not separated.",
    "`DIAG-tb-idle` is invalid for H-T1: under the JMX override Kafka's healthcheck failed (the tool inherited `JMX_PORT`), so ThingsBoard never started; kept only as the client-less Kafka reference. Rerun as `DIAG-tb-idle2`.",
    "`DIAG-edgex-k4-r500` ran without exporters (no Prometheus persistence series or disk metrics); rerun as `DIAG-edgex-k4-r500b`. "
    "`DIAG-fiware-k4-r100-cpuprof` (no kill binary in the image) and `-cpuprof2` (profile truncated; ~5,000 devices, generator INVALID) "
    "produced no profile; `-cpuprof3` did.",
    "Grafana ran on ramonavm1 (pinned to core 3, not the generator cores) during all DIAG runs, unlike the calibration setup.",
]:
    out(f"- {x}")
out()
out("## Run index, commits, VM state")
out()
out("DIAG runs: " + ", ".join(sorted(list(E) + list(T) + list(TI) + list(F))) + ". Raw data: `results/diag/` (runs), "
    "`results/diag/sut/<run>/` (SUT collectors), `results/diag/<run>.diag.json` (`scripts/diag/analyze.py`). Code: commits from "
    f"`069faa5` to `{git}`. VMs after the session: no platform or collector containers on ramonavm2 (only the permanent "
    "monitoring firewall rule); ramonavm1 runs its monitoring stack and no sink or generator.")
