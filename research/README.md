# Research record — internal index

This tree is the **working record for the people writing the paper**. It is not the
reviewer entry point. If you are trying to understand, run, or re-run the benchmark,
start at [`../docs/README.md`](../docs/README.md) instead.

The split is deliberate:

| Tree | Audience | Contents |
|---|---|---|
| [`../docs/`](../docs/) | Reviewers, operators | What the system is, how to run and verify it, current authoritative status |
| `research/` (this tree) | Project team, paper authors | Findings, decisions, per-platform behaviour, frozen protocols, historical packets, qualification record |

Nothing here is an instruction to run anything. `scope.json` `execution_authorized`
remains `false` and the W matrix remains `PROPOSED_NOT_LAUNCH_CLEARED`. Findings
describe what was observed; they do not authorize, retry or replace an attempt.

## How to read this tree

Entries are dated in their filenames or headers. Several are explicitly marked
**SUPERSEDED** or **SUPERSEDED IN PART** — those markers are load-bearing. When two
documents disagree, the later dated one wins, and
[`../docs/status.md`](../docs/status.md) wins over both for status claims. Never
edit an older finding to match a newer one; add the correction and mark the old
document superseded.

Superseded-but-retained documents are kept because the paper needs to show what was
believed at the time and why it changed. Do not delete them.

---

## 1. Batch records — the closing statements

Start here for the experimental spine.

- [`findings/b0-final-batch-record.md`](findings/b0-final-batch-record.md) — B0 ordinary
  campaign closed 2026-11-10. 15 cells, each attempted exactly once: 11 valid
  measurements, 6 preserved failures, zero retries, zero replacements.
- [`findings/w-batch-status-20261110.md`](findings/w-batch-status-20261110.md) — W1–W4
  status as of 2026-11-10. **Superseded on the W1 verdict** by the generator-pool
  amendment below; its EdgeX/W2/W3 sections were the state before those were measured.
- [`findings/w3-rollup-20261112.md`](findings/w3-rollup-20261112.md) — W3 roll-up across
  platforms: payload preserved on ThingsBoard, delivered but unobservable on FIWARE.
  **Two of three platforms complete; EdgeX W3 not run.**
- [`findings/w2-2000-devices-measured-20261112.md`](findings/w2-2000-devices-measured-20261112.md) —
  W2 on all three platforms at 2,000 devices.
- [`findings/frozen-control-text-drift-20261113.md`](findings/frozen-control-text-drift-20261113.md) —
  two frozen control files still read as if a settled decision were open. Documented and
  **deliberately not edited**, because their hashes are bound by the matrix and the
  unused EdgeX W3 receipt. Read this before "fixing" `scope.json` or the gates file.

## 2. Per-workload results

### W1 — 50 msg/s, 1,000 devices

- [`findings/w1-qualified-thingsboard-20261111.md`](findings/w1-qualified-thingsboard-20261111.md) —
  `VALID`, 45,000/45,000, p99 347 ms.
- [`findings/w1-fiware-fails-at-50s-20261111.md`](findings/w1-fiware-fails-at-50s-20261111.md) —
  `INVALID_PLATFORM_FAILURE_OR_BACKLOG`. 13,481 of 30,000 breached the 5 s timeout,
  p99 ~35.3 s, with a clean harness. Accepted as a finding, not a defect to fix.
- [`findings/w1-edgex-qualified-20261112.md`](findings/w1-edgex-qualified-20261112.md) —
  `VALID`, 45,000/45,000, p99 8.5 ms.
- [`findings/w1-platform-behaviour-comparison-20261112.md`](findings/w1-platform-behaviour-comparison-20261112.md) —
  the same offered load across all three platforms, side by side. Written for
  interpretation, **not** as a capacity claim; every run is a single observation.

### W2 — 25 msg/s, 2,000 devices

Covered by the batch record above. ThingsBoard `INVALID_PLATFORM_FAILURE_OR_BACKLOG`;
FIWARE and EdgeX `VALID`. EdgeX's retained inventory of 13,192 devices was confirmed.

