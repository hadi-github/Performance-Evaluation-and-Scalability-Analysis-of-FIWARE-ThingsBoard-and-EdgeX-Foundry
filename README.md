# IoT platform benchmark deployments (replica-scalable, fixed resources, HTTP only)

One Docker Compose stack per platform. Every stack has the same shape:

```
load generator ──HTTP──► HAProxy (always present, also at N=1) ──► replicated tier(s) ──► single stateful tier
```

**The only thing you change to scale is the number of platform instances, N.** Each instance gets one dedicated
CPU core, so the platform runs on exactly N cores (see "Resource layout").

```bash
cd fiware      && make up N=4 && make smoke N=4
cd edgex       && make build && make up N=4 && make smoke N=4
cd thingsboard && make install && make up N=4 && make smoke N=4
make clean    # wipe all data between benchmark cells (cold start)
make check N=4   # print the resource layout and refuse it if it does not fit the host (also run by `make up`)
```

Only one platform can run at a time on the SUT (they share port 8404, and the platform pool cores).

| Platform | One instance = | Not replicated (by design) | LB ports |
|---|---|---|---|
| FIWARE | `orion` + `iot-agent` | MongoDB 8.0 | 1026 Orion · 4041 IoTA north · 7896 UL/HTTP |
| EdgeX 4.0.2 | `core-data` + `device-rest` | PostgreSQL, NATS, core-keeper, core-metadata | 59986 ingress · 59880 core-data · 59881 metadata |
| ThingsBoard 4.3.1.6 | `tb-core` + `tb-rule-engine` + `tb-http-transport` | PostgreSQL, Kafka, ZooKeeper, Valkey | 8080 REST + HTTP device API |

All stacks expose HAProxy stats on `:8404/stats` and Prometheus metrics on `:8404/metrics`. Every tier is its own
container, so per-tier CPU/memory can be monitored separately (`docker stats`, cAdvisor, ...). HAProxy discovers
replicas through Docker DNS (`server-template`, up to 16 per tier) within ~5 s.

## Running an experiment

The load generator and the monitoring stack run on **ramonavm1**, the platforms on **ramonavm2** (the SUT).

```bash
# ramonavm2: platform + its exporters
cd thingsboard && make install && make up N=1 && make smoke N=1     # (fiware / edgex: make up N=1)
cd ../monitoring/exporters && make up P=thingsboard
# ramonavm1: one benchmark cell = a fixed rate for a fixed time
cd workloads && ./run.sh thingsboard 200 10m
# ramonavm2: cold start before the next cell
cd thingsboard && make clean
```

See `workloads/README.md` (open workload model, options, what makes a run invalid) and `monitoring/README.md`
(Prometheus/Grafana, exporters, firewall, dashboards).

## Resource layout (fixed, pinned)

On the 8-core SUT (ramonavm2):

| Cores (`cpuset`) | What | Set by |
|---|---|---|
| 0 | OS, Docker daemon, logging — no containers | — |
| 1 | HAProxy (identical at every N) | `LB_CPUSET` in `.env` |
| 2-3 | stateful/infrastructure services | `INFRA_CPUSET` in `.env` |
| 4 … 4+N-1 | **platform pool**: all replicas of all replicated tiers | `make … N=` (N=1 → `4`, N=2 → `4-5`, N=4 → `4-7`) |

- **CPU:** there is no per-replica CPU quota. The replicated tiers of a platform share the pool, so the platform
  behaves as if it ran on a dedicated N-core machine, and the tiers split the cores as the load demands.
- **Memory:** every container has a hard limit with swap disabled (`mem_limit` = `memswap_limit`), and JVM heaps
  sit inside it. Memory per instance is constant across N: FIWARE 512 MB + 512 MB, EdgeX 512 MB + 512 MB,
  ThingsBoard 1 GB + 1 GB + 512 MB (three JVMs cannot share 1 GB).
- **Guards:** `scripts/resources.mk` refuses an N whose pool does not fit the host's cores, and
  `scripts/check-resources.sh` refuses a stack whose memory limits exceed host memory minus a 1.5 GB reserve, or
  any service without a cpuset/memory limit. N_max on the 8-core SUT is 4.
- **What to report:** infrastructure has fixed cores (2-3). Monitor its CPU: if the database or Kafka saturates, the
  bottleneck has moved out of the platform pool, and that is a finding, not noise.
- If `docker compose` is run by hand, set `POOL_CPUSET` and the `*_REPLICAS` variable yourself (or use make).

## What `make smoke` proves

1. **Every replica is alive.** It reads HAProxy's health table and requires exactly N servers UP per tier (the
   ThingsBoard rule engine has no LB backend; its running containers are counted instead).
2. **Messages are stored exactly once** (ThingsBoard, EdgeX): send N, count stored rows, require `== N`.
   Too many = duplicate processing across replicas; too few = loss.
3. **FIWARE:** only reachability and HTTP ordering. Orion keeps only the latest value, so exactly-once needs the
   notification sink from the experiment plan. This is a known gap, not a pass.

The smoke tests start as soon as HAProxy reports the replicas UP. Right after a cold start FIWARE and EdgeX may
still reject the first requests for a few seconds; wait ~30 s after `make up` before measuring or smoke testing.

## Design decisions (and what a reviewer could attack)

