#!/usr/bin/env bash
# Run one k6 feasibility case against the local fixture target (docs/methodology/k6-feasibility.md).
#
#   tests/feasibility/run-case.sh <case-name> <probe.js> [KEY=VALUE ...]
#
# KEY=VALUE pairs go to k6 as environment variables (RATE, DURATION, PRE_VUS, MAX_VUS, DELAY_MS, ...), except:
#   K6_CPUS=<n>      limit the k6 container's CPU (generator-delay cases)
#   TARGET_DELAY=<d> default target delay (Go duration, e.g. 200ms)
# Raw evidence: results/feasibility/<case>/{target.ndjson,k6.json.gz,k6.log,case.env}
set -euo pipefail
cd "$(dirname "$0")/../.."
[[ $# -ge 2 ]] || { sed -n '2,10p' "$0"; exit 2; }
CASE="$1"; PROBE="$2"; shift 2

K6_IMAGE="${K6_IMAGE:-docker.arvancloud.ir/grafana/k6:1.8.1}"
OUT="results/feasibility/$CASE"
rm -rf "$OUT"; mkdir -p "$OUT"
K6_CPUS="" TARGET_DELAY=0s
envs=()
for kv in "$@"; do
  case "$kv" in
    K6_CPUS=*) K6_CPUS="${kv#*=}" ;;
    TARGET_DELAY=*) TARGET_DELAY="${kv#*=}" ;;
    *) envs+=(-e "$kv") ;;
  esac
done
printf '%s\n' "case=$CASE" "probe=$PROBE" "k6_image=$K6_IMAGE" "k6_cpus=$K6_CPUS" "target_delay=$TARGET_DELAY" "$@" > "$OUT/case.env"

go build -o "$OUT/.fixture-target" ./cmd/fixture-target
"$OUT/.fixture-target" -addr 127.0.0.1:18080 -delay "$TARGET_DELAY" -out "$OUT/target.ndjson" 2> "$OUT/target.log" &
TPID=$!
trap 'kill $TPID 2>/dev/null || true' EXIT
for _ in $(seq 50); do curl -fs 127.0.0.1:18080/stats >/dev/null 2>&1 && break; sleep 0.1; done

cpus=(); [[ -n "$K6_CPUS" ]] && cpus=(--cpus "$K6_CPUS")
rc=0
docker run --rm --network host "${cpus[@]}" -u "$(id -u):$(id -g)" -v "$PWD:/w" -w /w "${envs[@]}" \
  "$K6_IMAGE" run --quiet --out "json=$OUT/k6.json.gz" "$PROBE" > "$OUT/k6.log" 2>&1 || rc=$?
curl -fs 127.0.0.1:18080/stats > "$OUT/target-stats.json" || true
kill -TERM $TPID; wait $TPID || rc=$?
trap - EXIT
rm -f "$OUT/.fixture-target"
echo "case $CASE: k6/target exit $rc, target $(cat "$OUT/target-stats.json" 2>/dev/null)"
exit $rc
