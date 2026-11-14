"""Offline tests for the campaign launcher's frozen-cell validation and retrieval."""
import importlib.util
import copy
import json
import os
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location(
    "run_feasible_campaign", ROOT / "results" / "feasible-campaign-20261106" / "run-feasible-campaign.py")
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


def clean_env(**overrides):
    """The environment with every IOTBENCH_* variable removed.

    A developer machine may have `.env` exported; a test must assert the
    documented defaults, not whatever host is currently configured.
    """
    env = {k: v for k, v in os.environ.items() if not k.startswith('IOTBENCH_')}
    env.update(overrides)
    return env


def resolved_for(platform="thingsboard", run_id="disp-tb-r01-20261106", **overrides):
    rep = int(run_id.split("-r")[1][:2])
    obj = {
        "adapter": platform,
        "devices": 1000,
        "seed": mod.SCOPE["seed_by_repetition"][rep - 1],
        "health": {"workload_policy": "dispatch-integrity-v1"},
        "steps": [
            {"phase": "warmup", "rate": 25, "duration_s": 300, "vus": 150},
            {"phase": "measurement", "rate": 25, "duration_s": 600, "vus": 150},
        ],
        "vus": {"pre_allocated": 150, "max": 150},
        "payload": {"encoding_id": "logical-json-ascii-v1", "size_offset_bytes": 0},
        "test_faults": {},
        "protocol_pilot_parameters": {"drain_seconds_after_measurement": 900,
                                      "quiet_interval_seconds": 30,
                                      "late_observation_seconds": 30},
    }
    obj.update(overrides)
    return obj


