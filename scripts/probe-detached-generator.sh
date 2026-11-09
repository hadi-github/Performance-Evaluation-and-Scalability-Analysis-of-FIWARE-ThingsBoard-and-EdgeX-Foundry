#!/usr/bin/env bash
# Diagnostic only: 625 requests to an owned loopback fixture, never an IoT platform.
set -euo pipefail
cd "$(dirname "$0")/.."
run_id=${1:?probe run ID required}
out=${2:?new absolute output directory required}
[[ "$run_id" =~ ^probe-detach-[A-Za-z0-9-]+$ ]] || { echo 'diagnostic ID required' >&2; exit 2; }
[[ "$out" = /* && ! -e "$out" && -n "${TMUX:-}" ]] || { echo 'new absolute output and tmux session required' >&2; exit 2; }
python3 - "$PWD" <<'PY'
import pathlib,importlib.util,sys
root=pathlib.Path(sys.argv[1])
spec=importlib.util.spec_from_file_location('probe_launcher',root/'results/feasible-campaign-20261106/run-feasible-campaign.py')
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
module.validate_prebuilt_tools(root/'bin', ('benchctl','fixture-target'))
print('Exact executed benchctl/fixture-target binaries and Go source verified')
PY
listeners=$(ss -Hlnpt 'sport = :18190')
containers=$(docker ps -q --filter name=k6-)
[[ -z "$listeners" ]] || { echo 'fixture port already occupied; do not stop its owner' >&2; exit 2; }
[[ -z "$containers" ]] || { echo 'competing k6 workload' >&2; exit 2; }
mkdir -m 700 "$out"
cp bin/build-provenance.json "$out/executed-build-provenance.json"
fixture_pid=''
cleanup() {
  if [[ -n "$fixture_pid" ]]; then
    kill -TERM "$fixture_pid" 2>/dev/null || true
    wait "$fixture_pid"
    fixture_pid=''
  fi
}
trap cleanup EXIT
python3 - "$out/plan.json" <<'PY'
import json,pathlib,sys
plan={'schema_version':1,'plan_id':'detached-fixture-probe-20261107',
 'purpose':'Diagnostic tmux/transport/generator-progress test; not qualification or an ordinary campaign attempt.',
 'pilot_only':True,'seed':202611071,'adapter':'fixture','devices':10,
 'target':{'base_url':'http://127.0.0.1:18190'},
 'payload':{'encoding_id':'logical-json-ascii-v1','size_offset_bytes':0},
 'request_timeout_ms':5000,'connection':{'keep_alive':True},'arrivals':{'model':'constant'},
 'vus':{'pre_allocated':150,'max':150},
 'schedule':{'type':'steps','steps':[{'phase':'warmup','rate':25,'duration_s':5,'vus':150},{'phase':'measurement','rate':25,'duration_s':20,'vus':150}]},
 'health':{'workload_policy':'dispatch-integrity-v1','min_achieved_ratio':.99,'send_lag_p99_ms':0,'early_tolerance_ms':2,'max_core_busy':0},
 'cleanup':False,'test_faults':{},'deviations':['Loopback engineering diagnostic only; no platform, sink latency, headroom or capacity conclusion.']}
pathlib.Path(sys.argv[1]).write_text(json.dumps(plan,indent=2)+'\n')
PY
bin/benchctl resolve -plan "$out/plan.json" -run-id "$run_id" > "$out/resolved-preflight.json"
taskset -c 2 bin/fixture-target -addr 127.0.0.1:18190 -out "$out/receiver.ndjson" > "$out/receiver.log" 2>&1 &
fixture_pid=$!
ready=false
for _ in $(seq 1 50); do
  if curl -fsS --max-time 1 http://127.0.0.1:18190/stats >/dev/null; then ready=true; break; fi
  kill -0 "$fixture_pid" || exit 2
  sleep .1
done
[[ "$ready" == true ]] || { echo 'fixture readiness failed' >&2; exit 2; }
bin/benchctl run -plan "$out/plan.json" -run-id "$run_id" -results "$out/runs" \
  -cpuset 0-1 -env GOGC=400 -prom-rw http://127.0.0.1:9090/api/v1/write
cleanup
trap - EXIT
python3 scripts/dispatch-integrity.py --run "$out/runs/$run_id" --out "$out/dispatch-integrity.json"
listeners=$(ss -Hlnpt 'sport = :18190')
containers=$(docker ps -q --filter name=k6-)
[[ -z "$listeners" ]] || { echo 'fixture listener remains' >&2; exit 2; }
[[ -z "$containers" ]] || { echo 'k6 container remains' >&2; exit 2; }
python3 - "$out" "$run_id" <<'PY'
import json,pathlib,sys,collections,datetime
out=pathlib.Path(sys.argv[1]);rid=sys.argv[2];run=out/'runs'/rid
request=[json.loads(x) for x in (run/'records.ndjson').read_text().splitlines() if x.strip()]
receiver=[json.loads(x) for x in (out/'receiver.ndjson').read_text().splitlines() if x.strip()]
expected={(rid,i) for i in range(625)}
for rows in (request,receiver):
 identities=collections.Counter((r['run_id'],r['seq']) for r in rows)
 assert len(rows)==len(identities)==625 and set(identities)==expected
assert all(r['outcome']=='ack_success' for r in request)
assert all(r['stored'] and r['status']==200 for r in receiver)
progress=json.loads((run/'generator-progress.json').read_text())
assert progress['state']=='generator_exited' and progress['k6_exit_code']==0
assert progress['unique_completed_ticks']==progress['ack_success']==625
assert progress['invalid_records']==progress['duplicate_response_records']==0
assert len((run/'generator-progress.log').read_text().splitlines())>=3
result={'verdict':'PASS_PERSISTENCE_GENERATOR_PROBE','reviewed_for_continuation':False,
 'run_id':rid,'request_receiver_identities':625,'ack_success':625,
 'diagnostic_only':True,'platform_traffic':False,'completed_utc':datetime.datetime.now(datetime.timezone.utc).isoformat()}
(out/'probe-result.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
PY
