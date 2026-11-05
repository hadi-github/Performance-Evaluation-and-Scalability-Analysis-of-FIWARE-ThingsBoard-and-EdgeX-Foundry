# M2 bounded read-only deployment/instrumentation foundation — 2026-11-05

**Ready for independent Kimi K3.1 Pro (low effort) review. Offline schema, validator, observer
foundation and dry-run renderer only. No Docker client call, SUT access, deployment, lifecycle
action, load or campaign action is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`) and exposes effort `high` as the
requested route intent; a separate effort control was not independently observable. Baseline HEAD:
`36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on `chore/ci-and-release`. Pre-edit baseline packet:
`results/m2-baseline-20261105-023722+0330/`. Implementation packet:
`results/m2-deployment-foundation-20261105-024722+0330/`. No commit or push was made, and
`docs/status.md` was not edited by this cycle.

This packet supplements the preserved M0b/M1a–M1f packets. It does not modify retained evidence,
`docs/status.md`, the protocol definition, the D44 profile policy, sink/receiver contracts,
existing analysis claims or any metric threshold.

## What changed

| File | Purpose |
|---|---|
| `internal/deployment/schema.go` | Strict versioned descriptor/inventory schemas, canonicalization, statuses, duplicate/null/unknown rejection and CPU-set validation. |
| `internal/deployment/validation.go` | Descriptor-versus-inventory validator with explicit per-service, total and global states. |
| `internal/deployment/observer.go` | Package-level read-only `Observer` interface and file-backed `FakeObserver`; no Docker implementation and no lifecycle method. |
| `internal/deployment/evidence.go` | Versioned evidence manifest with SHA-256 paths, optional caller timestamp and read-only re-hash verification. |
| `internal/deployment/render.go` | Deterministic canonical JSON/Markdown dry-run report, new-output-directory writer and `SHA256SUMS`. |
| `internal/deployment/deployment_test.go` | Targeted malformed/cross-project/duplicate/incomplete/ambiguous/unsupported/state tests, permutation invariance, deterministic output and integrity tests. |
| `cmd/benchctl/deployment.go`, `cmd/benchctl/main.go` | New offline `deployment-validate` command in the existing CLI. |
| `docs/development/deployment-foundation.md` | Documents the M2 schema, CLI, states and lifecycle exclusion. |
| `docs/archive/sessions/m2-deployment-foundation-20261105.md` | This packet. |

## 1. Versioned deployment descriptor v1

The descriptor is exact-key JSON with `schema_version: 1` and these required top-level
declarations:

- allowlisted `platform`: only `thingsboard`, `fiware` or `edgex`;
- Compose `project`;
- `allocation.application_cpusets` and `allocation.supporting_cpusets`;
- `protocol.binding_sha256` and `protocol.profile_sha256` (optional profile path);
- `provenance.source` and optional Compose path/hash, provenance documents and caller timestamp;
- explicit non-empty `services`.

Each service requires a valid Compose name, `class` of `application` or `supporting`, positive
`replicas`, a `cpuset` contained in the CPU sets declared for its class, and positive
`memory_bytes`. Optional `image` and `digest` declarations are shape-validated; a descriptor
digest must be `sha256:<64 lower-case hex>`. Optional top-level
`effective_configuration.sha256`/`.path` is a caller attestation.

Unknown fields, duplicate keys, explicit nulls, trailing JSON, missing required fields, duplicate
services, unsupported platforms, unknown classes, non-positive replicas/memory, invalid CPU sets,
CPU sets outside the class allocation, malformed hashes and unsupported evidence roles are
rejected. Service, allocation and evidence-document arrays are canonicalized by ordering, and a
canonical descriptor SHA-256 is emitted alongside the raw document SHA-256.

## 2. Versioned observed inventory v1

The inventory is caller-supplied only and has explicit `schema_version`, `project`, `source`, an
explicit non-empty `services` array, optional `captured_at_ms` and optional effective
configuration. Each service requires its enclosing `project` and `service`, a positive
`replicas` count and a non-empty `containers` array. Each container requires `project`, `service`,
`container_id`, replica index, image, explicit positive `started_at_ms`, health and state.

Decoding rejects:

- duplicate services, container IDs and replica indices;
- declared/actual replica mismatches and incomplete replica sets;
- service or container project mismatches, including an empty enclosing project;
- container service spoofing and cross-project rows;
- empty, unsupported or unreasoned non-healthy/non-running states;
- malformed image digests, missing identities and missing replica counts.

The validator additionally rejects a descriptor/inventory project mismatch, missing required
services, unexpected observed services, non-contiguous realized topology, unhealthy or
non-running containers, image/digest mismatches, cpuset mismatches and effective-configuration
mismatches. A missing observer remains `NOT_ASSESSED`; a missing declared digest or cpuset is
`UNAVAILABLE`, never a matching zero.

## 3. Result states and validation

`Validate` returns a versioned `deployment-attestation-v1` report with explicit `PASS`, `FAIL`,
`UNAVAILABLE` and `NOT_ASSESSED` states. It reports:

- required versus observed application and supporting replica totals;
- per-service `service_present`, `replicas`, `health`, `state`, `image`, `digest`, `cpuset` and
  `topology` checks;
- global `project`, `unexpected_services`, `effective_configuration`, `protocol_binding_declared`,
  `observer` and `live_validation` checks;
- descriptor/inventory raw and canonical hashes, protocol declaration, provenance and inventory
  source.

`live_validation` is always `NOT_ASSESSED` in M2. `PASS` means only that the supplied inventory
document agrees with the supplied descriptor; it does not mean a deployment exists, is healthy or
is reachable.

## 4. Read-only observer

