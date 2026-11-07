import json
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
RID = "postrun-fixture"


def write_json(path, obj):
    path.write_text(json.dumps(obj) + "\n")


def fixture(root):
    root = pathlib.Path(root)
    results = root / "results"
    run = results / RID
    run.mkdir(parents=True)
    plan = {
        "run_id": RID,
        "devices": 5,
        "health": {"workload_policy": "dispatch-integrity-v1", "early_tolerance_ms": 2},
        "arrivals": {"model": "constant"},
        "test_faults": {},
        "vus": {"pre_allocated": 150, "max": 150},
        "steps": [{
            "index": 0, "name": "measurement", "phase": "measurement",
            "seq_offset": 0, "ticks": 250, "tick_rate": 25,
            "duration_s": 10, "start_s": 0,
        }],
    }
    summary = {
        "steps": [{
            "index": 0, "name": "measurement", "scenario_start_ms": 100000,
            "attempted": 250, "ack_success": 250, "explicit_failure": 0,
            "unknown": 0, "dropped_iterations": 0,
        }]
    }
    records = []
    sink = []
    for i in range(250):
        scheduled = 100000 + i * 40
        dispatch = scheduled + 5
        records.append({
            "rec": "msg", "run_id": RID, "seq": i, "device_id": i % 5,
            "rate_step": 0, "tick": i, "phase": "measurement",
            "scheduled_send_ms": scheduled, "actual_send_ms": dispatch,
            "response_ms": dispatch + 10, "outcome": "ack_success", "early": False,
        })
        sink.append({
            "run_id": RID, "seq": i,
            "receipt_capture_semantics": "complete-body-read-before-parse-v1",
            "receipt_complete_body_read_ms": dispatch + 100, "dup": False,
        })
    write_json(run / "resolved.json", plan)
    write_json(run / "k6-summary.json", summary)
    write_json(run / "manifest.json", {"run_id": RID})
    (run / "records.ndjson").write_text("".join(json.dumps(x) + "\n" for x in records))
    (results / f"{RID}.sink.ndjson").write_text("".join(json.dumps(x) + "\n" for x in sink))
    write_json(results / f"{RID}.capture-window.json", {
        "schema_version": 1, "run_id": RID, "complete": True,
        "capture_recording_complete": True, "quiet_interval_passed": True,
        "observation_cutoff_ms": 120000, "capture_end_ms": 122000,
        "sink_metrics": {
            "received_total": len(sink), "duplicates_total": 0,
            "unparsed_total": 0, "overflow_total": 0,
            "incomplete_body_total": 0, "body_read_error_total": 0,
            "oversized_body_total": 0,
        },
    })
    samples = []
    for t in range(99000, 123000, 1000):
        samples.append({
            "load_realtime_ns": str(t * 1_000_000),
            "load_monotonic_before_ns": str(t * 1_000_000),
            "load_monotonic_after_ns": str(t * 1_000_000 + 1000),
            "clock_getres_ns": {"realtime": 1, "monotonic": 1},
            "realtime_step_discontinuity_candidate": False,
            "container_present": True, "container_id": "cid",
        })
    diag = results / f"{RID}.generator-diagnostics.jsonl"
    diag.write_text("".join(json.dumps(x) + "\n" for x in samples))
    write_json(results / f"{RID}.generator-diagnostics.jsonl.status.json", {
        "complete": True, "sampling_complete": True, "step_discontinuity_candidates": 0,
    })
    history = root / "history.ndjson"
    history.write_text("".join(
        json.dumps({"run_id": RID, "seq": i}) + "\n" for i in range(250)
    ))
    write_json(results / f'{RID}.hosts.json', {
        'generator_host_id':'fixture-load-host','sink_host_id':'fixture-load-host',
        'same_load_host_asserted':True,
        'sink_process':{'pid':123,'start_ticks':'456','output':'fixture.ndjson'},
    })
    return results, history


