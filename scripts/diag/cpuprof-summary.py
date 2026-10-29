#!/usr/bin/env python3
"""Summarise a V8 .cpuprofile (scripts/diag/node-cpuprof.js) by module group (docs/methodology/hypotheses.md H-F5).

  scripts/diag/cpuprof-summary.py <file.cpuprofile> [--json]

Shares are of non-idle samples. "inclusive": a sample counts for a group if any frame of its stack belongs to the
group (groups overlap). "self": the sample's own (top) frame's group (groups partition the non-idle samples).
"""
import json
import sys

GROUPS = [  # (group, substring of the frame's url or function name); first match wins for "self"
    ("gc", "(garbage collector)"),
    ("program/native", "(program)"),
    ("mongoose/mongodb/bson", "node_modules/mongoose/"), ("mongoose/mongodb/bson", "node_modules/mongodb/"),
    ("mongoose/mongodb/bson", "node_modules/bson/"), ("mongoose/mongodb/bson", "node_modules/mquery/"),
    ("jexl", "node_modules/jexl/"),
    ("logging", "node_modules/logops/"),
    ("http client to Orion", "node_modules/request/"), ("http client to Orion", "node_modules/got/"),
    ("http client to Orion", "node_modules/axios/"),
    ("http server (express/body-parser)", "node_modules/express/"), ("http server (express/body-parser)", "node_modules/body-parser/"),
    ("http server (express/body-parser)", "node_modules/raw-body/"),
    ("UL parser / iotagent-ul", "/opt/iotaul/lib/"),
    ("iotagent-node-lib NGSI", "iotagent-node-lib/lib/services/ngsi/"),
    ("iotagent-node-lib devices/groups", "iotagent-node-lib/lib/services/devices/"),
    ("iotagent-node-lib devices/groups", "iotagent-node-lib/lib/services/groups/"),
    ("iotagent-node-lib other", "iotagent-node-lib/"),
    ("other node_modules", "node_modules/"),
    ("node core", "node:"),
]


def group_of(frame):
    key = frame.get("url", "") or ""
    name = frame.get("functionName", "")
    for g, sub in GROUPS:
        if sub.startswith("(") and name == sub:
            return g
        if not sub.startswith("(") and sub in key:
            return g
    return "other"


def main(path, as_json):
    p = json.load(open(path))
    nodes = {n["id"]: n for n in p["nodes"]}
    parent = {}
    for n in p["nodes"]:
        for c in n.get("children", []):
            parent[c] = n["id"]
    total = len(p["samples"])
    idle = 0
    incl, self_ = {}, {}
    fn_self = {}
    for sid in p["samples"]:
        n = nodes[sid]
        if n["callFrame"].get("functionName") == "(idle)":
            idle += 1
            continue
        g = group_of(n["callFrame"])
        self_[g] = self_.get(g, 0) + 1
        k = f'{n["callFrame"].get("functionName") or "(anonymous)"} {n["callFrame"].get("url", "").split("node_modules/")[-1]}:{n["callFrame"].get("lineNumber", -1) + 1}'
        fn_self[k] = fn_self.get(k, 0) + 1
        seen, cur = set(), sid
        while cur is not None:
            seen.add(group_of(nodes[cur]["callFrame"]))
            cur = parent.get(cur)
        for s in seen:
            incl[s] = incl.get(s, 0) + 1
    busy = total - idle
    dur_s = (p["endTime"] - p["startTime"]) / 1e6
    out = {"file": path, "samples": total, "idle": idle, "busy_samples": busy, "duration_s": dur_s,
           "busy_share_of_wall": busy / total,
           "inclusive_share_of_busy": {k: v / busy for k, v in sorted(incl.items(), key=lambda kv: -kv[1])},
           "self_share_of_busy": {k: v / busy for k, v in sorted(self_.items(), key=lambda kv: -kv[1])},
           "top_self_functions": [(k, v / busy) for k, v in sorted(fn_self.items(), key=lambda kv: -kv[1])[:15]]}
    if as_json:
        print(json.dumps(out, indent=1))
        return
    print(f"{path}: {total} samples over {dur_s:.1f} s, busy {busy} ({busy / total:.1%})")
    print("inclusive share of busy samples (groups overlap):")
    for k, v in out["inclusive_share_of_busy"].items():
        print(f"  {k:38s} {v:6.1%}")
    print("self share of busy samples (partition):")
    for k, v in out["self_share_of_busy"].items():
        print(f"  {k:38s} {v:6.1%}")
    print("top functions by self time:")
    for k, v in out["top_self_functions"]:
        print(f"  {v:6.1%}  {k}")


if __name__ == "__main__":
    main(sys.argv[1], "--json" in sys.argv)
