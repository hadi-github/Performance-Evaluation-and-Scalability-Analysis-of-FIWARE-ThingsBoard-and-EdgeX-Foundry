# Finding: the 150-VU pool is the W1 headroom risk, and the qualification is the only way to test it

> **RESOLVED 2026-11-10.** The risk was real, was tested, and was correct: the pool
> bound on both platforms. It was then **fixed** rather than accepted, because the
> cause was our own generator constant, not either platform. The pool is now
> rate-derived (`scripts/pool_policy.py`): 150 at 25/s (B0-identical), 300 at 50/s.
> `w1-qual-tb-20261110d` qualified **`VALID`, 0 drops, send lag p99 6.0 ms**.
> Decision record: `generator-pool-amendment-20261110.md`.

Status as of 2026-11-10: **DECIDED, then SUPERSEDED — the pool bound on both
ThingsBoard and FIWARE under the frozen 150/150 allocation, and the allocation
was subsequently amended.**

The pre-registered decision tree fired its negative branch on both platforms.
`w1-qual-tb-20261110c` reported **134 dropped iterations** (all warm-up,
`vus_peak` 150/150) and `w1-qual-fw-20261110c` reported **65 dropped iterations**
(warm-up, `vus_peak` 149/150). Measurement was drop-free on both: 30,000/30,000
each, TB p99 212 ms and FIWARE p99 1,022 ms.

This is not a platform-capability negative. It is a **generator-allocation
negative**, reached by two different mechanisms: ThingsBoard binds immediately
from cold-start latency, FIWARE binds transiently in a single 30 s window at
t=210–240 s.

Both platform reports:
- `research/findings/w1-vu-pool-binds-both-platforms.md`
- `results/workload-batch-20261108/evidence/w1-qual-tb-20261110c/qualification-result.json`
- `results/workload-batch-20261108/evidence/w1-qual-fw-20261110c/qualification-result.json`

An earlier revision of this document recorded the load host as unreachable. That
was wrong: `ramonavm1` is reachable via `ProxyJump ramonavm2`. A later revision
then claimed the pool bound only on ThingsBoard, based on B0's 30 s qualification
where FIWARE peaked at 56 VUs — that was **also** wrong, and it repeated the
30 s-warm-up mistake this document warns about. The full-duration run settled it.

## The claim under test

The W1 plan (and every W plan) fixes:

```json
"vus": { "pre_allocated": 150, "max": 150 },
"request_timeout_ms": 5000
```

at 50 messages/second. The question is whether 150 VUs can sustain 50/s without
k6 dropping iterations, because `dropped_iterations > 0` makes the attempt
**INVALID** (`workloads/k6/gen.js:7-8`, and `zero_drops` in
`scripts/dispatch-integrity.py`). An invalid attempt must be preserved and is
never retried, so getting this wrong burns a proposed attempt.

## The arithmetic

Open-loop arrivals plus a fixed VU pool means in-flight concurrency is
`rate x observed latency`. The pool binds whenever that exceeds 150:

| Rate | Latency | In-flight | vs 150 pool |
|---:|---:|---:|---|
| 25/s | 0.033 s (TB measured median) | 0.8 | fine |
| 25/s | 0.16 s | 4.0 | fine |
| 50/s | 0.16 s | 8.0 | fine |
| 50/s | 2.0 s | 100 | fine |
| 50/s | 3.0 s | 150 | **exactly at the ceiling** |
| 50/s | 5.0 s (the timeout) | **250** | **over the pool** |

So a 150-VU pool at 50/s can only sustain the rate while requests complete in
**under ~3 s**. Above that, VUs are still held, arrivals have nowhere to go, and
k6 drops them.

## What the retained evidence shows

ThingsBoard is the only platform that stressed the pool at all, and it did so at
25/s, not 50/s:

| Run | peak VUs | warm-up p99 | warm-up max | measurement p99 | measurement max |
|---|---:|---:|---:|---:|---:|
| `disp-tb-r02-20261106` | 125 | 1,925 ms | 4,967 ms | 89 ms | 343 ms |
| `disp-tb-r03-20261106` | 126 | 2,154 ms | 4,975 ms | 100 ms | 370 ms |
| `disp-tb-r04b-20261109` | 125 | 2,071 ms | 4,988 ms | 107 ms | 299 ms |
| `disp-tb-r05-20261106` | 125 | 1,936 ms | 4,995 ms | 163 ms | 547 ms |

