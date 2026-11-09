import importlib.util, json, pathlib, tempfile, unittest

ROOT=pathlib.Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("dispatch_integrity",ROOT/"scripts/dispatch-integrity.py")
mod=importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)

class DispatchIntegrityTests(unittest.TestCase):
    def test_exact_constant_arrivals_pass_and_burst_fails(self):
        with tempfile.TemporaryDirectory() as d:
            run=pathlib.Path(d)
            plan={"run_id":"r","devices":5,"health":{"workload_policy":mod.POLICY,"early_tolerance_ms":2},"arrivals":{"model":"constant"},"test_faults":{},"vus":{"pre_allocated":150,"max":150},"steps":[{"index":0,"name":"s","phase":"measurement","seq_offset":0,"start_s":0,"ticks":250,"tick_rate":25,"duration_s":10}]}
            summary={"steps":[{"index":0,"name":"s","scenario_start_ms":100000,"attempted":250,"ack_success":250,"explicit_failure":0,"unknown":0,"dropped_iterations":0}]}
            rows=[{"rec":"msg","run_id":"r","seq":i,"device_id":i%5,"rate_step":0,"phase":"measurement","tick":i,"scheduled_send_ms":100000+i*40,"actual_send_ms":100000+i*40,"response_ms":100001+i*40,"outcome":"ack_success","early":False} for i in range(250)]
            (run/"resolved.json").write_text(json.dumps(plan)); (run/"k6-summary.json").write_text(json.dumps(summary))
            (run/"records.ndjson").write_text("".join(json.dumps(r)+"\n" for r in rows))
            result=mod.evaluate(run); self.assertTrue(result["valid"],result["findings"])
            self.assertEqual(result["phases"][0]["bins_1s"],[25]*10)
            for row in rows[:8]: row["actual_send_ms"]=100360; row["response_ms"]=100361
            (run/"records.ndjson").write_text("".join(json.dumps(r)+"\n" for r in rows))
            result=mod.evaluate(run); self.assertFalse(result["valid"])
            self.assertIn("100ms_bins",result["findings"])

    def test_legacy_plan_is_not_silently_reinterpreted(self):
        with tempfile.TemporaryDirectory() as d:
            run=pathlib.Path(d); (run/"resolved.json").write_text(json.dumps({"health":{},"steps":[]})); (run/"k6-summary.json").write_text("{}")
            with self.assertRaisesRegex(ValueError,"not explicitly bound"):
                mod.evaluate(run)

    def test_headroom_fifty_rate_checks_frozen_bins(self):
        with tempfile.TemporaryDirectory() as d:
            run=pathlib.Path(d); plan={"run_id":"r","devices":5,"health":{"workload_policy":mod.POLICY,"early_tolerance_ms":2},"arrivals":{"model":"constant"},"test_faults":{},"vus":{"pre_allocated":150,"max":150},"steps":[{"index":0,"name":"q","phase":"measurement","seq_offset":0,"start_s":0,"ticks":500,"tick_rate":50,"duration_s":10}]}
            summary={"steps":[{"index":0,"name":"q","scenario_start_ms":100000,"attempted":500,"ack_success":500,"explicit_failure":0,"unknown":0,"dropped_iterations":0}]}
            rows=[{"rec":"msg","run_id":"r","seq":i,"device_id":i%5,"rate_step":0,"phase":"measurement","tick":i,"scheduled_send_ms":100000+i*20,"actual_send_ms":100000+i*20,"response_ms":100001+i*20,"outcome":"ack_success","early":False} for i in range(500)]
            (run/"resolved.json").write_text(json.dumps(plan)); (run/"k6-summary.json").write_text(json.dumps(summary)); (run/"records.ndjson").write_text("".join(json.dumps(r)+"\n" for r in rows))
            result=mod.evaluate(run); self.assertTrue(result["valid"],result["findings"]); self.assertEqual(result["phases"][0]["bins_1s"],[50]*10)

    def test_100_rate_extension_keeps_strict_dispatch_integrity_checks(self):
        with tempfile.TemporaryDirectory() as d:
            run=pathlib.Path(d); plan={"run_id":"r100","devices":5,"health":{"workload_policy":mod.POLICY,"early_tolerance_ms":2},"arrivals":{"model":"constant"},"test_faults":{},"vus":{"pre_allocated":150,"max":150},"steps":[{"index":0,"name":"q100","phase":"measurement","seq_offset":0,"start_s":0,"ticks":1000,"tick_rate":100,"duration_s":10}]}
            summary={"steps":[{"index":0,"name":"q100","scenario_start_ms":200000,"attempted":1000,"ack_success":1000,"explicit_failure":0,"unknown":0,"dropped_iterations":0}]}
            rows=[{"rec":"msg","run_id":"r100","seq":i,"device_id":i%5,"rate_step":0,"phase":"measurement","tick":i,"scheduled_send_ms":200000+i*10,"actual_send_ms":200000+i*10,"response_ms":200001+i*10,"outcome":"ack_success","early":False} for i in range(1000)]
            (run/"resolved.json").write_text(json.dumps(plan)); (run/"k6-summary.json").write_text(json.dumps(summary)); (run/"records.ndjson").write_text("".join(json.dumps(r)+"\n" for r in rows))
            result=mod.evaluate(run); self.assertTrue(result["valid"],result["findings"])
            self.assertEqual(result["phases"][0]["bins_1s"],[100]*10)
            self.assertEqual(result["phases"][0]["bins_100ms"],[10]*100)
            rows[0]["early"]=True
            (run/"records.ndjson").write_text("".join(json.dumps(r)+"\n" for r in rows))
            with self.assertRaisesRegex(ValueError,"early-send marker mismatch"):
                mod.evaluate(run)

if __name__=="__main__": unittest.main()
