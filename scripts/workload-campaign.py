#!/usr/bin/env python3
"""Validate and report staged campaign evidence offline. Never executes runs."""
import argparse, hashlib, json, sys
from pathlib import Path

def sha(p): return hashlib.sha256(Path(p).read_bytes()).hexdigest()

def validate(matrix_path, scope_path, plan_root, records=(), b0_matrix=None, b0_scope=None, b0_plan_root=None):
    mp,sp,root=Path(matrix_path),Path(scope_path),Path(plan_root)
    m,s=json.loads(mp.read_text()),json.loads(sp.read_text())
    if m.get('status')!='PROPOSED_NOT_LAUNCH_CLEARED' or s.get('execution_authorized') is not False: raise ValueError('proposal is not execution-cleared')
    if m.get('scope_sha256')!=hashlib.sha256((json.dumps(s,indent=2)+'\n').encode()).hexdigest(): raise ValueError('scope hash mismatch')
    ids={}; groups={}
    def add(row,wid,kind,plans_root):
        rid=row.get('run_id')
        if not rid or rid in ids: raise ValueError('missing or duplicate matrix run ID: '+str(rid))
        p=(plans_root/row['plan']).resolve()
        if not p.is_relative_to(plans_root.resolve()): raise ValueError('plan escapes plan root')
        digest=sha(p)
        if row.get('plan_sha256',digest)!=digest: raise ValueError('plan hash mismatch: '+rid)
        if row.get('platform') not in ('thingsboard','fiware','edgex') or row.get('repetition') not in range(1,6): raise ValueError('invalid platform/repetition: '+rid)
        if wid!='B0':
            variant=s.get('variants',{}).get(wid)
            if not variant or row.get('execution_status')!='BLOCKED_PENDING_VARIANT_RELEASE': raise ValueError('not a staged variant: '+rid)
            if json.loads(p.read_text()).get('devices')!=variant['devices']: raise ValueError('source/config binding mismatch: '+rid)
            d=row.get('deployment',{})
            if d.get('replicas_per_replicated_service')!=variant['replicas_per_replicated_service'] or d.get('application_cpu_budget_k')!=2: raise ValueError('deployment intent mismatch: '+rid)
        ids[rid]={'workload_id':wid,'platform':row['platform'],'repetition':row['repetition'],'plan_sha256':digest,'run_kind':kind}
        g=groups.setdefault((wid,row['platform']),{'planned':[],'observed':[]}); g['planned'].append(rid)
    rows=m.get('experimental_runs')
    if not isinstance(rows,list) or not rows: raise ValueError('empty matrix')
    for row in rows: add(row,row.get('workload_id'),'variant-experimental',root)
    if bool(b0_matrix)!=bool(b0_scope): raise ValueError('B0 matrix and scope must be supplied together')
    if b0_matrix:
        bm,bs=Path(b0_matrix),Path(b0_scope)
        if json.loads(bm.read_text()).get('frozen_scope_sha256')!=sha(bs): raise ValueError('B0 frozen scope hash mismatch')
        # Frozen B0 plans are rooted beside its matrix; variant plans stay rooted
        # at the separately staged W plan directory.
        bp=Path(b0_plan_root) if b0_plan_root else bm.parent
        for row in json.loads(bm.read_text()).get('ordinary_runs',[]): add(row,'B0','ordinary',bp)
    seen=set(); isolated=[]
    for item in records:
        rec=json.loads(Path(item).read_text()) if isinstance(item,(str,Path)) else item
        rid=rec.get('run_id')
        if rid in seen: raise ValueError('duplicate observed run ID: '+str(rid))
        seen.add(rid)
        if rec.get('run_kind') in ('qualification','diagnostic','short-verification'):
            isolated.append({'run_id':rid,'run_kind':rec['run_kind'],'counted':False}); continue
        want=ids.get(rid)
        if not want: raise ValueError('unplanned run ID: '+str(rid))
        for key,value in want.items():
            if rec.get(key)!=value: raise ValueError(f'{key} mismatch for {rid}')
        groups[(want['workload_id'],want['platform'])]['observed'].append({
            'run_id':rid,'status':rec.get('status',rec.get('phase')),
            'latency_count':rec.get('latency_count',rec.get('latency_assessed_count')),
            'scheduled':rec.get('scheduled'),'received':rec.get('received_by_cutoff'),
            'missing':rec.get('missing_by_cutoff')})
    intent={w:{'replicas_per_replicated_service':v['replicas_per_replicated_service'],'application_cpu_budget_k':2,'mutated':False} for w,v in s.get('variants',{}).items()}
    return {'status':'OFFLINE_VALIDATION_ONLY_NOT_EXECUTION_RELEASE','groups':[{'workload_id':w,'platform':p,**g} for (w,p),g in sorted(groups.items())], 'isolated_records':isolated,'deployment_intent':intent,'execution_authorized':False}

def main():
    ap=argparse.ArgumentParser(description=__doc__); ap.add_argument('--matrix',type=Path,required=True); ap.add_argument('--scope',type=Path,required=True); ap.add_argument('--plan-root',type=Path,required=True); ap.add_argument('--record',type=Path,action='append',default=[]); ap.add_argument('--b0-matrix',type=Path); ap.add_argument('--b0-scope',type=Path); ap.add_argument('--b0-plan-root',type=Path,help='defaults to the directory containing --b0-matrix'); a=ap.parse_args()
    try: print(json.dumps(validate(a.matrix,a.scope,a.plan_root,a.record,a.b0_matrix,a.b0_scope,a.b0_plan_root),indent=2)); return 0
    except (ValueError,OSError,KeyError,json.JSONDecodeError) as e: print('REJECTED: '+str(e),file=sys.stderr); return 2
if __name__=='__main__': raise SystemExit(main())
