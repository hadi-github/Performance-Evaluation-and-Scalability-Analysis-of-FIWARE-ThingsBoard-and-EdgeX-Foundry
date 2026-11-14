# Project status — single authoritative page

## Prospective owner amendment — 2026-11-06

The owner has changed the future primary latency endpoint to **application dispatch → first complete-message sink receipt on the same load host** and withdrawn the universal 10 ms send-lag p99 rejection rule for future explicitly marked runs. Historical verdicts remain unchanged. Scheduled-to-receipt and send lag remain diagnostics/secondary evidence, not silently reinterpreted historical results. See `docs/methodology/dispatch-contract-20261106.md` and the narrowed review-candidate matrix at `results/feasible-campaign-20261106/frozen-scope.json`.

The feasible ordinary batch received conditional independent technical release after live qualification and owner approval. After the controller paused at TB repetition 2, independent review accepted its valid platform failure and cleared a **new persistent controller at original entry 7, EdgeX repetition 3**. Older gates/authorization text below remain dated historical context, not an override of this narrowed release.

**Last reconciled: 2026-11-15** (load host rebuilt on the new address and made operational; SUT still absent); actual recorded UTC timestamps govern run timing.
This page supersedes every per-session narrative. Session close-outs, progress logs and state snapshots are
preserved verbatim — and may contradict each other — under [`research/archive/sessions/`](../research/archive/sessions/).
When this page and an archived narrative disagree, **this page wins**; discrepancies are to be corrected *here*
(or re-derived from the raw evidence), never re-litigated from the narratives.

Current verdict: **bounded ordinary campaign partially executed; reviewed continuation
of original entries 7–15 running; campaign not complete**.
The broader capacity/scaling/fault campaign is not qualified by this release. Historical NO-GO
statements below retain their original dated scope and verdicts.

## Current ordinary campaign checkpoint — persistent continuation

The SUT tmux session `iot-feasible-ordinary-20261108` started at observed
**2026-11-07 21:22 UTC**, with controller PID 4078364 verified on **CPU 0**.
It accepted EX repetition 2, then halted with exit 1 at observed **22:41 UTC**
after TB repetition 2 returned a valid platform delivery failure. Original
controller state/log/exit and both attempts are retained. No entry 7 or retry
was started by that controller. Independent review accepted EX2 success and TB2
as `VALID_PLATFORM_FAILURE_OR_BACKLOG`, with no instrumentation blocker or
replacement/full-qualification rerun required. A fresh ordinal-7 receipt binds
the unchanged sources, binaries and remaining plans. The new tmux session
`iot-feasible-ordinary-r07-20261108` started at observed **22:52 UTC**, with PID
143674 verified on CPU 0, beginning `disp-ex-r03-20261106`. Old state and logs
remain unchanged; live state is now `state-r07/controller-state.json`. Never
reuse attempted IDs or the old state. The supervisor halts on nonzero/missing gates, collisions, input changes,
ownership/cleanup/history failures, aggregation errors or `STOP_AFTER_CURRENT`;
no retry/replacement and no automatic reboot resume. Do not start a competing
launcher. Live authority is the controller's remote `state-r07/controller-state.json`
under `/root/iot-feasible-controller-20261107-1810`, not a stale workstation copy.

The launcher now shares an explicit strict SSH/SCP configuration. The controller
uses native SUT → load access, not the unreliable workstation direct path.
A dedicated source/forwarding-restricted **root-capable** key is private to the
SUT; it is not command-restricted, and the general workstation key was not copied.
The administrative connection is not the measured telemetry path. No national
filtering cause is established.

FW repetition 2's original remote run completed despite local launcher
cancellation. Its raw evidence and original failed-history assessment were
preserved; independent recovery review accepted request/sink/history identity
reconciliation, and named services were safely stopped with cleanup verified.
History was exported at recovery, **not proven visible by the original cutoff**;
later reboot/lock absence is not reconstructed historical controller cleanup.
See [persistent execution](operations/persistent-execution.md) for receipts,
detach proof, viewing/pause commands and recovery caveats.

Frozen scope: 25 msg/s, 1,000 devices, payload selector 0, N=2/k=2, application pool `4-5`,
300 s warm-up + 600 s measurement, `GOGC=400`, monitoring enabled; five planned attempts per
platform in the exact interleaved matrix. Primary endpoint is application dispatch after payload
preparation → sink complete-body read, not wire transmission. Per-message offline analysis runs
automatically; send-lag p99 alone is not a rejection criterion under `dispatch-integrity-v1`.

