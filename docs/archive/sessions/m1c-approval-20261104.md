# M1c approval — 2026-11-04

**APPROVED for scoped M1c offline payload-producer golden fixture.** No campaign freeze,
live payload-storage qualification or clock qualification is granted.

Reviewer session: `ses_e5c73aa2fffeCZJFCdsIesIv8s`; requested route
`opencode-go/deepseek-v4-pro#high`. Harness identified DeepSeek V4 Pro; effort control unexposed.
Packet: `results/m1c-payload-fixture-20261104-020102+0330/`; baseline:
`results/m1c-baseline-20261104-015538+0330/`; scoped summary
[`m1c-payload-fixture-20261104.md`](m1c-payload-fixture-20261104.md).

Transcription of independent review; not a live experiment.

## Verified

- HEAD `36f8348…` and branch unchanged; changed-file hashes matched packet manifest, `final-files`
  and incremental patch. Five-file delta contained only four test/fixture files and the packet.
- Protocol SHA-256 `7176b1fa…d0d24ee` and D44 profile `1b6aa76d…ed1f8` unchanged. No encoder,
  contract, metric schema or protocol-definition edits.
- 1,939-entry results hash inventory identical before and after (`340c9772…713759`);
  `docs/status.md` absent from implementation delta.
- Reproduced exit 0 for focused protocol/offline tests, 39-vector Node golden run, scoped
  `./cmd/... ./internal/... ./tests/... -short`, diff check, vet and gofmt. Node v26.11.1 present.
  No unrestricted `go test ./...` claim; Docker integration was skipped by `-short`.

## Technical verification

- Tests import the actual `workloads/k6/lib/payload.js` and `hash.js` and use real Go
  `CanonicalBytes`/`BuildPayload`/`BuildPayloadOffset`/`ValidateStored`; no duplicate encoder.
- Reviewer hand-computed a vector and independently reproduced it in Python, giving three-way
  Go/JS/Python byte parity. Timestamp identity preserved for maximum safe value and a millisecond
  fractional value; invalid/out-of-range inputs reject.
- 5 cohort messages plus 8 edge cases at 0/1024/4096-byte offsets yield 39 vectors; too-small,
  odd, oversized, null, duplicate, omitted, truncated and altered content fail closed.
- M1b integration uses the real offline preparation/finish/audit/analyze path with explicit
  provenance and synthetic labels. Corrupt payload export yields a separately named payload
  verification result, does not alter the 5/3/2 cohort or p99, and makes no storage claim.

Nonblocking: the synthetic export helper tolerates a missing trailing newline and empty lines;
not production behavior. An unused test variable appears only in a failure message.

Next approved offline scope: complete-body receiver receipt contract (M1d), implementation and
offline tests only. No sink deployment or live qualification follows from this approval.
