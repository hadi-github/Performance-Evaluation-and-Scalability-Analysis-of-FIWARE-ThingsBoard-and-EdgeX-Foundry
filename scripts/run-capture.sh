#!/usr/bin/env bash
# Generic prospective capture for one ordinary run:
#   sampler + protocol capture + offline dispatch-to-receipt analysis.
#
#   scripts/run-capture.sh PLAN RUN_ID RESULTS_DIR PLATFORM
#
# Runs on the load host. It does not deploy or scale the SUT; readiness and
# stack switching belong to the campaign launcher.
set -euo pipefail
if [[ "${RUN_CAPTURE_TIMEOUT_WRAPPED:-0}" != 1 ]]; then
  export RUN_CAPTURE_TIMEOUT_WRAPPED=1
  exec timeout --signal=TERM --kill-after=180s "${RUN_CAPTURE_TIMEOUT:-3600}" "$0" "$@"
fi
cd "$(dirname "$0")/.."

plan=${1:?plan JSON required}
run_id=${2:?run id required}
results=${3:?results directory required}
platform=${4:?platform required}
case "$platform" in
  thingsboard|fiware|edgex) ;;
  *) echo "platform must be thingsboard, fiware or edgex" >&2; exit 2 ;;
esac
[[ "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || { echo "unsafe run id" >&2; exit 2; }
[[ -s "$plan" ]] || { echo "plan not found: $plan" >&2; exit 2; }

python=${PYTHON:-python3}
export SINK_PORT=${SINK_PORT:-18091}
export GENERATOR_CPUSET=${GENERATOR_CPUSET:-0-1}
export PROM_RW=${PROM_RW:-http://127.0.0.1:9090/api/v1/write}
export GRAFANA_URL=${GRAFANA_URL:-}
export BENCHCTL=${BENCHCTL:-scripts/benchctl-calibrated.sh}
sink_bin=${SINK_BIN:-bin/sink}
diag="$results/$run_id.generator-diagnostics.jsonl"
ready="$results/$run_id.sampler-ready.json"
sampler_log="$diag.log"
mkdir -p "$results"

for path in "$results/$run_id" "$results/$run_id.sink.ndjson" "$results/$run_id.capture-window.json" \
  "$results/$run_id.dispatch-analysis.json" "$diag" "$diag.status.json" "$ready"; do
  [[ ! -e "$path" ]] || { echo "refusing existing evidence: $path" >&2; exit 2; }
done
for executable in "$BENCHCTL" "$sink_bin" scripts/run-protocol-capture.sh \
  scripts/generator-diagnostic-sampler.py scripts/post-run-analysis.sh scripts/analyze-dispatch-run.py; do
  [[ -x "$executable" ]] || { echo "required executable missing: $executable" >&2; exit 2; }
done

sampler_pid=""
cleanup() {
  if [[ -n "$sampler_pid" ]] && kill -0 "$sampler_pid" 2>/dev/null; then
    kill -TERM "$sampler_pid" 2>/dev/null || true
    wait "$sampler_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

"$python" scripts/generator-diagnostic-sampler.py "$run_id" "$diag" \
  --prometheus "${PROMETHEUS_URL:-http://127.0.0.1:9090}" \
  --wait-seconds "${DIAGNOSTIC_WAIT_SECONDS:-600}" \
  --tail-seconds "${DIAGNOSTIC_TAIL_SECONDS:-930}" \
  --ready-file "$ready" > "$sampler_log" 2>&1 &
sampler_pid=$!

for _ in $(seq 1 240); do
  [[ -s "$ready" ]] && break
  kill -0 "$sampler_pid" 2>/dev/null || { echo "sampler exited before readiness; see $sampler_log" >&2; exit 2; }
  sleep 0.5
done
[[ -s "$ready" ]] || { echo "sampler readiness timed out; see $sampler_log" >&2; exit 2; }

set +e
scripts/run-protocol-capture.sh "$plan" "$run_id" "$results"
capture_rc=$?
set -e

set +e
wait "$sampler_pid" 2>/dev/null
sampler_rc=$?
set -e
sampler_pid=""

printf '%s\n' "$capture_rc" > "$results/$run_id.capture-exit-code"
printf '%s\n' "$sampler_rc" > "$results/$run_id.sampler-exit-code"

analysis_rc=0
if [[ -s "$results/$run_id.capture-window.json" ]]; then
  set +e
  scripts/post-run-analysis.sh "$run_id" "$results" "$platform"
  analysis_rc=$?
  set -e
else
  echo "capture window missing; offline analysis not run" >&2
  analysis_rc=4
fi

if (( capture_rc != 0 )); then
  exit "$capture_rc"
fi
if (( analysis_rc != 0 )); then
  exit "$analysis_rc"
fi
if (( sampler_rc != 0 )); then
  exit "$sampler_rc"
fi
