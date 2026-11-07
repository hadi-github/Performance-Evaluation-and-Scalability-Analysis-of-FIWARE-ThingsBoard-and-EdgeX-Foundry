#!/usr/bin/env python3
"""Evaluate explicit dispatch-integrity-v1 run ledgers without rewriting legacy verdicts."""
import argparse, json, math
from pathlib import Path

POLICY = "dispatch-integrity-v1"

def evaluate(run):
    run = Path(run)
    plan = json.loads((run / "resolved.json").read_text())
    summary = json.loads((run / "k6-summary.json").read_text())
    if plan.get("health", {}).get("workload_policy") != POLICY:
        raise ValueError("resolved plan is not explicitly bound to dispatch-integrity-v1")
    if plan.get("arrivals", {}).get("model") != "constant" or plan.get("test_faults", {}).get("duplicate_seqs") or plan.get("test_faults", {}).get("early_every"):
        raise ValueError("dispatch-integrity-v1 requires constant arrivals and no injected faults")
    if plan.get("vus", {}).get("pre_allocated") != 150 or plan.get("vus", {}).get("max") != 150:
        raise ValueError("dispatch-integrity-v1 frozen VU allocation is 150/150")
    steps = {s["index"]: s for s in plan["steps"]}
    sums = {s["index"]: s for s in summary["steps"]}
    rows = [json.loads(x) for x in (run / "records.ndjson").read_text().splitlines() if x.strip()]
    expected = {s["seq_offset"] + tick: (s, tick) for s in plan["steps"] for tick in range(s["ticks"])}
    byseq = {}
    for row in rows:
        seq = row.get("seq")
        if not isinstance(seq, int) or seq in byseq or seq not in expected:
            raise ValueError("duplicate or unexpected dispatch identity")
        step, tick = expected[seq]
        if row.get("rec") != "msg" or row.get("run_id") != plan["run_id"] or row.get("rate_step") != step["index"] or row.get("tick") != tick or row.get("phase") != step["phase"]:
            raise ValueError(f"dispatch identity mismatch at seq {seq}")
        if row.get("device_id") != seq % int(plan["devices"]): raise ValueError(f"device identity mismatch at seq {seq}")
        if row.get("outcome") not in {"ack_success","explicit_failure","unknown"}: raise ValueError(f"invalid outcome at seq {seq}")
        for field in ("scheduled_send_ms", "actual_send_ms", "response_ms"):
            if isinstance(row.get(field), bool) or not isinstance(row.get(field), (int,float)) or not math.isfinite(row[field]):
                raise ValueError(f"seq {seq}: missing/nonfinite {field}")
        byseq[seq] = row
    if set(byseq) != set(expected):
        raise ValueError(f"complete dispatch ledger required: {len(byseq)}/{len(expected)}")
    findings=[]; phases=[]
    for st in plan["steps"]:
        ss=sums.get(st["index"])
        if not ss: raise ValueError("summary step missing")
        selected=[byseq[st["seq_offset"]+i] for i in range(st["ticks"])]
        rate=float(st["tick_rate"]); period=1000/rate
        start=float(ss["scenario_start_ms"]); end=start+int(st["duration_s"])*1000
        epochs=[float(x["scenario_start_ms"])-float(s["start_s"])*1000 for s,x in zip(plan["steps"],summary["steps"])]
        if max(epochs)-min(epochs)>5: raise ValueError("scenario start metadata differs by more than 5 ms")
        for tick,row in enumerate([byseq[st["seq_offset"]+i] for i in range(st["ticks"]) ]):
            scheduled=start+tick*1000/rate
            if abs(row["scheduled_send_ms"]-scheduled)>.0011: raise ValueError(f"scheduled identity mismatch at seq {st['seq_offset']+tick}")
            if row["response_ms"]<row["actual_send_ms"]: raise ValueError("response precedes dispatch")
            early=row["actual_send_ms"]-scheduled < -float(plan["health"].get("early_tolerance_ms",0))
            if row.get("early") is not early: raise ValueError(f"early-send marker mismatch at seq {st['seq_offset']+tick}")
        for outcome,key in (("ack_success","ack_success"),("explicit_failure","explicit_failure"),("unknown","unknown")):
            if sum(r["outcome"]==outcome for r in selected)!=int(ss.get(key,-1)): raise ValueError(f"summary outcome mismatch at step {st['index']} / {outcome}")
        if sum(int(ss.get(k,0)) for k in ("ack_success","explicit_failure","unknown"))!=int(ss.get("attempted",-1)):
            raise ValueError(f"summary outcome accounting mismatch at step {st['index']}")
        # Physical bins include every phase's dispatches; cohorts are reported separately.
        physical=[r for r in rows if start <= r["actual_send_ms"] < end]
        one_s=[]; hundred=[]
        edge=start
        while edge+1000<=end:
            one_s.append(sum(edge<=r["actual_send_ms"]<edge+1000 for r in physical)); edge+=1000
        edge=start
        while edge+100<=end:
            hundred.append(sum(edge<=r["actual_send_ms"]<edge+100 for r in physical)); edge+=100
        if rate not in (25,50): raise ValueError("dispatch-integrity-v1 supports only frozen 25/s and 50/s rates")
        lo,hi= (48,52) if rate==50 else (23,27)
        max100=11 if rate==50 else 7
        ordered=sorted(physical,key=lambda r:r["actual_send_ms"])
        edge_gaps=[]
        if ordered:
            edge_gaps=[ordered[0]["actual_send_ms"]-start,end-ordered[-1]["actual_send_ms"]]
            edge_gaps += [b["actual_send_ms"]-a["actual_send_ms"] for a,b in zip(ordered,ordered[1:])]
        lags=[r["actual_send_ms"]-r["scheduled_send_ms"] for r in selected]
        n=len(lags); chunk=max(1,math.ceil(n*.1)); first=sorted(lags[:chunk]); last=sorted(lags[-chunk:])
        median=lambda v: (v[(len(v)-1)//2]+v[len(v)//2])/2
        growth=median(last)-median(first) if n else None
        checks={"all_planned_records":len(selected)==st["ticks"],
          "achieved_rate_ge_99":st["ticks"]>0 and len(physical)/(int(st["duration_s"])*rate)>=.99,
          "zero_drops":ss.get("dropped_iterations")==0,
          "1s_bins":all(lo<=x<=hi for x in one_s),"100ms_bins":all(x<=max100 for x in hundred),
          "max_gap_including_edges":max(edge_gaps,default=0)<=5*period,
          "decile_lag_growth":growth is not None and growth<=period,
          "no_early_sends":all(lag>=-float(plan["health"].get("early_tolerance_ms",0)) for lag in lags)}
        findings.extend([k for k,v in checks.items() if not v])
        phases.append({"index":st["index"],"phase":st["phase"],"physical_dispatches":len(physical),"physical_rate":len(physical)/(int(st["duration_s"])),
          "cohort_dispatches":len(selected),"dispatch_spill":len(selected)-sum(start<=r["actual_send_ms"]<end for r in selected),
          "bins_1s":one_s,"bins_100ms":hundred,"max_gap_ms":max(edge_gaps,default=None),"lag_growth_ms":growth,
          "send_lag_ms":{"p50":median(sorted(lags)) if lags else None,"p99":sorted(lags)[math.ceil(.99*n)-1] if n else None,"max":max(lags) if lags else None},"checks":checks})
    return {"policy":POLICY,"run_id":plan["run_id"],"valid":not findings,"findings":findings,"phases":phases,
      "resource_qualification":"not assessed by dispatch ledger; requires retained CPU/memory/swap/OOM/sampler evidence"}

def main():
    p=argparse.ArgumentParser(description=__doc__); p.add_argument("--run",required=True); p.add_argument("--out",required=True); a=p.parse_args()
    result=evaluate(a.run); Path(a.out).write_text(json.dumps(result,indent=2)+"\n"); print(a.out)
    if not result["valid"]: raise SystemExit(1)
if __name__=="__main__": main()
