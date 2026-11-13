# M1a independent review — 2026-11-04

**CHANGES_REQUIRED.** M0b's scoped offline foundation approval remains intact. M1a approval,
runtime capture qualification and campaign freeze are not granted.

Reviewer session `ses_e5cc0b14bffe0bqtEaS0wmAZmX`; requested route
`openai/gpt-6-astra#high`; harness identifies Astra, with no separate effort control exposed.
Implementation packet `results/m1a-review-packet-20261103-110904+0330/`. Scratch-only probes
and output: `/tmp/opencode/m1a-astra-review-bezwgf/`.

## Findings

| ID | Severity | Evidence | Required correction and acceptance |
|---|---|---|---|
| M1A-R1 | HIGH | `internal/analysis/analyze.go:223–234`, `types.go:223–247`: missing/null `receipts` yielded PASS, observed=0, missing=5; duplicate/case-alias fields and omitted zero `seq` accepted | Strict exact raw capture validation before typed decode; reject duplicate keys, inappropriate nulls and missing identity fields. Require receipts array, with explicit `[]` valid. Invalid capture must not produce platform missing counts. |
| M1A-R2 | HIGH | `protocol_binding.go:29–48`: protocol intent in plan/orphan pilot params and unknown interpretation markers could fall back to legacy; `analyze.go:139–166` returned legacy before checking surviving metadata on early error | Discover protocol intent across all available inputs before legacy selection/early returns. Reject partial, contradictory and unsupported markers. Any remaining protocol marker blocks legacy fallback; true legacy artifacts remain supported. |
| M1A-R3 | MEDIUM | `internal/plan/plan.go:198–206`: binding plus absent/null pilot parameters records an error then dereferences nil | Guard nil pointers and return actionable validation errors. Test absent/null pilot, binding-only and pilot-only without panic. |
| M1A-R4 | MEDIUM | `protocol_cohort.go:130–137`: conflicting later-duplicate ties made first-receipt result input-order-dependent | Select global earliest receipt before tie checks; deterministically handle genuine earliest ties and later conflicting duplicates. Permutations of identical receipt multisets must yield equal claims/counts/percentiles. |
| M1A-R5 | MEDIUM | `output.go:25–65,218–246,318–337`: Markdown omits metric/protocol/profile provenance; CSV omits protocol-delivery validity; comparison key ignores interpretation/binding | Report interpretation and verified hashes; label legacy assessments; export validity/reasons; compare only semantically compatible interpretation/binding groups. Test Markdown/JSON/CSV consistency and mixed-interpretation comparisons. |

## Independently verified

- All 22 packet scoped-file hashes matched `final-files` and working tree; all 56 baseline
  snapshot hashes verified; tracked patch reconstructed plan/runner files.
- M0b definition/amendment and D44 profile bytes unchanged.
- Main fixture: 5 scheduled, 3 observed, missing 2, p99 600 ms, 500-ms attainment 2/5.
- Existing tests covered ERROR/UNKNOWN inclusion, warm-up exclusion, inclusive cutoff/drain,
  late exclusion, duplicates and null percentile for zero observations. Invalid identity,
  endpoint, body and clock cases suppressed protocol latency; missing was not `NOT_SENT`.
- Passed `go test ./cmd/... ./internal/... ./tests/... -short -count=1`, focused protocol cohort
  tests and `git diff --check`. `-short` skipped Docker-backed integration execution.
- `go test ./... -run '^$' -short -count=1` independently confirmed archive-recursion compile
  failures under retained `results/` snapshots. CI's unscoped command is not shown to fail in a
  clean checkout; do not alter CI scope without cause. The CI label “no servers contacted” is
  inaccurate for non-short Docker/k6 integration tests.

No repository edits or live operations by reviewer. Fix R1–R5, submit a new evidence packet and
request independent re-review. Keep changes offline and preserve all evidence snapshots.