Two facts follow, and both matter:

1. **The pool was already fully saturated in warm-up at 25/s.** Peak VU is
   exactly **125 = 25/s x 5 s** — the theoretical maximum for a 25/s arrival rate
   with a 5 s timeout. ThingsBoard's warm-up first minute holds requests for
   seconds (`disp-tb-r05` minute 1: p99 4,468 ms, max 4,995 ms), so every
   allowed VU was in use. It never dropped only because 125 < 150.

2. **Measurement is nowhere near the limit.** After warm-up, p99 is 89–163 ms,
   i.e. ~4–8 VUs in flight at 25/s. The risk is entirely in the cold/thundering
   phase, not in steady state.

At 50/s the same warm-up behaviour would demand **250 VUs against a 150 pool**.
That is not a marginal risk; it is a 100-VU shortfall, and it is precisely the
window in which ThingsBoard is slowest.

## Why this cannot be waved away

- **The pool is frozen in the policy, not just the plan.** `dispatch-integrity.py`
  raises `dispatch-integrity-v1 frozen VU allocation is 150/150` if the resolved
  plan deviates. Raising the pool is therefore not a tuning knob; it is a change
  to the frozen workload policy, which the scope explicitly forbids doing
  silently before outcomes are seen.
- **The existing 50/s qualification cannot answer it.** It is 30 s / 120 s
  (`tb-dispatch-endpoint-qual-20261106`, 1,500 + 6,000 identities) and reported
  peak 74 VUs with 0 drops — but its warm-up is only 30 s, so it never enters the
  multi-second first-minute regime that B0's 300 s warm-up hits. It is not
  full-duration W1 clearance, which is exactly what the gate says.
- **`vus_max_allocated: 300` in the summaries is not evidence of headroom.** It is
  the *sum of the two 150-VU scenario pools* (`vus_max` metric), not a pool that
  was available to either step. Reading it as 300 available VUs would be wrong.
  The real per-step pool is 150 and `vus_peak` (125) is the meaningful number.

## What the staged qualification does about it

`scripts/stage-w1-qualification.py` stages three 50/s, 300 s / 600 s plans
(15,000 + 30,000 identities) that reproduce the W1 attempt shape **exactly**
except identity, including the 150/150 pool:

```
workloads/campaigns/fixed-points-w1-w4/qualification/
  index.json                          # NOT_LAUNCH_CLEARED, execution_authorized:false
  w1-qual-tb-20261110.json
  w1-qual-fw-20261110.json
  w1-qual-ex-20261110.json
  SHA256SUMS
```

The pool is deliberately **not** enlarged. The run's job is to find out whether
150 binds at 50/s, at the cost of one 15-minute qualification instead of a
wasted proposed attempt.

### Decision tree, fixed before the outcome is seen

This is written down in advance so the result cannot be reinterpreted later:

- **Zero drops, 45,000 identities, capture/history clean** → the pool does not
  bind; W1 may proceed on the qualified platforms. Record the peak VU and the
  warm-up tail as the headroom evidence the gate asks for.
- **Any `dropped_iterations > 0`** → the frozen 150/150 policy is not viable at
  50/s. The qualification is preserved as a *valid negative result*, W1 is NOT
  launched, and the pool question becomes an owner decision about amending the
  frozen policy with a recorded comparability note. It is not retried, and the
  150 pool is not silently raised.

Neither branch is "run it again until it passes."

### Outcome (recorded 2026-11-10, after the runs)

The negative branch fired on **both** platforms tested.

ThingsBoard — `w1-qual-tb-20261110c`:

| Step | planned | attempted | dropped | ACKed | response p99 | in-flight at p99 |
|---|---:|---:|---:|---:|---:|---:|
| warm-up | 15,000 | 14,917 | **134** | 14,781 | 4,861 ms | **243 vs 150** |
| measurement | 30,000 | 30,000 | **0** | 30,000 | 211 ms | 10.6 vs 150 |

`vus_peak` = **150 of 150**. k6 warned `no headroom left`.

