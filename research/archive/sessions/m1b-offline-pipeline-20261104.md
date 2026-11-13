# M1b offline end-to-end pipeline — 2026-11-04

**Ready for independent DeepSeek Pro review. Offline engineering fixtures only; no live evidence,
campaign qualification, runtime capture qualification or clock qualification is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies
the model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`); it does not expose a
separate effort control. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. Pre-edit baseline:
`results/m1b-baseline-20261104-013210+0330/`. Review packet:
`results/m1b-pipeline-20261104-013816+0330/`. No commit or push was made.

This packet supplements the preserved M0b/M1a packets and does not replace them. It does not
modify retained evidence, `docs/status.md`, the protocol definition, or the D44 profile policy.

## What changed

`internal/runner/runner.go` now factors the non-Docker preparation path out of `Run`:

- `prepareRun` performs strict plan decode, `plan.Resolve`, seq-overflow check, protocol
  definition/profile verification, binding eligibility, run-directory creation, and writes
  `plan.json` and `resolved.json`.
- `newManifest` constructs the shared runner manifest metadata from that prepared state.
- `PrepareOfflineFixture` exposes a bounded offline path that runs the same preparation and
  manifest construction but never calls Docker/k6 or any service. It writes only
  `plan.json`, `resolved.json`, and `manifest.json`, sets `synthetic=true`,
  `offline_engineering_fixture=true`, k6 `mode=offline-engineering-fixture`/`executed=false`,
  and retains `runtime_metric_interpretation=legacy-acknowledgement-only-v1`.
- `Run` now uses the same `prepareRun`/`newManifest` code; live behavior order is preserved.
- `Finish` labels a health-valid offline fixture `SYNTHETIC_FIXTURE`, never `VALID`, so fixture
  health cannot be mistaken for a live run verdict.

New focused tests:

- `internal/runner/offline_test.go`: real plan/resolve/manifest hash flow, protocol
  binding/pilot/runtime metadata consistency, altered/missing metadata rejection, campaign
  fail-closed before a run directory is written, and run-directory reuse rejection.
- `tests/offline/pipeline_test.go`: plan -> resolve -> runner manifest -> `runner.Finish` ->
  `analysis.Analyze` protocol-bound pipeline using an explicitly synthetic receipt-capture
  fixture. It asserts the preserved main cohort and prior adversarial semantics, Markdown/JSON/CSV
  consistency, deterministic report equality, input/output path boundary, campaign fail-closed,
  plan/resolved/manifest hash and metadata mismatches, capture strictness (invalid endpoint,
  incomplete body/capture, wrong binding, `NOT_SENT`, cutoff mismatch, missing capture, missing
  receipts), and mixed legacy+protocol comparison separation.

No production adapter, sink, payload, clock, or live capture wiring was added.

## Preserved main fixture

The pipeline fixture is still 5 scheduled measurement identities, 3 observed unique first
receipts by the inclusive cutoff, 2 missing, p50 100 ms, p95/p99 600 ms, 500-ms attainment
2/5 (`0.4`), one duplicate extra receipt, one ERROR first receipt and one UNKNOWN first receipt.
Warm-up receipts remain excluded from the measurement cohort. Existing M1a adversarial tests
remain unchanged and pass, including capture strictness, intent fail-closed behavior, tie/permutation
semantics, zero-observation null percentiles, late/cutoff handling, and mixed-interpretation
provenance.

## Validation

Commands and exits are recorded in the review packet:

| Command | Exit |
|---|---|
| `go test ./internal/plan ./internal/runner ./internal/analysis ./tests/offline -count=1` | 0 |
| `go test ./cmd/... ./internal/... ./tests/... -short -count=1` | 0 |
| `git diff --check` | 0 |
| `go vet ./internal/runner ./internal/analysis ./tests/offline ./cmd/benchctl` | 0 |

`-short` skips the Docker-backed integration execution. The known archive-recursion failure of
unrestricted `go test ./...` under retained `results/` source snapshots was not invoked and those
snapshots were not edited.

## Preserved hashes and limits

- Protocol definition SHA-256:
  `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`
- D44 profile SHA-256:
  `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`
- Retained results tree before M1b:
  `c8a5aa3e31a3f7727280c3d845ff5e0503d42dbbae861bb2f77776c366bd4f99`
  (1,921 files, recorded in the baseline manifest).

The offline fixture's git metadata is real and the working tree is dirty; provenance/comparability
therefore remains `NOT_ASSESSED`. That is intentional: passing protocol cohort arithmetic in a
synthetic fixture is not a provenance or campaign claim. Runtime receipt capture, monotonic clock
qualification, live generator work, Docker integration and production payload/sink wiring remain
out of scope.
