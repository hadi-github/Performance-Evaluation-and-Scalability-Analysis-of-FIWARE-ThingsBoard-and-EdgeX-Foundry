# Monitoring: project state and findings (2026-10-22)

## 1. Purpose and exact scope

The study compares FIWARE, ThingsBoard and EdgeX Foundry on one operation: **receive identifiable sensor telemetry
and retain its history.**
- It measures SLO capacity, its growth with application CPU (k ∈ {1,2,4} cores), and CPU per persisted message.
- The testbed is fixed: SUT ramonavm2 (8 vCPU / 16 GB) with the supporting tier on fixed cores, and load host
  ramonavm1 (4 vCPU / 8 GB).
- This is application-tier scaling on one host, not multi-host scaling.

This task added the **monitoring layer**: Prometheus, Grafana, exporters, recording rules, the k6 stream and the
documentation. **Not in this task:** sink, reference ingester, database audit, capacity controller, statistics.
Their interfaces are in `docs/tooling-interfaces.md`.

## 2. Before / after

**Before:**
- Prometheus + Grafana on ramonavm1 and exporters on ramonavm2 already existed (commit `f00fb8d`, `monitoring/`).
- The dashboard targeted the first-generation `workloads/run.sh` stream.
- No tier or platform labels, no recording rules, no load-host container metrics, and no k6 integration
  for the current generator (`benchctl`).

**Changed (commits `7a8e132` .. this report):**

| Change | Files |
|---|---|
| Labels `platform` / `tier` / `service` / `container` derived from compose labels; raw cAdvisor labels dropped; Grafana + load-host cAdvisor scrapes; 5 s / 4 s | `monitoring/prometheus.yml` |
| 10 recording rules (CPU per container/service/tier/platform, host core busy, k6 sends/drops/outcomes/planned rate/latency buckets), CPU rules gated on `up` | `monitoring/rules/bench.yml` |
| Prometheus `--enable-feature=promql-delayed-name-removal`, retention 30 d / 20 GB, rules mount; load-host cAdvisor | `monitoring/compose.yaml`, `.env.example` |
| Container facts (cpuset, restarts, OOM-killed) via node-exporter textfile + systemd timer | `monitoring/exporters/container-info.sh`, `exporters/compose.yaml` |
| CPU boundary snapshot helper (cgroup v2 `cpu.stat`) | `monitoring/exporters/cpu-snapshot.sh` |
| Primary dashboard rebuilt: 8 groups, gated exporter panels, explicit unavailable panels, run annotations, clock panel | `monitoring/grafana/build_dashboards.py`, `dashboards/iot-benchmark-overview.json` |
| Generator instrumentation (minimal): run labels `run_id, plan_id, adapter`; histogram buckets named by edge (`_le<edge>`); planned-rate gauge | `workloads/k6/gen.js` |
| `benchctl run -prom-rw URL -grafana-url URL` (k6 remote write, run annotation); recorded in the manifest | `internal/runner/runner.go`, `cmd/benchctl` |
| Commands: `make monitoring-{up,check,reload,down}`, `exporters-{up,down,install}`, `dashboards`; `scripts/monitoring.sh` (no make on ramonavm1) | `Makefile`, `scripts/monitoring.sh` |
| Docs: metrics dictionary, tooling interfaces/backlog, monitoring README, this report | `docs/metrics-dictionary.md`, `docs/tooling-interfaces.md`, `monitoring/README.md` |

## 3. Actual versions and topology

| Component | Version / image digest |
|---|---|
| Prometheus | v3.13.4 `sha256:87861b8cf91579109319ebc300f3f1060e6da9c05d6ae8ad15a20c879e84e32e` |
| Grafana | 12.4.13 `sha256:41bbc1b5e04416f81891806076db2a2ff60a5d4da5f78d36a88b033a8c199185` |
| node-exporter | v1.12.0 `sha256:9b0ade5e607f9dbedb0a8e11151b6011ae5bd79304c261804cfdd2cadf200a80` (both hosts) |
| cAdvisor | v0.60.6 `sha256:b8e7d1093144fd088f425ff003d75a4aa405de075db78dae3bc563730b1bd07a` (both hosts) |
| postgres-exporter | v0.20.1 `sha256:ac5ec343104fae0e2d84a27bb8d69b38430a11910c5382cad85d478d2bab713e` |
| kafka-exporter | v1.9.0 `sha256:4150e46b2e9697ac30e4be8de03ec1704b4bdc7037577b83b8d35a32be298c16` |
| mongodb_exporter | 0.52.0 `sha256:12bebeed49da6d31d92e6355f5647585cda48848f1e415eba05f077225751a71` |
| prometheus-nats-exporter | 0.20.1 `sha256:4fbf6dacb84780a45a1c3af9b1080c69451a288d20902deae671b80717bb8f61` |
| k6 | v1.8.1 `sha256:23f2279054c3e01535455c4d92914a625df12aeb631ea0e3ea7a43f96bcbc843` |
| Runtime | Docker 29.9.1, cgroup v2 (systemd driver), both hosts |

