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


def panel(title, exprs, unit="short", w=12, h=8, kind="timeseries", desc="", stack=False, legend_calcs=None):
    _id[0] += 1
    targets = [{"datasource": DS, "expr": e, "legendFormat": lf, "refId": chr(65 + i), "range": True}
               for i, (e, lf) in enumerate(exprs)]
    p = {"id": _id[0], "type": kind, "title": title, "description": desc, "datasource": DS, "targets": targets,
         "gridPos": {"w": w, "h": h},
         "fieldConfig": {"defaults": {"unit": unit, "custom": {"fillOpacity": 10 if not stack else 40,
                                                                "stacking": {"mode": "normal" if stack else "none"},
                                                                "showPoints": "never"}},
                         "overrides": []},
         "options": {"legend": {"displayMode": "table", "placement": "right",
                                "calcs": legend_calcs or ["mean", "max"]},
                     "tooltip": {"mode": "multi", "sort": "desc"}}}
    return p


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


def overview():
    K = 'testid=~"$testid"'
    SEND = K + ',name="send"'
    C = 'job="cadvisor",project="$project"'
    R = "[$__rate_interval]"
    panels = [
        row("Load (k6 on ramonavm1)"),
        panel("Telemetry requests/s by phase", [
            ('sum by (testid, scenario) (rate(k6_http_reqs_total{%s}%s))' % (SEND, R), "{{scenario}} {{testid}}")],
            "reqps", desc="Achieved arrival rate. The steady phase must equal the target rate (run.sh checks within 1%)."),
        panel("Latency (steady phase)", [
            ('max by (testid) (k6_http_req_duration_p50{%s,scenario="steady"})' % SEND, "p50 {{testid}}"),
            ('max by (testid) (k6_http_req_duration_p95{%s,scenario="steady"})' % SEND, "p95 {{testid}}"),
            ('max by (testid) (k6_http_req_duration_p99{%s,scenario="steady"})' % SEND, "p99 {{testid}}")],
            "s", desc="Per push interval (5 s) percentiles reported by k6."),
        panel("Errors and dropped iterations /s", [
            ('sum by (testid, status) (rate(k6_http_reqs_total{%s,expected_response="false"}%s))' % (SEND, R),
             "HTTP {{status}} {{testid}}"),
            ('sum by (testid) (rate(k6_dropped_iterations_total{%s}%s))' % (K, R), "dropped {{testid}}")],
            "short", desc="Dropped iterations mean the generator could not hold the rate: the run is INVALID."),
        panel("Active VUs", [('sum by (testid) (k6_vus{%s})' % K, "VUs {{testid}}"),
                             ('sum by (testid) (k6_vus_max{%s})' % K, "max VUs {{testid}}")], "short"),

        row("SUT CPU (ramonavm2, cAdvisor / node-exporter)"),
        panel("CPU per tier (cores)", [
            ('sum by (service) (rate(container_cpu_usage_seconds_total{%s}%s))' % (C, R), "{{service}}")],
            "short", stack=True, desc="All replicas of a tier summed. Pool tiers share the platform pool (N cores)."),
        panel("CPU per container / replica (cores)", [
            ('sum by (container) (rate(container_cpu_usage_seconds_total{%s}%s))' % (C, R), "{{container}}")],
            "short"),
        panel("SUT busy per core", [
            ('1 - avg by (cpu) (rate(node_cpu_seconds_total{job="node",mode="idle"}%s))' % R, "cpu{{cpu}}")],
            "percentunit", w=12,
            desc="Core 0: OS + exporters, 1: HAProxy, 2-3: infrastructure, 4..: platform pool (README, Resource layout)."),
        panel("Exporter overhead on the SUT (cores)", [
            ('sum by (container) (rate(container_cpu_usage_seconds_total{job="cadvisor",project="exporters"}%s))' % R,
             "{{container}}")], "short", w=12, desc="Monitoring agents, pinned to core 0."),

        row("SUT memory"),
        panel("Memory working set / limit", [
            ('max by (container) (container_memory_working_set_bytes{%s}) / max by (container) '
             '(container_spec_memory_limit_bytes{%s} > 0)' % (C, C), "{{container}}")],
            "percentunit", desc="Above ~90% the kernel OOM-kills the container (see next panel)."),
        panel("OOM kills and container (re)starts", [
            ('sum by (container) (increase(container_oom_events_total{%s}[1m]))' % C, "OOM {{container}}"),
            ('sum by (container) (changes(container_start_time_seconds{%s}[1m]))' % C, "start {{container}}")],
            "short", desc="Any non-zero value during a run makes it INVALID (run.sh checks this)."),

        row("Load balancer (HAProxy, per replica)"),
        panel("HTTP responses/s per backend server", [
            ('sum by (proxy, server) (rate(haproxy_server_http_responses_total[$__rate_interval])) > 0', "{{proxy}}/{{server}}")],
            "reqps", desc="How evenly the LB spreads load over the replicas of each tier."),
        panel("Average response time per backend server", [
            ('max by (proxy, server) (haproxy_server_response_time_average_seconds) > 0', "{{proxy}}/{{server}}")],
            "s", desc="HAProxy's moving average over the last 1024 requests."),
        panel("5xx responses/s per backend", [
            ('sum by (proxy) (rate(haproxy_backend_http_responses_total{code="5xx"}[$__rate_interval]))', "{{proxy}}")],
            "reqps"),
        panel("Queued requests per backend", [
            ('sum by (proxy) (haproxy_backend_current_queue)', "{{proxy}}")], "short"),

        row("Network and load generator"),
        panel("SUT network", [
            ('sum(rate(node_network_receive_bytes_total{job="node",device!~"lo|veth.*|docker.*|br-.*"}%s))' % R, "rx"),
            ('sum(rate(node_network_transmit_bytes_total{job="node",device!~"lo|veth.*|docker.*|br-.*"}%s))' % R, "tx")],
            "Bps"),
        panel("Load generator busy per core (ramonavm1)", [
            ('1 - avg by (cpu) (rate(node_cpu_seconds_total{job="loadgen-node",mode="idle"}%s))' % R, "cpu{{cpu}}")],
            "percentunit", desc="k6 runs on cores 0-2, Prometheus/Grafana on 3. Near 100% means the generator limits the test."),
    ]
    return {
        "uid": "iot-bench-overview",
        "title": "IoT benchmark overview",
        "tags": ["iot-bench"],
        "timezone": "utc",
        "refresh": "10s",
        "time": {"from": "now-30m", "to": "now"},
        "schemaVersion": 39,
        "editable": True,
        "annotations": {"list": [
            {"builtIn": 1, "datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": True, "hide": True,
             "iconColor": "rgba(0, 211, 255, 1)", "name": "Annotations & Alerts", "type": "dashboard"},
            {"datasource": {"type": "grafana", "uid": "-- Grafana --"}, "enable": True, "iconColor": "orange",
             "name": "Benchmark runs", "target": {"type": "tags", "tags": ["benchmark"], "limit": 100,
                                                  "matchAny": True}},
        ]},
        "templating": {"list": [
            {"name": "project", "label": "platform", "type": "custom", "query": "thingsboard : tb,fiware : fiware,edgex : edgex",
             "current": {"text": "fiware", "value": "fiware"}, "options": [], "hide": 0},
            {"name": "testid", "label": "test id", "type": "query", "datasource": DS,
             "query": {"query": 'label_values(k6_http_reqs_total{platform="${project:text}"}, testid)', "refId": "A"},
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
