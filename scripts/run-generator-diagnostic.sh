#!/usr/bin/env bash
# Bounded protocol-bound run plus the diagnostic sampler; does not deploy or scale the SUT.
# Usage: scripts/run-generator-diagnostic.sh PLAN RUN_ID RESULTS_DIR
set -euo pipefail
if [[ "${GENERATOR_DIAGNOSTIC_TIMEOUT_WRAPPED:-0}" != 1 ]]; then
  export GENERATOR_DIAGNOSTIC_TIMEOUT_WRAPPED=1
  exec timeout --signal=TERM --kill-after=180s 3600s "$0" "$@"
fi
cd "$(dirname "$0")/.."
plan=${1:?plan JSON required}; run_id=${2:?run id required}; results=${3:?results directory required}
[[ "$run_id" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$ ]] || { echo 'run_id must be a safe artifact/container identifier' >&2; exit 2; }
python=${PYTHON:-python3}
benchctl=${BENCHCTL:-bin/benchctl}
sink_bin=${SINK_BIN:-bin/sink}
sampler_ready="$results/$run_id.sampler-ready.json"
resolved_preflight="$results/$run_id.preflight-resolved.json"
inventory_preflight="$results/$run_id.preflight-inventory.json"
haproxy_preflight="$results/$run_id.preflight-haproxy.prom"
mkdir -p "$results"
diag="$results/$run_id.generator-diagnostics.jsonl"
for path in "$diag" "$diag.status.json" "$diag.log" "$sampler_ready" "$resolved_preflight" "$inventory_preflight" "$haproxy_preflight" \
  "$results/$run_id.generator-diagnostics-exit-code" "$results/$run_id.wrapper-exit-code" \
  "$results/$run_id.diagnostic-wrapper.log" "$results/$run_id.sink.pid" "$results/$run_id.sink.pid.rule-added" \
  "$results/$run_id.cancel-status.json" "$results/$run_id.cancel-sink-stop.txt" \
  "$results/$run_id.generator-diagnostics-unavailable.txt" \
  "$results/$run_id.benchctl.pid" "$results/$run_id.cancel-cleanup-failure.txt" \
  "$results/$run_id.sink.ndjson" "$results/$run_id.sink.ndjson.log" "$results/$run_id.sink-start.txt" \
  "$results/$run_id.sink-stop.txt" "$results/$run_id.capture-window.json" "$results/$run_id.benchctl-exit-code" \
  "$results/$run_id.wrapper-run.log" "$results/$run_id"; do
  [[ ! -e "$path" ]] || { echo "refusing existing diagnostic evidence: $path" >&2; exit 2; }
done
lock="$results/$run_id.launch-lock"
mkdir "$lock" || { echo "another launch owns this diagnostic identity: $lock" >&2; exit 2; }
release_lock() { rmdir "$lock" 2>/dev/null || true; }
trap release_lock EXIT
for executable in "$benchctl" "$sink_bin" scripts/run-protocol-capture.sh scripts/generator-diagnostic-sampler.py; do
  [[ -x "$executable" ]] || { echo "required executable missing/not executable: $executable" >&2; exit 2; }
