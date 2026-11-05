# Project status — single authoritative page

**Last reconciled: 2026-10-31** (branch `chore/ci-and-release`).
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
| B10 | Run | PARTIAL | Approved initial ceiling remains **200/s conditional on the actual configuration qualifying at 400/s**. Earlier loopback and 100-device fixture results retain their limited scope. New 1,000-device ThingsBoard-adapter immediate-response run is generator-VALID at 400/s, but its independent receiver ledger is not yet retrieved; delayed and remaining adapter attempts were interrupted by SUT network loss. No common campaign ceiling is newly qualified. See `docs/archive/sessions/qualification-next-20261031.md`. |
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
The historical history-export reconciliations alone did not close B8. The subsequent **read-only
device-index audit of 100 populated registry rows** did close that specific condition, with zero
incomplete keys and duplicate groups (`results/verification-20261031/d27/`). No index was created.

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

**Former incompleteness resolved:** per-run originals were fetched from the load host and preserved
under `results/auto-20261029/per-run-originals/`; see §3.3 and its dated amendment. Original evidence
was not rewritten. A checksum inventory certifies the included files, not experimental validity.

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

## 9. Current blocker: SUT network loss during 1,000-device qualification

The continuation from `9c7fc5f` is retained in `results/qualification-next-20261031/` and
`docs/archive/sessions/qualification-next-20261031.md`. All unrelated benchmark stacks were
stopped before the controlled-receiver performance tests; required monitoring stayed up.

The TB adapter immediate-response run delivered 48,000 measured requests at 400/s with zero
drops and complete HTTP outcomes. During the delayed run, SUT reachability disappeared around
**18:48:35–18:48:40 UTC** (node/cAdvisor scrape failures); the delayed run is INVALID, and FIWARE/
EdgeX attempts aborted during setup. The load host remains reachable and its bundles were retrieved.
The SUT's receiver ledgers and final service state could not be retrieved after bounded retries.

No new platform CPU workload, Kafka traffic validation, FIWARE rate extension or live k=1/2/3
scaling test was started after that failure. Static fit checks are **arithmetic only**, not live
qualification. CPU tier reporting, clock/capture helpers, resume-safe EdgeX preparation and
FIWARE-history monitoring coverage fixes are tested locally but not claimed live-qualified.
Restore SUT reachability, retrieve/flush the existing receiver ledgers without reusing output paths,
then resume the gates. Main campaign remains **NO-GO**; no rate or SLO decision is changed.

**Bounded recovery check, 2026-11-01:** SUT SSH still times out. Load-host observations at
2026-10-31 23:03 UTC show healthy load-host services, a timed-out SUT receiver probe and all SUT
scrape targets down. No tests, restoration or network changes were attempted. SUT identity/state,
ledger recovery and reconciliation remain unavailable. Evidence: `results/recovery-20261101/`;
handoff: `docs/archive/sessions/recovery-check-20261101.md`.

## 10. Owner profile mappings approved — 2026-11-01 (offline only)

[D44](architecture/design-decisions.md#d44--owner-profile-mappings-2026-11-01) records the
owner's approval of Quick delivery (500 ms scheduled→first-sink p99 primary, with separate
250/1000/5000 ms sensitivities), Reliability of saved data (zero acknowledged identities
missing from independent history and zero duplicate stored events in the stated scope/window),
and High number of devices (W2 100-vs-10,000 at matched ~50%/~75% reference rates, N=1;
not a device-capacity cutoff). This supersedes §8's pending numerical mapping status only.
The dated [policy](../research/protocols/offline-profiles-20261101.json) is the analyzer default.

