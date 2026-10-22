# Exploratory capacity check: do the platforms exceed 500 msg/s?

Status: **exploratory, 2026-10-22. Not paper results**; it chooses no thresholds. Run ids `EXPL-*`, raw data in
`results/exploratory/` on ramonavm1 (copy on the workstation). Question: k6 is qualified to ~500 msg/s on 2 cores
(`docs/calibration.md`); if any platform sustains more, a Go generator is required.

## Decision

**A Go generator is REQUIRED** by the declared rule: EdgeX at k = 4 passed the 500 msg/s step cleanly (and 700),
and its utilization-law estimate at the highest passing step exceeds 500.

Read with three caveats, all reported rather than re-evaluated (the rules were fixed before the data):

1. **EdgeX is observed upstream of persistence.** Its export service subscribes to the event stream on NATS in
   parallel with core-data, so the sink sees a message before (or without) it being stored. Persistence saturated at
   **~400 readings/s at k = 4** and **~135/s at k = 1** (Postgres insert rate, table below) while the sink-based rule
   passed 500/700 (k = 4) and 200/350 (k = 1). Every message of the passing steps was stored in the end (backlog
   drained within ~7 min after the k = 4 run). ThingsBoard (REST node after Save Timeseries) and FIWARE (Orion
   notification after the entity update) are observed after persistence. Judged by persistence, EdgeX k = 4 would fail
   between 350 and 500, every platform would fail below 500, and the decision would read "k6 is sufficient". **This
   is the main open question for the user.**
2. **The generator itself reached its limit on this host:** at 1000 msg/s k6 grew to 5.9 GB on the 8 GB load host,
   the host swapped (77 major faults/s) and the step went INVALID (174 drops, send-lag p99 456 ms). The 2-minute
   pre-flight at 1000/s (2.7 GB) did not show this. This argues for a Go generator independently of the platforms.
3. **The EdgeX export service is a large observer cost:** ~1.4-2.0 ms CPU per message, more than core-data plus
   device-rest; at k = 1 it took 43 % of the single pool core and was OOM-killed at 500/s.

## Results

| Platform | k | Highest passing | First failing | Failure reason | Smallest B_i at highest pass (container) | Pool bound (labelled) | Generator health | Verdict |
|---|---|---|---|---|---|---|---|---|
| EdgeX | 4 | **700** | 1000: INVALID (generator) | k6 host memory exhausted (5.9 GB, swap): 174 drops, send-lag p99 456 ms | 2221 msg/s (edgex-database-1) | 939 (cpuset 4-7) | OK at 50-700 (lag p99 ≤ 8.3 ms, 0 drops); INVALID at 1000 | **exceeds 500 msg/s** |
| EdgeX | 1 | 350 | 500: FAIL | outstanding work grows (4812 → 19236 > 4812 + 500); export service OOM-killed, core-data NATS slow-consumer drops | 925 msg/s (edgex-app-http-export-1) | 392 (cpuset 4) | OK at every step (lag p99 ≤ 2.6 ms, 0 drops) | **unclear** (failed below 500, but smallest B_i > 500) |
| ThingsBoard | 4 (N = 1, pool 4-7) | 200 | 350: FAIL | outstanding work grows (450 → 1209 > 450 + 350); no restart/OOM during the step | 404 msg/s (tb-kafka-1) | 222 (cpuset 2-3) | OK at 50-500 (lag p99 ≤ 8 ms, 0 drops) | within k6 range |
| FIWARE-no-history | 4 | 100 | 200: FAIL | ack p99 > 1 s in every minute (1235/1768/1579 ms); no restart/OOM during the step | 149 msg/s (fiware-mongo-1) | 125 (cpuset 4-7) | OK at 50-200 (lag p99 ≤ 3 ms, 0 drops) | within k6 range |

k = 1 was run only where k = 4 passed 500 (EdgeX). Per-step generator health, per-minute ack p99 and outstanding work
for every step are in the appendix; every number there is copied from `explore.json` by `scripts/explore-tables.py`.

