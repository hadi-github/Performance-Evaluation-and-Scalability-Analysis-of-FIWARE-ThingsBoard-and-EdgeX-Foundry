# Tooling interfaces and backlog

The components the measurement method needs beyond monitoring. Each has its purpose, inputs, outputs,
dependencies and monitoring hooks. "Exists" means implemented in this repository today; everything else is a
documented interface only.

| Component | State |
|---|---|
| Fault fixture | **exists**: `internal/fixture`, `cmd/fixture-target` (5xx, timeout-but-stored, slowdown, 207 batches) |
| Generator (k6 + benchctl) | **exists**: `docs/methodology/generator.md`, calibration in `docs/methodology/calibration.md` |
| Reconciliation | **exists**: fixture store (`benchctl reconcile`) and EdgeX/ThingsBoard history (`scripts/history-ids.sh`, `benchctl explore -history`); FIWARE has no history (D9) |
| Reference ingester | missing (being built separately) |
| Sink | **exists**: `internal/sink`, `cmd/sink`, run by `scripts/sink.sh` / `scripts/run-with-sink.sh` |
| DB timestamp / audit instrumentation | missing for per-message write timing; the D23 persistence rate uses a cumulative stored counter (`scripts/persist-series.sh`, `benchctl explore -persist`) |
| Experiment controller | partial: `benchctl run` covers one run; no campaign, reset/readiness or search |
| Offline analysis | partial: `benchctl explore` (capacity brackets, outstanding work, D23 persistence), `scripts/demand-fit.py`, `scripts/explore-tables.py`; no statistical analysis or confidence intervals |

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
- **Exists:** `internal/sink`, `cmd/sink`; `scripts/sink.sh` starts it on the load host and
  `scripts/run-with-sink.sh` writes `<run_id>.sink.ndjson`.
- **Purpose:** receive each platform's outbound copy of a message and record the observation (latency endpoint).
- **Inputs:** HTTP POST from the platform's outbound path (ThingsBoard rule chain REST call, FIWARE subscription,
  EdgeX app-service export). Any JSON body is walked for `run_id` and `seq`; NGSI `{"value": ...}` attributes are
  unwrapped and the URL path names the platform (`internal/sink/sink.go:3-8,92-119,141`).
- **Outputs:** append-only NDJSON, one record per observation: `recv_ms, platform, run_id, seq, dup`
  (`internal/sink/sink.go:24-31`; `scripts/run-with-sink.sh:7,12`).
- **Dependencies:** runs on the load host, pinned by default to core 2 (`SINK_CPU`) on port 18090, with an iptables
  rule that admits only the SUT; Prometheus scrapes it on loopback (`scripts/sink.sh:6-8,14-15`;
  `monitoring/prometheus.yml:34-37`). The DIAG sink runs used generator cores 0-1 (`scripts/run-with-sink.sh:13`).
- **Monitoring hooks:**
  - `/metrics` counters with no labels: `sink_requests_total`, `sink_received_total`, `sink_duplicates_total`,
    `sink_unparsed_total`, `sink_overflow_total` (`internal/sink/sink.go:125-129`). Overflow makes the process
    exit non-zero (`cmd/sink/main.go:47-50`).
  - scrape job `sink` on 127.0.0.1 (`monitoring/prometheus.yml:34-37`);
  - dashboard group 3's "Sink observations (per s)" panel plots `rate(sink_received_total)` and
    `rate(sink_duplicates_total)` (`monitoring/grafana/build_dashboards.py:191-193`).
  - observation latency and outstanding work are computed offline from the NDJSON by `benchctl explore -sink`
    (`cmd/benchctl/main.go:95,301-321`), not from Prometheus.
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
- **Exists for EdgeX and ThingsBoard history:** `scripts/history-ids.sh <platform> <run_id>` exports the
  `(run_id, seq)` rows stored in ThingsBoard `ts_kv` or EdgeX `core_data.reading`; `benchctl explore -history FILE`
  runs the same join and writes `history_reconciliation` into `explore.json` (`cmd/benchctl/main.go:97,332-341`).
  FIWARE keeps no history, so its reader exits 3 (D9).
- **Exists:** the sink-observation join: `benchctl explore -sink FILE` reports `planned_reached_sink`,
  `sink_unique`, `sink_duplicates` and `sink_other_run_ids` (`cmd/benchctl/main.go:301-318`;
  `internal/explore/explore.go:46,117-121`).
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

**Compute step (implemented 2026-10-29, `internal/cpubound` + `benchctl cpu`):**

**Automatic bundle capture (2026-10-31):** `benchctl run -cpu-snapshot-command '<trusted command>'`
continuously captures the command's complete snapshot (every 250 ms after the previous call completes;
10 s call timeout). The command normally invokes `cpu-snapshot.sh` on the SUT, over existing
authenticated access. Include **every relevant compose project** in one command; FIWARE requires
both `fiware` and `fiwhc`, not just the baseline. `--name CONTAINER` is available for a standalone
controlled receiver. Do not put credentials in the command: it is recorded in the manifest.

The runner retains `cpu-snapshots.jsonl` (request/response UTC, status, errors, raw TSV), selects
snapshots bracketing each non-warmup scenario using `scenario_start_ms` from the k6 summary, and
writes `cpu-step-NN-before.tsv`, `cpu-step-NN-after.tsv` and `cpu-boundary.json`. It reports the
actual capture interval and boundary skew, **not falsely exact phase CPU**. Counter deltas include
that skew. The automatic denominator is explicitly `attempted HTTP sends in this measured step`;
use `benchctl cpu -audit-file ...` separately for a correctness-qualified denominator after audit.
Cross-host phase alignment requires a qualified clock relationship; the counter delta itself is
on the SUT and does not require cross-host clock subtraction. An omitted capture command or failed
capture produces an explicit `UNAVAILABLE` artifact. Existing historical bundles are never backfilled
with estimated CPU endpoint counters.

```bash
# on the SUT, immediately before and after the measurement interval:
monitoring/exporters/cpu-snapshot.sh <project> > cpu-before.tsv
# ... measurement interval ...
monitoring/exporters/cpu-snapshot.sh <project> >   cpu-after.tsv

# on the analysis host, into the run bundle:
bin/benchctl cpu -before cpu-before.tsv -after cpu-after.tsv \
  [-audit-file <run>/audit.json | -messages N -messages-definition "..." \
     -messages-source "explicit|benchctl audit -history"] -out <run>/cpu-boundary.json -note "..."
```

- `cpu-boundary.json` records per-container before/after `usage_usec`, container id, `StartedAt`, cpuset, the
  interval delta (`cpu_seconds`), the aggregate `total_cpu_seconds`, and — only when all counters are `ok` and a
  denominator is available — `total_cpu_seconds_per_message` with the denominator named (`denominator.source`,
  `denominator.definition`, `denominator.count`).
- Statuses `missing_before`/`missing_after`/`no_counter_before`/`no_counter_after`/`restarted`/`rewound` mark the
  container unmeasured; the aggregate claim is withheld (`counters_ok: false`) and nothing is zero-filled. The
  known-answer tests in `internal/cpubound/cpubound_test.go` cover exact deltas, restart, rewind, missing and NA
  counters.

## Offline analysis
- **Exists:** `benchctl explore` applies the declared pass rule and writes `explore.json` (capacity bracket,
  outstanding work, optional D23 persistence verdict); `scripts/explore-tables.py` renders the per-step tables and
  `scripts/demand-fit.py` fits the regression service demands.
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
