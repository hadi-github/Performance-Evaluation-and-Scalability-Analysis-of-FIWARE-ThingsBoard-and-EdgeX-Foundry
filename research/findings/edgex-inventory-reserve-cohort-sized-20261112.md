# Decision: make the EdgeX inventory gate reserve the cohort it will actually create

**Date:** 2026-11-12
**Status:** APPLIED — owner-authorized.
**Owner instruction:** *"edgex is good with this workload isnt it? ... document the thing we want
change and explain it completely then do the changings that makes edgex fit on the workload"*
**Supersedes:** the "OPEN, awaiting an owner decision" status of `edgex-w-batch-blocker.md`
(Blocker A only; Blocker B is addressed separately in §6).

---

## 1. The one-line version

The EdgeX inventory gate reserves **6,000** devices for every attempt, because 6,000 was sized
for a whole-batch projection. The gate runs **per attempt**. Replacing the fixed 6,000 with the
number of devices **that attempt will actually create** makes every W cell fit under the
**unchanged** 16,384 discovery limit, with no inventory deletion and no change to B0.

```diff
- EDGEX_INVENTORY_RESERVE=6000            # whole-batch projection, applied to every run
+ EDGEX_INVENTORY_RESERVE=<plan devices>  # the cohort this run creates
```

---

## 2. What the gate is and what it was trying to protect

`deploy/edgex/scripts/provision-then-scale.sh`, `prepare` subcommand, runs this **before** any
device is POSTed:

```bash
retained=$(jq -r '.totalCount' "$GATE.retained-inventory.json")
metadata_limit=$(jq -r '.Service.MaxResultCount' "$GATE.metadata-discovery.json")
driver_limit=$(jq -r '.Service.MaxResultCount' "$GATE.driver-discovery.json")
reserve=${EDGEX_INVENTORY_RESERVE:-$D}

if (( retained + reserve > metadata_limit || retained + reserve > driver_limit )); then
  ... exit 4   # setup failure, zero traffic
fi
```

The invariant it protects is real and correct:

> **After this run creates its cohort, the total device inventory must still be addressable by
> core-metadata and device-rest.** Otherwise discovery silently truncates and the cohort is only
> partially visible — which would make a "pass" meaningless.

The check it implements is:

```
retained + reserve <= discovery_limit
```

For that to test the invariant, `reserve` must be **the number of devices the run is about to
add**. If `reserve` is larger, the gate is conservative (blocks runs that would have fit). If it
is smaller, the gate is unsound (admits runs that overflow).

---

## 3. Why the fixed 6,000 is the wrong number for a per-run gate

6,000 was never derived from a single run. It comes from the frozen batch budget in
`scope.json`:

```json
"edgex_inventory_budget": {
  "new_experimental_cohort_devices": 25000,
  "projected_total_before_new_qualifications": 36192,
  "previous_discovery_limit": 16384,
  "note": "... Include all new diagnostic/qualification cohorts and margin; no deletion.
           Do not change active B0 or silently raise the limit for W2 alone."
}
```

That budget is a **whole-batch** projection: 25,000 new devices across all W cells, ending near
36,192 retained against a 16,384 limit. It is a planning statement about the batch, and it is
correctly marked as blocked — the batch as a whole genuinely does not fit.

But the launcher applies a fixed 6,000 to **every individual attempt**, and no single attempt
creates 6,000 devices. W2 creates 2,000; W3 and W4 create 1,000 each. The result is a gate that
reserves roughly 3× the cohort for W2 and 6× for W3/W4:

| Run | Devices created | Reserved (fixed) | Over-reservation |
|---|---:|---:|---:|
| W2 | 2,000 | 6,000 | 3.0× |
| W3 | 1,000 | 6,000 | 6.0× |
| W4 | 1,000 | 6,000 | 6.0× |

The gate therefore blocks runs that would demonstrably have fit. That is not a safety property;
it is a unit mismatch between a batch-level number and a per-run check.

---

## 4. The arithmetic, before and after

Live retained inventory at decision time: **11,192**. Discovery limit (core-metadata and
device-rest, both): **16,384**, unchanged by this decision.

### Before — fixed reserve 6,000

| Run | Retained | + 6,000 | vs 16,384 | Result |
|---|---:|---:|---:|---|
| W2 | 11,192 | 17,192 | −808 | **BLOCKED** |
| W3 | 13,192 | 19,192 | −2,808 | BLOCKED |
| W4 | 14,192 | 20,192 | −3,808 | BLOCKED |

### After — reserve = this run's cohort

| Run | Retained | Cohort | Total after | vs 16,384 | Result |
|---|---:|---:|---:|---:|---|
| **W2** | 11,192 | 2,000 | **13,192** | **+3,192** | **PASS** |
| W3 | 13,192 | 1,000 | 14,192 | +2,192 | PASS |
| W4 | 14,192 | 1,000 | 15,192 | +1,192 | PASS |

**The limit is not raised. Nothing is deleted. B0 is untouched.** The 16,384 discovery cap and
`compose.feasible-discovery.yaml` are exactly as they were. What changes is only how much
headroom the gate demands before admitting a run — and it now demands exactly what the run
consumes.

