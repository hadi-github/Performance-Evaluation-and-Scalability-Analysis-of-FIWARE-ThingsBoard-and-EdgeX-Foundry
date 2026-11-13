# Deployment orchestration blueprint (M2b)

**Status: offline blueprint only. Actual deployment is NOT executed.** This document extends the
approved M2 deployment/instrumentation contract with a versioned orchestration blueprint and an
artifact packaging contract. It is not a new project or a new framework: it reuses the existing
bounded-milestone framework and the already approved M2 schema, validator, observer and receipt/sink
contracts. It is not an M3 fragility/fault campaign and makes no M3 readiness claim. M3 and any live
orchestration remain future milestones requiring separate review and explicit authorization.

- Baseline: branch `chore/ci-and-release`, HEAD `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`.
- Approved predecessor: M2 foundation, `research/archive/sessions/m2-approval-20261105.md`.
- No Docker client call, SUT access, deployment, lifecycle action, load or campaign action is
  performed by M2b.

## 1. Execution boundary and action taxonomy

M2b documents and schemas carry two disjoint action sets. The distinction is semantic, not
cosmetic: a safe action observes or describes; a blocked action changes platform state.

| Safe taxonomy | Meaning | Admitted in a stage's `permissible_actions` |
|---|---|---|
| `Read` | Read an explicit, already-existing file or document | yes |
| `Validate` | Compare/schema-check caller-supplied documents | yes |
| `RenderDryRun` | Produce deterministic text/JSON from supplied documents | yes |
| `Attest` | Record what was supplied and what was not assessed | yes |

| Blocked taxonomy | Meaning | Admitted only as a blocked reference |
|---|---|---|
| `Apply` | Create/update a Compose deployment | no |
| `Scale` | Change replica counts | no |
| `Restart` | Restart a service/container | no |
| `Stop` | Stop a service/container | no |
| `Fault` | Inject a fault | no |
| `Load` | Start a load/campaign action | no |
| `Reset` | Reset state or storage | no |
| `Delete` | Delete a container, volume, run directory or evidence | no |

No stage may call a blocked action. A stage that references a blocked action must set
`execution_blocked_here: true` and is rendered with the literal marker `EXECUTION_BLOCKED_HERE`.
The marker is metadata for a future separately authorized capability; it is not a callable
operation. The `apply` stage is always blocked and must reference `Apply`.

In particular, M2b contains **no** `docker compose up/down/stop/restart`, `docker compose scale`,
`docker run`, `docker exec`, fault injection, k6 execution, probe, reset or delete path. Existing
Makefile targets are definitions in the current repository, not commands authorized by this
milestone.

## 2. Versioned orchestration stages

The stage sequence is fixed and versioned. Each platform declares the same state machine.

| # | Stage | Permissible actions | Blocked reference | Expected output |
|---:|---|---|---|---|
| 1 | `pre_deploy_inventory` | `Read`, `Validate`, `Attest` | — | Validated descriptor, observed inventory, compose/`.env`/Makefile hashes, protocol/profile hashes |
| 2 | `provision_on_sut` | `Validate`, `Attest` | `Apply`, `Scale` — `EXECUTION_BLOCKED_HERE` | Declaration of the intended replica set; no provisioning |
| 3 | `dry_run_render` | `Read`, `Validate`, `RenderDryRun`, `Attest` | — | Deterministic orchestration plan and dry-run attestation |
| 4 | `apply` | `Attest` | `Apply`, `Scale`, `Restart`, `Stop` — `EXECUTION_BLOCKED_HERE` | Reference row only; **apply is NOT EXECUTED** |
| 5 | `validate_health` | `Read`, `Validate`, `Attest` | — | Comparison of caller-supplied health/inventory evidence; `live_validation=NOT_ASSESSED` |
| 6 | `evidence` | `Read`, `Validate`, `Attest` | — | Manifest, hashes, logs and preserved prior ledgers |
| 7 | `rollback` | `Read`, `Validate`, `Attest` | `Restart`, `Stop`, `Reset`, `Delete` — `EXECUTION_BLOCKED_HERE` | Restoration rules and interruption exception; no restoration executed |

Stage rules:

1. `provision_on_sut` does not provision. It only declares the expected Compose project, service
   set and replica set and validates that the declaration matches the unchanged repository.
2. `dry_run_render` renders from caller-supplied documents. It does not inspect Docker, a SUT or a
   live endpoint.
