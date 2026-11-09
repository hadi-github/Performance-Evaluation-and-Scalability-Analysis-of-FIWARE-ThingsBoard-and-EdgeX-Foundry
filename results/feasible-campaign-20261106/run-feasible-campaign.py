#!/usr/bin/env python3
"""Scoped remote stack handoff and run launcher for feasible ordinary batch.

The operator must obtain integrated Sol review before --execute-reviewed. `check`
is read-only. Stack transitions are explicit, serial, volume-preserving, and
never use down/rm/prune/pull/build. Runs are delegated to the reviewed remote
executor staged by the parent qualification workflow.
"""
import argparse
import fcntl
import hashlib
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
SCOPE = json.loads((HERE / "frozen-scope.json").read_text())
SUT = "ramonavm2"
LOAD = "ramonavm1"
TB = "/root/iot-qual-tb-20261105/deploy/thingsboard/compose.yaml"
FW = "/root/iot-bench-deploy-cand-auto20261027"
EDGEX = FW + "/edgex"
REMOTE_RESULTS = "/root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106"
REMOTE_SOURCE = REMOTE_RESULTS + "/source"
REMOTE_RUNS = REMOTE_RESULTS + "/runs"
REMOTE_LOCK = "/run/iot-bench-workload.lock"
EVIDENCE = HERE / "evidence"
COMPOSE = "docker compose"
SUT_TB_PROJECT = "tbq20261105"
SUT_FWHC_CONTAINER = "fiwhc-timescale-1"
PLATFORM_PREFIX = {"thingsboard": "tb", "fiware": "fw", "edgex": "ex"}


def qualification_run_ids(platform):
    ids = [f'{PLATFORM_PREFIX[platform]}-dispatch-endpoint-qual-20261106']
    if platform == 'fiware':
        ids.append('fw-dispatch-endpoint-qual-20261106-encodingfix1')
        ids.append('fw-dispatch-endpoint-qual-20261106-reviewedheadroom1')
    if platform in ('fiware','edgex'):
        ids.append(f'{PLATFORM_PREFIX[platform]}-dispatch-endpoint-qual-20261106-finalshort1')
    if platform == 'edgex':
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort2')
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort3')
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort4')
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort5')
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort6')
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort7')
        ids.append('ex-dispatch-endpoint-qual-20261106-finalshort8')
        ids.append('ex-dispatch-endpoint-qual-20261106-reviewedheadroom1')
    return ids


def frozen_plan_path(platform, run_id):
    prefix = PLATFORM_PREFIX[platform]
    # Owner-authorized supplemental attempts are kept outside the frozen B0
    # matrix and require a separately reviewed amendment plus unique run ID.
    amendment_path = HERE / 'supplemental-attempts-20261109.json'
    if amendment_path.is_file():
        amendment = json.loads(amendment_path.read_text())
        for attempt in amendment.get('attempts', []):
            if attempt.get('run_id') != run_id:
                continue
            if attempt.get('platform') != platform:
                raise SystemExit('supplemental run ID platform does not match amendment')
            plan = (HERE / attempt['plan']).resolve()
            if not plan.is_relative_to(HERE.resolve()) or not plan.is_file():
                raise SystemExit('supplemental plan path is invalid')
            if sha256_file(plan) != attempt.get('plan_sha256'):
                raise SystemExit('supplemental plan hash does not match amendment')
            baseline = frozen_plan_path(platform, attempt['baseline_run_id'])
            expected = json.loads(baseline.read_text())
            if json.loads(plan.read_text()) != expected:
                raise SystemExit('supplemental attempt must preserve the baseline workload plan exactly')
            return plan
    if platform in ('fiware','edgex') and run_id in qualification_run_ids(platform) and '-finalshort' in run_id:
        return HERE / f'{platform}-final-short-plan.json'
    if run_id in qualification_run_ids(platform):
        return HERE / ('qualification-plan.json' if platform == 'thingsboard'
                       else f'qualification-{platform}-plan.json')
    if re.fullmatch(rf'disp-{prefix}-r(0[1-5])-20261106', run_id or ''):
        return HERE / 'plans' / (run_id + '.json')
    raise SystemExit('run ID does not match a frozen ordinary or qualification cell')


def transport_options():
    """One explicit, private configuration shared by SSH and every SCP path."""
    config = os.environ.get('IOT_BENCH_SSH_CONFIG')
    if not config:
        return []
    path = Path(config).resolve()
    if not path.is_file():
        raise RuntimeError('IOT_BENCH_SSH_CONFIG is not a file')
    return ['-F', str(path), '-o', 'BatchMode=yes', '-o', 'StrictHostKeyChecking=yes',
            '-o', 'ConnectTimeout=10', '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=3']


def scp_command(*args):
    return ['scp', *transport_options(), *map(str, args)]


def ssh(host, command, timeout=180, check=True):
    p = subprocess.run(["ssh", *transport_options(), "-o", "BatchMode=yes", host, command], text=True,
                       capture_output=True, timeout=timeout)
    if check and p.returncode:
        raise RuntimeError(f"ssh {host} failed ({p.returncode}):\n{p.stdout}\n{p.stderr}\n$ {command}")
    return p


def remote(host, body, timeout=180):
    return ssh(host, "bash -lc " + shlex.quote("set -euo pipefail; " + body), timeout=timeout)


def compose(project, file, env, action, extra=""):
    assignments = " ".join(f"{k}={shlex.quote(str(v))}" for k, v in env.items())
    return f"cd {shlex.quote(str(Path(file).parent))}; {assignments} {COMPOSE} -p {project} -f {shlex.quote(file)} {extra} {action}".strip()


def owned_workload_check():
    # Never kill or stop anything on the load host. Presence of any k6 or a
    # leftover benchctl host process is a hard stop.
    running = ssh(LOAD, "docker ps -q --filter name=k6-").stdout.strip()
    if running:
        raise RuntimeError("load-host workload container exists; inspect before proceeding")
    competing = ssh(LOAD, "ps -eo stat=,comm= | awk '$1 !~ /^Z/ && ($2 == \"k6\" || $2 == \"benchctl\") {bad=1} END {exit bad}'", check=False)
    if competing.returncode != 0:
        raise RuntimeError("load-host k6/benchctl process exists; inspect before proceeding")
    sink = ssh(LOAD, "ss -Hlnpt 'sport = :18091'", check=False).stdout.strip()
    if sink:
        raise RuntimeError("dedicated capture port 18091 is occupied; do not stop its owner")
    # Port 18090 is deliberately not examined as a free port; it is an existing sink.


def assert_no_other_platform(platform):
    projects = {"thingsboard": {"tbq20261105"}, "fiware": {"fiware", "fiwhc"}, "edgex": {"edgex"}}
    forbidden = set().union(*[v for k, v in projects.items() if k != platform])
    for project in forbidden:
        active = ssh(SUT, "docker ps -q --filter " + shlex.quote("label=com.docker.compose.project=" + project)).stdout.strip()
        if active:
            raise RuntimeError(f"unrelated owned platform project {project} is running; run reviewed switch first")


