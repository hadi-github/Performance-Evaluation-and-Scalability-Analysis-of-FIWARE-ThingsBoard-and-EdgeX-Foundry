# M1a approval — 2026-11-04

**APPROVED for scoped M1a offline analysis.** Campaign freeze, runtime receipt qualification,
clock qualification and live platform work remain unapproved.

Reviewer session: `ses_e5ca28f17ffezpYAWTi38I5QWO`. Requested route
`opencode-go/deepseek-v4-pro#high`; harness identified DeepSeek V4 Pro. No separate effort
control was exposed to the reviewer. Implementation fix packet:
`results/m1a-fix-packet-20261104-005644+0330/`; earlier findings:
[`m1a-independent-review-20261104.md`](m1a-independent-review-20261104.md).

This record transcribes the independent review; it is not a new experiment or a live run.

## Verified

- Six scoped source SHA-256 values matched the working tree and packet `final-files`;
  `incremental.patch` hash matched the packet manifest.
- Protocol definition hash `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`
  and D44 profile hash `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`
  were recomputed and unchanged.
- Packet command exit statuses reproduced: focused unit tests, scoped
  `./cmd/... ./internal/... ./tests/... -short -count=1`, and `git diff --check`.
  `docs/status.md` was not edited by the fix packet.
- Reviewer probes in `/tmp/opencode/m1a-probe/`; repository and retained evidence remained
  read-only. No live operations.

## Finding dispositions

| ID | Result | Reviewer evidence |
|---|---|---|
| M1A-R1 | RESOLVED | Raw capture validation runs before typed decode; verbatim strict fields, recursive duplicate-key rejection, null/missing identity rejection, explicit receipt array. Added probes for omitted seq/observed_ms, case aliases and nested duplicate outcomes. Invalid capture does not fabricate observations, missing counts or `NOT_SENT`. |
| M1A-R2 | RESOLVED | Protocol intent is checked before legacy selection and early returns; partial/contradictory/unsupported markers fail closed; genuine legacy fixtures still pass unchanged. |
| M1A-R3 | RESOLVED | Nil pilot guard precedes dereference; binding-only, pilot-only and null-pilot combinations return validation errors without panic. |
| M1A-R4 | RESOLVED | Global minimum receipt timestamp is selected before tie checks; all six permutations of the three conflicting receipts produced identical claims/counts/percentiles; only the earliest tie can invalidate; one latency sample per identity. |
| M1A-R5 | RESOLVED | Markdown/JSON carry interpretation and verified protocol/profile hashes; CSV carries validity/reasons; comparison keys separate interpretation/binding; mixed protocol/legacy produces separate groups; protocol rows require `protocol_delivery=PASS` for eligibility. |

## Preserved limits

`go test ./...` still recurses into partial Go source snapshots retained under `results/`, which
fails compilation independently of current source. That retained evidence must not be deleted or
edited to silence it. Current source scopes `./cmd/... ./internal/... ./tests/...` pass. Docker
integration execution was skipped with `-short`.

M1b may exercise the non-Docker plan/resolve/manifest/analyze path offline. No live generator,
adapter/sink payload wiring, monotonic receipt capture or clock qualification follows from M1a.
