# Metrics dictionary

What each research quantity is, which producer and metric actually supply it today, and how far that has been
verified. **Status:**
- **live** = checked against real traffic on the testbed (with the evidence named);
- **static** = configuration checked only;
- **unavailable** = no producer exists yet.

The run used for "live" evidence is named in `MONITORING_FINDINGS.md`. Dates: 2026-10-22.

Common labels: `host` (ramonavm1 | ramonavm2), `role` (loadgen | sut | monitoring), `platform`
(thingsboard | fiware | edgex), `tier` (application | supporting | monitoring), `service` (compose service),
`container` (one per replica). k6 stream labels: `run_id`, `plan_id`, `adapter`, `scenario` (step name), `status`,
`name` (setup/send/verify/cleanup), `expected_response`. **Never** message, seq, device, URL or token labels.

## Workload delivery (generator)

| Quantity | Producer / metric | Units, labels | Aggregation | Panel | Caveat | Status |
|---|---|---|---|---|---|---|
| Planned (offered) rate | k6 gauge `k6_g_planned_rate_s<step>` → `bench:k6_planned_rate{step}` | msg/s; run_id, step | max | 2 "Planned vs sent" | Refreshed every 500th tick; the authoritative plan is `resolved.json` | live (run A: 100) |
| Sent requests | k6 `k6_http_reqs_total{name="send"}` → `bench:k6_send_requests:rate30s` | req/s; scenario, status | sum, 30 s rate | 2 | Rates are approximate (~5% low when integrated); exact counts = final counter value or `k6-summary.json` | live (12000 = run) |
| Outcomes ack_success / explicit_failure / unknown | k6 counters `k6_g_<outcome>_s<step>_total` → `bench:k6_outcome:rate30s{outcome,step}` | msg/s | sum, 30 s rate | 2 | Rule needs Prometheus `--enable-feature=promql-delayed-name-removal` (without it, outcomes were merged into one series: found and fixed) | live (run B: 2895/75/30 exact in raw counters) |
| Dropped iterations (not_sent) | k6 `k6_dropped_iterations_total{scenario}` → `bench:k6_dropped_iterations:rate30s` | iter/s | sum | 2 | Absent series = no drop was reported **or** no stream. The verdict comes from `health.json` (dropped == 0 gate), never from this panel | static (no drops occurred in monitored runs) |
| Early sends | `k6_g_early_s<step>_total` | sends/s | sum | 2 | gate in health.json | static |
| Active / allocated VUs | `k6_vus`, `k6_vus_max` | VUs | max | 2 | | live |
| HTTP status | `k6_http_reqs_total{status}` | req/s | sum | 2 | status 0 = no response | live (run B: 200/503/0) |

## Latency

| Quantity | Producer / metric | Units | Aggregation | Panel | Caveat | Status |
|---|---|---|---|---|---|---|
| Send lag (actual JS send − scheduled) | generator bucket counters `k6_g_lag_s<step>_le<edge>_total` → `bench:k6_latency_bucket:rate30s{histogram="lag",le}` | ms buckets (non-cumulative), counts/s | sum by le | 3 | JS ms clock; the authoritative per-run p99 is from exact counts in `health.json`. Edges come from the plan | live (run B) |
| Ack latency (HTTP response − scheduled) | `k6_g_ack_s<step>_le<edge>_total` → same rule, `histogram="ack"` | ms buckets | sum by le | 3 | Acknowledgement, not persistence/observation | live |
| k6 http_req_duration (actual send → response) | `k6_http_req_duration_{p50,p99,max}` | s | max per scenario | 3 (diagnostic) | k6 computes the quantiles per 5 s push: **not mergeable**, never averaged into a run p99 | live |
| Scheduled → sink observation | future Go sink | — | — | 3 (unavailable text) | | **unavailable** |
| History-write / committed visibility | future DB instrumentation | — | — | 3 (unavailable text) | | **unavailable** |

## SUT CPU