| Decision | Why | Likely reviewer objection → answer |
|---|---|---|
| HAProxy in front even at N=1 | Replica count becomes the only changed factor; the LB hop is constant | "LB adds latency" → constant across all cells, and reported |
| Same LB (HAProxy) for all 3 platforms | LB choice is a controlled variable, not a platform property | — |
| N cores pinned per N instances, infra and LB on their own cores | Scaling adds a known, isolated amount of CPU; infra cannot steal from the platform | "Pinned cores ≠ real nodes" → state it; optionally validate N=1 vs N=2 on two hosts |
| Same CPU unit (1 core) per instance for every platform | Platforms compared at equal CPU | "TB gets more memory" → yes, the JVM minimum; memory is not the scaled factor and is reported |
| Stateful tier not replicated | The research factor is horizontal scaling of the processing tier; DB HA is a separate scenario | "Of course it plateaus, the DB is the bottleneck" → that **is** a finding if you show where the bottleneck moves |
| HTTP only | One ingestion protocol for all three platforms | — |
| Official images, official versions, official topology | Deviations are listed in each `compose.yaml` header | "Not representative" → every deviation has a written reason |
| EdgeX: NATS-enabled images built from v4.0.2 source | Official images lack NATS; NATS queue groups are EdgeX's only built-in competing-consumer mechanism | "Modified platform" → no source change, official build flag only |
| EdgeX: core NATS (not JetStream) | At-most-once delivery, like EdgeX's default MQTT bus at QoS 0 | "Weaker durability than TB/Kafka" → yes; delivery semantics must be a reported dimension |
| ThingsBoard pinned to 4.3.x | 4.4 moved to BUSL 1.1; 4.3.x is the last Apache-2.0 line | "Not latest" → licence scope of the paper |
| TB: own HAProxy config | Official config limits each client IP to 100 req/10 s and pins REST by source IP | — |

## Status: what was verified

All three stacks were booted and smoke-tested on the SUT (ramonavm2, 8 vCPU / 16 GB) and exercised end to end
from the load-generator host with the Yaade collections in `yaade/` (provision a device, send an integer, read the
same integer back). Fixes found while booting (all listed in the compose headers): HAProxy file-descriptor limit,
ThingsBoard installer dependency on Valkey and transport `/config` mount, EdgeX build version / Go module proxy /
message-bus type, and the EdgeX smoke test's count field.

## Known limitations / open decisions

- **ThingsBoard partitions.** Kafka topic partition counts cap how many core/rule-engine replicas can do useful
  work. The defaults (10) are above N_max = 4.
- **ThingsBoard install** loads demo data (`TB_LOAD_DEMO=true`) so the smoke test has a tenant. For clean
  benchmark runs, decide whether to keep it, and state it.
- **EdgeX build** needs github.com and a reachable Go module proxy during `make build`. Where proxy.golang.org is
  blocked, build a builder image that sets `GOPROXY` and point `GO_BUILDER_BASE` at it in `.env`:
  `printf 'FROM golang:1.25-alpine3.22\nENV GOPROXY=https://goproxy.cn,direct\n' | docker build -t local/golang-goproxy:1.25-alpine3.22 -`
  Build once, then pin the resulting image IDs in the run manifest.
- **"200 OK" means different things.** TB and EdgeX acknowledge before persistence; the IoT Agent acknowledges
  after the Orion update. Ingress latency is not comparable across platforms. End-to-end latency needs the sink.
- **Image digests.** Tags are pinned; run `scripts/pin-images.sh <platform>` before a campaign and deploy with
  `-f compose.yaml -f compose.pinned.yaml`.

## Layout

```
scripts/lib.sh            shared smoke-test helpers (LB health table, env precedence)
scripts/resources.mk      N -> platform pool cores; refuses N that does not fit
scripts/check-resources.sh  memory budget + "every service pinned and limited" check
scripts/pin-images.sh     resolve tags -> digests into compose.pinned.yaml
<platform>/compose.yaml   the stack (header lists every deviation from the official compose)
<platform>/.env.example   versions, resource layout, tuning knobs (copied to .env by make)
<platform>/haproxy/       LB config
<platform>/scripts/smoke.sh
edgex/config/core-data.yaml   official core-data config + NATS QueueGroup
fiware/mongo/init-indexes.js  entity indexes from Orion's perf-tuning guide
thingsboard/tb-transports/    the HTTP transport image's own default config, mounted at /config
yaade/                    Yaade collections (provision → send → read back) and the scripts that build/run them
workloads/k6/             open-model generator (gen.js + adapters) run by benchctl (docs/generator.md)
workloads/plans/          generator plans (smoke, calibration, Poisson)
cmd/, internal/           Go: benchctl, fixture-target, plan/runner/report/reconcile/arrivals
tests/                    feasibility probes and Docker integration tests
docs/                     k6-feasibility, generator, calibration, generator-status, design-decisions
workloads/                (first-generation smoke workloads: run.sh + *.js, not confirmatory)
monitoring/               Prometheus + Grafana (load generator) and monitoring/exporters (SUT)
```

## Sources

- ThingsBoard cluster compose: github.com/thingsboard/thingsboard, branch `release-4.3`, `docker/`
- ThingsBoard licence change: thingsboard.io/blog/one-thingsboard-source-available/
- EdgeX compose: github.com/edgexfoundry/edgex-compose tag `v4.0.2` (incl. `compose-builder/Makefile` NATS note)
- EdgeX override logic: go-mod-bootstrap v4.0.5 `bootstrap/config/config.go`, `bootstrap/environment/variables.go`
- EdgeX NATS queue groups: go-mod-messaging v4.0.3 `internal/pkg/nats/`; docs.edgexfoundry.org/4.0/microservices/general/messagebus/
- Orion compose + tuning: github.com/telefonicaid/fiware-orion `4.5.0` `docker/`, `doc/manuals/admin/perf_tuning.md`, `cli.md`
- IoT Agent UL: github.com/telefonicaid/iotagent-ul tag `3.7.0`
