#!/usr/bin/env python3
"""Build the Grafana dashboards in ./dashboards (run on a machine that can reach grafana.com; the VMs cannot).

- Community dashboards are downloaded and their datasource inputs (${DS_...}) pointed at the provisioned
  Prometheus datasource (uid "prometheus").
- "IoT benchmark overview" is generated here: k6 load, per-tier / per-replica CPU, memory vs limit, OOM/restarts,
  HAProxy per replica, SUT cores and the load generator's own CPU, filtered by platform and test id.

Usage:  ./build_dashboards.py            (writes dashboards/*.json; commit them)
"""
import json
import os
import re
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "dashboards")
DS = {"type": "prometheus", "uid": "prometheus"}

COMMUNITY = {  # grafana.com id -> file name
    1860: "node-exporter-full",
    19665: "k6-prometheus",
    12693: "haproxy",
    9628: "postgresql",
    2583: "mongodb",
    7589: "kafka-exporter",
    2279: "nats-server",
}


def fix_datasources(node):
    if isinstance(node, dict):
        for k, v in list(node.items()):
            if isinstance(v, str) and re.fullmatch(r"\$\{DS_[A-Za-z0-9_-]+\}", v):
                node[k] = DS if k == "datasource" else "prometheus"
            else:
                fix_datasources(v)
    elif isinstance(node, list):
        for v in node:
            fix_datasources(v)


def community():
    for gid, name in COMMUNITY.items():
        url = "https://grafana.com/api/dashboards/%d/revisions/latest/download" % gid
        with urllib.request.urlopen(url, timeout=60) as r:
            d = json.load(r)
        for k in ("__inputs", "__requires", "__elements"):
            d.pop(k, None)
        fix_datasources(d)
        d["id"] = None
        d["uid"] = "community-%d" % gid
        d.setdefault("tags", []).append("community")
        write(name, d)


# ------------------------------------------------------------------------------------------------ overview
_id = [0]


def panel(title, exprs, unit="short", w=12, h=8, kind="timeseries", desc="", stack=False, legend_calcs=None,
          instant=False, decimals=None):
    _id[0] += 1
    targets = [{"datasource": DS, "expr": e, "legendFormat": lf, "refId": chr(65 + i),
                "range": not instant, "instant": instant, "format": "table" if kind == "table" else "time_series"}
               for i, (e, lf) in enumerate(exprs)]
    defaults = {"unit": unit, "custom": {"fillOpacity": 40 if stack else 10,
                                         "stacking": {"mode": "normal" if stack else "none"},
                                         "showPoints": "never", "spanNulls": False}}
    if decimals is not None:
        defaults["decimals"] = decimals
    p = {"id": _id[0], "type": kind, "title": title, "description": desc, "datasource": DS, "targets": targets,
         "gridPos": {"w": w, "h": h}, "fieldConfig": {"defaults": defaults, "overrides": []}}
    if kind == "timeseries":
        p["options"] = {"legend": {"displayMode": "table", "placement": "right", "calcs": legend_calcs or ["mean", "max"]},
                        "tooltip": {"mode": "multi", "sort": "desc"}}
    if kind == "stat":
        p["options"] = {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "none", "graphMode": "none",
                        "textMode": "value_and_name"}
    if kind == "table":
        p["options"] = {"showHeader": True}
        p["transformations"] = [{"id": "merge"}]
    if kind == "barchart":
        p["options"] = {"stacking": "normal", "legend": {"displayMode": "list", "placement": "bottom"}}
    return p


def text(title, md, w=12, h=5):
    _id[0] += 1
    return {"id": _id[0], "type": "text", "title": title, "gridPos": {"w": w, "h": h},
            "options": {"mode": "markdown", "content": md}}


def row(title):
    _id[0] += 1
    return {"id": _id[0], "type": "row", "title": title, "collapsed": False, "panels": [], "gridPos": {"w": 24, "h": 1}}


def layout(items):
    x = y = rowh = 0
    for p in items:
        w, h = p["gridPos"]["w"], p["gridPos"]["h"]
        if p["type"] == "row" or x + w > 24:
            x, y = 0, y + rowh
            rowh = 0
        p["gridPos"].update(x=x, y=y)
        x += w
        rowh = max(rowh, h)
        if p["type"] == "row":
            x, y, rowh = 0, y + 1, 0
    return items


