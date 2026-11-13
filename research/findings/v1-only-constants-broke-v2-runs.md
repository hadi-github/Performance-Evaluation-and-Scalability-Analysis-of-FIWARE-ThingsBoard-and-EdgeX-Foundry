# Finding: three v1-only constants silently disabled v2 runs, and one of them faked a platform failure

**Date:** 2026-11-11
**Run:** `w1-qual-tb-20261110d` (qualification, not a proposed attempt)
**Verdict:** `INVALID_HARNESS_LEDGER_MODE` — **not a platform result.** Platform served the load with zero drops.
**Consumed ID.** Never reused. Successor generation is `e` with its own receipt.

---

## 1. What happened

`w1-qual-tb-20261110d` ran at 50/s with the amended 300-VU pool and the platform was
clean:

| Phase | Planned | Attempted | Dropped | Send lag p99 | Ack p99 |
|---|---:|---:|---:|---:|---:|
| warm-up 300 s | 15,000 | 15,000 | **0** | 6.0 ms | 3,969 ms |
| measurement 600 s | 30,000 | 30,000 | **0** | 6.0 ms | 195 ms |

The pool amendment worked exactly as your reasoning predicted: send lag collapsed from
**2,682 ms** at 150 VUs to **6.0 ms** at 300 VUs, with 134 drops becoming 0.

Then the analysis step refused the run:

```
ValueError: complete dispatch ledger required: 406/45000
```

The generator had written only 406 ledger rows. Those 406 were exactly the warm-up
`unknown` (first-contact 5 s timeouts). Every `ack_success` — all 44,594 of them — was
missing, because the generator had taken the *compact error-only* log path.

## 2. Root cause: a v1-only constant in the generator

`workloads/k6/gen.js:45` decided ledger mode like this:

```js
const COMPLETE_LEDGER = !!cfg.protocol_binding || cfg.health.workload_policy === 'dispatch-integrity-v1';
```

A `dispatch-integrity-v2` plan therefore fell through to the compact path. The comment
directly above the line stated the correct intent — *"Explicit dispatch-integrity plans
need a full per-message ledger"* — but the code named only v1.

The two earlier v1 qualifications never exposed this because `dispatch-integrity._check_pool`
raised on the VU pool **before** the ledger check ever ran. Raising the pool removed the
first failure and revealed the second. That is why a fix that worked perfectly still
produced an unscorable run.

### Verified, not assumed

I ran the real `gen.js` under the real k6 image locally against a local fixture ingester
(a 100-tick, 20/s probe):

| Source | Policy | Ledger rows |
|---|---|---:|
| pre-fix `gen.js` | v2 | **0** |
| fixed `gen.js` | v2 | **100 / 100** |
| fixed `gen.js` | v1 | **100 / 100** |

The pre-fix v2 case reproduces the live failure exactly: zero `ack_success` rows, because
all 100 probe ticks acked successfully and the compact path logs only non-successes.

## 3. Three more v1-only checks were found and fixed

This was a class of defect, not one line. Each one would have corrupted a v2 run:

| Location | Effect on a v2 run | Fix |
|---|---|---|
| `workloads/k6/gen.js:45` | Compact error-only ledger → unscorable | Consult a policy list matching `internal/plan.IsDispatchIntegrity` |
| `internal/runner/runner.go` | Progress observation lost (v1-only check) | Accept both revisions |
| `internal/report/report.go` | Re-scored on the withdrawn 10 ms send-lag gate | Accept both revisions |
| `results/feasible-campaign-20261106/run-feasible-campaign.py` | `run-result.json` labelled the run `workload_policy: dispatch-integrity-v1` and `run_kind: ordinary` | Derive both from the run's own `resolved.json` |

The fourth is worth calling out because it produced a **false record**. The launcher wrote:

```json
"run_kind": "ordinary",
"workload_policy": "dispatch-integrity-v1"
```

for a run that was a `dispatch-integrity-v2` qualification. `SCOPE["workload_policy"]` is
v1 by construction (it is B0's frozen scope), and `qualification_run_ids()` only listed the
pre-20261108 B0 qualification names. Had this gone unnoticed, the aggregator would have
filed a v2 qualification as an ordinary B0-era run.

## 4. Preserved, not retried

- The run ID `w1-qual-tb-20261110d` is **consumed and never reused.**
- Its measurement evidence is real and kept: `qualification-result.json` records
  `INVALID_HARNESS_LEDGER_MODE` with the full per-step numbers.
- It is **not** evidence that ThingsBoard cannot serve W1. It is evidence that our
  generator was mis-configured for v2, plus a clean platform measurement underneath.
- The load lock it retained was released only after ownership verification
  (`lock-recovery.json`): cleanup verified, no containers, no listener, no processes.

## 5. Consequence for the other platforms

The defect was in shared code, so `w1-qual-fw-20261110d` and `w1-qual-ex-20261110d` would
have failed identically. Both were superseded **unused** rather than run and thrown away.
Generation `e` carries all three platforms under new IDs:
`w1-qual-{tb,fw,ex}-20261110e` at 300 VUs / v2.

## 6. Tests added

- `tests/offline/complete-ledger-policy.test.mjs` pins that both revisions enable the
  complete ledger and that the switch is not pinned to a single literal. It fails against
  the pre-fix source and passes against the fix.
- `tests/fixed_point_variants_test.py` was updated: it previously asserted W1's `vus` and
  `health` were byte-identical to B0, which the amendment intentionally changed. It now
  asserts B0-identity for W2/W3/W4 and the documented 300 VU / v2 policy for W1 only.

All 156 Python tests pass; `go build ./cmd/... ./internal/...` and `go test ./internal/...`
pass.

## 7. Non-claims

- This does not establish that W1 *will* qualify on any platform. It removes a harness
  reason it could not be scored.
- The 406 warm-up `unknown` on ThingsBoard are a platform warm-up behaviour (same warm-up
  receipt lag documented for B0), not generator starvation — zero iterations were dropped.
- No capacity claim is made from `vus_peak: 251`.
- No commit or push was made.
