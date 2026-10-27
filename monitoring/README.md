# Monitoring: Prometheus + Grafana on the load host, exporters on the SUT

Operational monitoring for the IoT benchmark. It shows what happens during a run. **It is not evidence:**
- generator verdicts come from `benchctl check` (`health.json`);
- identity audits come from `benchctl reconcile`;
- CPU accounting uses counter boundary snapshots;
- figures are generated from `results/`.

What each metric means and how far it is verified: `docs/metrics-dictionary.md`. Validation evidence:
`MONITORING_FINDINGS.md`.

```
ramonavm1 (load host, 4 vCPU)                                    ramonavm2 (SUT, 8 vCPU)
 cores 0-2  k6 (benchctl run ... -prom-rw) ──remote write─┐        core 0: node-exporter :9100 (host, textfile facts)
 core 3     Prometheus 127.0.0.1:9090 ◄───────────────────┘                cAdvisor      :8089 (every container)
            Grafana    :3000 (public, password)  ──scrape──►      core 1: HAProxy native /metrics :8404
            node-exporter 127.0.0.1:9100                           core 0: postgres-exporter :9187 (TB / EdgeX)
            cAdvisor      127.0.0.1:8089 (k6, monitoring)                  kafka-exporter :9308 (TB)
                                                                            mongodb-exporter :9216 (FIWARE)
                                                                            nats-exporter :7777 (EdgeX)
```

## Deploy

**Load host (ramonavm1),** from the repository root (`make` is not installed there, so use the script):

```bash
cp monitoring/.env.example monitoring/.env
sed -i "s|^GF_ADMIN_PASSWORD=.*|GF_ADMIN_PASSWORD=$(openssl rand -base64 18)|" monitoring/.env
scripts/monitoring.sh up        # = make monitoring-up
scripts/monitoring.sh check     # promtool config + rules, then target health
scripts/monitoring.sh reload    # after editing prometheus.yml or rules/
scripts/monitoring.sh down
```

**SUT (ramonavm2),** after the platform is up (the platform exporters join its Docker network):

```bash
make exporters-install           # once: firewall (monitoring ports only from ramonavm1) + container-info timer
make exporters-up P=edgex        # node-exporter + cAdvisor + that platform's exporters (thingsboard | fiware | edgex)
make exporters-down
```

**Generator stream:**

```bash
export BENCH_GRAFANA_PASSWORD=$(sed -n 's/^GF_ADMIN_PASSWORD=//p' monitoring/.env)
bin/benchctl run -plan workloads/plans/<plan>.json -cpuset 0-2 -env GOGC=400 \
  -prom-rw http://127.0.0.1:9090/api/v1/write -grafana-url http://127.0.0.1:3000
```

Without `-prom-rw` a run is identical but invisible to Prometheus. The raw artifacts in `results/runs/<run_id>/` are
produced in both cases.

## Images

All images are pinned; their digests are recorded in `MONITORING_FINDINGS.md`.
- Docker Hub images come through `docker.arvancloud.ir` on ramonavm1 and the Liara mirror on ramonavm2.
- cAdvisor comes from `ghcr.io/google/cadvisor:v0.60.6`, because gcr.io answers 403 from Iran. ramonavm1 cannot
  reach ghcr.io at all, so its copy was transferred with `docker save | docker load` from ramonavm2 (same image id).

## Configuration choices

- **Scrape interval 5 s, timeout 4 s, rule evaluation 5 s.** cAdvisor housekeeping is 5 s and it sends its own
  timestamps, so some scrapes repeat a sample: expect ~19 samples per 134 s from cAdvisor, not 27. Rate rules use
  30 s windows.
- **Retention:** 30 days or 20 GB, whichever comes first. At ~2,000 active series a 10-minute run adds well under
  100 MB.
- **Labels:** `platform`, `tier`, `service`, `container` are derived from compose labels (see `prometheus.yml`), then
  the raw cAdvisor labels (`container_label_*`, `id`, `image`, `name`) are dropped.
  - Replicas stay distinct by `container`.
  - k6 adds only `run_id`, `plan_id`, `adapter` (one value per run): about 80 series per run.
  - No message, sequence, device, URL or token labels.