### W3 — 25 msg/s, 1,000 devices, +1,024-byte payload

- [`findings/w3-thingsboard-payload-preserved-20261112.md`](findings/w3-thingsboard-payload-preserved-20261112.md) —
  padding survived end to end; `INVALID_PLATFORM_FAILURE_OR_BACKLOG` on warm-up
  cold-start only, measurement clean.
- [`findings/w3-payload-evidence-path-20261112.md`](findings/w3-payload-evidence-path-20261112.md) —
  which gate is demonstrated by which evidence, per platform, including what is
  unavailable by design.
- [`findings/fiware-w3-payload-evidence-proposal-20261112.md`](findings/fiware-w3-payload-evidence-proposal-20261112.md) —
  **PROPOSAL ONLY, NOT APPLIED.** A subscription change that would make FIWARE's stored
  payload observable. Applying it needs a new receipt and a comparability note.

### W4 — 25 msg/s, 1,000 devices, one replica

**Not measured on any platform.** No qualification plan is staged and no attempt is
authorized. See [`../docs/status.md`](../docs/status.md).

## 3. Generator and harness behaviour

These documents explain why some runs were invalid for reasons that had nothing to do
with a platform. They matter for the paper's validity argument.

- [`findings/generator-pool-amendment-20261110.md`](findings/generator-pool-amendment-20261110.md) —
  **APPLIED.** The VU pool became rate-derived (300 at 50/s, 150 at 25/s) instead of a
  fixed 150. Supersedes the "W1 is not qualifiable" conclusion.
- [`findings/w1-headroom-gate-is-unresolvable.md`](findings/w1-headroom-gate-is-unresolvable.md) —
  arithmetic of the frozen-pool bind. **Superseded on the remedy** by the amendment.
- [`findings/w1-vu-pool-binds-both-platforms.md`](findings/w1-vu-pool-binds-both-platforms.md) —
  measurements stand, conclusion does not. **Partly superseded.**
- [`findings/w1-vu-pool-headroom-risk.md`](findings/w1-vu-pool-headroom-risk.md) —
  **RESOLVED 2026-11-10.** The predicted bind was real and was fixed at the source.
- [`findings/v1-only-constants-broke-v2-runs.md`](findings/v1-only-constants-broke-v2-runs.md) —
  three v1-only constants disabled v2 runs, one of which **faked a platform failure**.
  Consumed ID, never reused.

## 4. Readiness and blocker analysis

- [`findings/w1-readiness-report-20261110.md`](findings/w1-readiness-report-20261110.md) —
  **superseded in part.** Its blocker 1 was a false reading later corrected.
- [`findings/w1-readiness-after-b0.md`](findings/w1-readiness-after-b0.md) — what was
  verified runnable after B0. Authorizes nothing.
- [`findings/edgex-w-batch-blocker.md`](findings/edgex-w-batch-blocker.md) — the two
  EdgeX blockers, with a measured basis rather than a projection.
- [`findings/edgex-readiness-measured-20261111.md`](findings/edgex-readiness-measured-20261111.md) —
  readiness measured at 224 s against a 468 s projection: **the blocker was overstated
  by about 2×.**
- [`findings/edgex-inventory-reserve-cohort-sized-20261112.md`](findings/edgex-inventory-reserve-cohort-sized-20261112.md) —
  **APPLIED, owner-authorized.** The inventory gate reserves the cohort the run will
  actually create rather than a fixed projection.

## 5. Platform-specific findings

### EdgeX

- [`findings/edgex-device-validation-panic.md`](findings/edgex-device-validation-panic.md) —
  core-metadata panics when a device is added while device-rest is scaled out.
- [`findings/edgex-discovery-limit-and-inventory-scaling.md`](findings/edgex-discovery-limit-and-inventory-scaling.md) —
  what `MaxResultCount` does and does not limit; the B0→W inventory ceiling.
