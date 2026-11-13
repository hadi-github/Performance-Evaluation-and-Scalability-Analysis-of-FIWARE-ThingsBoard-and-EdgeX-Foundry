# M1e review — 2026-11-04

**CHANGES_REQUIRED.** M1e is not approved. No live, deployment or runtime qualification
is affected.

Reviewer session: `ses_e5bf776acffewIermuL5GrjYPw`; requested
`opencode-go/deepseek-v4-pro#high`. Harness reports DeepSeek V4 Pro; effort control unexposed.
Packet: `results/m1e-adapter-payload-20261104-035029+0330/`; baseline:
`results/m1e-baseline-20261104-031903+0330/`; summary
[`m1e-adapter-payload-20261104.md`](m1e-adapter-payload-20261104.md).

## Findings

### F1 — HIGH — boundary outcomes corrupt legacy ledger consumers

`workloads/k6/gen.js` records `incomplete_response` / `oversized_response` verbatim in
`records.ndjson`. Summary counters are correct, but:

- `internal/analysis/analyze.go:508–510` accepts only `ack_success`, `explicit_failure`,
  `unknown`; one boundary record makes offered-load analysis INVALID.
- `internal/reconcile/reconcile.go:105–121` has no default branch, so such records are silently
  dropped and the planned identity equation can still appear clean.
- M1e tests covered summary evaluation only, not these ledger consumers.

Required fix: keep distinct summary subtype counters, but write the ledger outcome as
`explicit_failure` and retain the subtype in `detail`; alternatively version and normalize
reader semantics explicitly. Acceptance: boundary ledger records pass validation, reconcile with
`planned = ack_success + explicit_failure + unknown`, are not clean unless stored, keep subtype
diagnostics, and leave legacy ledgers unchanged. Add direct tests for both consumers.

### F2 — LOW — plan payload bound looser than encoder

`internal/plan/plan.go:214` allows offsets up to 1 MiB, but encoding requires
baseline + offset ≤ 1 MiB. The top band passes resolution then fails at first encode. Fail loudly
at plan validation using actual canonical bounds/helper; test maximum valid, maximum+1 and nearby
boundaries.

## Verified clean

Canonical encoder is single-source; adapter projection preserves identity, numeric and timestamp
bytes with logical payload sizes 200/1224/4296, while FIWARE wire expansion is reported separately.
Legacy bodies remain byte-identical when payload mode is absent. Stored-field export reads actual
platform fields with identifier quoting and stop-on-error; legacy exports are unchanged. Strict
plan decoding rejects malformed/odd/oversized settings. Sink and receipt code were not modified.

Protocol and D44 hashes are unchanged; retained results (2,001 files excluding M1e packets) hash
identically before/after; patch round-trips; all claimed commands reproduced, including race,
scoped `-short`, Node/Python tests, vet and diff. Docker integration skipped by `-short`. No false
all-suite claim. The earlier 1,966 count predates retained M1c/M1d packets.

Fix F1/F2, then resubmit for independent review.