Missing C_SLO(500,1) reference evidence, observation windows/capture completeness, effective
configuration and population qualification remain unavailable rather than guessed. See
[current offline configuration](development/offline-analysis.md#owner-approved-configuration-2026-11-01).
This update authorizes no runtime action. **Campaign remains NO-GO.**

## 11. M0a baseline preservation and receiver recovery — 2026-11-03

Scoped recovery and review packet: [`docs/archive/sessions/m0a-baseline-recovery-20261103.md`](archive/sessions/m0a-baseline-recovery-20261103.md);
raw evidence: [`results/m0a-20261103-020828+0330/`](../results/m0a-20261103-020828%2B0330/).
Before recovery, the dirty baseline on `chore/ci-and-release` at `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`
was snapshotted (tracked diff SHA-256 `f0da0d534b343c0db6e8f1d8389becdc828b69de136f78935338f5464e2680c2`,
plus hashes/copies of all untracked source/doc/test files); no pre-existing work was reset or overwritten.
The requested route was `openai/gpt-6-luna#high`; the running harness identified GPT-6 Luna but did not
expose a separate `#high` routing confirmation.

Both hosts were reachable at the recorded host-local UTC time 2026-11-02 22:38:39. The SUT reported
`ramona-vm-2`, 16,375,816 kB MemTotal, and exactly the two expected receiver command lines/listeners.
Pre-flush copies matched the existing ledger hashes. After argv verification, only PIDs 3830274 and
3830275 received SIGTERM; both exited within the bounded wait. Final ledgers and logs were copied and
hash-checked. Immediate reconciles to 49,500 unique expected receipts / 1,000 devices, zero missing,
unexpected, duplicate, or wrong-device identities (including warm-up); this supports the existing
generator-VALID label, not platform qualification. Delayed remains generator-INVALID: 45,222 receipts,
1,000 devices, 3,278 dropped iterations, and no unexpected/duplicate/wrong-device rows. Missing count
is intentionally null because dropped iterations prevent exact planned-sequence inference; retain the
unknown-outcome relationship. Four setup-aborted FIWARE/EdgeX cases remain INVALID. The verifier's
raw “missing=49,500” for these zero-send cases is a plan-range artifact, not measured loss.

Offline baseline commands passed: `go test ./internal/... ./cmd/...`,
`python3 tests/adapter_receiver_test.py`, `python3 tests/resume_safe_provision_test.py`, and
`python3 -m unittest discover -s tests/offline -v`; reconciliation command/output and exact evidence
hashes are in the packet. `git diff --check` passed. No platform stack was started/restored, no load
campaign or protocol change was made, and no commit/push/release occurred.

Current decision: prior owner scope N1/N2/N3, P/D/rate, scheduled-cohort p99, and a manual single
application-replica fault remain **not qualified**. The immediate receiver recovery resolves ledger
availability only; all numerical/profile, population, clock/capture, platform, and fault evidence gates
remain open. Preserve the delayed INVALID run. Next: independent review of this M0a packet and baseline;
only then scope the next milestone. Main campaign remains **NO-GO**.

### Independent M0a review and current task ledger — 2026-11-03

M0a is **APPROVED for recovery scope only** by the separate Astra review session
`ses_e6193170affeUiLBF5ROkzdEfP` (requested `openai/gpt-6-astra#high`; harness identified
GPT-6 Astra, effort routing not independently exposed). Review record:
[`m0a-independent-review-20261103.md`](archive/sessions/m0a-independent-review-20261103.md).
The preceding recovery narrative is retained as originally reported. Its 22:38:39 timestamp,
memory observation, failed guard, bounded wait and original test results are session-reported;
not every underlying transcript was retained. The retained pre-stop transcript begins at
22:39:04 UTC. No-restoration was session-reported, not independently established by a post-stop
container inventory. These provenance limitations do not invalidate the corroborated receiver
closure, hashes or identity reconciliation. Do not regenerate historical transcripts.

| Task | Dependency | Owner / requested model | State | Evidence / last check | Next action |
|---|---|---|---|---|---|
| M0a recovery | Current-state inventory | Luna `openai/gpt-6-luna#high`; independent Astra `openai/gpt-6-astra#high` | APPROVED, scoped | Recovery bundle above; independent review 2026-11-03 | Preserve evidence; no repeat recovery needed |
| M0b preliminary contract | Initial Astra design review; preserved baseline | Luna `openai/gpt-6-luna#high`; independent Astra `openai/gpt-6-astra#high` | APPROVED, offline foundation only | [Final independent review](archive/sessions/m0b-independent-review-20261103.md#final-r4-re-review--2026-11-03); all R1–R7 closed, 2026-11-03 | Preserve approved foundation; integrate explicit consumers next |
| M1a protocol-bound cohort analysis | Approved M0b | Implementation: Luna sessions, requested `openai/gpt-6-luna#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m1a-approval-20261104.md); fix packet `results/m1a-fix-packet-20261104-005644+0330/`; review `ses_e5ca28f17ffezpYAWTi38I5QWO`, 2026-11-04 | Preserve; runtime receipt/clock qualification remain outstanding |
| M1b offline end-to-end path | Approved M1a | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m1b-approval-20261104.md); packet `results/m1b-pipeline-20261104-013816+0330/`; review `ses_e5c831c28ffeZ2P8S8xUfbkZkA`, 2026-11-04 | Preserve; live qualification remains outstanding |
| M1c payload golden fixture | Approved M1b | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m1c-approval-20261104.md); packet `results/m1c-payload-fixture-20261104-020102+0330/`; review `ses_e5c73aa2fffeCZJFCdsIesIv8s`, 2026-11-04 | Preserve; live payload-storage qualification remains outstanding |
| M1d complete-body receiver | Approved M1c | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m1d-approval-20261104.md); packet `results/m1d-complete-body-20261104-024117+0330/`; review `ses_e5c40004cffeqr4BbZeigwIZ1w`, 2026-11-04 | Preserve; no deployment or clock qualification |
| M1e adapter payload controls | Approved M1d | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m1e-approval-20261104.md); packets `results/m1e-*`; review sessions `ses_e5bf776acffewIermuL5GrjYPw`, `ses_e5bda0110ffezYcVouts5ruYrK`, `ses_e5bc73feaffes8ZYvVRg1HmbVl`, 2026-11-04 | Preserve; no deployment or runtime evidence |
| M1f manual fault/capacity reducers | Approved M1e | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/kimi-k3.1-pro#low` | APPROVED, scoped offline | [Approval record](archive/sessions/m1f-approval-20261105.md); packets and review under `docs/archive/sessions/` and `results/m1f-*`, 2026-11-05 | Preserve; live fault/deployment qualification outstanding |
| M2 deployment/instrumentation foundation | Approved M1f | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/kimi-k3.1-pro#low` | APPROVED, scoped offline | [Approval record](archive/sessions/m2-approval-20261105.md); fix packet `docs/archive/sessions/m2-fix-20261105.md`; review sessions under [`m2-independent-review-20261105.md`](archive/sessions/m2-independent-review-20261105.md), 2026-11-05 | Preserve; live/deployment qualification outstanding |
| M2b deployment orchestration blueprint | Approved M2 | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m2b-approval-20261105.md); packets `docs/archive/sessions/m2b-*`; review `ses_e55da6cd4ffeV6C6UiZxuwBTLJ`, 2026-11-05 | Preserve; no lifecycle/live claim |
| M2c offline render/verify validation | Approved M2b | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m2c-approval-20261105.md); packet `docs/archive/sessions/m2c-render-verify-20261105.md`; review `ses_e557e06aefferRFlsPQc9HpYVD`, 2026-11-05 | Preserve; nonblocking F1/F2/F3 noted; no live claim |
| M2d offline execution-plan render | Approved M2c | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](archive/sessions/m2d-approval-20261105.md); packet `docs/archive/sessions/m2d-execution-plan-20261105.md`; review `ses_e55615fd5ffeRiVCyh2iO9Qs1i`, 2026-11-05 | Preserve; no lifecycle/live claim |
| M2e health/evidence dry-run | Plan by Kimi; implementation by DeepSeek Flash | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review intentionally waived by owner | COMPLETE, SELF-TESTED, NO INDEPENDENT REVIEW | Packet `docs/archive/sessions/m2e-health-evidence-20261105.md`; evidence `results/m2e-health-evidence-20261105-113210+0330/`; 2026-11-05 | Synthetic offline only; independently unverified by owner choice; no live claim |
| M3 live preflight | M2e self-test; approved offline foundations | Orchestrator session | COMPLETE READ-ONLY; deployment not started | `results/live-preflight-20261105T080836Z/` with Gate A/B/C captures and SHA256SUMS, 2026-11-05 | Clock offset unmeasured; no platform workload running; image/effect-config/digest and resource mapping not frozen; Gate D not entered |
| M3–M7 live qualification/campaign | Approved offline foundations; deployed producers; calibrated clock/drain/recovery policies | Not started | BLOCKED at session checkpoint | No live revised-scope evidence; both hosts were reachable in a bounded read-only check at 2026-11-05 07:37 UTC, with only exporters/monitoring running; no deployment or load was performed | Resume live preflight and deployment preparation; campaign remains NO-GO |
| Protocol freeze | M0b, producer/analyzer integration, policy calibration | Astra review | BLOCKED on evidence | Initial review CHANGES_REQUIRED for campaign freeze | Qualify policies; keep campaign eligibility fail-closed |
| M1–M7 remaining scope | Approved contract and preceding milestone gates | Luna implementation / Astra independent review | NOT_COMPLETE | No revised-scope acceptance evidence | Follow dependency-ordered handoff; manual fault cells require operator |

