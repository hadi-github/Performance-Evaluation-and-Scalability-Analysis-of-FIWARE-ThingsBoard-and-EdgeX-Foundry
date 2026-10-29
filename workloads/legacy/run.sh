#!/usr/bin/env bash
# Run one benchmark cell from the load-generator host (ramonavm1):
#
#   ./run.sh <thingsboard|fiware|edgex> <rate req/s> [duration] [--warmup 1m] [--devices 100] [--n 1]
#            [--sut 87.107.191.107] [--max-vus V] [--keep] [--no-verify]
#            [--record-identity <ssh-alias>:<repo-dir-on-SUT>]
#
# k6 runs in Docker (pinned to K6_CPUSET), streams its metrics to the local Prometheus (remote write) and writes
# results/<testid>/{summary.json,summary.txt,k6-summary.json,manifest.json} at the repository root. The gate is
# the planned schedule (planned arrivals, dropped iterations == 0, achieved rate), record completeness (SUT health
# + the sink's received/duplicates/unparsed counters when reachable; stored == sent where a count exists) and,
# with --record-identity, identity-set reconciliation: the run's (run_id, seq) identities are exported from the
# SUT store (scripts/history-ids-audit.sh, needs SUT-side access; supported thingsboard + QL candidate fiware)
# and compared to the planned arrivals — a missing identity is not allowed to be offset by a duplicate.
# The verdict is INVALID on: dropped iterations, achieved-rate deviation, OOM/restarts, sink duplicates/unparsed,
# or (with identity recording) missing/duplicate identities.
set -euo pipefail
cd "$(dirname "$0")"

usage() { sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }
[[ $# -ge 2 ]] || usage
PLATFORM="$1"; RATE="$2"; shift 2
case "$PLATFORM" in thingsboard|fiware|edgex) ;; *) usage ;; esac
[[ "$RATE" =~ ^[0-9]+$ ]] || usage
DURATION=10m
if [[ $# -gt 0 && "$1" != --* ]]; then DURATION="$1"; shift; fi
WARMUP=1m DEVICES=100 N=1 SUT_HOST=87.107.191.107 MAX_VUS="" CLEANUP=1 VERIFY=1 RECORD_IDENTITY=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --warmup) WARMUP="$2"; shift 2 ;;
    --devices) DEVICES="$2"; shift 2 ;;
    --n) N="$2"; shift 2 ;;
    --sut) SUT_HOST="$2"; shift 2 ;;
    --max-vus) MAX_VUS="$2"; shift 2 ;;
    --keep) CLEANUP=0; shift ;;
    --no-verify) VERIFY=0; shift ;;
    --record-identity) RECORD_IDENTITY="${2:?usage: --record-identity '<ssh-alias>:<repo-dir-on-SUT>'}"; shift 2 ;;
    *) usage ;;
  esac
done

K6_IMAGE="${K6_IMAGE:-docker.arvancloud.ir/grafana/k6:1.8.1}"
K6_CPUSET="${K6_CPUSET:-0-2}"
PROM="${PROM_URL:-http://127.0.0.1:9090}"
GRAFANA="${GRAFANA_URL:-http://127.0.0.1:3000}"
# the sink's exported counters turn the recording path into part of the verdict (duplicates/unparsed = INVALID)
SINK_METRICS="${SINK_METRICS_URL:-http://127.0.0.1:18090/metrics}"
GF_PASS="$(sed -n 's/^GF_ADMIN_PASSWORD=//p' ../../monitoring/.env 2>/dev/null || true)"
case "$PLATFORM" in thingsboard) PROJECT=tb ;; *) PROJECT="$PLATFORM" ;; esac

TESTID="${PLATFORM}-n${N}-r${RATE}-$(date -u +%Y%m%dT%H%M%SZ)"
# run records always land in the repository-root results/ directory (same ledger as benchctl runs)
REPO="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$REPO/results/$TESTID"
mkdir -p "$OUT"
echo "== $TESTID: $RATE req/s for $DURATION (+ $WARMUP warm-up), $DEVICES devices, SUT $SUT_HOST"