def read_only_check(platform):
    owned_workload_check()
    if platform == "thingsboard":
        inv = ssh(SUT, "curl -fsS --max-time 5 http://127.0.0.1:18085/inventory").stdout
        rows = json.loads(inv)
        for svc, mem in (("tb-core", 1342177280), ("tb-rule-engine", 1342177280), ("tb-http-transport", 805306368)):
            rs = [r for r in rows if r.get("project") == "tbq20261105" and r.get("service") == svc]
            if len(rs) != 2 or any(r.get("cpuset") != "4-5" or r.get("memory_bytes") != mem or
                                   r.get("restarts") != 0 or r.get("oom_killed") for r in rs):
                raise RuntimeError(f"TB effective inventory failed for {svc}: {rs}")
        return "TB tbq20261105 N=2 resource inventory matches frozen cell"
    if platform == "fiware":
        baseline = FW + "/fiware-baseline/compose.yaml"
        history = FW + "/fiware-history-candidate/compose.yaml"
        env = {"FIWARE_REPLICAS": "2", "POOL_CPUSET": "4-5", "INFRA_CPUSET": "2-3"}
        fwcfg = json.loads(remote(SUT, compose("fiware", baseline, env, "config --format json")).stdout)
        hccfg = json.loads(remote(SUT, compose("fiwhc", history, env, "config --format json")).stdout)
        for name in ("iot-agent", "orion"):
            svc = fwcfg["services"][name]
            expected_n = 2
            if svc.get("deploy", {}).get("replicas") != expected_n or svc.get("cpuset") != "4-5" or int(svc["mem_limit"]) != 536870912:
                raise RuntimeError(f"FIWARE N=2 app resources mismatch: {name}")
        for name, image, memory in (("timescale", "timescale/timescaledb-postgis:2.3.0-pg13", 1610612736),
                                    ("quantumleap", "orchestracities/quantumleap:1.0.0", 536870912)):
            svc = hccfg["services"].get(name)
            if svc is None or svc.get("image") != image or int(svc["mem_limit"]) != memory or svc.get("cpuset") != "2-3":
                raise RuntimeError(f"FIWARE history tier mismatch: {name}")
        if fwcfg["services"]["mongo"].get("image") != "mongo:8.0":
            raise RuntimeError("FIWARE baseline image mismatch")
        return "FIWARE live rendered config confirms baseline N=2 plus pinned QL/Timescale 2 GiB history tier"
    if platform == "edgex":
        env = {"EDGEX_REPLICAS": "2", "POOL_CPUSET": "4-5", "INFRA_CPUSET": "2-3",
               "SINK_URL": "http://87.107.191.32:18091"}
        cfg = remote(SUT, compose("edgex", EDGEX + "/compose.yaml", env, "config --format json", "--profile export")).stdout
        obj = json.loads(cfg)
        required = {"database": 1610612736, "core-data": 536870912, "device-rest": 536870912,
                    "app-http-export": 536870912}
        # The pinned source includes build stanzas, but rendered image tags are already installed. No build/pull.
        # Expected budget is the effective fixed allocation, including all infra and the one exporter.
        total_bytes = 0
        expected_images = {"core-data": "edgex-nats/core-data:4.0.2",
                           "device-rest": "edgex-nats/device-rest:4.0.2",
                           "app-http-export": "edgex-nats/app-service-configurable:4.0.2"}
        for name, memory in required.items():
            svc = obj.get("services", {}).get(name)
            if svc is None:
                raise RuntimeError(f"EdgeX rendered config lacks {name}")
            if int(svc["mem_limit"]) != memory:
                raise RuntimeError(f"EdgeX memory mismatch for {name}: {svc.get('mem_limit')}")
        for name, image in expected_images.items():
            if obj["services"][name].get("image") != image:
                raise RuntimeError(f"EdgeX image mismatch for {name}: {obj['services'][name].get('image')}")
        for name in ("core-data", "device-rest"):
            svc = obj["services"][name]
            if svc.get("deploy", {}).get("replicas") != 2 or svc.get("cpuset") != "4-5":
                raise RuntimeError(f"EdgeX actual replica mapping or app pool mismatch: {name}")
        if (obj["services"]["app-http-export"].get("cpuset") != "4-5" or
            not obj["services"]["app-http-export"].get("environment", {}).get("WRITABLE_PIPELINE_FUNCTIONS_HTTPEXPORT_PARAMETERS_URL", "").endswith(":18091/edgex")):
            raise RuntimeError("EdgeX app export pool or sink route mismatch")
        for svc in obj["services"].values():
            if svc.get("profiles") and "export" not in svc["profiles"]:
                continue
            if svc.get("mem_limit") is None or svc.get("memswap_limit") is None or not svc.get("cpuset"):
                raise RuntimeError("EdgeX service lacks a hard memory/swap limit or CPU set")
            total_bytes += int(svc["mem_limit"]) * int(svc.get("deploy", {}).get("replicas", 1))
        if total_bytes != 5248 * 1024 * 1024:
            raise RuntimeError(f"EdgeX effective fixed allocation changed: {total_bytes} bytes, expected 5248 MiB")
        return "EdgeX rendered config confirms N=2 core-data + N=2 device-rest + one 512 MiB exporter; service total includes infra"
    raise ValueError(platform)


def stop_owned_stacks():
    # Explicit stop only; Compose project volumes and networks remain untouched.
    specs = [("tbq20261105", TB), ("fiware", FW + "/fiware-baseline/compose.yaml"),
             ("fiwhc", FW + "/fiware-history-candidate/compose.yaml"), ("edgex", EDGEX + "/compose.yaml")]
    for project, file in specs:
        remote(SUT, compose(project, file, {}, "stop", '--profile export' if project == 'edgex' else ''))


def check_effective_containers(expected, containers, image_ids):
    """Compare live Docker state with the reviewed rendered resource contract."""
    grouped = {}
    for c in containers:
        labels = c.get('Config', {}).get('Labels', {})
        key = labels.get('com.docker.compose.project'), labels.get('com.docker.compose.service')
        if key not in expected:
            raise RuntimeError(f'unexpected running platform service: {key}')
        grouped.setdefault(key, []).append(c)
    for key, svc in expected.items():
        rows = grouped.get(key, [])
        if len(rows) != int(svc.get('deploy', {}).get('replicas', 1)):
            raise RuntimeError(f'effective replica count mismatch: {key}')
        for c in rows:
            host, state = c.get('HostConfig', {}), c.get('State', {})
            healthcheck = svc.get('healthcheck') or {}
            if healthcheck and not healthcheck.get('disable') and healthcheck.get('test') != ['NONE'] and state.get('Health',{}).get('Status') != 'healthy':
                raise RuntimeError(f'missing/unhealthy configured healthcheck: {key}')
            if (state.get('Running') is not True or state.get('OOMKilled') is not False or
                    c.get('RestartCount') != 0 or state.get('Health', {}).get('Status', 'healthy') != 'healthy'):
                raise RuntimeError(f'effective lifecycle/health mismatch: {key}')
            if (host.get('CpusetCpus') != svc.get('cpuset') or
                    host.get('Memory') != int(svc.get('mem_limit', 0)) or
                    host.get('MemorySwap') != int(svc.get('memswap_limit', 0)) or
                    not host.get('Memory') or host.get('NanoCpus', 0) != 0 or
                    host.get('CpuQuota', 0) not in (0, -1)):
                raise RuntimeError(f'effective resource mismatch: {key}')
            image = svc.get('image')
            if c.get('Config', {}).get('Image') != image or c.get('Image') != image_ids.get(image):
                raise RuntimeError(f'effective image mismatch: {key}')
            runtime = c.get('RuntimeResources', {})
            if (runtime.get('cpuset') != svc.get('cpuset') or
                    runtime.get('memory_max') != str(svc['mem_limit']) or runtime.get('memory_swap_max') != '0'):
                raise RuntimeError(f'kernel cgroup resource mismatch: {key}')
            constraints = runtime.get('cpu_constraints')
            if not constraints:
                raise RuntimeError(f'missing ancestor resource evidence: {key}')
            cores = sum(int(p.split('-')[-1])-int(p.split('-')[0])+1 for p in svc['cpuset'].split(','))
            for constraint in constraints:
                quota = constraint.get('cpu_max')
                if quota is None and constraint.get('path') == '/sys/fs/cgroup': continue
                if not quota or len(quota) != 2 or int(quota[1]) <= 0 or (quota[0] != 'max' and int(quota[0])/int(quota[1]) < cores):
                    raise RuntimeError(f'restrictive/missing effective CPU quota: {key}')