3. `apply` is a **reference stage**. Its presence in the plan records where a future authorized
   apply would occur; it is permanently marked `EXECUTION_BLOCKED_HERE` in M2b.
4. `validate_health` compares supplied observations. A missing observation is `UNAVAILABLE` or
   `NOT_ASSESSED`, never a healthy zero. M2 keeps `live_validation: NOT_ASSESSED`.
5. `evidence` packages identity only. It must not rewrite, move or delete a prior artifact.
6. `rollback` records restoration rules. It must not execute restoration in M2b.

The machine-readable plan is `deployment.OrchestrationPlan` schema version 2
(interpretation `deployment-orchestration-plan-v2`). Version 2 adds the explicit
`lifecycle`/`activation`/`healthcheck` service declarations described in section 3. Version 1 is
rejected rather than silently reinterpreted. Decoding uses the same exact-case-key, duplicate-key,
explicit-null and trailing-JSON checks as the approved M2 documents. `Validate` rejects a plan that
is executable (`execution_blocked: false`), reorders the stages, substitutes a platform's Compose
project, admits a lifecycle action as permissible, omits the apply marker, makes missing evidence
`PASS`/`FAIL`, uses an unknown lifecycle/activation/healthcheck value, gives a `one_shot` service
more than one replica, or omits a default-start service from the unchanged Compose stack.

## 3. State machine, lifecycle and health semantics

Common read-only state machine:

```text
pre_deploy_inventory -> provision_on_sut -> dry_run_render -> apply
  -> validate_health -> evidence -> rollback
```

The `apply` step is a declared reference and is blocked. The transition from `dry_run_render` to a
real `apply` is not permitted in M2b; a future milestone must obtain separate authorization and
must replace the marker only through an explicit capability change.

Replica counts below are the N=1 baseline declared by the unchanged `deploy/*/compose.yaml` files.
Scaling is environment-driven (`TB_REPLICAS`, `FIWARE_REPLICAS`, `EDGEX_REPLICAS`), but M2b does
not scale anything. A future observation must declare its own N and prove a contiguous replica set.

### Lifecycle, activation and healthcheck declarations

The schema (`deployment-orchestration-plan-v2`) declares three independent properties for every
Compose service. They are read from the unchanged Compose files; M2b neither changes nor reinterprets
the stacks.

| Property | Value | Exact meaning in the current stacks |
|---|---|---|
| `lifecycle` | `persistent` | A long-running service that must remain running for the run. |
| | `one_shot` | A Compose job with `restart: "no"` that is expected to complete and exit. Its successful terminal state is **exit code 0**, not `running`. |
| `activation` | `default_start` | Started by an unqualified Compose invocation, subject only to the existing `depends_on` conditions. |
| | `optional_profile` | Gated behind a Compose profile and **not** started by default. It is `NOT_ASSESSED` unless the run explicitly activates the profile. |
| `healthcheck` | `declared` | The unchanged Compose file declares an explicit `healthcheck`; `healthy` and `unhealthy` are meaningful states. |
| | `none` | The unchanged Compose file declares **no** `healthcheck`. This is a property, not a failed check, and it must never be rendered as `healthy`. |

Evidence predicates:

1. A `one_shot` service passes only on a supplied **successful exit (exit code 0)**. A non-zero exit
   is `FAIL`. A missing exit code is `UNAVAILABLE`; an observed `running` state or a synthetic
   `healthy` value is not completion evidence and is not a pass. A one-shot service is never
   treated as persistent.
2. A `persistent` service passes only while it is `running` with the declared replica count.
3. `healthcheck: declared` plus an observed `unhealthy` is `FAIL`; missing health evidence is
   `UNAVAILABLE`; only an observed `healthy` is `PASS` for that predicate.
4. `healthcheck: none` requires **running plus functional evidence** (for example an attested
   functional probe or an equivalent caller-supplied observation). Missing functional evidence is
   `UNAVAILABLE`. Absence of a healthcheck is never `FAIL` by itself.
5. `activation: optional_profile` and not activated is `NOT_ASSESSED`, not `FAIL`. When explicitly
   activated, the service is evaluated with its declared lifecycle and healthcheck semantics.
6. Where current semantics cannot be established from the repository, the value is `NOT_ASSESSED`
   or `UNAVAILABLE`; no implicit behaviour is invented.

"Failed health" therefore means only: an **explicit healthcheck is present and reports
`unhealthy`**, or a declared functional/running predicate is supplied and contradicts the
expectation. `health: none` on a service without a healthcheck is not a failed health result.

