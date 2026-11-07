#!/usr/bin/env bash
# Offline dispatch-to-receipt assessment for one completed capture.
#
#   scripts/post-run-analysis.sh RUN_ID RESULTS_DIR PLATFORM
#
# Runs on the load host, after run-protocol-capture.sh has written the capture
# window and while the platform is still up for history export. It never
# fabricates a receipt, a history row or a latency value.
#
# Options:
#   --history PATH        use an existing history NDJSON instead of exporting
#   --host-evidence PATH  use an existing host-evidence JSON
#   --force               overwrite an existing analysis output
#   --require-valid       exit nonzero unless measurement and latency are assessed
set -euo pipefail
cd "$(dirname "$0")/.."

run_id=${1:?run_id required}
results=${2:?results directory required}
platform=${3:?platform required}
shift 3
history_override=""
host_override=""
force=false
require_valid=false
while (($#)); do
  case "$1" in
    --history) history_override=${2:?history path required}; shift 2 ;;
    --host-evidence) host_override=${2:?host evidence path required}; shift 2 ;;
    --force) force=true; shift ;;
    --require-valid) require_valid=true; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

case "$platform" in
  thingsboard|fiware|edgex) ;;
  *) echo "platform must be thingsboard, fiware or edgex" >&2; exit 2 ;;
esac
[[ "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9_-]{0,79}$ ]] || { echo "unsafe run id" >&2; exit 2; }

python=${PYTHON:-python3}
sut=${SUT:-87.107.191.107}
run_dir="$results/$run_id"
window="$results/$run_id.capture-window.json"
sink="$results/$run_id.sink.ndjson"
diag="$results/$run_id.generator-diagnostics.jsonl"
diag_status="$diag.status.json"
host_evidence="$results/$run_id.hosts.json"
out="$results/$run_id.dispatch-analysis.json"
status="$results/$run_id.analysis-status.json"

[[ -d "$run_dir" ]] || { echo "missing run directory: $run_dir" >&2; exit 2; }
for name in resolved.json k6-summary.json manifest.json records.ndjson; do
  [[ -s "$run_dir/$name" ]] || { echo "missing run artifact: $run_dir/$name" >&2; exit 2; }
done
[[ -s "$window" ]] || { echo "missing capture window: $window" >&2; exit 2; }
[[ -f "$sink" ]] || { echo "missing sink receipts: $sink" >&2; exit 2; }

if [[ -e "$out" || -e "$status" ]]; then
  if [[ "$force" != true ]]; then
    echo "analysis already exists (use --force to replace): $out" >&2
    exit 2
  fi
  rm -f "$out" "${out%.json}.md" "${out%.json}.csv" "$status"
fi

# History must be exported while the platform is still running. A history
# export failure is an evidence failure, not a claim that history is missing.
history=""
history_status="OK"
if [[ -n "$history_override" ]]; then
  [[ -f "$history_override" ]] || { echo "history file is missing: $history_override" >&2; exit 2; }
  if [[ ! "$history_override" -ef "$results/$run_id.history.ndjson" ]]; then
    cp "$history_override" "$results/$run_id.history.ndjson"
  fi
  history="$results/$run_id.history.ndjson"
elif [[ -s "$results/$run_id.history.ndjson" ]]; then
  history="$results/$run_id.history.ndjson" # resumable: a previously retrieved history export is reused
else
  [[ "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9-]*$ ]] || {
    echo "history-ids.sh accepts hyphen-only run ids; got '$run_id'" >&2; exit 2;
  }
  case "$platform" in
    thingsboard) remote_env="TB_PROJECT=${TB_PROJECT:-tbq20261105}" ;;
    fiware) remote_env="FWHC_CONTAINER=${FWHC_CONTAINER:-fiwhc-timescale-1}" ;;
    edgex) remote_env="" ;;
  esac
  echo "exporting $platform history for $run_id from $sut" >&2
  if ssh -o BatchMode=yes -o ConnectTimeout=10 "$sut" "env $remote_env bash -s '$platform' '$run_id'" \
      < scripts/history-ids.sh > "$results/$run_id.history.ndjson.tmp" 2> "$results/$run_id.history.log"; then
    mv "$results/$run_id.history.ndjson.tmp" "$results/$run_id.history.ndjson"
    history="$results/$run_id.history.ndjson"
  else
    rm -f "$results/$run_id.history.ndjson.tmp"
    history_status="EXPORT_FAILED"
    printf '{"schema_version":1,"run_id":"%s","status":"EXPORT_FAILED","log":"%s"}\n' \
      "$run_id" "$run_id.history.log" > "$results/$run_id.history-status.json"
    echo "history export failed; recorded EXPORT_FAILED (analysis continues for dispatch latency)" >&2
  fi
