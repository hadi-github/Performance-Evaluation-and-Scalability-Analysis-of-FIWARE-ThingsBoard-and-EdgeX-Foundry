# Deployment/instrumentation foundation (M2)

This document describes the bounded M2 package `internal/deployment` and the offline
`benchctl deployment-validate` command. M2 is a schema, validator and observer foundation with a
dry-run renderer. It does not deploy, start, stop, restart, exec, fault, probe or load anything.

## Non-goals and lifecycle exclusion

There is no Docker client, SUT client, deployment implementation, `apply`/`up`/`down`/`restart`/
`stop`/`fault` action, container exec, log action, health probe or campaign action. The only
observer implementation is a fake that reads one explicitly supplied JSON file. No live
observation is implemented in M2, so every report carries `live_validation: "NOT_ASSESSED"`.

## Versioned documents

All documents use `schema_version: 1`. Every object, including nested objects and objects inside
arrays, is validated against its exact case-sensitive JSON key set before typed decoding. Unknown
or case-variant keys, duplicate JSON keys at any nesting level, explicit nulls, trailing JSON,
missing required fields and malformed values are rejected. `encoding/json`'s case-insensitive
field matching and `DisallowUnknownFields` are not relied on as the only check.

### Deployment descriptor

Required top-level keys:

- `schema_version`
- `platform` — one of `thingsboard`, `fiware`, `edgex`
- `project` — Compose project name
- `allocation.application_cpusets` and `allocation.supporting_cpusets`
- `protocol.binding_sha256` and `protocol.profile_sha256`; optional `protocol.profile_path`
- `provenance.source`; optional `compose_path`/`compose_sha256`, documents and caller timestamp
- `services` — non-empty array

Each service requires `service`, `class` (`application` or `supporting`), positive `replicas`,
`cpuset`, positive `memory_bytes`, and may declare `image` and `digest`. A service cpuset must be
a subset of the CPU sets declared for its class. Optional top-level
`effective_configuration.sha256` and `.path` are caller attestations.

### Observed inventory

Required top-level keys are `schema_version`, `project`, `source` and `services`. Optional
`captured_at_ms` and `effective_configuration` are caller-supplied only. Each service requires
`project`, `service`, positive `replicas`, optional `cpuset` and a non-empty `containers` array.
Every container requires `project`, `service`, `container_id`, `replica`, `image`, explicit
`started_at_ms`, `health` and `state`. A non-healthy or non-running container must carry
`health_reason` or `state_reason`.

Decoding rejects duplicate services, duplicate container IDs, duplicate or incomplete replica
sets, declared/actual replica mismatches, cross-project rows, empty or unexpected states, and
unreasoned unhealthy/non-running states. The validator additionally refuses a different
project, unknown observed services, missing required services, health/state failures, image or
digest mismatches, cpuset mismatches, effective-configuration mismatches and non-contiguous
observed topology.

### Result states

The report uses explicit `PASS`, `FAIL`, `UNAVAILABLE` and `NOT_ASSESSED` states. A missing
observer is `NOT_ASSESSED`, not a zero count. A declared digest or cpuset that is absent from the
inventory is `UNAVAILABLE`, not a match. `PASS` means the supplied inventory matches the supplied
descriptor; it does not mean a deployment exists.

### Evidence manifest

`EvidenceManifest` (schema version 1) lists explicit `role`, `path` and `sha256` documents and
contains no timestamp unless a caller supplies `captured_at_ms`. Verification re-hashes the
listed documents read-only. The CLI generates one from the descriptor, inventory and optional
fake-observer document, or verifies and preserves a caller-supplied manifest.

## CLI

```sh
go run ./cmd/benchctl deployment-validate \
  -descriptor descriptor.json \
  -inventory inventory.json \
  [-observer-fake observer-inventory.json] \
  [-evidence-manifest evidence-manifest.json] \
  -out NEW_DIRECTORY
```

The command reads only the named files and writes `deployment-validation.json`,
`deployment-validation.md`, `evidence-manifest.json` and `SHA256SUMS` inside the required new
output directory. It exits `0` only for a `PASS` attestation, `1` for a non-`PASS` attestation and
`2` for usage/input errors.
