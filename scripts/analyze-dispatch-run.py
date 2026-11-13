#!/usr/bin/env python3
"""Join an explicitly dispatch-integrity-v1 run's dispatch, complete-body sink, history and clock evidence."""
import argparse, importlib.util, json, math
from collections import Counter
from pathlib import Path

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location("dispatch_integrity",HERE/"dispatch-integrity.py")
integrity=importlib.util.module_from_spec(spec); spec.loader.exec_module(integrity)

def lines(path):
    out=[]
    for n,line in enumerate(Path(path).read_text().splitlines(),1):
        if line.strip():
            try: out.append(json.loads(line))
            except json.JSONDecodeError as e: raise ValueError(f"{path}:{n}: invalid JSON") from e
    return out
def evidence(path):
    text=Path(path).read_text()
    try:
        obj=json.loads(text)
        return obj if isinstance(obj,dict) else {"samples":obj}
    except json.JSONDecodeError:
        return {"samples":lines(path)}
def percentile(v,p): return None if not v else sorted(v)[max(0,math.ceil(p*len(v))-1)]

def cpu_set(value):
    try:
        if isinstance(value,list): value=','.join(value)
        if not isinstance(value,str) or not value: return set()
        result=set()
        for part in value.split(','):
            edges=[int(x) for x in part.split('-')]
            if len(edges)==1: result.add(edges[0])
            elif len(edges)==2 and edges[0]<=edges[1]: result.update(range(edges[0],edges[1]+1))
            else: return set()
        return result
    except (ValueError,TypeError): return set()

def quota_allows_two(value):
    if not isinstance(value,list) or len(value)!=2: return False
    try: return int(value[1])>0 and (value[0]=='max' or int(value[0])/int(value[1])>=2)
    except (ValueError,TypeError,ZeroDivisionError): return False

def generator_allocation(sample):
    cg=sample.get('cpu_stat') or {}
    if not isinstance(cg,dict): return False,False
    parents=cg.get('ancestor_constraints')
    cpus=cpu_set(sample.get('container_cpuset_requested'))=={0,1} and cpu_set(cg.get('cpuset_cpus_effective'))=={0,1}
    quota=quota_allows_two(cg.get('cpu_max'))
    if not isinstance(parents,list) or not parents: return False,False
    for parent in parents:
        if not isinstance(parent,dict): return False,False
        cpus=cpus and {0,1}.issubset(cpu_set(parent.get('cpuset_cpus_effective')))
        # The cgroup-v2 root has no cpu.max file; every non-root ancestor must
        # provide its actual constraint. Missing evidence never means unlimited.
        root=parent.get('path')=='/sys/fs/cgroup' and parent.get('cpu_max') is None
        quota=quota and (root or quota_allows_two(parent.get('cpu_max')))
    return bool(cpus),bool(quota)