**Reconciliation (passing steps, by unique id, after the platform drained):** every planned message of every passing
step was stored and reached the sink, 0 duplicates: EdgeX k = 4 steps 0-6 (348,000), EdgeX k = 1 steps 0-4 (132,000),
ThingsBoard steps 0-3 (69,000). FIWARE-no-history keeps no history (sink only: all 33,000 of steps 0-2 reached it).
The whole-run reconciliation in `explore.json` is refused for runs with drops in later steps (identities inexact
there); the per-step figures come from the same files (`<run>.history.ndjson`, `<run>.sink.ndjson`, `resolved.json`).

**Utilization-law estimates are optimistic for EdgeX and pessimistic at low rates in general.** D_i falls with the
rate (fixed background CPU: Kafka ~0.9 cores idle, Mongo, NATS), so B_i from low steps underestimates; and for EdgeX
at ≥ 500 msg/s core-data persisted less than it acknowledged, so its D_i is per acknowledged, not per persisted,
message. The EdgeX persistence ceiling is not a CPU limit: at 500 and 700 msg/s each core-data replica sat at 0.27
cores and the database at 0.63 cores, with spare cores in both pools.

## EdgeX persistence rate (Postgres `core_data.reading` inserts/s, postgres-exporter, 1-minute rate)

| k | 50 | 100 | 200 | 350 | 500 | 700 | 1000 | after the run |
|---|---|---|---|---|---|---|---|---|
| 4 | 50 | 100-102 | 199-203 | 351-352 | ~402 | 385-391 | 372-396 | ~460 until 13:51 (528,000 stored, nothing lost) |
| 1 | 50 | 97-101 | 132-138 | 123-125 | 128-138 | | | ~155-160 until 14:22 (214,812 stored: 7,188 lost, all in the 500 step) |

## Setup (provisional mapping)

| | |
|---|---|
| Load host | ramonavm1, 4 vCPU / 8 GB: k6 v1.8.1 on cores 0-1 (`GOGC=400`), Go sink on core 2, Prometheus/cAdvisor/node-exporter on core 3, Grafana stopped |
| SUT | ramonavm2, 8 vCPU / 16 GB: core 0 OS + exporters + CPU snapshot loop; core 1 HAProxy; cores 2-3 stateful tier (DB, broker, cache); pool 4-7 at k = 4, core 4 at k = 1 |
| EdgeX k = 4 | N = 4 replicas of device-rest and core-data + 1 app-service http-export, all on 4-7; core-metadata/keeper/NATS/Postgres on 2-3 |
| ThingsBoard k = 4 | N = 1 of tb-core, tb-rule-engine, tb-http-transport on the 4-core pool 4-7 (4 replicas do not fit memory: D11); Kafka, ZooKeeper, Postgres, Valkey on 2-3 |
| FIWARE k = 4 | N = 4 of Orion and IoT Agent UL on 4-7; MongoDB on 2-3 |
| Sweep | cold start + DB wipe per platform; warm-up 120 s at 50; then one continuous 180 s constant-rate step each at 50, 100, 200, 350, 500, 700, 1000 (k = 1: 50..500); VU pool ceil(rate × 1.5 s) per step; request timeout 5 s; 1000 devices round-robin |
| Observation | Go sink (`cmd/sink`): first receipt per unique (run_id, seq); outstanding work = scheduled ≤ t minus first sink receipt ≤ t, exact time-average over the last 10 s of each minute |
| CPU | cgroup v2 `usage_usec` snapshots every 1 s (`monitoring/exporters/cpu-snapshot.sh --loop`); D_i = CPU-s / acknowledged messages of the step; B_i = cpuset cores / D_i; pool bound = pool cores / Σ D_i of the containers sharing the cpuset |
| Code | `671262e` for all four runs (vm1 was not redeployed in between; each `manifest.json` records its SHA and `k6_scripts_sha256`). Later commits changed analysis/setup scripts only; `explore.json` of every run was regenerated with `b844010` (identical verdicts) |

## Pass rule (declared before any data, applied unchanged)