class ValidateCellPlanTests(unittest.TestCase):
    def test_qualification_plan_is_distinct_from_ordinary_plan(self):
        rid = 'fw-dispatch-endpoint-qual-20261106'
        path = mod.frozen_plan_path('fiware', rid)
        plan = json.loads(path.read_text())
        resolved = dict(plan, steps=plan['schedule']['steps'])
        self.assertIn('qualification', mod.validate_cell_plan('fiware', rid, path, resolved))
        resolved['steps'][1]['rate'] = 100
        with self.assertRaises(SystemExit):
            mod.validate_cell_plan('fiware', rid, path, resolved)

    def test_qualification_platform_must_match_run_id(self):
        with self.assertRaises(SystemExit):
            mod.frozen_plan_path('edgex', 'fw-dispatch-endpoint-qual-20261106')

    def test_qualification_rejects_changed_windows_payload_and_vus(self):
        rid='fw-dispatch-endpoint-qual-20261106-reviewedheadroom1'
        path=mod.frozen_plan_path('fiware',rid)
        plan=json.loads(path.read_text())
        baseline=dict(plan,steps=plan['schedule']['steps'])
        for field,key,value in [('protocol_pilot_parameters','late_observation_seconds',5),
                                ('protocol_pilot_parameters','drain_seconds_after_measurement',60),
                                ('payload','size_offset_bytes',1024),('vus','max',300)]:
            bad=copy.deepcopy(baseline);bad[field][key]=value
            with self.assertRaises(SystemExit):mod.validate_cell_plan('fiware',rid,path,bad)

    def test_frozen_cell_passes(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "plan.json"
            path.write_text((mod.HERE / 'plans/disp-tb-r01-20261106.json').read_text())
            self.assertIn("matches", mod.validate_cell_plan("thingsboard", "disp-tb-r01-20261106", path,
                                                            resolved_for()))

    def test_wrong_rate_or_devices_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "plan.json"
            path.write_text((mod.HERE / 'plans/disp-tb-r01-20261106.json').read_text())
            bad = resolved_for(steps=[{"phase": "warmup", "rate": 50, "duration_s": 300, "vus": 150},
                                      {"phase": "measurement", "rate": 50, "duration_s": 600, "vus": 150}])
            with self.assertRaises(SystemExit) as e:
                mod.validate_cell_plan("thingsboard", "disp-tb-r01-20261106", path, bad)
            self.assertIn("steps", str(e.exception))

    def test_seed_must_match_repetition(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "plan.json"
            path.write_text((mod.HERE / 'plans/disp-tb-r03-20261106.json').read_text())
            with self.assertRaises(SystemExit) as e:
                mod.validate_cell_plan("thingsboard", "disp-tb-r03-20261106", path,
                                       resolved_for(seed=mod.SCOPE["seed_by_repetition"][0]))
            self.assertIn("seed", str(e.exception))

    def test_platform_prefix_must_match(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "plan.json"
            path.write_text(json.dumps({"adapter": "thingsboard", "devices": 1000}))
            with self.assertRaises(SystemExit) as e:
                mod.validate_cell_plan("fiware", "disp-tb-r01-20261106", path, resolved_for())
            self.assertIn("prefix", str(e.exception))

    def test_non_frozen_run_id_rejected(self):
        with tempfile.TemporaryDirectory() as d:
            path = pathlib.Path(d) / "plan.json"
            path.write_text(json.dumps({"adapter": "thingsboard", "devices": 1000}))
            with self.assertRaises(SystemExit) as e:
                mod.validate_cell_plan("thingsboard", "tb-r06-20261106", path, resolved_for())
            self.assertIn("template", str(e.exception))

    def test_owner_amended_supplemental_attempt_has_unique_id_and_identical_plan(self):
        rid = 'disp-tb-r04a-20261109'
        amendment = json.loads((mod.HERE / 'supplemental-attempts-20261109.json').read_text())
        row = next(x for x in amendment['attempts'] if x['run_id'] == rid)
        plan_path = mod.frozen_plan_path('thingsboard', rid)
        baseline = mod.frozen_plan_path('thingsboard', row['baseline_run_id'])
        matrix = json.loads((mod.HERE / 'run-matrix.json').read_text())['ordinary_runs']
        self.assertNotIn(rid, [x['run_id'] for x in matrix])
        self.assertEqual(plan_path.read_bytes(), baseline.read_bytes())
        self.assertIn('matches', mod.validate_cell_plan('thingsboard', rid, plan_path,
                                                         resolved_for(run_id=rid)))

    def test_supplemental_attempt_cannot_change_platform_or_plan(self):
        rid = 'disp-tb-r04a-20261109'
        with self.assertRaises(SystemExit):
            mod.frozen_plan_path('fiware', rid)
        with tempfile.TemporaryDirectory() as d:
            altered = pathlib.Path(d) / 'plan.json'
            plan = json.loads(mod.frozen_plan_path('thingsboard', rid).read_text())
            plan['schedule']['steps'][1]['rate'] = 50
            altered.write_text(json.dumps(plan))
            with self.assertRaises(SystemExit):
                mod.validate_cell_plan('thingsboard', rid, altered, resolved_for(run_id=rid))


class HostRelocationTests(unittest.TestCase):
    """A moved testbed runs the same experiment; a moved port does not.

    The frozen plans name the SUT inside `target`. Moving the testbed to a new
    address must not change what is measured, and it must not be possible to
    smuggle a workload change in through the address rewrite.
    """

    PLAN = ROOT / 'workloads/campaigns/fixed-points-w1-w4/plans/w4dp-fw-r01-20261108.json'

    def test_same_host_needs_no_overlay(self):
        plan = json.loads(self.PLAN.read_text())
        self.assertFalse(mod.PLAN_OVERLAY.needs_overlay(plan, '87.107.191.107'))

    def test_different_host_needs_an_overlay(self):
        plan = json.loads(self.PLAN.read_text())
        self.assertTrue(mod.PLAN_OVERLAY.needs_overlay(plan, '10.20.30.40'))

    def test_overlay_keeps_ports_and_paths(self):
        plan = json.loads(self.PLAN.read_text())
        moved = mod.PLAN_OVERLAY.rewrite_target(plan, '10.20.30.40')
        self.assertEqual(moved['target'], {
            'base_url': 'http://10.20.30.40:1026',
            'north_url': 'http://10.20.30.40:4041',
            'south_url': 'http://10.20.30.40:7896'})

    def test_relocated_plan_is_accepted(self):
        """A target-only overlay is the same experiment, so validation passes."""
        plan = json.loads(self.PLAN.read_text())
        moved = mod.PLAN_OVERLAY.rewrite_target(plan, '10.20.30.40')
        self.assertTrue(mod.plan_matches_frozen(moved, plan, '10.20.30.40'))

    def test_moved_port_is_rejected(self):
        """Only the host is deployment; the port is part of the qualified shape."""
        plan = json.loads(self.PLAN.read_text())
        moved = mod.PLAN_OVERLAY.rewrite_target(plan, '10.20.30.40')
        moved['target']['base_url'] = 'http://10.20.30.40:9999'
        self.assertFalse(mod.plan_matches_frozen(moved, plan, '10.20.30.40'))

    def test_workload_change_is_rejected(self):
        plan = json.loads(self.PLAN.read_text())
        moved = mod.PLAN_OVERLAY.rewrite_target(plan, '10.20.30.40')
        moved['devices'] = 2000
        self.assertFalse(mod.plan_matches_frozen(moved, plan, '10.20.30.40'))

    def test_overlay_refuses_to_write_a_workload_change(self):
        plan = json.loads(self.PLAN.read_text())
        with tempfile.TemporaryDirectory() as d:
            frozen = pathlib.Path(d) / 'frozen.json'
            frozen.write_text(json.dumps(plan))
            # Force a rewrite that would touch something outside target.
            original = mod.PLAN_OVERLAY.rewrite_target
            mod.PLAN_OVERLAY.rewrite_target = lambda p, h: dict(original(p, h), devices=9999)
            try:
                with self.assertRaises(SystemExit):
                    mod.PLAN_OVERLAY.overlay_plan(frozen, pathlib.Path(d) / 'out.json', '10.0.0.1')
            finally:
                mod.PLAN_OVERLAY.rewrite_target = original
            self.assertFalse((pathlib.Path(d) / 'out.json').exists())

    def test_overlay_record_names_both_hashes_and_hosts(self):
        with tempfile.TemporaryDirectory() as d:
            record = mod.PLAN_OVERLAY.overlay_plan(self.PLAN, pathlib.Path(d) / 'out.json',
                                                   '10.20.30.40')
            self.assertEqual(record['from_hosts'], ['87.107.191.107'])
            self.assertEqual(record['to_host'], '10.20.30.40')
            self.assertEqual(record['workload_differences'], [])
            self.assertNotEqual(record['frozen_sha256'], record['overlay_sha256'])
            self.assertTrue(all(p.startswith('target') for p in record['changed_paths']))


class ShellLoaderTest(unittest.TestCase):
    """`scripts/iotbench-env.sh` must never override what the caller decided.

    The load host has no `.env`, and the launcher starts the capture with an
    explicit `SUT_IP=`/`SINK_PORT=` for this run. A loader that assigned
    unconditionally would revert those to the qualified defaults and send
    traffic at the wrong machine — a failure that looks like a platform fault.
    """

    SCRIPT = ROOT / 'scripts' / 'iotbench-env.sh'

    def run_loader(self, exports='', env_file='/nonexistent'):
        # `export` with no arguments dumps the whole environment, so it is only
        # emitted when there is something to export.
        prefix = f'export {exports}; ' if exports else ''
        prog = (f'cd "{ROOT}"; {prefix}'
                'source scripts/iotbench-env.sh; '
                'printf "%s|%s|%s|%s" "$SUT_IP" "$SINK_PORT" "$SINK_URL_BASE" "$LOAD_IP"')
        return subprocess.run(['bash', '-c', prog], capture_output=True, text=True,
                              env=clean_env(IOTBENCH_ENV_FILE=env_file))

    def test_caller_values_survive(self):
        p = self.run_loader('SUT_IP=10.99.0.7 SINK_PORT=18090')
        self.assertEqual(p.returncode, 0, p.stderr)
        sut_ip, sink_port, sink_url, _ = p.stdout.split('|')
        self.assertEqual(sut_ip, '10.99.0.7')
        self.assertEqual(sink_port, '18090')
        self.assertEqual(sink_url, 'http://87.107.191.32:18090')

    def test_env_file_fills_the_gaps(self):
        with tempfile.TemporaryDirectory() as d:
            env = pathlib.Path(d) / '.env'
            env.write_text('IOTBENCH_SUT_IP=10.5.5.5\nIOTBENCH_LOAD_IP=10.6.6.6\n')
            p = self.run_loader('SUT_IP=10.99.0.7', env_file=str(env))
            self.assertEqual(p.returncode, 0, p.stderr)
            sut_ip, _, _, load_ip = p.stdout.split('|')
            self.assertEqual(sut_ip, '10.99.0.7')   # caller wins over .env
            self.assertEqual(load_ip, '10.6.6.6')   # .env fills the unset one

    def test_unset_everything_gives_the_qualified_defaults(self):
        p = self.run_loader()
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(p.stdout, '87.107.191.107|18091|http://87.107.191.32:18091|87.107.191.32')


    def test_values_with_spaces_and_quotes_are_not_corrupted(self):
        """A shell-quoted value must not keep its quote characters.

        `: "${K:=v}"` kept them, so SSH_KEY arrived as `'~/.ssh/key'` with the
        quotes included and every expanded path was wrong.
        """
        with tempfile.TemporaryDirectory() as d:
            env = pathlib.Path(d) / '.env'
            env.write_text("IOTBENCH_SSH_KEY=~/.ssh/some key\n"
                           "IOTBENCH_JUMP=user@host:2222\n")
            prog = (f'cd "{ROOT}"; source scripts/iotbench-env.sh; '
                    'printf "%s|%s" "$SSH_KEY" "$JUMP"')
            p = subprocess.run(['bash', '-c', prog], capture_output=True, text=True,
                               env=clean_env(IOTBENCH_ENV_FILE=str(env)))
            self.assertEqual(p.returncode, 0, p.stderr)
            self.assertEqual(p.stdout, '~/.ssh/some key|user@host:2222')


class StagedSourceTest(unittest.TestCase):
    """Everything a staged script runs must itself be staged.

    `stage_reviewed_source` copies a fixed list to the load host and runs it under
    `set -e`. A staged script that sources a file missing from that list dies on
    the first run, after the stack has already been switched — the worst possible
    moment. This test walks the staged scripts' own references instead of trusting
    a list to stay complete.
    """

    STAGED = mod.STAGED_SCRIPTS

    def test_staged_scripts_exist(self):
        for rel in self.STAGED:
            self.assertTrue((ROOT / rel).is_file(), f'staged file is missing: {rel}')

    def test_no_staged_script_sources_an_unstaged_file(self):
        import re
        problems = []
        for rel in self.STAGED:
            path = ROOT / rel
            if path.suffix not in ('.sh', '.bash'):
                continue
            for line in path.read_text().splitlines():
                if 'source ' not in line or line.lstrip().startswith('#'):
                    continue
                # Only literal paths can be checked; a variable is resolved at
                # runtime and is the caller's business.
                for m in re.finditer(r'source\s+"([^"$]+)"|source\s+([^\s"$;]+)', line):
                    target = m.group(1) or m.group(2)
                    if not target or '$' in target:
                        continue
                    candidates = [(path.parent / target), (path.parent.parent / target),
                                  (ROOT / target), (ROOT / 'scripts' / target)]
                    resolved = next((c for c in candidates if c.is_file()), None)
                    if resolved is None:
                        continue  # deliberately optional (guarded by `|| true`)
                    if not (resolved.resolve().is_relative_to(ROOT)):
                        continue
                    rel_target = str(resolved.resolve().relative_to(ROOT))
                    if rel_target not in self.STAGED:
                        problems.append(f'{rel} sources {rel_target}, which is not staged')
        self.assertEqual(problems, [])

    def test_python_staged_scripts_import_nothing_unstaged(self):
        """A staged Python file may only import modules that are staged or stdlib."""
        import ast
        allowed = {pathlib.Path(r).stem for r in self.STAGED}
        local = {p.stem for p in (ROOT / 'scripts').glob('*.py')}
        problems = []
        for rel in self.STAGED:
            path = ROOT / rel
            if path.suffix != '.py':
                continue
            tree = ast.parse(path.read_text())
            for node in ast.walk(tree):
                names = []
                if isinstance(node, ast.Import):
                    names = [a.name.split('.')[0] for a in node.names]
                elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module:
                    names = [node.module.split('.')[0]]
                for name in names:
                    if name in local and name not in allowed:
                        problems.append(f'{rel} imports {name}, which is not staged')
        self.assertEqual(problems, [])


class EvidenceTests(unittest.TestCase):
    def test_tb_backend_gate_requires_both_exact_n2_backends(self):
        lines=[]
        for proxy in ('tb-core','tb-http-transport'):
            lines.append(f'haproxy_backend_status{{proxy="{proxy}",state="UP"}} 1')
            for n in (1,2):lines.append(f'haproxy_server_status{{proxy="{proxy}",server="s{n}",state="UP"}} 1')
        text='\n'.join(lines)
        self.assertTrue(mod.tb_backends_ready(text))
        self.assertFalse(mod.tb_backends_ready(''))
        self.assertFalse(mod.tb_backends_ready(text.replace('server="s2",state="UP"} 1','server="s2",state="UP"} 0')))
        with mock.patch.object(mod,'ssh') as ssh:
            with self.assertRaises(RuntimeError):mod.wait_tb_backends(timeout=0)
            ssh.assert_not_called()
    def effective_fixture(self):
        svc={'image':'example:pinned','cpuset':'4-5','mem_limit':512,'memswap_limit':512,'deploy':{'replicas':1}}
        c={'Id':'container','Name':'app','Image':'sha256:image','RestartCount':0,
           'Config':{'Image':'example:pinned','Labels':{'com.docker.compose.project':'p','com.docker.compose.service':'app'}},
           'HostConfig':{'CpusetCpus':'4-5','Memory':512,'MemorySwap':512,'NanoCpus':0,'CpuQuota':0},
           'State':{'Running':True,'OOMKilled':False,'StartedAt':'start'},
           'RuntimeResources':{'cpuset':'4-5','memory_max':'512','memory_swap_max':'0',
                               'cpu_constraints':[{'path':'/sys/fs/cgroup/app','cpu_max':['max','100000']}]}}
        return {('p','app'):svc},c,{'example:pinned':'sha256:image'}

    def test_effective_resource_and_image_mismatches_fail(self):
        expected,c,images=self.effective_fixture()
        mod.check_effective_containers(expected,[c],images)
        for field,value in [('CpusetCpus','0-7'),('Memory',1024),('MemorySwap',-1),('CpuQuota',50000)]:
            bad=copy.deepcopy(c);bad['HostConfig'][field]=value
            with self.assertRaises(RuntimeError):mod.check_effective_containers(expected,[bad],images)
        bad=copy.deepcopy(c);bad['RuntimeResources']['cpu_constraints'][0]['cpu_max']=['100000','100000']
        with self.assertRaises(RuntimeError):mod.check_effective_containers(expected,[bad],images)
        bad=copy.deepcopy(c);bad['Image']='sha256:wrong'
        with self.assertRaises(RuntimeError):mod.check_effective_containers(expected,[bad],images)
        with self.assertRaises(RuntimeError):mod.check_effective_containers(expected,[],images)

    def test_container_replacement_is_not_resource_continuity(self):
        _,c,_=self.effective_fixture();other=copy.deepcopy(c);other['State']['StartedAt']='new-start'
        with self.assertRaises(RuntimeError):mod.check_resource_continuity({'containers':[c]},{'containers':[other]})

    def test_historical_restart_is_admitted_only_with_a_baseline(self):
        """RestartCount is lifetime-cumulative, so a persisted container carrying a
        restart from an earlier ordinal is healthy. It is admitted only when a
        baseline records that count, and a restart DURING the run still fails."""
        expected,c,images=self.effective_fixture()
        c['RestartCount']=1
        # no baseline -> absolute zero still required, so the check is not weakened
        with self.assertRaises(RuntimeError):
            mod.check_effective_containers(expected,[c],images)
        # baseline records the pre-existing count -> admitted
        mod.check_effective_containers(expected,[c],images,{('p','app'):1})
        # a restart during the run (count above baseline) still fails
        c['RestartCount']=2
        with self.assertRaises(RuntimeError):
            mod.check_effective_containers(expected,[c],images,{('p','app'):1})

    def test_restart_baseline_uses_max_count_per_service(self):
        """Replicated services collapse to one baseline entry: the highest count."""
        expected,c,images=self.effective_fixture()
        svc=expected[('p','app')]
        second=copy.deepcopy(c);second['Name']='app2';second['Id']='container2'
        second['RestartCount']=2
        c['RestartCount']=1
        baseline={}
        for row in [c,second]:
            labels=row.get('Config',{}).get('Labels',{})
            key=(labels.get('com.docker.compose.project'),labels.get('com.docker.compose.service'))
            baseline[key]=max(baseline.get(key,0),row.get('RestartCount',0))
        self.assertEqual(baseline,{('p','app'):2})
        # With replicas=2 both must be admitted at the shared baseline.
        expected[('p','app')]=dict(svc,deploy={'replicas':2})
        mod.check_effective_containers(expected,[c,second],images,baseline)

    def test_resume_is_disabled_before_remote_access(self):
        with mock.patch.object(mod,'ssh') as ssh:
            with self.assertRaises(SystemExit):mod.launch('edgex','anything','missing','',4000,True)
            ssh.assert_not_called()

    def test_stack_stop_includes_edgex_export_profile(self):
        with mock.patch.object(mod, 'remote') as remote:
            mod.stop_owned_stacks()
        edgex = [call.args[1] for call in remote.call_args_list if '-p edgex ' in call.args[1]]
        self.assertEqual(len(edgex), 1)
        self.assertIn('--profile export stop', edgex[0])

    def test_finish_stops_edgex_exporter_too(self):
        with mock.patch.object(mod, 'owned_workload_check'), \
                mock.patch.object(mod, 'stage_edgex_helper', return_value='/safe/helper.sh'), \
                mock.patch.object(mod, 'remote') as remote:
            mod.finish('edgex', 'test-run')
        self.assertIn('--profile export stop', remote.call_args_list[-1].args[1])

    def test_edgex_helper_has_no_destructive_compose_actions(self):
        source = (ROOT / 'deploy/edgex/scripts/provision-then-scale.sh').read_text()
        commands = '\n'.join(line for line in source.splitlines() if not line.lstrip().startswith('#'))
        self.assertNotIn('compose down', commands)
        self.assertNotIn('--remove-orphans', commands)

    def test_edgex_helper_hash_mismatch_is_refused(self):
        with mock.patch.object(mod, 'ssh', return_value=mock.Mock(returncode=0, stdout='bad  helper.sh')):
            with self.assertRaisesRegex(RuntimeError, 'hash mismatch'):
                mod.stage_edgex_helper()

    def test_existing_non_empty_evidence_is_refused(self):
        with tempfile.TemporaryDirectory() as d:
            with mock.patch.object(mod, "EVIDENCE", pathlib.Path(d)):
                out = mod.ensure_evidence_dir("disp-tb-r01-20261106")
                (out / "keep.json").write_text("{}\n")
                with self.assertRaises(SystemExit):
                    mod.ensure_evidence_dir("disp-tb-r01-20261106")

    def test_fetch_flattens_run_dir_and_sidecars(self):
        with tempfile.TemporaryDirectory() as d:
            out = pathlib.Path(d)

            def fake_scp(argv, **kwargs):
                dest = pathlib.Path(argv[-1])
                rid = "disp-tb-r01-20261106"
                if dest.name == ".run-dir":
                    (dest / rid).mkdir(parents=True)
                    (dest / rid / "records.ndjson").write_text("{}\n")
                    (dest / rid / "resolved.json").write_text("{}\n")
                else:
                    (dest / f"{rid}.sink.ndjson").write_text("\n")
                    (dest / f"{rid}.dispatch-analysis.json").write_text("{}\n")
                return mock.Mock(returncode=0, stderr="")

            with mock.patch.object(mod.subprocess, "run", side_effect=fake_scp):
                errors = mod.fetch_evidence("disp-tb-r01-20261106", out)
            self.assertEqual(errors, [])
            self.assertTrue((out / "disp-tb-r01-20261106" / "records.ndjson").exists())
            self.assertTrue((out / "disp-tb-r01-20261106" / "resolved.json").exists())
            self.assertTrue((out / "disp-tb-r01-20261106.sink.ndjson").exists())
            self.assertTrue((out / "disp-tb-r01-20261106.dispatch-analysis.json").exists())
            self.assertFalse((out / ".run-dir").exists())
            self.assertFalse((out / ".files").exists())


class LaunchOrderingTests(unittest.TestCase):
    def exercise(self, root, cleanup_ok=True, history_error=False, post_defect=False):
        platform = 'edgex' if post_defect else 'thingsboard'
        rid = 'disp-ex-r01-20261106' if post_defect else 'disp-tb-r01-20261106'
        events = []
        def ssh(host, command, **kwargs):
            if command.startswith('cat '):
                return mock.Mock(returncode=0, stdout=json.dumps(resolved_for(platform,rid)), stderr='')
            if '/snapshot' in command:
                return mock.Mock(returncode=0, stdout='snapshot\trow\n', stderr='')
            if command.startswith('rmdir '):
                events.append('unlock')
            return mock.Mock(returncode=0, stdout='', stderr='')
        def cleanup(run_id, out, owned):
            events.append('cleanup')
            (out / 'cleanup-verification.json').write_text(json.dumps({'exit': 0 if cleanup_ok else 1}))
            return cleanup_ok
        def fetch(run_id, out):
            events.append('retrieve')
            (out / f'{rid}.hosts.json').write_text('{}')
            return []
        def history(platform, run_id, out):
            events.append('history')
            if history_error:
                raise TimeoutError('synthetic history timeout')
            (out / f'{rid}.history.ndjson').write_text('')
            return True
        def analyze(platform, run_id, out):
            events.append('analysis')
            (out / f'{rid}.dispatch-analysis.json').write_text(json.dumps({
                'measurement_valid': True, 'capture_valid': True, 'latency_assessed': True,
                'generator_resource_qualified': True, 'history_assessed': True,
                'platform_result': 'VALID_PLATFORM_SUCCESS'}))
            return True
        def gate(run_id,out):
            (out/'edgex-gate.check.json').write_text(json.dumps({'defect':post_defect,'new_restarts':int(post_defect)}))
        def finish(*args):
            events.append('stop')
            if post_defect: raise RuntimeError('metadata restarted during run')
        with mock.patch.multiple(mod, EVIDENCE=root, ssh=ssh,
                remote=mock.Mock(), owned_workload_check=mock.Mock(),
                stage_reviewed_source=mock.Mock(return_value=[]),
                activate=mock.Mock(), verify_live=mock.Mock(return_value='verified'),
                wait_tb_backends=mock.Mock(return_value='ready metrics'),
                read_only_check=mock.Mock(), verify_effective_resources=mock.Mock(return_value={}),
                cleanup_run=cleanup, fetch_evidence=fetch,
                export_history_from_workstation=history, reanalyze_with_history=analyze,
                fetch_edgex_gate=gate, finish=finish):
            rc = mod.launch(platform, rid, mod.HERE / 'plans' / f'{rid}.json', '', 4000)
        return rc, events, json.loads((root / rid / 'run-result.json').read_text())

    def test_unlock_is_last_after_history_analysis_and_stop(self):
        with tempfile.TemporaryDirectory() as d:
            rc, events, result = self.exercise(pathlib.Path(d))
            self.assertEqual(rc, 0)
            self.assertEqual(events, ['cleanup', 'retrieve', 'history', 'analysis', 'stop', 'unlock'])
            self.assertFalse(result['workload_lock_retained'])

    def test_failed_cleanup_keeps_lock_and_does_not_stop_platform(self):
        with tempfile.TemporaryDirectory() as d:
            rc, events, result = self.exercise(pathlib.Path(d), cleanup_ok=False)
            self.assertEqual(rc, 1)
            self.assertNotIn('stop', events)
            self.assertNotIn('unlock', events)
            self.assertTrue(result['workload_lock_retained'])

    def test_history_timeout_keeps_result_and_lock(self):
        with tempfile.TemporaryDirectory() as d:
            rc, events, result = self.exercise(pathlib.Path(d), history_error=True)
            self.assertEqual(rc, 1)
            self.assertFalse(result['history_retrieved'])
            self.assertNotIn('stop', events)
            self.assertNotIn('unlock', events)

    def test_metadata_defect_overrides_platform_success_without_erasing_latency(self):
        with tempfile.TemporaryDirectory() as d:
            rc, events, result = self.exercise(pathlib.Path(d), post_defect=True)
            self.assertEqual(rc,1)
            self.assertTrue(result['post_run_defect'])
            self.assertTrue(result['latency_assessed'])
            self.assertEqual(result['platform_result'],'VALID_PLATFORM_FAILURE_POST_RUN_DEFECT')
            self.assertTrue(result['workload_lock_retained'])



class RemoteAbsentTest(unittest.TestCase):
    """`test ! -e` over ssh is three-valued; a transport failure is not "present".

    Regression: a transient 255 (jump-host hiccup) was read as "remote evidence
    already exists", which refused a run that had never started and, because the
    local evidence dir had already been created, poisoned the retry as well.
    """

    def test_absent_path_is_true(self):
        with mock.patch.object(mod, 'ssh', return_value=mock.Mock(returncode=0, stdout='', stderr='')):
            self.assertTrue(mod.remote_absent('load', '/runs/x.capture-window.json'))

    def test_present_path_is_false(self):
        with mock.patch.object(mod, 'ssh', return_value=mock.Mock(returncode=1, stdout='', stderr='')):
            self.assertFalse(mod.remote_absent('load', '/runs/x.capture-window.json'))

    def test_transport_failure_raises_instead_of_claiming_presence(self):
        with mock.patch.object(mod, 'ssh',
                               return_value=mock.Mock(returncode=255, stdout='',
                                                      stderr='No route to host')):
            with self.assertRaisesRegex(RuntimeError, 'could not determine whether'):
                mod.remote_absent('load', '/runs/x.capture-window.json')


if __name__ == "__main__":
    unittest.main()
