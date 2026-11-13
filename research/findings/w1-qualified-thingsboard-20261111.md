# W1 is qualified on ThingsBoard — and this run found two more harness defects

**Date:** 2026-11-11
**Run:** `w1-qual-tb-20261110e` (qualification; does **not** count toward the 60 attempts)
**Verdict:** **`VALID`** — `VALID_PLATFORM_SUCCESS`, all gates true.
**Scope of the claim:** W1 is qualified on **one of three** required platforms. It is not
a W1 clearance, and it authorizes no attempt.

---

## 1. Result

```
== w1-qual-tb-20261110e: VALID
  s00-warmup-r50       15000/15000  ack 15000  fail 0  unknown 0  dropped 0  lag p99 7.0 ms  ack p99 625.0 ms
  s01-measurement-r50  30000/30000  ack 30000  fail 0  unknown 0  dropped 0  lag p99 5.0 ms  ack p99 150.0 ms

records.ndjson 45000/45000     sink received_total 45000, dup/unparsed/overflow all 0
workload_integrity valid=true findings=[]   dispatch_spill 0 in both phases
generator_resource_qualified true   saturation false   clock VALID (1956 samples)
history_valid true   cleanup_verified true   lock released
```

### The three generations, side by side

| | v1 `c` (150 VU) | `d` (300 VU) | `e` (300 VU) |
|---|---|---|---|
| Verdict | `INVALID_VU_POOL_BOUND` | `INVALID_HARNESS_LEDGER_MODE` | **`VALID`** |
| Dropped iterations | 134 | 0 | **0** |
| Warm-up unknown | 136 | 406 | **0** |
| Ledger rows | 44,917 / 45,000 | **406** / 45,000 | **45,000 / 45,000** |
| Warm-up ack p99 | 6,042 ms | 3,969 ms | 625 ms |

Each generation failed for a *different, genuine* reason, and each failure was preserved
rather than retried. That is why `e` is trustworthy: it is the first run where none of the
three known harness failure modes was active.

## 2. Two more harness defects found by running it

### 2a. `pool_policy.py` was never staged for remote analysis

The remote post-run analysis died **after the capture was already complete**:

```
ModuleNotFoundError: No module named 'pool_policy'
analysis did not produce .../w1-qual-tb-20261110e.dispatch-analysis.json
```

`scripts/dispatch-integrity.py` imports `pool_policy` at module scope, but
`STAGED_SCRIPTS` listed the importer and not the module. This is a direct omission from
the pool amendment itself — the new module was wired into the code but not into the
deployment set.

**Fixed:** `scripts/pool_policy.py` added to `STAGED_SCRIPTS`.

**Important:** the measurement was **not lost**. The local reanalysis pass runs from the
repo root, where `pool_policy.py` exists, and it produced the full verdict with
`exit: 0`. So this was a *reporting* failure with the evidence intact, not a lost run.
That distinction is why the run still counts as `VALID`.

### 2b. The ledger fix is confirmed on live traffic

`records.ndjson` holds exactly **45,000** rows on a `dispatch-integrity-v2` plan. Under
the `d` generation the same policy produced 406. This is the end-to-end confirmation of
the `gen.js` fix, beyond the local k6 probe.

## 3. An honest caveat: this was a warm start

ThingsBoard had been up and had just served the `d` run. The warm-up ack p99 fell from
3,969 ms to 625 ms, which is platform warmth, not generator improvement.

The plan documents this deliberately (*"Existing deployed state, device cohorts and
history are retained; not a cold-start repetition"*), and the W1 gate is about
full-duration generator/capture integrity — which this establishes. But it is **not**
evidence about ThingsBoard's cold-start behaviour at 50/s, and I am not claiming it is.

## 4. What this does *not* clear

W1 requires all three platforms. Current state:

| Platform | State |
|---|---|
| ThingsBoard | **`VALID`** (this run) |
| FIWARE | `NOT_ASSESSED_PREFLIGHT_REFUSAL` — correctly refused because ThingsBoard was up |
| EdgeX | not run; readiness ~489 s vs 180 s deadline, open owner decision |

And the batch release is untouched:

```
execution_authorized : false
matrix status        : PROPOSED_NOT_LAUNCH_CLEARED
all 60 rows          : BLOCKED_PENDING_VARIANT_RELEASE
```

**W2/W3/W4 were never pool-blocked** (25 × 5 s = 125 < 150) and are unaffected by any of
this work. Each still has its own unqualified gate: W2 2,000-device provisioning, W3
`size_offset_bytes=1024` with no existing payload-preserving evidence path (the FIWARE
subscription omits `padding`), W4 N=1 replicas with k=2 effective cpusets.

## 5. Related

- `v1-only-constants-broke-v2-runs.md` — the ledger/label defect class and its four fixes
- `generator-pool-amendment-20261110.md` — the rate-derived pool decision
- `edgex-w-batch-blocker.md` — the open EdgeX decision
- `workloads/campaigns/fixed-points-20261108/qualification/index.json` — consumed IDs