Per step: generator health (achieved ≥ 99 %, 0 dropped iterations, send-lag p99 ≤ 10 ms; else INVALID (generator));
unsuccessful responses ≤ 0.1 %; ack p99 of every minute ≤ 1 s and p99(minute 3) ≤ max(1.5 × p99(minute 1),
p99(minute 1) + 50 ms); outstanding work avg(last 10 s of minute 3) ≤ avg(last 10 s of minute 1) + rate × 1 s; stored ids
reconciled after the run. The first non-passing step stops the evaluation; later steps are "not evaluated".
EdgeX only (user decision): a failed setup gate makes the run INVALID (setup failure); a core-metadata panic/restart
during the run makes it FAIL (platform defect). Both EdgeX runs passed the gate and showed no defect.

## Deviations (in each run's `manifest.json`, `deviations`, except where noted)

Exception: the ThingsBoard memory deviation was committed to the plan after vm1's last deploy, so
`EXPL-tb-k4b/manifest.json` does not list it. It was applied (`results/exploratory/sut-logs/tb-prep.sh`; container
limits 1.5 GiB / 1.5 GiB / 768 MiB recorded in `tb-prep.log`) and is recorded here and in decision D11.


- Outbound delivery to the sink: ThingsBoard REST API Call node after Save Timeseries (in tb-rule-engine, counted in
  platform CPU); EdgeX app-service-configurable http-export (profile `export`) in the application pool; FIWARE Orion
  subscription (keyValues). Setup for the measurement, not tuning; it costs CPU (caveat 3).
- **EdgeX provision-then-scale at every k** (user decision): devices are created at one device-rest replica, then the
  deployment is scaled out, because adding a device with several replicas makes core-metadata panic
  (`docs/findings/edgex-device-validation-panic.md`). The generator's EdgeX setup reuses existing devices.
- **ThingsBoard memory** (user decision, D11): tb-core and tb-rule-engine 1536m, transport 768m (heap flags
  unchanged, `-Xmx512m`). At 1 GB tb-core was OOM-killed during device provisioning (`EXPL-tb-k4`, 09:41 UTC).
- 1000 devices instead of 100 (ThingsBoard keys telemetry by device and arrival millisecond).
- Generator: per-minute ack latency as tagged k6 Trends (replacing bucket counters), `GOGC=400`, upper-case run ids.
- FIWARE: the benchmark waits 65 s after creating the subscription (replicated Orion refreshes its subscription cache
  periodically; the first trace saw 50 of 200 notifications).

## Anomalies

**EdgeX**
- core-metadata panics (`send on closed channel`) when a device is added with 4 device-rest replicas: 3 of 3 attempts,
  plus 4 restarts during a cold start at N = 4. Not reproduced at N = 2 in one attempt (race). Finding document above.
- `EXPL-edgex-k4`: first provisioning attempt failed at batch 0 (device-rest answers ping before it serves validation
  requests); the script now retries. core-data-1 restarted once at start-up (registration not found), before the gate.
- `EXPL-edgex-k1`, 500 msg/s: export service OOM-killed at 512 MB (14:08:33); core-data's NATS client dropped events
  as a slow consumer (14:09:47-14:10:07): **7,188 acknowledged readings never stored** (core NATS is at-most-once).
  The sink was stopped 30 s after this run while the export still had a backlog, so its count for this step is a lower
  bound (later runs wait until the sink is idle).

**ThingsBoard**
- `EXPL-tb-k4` (first attempt, 1 GB limits): tb-core OOM-killed while creating device 434; run aborted in setup. The
  same tb-core OOM is in the kernel log on 19 Oct.
- `EXPL-tb-k4b`, from 14:52 (700/1000 msg/s, not evaluated): tb-rule-engine `OutOfMemoryError: Java heap space`
  (74 times, no restart); the rule engine stalled with ~193,700 messages left in Kafka (`re-Main` consumer lag); only
  276,022 stored when the counts stopped changing. Steps up to 350 are unaffected (all 63,000 of the 350 step stored).
- Kafka uses ~0.9 cores at 50 msg/s; at 200 msg/s the stateful cores 2-3 were ~1.8/2 busy and the pool 3.1/4.