- [`findings/edgex-ordinal-7-harness-false-negative.md`](findings/edgex-ordinal-7-harness-false-negative.md) —
  a harness false negative, **not** a platform failure. Original verdict stands.
- [`findings/edgex-ordinal-15-readiness-failure.md`](findings/edgex-ordinal-15-readiness-failure.md) —
  device-rest never registered its ingest route; zero-traffic setup failure, not retried.
- [`findings/edgex-r11-second-replica-readiness-failure.md`](findings/edgex-r11-second-replica-readiness-failure.md) —
  second replica never exposed its ingest route; zero-traffic setup failure.

### ThingsBoard

- [`findings/thingsboard-warmstart-delivery-gap.md`](findings/thingsboard-warmstart-delivery-gap.md) —
  the five B0 cursors separated into one delivery gap and several harness faults, because
  conflating them would misattribute a platform behaviour.

### Cross-platform

- [`findings/monitoring-findings.md`](findings/monitoring-findings.md) — monitoring state
  and the interface contract for CPU/clock boundary snapshots.

## 6. Frozen protocols

Byte-preserved under [`protocols/`](protocols/). Do not edit; amendments are separate files.

- [`protocols/iotbench-telemetry-v0.1.0-pilot.1.json`](protocols/iotbench-telemetry-v0.1.0-pilot.1.json) —
  the protocol definition, with its `.sha256`.
- [`protocols/iotbench-telemetry-v0.1.0-pilot.1.amendments.md`](protocols/iotbench-telemetry-v0.1.0-pilot.1.amendments.md) —
  the amendment record.
- [`protocols/offline-profiles-20261101.json`](protocols/offline-profiles-20261101.json) —
  profile thresholds and their decision provenance.
- [`protocols/slo-profile-reconciliation-20261029.md`](protocols/slo-profile-reconciliation-20261029.md) —
  reconciliation of the SLO profiles.
- [`protocols/generator-ceiling-20261028.md`](protocols/generator-ceiling-20261028.md) —
  the 2× headroom rule and where it governs.

## 7. Preregistration archive

[`preregistration/preregistration-archive/`](preregistration/preregistration-archive/) holds
the **original bytes** of the preregistration material, with
[`SOURCE-MANIFEST.md`](preregistration/preregistration-archive/SOURCE-MANIFEST.md) and
`SHA256SUMS`. `scripts/check-preregistration.sh` verifies it; run that before claiming the
preregistration is intact.

The archive is deliberately unedited. Dated findings elsewhere in this tree are **not** a
retroactive preregistration — see
[`../docs/research-guide.md`](../docs/research-guide.md) §1 on that distinction.

## 8. Historical record

- [`qualification/`](qualification/) — the row-level qualification blockers (B1–B17),
  D28/D29a verification, the 2026-11-05 readiness checklist, and the Step-5 decision packet
  and owner sign-off. Moved here from `docs/` because it is paper/proposal material, not
  onboarding. `qualification-blockers.md` remains the row-level record of truth that
  [`../docs/status.md`](../docs/status.md) reconciles against.
- [`archive/sessions/`](archive/sessions/) — verbatim session narratives, progress logs,
  handoffs and state snapshots, preserved exactly as written. They may contradict each
  other and contradict later findings; that is intended. `../docs/status.md` wins.

Paths inside archived packets refer to where files lived when they were written (often
`docs/archive/sessions/`). Those references are left unedited on purpose: rewriting a
historical packet would falsify the record it exists to preserve.

## 9. Rules for adding to this tree

1. Date every finding, in the filename or the header. Prefer a dated filename.
2. State the scope of the claim in the header — a single run is not a capacity result.
3. Never retry a consumed run ID and never replace a failure in place. Add the successor
   under its own ID and its own receipt.
4. When you correct an earlier finding, mark it **SUPERSEDED** and link forward. Do not
   edit history to agree with the present.
5. Keep unavailable measurements explicitly unavailable. Do not let a missing gate read as
   a pass.
6. Reviewer-facing facts belong in [`../docs/`](../docs/), not here.
