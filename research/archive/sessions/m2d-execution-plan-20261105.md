# M2d render-only execution-plan packet — 2026-11-05

**Ready for independent review. Offline render-only implementation and evidence only. No Docker client
call, SUT access, deployment, provisioning, apply/up/down, scale, restart, stop, fault, load, reset,
delete, observer implementation or live validation is claimed. No executor, lifecycle method or CLI
command that runs anything was added.**

Requested route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the exposed provider
model as `opencode-go/deepseek-v4.1-flash` (DeepSeek V4.1 Flash) at the requested `#high` route intent.
A separate effort control was not independently exposed by the harness.

Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on `chore/ci-and-release`. No commit or push
was made. `docs/status.md` was not edited by this cycle; the orchestrator owns it.

Evidence directory: `results/m2d-execution-plan-20261105-105500+0330/`.
Approved surfaces reused: `internal/deployment` descriptor/inventory validation,
`deployment-orchestration-plan-v2`, `runtime-artifact-manifest-v1` and the approved M2/M2b fixtures.
Preserved prior packet checked by hash: `docs/archive/sessions/m2c-render-verify-20261105.md` and the
prior M1f/M2/M2b/M2c packets listed in `prior-packet-hashes-before.txt` / `-after.txt`.

## 1. Scope and boundary

M2d adds a pure package API that decodes the already-approved descriptor, inventory and orchestration
v2 plan, derives a deterministic runbook, and writes JSON/Markdown plus `SHA256SUMS` into a **new**
output directory. It does not add a CLI command, Docker client, SUT access, observer, provisioning,
apply/up/down, scale, restart, stop, fault, load, reset, delete, exec, log read or shell invocation.

The new production surface is `internal/deployment/execution.go`:

| API | Behaviour |
|---|---|
| `BuildExecutionPlan(descriptorRaw, inventoryRaw, planRaw)` | Strictly decodes the three approved documents with the existing M2/M2b decoders, checks platform/project/service consistency, derives the runbook and validates it. Pure and offline. |
| `ExecutionPlan.Validate()` | Enforces the blocked-execution invariants, step order, expected-service field/health-mode consistency, resource-guard consistency and evidence-checklist semantics. |
| `DecodeExecutionPlan` | Strict exact-key/duplicate-key/null/trailing-JSON decode followed by `Validate`; an unblocked execution row is rejected. |
| `CanonicalJSON`, `CanonicalSHA256` | Deterministic compact JSON and semantic hash. |
| `MarshalExecutionPlan`, `RenderExecutionPlanMarkdown` | Deterministic pretty JSON and Markdown; no timestamp or live claim. |
| `WriteExecutionPlan(out, plan, inputs)` | Requires a new output directory outside input evidence and writes `deployment-execution-plan.json`, `deployment-execution-plan.canonical.json`, `deployment-execution-plan.md` and `SHA256SUMS`. |

No command string is emitted. Steps use structured operation identifiers such as
`descriptor.validate`, `compose.render_dry_run_reference` and `orchestration.apply_reference`; they are
documentation identifiers, not executables. No absolute working-directory path or environment probe is
included in the output.

## 2. Ordered runbook and execution blocking

The runbook is fixed in this order:

1. `preflight_validate` — descriptor/inventory/plan validation (`Read`, `Validate`, `Attest`)
2. `compose_render_guard_reference` — compose render/guard reference (`Read`, `Validate`, `RenderDryRun`, `Attest`)
3. `provision_reference` — blocked provisioning reference (`Validate`, `Attest`; blocked `Apply`, `Scale`)
4. `start_apply_reference` — blocked apply reference (`Attest`; blocked `Apply`, `Scale`, `Restart`, `Stop`)
5. `health_validate` — caller-supplied health/inventory comparison checklist (`Read`, `Validate`, `Attest`)
6. `evidence_package` — packaging checklist (`Read`, `Validate`, `Attest`)
7. `rollback_reference` — blocked restoration reference (`Read`, `Validate`, `Attest`; blocked `Restart`, `Stop`, `Reset`, `Delete`)