### ThingsBoard — project `tb`

| Service | Tier | Replicas | Lifecycle | Activation | Healthcheck |
|---|---|---:|---|---|---|
| `tb-core` | application | N | persistent | default_start | none |
| `tb-rule-engine` | application | N | persistent | default_start | none |
| `tb-http-transport` | application | N | persistent | default_start | none |
| `zookeeper` | supporting | 1 | persistent | default_start | none |
| `kafka` | supporting | 1 | persistent | default_start | declared |
| `postgres` | supporting | 1 | persistent | default_start | declared |
| `valkey` | supporting | 1 | persistent | default_start | declared |
| `haproxy` | supporting | 1 | persistent | default_start | none |
| `tb-install` | supporting (installer) | 1 | one_shot | optional_profile | none |

- `kafka`, `postgres` and `valkey` must report their explicit healthcheck as `healthy`; `unhealthy`
  is `FAIL` and missing health evidence is `UNAVAILABLE`.
- `tb-core`, `tb-rule-engine`, `tb-http-transport`, `zookeeper` and `haproxy` have no Compose
  healthcheck. They require `running` plus functional evidence, never an unconditional `healthy`.
- `tb-install` is **not default-start**. The unchanged Compose file puts it behind the `install`
  profile with `restart: "no"`; an unqualified Compose project does not start it. It is
  `NOT_ASSESSED` unless explicitly activated. When activated, it is one-shot: exit code 0 is
  `PASS`, a non-zero exit is `FAIL`, and a missing exit code is `UNAVAILABLE`.
- Declared replicas must equal observed contiguous replicas `[0,N)` for persistent default-start
  services. HAProxy stats are read-only evidence only.
- Allowed evidence: descriptor, inventory, compose, `.env`, Makefile, protocol, profile, run
  manifest, monitoring export, log, checksum manifest, packet.
- Required attestation: descriptor, inventory, run manifest, checksum manifest, packet.
- Missing evidence: `UNAVAILABLE` (never a successful zero).

### FIWARE — project `fiware`

| Service | Tier | Replicas | Lifecycle | Activation | Healthcheck |
|---|---|---:|---|---|---|
| `orion` | application | N | persistent | default_start | none |
| `iot-agent` | application | N | persistent | default_start | none |
| `mongo` | supporting | 1 | persistent | default_start | declared |
| `mongo-init` | supporting (one-shot job) | 1 | one_shot | default_start | none |
| `haproxy` | supporting | 1 | persistent | default_start | none |

- `mongo` must report its explicit healthcheck as `healthy`; `unhealthy` is `FAIL` and missing
  health evidence is `UNAVAILABLE`.
- `mongo-init` is a **default-start one-shot job**: the unchanged Compose file sets
  `restart: "no"` and `orion` depends on it with `condition: service_completed_successfully`.
  Successful completion (exit code 0) is required; `running` is not applicable and `healthy` is
  never expected. A non-zero exit is `FAIL`; a missing exit code is `UNAVAILABLE`.
- `orion`, `iot-agent` and `haproxy` have no Compose healthcheck. They require `running` plus
  functional evidence; ingress is through HAProxy only.
- Declared replicas must equal observed contiguous replicas.
- Allowed evidence: descriptor, inventory, compose, `.env`, Makefile, protocol, profile, run
  manifest, monitoring export, log, checksum manifest, packet.
- Required attestation: descriptor, inventory, run manifest, checksum manifest, packet.
- Missing evidence: `NOT_ASSESSED` when the optional live observation is absent; `UNAVAILABLE`
  when declared evidence exists but a required file/field is missing.

### EdgeX — project `edgex`

| Service | Tier | Replicas | Lifecycle | Activation | Healthcheck |
|---|---|---:|---|---|---|
| `core-data` | application | N | persistent | default_start | none |
| `device-rest` | application | N | persistent | default_start | none |
| `database` | supporting | 1 | persistent | default_start | declared |
| `nats` | supporting | 1 | persistent | default_start | declared |
| `core-keeper` | supporting | 1 | persistent | default_start | none |
| `core-common-config-bootstrapper` | supporting (one-shot job) | 1 | one_shot | default_start | none |
| `core-metadata` | supporting | 1 | persistent | default_start | none |
| `haproxy` | supporting | 1 | persistent | default_start | none |
| `app-http-export` | supporting (optional) | 1 | persistent | optional_profile | none |
| `ui` | supporting (optional) | 1 | persistent | optional_profile | none |