| Entry | Attempt | State | Measurement dispatch p99 |
|---|---|---|---|
| 1 | `disp-tb-r01-20261106` | Setup failure: first login HTTP 503; 0/22,500 offered/planned telemetry; no measurement. Recovery accepted, cleanup verified; no replacement. | Not assessed |
| 2 | `disp-fw-r01-20261106` | Valid platform/history success; 22,500 exact request/sink/history identities; cleanup verified. | 107.137939453125 ms |
| 3 | `disp-ex-r01-20261106` | Valid platform/history success; 22,500 exact request/sink/history identities; cleanup verified; no new metadata panic/restart. | 7.9072265625 ms |
| 4 | `disp-fw-r02-20261106` | Original completed attempt recovered and independently accepted; 22,500 exact identities; delayed history/cleanup caveats above. | 122.3681640625 ms |
| 5 | `disp-ex-r02-20261106` | Valid platform/history success; 22,500 exact identities; cleanup verified; no new metadata defect. | 8.38134765625 ms |
| 6 | `disp-tb-r02-20261106` | Independently accepted valid platform delivery failure: 200 warm-up identities absent at sink cutoff; full history; safe finish/cleanup. | 203.120849609375 ms (15,000/15,000 measurement receipts) |
| 7 | `disp-ex-r03-20261106` | New persistent controller preparing/running original attempt; no result accepted yet. | Pending |

Six original attempts have completed: four valid platform successes, one
zero-traffic setup failure and TB repetition 2's valid platform delivery failure.
Nine original entries remain including the active `disp-ex-r03-20261106`;
eight further entries have not started. TB still has five planned attempts and at most four valid measured
repetitions; a valid platform failure is not an invalid measurement or a reason
to substitute another run.

TB repetition 2 sent all 22,500 planned identities. Measurement delivered all
15,000; warm-up delivered 7,300/7,500 by cutoff, with conditional p99
33,786.958740234375 ms and 200 missing. Of 36 warm-up UNKNOWN HTTP outcomes,
22 reached the sink and 14 did not; the other 186 missing sink identities were
acknowledged. All 22,500 identities were independently found in history. There
were no duplicate, late or malformed receipts, and no clock/capture/resource
failure was reported. Do not discard warm-up, infer permanent loss beyond the
observation window, or claim a cold-start/network root cause from these counts.

The separate 625-message loopback diagnostic passed and survived the initiating
SSH disconnect: reconnection witnessed tmux/k6 active at 208/625; final wrapper
exit was 0, with 625 exact request/receiver identities and verified cleanup.
It is not a qualification, headroom result or ordinary repetition. Independent
artifact review issued `ACCEPTED_PERSISTENT_ORDINARY_CONTINUATION` for exact
original entries 5–15, bound to frozen plans, complete source/binary manifests
and a separate reviewed-probe receipt. The raw probe's false review flag remains
unchanged. No successful run or failed TB attempt was rerun or replaced.

Evidence and execution records:
- [Approved plan and chronological checkpoints](../results/feasible-campaign-20261106/PLAN.md),
  [frozen matrix](../results/feasible-campaign-20261106/run-matrix.json), and
  [source/evidence policy](../results/feasible-campaign-20261106/README.md) are versioned.
- Externally retained under `results/feasible-campaign-20261106/` (not included
  in Git): `campaign-progress.json`, `independent-review-ordinary-release.json`,
  `independent-review-tb-r01-recovery.json`, `independent-review-fw-r02-recovery.json`,
  `independent-review-persistent-continuation.json`, `independent-review-persistent-r07-continuation.json`,
  `controller-pause-r06-20261108/`, `persistence-probe-20261108a1/`,
  `resume-check-20261107.json`, `evidence/` and `aggregate/summary.md`. A reviewed public archive location is
  still an owner publication decision; these are provenance paths, not shipped datasets.

Full repaired FIWARE and EdgeX endpoint/headroom qualifications were independently accepted,
each reconciling 7,500 request/sink/history identities. TB qualification reuse carries an explicit
**reconstructed host-provenance caveat**, not a contemporaneous sink-process attestation.
Phase-aligned SUT CPU-cost snapshots were unavailable (HTTP 503): **no phase CPU-cost claim**.
Optional plotting is unavailable and nonblocking. No capacity/scalability, hardware/framework
upgrade, image replacement, data reset, historical-device deletion or automated replica-stop
experiment is authorized or implied by these bounded results.

### Generator progress logging follow-up — 2026-11-07

