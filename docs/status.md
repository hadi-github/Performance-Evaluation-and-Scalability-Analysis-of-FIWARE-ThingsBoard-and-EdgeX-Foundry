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

Reconciled from `docs/qualification-blockers.md` (the row-level record of truth), the 2026-10-29 interpretation
correction (`docs/interpretation-correction-20261029.md`) and the raw evidence under `results/`.
Columns: **State** (CLOSED / CLAIM-READY / OPEN / PENDING-OWNER), one-line evidence, agreed limitation.

| ID | Gate | State | Evidence & limitation |
|---|---|---|---|
| B1 | Deploy/Run | CLOSED | Host identities + clocks live-verified 2026-10-27 (`results/auto-20261027/preflight-*.txt`); re-verify before any future platform change. |
| B2 | Deploy (FIWARE) | CLAIM-READY | Four-payload probe + duplicate/same-timestamp behaviour reproduced on the isolated candidate (`fiwhc`), QL 1.0.0. Rerun probe if any pinned image changes. |
| B3 | Deploy (FIWARE) | CLAIM-READY | Orion 4.5.0 → QL 1.0.0 end-to-end live pairing (10/10 labelled; deliberate drop preserved). One live low-rate pairing only; upstream-tested range still ends at 3.3.1. |
| B4 | Deploy (FIWARE) | ADOPTED | 2,048 MiB history tier adopted by the owner 2026-10-29; 3,001/3,001 identities live at 50 r/s (`results/auto-20261029/fiware/`). **Open: adequacy above 50 r/s.** |
| B5 | Claim/Smoke (FIWARE) | CLAIM-READY (store-side) | `history-ids.sh fiware` implemented + live-replayed 24 identities incl. deliberate drop/restart. **Open: exercise through the full-stack (IoT-Agent) path.** |
| B6 | Run (ThingsBoard) | PENDING-OWNER | 1280 MiB qualified under load: 50 + 200 r/s VALID ×2, 400 r/s INVALID at k=2; memory 76–79%; k=4 overflow 1,280 MiB. Owner options O1–O5 recorded; O4 (cap k=3) is the measured-consistent choice. Nothing runs further until the owner records the k decision. |
| B7 | Deploy (TB) | CLOSED (except probe) | Config/entrypoint/JVM/log isolation live-verified 2026-10-27. Remaining: intentional isolated OOM probe on a disposable container (needs authorization). |
| B8 | Claim/Run (FIWARE index) | OPEN | Live read-only audit 2026-10-27 found the registry **empty at rest** — duplicate detection logic has not surfaced on *populated* data through the device index. The 2026-10-29 populated identity reconciliations (3,001 FIWARE history rows; 600 EdgeX readings) went through the history exports, **not** the device-index duplicate audit. To-clear stands: repeat the live audit on a populated registry. |
| B9 | Claim (EdgeX disk+WAL) | CLOSED | Disk 2026-10-28, **WAL 2026-10-29**: benchctl vs independent recomputation match to 4+ decimals via the DIAG-only postgres overlay (`results/auto-20261029/wal/`, commit `a51fcc1`). Benchmark config stays the plain compose. |
| B10 | Run | PARTIAL | Generator-only ceiling measured (loopback): 3200 r/s ×2 consecutive VALID, 6400 INVALID; sink qualified at 3200 r/s through the real path; **recommended max campaign rate ≤ 1600 r/s**, subject to cross-host path + platform rate. Platform side: 100/200 r/s VALID at 1280 MiB; **nothing above 200 r/s is a capacity statement** (see §3.4). |
| B11 | Claim (cross-host timing) | OPEN | Corrected probe: load↔SUT offset ∈ [−63, +50] ms. No SLO exists yet; U ≤ P/10 unadopted. Cross-host timing claims remain unqualified. |
| B12 | Run (sink capacity) | PARTIAL | Sink qualified AT 3200 r/s (0 unparsed/overflow, 240k msgs). Own envelope stress beyond the ceiling steps not yet run. |
| B13 | Run (monitoring cost) | OPEN | SUT-side exporters installed (cpuset 0, 128m limits); their cost inside a 2× check not isolated. |
| B14 | Claim (identity exports) | CLOSED | All three platforms verified with non-empty data: FIWARE 3,001/3,001; EdgeX 600/600 exact; TB device/ts export verified earlier. |
| B15 | Overload stage | OPEN (owner) | D1 anchor + D2 uncertainty method; owner decision per `docs/methodology-reconciliation.md` O5/O6. |
| B16 | Claim (reproducibility) | MAINTAINED | 758/758 historical manifest files re-verify (`sha256sum -c docs/evidence-integrity.sha256`); newer sessions carry separate SHA256SUMS. Publication of the inventory happens at report time (see §5). |
| B17 | Scaling claims | OPEN | S1–S5 amendments recorded (D35–D39), not yet experimentally validated. |

