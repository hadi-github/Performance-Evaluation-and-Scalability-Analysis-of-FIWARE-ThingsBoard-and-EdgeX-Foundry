# Finding: the frozen EdgeX inventory limit admits only 2 of W4's 5 EdgeX repetitions

**Status:** blocker for W4-EdgeX repetitions 3–5. No run was attempted; no ID was consumed.
**Date:** 2026-11-14

## What was found

W4 is a one-factor replica experiment with five repetitions per platform, so each
platform needs five attempts. ThingsBoard and FIWARE have no inventory cap.
EdgeX does.

The earlier decision
(`research/findings/edgex-inventory-reserve-cohort-sized-20261112.md`) changed the
pre-run gate from reserving a fixed 6,000 devices to reserving exactly the cohort
the run will create. That was correct and it unblocked W2, W3 and W4 — but it sized
**one** W4 EdgeX run, not five:

> "W4 at 15,192. A fifth EdgeX run of 1,000 devices would land at 16,192 and still
> fit (+192); a sixth would not."

That sentence counts *EdgeX runs in total*, not *W4 repetitions*. W4 needs five
EdgeX repetitions of its own.

## The live arithmetic

- Discovery limit (`core-metadata` and `device-rest`, unchanged): **16,384**
- Retained inventory after W3 EdgeX completed: **14,192**
- Cohort per EdgeX run: **1,000** devices

Inventory is deliberately preserved across runs, so each repetition permanently
consumes its cohort.

| Run | Retained | + cohort | Total | vs 16,384 |
|---|---:|---:|---:|---|
| `w4dp-ex-r01` | 14,192 | 1,000 | 15,192 | +1,192 — fits |
| `w4dp-ex-r02` | 15,192 | 1,000 | 16,192 | +192 — fits |
| `w4dp-ex-r03` | 16,192 | 1,000 | 17,192 | **−808 — blocked** |
| `w4dp-ex-r04` | 16,192 | 1,000 | 17,192 | **−808 — blocked** |
| `w4dp-ex-r05` | 16,192 | 1,000 | 17,192 | **−808 — blocked** |

**3 of 5 EdgeX repetitions cannot run under the frozen limit.**

## Why the cohort cannot simply be reused

EdgeX device identity is bound to the run ID, both when provisioned and when
verified:

- `deploy/edgex/scripts/provision-then-scale.sh` creates
  `bench-<RUN_ID>-<seq>` (`"bench-" + $run + "-" + ...`).
- `internal/analysis/protocol_cohort.go:146` independently recomputes the same
  expected name, `fmt.Sprintf("bench-%s-%05d", r.RunID, p.Device)`.

A second attempt therefore cannot address a previous attempt's devices: the
generator and the analysis both require the cohort to carry the current run ID.
Reusing a cohort would mean either driving devices whose names do not match the
run (the protocol-cohort check fails) or rewriting the identity rule for W4 alone
(which changes the measurement definition, not just its plumbing).

## Scope of the problem

| Platform | W4 repetitions | Constraint |
|---|---:|---|
| ThingsBoard | 5 | none |
| FIWARE | 5 | none |
| EdgeX | 5 | **2 runnable, 3 blocked** |

W4 as currently proposed is **12 of 15 attempts runnable**; 3 are blocked by a
frozen capacity limit that this campaign may not change.

## Why this is not being worked around

Every available workaround changes something frozen or fabricates evidence:

1. **Raise `MaxResultCount` to ≥ 17,192.** This is the option the 2026-11-12
   finding explicitly rejected ("does not silently raise the limit"), and it
   would change active-B0 discovery configuration.
2. **Delete inventory between repetitions.** Explicitly forbidden: inventory is
   retained by design, and deletion invalidates the preserved-history premise.
3. **Reuse the previous cohort.** Fails the protocol-cohort identity check; see
   above.
4. **Run fewer repetitions and call it W4.** That is a plan change and a capacity
   claim, not a measurement.

## What this does not affect

- ThingsBoard and FIWARE W4 repetitions: unaffected, all 5 each.
- W1/W2/W3 EdgeX qualification evidence already recorded: unaffected, consumed,
  never re-run.
- W3 EdgeX (2026-11-14): ran inside the limit and is the run that took retained
  inventory from 13,192 to 14,192.

## Owner decision required

Choose one, and the choice is recorded as an owner amendment rather than a silent
edit:

1. **Run 12 of 15 W4 attempts now** (TB 5 + FIWARE 5 + EdgeX 2), and record
   EdgeX reps 3–5 as `BLOCKED_INVENTORY_LIMIT` with this finding as the reason.
   The matrix stays at 60 rows; the 3 blocked rows stay unconsumed and honest.
2. **Defer all W4-EdgeX** until a discovery-limit decision is made, and run the
   10 TB+FIWARE attempts now.
3. **Authorise a discovery-limit change** as a separately recorded amendment,
   accepting that it departs from the frozen configuration and must be
   re-qualified.

Recommendation: **option 1**. It produces valid W4 evidence for 12 cells, keeps
every blocked cell visible instead of quietly dropping it, and does not touch a
frozen control file or delete retained evidence. It is also reversible: if the
limit is later raised, the three unconsumed IDs are still available.

## Verification

```
$ curl -fsS http://127.0.0.1:59881/api/v3/device/all?limit=1 | jq .totalCount
14192
```
