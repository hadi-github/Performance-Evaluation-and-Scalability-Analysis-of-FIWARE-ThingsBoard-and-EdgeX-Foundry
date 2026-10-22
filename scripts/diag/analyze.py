#!/usr/bin/env python3
"""Diagnostics round 1: numbers for docs/diagnostics.md, computed from the raw files of DIAG runs.

  scripts/diag/analyze.py <results/diag> <run_id> [...]     (on ramonavm1: Prometheus at 127.0.0.1:9090)
  scripts/diag/analyze.py <results/diag> --idle DIAG-tb-idle

For a load run the window is the measured (second) step: scenario start .. + duration (k6-summary.json,
resolved.json). Counters sampled every 1-10 s are differenced at the samples nearest to the window edges; the
sampling interval is reported with each figure. Prints JSON (one object per run) and writes <run>.diag.json.
"""
import csv
import glob
import json
import os
import re
import sys
import urllib.parse
import urllib.request

PROM = os.environ.get("PROM", "http://127.0.0.1:9090")
SUT_NODE = os.environ.get("SUT_NODE", "87.107.191.107:9100")


def load(p):
    with open(p) as f:
        return json.load(f)


def nearest(samples, t):  # samples: list of (t_ms, value)
    return min(samples, key=lambda s: abs(s[0] - t)) if samples else None


def window(base, run):
    summ = load(f"{base}/{run}/k6-summary.json")
    res = load(f"{base}/{run}/resolved.json")
    i = len(res["steps"]) - 1
    t0 = summ["steps"][i]["scenario_start_ms"]
    dur = res["steps"][i]["duration_s"]
    ack = summ["steps"][i].get("ack_success")
    return t0, t0 + dur * 1000, res["steps"][i]["rate"], ack


def cpu_window(sut, t0, t1):
    """cores used per container over [t0, t1] (ms) from cpu-snapshot TSV (time s, name, id, usage_usec, started, cpuset)"""
    snaps = {}
    for line in open(f"{sut}/cpu.tsv"):
        f = line.rstrip("\n").split("\t")
        if len(f) < 6 or not f[3].isdigit():
            continue
        snaps.setdefault(f[1], []).append((float(f[0]) * 1000, f[2], int(f[3]), f[4], f[5]))
    out = {}
    for c, ss in snaps.items():
        a, b = min(ss, key=lambda v: abs(v[0] - t0)), min(ss, key=lambda v: abs(v[0] - t1))
        if a[1] != b[1] or a[3] != b[3] or b[0] <= a[0]:
            out[c] = {"cpuset": b[4], "cores": None, "note": "restarted/replaced in window"}
            continue
        out[c] = {"cpuset": b[4], "cores": (b[2] - a[2]) / 1e3 / (b[0] - a[0]), "edge_error_ms": [a[0] - t0, b[0] - t1]}
    return out


def prom_range(q, t0, t1, step=5):
    url = f"{PROM}/api/v1/query_range?" + urllib.parse.urlencode(
        {"query": q, "start": t0 / 1000, "end": t1 / 1000, "step": step})
    return json.load(urllib.request.urlopen(url, timeout=30))["data"]["result"]


def avg_series(result):
    out = {}
    for r in result:
        vals = [float(v[1]) for v in r["values"] if v[1] not in ("NaN", "+Inf", "-Inf")]
        if vals:
            out[json.dumps(r["metric"], sort_keys=True)] = sum(vals) / len(vals)
    return out


def ndjson(p):
    out = []
    if os.path.exists(p):
        for line in open(p):
            line = line.strip()
            if line.startswith("{"):
                try:
                    out.append(json.loads(line))
                except json.JSONDecodeError:
                    pass
    return out


