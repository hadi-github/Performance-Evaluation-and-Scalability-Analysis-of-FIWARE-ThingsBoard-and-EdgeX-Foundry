# Measurement integrity (D33)

**Status: implemented locally and tested on synthetic fixtures. Validated once against preserved exploratory evidence.
Not run against a live platform. No verdict semantics changed.**
Recorded 2026-10-26 under D33 and D41 (ChatGPT under the owner's delegation).

Evidence labels: **[M]** measured fact; **[C]** measured fact conditional on an exporter; **[D]** derived from measured
facts; **[X]** proxy, not a per-message proof; **[U]** unavailable, shown as a limitation rather than estimated.

---

## 1. Observation points (D30, normative)

Each observation point proves something different. Reports must name the one they use.

| Point | Source | What it proves | What it does **not** prove |
|---|---|---|---|
| HTTP acknowledgment | generator outcome ledger (`records.ndjson`, absence of a non-success record = `ack_success`) | the platform accepted the request | delivery, storage |
| Sink delivery | external sink NDJSON, first `(run_id, seq)` receipt; same host as the generator (`scripts/run-with-sink.sh`) | the platform's outbound path delivered the message | that history stored it |
| Stored-row visibility | history export (`scripts/history-ids.sh` / `history-ids-audit.sh`) | a row with that identity is retrievable **now** | when it was committed |
| Durable commit | none per message | — | **not measured.** No per-message commit timestamp exists on any platform in this setup. |

Table counters (e.g. EdgeX `n_tup_ins`) and WAL statistics are **[X] proxies**. They are never reported as
per-message durability.

## 2. Components D33 requires, and their status

| # | Requirement (D33) | Implementation | Test | Status |
|---|---|---|---|---|
| 1 | Join acknowledged identities to sink receipts by `(run_id, seq)` | `internal/audit` `Analyse`: sink first receipt per identity, missing and orphan accounting | `TestAnalyseClassifiesMissingDuplicateAmbiguousDelayed` | **Implemented, tested** |
| 2 | Reconcile acknowledged and delivered sets with independently retrieved history | `Analyse` history set, missing and orphans; `benchctl audit -history` | same test; `TestEmptyHistoryExportStaysDistinguishable` | **Implemented, tested.** No FIWARE history exists yet (B5). |
| 3 | Count duplicates separately at sink and history | `sequences_with_extra_copies` / `extra_copies` at each point; `Clean` requires zero | same test | **Implemented, tested** |
| 4 | Detect and report ThingsBoard same-millisecond collisions | planned same-ms pairs per device (risk bound, **[D]**); stored same-timestamp groups (**[C]**, needs `device` and `ts`) | `TestCollisionRiskAndStoredEvidence` | **Implemented, tested.** Stored-evidence path needs a `device`/`ts` export: `scripts/history-ids-audit.sh` is **UNVERIFIED** on the SUT. Existing exports lack these fields, so the stored check reports *unavailable*. |
| 5 | PostgreSQL table counters are diagnostic proxies | labelled in this document; the audit does not consume table counters | — | **Labelling in place.** Existing persistence reports were not re-audited in this stage. |
| 6 | Distinguish HTTP ack, sink delivery, stored-row visibility, durable commit | §1 above; `Labels` map in every audit JSON | — | **Implemented as labels** |
| 7 | Primary delivery latency on the generator/sink shared host clock | latency = sink `recv_ms` − planned `scheduled_send_ms`; the sink runs on the load host (`scripts/run-with-sink.sh`) | `TestByStep…`, known-answer tests | **Implemented.** Shared-clock premise **[V]** from `run-with-sink.sh`. |
| 8 | Bounded clock-offset/drift procedure before any cross-host timing claim | §4 below | — | **Procedure defined; not executed** (no measured offset, B11) |

Correctness-qualified goodput (D36) is implemented as the set acknowledged **and** at the sink **and** in history,
within the deadline, per step and in aggregate. Sweeps are reported per step: the aggregate spans steps at different
offered rates, and the audit now says so in its limitations.

## 3. Local validation on preserved evidence (not a verdict)

Run: `benchctl audit` on `results/exploratory/EXPL-edgex-k1` (sink and history files preserved in `results/`; output
written to `/tmp/opencode`, nothing written under `results/`).

| Quantity | Value | Cross-check |
|---|---|---|
| Planned = acknowledged identities | 222,000 | `explore.json` `planned_messages` = 222,000 |
| Sink unique / missing | 202,815 / 19,185 | `explore.json` outstanding work at minute 3 of `s05-sweep-r500` ≈ 19,236 (consistent) |
| History unique / missing | 214,812 / 7,188 | `explore.json` `stored_unique` = 214,812, `ack_lost` = 7,188 (**exact**) |
| Duplicates at sink / history | 0 / 0 | — |
| Orphans / other run ids | 0 / 0 | — |
| Sink ∩ history | 199,170 | — |
| Qualified (measured window, warm-up excluded) | 193,170 (199,170 − 6,000 warm-up) | arithmetic |
| Qualified within 500 ms | 169,190 | sum of per-step within-deadline minus warm-up (43,190 + 126,000 = 169,190) |

Per step (`by_step`), delivery latency p99 in ms:

| Step | Offered rate | Qualified / planned | p99 (ms) |
|---|---|---|---|
| s00 warm-up | 50/s | 6,000 / 6,000 | 9.7 |
| s01 | 50/s | 9,000 / 9,000 | 9.8 |
| s02 | 100/s | 18,000 / 18,000 | 10.0 |
| s03 | 200/s | 36,000 / 36,000 | 23.4 |
| s04 | 350/s | 63,000 / 63,000 | 107.9 |
| s05 | 500/s | 67,170 / 90,000 | **22,316.3** |

Reading: the aggregate delivery p95 of about 17 s is almost entirely step s05 (500 msg/s), the step the exploratory
explore check already FAILs on outstanding work (`outstanding work grows`). Steps s00–s04 have p99 at or below 108 ms.
The per-step table is the reporting unit for this run. The aggregate is not a C_SLO and not a platform verdict.

Limitations shown by the audit for this run:

- History export has no `device` or `ts`, so the collision analysis is **unavailable** (U).
- No per-message durable-commit timestamp exists.
- The acknowledged set is exact because the generator reported zero dropped iterations. `records.ndjson` is empty for
  this run, which is consistent with every planned seq being `ack_success` (D18); this is inferred from absence, and
  the audit labels it that way.

## 4. Clock-offset and drift procedure (defined, not executed)

Required before any **cross-host** timing claim (for example SUT-side timestamps against generator timestamps). Not
needed for the primary latency, which uses the shared load-host clock.

1. Record which clock source each host uses (`chronyc tracking` or `timedatectl`) before and after a run window.
2. Take at least 30 paired samples over at least 10 minutes, before and after a run, each by a timestamped request
   and response between the two hosts (round-trip-corrected offset).
3. Report the median, maximum absolute offset and the drift between the start and end of the window, per pair of
   hosts.
4. A cross-host claim is allowed only if the maximum absolute offset is below a threshold **that the owner sets**. No
   threshold is chosen here. Until one is recorded, cross-host latency is reported as **unqualified** and not compared.
5. Store the samples with the run's manifest. A failed or missing offset measurement is recorded, never filled in.

## 5. Reporting restrictions until qualification

- Delivery results are reported as **aggregate** delivery unless the run is qualified; the per-step table is shown.
- No per-message commit latency. No "sink receipt therefore stored" inference (D36).
- FIWARE store-side results are **unavailable**, not estimated (B5).
- ThingsBoard collision results are **unavailable** unless a `device`/`ts` export exists and has passed the
  collision check on a qualified run.
- No verdict or SLO pass/fail rule is changed (sign-off §9.2, D39).

## 6. Tests run

- `go test ./internal/audit` covers all named known-answer tests: missing, duplicate, ambiguous, delayed (deadline),
  per-step separation, warm-up exclusion, collision risk and stored evidence, missing history, dropped iterations and
  empty history.
- The full suite was run with `go test ./...` (see the final report for the result of this checkpoint).
