# EdgeX qualifies W1 cleanly — and the recorded blocker was overstated 2×

**Date:** 2026-11-12
**Run:** `w1-qual-ex-20261110e` (qualification; does **not** count toward the 60 attempts)
**Verdict:** **`VALID`** — `VALID_PLATFORM_SUCCESS`, history `VALID_HISTORY_SUCCESS`.
**Scope of the claim:** W1 is qualified on EdgeX. This authorizes no attempt.

---

## 1. Result

```
warmup       15000/15000  recv 15000  missing 0  unknown 0  fail 0  p99  8.7 ms
measurement  30000/30000  recv 30000  missing 0  unknown 0  fail 0  p99  8.5 ms

sink     45000 rows / 45000 unique / 0 duplicates / 0 late / 0 malformed
history  45000 rows / 45000 unique
workload_integrity valid=true findings=[]        dropped_iterations 0
generator_resource_qualified true (all 10 checks)   saturation false
setup gate PASS   1000/1000 devices on both replicas, attempt 1, 0 repair restarts, 0 panics
```

This is the cleanest of the three platforms by a wide margin. Dispatch-to-receipt
p99 of 8.5 ms sits next to ThingsBoard's 347 ms and FIWARE's 35,338 ms.

## 2. The blocker was overstated by roughly 2×

The recorded blocker (`edgex-w-batch-blocker.md`) projected readiness at
**~468 s** from a 46 ms/device profile-fetch model, against a **180 s** deadline.

Measured directly on this host, before spending the run ID:

| Quantity | Recorded | Measured |
|---|---:|---:|
| Readiness route (405) after N=1 boot | ~468 s (projected) | **224 s** |
| Scale to N=2, both replicas ready | "fails the deadline" | **~216 s**, attempt 1 |
| Historical default vs real need | 180 s vs 468 s | 180 s vs 224 s |

The projection was linear extrapolation from a 9k-era observation. The real curve is
flatter. **The direction was right; the magnitude was wrong by about 2×.**

Two further facts mattered:

- `EDGEX_DRIVER_READY_TIMEOUT=180` is an **env-var default inside the helper**, not a
  frozen scope value. `grep` for it in the launcher returned zero hits — the launcher
  never set it, so it silently used 180 while the real requirement is 224.
- Inventory fits: 10,192 + 6,000 reserve = **16,192 ≤ 16,384**.

**Decision taken:** the launcher now exports explicit setup budgets
(`EDGEX_DRIVER_READY_TIMEOUT=600 EDGEX_SCALE_TIMEOUT=600 EDGEX_PING_TIMEOUT=180`).
This is a **setup wait only**. Devices (1,000), rate (50/s), duration (300+600 s),
payload (`size_offset_bytes=0`), VUs (300), replicas (2) and cpusets (4-5 / 2-3) are
unchanged.

**What this does not do:** it does not fix the underlying O(N) per-device profile
fetch. Setup cost still grows with retained inventory, which is exactly why **rep 2
onward remains blocked by the 16,384 cap** (11,192 + 6,000 > 16,384 after this run).
The real fix — deduplicating the profile fetch — is still the right long-term answer.

## 3. What happened operationally (disclosed, not hidden)

The workstation hosting the launcher was restarted **twice** by infrastructure while
EdgeX activation was in progress. Timeline:

| Stage | Outcome |
|---|---|
| Provision 1,000 devices at N=1 | completed |
| Scale to N=2, gate both replicas | **PASSED** — 1000/1000 each, attempt 1, 0 panics |
| Traffic | **never started** — launcher killed |

The dead launcher left an empty load lock. It was verified to have zero owner
processes, zero sink listeners, zero containers and no owner file, then released by
`rmdir`; recorded in `lock-recovery.json`.

**How the measurement was then obtained without misrepresenting it:** the deployment
was already in the exact intended pre-traffic state, and the EdgeX adapter has a
designed pre-provisioned path (`EDGEX_PREPROVISIONED_RUN_ID`) intended for precisely
this case — it *requires* the cohort to exist and creates nothing (it aborts if a
device is missing). Traffic was launched against that gated deployment.

**Why the measurement is still sound:** the interruption was **between provisioning and
traffic**, never during traffic. The generator, sink, sampler and capture ran as one
continuous, unbroken capture: 45,000/45,000 dispatched, one ledger, one capture window,
no restart. The integrity evidence proves this — `same_container`, `fresh_sampler`,
and a complete 45,000-row ledger.

**Why this is not a retry of a failed attempt:** no measurement occurred before the
interruption, so nothing failed. The setup gate had passed. The run ID was consumed
exactly once, by exactly one traffic window.

## 4. Inventory consequence

| | Value |
|---|---:|
| Retained before | 10,192 |
| Provisioned this run | 1,000 |
| Retained after | **11,192** |
| Cap (metadata / driver) | 16,384 |
| Next attempt would need | 11,192 + 6,000 = 17,192 → **exceeds cap** |

So this was **the only EdgeX attempt that fit**. A further EdgeX run needs an owner
decision on the discovery limit — Option 1 from `edgex-w-batch-blocker.md` (freeze a
common limit ≥ 41,192 and fix the init path), which is a batch of its own.

## 5. Where W1 now stands

| Platform | Verdict | p99 (measurement) | Notes |
|---|---|---:|---|
| ThingsBoard | **`VALID`** | 347 ms | warm start |
| FIWARE | **`INVALID_PLATFORM_FAILURE_OR_BACKLOG`** | 35,338 ms | 13,481/30,000 timed out; 20 lost |
| EdgeX | **`VALID`** | **8.5 ms** | clean, both history and dispatch |

**W1's full-duration 50/s generator/capture gate is qualified on two of three
platforms, and honestly measured-and-failed on the third.** That is materially
different from "W1 is cleared": FIWARE does not absorb 50/s at W1 shape.

Batch release is untouched:

```
execution_authorized : false
matrix status        : PROPOSED_NOT_LAUNCH_CLEARED
all 60 rows          : BLOCKED_PENDING_VARIANT_RELEASE
```

## 6. Artifacts

- `results/workload-batch-20261108/evidence/w1-qual-ex-20261110e/qualification-result.json`
- `results/workload-batch-20261108/evidence/w1-qual-ex-20261110e/lock-recovery.json`
- `results/workload-batch-20261108/evidence/w1-qual-ex-20261110e/edgex-gate-statuses/` — gate, inventory, discovery, per-replica HTTP statuses
- `results/workload-batch-20261108/reports/` — `w1-latency-percentiles.svg`, `w1-ack-completeness.svg`, `w1-throughput-timeline.svg`, `w1-summary.md`
- `scripts/render-w1-charts.py` — regenerates the charts from evidence

## 7. Related

- `edgex-readiness-measured-20261111.md` — the 224 s measurement and the setup-budget decision
- `edgex-w-batch-blocker.md` — the document this corrects on readiness magnitude
- `w1-qualified-thingsboard-20261111.md` — TB `VALID` and its warm-start caveat
- `w1-fiware-fails-at-50s-20261111.md` — the FIWARE platform finding
