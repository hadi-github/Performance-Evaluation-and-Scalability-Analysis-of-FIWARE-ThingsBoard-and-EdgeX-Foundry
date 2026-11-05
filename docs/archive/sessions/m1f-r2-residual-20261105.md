# M1f R2 residual correction — 2026-11-05

**Ready for independent re-review. Offline code and tests only; no live/Docker, SUT, deployment,
fault execution, stop command, load or campaign qualification is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`). A separate effort control is not
exposed by the harness; the requested `#high` intent is recorded but effort is not independently
observable. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. No commit or push was made, and `docs/status.md` was not edited by this
cycle. (`docs/status.md` already carried an uncommitted modification in the preserved working tree
before this cycle; it was left untouched.)

This is a **minimal append-only residual correction packet** for the M1f re-review finding
`M1F-R2-residual` (MEDIUM). Evidence directory:
`results/m1f-r2-residual-20261105-022522+0330/`.

## Preservation

The prior review `docs/archive/sessions/m1f-independent-review-20261105.md`, the fix packet
`docs/archive/sessions/m1f-fault-fix-20261105.md`, and their evidence directory
`results/m1f-fault-fix-20261105-021405+0330/` are preserved exactly and were not overwritten.
No prior packet content was edited or re-hashed here.

## Finding

`M1F-R2-residual` (MEDIUM): `internal/analysis/fault_target.go` guarded project validation with
`svc.Project != ""` at the enclosing-service check and the enclosing-service/container check
(original lines 156 and 165). An otherwise valid `fiware`/`iot-agent` inventory plus an unrelated
`postgres` service whose enclosing `project` was empty and whose container declared
`project="bench"` therefore skipped both checks and returned `Eligible=true` with an emitted
target. The reviewer reproduced exactly that probe.

## Correction

`internal/analysis/fault_target.go` now treats project identity as an assertion, never an optional
hint:

- Every inventory service, including a service unrelated to the requested target, must declare the
  requested project. The check no longer short-circuits on an empty enclosing project.
- Every container must match its enclosing service project exactly, including when both are empty.
- The requested target's container is still required to match the requested project through the
  existing target-level check.
- Target selection still comes only from the uniquely matched allowlisted service; the new checks
  add validation and do not widen the allowlist.

Explicit rationale: an empty enclosing project is not "unknown" and must not waive consistency.
Without the correction, a service with `project=""` could enclose a container declaring
`project="bench"` (or another empty project) while the valid allowlisted service still resolves, so
the malformed row would pass silently. Refusing both shapes closes that path. A genuinely unrelated
service that declares the requested project and whose containers agree with their enclosing service
is still accepted, so this does not overreject consistent inventories.

`docs/development/offline-analysis.md` gained one targeted paragraph in the existing fault-target
section recording this semantics; the rest of the document was not rewritten.

## Regression tests

Added to `internal/analysis/fault_capacity_test.go`:

- `TestFaultTargetEmptyEnclosingProjectRefused` — a valid allowlisted `iot-agent` inventory plus an
  unrelated `postgres` service with empty enclosing project, in two variants:
  - container project empty (matching-empty): refused, `Eligible=false`, `Target=nil`, a
    project-refusal reason is recorded, and the serialized result contains no `target` field.
  - container project `bench` (different): refused with both the empty-project service refusal and
    the container/enclosing-project mismatch, `Eligible=false`, `Target=nil`, no serialized target.
- `TestFaultTargetConsistentUnrelatedServiceAllowed` — an unrelated `postgres` service declaring
  `project="bench"` with a container also declaring `project="bench"` resolves the valid
  `iot-agent` target, guarding against overrejection.

Retained unchanged and re-run: the three allowlisted-platform success cases,
`TestFaultTargetRefusals` (including cross-project, stateful, HAProxy and monitoring refusal),
`TestFaultTargetNestedServiceSpoofRefused`, and
`TestFaultTargetReplicaSetMustBeCompleteAndContiguous` (duplicate/missing/negative/`>=N`).

The pre-fix code was also run against the new negative test in a scratch copy to demonstrate the
regression is real: both variants returned `Eligible=true` with a target under the old guards. The
scratch run is recorded as an expected-failure artifact, separate from the passing command set.

## Evidence artifacts

`results/m1f-r2-residual-20261105-022522+0330/` contains:

- `incremental.patch` — unified diff of the three changed files from the pre-fix copies.
- `pre-fix-files.sha256` — SHA-256 of the pre-fix copies captured before editing.
- `final-files.sha256` — SHA-256 of the final working-tree files.
- `evidence-manifest.json` — hashes, command exits, regression-demonstration result, integrity
  values and the preserved-packet note.
- `patch-roundtrip.log`, `patch-roundtrip.exit`, `patch-roundtrip-verify.log`,
  `patch-roundtrip-verify.exit` and `patch-roundtrip-verified-count.txt` — scratch round-trip.
- `command-01-*` through `command-10-*` — focused, scoped, race, vet, diff-check, gofmt, build,
  protocol-hash, results-tree and targeted-verbose logs with exit codes.
- `pre-fix-regression-demonstration.log` / `.exit` — expected failure (`exit 1`) of the new
  negative test against the pre-fix resolver.
- `results-tree-excluding-m1f.sha256` — retained-results integrity manifest.

Patch generation used the pre-fix copies. Scratch verification copied them into
`/tmp/opencode/m1f-r2-roundtrip`, applied `patch -p1 --fuzz=0`, and compared every resulting
SHA-256 to the final file: exit `0`, **3/3 files verified**.

## Validation

All recorded positive commands exited `0`:

```sh
go test -count=1 ./internal/analysis ./cmd/benchctl
go test -short -count=1 ./cmd/... ./internal/... ./tests/...
go test -race -count=1 ./internal/analysis
go vet ./internal/analysis ./cmd/benchctl
git diff --check
gofmt -l internal/analysis/fault_target.go internal/analysis/fault_capacity_test.go   # empty
go build -o /tmp/opencode/m1f-r2-build/benchctl ./cmd/benchctl
sha256sum research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json \
  research/protocols/offline-profiles-20261101.json
go test -count=1 -v -run 'TestFaultTarget...' ./internal/analysis
LC_ALL=C find results -type f ! -path 'results/m1f-*' ...   # retained-results tree
```

The scoped command uses the task-specified paths, not unrestricted `go test ./...`; retained source
snapshots under `results/` are known to recurse and Docker-backed integration is skipped by
`-short`. No claim is made that unrestricted `go test ./...` passes. The three changed files are
untracked in the preserved working tree, so `git diff --check` (tracked changes only) is not the
formatting authority for them; `gofmt -l` returned empty output for the Go files.

## Integrity

| Item | Value |
|---|---|
| HEAD / branch | `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` / `chore/ci-and-release` |
| `fault_target.go` before → after | `fc354bafad8d42b513775163b33df18381fd93c949e2746a9783aec4a8127ed6` → `2fedf623a751bf54f820e62623be4bfa4c5dceb9af7e6e9888f1165417a5f5d3` |
| `fault_capacity_test.go` before → after | `51700607edc99965b8654eaf7c2670f7edc222d6815a55b494db28d3180e6510` → `9637a58db7057816e9268fb7b8be8fd3a698efc4f164d5f3574a6879b27028f8` |
| `offline-analysis.md` before → after | `f106cde7a29f4613a76613d053b590f3acf1355c4b8ba7f50d8570cf8ce124a4` → `b7a0a7bb5760323dcd93a73a1fc5d0e6b72fc4fe51786726a62240067b273561` |
| `incremental.patch` SHA-256 | `1aff50c68a165860c5e91e48ef45a66a98208f027fc7816580a7cf066bbe91b0` |
| Patch round-trip | exit `0`, 3/3 files match final SHA-256 |
| Protocol definition SHA-256 | `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` |
| D44 profile SHA-256 | `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` |
| Retained `results/` excluding `results/m1f-*` | `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`, 2,136 files |

The retained-results manifest is unchanged because the new packet directory matches the excluded
`results/m1f-*` prefix.

## Preserved limits

No live Docker, SUT, platform, sink, adapter, deployment, stop command, fault execution, recovery
observation, runtime capture, clock qualification, storage proof or load was exercised. The target
allowlist remains an engineering policy, not evidence of cross-platform ingress equivalence. Manual
event and window documents remain operator attestations. Capacity bounds remain descriptive
observations under an explicitly supplied predicate/search policy, not a campaign C_SLO.

**Stop for independent re-review.**