**FIWARE**
- From 15:22 (500 msg/s, not evaluated): IoT Agent replicas OOM-killed at 512 MB repeatedly (3-6 restarts each);
  unsuccessful responses 3 % at 350, 48 % at 500, 99.5 % at 1000. The 200 msg/s failure precedes all of this.

**Generator / tooling**
- 1000 msg/s: k6 5.9 GB, load-host swap (caveat 2).
- Fixed during the check (no rule changed): outstanding work was point-sampled on the send grid (+1 bias; now an
  exact time-average); `explore.json` was written empty when a value was NaN (now `null`, and marshal errors are
  fatal); a CPU-loop `pkill` matched its own shell once (EdgeX k = 4 history exported after the fact, same data).
- `EXPL-tb-k4` was analysed by mistake when the rerun reused its id; that output is kept as
  `EXPL-tb-k4/explore.invalid-rerun-mistake.json` and is not a result.

## Appendix: per-step tables and CPU per container

Generated by `scripts/explore-tables.py results/exploratory/<run>`. "Cores used" = container CPU-seconds over the
step / step duration. Per-container B_i inside a shared pool is optimistic; the pool bound is the binding figure there.

### EXPL-edgex-k4

| Step | Rate | Verdict | Unsucc. | Send-lag p99 (ms) | Ack p99 min 1/2/3 (ms) | Outstanding min 1/2/3 | Smallest B_i (msg/s) | Reasons |
|---|---|---|---|---|---|---|---|---|
| s00-warmup-r50 | 50 | warm-up | 0.0000% | 3.0 | 14/5 | 0.3/0.3 | - |  |
| s01-sweep-r50 | 50 | PASS | 0.0000% | 2.0 | 7/5/6 | 0.3/0.3/0.3 | 632 (edgex-database-1) |  |
| s02-sweep-r100 | 100 | PASS | 0.0000% | 2.0 | 7/4/8 | 0.5/0.5/0.5 | 877 (edgex-database-1) |  |
| s03-sweep-r200 | 200 | PASS | 0.0000% | 2.0 | 6/7/4 | 0.9/0.9/0.9 | 1008 (edgex-database-1) |  |
| s04-sweep-r350 | 350 | PASS | 0.0000% | 3.6 | 11/6/11 | 1.9/1.8/1.8 | 1217 (edgex-database-1) |  |
| s05-sweep-r500 | 500 | PASS | 0.0000% | 3.0 | 9/11/7 | 2.9/2.9/2.8 | 1567 (edgex-database-1) |  |
| s06-sweep-r700 | 700 | PASS | 0.0000% | 8.3 | 25/8/30 | 5.4/5.3/6.1 | 2221 (edgex-database-1) |  |
| s07-sweep-r1000 | 1000 | INVALID | 0.0000% | 456.0 | 27/1529/187 | 5266.0/9490.2/11783.6 | - | generator: dropped_iterations: 174 dropped (must be 0: seq/scheduled-time mapping needs it); generator: send_lag_p99: p99 456.01 ms (limit 10 ms; p50 174.00, max 1592.00) |

Sink: 527886 of 528000 planned reached, 0 duplicate receipts. History reconciliation (whole run): refused: run has 174 dropped iterations: identities are not exact, reconciliation refused.

CPU per container (cores used = CPU-s / step duration; D_i in ms CPU per message; B_i = allocated cores / D_i):