def analyze(run,sink_path,window_path,history_path,clock_path,hosts_path,resource_path,status_path,gen_cpu_path,history_status="OK"):
    run=Path(run); load=integrity.evaluate(run)
    plan=json.loads((run/"resolved.json").read_text()); summary=json.loads((run/"k6-summary.json").read_text()); rid=plan["run_id"]
    manifest=json.loads((run/"manifest.json").read_text())
    window=json.loads(Path(window_path).read_text()); clock=evidence(clock_path); hosts=json.loads(Path(hosts_path).read_text())
    dispatch=lines(run/"records.ndjson"); byseq={r["seq"]:r for r in dispatch}
    sink=lines(sink_path); history=lines(history_path) if history_status=="OK" else []; cutoff=float(window.get("observation_cutoff_ms",float("nan")))
    errors=[]; clock_errors=[]
    if window.get("run_id")!=rid or window.get("complete") is not True: errors.append("incomplete_or_mismatched_capture_window")
    if not math.isfinite(cutoff): errors.append("missing_observation_cutoff")
    sink_process=hosts.get('sink_process') or {}
    capture_proof=(type(sink_process.get('pid')) is int and sink_process['pid']>0 and
                   str(sink_process.get('start_ticks','')).isdigit() and bool(sink_process.get('output')))
    reviewed_reconstruction=(hosts.get('reconstructed_after_capture') is True and
                             isinstance(hosts.get('reconstruction_review'),str) and bool(hosts['reconstruction_review'].strip()))
    host_ok=(isinstance(hosts.get('generator_host_id'),str) and bool(hosts['generator_host_id']) and
             hosts.get("generator_host_id")==hosts.get("sink_host_id") and hosts.get("same_load_host_asserted") is True and
             (capture_proof or reviewed_reconstruction))
    if not host_ok: clock_errors.append("load_host_identity_not_proven")
    stamps=clock.get("samples",[])
    for x in stamps:
        if "mono_ms" not in x and "monotonic_ms" in x: x["mono_ms"]=x["monotonic_ms"]
        if "load_realtime_ns" in x:
            x["realtime_ms"]=int(x["load_realtime_ns"])/1e6
            x["mono_ms"]=(int(x["load_monotonic_before_ns"])+int(x["load_monotonic_after_ns"]))/2e6
            x["step_candidate"]=x.get("realtime_step_discontinuity_candidate")
    clock_gaps=[b.get("realtime_ms",0)-a.get("realtime_ms",0) for a,b in zip(stamps,stamps[1:])]
    step_candidates=any(x.get("step_candidate") is True for x in stamps)
    resolution_ok=bool(stamps) and all(isinstance(x.get("clock_getres_ns"),dict) and isinstance(x["clock_getres_ns"].get("realtime"),int) and x["clock_getres_ns"]["realtime"]>0 for x in stamps)
    clock_ok=(bool(stamps) and resolution_ok and window.get("capture_end_ms") is not None
      and stamps[0].get("realtime_ms",math.inf)<=min(float(r["actual_send_ms"]) for r in dispatch)
      and stamps[-1].get("realtime_ms",-math.inf)>=float(window["capture_end_ms"])
      and all(0 < g <= 2000 for g in clock_gaps) and not step_candidates)
    if not clock_ok: clock_errors.append("clock_coverage_missing_gap_over_2s_or_step_candidate")
    if clock_ok and any(abs((b["realtime_ms"]-a["realtime_ms"])-(b["mono_ms"]-a["mono_ms"]))>5 for a,b in zip(stamps,stamps[1:])):
        clock_ok=False; clock_errors.append("clock_step_discontinuity_candidate")
    if window.get("sink_metrics",{}).get("received_total")!=len(sink): errors.append("sink_count_counter_mismatch")
    counters=window.get("sink_metrics",{})
    for key in ("duplicates_total","unparsed_total","overflow_total","incomplete_body_total","body_read_error_total","oversized_body_total"):
        if type(counters.get(key)) is not int or counters[key]<0: errors.append("missing_or_invalid_sink_counter:"+key)
    # Attribution: overflow is capture loss. Oversized bodies exceed the receiver's
    # declared bound. Incomplete reads are truncated deliveries (no receipt, so the
    # identity stays conditional). Parse failures are malformed platform output.
    # The latter two are platform behaviour, not automatic measurement invalidity.
    overflow=counters.get("overflow_total",0); oversized=counters.get("oversized_body_total",0)
    read_errors=counters.get("body_read_error_total",0); incomplete=counters.get("incomplete_body_total",0)
    unparsed=counters.get("unparsed_total",0)
    if overflow>0: errors.append("sink_overflow_capture_loss")
    if oversized>0: errors.append("sink_oversized_body_limit")
    if incomplete!=read_errors+oversized: errors.append("incomplete_body_counter_inconsistent")
    parse_failures=unparsed-incomplete
    if unparsed<incomplete: errors.append("unparsed_counter_inconsistent")
    platform_anomalies={"malformed_bodies":max(0,parse_failures),"truncated_bodies":read_errors,"oversized_bodies":oversized}
    identity={}; receipt_rows=[]; late=[]; earliest={}
    for r in sink:
        if r.get("run_id")!=rid or not isinstance(r.get("seq"),int) or r["seq"] not in byseq:
            errors.append("unmatched_sink_identity"); continue
        if r.get("receipt_capture_semantics")!="complete-body-read-before-parse-v1": errors.append("receipt_semantics_missing")
        t=r.get("receipt_complete_body_read_ms")
        if isinstance(t,bool) or not isinstance(t,(int,float)) or not math.isfinite(t): errors.append("invalid_receipt_time"); continue
        seq=r["seq"]; identity[seq]=identity.get(seq,0)+1
        ev=byseq[seq]
        sink_sched=r.get("scheduled_send_ms")
        if isinstance(sink_sched,(int,float)) and not isinstance(sink_sched,bool) and abs(sink_sched-ev["scheduled_send_ms"])>.0011:
            errors.append(f"scheduled_clock_disagreement:{seq}")
        if t>cutoff: late.append(r); continue
        delay=t-ev["actual_send_ms"]
        if delay<0: errors.append(f"negative_dispatch_delay_or_clock_step:{seq}"); continue
        receipt_rows.append({"seq":seq,"phase":ev["phase"],"delay_ms":delay,"scheduled_delay_ms":t-ev["scheduled_send_ms"],"outcome":ev["outcome"]})
        if seq not in earliest or t < earliest[seq][0]: earliest[seq]=(t,receipt_rows[-1])
    duplicate=sum(n-1 for n in identity.values() if n>1)
    if counters.get("duplicates_total")!=duplicate: errors.append("duplicate_count_counter_mismatch")
    hist=Counter((r.get("run_id"),r.get("seq")) for r in history) if history_status=="OK" else None
    if hist is not None and any(runid!=rid or not isinstance(seq,int) for runid,seq in hist): errors.append("malformed_history_identity")
    phases=[]
    for phase in sorted({s["phase"] for s in plan["steps"]}):
        expected={s["seq_offset"]+i for s in plan["steps"] if s["phase"]==phase for i in range(s["ticks"])}
        rec=[entry[1] for seq,entry in earliest.items() if seq in expected]; n=len(expected)
        observed={r["seq"] for r in rec}; missing=len(expected-observed)
        delays=[r["delay_ms"] for r in rec]; sched=[r["scheduled_delay_ms"] for r in rec]
        phases.append({"phase":phase,"scheduled":n,"received_by_cutoff":len(observed),"missing_by_cutoff":missing,
          "conditional_dispatch_to_receipt_ms":{"p50":percentile(delays,.50),"p95":percentile(delays,.95),"p99":percentile(delays,.99)},
          "conditional_scheduled_to_receipt_ms":{"p50":percentile(sched,.50),"p95":percentile(sched,.95),"p99":percentile(sched,.99)},
          "outcomes":{"unknown":sum(r["outcome"]=="unknown" for r in byseq.values() if r["seq"] in expected),"failure":sum(r["outcome"]=="explicit_failure" for r in byseq.values() if r["seq"] in expected),
            "unknown_but_delivered":sum(byseq[seq]["outcome"]=="unknown" and seq in observed for seq in expected),
            "acknowledged_but_history_missing":(sum(byseq[seq]["outcome"]=="ack_success" and hist[(rid,seq)]==0 for seq in expected) if hist is not None else None)},
          "history_present":(sum(hist[(rid,seq)]>0 for seq in expected) if hist is not None else None),"history_missing":(sum(hist[(rid,seq)]==0 for seq in expected) if hist is not None else None),
          "history_duplicate_extra":(sum(max(0,hist[(rid,seq)]-1) for seq in expected) if hist is not None else None)})
    resource={"qualified":False,"reasons":[]}; cpu_samples=[]
    if resource_path:
        resource=evidence(resource_path)
        sampler=resource.get("samples",[])
        # An explicit --gen-cpu must exist. The implicit run-directory copy is optional:
        # runs without a generator CPU snapshot stay analytically valid but fail the
        # (separate) resource qualification.
        if gen_cpu_path:
            gen_cpu=lines(gen_cpu_path)
        else:
            default_cpu=run/"gen-cpu.ndjson"
            gen_cpu=lines(default_cpu) if default_cpu.exists() else []
        for s in sampler:
            s["realtime_ms"]=int(s.get("load_realtime_ns",0))/1e6 if s.get("load_realtime_ns") else None
        present=[s for s in sampler if s.get("container_present") is True]
        # Docker can still report an exited container with PID 0 for one poll.
        # It has no live cgroup to read; retain it for exit/OOM evidence, not as
        # an active resource sample. Missing data with a live PID still fails.
        active=[s for s in present if s.get('container_pid') != 0]
        ids={s.get("container_id") for s in active}; times=[s.get("realtime_ms") for s in sampler if s.get("realtime_ms") is not None]
        phase_end=max(float(summary["steps"][i]["scenario_start_ms"])+int(st["duration_s"])*1000 for i,st in enumerate(plan["steps"]))
        cpu_times=[float(s["ms"]) for s in gen_cpu if isinstance(s.get("ms"),(int,float))]
        sampler_status=json.loads(Path(status_path).read_text()) if status_path else {}
        active_times=[int(s["load_realtime_ns"])/1e6 for s in active if s.get("load_realtime_ns")]
        checks={"complete_sampler_status":sampler_status.get("complete") is True and sampler_status.get("sampling_complete") is True and sampler_status.get("step_discontinuity_candidates")==0,
          "fresh_sampler":bool(times) and times[0]<=min(float(r["actual_send_ms"]) for r in dispatch) and window.get("capture_end_ms") is not None and times[-1]>=float(window["capture_end_ms"]) and all(0<b-a<=2000 for a,b in zip(times,times[1:])),
          "same_container":len(ids)==1 and None not in ids and bool(active_times) and active_times[0]<=min(float(r["actual_send_ms"]) for r in dispatch) and active_times[-1]>=phase_end,
          "gogc400":bool(active) and all(s.get("generator_gogc")=="400" for s in active),
          "effective_generator_cpuset":bool(active) and all(generator_allocation(s)[0] for s in active),
          "generator_and_ancestor_quota":bool(active) and all(generator_allocation(s)[1] for s in active),
          "zero_oom_restarts":bool(present) and all(s.get("container_oom_killed") is False and s.get("container_restarts")==0 for s in present),
          "memory_cgroup_evidence":bool(active) and all(isinstance(s.get("cpu_stat"),dict) and all(isinstance(s["cpu_stat"].get(k),int) for k in ("memory_current_bytes","memory_peak_bytes","memory_swap_current_bytes")) and isinstance(s["cpu_stat"].get("memory_events"),dict) for s in active),
          "no_swap_or_oom_events":bool(active) and all(isinstance(s.get("cpu_stat"),dict) and isinstance(s["cpu_stat"].get("memory_events"),dict) and s["cpu_stat"].get("memory_swap_current_bytes")==0 and s["cpu_stat"]["memory_events"].get("oom")==0 and s["cpu_stat"]["memory_events"].get("oom_kill")==0 for s in active),
          "gen_cpu_samples":bool(gen_cpu) and cpu_times[0]<=min(float(r["actual_send_ms"]) for r in dispatch) and cpu_times[-1]>=phase_end and all(0<b-a<=2000 for a,b in zip(cpu_times,cpu_times[1:]))}
        cpu_samples=gen_cpu
        saturation=False
        for a in cpu_samples:
            start_ms=a.get("ms",0); window_samples=[x for x in cpu_samples if start_ms<=x.get("ms",-1)<start_ms+10000]
            if len(window_samples)>=10 and max(x.get("ms",0) for x in window_samples)-start_ms>=9000:
                for core in range(min((len(x.get("core_busy",[])) for x in window_samples),default=0)):
                    if sum(x["core_busy"][core] for x in window_samples)/len(window_samples)>=.95: saturation=True
        throttled=False
        try:
            start_stat=active[0]["cpu_stat"]["cpu_stat"]; end_stat=active[-1]["cpu_stat"]["cpu_stat"]
            throttled=end_stat.get("nr_throttled",0)>start_stat.get("nr_throttled",0)
        except (KeyError,TypeError,IndexError): pass
        resource={"checks":checks,"qualified":all(checks.values()),"saturation_flag":saturation,"cgroup_throttling_observed":throttled,"container_ids":sorted(str(x) for x in ids),"sampler_count":len(sampler),"gen_cpu_count":len(gen_cpu),"sampler_status":sampler_status}
        resource["reasons"]=[k for k,v in checks.items() if not v]
    saturation=resource.get("saturation_flag",False)
    resource_qualified=resource.get("qualified",False) and not (saturation and not load["valid"])
    valid_measurement=load["valid"] and not errors
    latency_assessed=valid_measurement and clock_ok and host_ok
    capture_valid=window.get("capture_recording_complete") is True and window.get("complete") is True
    if not latency_assessed:
        for phase in phases:
            phase["conditional_dispatch_to_receipt_ms"]={"p50":None,"p95":None,"p99":None}
            phase["conditional_scheduled_to_receipt_ms"]={"p50":None,"p95":None,"p99":None}
    platform_anomaly_count=sum(platform_anomalies.values())
    history_failure=hist is not None and any(p['history_missing']>0 or p['history_duplicate_extra']>0 for p in phases)
    platform_failure=(any(p["missing_by_cutoff"]>0 or p["outcomes"]["failure"]>0 or p["outcomes"]["unknown"]>0 for p in phases)
                      or duplicate>0 or bool(late) or window.get("quiet_interval_passed") is False or platform_anomaly_count>0 or history_failure)
    return {"schema_version":1,"policy":plan.get("health",{}).get("workload_policy") or "dispatch-integrity-v1","run_id":rid,"endpoint":"application_dispatch_to_response_complete_body_read",
      "clock_evidence":{"status":"VALID" if clock_ok else "NOT_ASSESSED","errors":clock_errors,"sample_count":len(stamps),"host_identity":host_ok},"host_evidence":hosts,"capture_window":window,"workload_integrity":load,"phases":phases,
      "sink":{"rows":len(sink),"unique_identities":len(identity),"duplicate_extra_receipts":duplicate,"late_rows_after_cutoff":len(late),"missing_by_cutoff_total":sum(p["missing_by_cutoff"] for p in phases),"platform_anomalies":platform_anomalies},
      "history_status":history_status,"history_assessed":hist is not None,
      "history_result":('VALID_HISTORY_FAILURE' if history_failure else 'VALID_HISTORY_SUCCESS') if hist is not None else 'NOT_ASSESSED',
      "measurement_errors":errors,"measurement_valid":valid_measurement,"capture_valid":capture_valid,"clock_errors":clock_errors,"latency_assessed":latency_assessed,
      "platform_result":("VALID_PLATFORM_FAILURE_OR_BACKLOG" if platform_failure else "VALID_PLATFORM_SUCCESS") if valid_measurement and capture_valid else "NOT_ASSESSED_MEASUREMENT_INVALID",
      "resource_evidence":resource,"saturation_flag":saturation,"generator_resource_qualified":resource_qualified}

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for name in ("run","sink","window","clock-samples","host-evidence","out"): p.add_argument("--"+name,required=True)
    p.add_argument("--history",help="history NDJSON exported while the platform was up; required unless --history-status is not OK")
    p.add_argument("--history-status",default="OK",help="OK, EXPORT_FAILED, or another recorded non-OK status")
    p.add_argument("--resource-evidence",help="generator-diagnostics.jsonl from generator-diagnostic-sampler.py")
    p.add_argument("--sampler-status",help="paired .status.json file from generator-diagnostic-sampler.py")
    p.add_argument("--gen-cpu",help="gen-cpu.ndjson (defaults to the run directory copy)")
    a=p.parse_args()
    if a.history_status=="OK" and not a.history:
        p.error("--history is required when --history-status is OK")
    if a.history and a.history_status!="OK":
        p.error("--history cannot be combined with a non-OK --history-status")
    r=analyze(a.run,a.sink,a.window,a.history,a.clock_samples,a.host_evidence,a.resource_evidence,a.sampler_status,a.gen_cpu,a.history_status)
    out=Path(a.out); out.write_text(json.dumps(r,indent=2)+"\n")
    table=["# Dispatch-integrity assessment: "+r["run_id"],"",f"- Policy: `{r['policy']}`",f"- Measurement/capture valid: `{r['measurement_valid']}` / `{r['capture_valid']}`",f"- Clock-qualified latency: `{r['latency_assessed']}`",f"- Platform result: **{r['platform_result']}**",f"- Generator resource qualification: `{r['generator_resource_qualified']}`","", "| Phase | Scheduled | Received by cutoff | Missing | Dispatch p99 ms | Scheduled p99 ms | History missing |", "|---|---:|---:|---:|---:|---:|---:|"]
    for x in r["phases"]:
        table.append(f"| {x['phase']} | {x['scheduled']} | {x['received_by_cutoff']} | {x['missing_by_cutoff']} | {x['conditional_dispatch_to_receipt_ms']['p99']} | {x['conditional_scheduled_to_receipt_ms']['p99']} | {x['history_missing']} |")
    table.extend(["", "## Workload integrity", "", "```json",json.dumps(r["workload_integrity"],indent=2),"```","", "## Measurement and clock diagnostics", "", "```json",json.dumps({"errors":r["measurement_errors"],"clock_errors":r["clock_errors"],"sink":r["sink"]},indent=2),"```","", "Full machine-readable record: `"+out.name+"`."])
    out.with_suffix(".md").write_text("\n".join(table)+"\n")
    import csv
    with out.with_suffix(".csv").open("w",newline="") as f:
        w=csv.DictWriter(f,fieldnames=["run_id","phase","scheduled","received_by_cutoff","missing_by_cutoff","dispatch_p50_ms","dispatch_p95_ms","dispatch_p99_ms","scheduled_p99_ms","history_missing","history_duplicate_extra"]); w.writeheader()
        for x in r["phases"]: w.writerow({"run_id":r["run_id"],"phase":x["phase"],"scheduled":x["scheduled"],"received_by_cutoff":x["received_by_cutoff"],"missing_by_cutoff":x["missing_by_cutoff"],"dispatch_p50_ms":x["conditional_dispatch_to_receipt_ms"]["p50"],"dispatch_p95_ms":x["conditional_dispatch_to_receipt_ms"]["p95"],"dispatch_p99_ms":x["conditional_dispatch_to_receipt_ms"]["p99"],"scheduled_p99_ms":x["conditional_scheduled_to_receipt_ms"]["p99"],"history_missing":x["history_missing"],"history_duplicate_extra":x["history_duplicate_extra"]})
    print(out)
    if not r["measurement_valid"]: raise SystemExit(1)
if __name__=="__main__": main()