# ------------------------------------------------------------------ EdgeX
def edgex(base, run, sut, t0, t1, rate, ack):
    d = {}
    cnt = []  # json_agg pretty-prints arrays over several lines: split records at their {"t_ms" start
    if os.path.exists(f"{sut}/pg-counters.ndjson"):
        for chunk in re.split(r'\n(?=\{"t_ms")', open(f"{sut}/pg-counters.ndjson").read()):
            try:
                cnt.append(json.loads(chunk))
            except json.JSONDecodeError:
                pass
    if cnt:
        a, b = nearest([(r["t_ms"], r) for r in cnt], t0)[1], nearest([(r["t_ms"], r) for r in cnt], t1)[1]
        dt = (b["t_ms"] - a["t_ms"]) / 1000
        dw = {k: b["wal"][k] - a["wal"][k] for k in ("wal_records", "wal_write", "wal_sync", "wal_write_time", "wal_sync_time")}
        commits = b["db"]["xact_commit"] - a["db"]["xact_commit"]
        d["postgres"] = {
            "sample_interval_s": 10, "window_s": dt, "edge_error_ms": [a["t_ms"] - t0, b["t_ms"] - t1],
            "commits_per_s": commits / dt, "wal_syncs_per_s": dw["wal_sync"] / dt,
            "wal_sync_ms_per_sync": dw["wal_sync_time"] / dw["wal_sync"] if dw["wal_sync"] else None,
            "wal_write_ms_per_write": dw["wal_write_time"] / dw["wal_write"] if dw["wal_write"] else None,
            "wal_sync_time_share_of_window": dw["wal_sync_time"] / 1000 / dt,
        }
        sa = {s["queryid"]: s for s in (a["stmts"] or [])}
        stm = []
        for s in b["stmts"] or []:
            p = sa.get(s["queryid"], {"calls": 0, "total_exec_time": 0})
            calls = s["calls"] - p["calls"]
            if calls > 0:
                stm.append({"query": s["query"], "calls_per_s": calls / dt,
                            "mean_exec_ms": (s["total_exec_time"] - p["total_exec_time"]) / calls})
        d["postgres"]["statements"] = sorted(stm, key=lambda s: -s["calls_per_s"])[:8]
    # pg_stat_activity: core-data backends by state / wait event, connections per replica
    ips = {}
    for line in open(f"{sut}/containers.tsv"):
        f = line.rstrip("\n").split("\t")
        if f[0].startswith("edgex-core-data-"):
            ips[f[1]] = f[0]
    act = {}
    if os.path.exists(f"{sut}/pg-activity.tsv"):
        for line in open(f"{sut}/pg-activity.tsv"):
            f = line.rstrip("\n").split("\t")
            if len(f) < 8 or not f[0].isdigit() or not (t0 <= int(f[0]) <= t1) or f[2] not in ips:
                continue
            r = act.setdefault(ips[f[2]], {"samples": 0, "pids": set(), "by_state": {}})
            key = f[4] + ("/" + f[5] + ":" + f[6] if f[5] else "")
            r["by_state"][key] = r["by_state"].get(key, 0) + 1
            r["samples"] += 1
            r["pids"].add(f[1])
    ticks = None
    if os.path.exists(f"{sut}/pg-activity.tsv"):
        ticks = len({line.split("\t")[0][:-3] for line in open(f"{sut}/pg-activity.tsv")
                     if line[:1].isdigit() and t0 <= int(line.split("\t")[0]) <= t1})
    d["pg_activity"] = {"sample_interval_s": 1, "samples_in_window": ticks, "by_core_data_replica": {
        c: {"connections": len(r["pids"]),
            "share_by_state": {k: v / r["samples"] for k, v in sorted(r["by_state"].items(), key=lambda kv: -kv[1])}}
        for c, r in sorted(act.items())}}
    # NATS: bytes per event and pending at the subscriber
    varz = [(r["t_ms"], r["data"]) for r in ndjson(f"{sut}/http.ndjson") if r.get("src") == "nats_varz"]
    if varz:
        a, b = nearest(varz, t0)[1], nearest(varz, t1)[1]
        dm = b["in_msgs"] - a["in_msgs"]
        d["nats"] = {"sample_interval_s": 10, "in_msgs": dm, "in_bytes_per_msg": (b["in_bytes"] - a["in_bytes"]) / dm if dm else None,
                     "slow_consumers_total": b.get("slow_consumers")}
        pend = [(r["t_ms"], c) for r in ndjson(f"{sut}/http.ndjson") if r.get("src") == "nats_connz?subs=1"
                for c in r["data"].get("connections", [])]
        mx = {}
        for t, c in pend:
            if t0 <= t <= t1 + 60000:
                k = c.get("name") or str(c.get("cid"))
                mx[k] = max(mx.get(k, 0), c.get("pending_bytes", 0))
        d["nats"]["max_pending_bytes_by_client_in_window_plus_60s"] = mx
    # persisted rate in the window
    p = f"{base}/{run}.persist.tsv"
    if os.path.exists(p):
        pts = [(float(a), float(b)) for a, b in (l.split("\t") for l in open(p) if l[:1].isdigit())]
        if pts:
            a, b = nearest(pts, t0), nearest(pts, t1)
            d["persisted_per_s_in_window"] = (b[1] - a[1]) / ((b[0] - a[0]) / 1000)
        else:
            d["persisted_per_s_in_window"] = "no persistence series"
    # disk write await on the SUT
    try:
        res = prom_range(f'rate(node_disk_write_time_seconds_total{{instance="{SUT_NODE}"}}[30s]) / '
                         f'rate(node_disk_writes_completed_total{{instance="{SUT_NODE}"}}[30s])', t0, t1)
        d["disk_write_await_ms"] = {json.loads(k).get("device"): v * 1000 for k, v in avg_series(res).items()}
        res = prom_range(f'rate(node_disk_writes_completed_total{{instance="{SUT_NODE}"}}[30s])', t0, t1)
        d["disk_writes_per_s"] = {json.loads(k).get("device"): v for k, v in avg_series(res).items()}
    except Exception as e:  # noqa: BLE001
        d["disk_write_await_ms"] = f"query failed: {e}"
    s = glob.glob(f"{base}/{run}.pgsettings.tsv")
    if s:
        d["pg_settings"] = dict(l.rstrip("\n").split("|", 1) for l in open(s[0]) if "|" in l)
    return d


