import importlib.util
import json
import pathlib
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("protocol_receipts", ROOT / "scripts/protocol-receipts.py")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class ProtocolReceiptAssemblyTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = pathlib.Path(self.tmp.name)
        self.run = self.root / "run"
        self.run.mkdir()
        binding = {"protocol_id": "p", "protocol_sha256": "a" * 64, "execution_class": "engineering_pilot"}
        pilot = {"drain_cutoff_unix_ms": 2500}
        resolved = {
            "run_id": "pilot-a", "adapter": "thingsboard", "total_ticks": 2, "devices": 1,
            "arrivals": {"model": "constant"}, "test_faults": {},
            "protocol_binding": binding, "protocol_pilot_parameters": pilot,
            "health": {"early_tolerance_ms": 2},
            "total_duration_s": 2,
            "steps": [
                {"index": 0, "name": "warmup", "phase": "warmup", "start_s": 0, "duration_s": 1,
                 "ticks": 1, "tick_rate": 1, "seq_offset": 0},
                {"index": 1, "name": "measurement", "phase": "measurement", "start_s": 1, "duration_s": 1,
                 "ticks": 1, "tick_rate": 1, "seq_offset": 1},
            ],
        }
        manifest = {
            "run_id": "pilot-a", "resolved_plan": resolved,
            "protocol_binding": binding,
            "protocol_pilot_parameters": pilot,
            "outcome_ledger_interpretation": "complete-dispatch-outcome-v1",
            "verdict": "VALID", "k6_exit_code": 0,
        }
        summary = {"run_id": "pilot-a", "steps": [
            {"index": 0, "name": "warmup", "scenario_start_ms": 500, "attempted": 1,
             "ack_success": 1, "explicit_failure": 0, "unknown": 0, "dropped_iterations": 0, "thinned": 0},
            {"index": 1, "name": "measurement", "scenario_start_ms": 1500, "attempted": 1,
             "ack_success": 1, "explicit_failure": 0, "unknown": 0, "dropped_iterations": 0, "thinned": 0},
        ]}
        self.write("resolved.json", resolved)
        self.write("manifest.json", manifest)
        (self.root / "pilot-a.benchctl-exit-code").write_text("0\n")
        self.write("k6-summary.json", summary)
        self.records = [
            {"rec": "msg", "run_id": "pilot-a", "seq": i, "device_id": 0,
             "rate_step": i, "tick": 0, "scheduled_send_ms": 500 + i * 1000,
             "actual_send_ms": 501 + i * 1000, "response_ms": 550 + i * 1000,
             "outcome": "ack_success", "early": False, "phase": "warmup" if i == 0 else "measurement"}
            for i in range(2)
        ]
        self.sink = [
            {"run_id": "pilot-a", "seq": i, "device_id": "bench-pilot-a-00000", "platform": "tb",
             "scheduled_send_ms": 500 + i * 1000, "receipt_complete_body_read_ms": 600 + i * 1000, "dup": False,
             "receipt_capture_semantics": MODULE.SEMANTICS}
            for i in range(2)
        ]
        self.write_run_ndjson("records.ndjson", self.records)
        self.write_ndjson("sink.ndjson", self.sink)
        self.write_root("window.json", {
            "schema_version": 1, "run_id": "pilot-a", "complete": True,
            "capture_start_ms": 400, "capture_end_ms": 3000, "observation_cutoff_ms": 2500,
            "sink_metrics": {"received_total": 2, "duplicates_total": 0, "unparsed_total": 0,
                             "overflow_total": 0, "incomplete_body_total": 0,
                             "body_read_error_total": 0, "oversized_body_total": 0},
        })

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, name, value):
        (self.run / name).write_text(json.dumps(value))

    def write_root(self, name, value):
        (self.root / name).write_text(json.dumps(value))

    def write_ndjson(self, name, rows):
        (self.root / name).write_text("".join(json.dumps(row) + "\n" for row in rows))

    def write_run_ndjson(self, name, rows):
        (self.run / name).write_text("".join(json.dumps(row) + "\n" for row in rows))

    def assemble(self):
        return MODULE.assemble(self.run, self.root / "sink.ndjson", self.root / "window.json")

    def test_assembles_first_complete_body_receipts_and_http_outcomes(self):
        capture = self.assemble()
        self.assertFalse(capture["synthetic"])
        self.assertEqual(capture["schema_version"], 2)
        self.assertEqual(capture["metric_interpretation"], "protocol-scheduled-cohort-v1")
        self.assertEqual(capture["endpoint"], "response_complete_body_read")
        self.assertEqual([r["seq"] for r in capture["receipts"]], [0, 1])
        self.assertEqual([r["outcome"] for r in capture["receipts"]], ["OK", "OK"])
        self.assertEqual(capture["receipts"][0]["device_id"], "bench-pilot-a-00000")
        self.assertNotIn("actual_dispatch_ms", capture["receipts"][0])

    def test_schema3_dispatch_capture_requires_explicit_plan_marker(self):
        resolved=json.loads((self.run/"resolved.json").read_text()); resolved["health"]["workload_policy"]="dispatch-integrity-v1"
        manifest=json.loads((self.run/"manifest.json").read_text()); manifest["resolved_plan"]=resolved
        self.write("resolved.json",resolved); self.write("manifest.json",manifest)
        capture=self.assemble()
        self.assertEqual(capture["schema_version"],3)
        self.assertEqual(capture["receipts"][0]["actual_dispatch_ms"],501)

    def test_rejects_nonzero_wrapper_or_generator_exit(self):
        (self.root / "pilot-a.benchctl-exit-code").write_text("2\n")
        with self.assertRaisesRegex(ValueError, "wrapper benchctl exit status is nonzero"):
            self.assemble()
        (self.root / "pilot-a.benchctl-exit-code").write_text("0\n")
        manifest = json.loads((self.run / "manifest.json").read_text())
        manifest["k6_exit_code"] = 1
        self.write("manifest.json", manifest)
        with self.assertRaisesRegex(ValueError, "valid zero-exit generator run"):
            self.assemble()

    def test_resolves_measurement_relative_cutoff_from_actual_scenario_end(self):
        pilot = {"drain_seconds_after_measurement": 900, "quiet_interval_seconds": 30, "late_observation_seconds": 30}
        resolved = json.loads((self.run / "resolved.json").read_text())
        manifest = json.loads((self.run / "manifest.json").read_text())
        resolved["protocol_pilot_parameters"] = pilot
        manifest["protocol_pilot_parameters"] = pilot
        manifest["resolved_plan"] = resolved
        self.write("resolved.json", resolved)
        self.write("manifest.json", manifest)
        window = json.loads((self.root / "window.json").read_text())
        window["observation_cutoff_ms"] = 902500
        window["capture_end_ms"] = 932500
        window["quiet_interval"] = {"required_seconds": 30, "start_ms": 872500, "end_ms": 902500, "passed": True}
        window["late_observation"] = {"required_seconds": 30, "start_ms": 902500, "end_ms": 932500, "late_receipt_rows": 0}
        self.write_root("window.json", window)
        capture = self.assemble()
        self.assertEqual(capture["observation_cutoff_ms"], 902500)
        window["observation_cutoff_ms"] = 902499
        self.write_root("window.json", window)
        with self.assertRaisesRegex(ValueError, "contradicts declared drain cutoff"):
            self.assemble()

    def test_relative_cutoff_requires_quiet_and_late_observation_windows(self):
        pilot = {"drain_seconds_after_measurement": 900, "quiet_interval_seconds": 30,
                 "late_observation_seconds": 30}
        resolved = json.loads((self.run / "resolved.json").read_text())
        manifest = json.loads((self.run / "manifest.json").read_text())
        resolved["protocol_pilot_parameters"] = pilot
        manifest["protocol_pilot_parameters"] = pilot
        manifest["resolved_plan"] = resolved
        self.write("resolved.json", resolved)
        self.write("manifest.json", manifest)
        window = json.loads((self.root / "window.json").read_text())
        window.update({"observation_cutoff_ms": 902500, "capture_end_ms": 932500,
                       "quiet_interval": {"start_ms": 872500, "end_ms": 902500, "passed": True},
                       "late_observation": {"start_ms": 902500, "end_ms": 932500, "late_receipt_rows": 0}})
        self.write_root("window.json", window)
        self.assertTrue(MODULE.assemble(self.run, self.root / "sink.ndjson", self.root / "window.json")["complete"])
        del window["late_observation"]
        self.write_root("window.json", window)
        with self.assertRaisesRegex(ValueError, "lacks explicit quiet or late-observation"):
            self.assemble()

    def test_relative_quiet_interval_must_be_inside_capture_coverage(self):
        pilot = {"drain_seconds_after_measurement": 900, "quiet_interval_seconds": 30,
                 "late_observation_seconds": 30}
        resolved = json.loads((self.run / "resolved.json").read_text())
        manifest = json.loads((self.run / "manifest.json").read_text())
        resolved["protocol_pilot_parameters"] = pilot
        manifest["protocol_pilot_parameters"] = pilot
        manifest["resolved_plan"] = resolved
        self.write("resolved.json", resolved)
        self.write("manifest.json", manifest)
        window = json.loads((self.root / "window.json").read_text())
        window.update({"observation_cutoff_ms": 902500, "capture_start_ms": 880000,
                       "capture_end_ms": 932500,
                       "quiet_interval": {"start_ms": 872500, "end_ms": 902500, "passed": True},
                       "late_observation": {"start_ms": 902500, "end_ms": 932500, "late_receipt_rows": 0}})
        window["sink_metrics"]["received_total"] = 0
        self.write_ndjson("sink.ndjson", [])
        self.write_root("window.json", window)
        with self.assertRaisesRegex(ValueError, "quiet interval begins before capture coverage"):
            self.assemble()

    def test_refuses_incomplete_window_and_nonzero_sink_diagnostics(self):
        window = json.loads((self.root / "window.json").read_text())
        window["complete"] = False
        self.write_root("window.json", window)
        with self.assertRaisesRegex(ValueError, "not explicitly complete"):
            self.assemble()
        window["complete"] = True
        window["sink_metrics"]["overflow_total"] = 1
        self.write_root("window.json", window)
        with self.assertRaisesRegex(ValueError, "nonzero"):
            self.assemble()

    def test_refuses_missing_dispatch_and_wrong_device_identity(self):
        self.write_run_ndjson("records.ndjson", self.records[:1])
        with self.assertRaisesRegex(ValueError, "identity set"):
            self.assemble()
        self.write_run_ndjson("records.ndjson", self.records)
        self.sink[0]["device_id"] = "wrong-device"
        self.write_ndjson("sink.ndjson", self.sink)
        with self.assertRaisesRegex(ValueError, "device identity mismatch"):
            self.assemble()

    def test_refuses_summary_mismatch_and_out_of_plan_sequence(self):
        summary = json.loads((self.run / "k6-summary.json").read_text())
        summary["run_id"] = "other-run"
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        with self.assertRaisesRegex(ValueError, "summary run_id"):
            self.assemble()
        summary["run_id"] = "pilot-a"
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        self.records[1]["seq"] = 100
        self.write_run_ndjson("records.ndjson", self.records)
        with self.assertRaisesRegex(ValueError, "identity/step/device mismatch"):
            self.assemble()

    def test_refuses_fractional_summary_counts_and_repeated_receipts(self):
        summary = json.loads((self.run / "k6-summary.json").read_text())
        summary["steps"][0]["ack_success"] = 1.9
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        with self.assertRaisesRegex(ValueError, "must be an integer"):
            self.assemble()
        summary["steps"][0]["ack_success"] = 1
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        duplicate = dict(self.sink[0])
        duplicate["dup"] = False
        self.write_ndjson("sink.ndjson", self.sink + [duplicate])
        window = json.loads((self.root / "window.json").read_text())
        window["sink_metrics"]["received_total"] = 3
        self.write_root("window.json", window)
        with self.assertRaisesRegex(ValueError, "dup flag|repeated receipt identities"):
            self.assemble()

    def test_refuses_manifest_binding_or_total_ticks_mismatch(self):
        manifest = json.loads((self.run / "manifest.json").read_text())
        manifest["protocol_binding"]["protocol_id"] = "other"
        (self.run / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "binding/pilot metadata"):
            self.assemble()
        manifest["protocol_binding"]["protocol_id"] = "p"
        (self.run / "manifest.json").write_text(json.dumps(manifest))
        resolved = json.loads((self.run / "resolved.json").read_text())
        resolved["total_ticks"] = 999
        self.write("resolved.json", resolved)
        manifest["resolved_plan"] = resolved
        (self.run / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "total_ticks"):
            self.assemble()

    def test_refuses_inconsistent_schedule_and_out_of_window_receipt(self):
        resolved = json.loads((self.run / "resolved.json").read_text())
        resolved["steps"][0]["duration_s"] = 100
        self.write("resolved.json", resolved)
        manifest = json.loads((self.run / "manifest.json").read_text())
        manifest["resolved_plan"] = resolved
        (self.run / "manifest.json").write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "tick_rate × duration"):
            self.assemble()
        resolved["steps"][0]["duration_s"] = 1
        self.write("resolved.json", resolved)
        manifest["resolved_plan"] = resolved
        (self.run / "manifest.json").write_text(json.dumps(manifest))
        summary = json.loads((self.run / "k6-summary.json").read_text())
        summary["steps"][1]["scenario_start_ms"] = 1600
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        with self.assertRaisesRegex(ValueError, "scenario_start_ms contradicts"):
            self.assemble()
        summary["steps"][1]["scenario_start_ms"] = 1500
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        self.sink[0]["receipt_complete_body_read_ms"] = 4000
        self.write_ndjson("sink.ndjson", self.sink)
        with self.assertRaisesRegex(ValueError, "outside the attested capture window"):
            self.assemble()
        self.sink[0]["receipt_complete_body_read_ms"] = 600
        self.sink[0]["scheduled_send_ms"] = 500
        self.sink[0]["receipt_complete_body_read_ms"] = 650
        self.records[0]["actual_send_ms"] = 700
        self.records[0]["response_ms"] = 750
        self.write_run_ndjson("records.ndjson", self.records)
        self.write_ndjson("sink.ndjson", self.sink)
        with self.assertRaisesRegex(ValueError, "precedes actual dispatch"):
            self.assemble()

    def test_accepts_observed_scenario_start_jitter_but_rejects_excess(self):
        summary = json.loads((self.run / "k6-summary.json").read_text())
        summary["steps"][1]["scenario_start_ms"] = 1499
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        self.records[1]["scheduled_send_ms"] = 1499
        self.records[1]["actual_send_ms"] = 1500
        self.records[1]["response_ms"] = 1549
        self.sink[1]["scheduled_send_ms"] = 1499
        self.sink[1]["receipt_complete_body_read_ms"] = 1599
        self.write_run_ndjson("records.ndjson", self.records)
        self.write_ndjson("sink.ndjson", self.sink)
        self.assertTrue(self.assemble()["complete"])
        summary["steps"][1]["scenario_start_ms"] = 1506
        (self.run / "k6-summary.json").write_text(json.dumps(summary))
        with self.assertRaisesRegex(ValueError, "scenario_start_ms contradicts"):
            self.assemble()


if __name__ == "__main__":
    unittest.main()
