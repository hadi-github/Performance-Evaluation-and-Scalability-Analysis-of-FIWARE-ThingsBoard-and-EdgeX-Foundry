# Decision: the generator VU pool is rate-derived, not a fixed 150

**Date:** 2026-11-10 (evidence completed 2026-11-11)
**Status:** APPLIED. Supersedes the "W1 is not qualifiable under frozen policy" conclusion.
**Authority:** owner instruction to raise the pool when the binding limit is a
framework artifact rather than a platform limit.

---

## What was actually wrong

The frozen policy set `vus_preallocated_per_scenario = vus_max_per_scenario = 150`
for **every** arrival rate. That constant is correct at 25/s and structurally too
small at 50/s.

The generator is an open-loop constant-arrival executor. At rate `r` with client
timeout `t` it must hold `r x t` requests in flight at once. If it cannot, it
starves itself, and k6 reports that as `dropped_iterations` — an outcome that is
**indistinguishable in the ledger from loss the platform caused**.

```
50/s x 5 s = 250 concurrent requests required
frozen pool          = 150
shortfall            = 100 concurrent requests
```

This was never a platform property. It was arithmetic about our own harness, and
the two v1 qualifications proved it in the ledger:

| Run | Recorded evidence | Meaning |
|---|---|---|
| `w1-qual-tb-20261110c` | `implied_in_flight_at_p99: 243` vs 150 pool; 134 drops, all in warm-up; **measurement 30,000/30,000, p99 212 ms** | the platform served the rate fine once warm; the generator could not keep up while it was cold |
| `w1-qual-fw-20261110c` | 65 drops in a single 30 s stall; `send lag p50 = 2 ms` otherwise; **measurement 0 drops** | same conclusion, different transient |

Both platforms were healthy in measurement. The drops lived entirely in the
window where concurrency demand peaked. That is the signature of a starved
executor, not of a saturated platform.

**Corroborating control:** B0 ran the same generator at 25/s with the same 150
pool and recorded `vus_peak` of **125** (ThingsBoard) and **16** (FIWARE) with
zero drops. The 150 cap never bound a single B0 cell.

## The rule

```
bound = rate_per_s x request_timeout_ms / 1000
pool  = ceil(bound x 1.10 / 50) x 50
```

| Rate | Bound | Pool | Unchanged? |
|---:|---:|---:|---|
| 25/s | 125 | **150** | **identical to B0** |
| 50/s | 250 | **300** | W1 only |
| 100/s | 500 | **550** | not used by this batch |

The **10% headroom is evidence-based, not cosmetic.** B0's own 25/s cells peaked
at **126 VUs** against a theoretical 125. A pool of exactly `rate x timeout` sits
on the boundary and would flip to INVALID on ordinary jitter, so an exact-equal
pool would be a latent flake in every future 25/s run.

`scripts/pool_policy.py` is the single source of truth. The numbers are *also*
written out explicitly in a frozen table, and a test asserts the formula
reproduces that table, so a change to the arithmetic cannot silently move a
frozen number without touching the table too.

## Why this does not break comparability

The rule was chosen specifically so the amendment is a no-op for everything
except the rate that was broken:

- **All 45 W2/W3/W4 plans** (25/s) keep `vus = 150/150` byte-identically. Verified:
  `vus`, `schedule` and `health` are unchanged in every one; only the
  human-readable `deviations` note differs.
- **All B0 evidence** stays on `dispatch-integrity-v1` and re-evaluates to the
  same verdict. Verified against `disp-fw-r05-20261106`: `valid: true`,
  `policy: dispatch-integrity-v1`.
- **Only W1 (50/s)** moves 150 → 300. W1 differs from B0 in arrival rate anyway,
  so no fixed-point comparison is broken — a 50/s cell was never comparable to a
  25/s cell.

## Policy versioning

A new label `dispatch-integrity-v2` carries the rule. `v1` is **retained
unchanged** so historical runs cannot be reinterpreted: it still answers 150 for
any rate, which is what it meant when those runs were scored.

v2 is gate-identical to v1 in every other respect — identity, timing, loss, bin,
gap, lag-integrity, the 25/s and 50/s bins. Only the pool rule differs.

Code changed to honour both labels:

- `scripts/pool_policy.py` (new) — the arithmetic and frozen table.
- `scripts/dispatch-integrity.py` — `_check_pool` dispatches on the plan's policy;
  rejects a pool from a different rate, a too-tight pool, and an unqualified rate.
