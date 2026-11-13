# M1f bounded offline fault target, manual event and recovery/capacity foundation — 2026-11-04

**Ready for independent DeepSeek Pro review. Offline code and tests only; no live/Docker, SUT,
deployment, fault execution, stop command or campaign qualification is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`); it does not expose a separate
effort control. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. Pre-edit baseline packet:
`results/m1f-baseline-20261105-013100+0330/`. Review packet:
`results/m1f-fault-capacity-20261105-013824+0330/`. No commit or push was made.

This packet supplements the preserved M0b/M1a/M1b/M1c/M1d/M1e packets and does not replace them.
It does not modify retained evidence, `docs/status.md`, the protocol definition, the D44 profile
policy, sink/receiver contracts, existing analysis claims or any metric threshold.

## What changed

| File | Purpose |
|---|---|
| `internal/analysis/fault_target.go` | Versioned allowlist and pure dry-run resolver for exactly one application replica from a supplied inventory. |
| `internal/analysis/fault_event.go` | Versioned manual fault-event schema, ordering/identity validation and stop/rejoin/topology reduction. |
| `internal/analysis/capacity.go` | Explicit-window recovery and capacity-bound reducer with a versioned predicate and search policy. |
| `internal/analysis/fault_capacity.go` | Versioned combined evidence document, reduction entry point, deterministic JSON/Markdown writer. |
| `internal/analysis/types.go` | Optional `fault_target`, `fault_event`, `fault_capacity` fields on the existing `Result`; left nil by `Analyze`. |
| `internal/analysis/output.go` | Renders those optional fields only when a caller attached them; no default capacity claim. |
| `cmd/benchctl/analysis.go`, `cmd/benchctl/main.go` | New `fault-capacity -in FILE -out NEWDIR` command in the existing offline command path. |
| `internal/analysis/fault_capacity_test.go` | Hand-computable resolver, event, capacity, recovery, strict-decode and serialization tests. |
| `docs/development/offline-analysis.md` | Documents the new offline schema, command and preserved limits. |
| `docs/archive/sessions/m1f-fault-capacity-20261104.md` | This packet. |

No Docker client call, container stop, fault scheduler, fault-execution loop or unattended target
selection exists. The resolver is a pure function of a caller-supplied inventory; the reducer is a
pure function of caller-supplied event/window documents.

## 1. Fault target resolver

The only admitted platform/service mappings are:

| Platform | Compose service |
|---|---|
| `thingsboard` | `tb-http-transport` |
| `fiware` | `iot-agent` |
| `edgex` | `device-rest` |

Stateful stores, HAProxy, monitoring exporters and whole-stack selectors are outside the
allowlist. `ResolveFaultTarget` requires a dry-run request and verifies the supplied project,
service, replica index and N, container id, image, started-at, healthy flag and every supplied
expected label. It rejects missing, ambiguous, cross-project, duplicate, unhealthy, out-of-range,
unexpected-id and non-dry-run requests. Refusal reasons are sorted and de-duplicated; a target
identity is emitted only when every supplied assertion agrees with exactly one container. The
output contains no stop command and no scheduling field.

## 2. Versioned manual fault event and reducer

`FaultEvent` schema version 1 carries: `run_id`, `protocol_binding_sha256`, platform, project,
service, container id/image, `replica`/`replicas`, `service_replicas`, stop request/completion,
signal/grace/forced, observed unavailability start/end, LB removal, restart request, healthy
rejoin, unexpected transitions, offered rate and window, clock domain/source, `event_source`
(`operator/manual`) and `dry_run`.

`ReduceFaultEvent` validates identity, allowlist, N/replica consistency, protocol-hash shape,
window containment, signal/grace/forced consistency and causal ordering where endpoints exist.
Any unexpected transition is invalid and visible. A missing stop completion, clock, LB removal or
restart attribution is `UNAVAILABLE`; a missing unavailability end or healthy rejoin is
`RIGHT_CENSORED`. A dry-run event is `DRY_RUN` and can never qualify. No timestamp, readiness or
duration is fabricated: durations appear only when both endpoints were explicitly supplied.

The reducer emits separate transient stop and rejoin observations plus a declared degraded
topology reference. The reference is computed from N and the single intended replica, not from the
transient interval; `degradation_measured=false` and `inferred_from_transient_window=false` are
explicit. `N=1` is labelled `N=1_service_outage` separately from `replica_degradation`.

## 3. Recovery and capacity reducer

`CapacityInput` schema version 1 requires an explicit versioned predicate (`p99_max_ms`,
`attainment_min_ratio`, `missing_max`, `achieved_rate_min_ratio`, optional `backlog_max`,
`min_samples`), an explicit search policy (`version`, `monotonicity_assumed`,
`min_confirmations`, `source`) and predeclared windows. Each window supplies scheduled, observed,
missing, p99, attainment, achieved rate, backlog, sample count and explicit fault/recovery
classification (`none`, `failover`, `rejoin`, `not_recovered`).

No default threshold, hybrid search rule, 5-of-5 rule or fabricated sample exists. No-fault windows
can establish a capacity point; fault windows are recovery observations only. Failover and rejoin
are separate. Non-recovery, missing recovery time, recovery outside the declared window and
insufficient samples are right-censored or invalid with `exact=false`. Invalid points are visible
and cannot establish bounds. The status is exactly one of `bracketed`, `lower_bound_only`,
`no_passing_rate` or `unresolved`; the result carries lower/upper bounds, `bound_evidence`,
`stop_reason`, per-rate confirmation counts, visible contradictions and a recovery summary. An
all-pass set yields a lower bound only. Fault/no-fault separation means transient unavailability
cannot create capacity evidence.

## 4. Tests

`fault_capacity_test.go` covers, with hand-computable values:

- allowlisted success for ThingsBoard `tb-http-transport`, FIWARE `iot-agent` and EdgeX
  `device-rest`;
- wrong project, stateful service, HAProxy, monitoring, ambiguous, missing, duplicate id,
  unexpected id, invalid N, out-of-range replica, non-dry-run, cross-project, unhealthy, missing
  started-at, label mismatch and image mismatch refusals;
- one-replica stop/rejoin qualification, N=1 outage labelling, unexpected extra transition
  invalidity, dry-run non-qualification, ordering violation and missing-stage
  unavailable/right-censored outcomes;
- bracketed bounds, all-pass lower bound, invalid-upper rejection, contradictory repeat visibility,
  no-passing-rate, zero denominator, insufficient samples, right-censored recovery, non-recovery
  and fault/no-fault separation;
- explicit observed-violation flags, censored `exact=false`, deterministic JSON/Markdown, unknown
  field/null/duplicate-key/trailing-JSON rejection and new-output-directory enforcement.

## 5. Validation

See `command-*.log`, `command-*.exit`, `incremental.patch`, `final-files.sha256` and
`evidence-manifest.json` in the review packet. The recorded commands are:

```sh
go test -count=1 ./internal/analysis ./cmd/benchctl
go test -short -count=1 ./cmd/... ./internal/... ./tests/...
go test -race -count=1 ./internal/analysis
go vet ./internal/analysis ./cmd/benchctl
git diff --check
gofmt -l <changed Go files>          # empty output
go build ./cmd/benchctl
sha256sum research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json \
  research/protocols/offline-profiles-20261101.json