# ------------------------------------------------------------------ ThingsBoard
def valkey_blocks(p):
    out, cur = [], None
    for line in open(p):
        line = line.strip()
        if line == "##INFO":
            cur = {"t_ms": None, "cmd": {}}
            out.append(cur)
        elif cur is not None and line.startswith("server_time_usec:"):
            cur["t_ms"] = int(line.split(":")[1]) / 1000
        elif cur is not None and line.startswith("cmdstat_"):
            name, rest = line[8:].split(":", 1)
            kv = dict(x.split("=") for x in rest.split(","))
            cur["cmd"][name] = (int(kv["calls"]), int(float(kv["usec"])))
    return [b for b in out if b["t_ms"]]


def gc_pauses(path, t0, t1):
    """sum of GC pause ms with wall time in [t0, t1] from a unified JVM log (time decoration first)"""
    tot, n, heap_after = 0.0, 0, []
    for line in open(path, errors="replace"):
        m = re.match(r"\[(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d+)([+-]\d{4})\]", line)
        if not m or "Pause" not in line:
            continue
        p = re.search(r"(\d+)M->(\d+)M\((\d+)M\) (\d+\.\d+)ms$", line.strip())
        if not p:
            continue
        from datetime import datetime
        t = datetime.strptime(m.group(1) + m.group(2), "%Y-%m-%dT%H:%M:%S.%f%z").timestamp() * 1000
        if t0 <= t <= t1:
            tot += float(p.group(4)); n += 1; heap_after.append(int(p.group(2)))
    return {"pauses": n, "pause_ms": tot, "pause_share": tot / (t1 - t0),
            "heap_after_gc_mb_max": max(heap_after) if heap_after else None}


