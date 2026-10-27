# Autonomous session plan (Prompt 6, started 2026-10-27)

**Session ID:** `auto-20261027`
**Started:** 2026-10-27T10:08:50Z · **Hard deadline:** 3 h (~13:10Z); last 30 min (12:40Z+) reserved for close-out.
**Baseline HEAD:** `73dea4d5493280125c7d330356a90433d78889d2` · dirty baseline: 9 modified tracked files + 26 untracked
paths, diff saved at `results/auto-20261027/baseline-tracked.diff` and inventory `baseline-untracked.txt` (evidence
spool). Nothing committed before this session; baseline preserved — no resets, cleans, stashes.

## Owner authorization (recorded verbatim summary)

The owner authorized, for **this session only** and recorded here in full force (supersedes earlier local-only
restrictions for these operations; it does not prove their technical prerequisites passed):

- Access to SUT `87.107.191.107` and load host `87.107.191.32` via existing configured SSH aliases; verify which
  host is which; no credential capture anywhere.
- Read-only preflight Gates A–C; local implementation/test/review; deployment of test candidates in dedicated
  benchmark environments after collision/resource checks.
- Controlled restart/recreation of **our benchmark** ThingsBoard test services for D28 live verification, if it
  affects only our deployment and preserves its data; else isolated candidate.
- **B3 choice: Orion 4.5.0 ↔ QuantumLeap 1.0.0 live pair-test. Orion stays 4.5.0. No downgrade.**
- Isolated FIWARE history candidate: fresh volumes, db setup, subscriptions, probes, history export (B2–B5).
- Read-only D27 registry audits; **no index creation** on an existing registry.
- ThingsBoard feasibility **F1–F2** (k=1, k=2), bounded smokes, schema/export checks, restart-visibility tests
  on disposable data, generator/sink/monitoring qualification.
- New evidence in new, uniquely named session/run dirs incl. `results/`; pre-existing evidence preserved.
- Documentation/status updates and **local commits of this session's changes only**.

**Not authorized:** deleting existing DBs/volumes, overwriting retained results, global docker prune, touching
unrelated services, host-wide settings or clock changes, threshold changes, version switches, D27 live index
creation, F3/k=4, optional tuning, push/merge/publish, main campaign. Own disposable resources cleaned at end.
If a restart would affect unrelated workloads or require deleting existing state ⇒ isolated candidate; else block.

## Host mapping (verified 2026-10-27 from /etc/hosts + SSH, not inferred)

| Role | IP | alias | uname hostname |
|---|---|---|---|
| **SUT** | 87.107.191.107 | ramonavm2 | `ramona-vm-2` |
| **Load** | 87.107.191.32 | ramonavm1 | `ramona-vm-q` |

## Queue (state machine: READY / IN_PROGRESS / IMPLEMENTED_UNVERIFIED / VERIFIED / BLOCKED / DEFERRED)

| # | Task | Gate | Depends | Acceptance | State |
|---|---|---|---|---|---|
| 0 | Session records + authorization log + baseline capture | — | — | files exist, diff saved | VERIFIED |
| 1 | Preflight Gates A–C (both hosts) | B1 | 0 | sanitized transcripts, identity+clock+capacity+collision+docker+pins recorded | IN_PROGRESS |
| 2 | D28 live verification §5 (+controlled restart of our TB services) | B7 | 1 | in-container hashes, TB_SERVICE_ID==hostname, JVM flags, per-replica writable logs, health+low-rate smoke | READY |
| 3 | D27 read-only registry audit (no index creation) + B14 schema/export vs pinned schema | B8, B14 | 1 | exact counts retained read-only; reader fixes only with schema evidence (+regressions) | READY |
| 4 | FIWARE isolated candidate: pair-test + history implementation | B2–B5 | 1 | 4-payload probe, Orion→QL replay, drop audit, restart visibility, `history-ids.sh fiware` export, resource fit; else B4 BLOCKED | READY |
| 5 | TB feasibility F1 (k=1) → F2 (k=2), arithmetic first | B6 | 2 (0 for arithmetic) | boot + idle/loaded RSS + bounded fixed load; stop on documented failure; no F3/k=4 | READY |
| 6 | Clock offset (read-only) · `benchctl resources` vs live Prometheus · generator/sink/monitoring qualification (fixture first, then 2× target rates; labelled ceiling exploration if max rate unknown) · bounded low-rate smokes | B9–B14 | 1,5 (sequence per platform) | measured range retained (no invented thresholds); raw Prom responses saved; qualification per current plans | READY |
| 7 | Close-out: rerun checks, evidence manifest addendum, commits, handoff | — | all | all checks rerun, diff staged (own changes only), resume/rollback documented | READY |

Rules: smallest deliverable first; review actual diffs (subagent for substantial changes); ~30 min or 3 failed
attempts without progress ⇒ BLOCKED + move on; live experiments sequential; timeouts on every remote op.
