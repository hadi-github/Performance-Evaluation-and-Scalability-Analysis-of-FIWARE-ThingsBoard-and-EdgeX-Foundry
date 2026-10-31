#!/usr/bin/env python3
"""Identity and device-rotation reconciliation for controlled platform-adapter tests."""
import argparse
import collections
import json
import pathlib

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--runs', required=True)
p.add_argument('--receiver', action='append', required=True)
a = p.parse_args()
rows = collections.defaultdict(list)
for path in a.receiver:
    for line in pathlib.Path(path).read_text().splitlines():
        r = json.loads(line)
        if r['seq'] >= 0:
            rows[r['run_id']].append(r)
results = {}
for d in sorted(pathlib.Path(a.runs).iterdir()):
    if not (d/'manifest.json').exists():
        continue
    m = json.loads((d/'manifest.json').read_text())
    plan = json.loads((d/'resolved.json').read_text())
    summary = json.loads((d/'k6-summary.json').read_text())
    expected = {seq for step in plan['steps'] for seq in range(step['seq_offset'], step['seq_offset']+step['ticks'])}
    seen = collections.Counter(r['seq'] for r in rows[d.name])
    devices = {r['device_id'] for r in rows[d.name]}
    wrong_device = sum(r['device_id'] != f"bench-{d.name}-{r['seq'] % plan['devices']:05d}" for r in rows[d.name])
    drops = sum(s['dropped_iterations'] for s in summary['steps'])
    result = dict(generator_verdict=m['verdict'], adapter=plan['adapter'], logical_devices=plan['devices'],
                  observed_devices=len(devices), received=len(rows[d.name]), unique=len(seen),
                  missing=len(expected-set(seen)) if drops==0 else None, unexpected=len(set(seen)-expected),
                  duplicate_extra_copies=sum(v-1 for v in seen.values()), wrong_device=wrong_device,
                  dropped_iterations=drops, early_sends=sum(s['early'] for s in summary['steps']),
                  complete_outcomes=all(s['attempted']==s['ack_success']+s['explicit_failure']+s['unknown'] for s in summary['steps']),
                  script_hash=m['k6_scripts_sha256'], source=m['git'])
    if m['verdict']=='VALID':
        assert not any(result[k] for k in ('missing','unexpected','duplicate_extra_copies','wrong_device','dropped_iterations','early_sends')), result
        assert result['complete_outcomes'] and len(devices)==plan['devices'], result
        assert len(rows[d.name])==sum(s['attempted'] for s in summary['steps']), result
    results[d.name]=result
print(json.dumps(results, indent=2))
