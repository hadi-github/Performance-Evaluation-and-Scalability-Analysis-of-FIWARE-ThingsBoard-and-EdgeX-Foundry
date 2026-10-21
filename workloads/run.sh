#!/usr/bin/env bash
# Run one benchmark cell from the load-generator host (ramonavm1):
#
#   ./run.sh <thingsboard|fiware|edgex> <rate req/s> [duration] [--warmup 1m] [--devices 100] [--n 1]
#            [--sut 87.107.191.107] [--max-vus V] [--keep] [--no-verify]
#
# k6 runs in Docker (pinned to K6_CPUSET), streams its metrics to the local Prometheus (remote write) and writes
# results/<testid>/{summary.json,summary.txt,k6-summary.json,manifest.json}. Afterwards the SUT is checked through
# Prometheus/cAdvisor: any OOM kill or container (re)start during the run marks the run INVALID.
set -euo pipefail
cd "$(dirname "$0")"

usage() { sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }
[[ $# -ge 2 ]] || usage
PLATFORM="$1"; RATE="$2"; shift 2
case "$PLATFORM" in thingsboard|fiware|edgex) ;; *) usage ;; esac
[[ "$RATE" =~ ^[0-9]+$ ]] || usage
DURATION=10m
if [[ $# -gt 0 && "$1" != --* ]]; then DURATION="$1"; shift; fi
WARMUP=1m DEVICES=100 N=1 SUT_HOST=87.107.191.107 MAX_VUS="" CLEANUP=1 VERIFY=1
while [[ $# -gt 0 ]]; do
  case "$1" in
    --warmup) WARMUP="$2"; shift 2 ;;
    --devices) DEVICES="$2"; shift 2 ;;
    --n) N="$2"; shift 2 ;;
    --sut) SUT_HOST="$2"; shift 2 ;;
    --max-vus) MAX_VUS="$2"; shift 2 ;;
    --keep) CLEANUP=0; shift ;;
    --no-verify) VERIFY=0; shift ;;
    *) usage ;;
  esac
done

K6_IMAGE="${K6_IMAGE:-docker.arvancloud.ir/grafana/k6:1.8.1}"
K6_CPUSET="${K6_CPUSET:-0-2}"
PROM="${PROM_URL:-http://127.0.0.1:9090}"
GRAFANA="${GRAFANA_URL:-http://127.0.0.1:3000}"
GF_PASS="$(sed -n 's/^GF_ADMIN_PASSWORD=//p' ../monitoring/.env 2>/dev/null || true)"
case "$PLATFORM" in thingsboard) PROJECT=tb ;; *) PROJECT="$PLATFORM" ;; esac

TESTID="${PLATFORM}-n${N}-r${RATE}-$(date -u +%Y%m%dT%H%M%SZ)"
OUT="results/$TESTID"
mkdir -p "$OUT"
echo "== $TESTID: $RATE req/s for $DURATION (+ $WARMUP warm-up), $DEVICES devices, SUT $SUT_HOST"

# k6 redraws its progress bars every second; without a terminal that is one log line per second.
QUIET=""; [[ -t 1 ]] || QUIET="--quiet"
START=$(date -u +%s)
rc=0
docker run --rm $([[ -t 1 ]] && echo -t) --name "k6-$TESTID" --network host --cpuset-cpus "$K6_CPUSET" \
  -u "$(id -u):$(id -g)" -v "$PWD:/w" -w /w \
  -e PLATFORM="$PLATFORM" -e RATE="$RATE" -e DURATION="$DURATION" -e WARMUP="$WARMUP" -e DEVICES="$DEVICES" \
  -e N="$N" -e TESTID="$TESTID" -e SUT_HOST="$SUT_HOST" -e MAX_VUS="$MAX_VUS" -e CLEANUP="$CLEANUP" -e VERIFY="$VERIFY" \
  -e K6_PROMETHEUS_RW_SERVER_URL="$PROM/api/v1/write" \
  -e K6_PROMETHEUS_RW_TREND_STATS='p(50),p(95),p(99),avg,max' \
  -e K6_PROMETHEUS_RW_PUSH_INTERVAL=5s \
  "$K6_IMAGE" run $QUIET -o experimental-prometheus-rw "$PLATFORM.js" || rc=$?
END=$(date -u +%s)

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

# Mark the run window in Grafana.
if [[ -n "$GF_PASS" ]]; then
  curl -s -o /dev/null -u "admin:$GF_PASS" -H 'Content-Type: application/json' -X POST "$GRAFANA/api/annotations" \
    -d "{\"time\":$((START * 1000)),\"timeEnd\":$((END * 1000)),\"tags\":[\"benchmark\",\"$PLATFORM\",\"$TESTID\"],\"text\":\"$TESTID ($RATE req/s)\"}" \
    || echo "grafana annotation failed"
fi
echo "results: $(pwd)/$OUT"
exit "$rc"
