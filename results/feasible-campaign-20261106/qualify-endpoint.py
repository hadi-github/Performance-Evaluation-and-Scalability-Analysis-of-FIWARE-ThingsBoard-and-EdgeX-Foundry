#!/usr/bin/env python3
"""Stage exact reviewed bytes and execute the one bounded TB endpoint qualification."""
import hashlib
import json
import pathlib
import shlex
import signal
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
REPO = HERE.parents[1]
REMOTE = '/root/iot-qual-load-20261105/dispatch-endpoint-qual-20261106'
OLD = '/root/iot-qual-load-20261105/protocol-pilot-20261105T1340Z/source-n2-diagnostic'
RID = 'tb-dispatch-endpoint-qual-20261106'
OUT = HERE / 'endpoint-qualification'
lock_owned = False
launch_attempted = False
remote_created = False
cleanup_verified = True

def interrupted(sig, _):
    raise KeyboardInterrupt('signal ' + str(sig))

for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
    signal.signal(sig, interrupted)

def no_competing_load():
    ssh('ramonavm1', '''test -z "$(docker ps -q --filter name=k6-)" &&
ps -eo stat=,comm= | awk '$1 !~ /^Z/ && ($2 == "k6" || $2 == "benchctl") {bad=1} END {exit bad}' ''')

def ssh(host, command, timeout=60, check=True):
    p = subprocess.run(['ssh', '-o', 'BatchMode=yes', host, command], text=True, capture_output=True, timeout=timeout)
    if check and p.returncode:
        raise RuntimeError(f'{host}: {p.stderr}\n{p.stdout}')
    return p

def scp(path, host, dest):
    subprocess.run(['scp', str(path), host + ':' + dest], check=True, timeout=60)

if '--execute-reviewed' not in sys.argv:
    print('No load launched. Requires --execute-reviewed following integrated Sol approval.')
    sys.exit(0)
OUT.mkdir(exist_ok=False)
try:
    # Build once from the reviewed working tree; never reuse an obsolete executable.
    build = OUT / 'bin'
    build.mkdir()
    for name in ('benchctl', 'sink'):
        subprocess.run(['go', 'build', '-o', str(build / name), './cmd/' + name], cwd=REPO, check=True, timeout=120)
    ssh('ramonavm1', 'mkdir /run/iot-bench-workload.lock')
    lock_owned = True
    no_competing_load()
    assert not ssh('ramonavm1', "ss -Hlnpt 'sport = :18091'").stdout.strip(), 'dedicated sink port occupied'
    inventory = json.loads(ssh('ramonavm2', 'curl -fsS --max-time 5 http://127.0.0.1:18085/inventory').stdout)
    for svc, memory in [('tb-core', 1342177280), ('tb-rule-engine', 1342177280), ('tb-http-transport', 805306368)]:
        rows = [r for r in inventory if r['project'] == 'tbq20261105' and r['service'] == svc]
        assert len(rows) == 2 and all(r['cpuset'] == '4-5' and r['memory_bytes'] == memory and r['restarts'] == 0 and not r['oom_killed'] for r in rows), svc
    (OUT / 'inventory-before.json').write_text(json.dumps(inventory, indent=2))
    (OUT / 'frozen-contract.md').write_bytes((REPO / 'docs/methodology/dispatch-contract-20261106.md').read_bytes())
    (OUT / 'frozen-scope.json').write_bytes((HERE / 'frozen-scope.json').read_bytes())
    ssh('ramonavm2', 'umask 077; mkdir /root/dispatch-endpoint-qual-20261106')
    scp(REPO / 'scripts/history-ids.sh', 'ramonavm2', '/root/dispatch-endpoint-qual-20261106/history-ids.sh')
    history_digest = hashlib.sha256((REPO / 'scripts/history-ids.sh').read_bytes()).hexdigest()
    assert ssh('ramonavm2', 'sha256sum /root/dispatch-endpoint-qual-20261106/history-ids.sh').stdout.split()[0] == history_digest
    (OUT / 'history-source.sha256').write_text(history_digest + '\n')
    ssh('ramonavm1', f'set -eu; umask 077; mkdir {REMOTE}; cp -a {OLD} {REMOTE}/source; mkdir {REMOTE}/runs')
    remote_created = True
    files = [(build / 'benchctl', 'bin/benchctl'), (build / 'sink', 'bin/sink'),
             (HERE / 'benchctl-calibrated.sh', 'benchctl-calibrated.sh'),
             (HERE / 'qualification-plan.json', 'qualification-plan.json')]
    files += [(path, str(path.relative_to(REPO))) for path in sorted((REPO / 'workloads/k6').rglob('*')) if path.is_file()]
    for name in ['run-generator-diagnostic.sh', 'run-protocol-capture.sh', 'generator-diagnostic-sampler.py', 'sink.sh']:
        files.append((REPO / 'scripts' / name, 'scripts/' + name))
    hashes = []
    for path, dest in files:
        ssh('ramonavm1', 'mkdir -p ' + REMOTE + '/source/' + str(pathlib.PurePosixPath(dest).parent))
        scp(path, 'ramonavm1', REMOTE + '/source/' + dest)
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        assert ssh('ramonavm1', 'sha256sum ' + REMOTE + '/source/' + dest).stdout.split()[0] == digest
        hashes.append({'path': str(path.relative_to(REPO)), 'staged': dest, 'sha256': digest})
    (OUT / 'staged-source.json').write_text(json.dumps(hashes, indent=2))
    ssh('ramonavm1', f'set -eu; chmod 700 {REMOTE}/source/benchctl-calibrated.sh {REMOTE}/source/bin/* {REMOTE}/source/scripts/*.sh {REMOTE}/source/scripts/generator-diagnostic-sampler.py; '
        f'{REMOTE}/source/bin/benchctl resolve -plan {REMOTE}/source/qualification-plan.json -run-id {RID} > {REMOTE}/resolved-preflight.json')
    resolved = json.loads(ssh('ramonavm1', f'cat {REMOTE}/resolved-preflight.json').stdout)
    assert resolved['health']['workload_policy'] == 'dispatch-integrity-v1'
    assert resolved['health']['send_lag_p99_ms'] == 0
    assert [(s['rate'], s['duration_s'], s['vus']) for s in resolved['steps']] == [(50, 30, 150), (50, 120, 150)]
    (OUT / 'resolved-preflight.json').write_text(json.dumps(resolved, indent=2))
    # Host/clock attestation and SUT inventory are separate from the load clock sampler.
    clock_program = 'import time; print(time.get_clock_info("time")); print(time.get_clock_info("monotonic"))'
    (OUT / 'load-host.txt').write_text(ssh('ramonavm1', 'hostname; cat /proc/sys/kernel/random/boot_id; python3 -c ' + shlex.quote(clock_program)).stdout)
    no_competing_load()
    command = (f'cd {REMOTE}/source; setsid env BENCHCTL={REMOTE}/source/benchctl-calibrated.sh '
               f'SINK_BIN={REMOTE}/source/bin/sink GENERATOR_CPUSET=0-1 SINK_CPU=2 '
               f'scripts/run-generator-diagnostic.sh {REMOTE}/source/qualification-plan.json {RID} {REMOTE}/runs '
               f'> {REMOTE}/qualification.log 2>&1 & pid=$!; echo "$pid" > {REMOTE}/launcher.pid; '
               f'wait "$pid"; rc=$?; echo "$rc" > {REMOTE}/exit.txt; exit 0')
    launch_attempted = True
    ssh('ramonavm1', command, timeout=4000)