Owner requested more visible generator progress and a short successful-send
report. The local runner now observes its existing complete outcome log every
10 seconds, printing completed/planned tick counts, ACK/failure/unknown totals
and the latest completed-response phase. Future builds write
`generator-progress.log`, `generator-progress.json` and `generator-send-report.md`
inside each run directory, with per-phase counts and a final k6 exit status.
No k6 request-timing code, frozen workload or verdict gate is changed. ACKs are
not sink/history delivery; 100% does not mean drain/analysis/cleanup completed.
Details and live-view commands: [generator progress guide](development/generator-progress.md).
This observer is now proven by the detached diagnostic and included in the
hash-verified tools used for the remaining campaign. It does not retroactively
add progress logs to older runs or replace the separate FW2 recovery review.
Validation: `go test -race -count=1 ./internal/runner ./cmd/benchctl` and
`git diff --check` passed. Focused tests cover partial/incremental log reads,
per-phase outcomes, duplicate and invalid identities, zero-response setup aborts,
preservation of raw logs and existing progress evidence, and ACK/delivery caveats.

### Repository handoff and testing documentation — 2026-11-07

The owner requested documentation/commits rather than more runtime work. Start
with the [publication checklist](operations/publication-handoff.md),
[ordinary-campaign runbook](operations/ordinary-campaign.md) and
[test catalogue](development/testing.md). Frozen control files are versioned;
raw evidence, review receipts and mutable campaign progress are not. The broader
historical requirements below are not silently declared complete. Publication
still needs owner licence/citation decisions, a full-history secret scan and a
stable reviewed archive location. No GitHub push or licence choice is implied.
Independent re-review accepted the source/documentation handoff after packaging,
readiness wording and diagnostic progress corrections. That acceptance does not
clear public release, runtime continuation, FW2 acceptance or experiment completion.

### Separate W1–W4 configuration batch staged

The owner approved four one-factor fixed-point configurations for all three
platforms: W1 rate 50/s, W2 2,000 devices, W3 1,024 extra logical padding bytes,
and W4 one replica per replicated service on the **same CPU pool 4–5/k=2**.
The active B0 controller, frozen settings and existing attempt IDs are unchanged.
Independent review accepted staging/documentation only, not runtime execution.

The new [batch controls](../workloads/campaigns/fixed-points-w1-w4/README.md)
contain 60 distinct interleaved experimental entries: **five attempts per
workload/platform proposed**, separate from B0's existing five. All 60 generator
plans resolve offline; one-factor/count/interleaving/preservation tests and the
three-adapter logical-padding test pass. These are source/engineering checks,
not live deployment, qualification or experimental results.

#### Measured qualification state — updated 2026-11-14

The staging text above describes the batch as it was reviewed. Since then the
**release qualifications** for W1, W2 and most of W3 have run. These are
qualifications: they use their own run IDs and seeds, and **none of them counts
toward the 60 proposed attempts**. `execution_authorized` remains `false` and the
matrix remains `PROPOSED_NOT_LAUNCH_CLEARED`.