Rows 3, 4 and 7 carry `execution_blocked_here: true`, the literal marker `EXECUTION_BLOCKED_HERE`,
`execution_status: NOT EXECUTED`, an explicit blocked reason and
`required_authorization: separate-review-and-authorization-required`. Rows 1, 2, 5 and 6 are pure
in-process read/validate/render/attest rows: `execution_blocked_here: false`, no blocked actions and
marker `NOT EXECUTED`.

`ExecutionPlan.Validate` rejects a plan if `execution_blocked` is false, if any blocked row is
unblocked, if an execution row loses its reason/authorization/marker, if a read-only row is marked
blocked, if steps are reordered/removed/duplicated, or if any expected-service health mode, resource
total, observed-resource status or evidence status is fabricated. The strict decoder applies the same
checks to JSON. This is the schema-level impossibility of setting an execution stage to
`execution_blocked: false`. Service/role/health declaration derivation itself is performed by
`BuildExecutionPlan` from the approved plan (section 3), not by a second hardcoded table.

## 3. Derivation from orchestration v2

Expected services are derived only from the selected `PlatformBlueprint` in
`deployment-orchestration-plan-v2`. No service, tier, lifecycle, activation, healthcheck or evidence
role is duplicated as a second source. Services are sorted by tier (application then supporting) and
service name. Health modes are expanded from the plan's declarations:

| Plan declaration | Derived mode |
|---|---|
| `activation: default_start` | `default_start` |
| `activation: optional_profile` | `optional_profile_not_assessed` |
| `lifecycle: persistent` | `persistent_running` |
| `lifecycle: one_shot` | `one_shot_exit_code_zero` |
| `healthcheck: declared` | `explicit_healthcheck_healthy` |
| `healthcheck: none` | `healthcheck_none_functional_evidence` |

Evidence roles are the union of the platform's allowed evidence roles and the plan stages' expected
evidence roles, filtered against the existing packaging artifact taxonomy. Required roles remain
exactly the platform's `required_attestation_roles`.

## 4. Resource guard: declared versus observed

The resource guard is computed from the **descriptor declaration**, never from the supplied inventory
as a live measurement:

| Field | Source and meaning |
|---|---|
| `application_cpusets`, `supporting_cpusets` | Descriptor allocation lists, sorted |
| service `replicas` | Descriptor declared replicas |
| service `memory_bytes` | Descriptor declared memory bytes |
| `application_memory_bytes`, `supporting_memory_bytes` | `memory_bytes × declared replicas`, summed |
| `total_replicas`, `total_memory_bytes` | Sum of the declared services |
| `observed.status` | `NOT_ASSESSED` |
| `observed.source` | `no_live_observation` |

The inventory is validated as a caller-supplied document and checked for project/service ambiguity,
but its values are not substituted for resource measurements. No live value, zero or placeholder is
fabricated for the absent observation.

## 5. Artifact/evidence checklist

The checklist uses the approved `runtime-artifact-manifest-v1` interpretation, the packaging run-root
template `results/{run_id}/`, the manifest/checksum templates, and the packaging artifact taxonomy. Every
role row is `NOT_ASSESSED` with an explicit reason; no artifact was read, verified, copied or written.
The checklist records:

- `prior_manifest_verification: NOT_IMPLEMENTED`
- `prior_manifest_binding_status: NOT_ASSESSED`
- `live_validation: NOT_ASSESSED`

The required roles in all three platform outputs are `descriptor`, `inventory`, `run_manifest`,
`sha256sums` and `packet`; the role taxonomy is the full approved packaging taxonomy.

## 6. Fixtures

All fixtures are synthetic and labelled as such. The ThingsBoard input reuses the approved M2c synthetic
descriptor/inventory. FIWARE and EdgeX add descriptor/inventory fixtures whose service names, tiers,
lifecycle/activation/healthcheck semantics come from the approved plan; their images, cpusets and
memory declarations follow the unchanged `deploy/*/compose.yaml` and `.env.example` defaults. No Compose
file was edited or invoked.