- **Recording rules** (`rules/bench.yml`) need `--enable-feature=promql-delayed-name-removal`. The k6 rules turn
  per-step metric names into `outcome`, `step` and `le` labels after `rate()`. Without the flag they silently
  merged all outcomes into one series.
- **Gating:** CPU rules and exporter panels are gated on `up == 1` of their target, so an outage shows no data
  rather than a frozen value or 0.

## Credentials and access

- **Grafana:** `admin` with the password in `monitoring/.env` (gitignored, mode 600 on ramonavm1). It is the only
  public port on the load host. Prometheus listens on 127.0.0.1 only: use `ssh -L 9090:127.0.0.1:9090 ramonavm1`.
- **SUT exporters:** reachable only from ramonavm1 (`firewall.sh`: iptables INPUT for host-network node-exporter,
  DOCKER-USER for the published ports, re-applied after every Docker start by `iot-bench-firewall.service`).
- **PostgreSQL exporters** use the platforms' `postgres` superuser (as configured by the stacks). A
  least-privilege `pg_monitor` role is in the backlog (`docs/tooling-interfaces.md`).
- **Privileged mounts:** cAdvisor runs privileged with read-only `/`, `/sys`, `/var/lib/docker` and `/dev/kmsg` (OOM
  events). node-exporter has the host root read-only. `container-info.sh` runs as root via a systemd timer and calls
  `docker inspect`.

## Overhead (indicative, one matched pair: details in MONITORING_FINDINGS.md)

| What | Measured |
|---|---|
| k6 remote write on the generator (EdgeX, 200 msg/s) | +1-2 points CPU per generator core, +7 MB, no change in send lag |
| Prometheus | ~0.03 cores at 200 msg/s |
| SUT exporters (core 0) | visible on the dashboard ("Exporter CPU on the SUT") |

Not qualified: that needs repeated randomized pairs. **Grafana should stay closed (or stopped) during runs**: a
dashboard refreshing every 10 s runs ~45 queries per refresh on the monitoring core.

## Dashboard

"IoT benchmark overview" (home dashboard, folder "IoT benchmark"). Select:
- `platform`;
- optionally `service`;
- `run (k6 stream)`;
- the time range: click a run in "Runs" (group 8) or its annotation.

Groups:
1. monitoring health;
2. workload delivery;
3. latency;
4. SUT CPU;
5. host/container resources;
6. queue/storage diagnostics of the selected platform;
7. measurement integrity;
8. runs.

Text panels mark what is **unavailable** (history-write timing, audit, derived metrics). Sink receipt and duplicate
rates are live (`sink_received_total`, `sink_duplicates_total`; job `sink` in `prometheus.yml`); sink observation
latency and outstanding work are computed offline by `benchctl explore`. These panels are never filled from other
metrics. Community dashboards (Node Exporter Full, k6, HAProxy, PostgreSQL, MongoDB,
Kafka exporter, NATS) are provisioned next to it. Rebuild all with `make dashboards` on the workstation.

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| A target is DOWN | Its platform is not running (expected for the other two platforms), or `make exporters-up P=` was not run after the platform came up. Check the firewall for the load host's IP |
| k6 panels empty | The run was started without `-prom-rw`, or the `run` variable does not include it |
| Outcomes look merged / `outcome` label missing | Prometheus started without `--enable-feature=promql-delayed-name-removal` |
| `bench_container_*` missing | `make exporters-install` not run, or the textfile directory isn't mounted (node-exporter flag `--collector.textfile.directory`) |
| CPU totals look low compared with a cost calculation | Expected: never integrate rate panels. Use counter differences (`monitoring/exporters/cpu-snapshot.sh`) |
| ghcr.io pull fails on ramonavm1 | Domestic-only network: `docker save` on ramonavm2, `docker load` on ramonavm1 |