FIWARE — `w1-qual-fw-20261110c`:

| Step | planned | attempted | dropped | ACKed | response p99 | in-flight at p99 |
|---|---:|---:|---:|---:|---:|---:|
| warm-up | 15,000 | 14,986 | **65** | 14,950 | 4,397 ms | ~220 vs 150 |
| measurement | 30,000 | 30,000 | **0** | 30,000 | 1,022 ms | 51 vs 150 |

`vus_peak` = **149 of 150**.

**The mechanisms differ, and that matters.** ThingsBoard bound immediately (send
lag p50 2,682 ms from the first minute, never recovering). FIWARE bound
transiently: send lag p50 stayed at 2 ms almost throughout, with a single 30 s
window at t=210–240 s where response p50 jumped to 2,347 ms and p99 to 5,001 ms.
FIWARE then recovered to p99 975 ms by t=270–300 s. The FIWARE trigger is
unidentified: all containers show `RestartCount 0`, `OOMKilled false`, and no
restart occurred during the run. Recorded as open, not guessed.

The TB 134 drops decompose exactly: 83 ticks (seq 14,917–14,999) never
dispatched, plus 51 warm-up sends that spilled past the 300,000 ms step boundary
(seq 14,866–14,916). FIWARE reconciles as 14 never-dispatched + 51 spilled = 65.

**Consequence.** W1 is not launched on either platform. No proposed attempt is
spent. The pool is not raised: `scripts/dispatch-integrity.py` hard-locks 150/150,
so changing it is an amendment to frozen workload policy requiring an owner
decision and a comparability note, not a tuning step. Any re-qualification must
use a new generation ID with its own receipt; these IDs stay consumed.

**Two readings the owner may choose between:**

1. *Accept the negative.* W1 at 50/s is not qualifiable under the frozen policy on
   either platform tested; report it as a frozen-policy limit.
2. *Amend the pool.* Provision warm-up capacity for `rate x timeout` (>=250 VUs)
   under a new recorded policy revision, then re-run this exact qualification
   shape under a new generation. This changes comparability with B0 and must be
   declared as such.

Note that "the platforms cannot do 50/s" is **not** supported: measurement at
50/s was drop-free on both.

## Second-order note: the gate order

For EdgeX the readiness gate binds before the inventory gate, and for all
platforms this VU question binds before any platform claim can be made. It is
worth listing the bind order plainly, because a run can fail for reasons that
look like a platform defect:

1. Load host reachable? (**yes** — `ramonavm1` is reachable via
   `ProxyJump ramonavm2`; direct connections fail. An earlier revision of this
   document wrongly recorded it as down.)
2. Readiness/provisioning gate (EdgeX only; see `edgex-w-batch-blocker.md`)
3. **VU pool / dispatch integrity** (this document) — **now measured as binding
   on ThingsBoard**
4. Platform delivery and latency
5. History reconciliation / resource qualification

## Reproduction

```bash
# Per-step VU pool vs observed peak (the 300 is the sum of two 150 pools)
python3 - <<'PY'
import json,pathlib
for p in pathlib.Path('results/feasible-campaign-20261106').rglob('*k6-summary.json'):
    d=json.loads(p.read_text())
    if any('-r50' in (s.get('name') or '') or '-r25' in (s.get('name') or '') for s in d.get('steps') or []):
        print(f"{p.parent.name[:44]:44s} peakVU={d['vus_peak']:.0f} maxAllocated={d['vus_max_allocated']:.0f} drops={d['dropped_iterations_total']:.0f}")
PY

# The in-flight demand model
python3 -c "print('50/s at 5s timeout demands', 50*5, 'VUs vs a 150 pool')"

# The policy lock on the pool
grep -n 'frozen VU allocation' scripts/dispatch-integrity.py
```

## Related

- `research/findings/w1-readiness-after-b0.md` — prerequisite list this refines.
- `research/findings/edgex-w-batch-blocker.md` — the EdgeX gate that binds earlier.
- `workloads/campaigns/fixed-points-w1-w4/qualification-gates.json` — the W1 gate text.
- `scripts/dispatch-integrity.py:20` — the 150/150 policy lock.
- `workloads/k6/gen.js:7-8` — `dropped_iterations > 0` ⇒ INVALID.