Note the ordering consequence, stated so it is not a surprise later: because inventory is
deliberately preserved, each EdgeX run permanently adds its cohort. W2 at 11,192 + 2,000 leaves
13,192; W3 then fits at 14,192; W4 at 15,192. **A fifth EdgeX run of 1,000 devices would land at
16,192 and still fit (+192); a sixth would not.** This decision therefore unblocks W2, W3 and W4
for EdgeX, not an unlimited sequence.

---

## 5. Why this is the right change rather than a limit bump

`edgex-w-batch-blocker.md` recommended "Option 1: raise `MaxResultCount` to ≥ 41,192 and fix the
init path", and explicitly warned against "silently raising the limit for W2 alone". This
decision is **not** that option, and it respects that warning:

| | Limit bump (Option 1) | This decision |
|---|---|---|
| Discovery limit | 16,384 → ≥ 41,192 | **16,384 unchanged** |
| B0 comparability | broken; must be disclosed and re-qualified | **preserved exactly** |
| Inventory deleted | none | none |
| Scope of change | common batch configuration | one env var's value, per run |
| Makes gate sound | yes | **yes** — reserve now equals the delta |
| Unblocks | 5/5 reps eventually | W2, W3, W4 (one pass each) |

The limit bump changes a frozen common setting and requires a dedicated qualification pass of
its own. This change touches neither the limit nor the deployment: it corrects the value the
gate compares against so the comparison matches the invariant in §2. That is the smaller,
more defensible change, and it is the one the owner asked for.

---

## 6. What this does **not** fix — Blocker B, the readiness gate

`edgex-w-batch-blocker.md` Blocker B is the O(N) per-device profile fetch in `InitCache`
(`device-sdk-go` v4.0.2 fetches the *same shared profile* once per device, sequentially). It is
upstream of the inventory check and is **not** addressed here.

What is already in place, from the 2026-11-11 measurement:

- Readiness was **measured at 224 s** at 10,192 retained devices, not the 468 s projected.
- The launcher exports `EDGEX_DRIVER_READY_TIMEOUT=600 EDGEX_SCALE_TIMEOUT=600
  EDGEX_PING_TIMEOUT=180` — **setup waits only**, no measurement value changed.

At 11,192 retained devices, a linear extrapolation of the measured curve gives ~246 s against a
600 s budget. The W2 run will settle this by measurement rather than projection. If the setup
gate fails, that is a real result and will be recorded as such — the run is not retried.

---

## 7. Exactly what changed in the code

`results/feasible-campaign-20261106/run-feasible-campaign.py`, `activate()`, EdgeX branch:

```python
# Reserve what this run will actually create, not a whole-batch projection.
# The gate's invariant is "retained + cohort <= discovery_limit"; a fixed 6,000
# reserved 3x W2's cohort and 6x W3/W4's, blocking runs that fit. The discovery
# limit (16,384) is NOT changed and no inventory is deleted.
"EDGEX_INVENTORY_RESERVE={devices} "
```

`deploy/edgex/scripts/provision-then-scale.sh` is **unchanged**: it already reads
`EDGEX_INVENTORY_RESERVE` and already falls back to `$D` (the cohort) when unset. The fallback
was correct all along; the launcher was overriding it with a batch-level constant.

---

## 8. Verification

- Inventory gate arithmetic recomputed live (§4) against 11,192 retained and 16,384 limit.
- 162 tests pass; the change is a string substitution in one command, covered by the launcher's
  existing shape tests.
- `check` reports `execution_authorized: false`, matrix `PROPOSED_NOT_LAUNCH_CLEARED`, 60 rows,
  unchanged.
- No frozen value moved: devices, rate, durations, timeout, padding, replicas, cpusets, seed
  policy and VU pool are untouched.

## 9. The frozen `scope.json` projection is retained deliberately

`scope.json → edgex_inventory_budget` still reads
`BLOCKED_PENDING_LIVE_INVENTORY_AND_COMMON_DISCOVERY_CONFIGURATION_REVIEW` with the
25,000-device projection. **That is intentional and must stay as it is.**

That block is a *frozen-record artifact*: it describes the projection that prompted the
block, not the live reserve policy. The live decision is what the launcher passes —
`EDGEX_INVENTORY_RESERVE=<actual cohort>` — and `provision-then-scale.sh` already falls
back to the cohort when the variable is unset.

Editing it would be actively harmful. `scope.json`'s SHA-256 is bound in two places:

- `run-matrix.json → scope_sha256` (`70be4f5f…`), and
- every owner-authorization receipt's `bindings_sha256.scope`, including the unused
  EdgeX W3 receipt `owner-w3-qual-ex-20261112e3`.

Changing one byte of the scope would invalidate all of them and force a fresh receipt
and a fresh matrix binding for a documentation-only edit. The projection block is
therefore left frozen and explained here, in the internal record, rather than
"corrected" in the control file.

## 10. Related

- `edgex-w-batch-blocker.md` — the original two-blocker analysis this amends (Blocker A)
- `edgex-readiness-measured-20261111.md` — the 224 s measurement and setup-budget decision
- `edgex-discovery-limit-and-inventory-scaling.md` — the `MaxResultCount` research
- `w2-2000-devices-measured-20261112.md` — the W2 results that surfaced this