| Workload | ThingsBoard | FIWARE | EdgeX |
|---|---|---|---|
| W1 (50/s, 1,000 devices) | `VALID` — 45,000/45,000, p99 347 ms | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` — 13,481/30,000 over the 5 s timeout, p99 ~35.3 s, harness clean | `VALID` — 45,000/45,000, p99 8.5 ms |
| W2 (25/s, 2,000 devices) | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` | `VALID` | `VALID` (retained inventory 13,192 confirmed) |
| W3 (25/s, 1,000 devices, +1,024 B) | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` (warm-up only; measurement 15,000/15,000 clean, p99 257.7 ms); payload gate **PASS** | `VALID_PLATFORM_SUCCESS` — 22,500/22,500, p99 407.3 ms; stored-payload gate **UNAVAILABLE** | `VALID_PLATFORM_SUCCESS` — 22,500/22,500, p99 15.5 ms; stored-payload gate **PASS** (22,500/22,500) |
| W4 (25/s, 1,000 devices, N=1) | not launched | not launched | not launched (1 of 5 repetitions admitted; see below) |

Reading this table:

- **A platform failure is a result, not a defect to repair.** FIWARE failing W1 at
  50/s is an accepted finding. It is not to be re-run to obtain a better number.
- **W3's three platforms disagree legitimately.** ThingsBoard preserved the padding
  end to end but its delivery verdict is a warm-up cold-start artifact; FIWARE
  delivered perfectly but its frozen subscriptions never forward `padding`, so the
  stored payload is unobservable. EdgeX stores the whole canonical object, so its
  stored-payload gate is both available and passed. A single pass/fail column would
  misstate all three.
- **The W3 FIWARE stored-payload gate stays explicitly unavailable.** It must never
  be read as passed. Making it observable requires the documented subscription
  change, a new receipt and a comparability note.
- **W4 has not been measured anywhere.** Its N=1 configuration is now implemented and
  verified on all three platforms, but **only 1 of its 5 EdgeX repetitions fits under
  the frozen 16,384-device discovery limit**. Retained EdgeX inventory is 15,192;
  one further 1,000-device cohort reaches 16,192 and fits, the next would reach
  17,192. EdgeX also refuses `DELETE`, so the inventory cannot be reclaimed. See
  [`research/findings/w4-edgex-inventory-blocks-four-repetitions-20261114.md`](../research/findings/w4-edgex-inventory-blocks-four-repetitions-20261114.md).
- **The W4 launch is blocked on infrastructure, not on the benchmark.** The first
  attempt (`w4dp-tb-r01-20261108`) was aborted twice **before any traffic** and then
  the load generator host went offline. No matrix row was consumed: an attempt counts
  only once it opens a traffic window, and none did. Two harness defects found in the
  process are fixed (a transport failure was read as "evidence already exists"; the
  launch gate verified 3 of the 13 receipt bindings, ignoring `b0_launcher`). See
  [`research/findings/w4-r01-aborted-pretraffic-20261114.md`](../research/findings/w4-r01-aborted-pretraffic-20261114.md).
- **The testbed address is now configuration, and the load host is operational again.**
  Hosts, ports, keys, paths and the sink port come from one git-ignored `.env`
  (`scripts/iotbench_config.py`, `.env.example` documents it); every code default still
  reproduces the qualified testbed, so an unconfigured checkout behaves as before. Frozen
  plans are never rewritten: a moved address produces a recorded target-only overlay
  (`plan-overlay.json`) that proves no workload field changed. The replacement load host
  runs the pinned k6 image and completed a fixture pilot with `achieved_ratio=1` on every
  phase, so the generator path is verified independently of any SUT. See
  [`docs/operations/fresh-host-deployment.md`](operations/fresh-host-deployment.md).
- **The replacement load host is Ubuntu 22.04, not the pre-registered 24.04.** 4 vCPU /
  7.75 GiB matches the pre-registered size, cgroup v2 is unified, and Docker 29.1.3 with
  Compose 2.40.3 runs the pinned generator image. The OS and CPU model differ from the
  qualified host. That is a comparability statement, not an operational footnote: it must
  be recorded as a deviation before any run whose evidence will be pooled with earlier
  cells, and it is not yet resolved.

The detailed per-run narratives, including every superseded intermediate
qualification and the harness defects found along the way, are in the internal
record under [`research/findings/`](../research/README.md), not here.

#### What each workload still needs

W1 and W2 have run their qualifications on all three platforms. W3 has run on all
three. W4's N=1 configuration is implemented and verified but has not run.

- **W1** — measured on all three. ThingsBoard and EdgeX `VALID`; FIWARE
  `INVALID_PLATFORM_FAILURE_OR_BACKLOG`, accepted as a platform finding. Its former
  50/s headroom-policy gap was resolved by the rate-derived generator pool (300 VUs
  at 50/s), applied 2026-11-10.
- **W2** — measured on all three. FIWARE and EdgeX `VALID`; ThingsBoard
  `INVALID_PLATFORM_FAILURE_OR_BACKLOG`. The live retained EdgeX inventory check was
  satisfied by making the inventory gate reserve the actual cohort rather than a
  projection.
- **W3** — measured on all three. EdgeX is `VALID_PLATFORM_SUCCESS` and, unlike
  FIWARE, its stored-payload gate is available and passed (22,500/22,500 rows,
  1,024 bytes of padding on every one). The FIWARE stored-payload gate and the
  transport/wire measurement remain unavailable by design: current FIWARE sink and
  history subscriptions do not retain padding, and no on-wire capture exists in this
  harness. Computed header bytes are not measured wire bytes.
- **W4** — **not measured on any platform.** The N=1 deployment it requires is now
  implemented: replica count is a launcher parameter defaulting to the previous
  value, an explicit two-core pool `4-5` survives the N=1 mapping that would
  otherwise pick a single core, and all three platforms were verified live at
  exactly one replica per replicated service before any attempt was authorized. Its
  five EdgeX repetitions are limited to one by the frozen device-discovery cap
  described above.

"Measured" does not mean "cleared". No workload is released for the 60 proposed
attempts, and a `VALID` qualification on one platform does not qualify the workload
across platforms.

These remaining prerequisites do not establish infeasibility. Do not modify active
B0 to clear them, delete history/devices or count qualification as repetitions.

For the configuration table, rationale, evidence boundaries and comparison
limits, see [workload methodology](methodology.md#8-workload-configurations-b0-and-the-separate-w1w4-batch).

## Historical qualification and decision record (through 2026-11-05)

---

## 1. Qualification state (blocks B1–B17)

Reconciled from `research/qualification/qualification-blockers.md` (the row-level record of truth), the 2026-10-29 interpretation
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
| B10 | Run | PARTIAL | Approved initial ceiling remains **200/s conditional on the actual configuration qualifying at 400/s**. Earlier loopback and 100-device fixture results retain their limited scope. New 1,000-device ThingsBoard-adapter immediate-response run is generator-VALID at 400/s, but its independent receiver ledger is not yet retrieved; delayed and remaining adapter attempts were interrupted by SUT network loss. No common campaign ceiling is newly qualified. See `research/archive/sessions/qualification-next-20261031.md`. |
| B11 | Claim (cross-host timing) | OPEN | Corrected probe: load↔SUT offset ∈ [−63, +50] ms. No SLO exists yet; U ≤ P/10 unadopted. Cross-host timing claims remain unqualified. |
| B12 | Run (sink capacity) | PARTIAL | Sink qualified AT 3200 r/s (0 unparsed/overflow, 240k msgs). Own envelope stress beyond the ceiling steps not yet run. |
| B13 | Run (monitoring cost) | OPEN | SUT-side exporters installed (cpuset 0, 128m limits); their cost inside a 2× check not isolated. |
| B14 | Claim (identity exports) | CLOSED | All three platforms verified with non-empty data: FIWARE 3,001/3,001; EdgeX 600/600 exact; TB device/ts export verified earlier. |
| B15 | Overload stage | OPEN (owner) | D1 anchor + D2 uncertainty method; owner decision per `docs/methodology/methodology-reconciliation.md` O5/O6. |
| B16 | Claim (reproducibility) | MAINTAINED | 758/758 historical manifest files re-verify (`sha256sum -c evidence/evidence-integrity.sha256`); newer sessions carry separate SHA256SUMS. Publication of the inventory happens at report time (see §5). |
| B17 | Scaling claims | OPEN | S1–S5 amendments recorded (D35–D39), not yet experimentally validated. |

The per-row blocker text, evidence and history remain in `research/qualification/qualification-blockers.md` (2026-10-27 basis,
maintained through 2026-10-29). This section reconciles it against the 2026-10-29 correction; rows above carry
the reconciled wording. **2026-10-29 addition:** the D33 measurement-integrity components (identity/audit chain,
D33 items 1–7) were live-validated for the first time in the bounded rehearsals on all three platforms —
`results/rehearsals-20261029/` (TS `REH-TB-V2`, `REH-EDGEX-V1`, `REH-FIWARE-V4`, each 3,300/3,300 identities
ack → sink → history with 0 duplicates/orphans/malformed; session close-out
`research/archive/sessions/verify-20261029.md`).

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
`research/archive/sessions/autonomous-handoff-20261029.md` line 19 lists “D29a WAL live verification (pg-counters
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

**Historical policy record:** the orphan-publication strategy below is retained
from the earlier checkpoint, not current authorization to rewrite history or
force-push. The current [publication handoff](operations/publication-handoff.md)
requires explicit owner decisions and grants no push/history-rewrite permission.

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

Full follow-up handoff: `research/archive/sessions/verify-followup-20261031.md`.

## 9. Current blocker: SUT network loss during 1,000-device qualification

The continuation from `9c7fc5f` is retained in `results/qualification-next-20261031/` and
`research/archive/sessions/qualification-next-20261031.md`. All unrelated benchmark stacks were
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
handoff: `research/archive/sessions/recovery-check-20261101.md`.

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

Scoped recovery and review packet: [`research/archive/sessions/m0a-baseline-recovery-20261103.md`](../research/archive/sessions/m0a-baseline-recovery-20261103.md);
raw evidence: [`results/m0a-20261103-020828+0330/`](../results/m0a-20261103-020828+0330/).
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
[`m0a-independent-review-20261103.md`](../research/archive/sessions/m0a-independent-review-20261103.md).
The preceding recovery narrative is retained as originally reported. Its 22:38:39 timestamp,
memory observation, failed guard, bounded wait and original test results are session-reported;
not every underlying transcript was retained. The retained pre-stop transcript begins at
22:39:04 UTC. No-restoration was session-reported, not independently established by a post-stop
container inventory. These provenance limitations do not invalidate the corroborated receiver
closure, hashes or identity reconciliation. Do not regenerate historical transcripts.

| Task | Dependency | Owner / requested model | State | Evidence / last check | Next action |
|---|---|---|---|---|---|
| M0a recovery | Current-state inventory | Luna `openai/gpt-6-luna#high`; independent Astra `openai/gpt-6-astra#high` | APPROVED, scoped | Recovery bundle above; independent review 2026-11-03 | Preserve evidence; no repeat recovery needed |
| M0b preliminary contract | Initial Astra design review; preserved baseline | Luna `openai/gpt-6-luna#high`; independent Astra `openai/gpt-6-astra#high` | APPROVED, offline foundation only | [Final independent review](../research/archive/sessions/m0b-independent-review-20261103.md#final-r4-re-review--2026-11-03); all R1–R7 closed, 2026-11-03 | Preserve approved foundation; integrate explicit consumers next |
| M1a protocol-bound cohort analysis | Approved M0b | Implementation: Luna sessions, requested `openai/gpt-6-luna#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m1a-approval-20261104.md); fix packet `results/m1a-fix-packet-20261104-005644+0330/`; review `ses_e5ca28f17ffezpYAWTi38I5QWO`, 2026-11-04 | Preserve; runtime receipt/clock qualification remain outstanding |
| M1b offline end-to-end path | Approved M1a | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m1b-approval-20261104.md); packet `results/m1b-pipeline-20261104-013816+0330/`; review `ses_e5c831c28ffeZ2P8S8xUfbkZkA`, 2026-11-04 | Preserve; live qualification remains outstanding |
| M1c payload golden fixture | Approved M1b | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m1c-approval-20261104.md); packet `results/m1c-payload-fixture-20261104-020102+0330/`; review `ses_e5c73aa2fffeCZJFCdsIesIv8s`, 2026-11-04 | Preserve; live payload-storage qualification remains outstanding |
| M1d complete-body receiver | Approved M1c | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m1d-approval-20261104.md); packet `results/m1d-complete-body-20261104-024117+0330/`; review `ses_e5c40004cffeqr4BbZeigwIZ1w`, 2026-11-04 | Preserve; no deployment or clock qualification |
| M1e adapter payload controls | Approved M1d | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m1e-approval-20261104.md); packets `results/m1e-*`; review sessions `ses_e5bf776acffewIermuL5GrjYPw`, `ses_e5bda0110ffezYcVouts5ruYrK`, `ses_e5bc73feaffes8ZYvVRg1HmbVl`, 2026-11-04 | Preserve; no deployment or runtime evidence |
| M1f manual fault/capacity reducers | Approved M1e | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/kimi-k3.1-pro#low` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m1f-approval-20261105.md); packets and review under `research/archive/sessions/` and `results/m1f-*`, 2026-11-05 | Preserve; live fault/deployment qualification outstanding |
| M2 deployment/instrumentation foundation | Approved M1f | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/kimi-k3.1-pro#low` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m2-approval-20261105.md); fix packet `research/archive/sessions/m2-fix-20261105.md`; review sessions under [`m2-independent-review-20261105.md`](../research/archive/sessions/m2-independent-review-20261105.md), 2026-11-05 | Preserve; live/deployment qualification outstanding |
| M2b deployment orchestration blueprint | Approved M2 | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m2b-approval-20261105.md); packets `research/archive/sessions/m2b-*`; review `ses_e55da6cd4ffeV6C6UiZxuwBTLJ`, 2026-11-05 | Preserve; no lifecycle/live claim |
| M2c offline render/verify validation | Approved M2b | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m2c-approval-20261105.md); packet `research/archive/sessions/m2c-render-verify-20261105.md`; review `ses_e557e06aefferRFlsPQc9HpYVD`, 2026-11-05 | Preserve; nonblocking F1/F2/F3 noted; no live claim |
| M2d offline execution-plan render | Approved M2c | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review: `opencode-go/deepseek-v4-pro#high` | APPROVED, scoped offline | [Approval record](../research/archive/sessions/m2d-approval-20261105.md); packet `research/archive/sessions/m2d-execution-plan-20261105.md`; review `ses_e55615fd5ffeRiVCyh2iO9Qs1i`, 2026-11-05 | Preserve; no lifecycle/live claim |
| M2e health/evidence dry-run | Plan by Kimi; implementation by DeepSeek Flash | Implementation: `opencode-go/deepseek-v4.1-flash#high`; review intentionally waived by owner | COMPLETE, SELF-TESTED, NO INDEPENDENT REVIEW | Packet `research/archive/sessions/m2e-health-evidence-20261105.md`; evidence `results/m2e-health-evidence-20261105-113210+0330/`; 2026-11-05 | Synthetic offline only; independently unverified by owner choice; no live claim |
| M3 live preflight | M2e self-test; approved offline foundations | Orchestrator session | COMPLETE READ-ONLY (pre-deployment snapshot) | `results/live-preflight-20261105T080836Z/` with Gate A/B/C captures and SHA256SUMS, 2026-11-05 | Host state now superseded by the bounded live smoke; clock offset still unmeasured |
| M3 live smoke | M2e self-test; preflight; source checkpoint `29f5dead` | Orchestrator session | PARTIAL — N1 VALID; N2 INVALID generator gate | `results/live-tb-20261105/README.md`; N1 sink 3300/3300 unique, N2 isolated sink 3300/3300 but warm-up send-lag 13 ms, 2026-11-05 | Preserve runs; rerun N2 with qualified generator configuration before any scaling claim; no capacity claim |
| M3–M7 live qualification/campaign | Approved offline foundations; deployed producers; calibrated clock/drain/recovery policies | Not started | BLOCKED at session checkpoint | No live revised-scope evidence; both hosts were reachable in a bounded read-only check at 2026-11-05 07:37 UTC, with only exporters/monitoring running; no deployment or load was performed | Resume live preflight and deployment preparation; campaign remains NO-GO |
| Protocol freeze | M0b, producer/analyzer integration, policy calibration | Astra review | BLOCKED on evidence | Initial review CHANGES_REQUIRED for campaign freeze | Qualify policies; keep campaign eligibility fail-closed |
| M1–M7 remaining scope | Approved contract and preceding milestone gates | Luna implementation / Astra independent review | NOT_COMPLETE | No revised-scope acceptance evidence | Follow dependency-ordered handoff; manual fault cells require operator |

