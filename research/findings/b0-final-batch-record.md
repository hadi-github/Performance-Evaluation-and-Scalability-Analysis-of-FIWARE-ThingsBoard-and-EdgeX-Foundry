# B0 ordinary campaign — final batch record

Closed 2026-11-10. This is the closing statement for the frozen B0 matrix
(`results/feasible-campaign-20261106/run-matrix.json`, sha256 `44bbc3f0…`).
Fifteen ordinal cells, each attempted **exactly once**. Zero retries, zero
replacements, zero discarded failures.

## Tally

| Platform | Cells | Valid measurements | Preserved failures | Target | Shortfall |
|---|---:|---:|---:|---:|---:|
| FIWARE | 5 | **5** | 0 | 5 | **0** |
| ThingsBoard | 5 | **4** | 3 (ordinal 1 never offered traffic; r04 and r04a not assessed) | 5 | 1 |
| EdgeX | 5 | **2** | 3 (ordinals 7, 11, 15) | 5 | 3 |
| **Total** | **15** | **11** | **6** | 15 | 4 |

Measured, from `aggregate/runs.csv` regenerated from the SUT evidence root with
the bound `aggregate-runs.py` (`899cd79d…`):

```
edgex        attempted=5  measured=2  disp-ex-r01, disp-ex-r02
fiware       attempted=5  measured=5  disp-fw-r01..r05
thingsboard  attempted=7  measured=4  disp-tb-r02, r03, r04b, r05
```

ThingsBoard shows 7 run IDs for 5 cells because ordinals 4 and 4a were separate
non-measuring attempts before the measured `disp-tb-r04b`.

## FIWARE — 5/5, complete

| Run | Measurement p99 (dispatch) |
|---|---:|
| `disp-fw-r01-20261106` | 107.1 ms |
| `disp-fw-r02-20261106` | 122.4 ms |
| `disp-fw-r03-20261106` | 131.8 ms |
| `disp-fw-r04-20261106` | 274.7 ms |
| `disp-fw-r05-20261106` | 286.3 ms |

All five: 15,000/15,000 measurement-phase messages received, 0 missing, 0
unknown, 0 history-missing. The spread is real run-to-run variation and is
reported as such — percentiles are never pooled across runs.

## ThingsBoard — 4/5, with a documented limitation

| Run | Measurement p99 | Warm-up missing |
|---|---:|---:|
| `disp-tb-r02-20261106` | 203.1 ms | 200 |
| `disp-tb-r03-20261106` | 217.7 ms | 160 |
| `disp-tb-r04b-20261109` | 214.9 ms | 144 |
| `disp-tb-r05-20261106` | 281.4 ms | 126 |

The five-run target cannot be met: **ordinal 1 was never offered traffic** and is
not replaced. Four valid measured repetitions is the permanent ceiling for this
batch.

The five TB failures are **three different mechanisms**, and only one belongs to
ThingsBoard:

| Mechanism | Cells | Owner |
|---|---|---|
| First-contact egress stall, 30.9–39.3 s | r02, r03, r04b, r05 | ThingsBoard behaviour |
| Launched ~27 s into a ~215 s boot; HAProxy 503 | r01 | Harness (no app-readiness gate) |
| Diagnostic helper dead on `:18085` / aggregate `/snapshot` 503 | r04, r04a | Harness / infrastructure |

So ThingsBoard failed **4 for 4 whenever it was actually measured**, and was
never measured in the other three attempts because the harness stopped them
pre-traffic with zero messages offered. r04b is a reviewed supplement to ordinal
10 (`TB-R04-A2`), not a retry of a platform failure — r04 and r04a never reached
ThingsBoard.

All four measured runs share one signature: first sink arrival 30.9–39.3 s into
traffic with **zero receipts before t=30 s**, a warm-up deficit of 200/160/144/126
confined to the first 9.9–23.1 s of scheduled warm-up, a measurement phase that is
always **15,000/15,000 clean**, every sink-missing identity present in history, and
`dispatch_spill: 0`. The missing messages never arrive at all — the last receipt
lands within 0.1 s of the last scheduled send. Full analysis:
`research/findings/thingsboard-warmstart-delivery-gap.md`.

