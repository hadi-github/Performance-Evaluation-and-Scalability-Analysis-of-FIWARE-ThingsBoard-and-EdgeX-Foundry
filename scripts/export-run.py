#!/usr/bin/env python3
"""Export a fixed UTC run window from Prometheus; no Grafana renderer required."""
import argparse,csv,datetime as dt,hashlib,html,json,math,pathlib,re,urllib.parse,urllib.request

def timestamp(s):
    d=dt.datetime.fromisoformat(s.replace('Z','+00:00'))
    if d.tzinfo is None: raise ValueError('timestamps must include timezone')
    return d.timestamp()

def main():
    p=argparse.ArgumentParser(description=__doc__)
    for k in ('run-id','platform','start','end','out'): p.add_argument('--'+k,required=True)
    p.add_argument('--prometheus',default='http://127.0.0.1:9090')
    p.add_argument('--grafana',default='http://127.0.0.1:3000')
    p.add_argument('--step',type=int,default=5)
    p.add_argument('--context',type=int,default=60)
    p.add_argument('--responses-dir',help='Render offline using previously exported raw query-response JSON files (same dashboard and UTC window)')
    p.add_argument('--dashboard',default=str(pathlib.Path(__file__).resolve().parents[1]/'monitoring/grafana/dashboards/iot-run-review.json'))
    a=p.parse_args()
    if not re.fullmatch(r'[A-Za-z0-9_.:-]+',a.run_id):p.error('invalid run id')
    if a.platform not in ('thingsboard','fiware','edgex'):p.error('invalid platform')
    start,end=timestamp(a.start),timestamp(a.end)
    if end<=start or a.step<1 or a.context<0:p.error('invalid window/step/context')
    if (end-start+2*a.context)/a.step>10000:p.error('over 10000 points: increase --step')
    prior=None
    if a.responses_dir:
        prior=json.loads((pathlib.Path(a.responses_dir)/'export.json').read_text())
        for k,v in {'run_id':a.run_id,'platform':a.platform,'measurement_start':a.start,'measurement_end':a.end,'step_seconds':a.step,'context_seconds':a.context}.items():
            if prior.get(k)!=v:p.error('offline response metadata mismatch: '+k)
    out=pathlib.Path(a.out)
    try:
        import matplotlib
        matplotlib.use('Agg')
        import matplotlib.pyplot as plt
        import matplotlib.dates as mdates
    except ImportError:
        raise SystemExit('chart export needs matplotlib in a pinned environment; '
                         'offline dispatch analysis does not (see PLAN P2.2)')
    out.mkdir(parents=True,exist_ok=False)
    dashboard=json.loads(pathlib.Path(a.dashboard).read_text())
    report={'run_id':a.run_id,'platform':a.platform,'measurement_start':a.start,'measurement_end':a.end,'step_seconds':a.step,'context_seconds':a.context,'campaign_verdict':'NOT_EVALUATED','query_start_epoch_seconds':start-a.context,'query_end_epoch_seconds':end+a.context,'prometheus':a.prometheus,'queries':[]}
    report['grafana_url']=a.grafana.rstrip('/')+'/d/iot-run-review?'+urllib.parse.urlencode({'var-run_id':a.run_id,'var-platform':a.platform,'from':int((start-a.context)*1000),'to':int((end+a.context)*1000)})
    rows=[];images=[]
    for panel in dashboard['panels']:
        if not panel.get('targets'):continue
        fig,ax=plt.subplots(figsize=(12,4.5));count=0;has_data=False;errors=[]
        for i,target in enumerate(panel['targets']):
            expr=target['expr'].replace('$platform',a.platform).replace('$run_id',json.dumps(re.escape(a.run_id))[1:-1]).replace('$service','.*').replace('$__rate_interval','30s')
            key=f"{panel['id']:02d}-{i}"
            item={'id':key,'title':panel['title'],'expression':expr,'unit':panel['fieldConfig']['defaults'].get('unit','short')}
            try:
                params=urllib.parse.urlencode({'query':expr,'start':start-a.context,'end':end+a.context,'step':a.step})
                if a.responses_dir:
                    previous=next(q for q in prior['queries'] if q['id']==key)
                    if previous['expression']!=expr:raise ValueError('offline query expression mismatch')
                    data=json.loads((pathlib.Path(a.responses_dir)/(key+'.json')).read_text())
                else:
                    with urllib.request.urlopen(a.prometheus.rstrip('/')+'/api/v1/query_range?'+params,timeout=45) as r:data=json.load(r)
                (out/(key+'.json')).write_text(json.dumps(data,indent=2))
                if data.get('status')!='success':raise ValueError(data.get('error','query failed'))
                if data['data']['resultType']!='matrix':raise ValueError('expected matrix')
                (out/(key+'.json')).write_text(json.dumps(data,indent=2))
                series=data['data']['result'];item['status']='OK' if any(math.isfinite(float(v)) for s in series for _,v in s['values']) else 'NO_DATA';item['warnings']=data.get('warnings',[])
                has_data=has_data or item['status']=='OK'
                for si,s in enumerate(series):
                    label=json.dumps(s['metric'],sort_keys=True);points={round(float(t),3):float(v) for t,v in s['values']}
                    xs=[];ys=[]
                    for j in range(int((end-start+2*a.context)//a.step)+1):
                        t=round(start-a.context+j*a.step,3);v=points.get(t,float('nan'))
                        xs.append(dt.datetime.fromtimestamp(t,dt.timezone.utc));ys.append(v)
                        rows.append([key,label,t,dt.datetime.fromtimestamp(t,dt.timezone.utc).isoformat(),v,'measurement' if start<=t<=end else 'context'])
                    short=', '.join(str(s['metric'][k]) for k in ('container','name','service','step','scenario','instance','job') if k in s['metric'])
                    legend=re.sub(r'\{\{\s*(\w+)\s*\}\}',lambda m:str(s['metric'].get(m.group(1),'?')),target.get('legendFormat',short or key))
                    ax.plot(xs,ys,lw=1.3,label=f'{key}:{si} '+legend[:110]);count+=1
            except Exception as e:
                item.update(status='ERROR',error=str(e));errors.append(key)
            report['queries'].append(item)
        ax.axvspan(dt.datetime.fromtimestamp(start,dt.timezone.utc),dt.datetime.fromtimestamp(end,dt.timezone.utc),alpha=.08,color='green',label='Measurement window')
        unit=panel['fieldConfig']['defaults'].get('unit','value')
        if unit=='short' and 'messages/s' in panel['title']:unit='messages/s'
        if unit=='short' and '(cores)' in panel['title']:unit='CPU cores'
        ax.set(title=panel['title'],ylabel=unit,xlabel='UTC; shaded = measurement window')
        ax.xaxis.set_major_formatter(mdates.DateFormatter('%H:%M:%S',tz=dt.timezone.utc));ax.grid(alpha=.2)
        if not has_data:ax.text(.5,.5,'NO DATA / QUERY ERROR — not zero',transform=ax.transAxes,ha='center')
        if errors:ax.text(.99,.99,'QUERY ERROR: '+', '.join(errors),transform=ax.transAxes,ha='right',va='top',color='red')
        if count<=8:ax.legend(fontsize=7,loc='upper left')
        else:ax.text(0,-.22,f'{count} series; exact labels and values in measurements.csv',transform=ax.transAxes,fontsize=8)
        fig.tight_layout();name=f"panel-{panel['id']:02d}"
        for ext in ('png','svg'):fig.savefig(out/(name+'.'+ext),dpi=160)
        plt.close(fig);images.append(name+'.png')
    with (out/'measurements.csv').open('w') as f:
        w=csv.writer(f);w.writerow(['query_id','labels_json','unix_seconds','utc','value','phase']);w.writerows(rows)
    (out/'export.json').write_text(json.dumps(report,indent=2))
    (out/'index.html').write_text('<meta charset="utf-8"><title>Run review</title><style>body{font:16px sans-serif;max-width:1200px;margin:32px auto;background:#f4f7fb}img{width:100%;background:white;margin:12px 0}pre{white-space:pre-wrap}</style><h1>'+html.escape(a.run_id)+'</h1><p>Diagnostic plots; not a service-SLO verdict. Missing samples stay missing. Windowed percentiles must not be averaged.</p><a href="'+html.escape(report['grafana_url'],quote=True)+'">Open fixed Grafana window</a>'+''.join('<img src="'+n+'">' for n in images)+'<h2>Query coverage</h2><pre>'+html.escape(json.dumps(report,indent=2))+'</pre>')
    (out/'SHA256SUMS').write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest()+'  '+f.name+'\n' for f in sorted(out.iterdir()) if f.is_file()))
    print(out/'index.html')
    return 2 if any(q['status']=='ERROR' for q in report['queries']) else 0
if __name__=='__main__':raise SystemExit(main())
