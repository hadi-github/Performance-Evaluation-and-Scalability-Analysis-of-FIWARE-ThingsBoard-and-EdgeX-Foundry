# M1f corrective fix packet — 2026-11-05

**Ready for independent re-review. Offline code and tests only; no live/Docker, SUT, deployment,
fault execution, stop command, load or campaign qualification is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`) and does not expose a separate
effort control; the requested `#high` intent is recorded but effort is not independently
observable. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. No commit or push was made.

This is a **supplemental corrective packet** for the M1f CHANGES_REQUIRED review
(`docs/archive/sessions/m1f-independent-review-20261105.md`). Evidence directory:
`results/m1f-fault-fix-20261105-021405+0330/`.

## Preservation of the original packet

The original implementation packet
`docs/archive/sessions/m1f-fault-capacity-20261104.md` and its evidence directory
`results/m1f-fault-capacity-20261105-013824+0330/` are preserved exactly and were not overwritten.
The original packet's section 5 claims that `incremental.patch`, `final-files.sha256` and
`evidence-manifest.json` exist in that packet. **Those artifacts do not exist there; that claim is
false and is not treated as valid for the fix.** This supplemental packet supplies real artifacts
and a scratch-verified patch round-trip, and its manifest records that prior claim explicitly.

`docs/status.md` was not edited. No commit or push occurred.

## Findings and corrections

### M1F-R1 — HIGH — capacity rate ordering and contradictions

`internal/analysis/capacity.go`

- No-fault rates (`capacityRates`) are sorted ascending **before** the contiguous passing-prefix and
  upper-bound searches, so status and bounds no longer depend on caller window order.
- A confirmed passing rate above a confirmed failing rate now sets an explicit monotonicity
  conflict. Any candidate upper bound is discarded, `bracketed` is impossible, the conflict remains
  in `contradictions`, and an independently established lower bound is preserved as
  `lower_bound_only`.
- Same-rate contradictory repetitions keep their previous semantics (visible, excluded from the
  prefix, do not by themselves create a pass-above-fail conflict).
- No capacity meaning, status, or lower bound was silently reinterpreted; no default hybrid or new
  search rule was added.

Hand-computable regression tests:

- `TestCapacityPermutationInvariantContradiction`: all 6 permutations of
  `[pass20, fail10, fail40]` yield `unresolved`, no lower/upper bound, and the visible contradiction
  “passing rate 20 lies above failing rate 10”.
- `TestCapacityPassAboveFailBlocksBracket`: all 24 permutations of
  `[pass10, fail20, pass30, fail40]` yield `lower_bound_only` with lower 10 and no upper bound; the
  pass-above-fail contradiction is visible.
- Existing passing bracket, all-pass lower-bound, invalid-upper and same-rate contradiction tests
  still pass unchanged.

### M1F-R2 — HIGH — inventory enclosure and complete replica set

`internal/analysis/fault_target.go`

- Every container row is validated against the service and project that enclose it. A row whose
  declared `compose_service` or project differs from its enclosing service is refused; a
  self-declared string is no longer a selection authority.
- Target containers are resolved only from the uniquely matched allowlisted service entry.
- The **complete** declared replica set of the matched service must be unique and contiguous over
  `[0,N)`. Duplicate, missing, negative and `>=N` indices are refused anywhere in that service, not
  merely for the requested replica.

Hand-computable regression tests:

- `TestFaultTargetNestedServiceSpoofRefused`: the declared `iot-agent` service misses replica 0
  while a `postgres` service contains a row claiming `compose_service=iot-agent`/replica 0. No
  target is emitted; both the spoof and the missing replica are refused.
- `TestFaultTargetReplicaSetMustBeCompleteAndContiguous`: duplicate, missing, negative and `>=N`
  replica cases are each refused with no target.
- The three allowlisted platform success tests and all existing refusal tests still pass.

### M1F-R3 — HIGH — every stage timestamp against the declared window

`internal/analysis/fault_event.go`

- `stop_completion_ms`, `observed_unavailability_start_ms`,
  `observed_unavailability_end_ms`, `lb_removal_ms`, `restart_request_ms` and `healthy_rejoin_ms`
  are each checked against `window_start_ms/window_end_ms`, in addition to `stop_request_ms`.
- A stage outside the declared window makes the event `INVALID` and produces no qualification and
  no transient/derived recovery observation.

`TestFaultEventEverySuppliedStageMustBeInsideWindow` covers before-start and after-end values for
each of those six stages individually (hand-computable: window 900–1100; values 890 or 1105), and
asserts `INVALID`, not qualified, no transient observation, and a stage-specific reason.

### M1F-R4 — MEDIUM — grace is a duration; stop request vs observation

`internal/analysis/fault_event.go`

- `grace_ms` is no longer treated as a timestamp. It is a finite duration `>=0`: zero is valid;
  negative or non-finite is `INVALID`. Forced-positive-grace consistency is unchanged.
