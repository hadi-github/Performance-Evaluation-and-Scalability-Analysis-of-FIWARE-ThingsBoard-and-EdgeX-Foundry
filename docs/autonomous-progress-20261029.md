# Autonomous progress (auto-20261029)

Started 2026-10-28T22:41Z (workstation local). 5 h budget. Authorization + interpretation correction:
`docs/interpretation-correction-20261029.md`, `docs/autonomous-state-20261029.json`.

## Task log

### Task 0 — correction + records — VERIFIED
`docs/interpretation-correction-20261029.md` written and applied to the 20261028 handoff + blocker B10
(50/s valid; 100/s = VU starvation not platform FAIL; increased-VU result separate (zero loss, 1.1%
dropped); no platform-capacity bracket; 1280 MiB = promising candidate; clocks unverified). Manifests
re-verified: 758 + 26 + 17.

### Task A — generator-only + receiver qualification — VERIFIED (bracket + confirm)
k6 on the load host (3-core cpuset, Prometheus remote-write, the real arrival machinery via a new
`workloads/sink.js` reusing `lib/common.js`) → the sink on loopback. Steps (60 s each + 15 s warm-up):

| rate | achieved | dropped | p95 / p99 (ms) | sink received delta |
|---|---|---|---|---|
| 200 | 200.017 | 0 | 0.75 / 0.91 | 15,001 (all) |
| 400 | 400.017 | 0 | 0.70 / 0.93 | 30,002 (all) |
| 800 | 800.017 | 0 | 0.76 / 1.44 | 48,001 (all) |
| 1600 | 1600.017 | 0 | 1.02 / 3.68 | 120,002 (all) |
| **3200** | **3200.017** | **0** | 10.9 / 82.0 | 240,001 (all) |
| 6400 | 6,264.4 | 13,824 | 226 / 459 | 466,179 (all; 0 unparsed/overflow) |
| **3200 confirm** | **3200.017** | **0** | 16.1 / 93.3 | 240,002 (all) |

**Generator-only ceiling (loopback): 3200 r/s sustainable (two consecutive VALID passes), 6400 INVALID**
(achieved 97.9%, p99 459 ms — loopback stack saturation; VUs were available (3,237 of 12,800), the drop is
scheduling under CPU saturation). **Recommended maximum campaign rate ≤ 1600 r/s** (= ½ × 3200, the adopted
S3/D34/D37 rule), subject to the cross-host path and platform qualification — the loopback result excludes
both. The sink is now qualified AT 3200 r/s through the real generator path (240k messages, zero
unparsed/overflow) — stronger than yesterday's python-driver check. k6 CPU series absent from cAdvisor
(instrumentation gap recorded; the drop-free 3200 runs are the headroom evidence).

### Task B — 1280 MiB TB qualification under cross-host load — VERIFIED
TB recreated at the owner's candidate limits (core/rule-engine 1280 MiB, heap unchanged; k=2; registry
2,213 + per-run bench devices). Monitor v2 on the SUT throughout. Runs (all cross-host via run.sh):

| rate | MAX_VUS | verdict | achieved | dropped | p95 / p99 (ms) | note |
|---|---|---|---|---|---|---|
| 50 | 200 | INVALID | 50 | 30 | — | VU boundary (yesterday's 300 was VALID at 1024) |
| 50 | 400 | **VALID** | 50.00 | 0 | 66.9 / 126.5 | stored==sent 4,500 |
| 100 | 800 | **VALID** | 100.017 | 0 | 151.2 / 259.1 | stored==sent 9,001 — **yesterday's 2 s tails were VU-queuing artifacts, not platform latency** |
| 200 | 1200 | INVALID | 200.017 | 8 (0.07%) | 291.9 / 415.2 | marginal VU availability during spikes |
| 200 | 2400 | **VALID** | 200.017 | 0 | — / 330.0 | stored==sent |
| 400 | 3600 | INVALID | 316.4 | 6,465 | **10,000 / 10,010** | **saturation: p95 = the 10 s HTTP timeout** |
| 200 confirm | 2400 | **VALID** | 200.017 | 0 | — / 1,469.8 | second pass; p99 variability at this rate is real |

**Platform qualification at 1280 MiB, k=2: 200 r/s confirmed VALID (two passes); 400 r/s INVALID
(saturation).** Memory under load: cores 76.6–78.8% of 1280, rule-engines 70–71%; OOMKilled=false,
RestartCount=0 throughout. The binding constraint above 200 r/s is platform CPU/latency (2 cores for 4 app
containers), not memory — consistent with the pilot's conclusion that 1280 MiB removes the memory stop
condition. The VU-sizing lesson is recorded: size VUs from observed p99 × rate × margin; yesterday's
"2 s tails" were largely VU-queuing artifacts.

### Task C — memory-budget table — VERIFIED
`results/auto-20261029/tb-memory-budget.md`: k=1..4 at 1280 MiB (3,072/replica): k=1 6,912 ✓, k=2 9,984 ✓,
k=3 13,056 ✓ (+1,792), **k=4 16,128 ✗ (overflow exactly 1,280 MiB)**. Conflict driver: the raised per-replica
limits cost 4×512 = 2,048 MiB at k=4 vs the 768 MiB headroom the old limits left. Five concrete owner
options recorded (O1 keep 1024@k4 / O2 transport cut (insufficient alone) / O3 infra cut (fingerprint
deviation) / **O4 cap at k=3 per the adopted S4 fallback** / O5 larger host). Nothing adopted.

### Task D — FIWARE history tier under load — VERIFIED
Candidate up at the ADOPTED limits (timescale 1536 / QL 512 / mongo 2048 / orion 512). Labelled load:
3,001/3,001 updates through Orion→QL at 50.02/s (co-located generator, recorded deviation — the segment
under test is the internal history path). **Reconciliation: rows=3,001, distinct=3,001, missing=[], dup_seqs={},
unrecoverable=0** (the extra seq 3000 was sent by the loop and stored — exact). Memory under load:
timescale 136.8 MiB / 1536 (8.9%), QL 77.1 MiB / 512 (15.1%). The adopted allocation is validated at this
rate; higher-rate adequacy remains open.

### Task E — EdgeX non-empty history test — VERIFIED
EdgeX stack up (volumes preserved), `provision-then-scale.sh prepare 1 20 edgexhist-20261029`: gate 20/20
devices, health gate PASS. 600 labelled readings via the gate's own contract
(`/api/v3/resource/<device>/reading`, JSON run_id/seq) at 20 r/s: sent=600, fails=0. **Export + reconcile:
rows=600, distinct=600, missing=[], dup_seqs={}** — the identity export is proven on real data, not just
query execution. Stack stopped afterwards (volumes kept).

### Task F — corrected clock probe — VERIFIED (bound obtained; claims stay unqualified)
`results/auto-20261029/clock/clock-probe-results.md`: interval-intersection over persistent ControlMaster,
35 pairs/leg. workstation↔SUT [1334, 1388] ms; workstation↔load [1325, 1384] ms; **inferred load↔SUT offset
∈ [−63, +50] ms** (|offset| ≤ 63 ms at this method's uncertainty; both hosts NTP stratum-2, self-reports
captured). The workstation's own clock is ~1.36 s off the hosts (hotspot) — never a timing reference.
Cross-host claims remain unqualified: U=63 ms would need P ≥ 630 ms under the (still-proposed) U ≤ P/10
rule, and no SLO exists. Narrowing U needs a direct SUT→load probe (key path not authorized to create).

### Access notes
The load host remained reachable only via the jump route throughout; two brief SSH dropouts early in the
session (retried successfully). All live work completed.