fi

# Same-load-host evidence. The sink is started by the local wrapper and k6 runs
# with host networking on the same host; record the actual host identity rather
# than claiming a cross-host clock was qualified.
if [[ -n "$host_override" ]]; then
  [[ -s "$host_override" ]] || { echo "host evidence is empty: $host_override" >&2; exit 2; }
  if [[ ! "$host_override" -ef "$host_evidence" ]]; then
    cp "$host_override" "$host_evidence"
  fi
elif [[ ! -s "$host_evidence" ]]; then
  # The analysis machine is not evidence of where a prior capture ran.
  # Preserve an explicit absence marker, never attest matching local boot IDs.
  printf '{"schema_version":1,"same_load_host_asserted":false,"evidence":"capture-time host provenance missing"}\n' > "$host_evidence"
fi

clock_samples="$diag"
if [[ ! -s "$clock_samples" ]]; then
  clock_samples="$results/$run_id.clock-samples-missing.json"
  printf '{"samples":[],"missing":true}\n' > "$clock_samples"
fi

args=(--run "$run_dir" --sink "$sink" --window "$window"
      --history-status "$history_status"
      --clock-samples "$clock_samples" --host-evidence "$host_evidence" --out "$out")
[[ "$history_status" == "OK" ]] && args+=(--history "$history")
[[ -s "$diag" ]] && args+=(--resource-evidence "$diag")
[[ -s "$diag_status" ]] && args+=(--sampler-status "$diag_status")
[[ -s "$run_dir/gen-cpu.ndjson" ]] && args+=(--gen-cpu "$run_dir/gen-cpu.ndjson")

set +e
"$python" scripts/analyze-dispatch-run.py "${args[@]}"
analysis_rc=$?
set -e

if [[ ! -s "$out" ]]; then
  printf '{"schema_version":1,"run_id":"%s","analysis_exit":%d,"produced":false}\n' \
    "$run_id" "$analysis_rc" > "$status"
  echo "analysis did not produce $out" >&2
  exit 4
fi

"$python" - "$out" "$status" "$analysis_rc" <<'PY'
import json, pathlib, sys
analysis = json.loads(pathlib.Path(sys.argv[1]).read_text())
status = {
    "schema_version": 1,
    "run_id": analysis.get("run_id"),
    "analysis_exit": int(sys.argv[3]),
    "produced": True,
    "measurement_valid": analysis.get("measurement_valid"),
    "capture_valid": analysis.get("capture_valid"),
    "latency_assessed": analysis.get("latency_assessed"),
    "platform_result": analysis.get("platform_result"),
    "generator_resource_qualified": analysis.get("generator_resource_qualified"),
    "history_status": analysis.get("history_status"),
    "history_assessed": analysis.get("history_assessed"),
}
pathlib.Path(sys.argv[2]).write_text(json.dumps(status, indent=2) + "\n")
PY

if [[ "$require_valid" == true ]]; then
  "$python" - "$status" <<'PY'
import json, pathlib, sys
s = json.loads(pathlib.Path(sys.argv[1]).read_text())
if not (s["measurement_valid"] and s["latency_assessed"]):
    raise SystemExit("measurement/latency not assessed: " + json.dumps(s))
PY
fi

echo "$out"