The per-row blocker text, evidence and history remain in `docs/qualification-blockers.md` (2026-10-27 basis,
maintained through 2026-10-29). This section reconciles it against the 2026-10-29 correction; rows above carry
the reconciled wording.

---

## 2. What makes a result "VALID" — two different harnesses

Critical to interpret any recent verdict correctly. There are two generator paths in the tree:

| | **Legacy path** `workloads/{edgex,fiware,thingsboard}.js` + `workloads/run.sh` | **Generator path** `workloads/k6/gen.js` via `benchctl` |
|---|---|---|
| Gate | dropped iteration == 0; achieved ≈ target rate ±1%; TB/EdgeX: **stored count == sent count**; FIWARE: devices-with-data; no SUT OOM/restart during the run window | the legacy gates **plus** planned-time schedule integrity, error rate, complete run record, identity-based reconciliation through the store export by design |
| Identity basis | counts only (a missing identity can offset a duplicate and still read "stored == sent") | unique `(run_id, seq)` identities (`workloads/sink.js` before the fix increments `seq` per VU — scheduled for the integrity branch) |
| Verdict label | **exploratory** | **campaign-grade** |

Session auto-20261029's generator-ceiling and TB 1280 MiB runs went through the **legacy path**. Their VALID
verdicts establish drops/rate/recording sanity — they are real and retained — but they do **not** establish the
full planned schedule/record/reconciliation gate. Every such result is labelled *exploratory* in §4.
`fix/qualification-integrity` closes the tooling gap (§6).

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

### 3.3 The 200 r/s confirm latency — unit conflict, **unresolved**
`results/auto-20261029/ceiling/all-summaries.txt:24` reports `p99=1.469761` (and `p95=0.997103`) for
`thingsboard-n1-r200-20261027T233211Z`; the archived progress report states **1,469.8 ms**. K6 `http_req_duration`
trend values are milliseconds, and every other run in the same combined summary is consistent in ms
(gen rows 0.70–82 ms; TB r50/r100/r200-first/r400 66–10010 ms) — the confirm row is the sole outlier, so a
seconds-vs-milliseconds reporting defect in whatever produced that run is suspected. The repository contains only
the combined summary for these runs; the per-run `summary.json`/`manifest.json` remain on the load host under
`/root/iot-bench-cand-auto20261027/workloads/results/`. **Until those originals are fetched and checked, the value
is published nowhere and no latency statement at 200 r/s beyond the first pass (p95 ≈ 230 ms, p99 ≈ 330 ms) is
made.** The “p99 1.47 s, variability is real” and the VU-sizing lesson derived from it are provisional (§4).

### 3.4 “Platform bottleneck above 200 r/s” — attribution withdrawn
The 400 r/s run (p95 = the 10 s timeout, 6,465 dropped) is **INVALID and cannot establish the service-SLO
capacity boundary**, and the 2026-10-28 100 r/s INVALIDs were generator VU starvation, not platform failure
(`docs/interpretation-correction-20261029.md`). What *is* owned: 50 and 200 r/s VALID at TB k=2 / 1280 MiB, the
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

1. `.gitignore` still carries a malformed `workloads/` exclusion line → repaired on the layout branch.
2. Full-history secret scan before any publish (tracked tree is currently clean of `.env`/credential files;
   `yaade/` collections reviewed before they ship).
3. Sanitized `.env.example` templates are **present and tracked** for all four stacks
   (`edgex/.env.example`, `fiware/.env.example`, `thingsboard/.env.example`, `monitoring/.env.example`) — the
   missing-templates gap was an artifact of the 2026-10-29 archive tarball, not of the checkout. Each template
   gets a documented sanitization pass on the release branch.
4. Results (≈ 416 MB) ship as **reviewed bundles to a separate research archive** with checksums + a
   machine-readable index linking each bundle to its evidence source, not in Git; Git keeps known-answer fixtures.
5. Original preregistration bytes are preserved untouched under `docs/preregistration/`; subsequent changes are
   recorded as dated decisions, never by editing the archive.

## 6. Migration / tooling follow-up (in flight)

| Branch | Scope | State |
|---|---|---|
| `docs/state-reconciliation` | this page + archived narratives + README readiness | **this branch** |
| `chore/repository-layout` | target directory layout, `git mv` + path updates only | pending |
| `fix/qualification-integrity` | sink unique identities, legacy run-gate completion, explicit ms, TB identity-set reconciliation | pending |
| `chore/ci-and-release` | CI (build/vet/test, shellcheck, compose render, resource + prereg checks), contribution/security files | pending |
| `feat/fiware-history-integration` | full-stack IoT-Agent/HAProxy path for the validated history candidate | deferred, explicitly experimental |

## 7. Owner inputs still owed

1. Per-run summaries/manifests from the load host (§4) — needed for §3.3 and the ceiling record.
2. The B6 k decision (O1–O5; O4 is the measured-consistent choice).
3. Target external research-archive location/CID range for the evidence bundles.
4. Licence + citation metadata decision (LICENSE/CITATION.cff are owner content, not agent-generated).
