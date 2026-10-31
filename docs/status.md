# Project status — single authoritative page

**Last reconciled: 2026-10-29** (branch `docs/state-reconciliation`).
This page supersedes every per-session narrative. Session close-outs, progress logs and state snapshots are
preserved verbatim — and may contradict each other — under [`docs/archive/sessions/`](archive/sessions/).
When this page and an archived narrative disagree, **this page wins**; discrepancies are to be corrected *here*
(or re-derived from the raw evidence), never re-litigated from the narratives.

Overall verdict unchanged: **NO-GO for the main experimental campaign.** What exists is benchmark *tooling* whose
novel parts are qualified, plus **exploratory qualification evidence** (labelled as such below). Nothing here is a
completed benchmark campaign.

---

## 1. Qualification state (blocks B1–B17)

Reconciled from `docs/qualification/qualification-blockers.md` (the row-level record of truth), the 2026-10-29 interpretation
correction (`docs/methodology/interpretation-correction-20261029.md`) and the raw evidence under `results/`.
Columns: **State** (CLOSED / CLAIM-READY / OPEN / PENDING-OWNER), one-line evidence, agreed limitation.

| ID | Gate | State | Evidence & limitation |
|---|---|---|---|
| B1 | Deploy/Run | CLOSED | Host identities + clocks live-verified 2026-10-27 (`results/auto-20261027/preflight-*.txt`); re-verify before any future platform change. |
| B2 | Deploy (FIWARE) | CLAIM-READY | Four-payload probe + duplicate/same-timestamp behaviour reproduced on the isolated candidate (`fiwhc`), QL 1.0.0. Rerun probe if any pinned image changes. |
| B3 | Deploy (FIWARE) | CLAIM-READY | Orion 4.5.0 → QL 1.0.0 end-to-end live pairing (10/10 labelled; deliberate drop preserved). One live low-rate pairing only; upstream-tested range still ends at 3.3.1. |
| B4 | Deploy (FIWARE) | ADOPTED | 2,048 MiB history tier adopted by the owner 2026-10-29; 3,001/3,001 identities live at 50 r/s (`results/auto-20261029/fiware/`). **Open: adequacy above 50 r/s.** |
| B5 | Claim/Smoke (FIWARE) | CLOSED (rehearsal scope) | 2026-10-29 rehearsal `REH-FIWARE-V4` exercised the **full-stack path** (HAProxy → IoT Agent UL → Orion → QL/Timescale candidate + external sink): 3,000/3,000 measurement identities reconciled with 0 missing/dups/unrecoverable at 25 r/s (`results/rehearsals-20261029/`). Store-side results remain scoped to that rehearsal rate; above 50 r/s still open. |
| B6 | Run (ThingsBoard) | ADOPTED (2026-10-29) | 1280 MiB qualified under load: 50 + 200 r/s VALID ×2, 400 r/s INVALID at k=2. The owner's k decision was recorded on 2026-10-29 per the working authorization: **application CPU budgets k ∈ {1,2,3}, actual replica counts recorded per run**, consistent with option O4. |
| B7 | Deploy (TB) | CLOSED (except probe) | Config/entrypoint/JVM/log isolation live-verified 2026-10-27. Remaining: intentional isolated OOM probe on a disposable container (needs authorization). |
| B8 | Claim/Run (FIWARE index) | CLOSED (populated read-only audit) | D27 export plus file audit on the populated baseline IoT Agent registry: **100 rows, 0 incomplete keys, 0 duplicate groups**. Evidence: `results/verification-20261031/d27/`. No index created and no devices deleted. This is a snapshot audit, not a guarantee of future uniqueness. |
| B9 | Claim (EdgeX disk+WAL) | CLOSED | Disk 2026-10-28, **WAL 2026-10-29**: benchctl vs independent recomputation match to 4+ decimals via the DIAG-only postgres overlay (`results/auto-20261029/wal/`, commit `a51fcc1`). Benchmark config stays the plain compose. |
| B10 | Run | PARTIAL | Generator-only ceiling measured (loopback): 3200 r/s ×2 consecutive VALID, 6400 INVALID; sink qualified at 3200 r/s through the real path; **recommended max campaign rate ≤ 1600 r/s**, subject to cross-host path + platform rate. Platform side: 100/200 r/s VALID at 1280 MiB; **nothing above 200 r/s is a capacity statement** (see §3.4). |
| B11 | Claim (cross-host timing) | OPEN | Corrected probe: load↔SUT offset ∈ [−63, +50] ms. No SLO exists yet; U ≤ P/10 unadopted. Cross-host timing claims remain unqualified. |
| B12 | Run (sink capacity) | PARTIAL | Sink qualified AT 3200 r/s (0 unparsed/overflow, 240k msgs). Own envelope stress beyond the ceiling steps not yet run. |
| B13 | Run (monitoring cost) | OPEN | SUT-side exporters installed (cpuset 0, 128m limits); their cost inside a 2× check not isolated. |
| B14 | Claim (identity exports) | CLOSED | All three platforms verified with non-empty data: FIWARE 3,001/3,001; EdgeX 600/600 exact; TB device/ts export verified earlier. |
| B15 | Overload stage | OPEN (owner) | D1 anchor + D2 uncertainty method; owner decision per `docs/methodology/methodology-reconciliation.md` O5/O6. |
| B16 | Claim (reproducibility) | MAINTAINED | 758/758 historical manifest files re-verify (`sha256sum -c evidence/evidence-integrity.sha256`); newer sessions carry separate SHA256SUMS. Publication of the inventory happens at report time (see §5). |
| B17 | Scaling claims | OPEN | S1–S5 amendments recorded (D35–D39), not yet experimentally validated. |

