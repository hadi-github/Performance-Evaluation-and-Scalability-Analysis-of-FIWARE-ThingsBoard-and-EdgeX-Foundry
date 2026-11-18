"""Aggregation tests: per-run percentiles reported, never pooled; invalid runs retained."""
import csv
import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
RID = "disp-tb-r01-20261106"


def analysis(platform_run, p99, valid=True, assessed=True):
    return {
        "schema_version": 1,
        "run_id": platform_run,
        "measurement_valid": valid,
        "capture_valid": True,
        "latency_assessed": assessed,
        "platform_result": "VALID_PLATFORM_SUCCESS" if valid else "NOT_ASSESSED_MEASUREMENT_INVALID",
        "history_status": "OK",
        "history_assessed": True,
        "workload_integrity": {"valid": True, "findings": []},
        "generator_resource_qualified": True,
        "sink": {"duplicate_extra_receipts": 0, "late_rows_after_cutoff": 0,
                 "platform_anomalies": {"malformed_bodies": 0, "truncated_bodies": 0, "oversized_bodies": 0}},
        "phases": [{
            "phase": "measurement", "scheduled": 15000, "received_by_cutoff": 15000,
            "missing_by_cutoff": 0,
            "conditional_dispatch_to_receipt_ms": {"p50": 1, "p95": 2, "p99": p99},
            "conditional_scheduled_to_receipt_ms": {"p99": 3},
            "outcomes": {"unknown": 0, "failure": 0, "unknown_but_delivered": 0,
                         "acknowledged_but_history_missing": 0},
            "history_present": 15000, "history_missing": 0, "history_duplicate_extra": 0,
        }],
    }


