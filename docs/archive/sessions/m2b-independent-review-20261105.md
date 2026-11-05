# M2b independent review — 2026-11-05

**CHANGES_REQUIRED.** M2b is not approved. No live, Docker, SUT, deployment, lifecycle, load or
campaign qualification is granted.

Review session: `ses_e56f5f069ffeyi56DY1tvJWBGO`, requested `opencode-go/kimi-k3.1-pro#low`;
harness identified Kimi K3.1 Pro and exposed effort `low`. Implementation packet
[`m2b-blueprint-20261105.md`](m2b-blueprint-20261105.md); implementation evidence
`results/m2b-blueprint-20261105-033125+0330/`; baseline
`results/m2b-baseline-20261105-032542+0330/`.

## Findings

### M2B-R1 — HIGH — state gates contradict the unchanged Compose stack

`docs/development/deployment-orchestration-blueprint.md:94–129` requires `healthy` containers that
the existing stacks never declare Compose healthchecks for:

- ThingsBoard: application services, Zookeeper and HAProxy (`deploy/thingsboard/compose.yaml`
  59–270)
- FIWARE: Orion and IoT Agent (`deploy/fiware/compose.yaml` 66–127)
- EdgeX: `core-data` and `device-rest` (`deploy/edgex/compose.yaml` 192–245)

It also misclassifies lifecycle:

- EdgeX `core-common-config-bootstrapper` is a one-shot `restart: "no"` job
  (`deploy/edgex/compose.yaml` 139–165)
- FIWARE omits the default-start dependency-required `mongo-init`
  (`deploy/fiware/compose.yaml` 49–62, 90–91) without separately documenting that omission
- ThingsBoard `tb-install` handling was not described

As written, a correct deployment of the unchanged stacks could not pass the proposed future gate.

Required fix: model explicit persistent-service versus one-shot lifecycle semantics; successful
completion for one-shot jobs; document optional `tb-install`; distinguish `health:none` from a
failed check; require running/functional evidence only where no healthcheck exists. Reflect this
in schemas/fixtures and tests, or explicitly defer any healthcheck changes to a separately
reviewed stack-change milestone. Do not silently alter deployment semantics.

### M2B-R2 — MEDIUM — empty run identifier suffix accepted

`internal/deployment/packaging.go:178–181` requires only a prefix. A value such as
`m2b-blueprint-20261105-` passes validation with an empty `{run identifier}` although the
specification expects a non-empty suffix after `{tag}-{YYYYMMDD}-`.

Required fix: require a non-empty suffix after that date separator. Add an exact-prefix rejection
test.

### M2B-R3 — MEDIUM — `Verify` documentation overstates prior-manifest checks

`runtime-artifact-packaging.md:130–134` says `ArtifactManifest.Verify` implements rules 5–7, but
rule 7 requires prior artifacts to be verified against their own prior-manifest hashes, not merely
the current manifest. The implementation only re-hashes declared current paths
(`packaging.go:310–345`).

Required fix: either narrow the documented API to the checks actually implemented and explicitly
leave prior-manifest verification to a future packaging workflow, or implement and test a
validated prior-manifest binding. Do not imply capability that the code does not perform.

## Verified safe

- No execution pathway exists: no new CLI wiring, Docker/SUT client, scheduler, lifecycle method
  or hidden command. Existing stack/deployment/workload assets are reuse references only.
- Execution blockers are explicit and disjoint; provisioning, apply and rollback require lifecycle
  references plus `execution_blocked_here`; apply specifically requires blocked `Apply`; the
  documentation says apply is NOT EXECUTED.
- Recursive exact-key validation remains effective; duplicate keys, nulls, unknown keys, missing
  keys and trailing JSON fail closed.
- Canonicalization/rendering is deterministic; no caller-independent timestamps or live
  observations are introduced.
- Packaging rules reject traversal, absolute paths, duplicate paths, current-run placement of
  prior artifacts and self-referential manifests/checksums. The only filesystem operation added is
  read-only `os.ReadFile` in `Verify`.
- Patch round-trips 6/6 (`fc67c1f7…a8f`).
- Retained results independently verify exactly 2,136/2,136, `8e25006644…25ec`.
- Protocol/D44 remain `7176b1fa…d24ee` and `1b6aa76d…d1f8`.
- HEAD remains `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`; no new commit. Recorded `docs/status.md`
  before/after hashes match; the current file only differs because its M2b row was updated by the
  orchestrator before review, and the review made no status edit.

## Independent reproduced checks

Focused tests, scoped `-short`, race, vet, formatting, diff check and build all passed
reproduced. No unrestricted test or live operation was run.

## Next

DeepSeek Flash corrects M2B-R1–R3 offline; Kimi re-reviews before approval. Any future live
deployment remains a separate explicitly authorized milestone, not a completion claim.
