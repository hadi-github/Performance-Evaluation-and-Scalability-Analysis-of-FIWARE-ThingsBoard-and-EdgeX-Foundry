# Session checkpoint — 2026-11-04

Resumable handoff after the M1e approval. This checkpoint does **not** represent a completed
benchmark campaign or live qualification.

## Completed and independently approved (offline scope only)

| Milestone | Scope | Independent approval |
|---|---|---|
| M0a | Dirty-work preservation; stopped only authorized receiver PIDs; recovered and reconciled interrupted ledgers | [M0a review](m0a-independent-review-20261103.md) |
| M0b | Preliminary pilot protocol, canonical payload contract, Go/JS golden vectors | [M0b review](m0b-independent-review-20261103.md) |
| M1a | Protocol-bound plan/manifest/analyzer; scheduled-cohort observed-delivery metrics; legacy separation | [M1a approval](m1a-approval-20261104.md) |
| M1b | Real offline prepare→manifest→finish→analyze path; synthetic labeling | [M1b approval](m1b-approval-20261104.md) |
| M1c | Payload-producer golden fixture; 39 vectors; three-language byte parity; stored-content validation | [M1c approval](m1c-approval-20261104.md) |
| M1d | Complete-body receipt contract, failure handling, legacy schema preservation | [M1d approval](m1d-approval-20261104.md) |
| M1e | Adapter payload controls, stored-padding exports, boundary response semantics, ledger fix, worst-case bound | [M1e approval](m1e-approval-20261104.md) |

Implementation route for M1b–M1e was requested `opencode-go/deepseek-v4.1-flash#high`; review was
requested `opencode-go/deepseek-v4-pro#high`. Harnesses identified the respective model families;
separate effort-tier confirmation was not exposed.

## Not done

- M1f offline manual-fault target resolver, event reducer and capacity outputs.
- Deployment of the new sink receipt schema, adapters, provisioning and exporters.
- Clock/drain/recovery calibration and campaign-policy freeze.
- Generator/receiver qualification for the revised payload, device, rate and replica envelope.
- Any live N=1/2/3 qualification, platform resource evidence, payload/device sensitivity runs,
  degraded-capacity search, stop/rejoin experiment or independent confirmation repetitions.
- Campaign manifest execution, figures/tables from new evidence and final independent audit.

## Current environment

At the start of this session, both hosts were reachable. The SUT reported `ramona-vm-2`,
8 VMware vCPUs, 16,375,816 kB MemTotal, benchmark platforms stopped and four exporters running.
The load host reported `ramona-vm-q` with monitoring and yaade running. The two old receiver
processes were stopped by M0a; their ledgers were preserved. No platform stack was started and no
live load was run in this session. Re-verify host identity, clocks and container state before any
live operation; do not assume the recorded state persists.

## Revised-scope completion

- `SOFTWARE_VERIFIED=false`
- `LIVE_QUALIFICATION_COMPLETE=false`
- `CAMPAIGN_COMPLETE=false`
- `ARTIFACT_REPRODUCIBLE=false`

Offline tooling has independent scoped approvals, but the four completion claims require live
qualification, campaign evidence and reproduction from archived inputs.

## Resume order

1. Implement and independently review M1f offline (manual fault target/event reducers, capacity
   outputs). No unattended fault activation.
2. Deploy new sink/adapters/exporters only to verified project resources, preserving old ledgers
   and using new output paths. Record image/binary hashes and effective configuration.
3. Calibrate and freeze clock uncertainty, drain/capture deadlines, recovery windows and capacity
   predicate before collecting paper evidence.
4. Qualify generator/receiver and each platform at N=1/2/3 for the declared qualification matrix.
5. Execute the frozen campaign with independent repetitions; archive hashes before resets.
6. Re-run representative offline analyses from archived raw evidence and complete the independent
   final audit.

Do not mark campaign complete or report revised-scope platform results until steps 2–6 produce
valid evidence. A valid platform FAIL or measured non-recovery is a result, not a software defect.