One evidence caveat, recorded rather than smoothed: **r03 is a valid measurement but
not a campaign-qualified cell.** Its in-band history export failed
(`EXPORT_FAILED`), so reconciliation was done by late retrieval instead
(`OK_LATE_RETRIEVAL`, 22,500 rows, `history_missing: 0` in both phases — the numbers
stand), and its post-run `effective-resources` snapshot timed out at 180 s on
`docker compose config`, leaving `sut_resource_qualified: False` and
`campaign_qualified: False`.

## EdgeX — 2/5, ended by a measured platform defect

Valid: `disp-ex-r01` (7.91 ms p99), `disp-ex-r02` (8.38 ms p99).

| Ordinal | Run | Failure |
|---:|---|---|
| 7 | `disp-ex-r03-20261106` | lifecycle/health mismatch (`'edgex', 'core-data'`) — gates passed, launch refused. **Post-campaign analysis: harness false negative**, not a platform failure — see `edgex-ordinal-7-harness-false-negative.md` |
| 11 | `disp-ex-r04-20261106` | second device-rest replica never registered its ingest route (180 s) |
| 15 | `disp-ex-r05-20261106` | **first** replica never registered its ingest route (180 s) |

Ordinal 15 is the important one and is fully documented in
`research/findings/edgex-ordinal-15-readiness-failure.md`. Root cause, measured
not inferred: `device-sdk-go` v4.0.2 `InitCache` fetches the single shared
profile **once per device**, sequentially — ~46 ms each, projecting ~468 s at
10,192 devices and ~1,618 s at 35,192, against a **180 s** deadline. The cost
grows with retained inventory, so each EdgeX ordinal was slower than the last.

This is **not** the `MaxResultCount` cap. The readiness gate runs before
provisioning and before the inventory arithmetic, so the cap never bound. The
cap bump proposed for the W batch is necessary but **not sufficient** for EdgeX.

## Integrity of the record

- Every failure above is **preserved in the matrix** and appears in
  `aggregate/runs.csv` with `measurement_valid` empty/None. Failures are never
  dropped, averaged or replaced.
- No failed attempt was retried under the same ID.
- Ordinal 15 created **zero traffic**: no run directory on the SUT, retained
  inventory unchanged at 10,192, no `bench-disp-ex-r05-*` devices.
- Percentiles are per-run. The summary reports min/median/max across runs; it
  never pools messages.
- The pre-regeneration aggregate is preserved as
  `runs.prior-20261109.csv` / `summary.prior-20261109.md`, superseded and not to
  be cited.

## Known limitations of this batch (stated, not hidden)

1. **EdgeX has 2 of 5 repetitions.** The platform defect is not a measurement
   artifact; it is a startup-path cost that the campaign's retained-inventory
   design makes monotonically worse.
2. **ThingsBoard has 4 of 5.** One repetition was never offered traffic. Of the five
   cells attempted, four were measured and all four showed the same delivery stall;
   the other three attempts (r01, r04, r04a) died pre-traffic on harness faults, so the
   4/5 shortfall is a harness outcome, not a platform one.
3. **`dispatch-integrity.py` differs between the local checkout and the executing
   tree.** The executing bytes (`d59876b3…`) were bound by every receipt and are
   what ran; the local copy (`335e8451…`) adds a prospective 100/s bin. The
   executable campaign is correctly described by the SUT tree.
4. **Ordinal 7 was a harness false negative** (post-campaign analysis). The
   platform passed every gate; the launcher refused on a cumulative
   `RestartCount` it should have treated as a baseline. The check is now fixed,
   but the cell cannot be recovered — one attempt per cell, no retry.

## Status

**B0 is closed.** No controller is running, no cell is pending, there is no
ordinal 16. The W1–W4 batch remains a separate, unlaunched proposal
(`execution_authorized: false`) and must not be appended to this matrix.