def verify_effective_resources(platform):
    env = {'POOL_CPUSET':'4-5', 'INFRA_CPUSET':'2-3'}
    if platform == 'fiware':
        env['FIWARE_REPLICAS'] = '2'
        specs = [('fiware', FW+'/fiware-baseline/compose.yaml', '',
                  {'iot-agent','orion','mongo','haproxy'}),
                 ('fiwhc', FW+'/fiware-history-candidate/compose.yaml', '', {'quantumleap','timescale'})]
    elif platform == 'edgex':
        env.update(EDGEX_REPLICAS='2', SINK_URL='http://87.107.191.32:18091')
        override = EDGEX+'/compose.feasible-discovery-'+sha256_file(REPO/'deploy/edgex/compose.feasible-discovery.yaml')[:16]+'.yaml'
        specs = [('edgex', EDGEX+'/compose.yaml', '--profile export -f '+shlex.quote(override),
                  {'database','nats','haproxy','core-keeper','core-metadata','core-data','device-rest','app-http-export','core-common-config-bootstrapper'})]
    else:
        # TB's inventory gate already checks app allocation; inspect its actual
        # rendered project too, using the deployed qualification environment.
        env.update(TB_REPLICAS='2')
        specs = [('tbq20261105', TB, '', None)]
    expected, actual = {}, []
    for project, file, extra, required in specs:
        cfg = json.loads(remote(SUT, compose(project,file,env,'config --format json',extra)).stdout)
        ids = ssh(SUT,'docker ps -q --filter '+shlex.quote('label=com.docker.compose.project='+project)).stdout.split()
        rows = json.loads(ssh(SUT,'docker inspect '+' '.join(ids)).stdout) if ids else []
        for attempt in range(30):
            if not any(c.get('State',{}).get('Health',{}).get('Status')=='starting' for c in rows):
                break
            time.sleep(2)
            rows = json.loads(ssh(SUT,'docker inspect '+' '.join(ids)).stdout)
        actual.extend(rows)
        selected = required if required is not None else {c['Config']['Labels']['com.docker.compose.service'] for c in rows}
        for name in selected:
            expected[(project,name)] = cfg['services'][name]
    # Record only resource/provenance fields, not container environment secrets.
    actual = [{k:c[k] for k in ('Id','Name','Image','State','RestartCount','HostConfig','Config')} for c in actual]
    for c in actual:
        c['Config'] = {k:c['Config'][k] for k in ('Image','Labels')}
        c['HostConfig'] = {k:c['HostConfig'].get(k) for k in ('CpusetCpus','Memory','MemorySwap','NanoCpus','CpuQuota','CpuPeriod')}
    pids = [c['State']['Pid'] for c in actual]
    program = '''import json,pathlib
out={}
for pid in PIDS:
 status=pathlib.Path(f'/proc/{pid}/status').read_text().splitlines()
 cpus=next(x.split(':',1)[1].strip() for x in status if x.startswith('Cpus_allowed_list:'))
 rel=next(x.split('::',1)[1] for x in pathlib.Path(f'/proc/{pid}/cgroup').read_text().splitlines() if '::' in x)
 root=pathlib.Path('/sys/fs/cgroup'); cg=root/rel.lstrip('/')
 constraints=[]; parent=cg
 while True:
  f=parent/'cpu.max'
  constraints.append({'path':str(parent),'cpu_max':f.read_text().split() if f.exists() else None})
  if parent==root: break
  parent=parent.parent
 out[str(pid)]={'cpuset':cpus,'memory_max':(cg/'memory.max').read_text().strip(),
                'memory_swap_max':(cg/'memory.swap.max').read_text().strip(),'cpu_constraints':constraints}
print(json.dumps(out))
'''.replace('PIDS',repr(pids))
    runtime = json.loads(remote(SUT,"python3 - <<'PY'\n"+program+"\nPY").stdout)
    for c in actual:
        c['RuntimeResources'] = runtime[str(c['State']['Pid'])]
    tags = sorted({s['image'] for s in expected.values()})
    images = json.loads(ssh(SUT,'docker image inspect '+' '.join(shlex.quote(x) for x in tags)).stdout)
    image_ids = {tag:c['Id'] for tag,c in zip(tags,images)}
    check_effective_containers(expected,actual,image_ids)
    fields = ('image','cpuset','mem_limit','memswap_limit','deploy','healthcheck')
    return {'expected':{p+'/'+s:{k:cfg[k] for k in fields if k in cfg} for (p,s),cfg in expected.items()},
            'containers':actual,'image_ids':image_ids}


def check_resource_continuity(before, after):
    identity = lambda snapshot: {c['Name']:(c['Id'],c['State']['StartedAt']) for c in snapshot.get('containers',[])}
    if identity(before) != identity(after):
        raise RuntimeError('SUT container identity/start time changed during the run')


def stage_edgex_helper():
    source = REPO / 'deploy/edgex/scripts/provision-then-scale.sh'
    digest = sha256_file(source)
    target = EDGEX + '/scripts/provision-feasible-' + digest[:16] + '.sh'
    if ssh(SUT, 'test -e ' + shlex.quote(target), check=False).returncode:
        subprocess.run(scp_command(source, SUT + ':' + target), check=True, timeout=60)
    actual = ssh(SUT, 'sha256sum ' + shlex.quote(target)).stdout.split()[0]
    if actual != digest:
        raise RuntimeError('staged EdgeX helper hash mismatch; do not execute')
    return target


def stage_edgex_discovery_override():
    source = REPO / 'deploy/edgex/compose.feasible-discovery.yaml'
    digest = sha256_file(source)
    target = EDGEX + '/compose.feasible-discovery-' + digest[:16] + '.yaml'
    if ssh(SUT, 'test -e ' + shlex.quote(target), check=False).returncode:
        subprocess.run(scp_command(source, SUT + ':' + target), check=True, timeout=60)
    if ssh(SUT, 'sha256sum ' + shlex.quote(target)).stdout.split()[0] != digest:
        raise RuntimeError('EdgeX discovery override hash mismatch')
    return target


