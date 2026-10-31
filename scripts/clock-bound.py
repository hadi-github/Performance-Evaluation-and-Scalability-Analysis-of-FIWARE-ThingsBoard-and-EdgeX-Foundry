#!/usr/bin/env python3
"""Bound observed SUT minus load-host realtime offset without using a workstation clock."""
import argparse
import json
import time
import urllib.request

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--url', required=True)
p.add_argument('--out', required=True)
p.add_argument('--samples', type=int, default=30)
a = p.parse_args()
samples = []
for _ in range(a.samples):
    before = time.time_ns()
    mono = time.monotonic_ns()
    with urllib.request.urlopen(a.url, timeout=5) as response:
        remote = int(json.load(response)['epoch_ns'])
    after = time.time_ns()
    elapsed = time.monotonic_ns() - mono
    samples.append(dict(load_before_ns=str(before), sut_ns=str(remote), load_after_ns=str(after),
                        elapsed_monotonic_ns=elapsed, offset_lower_ms=(remote-after)/1e6,
                        offset_upper_ms=(remote-before)/1e6))
    time.sleep(.1)
result = dict(clock_domains='load CLOCK_REALTIME vs SUT CLOCK_REALTIME; monotonic used only for RTT',
              samples=samples, observed_envelope_ms=[min(s['offset_lower_ms'] for s in samples), max(s['offset_upper_ms'] for s in samples)],
              qualification='Observed bounds only; no SLO timing threshold adopted; unsampled clock excursions are not excluded.')
with open(a.out, 'x') as f:
    json.dump(result, f, indent=2)
print(json.dumps({'observed_envelope_ms': result['observed_envelope_ms']}))
