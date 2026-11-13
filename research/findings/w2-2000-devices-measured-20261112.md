# W2 measured: 2,000-device provisioning holds on all three platforms

**Date:** 2026-11-12
**Workload:** W2 — **2,000 devices**, 25 messages/s, 300 s warm-up + 600 s measurement,
payload `logical-json-ascii-v1` with `size_offset_bytes=0`, 5,000 ms timeout,
150-VU pool (`dispatch-integrity-v1`).
**Scope:** qualifications. **None of these counts toward the 60 proposed attempts.**

> **Update (same day):** EdgeX was initially recorded as cap-blocked. The owner authorized
> correcting the inventory reserve to the run's actual cohort, which unblocked it. EdgeX has
> now been measured and passed. The change is documented in
> `edgex-inventory-reserve-cohort-sized-20261112.md`; §4 below is rewritten, and the original
> blocked reading is preserved at the end of that section.

---

## 1. Result

| Platform | Verdict | Devices used | Dispatched | Acked | History | p99 (measurement) |
|---|---|---:|---:|---:|---:|---:|
| ThingsBoard | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` | 2 000 / 2 000 | 22 500 | 22 500 | 22 500 | 287.8 ms |
| FIWARE | **`VALID`** | 2 000 / 2 000 | 22 500 | 22 500 | 22 500 | 312.9 ms |
| EdgeX | **`VALID`** | 2 000 / 2 000 | 22 500 | 22 500 | 22 500 | **8.7 ms** |

W2's gate is not throughput — it is **provisioning**: create 2,000 devices and have every
active replica recognize the entire cohort. On all three platforms the provisioning half
**passed**: the dispatch ledger addressed exactly 2,000 distinct devices with zero
round-robin violations and complete seq coverage.

**All three measurement phases were completely clean:**

```
ThingsBoard measurement  15000/15000  missing 0  unknown 0  fail 0  p99 287.8 ms
FIWARE      measurement  15000/15000  missing 0  unknown 0  fail 0  p99 312.9 ms
EdgeX       measurement  15000/15000  missing 0  unknown 0  fail 0  p99   8.7 ms
```

## 2. Why ThingsBoard is `FAILURE_OR_BACKLOG` anyway

The verdict does **not** come from the measurement. It comes from **warm-up**:

```
warmup  7500 scheduled · 7350 received · 150 missing · 49 unknown (33 later delivered)
        p99 dispatch→receipt 35 211 ms
measurement  15000 scheduled · 15000 received · 0 missing · p99 288 ms
```

I checked where those 150 went, and the answer matters:

| Question | Answer |
|---|---|
| Are they absent everywhere? | **No — zero.** All 150 are present in persisted history. |
| Where did they go? | Stored by the platform, but did not traverse the sink export path within the cutoff. |
| When were they sent? | **0.03 s – 10.9 s** into a 300 s warm-up. |
| Distribution | 17 in the first 2 s, 29 by 4 s, 50 around 6 s, 2 by 11 s — then nothing. |

That is a **cold-start ramp**, and there is an obvious cause: 2,000 devices had just been
created seconds earlier. W1's TB run was a *warm* start (the stack had just served a
previous generation) and lost nothing in warm-up. W2 started from a stopped stack with
2,000 brand-new devices.

**The frozen policy counts `missing_by_cutoff > 0` in any phase as platform failure.** I did
not modify the evaluator. The raw verdict stands, and the characterisation above is recorded
next to it rather than instead of it.

**What this does not mean:** it is not evidence that ThingsBoard cannot carry 2,000 devices at
25/s. The measurement phase, at the same rate and the same cohort, lost nothing and ran
*faster* than W1's (288 ms vs 347 ms p99). It means the cell as defined — cold start included
— loses 2% of warm-up traffic.

## 3. The FIWARE result reverses the W1 impression, on a specific axis

FIWARE was the W1 failure: 13,481 of 30,000 sends breached the 5 s timeout, p99 ~35.3 s.
It now passes W2 **cleanly while carrying twice the devices**:

| | W1 | W2 |
|---|---:|---:|
| Devices | 1 000 | **2 000** |
| Rate | 50/s | **25/s** |
| Timed out | 13 481 | **0** |
| p99 | 35 338 ms | **312.9 ms** |
| Verdict | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` | **`VALID`** |