UNAVAILABLE = ("**Unavailable: producer not implemented yet.** This panel stays empty on purpose; nothing here is "
               "derived from other metrics. See `docs/tooling-interfaces.md`.")


def overview():
    PL = 'platform="$platform"'
    RUN = 'run_id=~"$run_id"'
    SVC = 'service=~"$service"'
    R = "[$__rate_interval]"
    panels = [
        # 1 ---------------------------------------------------------------------------------------------
        row("1. Context and monitoring health (operational status, not a run verdict)"),
        panel("Scrape targets (1 = up, 0 = down; a missing row = never scraped)", [
            ('max by (job, instance, host) (up)', "{{job}} {{instance}}")], "short", w=8, h=8, kind="table", instant=True,
            desc="Prometheus scrape health. Exporters of platforms that are not running are expected to be 0. "
                 "A healthy scrape says nothing about benchmark validity."),
        panel("Scrape duration", [('max by (job) (scrape_duration_seconds)', "{{job}}")], "s", w=8, h=8,
              desc="Per scrape (interval 5 s, timeout 4 s)."),
        panel("Samples per scrape", [('max by (job) (scrape_samples_scraped)', "{{job}}")], "short", w=8, h=8),
        panel("Platform containers (state from docker inspect, every 10 s)", [
            ('max by (service, container, cpuset, status) (bench_container_info{platform="$platform"})', "")],
            "short", w=12, h=8, kind="table", instant=True,
            desc="bench_container_info from monitoring/exporters/container-info.sh (textfile collector). "
                 "cpuset = pinned CPUs. Absent = the timer is not installed on the SUT."),
        panel("Container starts and OOM kills", [
            ('sum by (container) (changes(container_start_time_seconds{job="cadvisor",' + PL + '}[10m]))', "starts in 10 min {{container}}"),
            ('max by (container) (bench_container_oom_killed{platform="$platform"})', "last exit OOM {{container}}"),
            ('sum by (container) (increase(container_oom_events_total{job="cadvisor",' + PL + '}[5m]))', "OOM events 5m {{container}}")],
            "short", w=12, h=8, desc="Starts from cAdvisor's container_start_time_seconds (covers manual restarts and "
                                     "recreation; Docker's RestartCount only counts restart-policy restarts). Platform "
                                     "restarts/OOM are platform evidence, not automatically an invalid run."),
        panel("Load host pressure (ramonavm1): busy per core", [
            ('bench:host_core_busy:ratio30s{host="ramonavm1"}', "cpu{{cpu}}")], "percentunit", w=12, h=7,
            desc="k6 runs on cores 0-2, Prometheus/Grafana/cAdvisor on core 3."),
        panel("Monitoring stack resources", [
            ('rate(process_cpu_seconds_total{job=~"prometheus|grafana"}' + R + ')', "{{job}} CPU cores"),
            ('sum by (container) (rate(container_cpu_usage_seconds_total{job="loadgen-cadvisor",service=~"prometheus|grafana|cadvisor|node-exporter"}' + R + '))', "{{container}} cores (cAdvisor)")],
            "short", w=12, h=7),

        # 2 ---------------------------------------------------------------------------------------------
        row("2. Workload delivery (k6 remote-write stream; present only for runs started with benchctl -prom-rw)"),
        panel("Planned vs sent vs acknowledged (messages/s)", [
            ('sum by (step) (bench:k6_planned_rate{' + RUN + '})', "planned, step {{step}}"),
            ('sum by (scenario) (bench:k6_send_requests:rate30s{' + RUN + '})', "sent {{scenario}}"),
            ('sum by (step) (bench:k6_outcome:rate30s{' + RUN + ',outcome="ack_success"})', "ack_success, step {{step}}")],
            "short", w=12, h=8, desc="planned = step rate from the resolved plan; sent = k6 http_reqs (name=send); "
                                     "ack = generator outcome counter. 30 s rate windows."),
        panel("Explicit failures, unknown outcomes, dropped iterations (per s)", [
            ('sum by (outcome, step) (bench:k6_outcome:rate30s{' + RUN + ',outcome=~"explicit_failure|unknown"})', "{{outcome}} step {{step}}"),
            ('sum by (scenario) (bench:k6_dropped_iterations:rate30s{' + RUN + '})', "dropped {{scenario}}"),
            ('sum by (step) (bench:k6_outcome:rate30s{' + RUN + ',outcome="early"})', "early sends step {{step}}")],
            "short", w=12, h=8, desc="Any dropped iteration makes the run INVALID (benchctl health). No data = no "
                                     "such events in the stream, or no stream: check the run's health.json."),
        panel("Active VUs vs allocated", [
            ('max by (run_id) (k6_vus{' + RUN + '})', "active {{run_id}}"),
            ('max by (run_id) (k6_vus_max{' + RUN + '})', "allocated {{run_id}}")], "short", w=12, h=7),
        panel("HTTP status of telemetry requests (per s)", [
            ('sum by (status) (bench:k6_send_requests:rate30s{' + RUN + '})', "HTTP {{status}}")], "short", w=12, h=7,
            desc="status 0 = no response (timeout/transport error). A 5xx is a request failure, not a run FAIL."),

        # 3 ---------------------------------------------------------------------------------------------
        row("3. Latency (only qualified sources; never substituted)"),
        panel("Send lag (actual JS send − scheduled), bucket counts/s by upper edge (ms)", [
            ('sum by (le) (bench:k6_latency_bucket:rate30s{' + RUN + ',histogram="lag"})', "le {{le}}")],
            "short", w=12, h=8, stack=True,
            desc="Non-cumulative bucket rates from the generator histogram (edges from the plan, 'inf' = overflow, "
                 "0p5 = 0.5). Gate: p99 <= 10 ms per step, evaluated offline from exact counts (health.json)."),
        panel("Acknowledgement latency (HTTP response − scheduled), bucket counts/s (ms)", [
            ('sum by (le) (bench:k6_latency_bucket:rate30s{' + RUN + ',histogram="ack"})', "le {{le}}")],
            "short", w=12, h=8, stack=True,
            desc="Starts at the scheduled time (includes send lag). This is acknowledgement, not persistence or "
                 "sink observation. The provisional P1 platform budget is 500 ms (not frozen)."),
        panel("k6 http_req_duration (diagnostic: actual send → response; precomputed per 5 s push, NOT mergeable)", [
            ('max by (scenario) (k6_http_req_duration_p99{' + RUN + ',name="send"})', "p99 {{scenario}}"),
            ('max by (scenario) (k6_http_req_duration_p50{' + RUN + ',name="send"})', "p50 {{scenario}}")],
            "s", w=12, h=7, desc="Per-interval quantiles computed by k6; never average them into a run p99."),
        text("Scheduled → sink observation latency", UNAVAILABLE + " Needs the Go sink and outbound delivery per platform.", w=6, h=7),
        text("History-write / committed-visibility latency", UNAVAILABLE + " Needs qualified database instrumentation.", w=6, h=7),

        # 4 ---------------------------------------------------------------------------------------------
        row("4. SUT CPU (cAdvisor, cgroup v2 usage, core-equivalents)"),
        panel("Platform CPU: application vs supporting tier (cores)", [
            ('bench:tier_cpu_cores:rate30s{' + PL + ',tier=~"application|supporting"}', "{{tier}}")],
            "short", w=12, h=8, stack=True,
            desc="Core-equivalents (CPU-seconds per second), not percent. Platform boundary = application + "
                 "supporting tier incl. HAProxy, databases, brokers; exporters excluded (tier=monitoring)."),
        panel("CPU per service (cores)", [
            ('bench:service_cpu_cores:rate30s{' + PL + ',' + SVC + '}', "{{tier}} / {{service}}")], "short", w=12, h=8),
        panel("CPU per container / replica (cores)", [
            ('bench:container_cpu_cores:rate30s{' + PL + ',' + SVC + '}', "{{container}}")], "short", w=12, h=8),
        panel("SUT busy per core (ramonavm2)", [
            ('bench:host_core_busy:ratio30s{host="ramonavm2"}', "cpu{{cpu}}")], "percentunit", w=12, h=8,
            desc="Core 0: OS + exporters, 1: HAProxy, 2-3: supporting services, 4..: application pool (k cores)."),
        panel("Exporter CPU on the SUT (cores, tier=monitoring)", [
            ('bench:service_cpu_cores:rate30s{tier="monitoring"}', "{{service}}")], "short", w=12, h=7),
        text("CPU throttling", "Not applicable in the current layout: containers are pinned with **cpuset** and have "
             "**no CFS quota**, so `container_cpu_cfs_throttled_*` is not produced. A cpuset limits *which* cores; it "
             "does not throttle. If quotas are introduced, add `rate(container_cpu_cfs_throttled_periods_total)` / "
             "`rate(container_cpu_cfs_periods_total)`.", w=12, h=7),

        # 5 ---------------------------------------------------------------------------------------------
        row("5. Host and container resources"),
        panel("Memory working set / limit per container", [
            ('max by (container) (container_memory_working_set_bytes{job="cadvisor",' + PL + ',' + SVC + '}) / '
             'max by (container) (container_spec_memory_limit_bytes{job="cadvisor",' + PL + ',' + SVC + '} > 0)', "{{container}}")],
            "percentunit", w=12, h=8),
        panel("Disk busy and throughput (SUT)", [
            ('rate(node_disk_io_time_seconds_total{job="node",device!~"dm-.*|loop.*"}' + R + ')', "busy {{device}}"),
            ('rate(node_disk_written_bytes_total{job="node",device!~"dm-.*|loop.*"}' + R + ') / 1e6', "written MB/s {{device}}")],
            "short", w=12, h=8, desc="Host-level device counters: activity, not per-transaction fsync latency."),
        panel("Network throughput and errors (both hosts)", [
            ('sum by (host) (rate(node_network_receive_bytes_total{device!~"lo|veth.*|docker.*|br-.*"}' + R + '))', "rx B/s {{host}}"),
            ('sum by (host) (rate(node_network_transmit_bytes_total{device!~"lo|veth.*|docker.*|br-.*"}' + R + '))', "tx B/s {{host}}"),
            ('sum by (host) (rate(node_network_receive_errs_total' + R + ') + rate(node_network_transmit_errs_total' + R + '))', "errors/s {{host}}")],
            "short", w=12, h=8),
        panel("Filesystem free (both hosts)", [
            ('min by (host, mountpoint) (node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay"})', "{{host}} {{mountpoint}}")],
            "percentunit", w=12, h=8),

        # 6 ---------------------------------------------------------------------------------------------
        row("6. Queue and storage diagnostics (only the selected platform's sources)"),
        panel("Kafka consumer-group lag (ThingsBoard)", [
            ('sum by (consumergroup, topic) (kafka_consumergroup_lag and on (instance) (up{job="kafka"} == 1))', "{{consumergroup}} {{topic}}")], "short", w=12, h=8,
            desc="kafka-exporter: offset lag of each consumer group = messages produced but not yet consumed by "
                 "that group. A backlog signal, not proof of persistence."),
        panel("NATS (EdgeX, core NATS: no persistence, no durable queue)", [
            ('rate(gnatsd_varz_in_msgs' + R + ') and on (instance) (up{job="nats"} == 1)', "in msgs/s"),
            ('rate(gnatsd_varz_out_msgs' + R + ') and on (instance) (up{job="nats"} == 1)', "out msgs/s"),
            ('gnatsd_varz_slow_consumers and on (instance) (up{job="nats"} == 1)', "slow consumers")], "short", w=12, h=8,
            desc="Core NATS has no queue depth to report; out < in means fan-out or loss, not backlog."),
        panel("PostgreSQL: commits/s and inserted rows/s (ThingsBoard, EdgeX)", [
            ('sum by (datname) (rate(pg_stat_database_xact_commit{datname!~"template.*|postgres"}' + R + ') and on (instance) (up{job="postgres"} == 1))', "commits {{datname}}"),
            ('sum by (datname) (rate(pg_stat_database_tup_inserted{datname!~"template.*|postgres"}' + R + ') and on (instance) (up{job="postgres"} == 1))', "rows inserted {{datname}}")],
            "short", w=12, h=8, desc="Database activity. Rows are not logical messages (ThingsBoard stores 6 rows per reading)."),
        panel("PostgreSQL connections by state", [
            ('sum by (state) (pg_stat_activity_count and on (instance) (up{job="postgres"} == 1))', "{{state}}")], "short", w=12, h=8),
        panel("MongoDB operations/s (FIWARE)", [
            ('sum by (type) (rate(mongodb_op_counters_total' + R + ') and on (instance) (up{job="mongodb"} == 1))', "{{type}}")], "short", w=12, h=8),
        panel("HAProxy: responses/s and avg response time per replica (server)", [
            ('sum by (proxy, server) (rate(haproxy_server_http_responses_total' + R + ') and on (instance) (up{job="haproxy"} == 1)) > 0', "{{proxy}}/{{server}} resp/s"),
            ('max by (proxy, server) (haproxy_server_response_time_average_seconds and on (instance) (up{job="haproxy"} == 1)) * 1000 > 0', "{{proxy}}/{{server}} avg ms")],
            "short", w=12, h=8, desc="avg = HAProxy's moving average over its last 1024 requests."),

        # 7 ---------------------------------------------------------------------------------------------
        row("7. Measurement integrity and audit status"),
        panel("Generator (k6) container CPU and memory on the load host", [
            ('sum by (container) (rate(container_cpu_usage_seconds_total{job="loadgen-cadvisor",service="k6"}' + R + '))', "CPU cores {{container}}"),
            ('max by (container) (container_memory_working_set_bytes{job="loadgen-cadvisor",service="k6"}) / 1e9', "memory GB {{container}}")],
            "short", w=12, h=8),
        text("Generator health, records, audit", "Per-run generator health (drops, ticks, achieved rate, send-lag p99, "
             "early sends, record completeness) is computed **offline** from the run directory "
             "(`benchctl check`, `health.json`), and reconciliation by message ID is `benchctl reconcile`. "
             "Neither is in Prometheus. Raw records: `results/runs/<run_id>/` on ramonavm1.\n\n"
             "**Unavailable:** sink receipts, clock-synchronisation evidence, history audit. " + UNAVAILABLE, w=12, h=8),

        # 8 ---------------------------------------------------------------------------------------------
        row("8. Runs and derived results"),
        {"id": 900, "type": "annolist", "title": "Runs (annotations from benchctl -grafana-url): run id, plan, generator verdict",
         "gridPos": {"w": 12, "h": 8}, "options": {"onlyFromThisDashboard": False, "onlyInTimeRange": False,
                                                    "tags": ["benchmark"], "limit": 50, "showTags": True, "showUser": False,
                                                    "showTime": True, "navigateToPanel": False}},
        text("Derived research metrics", "C_SLO, CPU per persisted message (U), reference overhead (O), scaling "
             "efficiency (E), headroom and overload/recovery are **not computed here**. They need complete runs, "
             "audits and the future controller/offline analysis (`docs/tooling-interfaces.md`). Peak instantaneous "
             "throughput on this dashboard is not a capacity.", w=12, h=8),
    ]
    return {
        "uid": "iot-bench-overview", "title": "IoT benchmark overview", "tags": ["iot-bench"],
        "timezone": "utc", "refresh": "10s", "time": {"from": "now-30m", "to": "now"},
        "schemaVersion": 39, "editable": True,
        "annotations": {"list": [
            {"builtIn": 1, "datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": True, "hide": True,
             "iconColor": "rgba(0, 211, 255, 1)", "name": "Annotations & Alerts", "type": "dashboard"},
            {"datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": True, "iconColor": "orange",
             "name": "Benchmark runs", "target": {"type": "tags", "tags": ["benchmark"], "limit": 100, "matchAny": True}},
        ]},
        "templating": {"list": [
            {"name": "platform", "label": "platform", "type": "custom", "query": "thingsboard,fiware,edgex",
             "current": {"text": "edgex", "value": "edgex"}, "options": [], "hide": 0},
            {"name": "service", "label": "service", "type": "query", "datasource": DS,
             "query": {"query": 'label_values(bench:service_cpu_cores:rate30s{platform="$platform"}, service)', "refId": "A"},
             "refresh": 2, "multi": True, "includeAll": True, "allValue": ".*", "hide": 0},
            {"name": "run_id", "label": "run (k6 stream)", "type": "query", "datasource": DS,
             "query": {"query": "label_values(k6_http_reqs_total, run_id)", "refId": "A"},
             "refresh": 2, "sort": 2, "multi": True, "includeAll": True, "allValue": ".*", "hide": 0},
        ]},
        "panels": layout(panels),
    }


def write(name, d):
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, name + ".json"), "w") as f:
        json.dump(d, f, indent=1)
    print("wrote dashboards/%s.json" % name)


if __name__ == "__main__":
    write("iot-benchmark-overview", overview())
    community()