**Platforms** (images and digests in the SUT inventory; one stack at a time):
- **ThingsBoard 4.3.1.6:** `tb-core`, `tb-rule-engine`, `tb-http-transport` (application tier) + PostgreSQL 16,
  Kafka 4.0.0, ZooKeeper 3.8.1, Valkey 8.0, HAProxy 3.2.
- **FIWARE:** Orion 4.5.0 + IoT Agent UL 3.7.0 (application) + MongoDB 8.0, HAProxy. **No QuantumLeap / history
  backend is deployed**, so FIWARE cannot yet meet the "retain history" requirement.
- **EdgeX 4.0.2:** NATS-enabled `core-data` + `device-rest` (application) + PostgreSQL 16.3, NATS 2.12.9 (core,
  no JetStream), core-keeper, core-metadata, HAProxy.

**Core mapping** (observed via `bench_container_info`, EdgeX example):

| Cores | Contents |
|---|---|
| 0 | OS + exporters |
| 1 | HAProxy |
| 2-3 | supporting services |
| 4.. | application pool |

- Here `core-data` and `device-rest` are on cpuset 4 (k = 1).
- Containers have cpusets and memory limits but **no CFS quotas**, so throttling metrics do not exist.
- The application budget k and the replica count are still tied in the stacks (`scripts/resources.mk`: k = N).
  The spec wants them separate; this task did not change platform budgets.

**Mismatches with the earlier notes:**
- FIWARE history backend absent (QuantumLeap/TimescaleDB is planned in `research/notes/decisions.md`).
- The load host is planned as generator 2 / sink 1 / monitoring 1 cores. Calibration needs **3 generator cores**
  with `GOGC=400` for > 1,000 msg/s (`docs/calibration.md`), which leaves core 3 for both sink and monitoring.
  This is not yet measured with a sink.

## 4. Flows

```
k6 (ramonavm1) ──HTTP──► platform HAProxy ──► application tier ──► broker/DB (ramonavm2)
   │ per-message non-success records, exact counters/histograms ──► results/runs/<run_id>/ (evidence; offline)
   └ remote write (optional) ──► Prometheus ◄── scrape: node, cAdvisor (both hosts), HAProxy, DB/broker exporters
                                    └──► Grafana (display only)
future: platform outbound ──► sink (ramonavm1) ──► sink.ndjson ; history tables ──► audit readers ──► reconcile
```

- **Prometheus collects** host, container, HAProxy, DB/broker and k6-stream series.
- **Grafana computes** rates and aggregations for display only.
- **Offline / unimplemented:** run verdicts (`benchctl check`), reconciliation (`benchctl reconcile`, fixture store
  only), sink observation, history-write timing, CPU per persisted message, capacity and statistics.

## 5. Connected sources and verification