Revised-scope status: **SOFTWARE_VERIFIED=false**, **LIVE_QUALIFICATION_COMPLETE=false**,
**CAMPAIGN_COMPLETE=false**, **ARTIFACT_REPRODUCIBLE=false**. Campaign remains **NO-GO**.

### Session checkpoint — 2026-11-04

Offline milestones M0a–M1e completed with independent review (approval records under
`research/archive/sessions/`). No live qualification or campaign run was performed. Resumable handoff:
[`session-checkpoint-20261104.md`](../research/archive/sessions/session-checkpoint-20261104.md). The next
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

Full live qualification and campaign execution remain blocked pending the remaining protocol
freeze, artifact digests, clock/capture procedure, output paths and rollback plan.

### Current qualification continuation — 2026-11-05

The concise, evidence-linked open-gate ledger is
[`research/qualification/readiness-checklist-20261105.md`](../research/qualification/readiness-checklist-20261105.md).
M1f remains **complete/approved for offline reducers and manual-operator attestations**; live fault
execution is outstanding. The existing N=2 failure has not been attributed to CPU or VU
starvation. First-attempt UNKNOWN seq 0–17 are each 5 s transport timeouts and all 18 later appear
at sink; the second attempt has no unknowns. Both fail unchanged warm-up send-lag p99 ≤10 ms at
13 ms. Peak VUs were 50 then 4, k6 memory peaks 415,305,728 B and 382,894,080 B, and all
generator-host core-busy maxima stayed below 50%. Phase-aligned SUT counters and throttling
counters are absent, so a limiting component is **not identified**; no identical retry or gate
relaxation is authorized by this evidence.

