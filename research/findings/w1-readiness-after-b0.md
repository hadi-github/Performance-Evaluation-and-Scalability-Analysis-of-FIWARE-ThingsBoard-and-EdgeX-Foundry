# W1 readiness after B0: what is executable, what is blocked, what must be built

Prepared 2026-11-10, while B0's final EdgeX ordinal is still pending. **This note
authorises nothing.** The W1–W4 batch remains `PROPOSED_NOT_LAUNCH_CLEARED` with
`execution_authorized: false`. It records what has been *verified runnable*, so
that the transition after B0 is a decision rather than a discovery.

## Verified runnable today

All three W1 platform plans resolve against the **same prebuilt `benchctl`
already in use for B0** — byte-identical, `64f66385…`, the digest recorded in
`offline-resolution.json`:

| Plan | Resolves | Warm-up | Measurement | Total ticks |
|---|---|---|---|---|
| `w1dp-tb-r01-20261108` | yes | 50/s × 300 s = 15,000 | 50/s × 600 s = 30,000 | 45,000 |
| `w1dp-fw-r01-20261108` | yes | 15,000 | 30,000 | 45,000 |
| `w1dp-ex-r01-20261108` | yes | 15,000 | 30,000 | 45,000 |

Verified by running the actual prebuilt binary, not by reading the staged JSON:

```bash
benchctl resolve -plan workloads/campaigns/fixed-points-20261108/plans/w1dp-tb-r01-20261108.json \
                 -run-id w1dp-tb-r01-20261108
# -> steps: warmup 50/s x 15000 ticks, measurement 50/s x 30000 ticks
```

The measurement stack is configuration-driven and already supports 50/s:

| Component | Evidence |
|---|---|
| Generator | `workloads/k6/gen.js:73` — `executor: 'constant-arrival-rate', rate: s.tick_rate`; rate comes from the plan |
| Capture | `scripts/run-capture.sh` — takes `PLAN RUN_ID RESULTS_DIR PLATFORM`; no rate/device/replica constants |
| Integrity gate | `scripts/dispatch-integrity.py:10` — `RATE_BIN_LIMITS` contains an explicit `50: (48, 52, 11)`; covered by the frozen test `test_headroom_fifty_rate_checks_frozen_bins` |
| Analysis | `scripts/analyze-dispatch-run.py` — phase/rate/duration read from the resolved plan |

The only differences from B0 are the intended ones: seed, 50/s instead of 25/s,
and double the tick count. Devices, payload, timeouts, keep-alive, VU ceiling and
health policy are identical to B0.

## Genuinely blocked

| Block | Why | Unblocks by |
|---|---|---|
| **W1 EdgeX** | **Two independent blockers, not one, and only one attempt clears even the inventory half.** (a) Retained 10,192 + reserve 6,000 = 16,192 vs `MaxResultCount` 16,384 — **W1 rep 1 passes with +192 margin**; every later EdgeX attempt fails it (rep 2: 17,192, −808). (b) The readiness gate binds *first*: `device-rest` fetches the shared profile once per device (`48 ms` measured live), projecting ~489 s at 10,192 devices against a 180 s deadline. | (a) raising the discovery cap for the whole batch; (b) **a platform fix** — deduplicate the profile fetch, or stop retaining inventory across runs. Raising the deadline alone is not a fix. Superseded detail: `edgex-w-batch-blocker.md`. |
| **W2 EdgeX, W3 EdgeX, W4 EdgeX** | Same ceiling, worse. Full batch needs ≥ 41,192. | Same single bump, frozen once as the W baseline |
| **W3 on FIWARE** | Orion subscription selects `attrs: ['seq','run_id','scheduled_send_ms']`; 1,024-byte padding is never delivered, so the padding claim has no evidence path | A reviewed payload-preserving subscription/evidence path |
| **Appending W to the B0 matrix** | B0 scope is frozen; `continue-ordinary-campaign.py` and `run-feasible-campaign.py:88` reject any ID not matching `disp-(tb\|fw\|ex)-r(0[1-5])-20261106`. W must not contaminate B0 provenance. | A separate W runner |

## Must be built before any W1 traffic

**Status 2026-11-10: items 1, 2 and 6 are now built and tested offline. 3–5 remain
open, and 5 still blocks all EdgeX.**

1. **A workload-aware launcher — BUILT.** `scripts/run-workload-campaign.py`
   validates against the W scope/matrix/plans and stages evidence under its own
   root. It does not re-implement the risky machinery: it imports the B0 launcher
   as a library and rebinds only plan resolution, plan validation and the
   evidence roots, so staging/activation/gates/capture/cleanup remain the single
   proven implementation. `check` validates 60 rows and 0 problems offline.
2. **Receipts per attempt.** Still modelled on the B0 `input_binding()`
   fail-closed equality; the owner-authorization receipt is a separate artefact
   and is **not** yet recorded. `scope.json` still has `execution_authorized: false`.
3. **A memory gate on EdgeX.** Raising the cap allocates nothing, but each
   `device-rest` replica caches the **whole** cohort (no queue group). Sample
   cgroup `memory.peak` around setup and refuse to launch if projected usage
   approaches `EDGEX_DEVICE_REST_MEM`. Fail closed rather than extrapolate.
4. **A whole-cohort acceptance check for EdgeX.** The current gate POSTs only to
   `bench-$RUN-*` devices, so it proves the current cohort is accepted, not that
   every replica cached the full historical cohort. That gap is benign at 10,192
   and would not be at 35,192, where silent truncation becomes possible.