- `database` and `nats` must report their explicit healthchecks as `healthy`; `unhealthy` is `FAIL`
  and missing health evidence is `UNAVAILABLE`.
- `core-common-config-bootstrapper` is a **default-start one-shot job**: the unchanged Compose file
  sets `restart: "no"` and its command pushes common config and exits. `core-metadata` and
  `core-data` depend on it with `condition: service_started` (not `service_completed_successfully`),
  so its successful completion must be attested separately and must not be inferred from dependent
  startup. Exit code 0 is `PASS`, a non-zero exit is `FAIL`, and a missing exit code is
  `UNAVAILABLE`. It is never treated as persistent.
- `core-keeper`, `core-metadata`, `core-data`, `device-rest` and `haproxy` have no Compose
  healthcheck. They require `running` plus functional evidence, never an unconditional `healthy`.
- `app-http-export` (profile `export`) and `ui` (profile `ui`) are declared only as
  `optional_profile`. They are not part of the default baseline, are not started by an unqualified
  Compose invocation, and are `NOT_ASSESSED` unless a run explicitly activates the profile. Their
  presence in the declaration does not activate them.
- Declared replicas must equal observed contiguous replicas.
- Allowed evidence: descriptor, inventory, compose, `.env`, Makefile, protocol, profile, run
  manifest, monitoring export, log, checksum manifest, packet.
- Required attestation: descriptor, inventory, run manifest, checksum manifest, packet.
- Missing evidence: `UNAVAILABLE` for a required declared artifact; `NOT_ASSESSED` for an optional
  live observation or an unactivated optional profile.

Across all platforms, `PASS` means "the supplied documents and observations match the supplied
declaration". It does not mean a deployment exists, a container is healthy, a volume exists or a
service is reachable. `healthy` is only meaningful where `healthcheck: declared`; for
`healthcheck: none` the required result is running plus functional evidence.

## 4. Existing stack integration (no new topology)

M2b adds no Compose file, service, network, volume, receiver, ledger or workload adapter. It reuses
the current repository exactly as it exists:

| Existing asset | Role in the blueprint | M2b treatment |
|---|---|---|
| `deploy/thingsboard/compose.yaml`, `deploy/fiware/compose.yaml`, `deploy/edgex/compose.yaml` | Expected project, services, replicas, healthchecks and ports | Read/hash only; never invoked |
| `deploy/*/.env.example` and operator `.env` | Declared versions, CPU sets, memory limits and scaling variables | Read/hash only; never copied or rewritten |
| `deploy/*/Makefile` | Existing target definitions and defaults | Reference only; no target executed |
| `internal/deployment` descriptor/inventory | Approved M2 schema for platform, project, allocation, protocol, provenance and services | Reused unchanged; M2b adds no replacement schema |
| `internal/receipt` | Complete-body receipt semantics for sink records | Reused unchanged; old ledgers stay readable |
| `internal/sink` | Receipt ledger writer and response classification | Reused unchanged; no receiver restart or rewrite |
| `scripts/monitoring.sh` | Existing load-host/exporters commands | Read-only inventory of its paths; never run |
| `scripts/history-payload.sh` | Read-only stored-payload export | Reused unchanged; no database query executed |
| `scripts/adapter-receiver.py` | Controlled receiver for unchanged k6 adapters | Reused unchanged; existing receiver and output remain untouched |
| `workloads/k6/lib/*.js` | Payload, adapter and response-semantics definitions | Reused unchanged; no load executed |

**Labels vs schema.** Compose service names, project names, `.env` variables, Makefile target names
and helper paths are *existing labels and paths*. They are descriptive inputs to the blueprint.
They are not the approved M2 schema and do not silently become new schema fields. The approved M2
contract remains `Descriptor`, `Inventory`, `EvidenceManifest` and the `Observer` interface. The M2b
`PlatformBlueprint` is a declaration layered on top; it does not reinterpret, replace or weaken the
M2 descriptors, and it never turns a label into a live observation.

## 5. Runtime artifact packaging

Artifact layout, taxonomy, hashing and manifest rules are specified separately in
[`runtime-artifact-packaging.md`](runtime-artifact-packaging.md). The blueprint requires:

- a new, never-reused run directory;
- a content-addressed manifest for every new artifact;
- preserved prior receivers and ledgers, byte-identical;
- no self-referential manifest hashing;
- no deletion or in-place rewrite of an old run.

## 6. Rollback and restoration rules