The per-row blocker text, evidence and history remain in `docs/qualification/qualification-blockers.md` (2026-10-27 basis,
maintained through 2026-10-29). This section reconciles it against the 2026-10-29 correction; rows above carry
the reconciled wording. **2026-10-29 addition:** the D33 measurement-integrity components (identity/audit chain,
D33 items 1–7) were live-validated for the first time in the bounded rehearsals on all three platforms —
`results/rehearsals-20261029/` (TS `REH-TB-V2`, `REH-EDGEX-V1`, `REH-FIWARE-V4`, each 3,300/3,300 identities
ack → sink → history with 0 duplicates/orphans/malformed; session close-out
`docs/archive/sessions/verify-20261029.md`).

---

## 2. What makes a result "VALID" — two different harnesses

Critical to interpret any recent verdict correctly. There are two generator paths in the tree:

| | **Legacy path** `workloads/legacy/{edgex,fiware,thingsboard}.js` + `run.sh` | **Generator path** `workloads/k6/gen.js` via `benchctl` |
|---|---|---|
| Gate | dropped iteration == 0; achieved ≈ target rate ±1%; TB/EdgeX: **stored count == sent count**; FIWARE: devices-with-data; sink counter deltas (dup/unparsed/overflow = INVALID); with `--record-identity`: identity-set reconciliation vs planned arrivals; no SUT OOM/restart during the run window | the legacy gates **plus** planned-time schedule integrity, error rate, complete run record, identity-based reconciliation through the store export by design |
| Identity basis | unique `(run_id, seq)` since the 2026-10-29 repair (`iterationInTest`); EdgeX remains count-based (transport limitation, documented in the harness header) | unique `(run_id, seq)` identities by design (`workloads/k6/...` + Go runner) |
| Verdict label | **exploratory** | **campaign-grade** |

Session auto-20261029's generator-ceiling and TB 1280 MiB runs went through the **legacy path pre-repair**
(count-only, per-process `seq`). Their VALID verdicts establish drops/rate/recording sanity — they are real and
retained — but they do **not** establish identity reconciliation or planned-schedule parity. Every such result is
labelled *exploratory* in §4. The repair (2026-10-29, `fix/qualification-integrity`) gives the legacy harness
unique identities, sink-counter verdicts and optional `--record-identity` reconciliation; campaign parity still
lives on the benchctl path only.

---

## 3. Reconciled contradictions

### 3.1 D29a WAL verification — handoff stale, blocker table correct
`docs/archive/sessions/autonomous-handoff-20261029.md` line 19 lists “D29a WAL live verification (pg-counters
overlay) still pending”. **Stale.** The WAL block was live-verified later the same session (commit `a51fcc1`,
B9 row above): benchctl values match an independent recomputation from the raw pg-counters to 4+ decimals.
Status: **CLOSED**. Sanity note: the handoff was written before the WAL run; its evidence map has no `wal/` entry
while `results/auto-20261029/wal/` was added afterwards.