5. **A working cache-init path for EdgeX (still blocking).** Ordinal 15 proved the
   per-device profile fetch costs O(N) and exceeds the readiness deadline at
   today's inventory. Until that is addressed, no EdgeX run can reach the
   inventory gate at all, so the cap bump and the memory gate are moot for EdgeX.
   See `edgex-ordinal-15-readiness-failure.md`.
6. **A workload-aware aggregator — BUILT.** `scripts/aggregate-runs.py` now
   identifies `w{N}dp-*` run IDs, tags each row with `workload_id`, and groups
   summary sections by (workload, platform) so W1 and W2 on one platform can
   never be pooled into one replicate statistic. Verified behaviour-neutral on B0:
   regenerating from the same evidence changes **0** existing cells. Covered by
   `tests/aggregate_runs_test.py`.
7. **A W1 50/s full-duration qualification — RUN, both platforms failed.**
   `scripts/stage-w1-qualification.py` staged three 50/s 300/600 s plans
   (15,000 + 30,000 identities) reproducing the W1 attempt shape exactly except
   identity, at the real 150/150 pool. TB and FIWARE have since run and both
   returned `INVALID_VU_POOL_BOUND` (134 and 65 warm-up drops respectively).
   Current state: `research/findings/w-batch-status-20261110.md`.

## Two W1-specific risks to state before running

**VU headroom — corrected 2026-11-10: this is the more likely of the two, not the lesser.**

W1 reuses B0's `vus: {pre_allocated: 150, max: 150}` at 50/s with a 5,000 ms
timeout. An earlier version of this note said the risk was "unlikely to bind"
because measurement p99 is ~215 ms. That reasoning was wrong: it looked only at
steady state, and B0's own evidence shows the pool saturating in **warm-up**.

Every measured ThingsBoard cell peaked at **125 VUs at 25/s** — exactly
`25/s × 5 s`, the theoretical ceiling for that arrival rate — with warm-up first
minute p99 of 1,925–2,154 ms and maxima at 4,967–4,995 ms. The pool never dropped
only because 125 < 150. At 50/s the same warm-up behaviour demands **250 VUs**,
a 100-VU shortfall, in precisely the window where the platform is slowest.

The pool was frozen in the policy itself (`dispatch-integrity.py` raised
`frozen VU allocation is 150/150`), so it could not be raised as tuning. It was
tested instead: the staged 50/s 300/600 s qualification ran at the real 150/150
pool.

**Result (recorded 2026-11-10): it bound on both platforms.** ThingsBoard dropped
134 iterations (send lag 2,682 ms p50 from the first minute, immediate cold
start); FIWARE dropped 65 (a single transient stall at t=210–240 s where p99 hit
5,001 ms, with send lag p50 otherwise 2 ms). Both had **zero measurement drops**.

**Then it was fixed (2026-11-10).** The freeze was a framework artifact, not a
platform limit, so the owner authorised a rate-derived pool: 150 at 25/s
(B0-identical), 300 at 50/s. `w1-qual-tb-20261110d` qualified **`VALID` with 0
drops and send lag p99 6.0 ms**. See `generator-pool-amendment-20261110.md`; the
"cannot be qualified" conclusion below is withdrawn.
(`latency > 150 / rate` = 3.0 s at 50/s) and `w-batch-status-20261110.md` for the
owner options.

**W2/W3/W4 are NOT blocked by this.** They run at 25/s, where
`25 × 5 s = 125 < 150`, so the pool cannot bind by construction.

**The workload carries 2× the messages per attempt.** 45,000 vs B0's 22,500, in
the same 900 s window (300 s warm-up + 600 s measurement) — so the offered rate
is doubled rather than the run lengthened. Host and clock evidence must cover the
full window; the same `dispatch-integrity-v1` policy applies unchanged.

## Sequencing

```
B0: ordinal 15 FAILED (zero-traffic setup failure, preserved; not retried)
      |
      +--> B0 batch CLOSES: EdgeX has 2 valid measurements (ordinals 3, 5)
      |    and 3 preserved setup failures (ordinals 7, 11, 15)
      |    aggregate refreshed; TB limitation finalised
      |
      v
W pre-conditions: W launcher + receipts
      +--> W1 qualifications RUN: TB and FIWARE both INVALID_VU_POOL_BOUND
      |    => W1 NOT launchable under the frozen 150/150 pool (owner decision)
      +--> W1 on EdgeX         BLOCKED on a platform defect:
      |                         cache-init O(N) profile fetch exceeds the
      |                         readiness deadline (needs dedup or no retention),
      |                         AND the inventory cap still needs raising to >=41192
      +--> W2/W3/W4            25/s, so NOT pool-blocked; each has its own
                               configuration gate (D=2000, padding=1024, N=1/k=2)
      v
W2, W4 -> W3 only when the FIWARE padding evidence path exists
```

W and B0 must never run concurrently: the scope requires
`isolated_one_platform_at_a_time`.

## What changed after this note was first written

Ordinal 15 ran and **failed** on 2026-11-10 as a zero-traffic setup failure
(`disp-ex-r05-20261106`, preserved, not retried). It did not fail on the
inventory cap as anticipated — it failed earlier, at the readiness gate, because
`device-rest` fetches the shared profile once per device and the cost exceeds the
180 s deadline at the current retained inventory. Full account:
`edgex-ordinal-15-readiness-failure.md`.

B0 therefore closes with **2 valid EdgeX measurements** (ordinals 3 and 5) and
**3 preserved setup failures** (ordinals 7, 11, 15). The W1 TB and W1 FIWARE
conclusions above are unaffected.

## What this note does not do

- It does not mark W1 launch-cleared, and it does not touch
  `scope.json`, `run-matrix.json` or any plan.
- It records no measurement. Resolving a plan offline is not running it.
- It does not decide the EdgeX cap value; that is argued in the companion
  finding and remains an owner decision.