- `internal/plan/plan.go` — `IsDispatchIntegrity()` accepts v1 or v2.
- `internal/runner/runner.go` — progress observation now triggers for v2 too
  (previously v1-only, which would have silently blinded v2 runs).
- `internal/report/report.go` — send-lag stays diagnostic-only under v2. Without
  this fix a v2 run would have been re-scored on the **withdrawn** universal 10 ms
  send-lag rejection rule.
- `scripts/protocol-receipts.py`, `scripts/run-protocol-capture.sh` — v2 runs are
  dispatch-integrity runs, not "unsupported workload policy".

Those last four are latent-bug fixes: the label was already in use before the
binary and the report understood it.

## What is *not* claimed

- This is **not** a platform capacity claim. It says nothing about maximum
  throughput, scaling, or headroom above the fixed points.
- It does **not** authorize a higher arrival rate. The rates stay frozen at the
  four fixed points.
- It does **not** reinterpret the two v1 negatives. They remain valid records of
  `INVALID_VU_POOL_BOUND`; they measured the harness. They are **not retried** —
  a new generation (`…-20261110d`, then `…-20261110e`) was staged under new IDs.

## Measured outcome (ThingsBoard, `w1-qual-tb-20261110d`, 300 VUs)

The amendment resolves the failure it was aimed at. **The run itself is
`INVALID_HARNESS_LEDGER_MODE`, not `VALID`** — a separate defect in the
generator's ledger switch made it unscorable (see
`v1-only-constants-broke-v2-runs.md`). The *measurement* underneath is clean and
is what the table below reports.

| | v1 @ 150 VUs | v2 @ 300 VUs |
|---|---|---|
| Verdict | `INVALID_VU_POOL_BOUND` | `INVALID_HARNESS_LEDGER_MODE` (harness, not platform) |
| Dropped iterations | 134 | **0** |
| Send lag p99 | 2,682 ms | **6.0 ms** |
| Warm-up sends | 14,917 / 15,000 | **15,000 / 15,000** |
| Measurement sends | 30,000 / 30,000 | 30,000 / 30,000 |
| Warm-up ack p99 | 6,042 ms | 3,969 ms |
| Measurement ack p99 | 212 ms | 195 ms |
| Dispatch ledger | 44,917 / 45,000 | **406 / 45,000** (compact path; fixed in gen.js) |

The send lag collapsing from 2.68 s to 6 ms is the direct proof: the generator
was behind schedule because it had too few workers, and with enough workers it is
on schedule to within single-digit milliseconds. The platform's own warm-up
latency (3.97 s ack p99) is now visible as *platform* behaviour rather than
leaking out as generator drops.

ThingsBoard still has a genuine cold-start characteristic — a 3,969 ms warm-up
ack p99 against a 5,000 ms client timeout is real platform behaviour, and the
B0 warm-start delivery gap is unchanged by this amendment. What changed is that
the harness no longer *manufactures* additional failures on top of it.

The victory here is narrow and worth stating precisely: **raising the pool fixed
the thing it was meant to fix, and that fix immediately exposed two further,
unrelated harness defects that had been masked by the first** (the generator's
v1-only ledger switch, and a missing `pool_policy.py` in the staged script set).
All are now fixed and pinned by tests. The platform evidence from `d` stood; only
its ability to be *scored* was lost.

**Superseded outcome (2026-11-11):** the `e` generation, carrying both fixes,
returned **`VALID`** on ThingsBoard — `VALID_PLATFORM_SUCCESS`, 0 drops, 0 unknown,
45,000/45,000 ledger rows. See `w1-qualified-thingsboard-20261111.md`. The `d` ID
remains consumed and is never retried.

## Related

- `research/findings/w1-vu-pool-binds-both-platforms.md` — the two v1 negatives
  (still accurate as measurements; its "not qualifiable" conclusion is superseded)
- `research/findings/w1-headroom-gate-is-unresolvable.md` — the structural
  argument (superseded on the remedy, correct on the arithmetic)
- `workloads/campaigns/fixed-points-w1-w4/run-matrix.json` —
  `pool_policy_revision`, the machine-readable revision record
- `scripts/pool_policy.py` — the rule
- `tests/pool_policy_test.py` — 20 tests pinning the two safety properties
