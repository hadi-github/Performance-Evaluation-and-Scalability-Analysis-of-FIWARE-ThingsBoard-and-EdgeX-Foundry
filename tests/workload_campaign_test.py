import hashlib
import importlib.util
import json
import tempfile
import unittest
from pathlib import Path

SCRIPT=Path(__file__).resolve().parents[1]/'scripts/workload-campaign.py'
spec=importlib.util.spec_from_file_location('workload_campaign',SCRIPT)
campaign=importlib.util.module_from_spec(spec); spec.loader.exec_module(campaign)

class CampaignTest(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory(); self.root=Path(self.tmp.name); (self.root/'plans').mkdir()
        self.scope={'execution_authorized':False,'variants':{'W1':{'devices':1000,'replicas_per_replicated_service':2},'W4':{'devices':1000,'replicas_per_replicated_service':1}}}
        self.rows=[]
        for wid,platform,rid,n in [('W1','tb','w1-r01',2),('W4','tb','w4-r01',1),('W1','fw','w1-fw',2)]:
            pdata={'devices':1000}; p=self.root/'plans'/f'{rid}.json'; p.write_text(json.dumps(pdata))
            self.rows.append({'workload_id':wid,'platform':{'tb':'thingsboard','fw':'fiware'}[platform], 'repetition':1,'run_id':rid,'plan':f'plans/{rid}.json','plan_sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'execution_status':'BLOCKED_PENDING_VARIANT_RELEASE','deployment':{'replicas_per_replicated_service':n,'application_cpu_budget_k':2}})
        self.write()
    def tearDown(self): self.tmp.cleanup()
    def write(self):
        (self.root/'scope.json').write_text(json.dumps(self.scope,indent=2)+'\n')
        raw=json.dumps(self.scope,indent=2)+'\n'
        (self.root/'matrix.json').write_text(json.dumps({'scope_sha256':hashlib.sha256(raw.encode()).hexdigest(),'status':'PROPOSED_NOT_LAUNCH_CLEARED','experimental_runs':self.rows}))
    def runcheck(self, records=()): return campaign.validate(self.root/'matrix.json',self.root/'scope.json',self.root,records)
    def test_grouping_and_replica_intent(self):
        out=self.runcheck(); self.assertEqual(len(out['groups']),3)
        self.assertEqual(out['deployment_intent']['W4']['replicas_per_replicated_service'],1)
        self.assertEqual(out['deployment_intent']['W4']['application_cpu_budget_k'],2)
    def record(self,row,kind='variant-experimental'):
        return dict(run_id=row['run_id'],workload_id=row['workload_id'],platform=row['platform'],repetition=row['repetition'],plan_sha256=row['plan_sha256'],run_kind=kind,scheduled=10,received_by_cutoff=9,missing_by_cutoff=1)
    def test_matched_id_once_and_outcomes(self):
        out=self.runcheck([self.record(self.rows[0])]); obs=next(g for g in out['groups'] if g['workload_id']=='W1' and g['platform']=='thingsboard')['observed']
        self.assertEqual(obs[0]['received'],9); self.assertIsNone(obs[0]['status'])
        with self.assertRaisesRegex(ValueError,'duplicate observed'): self.runcheck([self.record(self.rows[0]),self.record(self.rows[0])])
    def test_unplanned_and_qualification_isolated(self):
        rec=self.record(self.rows[0]); rec['run_id']='not-planned'
        with self.assertRaisesRegex(ValueError,'unplanned'): self.runcheck([rec])
        out=self.runcheck([{'run_id':'qual-1','run_kind':'qualification'}]); self.assertEqual(out['isolated_records'][0]['counted'],False)
    def test_duplicate_id_rejected(self):
        with self.assertRaisesRegex(ValueError,'duplicate'): self.rows.append(dict(self.rows[0])); self.write(); self.runcheck()
    def test_reused_id_and_kind_isolation(self):
        rec=self.record(self.rows[0]); rec['workload_id']='W4'
        with self.assertRaisesRegex(ValueError,'workload_id mismatch'): self.runcheck([rec])
        with self.assertRaisesRegex(ValueError,'unplanned'): self.runcheck([{'run_id':'old','run_kind':'ordinary'}])
    def test_plan_hash_and_configuration_binding(self):
        p=self.root/'plans/w1-r01.json'; p.write_text('{"devices":2000}')
        with self.assertRaisesRegex(ValueError,'hash mismatch'): self.runcheck()
    def test_b0_matrix_scope_group(self):
        bscope=self.root/'b0-scope.json'; bscope.write_text('{"execution_authorized":false}')
        plan=self.root/'plans/b0.json'; plan.write_text('{"devices":1000}')
        bm=self.root/'b0-matrix.json'; bm.write_text(json.dumps({'frozen_scope_sha256':hashlib.sha256(bscope.read_bytes()).hexdigest(),'ordinary_runs':[{'run_id':'b0-r1','repetition':1,'platform':'thingsboard','plan':'plans/b0.json'}]}))
        out=campaign.validate(self.root/'matrix.json',self.root/'scope.json',self.root,[{'run_id':'b0-r1','workload_id':'B0','platform':'thingsboard','repetition':1,'plan_sha256':hashlib.sha256(plan.read_bytes()).hexdigest(),'run_kind':'ordinary'}],bm,bscope)
        self.assertTrue(any(g['workload_id']=='B0' for g in out['groups']))



class TreeBindingTests(unittest.TestCase):
    """The launch gate must enforce every binding the receipt records.

    Regression: the verifier hardcoded three names and silently ignored the rest,
    including `b0_launcher` -- the process that actually runs the workload. A
    changed B0 launcher could therefore run under a stale receipt.
    """

    def setUp(self):
        import importlib.util
        spec = importlib.util.spec_from_file_location(
            'rwc', Path(__file__).resolve().parents[1] / 'scripts' / 'run-workload-campaign.py')
        self.mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.mod)

    def test_b0_launcher_drift_is_caught(self):
        receipt = {'bindings_sha256': {'b0_launcher': '0' * 64}}
        with self.assertRaisesRegex(SystemExit, 'b0_launcher'):
            self.mod.verify_tree_bindings(receipt, 'r')

    def test_any_recorded_binding_is_enforced_not_just_a_subset(self):
        receipt = {'bindings_sha256': {'pool_policy': 'f' * 64, 'aggregator': 'e' * 64}}
        with self.assertRaisesRegex(SystemExit, 'aggregator.*pool_policy|pool_policy.*aggregator'):
            self.mod.verify_tree_bindings(receipt, 'r')

    def test_receipt_with_no_bindings_fails_closed(self):
        with self.assertRaisesRegex(SystemExit, 'no tree bindings'):
            self.mod.verify_tree_bindings({'bindings_sha256': {}}, 'r')

    def test_matching_bindings_pass(self):
        import hashlib
        name, path = self.mod.BOUND_ARTEFACTS[0]
        receipt = {'bindings_sha256': {name: hashlib.sha256(path.read_bytes()).hexdigest()}}
        self.assertIs(self.mod.verify_tree_bindings(receipt, 'r'), receipt)


if __name__ == '__main__':
    unittest.main()
