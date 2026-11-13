# EdgeX readiness, measured: 224 s, not the projected 468 s

**Date:** 2026-11-11
**Trigger:** owner instruction to run the EdgeX measurement if all requests can be sent and measured.
**Outcome:** the recorded blocker was **overstated by 2×**. Readiness is a ~44 s setup-wait
shortfall, not a wall. One attempt was therefore made available; whether to spend it is
recorded below.

---

## 1. What I measured (live, not projected)

The blocker document `edgex-w-batch-blocker.md` projected `InitCache` at ~46 ms/device,
giving **~468 s at 10,192 devices** against a 180 s deadline. That projection was never
measured at this inventory. So I measured it directly on the load-free SUT.

| Probe | Measured |
|---|---:|
| `compose up` at N=1 to containers up | 13 s |
| first `device-rest` ping | 15 s after boot |
| readiness route answers **405** (ready) | **224 s** after boot |
| scale to N=2, `device-rest-1` ready | 211 s |
| scale to N=2, `device-rest-2` ready | 0 s (already warm) |
| scale + both replicas ready, total | **216 s** |
| core-metadata restarts / panics after probe | **0 / 0** |

**224 s measured vs 468 s projected.** The projection was extrapolated linearly from a
9k-era observation; the real curve is flatter. The direction was right, the magnitude was
wrong by roughly 2×.

## 2. Inventory does not block W1

| | Value |
|---|---:|
| Retained (live `totalCount`) | **10,192** |
| `MaxResultCount` (metadata / driver) | 16,384 |
| Reserve the launcher sets | 6,000 |
| 10,192 + 6,000 | **16,192** |
| Headroom | **+192 — passes** |

So W1 fits. Rep 2 onward does not (−808), which is unchanged and still true.

## 3. What the real blocker was

The 180 s figure is **not a frozen scope value**. It is a default inside the helper:

```bash
local base=$1 timeout=${2:-${EDGEX_DRIVER_READY_TIMEOUT:-180}}
SCALE_TIMEOUT="${EDGEX_SCALE_TIMEOUT:-180}"
PING_TIMEOUT="${EDGEX_PING_TIMEOUT:-120}"
```

`grep` for `EDGEX_DRIVER_READY_TIMEOUT` in the launcher returned **zero** hits — the
launcher never set it, so it silently used 180 while the real requirement is 224.

**Decision taken:** the launcher now exports explicit setup budgets
(`EDGEX_DRIVER_READY_TIMEOUT=600 EDGEX_SCALE_TIMEOUT=600 EDGEX_PING_TIMEOUT=180`).

**What this does and does not do:**

- It **is** a setup wait. It changes nothing about the measurement: devices 1,000,
  rate 50/s, 300+600 s, payload `size_offset_bytes=0`, 300 VUs, `EDGEX_REPLICAS=2`,
  `POOL_CPUSET=4-5`, `INFRA_CPUSET=2-3` are all untouched.
- It **does not** fix the underlying O(N) per-device profile fetch. It only stops a
  working-but-slow boot from being scored as a readiness timeout. The cost still grows
  with retained inventory, which is why rep 2 onward stays blocked on the cap, and why
  the real fix (dedupe the profile fetch) is still the right long-term answer.

This is recorded as a **changed decision**, not a silent reinterpretation. The prior
document's conclusion — "raising the deadline alone papers over an O(N) cost" — remains
correct about the *cost*; what was wrong was the claim that the deadline could not be met
at all at this inventory. It can: 224 s.

## 4. Honesty about the probe

The probe was run against the live SUT to measure the timing. It started the stack, scaled
it, and was then stopped with volumes preserved. Afterwards:

```
core-metadata restarts: 0   panics: 0   retained: 10,192 (unchanged)   edgex running: 0
```

No device cohort was created or deleted, so the inventory and the one-shot W1 headroom are
intact. The probe consumed no run ID.

## 5. What this means for the W1 EdgeX attempt

`w1-qual-ex-20261110e` is **unconsumed** and is the **only** EdgeX attempt that fits the
inventory cap. It is therefore a one-shot: if setup fails, it is preserved as a failure and
rep 2+ remains blocked on the cap, so there is no successor without an owner cap decision.

That is why the launch was **not** fired unilaterally. The measurement is now *possible*;
spending the one-shot is a decision with a real cost if it goes wrong.

## 6. Related

- `edgex-w-batch-blocker.md` — the document this corrects on the readiness magnitude
- `edgex-ordinal-15-readiness-failure.md` — the measured ~270 s route appearance (consistent with 224 s)
- `edgex-discovery-limit-and-inventory-scaling.md` — the cap research
- `results/feasible-campaign-20261106/run-feasible-campaign.py` — setup budgets now explicit
