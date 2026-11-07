"""Offline tests for the campaign launcher's frozen-cell validation and retrieval."""
import importlib.util
import copy
import json
import pathlib
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location(
    "run_feasible_campaign", ROOT / "results" / "feasible-campaign-20261106" / "run-feasible-campaign.py")
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


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


if __name__ == "__main__":
    unittest.main()
