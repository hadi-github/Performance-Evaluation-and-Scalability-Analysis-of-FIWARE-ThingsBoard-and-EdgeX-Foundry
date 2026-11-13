# M1e approval — 2026-11-04

**APPROVED for scoped M1e offline adapter payload controls and boundary semantics.**
No deployment, live benchmark, runtime payload/storage qualification, clock qualification or
campaign freeze is granted.

Reviewer sessions: initial `ses_e5bf776acffewIermuL5GrjYPw`, re-review
`ses_e5bda0110ffezYcVouts5ruYrK`, final F2r `ses_e5bc73feaffes8ZYvVRg1HmbVl`. All requested
`opencode-go/deepseek-v4-pro#high`; harness identified DeepSeek V4 Pro, effort control unexposed.

Packets: `results/m1e-adapter-payload-20261104-035029+0330/`,
`results/m1e-fix-20261104-044749+0330/`, `results/m1e-f2r-20261104-050724+0330/`.
Findings: [`m1e-independent-review-20261104.md`](m1e-independent-review-20261104.md).
Summary: [`m1e-adapter-payload-20261104.md`](m1e-adapter-payload-20261104.md).

## Final disposition

- **F1 (ledger boundary regression) — APPROVED.** Boundary outcomes are recorded as legacy
  `explicit_failure` with a stable subtype prefix in `detail`; summary subtype counters are
  distinct and not double-counted. Reconciliation now errors on unknown categories, preserves
  the planned identity equation, marks unstored boundary failures unclean and retains stored
  ones. Tests exercise both ledger consumers and end-to-end analysis.
- **F2r (payload worst-case bound) — CLOSED.** The canonical baseline bound uses generator maxima
  temperature 45, humidity 80, pressure 1150. Minimum baseline plus maximum offset equals exactly
  1 MiB; maximum baseline plus maximum offset stays within it; the accepted maximum encodes at
  pressure 1150; max+1/odd inputs fail. Timestamp worst-case coverage was extended to 18-byte
  canonical tokens. No other identity/timestamp under-bound was found.
- Shared numeric constants are centralized in `internal/hash`; generator values remain
  bit-identical and cross-language literals agree.

## Integrity

Protocol hash remains `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`;
D44 profile remains `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`.
Retained results excluding M1e packets were byte-identical before/after: 2,001 files,
`50e49df6c79cfc714e1a3495fac7ff2ecb94a0548ab22b05ef9020c75856a3a5`.
Incremental patches round-trip. All recorded focused Go/Node/Python, race, scoped `-short`, vet,
gofmt and diff commands reproduced exit 0. Docker integration was skipped by `-short`; no claim
that unrestricted `go test ./...` passes (retained-results source recursion remains).

## Preserved limits

Canonical adapter mappings and actual stored-padding exporters are offline-tested only. No sink
deployment, no platform ingestion, no live k6 load, no SUT access and no runtime receipt, storage
or clock evidence was produced. Pilot payload points remain engineering inputs, not a frozen
scientific grid.
