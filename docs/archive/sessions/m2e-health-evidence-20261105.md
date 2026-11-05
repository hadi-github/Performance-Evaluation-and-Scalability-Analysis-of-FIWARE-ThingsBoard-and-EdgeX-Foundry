# M2e synthetic health-evidence dry-run packet — 2026-11-05

**Self-tested offline implementation and evidence. The owner explicitly waived a separate M2e review;
this packet is not a review gate. No Docker client call, SUT access, deployment, lifecycle action,
observer implementation, fault, load, reset, delete or live validation is claimed. No CLI command,
executor or lifecycle method was added. No commit or push was made.**

Requested route: `opencode-go/deepseek-v4.1-flash#high`. Offline only.

Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on `chore/ci-and-release`. `docs/status.md`
was not edited; the orchestrator owns it. No prior packet or evidence file was edited.

Evidence directory: `results/m2e-health-evidence-20261105-113210+0330/`.

## 1. Scope

M2e adds a pure `internal/deployment` API that compares caller-supplied **synthetic** service
observations with an approved M2d execution plan and writes a new-directory evidence package. It adds
no CLI command, Docker client, SUT or network access, lifecycle action, observer implementation,
deployment, fault, load, reset or delete path.

Added files:

| File | Purpose |
|---|---|
| `internal/deployment/health_evidence.go` | Strict synthetic-observation schema, health evaluation, report schema/validation, deterministic render and new-directory writer |
| `internal/deployment/health_evidence_test.go` | M2e tests and env-gated canonical evidence helper |
| `internal/deployment/testdata/m2e/synthetic-observations-thingsboard-pass.json` | Synthetic ThingsBoard observations |
| `internal/deployment/testdata/m2e/synthetic-observations-fiware-pass.json` | Synthetic FIWARE observations |
| `internal/deployment/testdata/m2e/synthetic-observations-edgex-pass.json` | Synthetic EdgeX observations |
| `docs/development/health-evidence-dry-run.md` | Offline API documentation |
| `docs/archive/sessions/m2e-health-evidence-20261105.md` | This packet |
| `results/m2e-health-evidence-20261105-113210+0330/` | New evidence directory only |

No existing source file required modification. M2d has no observation document; M2e does not derive
observations from the M2 inventory and does not alter any M2d checklist to `PASS`.

## 2. API and contracts

Constants: `SyntheticObservationsSchemaVersion = 1`,
`SyntheticObservationsInterpretation = synthetic-service-observations-v1`,
`HealthEvidenceSchemaVersion = 1`,
`HealthEvidenceInterpretation = deployment-health-evidence-dry-run-v1`,
`HealthEvidenceMode = offline-synthetic-dry-run`.

`SyntheticServiceObservations` reuses `ServiceObservation` unchanged. Shared strict raw validation
rejects case aliases, unknown keys, duplicate keys, explicit nulls, trailing JSON and missing fields.
The source must begin with `SYNTHETIC M2e`; the run tag must begin with `m2e-synthetic`; the run
identity is validated through the resulting `ArtifactManifest.Validate`. An explicit empty observation
array is admitted. Duplicate/invalid service names, negative replicas and health strings outside
`healthy`, `unhealthy`, `none`, `unknown` are rejected. Contradictory but decodable observations are
evaluated to `FAIL`/`UNAVAILABLE`, not rejected.

`BuildHealthEvidenceDryRun(executionPlanRaw, observationsRaw)` is pure and file-free. It decodes with
`DecodeExecutionPlan` and `DecodeSyntheticServiceObservations`, rejects observations for services the
plan does not declare, copies `ExpectedService` declarations, and calls only
`EvaluateServiceExpectation`. An omitted expected service is represented explicitly as
`{service, present: false, health: "unknown"}`; no presence, replica, exit code or functional evidence
is invented. Results are sorted by tier then service.

`HealthEvidenceDryRun` carries `schema_version`, `interpretation`, `mode`, synthetic label, platform,
project, observation source, input raw/canonical hashes and byte counts, the unchanged canonical
execution plan, health results, health/evidence/dry-run aggregates, the declared artifact manifest,
actual artifact verification, and fixed status boundaries. `Validate` recomputes every health status
and reason, validates the nested plan canonical hash, checks the two-artifact manifest, verifies the
aggregates and rejects tampered execution, manifest, verification or status fields.

Fixed values: `live_validation = NOT_ASSESSED`, `observer_status = NOT_ASSESSED`,
`prior_manifest_verification = NOT_IMPLEMENTED`, `prior_manifest_binding_status = NOT_ASSESSED`.
`dry_run_status` is the aggregate of health and artifact evidence only.

## 3. Packaging and writer

`WriteHealthEvidenceDryRun` requires the output path to end with `results/<manifest.run_id>`, rejects
an existing directory and an output inside an input tree, and calls `newDirectory` before any write. It
copies the exact raw execution plan and observations, calls the existing `ArtifactManifest.Verify`
with the repository root (the parent of `results/`), records the actual verification rows, then writes
the final report. A health `FAIL` is a valid report, not a writer error.

The manifest declares exactly two required `this_run` artifacts and no `preserved_prior` artifact:

| Role | Copied file |
|---|---|
| `orchestration_plan` | `execution-plan.input.json` |
| `other` | `synthetic-service-observations.json` |