class AggregateTests(unittest.TestCase):
    def test_canonical_analysis_wins_over_nested_preserved_versions(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);rid='disp-fw-r02-20261106'
            canonical=root/rid;canonical.mkdir()
            original=canonical/'recovery'/'remote-original-analysis';original.mkdir(parents=True)
            a=canonical/(rid+'.dispatch-analysis.json');a.write_text('{}')
            (original/a.name).write_text('{}')
            import importlib.util
            spec=importlib.util.spec_from_file_location('aggregate_discovery',ROOT/'scripts/aggregate-runs.py')
            module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
            self.assertEqual(module.discover([root])[rid],a.resolve())
            a.unlink()
            other=canonical/'other-preserved';other.mkdir();(other/a.name).write_text('{}')
            with self.assertRaisesRegex(SystemExit,'ambiguous analyses'):module.discover([root])

    def test_zero_traffic_setup_recovery_stays_visible_without_message_loss(self):
        with tempfile.TemporaryDirectory() as d:
            rid='disp-tb-r01-20261106'
            result={'run_id':rid,'platform':'thingsboard','platform_result':'NOT_ASSESSED_NO_ANALYSIS',
                    'workload_lock_retained':True,'finish':'skipped'}
            self.write_run(d,rid,result=result)
            root=pathlib.Path(d);out=root/rid;run=out/rid;run.mkdir()
            (run/'records.ndjson').write_text('')
            (run/'k6-summary.json').write_text(json.dumps({'steps':[{'attempted':0,'scenario_start_ms':0}]}))
            recovery={'run_id':rid,'outcome':'NOT_ASSESSED_SETUP_FAILURE','original_run_result_preserved':True,
                      'offered_workload_messages':0,'planned_messages':22500,'automatic_replacement':False,
                      'finish':'stopped; volumes preserved','workload_lock_retained':False}
            (out/'recovery-result.json').write_text(json.dumps(recovery))
            original=(out/'run-result.json').read_bytes()
            p=self.run_tool(d);self.assertEqual(p.returncode,0,p.stderr)
            with (root/'runs.csv').open() as f:row=next(csv.DictReader(f))
            self.assertEqual(row['platform_result'],'NOT_ASSESSED_SETUP_FAILURE')
            self.assertEqual(row['workload_lock_retained'],'False')
            self.assertEqual(row['setup_offered_messages'],'0')
            self.assertEqual(row['setup_planned_messages'],'22500')
            self.assertEqual(row['missing_by_cutoff'],'')
            self.assertEqual((out/'run-result.json').read_bytes(),original)
            self.assertIn('not platform message loss',(root/'summary.md').read_text())
            (run/'records.ndjson').write_text('{"rec":"msg"}\n')
            self.assertNotEqual(self.run_tool(d,'--force').returncode,0)

    def test_harness_defect_is_excluded_but_stays_a_consumed_id(self):
        with tempfile.TemporaryDirectory() as d:
            # Three genuine repetitions plus one bench-defect run in the same cell.
            for i in (1, 2, 3):
                rid = f"disp-tb-r0{i}-20261106"
                self.write_run(d, rid, analysis(rid, 10 * i),
                               {"platform": "thingsboard", "run_id": rid, "finish": "stopped"})
            bad = "disp-tb-r04-20261106"
            self.write_run(d, bad, analysis(bad, 99),
                           {"platform": "thingsboard", "run_id": bad, "finish": "stopped"})
            (pathlib.Path(d) / bad / "harness-defect.json").write_text(json.dumps({
                'run_id': bad, 'platform_finding': False,
                'outcome': 'HARNESS_DEFECT_EXCLUDE_FROM_REPETITIONS',
                'defect': {'component': 'thingsboard root rule chain',
                           'missing_rule_node': 'bench sink'},
                'references': {'defect_narrative': 'HARNESS-DEFECT.md'}}))
            p = self.run_tool(d)
            self.assertEqual(p.returncode, 0, p.stderr)
            with (pathlib.Path(d) / 'runs.csv').open() as f:
                rows = {r['run_id']: r for r in csv.DictReader(f) if r['phase'] == 'measurement'}
            self.assertEqual(rows[bad]['campaign_qualified'], 'False')
            self.assertEqual(rows[bad]['harness_defect'], 'True')
            self.assertIn('bench sink', rows[bad]['harness_defect_reason'])
            for i in (1, 2, 3):
                self.assertEqual(rows[f"disp-tb-r0{i}-20261106"]['campaign_qualified'], 'True')
            text = (pathlib.Path(d) / 'summary.md').read_text()
            self.assertIn(bad, text)                      # still a consumed ID
            self.assertIn('Excluded from the repetition set', text)
            self.assertIn('across 3 latency-assessed run(s)', text)
            self.assertNotIn('Target is five', text)

    def test_resolved_harness_defect_is_not_excluded(self):
        with tempfile.TemporaryDirectory() as d:
            rid = "disp-ex-r04-20261106"
            self.write_run(d, rid, analysis(rid, 20),
                           {"platform": "edgex", "run_id": rid, "finish": "stopped"})
            (pathlib.Path(d) / rid / "harness-defect.json").write_text(json.dumps({
                'run_id': rid, 'platform_finding': None,
                'outcome': 'HARNESS_DEFECT_RESOLVED_RECOVERED'}))
            p = self.run_tool(d)
            self.assertEqual(p.returncode, 0, p.stderr)
            with (pathlib.Path(d) / 'runs.csv').open() as f:
                row = next(csv.DictReader(f))
            self.assertEqual(row['harness_defect'], 'False')
            self.assertEqual(row['campaign_qualified'], 'True')

    def write_run(self, root, run_id, data=None, result=None):
        d = pathlib.Path(root) / run_id
        d.mkdir(parents=True)
        if data is not None:
            (d / f"{run_id}.dispatch-analysis.json").write_text(json.dumps(data))
            result = dict(result or {}, sut_resource_qualified=True, post_run_check_qualified=True, history_retrieved=True)
        if result is not None:
            (d / "run-result.json").write_text(json.dumps(result))

    def build(self, root):
        self.write_run(root, "disp-tb-r01-20261106", analysis("disp-tb-r01-20261106", 10),
                       {"platform": "thingsboard", "run_id": "disp-tb-r01-20261106", "finish": "stopped"})
        self.write_run(root, "disp-tb-r02-20261106", analysis("disp-tb-r02-20261106", 30),
                       {"platform": "thingsboard", "run_id": "disp-tb-r02-20261106", "finish": "stopped"})
        self.write_run(root, "disp-tb-r03-20261106", analysis("disp-tb-r03-20261106", None, valid=False, assessed=False))
        self.write_run(root, "disp-fw-r01-20261106", analysis("disp-fw-r01-20261106", 50))
        self.write_run(root, "disp-ex-r01-20261106", None,
                       {"platform": "edgex", "run_id": "disp-ex-r01-20261106", "measurement_valid": None,
                        "latency_assessed": None, "platform_result": "NOT_ASSESSED_NO_ANALYSIS",
                        "finish": "skipped: history or analysis not retrieved"})

    def run_tool(self, root, *extra):
        return subprocess.run([sys.executable, "scripts/aggregate-runs.py", str(root), *extra],
                              cwd=ROOT, text=True, capture_output=True)

    def test_summary_reports_per_run_and_never_pools(self):
        with tempfile.TemporaryDirectory() as d:
            self.build(d)
            p = self.run_tool(d)
            self.assertEqual(p.returncode, 0, p.stderr)
            text = (pathlib.Path(d) / "summary.md").read_text()
            self.assertIn("min 10.000 ms, median 20.000 ms, max 30.000 ms", text)
            self.assertIn("disp-tb-r03-20261106", text)  # invalid attempt retained
            self.assertIn("Invalid attempts kept in the matrix", text)
            self.assertIn("not assessed", text)  # no-analysis attempt
            with (pathlib.Path(d) / "runs.csv").open() as f:
                rows = list(csv.DictReader(f))
            tb_measurements = [r for r in rows if r["platform"] == "thingsboard"
                               and r["phase"] == "measurement" and r["latency_assessed"] == "True"]
            self.assertEqual([r["dispatch_p99_ms"] for r in tb_measurements], ["10", "30"])
            self.assertTrue(any(r["phase"] == "no-analysis" for r in rows))

    def test_workload_ids_are_identified_and_never_pooled_across_workloads(self):
        """W1 and W2 on one platform are distinct fixed points, never one group."""
        import importlib.util
        spec = importlib.util.spec_from_file_location('agg_w', ROOT / 'scripts/aggregate-runs.py')
        m = importlib.util.module_from_spec(spec); spec.loader.exec_module(m)
        self.assertEqual(m.platform_of('w1dp-tb-r01-20261108', {}), 'thingsboard')
        self.assertEqual(m.platform_of('w2dp-fw-r03-20261108', {}), 'fiware')
        self.assertEqual(m.platform_of('w4dp-ex-r05-20261108', {}), 'edgex')
        self.assertEqual(m.workload_of('w3dp-tb-r02-20261108'), 'W3')
        self.assertEqual(m.run_kind('w1dp-tb-r01-20261108'), 'workload')
        self.assertIsNone(m.workload_of('disp-tb-r01-20261106'))
        self.assertEqual(m.run_kind('disp-tb-r01-20261106'), 'ordinary')
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            self.write_run(root, 'w1dp-tb-r01-20261108', analysis('w1dp-tb-r01-20261108', 11),
                           {'platform': 'thingsboard', 'run_id': 'w1dp-tb-r01-20261108', 'finish': 'stopped'})
            self.write_run(root, 'w2dp-tb-r01-20261108', analysis('w2dp-tb-r01-20261108', 99),
                           {'platform': 'thingsboard', 'run_id': 'w2dp-tb-r01-20261108', 'finish': 'stopped'})
            p = self.run_tool(root)
            self.assertEqual(p.returncode, 0, p.stderr)
            text = (root / 'summary.md').read_text()
            self.assertIn('thingsboard — W1', text)
            self.assertIn('thingsboard — W2', text)
            # Each section's replicate statistic must reflect only its own workload.
            w1 = text.split('thingsboard — W1', 1)[1].split('thingsboard — W2', 1)[0]
            self.assertIn('min 11.000 ms, median 11.000 ms, max 11.000 ms', w1)
            self.assertNotIn('99.000', w1)
            with (root / 'runs.csv').open() as f:
                ids = {(r['run_id'], r['workload_id']) for r in csv.DictReader(f)}
            self.assertIn(('w1dp-tb-r01-20261108', 'W1'), ids)
            self.assertIn(('w2dp-tb-r01-20261108', 'W2'), ids)

    def test_refuses_to_overwrite_without_force(self):
        with tempfile.TemporaryDirectory() as d:
            self.build(d)
            self.assertEqual(self.run_tool(d).returncode, 0)
            p = self.run_tool(d)
            self.assertNotEqual(p.returncode, 0)
            self.assertEqual(self.run_tool(d, "--force").returncode, 0)

    def test_qualification_never_enters_ordinary_replicate_statistics(self):
        with tempfile.TemporaryDirectory() as d:
            self.build(d)
            rid = 'tb-dispatch-endpoint-qual-20261106'
            self.write_run(d, rid, analysis(rid, 9999))
            p = self.run_tool(d)
            self.assertEqual(p.returncode, 0, p.stderr)
            text = (pathlib.Path(d) / 'summary.md').read_text()
            self.assertIn('min 10.000 ms, median 20.000 ms, max 30.000 ms', text)
            self.assertIn('| qualification |', text)

    def test_unqualified_resources_and_post_run_defects_stay_diagnostic(self):
        with tempfile.TemporaryDirectory() as d:
            self.build(d)
            root=pathlib.Path(d)
            path=root/'disp-tb-r01-20261106'/'run-result.json'
            result=json.loads(path.read_text());result['post_run_defect']=True
            path.write_text(json.dumps(result))
            path=root/'disp-tb-r02-20261106'/'disp-tb-r02-20261106.dispatch-analysis.json'
            a=json.loads(path.read_text());a['generator_resource_qualified']=False
            path.write_text(json.dumps(a))
            p=self.run_tool(d);self.assertEqual(p.returncode,0,p.stderr)
            text=(root/'summary.md').read_text()
            self.assertIn('VALID_PLATFORM_FAILURE_POST_RUN_DEFECT',text)
            self.assertNotIn('median 20.000 ms',text)
            with (root/'runs.csv').open() as f:rows=list(csv.DictReader(f))
            rows=[r for r in rows if r['run_id'] in ('disp-tb-r01-20261106','disp-tb-r02-20261106')]
            self.assertTrue(all(r['campaign_qualified']=='False' for r in rows))
            self.assertEqual([r['dispatch_p99_ms'] for r in rows],['10','30'])

    def test_missing_history_export_is_not_exported_empty_history(self):
        with tempfile.TemporaryDirectory() as d:
            self.build(d);root=pathlib.Path(d)
            rid='disp-tb-r01-20261106';path=root/rid/f'{rid}.dispatch-analysis.json'
            a=json.loads(path.read_text());a['history_status']='EXPORT_FAILED';a['history_assessed']=False
            path.write_text(json.dumps(a))
            rid='disp-tb-r02-20261106';path=root/rid/f'{rid}.dispatch-analysis.json'
            a=json.loads(path.read_text());a['phases'][0]['history_missing']=15000
            a['phases'][0]['history_present']=0;a['platform_result']='VALID_PLATFORM_FAILURE_OR_BACKLOG'
            path.write_text(json.dumps(a))
            p=self.run_tool(d);self.assertEqual(p.returncode,0,p.stderr)
            with (root/'runs.csv').open() as f:rows={r['run_id']:r for r in csv.DictReader(f)}
            self.assertEqual(rows['disp-tb-r01-20261106']['campaign_qualified'],'False')
            self.assertEqual(rows['disp-tb-r02-20261106']['campaign_qualified'],'True')


if __name__ == "__main__":
    unittest.main()
