# Synthetic health evidence dry run (M2e)

**Status: offline synthetic dry run only. No Docker client call, SUT access, deployment, lifecycle
action, observer implementation, fault, load or live validation is performed.** This document describes
the M2e `internal/deployment` API. It extends the approved M2, M2b and M2d offline surfaces and adds no
CLI command, executor, lifecycle method or runnable platform command.

## 1. Scope

M2e compares caller-supplied **synthetic** service observations with an already-approved M2d execution
plan. It produces a deterministic report that says whether the supplied observations are internally
consistent with the plan declaration. A `PASS` is never live qualification, reachability evidence,
campaign success or proof that a service exists.

The implementation is `internal/deployment/health_evidence.go`. It decodes documents through the
existing shared strict raw validation (exact case-sensitive keys, no unknown keys, no duplicate keys,
no explicit nulls, no trailing JSON), reuses `ServiceObservation` and `EvaluateServiceExpectation`, and
reuses `ArtifactManifest.Validate` / `ArtifactManifest.Verify` for run identity and artifact checks.

## 2. Synthetic observation document

`SyntheticServiceObservations` is `synthetic-service-observations-v1` at schema version `1`:

| Field | Meaning |
|---|---|
| `schema_version` | Must be `1` |
| `interpretation` | Must be `synthetic-service-observations-v1` |
| `source` | Must begin with `SYNTHETIC M2e` |
| `run_id`, `run_date`, `run_tag` | Declared run identity; validated through the resulting `ArtifactManifest.Validate` |
| `run_tag` prefix | Must begin with `m2e-synthetic` |
| `observations` | Explicit array; an explicit empty array is admitted |

Each observation reuses the approved `ServiceObservation` shape and semantics. Duplicate service names,
invalid service names, negative replica counts and health values outside `healthy`, `unhealthy`, `none`
and `unknown` are rejected. Semantically contradictory but decodable observations are deliberately
admitted: the evaluator reports `FAIL` or `UNAVAILABLE` instead of treating the input as malformed.

The observation service set must be a subset of the plan's expected services. A service that the plan
does not declare is rejected as unknown. Omitted expected services are represented explicitly as
`{service, present: false, health: "unknown"}`; presence, replicas, exit code and functional evidence
are never invented.

Canonical form sorts observations by service. `CanonicalJSON` is compact and order-invariant;
`CanonicalSHA256` hashes those canonical bytes. The raw input identity is the separate exact-byte
SHA-256.

## 3. Evaluation

For each expected plan service, M2e converts the declaration to `ServiceExpectation` and calls only the
existing `EvaluateServiceExpectation`. Inherited semantics include:

- missing `default_start` service: `UNAVAILABLE`;
- absent `optional_profile` service: `NOT_ASSESSED`;
- persistent service: running plus the declared replica count;
- declared healthcheck: `healthy` is `PASS`, `unhealthy` is `FAIL`, missing/`none`/`unknown` is
  `UNAVAILABLE`;
- `healthcheck: none`: running plus functional evidence is `PASS`; missing functional evidence is
  `UNAVAILABLE`, and `none` is not a failed check;
- one-shot service: exit code `0` is `PASS`, non-zero is `FAIL`, and a missing exit code is
  `UNAVAILABLE` even when the observation says running and healthy;
- activated optional services follow the same rules; unactivated optional services remain
  `NOT_ASSESSED`.

`HealthServiceResult` copies the expected tier/health-mode declaration from the execution plan, embeds
the supplied observation and records the recomputed status and reason. Results are sorted by tier and
service. The health aggregate is `FAIL > UNAVAILABLE > NOT_ASSESSED > PASS`.

`HealthEvidenceDryRun.Validate` recomputes every service status and reason with the same predicate and
rejects a tampered status, reason, expected declaration, health aggregate, artifact aggregate, dry-run
aggregate or execution-boundary field.

## 4. Report and packaging

The report is `deployment-health-evidence-dry-run-v1` at schema version `1`, mode
`offline-synthetic-dry-run`, and is prominently labeled `SYNTHETIC M2e — OFFLINE DRY RUN`.

It embeds the canonical M2d execution plan unchanged: the ordered steps are not sorted, the three
execution-bearing rows remain `EXECUTION_BLOCKED_HERE`, and all seven rows remain `NOT EXECUTED`.

Fixed boundaries:

| Field | Value |
|---|---|
| `live_validation` | `NOT_ASSESSED` |
| `observer_status` | `NOT_ASSESSED` |
| `prior_manifest_verification` | `NOT_IMPLEMENTED` |
| `prior_manifest_binding_status` | `NOT_ASSESSED` |
| `dry_run_status` | Aggregate of health and artifact evidence only |

`BuildHealthEvidenceDryRun` is pure and file-free. Its `evidence_status` is `NOT_ASSESSED` because no
file has been verified yet. `WriteHealthEvidenceDryRun` creates a new `results/<run_id>/` directory,
copies the two raw inputs byte-identically, calls the existing `ArtifactManifest.Verify`, records the
actual verification rows, and then writes the final report. A health `FAIL` is a valid report, not a
writer error.

The manifest declares exactly two required `this_run` artifacts and no `preserved_prior` artifact:

| Role | File |
|---|---|
| `orchestration_plan` | `execution-plan.input.json` |
| `other` | `synthetic-service-observations.json` |

The output directory contains:

- `health-evidence-dry-run.json` (pretty JSON, two-space indent, trailing newline);
- `health-evidence-dry-run.canonical.json` (compact canonical JSON, no trailing newline);
- `health-evidence-dry-run.md`;
- `execution-plan.input.json` (exact raw input);
- `synthetic-service-observations.json` (exact raw input);
- `artifact-manifest.json`;
- `SHA256SUMS`.

The manifest excludes itself and `SHA256SUMS`; `SHA256SUMS` excludes itself. Report services are sorted
by tier then service, manifest artifacts and verification rows by path, and observations by service.
No wall-clock value, environment value, absolute working-directory path or map iteration influences the
output.

## 5. Evidence generation

The canonical renders are generated only by an env-gated test helper:

```sh
M2E_CANONICAL_OUT=/path/to/new/dir \
  go test -count=1 -run 'TestM2EWriteCanonicalOutputs' ./internal/deployment
```

The helper writes one `results/<run_id>/` directory per platform fixture. It performs no Docker, SUT,
lifecycle, observer, deployment or load action.

## 6. Non-goals

- No Docker client call, SUT access, deployment, provisioning, apply/up/down, scale, restart, stop,
  fault, load, reset or delete.
- No live observer implementation, health probe, log read or exec.
- No live service claim, campaign result, prior success or qualification claim.
- No fabricated observation, replica, exit code, functional evidence, artifact hash or timestamp.
- No protocol/profile file is read or re-verified; M2e output includes protocol/profile data only if a
  supplied input declaration already contains it, and it makes no claim about it.