### 3.2 B8 — empty audit vs later populated tests
B8’s to-clear condition (“live audit when a registry holds provisioned data”) is **still open**. The later
*populated* reconciliations (2026-10-29) exercise the **history exports**, not the device-index duplicate audit.
Do not mark B8 closed on the strength of §4’s populated-data runs.

### 3.3 The 200 r/s confirm latency — **RESOLVED 2026-10-29 (unit misreading)**
The per-run originals were fetched from the load host (`results/auto-20261029/per-run-originals/`,
bag SHA-256 `99c74f56…`; amendment `latency-unit-resolution.amendment.json`). The value `p(99)=1.469761`
in `thingsboard-n1-r200-20261027T233211Z/summary.json` is **milliseconds** and belongs to a run whose
steady phase had `error_rate = 1.0` (every request failed) — it is the latency profile of the failing
requests (max 18.7 ms), **not** a 1.47 s service latency. The archived "1,469.8 ms" narrative was a
unit misreading of that figure. The healthy 200 r/s confirm runs (`…T232557Z`, `…T232253Z`) report
p95 230.3 / 291.9 ms, p99 330.0 / 415.2 ms, consistent with the first-pass quote; historical raw
evidence was not edited.

### 3.4 “Platform bottleneck above 200 r/s” — attribution withdrawn
The 400 r/s run (p95 = the 10 s timeout, 6,465 dropped) is **INVALID and cannot establish the service-SLO
capacity boundary**, and the 2026-10-28 100 r/s INVALIDs were generator VU starvation, not platform failure
(`docs/methodology/interpretation-correction-20261029.md`). What *is* owned: 50 and 200 r/s VALID at TB k=2 / 1280 MiB, the
generator ceiling §1-B10, and the sentence “the platform binds first at this configuration” — which holds at
200 r/s and below and is not extended to higher rates without a qualified run. The archived narratives’
“loopback stack saturation” referral for 6400 r/s retains *loopback*, not platform, attribution.

---

## 4. 2026-10-29 evidence inventory (`results/auto-20261029/`, SHA256SUMS inside)

| Item | Files | Level |
|---|---|---|
| Generator ceiling, loopback (7 steps, legacy harness) | `ceiling/all-summaries.txt` | exploratory |
| TB 1280 MiB under load (5 TB runs, legacy harness) | ids inside the same combined summary | exploratory |
| SUT monitor CSV for the 1280 run | `qual1280/monitor.csv` | supporting |
| FIWARE history identity export, 3,001 rows at 50 r/s | `fiware/hist-export.ndjson` | qualification-grade reconciliation, 50 r/s only |
| EdgeX readings export, 600/600 exact | `edgex/edgex-hist-export.ndjson` | qualification-grade reconciliation |
| Corrected clock probe | `clock/clock-probe-results.md`, `ws-to-*.csv` | bound only (§1-B11) |
| D29a WAL live verification | `wal/` + README + `wal-verify.py` | CLOSED (§3.1) |
| TB memory budget k=1..4 + owner options O1–O5 | `tb-memory-budget.md` | decision input |

**Known incompleteness:** only `ceiling/all-summaries.txt` (combined) exists here — the per-run summaries and
manifests for the ceiling and TB-qualification runs live on the **load host** (`/root/iot-bench-cand-auto20261027/
workloads/results/`, the 11 new dirs recorded in the archived close-out). Checksums in `SHA256SUMS` guarantee
integrity of what was fetched, not completeness of what was fetched. Fetching the originals is required before
any latency above 50 r/s is quoted (§3.3).

---

## 5. Publication boundary (backlog)

1. ~~`.gitignore` carried a malformed `workloads/` exclusion line~~ — repaired on the layout branch (single
   correct exclusion block, plus `deploy/*/.env` and `bin/*`).
2. Full-history secret scan before any publish (tracked tree is currently clean of `.env`/credential files;
   `yaade/` reviewed 2026-10-29: collections are generated by script, only the public TB demo tenant appears,
   no stored tokens). Publication uses a squashed orphan `main`; the original history stays private.
