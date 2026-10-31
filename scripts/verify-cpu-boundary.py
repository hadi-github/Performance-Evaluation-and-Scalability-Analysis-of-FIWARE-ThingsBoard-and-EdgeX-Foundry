#!/usr/bin/env python3
"""Independent microsecond-counter arithmetic and inventory coverage check for a run."""
import argparse
import json
import math
import pathlib

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('run')
p.add_argument('--inventory', required=True)
a = p.parse_args()
root = pathlib.Path(a.run)
expected = {r['name'] for r in json.loads(pathlib.Path(a.inventory).read_text())}
assert expected, 'empty expected inventory'
reports = json.loads((root/'cpu-boundary.json').read_text())
assert isinstance(reports, list), 'CPU capture unavailable'
for phase in reports:
    assert phase['status'] == 'CAPTURED', phase
    index = phase['step']
    def read(side):
        return {f[1]: f for line in (root/f'cpu-step-{index:02d}-{side}.tsv').read_text().splitlines() if (f := line.split('\t'))}
    before, after = read('before'), read('after')
    assert set(before) == expected == set(after), {'expected': sorted(expected), 'before': sorted(before), 'after': sorted(after)}
    totals = {}
    for name in expected:
        b, e = before[name], after[name]
        assert b[2] == e[2] and b[4] == e[4], 'container replaced/restarted: '+name
        assert b[3] != 'NA' and e[3] != 'NA', 'counter unavailable: '+name
        delta = int(e[3])-int(b[3])
        assert delta >= 0, 'counter reset: '+name
        tier = b[7] if len(b)>7 else 'unclassified'
        totals[tier] = totals.get(tier, 0) + delta
    report = phase['report']
    assert report['counters_ok'], report
    assert math.isclose(sum(totals.values())/1e6, report['total_cpu_seconds'], rel_tol=1e-12)
    for tier, value in totals.items():
        assert math.isclose(value/1e6, report['cpu_seconds_by_tier'][tier], rel_tol=1e-12)
    print(json.dumps(dict(step=index, status='VERIFIED_CAPTURE_INTERVAL', cpu_seconds_by_tier={k:v/1e6 for k,v in totals.items()}, total_cpu_seconds=sum(totals.values())/1e6,
                         phase_specific_cost='UNAVAILABLE: enclosing capture interval is not an exact phase interval', before_skew_seconds=phase['before_skew_seconds'], after_skew_seconds=phase['after_skew_seconds'])))
