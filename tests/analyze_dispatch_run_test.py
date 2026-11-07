import importlib.util,json,pathlib,tempfile,unittest
ROOT=pathlib.Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location("analyze_dispatch_run",ROOT/"scripts/analyze-dispatch-run.py")
mod=importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)

class DispatchRunJoinTests(unittest.TestCase):
 def test_effective_cpu_and_ancestor_quota_are_required(self):
  s={'container_cpuset_requested':'0-1','cpu_stat':{'cpuset_cpus_effective':'0-1','cpu_max':['max','100000'],'ancestor_constraints':[{'path':'/sys/fs/cgroup','cpuset_cpus_effective':['0-3'],'cpu_max':None}]}}
  self.assertEqual(mod.generator_allocation(s),(True,True))
  s['cpu_stat']['cpuset_cpus_effective']='0-7'
  self.assertFalse(mod.generator_allocation(s)[0])
  s['cpu_stat']['cpuset_cpus_effective']='0-1'
  s['cpu_stat']['cpu_max']=['50000','100000']
  self.assertFalse(mod.generator_allocation(s)[1])
  s['cpu_stat']['cpu_max']=['max','100000']
  s['cpu_stat']['ancestor_constraints'][0]['cpu_max']=['100000','100000']
  self.assertFalse(mod.generator_allocation(s)[1])
  s['cpu_stat']['ancestor_constraints']=[]
  self.assertEqual(mod.generator_allocation(s),(False,False))
 def fixture(self,d):
  root=pathlib.Path(d); run=root/"run"; run.mkdir(); rid="r"
  plan={"run_id":rid,"devices":5,"health":{"workload_policy":"dispatch-integrity-v1","early_tolerance_ms":2},"arrivals":{"model":"constant"},"test_faults":{},"vus":{"pre_allocated":150,"max":150},"steps":[{"index":0,"name":"measurement","phase":"measurement","seq_offset":0,"ticks":250,"tick_rate":25,"duration_s":10,"start_s":0}]}
  summary={"steps":[{"index":0,"scenario_start_ms":100000,"attempted":250,"ack_success":248,"explicit_failure":1,"unknown":1,"dropped_iterations":0}]}
  records=[]; sink=[]
  for i in range(250):
   outcome="unknown" if i==249 else ("explicit_failure" if i==248 else "ack_success")
   records.append({"rec":"msg","run_id":rid,"seq":i,"device_id":i%5,"rate_step":0,"phase":"measurement","tick":i,"scheduled_send_ms":100000+i*40,"actual_send_ms":100005+i*40,"response_ms":100010+i*40,"outcome":outcome,"early":False})
   if i!=249: sink.append({"run_id":rid,"seq":i,"receipt_capture_semantics":"complete-body-read-before-parse-v1","receipt_complete_body_read_ms":100205+i*40,"dup":False})
  for name,obj in (("resolved.json",plan),("k6-summary.json",summary),("manifest.json",{"run_id":rid})): (run/name).write_text(json.dumps(obj))
  (run/"records.ndjson").write_text("".join(json.dumps(x)+"\n" for x in records))
  paths={}
  for key,rows in (("sink",sink),("history",[{"run_id":rid,"seq":i} for i in range(248)])):
   path=root/(key+".ndjson"); path.write_text("".join(json.dumps(x)+"\n" for x in rows)); paths[key]=path
  window={"run_id":rid,"complete":True,"capture_recording_complete":True,"quiet_interval_passed":True,"observation_cutoff_ms":120000,"capture_end_ms":122000,"sink_metrics":{"received_total":len(sink),"duplicates_total":0,"unparsed_total":0,"overflow_total":0,"incomplete_body_total":0,"body_read_error_total":0,"oversized_body_total":0}}
  paths["window"]=root/"window.json"; paths["window"].write_text(json.dumps(window))
  samples=[{"realtime_ms":t,"monotonic_ms":t-10000,"clock_getres_ns":{"realtime":1,"monotonic":1}} for t in range(99000,123000,1000)]
  paths["clock"]=root/"clock.json"; paths["clock"].write_text(json.dumps({"same_host":True,"coverage_complete":True,"status":"complete","step_candidates":[],"resolution_ms":1,"samples":samples}))
  paths["host"]=root/"hosts.json"; paths["host"].write_text(json.dumps({"same_load_host_asserted":True,"generator_host_id":"h","sink_host_id":"h","sink_process":{"pid":123,"start_ticks":"456","output":"fixture.ndjson"}}))
  return run,paths
 def analyze(self,run,p): return mod.analyze(run,p["sink"],p["window"],p["history"],p["clock"],p["host"],None,None,None)
 def test_per_message_dispatch_percentile_and_separate_unknown_missing(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); r=self.analyze(run,p); phase=r["phases"][0]
   self.assertTrue(r["measurement_valid"]); self.assertTrue(r["latency_assessed"])
   self.assertEqual(phase["conditional_dispatch_to_receipt_ms"]["p99"],200)
   self.assertEqual(phase["missing_by_cutoff"],1); self.assertEqual(phase["outcomes"]["unknown"],1)
   self.assertEqual(phase["history_missing"],2)
 def test_duplicate_receipts_preserved_and_first_receipt_used(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); row=json.loads(p["sink"].read_text().splitlines()[0]); row["receipt_complete_body_read_ms"]+=100
   with p["sink"].open("a") as f:f.write(json.dumps(row)+"\n")
   w=json.loads(p["window"].read_text()); w["sink_metrics"]["received_total"]+=1; w["sink_metrics"]["duplicates_total"]=1; p["window"].write_text(json.dumps(w))
   r=self.analyze(run,p); self.assertTrue(r["measurement_valid"]); self.assertEqual(r["sink"]["duplicate_extra_receipts"],1)
   self.assertEqual(r["phases"][0]["conditional_dispatch_to_receipt_ms"]["p99"],200)
 def test_negative_delay_disqualifies_clocked_latency_not_platform_result(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); row=json.loads(p["sink"].read_text().splitlines()[0]); row["receipt_complete_body_read_ms"]=row["receipt_complete_body_read_ms"]-500
   lines=p["sink"].read_text().splitlines(); lines[0]=json.dumps(row); p["sink"].write_text("\n".join(lines)+"\n")
   r=self.analyze(run,p); self.assertFalse(r["measurement_valid"]); self.assertFalse(r["latency_assessed"]); self.assertEqual(r["platform_result"],"NOT_ASSESSED_MEASUREMENT_INVALID")
 def test_clock_gap_over_two_seconds_unassessed(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); c=json.loads(p["clock"].read_text()); c["samples"]=[c["samples"][0],c["samples"][-1]]; p["clock"].write_text(json.dumps(c))
   r=self.analyze(run,p); self.assertTrue(r["measurement_valid"]); self.assertFalse(r["latency_assessed"])
 def test_quiet_failure_is_a_platform_failure_not_capture_loss(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); w=json.loads(p["window"].read_text()); w["quiet_interval_passed"]=False; p["window"].write_text(json.dumps(w))
   r=self.analyze(run,p); self.assertTrue(r["capture_valid"]); self.assertEqual(r["platform_result"],"VALID_PLATFORM_FAILURE_OR_BACKLOG")
 def test_resource_qualification_requires_fresh_gogc_and_container_evidence(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d)
   sampler=[]
   for t in range(99000,123000,1000): sampler.append({"load_realtime_ns":str(t*1000000),"load_monotonic_before_ns":str(t*1000000),"load_monotonic_after_ns":str(t*1000000+1000),"container_present":True,"container_id":"cid","container_oom_killed":False,"container_restarts":0,"generator_gogc":"400","cpu_stat":{"memory_current_bytes":100,"memory_peak_bytes":200,"memory_swap_current_bytes":0,"memory_events":{"oom":0,"oom_kill":0},"cpu_stat":{"nr_throttled":0}}})
   for s in sampler:
    s['container_cpuset_requested']='0-1'
    s['cpu_stat'].update({'cpuset_cpus_effective':'0-1','cpu_max':['max','100000'],'ancestor_constraints':[{'path':'/sys/fs/cgroup','cpuset_cpus_effective':['0-3'],'cpu_max':None}]})
   p["resource"]=pathlib.Path(d)/"sampler.jsonl"; p["resource"].write_text("".join(json.dumps(x)+"\n" for x in sampler))
   p["status"]=pathlib.Path(d)/"status.json"; p["status"].write_text(json.dumps({"complete":True,"sampling_complete":True,"step_discontinuity_candidates":0}))
   p["gen_cpu"]=pathlib.Path(d)/"gen-cpu.ndjson"; p["gen_cpu"].write_text("".join(json.dumps({"ms":t,"core_busy":[.5,.4]})+"\n" for t in range(100000,111000,1000)))
   r=mod.analyze(run,p["sink"],p["window"],p["history"],p["clock"],p["host"],p["resource"],p["status"],p["gen_cpu"])
   self.assertTrue(r["generator_resource_qualified"],r["resource_evidence"]["reasons"])
   sampler[0]["generator_gogc"]="100"; p["resource"].write_text("".join(json.dumps(x)+"\n" for x in sampler))
   r=mod.analyze(run,p["sink"],p["window"],p["history"],p["clock"],p["host"],p["resource"],p["status"],p["gen_cpu"])
   self.assertTrue(r["measurement_valid"]); self.assertFalse(r["generator_resource_qualified"])
   sampler[0]['cpu_stat']=None
   p['resource'].write_text(''.join(json.dumps(x)+'\n' for x in sampler))
   r=mod.analyze(run,p['sink'],p['window'],p['history'],p['clock'],p['host'],p['resource'],p['status'],p['gen_cpu'])
   self.assertTrue(r['measurement_valid'])
   self.assertFalse(r['generator_resource_qualified'])
   self.assertIn('memory_cgroup_evidence',r['resource_evidence']['reasons'])
   self.assertIn('no_swap_or_oom_events',r['resource_evidence']['reasons'])
   sampler[0]['container_pid']=0
   sampler[0]['generator_gogc']='400'
   p['resource'].write_text(''.join(json.dumps(x)+'\n' for x in sampler))
   r=mod.analyze(run,p['sink'],p['window'],p['history'],p['clock'],p['host'],p['resource'],p['status'],p['gen_cpu'])
   self.assertTrue(r['generator_resource_qualified'],r['resource_evidence']['reasons'])
   sampler[0]['container_oom_killed']=True
   p['resource'].write_text(''.join(json.dumps(x)+'\n' for x in sampler))
   r=mod.analyze(run,p['sink'],p['window'],p['history'],p['clock'],p['host'],p['resource'],p['status'],p['gen_cpu'])
   self.assertFalse(r['generator_resource_qualified'])
   self.assertIn('zero_oom_restarts',r['resource_evidence']['reasons'])
 def test_malformed_platform_body_is_platform_failure_not_invalid_capture(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); w=json.loads(p["window"].read_text())
   w["sink_metrics"]["unparsed_total"]=1
   p["window"].write_text(json.dumps(w))
   r=self.analyze(run,p)
   self.assertTrue(r["measurement_valid"]); self.assertTrue(r["latency_assessed"])
   self.assertEqual(r["sink"]["platform_anomalies"]["malformed_bodies"],1)
   self.assertEqual(r["platform_result"],"VALID_PLATFORM_FAILURE_OR_BACKLOG")
 def test_truncated_body_keeps_conditional_latency(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); w=json.loads(p["window"].read_text())
   w["sink_metrics"]["incomplete_body_total"]=1; w["sink_metrics"]["body_read_error_total"]=1; w["sink_metrics"]["unparsed_total"]=1
   p["window"].write_text(json.dumps(w)); r=self.analyze(run,p)
   self.assertTrue(r["measurement_valid"]); self.assertTrue(r["latency_assessed"])
   self.assertEqual(r["sink"]["platform_anomalies"]["truncated_bodies"],1)
   self.assertEqual(r["platform_result"],"VALID_PLATFORM_FAILURE_OR_BACKLOG")
 def test_sink_overflow_is_capture_invalidity(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); w=json.loads(p["window"].read_text()); w["sink_metrics"]["overflow_total"]=1
   p["window"].write_text(json.dumps(w)); r=self.analyze(run,p)
   self.assertFalse(r["measurement_valid"]); self.assertIn("sink_overflow_capture_loss",r["measurement_errors"])
   self.assertEqual(r["platform_result"],"NOT_ASSESSED_MEASUREMENT_INVALID")
 def test_inconsistent_body_counters_are_capture_invalidity(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d); w=json.loads(p["window"].read_text()); w["sink_metrics"]["unparsed_total"]=0
   w["sink_metrics"]["incomplete_body_total"]=1; w["sink_metrics"]["body_read_error_total"]=1
   p["window"].write_text(json.dumps(w)); r=self.analyze(run,p)
   self.assertFalse(r["measurement_valid"]); self.assertIn("unparsed_counter_inconsistent",r["measurement_errors"])
 def test_history_export_failure_is_not_assessed_not_every_row_missing(self):
  with tempfile.TemporaryDirectory() as d:
   run,p=self.fixture(d)
   r=mod.analyze(run,p["sink"],p["window"],None,p["clock"],p["host"],p.get("resource"),p.get("status"),p.get("gen_cpu"),"EXPORT_FAILED")
   self.assertFalse(r["history_assessed"]); self.assertEqual(r["history_status"],"EXPORT_FAILED")
   self.assertIsNone(r["phases"][0]["history_missing"])
   self.assertIsNone(r["phases"][0]["outcomes"]["acknowledged_but_history_missing"])
   self.assertEqual(r['history_result'],'NOT_ASSESSED')
   self.assertTrue(r["measurement_valid"]); self.assertTrue(r["latency_assessed"])
if __name__=="__main__":unittest.main()