done
command -v timeout >/dev/null || { echo 'timeout utility is required' >&2; exit 2; }
command -v bash >/dev/null || { echo 'bash is required' >&2; exit 2; }
"$python" -c "compile(open('scripts/generator-diagnostic-sampler.py').read(), 'sampler', 'exec')"
bash -n scripts/run-protocol-capture.sh
bash -n scripts/run-generator-diagnostic.sh
"$benchctl" resolve -plan "$plan" > "$resolved_preflight"
inventory_url=${SUT_INVENTORY_URL:-http://87.107.191.107:18085/inventory}
curl -fsS --max-time 10 "$inventory_url" > "$inventory_preflight"
curl -fsS --max-time 10 "${HAPROXY_METRICS_URL:-http://87.107.191.107:8404/metrics}" > "$haproxy_preflight"
"$python" - "$inventory_preflight" "${SUT_PROJECT:-tbq20261105}" "${EXPECTED_SUT_CPUSET:-4-5}" <<'PY'
import json,sys
rows=json.load(open(sys.argv[1])); project,cpuset=sys.argv[2:]
expected={'tb-core':(2,1342177280,'thingsboard/tb-node:4.3.1.6'),
          'tb-rule-engine':(2,1342177280,'thingsboard/tb-node:4.3.1.6'),
          'tb-http-transport':(2,805306368,'thingsboard/tb-http-transport:4.3.1.6')}
for service,(count,memory,image) in expected.items():
    selected=[r for r in rows if r.get('project')==project and r.get('service')==service]
    assert len(selected)==count, (service,len(selected),count)
    assert all(r.get('cpuset')==cpuset and r.get('memory_bytes')==memory for r in selected), (service,selected)
    assert all(not r.get('oom_killed') and r.get('restarts')==0 for r in selected), (service,selected)
    assert all(r.get('image')==image and r.get('image_id') for r in selected), (service,selected)
    assert len({r['image_id'] for r in selected})==1, (service,selected)
    assert all(r.get('health') not in ('unhealthy','starting') for r in selected), (service,selected)
print('verified N=2 application replica count, cpuset, memory, restart/OOM and Docker health state')
PY
"$python" - "$haproxy_preflight" <<'PY'
import re,sys
text=open(sys.argv[1]).read()
for proxy in ('tb-core','tb-http-transport'):
    backend=re.search(r'^haproxy_backend_status\{proxy="'+re.escape(proxy)+r'",state="UP"\} ([0-9.]+)$',text,re.M)
    servers=re.findall(r'^haproxy_server_status\{proxy="'+re.escape(proxy)+r'",server="[^"]+",state="UP"\} ([0-9.]+)$',text,re.M)
    assert backend and backend.group(1)=='1', (proxy,backend)
    assert sum(value=='1' for value in servers)==2, (proxy,servers)
print('verified both N=2 HAProxy application backends UP')
PY
sampler_pid= wrapper_pid=
cancel() {
  trap - INT TERM
  # Signal both owned sessions immediately; don't spend watchdog grace on Docker first.
  [[ -z "$wrapper_pid" ]] || { kill -TERM -- "-$wrapper_pid" 2>/dev/null || kill -TERM "$wrapper_pid" 2>/dev/null || true; }
  [[ -z "$sampler_pid" ]] || kill -TERM "$sampler_pid" 2>/dev/null || true
  # benchctl owns a distinct process session; terminate its recorded group as well.
  if [[ -s "$results/$run_id.benchctl.pid" ]]; then
    bench_pid=$(cat "$results/$run_id.benchctl.pid")
    if [[ "$bench_pid" =~ ^[0-9]+$ ]]; then
      kill -TERM -- "-$bench_pid" 2>/dev/null || kill -TERM "$bench_pid" 2>/dev/null || true
      timeout --signal=TERM --kill-after=2s 20s tail --pid="$bench_pid" -f /dev/null 2>/dev/null || true
      # benchctl is intentionally in its own session; escalate that exact session too.
      kill -KILL -- "-$bench_pid" 2>/dev/null || kill -KILL "$bench_pid" 2>/dev/null || true
    fi
  fi
  timeout --signal=TERM --kill-after=2s 15s docker stop --time 10 "k6-$run_id" >/dev/null 2>&1 || true
  [[ -z "$wrapper_pid" ]] || timeout --signal=TERM --kill-after=2s 20s tail --pid="$wrapper_pid" -f /dev/null 2>/dev/null || true
  if timeout --signal=TERM --kill-after=2s 5s docker inspect "k6-$run_id" >/dev/null 2>&1; then
    timeout --signal=TERM --kill-after=2s 15s docker kill "k6-$run_id" >/dev/null 2>&1 || true
  fi
  # Repeat after benchctl termination to cover creation racing the first stop.
  timeout --signal=TERM --kill-after=2s 15s docker stop --time 2 "k6-$run_id" >/dev/null 2>&1 || true
  if timeout --signal=TERM --kill-after=2s 5s docker inspect "k6-$run_id" >/dev/null 2>&1; then
    timeout --signal=TERM --kill-after=2s 15s docker kill "k6-$run_id" >/dev/null 2>&1 || true
    printf '%s\n' "target container k6-$run_id remains after bounded cancellation" > "$results/$run_id.cancel-cleanup-failure.txt"
  fi
  [[ -z "$wrapper_pid" ]] || kill -KILL -- "-$wrapper_pid" 2>/dev/null || true
  if [[ -e "$results/$run_id.sink.pid" ]]; then
    timeout --signal=TERM --kill-after=2s 15s env SINK_PORT="${SINK_PORT:-18091}" SINK_BIN="$sink_bin" SINK_PID_FILE="$results/$run_id.sink.pid" \
      SINK_LOG_FILE="$results/$run_id.sink.ndjson.log" scripts/sink.sh stop >"$results/$run_id.cancel-sink-stop.txt" 2>&1 || true
  fi
  [[ -z "$wrapper_pid" ]] || timeout --signal=TERM --kill-after=2s 10s tail --pid="$wrapper_pid" -f /dev/null 2>/dev/null || true
  [[ -z "$wrapper_pid" ]] || wait "$wrapper_pid" 2>/dev/null || true
  [[ -z "$sampler_pid" ]] || { kill -KILL "$sampler_pid" 2>/dev/null || true; wait "$sampler_pid" 2>/dev/null || true; }
  printf '%s\n' 130 > "$results/$run_id.wrapper-exit-code"
  printf '%s\n' 130 > "$results/$run_id.generator-diagnostics-exit-code"
  if [[ ! -e "$diag.status.json" ]]; then
    printf '{"schema_version":1,"run_id":"%s","complete":false,"cancelled":true}\n' "$run_id" > "$results/$run_id.cancel-status.json"
  fi
  exit 130
}
trap cancel INT TERM
docker inspect "k6-$run_id" >/dev/null 2>&1 && { echo "refusing existing k6 container k6-$run_id" >&2; exit 2; }
"$python" scripts/generator-diagnostic-sampler.py "$run_id" "$diag" \
  --prometheus "${PROMETHEUS_URL:-http://127.0.0.1:9090}" \
  --wait-seconds "${DIAGNOSTIC_WAIT_SECONDS:-600}" \
  --tail-seconds "${DIAGNOSTIC_TAIL_SECONDS:-930}" --ready-file "$sampler_ready" >"$diag.log" 2>&1 &
sampler_pid=$!
setsid scripts/run-protocol-capture.sh "$plan" "$run_id" "$results" >"$results/$run_id.diagnostic-wrapper.log" 2>&1 &
wrapper_pid=$!
set +e
wait "$wrapper_pid"; wrapper_rc=$?
wrapper_pid=
wait "$sampler_pid"; sampler_rc=$?
sampler_pid=
set -e
printf '%s\n' "$wrapper_rc" > "$results/$run_id.wrapper-exit-code"
printf '%s\n' "$sampler_rc" > "$results/$run_id.generator-diagnostics-exit-code"
trap - INT TERM
[[ "$wrapper_rc" -eq 0 ]] || {
  echo "protocol capture failed: wrapper=$wrapper_rc sampler=$sampler_rc; preserve all artifacts" >&2
  exit 1
}
if [[ "$sampler_rc" -ne 0 ]]; then
  printf '%s\n' "optional sampler unavailable/incomplete; protocol outcome remains separately assessable" > "$results/$run_id.generator-diagnostics-unavailable.txt"
fi
if docker inspect "k6-$run_id" >/dev/null 2>&1; then
  echo "diagnostic workload container still exists after wrapper; run incomplete" >&2
  docker stop --time 10 "k6-$run_id" >/dev/null 2>&1 || true
  exit 1
fi