| Container | 50/s | 100/s | 200/s | 350/s | 500/s | 700/s |
|---|---|---|---|---|---|---|
| edgex-app-http-export-1 | 0.10 c, D 2.031, B 1969 | 0.19 c, D 1.903, B 2102 | 0.34 c, D 1.708, B 2342 | 0.59 c, D 1.678, B 2384 | 0.77 c, D 1.540, B 2597 | 0.98 c, D 1.394, B 2870 |
| edgex-core-common-config-bootstrapper-1 | 0.00 c, D 0.001, B 1337594 | 0.00 c, D 0.001, B 2967604 | 0.00 c, D 0.000, B 5999000 | 0.00 c, D 0.000, B 9762145 | 0.00 c, D 0.000, B 14722722 | 0.00 c, D 0.000, B 20334060 |
| edgex-core-data-1 | 0.04 c, D 0.810, B 4935 | 0.08 c, D 0.766, B 5221 | 0.14 c, D 0.724, B 5521 | 0.24 c, D 0.676, B 5920 | 0.27 c, D 0.548, B 7304 | 0.27 c, D 0.386, B 10355 |
| edgex-core-data-2 | 0.04 c, D 0.769, B 5204 | 0.08 c, D 0.755, B 5295 | 0.14 c, D 0.719, B 5560 | 0.24 c, D 0.683, B 5858 | 0.27 c, D 0.546, B 7322 | 0.27 c, D 0.384, B 10420 |
| edgex-core-data-3 | 0.04 c, D 0.760, B 5267 | 0.08 c, D 0.769, B 5204 | 0.14 c, D 0.710, B 5636 | 0.24 c, D 0.681, B 5871 | 0.27 c, D 0.543, B 7360 | 0.27 c, D 0.385, B 10396 |
| edgex-core-data-4 | 0.04 c, D 0.802, B 4989 | 0.08 c, D 0.760, B 5265 | 0.14 c, D 0.702, B 5700 | 0.24 c, D 0.678, B 5901 | 0.27 c, D 0.542, B 7382 | 0.27 c, D 0.384, B 10418 |
| edgex-core-keeper-1 | 0.00 c, D 0.043, B 47009 | 0.00 c, D 0.022, B 89378 | 0.00 c, D 0.012, B 167787 | 0.00 c, D 0.007, B 279646 | 0.00 c, D 0.005, B 397003 | 0.00 c, D 0.004, B 560655 |
| edgex-core-metadata-1 | 0.00 c, D 0.018, B 113631 | 0.00 c, D 0.008, B 244096 | 0.00 c, D 0.005, B 432542 | 0.00 c, D 0.003, B 780273 | 0.00 c, D 0.002, B 1043872 | 0.00 c, D 0.001, B 1656238 |
| edgex-database-1 | 0.16 c, D 3.164, B 632 | 0.23 c, D 2.281, B 877 | 0.40 c, D 1.983, B 1008 | 0.58 c, D 1.644, B 1217 | 0.64 c, D 1.276, B 1567 | 0.63 c, D 0.901, B 2221 |
| edgex-device-rest-1 | 0.02 c, D 0.490, B 8162 | 0.04 c, D 0.441, B 9060 | 0.08 c, D 0.390, B 10258 | 0.13 c, D 0.364, B 10981 | 0.18 c, D 0.359, B 11146 | 0.23 c, D 0.334, B 11966 |
| edgex-device-rest-2 | 0.02 c, D 0.490, B 8157 | 0.04 c, D 0.441, B 9075 | 0.08 c, D 0.391, B 10237 | 0.13 c, D 0.363, B 11020 | 0.18 c, D 0.354, B 11288 | 0.23 c, D 0.331, B 12091 |
| edgex-device-rest-3 | 0.02 c, D 0.481, B 8318 | 0.04 c, D 0.442, B 9047 | 0.08 c, D 0.390, B 10259 | 0.13 c, D 0.363, B 11016 | 0.18 c, D 0.355, B 11267 | 0.23 c, D 0.331, B 12080 |
| edgex-device-rest-4 | 0.02 c, D 0.488, B 8198 | 0.04 c, D 0.444, B 9007 | 0.08 c, D 0.392, B 10194 | 0.13 c, D 0.365, B 10969 | 0.18 c, D 0.355, B 11265 | 0.23 c, D 0.333, B 12023 |
| edgex-haproxy-1 | 0.04 c, D 0.723, B 1383 | 0.06 c, D 0.575, B 1738 | 0.10 c, D 0.476, B 2100 | 0.16 c, D 0.455, B 2197 | 0.22 c, D 0.433, B 2310 | 0.28 c, D 0.395, B 2535 |
| edgex-nats-1 | 0.06 c, D 1.180, B 1695 | 0.09 c, D 0.858, B 2330 | 0.13 c, D 0.654, B 3059 | 0.20 c, D 0.578, B 3461 | 0.27 c, D 0.533, B 3752 | 0.34 c, D 0.480, B 4164 |

