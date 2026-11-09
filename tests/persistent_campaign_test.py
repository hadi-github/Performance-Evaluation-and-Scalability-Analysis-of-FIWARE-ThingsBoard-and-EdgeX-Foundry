"""No SSH/traffic: persistent supervisor must halt rather than retry or skip failures."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('persistent', ROOT/'scripts/continue-ordinary-campaign.py')
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
launcher_spec = importlib.util.spec_from_file_location('persistent_launcher', ROOT/'results/feasible-campaign-20261106/run-feasible-campaign.py')
launcher = importlib.util.module_from_spec(launcher_spec)
launcher_spec.loader.exec_module(launcher)


class TransportTests(unittest.TestCase):
    def test_explicit_config_applies_to_both_ssh_and_scp(self):
        with tempfile.TemporaryDirectory() as d:
            config = Path(d)/'ssh_config'; config.write_text('Host *\n')
            with mock.patch.dict(os.environ, IOT_BENCH_SSH_CONFIG=str(config)):
                self.assertEqual(launcher.scp_command('a','b')[:3], ['scp','-F',str(config)])
                with mock.patch.object(launcher.subprocess, 'run') as run:
                    run.return_value.returncode=0
                    launcher.ssh('ramonavm1','hostname')
                    self.assertEqual(run.call_args.args[0][:3], ['ssh','-F',str(config)])
                    self.assertIn('StrictHostKeyChecking=yes', run.call_args.args[0])
            with mock.patch.dict(os.environ, IOT_BENCH_SSH_CONFIG=str(config)+'-absent'):
                with self.assertRaises(RuntimeError): launcher.transport_options()

    def test_prebuilt_source_mismatch_fails_before_remote_access(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d); prebuilt=root/'prebuilt'; prebuilt.mkdir()
            (prebuilt/'build-provenance.json').write_text(json.dumps({'go_source_sha256':'wrong'}))
            out=root/'out'; out.mkdir()
            with mock.patch.dict(os.environ, IOT_BENCH_PREBUILT_DIR=str(prebuilt)), mock.patch.object(launcher,'ssh') as ssh:
                with self.assertRaisesRegex(RuntimeError, 'Go source'):
                    launcher.stage_reviewed_source('probe', root/'unused-plan', out)
                ssh.assert_not_called()

    def test_prebuilt_binary_mismatch_fails_before_remote_access(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);prebuilt=root/'prebuilt';prebuilt.mkdir();(prebuilt/'benchctl').write_bytes(b'changed')
            (prebuilt/'build-provenance.json').write_text(json.dumps({'go_source_sha256':'source','sha256':{'benchctl':'wrong'}}))
            out=root/'out';out.mkdir()
            with mock.patch.dict(os.environ,IOT_BENCH_PREBUILT_DIR=str(prebuilt)),mock.patch.object(launcher,'build_source_sha256',return_value='source'),mock.patch.object(launcher,'ssh') as ssh:
                with self.assertRaisesRegex(RuntimeError,'binary hash mismatch'):launcher.stage_reviewed_source('probe',root/'unused',out)
                ssh.assert_not_called()

    def test_launch_rejects_prebuilt_before_remote_checks_or_evidence_creation(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);(root/'build-provenance.json').write_text(json.dumps({'go_source_sha256':'wrong'}))
            with mock.patch.dict(os.environ,IOT_BENCH_PREBUILT_DIR=str(root)),mock.patch.object(launcher,'ssh') as ssh,mock.patch.object(launcher,'ensure_evidence_dir') as evidence:
                with self.assertRaisesRegex(RuntimeError,'Go source'):
                    launcher.launch('fiware','disp-fw-r03-20261106',ROOT/'unused','',4000)
                ssh.assert_not_called();evidence.assert_not_called()


class ProbeGuardTests(unittest.TestCase):
    def test_probe_refuses_ordinary_id_and_missing_tmux_before_creating_output(self):
        with tempfile.TemporaryDirectory() as d:
            out=Path(d)/'never-created'
            for rid in ('disp-fw-r03-20261106','probe-detach-test'):
                result=subprocess.run(['bash',str(ROOT/'scripts/probe-detached-generator.sh'),rid,str(out)],
                    env=dict(os.environ,TMUX=''),text=True,capture_output=True,timeout=10)
                self.assertNotEqual(result.returncode,0)
                self.assertFalse(out.exists())


class SupervisorTests(unittest.TestCase):
    def fixture(self, root):
        base=root/'campaign'; base.mkdir()
        cells=[{'ordinal':i,'platform':'fiware','run_id':f'example-{i}','plan':f'plan-{i}.json'} for i in (1,2)]
        matrix=base/'run-matrix.json'; matrix.write_text(json.dumps({'ordinary_runs':cells}))
        (base/'frozen-scope.json').write_text('{}')
        for cell in cells:(base/cell['plan']).write_text('{}')
        probe=root/'probe.json';probe.write_text(json.dumps({'verdict':'PASS_PERSISTENCE_GENERATOR_PROBE','reviewed_for_continuation':True}))
        review=root/'review.json'
        prebuilt=root/'prebuilt';prebuilt.mkdir()
        for name in ('benchctl','sink'):(prebuilt/name).write_bytes(name.encode())
        provenance={'go_source_sha256':launcher.build_source_sha256(),
                    'sha256':{name:hashlib.sha256((prebuilt/name).read_bytes()).hexdigest() for name in ('benchctl','sink')}}
        (prebuilt/'build-provenance.json').write_text(json.dumps(provenance))
        with mock.patch.object(mod,'BASE',base),mock.patch.dict(os.environ,IOT_BENCH_PREBUILT_DIR=str(prebuilt)):
            binding=mod.input_binding(cells,1)
        review.write_text(json.dumps({'verdict':'ACCEPTED_PERSISTENT_ORDINARY_CONTINUATION','authorized_start_ordinal':1,
            'matrix_sha256':hashlib.sha256(matrix.read_bytes()).hexdigest(),
            'controller_source_sha256':hashlib.sha256(Path(mod.__file__).read_bytes()).hexdigest(),
            'launcher_source_sha256':hashlib.sha256(mod.LAUNCHER.read_bytes()).hexdigest(),
            'probe_receipt_sha256':hashlib.sha256(probe.read_bytes()).hexdigest(),'input_binding':binding}))
        args=argparse.Namespace(review=review,probe_receipt=probe,start_ordinal=1,execute_reviewed=True,state_dir=root/'state',prebuilt=prebuilt)
        return base,cells,args

    def result(self, cell):
        return dict({key:True for key in mod.REQUIRED},run_id=cell['run_id'],platform=cell['platform'],
            run_kind='ordinary',workload_policy='dispatch-integrity-v1',platform_result='VALID_PLATFORM_SUCCESS',
            history_result='VALID_HISTORY_SUCCESS',workload_lock_retained=False,post_run_defect=None,
            fetch_errors=[],finish='stopped owned services; volumes preserved',plan_sha256=hashlib.sha256(b'{}').hexdigest())

    def test_success_is_serial_and_completed_ids_are_recorded(self):
        with tempfile.TemporaryDirectory() as d:
            base,cells,args=self.fixture(Path(d)); launched=[]
            def run(cmd,**kwargs):
                if 'launch' in cmd:
                    rid=cmd[cmd.index('--run-id')+1];cell=next(c for c in cells if c['run_id']==rid)
                    launched.append(rid);out=base/'evidence'/rid;out.mkdir(parents=True)
                    (out/'run-result.json').write_text(json.dumps(self.result(cell)))
            with mock.patch.object(mod,'BASE',base),mock.patch.dict(os.environ,IOT_BENCH_SSH_CONFIG='mock',IOT_BENCH_PREBUILT_DIR=str(args.prebuilt)),mock.patch.object(mod.subprocess,'run',side_effect=run):
                self.assertEqual(mod.execute(args),0)
                self.assertEqual(launched,['example-1','example-2'])
                state=json.loads((args.state_dir/'controller-state.json').read_text())
                self.assertEqual(state['state'],'FINISHED_ORIGINAL_MATRIX')
                with self.assertRaisesRegex(RuntimeError,'already exists'):mod.execute(args)

    def test_failure_halts_without_retry_or_next_cell(self):
        with tempfile.TemporaryDirectory() as d:
            base,cells,args=self.fixture(Path(d)); launched=[]
            def run(cmd,**kwargs):
                if 'launch' in cmd:
                    launched.append(cmd[cmd.index('--run-id')+1]);raise subprocess.CalledProcessError(1,cmd)
            with mock.patch.object(mod,'BASE',base),mock.patch.dict(os.environ,IOT_BENCH_SSH_CONFIG='mock',IOT_BENCH_PREBUILT_DIR=str(args.prebuilt)),mock.patch.object(mod.subprocess,'run',side_effect=run):
                with self.assertRaises(subprocess.CalledProcessError):mod.execute(args)
            self.assertEqual(launched,['example-1'])
            self.assertEqual(json.loads((args.state_dir/'controller-state.json').read_text())['state'],'PAUSED_REQUIRES_REVIEW')

    def test_stop_file_pauses_before_any_remote_command(self):
        with tempfile.TemporaryDirectory() as d:
            base,cells,args=self.fixture(Path(d));args.state_dir.mkdir();(args.state_dir/'STOP_AFTER_CURRENT').touch()
            with mock.patch.object(mod,'BASE',base),mock.patch.dict(os.environ,IOT_BENCH_SSH_CONFIG='mock',IOT_BENCH_PREBUILT_DIR=str(args.prebuilt)),mock.patch.object(mod.subprocess,'run') as run:
                self.assertEqual(mod.execute(args),0);run.assert_not_called()

    def test_missing_cleanup_or_held_lock_cannot_be_success(self):
        cell={'run_id':'example','platform':'fiware'}
        for key,value in [('cleanup_verified',False),('history_retrieved',False),('workload_lock_retained',True),('post_run_defect',True)]:
            result=self.result(cell);result[key]=value
            with self.subTest(key=key),self.assertRaises(RuntimeError):mod.verify_success(result,cell)

    def test_missing_or_null_fetch_errors_is_not_verified_empty_fetch(self):
        cell={'run_id':'example','platform':'fiware'}
        for value in (None,'',{}):
            result=self.result(cell);result['fetch_errors']=value
            with self.subTest(value=value),self.assertRaises(RuntimeError):mod.verify_success(result,cell)
        result=self.result(cell);del result['fetch_errors']
        with self.assertRaises(RuntimeError):mod.verify_success(result,cell)

    def test_changed_frozen_plan_rejected_before_any_subprocess(self):
        with tempfile.TemporaryDirectory() as d:
            base,cells,args=self.fixture(Path(d));(base/cells[1]['plan']).write_text('{"changed":true}')
            with mock.patch.object(mod,'BASE',base),mock.patch.dict(os.environ,IOT_BENCH_SSH_CONFIG='mock',IOT_BENCH_PREBUILT_DIR=str(args.prebuilt)),mock.patch.object(mod.subprocess,'run') as run:
                with self.assertRaisesRegex(RuntimeError,'scope/plans/measurement'):mod.execute(args)
                run.assert_not_called()
                self.assertFalse(args.state_dir.exists())
