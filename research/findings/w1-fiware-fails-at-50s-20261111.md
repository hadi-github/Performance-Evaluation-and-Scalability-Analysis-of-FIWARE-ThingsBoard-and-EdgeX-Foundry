# FIWARE fails W1 at 50/s — and the harness was clean when it did

**Date:** 2026-11-11
**Run:** `w1-qual-fw-20261110f` (qualification; does **not** count toward the 60 attempts)
**Outcome:** `INVALID_PLATFORM_FAILURE_OR_BACKLOG`
**Scope of the claim:** This is a *platform* result. The generator and capture were sound.

---

## 1. Why this run is trustworthy

This is the first FIWARE v2 run where **both known harness defects were absent**:

| Harness condition | State |
|---|---|
| Complete dispatch ledger on a v2 plan | **45,000 / 45,000** rows |
| `pool_policy.py` in the staged set | present, remote analysis ran clean |
| `dropped_iterations` | **0** |
| Dispatch-integrity findings | **none** |
| Send lag p99 | **4–5 ms** |

The generator dispatched exactly 50/s in every one-second bin, in both phases, with zero
spill. Whatever happened next was FIWARE's doing.

## 2. What FIWARE did

```
warmup       15000/15000  ack 14424  fail 0  unknown    576  dropped 0  lag p99 5 ms  ack p99 4601 ms
measurement  30000/30000  ack 16519  fail 0  unknown  13481  dropped 0  lag p99 4 ms  ack p99 4896 ms
```

**13,481 of 30,000 measurement sends exceeded the 5,000 ms request timeout.**

The important nuance: these were not lost messages. **13,461 of the 13,481 timed-out sends
did eventually arrive.** The p99 dispatch-to-receipt was **~35.3 s against a 5 s timeout —
about 7× the limit**. So FIWARE was not dropping traffic, it was falling catastrophically
behind under sustained load.

What was genuinely lost:

- **20 identities** never arrived before the capture cutoff
- the **same 20** are missing from FIWARE history (`VALID_HISTORY_FAILURE`)
- sink: 44,980 rows / 44,980 unique, **0** duplicates, **0** late rows, **0** malformed bodies

So the failure is: roughly 45% of measurement requests breached the timeout, the queue
never drained, and 20 messages were still in flight when the window closed.

## 3. It was not the generator

This is the check that matters, because a slow generator would have produced the same
console line:

| Evidence | Value |
|---|---|
| Mean generator CPU during traffic | **0.20 cores** of 2 allocated (10%) |
| `saturation_flag` | `false` |
| `cgroup_throttling_observed` | `false` |
| `dropped_iterations` | **0** |
| Send-lag p99 | 4–5 ms |

The generator used a tenth of its budget and never throttled. **Raising the VU pool or
giving the generator more CPU would not have changed this result.** That is what
distinguishes a platform failure from the two earlier `c` runs, which *were* generator
starvation.

## 4. One honest wrinkle: `generator_resource_qualified: false`

The evaluator reports **four failing resource checks**:

```
effective_generator_cpuset      false
generator_and_ancestor_quota    false
memory_cgroup_evidence          false
no_swap_or_oom_events           false
```

I investigated these rather than accepting the label, because all four passed on
ThingsBoard with the same host, cpuset and generator.

**Finding: all four failures come from a single sample.** Of 926 active samples, exactly
one fails — the **last** one, **8.4 s after the measurement phase had already ended** —
with `cpu_stat: null` and a transient teardown error:

```
collection_errors: [{"area":"cgroup-or-schedstat-or-sockets","error":"read-or-parse-failed"}]
```

Recomputing with only in-window traffic samples (917 of them):

```
effective_generator_cpuset      true
generator_and_ancestor_quota    true
memory_cgroup_evidence          true
no_swap_or_oom_events           true
```

**I did not change the evaluator.** Its rule is deliberately fail-closed — *"Missing data
with a live PID still fails"* — and that rule is correct for traffic-time evidence. I am
not reinterpreting it after seeing the outcome. The raw `false` is recorded verbatim in
`qualification-result.json`, alongside the window-scoped analysis, so a reviewer sees both.

**And it does not change the verdict.** Even with the generator fully resource-qualified,
`platform_result` remains `VALID_PLATFORM_FAILURE_OR_BACKLOG`, because that is established
by dispatch, capture and history evidence — all independent of this check.

If anything, the fix belongs in the *sampler*: a teardown read failure after the traffic
window should not be able to fail a traffic-time qualification. That is a policy
amendment, and an owner decision, not something I will quietly patch.

## 5. Two generations of FIWARE evidence, opposing conclusions

| | `c` (v1, 150 VU) | `f` (v2, 300 VU) |
|---|---|---|
| Verdict | `INVALID_VU_POOL_BOUND` | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` |
| Dropped | 134 | 0 |
| Cause | **generator starvation** | **platform backlog** |
| Scorable? | no — measured the harness | **yes** |

The `c` run could not tell us anything about FIWARE because the generator starved. The `f`
run can, and what it says is that **FIWARE does not absorb 50/s at W1 shape**.

## 6. Where W1 stands

| Platform | State |
|---|---|
| ThingsBoard | **`VALID`** (`w1-qual-tb-20261110e`, warm-start caveat) |
| FIWARE | **`INVALID_PLATFORM_FAILURE_OR_BACKLOG`** (this run) |
| EdgeX | excluded this round by owner instruction; open decision |

**W1 is not cleared.** One platform qualified, one failed, one excluded.

Batch release is untouched:

```
execution_authorized : false
matrix status        : PROPOSED_NOT_LAUNCH_CLEARED
all 60 rows          : BLOCKED_PENDING_VARIANT_RELEASE
```

## 7. Open decisions for the owner

1. **Does FIWARE failing W1 at 50/s block W1, or is it a finding to report?** The gate says
   full-duration 50/s validation on all three platforms. FIWARE has now been honestly
   measured and does not pass. That is either a redesign trigger or a documented result —
   not something I should decide.
2. **EdgeX** remains excluded from this round; its readiness (~489 s vs 180 s) and
   inventory cap (16,384 failing from rep 2) are unresolved.
3. **The teardown-sample rule** in the resource evaluator is fail-closed by design and
   currently penalises runs for a post-window read error. Amending it is a policy change.

## 8. Related

- `w1-qualified-thingsboard-20261111.md` — the `VALID` TB result and warm-start caveat
- `v1-only-constants-broke-v2-runs.md` — the four v1-only constants and their fixes
- `generator-pool-amendment-20261110.md` — the rate-derived pool decision
- `edgex-w-batch-blocker.md` — the open EdgeX decision