| Fixture | SHA-256 |
|---|---|
| `internal/deployment/testdata/m2c/synthetic-descriptor-thingsboard.json` | `91e0dd8ebd79cf980a6d86a6a03459881805de27b5913b12a2838c8f88438681` |
| `internal/deployment/testdata/m2c/synthetic-inventory-thingsboard.json` | `f751636685bfb2563d4b6b95c44f6c1e7bdd2bb6663fa91bdf599f36ecb9cdae` |
| `internal/deployment/testdata/m2d/descriptor-fiware.json` | `5351d65712ed18b5eb15425dbd1ceb9cd2083f6af9958bc9e497f39aa51b50d4` |
| `internal/deployment/testdata/m2d/inventory-fiware.json` | `d929825636da8100d3fe2ab48d20b57907b4a31906715f8f2c57e9204016f9fe` |
| `internal/deployment/testdata/m2d/descriptor-edgex.json` | `6914264c47dab1493e5aa7531ce67872eba55d0c0db5d184587617e3e0ecf3a0` |
| `internal/deployment/testdata/m2d/inventory-edgex.json` | `8bfe1efd90c0e2f64522dfe67356a6be2320228879f2a81b815ddb6136bca9d8` |
| `internal/deployment/testdata/orchestration-plan-v2.json` | `d4b86042d078501bfe4b7e12a1d688fa0d3fa8861b5deccea76fa56d77a46fa5` |

## 7. Canonical outputs

The evidence helper `TestM2DWriteCanonicalOutputs` wrote all three platforms into the new
`canonical/` directory. Each platform directory contains the pretty JSON, canonical JSON, Markdown and
its own `SHA256SUMS`.

| Platform | Canonical SHA-256 | Canonical bytes | Markdown SHA-256 | Markdown bytes |
|---|---|---:|---|---:|
| ThingsBoard | `58c5f633590459427727470229a2196b01be1b9cf6890a12faf4417b8aa54ef3` | 13,940 | `55a8721f0744b20a08c362222b7411eb3608e5648e5441a6023854313d60097b` | 13,384 |
| FIWARE | `a01a48d8a7ae22c4dfebc3d573d38067780930557cc30af2b3bbbeb01a7a2c80` | 12,578 | `0eb71b57b96d728a43e2d5f5ef6d2e78b70e3d634fa2bafc47eca280268218e2` | 12,590 |
| EdgeX | `b0ce956c4e09754a5e1f5683953ac73e47ef2c3733c5a3d31fdee99943cef757` | 14,210 | `626fe6b94babd59445d760e37bf6952cb3e07b9faa63f88f7120f5e28bb9966c` | 13,571 |

Repeated builds from the same inputs produce byte-identical canonical JSON and Markdown. The canonical
bytes round-trip through `DecodeExecutionPlan` without changing the canonical hash.

## 8. Tests and command evidence

All recorded commands exited `0`. Logs and `.exit` files are in the evidence directory. The focused
tests cover all three platform fixtures, deterministic bytes/hash, stable step order, blocked/read-only
Markdown labelling, unblocked/mislabeled execution rejection, missing/incomplete/ambiguous input
rejection, resource counts, evidence checklist, strict round-trip and new-directory enforcement.

| Command | Result |
|---|---|
| `go test -count=1 -run 'TestM2D' ./internal/deployment` | 0 |
| `go test -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| `go test -short -count=1 ./cmd/... ./internal/... ./tests/...` | 0 |
| `go test -race -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| `go vet ./internal/deployment ./cmd/benchctl` | 0 |
| `go mod verify` | 0 |
| `git diff --check` | 0 |
| `gofmt -l internal/deployment cmd/benchctl` | empty, 0 |
| `go build -o /tmp/opencode/m2d-build/benchctl ./cmd/benchctl` | 0 |
| `go test -count=1 -v -run 'TestM2D' ./internal/deployment` | 0 |
| canonical evidence generation | 0 |
| preserved prior evidence re-hash | 0 (268 references, 0 problems) |
| protocol/profile hash comparison | 0 (unchanged) |
| hidden external-call/lifecycle scan | 0 (no matches) |
| patch round-trip `patch -p1 --fuzz=0` | 0 (6/6 files match) |

