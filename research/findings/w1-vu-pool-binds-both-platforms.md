# Finding: the frozen 150-VU pool binds at W1's 50/s on both ThingsBoard and FIWARE

> **PARTLY SUPERSEDED — the measurements below stand, the conclusion does not.**
> The pool was correctly diagnosed as a *framework* artifact, and on 2026-11-10 the
> owner authorised replacing the fixed 150 with a rate-derived pool (300 at 50/s).
> The v1 runs below remain valid evidence of generator starvation; they are **not
> retried**. Under v2, `w1-qual-tb-20261110d` returned **`VALID` with 0 drops and
> send lag p99 6.0 ms** (v1: 134 drops, 2,682 ms). See
> `generator-pool-amendment-20261110.md`. Everything below about *mechanism*
> remains accurate; only "W1 is not launchable" is withdrawn.

**Date:** 2026-11-10
**Runs:** `w1-qual-tb-20261110c`, `w1-qual-fw-20261110c` (qualifications, not proposed attempts)
**Verdict under v1:** `INVALID_VU_POOL_BOUND` on both. No proposed attempt was spent.
**Evidence:** `results/workload-batch-20261108/evidence/w1-qual-tb-20261110c/`,
`results/workload-batch-20261108/evidence/w1-qual-fw-20261110c/`

---

## 0. Summary

| Platform | vus_peak | drops | where | send lag p50 | mechanism |
|---|---:|---:|---|---:|---|
| ThingsBoard | 150/150 | 134 | entire warm-up | 2682 ms | immediate cold start |
| FIWARE | 149/150 | 65 | t=210–240 s only | 2 ms | transient mid-warm-up stall |

Both platforms dropped zero iterations in measurement. The pool is the binding
constraint, not platform throughput. The two failures have **different causes**,
so a fix aimed at only one will not resolve the other.

## 1. What was run, and why it had to be run

The W1 release gate requires a full-duration 50/s generator/capture validation at
the exact W1 shape: 300 s warm-up (15,000 identities) + 600 s measurement (30,000
identities). The only pre-existing 50/s evidence, `tb-dispatch-endpoint-qual-20261106`,
was 30 s / 120 s and the gate states in terms that it "does not transfer."

The qualification plan therefore reproduces the W1 attempt **exactly** — same
adapter, same 1,000 devices, same 50/s, same 300/600 s schedule, same **150/150 VU
pool**, same 5,000 ms timeout, same `dispatch-integrity-v1` policy. The pool was
deliberately *not* enlarged: the point of the run is to discover whether 150 binds
at 50/s, at the cost of one 15-minute qualification rather than a wasted proposed
attempt.

The gate rule was fixed before the outcome was known:

> `dropped_iterations > 0` (VU pool binds) or identity/capture mismatch ⇒ INVALID

## 2. What happened

| Step | planned | attempted | dropped | ACKed | explicit fail | unknown | response p99 | in-flight at p99 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| warm-up (300 s) | 15,000 | 14,917 | **134** | 14,781 | 0 | 136 | 4,861 ms | **243 vs 150** |
| measurement (600 s) | 30,000 | 30,000 | **0** | 30,000 | 0 | 0 | 211 ms | 10.6 vs 150 |

`vus_peak` = **150 of 150**. k6 reported its own warning:
`peak VUs 150 reached the largest step pool 150: no headroom left`.

Measurement was immaculate. Every failure is in the cold warm-up window.

### The 134 drops reconcile exactly

- **83 ticks never dispatched** — seq 14,917–14,999, the contiguous warm-up tail.
- **51 warm-up sends executed but spilled past the 300,000 ms step boundary** —
  seq 14,866–14,916, at offsets 300,002–300,082 ms.
- **Measurement contributed 0.**

83 + 51 = 134. There is no unexplained loss.

## 3. Mechanism

ThingsBoard's first-contact latency under a cold start holds a tail of requests
for up to the 5,000 ms client timeout (response max 5,003 ms; p99 4,861 ms). At
50/s, `rate x latency` = 50 x 4.86 s ≈ **243 in-flight requests**, against a frozen
pool of 150.

The generator consequently fell ~2.68 s behind schedule inside the first minute
and never recovered, because a fixed-pool constant-arrival executor cannot borrow
VUs it does not have. Warm-up send lag was already 2,682 ms at p50 and stayed
there; it was not a transient.

The warm-up *ack* latency (p99 6,042 ms) is measured from scheduled time, so it is
essentially the constant 2.68 s send lag plus the response latency. **It is
arithmetic, not a second platform defect.** Reporting it as platform latency would
be wrong.

Once warm, steady state needs ~11 VUs and 50/s is served cleanly.

## 4. What this does and does not establish

**Established:** the frozen `dispatch-integrity-v1` 150/150 allocation cannot drive
a 50/s arrival stream through ThingsBoard's cold first minute. A W1 TB attempt
under the current policy would be INVALID by construction.

**Not established:** that ThingsBoard cannot serve W1 at 50/s. Steady state was
clean: 30,000/30,000 sent, 30,000 ACKed, p99 212 ms, max 718 ms.

This matters because the tempting summary — "ThingsBoard fails at 50/s" — is
false. The accurate one is "the frozen generator allocation fails at 50/s against
a cold ThingsBoard."

## 5. The advance prediction was correct

`research/findings/w1-vu-pool-headroom-risk.md` predicted, before the run:

> At 50/s the same warm-up behaviour would demand 250 VUs against a 150 pool...
> The risk is entirely in the cold/thundering phase, not in steady state.

Observed: peak exactly 150/150, 134 warm-up drops, spotless measurement. Correct
in structure and arithmetic. This is worth noting because it means the decision
tree was not reinterpreted after the fact.

## 6. The pool is a frozen policy, not a tuning knob

`scripts/dispatch-integrity.py` raises
`dispatch-integrity-v1 frozen VU allocation is 150/150` if a resolved plan deviates.
Raising the pool is therefore **an amendment to frozen workload policy**, which
changes comparability with B0. It cannot be done silently, and it was not done.

## 7. Correction: this is NOT platform-specific — FIWARE binds too

An earlier revision of this document argued the pool bound only on ThingsBoard,
citing B0's 30 s qualification where FIWARE peaked at 56 VUs. **That inference was
wrong, and it was the exact trap this document warns about elsewhere: a 30 s
warm-up never enters the regime that binds.**

The full-duration FIWARE qualification (`w1-qual-fw-20261110c`, run immediately
after) bound too:

| Platform | run | vus_peak | drops | drop location | send lag p50 |
|---|---|---:|---:|---|---:|
| ThingsBoard | `w1-qual-tb-20261110c` | 150/150 | 134 | whole warm-up (immediate) | 2682 ms |
| FIWARE | `w1-qual-fw-20261110c` | 149/150 | 65 | single window t=210–240 s | 2 ms |

So the frozen 150/150 pool at 50/s with a 5000 ms timeout binds on **both**
platforms tested. This is a property of the policy, not of one SUT.

### But the mechanisms are genuinely different

That distinction still matters, because a remedy justified only by the TB story
would not fix FIWARE:

- **ThingsBoard — immediate cold start.** Send lag was 2682 ms at p50 from the
  first minute and never recovered. The pool was exhausted continuously.
- **FIWARE — transient mid-warm-up stall.** Send lag p50 stayed at 2 ms for
  essentially the entire run. A single 30 s window at t=210–240 s saw response
  p50 jump to 2347 ms and p99 to 5001 ms (the timeout ceiling), pushing in-flight
  demand past 150 and dropping 65. It then recovered: t=270–300 s was back to
  p99 975 ms.

The FIWARE trigger is **not identified**. All `fiware`/`fiwhc` containers show
`RestartCount 0`, `OOMKilled false`, and no process restarted during the run.
That is recorded as an open question rather than guessed at.

### What this changes about the owner decision

The decision in section 8 is now stronger and simpler: no platform tested can
qualify W1 at 50/s under the frozen 150/150 allocation. Either the policy is
amended for warm-up capacity, or W1 at 50/s is reported as not qualifiable under
current policy on every platform tried.

It does **not** change that steady state is fine: both platforms had zero
measurement drops.

## 8. Owner decision required

Two defensible readings, and this is genuinely the owner's call because one of
them rewrites frozen policy:

1. **Accept the negative.** W1 ThingsBoard is not qualifiable at 50/s under the
   frozen policy. Report it as a frozen-policy limit alongside the other TB-in-B0
   findings. No B0 comparability break.
2. **Amend the pool.** Provision warm-up capacity for `rate x timeout` (≥250 VUs)
   under a new, recorded policy revision with an explicit comparability note, then
   re-run *this exact shape* under a **new generation ID**. This is a policy change
   and must be declared as such; it is not a retry of this ID.

Either way the run ID stays consumed and is never relaunched.

## 9. Harness notes (secondary, no evidence impact)

- **Analysis correctly refused to score.** `dispatch-integrity.evaluate` raised
  `complete dispatch ledger required: 44917/45000` because 83 ticks were never
  dispatched. An incomplete ledger cannot be scored, so
  `platform_result: NOT_ASSESSED_NO_ANALYSIS` is correct fail-closed behaviour, not
  a harness bug.
- **The load lock was retained by design, then recovered.** The launcher releases
  `/run/iot-bench-workload.lock` only when cleanup verified *and* finish started
  with "stopped". Analysis failure skips finish, so the lock was preserved. The
  owner was identified as this completed run (no processes, no containers, no sink
  listener), and it was removed after verification. Recorded in
  `lock-recovery.json`. Same signature as B0 `disp-tb-r01` and `disp-tb-r03`.
- **Capture integrity is clean.** Sink duplicates 0, unparsed 0, overflow 0,
  incomplete bodies 0; capture window `complete: true` with a passing 30 s quiet
  interval and zero late receipts above the cutoff.
- **Identity integrity is exact.** 44,917 ledger rows, 44,917 unique seq, and the
  history export's seq set is *identical* to the ledger's.
- **574 ACKed-but-not-captured identities, all in warm-up** (send offsets
  1,086–173,123 ms). Measurement had **zero**. This is the same warm-up delivery
  gap documented for TB in B0, not measurement loss.

## 10. Related

- `research/findings/w1-vu-pool-headroom-risk.md` — the prediction and decision tree.
- `research/findings/w1-readiness-report-20261110.md` — consolidated W readiness.
- `research/findings/edgex-w-batch-blocker.md` — the EdgeX gate that binds earlier.
- `workloads/campaigns/fixed-points-20261108/qualification/index.json` — the row now
  reads `INVALID_VU_POOL_BOUND`.