# k6 redraws its progress bars every second; without a terminal that is one log line per second.
QUIET=""; [[ -t 1 ]] || QUIET="--quiet"
START=$(date -u +%s)
# Sink counters baseline: before/after deltas so a long-running multi-test sink does not carry old totals in.
if curl -sf --max-time 3 "$SINK_METRICS" > "$OUT/sink-before.txt" 2>/dev/null; then SINK_BEFORE=1; else SINK_BEFORE=0; fi
rc=0
docker run --rm $([[ -t 1 ]] && echo -t) --name "k6-$TESTID" --network host --cpuset-cpus "$K6_CPUSET" \
  -u "$(id -u):$(id -g)" -v "$REPO:/w" -w /w \
  -e PLATFORM="$PLATFORM" -e RATE="$RATE" -e DURATION="$DURATION" -e WARMUP="$WARMUP" -e DEVICES="$DEVICES" \
  -e N="$N" -e TESTID="$TESTID" -e SUT_HOST="$SUT_HOST" -e MAX_VUS="$MAX_VUS" -e CLEANUP="$CLEANUP" -e VERIFY="$VERIFY" \
  -e RESULTS_DIR="results" \
  -e K6_PROMETHEUS_RW_SERVER_URL="$PROM/api/v1/write" \
  -e K6_PROMETHEUS_RW_TREND_STATS='p(50),p(95),p(99),avg,max' \
  -e K6_PROMETHEUS_RW_PUSH_INTERVAL=5s \
  "$K6_IMAGE" run $QUIET -o experimental-prometheus-rw "legacy/$PLATFORM.js" || rc=$?
END=$(date -u +%s)
if curl -sf --max-time 3 "$SINK_METRICS" > "$OUT/sink-after.txt" 2>/dev/null; then SINK_AFTER=1; else SINK_AFTER=0; fi

# SUT health during the run, from cAdvisor on the SUT (via Prometheus), plus the replica count per tier.
python3 - "$OUT" "$PROM" "$START" "$END" "$PROJECT" "$K6_IMAGE" "$rc" <<'EOF'
import json, sys, urllib.parse, urllib.request
out, prom, start, end, project, image, rc = sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4]), sys.argv[5], sys.argv[6], int(sys.argv[7])
window = max(end - start, 60)

def q(expr):
    url = prom + "/api/v1/query?" + urllib.parse.urlencode({"query": expr, "time": end})
    try:
        with urllib.request.urlopen(url, timeout=20) as r:
            return json.load(r)["data"]["result"]
    except Exception as e:
        print("prometheus query failed (%s): %s" % (expr, e))
        return None

sel = 'job="cadvisor",container_label_com_docker_compose_project="%s"' % project
oom = q('sum(increase(container_oom_events_total{%s}[%ds]))' % (sel, window))
started = q('container_start_time_seconds{%s} > %d' % (sel, start))
tiers = q('count by (container_label_com_docker_compose_service) (time() - container_last_seen{%s} < 30)' % sel)

manifest = {"testid": out.split("/")[-1], "start_utc": start, "end_utc": end, "k6_image": image, "k6_exit_code": rc}
manifest["sut_oom_events"] = round(float(oom[0]["value"][1])) if oom else (0 if oom == [] else None)
manifest["sut_containers_started_during_run"] = sorted(
    r["metric"].get("name", r["metric"].get("container_label_com_docker_compose_service", "?")) for r in started
) if started is not None else None
manifest["sut_containers_per_service"] = {
    r["metric"].get("container_label_com_docker_compose_service", "?"): int(r["value"][1]) for r in (tiers or [])
}
try:
    summary = json.load(open(out + "/summary.json"))
except FileNotFoundError:
    summary = {"valid": False, "invalid_reasons": ["k6 produced no summary (exit code %d)" % rc]}
reasons = summary.setdefault("invalid_reasons", [])
if manifest["sut_oom_events"]:
    reasons.append("%d OOM kill(s) on the SUT during the run" % manifest["sut_oom_events"])
if manifest["sut_containers_started_during_run"]:
    reasons.append("SUT containers (re)started during the run: " + ", ".join(manifest["sut_containers_started_during_run"]))
if manifest["sut_oom_events"] is None or manifest["sut_containers_started_during_run"] is None:
    reasons.append("SUT health could not be checked (Prometheus/cAdvisor unavailable)")