def activate(platform, run_id):
    assert_no_other_platform(platform)
    if platform == "thingsboard":
        # Existing project; only bring up its declared services. No compose down/up recreation.
        remote(SUT, f"cd /root/iot-qual-tb-20261105/deploy/thingsboard; docker compose -p tbq20261105 -f {TB} start")
    elif platform == "fiware":
        env = {"FIWARE_REPLICAS": "2", "POOL_CPUSET": "4-5", "INFRA_CPUSET": "2-3"}
        remote(SUT, compose("fiware", FW + "/fiware-baseline/compose.yaml", env, "up -d --no-build --pull never"))
        remote(SUT, compose("fiwhc", FW + "/fiware-history-candidate/compose.yaml", env,
                            "up -d --no-build --pull never timescale quantumleap"))
        # Existing history route is a distinct Orion subscription; never point this at Orion latest-state.
        ql_sub = r'''python3 - <<'PY'
import json, subprocess, sys
base='http://127.0.0.1:1026/v2/subscriptions'
h=['-H','fiware-service: openiot','-H','fiware-servicepath: /']
rows=json.loads(subprocess.check_output(['curl','-fsS',*h,base]))
if not any(x.get('description') == 'bench quantumleap' for x in rows):
 body={'description':'bench quantumleap','subject':{'entities':[{'idPattern':'.*','type':'Sensor'}],
       'condition':{'attrs':['seq']}},'notification':{'http':{'url':'http://87.107.191.107:8668/v2/notify'},
       'attrs':['seq','run_id','scheduled_send_ms'],'attrsFormat':'normalized'}}
 p=subprocess.run(['curl','-fsS','-o','/dev/null','-w','%{http_code}','-X','POST',*h,
   '-H','Content-Type: application/json','--data-binary',json.dumps(body),base],text=True,capture_output=True,check=True)
 if p.stdout not in ('201','204'): raise SystemExit('unexpected Orion subscription status '+p.stdout)
rows=json.loads(subprocess.check_output(['curl','-fsS',*h,base]))
matches=[x for x in rows if x.get('description') == 'bench quantumleap']
assert len(matches)==1 and matches[0]['notification']['http']['url']=='http://87.107.191.107:8668/v2/notify'
route='http://87.107.191.107:8668/v2/notify'
changed=False
for x in rows:
 if x.get('notification',{}).get('http',{}).get('url') != route: continue
 if x['id'] == matches[0]['id']: wanted='active'
 elif x.get('description') == 'bench QL': wanted='inactive'
 else: raise SystemExit('unrecognized duplicate history route; inspect before changing '+x['id'])
 if x.get('status') != wanted:
  changed=True
  subprocess.run(['curl','-fsS','-X','PATCH',*h,'-H','Content-Type: application/json',
    '--data-binary',json.dumps({'status':wanted}),base+'/'+x['id']],check=True)
rows=json.loads(subprocess.check_output(['curl','-fsS',*h,base]))
active=[x for x in rows if x.get('notification',{}).get('http',{}).get('url')==route and x.get('status')=='active']
assert len(active)==1 and active[0]['id']==matches[0]['id'], 'exactly one active history route required'
print(json.dumps(matches[0],sort_keys=True))
if changed: print('HISTORY_ROUTE_CHANGED')
PY'''
        history_setup = remote(SUT, ql_sub)
        if 'HISTORY_ROUTE_CHANGED' in history_setup.stdout:
            time.sleep(65)  # bounded Orion replica subscription-cache refresh
        # Configure the existing bounded sink subscription and wait its documented 65 s replica cache refresh.
        local_script = Path(__file__).resolve().parents[2] / "scripts/outbound.sh"
        subprocess.run(["env", "SUT=87.107.191.107", "SINK_URL=http://87.107.191.32:18091",
                        "FIWARE_SERVICE=openiot", str(local_script), "fiware"], check=True, timeout=120)
    elif platform == "edgex":
        helper = stage_edgex_helper()
        discovery_override = stage_edgex_discovery_override()
        # Required helper avoids core-metadata panic: provision each run's device cohort at N=1,
        # then scale to N=2 and validate every replica before workload. Runs occur serially.
        env = {"EDGEX_REPLICAS": "2", "POOL_CPUSET": "4-5", "INFRA_CPUSET": "2-3",
               "SINK_URL": "http://87.107.191.32:18091"}
        # Helper's plain `up` is wrapped so Compose cannot pull/build even if its source has build stanzas.
        cmd = (f"test -f {EDGEX}/.env; mkdir -p {REMOTE_RESULTS}/gates; "
                f"cd {EDGEX}; export EDGEX_REPLICAS=2 POOL_CPUSET=4-5 INFRA_CPUSET=2-3 "
                "EDGEX_INVENTORY_RESERVE=6000 "
                f"COMPOSE_FILE={shlex.quote(EDGEX+'/compose.yaml:'+discovery_override)} "
               "SINK_URL=http://87.107.191.32:18091; "
                "docker() { if [[ \"$1\" == compose ]]; then shift; local -a args=(); local arg; "
                "for arg in \"$@\"; do case \"$arg\" in down|rm|pull|build|--remove-orphans) return 2;; "
                "up) args+=(up --no-build --pull never);; *) args+=(\"$arg\");; esac; done; "
                "command docker compose \"${args[@]}\"; else command docker \"$@\"; fi; }; export -f docker; "
                f"bash {shlex.quote(helper)} prepare 2 1000 {shlex.quote(run_id)} "
               f"/root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106/gates/{run_id}.json")
        remote(SUT, cmd, timeout=2400)
    else:
        raise ValueError(platform)
    verify_live(platform)


def tb_backends_ready(text):
    for proxy in ('tb-core','tb-http-transport'):
        label=re.escape(proxy)
        backend=re.search(r'^haproxy_backend_status\{proxy="'+label+r'",state="UP"\} ([0-9.]+)$',text,re.M)
        servers=re.findall(r'^haproxy_server_status\{proxy="'+label+r'",server="[^"]+",state="UP"\} ([0-9.]+)$',text,re.M)
        if not backend or float(backend.group(1))!=1 or sum(float(v)==1 for v in servers)!=2:
            return False
    return True


def wait_tb_backends(timeout=300):
    deadline=time.monotonic()+timeout
    while time.monotonic()<deadline:
        p=ssh(SUT,'curl -fsS --max-time 5 http://127.0.0.1:8404/metrics',timeout=15,check=False)
        if p.returncode==0 and tb_backends_ready(p.stdout):
            return p.stdout
        time.sleep(2)
    raise RuntimeError('TB pretraffic readiness timed out: both N=2 HAProxy backends must be UP')


def verify_live(platform):
    if platform == "thingsboard":
        wait_tb_backends()
        return read_only_check(platform)
    if platform == "fiware":
        # Require both project services healthy and validate Orion/IoT Agent/QL endpoints and database.
        body = ("docker ps --format '{{.Names}} {{.Status}}' | grep -E '^(fiware-|fiwhc-)' ; "
                "curl -fsS --max-time 5 http://127.0.0.1:1026/version; "
                 "curl -fsS --retry 12 --retry-delay 2 --retry-all-errors --max-time 5 http://127.0.0.1:8668/health; "
                "python3 -c 'import json,subprocess; h=[\"-H\",\"fiware-service: openiot\",\"-H\",\"fiware-servicepath: /\"]; "
                "s=json.loads(subprocess.check_output([\"curl\",\"-fsS\",*h,\"http://127.0.0.1:1026/v2/subscriptions\"])); "
                 "assert any(x.get(\"description\")==\"bench sink\" and x.get(\"notification\",{}).get(\"http\",{}).get(\"url\")==\"http://87.107.191.32:18091/fiware\" for x in s); "
                 "assert len([x for x in s if x.get(\"status\")==\"active\" and x.get(\"notification\",{}).get(\"http\",{}).get(\"url\")==\"http://87.107.191.107:8668/v2/notify\"])==1' ")
        remote(SUT, body)
        return "FIWARE/QL service and history API endpoints healthy"
    if platform == "edgex":
        remote(SUT, "docker ps --format '{{.Names}} {{.Status}}' | grep '^edgex-'; "
               "curl -fsS --max-time 5 http://127.0.0.1:59986/api/v3/ping; "
               "curl -fsS --max-time 5 http://127.0.0.1:59881/api/v3/ping; "
               "docker exec edgex-nats-1 wget -qO- http://127.0.0.1:8222/varz")
        return "EdgeX NATS, core metadata and ingress healthy; provision helper gate required"
    raise ValueError(platform)