| Quantity | Producer / metric | Units | Aggregation | Panel | Caveat | Status |
|---|---|---|---|---|---|---|
| CPU per container | cAdvisor `container_cpu_usage_seconds_total` (cgroup v2 `usage_usec`) → `bench:container_cpu_cores:rate30s` | core-equivalents | 30 s rate, gated on `up{job="cadvisor"}` | 4 | `--docker_only`: one series per container, no parent cgroups, no double counting | live |
| CPU per service / tier / platform | `bench:{service,tier,platform}_cpu_cores:rate30s` | cores | sum of containers | 4 | platform = application + supporting (incl. HAProxy, DBs, brokers); exporters excluded | live (EdgeX, FIWARE, ThingsBoard) |
| Interval CPU-seconds (accounting) | **counter boundary difference** (cAdvisor counter or `monitoring/exporters/cpu-snapshot.sh` from `cpu.stat`) | CPU-s | end − start per container, summed | — | Counter diff matched the kernel within 0.8% (platform total) over 134 s. **Integrating the 30 s rate underestimated by 8.8%** (1 m window: −18%): rates are for display only | live (run A) |
| Host busy per core | node-exporter `node_cpu_seconds_total` → `bench:host_core_busy:ratio30s` | fraction | 1 − idle rate | 1, 4 | | live |
| Affinity (cpuset) | textfile `bench_container_info{cpuset}`, `bench_container_cpuset_cpus` | CPUs | — | 1 | from `docker inspect` every 10 s | live |
| CPU throttling | — | — | — | 4 (text) | Not produced: containers are pinned by cpuset and have **no CFS quota** | n/a by design |
| CPU per persisted message (U) | needs audited persisted cohort | CPU-s/msg | offline | 8 (text) | | **unavailable** |

## Host / container resources

| Quantity | Producer / metric | Caveat | Status |
|---|---|---|---|
| Memory working set vs limit | cAdvisor `container_memory_working_set_bytes` / `container_spec_memory_limit_bytes` | working set includes active page cache | live |
| OOM kills | cAdvisor `container_oom_events_total`; textfile `bench_container_oom_killed` | | static (no OOM during monitoring runs) |
| Container (re)starts | cAdvisor `changes(container_start_time_seconds)` | Docker `RestartCount` (textfile `bench_container_restarts`) counts restart-policy restarts **only**: it stayed 0 after `docker restart` and recreation | live (2 starts seen) |
| Disk activity | node-exporter `node_disk_io_time_seconds_total`, `node_disk_written_bytes_total` | host device level; no per-container or per-transaction latency | live |
| Network | node-exporter `node_network_*_bytes_total`, `_errs_total` | host level | live |
| Filesystem free | node-exporter `node_filesystem_avail_bytes` | | live |
| Load-host containers (k6, Prometheus, Grafana) | load-host cAdvisor (`job="loadgen-cadvisor"`) | | live |

## Queue and storage diagnostics

| Quantity | Producer / metric | Exact semantics | Status |
|---|---|---|---|
| Kafka consumer-group lag (ThingsBoard) | kafka-exporter v1.9.0 `kafka_consumergroup_lag{consumergroup,topic,partition}` | log-end offset − committed offset per group/partition: produced but not yet committed by that group. Not persistence evidence | live (ThingsBoard smoke, all groups 0) |
| Kafka broker internals (JMX) | — | not connected (gap) | unavailable |
| NATS (EdgeX, core NATS) | prometheus-nats-exporter `gnatsd_varz_in_msgs`, `_out_msgs`, `_slow_consumers` | core NATS has **no persistence and no queue depth**; out < in is fan-out/loss, not backlog | live |
| PostgreSQL activity | postgres-exporter `pg_stat_database_xact_commit`, `_tup_inserted`, `pg_stat_activity_count{state}` | database activity; rows ≠ logical messages (ThingsBoard: 6 rows per reading). Superuser credentials (gap: no least-privilege role yet) | live (EdgeX, ThingsBoard) |
| MongoDB (FIWARE) | percona mongodb_exporter 0.52.0 `mongodb_op_counters_total{type}` (`--compatible-mode`) | operations by type | live (FIWARE smoke) |
| HAProxy per backend/server | native `haproxy_server_http_responses_total`, `haproxy_server_response_time_average_seconds`, `haproxy_backend_current_queue` | avg = moving average over last 1024 requests | live |
| Valkey (ThingsBoard cache), Mosquitto | — | not connected (gap; Mosquitto is not deployed) | unavailable |
| JVM / application metrics (ThingsBoard, Orion, EdgeX) | — | per-replica endpoints are not reachable from the load host (container networks); optional diagnostics, gap | unavailable |

## Monitoring health

| Quantity | Producer | Status |
|---|---|---|
| Target up/down | `up` | live (stopping nats-exporter → up=0 within 3 s) |
| Scrape duration / samples | `scrape_duration_seconds`, `scrape_samples_scraped` | live |
| Prometheus / Grafana resource use | `process_cpu_seconds_total{job=~"prometheus|grafana"}`, load-host cAdvisor | live (~0.028 cores for Prometheus at 200/s) |
| Absent data = no data | all exporter panels gated on `up == 1` | live (before the gate, a rate panel showed 0 during an exporter outage) |
| Clock sync (per host) | node-exporter timex: `node_timex_sync_status`, `node_timex_offset_seconds`, `node_timex_maxerror_seconds` | live on ramonavm1 (synced, offset −12 µs, max-error bound 0.59 s). Each host's view of its own NTP source: **not** a cross-host offset |