Raw identity reconciliation adds an important scope detail: the first N=2 run had 3,186/3,300
sink identities at capture end; all 3,000 measurement identities arrived, while 114 absent
identities were in warm-up. All 18 UNKNOWN identities were among the receipts, so the 114 absent
warm-up identities are within the aggregate ACK count; the legacy ledger lacks success-per-message
rows to name them. This INVALID run is not a platform verdict. The second N=2 run had 3,300/3,300
sink identities.

Current read-only inventory/CPU/clock captures are in
[`results/qualification-live-20261105T1045Z/`](../results/qualification-live-20261105T1045Z/).
The 30-sample clock probe has an all-sample interval union [−2.96,145.06] ms (one RTT outlier
147.51 ms); constant-offset intersection [−1.74,0.74] ms does not bound unsampled drift. The
primary load-host-local scheduled→sink clock does not require SUT clock conversion. The temporary
source-restricted, read-only capture helper is running on SUT port 18085, allowed only from the
load host; no reboot or clock adjustment occurred. Full qualification remains **NO-GO**.

Protocol integration milestone is **implemented and independently code-reviewed; final N=1 engineering-pilot evidence passed independent integrated review**.
Protocol-bound runs now derive full message outcome logging from resolved protocol metadata; analysis
checks the complete constant-arrival ledger against plan and k6 summary; native sink envelopes retain
per-identity device/scheduled-send metadata; live/synthetic provenance disagreement fails closed; and
`scripts/protocol-receipts.py` assembles only complete, identity-reconciled full-body captures. GPT-6.1
Sol reviews returned and the implementation addressed findings on caller environment override, plan/counter
validation, repeated-identity metadata, k6 scenario-start timing tolerance, sink lifecycle ownership, and
final capture-window/counter validation. Fourteen receipt/firewall tests and focused Go packages pass.
The first full protocol-bound N=1 run completed its 900-second relative cutoff, quiet interval and late
observation, but strict receipt assembly failed because every ThingsBoard sink receipt omitted `device_id`.
Read-only history was correctly not used to fill the missing receipt field; the independent integrated
review disposition is FAIL/BLOCKED. User approved canonical `logical-json-ascii-v1` baseline offset zero;
Sol reviewed the amended plan with no gate changes. A separate one-message diagnostic carried the actual
device name into the sink and passed strict receipt assembly (diagnostic only). Fresh full N=1 run
`tb-n1-protocol-pilot-20261105-final2` then completed with 18,000/18,000 sink observations, strict receipt
assembly, zero missing/duplicate/late measurement receipts, and clean read-only history reconciliation.
Claim-gated analysis and exact archived-input reproduction are retained in
`results/live-protocol-tb-n1-20261105T1340Z/analysis-final2-final/` and
`analysis-final2-final-reproduction/`; GPT-6.1 Sol's integrated PASS is retained in
`attempt-final2/integrated-review.md`. This clears the N=1 engineering-pilot review prerequisite only.
N=2 generator-readiness gates remain unresolved and N=3 remains unassessed; no live advancement was
performed. Campaign remains **NOT ASSESSED / NO-GO**; Kafka lag is unavailable, cross-host clock error
and exact-phase CPU cost are unqualified, and no capacity or payload-envelope claim is supported.

