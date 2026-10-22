# Tooling interfaces and backlog

The components the measurement method needs beyond monitoring. Each has its purpose, inputs, outputs,
dependencies and monitoring hooks. "Exists" means implemented in this repository today; everything else is a
documented interface only.

| Component | State |
|---|---|
| Fault fixture | **exists**: `internal/fixture`, `cmd/fixture-target` (5xx, timeout-but-stored, slowdown, 207 batches) |
| Generator (k6 + benchctl) | **exists**: `docs/generator.md`, calibration in `docs/calibration.md` |
| Reconciliation | **exists for the fixture store**: `benchctl reconcile`; platform history readers missing |
| Reference ingester | missing (being built separately) |
| Sink | missing |
| DB timestamp / audit instrumentation | missing |
| Experiment controller | partial: `benchctl run` covers one run; no campaign, reset/readiness or search |
| Offline analysis | partial: per-run health/arrivals; no capacity, CPU-cost or statistical analysis |

## Reference ingester (Go + PostgreSQL)
- **Purpose:** minimal receive / validate / store-history service, used as the CPU-cost reference (O = U_platform / U_reference).
- **Inputs:** the fixture adapter's JSON (`run_id, seq, device_id, scheduled_send_ms, temperature, humidity, pressure`).
  A device credential header if authentication is matched.
- **Outputs:** one history row per logical message, acknowledged only after the configured commit.
- **Dependencies:** PostgreSQL in the supporting tier.
- **Monitoring hooks:** deploy as compose project `reference` with services `ingester` (tier application) and
  `postgres` (supporting), and add `reference` to the cAdvisor keep/tier relabel rules. Its postgres-exporter can
  reuse port 9187 (one stack at a time).

## Sink (Go)
- **Purpose:** receive each platform's outbound copy of a message and record the observation (latency endpoint).
- **Inputs:** HTTP POST from the platform's outbound path (ThingsBoard rule chain REST call, FIWARE subscription,
  EdgeX app-service export) containing `run_id`, `seq`, `scheduled_send_ms`.
- **Outputs:** append-only `sink.ndjson`: `run_id, seq, recv_ms, duplicate_of`; counters for monitoring.
- **Dependencies:** runs on the load host (core 3 is shared with monitoring today: to qualify).
- **Monitoring hooks:**
  - a `/metrics` endpoint with `sink_received_total{run_id}`, `sink_duplicates_total{run_id}` and a bucketed
    `sink_observation_latency_ms` (same edge-in-name convention as the generator, or a native histogram);
  - scrape job `sink` on 127.0.0.1;
  - dashboard group 3's "Scheduled → sink" panel becomes live.
- **Caveat:** may be concurrent with persistence (FIWARE, EdgeX), so it is not a durability timestamp.

## Database timestamp / audit instrumentation
- **Purpose:** history-write execution time and committed-visibility evidence per message.
- **Inputs:** the platform's history tables (ThingsBoard `ts_kv` (6 rows per reading), QuantumLeap table for FIWARE,
  EdgeX core-data readings).
- **Outputs:** per message `(run_id, seq, first_write_ts, rows)`.
- **Caveat:** append-only trigger records roll back with their transaction, so aborted attempts are invisible;
  per-field storage must be mapped back to logical messages.
- **Monitoring hooks:** none live; dashboard group 3's history panel stays "unavailable".

## Reconciliation
- **Exists:** `benchctl reconcile -run DIR -store FILE` for the fixture store. It joins planned, attempted,
  acknowledged and stored identities and reports ack_lost, persisted_despite_{unknown,failure}, duplicates and
  orphans.
- **Missing:** history readers per platform (they produce the same `(run_id, seq)` set) and the sink-observation
  join.
- **Monitoring hooks:** none. Audit results are files, never metrics, so the dashboard's integrity text panel
  points to `reconcile.json`.

## Experiment controller
- **Purpose:** declarative campaign: platform, budget k, topology, profile, repetitions; isolated phases
  reset → readiness → warm-up → measurement → drain → audit → summarize; capacity search; manifests.
- **Inputs:** plans (`workloads/plans`), deployment mapping (`<platform>/.env`), SLO profile (missing).
- **Outputs:** per execution `results/<run_id>/` (exists for generator runs) plus campaign state.
- **Monitoring hooks:**
  - post Grafana annotations per phase (today: one annotation per run via `benchctl -grafana-url`);
  - take CPU **boundary snapshots** at phase edges with the contract below;
  - keep `run_id` the only run label in Prometheus.

### CPU boundary-snapshot contract

For each container in the declared SUT boundary (application + supporting tier of the platform's compose
project), record at measurement start and end:
- container name, full container id, compose service, tier;
- `cpu.stat usage_usec`, snapshot wall time (ns), container `StartedAt`.

`monitoring/exporters/cpu-snapshot.sh <project>` produces exactly these columns (TSV).

Interval CPU = Σ (end − start) / 1e6 over containers present at both edges with the same id and an unchanged
`StartedAt`. If a container restarted or appeared or disappeared, the interval is **incomplete**: report it as such
instead of a cost. Prometheus rate integrals are not a substitute: they were 9-38% low in validation.

## Offline analysis
- **Purpose:**
  - p50/p95/p99 from per-message evidence under the cohort rule;
  - capacity brackets (C_SLO with next fail and step);
  - U, O, E, headroom; run-level intervals;
  - conventional summaries from the same runs.
- **Inputs:** run directories, audits, CPU snapshots.
- **Monitoring hooks:** none (Grafana is not evidence; figures are generated from `results/`).

## Monitoring backlog

| Item | Acceptance |
|---|---|
| Least-privilege PostgreSQL monitoring role (`pg_monitor`) instead of the superuser | exporter works with the role; credentials in `.env`, not compose |
| Kafka broker JMX (optional) | only if a diagnosis needs broker internals |
| Valkey exporter for the ThingsBoard cache (optional) | verified against valkey 8.0 |
| Per-replica JVM / application metrics (optional) | reachable from Prometheus without publishing one port per replica (e.g. a Prometheus agent on the SUT) |
| Repeated, randomized monitoring-overhead pairs | ≥ 5 pairs per configuration, with the difference and its spread reported |
| Qualified cross-host clock offset (e.g. chrony with offset logging, or a ping-pong offset probe between the hosts) | measured offset and uncertainty per run, below the smallest latency budget; today only each host's own timex view is shown |