def tb(base, run, sut, t0, t1, rate, ack):
    d = {}
    if os.path.exists(f"{sut}/valkey.txt"):
        bl = valkey_blocks(f"{sut}/valkey.txt")
        a, b = min(bl, key=lambda x: abs(x["t_ms"] - t0)), min(bl, key=lambda x: abs(x["t_ms"] - t1))
        dt = (b["t_ms"] - a["t_ms"]) / 1000
        msgs = rate * dt
        cmds = {}
        for c, (calls, usec) in b["cmd"].items():
            pc, pu = a["cmd"].get(c, (0, 0))
            if calls - pc > 0:
                cmds[c] = {"calls_per_msg": (calls - pc) / msgs, "usec_per_msg": (usec - pu) / msgs}
        d["valkey"] = {"sample_interval_s": 10, "window_s": dt, "messages_assumed": msgs,
                       "total_calls_per_msg": sum(v["calls_per_msg"] for v in cmds.values()),
                       "by_command": dict(sorted(cmds.items(), key=lambda kv: -kv[1]["calls_per_msg"]))}
    gc = {}
    for f in glob.glob(f"{sut}/gc/*/*gc.log*"):
        c = f.split("/")[-2]
        r = gc_pauses(f, t0, t1)
        g = gc.setdefault(c, {"pauses": 0, "pause_ms": 0.0, "heap_after_gc_mb_max": None})
        g["pauses"] += r["pauses"]; g["pause_ms"] += r["pause_ms"]
        if r["heap_after_gc_mb_max"] is not None:
            g["heap_after_gc_mb_max"] = max(g["heap_after_gc_mb_max"] or 0, r["heap_after_gc_mb_max"])
    for g in gc.values():
        g["pause_share_of_wall"] = g["pause_ms"] / (t1 - t0)
    d["gc"] = gc
    return d


def tb_idle(base, run):
    sut = f"{base}/sut/{run}"

    def jmx(p):  # JmxTool CSV: "time" (epoch ms) + one Count column per (request, version); summed per request
        rows = [r for r in csv.reader(l for l in open(p) if l.strip() and not l.strip().isdigit())]
        hdr = next((r for r in rows if r and r[0] == "time"), None)
        if not hdr:
            return None, {}
        v = rows[rows.index(hdr) + 1]
        counts = {}
        for k, x in zip(hdr[1:], v[1:]):
            req = re.search(r"request=([A-Za-z]+)", k).group(1)
            counts[req] = counts.get(req, 0) + float(x)
        return int(v[0]), counts
    ta, a = jmx(f"{sut}/jmx-a.csv")
    tb_, b = jmx(f"{sut}/jmx-b.csv")
    d = {"window_s": (tb_ - ta) / 1000 if ta and tb_ else None}
    if ta and tb_:
        dt = (tb_ - ta) / 1000
        d["kafka_requests_per_s"] = dict(sorted(((k, (b[k] - a.get(k, 0)) / dt) for k in b if b[k] - a.get(k, 0) > 0),
                                                key=lambda kv: -kv[1]))
        d["kafka_requests_per_s_total"] = sum(d["kafka_requests_per_s"].values())
        d["cores_used"] = {c: v for c, v in cpu_window(sut, ta + 5000, tb_ - 5000).items()}
    groups = [l for l in open(f"{sut}/kafka-groups.txt") if l.strip() and not l.startswith("GROUP")] if os.path.exists(f"{sut}/kafka-groups.txt") else []
    d["consumer_group_member_rows"] = len(groups)
    d["consumer_groups"] = len({l.split()[0] for l in groups})
    topics = [l for l in open(f"{sut}/kafka-topics.txt") if l.startswith("Topic:")] if os.path.exists(f"{sut}/kafka-topics.txt") else []
    d["topics"] = len(topics)
    d["partitions_total"] = sum(int(re.search(r"PartitionCount: (\d+)", l).group(1)) for l in topics)
    return d