| Pool bound (cores / sum D) | 50/s | 100/s | 200/s | 350/s | 500/s | 700/s |
|---|---|---|---|---|---|---|
| cpuset 1 (1 c) | 1383 | 1738 | 2100 | 2197 | 2310 | 2535 |
| cpuset 2-3 (2 c) | 454 | 631 | 754 | 896 | 1101 | 1443 |
| cpuset 4-7 (4 c) | 562 | 595 | 653 | 684 | 778 | 939 |

### EXPL-edgex-k1

| Step | Rate | Verdict | Unsucc. | Send-lag p99 (ms) | Ack p99 min 1/2/3 (ms) | Outstanding min 1/2/3 | Smallest B_i (msg/s) | Reasons |
|---|---|---|---|---|---|---|---|---|
| s00-warmup-r50 | 50 | warm-up | 0.0000% | 2.0 | 7/5 | 0.3/0.3 | - |  |
| s01-sweep-r50 | 50 | PASS | 0.0000% | 2.0 | 6/6/5 | 0.3/0.3/0.3 | 420 (edgex-core-data-1) |  |
| s02-sweep-r100 | 100 | PASS | 0.0000% | 2.0 | 7/5/7 | 0.5/0.5/0.5 | 486 (edgex-core-data-1) |  |
| s03-sweep-r200 | 200 | PASS | 0.0000% | 2.0 | 10/7/8 | 1.0/1.0/1.0 | 748 (edgex-core-data-1) |  |
| s04-sweep-r350 | 350 | PASS | 0.0000% | 2.6 | 22/20/19 | 9.7/7.8/7.7 | 925 (edgex-app-http-export-1) |  |
| s05-sweep-r500 | 500 | FAIL | 0.0000% | 2.0 | 43/34/30 | 4812.3/9052.6/19236.2 | - | outstanding work grows: minute 3 19236.2 > minute 1 4812.3 + rate x 1 s = 5312.3 |

Sink: 202815 of 222000 planned reached, 0 duplicate receipts. History reconciliation (whole run): stored unique 214812, ack_lost 7188, duplicates 0, orphans 0.

CPU per container (cores used = CPU-s / step duration; D_i in ms CPU per message; B_i = allocated cores / D_i):

| Container | 50/s | 100/s | 200/s | 350/s |
|---|---|---|---|---|
| edgex-app-http-export-1 | 0.07 c, D 1.474, B 678 | 0.14 c, D 1.364, B 733 | 0.24 c, D 1.216, B 823 | 0.38 c, D 1.081, B 925 |
| edgex-core-common-config-bootstrapper-1 | 0.00 c, D 0.002, B 1215067 | 0.00 c, D 0.001, B 2723765 | 0.00 c, D 0.000, B 5370329 | 0.00 c, D 0.000, B 10050251 |
| edgex-core-data-1 | 0.12 c, D 2.383, B 420 | 0.21 c, D 2.056, B 486 | 0.27 c, D 1.337, B 748 | 0.22 c, D 0.628, B 1592 |
| edgex-core-keeper-1 | 0.00 c, D 0.046, B 43338 | 0.00 c, D 0.023, B 86307 | 0.00 c, D 0.012, B 173372 | 0.00 c, D 0.007, B 297862 |
| edgex-core-metadata-1 | 0.00 c, D 0.017, B 115625 | 0.00 c, D 0.009, B 222845 | 0.00 c, D 0.004, B 467144 | 0.00 c, D 0.002, B 851858 |
| edgex-database-1 | 0.15 c, D 3.021, B 662 | 0.22 c, D 2.172, B 921 | 0.27 c, D 1.357, B 1473 | 0.25 c, D 0.705, B 2836 |
| edgex-device-rest-1 | 0.06 c, D 1.181, B 847 | 0.11 c, D 1.055, B 948 | 0.19 c, D 0.936, B 1068 | 0.29 c, D 0.842, B 1188 |
| edgex-haproxy-1 | 0.03 c, D 0.635, B 1574 | 0.06 c, D 0.553, B 1807 | 0.09 c, D 0.472, B 2120 | 0.15 c, D 0.427, B 2344 |
| edgex-nats-1 | 0.06 c, D 1.130, B 1769 | 0.08 c, D 0.809, B 2471 | 0.13 c, D 0.634, B 3155 | 0.18 c, D 0.515, B 3881 |