summary["valid"] = not reasons
summary["manifest"] = manifest
json.dump(summary, open(out + "/summary.json", "w"), indent=2)
json.dump(manifest, open(out + "/manifest.json", "w"), indent=2)
print("SUT: oom=%s, started=%s, containers per service=%s" % (
    manifest["sut_oom_events"], manifest["sut_containers_started_during_run"], manifest["sut_containers_per_service"]))
print("==> %s: %s%s" % (manifest["testid"], "VALID" if summary["valid"] else "INVALID",
                        "" if summary["valid"] else " (" + "; ".join(reasons) + ")"))
EOF

# Sink counter deltas change the verdict: duplicate observations or unparsed requests on the recording
# path invalidate even a count-clean run.
if [[ "$SINK_BEFORE" == 1 && "$SINK_AFTER" == 1 ]]; then
python3 - "$OUT" <<'EOF'
import json, re, sys
out = sys.argv[1]
summary = json.load(open(out + "/summary.json")); manifest = summary["manifest"]
reasons = summary.setdefault("invalid_reasons", [])

def counters(path):
    c = {}
    text = open(path).read()
    for key in ("sink_received_total", "sink_unparsed_total", "sink_overflow_total", "sink_duplicates_total"):
        m = re.search(r"^" + key + r" ([0-9.e+E-]+)$", text, re.M)
        if m: c[key] = float(m.group(1))
    return c

before, after = counters(out + "/sink-before.txt"), counters(out + "/sink-after.txt")
delta = {}
for k, v in before.items():
    if k in after: delta[k] = round(after[k] - v, 3)
manifest["sink_counters_delta"] = delta
bad = []
if not delta:
    summary.setdefault("findings", []).append("sink counters were reachable but no expected counters parsed; record verification by sink inconclusive")
if delta.get("sink_unparsed_total", 0) > 0:
    bad.append("sink recorded %g unparsed request(s)" % delta["sink_unparsed_total"])
if delta.get("sink_overflow_total", 0) > 0:
    bad.append("sink recorded %g overflow drop(s)" % delta["sink_overflow_total"])
if delta.get("sink_duplicates_total", 0) > 0:
    bad.append("sink recorded %g duplicate identity/identities" % delta["sink_duplicates_total"])
if "sink_received_total" in delta and delta["sink_received_total"] <= 0:
    bad.append("sink recorded no messages (received delta %.03f)" % delta["sink_received_total"])
reasons.extend(bad)
summary["valid"] = not reasons
json.dump(summary, open(out + "/summary.json", "w"), indent=2)
print("sink: delta=" + json.dumps(delta, sort_keys=True) + (" -> " + "; ".join(bad) if bad else ""))
EOF
elif [[ "$SINK_BEFORE" == 0 && "$SINK_AFTER" == 0 ]]; then
python3 - "$OUT" <<'EOF'
import json, sys
out = sys.argv[1]
summary = json.load(open(out + "/summary.json")); manifest = summary["manifest"]
manifest["sink_counters_delta"] = None
reasons = summary.setdefault("invalid_reasons", [])
summary.setdefault("findings", []).append("sink counters unavailable at " + "$SINK_METRICS" + ": record-completeness by sink not verified")
summary["valid"] = not reasons
json.dump(summary, open(out + "/summary.json", "w"), indent=2)
print("sink: counters unavailable (finding recorded)")
EOF
fi

# Identity-set reconciliation (optional --record-identity): export the run's (run_id, seq) identities from the
# SUT store and compare them with the planned arrivals. Missing identities cannot be offset by duplicates.
if [[ -n "$RECORD_IDENTITY" ]]; then
  ID_ALIAS="${RECORD_IDENTITY%%:*}"; ID_DIR="${RECORD_IDENTITY#*:}"
  if ssh "$ID_ALIAS" "cd '$ID_DIR' && ./scripts/history-ids-audit.sh $PLATFORM $TESTID" \
      > "$OUT/identity-export.ndjson" 2> "$OUT/identity-export.errors.log"; then
    python3 - "$OUT" "$TESTID" <<'EOF'
import collections, json, sys
out, testid = sys.argv[1], sys.argv[2]
summary = json.load(open(out + "/summary.json")); manifest = summary["manifest"]
reasons = summary.setdefault("invalid_reasons", [])