# ------------------------------------------------------------------ FIWARE
def fiware(base, run, sut, t0, t1, rate, ack):
    d = {}
    for r in ndjson(f"{sut}/mongo.ndjson"):
        if "profile" in r:
            p = r["profile"]
            msgs = rate * (p["off_ms"] - p["on_ms"]) / 1000
            groups = []
            for g in p["groups"]:
                i = g["_id"]
                groups.append({"ns": i.get("ns"), "op": i.get("op"), "plan": i.get("plan"),
                               "per_msg": g["n"] / msgs, "ms_per_msg": g["millis"] / msgs,
                               "docsExamined_per_op": g["docsExamined"] / g["n"], "keysExamined_per_op": g["keysExamined"] / g["n"],
                               "nreturned_per_op": g["nreturned"] / g["n"]})
            d["mongo_profile"] = {"window_s": (p["off_ms"] - p["on_ms"]) / 1000, "messages_in_window": msgs,
                                  "ops_per_msg_total": sum(g["per_msg"] for g in groups),
                                  "collscan_groups": [g for g in groups if g["plan"] and "COLLSCAN" in g["plan"]],
                                  "groups": sorted(groups, key=lambda g: -g["per_msg"]), "indexes": p["indexes"]}
    # Orion incoming transactions (all replicas, service openiot) vs messages sent
    met = [(r["t_ms"], r["container"], r["data"]) for r in ndjson(f"{sut}/http.ndjson") if r.get("src") == "orion_metrics"]
    if met:
        def tx(sample):
            try:
                m = json.loads(sample)
                return m["services"].get("openiot", {}).get("sum", {}).get("incomingTransactions", 0)
            except Exception:  # noqa: BLE001
                return None
        by_c = {}
        for t, c, s in met:
            v = tx(s)
            if v is not None:
                by_c.setdefault(c, []).append((t, v))
        da = sum(nearest(v, t1)[1] - nearest(v, t0)[1] for v in by_c.values())
        d["orion_openiot_incoming_tx_in_window"] = da
        d["messages_sent_in_window"] = ack
        d["orion_tx_per_message"] = da / ack if ack else None
        d["orion_sample_interval_s"] = 10
        # whole run (H-F3 duplicate check): first sample (before k6 setup) to last (after the sink drained), all
        # replicas, vs every message sent; window edges cannot bias this
        summ = load(f"{base}/{run}/k6-summary.json")
        sent = sum(s.get("attempted") or 0 for s in summ["steps"])
        whole = sum(max(v)[1] - min(v)[1] for v in by_c.values())
        d["orion_openiot_incoming_tx_whole_run"] = whole
        d["messages_sent_whole_run"] = sent
        d["orion_tx_minus_sent_whole_run"] = whole - sent
    try:
        res = prom_range('max by (container) (container_memory_rss{container=~"fiware-iot-agent.*"})', t0, t1)
        d["iot_agent_rss_mb_avg"] = {json.loads(k)["container"]: v / 2**20 for k, v in avg_series(res).items()}
        res = prom_range('max by (container) (container_memory_working_set_bytes{container=~"fiware-iot-agent.*"})', t0, t1)
        d["iot_agent_working_set_mb_avg"] = {json.loads(k)["container"]: v / 2**20 for k, v in avg_series(res).items()}
    except Exception as e:  # noqa: BLE001
        d["iot_agent_rss_mb_avg"] = f"query failed: {e}"
    return d


def analyze(base, run):
    sut = f"{base}/sut/{run}"
    platform = open(f"{sut}/platform").read().split()[0]
    t0, t1, rate, ack = window(base, run)
    cpu = cpu_window(sut, t0, t1)
    rate_ack = (ack or 0) / ((t1 - t0) / 1000)
    for c in cpu.values():
        if c.get("cores") is not None and rate_ack:
            c["ms_cpu_per_msg"] = c["cores"] * 1000 / rate_ack
    out = {"run_id": run, "platform": platform, "window_ms": [t0, t1], "rate": rate, "ack_success": ack,
           "cpu_per_container": dict(sorted(cpu.items()))}
    out.update({"edgex": edgex, "thingsboard": tb, "fiware": fiware}[platform](base, run, sut, t0, t1, rate, ack))
    return out


if __name__ == "__main__":
    base, args = sys.argv[1].rstrip("/"), sys.argv[2:]
    res = []
    if args and args[0] == "--idle":
        res = [dict(run_id=a, **tb_idle(base, a)) for a in args[1:]]
    else:
        res = [analyze(base, a) for a in args]
    for r in res:
        with open(f"{base}/{r['run_id']}.diag.json", "w") as f:
            json.dump(r, f, indent=1, default=list)
    print(json.dumps(res, indent=1, default=list))