3. Sanitized `.env.example` templates are **present and tracked** for all four stacks
   (`deploy/*/.env.example`, `monitoring/.env.example`) — the missing-templates gap was an artifact of the
   2026-10-29 archive tarball, not of the checkout. Documented sanitization pass on the release branch.
4. Results (≈ 416 MB) ship as **reviewed bundles to a separate research archive** with checksums + a
   machine-readable index linking each bundle to its evidence source, not in Git; Git keeps known-answer fixtures.
   Integrity layer + historical-path mapping table: `evidence/README.md`.
5. Original preregistration bytes are preserved untouched (now
   `research/preregistration/preregistration-archive/`, verified by `scripts/check-preregistration.sh`); later
   changes are recorded as dated decisions, never by editing the archive.

## 6. Migration / tooling follow-up

| Branch | Scope | State |
|---|---|---|
| `docs/state-reconciliation` | this page + archived narratives + README readiness | committed (c53305c) |
| `chore/repository-layout` | target directory layout, `git mv` + path updates only; SECURITY/CONTRIBUTING/CITATION + issue templates | committed (aee9e83) |
| `fix/qualification-integrity` | sink unique identities (`iterationInTest`), TB/FIWARE identity attributes, sink-counter deltas in the verdict, `--record-identity` reconciliation vs planned arrivals, explicit ms, repo-root results root | **this branch** |
| `chore/ci-and-release` | CI workflows (build/vet/test, shellcheck, compose render + resource guards, D28 static check, prereg + evidence verification) — stacked on the integrity branch, which is the intended merge order | **this branch** (after fix commits) |
| `feat/fiware-history-integration` | full-stack IoT-Agent/HAProxy path for the validated history candidate | deferred, explicitly experimental |

## 7. Owner inputs still owed

1. ~~Per-run summaries/manifests from the load host (§4)~~ — **fetched 2026-10-29** and archived under
   `results/auto-20261029/per-run-originals/` (§3.3 closed; §4's "Known incompleteness" resolved).
2. ~~The B6 k decision (O1–O5)~~ — **recorded 2026-10-29** per the working authorization: k ∈ {1,2,3}, actual replica counts recorded per run.
3. Target external research-archive location/CID range for the evidence bundles.
4. Licence + citation metadata decision (LICENSE/CITATION.cff are owner content, not agent-generated).

## 8. Remaining technical blockers (session verify-20261029)

Updated **2026-10-31**, follow-up evidence: `results/verification-20261031/`.

1. **Rehearsal exports completed** without rerunning the platforms: three offline reports, 36 PNG/SVG
   pairs, raw Prometheus responses and CSVs. All 10,302 finite CSV values match raw responses;
   four unavailable queries per run stay `NO_DATA`, not zero. Reports:
   `results/verification-20261031/reports-final/<run_id>/index.html`.
2. **Cross-host controlled receiver path verified** at 400/s for 120 s, both immediate and 700 ms
   delayed responses (700 VUs); at 800/s for 60 s with immediate responses. All valid runs reconciled
   51,000/51,000 identities (warm-up included). The undersized 100-VU control was correctly INVALID.
   Scope: fixture adapter, logical telemetry payload, 100 round-robin device identities, load host
   cpuset 0-1 → SUT receiver cores 6/7, recording and monitoring enabled. This does **not** qualify
   1,000-device provisioning or platform-specific adapter overhead at 400/s; those remain gates
   before calling the complete campaign generator configuration qualified.
3. **CPU bundle capture implemented and live-tested** on the controlled receiver: raw cumulative
   counters, IDs, StartedAt, timestamps, bracketing skew and named denominator are retained.
   Counter-reset/restart/missing tests pass. Old platform rehearsals still have no endpoint CPU
   counters; no exact platform CPU cost is inferred. Cross-host phase alignment and full platform
   boundary membership require verification on the next platform qualification run.
4. **SLO profiles remain pending**. The archived TELEM-500 primary candidate and 250/1000/5000 ms
   sensitivities are reconciled in `research/protocols/slo-profile-reconciliation-20261029.md`;
   external 3GPP attribution remains unverified. No SLO verdicts are introduced.

Full follow-up handoff: `docs/archive/sessions/verify-followup-20261031.md`.