go run ./cmd/benchctl fault-capacity -in <packet>/cli-evidence.json -out <new dir>
```

`command-02-scoped` intentionally uses the scoped paths named in the task, not unrestricted
`go test ./...`; Docker-backed integration is skipped by `-short`, and retained source snapshots
under `results/` are known to recurse. No claim is made that unrestricted `go test ./...` passes.
The CLI smoke run writes `fault-capacity.json`, `fault-capacity.md` and `SHA256SUMS`; its Markdown
states `Campaign capacity: **NOT CLAIMED**`, target eligible, event qualified, recovery observed
and capacity bracketed at 20/40 messages/s from the explicit synthetic windows.

## 6. Integrity and preservation

- HEAD remains `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`; branch `chore/ci-and-release`. No commit
  or push occurred.
- Protocol definition SHA-256 remains
  `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`.
- D44 profile SHA-256 remains
  `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`.
- Retained `results/` files excluding `results/m1f-*` hash identically before and after:
  `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec` (2,136 files).
- Pre-edit source snapshot (`351` files outside `.git`/`results`):
  `70cfc5b7d4f53911e24d414655271c350c823a13a5fa781c44fce47696e4d560`.
- `incremental.patch` is generated against a reconstructed pre-edit baseline for the five modified
  files; its pre-edit copies were verified byte-identical to the pre-edit source hashes. It
  round-trips to the final files.
- `docs/status.md` was not edited.

## Preserved limits

No live Docker, SUT, platform, sink, adapter, deployment, stop command, fault execution, recovery
observation, runtime capture, clock qualification, storage proof or load was exercised. The target
allowlist is an engineering policy, not evidence that ingress topologies are identical across
platforms. Manual event and window documents are operator attestations; the reducers do not
independently observe them. Capacity bounds are descriptive observations under an explicitly
supplied predicate/search policy and monotonicity assumption, not a campaign C_SLO. The existing
working tree contains pre-existing M1a–M1e uncommitted work and real dirty git metadata, so run
provenance/comparability remains `NOT_ASSESSED` by design.

**Stop for independent DeepSeek Pro review.**