### Bounded live smoke — 2026-11-05

Owner authorization received for the recommended path. Source checkpoint `29f5dead9d475fa0a41763a772252623673b350c`;
new Compose project `tbq20261105` with fresh volumes; old projects, volumes, ledgers and load-host
sink preserved. ThingsBoard 4.3.1.6 uses explicit 1280 MiB tb-core/rule-engine and 768 MiB
transport limits, application cpuset `4` at N=1 and `4-5` at N=2, supporting tier `2-3`.

- Smoke: 100/100 HTTP messages stored, all 200 responses. The first smoke invocation reported a
  false rule-engine failure because it queried the old Compose project; the corrected invocation
  passed all checks.
- `live-tb-n1-25-valid-20261105`: generator **VALID**. 3,300/3,300 sends acknowledged, zero
  drops/errors/unknowns; new sink received 3,300 unique identities with zero duplicates,
  overflow, unparsed records or body errors.
- `live-tb-n2-25-20261105`: INVALID. Warm-up send-lag p99 13 ms; 18 warm-up unknowns; sink
  received 3,186 unique identities.
- `live-tb-n2-25-isolated-20261105`: INVALID. All 3,000 measurement sends acknowledged and sink
  received 3,300 unique identities, but warm-up send-lag p99 remained 13 ms. This is a generator
  gate failure, not a platform performance verdict.

Evidence: [`results/live-tb-20261105/README.md`](../results/live-tb-20261105/README.md). These
runs use the legacy `legacy-acknowledgement-only-v1` interpretation; they do not provide
scheduled-cohort delivery p99, payload-size results, capacity, replica-failure or campaign
evidence. N=2 currently needs a qualified generator configuration and longer warm-up before any
scaling claim. The live deployment is left running; no reset/delete occurred. Campaign remains
**NO-GO**.