| Source | Covers | Credentials / permissions | Verification |
|---|---|---|---|
| node-exporter (both hosts) | host CPU/disk/network/fs, timex clock view, textfile facts on SUT | host `/` read-only, pid host | live |
| cAdvisor (both hosts) | every container's CPU/memory/network/OOM/start time | privileged, read-only mounts | live: counters vs kernel (§7) |
| HAProxy native `/metrics` | per backend and per replica server | none (port firewalled to ramonavm1) | live (EdgeX, FIWARE) |
| postgres-exporter | ThingsBoard / EdgeX databases | platform `postgres` superuser (gap) | live (EdgeX, ThingsBoard) |
| kafka-exporter | ThingsBoard consumer-group lag | none (PLAINTEXT) | live (§6a) |
| mongodb_exporter | FIWARE MongoDB ops/connections | none (no auth on the stack's MongoDB) | live (FIWARE smoke: insert/update/query/command rates) |
| prometheus-nats-exporter | EdgeX core NATS varz/connz/subz | none | live (incl. outage test) |
| k6 remote write | generator stream | Prometheus remote-write receiver on 127.0.0.1 only | live (runs A, B) |

Not connected (documented gaps): Kafka broker JMX, Valkey, JVM/application metrics per replica, Mosquitto (not
deployed), database write timing, sink.

## 6. Validation runs (all on 2026-10-22, UTC)

| # | Workload / configuration | Interval | Observed | Result |
|---|---|---|---|---|
| A | EdgeX N=1, 100 msg/s × 120 s, 100 devices, `edgex-monitoring-20261022t075814z-f896`, k6 3 cores, `GOGC=400`, `-prom-rw`, annotation | 07:58:16-08:00:22 + idle before/after | Platform CPU rose from ~0.10 cores idle; Prometheus counts = run counts (12000 sends, 12000 ack) | **live** |
| A-CPU | Kernel `cpu.stat` snapshots vs Prometheus over 134.4 s | same | kernel 83.20 CPU-s; cAdvisor counter diff 82.54 (−0.8%); integral of the 30 s rate 75.91 (−8.8%); same with a 1 m window −18%, 2 m −38% | **live**: counter diffs are exact enough; rate integrals are not accounting |
| B | Fixture (ramonavm2, cores 4-7) with 503 every 40th seq and 3 s delay every 97th, 50 msg/s × 60 s, 2 s timeout, `fixture-faults-monitoring-20261022t080309z-e3e4` | 08:03 | expected 2895/75/30: raw counters 2895/75/30; HTTP status 200/503/0 = 2895/75/30; generator verdict VALID (request failures ≠ run failure) | **live** |
| B-rules | Outcome / bucket recording rules | B | Before the feature flag: `bench:k6_outcome` had **no outcome label** (all outcomes summed); fixed with `promql-delayed-name-removal`. After: correct labels; integrals of rule rates ~5% below exact counts | **live**, bug found and fixed |
| C | Stop `nats-exporter` | 08:04:56 | `up{job="nats"}`=0 within 3 s; gauge panel → no data; **rate panel showed 0** (old samples inside the window) → all exporter panels now gated on `up == 1` | **live**, bug found and fixed |
| D | `docker restart` and `--force-recreate` of `edgex-core-data-1` | 08:05-08:07 | rate never negative (min 0.0005 cores); 1 series per container at any instant (no collision after dropping `id`); `changes(container_start_time_seconds)` = 2; 0 duplicate/out-of-order samples. Docker `RestartCount` stayed 0 → panel switched to cAdvisor start times | **live** |
| E | Overhead pair: EdgeX 200 msg/s × 60 s without / with `-prom-rw` | 08:08-08:11 | generator cores 0-2 mean busy 16.5/16.5/13.6% → 18.2/17.6/14.7%; k6 memory 101 → 108 MB; send lag p99 ≤ 2 ms both; Prometheus 0.028 cores both; SUT platform CPU 0.871 vs 0.892 cores (kernel; the stream is load-host-only, so this is run-to-run noise) | **indicative** (one pair) |
| F | Every dashboard query (platform = edgex, last hour) | 08:12 | all 47 queries valid; all EdgeX-relevant ones return series. Empty: dropped iterations, early sends (none occurred), Kafka, MongoDB (platform not running) | **live** |
| G | FIWARE N=1 + exporters, `fiware-smoke-20261022t081247z-27e3` (10 msg/s × 20 s, streamed) | 08:12-08:13 | mongodb target up; tiers application 0.16 / supporting 0.32 cores; MongoDB op rates by type | **live** |
| H | ThingsBoard N=1 + exporters, `thingsboard-smoke-20261022t082339z-0fd2` (10 msg/s × 20 s, streamed) | 08:23 | kafka + postgres targets up; consumer-group lag per group/topic (all 0); PostgreSQL ~25 inserted rows/s; tiers application 0.16-0.24 cores per service, **Kafka 0.92 cores** | **live** (see §6a) |

### 6a. ThingsBoard

**Monitoring sources:**
- kafka-exporter: `kafka_consumergroup_lag` per consumer group (`tb-core-node`, `re-Main-consumer-*`,
  `tb-core-transport-api-consumer`, ...) and topic.
- postgres-exporter on the `thingsboard` database.
- cAdvisor tiers.

All verified live. At 10 msg/s Kafka alone used 0.92 cores of the supporting tier, the largest consumer, which
matters for ThingsBoard's CPU accounting.

**Platform readiness, a finding for the future controller (not a monitoring defect):** the stack's own smoke test
ran as soon as HAProxy's TCP health checks passed and got 200 non-200 responses (0/200 stored). A `benchctl` run
right afterwards was VALID with 200/200 acknowledged, but ack p99 ≤ 5 s while ThingsBoard warmed up. A TCP check is
not readiness. The controller's readiness phase needs an application-level probe (e.g. N accepted telemetry posts)
before warm-up.

## 7. CPU accounting status

- **Source:** cAdvisor `container_cpu_usage_seconds_total` = cgroup v2 `usage_usec` / 1e6.
  - One series per container (`--docker_only`, no parent cgroups), so sums over containers do not double count.
  - Restarts are handled by `rate()` (resets) and visible via `container_start_time_seconds`.
- **Interval accounting:** counter differences at the boundaries agree with the kernel within 0.8% (platform total,
  134 s; per container within 2%, limited by the 5 s scrape offset). The exact method is
  `cpu-snapshot.sh`-style boundary snapshots. The contract is in `docs/tooling-interfaces.md`.
- **Not accounting:** Prometheus rate integrals were 9-38% low depending on the window.
- **CPU per persisted message cannot be calculated correctly yet:** there is no audited persisted-message cohort
  (no history readers; FIWARE has no history) and no controller aligning the CPU interval with the cohort. Never
  substitute HTTP successes or database rows.

## 8. Dashboard

- **Where:** `http://87.107.191.32:3000` (admin password in `monitoring/.env` on ramonavm1), home dashboard
  "IoT benchmark overview".
- **Use:** pick a platform, then a run (k6 stream), then the time range from the run annotation.
- **Interpretation notes** are in each panel's description.
- **Limitations:** operational quantiles are per-interval estimates; final percentiles come from run artifacts.
  Unavailable sources are text panels, not zeros. Panels are gated on target health.
- **Reproduce:** `make dashboards` (workstation), `scripts/monitoring.sh up|check` (ramonavm1),
  `make exporters-up P=<platform>` (ramonavm2).

## 9. Monitoring overhead

One matched pair (§6 E): the k6 stream costs about 1-2 points of CPU per generator core at 200 msg/s and no visible
send lag. Prometheus uses ~0.03 cores. **Not qualified:** that needs ≥ 5 randomized pairs per configuration,
including SUT exporter on/off pairs, and a sink on the monitoring core.

## 10. Open scientific choices (unchanged by this task)

- **The shared latency endpoint** (sink observation vs history write).
- **Durations** (warm-up, measurement, drain).
- **Backlog thresholds**: core NATS has no queue depth, and Kafka lag is only per group.
- **Device-population scope; repetition and confirmation rule; capacity-search step and criteria.**
- **Maximum planned rate** (decides whether k6 on this host is sufficient: `docs/generator-status.md`).

## 11. Next work

| Priority | Item | Acceptance |
|---|---|---|
| Monitoring fixes | Least-privilege PostgreSQL role for the exporter | exporter works with `pg_monitor`; no superuser in configs |
| | Repeated overhead pairs (stream on/off, exporters on/off) | ≥ 5 pairs, difference and spread reported |
| | Qualified cross-host clock offset | per-run offset and uncertainty below the smallest latency budget |
| Harness (later) | Sink + its `/metrics`; history readers per platform; QuantumLeap for FIWARE | sink panel live; reconciliation against each platform's history |
| | Controller with CPU boundary snapshots per phase | interval CPU from snapshots; incomplete intervals flagged |
| | Separate k from replica count in the stacks | plan/deployment mapping fields; verified cpusets per k |

## 12. Status

| Component | Implemented | Live-tested | Evidence | Remaining blocker |
|---|---|---|---|---|
| Prometheus config + 10 rules | yes | yes | promtool; §6 A-F | — |
| Grafana dashboard (8 groups) | yes | queries yes (§6 F); visual check not done in a browser | query validation | none |
| node-exporter, cAdvisor (both hosts) | yes | yes | §6 A, D, §7 | — |
| HAProxy, NATS, postgres (EdgeX) exporters | yes | yes | §6 A, C, F | — |
| mongodb exporter (FIWARE) | yes | yes | §6 G | — |
| kafka + postgres exporters (ThingsBoard) | yes | yes | §6 H, §6a | readiness probe for the controller |
| k6 remote write + annotations | yes | yes | §6 A, B, E | — |
| Container facts (cpuset, OOM) | yes | yes | §2, §6 D | Docker restart count is policy-only (documented) |
| CPU boundary snapshot | helper + contract | yes (manual) | §6 A-CPU | controller integration |
| Sink / history-write / audit panels | placeholders | n/a | — | producers not implemented |
| Overhead qualification | — | one pair | §6 E | repeated pairs |

**How far it got:**
- The monitoring layer is implemented, deployed on both hosts and validated with real traffic on EdgeX and FIWARE
  and ThingsBoard.
- Validation found and fixed three defects that would have produced wrong charts: merged outcomes, zero instead of
  no data during an outage, and restarts not counted.
- It does **not** validate any benchmark result, and no capacity or CPU-cost figure exists yet.
