# M1b approval — 2026-11-04

**APPROVED for scoped M1b offline pipeline.** Campaign freeze, live qualification and runtime
capture/clock qualification remain unapproved.

Reviewer session: `ses_e5c831c28ffeZ2P8S8xUfbkZkA`; requested route
`opencode-go/deepseek-v4-pro#high`. Harness identified DeepSeek V4 Pro; separate effort control
unexposed. Implementation packet: `results/m1b-pipeline-20261104-013816+0330/`, baseline
`results/m1b-baseline-20261104-013210+0330/`, scoped summary
[`m1b-offline-pipeline-20261104.md`](m1b-offline-pipeline-20261104.md).

This is a transcription of the independent review, not a live experiment.

## Verified

- Scoped implementation delta: modified `internal/runner/runner.go`, new
  `internal/runner/offline_test.go`, new `tests/offline/pipeline_test.go`, new packet document.
  Earlier M0b/M1a code was not re-reviewed here.
- Changed-file SHA-256 values matched working tree, packet `final-files` and manifest.
  Incremental patch hash matched and applied cleanly to M1a final files, reproducing working files.
- Protocol hash `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` and D44 hash
  `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` unchanged.
- `results-before.sha256` and `results-after.sha256` were byte-identical across 1,921 entries;
  `docs/status.md` was absent from the implementation delta; 32-line dirty worktree unchanged.
- Focused, scoped `-short`, diff and vet commands were independently reproduced with exit 0.
- No hidden skips in changed tests; Docker integration remains gated by `-short`; no claim that
  unrestricted `go test ./...` passes. Retained-results source recursion remains the independent
  cause of that failure and must not be "fixed" by deleting evidence.

## Behavior reviewed

`prepareRun`/`newManifest` extraction preserves strict decoding, resolution, protocol
verification/eligibility order and fail-closed behavior before writes; campaign rejection still
precedes directory creation. The offline fixture calls shared preparation/manifest/Finish/Analyze
paths and never calls Docker/k6/host sampling. Synthetic fixtures are labeled and cannot become
`VALID`; comparability remains `NOT_ASSESSED`.

Required invariants passed: 5 scheduled, 3 observed, 2 missing, p50 100 ms, p99 600 ms,
500-ms attainment 2/5; duplicate/ERROR/UNKNOWN and capture strictness; mixed legacy/protocol
comparison separation; deterministic report bytes; output boundary; altered/missing metadata and
campaign rejection.

Nonblocking observations: a write failure returns nil instead of a partial directory, and a
partial run directory is not rolled back. Neither can overwrite evidence/input because of fresh
run IDs, and both match prior behavior on failure paths.

Next approved scope: offline payload-producer golden fixture (M1c). No live adapter/sink wiring.
