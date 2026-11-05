# M1f approval — 2026-11-05

**APPROVED for scoped M1f offline foundation.** This covers offline target resolution, manual
event/recovery/capacity reducers, strict schema validation, tests and evidence capture. It does
not approve Docker access, SUT access, deployment, lifecycle/stop/fault execution or any campaign
qualification.

Reviewer: `ses_e5740adf2ffemPKzFBIfYARoi6`, requested `opencode-go/kimi-k3.1-pro#low`; harness
identified Kimi K3.1 Pro and exposed effort `low`. Implementation: DeepSeek V4.1 Flash, requested
`#high`; the harness did not expose a separate effort control.

Packets:
- implementation: [`m1f-fault-capacity-20261104.md`](m1f-fault-capacity-20261104.md)
- first fix: [`m1f-fault-fix-20261105.md`](m1f-fault-fix-20261105.md)
- residual fix: [`m1f-r2-residual-20261105.md`](m1f-r2-residual-20261105.md)
- review findings: [`m1f-independent-review-20261105.md`](m1f-independent-review-20261105.md)

## Verifications

- **M1F-R1:** no-fault capacity rates sorted before all bound calculations; all permutations of
  the two probe inputs produced consistent non-bracketed or lower-bound results, with visible
  monotonicity conflict and no unsupported upper bound.
- **M1F-R2 / residual:** every container is validated against its enclosing service/project and
  every service against the requested project, including empty projects. Target selection remains
  restricted to the uniquely matched allowlisted service. All declared replicas must form a unique
  contiguous `[0,N)` set. Nested-service spoofing, empty-project bypass, and incomplete/incorrect
  replica sets are refused. A consistent unrelated service remains accepted.
- **M1F-R3:** every supplied event-stage timestamp is validated against the declared window.
  Out-of-window stop/unavailability/LB/restart/rejoin stages are invalid and produce no recovery
  or transient claim; each has a test.
- **M1F-R4:** `grace_ms` is a finite non-negative duration; zero is valid. Stop observation is
  reported only from explicit stop-completion evidence; request-only events do not fabricate stop
  observations or durations.
- **M1F-R5:** corrected packets include actual patch, pre-fix/final hashes, manifest and verified
  round-trip. The reviewer independently verified 3/3 patch round-trips; manifests and hashes
  match the working tree.

## Integrity

- Protocol SHA-256 remains `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`.
- D44 profile SHA-256 remains `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`.
- Retained results excluding `results/m1f-*` reproduce exactly: 2,136 files,
  SHA-256 `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`.
- Focused, scoped, race, vet, build, format and diff commands reproduced exit 0 with no claim that
  unrestricted `go test ./...` passes over retained historical snapshots.
- CLI stays file-only with a new output directory and reports
  `Campaign capacity: **NOT CLAIMED**` unless explicitly given an evidence file.
- Normal analysis does not attach optional fault/capacity fields by default.

## Limits

Manual event documents are operator attestations; the reducers do not observe them independently.
No live inventory was collected. Target allowlists are engineering policy, not proof that ingress
topologies are identical across platforms. Capacity bounds are evidence-scoped and remain
descriptive, not an approved campaign C_SLO.

Next approved scope is a versioned, read-only instrumentation/deployment foundation (M2). It must
exclude lifecycle execution, load and any live campaign action.
