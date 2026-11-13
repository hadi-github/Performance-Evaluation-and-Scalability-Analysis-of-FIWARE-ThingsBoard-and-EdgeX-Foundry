"""Offline staging only: W1-W4 must not mutate B0 or launch benchmark traffic."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('variants',ROOT/'scripts/stage-fixed-point-variants.py')
mod=importlib.util.module_from_spec(spec);spec.loader.exec_module(mod)


class FixedPointVariantsTests(unittest.TestCase):
    def test_same_one_factor_matrix_and_seeds_on_all_platforms(self):
        files=mod.artifacts();scope=files['scope.json'];rows=files['run-matrix.json']['experimental_runs']
        self.assertFalse(scope['execution_authorized']);self.assertEqual(len(rows),60)
        self.assertNotIn('warmup_rate_per_second',scope['shared'])
        self.assertEqual(scope['shared']['warmup_rate_rule'],'same as each workload total_messages_per_second')
        self.assertEqual(len({r['run_id'] for r in rows}),60)
        self.assertEqual([r['ordinal'] for r in rows],list(range(1,61)))
        for wid,(changed,value) in mod.CHANGES.items():
            variant=scope['variants'][wid]
            self.assertEqual({k for k in mod.BASELINE if variant[k]!=mod.BASELINE[k]},{changed})
            self.assertEqual(variant[changed],value)
            for platform in mod.PLATFORMS:
                selected=[r for r in rows if r['workload_id']==wid and r['platform']==platform]
                self.assertEqual([r['repetition'] for r in selected],[1,2,3,4,5])
                for row in selected:
                    baseline=json.loads((mod.B0/'plans'/f"disp-{mod.PREFIX[platform]}-r{row['repetition']:02d}-20261106.json").read_text())
                    plan=files[row['plan']]
                    self.assertEqual(plan['seed'],baseline['seed'])
                    self.assertEqual(len(row['run_id']),len(f"disp-{mod.PREFIX[platform]}-r01-20261106"))
                    for key in ('target','connection','request_timeout_ms','cleanup','test_faults'):
                        self.assertEqual(plan[key],baseline[key],key)
                    # vus and health are B0-identical everywhere except W1, which runs at 50/s and
                    # therefore carries the 2026-11-10 rate-derived pool (v2). Both deviations are
                    # recorded in scope.json; W2/W3/W4 stay byte-identical to B0 on these keys.
                    if wid=='W1':
                        self.assertEqual(plan['vus'],{'pre_allocated':300,'max':300})
                        self.assertEqual(plan['health']['workload_policy'],'dispatch-integrity-v2')
                        self.assertEqual(baseline['vus'],{'pre_allocated':150,'max':150})
                        self.assertEqual(baseline['health']['workload_policy'],'dispatch-integrity-v1')
                        self.assertEqual({k for k in plan['health'] if plan['health'][k]!=baseline['health'].get(k)},{'workload_policy'})
                    else:
                        for key in ('vus','health'):
                            self.assertEqual(plan[key],baseline[key],key)
                    self.assertEqual(row['deployment']['application_cpu_pool'],'4-5')
                    self.assertEqual(row['deployment']['application_cpu_budget_k'],2)
                    self.assertEqual(row['deployment']['replicas_per_replicated_service'],1 if wid=='W4' else 2)
                    self.assertEqual(plan['devices'],2000 if wid=='W2' else 1000)
                    self.assertEqual(plan['payload']['size_offset_bytes'],1024 if wid=='W3' else 0)
                    self.assertEqual([s['duration_s'] for s in plan['schedule']['steps']],[300,600])
                    self.assertEqual([s['rate'] for s in plan['schedule']['steps']],[50,50] if wid=='W1' else [25,25])
                    self.assertEqual((row['expected_warmup_messages'],row['expected_measurement_messages']),(15000,30000) if wid=='W1' else (7500,15000))
                    self.assertEqual(row['plan_sha256'],hashlib.sha256(mod.encoded(plan)).hexdigest())

    def test_interleaving_is_complete_and_contains_no_b0_replacement(self):
        rows=mod.artifacts()['run-matrix.json']['experimental_runs']
        for i in range(0,len(rows),3):
            group=rows[i:i+3]
            self.assertEqual(len({r['workload_id'] for r in group}),1)
            self.assertEqual({r['platform'] for r in group},set(mod.PLATFORMS))
        first_workloads=[rows[i]['workload_id'] for i in range(0,60,12)]
        self.assertEqual(first_workloads,['W1','W2','W3','W4','W1'])
        self.assertTrue(all(not r['run_id'].startswith('disp-') for r in rows))
        self.assertEqual(sum(r['expected_warmup_messages']+r['expected_measurement_messages'] for r in rows),1687500)
        self.assertEqual(mod.artifacts()['scope.json']['edgex_inventory_budget']['new_experimental_cohort_devices'],25000)

    def test_staging_preserves_b0_and_refuses_overwrite_or_changed_proposal(self):
        before=mod.baseline_hashes()
        with tempfile.TemporaryDirectory() as d:
            out=Path(d)/'batch';mod.stage(out);mod.stage(out,check=True)
            with self.assertRaises(FileExistsError):mod.stage(out)
            row=mod.artifacts()['run-matrix.json']['experimental_runs'][0]
            (out/row['plan']).write_text('{}')
            with self.assertRaisesRegex(ValueError,'differs'):mod.stage(out,check=True)
        self.assertEqual(before,mod.baseline_hashes())

    def test_check_with_missing_resolution_never_writes_or_runs_a_binary(self):
        with tempfile.TemporaryDirectory() as d:
            out=Path(d)/'batch';mod.stage(out)
            before={str(p.relative_to(out)):p.read_bytes() for p in out.rglob('*') if p.is_file()}
            with mock.patch.object(mod.subprocess,'run') as run:
                with self.assertRaisesRegex(ValueError,'offline resolution missing'):
                    mod.stage(out,benchctl=Path('/unused/benchctl'),check=True)
                run.assert_not_called()
            self.assertEqual(before,{str(p.relative_to(out)):p.read_bytes() for p in out.rglob('*') if p.is_file()})
