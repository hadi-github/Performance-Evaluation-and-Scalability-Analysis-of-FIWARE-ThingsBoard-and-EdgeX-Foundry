# Finding: the 50/s headroom gate is unresolvable under the frozen pool — and only W1 needs it

> **SUPERSEDED ON THE REMEDY.** The arithmetic here is correct and was the basis for
> the fix: the freeze was a framework artifact, not a platform limit. The owner
> authorised a rate-derived pool on 2026-11-10, which makes 50/s qualifiable while
> leaving the 25/s value at exactly 150 (B0-identical). ThingsBoard then qualified
> **`VALID` with 0 drops** under v2. Read `generator-pool-amendment-20261110.md`
> for the applied decision; this page is retained for the derivation.

**Date:** 2026-11-10
**Status:** derived from arithmetic + the two measured v1 qualifications; remedy superseded 2026-11-10

---

## The core inequality

The frozen workload policy fixes:

```
vus_preallocated_per_scenario: 150
vus_max_per_scenario:        150
request_timeout_ms:          5000
keep_alive: true, open_loop: true
```

For an open-loop constant-arrival generator, in-flight concurrency is
`rate x latency`, capped by the pool. The pool therefore binds whenever:

```
rate x latency > 150     ⇔     latency > 150 / rate
```

| Workload | rate | pool binds when latency > | timeout ceiling |
|---|---:|---:|---:|
| W1 | 50/s | **3.0 s** | 5.0 s |
| W2 | 25/s | 6.0 s | 5.0 s |
| W3 | 25/s | 6.0 s | 5.0 s |
| W4 | 25/s | 6.0 s | 5.0 s |

## What follows — and it splits the batch cleanly

**At 25/s the pool cannot bind by construction.** The worst possible latency is
the 5,000 ms client timeout, and 25 x 5.0 s = 125 < 150. There is no reachable
latency that exceeds the 6.0 s threshold, because the client aborts at 5.0 s.
This is arithmetic, not measurement. It matches B0: every measured 25/s cell had
`workload_valid: true` with zero drops.

**At 50/s the pool can pass only if latency stays under 3.0 s.** Whether it does
is an empirical property of each platform's first-contact behaviour.

## The measured answer for W1

Both platforms tested exceeded 3.0 s in warm-up, so both bound:

| Platform | warm-up latency vs 3.0 s threshold | drops | peak VUs |
|---|---|---|---:|
| ThingsBoard | p99 4.86 s, max 5.00 s — **1.6x over** | 134 | 150/150 |
| FIWARE | peak p99 5.00 s in a t=210–240 s stall — **1.7x over** | 65 | 149/150 |

And both were clean in measurement, where latency was far under 3.0 s (TB 211 ms,
FIWARE 1,022 ms).

## The consequence for the release gates

This means the two gates have very different standing:

1. **`W1` full-duration 50/s gate — cannot be satisfied under the frozen pool.**
   Not "hasn't been yet"; cannot be, unless latency stays under 3.0 s for the
   whole 300 s warm-up on a cold platform. Two of two platforms failed that. Any
   further attempt with the same plan is a foregone INVALID and would waste a
   qualification ID, so it should not be run for information.

2. **`other_configuration_headroom` (the 50/s headroom clause for W2/W3/W4)** —
   **does not apply as written.** The gate says "Qualify applicable 50/s headroom
   for D=2000, padding=1024 or N=1/k=2." But W2/W3/W4 are specified at
   **25 messages/second** (`scope.json` variants), not 50/s. At 25/s the pool
   cannot bind by the arithmetic above. The clause appears to assume the W2–W4
   variants inherit W1's 50/s, which they do not.

   The honest reading: the *rate-specific* part of that clause is inert for
   W2–W4 **if** the clause means "the rate those variants actually run at."
   The *configuration-specific* parts — D=2000 provisioning, padding=1024
   payload fidelity, N=1/k=2 replica integrity — remain real, live gates and are
   documented separately in `qualification-gates.json` W2/W3/W4 and in
   `edgex-w-batch-blocker.md` / the W3 FIWARE subscription gap.

   **But there is a second reading that must not be glossed over.** If the clause
   literally requires *50/s headroom* evidence for those configurations — i.e. a
   claim that W2–W4 configurations could sustain W1's rate — then the gate is
   **also unresolvable**, because 50/s cannot be qualified at all under the
   frozen pool. Under that reading W2/W3/W4 are blocked by the same policy defect
   as W1, not merely by their own configuration gates.

   Which reading is correct is genuinely ambiguous in the gate text, and
   `scope.json` plus the 60 frozen plans (all 25/s) contradict the literal
   "50/s" wording. The owner must resolve which is authoritative; I am not
   going to pick the convenient one silently.

## Why I am not "fixing" the pool

`scripts/dispatch-integrity.py` raises
`dispatch-integrity-v1 frozen VU allocation is 150/150` on any deviation. Raising
it would:
- change the frozen workload policy mid-batch;
- break comparability with every B0 25/s cell;
- and, for W1, change the meaning of the result (a 250-VU pool that passes says
  "the platform can serve 50/s with enough concurrency", which is a different
  claim from the frozen one).

That is an owner decision with a recorded comparability note, not a defect fix.

## The options that remain, stated plainly

1. **Report W1 as not-qualifiable under frozen policy** on both platforms. Do not
   launch W1. This is the conservative, evidence-faithful option.
2. **Amend the pool** to `>= rate x timeout` under a new recorded policy revision,
   then re-qualify under new generation IDs. Declares the comparability break.
3. **Reduce W1's rate** to a value where `rate x 5 s <= 150` (i.e. <=30/s). This
   changes the experiment's definition and must be an explicit owner decision, not
   a silent downgrade.

Any of the three is defensible. What is *not* defensible is relaunching the same
50/s plan against the same 150 pool and hoping for a different result.

## What is actionable right now, independent of the pool decision

- **W2/W3/W4 are not pool-blocked** (25/s). Their real blockers are: EdgeX
  device caps + readiness (documented, owner decision pending), W2 D=2000
  provisioning, W3 FIWARE payload-preserving subscription, W4 N=1/k=2 effective
  cpuset verification.
- Nothing about W2–W4 requires the W1 pool question to be settled first.

## Related

- `research/findings/w1-vu-pool-binds-both-platforms.md` — the two measurement reports.
- `research/findings/w1-vu-pool-headroom-risk.md` — the original prediction.
- `workloads/campaigns/fixed-points-w1-w4/qualification-gates.json` — gate text.
- `workloads/campaigns/fixed-points-w1-w4/scope.json` — W2–W4 = 25/s.