- `FaultTransientObservation` now separates `stop_requested` from `stop_observed`.
  `stop_observed` is true only with explicit `stop_completion_ms`; `stop_duration_ms` is absent
  without that completion evidence. A request-only event cannot assert an observed stop or
  fabricate a duration. This is output-compatible with schema version 1 (no input field removed).

Hand-computable tests: `TestFaultEventGraceIsNonNegativeDuration` (zero qualifies; negative and NaN
invalid) and `TestFaultEventRequestOnlyDoesNotObserveStop` (missing completion is `UNAVAILABLE`,
`stop_requested=true`, `stop_observed=false`, no stop duration).

### M1F-R5 — MEDIUM — evidence integrity

The new packet `results/m1f-fault-fix-20261105-021405+0330/` contains the actual artifacts:

- `incremental.patch` — unified diff from the pre-fix copies to the final files for the five files
  changed by this fix.
- `pre-fix-files.sha256` — SHA-256 of those pre-fix copies.
- `final-files.sha256` — SHA-256 of the final working-tree files.
- `evidence-manifest.json` — hashes, command exits, patch round-trip result, integrity values and
  the preserved-prior-packet note.
- `patch-roundtrip.log`, `patch-roundtrip.exit`, `patch-roundtrip-verify.log` and
  `patch-roundtrip-verified-count.txt` — scratch round-trip evidence.

Patch generation used pre-fix copies captured before editing. The scratch verification copied those
pre-fix copies into `/tmp/opencode/m1f-fix-roundtrip`, applied `patch -p1 --fuzz=0`, and compared
every resulting SHA-256 to the final file. Exit code `0`, **5/5 files verified**.

## Files changed by the fix

| File | Purpose |
|---|---|
| `internal/analysis/capacity.go` | Sorted bound computation; pass-above-fail conflict blocks bracket/upper. |
| `internal/analysis/fault_target.go` | Enclosing service/project validation; uniquely matched service; complete contiguous replica set. |
| `internal/analysis/fault_event.go` | All-stage window containment; grace duration; separate stop request/observation. |
| `internal/analysis/fault_capacity_test.go` | Adversarial permutation, contradiction, spoof, replica-set, per-stage window, grace and request-only tests. |
| `docs/development/offline-analysis.md` | Documents the corrected offline semantics. |

No change was made to protocol definitions, the D44 profile policy, retained evidence, sink/receiver
contracts, `docs/status.md`, existing claims or any metric threshold. No Docker client, actual
container stop, scheduler, fault-execution loop, live SUT access, deployment or load was used.

## Validation

All recorded commands exited `0`:

```sh
go test -count=1 ./internal/analysis ./cmd/benchctl
go test -short -count=1 ./cmd/... ./internal/... ./tests/...
go test -race -count=1 ./internal/analysis
go vet ./internal/analysis ./cmd/benchctl
git diff --check
gofmt -l internal/analysis/capacity.go internal/analysis/fault_target.go \
  internal/analysis/fault_event.go internal/analysis/fault_capacity_test.go   # empty output
go build -o /tmp/opencode/m1f-fix-build/benchctl ./cmd/benchctl
sha256sum research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json \
  research/protocols/offline-profiles-20261101.json
go run ./cmd/benchctl fault-capacity -in <packet>/cli-evidence.json -out <packet>/cli-smoke
```

The scoped command uses the task-specified paths, not unrestricted `go test ./...`; retained source
snapshots under `results/` are known to recurse and Docker-backed integration is skipped by
`-short`. No claim is made that unrestricted `go test ./...` passes.

The CLI smoke run writes `fault-capacity.json`, `fault-capacity.md` and `SHA256SUMS`; its Markdown
states `Campaign capacity: **NOT CLAIMED**`, target eligible, event qualified, recovery observed
and capacity bracketed at 20/40 messages/s from the explicit synthetic windows.

## Integrity

| Item | Value |
|---|---|
| HEAD / branch | `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` / `chore/ci-and-release` |
| Protocol definition SHA-256 | `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` |
| D44 profile SHA-256 | `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` |
| Retained `results/` excluding `results/m1f-*` | `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`, 2,136 files |
| `incremental.patch` SHA-256 | `98229e813d591e5b6a5dbe3485a8ed6da56cc827fa6b80c492f80931528b0adf` |
| Patch round-trip | exit `0`, 5/5 files match final SHA-256 |

New artifacts exist only under the new `results/m1f-fault-fix-*` directory; the retained results
tree hash is unchanged.

## Preserved limits

No live Docker, SUT, platform, sink, adapter, deployment, stop command, fault execution, recovery
observation, runtime capture, clock qualification, storage proof or load was exercised. The target
allowlist remains an engineering policy, not evidence of cross-platform ingress equivalence. Manual
event and window documents are operator attestations; the reducers do not independently observe
them. Capacity bounds remain descriptive observations under an explicitly supplied
predicate/search policy and monotonicity assumption, not a campaign C_SLO. Run
provenance/comparability remains `NOT_ASSESSED` because the working tree contains pre-existing
M1a–M1e uncommitted work.

**Stop for independent re-review.**