This is a scoped result, not an unrestricted full-suite claim. Docker integration is skipped by
`-short`; no claim is made that unrestricted `go test ./...` passes over retained historical snapshots
under `results/`.

## 9. Integrity and preservation

| Item | Value |
|---|---|
| HEAD / branch | `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` / `chore/ci-and-release` |
| Source snapshot before edits | 397 files, `afb2338ef5d40259433ad12b11c7113a3bd489b3fa04bcb920a71c31ad83c266` |
| Source snapshot after edits | recorded in `source-final.sha256` / `source-final-count.txt` / `summary.txt` |
| Retained `results/` excluding `m1f-*`, `m2-*`, `m2b-*`, `m2c-*`, `m2d-*` | 2,136 files, `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`, identical before and after |
| Protocol definition SHA-256 | `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` |
| D44 profile SHA-256 | `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` |
| `docs/status.md` before / after | `d2660ba975e8e27e22edcb9396bd218fd1e3b5e3e4b8090ad033546257b25d85` / unchanged (pre-existing working-tree edit; not touched by M2d) |
| Prior M1f/M2/M2b/M2c packets | hash list unchanged before/after; diff is empty |
| Preserved prior evidence manifests | 268 declared artifact references re-hashed, 0 problems |
| `incremental.patch` | `bb1a7583ca7f461d45719dd54c295e647ec28dc51ebf6f3a3ea5ac7f99c1bafa`, 102,045 bytes |
| Patch round-trip | exit 0, 6/6 files match `final-files.sha256` |
| Changed files | 6 additions; no existing source, schema, CLI, Compose, `.env`, Makefile or documentation file modified |

`incremental.patch` contains exactly the six new source/fixture files. It excludes this packet and the
evidence directory so the packet can record the patch hash without a circular reference. The pre-edit
hash manifest is empty because every changed file is an addition; `pre-edit-absence.txt` records the
absent paths.

## 10. Hidden-action and lifecycle exclusion

A scan of the new source, tests and fixtures found no `os/exec`, `exec.Command`, `net/http`,
`net.Dial`, `syscall`, `docker compose/run/exec`, Compose up/down/stop/restart/scale, `kubectl`,
`curl` or `wget` path. The only lifecycle words are explicit exclusion text, the approved blocked
action taxonomy or test names that exercise rejection. See `no-hidden-lifecycle-scan.log`.

No Docker daemon, SUT, platform, receiver, ledger, monitoring service or network endpoint was
contacted. No live health, replica, label, volume or resource value is claimed. No command is exposed
as a capability.

## 11. Limits and non-goals

- No live execution, provisioning, apply/up/down, scale, restart, stop, fault, load, reset, delete,
  exec, log read or observer implementation.
- The runbook is a render-only document. `EXECUTION_BLOCKED_HERE` and `NOT EXECUTED` are metadata for
  a future separately reviewed and explicitly authorized milestone, not callable operations.
- Resource guard values are descriptor declarations; observed resource state is `NOT_ASSESSED`.
- Evidence checklist rows are `NOT_ASSESSED`; prior-manifest binding remains `NOT_IMPLEMENTED` and
  live validation remains `NOT_ASSESSED`.
- No CLI command was added. No commit, push or `docs/status.md` edit was made.
- Docker integration is skipped by `-short`; no unrestricted full-suite claim is made.

**Stop for independent review.** M2d is not approved by this packet. Any live deployment, provisioning,
apply, observation or campaign requires separate review and explicit authorization.