| Pool bound (cores / sum D) | 50/s | 100/s | 200/s | 350/s |
|---|---|---|---|---|
| cpuset 1 (1 c) | 1574 | 1807 | 2120 | 2344 |
| cpuset 2-3 (2 c) | 474 | 664 | 996 | 1626 |
| cpuset 4 (1 c) | 198 | 223 | 287 | 392 |

### EXPL-tb-k4b

| Step | Rate | Verdict | Unsucc. | Send-lag p99 (ms) | Ack p99 min 1/2/3 (ms) | Outstanding min 1/2/3 | Smallest B_i (msg/s) | Reasons |
|---|---|---|---|---|---|---|---|---|
| s00-warmup-r50 | 50 | warm-up | 0.0000% | 6.0 | 124/84 | 5.4/5.8 | - |  |
| s01-sweep-r50 | 50 | PASS | 0.0000% | 5.0 | 76/81/86 | 5.3/4.7/5.0 | 108 (tb-kafka-1) |  |
| s02-sweep-r100 | 100 | PASS | 0.0000% | 4.0 | 127/107/128 | 19.3/17.4/18.2 | 193 (tb-kafka-1) |  |
| s03-sweep-r200 | 200 | PASS | 0.0000% | 5.0 | 159/172/153 | 59.0/57.3/63.3 | 404 (tb-kafka-1) |  |
| s04-sweep-r350 | 350 | FAIL | 0.0000% | 4.1 | 259/274/219 | 449.6/1592.7/1208.8 | - | outstanding work grows: minute 3 1208.8 > minute 1 449.6 + rate x 1 s = 799.6 |
| s05-sweep-r500 | 500 | not evaluated | 0.0000% | 8.0 | 579/526/488 | 11688.5/23887.4/35525.8 | - |  |
| s06-sweep-r700 | 700 | not evaluated | 0.0000% | 51.9 | 1607/365/635 | 61441.2/98698.5/140696.1 | - |  |
| s07-sweep-r1000 | 1000 | not evaluated | 0.5712% | 53533.0 | 24462/53465/56677 | 199186.5/259020.5/318987.5 | - |  |

Sink: 204013 of 528000 planned reached, 0 duplicate receipts. History reconciliation (whole run): refused: run has 54457 dropped iterations: identities are not exact, reconciliation refused.

CPU per container (cores used = CPU-s / step duration; D_i in ms CPU per message; B_i = allocated cores / D_i):

| Container | 50/s | 100/s | 200/s |
|---|---|---|---|
| tb-haproxy-1 | 0.04 c, D 0.773, B 1294 | 0.07 c, D 0.654, B 1529 | 0.10 c, D 0.519, B 1928 |
| tb-kafka-1 | 0.93 c, D 18.524, B 108 | 1.04 c, D 10.381, B 193 | 0.99 c, D 4.952, B 404 |
| tb-postgres-1 | 0.20 c, D 3.958, B 505 | 0.24 c, D 2.381, B 840 | 0.29 c, D 1.475, B 1356 |
| tb-tb-core-1 | 0.49 c, D 9.845, B 406 | 0.71 c, D 7.109, B 563 | 0.94 c, D 4.704, B 850 |
| tb-tb-http-transport-1 | 0.35 c, D 7.022, B 570 | 0.49 c, D 4.901, B 816 | 0.67 c, D 3.364, B 1189 |
| tb-tb-rule-engine-1 | 0.89 c, D 17.743, B 225 | 1.12 c, D 11.216, B 357 | 1.53 c, D 7.649, B 523 |
| tb-valkey-1 | 0.22 c, D 4.353, B 459 | 0.35 c, D 3.456, B 579 | 0.52 c, D 2.577, B 776 |
| tb-zookeeper-1 | 0.01 c, D 0.129, B 15483 | 0.00 c, D 0.045, B 44369 | 0.00 c, D 0.021, B 95901 |

