# M2b blueprint and runtime artifact packaging packet — 2026-11-05

**Ready for independent review. Offline documentation and pure schema/render code only. Actual
deployment remains NOT executed. No Docker client call, SUT access, deployment, lifecycle action,
load, campaign action or live observation is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
exposed provider model as `opencode-go/deepseek-v4.1-flash` (DeepSeek V4.1 Flash) at the requested
`#high` route intent. A separate effort control was not independently observable. Baseline HEAD:
`36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on `chore/ci-and-release`. No commit or push was made.
`docs/status.md` was not edited by this cycle.

Evidence directory: `results/m2b-blueprint-20261105-033125+0330/`.
Baseline directory: `results/m2b-baseline-20261105-032542+0330/`.

## 1. Scope

M2b extends the approved M2 deployment/instrumentation contract. It is not a new project, framework
or stack. It defines:

- a versioned orchestration blueprint with an explicit safe/blocked action taxonomy;
- a per-platform read-only state machine declaring the existing Compose project, services,
  replicas, health checks, allowed evidence, required attestation and missing-evidence semantics;
- a runtime artifact packaging specification with path templates, manifest schema, hashing rules
  and preservation rules;
- pure schema/decode/validate/canonical-render additions in `internal/deployment`.

No stage may call a lifecycle action. The `apply` stage and any rollback/restore action are
reference-only and marked `EXECUTION_BLOCKED_HERE`. Actual deployment is **not executed**.

The blueprint uses the repository's existing bounded-milestone framework. It is not an M3
fragility/fault campaign and makes no M3 readiness claim; any future M3/live milestone requires
separate review and explicit authorization.

## 2. Files added

| File | Purpose |
|---|---|
| `docs/development/deployment-orchestration-blueprint.md` | Scoped M2b orchestration blueprint; stages, taxonomy, per-platform state machines, integration, rollback rules, gate transition. |
| `docs/development/runtime-artifact-packaging.md` | Artifact taxonomy, run identity/path templates, manifest schema, hashing and retention rules, prohibited actions. |
| `internal/deployment/orchestration.go` | Strict schema v1 `OrchestrationPlan`, safe/blocked action predicates, validation, canonical JSON/hash and deterministic Markdown render. |
| `internal/deployment/orchestration_test.go` | Valid plan, stage order, unsafe taxonomy, apply marker, platform/project substitution, missing-evidence state, strict-key and determinism tests. |
| `internal/deployment/packaging.go` | Strict schema v1 `ArtifactManifest`, path/identity/role validation, canonical JSON/hash, read-only `Verify` and deterministic Markdown render. |
| `internal/deployment/packaging_test.go` | Path reuse/traversal/absolute rejection, prior-vs-new placement, self-reference rejection, verify PASS/FAIL/UNAVAILABLE, strict-key and order-invariance tests. |
| `docs/archive/sessions/m2b-blueprint-20261105.md` | This packet. |

No existing source file was modified. The additions are new files only; the approved M2 descriptor,
inventory, observer, evidence-manifest, validation, render and CLI code are unchanged. The existing
`benchctl deployment-validate` command was not changed and no new CLI execution path was added.

## 3. Machine-readable contract summary

`OrchestrationPlan` (schema version 1, interpretation `deployment-orchestration-plan-v1`) requires:

- `mode: offline-blueprint-only`;
- `authorization: separate-review-and-authorization-required`;
- `execution_blocked: true`;
- exactly the seven versioned stages in order:
  `pre_deploy_inventory`, `provision_on_sut`, `dry_run_render`, `apply`, `validate_health`,
  `evidence`, `rollback`;
- `permissible_actions` limited to `Read`, `Validate`, `RenderDryRun`, `Attest`;
- blocked lifecycle actions only in `blocked_actions`, with `execution_blocked_here: true`;
- an `apply` stage that is always marked and references `Apply`;
- per-platform Compose project and service declarations matching the unchanged existing names;
- `missing_evidence_status` limited to `UNAVAILABLE` or `NOT_ASSESSED`.

`ArtifactManifest` (schema version 1, interpretation `runtime-artifact-manifest-v1`) requires:

- a run id of the form `{tag}-{YYYYMMDD}-{run identifier}`, a valid date and a bounded tag;
- platform `thingsboard`, `fiware`, `edgex` or `all`;
- roles from the explicit artifact taxonomy;
- relative, clean paths with no reuse or traversal;
- `this_run` paths under the new run directory or `docs/archive/sessions/`;
- `preserved_prior` paths outside the current run directory and under `results/` or `docs/`;
- SHA-256 plus positive byte count for every artifact;
- at least one required artifact;
- no manifest or `SHA256SUMS` self-listing.

Both decoders reuse the M2 exact-case-key, duplicate-key, explicit-null and trailing-JSON checks.
Both canonicalizers are order-invariant and add no timestamps or values the caller did not supply.
`ArtifactManifest.Verify` re-hashes read-only; missing is `UNAVAILABLE`, mismatch is `FAIL`.

## 4. Verification

All positive commands exited `0`. Logs and `.exit` files are in the evidence directory.

| # | Command | Exit |
|---:|---|---:|
| 01 | `go test -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| 02 | `go test -short -count=1 ./cmd/... ./internal/... ./tests/...` | 0 |
| 03 | `go test -race -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| 04 | `go vet ./internal/deployment ./cmd/benchctl` | 0 |
| 05 | `go mod verify` | 0 |
| 06 | `git diff --check` | 0 |
| 07 | `gofmt -l internal/deployment cmd/benchctl/deployment.go cmd/benchctl/deployment_cli_test.go cmd/benchctl/main.go` (empty output) | 0 |
| 08 | `go build -o /tmp/opencode/m2b-build/benchctl ./cmd/benchctl` | 0 |
| 09 | Targeted verbose orchestration/artifact tests | 0 |
| 10 | Protocol/profile SHA-256 unchanged | 0 |
| 11 | Retained results tree unchanged | 0 |
| 12 | New-file whitespace scan (empty output) | 0 |

The scoped command uses only the task-specified paths, not unrestricted `go test ./...`. Docker
integration is skipped by `-short`; no claim is made that unrestricted `go test ./...` passes over
retained historical snapshots under `results/`.

## 5. Integrity and preservation

| Item | Value |
|---|---|
| HEAD / branch | `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` / `chore/ci-and-release` |
| Source snapshot before edits | 375 files, `b3b9258eaadb7e012dc177e279c6f449fa11b680621c9a3c0754249af73fb01b` |
| Source snapshot after edits | 382 files, see `source-final.sha256` and `summary.txt` |
| Retained `results/` excluding `results/m1f-*`, `results/m2-*`, `results/m2b-*` | `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`, 2,136 files, identical before and after |
| Protocol definition SHA-256 | `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` |
| D44 profile SHA-256 | `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` |
| `docs/status.md` before / after | `0887605cd5db511ed4e2a19496be03db5454fd83c0b91fc6f39b1c10e086891a` / unchanged |
| `incremental.patch` SHA-256 | `fc67c1f7bf2ec1d7788d544e70438a308c11bce6dc4911c999f51ee2b2c09a8f` (87,147 bytes) |
| Patch round-trip | exit `0`, **6/6 files** match final SHA-256 |

The incremental patch is generated from `/dev/null` for the six new files and applies to a scratch
tree with `patch -p1 --fuzz=0`. Every resulting file hash matches the final working tree. No prior
M1f/M2 artifact is included in the new patch.

### Prior packet verification

A read-only recursive verifier re-hashed every `path`/`sha256` pair in the prior evidence manifests:

- `results/m1f-fault-fix-20261105-021405+0330/evidence-manifest.json`
- `results/m2-deployment-foundation-20261105-024722+0330/evidence-manifest.json`
- `results/m2-fix-20261105-030848+0330/evidence-manifest.json`

Result: **61 artifacts checked, 0 problems**, exit `0`
(`preserved-prior-packets-verify.log`). The prior packets and results were not edited.

## 6. Baseline and pre-edit boundary

The baseline was captured before any edit:

- `source-before.sha256`: 375 files, `b3b9258e…fb01b`;
- `results-before-retained.sha256`: 2,136 files, `8e250066…25ec`;
- `status-before.txt`: `0887605c…891a`;
- `tracked-before.patch`: existing uncommitted prior work, unchanged.

The six new files had no pre-edit content (`pre-edit-files.sha256` is empty); the patch boundary is
therefore exactly "no file" to final file. `final-files.sha256` records the six final hashes.

## 7. Non-goals and lifecycle exclusion

- No Docker client call, SUT/platform/sink/adapter access, deployment, provision, apply/up/down,
  scale, restart, stop, fault, load, reset, delete, container exec, log action or campaign action.
- No live observer implementation and no live validation claim; M2's `live_validation` remains
  `NOT_ASSESSED`.
- No new stack topology, service, network, volume, receiver, ledger or workload adapter.
- No modification of `deploy/*/compose.yaml`, `.env`, Makefile, monitoring/history scripts,
  `scripts/adapter-receiver.py`, `workloads/k6/lib/*.js`, `internal/receipt`, `internal/sink` or the
  approved M2 schema/CLI.
- No protocol/profile hash change and no fabricated hash, timestamp, replica count, health state
  or artifact byte count.
- No commit or push; no edit to `docs/status.md`.

**Stop for independent review. Actual deployment remains NOT executed; any execution requires a
separate explicitly authorized milestone.**
