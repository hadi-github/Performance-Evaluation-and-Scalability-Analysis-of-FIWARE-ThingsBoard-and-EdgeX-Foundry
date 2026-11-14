#!/usr/bin/env bash
# Run one protocol-bound pilot with an isolated sink and measurement-relative receipt cutoff.
# Usage: scripts/run-protocol-capture.sh PLAN RUN_ID RESULTS_DIR
set -euo pipefail
cd "$(dirname "$0")/.."
plan=${1:?plan JSON required}; run_id=${2:?run id required}; results=${3:?results directory required}
port=${SINK_PORT:-18091}
benchctl=${BENCHCTL:-bin/benchctl}
sink_bin=${SINK_BIN:-bin/sink}
# Deployment addresses come from .env; see scripts/iotbench-env.sh.
# shellcheck source=scripts/iotbench-env.sh
source "$(dirname "$0")/iotbench-env.sh"
cpu_snapshot_command=${CPU_SNAPSHOT_COMMAND:-curl -fsS http://${SUT_IP}:18085/snapshot}
python=${PYTHON:-python3}
mkdir -p "$results"
run_dir="$results/$run_id"
sink_pid_file="$results/$run_id.sink.pid"
sink_file="$results/$run_id.sink.ndjson"
bench_pid_file="$results/$run_id.benchctl.pid"
for path in "$run_dir" "$sink_file" "$sink_file.log" "$results/$run_id.capture-window.json" "$sink_pid_file" "$sink_pid_file.rule-added"; do
  [[ ! -e "$path" ]] || { echo "refusing existing evidence/ownership path: $path" >&2; exit 2; }
done
start_ms=$(date +%s%3N)
stop_rc=0
sink_owned=false
cleanup() {
  [[ "$sink_owned" == true ]] || return 0
  set +e
  timeout --signal=TERM --kill-after=2s 15s env SINK_PORT="$port" SINK_BIN="$sink_bin" SINK_PID_FILE="$sink_pid_file" SINK_LOG_FILE="$sink_file.log" scripts/sink.sh stop >"$results/$run_id.sink-stop.txt" 2>&1
  stop_rc=$?
  if [[ $stop_rc -eq 0 ]]; then sink_owned=false; fi
  set -e
}
cancel_run() {
  trap - INT TERM
  if [[ -n "${bench_pid:-}" ]]; then
    kill -TERM -- "-$bench_pid" 2>/dev/null || kill -TERM "$bench_pid" 2>/dev/null || true
    timeout --signal=TERM --kill-after=2s 15s docker stop --time 10 "k6-$run_id" >/dev/null 2>&1 || true
    timeout --signal=TERM --kill-after=2s 15s tail --pid="$bench_pid" -f /dev/null 2>/dev/null || true
    kill -KILL -- "-$bench_pid" 2>/dev/null || true
    timeout --signal=TERM --kill-after=2s 10s tail --pid="$bench_pid" -f /dev/null 2>/dev/null || true
    wait "$bench_pid" 2>/dev/null || true
    bench_pid=""
  fi
  rm -f "$bench_pid_file"
  cleanup
  exit 130
}
trap cleanup EXIT
trap cancel_run INT TERM
[[ ! -e "$sink_pid_file" && ! -e "$sink_pid_file.rule-added" ]] || { echo "refusing pre-existing sink ownership state: $sink_pid_file" >&2; exit 2; }
sink_owned=true
SINK_PORT="$port" SINK_BIN="$sink_bin" SINK_PID_FILE="$sink_pid_file" SINK_LOG_FILE="$sink_file.log" \
  scripts/sink.sh start "$sink_file" >"$results/$run_id.sink-start.txt" 2>&1

sink_is_owned_and_listening() {
  SINK_PORT="$port" SINK_BIN="$sink_bin" SINK_PID_FILE="$sink_pid_file" scripts/sink.sh verify >/dev/null 2>&1
}
sink_owned=true

# Record host and owned sink identity while the sink is still alive. Do not
# reconstruct this from whichever machine later runs offline analysis.
"$python" - "$sink_pid_file" "$results/$run_id.hosts.json" <<'PY'
import json,pathlib,platform,socket,sys
rows=pathlib.Path(sys.argv[1]).read_text().splitlines()
boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
obj={'schema_version':1,'generator_host_id':boot,'sink_host_id':boot,
     'same_load_host_asserted':True,'hostname':socket.gethostname(),
     'kernel':platform.release(),
     'sink_process':{'pid':int(rows[0]),'start_ticks':rows[1],
                     'output':rows[2] if len(rows)>2 else None},
     'evidence':'capture wrapper starts owned sink locally and benchctl k6 with host networking'}
with pathlib.Path(sys.argv[2]).open('x') as f:
    json.dump(obj,f,indent=2); f.write('\n')
PY

grafana_args=()
[[ -n "${GRAFANA_URL:-}" ]] && grafana_args=(-grafana-url "$GRAFANA_URL")
set +e
setsid "$benchctl" run -plan "$plan" -results "$results" -run-id "$run_id" -cpuset "${GENERATOR_CPUSET:-0-1}" \
  -cpu-snapshot-command "$cpu_snapshot_command" \
  -prom-rw "${PROM_RW:-http://127.0.0.1:9090/api/v1/write}" "${grafana_args[@]}" \
  >"$results/$run_id.wrapper-run.log" 2>&1 &
bench_pid=$!
printf '%s\n' "$bench_pid" > "$bench_pid_file"
wait "$bench_pid"
run_rc=$?
bench_pid=""
rm -f "$bench_pid_file"
set -e
printf '%s\n' "$run_rc" > "$results/$run_id.benchctl-exit-code"
[[ -d "$run_dir" ]] || { echo "benchctl did not create run directory" >&2; exit 1; }

# Derive the cutoff from k6's actual measurement scenario end, never wall-clock launch estimates.
cutoff_ms=$("$python" - "$run_dir/k6-summary.json" "$run_dir/manifest.json" "$run_dir/resolved.json" "$start_ms" <<'PY'
import json,sys,math
s=json.load(open(sys.argv[1])); m=json.load(open(sys.argv[2])); r=json.load(open(sys.argv[3]))
p=m.get('protocol_pilot_parameters') or {}
seconds=p.get('drain_seconds_after_measurement')
if isinstance(seconds,bool) or not isinstance(seconds,int) or seconds <= 0 or p.get('drain_cutoff_unix_ms') is not None:
    raise SystemExit('wrapper requires exactly a positive measurement-relative drain_seconds_after_measurement')
plan_steps={x['index']:x for x in r.get('steps',[])}
steps=[]
for x in s.get('steps',[]):
    pstep=plan_steps.get(x.get('index'))
    if pstep is None or pstep.get('name') != x.get('name'):
        raise SystemExit('k6 summary step identity contradicts resolved plan')
    if pstep.get('phase')=='measurement':
        epoch=x.get('scenario_start_ms')
        if type(epoch) not in (int,float) or not math.isfinite(epoch) or epoch<float(sys.argv[4]):
            raise SystemExit('measurement scenario did not start; no valid receipt cutoff can be derived')
        steps.append((x,pstep))
if not steps: raise SystemExit('k6 summary has no measurement phase')
end=max(float(x['scenario_start_ms'])+int(pstep['duration_s'])*1000 for x,pstep in steps)
print(repr(end+seconds*1000))
PY
)
now_ms=$(date +%s%3N)
deadline_ms=$("$python" -c 'import math,sys; print(math.ceil(float(sys.argv[1])))' "$cutoff_ms")
(( now_ms <= deadline_ms )) || { echo "cutoff already passed before post-run capture: now=$now_ms cutoff=$cutoff_ms" >&2; exit 1; }
# These durations are explicit run parameters; the quiet interval ends at cutoff, followed by late observation.
read -r quiet_seconds late_seconds < <("$python" - "$run_dir/manifest.json" <<'PY'
import json,sys
p=json.load(open(sys.argv[1])).get('protocol_pilot_parameters') or {}
q=p.get('quiet_interval_seconds'); late=p.get('late_observation_seconds')
if type(q) is not int or q <= 0 or type(late) is not int or late <= 0:
    raise SystemExit('relative cutoff requires explicit positive quiet_interval_seconds and late_observation_seconds')
print(q,late)
PY
)
while :; do
  now_ms=$(date +%s%3N)
  (( now_ms < deadline_ms )) || break
  sink_is_owned_and_listening || { echo "dedicated sink exited before receipt cutoff" >&2; exit 1; }
  curl -fsS "127.0.0.1:$port/metrics" >/dev/null
  sleep 1
done
late_deadline=$(( deadline_ms + late_seconds*1000 ))
while (( $(date +%s%3N) < late_deadline )); do
  sink_is_owned_and_listening || { echo "dedicated sink exited during late-observation window" >&2; exit 1; }
  curl -fsS "127.0.0.1:$port/metrics" >/dev/null
  sleep 1
done
sink_is_owned_and_listening || { echo "dedicated sink exited before late observation completed" >&2; exit 1; }
cleanup
trap - EXIT INT TERM
end_ms=$(date +%s%3N)
"$python" - "$run_id" "$start_ms" "$end_ms" "$cutoff_ms" "$quiet_seconds" "$late_seconds" "$stop_rc" "$run_rc" "$sink_file" "$sink_file.log" "$results/$run_id.capture-window.json" "$run_dir/resolved.json" <<'PY'
import json,re,sys
_,rid,start,end,cutoff,quiet_seconds,late_seconds,stop_rc,run_rc,sink_path,log_path,out,resolved_path=sys.argv
dispatch_policy=json.load(open(resolved_path)).get('health',{}).get('workload_policy') in ('dispatch-integrity-v1','dispatch-integrity-v2')
text=open(log_path).read()
names=('received','duplicates','unparsed','overflow','incomplete_body','body_read_error','oversized_body')
metrics={}
for name in names:
 m=re.search(r'\b'+name+r'=(\d+)\b',text)
 if m: metrics[name+'_total']=int(m.group(1))
rows=[]
try:
 for line in open(sink_path):
  if line.strip(): rows.append(json.loads(line))
except Exception:
 rows=[]
received=len(rows)
duplicates=sum(r.get('dup') is True for r in rows)
cutoff=float(cutoff); end=int(end); quiet_start=cutoff-int(quiet_seconds)*1000
times=[]
try: times=[float(r['receipt_complete_body_read_ms']) for r in rows]
except Exception: times=[]
quiet_pass=(len(times)==len(rows) and not any(quiet_start < t <= cutoff for t in times))
recording_ok=(len(metrics)==len(names) and int(stop_rc)==0 and metrics.get('received_total')==received and metrics.get('duplicates_total')==duplicates and end-cutoff>=int(late_seconds)*1000)
health_ok=all(metrics.get(k,0)==0 for k in ('unparsed_total','overflow_total','incomplete_body_total','body_read_error_total','oversized_body_total'))
overflow=metrics.get('overflow_total',0); oversized=metrics.get('oversized_body_total',0)
read_errors=metrics.get('body_read_error_total',0); incomplete=metrics.get('incomplete_body_total',0)
unparsed=metrics.get('unparsed_total',0)
parse_failures=unparsed-incomplete
# Prospective attribution: overflow is capture loss; oversized bodies exceed the
# receiver bound; truncated deliveries and malformed platform bodies are platform
# results (conditional latency, no fabricated receipt), not capture invalidity.
if dispatch_policy:
    capture_health_ok=(overflow==0 and oversized==0 and incomplete==read_errors+oversized and unparsed>=incomplete)
else:
    capture_health_ok=health_ok
complete=recording_ok and capture_health_ok and (True if dispatch_policy else int(run_rc)==0 and quiet_pass)
late_rows=sum(t>cutoff for t in times)
attribution={'overflow_total':'capture_failure' if overflow else 'none',
 'oversized_body_total':'measurement_unqualified' if oversized else 'none',
 'body_read_error_total':'platform_or_transport_truncation' if read_errors else 'none',
 'incomplete_body_total':'consistent' if incomplete==read_errors+oversized else 'inconsistent',
 'unparsed_total':'platform_malformed_body' if parse_failures>0 else ('inconsistent' if parse_failures<0 else 'none')}
obj={'schema_version':1,'run_id':rid,'capture_start_ms':int(start),'capture_end_ms':int(end),
 'observation_cutoff_ms':float(cutoff),'complete':complete,'quiet_interval':{'required_seconds':int(quiet_seconds),
 'start_ms':quiet_start,'end_ms':cutoff,'passed':quiet_pass},
  'late_observation':{'required_seconds':int(late_seconds),'start_ms':float(cutoff),'end_ms':int(end),'late_receipt_rows':late_rows},'sink_metrics':metrics,
  'generator_exit_code':int(run_rc),'capture_recording_complete':recording_ok,'sink_health_counters_zero':health_ok,
  'quiet_interval_passed':quiet_pass,'health_attribution':attribution,
  'platform_anomalies':{'malformed_bodies':max(0,parse_failures),'truncated_bodies':read_errors,'oversized_bodies':oversized} if dispatch_policy else {}}
with open(out,'x') as f: json.dump(obj,f,indent=2); f.write('\n')
if not complete: raise SystemExit('capture incomplete: final sink counters/records disagree, sink unhealthy, quiet interval failed, or late observation incomplete')
PY
exit "$run_rc"