These are rules for a future authorized milestone. M2b does not execute any of them.

1. **Old services and ledgers are preserved.** A new run must not stop, restart, reconfigure,
   truncate or delete an existing service, receiver, ledger or prior results directory.
2. **No stateful data loss.** Rollback must not drop volumes, remove named volumes, delete database
   files or reset storage. A rollback that would lose data is prohibited, not merely discouraged.
3. **No deletes.** `Delete`, `Reset`, `clean`-style volume removal and `down -v` semantics are
   outside the blueprint.
4. **New names only.** New runs use a fresh date+tag+run identifier. Reusing an old run directory,
   receiver port/output, ledger filename or manifest path is prohibited.
5. **Evidence first.** A future authorized apply must capture pre-apply inventory and post-apply
   evidence before any restoration decision. A restoration that occurs before evidence capture
   cannot claim a normal rollback.
6. **Interrupted-before-evidence exception.** If a future run is interrupted before evidence is
   captured, the only permitted outcome is to record the interruption explicitly, preserve whatever
   partial evidence exists, and stop. M2b does not authorize a cleanup or restoration action to make
   the interruption disappear. The exception is a future review decision, not an M2b execution path.
7. **No synthetic recovery claim.** A restored stack must be re-observed and re-attested. A rollback
   is not `PASS` merely because a command returned success.

## 7. Recommended gate transition

A future milestone may transition from blueprint to execution only after all of the following are
recorded and independently reviewed. None of these values may be fabricated; an unavailable value is
`UNAVAILABLE` or `NOT_ASSESSED`, not a placeholder hash.

1. **Protocol binding and profile hashes are unchanged.** The protocol definition remains
   `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`; the D44 profile remains
   `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`. A mismatch blocks the gate.
2. **Descriptor and inventory validate.** Strict schema version 1, exact keys, explicit platform,
   project, allocation, protocol, provenance and complete contiguous replica sets.
3. **Observer is explicitly declared.** No observer is `NOT_ASSESSED`; a file-backed fake is not a
   live observer. A live observer requires a separately reviewed implementation.
4. **Artifact hashes verify.** Every manifest entry re-hashes to its declared SHA-256 and byte count;
   prior artifacts remain byte-identical. Current `ArtifactManifest.Verify` re-hashes only the paths
   declared in the current manifest and does **not** bind a `preserved_prior` artifact to its own
   prior manifest (`PriorManifestVerificationNotImplemented = NOT_IMPLEMENTED`). Until a separately
   reviewed packaging workflow implements that binding, a gate must record prior-manifest
   verification as `NOT_ASSESSED` or `UNAVAILABLE`, not `PASS`.
5. **Blueprints are deterministic.** The orchestration plan and artifact manifest render identically
   from the same inputs and carry canonical hashes.
6. **Non-goals are acknowledged in the packet.** No lifecycle, execution or live claim is implied.
7. **Separate authorization exists.** The future milestone names the exact actions, hosts, run
   identity, stop conditions and rollback authority. An approval of M2b is an approval of this
   blueprint only.

## 8. Machine-readable surface

`internal/deployment/orchestration.go` provides:

- `DecodeOrchestrationPlan`, `OrchestrationPlan.Validate`;
- `OrchestrationPlan.CanonicalJSON`, `CanonicalSHA256`;
- `RenderOrchestrationPlanMarkdown`;
- `IsSafeAction`, `IsBlockedAction` and the stage/action constants;
- `ServiceLifecycle`, `ServiceActivation`, `HealthcheckDeclaration` and the per-service
  `lifecycle`/`activation`/`healthcheck` declaration fields;
- `EvaluateServiceExpectation(e, o)` with the caller-supplied `ServiceObservation` predicate for
  persistent/one-shot, optional-profile and declared/none healthcheck semantics. It is pure and
  offline; it does not collect an observation.

`internal/deployment/packaging.go` provides the artifact manifest contract described in the
packaging document. Neither file adds a CLI execution path, a Docker client or a lifecycle method.
The existing `benchctl deployment-validate` command is unchanged.

## 9. Explicit non-goals

- No Docker client, SUT, platform, sink, adapter or monitoring access.
- No deployment, provision, apply/up/down, scale, restart, stop, fault, load, reset or delete.
- No live health probe, log read, exec or observer implementation.
- No new stack topology, service, receiver, ledger or workload adapter.
- No modification or re-creation of prior M1f/M2 packets, results or ledgers.
- No claim that a blueprint approval is a live-system approval.