| Pool bound (cores / sum D) | 50/s | 100/s | 200/s |
|---|---|---|---|
| cpuset 1 (1 c) | 1294 | 1529 | 1928 |
| cpuset 2-3 (2 c) | 74 | 123 | 222 |
| cpuset 4-7 (4 c) | 116 | 172 | 254 |

### EXPL-fiware-k4

| Step | Rate | Verdict | Unsucc. | Send-lag p99 (ms) | Ack p99 min 1/2/3 (ms) | Outstanding min 1/2/3 | Smallest B_i (msg/s) | Reasons |
|---|---|---|---|---|---|---|---|---|
| s00-warmup-r50 | 50 | warm-up | 0.0000% | 2.0 | 95/103 | 1.7/2.0 | - |  |
| s01-sweep-r50 | 50 | PASS | 0.0000% | 2.0 | 131/111/103 | 1.9/2.1/1.8 | 114 (fiware-mongo-1) |  |
| s02-sweep-r100 | 100 | PASS | 0.0000% | 3.0 | 252/254/181 | 5.6/4.7/4.5 | 149 (fiware-mongo-1) |  |
| s03-sweep-r200 | 200 | FAIL | 0.0000% | 2.0 | 1235/1768/1579 | 35.8/54.8/93.5 | - | minute 1 ack p99 1235.0 ms > 1 s (n=12000); minute 2 ack p99 1768.1 ms > 1 s (n=12000); minute 3 ack p99 1579.0 ms > 1 s (n=12000) |
| s04-sweep-r350 | 350 | not evaluated | 3.1128% | 56103.4 | 27889/55850/60682 | 5714.9/12521.9/19599.2 | - |  |
| s05-sweep-r500 | 500 | not evaluated | 47.6216% | 95059.5 | 68278/97955/- | 35407.9/51662.1/68694.2 | - |  |
| s06-sweep-r700 | 700 | not evaluated | 72.0286% | 114456.0 | 105475/119592/- | 97183.0/125011.1/153261.8 | - |  |
| s07-sweep-r1000 | 1000 | not evaluated | 99.5138% | 121154.0 | 120707/-/- | 199619.6/249181.1/298914.0 | - |  |

Sink: 227139 of 528000 planned reached, 0 duplicate receipts. History reconciliation (whole run): not reconcilable (no history).

CPU per container (cores used = CPU-s / step duration; D_i in ms CPU per message; B_i = allocated cores / D_i):

| Container | 50/s | 100/s |
|---|---|---|
| fiware-haproxy-1 | 0.10 c, D 2.025, B 494 | 0.17 c, D 1.717, B 582 |
| fiware-iot-agent-1 | 0.42 c, D 8.471, B 472 | 0.70 c, D 6.951, B 575 |
| fiware-iot-agent-2 | 0.43 c, D 8.503, B 470 | 0.70 c, D 6.968, B 574 |
| fiware-iot-agent-3 | 0.42 c, D 8.474, B 472 | 0.69 c, D 6.937, B 577 |
| fiware-iot-agent-4 | 0.43 c, D 8.601, B 465 | 0.70 c, D 7.013, B 570 |
| fiware-mongo-1 | 0.88 c, D 17.546, B 114 | 1.34 c, D 13.399, B 149 |
| fiware-orion-1 | 0.06 c, D 1.212, B 3300 | 0.11 c, D 1.060, B 3772 |
| fiware-orion-2 | 0.06 c, D 1.212, B 3300 | 0.11 c, D 1.061, B 3770 |
| fiware-orion-3 | 0.06 c, D 1.215, B 3293 | 0.11 c, D 1.056, B 3790 |
| fiware-orion-4 | 0.06 c, D 1.212, B 3299 | 0.11 c, D 1.054, B 3797 |

| Pool bound (cores / sum D) | 50/s | 100/s |
|---|---|---|
| cpuset 1 (1 c) | 494 | 582 |
| cpuset 2-3 (2 c) | 114 | 149 |
| cpuset 4-7 (4 c) | 103 | 125 |