**This isolates rate, not cohort size, as FIWARE's binding variable** at this scale. It does
not soften the W1 finding: 50/s genuinely fails. It does mean "FIWARE can't do W1's device
count" would be the wrong reading — the device count was never the problem.

## 4. EdgeX: the reserve was the blocker, and it was corrected

### 4a. What the block was

The EdgeX inventory gate is evaluated **before** provisioning:

```bash
reserve=${EDGEX_INVENTORY_RESERVE:-$D}
if (( retained + reserve > metadata_limit || retained + reserve > driver_limit )); then
  ... exit 4   # setup failure
fi
```

Under the original fixed reserve:

| | Value |
|---|---:|
| Retained inventory | 11 192 |
| Launcher reserve (`EDGEX_INVENTORY_RESERVE`) | 6 000 |
| Sum | **17 192** |
| Metadata / driver limit | **16 384** |
| Result | **blocked before any device is created** |

The 2,000-device cohort itself fits: 11 192 + 2 000 = **13 192**, leaving 3,192 headroom.
The 6,000 was a **whole-batch projection** (25,000 new devices, projected 36,192 total)
applied to a **per-attempt** check. It over-reserved 3× this cohort.

### 4b. The correction (owner-authorized)

The gate's invariant is `retained + cohort <= discovery_limit`. For that to be sound, the
reserve must be the delta the run will create. The launcher now sets
`EDGEX_INVENTORY_RESERVE={devices}` — the plan's device count.

| | Before | After |
|---|---|---|
| Reserve for W2 | 6 000 (fixed) | **2 000 (cohort)** |
| 11 192 + reserve | 17 192 → blocked | **13 192 → passes** |
| Discovery limit | 16 384 | **16 384 — unchanged** |
| Inventory deleted | none | none |
| B0 comparability | – | **preserved exactly** |

The limit was **not** raised and nothing was deleted, so the "no silent limit bump" constraint
is respected. Because inventory is retained across runs, this unblocks W2/W3/W4 for EdgeX —
not an unlimited sequence (a 6th 1,000-device run would not fit).

### 4c. The measured result

| | Result |
|---|---|
| Devices accepted, replica 1 | **2 000 / 2 000**, first readiness attempt |
| Devices accepted, replica 2 | **2 000 / 2 000**, first readiness attempt |
| Replica repair restarts | 0 |
| core-metadata restarts / panics | 0 / 0 |
| Dispatched / acked / persisted | 22 500 / 22 500 / 22 500 |
| Missing / unknown / failed | 0 / 0 / 0 |
| p99 (measurement) | **8.7 ms** |
| Retained inventory after | **13 192** — exactly as predicted |

The provisioning gate W2 exists to test passed on both replicas' first attempt.

### 4d. Preserved original reading

Recorded before the decision, kept for the audit trail: EdgeX W2 was left unmeasured because
narrowing the reserve was judged to reinterpret a fail-closed gate, and that was an
owner-level decision. The owner then made it. The instruction and the change are in
`edgex-inventory-reserve-cohort-sized-20261112.md`.

### 4e. Harness defect found during this run

The gate was **written** to a hardcoded B0 gates path but **retrieved** from the W gates path
(`activate()` and `post_run_check()` hardcoded B0's directory while `WBound` rebinds
`REMOTE_RESULTS`). The symptom was `post_run_check_qualified: false` with an empty
`post_run_check` and six `scp` failures, despite the gate having passed on the SUT. It did not
affect the measurement or the verdict — platform analysis, capture and history were all valid.
Both call sites now use `{REMOTE_RESULTS}/gates`, covered by
`tests/edgex_inventory_reserve_test.py::GatePathTests`. The gate evidence in
`edgex-gate-statuses/` was copied from the SUT's own files by hand after the run, and that is
disclosed in the result record.