`internal/deployment.Observer` has exactly one method, `Observe(context.Context) (Inventory,
error)`. It has no start/stop/restart/exec/log-action or other lifecycle method. The only
implementation is `FakeObserver`, which reads one explicit JSON file. No Docker implementation is
present. An absent observer yields `NOT_ASSESSED` with no fabricated name, inventory hash or zero
replica count.

## 5. Dry-run rendering and CLI

`MarshalReport` and `RenderMarkdown` render deterministic canonical JSON/Markdown in memory. The
Markdown explicitly displays `Live validation: NOT_ASSESSED`, per-check `NOT_ASSESSED`/
`UNAVAILABLE`, the missing observer, provenance and the lifecycle exclusion. `Write` requires a
new output directory, refuses an existing directory or output under an input path, and writes
`deployment-validation.json`, `deployment-validation.md`, `evidence-manifest.json` and
`SHA256SUMS` only inside that directory.

The offline CLI is:

```sh
go run ./cmd/benchctl deployment-validate \
  -descriptor FILE -inventory FILE \
  [-observer-fake FILE] [-evidence-manifest FILE] -out NEW_DIRECTORY
```

The command reads only named files, performs no live probe and creates no temporary state outside
the specified output directory. It exits `0` only for `PASS`, `1` for a non-`PASS` attestation and
`2` for usage/input errors. A supplied fake observer must return the same canonical inventory as
`-inventory`; a mismatch is recorded as `FAIL` and the attestation exits non-zero.

## 6. Evidence manifest and integrity

`EvidenceManifest` schema version 1 lists `role`, `path` and `sha256` for caller-supplied
documents and has no timestamp by default. `captured_at_ms` appears only when a caller explicitly
supplies it; descriptor `provenance.created_at_ms` is likewise optional. Verification re-hashes
only the listed documents. The CLI emits a generated manifest from the descriptor, inventory and
optional fake-observer document, or verifies and preserves a caller-supplied manifest, including
its explicit timestamp. No timestamp is invented anywhere.

## 7. Tests and validation

The focused package tests cover valid descriptors/inventories, descriptor and inventory
permutation invariance, deterministic JSON/Markdown, malformed/unknown/null/duplicate-key/trailing
documents, unsupported platforms/classes/roles, duplicate and incomplete replica sets, cross-
project and nested project/service spoofing, missing services, image/digest/cpuset/configuration
mismatches, unavailable optional evidence, unexpected services, unhealthy/non-running states,
observer absence and mismatch, evidence-manifest verification/tamper detection and new-output-
directory enforcement.

Recorded commands and exits are in the packet. All positive commands exited `0`; the deliberate
mismatch CLI smoke exited `1` as expected:

```sh
go test -count=1 ./internal/deployment ./cmd/benchctl
go test -short -count=1 ./cmd/... ./internal/... ./tests/...
go test -race -count=1 ./internal/deployment
go vet ./internal/deployment ./cmd/benchctl
git diff --check
gofmt -l internal/deployment cmd/benchctl/deployment.go cmd/benchctl/main.go   # empty
go build -o /tmp/opencode/m2-build/benchctl ./cmd/benchctl
sha256sum research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json \
  research/protocols/offline-profiles-20261101.json
go test -count=1 -v -run 'TestDecode|TestValidate|TestObserver|TestRender|TestEvidence' ./internal/deployment
go run ./cmd/benchctl deployment-validate -descriptor ... -inventory ... -out NEWDIR
go run ./cmd/benchctl deployment-validate ... -observer-fake ... -out NEWDIR
go run ./cmd/benchctl deployment-validate ... -inventory mismatch ... -out NEWDIR   # exit 1
go run ./cmd/benchctl deployment-validate ... -evidence-manifest ... -out NEWDIR
```

The scoped command uses only the task-specified paths, not unrestricted `go test ./...`.
Docker-backed integration is skipped by `-short`; no claim is made that unrestricted
`go test ./...` passes over retained historical snapshots under `results/`.

## 8. Integrity and preservation

| Item | Value |
|---|---|
| HEAD / branch | `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` / `chore/ci-and-release` |
| Baseline source snapshot | 361 files, `b772a6d69ddc96101a68f11423bc02e1df394006555c1272be34d45fed74beff` |
| Final source snapshot | 371 files, see `source-final.sha256` and `summary.txt` |
| Protocol definition SHA-256 | `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` |
| D44 profile SHA-256 | `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` |
| Retained `results/` excluding `results/m1f-*` and `results/m2-*` | `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`, 2,136 files |
| `incremental.patch` SHA-256 | `406ab86dbaa013738b7194de981f30f1170083ffd020815ee2018ce184a75ce1` |
| Patch round-trip | exit `0`, **9/9 files** match final SHA-256 |

The incremental patch was generated against the verified pre-edit `cmd/benchctl/main.go` (its
reconstructed SHA-256 equals the baseline snapshot hash) plus `/dev/null` entries for the eight
new files. It round-trips in a scratch tree with `patch -p1 --fuzz=0`; every resulting file hash
matches the final working tree. `evidence-manifest.json` records artifact hashes, command exits
and the smoke-run states.

## Non-goals and lifecycle exclusion

No Docker client call, SUT/platform/sink/adapter access, deployment, apply/up/down/restart/
stop/fault, container exec, log action, load, campaign action or live qualification was performed.
There is no live observer implementation. The supplied descriptor, inventory and evidence
documents are caller attestations, not independently observed runtime facts. CPU sets, memory,
images, digests, configuration checksums and container health/state are declarations. `PASS` is a
document-attestation state, not proof of a deployment.

**Stop for independent Kimi K3.1 Pro (low effort) review.**