def finish(platform, run_id):
    """Called between runs only, after run capture and history export are retained."""
    owned_workload_check()
    if platform == "edgex":
        # Preserve the provision gate as a baseline for detecting core-metadata panic/restart.
        helper = stage_edgex_helper()
        remote(SUT, f"cd {EDGEX}; bash {shlex.quote(helper)} check {shlex.quote(run_id)} "
               f"/root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106/gates/{run_id}.json "
               f"/root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106/gates/{run_id}.check.json")
    files = {
        "thingsboard": [("tbq20261105", TB)],
        "fiware": [("fiware", FW + "/fiware-baseline/compose.yaml"),
                   ("fiwhc", FW + "/fiware-history-candidate/compose.yaml")],
        "edgex": [("edgex", EDGEX + "/compose.yaml")],
    }[platform]
    for project, file in files:
        remote(SUT, compose(project, file, {}, "stop", '--profile export' if project == 'edgex' else ''))


def sha256_file(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def build_source_sha256():
    """Bind prebuilt tools to this controller's exact Go source, not its label."""
    paths = [REPO / 'go.mod']
    if (REPO / 'go.sum').exists():
        paths.append(REPO / 'go.sum')
    for directory in ('cmd', 'internal'):
        paths.extend((REPO / directory).rglob('*.go'))
    h = hashlib.sha256()
    for path in sorted(paths):
        h.update(str(path.relative_to(REPO)).encode()+b'\0'+path.read_bytes()+b'\0')
    return h.hexdigest()


def validate_prebuilt_tools(source=None, names=('benchctl', 'sink')):
    """Pure local verification; call before locks, stack switches or traffic."""
    source = source or os.environ.get('IOT_BENCH_PREBUILT_DIR')
    if not source:
        return None
    source = Path(source).resolve()
    provenance = json.loads((source / 'build-provenance.json').read_text())
    if provenance.get('go_source_sha256') != build_source_sha256():
        raise RuntimeError('prebuilt tools do not match this controller Go source')
    for name in names:
        if sha256_file(source / name) != provenance['sha256'][name]:
            raise RuntimeError('prebuilt binary hash mismatch: ' + name)
    return source, provenance


STAGED_SCRIPTS = [
    "scripts/run-capture.sh",
    "scripts/run-protocol-capture.sh",
    "scripts/benchctl-calibrated.sh",
    "scripts/post-run-analysis.sh",
    "scripts/analyze-dispatch-run.py",
    "scripts/dispatch-integrity.py",
    "scripts/generator-diagnostic-sampler.py",
    "scripts/sink.sh",
    "scripts/history-ids.sh",
]


def ensure_evidence_dir(run_id):
    out = EVIDENCE / run_id
    if out.exists() and any(out.iterdir()):
        raise SystemExit(f"refusing existing evidence directory: {out}")
    out.mkdir(parents=True, exist_ok=True)
    return out


def stage_reviewed_source(run_id, plan_path, out_dir):
    """Build and stage exact reviewed bytes; verify every remote hash."""
    build = out_dir / "build"
    build.mkdir(parents=True)
    prebuilt = validate_prebuilt_tools()
    if prebuilt:
        source, provenance = prebuilt
        for name in ('benchctl', 'sink'):
            shutil.copy2(source / name, build / name)
        (build / 'build-provenance.json').write_text(json.dumps(provenance, indent=2)+'\n')
    else:
        for name in ("benchctl", "sink"):
            subprocess.run(["go", "build", "-o", str(build / name), "./cmd/" + name],
                           cwd=REPO, check=True, timeout=300)
    members = {}
    for rel in STAGED_SCRIPTS:
        members[rel] = REPO / rel
    for path in sorted((REPO / "workloads/k6").rglob("*")):
        if path.is_file():
            members[str(path.relative_to(REPO))] = path
    members["plan.json"] = plan_path
    members["bin/benchctl"] = build / "benchctl"
    members["bin/sink"] = build / "sink"
    archive = out_dir / "staged-source.tar.gz"
    manifest = []
    with tarfile.open(archive, "w:gz") as tar:
        for arcname in sorted(members):
            path = members[arcname]
            tar.add(path, arcname=arcname, recursive=False)
            manifest.append(f"{sha256_file(path)}  {arcname}")
    (out_dir / "staged-source.sha256").write_text("\n".join(manifest) + "\n")
    ssh(LOAD, f"set -eu; umask 077; mkdir -p {REMOTE_SOURCE}")
    subprocess.run(scp_command(archive, f"{LOAD}:{REMOTE_SOURCE}/staged-source.tar.gz"), check=True, timeout=300)
    subprocess.run(scp_command(out_dir / "staged-source.sha256", f"{LOAD}:{REMOTE_SOURCE}/staged-source.sha256"), check=True, timeout=120)
    remote(LOAD, f"cd {REMOTE_SOURCE}; tar -xzf staged-source.tar.gz; sha256sum -c staged-source.sha256; "
                 "chmod 700 bin/* scripts/*.sh scripts/*.py")
    return manifest


def validate_cell_plan(platform, run_id, plan_path, resolved):
    if run_id in qualification_run_ids(platform):
        expected = json.loads(frozen_plan_path(platform, run_id).read_text())
        plan = json.loads(Path(plan_path).read_text())
        if plan != expected:
            raise SystemExit('qualification plan differs from the frozen bounded check')
        for field in ('seed','payload','vus','target','request_timeout_ms','connection','arrivals','health','protocol_pilot_parameters'):
            want,got = expected.get(field),resolved.get(field)
            matches = all(isinstance(got,dict) and got.get(k)==v for k,v in want.items()) if isinstance(want,dict) else got==want
            if not matches:
                raise SystemExit('resolved qualification differs from frozen '+field)
        if (resolved.get('protocol_pilot_parameters') or {}).get('drain_cutoff_unix_ms') is not None or resolved.get('test_faults'):
            raise SystemExit('resolved qualification has absolute cutoff or injected faults')
        steps = [(s.get('phase'), s.get('rate'), s.get('duration_s'), s.get('vus'))
                 for s in resolved.get('steps', [])]
        expected_steps = [(s['phase'], s['rate'], s['duration_s'], s['vus'])
                          for s in expected['schedule']['steps']]
        if (resolved.get('adapter') != platform or resolved.get('devices') != 1000 or
                resolved.get('health', {}).get('workload_policy') != SCOPE['workload_policy'] or
                steps != expected_steps):
            raise SystemExit('resolved qualification differs from bounded 50/s scope')
        return 'resolved plan matches the bounded adapter-specific qualification'
    amendment_path = HERE / 'supplemental-attempts-20261109.json'
    supplemental = None
    if amendment_path.is_file():
        supplemental = next((a for a in json.loads(amendment_path.read_text()).get('attempts', [])
                             if a.get('run_id') == run_id), None)
    baseline_id = supplemental.get('baseline_run_id') if supplemental else run_id
    m = re.fullmatch(r"disp-(tb|fw|ex)-r(0[1-5])-20261106", baseline_id or "")
    if not m:
        raise SystemExit(f"run id does not match the frozen template: {run_id!r}")
    if m.group(1) != PLATFORM_PREFIX[platform]:
        raise SystemExit(f"run id prefix does not match platform {platform}")
    if supplemental and (supplemental.get('platform') != platform or
                         Path(plan_path).resolve() != frozen_plan_path(platform, run_id).resolve()):
        raise SystemExit('supplemental cell is not bound to its reviewed platform/plan')
    rep = int(m.group(2))
    cell = next(c for c in SCOPE["ordinary_cells"] if c["platform"] == platform)
    plan = json.loads(Path(plan_path).read_text())
    frozen_plan = json.loads((HERE / 'plans' / (run_id + '.json')).read_text())
    if plan != frozen_plan:
        raise SystemExit('plan differs from the generated frozen ordinary-cell plan')
    expected_steps = [(cell["rate_per_second"], cell["warmup_seconds"], 150),
                      (cell["rate_per_second"], cell["measurement_seconds"], 150)]
    got_steps = [(s.get("rate"), s.get("duration_s"), s.get("vus")) for s in resolved.get("steps", [])]
    checks = {
        "adapter": resolved.get("adapter") == platform,
        "plan_adapter": plan.get("adapter") == platform,
        "devices": resolved.get("devices") == cell["devices"] == plan.get("devices"),
        "policy": resolved.get("health", {}).get("workload_policy") == SCOPE["workload_policy"],
        "steps": got_steps == expected_steps,
        "vus": resolved.get("vus") == {"pre_allocated": 150, "max": 150} or
               (resolved.get("vus", {}).get("pre_allocated") == 150 and resolved.get("vus", {}).get("max") == 150),
        "seed": resolved.get("seed") == SCOPE["seed_by_repetition"][rep - 1],
        "payload": resolved.get("payload", {}).get("encoding_id") == cell["payload_encoding"] and
                   resolved.get("payload", {}).get("size_offset_bytes") == cell["payload_size_offset_bytes"],
        "no_faults": not resolved.get("test_faults", {}).get("duplicate_seqs") and not resolved.get("test_faults", {}).get("early_every"),
        "drain": (resolved.get("protocol_pilot_parameters") or {}).get("drain_seconds_after_measurement") ==
                 SCOPE["shared_configuration"]["maximum_receipt_drain_seconds"],
        "quiet_late": (resolved.get("protocol_pilot_parameters") or {}).get("quiet_interval_seconds") ==
                      SCOPE["shared_configuration"]["quiet_interval_seconds"] and
                      (resolved.get("protocol_pilot_parameters") or {}).get("late_observation_seconds") ==
                      SCOPE["shared_configuration"]["late_observation_seconds"],
    }
    failed = [k for k, ok in checks.items() if not ok]
    if failed:
        raise SystemExit("plan does not match the frozen ordinary cell: " + ", ".join(failed))
    return "resolved plan matches the frozen ordinary cell"


CLEANUP_PROGRAM = r'''
import json,os,pathlib,signal,subprocess,time
ROOT=ROOT_VALUE
RID=RID_VALUE
def owned_groups():
    result=set()
    for p in pathlib.Path('/proc').iterdir():
        if not p.name.isdigit(): continue
        try:
            args=(p/'cmdline').read_bytes().decode().strip('\0').split('\0')
            stat=(p/'stat').read_text().split(') ',1)[1].split()
            if stat[0]=='Z': continue
            # Exact run-ID argument plus this campaign's evidence root identify
            # owned benchctl/wrapper/sampler sessions, not the cleanup shell.
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
'''


def cleanup_run(run_id, out_dir, acquire_lock):
    """Stop only this run's processes/sink. Caller retains lock through retrieval."""
    program = CLEANUP_PROGRAM.replace("ROOT_VALUE", repr(REMOTE_RESULTS)).replace("RID_VALUE", repr(run_id))
    p = ssh(LOAD, "python3 - <<'PY'\n" + program + "\nPY", timeout=180, check=False)
    (out_dir / "cleanup-verification.json").write_text(json.dumps(
        {"exit": p.returncode, "stdout": p.stdout, "stderr": p.stderr}, indent=2) + "\n")
    if p.returncode == 0 and acquire_lock:
        return True
    if acquire_lock:
        (out_dir / "WORKLOAD-LOCK-RETAINED.txt").write_text(
            "Owned load termination not verified. Preserve the load-host workload lock pending explicit recovery.\n")
    return False


def launch(platform, run_id, plan_path, grafana_url, timeout, resume_setup=False):
    if resume_setup:
        raise SystemExit('pretraffic resume is disabled: use a separately reviewed fresh attempt')
    validate_prebuilt_tools()
    frozen_plan = frozen_plan_path(platform, run_id)
    plan_path = Path(plan_path).resolve()
    if not plan_path.is_file():
        raise SystemExit(f"plan not found: {plan_path}")
    try:
        if json.loads(plan_path.read_text()) != json.loads(frozen_plan.read_text()):
            raise SystemExit('plan differs from the frozen ordinary-cell plan')
    except FileNotFoundError:
        raise SystemExit(f'frozen plan missing: {frozen_plan}; run make-cell-plans.py')
    out_dir = ensure_evidence_dir(run_id)
    owned_workload_check()
    if ssh(LOAD, f"test ! -e {REMOTE_RUNS}/{run_id}.capture-window.json", check=False).returncode:
        raise SystemExit(f"remote evidence already exists for {run_id}; never overwrite a run")
    lock = ssh(LOAD, f"set -eu; test ! -e {REMOTE_LOCK}; mkdir {REMOTE_LOCK}", check=False)
    if lock.returncode:
        raise SystemExit("load-host workload lock is present; inspect pending recovery before launching")
    lock_owned = True
    launch_attempted = False
    try:
        manifest = stage_reviewed_source(run_id, plan_path, out_dir)
        (out_dir / "staged-source.json").write_text(json.dumps(manifest, indent=2) + "\n")
        remote(LOAD, f"cd {REMOTE_SOURCE}; bin/benchctl resolve -plan plan.json -run-id {shlex.quote(run_id)} > resolved.json")
        resolved = json.loads(ssh(LOAD, f"cat {REMOTE_SOURCE}/resolved.json").stdout)
        (out_dir / "resolved.json").write_text(json.dumps(resolved, indent=2) + "\n")
        print(validate_cell_plan(platform, run_id, plan_path, resolved))
        activate(platform, run_id)
        read_only_check(platform)
        effective = verify_effective_resources(platform)
        (out_dir/'effective-resources-before.json').write_text(json.dumps(effective,indent=2)+'\n')
        print(verify_live(platform))
        if platform == 'thingsboard':
            # The protocol-capture diagnostic sampler calls this helper during
            # the run. Prove its active-project CPU snapshot works before any
            # workload traffic can start.
            snapshot = ssh(SUT, "curl -fsS --max-time 8 http://127.0.0.1:18085/snapshot")
            if not snapshot.stdout.strip():
                raise RuntimeError('TB pretraffic CPU snapshot helper returned no data')
            (out_dir/'cpu-snapshot-pretraffic.tsv').write_text(snapshot.stdout)
            (out_dir/'tb-haproxy-before.prom').write_text(wait_tb_backends())
        owned_workload_check()
        grafana = f"GRAFANA_URL={shlex.quote(grafana_url)} " if grafana_url else ""
        pilot = resolved['protocol_pilot_parameters']
        sampler_tail = int(pilot['drain_seconds_after_measurement']) + int(pilot['late_observation_seconds']) + 5
        gate_env = f'EDGEX_PREPROVISIONED_RUN_ID={shlex.quote(run_id)} ' if platform == 'edgex' else ''
        command = (
            f"set -eu; umask 077; mkdir -p {REMOTE_RUNS}; cd {REMOTE_SOURCE}; "
            f"test ! -e {REMOTE_RUNS}/{run_id}.capture-window.json; "
            f"setsid env BENCHCTL={REMOTE_SOURCE}/scripts/benchctl-calibrated.sh "
            f"SINK_BIN={REMOTE_SOURCE}/bin/sink SINK_PORT=18091 GENERATOR_CPUSET=0-1 SINK_CPU=2 "
            f"PROM_RW=http://127.0.0.1:9090/api/v1/write DIAGNOSTIC_TAIL_SECONDS={sampler_tail} {gate_env}{grafana}SUT=87.107.191.107 "
            f"scripts/run-capture.sh {REMOTE_SOURCE}/plan.json {shlex.quote(run_id)} {REMOTE_RUNS} {platform} "
            f"> {REMOTE_RUNS}/{run_id}.launch.log 2>&1 & pid=$!; "
            f"echo $pid > {REMOTE_RUNS}/{run_id}.launch.pid; set +e; wait $pid; rc=$?; "
            f"echo $rc > {REMOTE_RUNS}/{run_id}.launch-exit-code; exit $rc"
        )
        launch_attempted = True
        p = ssh(LOAD, command, timeout=timeout, check=False)
        (out_dir / "launch-ssh.json").write_text(json.dumps(
            {"exit": p.returncode, "stdout": p.stdout, "stderr": p.stderr}, indent=2) + "\n")
    except subprocess.TimeoutExpired as e:
        (out_dir / "launch-timeout.txt").write_text(f"launcher timed out after {timeout}s: {e}\n")
    except Exception as e:
        (out_dir / 'launcher-error.json').write_text(json.dumps(
            {'error':str(e),'workload_launch_attempted':launch_attempted},indent=2)+'\n')
        raise
    finally:
        clean = not launch_attempted
        if launch_attempted:
            try:
                clean = cleanup_run(run_id, out_dir, lock_owned)
            except Exception as e:
                (out_dir / 'cleanup.error').write_text(str(e))
                clean = False
        try:
            fetch_errors = fetch_evidence(run_id, out_dir)
        except Exception as e:
            fetch_errors = [{'error': str(e)}]
            (out_dir / 'fetch.error').write_text(str(e))
        if platform == 'edgex':
            fetch_edgex_gate(run_id, out_dir)
        history_file = out_dir / f"{run_id}.history.ndjson"
        history_status_file = out_dir / f'{run_id}.history-status.json'
        history_status = json.loads(history_status_file.read_text()) if history_status_file.is_file() else {}
        history_ok = history_file.is_file() and history_status.get('status') == 'OK'
        reanalysis_ok = None
        if not history_ok and (out_dir / f"{run_id}.hosts.json").is_file():
            # Load host cannot reach the SUT; export from the workstation while
            # the platform is still up, then rerun the same offline analyzer.
            try:
                history_ok = export_history_from_workstation(platform, run_id, out_dir)
            except Exception as e:
                history_ok = False
                (out_dir / 'history-export.error').write_text(str(e))
            if history_ok:
                try:
                    reanalysis_ok = reanalyze_with_history(platform, run_id, out_dir)
                except Exception as e:
                    reanalysis_ok = False
                    (out_dir / 'reanalysis.error').write_text(str(e))
        analysis = out_dir / f"{run_id}.dispatch-analysis.json"
        sut_resources_ok = False
        if launch_attempted and clean:
            try:
                effective = verify_effective_resources(platform)
                check_resource_continuity(json.loads((out_dir/'effective-resources-before.json').read_text()),effective)
                (out_dir/'effective-resources-after.json').write_text(json.dumps(effective,indent=2)+'\n')
                sut_resources_ok = (out_dir/'effective-resources-before.json').is_file()
            except Exception as e:
                (out_dir/'effective-resources.error').write_text(str(e))
        result = {
            "schema_version": 1,
            "platform": platform,
            "run_id": run_id,
            "run_kind": ('short-verification' if '-finalshort' in run_id else
                         'qualification' if run_id in qualification_run_ids(platform) else 'ordinary'),
            "plan": str(plan_path),
            "plan_sha256": sha256_file(plan_path),
            "workload_policy": SCOPE["workload_policy"],
            "staged_source_manifest": str(out_dir / "staged-source.json"),
            "history_retrieved": history_ok,
            "reanalysis_with_history": reanalysis_ok,
            "analysis_retrieved": analysis.is_file(),
            "fetch_errors": fetch_errors,
            "cleanup_verified": (out_dir / "cleanup-verification.json").is_file() and
                                json.loads((out_dir / "cleanup-verification.json").read_text()).get("exit") == 0,
            "workload_lock_retained": lock_owned,
            "workload_launch_attempted": launch_attempted,
            "sut_resource_qualified": sut_resources_ok,
            "post_run_defect": None,
            "post_run_check_qualified": platform != 'edgex',
        }
        if analysis.is_file():
            a = json.loads(analysis.read_text())
            result.update({k: a.get(k) for k in ("measurement_valid", "capture_valid", "latency_assessed",
                                                 "platform_result", "generator_resource_qualified", "history_result")})
        else:
            result.update({"measurement_valid": None, "capture_valid": None,
                           "latency_assessed": None, "platform_result": "NOT_ASSESSED_NO_ANALYSIS",
                           "generator_resource_qualified": None})
        (out_dir / "run-result.json").write_text(json.dumps(result, indent=2) + "\n")
        if clean and history_ok and analysis.is_file() and reanalysis_ok is not False:
            try:
                finish(platform, run_id)
                if platform == 'edgex':
                    fetch_edgex_gate(run_id, out_dir)
                    check_file = out_dir / 'edgex-gate.check.json'
                    check = json.loads(check_file.read_text()) if check_file.is_file() else {}
                    result['post_run_check'] = check
                    result['post_run_defect'] = check.get('defect')
                    result['post_run_check_qualified'] = check.get('defect') is False
                result["finish"] = "stopped owned services; volumes preserved"
            except Exception as e:
                result["finish"] = f"failed: {e}"
                if platform == 'edgex':
                    fetch_edgex_gate(run_id, out_dir)
                    check_file = out_dir / 'edgex-gate.check.json'
                    check = json.loads(check_file.read_text()) if check_file.is_file() else {}
                    result['post_run_defect'] = check.get('defect')
                    result['post_run_check'] = check
                    if check.get('defect') is True:
                        result['platform_result'] = 'VALID_PLATFORM_FAILURE_POST_RUN_DEFECT'
        else:
            result["finish"] = "skipped: cleanup, history or analysis not verified"
        if clean and (not launch_attempted or result['finish'].startswith('stopped')):
            release = ssh(LOAD, f'rmdir {REMOTE_LOCK}', check=False)
            lock_owned = release.returncode != 0
        result['workload_lock_retained'] = lock_owned
        (out_dir / "run-result.json").write_text(json.dumps(result, indent=2) + "\n")
    return 0 if (not lock_owned and result.get('measurement_valid') is True and
                 result.get('capture_valid') is True and result.get('latency_assessed') is True and
                  result.get('platform_result') == 'VALID_PLATFORM_SUCCESS' and
                  result.get('post_run_check_qualified') is True and
                  result.get('sut_resource_qualified') is True and
                 result.get('generator_resource_qualified') is True) else 1


def fetch_evidence(run_id, out_dir):
    """Retrieve every artifact; record failures instead of dropping evidence."""
    errors = []
    for remote_path, name in ((f"{REMOTE_RUNS}/{run_id}", ".run-dir"),
                              (f"{REMOTE_RUNS}/{run_id}.*", ".files")):
        dest = out_dir / name
        dest.mkdir(exist_ok=False)
        try:
            p = subprocess.run(scp_command("-r", f"{LOAD}:{remote_path}", dest),
                               text=True, capture_output=True, timeout=600)
        except Exception as e:
            errors.append({'glob':remote_path,'error':str(e)})
            continue
        if p.returncode:
            errors.append({"glob": remote_path, "stderr": p.stderr})
    # Keep the run directory as a subdirectory: post-run analysis expects it.
    run_dir = out_dir / ".run-dir" / run_id
    if run_dir.is_dir():
        run_dir.replace(out_dir / run_id)
    files = out_dir / ".files"
    if files.is_dir():
        for item in files.iterdir():
            target = out_dir / item.name
            if target.exists():
                errors.append({"glob": f"{run_id}.*", "duplicate": item.name})
                continue
            item.replace(target)
        if not any(files.iterdir()):
            files.rmdir()
    for empty in (out_dir / ".run-dir",):
        if empty.is_dir() and not any(empty.iterdir()):
            empty.rmdir()
    return errors


def fetch_edgex_gate(run_id, out_dir):
    """Provision/check gates live on the SUT, not on the load host."""
    status = []
    for suffix in ('.json','.check.json','.json.retained-inventory.json',
                   '.json.metadata-discovery.json','.json.driver-discovery.json'):
        target = out_dir / ('edgex-gate'+suffix)
        try:
            p = subprocess.run(scp_command(f'{SUT}:{REMOTE_RESULTS}/gates/{run_id}{suffix}',target),
                               text=True,capture_output=True,timeout=60)
            status.append({'suffix':suffix,'exit':p.returncode,'stderr':p.stderr})
        except Exception as e:
            status.append({'suffix':suffix,'error':str(e)})
    attempts = out_dir / 'edgex-gate-statuses'
    attempts.mkdir(exist_ok=True)
    try:
        p = subprocess.run(scp_command(f'{SUT}:{REMOTE_RESULTS}/gates/{run_id}.json.*.http-statuses',attempts),
                           text=True,capture_output=True,timeout=60)
        status.append({'suffix':'readiness-statuses','exit':p.returncode,'stderr':p.stderr})
    except Exception as e:
        status.append({'suffix':'readiness-statuses','error':str(e)})
    (out_dir / 'edgex-gate-retrieval.json').write_text(json.dumps(status,indent=2)+'\n')


def export_history_from_workstation(platform, run_id, out_dir):
    """The load host has no SSH trust to the SUT; export history here, before finish."""
    env = {"thingsboard": f"TB_PROJECT={SUT_TB_PROJECT}",
           "fiware": f"FWHC_CONTAINER={SUT_FWHC_CONTAINER}",
           "edgex": ""}[platform]
    target = out_dir / f"{run_id}.history.ndjson"
    with (REPO / "scripts" / "history-ids.sh").open("rb") as src, target.open("wb") as dst, \
            (out_dir / f"{run_id}.history.log").open("wb") as log:
        p = subprocess.run(["ssh", *transport_options(), "-o", "BatchMode=yes", SUT,
                            f"env {env} bash -s '{platform}' '{run_id}'"],
                           stdin=src, stdout=dst, stderr=log, timeout=300)
    if p.returncode:
        (out_dir / f"{run_id}.history-status.json").write_text(json.dumps(
            {"schema_version": 1, "run_id": run_id, "status": "EXPORT_FAILED",
             "exit": p.returncode}, indent=2) + "\n")
        return False
    (out_dir / f"{run_id}.history-status.json").write_text(json.dumps(
        {"schema_version": 1, "run_id": run_id, "status": "OK", "exit": 0}, indent=2) + "\n")
    return True


def reanalyze_with_history(platform, run_id, out_dir):
    cmd = ["bash", "scripts/post-run-analysis.sh", run_id, str(out_dir), platform,
           "--history", str(out_dir / f"{run_id}.history.ndjson"),
           "--host-evidence", str(out_dir / f"{run_id}.hosts.json"), "--force"]
    p = subprocess.run(cmd, cwd=REPO, text=True, capture_output=True, timeout=600)
    (out_dir / "reanalysis-with-history.json").write_text(json.dumps(
        {"exit": p.returncode, "stdout": p.stdout, "stderr": p.stderr}, indent=2) + "\n")
    return p.returncode == 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("action", choices=("check", "switch", "activate", "verify", "finish", "launch"))
    ap.add_argument("--platform", choices=sorted(c["platform"] for c in SCOPE["ordinary_cells"]))
    ap.add_argument("--run-id")
    ap.add_argument("--plan", type=Path, help="frozen ordinary-cell plan for launch")
    ap.add_argument("--grafana-url", default="", help="optional Grafana annotation endpoint")
    ap.add_argument("--timeout", type=int, default=4000, help="remote launch timeout in seconds")
    ap.add_argument("--matrix", type=Path, default=HERE / "run-matrix.json")
    ap.add_argument("--execute-reviewed", action="store_true", help="required for any remote mutation or workload launch")
    ap.add_argument("--sol-review", help="review receipt ID recorded by integrated Sol; required with --execute-reviewed")
    args = ap.parse_args()
    if args.action in ("switch", "activate", "finish", "launch") and not (args.execute_reviewed and args.sol_review):
        raise SystemExit("mutating action refused: obtain integrated Sol review and pass --execute-reviewed --sol-review RECEIPT")
    if args.action in ('switch', 'activate', 'finish', 'launch'):
        validate_prebuilt_tools()
    lock_path = HERE / ".campaign.lock"
    with lock_path.open("w") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise SystemExit("another campaign operation holds the exclusive local lock")
        if args.action in ('switch', 'activate', 'finish'):
            if ssh(LOAD, f'test ! -e {REMOTE_LOCK}', check=False).returncode:
                raise SystemExit('load-host workload lock exists; preserve the active run or recovery evidence')
        if args.action == "check":
            if not args.platform:
                raise SystemExit("check requires --platform")
            print(read_only_check(args.platform))
        elif args.action == "switch":
            owned_workload_check()
            stop_owned_stacks()
            print("Stopped only four named owned compose projects; no volumes removed.")
        elif args.action == "activate":
            if not args.platform:
                raise SystemExit("activate requires --platform")
            owned_workload_check()
            activate(args.platform, args.run_id or "ordinary-dispatch")
            print(verify_live(args.platform))
        elif args.action == "verify":
            if not args.platform:
                raise SystemExit("verify requires --platform")
            print(verify_live(args.platform))
        elif args.action == "finish":
            if not args.platform or not args.run_id:
                raise SystemExit("finish requires --platform and --run-id")
            finish(args.platform, args.run_id)
            print(f"Stopped owned {args.platform} services after {args.run_id}; volumes preserved.")
        elif args.action == "launch":
            if not args.platform or not args.run_id or not args.plan:
                raise SystemExit("launch requires --platform, --run-id and --plan")
            rc = launch(args.platform, args.run_id, args.plan, args.grafana_url, args.timeout)
            print(f"Launch evidence captured under {EVIDENCE / args.run_id}")
            return rc
        else:
            raise SystemExit("unreachable action")
    return 0


if __name__ == "__main__":
    sys.exit(main())