Output files: `health-evidence-dry-run.json` (pretty JSON, two-space indent, trailing newline),
`health-evidence-dry-run.canonical.json` (compact canonical JSON, no trailing newline),
`health-evidence-dry-run.md`, the two exact raw inputs, `artifact-manifest.json` and `SHA256SUMS`. The
manifest excludes itself and `SHA256SUMS`; `SHA256SUMS` excludes itself. Services are sorted by tier
then service, artifacts and verification rows by path, and observations by service. The nested
execution step order and limitations are preserved.

The Markdown prominently labels the document `SYNTHETIC M2e — OFFLINE DRY RUN`, shows health and
artifact evidence separately, repeats the three `EXECUTION_BLOCKED_HERE` / seven `NOT EXECUTED` rows,
and states that `PASS` is supplied synthetic consistency only, not live qualification.

## 4. Canonical outputs

Generated with `M2E_CANONICAL_OUT` and retained under the evidence directory's `canonical/` tree:

| Platform | Canonical SHA-256 | Canonical bytes | Markdown SHA-256 | Markdown bytes |
|---|---|---:|---|---:|
| thingsboard | `2f4c1a2da19500c9a89ae9a870ced77fe20df0a691c1e2172e61c6e3898337b4` | 22132 | `999fd84381c628581f49815eb958b4259882a2a74d8dffde013d57593a305967` | 7114 |
| fiware | `d54dc07f3ce1e6f604615b713fc9d37a950a8948b20d0d95f02ef573d6eac880` | 18669 | `bdf33c589edefb26e64faac49d21ebf05cc70c847e9207b1ba8125949acee506` | 6386 |
| edgex | `fd791556a41c8089553edb5b23e2db894fa8f2c6d2348e0269fce8bd4d5e68c1` | 22974 | `52effadc226f3fa2ae6a2cb0a269f215b63df0f6d161e0adb352e257d9bc0162` | 7290 |

Each canonical report was written after actual verification of both copied inputs, so its
`evidence_status` and `dry_run_status` are `PASS`; all health results are `PASS` for the all-pass
synthetic fixtures. The pure build before writing correctly reports evidence `NOT_ASSESSED` because no
file has been verified yet.

## 5. Tests and retained commands

The new tests cover all three all-pass platforms; persistent healthy/functional evidence; declared
unhealthy `FAIL`; `none`/`unknown` `UNAVAILABLE`; no-healthcheck functional present/absent; not-running
and replica mismatch `FAIL`; one-shot exit `0`/non-zero/missing (including running+healthy with no exit
code); absent default `UNAVAILABLE`; absent optional persistent/one-shot `NOT_ASSESSED`; activated
optional one-shot success/failure; duplicate and unknown services; strict alias/unknown/duplicate/
null/trailing/missing/bad-schema/non-synthetic inputs; unblocked and tampered M2d plans; tampered output
status, reason, live/observer/prior fields, execution boundary, manifest, verification and aggregates;
deterministic bytes/hashes; round-trip; existing/input-tree/wrong-layout/nil writer rejection;
`Verify` pass then tamper `FAIL` and missing `UNAVAILABLE`; manifest/SHA self-reference; blocked rows;
and Markdown labels. `TestM2EWriteCanonicalOutputs` is env-gated by `M2E_CANONICAL_OUT`.

Commands are retained as `command-*.log` with `command-*.exit` in the evidence directory:

| Command | Expected exit |
|---|---|
| `go test -count=1 -run 'TestM2E' ./internal/deployment` | 0 |
| `go test -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| `go test -short -count=1 ./cmd/... ./internal/... ./tests/...` | 0 |
| `go test -race -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| `go vet ./internal/deployment ./cmd/benchctl` | 0 |
| `go mod verify` | 0 |
| `gofmt -l internal/deployment/health_evidence.go internal/deployment/health_evidence_test.go` | 0, empty output |
| `git diff --check` | 0 |
| `go build -o /tmp/opencode/m2e-build/benchctl ./cmd/benchctl` | 0 |

Docker integration is skipped by `-short`; no unrestricted full-suite qualification is claimed. This is
a self-test only; no independent M2e review gate was run by instruction.

## 6. Preservation and integrity

The evidence packet records:

- changed-file list and pre-edit absence at HEAD for every added path;
- before/after hashes for `docs/status.md` and prior session packets;
- protocol SHA-256 `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` and D44 profile
  SHA-256 `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`;
- retained prior results excluding `m1f`/`m2`/`m2b`/`m2c`/`m2d`/`m2e`, expected 2,136 files with
  retained hash `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`;
- 268 prior artifact references checked with zero problems;
- an incremental patch generated only from the added files with a clean apply/compare/reverse-applied
  round trip;
- no-hidden-lifecycle scan over the new production source;
- `SHA256SUMS` over the evidence directory excluding itself.

## 7. Limits

- This is a synthetic, offline dry run. `PASS` is not live qualification, deployment evidence,
  reachability proof, campaign success or a prior-success claim.
- The M2d execution plan remains render-only. Three execution rows stay `EXECUTION_BLOCKED_HERE` and all
  seven rows stay `NOT EXECUTED`.
- No protocol or profile file is read or re-verified by M2e; the report includes no protocol/profile
  claim unless a supplied input declaration already carried one. It makes no campaign claim.
- No live service, replica, health, exit code, functional evidence, artifact, timestamp or environment
  value was collected or fabricated.
- Owner waived independent M2e review. This packet is completion evidence produced by the implementing
  agent, not an independent review.