Revised-scope status: **SOFTWARE_VERIFIED=false**, **LIVE_QUALIFICATION_COMPLETE=false**,
**CAMPAIGN_COMPLETE=false**, **ARTIFACT_REPRODUCIBLE=false**. Campaign remains **NO-GO**.

### Session checkpoint — 2026-11-04

Offline milestones M0a–M1e completed with independent review (approval records under
`docs/archive/sessions/`). No live qualification or campaign run was performed. Resumable handoff:
[`session-checkpoint-20261104.md`](archive/sessions/session-checkpoint-20261104.md). The next
bounded work is M1f (offline fault/capacity reducers), then deployment and calibration before
qualification. Host state noted at session start must be re-verified; do not assume it persists.

### Updated checkpoint — 2026-11-05

Offline foundations are now approved through **M2d** (fault/capacity reducer, deployment
descriptors/validator, orchestration blueprint/packaging, synthetic render/verify validation, and
render-only execution planning). All approvals explicitly exclude live deployment, lifecycle,
fault execution, load and campaign evidence. M2e (synthetic health/evidence dry-run) is proposed
but not started.

Read-only host check at 2026-11-05 07:37 UTC:

| Host | Observed |
|---|---|
| SUT `ramonavm2` / ramona-vm-2 | reachable; 8 vCPU; MemTotal 16,375,816 kB; only four exporters running; no benchmark platform containers and no receiver processes |
| Load host via `root@87.107.191.32` / ramona-vm-q | reachable; monitoring Prometheus/cAdvisor/Grafana/node-exporter and yaade running |