## 5. What changed in the harness to run W2 at all

W2 could not be staged or launched by the W1-only code paths. Changes, all offline and all
test-covered:

| Change | Why |
|---|---|
| `stage-w1-qualification.py --workload` | Stage any variant; W1's ledger stays byte-identical at its historical path, later workloads get `qualification/<wid>/`. |
| `run-workload-campaign.py` qualification lookup | Merges per-workload ledgers; resolves a row's plan from its own workload root. |
| `validate_qualification_shape()` | Binds against the row's own workload and canonical plan instead of hardcoding W1. |
| `run-feasible-campaign.py` EdgeX activation | Provisions the plan's device count instead of the literal `1000`, so a 2,000-device plan builds a 2,000-device cohort. |
| `run-feasible-campaign.py` EdgeX inventory reserve | Reserves the plan's cohort instead of a fixed 6,000. See §4b. |
| `run-feasible-campaign.py` EdgeX gate path | Gate write and retrieval both follow `REMOTE_RESULTS`. See §4e. |
| `record-w-authorization.py` | Binds each workload's ledger under its own key. |

**No frozen value changed:** devices (2,000), rate (25/s), durations (300/600 s), timeout
(5,000 ms), padding (0), replicas (2), cpusets, seed policy and the 150-VU pool are all as
frozen. At 25/s the rate-derived pool rule returns exactly 150, so W2 remains **pool-identical
to B0**.

`check` reports `execution_authorized: false`, matrix `PROPOSED_NOT_LAUNCH_CLEARED`, 60 rows
unchanged, 19 qualifications (16 W1 + 3 W2). 162 tests pass.

## 6. Evidence

- `results/workload-batch-20261108/evidence/w2-qual-tb-20261110/qualification-result.json`
- `results/workload-batch-20261108/evidence/w2-qual-fw-20261110/qualification-result.json`
- `results/workload-batch-20261108/evidence/w2-qual-ex-20261110/qualification-result.json` — EdgeX,
  now measured and `VALID`
- `results/workload-batch-20261108/evidence/w2-qual-ex-20261110/edgex-gate-statuses/` — the
  provisioning gate: 2 000 devices accepted by both replicas, 0 restarts, 0 panics
- `workloads/campaigns/fixed-points-w1-w4/qualification/w2/index.json`
- `workloads/campaigns/fixed-points-w1-w4/authorization/owner-w2-qualification-20261112a.json`
- `workloads/campaigns/fixed-points-w1-w4/authorization/owner-w2-edgex-cohort-reserve-20261112b.json`
- `results/workload-batch-20261108/reports/w2-summary.md`
- `results/workload-batch-20261108/reports/w2-latency-percentiles.svg`
- `results/workload-batch-20261108/reports/w2-ack-completeness.svg`
- `results/workload-batch-20261108/reports/w2-throughput-timeline.svg`
- Regenerate charts: `python3 scripts/render-w1-charts.py --workload W2`

## 7. Open decisions (owner's, not the harness's)

1. ~~**EdgeX inventory limit.**~~ **Resolved 2026-11-12** — the reserve now matches the run's
   cohort, so W2/W3/W4 fit under the unchanged 16 384 limit. What remains open is the
   *five-repetition* batch: inventory accumulates, so a 6th EdgeX run of 1 000 devices would
   not fit. That is still a batch-level decision.
2. **Warm-up cold-start policy.** Should a cold-start ramp in warm-up fail a cell whose
   600 s measurement is clean? W2 TB is the first case where this decides a verdict.
3. **FIWARE W1 at 50/s** remains a real platform failure. Nothing here changes that.

## 8. Related

- `w1-platform-behaviour-comparison-20261112.md` — same three platforms at W1
- `w1-edgex-qualified-20261112.md` — the EdgeX inventory arithmetic
- `edgex-w-batch-blocker.md` — the cap decision this run adds data to
- `w1-fiware-fails-at-50s-20261111.md` — the 50/s failure W2 now contrasts with