class PostRunAnalysisTests(unittest.TestCase):
    def test_missing_capture_host_evidence_is_never_attested_from_analysis_host(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            (results / f'{RID}.hosts.json').unlink()
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 0, p.stderr)
            a = json.loads((results / f'{RID}.dispatch-analysis.json').read_text())
            self.assertTrue(a['measurement_valid'])
            self.assertFalse(a['latency_assessed'])
            self.assertFalse(a['host_evidence']['same_load_host_asserted'])
            self.assertNotIn('generator_host_id', a['host_evidence'])

    def test_duplicate_only_delivery_is_valid_platform_failure(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            sink = results / f'{RID}.sink.ndjson'
            row = json.loads(sink.read_text().splitlines()[0]); row['dup'] = True
            with sink.open('a') as f: f.write(json.dumps(row)+'\n')
            window_path = results / f'{RID}.capture-window.json'
            window = json.loads(window_path.read_text())
            window['sink_metrics']['received_total'] += 1
            window['sink_metrics']['duplicates_total'] = 1
            window_path.write_text(json.dumps(window))
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 0, p.stderr)
            a = json.loads((results / f'{RID}.dispatch-analysis.json').read_text())
            self.assertTrue(a['measurement_valid']); self.assertTrue(a['latency_assessed'])
            self.assertEqual(a['phases'][0]['conditional_dispatch_to_receipt_ms']['p99'], 100)
            self.assertEqual(a['platform_result'], 'VALID_PLATFORM_FAILURE_OR_BACKLOG')

    def run_script(self, results, history, *extra):
        return subprocess.run(
            ["bash", "scripts/post-run-analysis.sh", RID, str(results), "thingsboard",
             "--history", str(history), *extra],
            cwd=ROOT, text=True, capture_output=True,
        )

    def test_automatic_analysis_produces_primary_metric_and_status(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            p = self.run_script(results, history, "--require-valid")
            self.assertEqual(p.returncode, 0, p.stderr)
            analysis = json.loads((results / f"{RID}.dispatch-analysis.json").read_text())
            self.assertTrue(analysis["measurement_valid"])
            self.assertTrue(analysis["latency_assessed"])
            self.assertEqual(analysis["endpoint"], "application_dispatch_to_response_complete_body_read")
            phase = analysis["phases"][0]
            self.assertEqual(phase["conditional_dispatch_to_receipt_ms"]["p99"], 100)
            self.assertEqual(phase["missing_by_cutoff"], 0)
            status = json.loads((results / f"{RID}.analysis-status.json").read_text())
            self.assertTrue(status["produced"])
            self.assertEqual(status["analysis_exit"], 0)
            self.assertTrue((results / f"{RID}.dispatch-analysis.md").exists())
            self.assertTrue((results / f"{RID}.dispatch-analysis.csv").exists())

    def test_missing_capture_window_refuses_analysis(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            (results / f"{RID}.capture-window.json").unlink()
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 2)
            self.assertFalse((results / f"{RID}.dispatch-analysis.json").exists())

    def test_missing_clock_evidence_is_not_assessed_not_crashed(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            (results / f"{RID}.generator-diagnostics.jsonl").unlink()
            (results / f"{RID}.generator-diagnostics.jsonl.status.json").unlink()
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 0, p.stderr)
            analysis = json.loads((results / f"{RID}.dispatch-analysis.json").read_text())
            self.assertTrue(analysis["measurement_valid"])
            self.assertFalse(analysis["latency_assessed"])
            self.assertEqual(analysis["clock_evidence"]["status"], "NOT_ASSESSED")
            strict = self.run_script(results, history, "--require-valid")
            self.assertNotEqual(strict.returncode, 0)

    def test_existing_analysis_is_not_silently_overwritten(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            self.assertEqual(self.run_script(results, history).returncode, 0)
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 2)
            self.assertIn("already exists", p.stderr)

    def test_empty_sink_and_successful_empty_history_are_valid_failure_evidence(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            (results / f'{RID}.sink.ndjson').write_text('')
            history.write_text('')
            window_path = results / f'{RID}.capture-window.json'
            window = json.loads(window_path.read_text())
            window['sink_metrics']['received_total'] = 0
            window_path.write_text(json.dumps(window))
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 0, p.stderr)
            a = json.loads((results / f'{RID}.dispatch-analysis.json').read_text())
            self.assertTrue(a['measurement_valid'])
            self.assertTrue(a['history_assessed'])
            self.assertEqual(a['phases'][0]['missing_by_cutoff'], 250)
            self.assertEqual(a['phases'][0]['history_missing'], 250)
            self.assertEqual(a['platform_result'], 'VALID_PLATFORM_FAILURE_OR_BACKLOG')

    def test_sink_success_does_not_hide_missing_history(self):
        with tempfile.TemporaryDirectory() as d:
            results, history = fixture(d)
            history.write_text('')
            p = self.run_script(results, history)
            self.assertEqual(p.returncode, 0, p.stderr)
            a = json.loads((results / f'{RID}.dispatch-analysis.json').read_text())
            self.assertEqual(a['phases'][0]['received_by_cutoff'], 250)
            self.assertEqual(a['history_result'], 'VALID_HISTORY_FAILURE')
            self.assertEqual(a['platform_result'], 'VALID_PLATFORM_FAILURE_OR_BACKLOG')


if __name__ == "__main__":
    unittest.main()