seqs = []
for line in open(out + "/identity-export.ndjson"):
    line = line.strip()
    if not line: continue
    try: r = json.loads(line)
    except Exception: continue
    seq = r.get("seq")
    if isinstance(seq, str) and seq.isdigit(): seq = int(seq)
    if isinstance(seq, int): seqs.append(int(seq))
counts   = collections.Counter(seqs)
planned  = summary.get("planned_arrivals") or 0
if planned == 0:
    planned = round((summary.get("steady_duration_s", 0) + summary.get("warmup_s", 0)) * summary.get("target_rate", 0))
in_planned   = [k for k in counts if 1 <= k <= planned]
missing      = planned - len(in_planned)
duplicates   = sum(c - 1 for c in counts.values() if c > 1)
dropped      = summary.get("dropped_iterations", 0)
examples_missing = sorted(set(range(1, planned + 1)) - set(in_planned))[:10]
examples_dup     = sorted(k for k, c in counts.items() if c > 1)[:10]
other_runs = sorted(set(json.loads(l)["run_id"] for l in open(out + "/identity-export.ndjson") if l.strip()
                        and json.loads(l).get("run_id") != testid))
summary["reconciliation"] = {
    "basis": "identity export (run_id, seq) compared with planned arrivals",
    "planned_arrivals": planned, "exported_identities": len(seqs), "distinct": len(counts),
    "missing": missing, "duplicate_copies": duplicates,
    "examples_missing": examples_missing, "examples_duplicated": examples_dup,
    "other_run_ids_in_export": other_runs,
    "dropped_iterations_for_context": dropped,
    "note": "with dropped_iterations == 0, every missing identity is a platform-side loss; a positive duplicate count is a platform-side duplicate",
}
if len(seqs) == 0:
    summary.setdefault("findings", []).append(
        "identity export produced 0 rows for this run: the harness sent no (run_id, seq) identities; count reconciliation only (platform: $PLATFORM)")
else:
    if missing > 0: reasons.append("identity reconciliation: %d missing identities (examples %s)" % (missing, examples_missing))
    if duplicates > 0: reasons.append("identity reconciliation: %d extra duplicate copies (examples %s)" % (duplicates, examples_dup))
    if other_runs: summary.setdefault("findings", []).append("identity export contains foreign run ids: " + ", ".join(other_runs))
if open(out + "/identity-export.errors.log").read().strip():
    n_unrec = sum(l.count('"history_export_row_unrecoverable": true') for l in open(out + "/identity-export.errors.log"))
    if n_unrec: reasons.append("identity export: %d unrecoverable store row(s) (never silently dropped)" % n_unrec)
summary["valid"] = not reasons
json.dump(summary, open(out + "/summary.json", "w"), indent=2)
recon = summary["reconciliation"]
print("identity: planned %d, exported %d (distinct %d), missing %d, duplicate copies %d -> %s" % (
    recon["planned_arrivals"], recon["exported_identities"], recon["distinct"], recon["missing"],
    recon["duplicate_copies"], "clean" if (missing == 0 and duplicates == 0) else "NOT clean"))
EOF
  else
    python3 - "$OUT" <<'EOF'
import json, sys
out = sys.argv[1]
summary = json.load(open(out + "/summary.json")); manifest = summary["manifest"]
manifest["identity_export"] = "failed (see identity-export.errors.log)"
summary.setdefault("findings", []).append("identity export ssh/export step failed; identity reconciliation NOT performed (counts-only verdict)")
summary["valid"] = not summary["invalid_reasons"]
json.dump(summary, open(out + "/summary.json", "w"), indent=2)
print("identity: EXPORT FAILED — verdict is counts-only (not identity-reconciled)")
EOF
  fi
fi

# Mark the run window in Grafana.
if [[ -n "$GF_PASS" ]]; then
  curl -s -o /dev/null -u "admin:$GF_PASS" -H 'Content-Type: application/json' -X POST "$GRAFANA/api/annotations" \
    -d "{\"time\":$((START * 1000)),\"timeEnd\":$((END * 1000)),\"tags\":[\"benchmark\",\"$PLATFORM\",\"$TESTID\"],\"text\":\"$TESTID ($RATE req/s)\"}" \
    || echo "grafana annotation failed"
fi
echo "results: $OUT"
exit "$rc"