No service changes, deployment, load or fault action were performed by this check. The earlier
session checkpoint remains historical and should not be assumed current.

**Exact next action:** implement M2e only if continuing offline, or prepare a separately reviewed
live preflight/deployment milestone. Before any live action, re-verify hosts, preserve current
ledgers, and use new output paths. The campaign remains **NO-GO**.

### Live preflight result — 2026-11-05 08:08 UTC

Read-only captures: [`sut-gates.txt`](../results/live-preflight-20261105T080836Z/sut-gates.txt),
[`load-gates.txt`](../results/live-preflight-20261105T080836Z/load-gates.txt), checksums and
[summary](../results/live-preflight-20261105T080836Z/README.md). No remote state was changed.

- Both hosts reachable. SUT is `ramona-vm-2`, Ubuntu 24.04, kernel 6.8.0-31, 8 vCPU,
  15,638,421,504 B available memory, 23 GB root-disk headroom. Load host is `ramona-vm-q`,
  4 vCPU, 6,689,443,840 B available memory, 25 GB headroom.
- SUT benchmark platform containers are exited; four exporters are running; no receiver process
  remains. The legacy load-host `sink` process still listens on 18090 and must be preserved or
  replaced only through a new reviewed output path.
- Both hosts have active swap (1.9 GB file, swappiness 60). No `chronyc`/`ntpq` is present, so
  clock offset could not be measured; the SUT also reports a pending reboot.
- Cached image tags exist, but effective image digests, resource mapping and compose
  configuration were not frozen. No Gate D action, deployment, provisioning, load or data reset
  occurred.

**Live deployment remains blocked** pending explicit authorization and freeze of platform, N/K,
resource budgets, artifact digests, clock/capture procedure, output paths and rollback plan.