finally:
    host_evidence_path = OUT / (RID + '.hosts.json')
    try:
        program = '''import json,pathlib,platform,socket
pid_file=pathlib.Path(PIDFILE)
sink={}
if pid_file.exists():
    rows=pid_file.read_text().splitlines()
    if len(rows)>=2: sink={'pid':int(rows[0]),'start_ticks':rows[1],'output':rows[2] if len(rows)>2 else None}
boot=pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
print(json.dumps({'schema_version':1,'generator_host_id':boot,'sink_host_id':boot,
 'same_load_host_asserted':True,'hostname':socket.gethostname(),'kernel':platform.release(),
 'sink_process':sink,'evidence':'qualification sink and k6 share this load host'},indent=2))
'''.replace('PIDFILE', repr(REMOTE + '/runs/' + RID + '.sink.pid'))
        evidence = ssh('ramonavm1', 'python3 - <<\'PY\'\n' + program + '\nPY', check=False)
        if evidence.returncode == 0 and evidence.stdout.strip():
            host_evidence_path.write_text(evidence.stdout)
        else:
            (OUT / 'host-evidence.error').write_text(
                json.dumps({'exit': evidence.returncode, 'stdout': evidence.stdout,
                            'stderr': evidence.stderr}, indent=2))
    except Exception as e:
        (OUT / 'host-evidence.error').write_text(str(e))
    if launch_attempted:
        cleanup_verified = False
        try:
            cleanup_program = '''import os,pathlib,signal,subprocess,time,json
ROOT=ROOT_VALUE
RID=RID_VALUE
def owned_groups():
 result=set()
 for p in pathlib.Path('/proc').iterdir():
  if not p.name.isdigit(): continue
  try:
   args=(p/'cmdline').read_bytes().decode().strip('\\0').split('\\0')
   stat=(p/'stat').read_text().split(') ',1)[1].split()
   if stat[0]=='Z': continue
   # Exact run-ID argument and an exclusive evidence-root argument identify
   # owned benchctl/wrapper/sampler sessions, not the cleanup shell itself.
   if RID in args and any(a.startswith(ROOT+'/') for a in args):
    group=os.getpgid(int(p.name))
    if group!=os.getpgrp(): result.add(group)
  except (OSError,ValueError,UnicodeError): pass
 return result
def send(groups,sig):
 for g in groups:
  try: os.killpg(g,sig)
  except ProcessLookupError: pass
groups=owned_groups()
send(groups,signal.SIGTERM)
deadline=time.monotonic()+30
while owned_groups() and time.monotonic()<deadline: time.sleep(.25)
send(owned_groups(),signal.SIGKILL)
time.sleep(1)
name='k6-'+RID
containers=subprocess.check_output(['docker','ps','-q','--filter','name=^/'+name+'$'],text=True).strip()
if containers: subprocess.run(['docker','stop','--time','10',name],check=True,timeout=20)
pidfile=pathlib.Path(ROOT+'/runs/'+RID+'.sink.pid')
if pidfile.exists():
 env=os.environ.copy(); env.update(SINK_PORT='18091',SINK_BIN=ROOT+'/source/bin/sink',SINK_PID_FILE=str(pidfile),SINK_LOG_FILE=ROOT+'/runs/'+RID+'.sink.ndjson.log')
 subprocess.run(['scripts/sink.sh','stop'],cwd=ROOT+'/source',env=env,check=True,timeout=30)
remaining=subprocess.check_output(['docker','ps','-q','--filter','name=^/'+name+'$'],text=True).strip()
listeners=subprocess.check_output(['ss','-Hlnpt','sport = :18091'],text=True).strip()
groups=owned_groups()
print(json.dumps(dict(remaining_groups=sorted(groups),remaining_container=remaining,sink_listener=listeners)))
if groups or remaining or listeners: raise SystemExit(1)
'''.replace('ROOT_VALUE', repr(REMOTE)).replace('RID_VALUE', repr(RID))
            cleanup = ssh('ramonavm1', 'python3 - <<\'PY\'\n' + cleanup_program + '\nPY', timeout=120, check=False)
            cleanup_verified = cleanup.returncode == 0
            (OUT / 'cleanup-verification.json').write_text(json.dumps({'exit': cleanup.returncode, 'stdout': cleanup.stdout, 'stderr': cleanup.stderr}, indent=2))
        except Exception as e:
            (OUT / 'cleanup.error').write_text(str(e))
    for name, command in [('inventory-after.json', 'curl -fsS --max-time 5 http://127.0.0.1:18085/inventory'),
                          ('history.ndjson', 'TB_PROJECT=tbq20261105 bash /root/dispatch-endpoint-qual-20261106/history-ids.sh thingsboard ' + RID)]:
        try:
            p = ssh('ramonavm2', command, check=False)
            (OUT / name).write_text(p.stdout)
            (OUT / (name + '.status.json')).write_text(json.dumps({'exit': p.returncode, 'stderr': p.stderr}, indent=2))
        except Exception as e:
            (OUT / (name + '.error')).write_text(str(e))
    # Keep every result, including generator-invalid outcomes. Never overwrite old runs.
    for name in ['runs', 'qualification.log', 'exit.txt', 'resolved-preflight.json']:
        try:
            p = subprocess.run(['scp', '-r', 'ramonavm1:' + REMOTE + '/' + name, str(OUT)], text=True, capture_output=True, timeout=180)
            if p.returncode:
                (OUT / ('retrieval-' + name.replace('/', '-') + '.error')).write_text(p.stderr)
        except Exception as e:
            (OUT / ('retrieval-' + name.replace('/', '-') + '.error')).write_text(str(e))
    # Automatic offline primary analysis over the retrieved artifacts. History is
    # exported from this workstation (the load host has no SSH trust to the SUT)
    # and host evidence is captured above, so the same analyzer is authoritative.
    analysis_status = OUT / 'analysis-status.json'
    try:
        if (OUT / 'runs' / RID).is_dir() and (OUT / 'runs' / (RID + '.sink.ndjson')).is_file():
            # Keep a copy alongside the run artifacts for re-analysis.
            if host_evidence_path.is_file() and (OUT / 'runs').is_dir():
                (OUT / 'runs' / (RID + '.hosts.json')).write_text(host_evidence_path.read_text())
            cmd = ['bash', 'scripts/post-run-analysis.sh', RID, str(OUT / 'runs'), 'thingsboard',
                   '--history', str(OUT / 'history.ndjson'),
                   '--host-evidence', str(host_evidence_path)]
            p = subprocess.run(cmd, cwd=str(REPO), text=True, capture_output=True, timeout=900)
            analysis_status.write_text(json.dumps({'exit': p.returncode, 'stdout': p.stdout, 'stderr': p.stderr}, indent=2))
        else:
            analysis_status.write_text(json.dumps({'exit': None, 'skipped': 'capture artifacts not retrieved'}, indent=2))
    except Exception as e:
        analysis_status.write_text(json.dumps({'exit': None, 'error': str(e)}, indent=2))
    if lock_owned and cleanup_verified:
        ssh('ramonavm1', 'rmdir /run/iot-bench-workload.lock', check=False)
    elif lock_owned:
        (OUT / 'WORKLOAD-LOCK-RETAINED.txt').write_text('Owned load termination not verified. Preserve /run/iot-bench-workload.lock pending explicit recovery.\n')
if not cleanup_verified:
    raise SystemExit('Qualification cleanup uncertain; workload lock retained and no further launch allowed')
print('Qualification evidence captured:', OUT)
print('Full workload, clock/resource, receipt/history and endpoint assessment remains required before GO.')
