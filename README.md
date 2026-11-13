# IoT platform benchmark deployments (replica-scalable, fixed resources, HTTP only)

One Docker Compose stack per platform. Every stack has the same shape:

```
load generator ──HTTP──► HAProxy (always present, also at N=1) ──► replicated tier(s) ──► single stateful tier
```

The deployment tooling supports a shared application CPU pool covering `4 … 4+N-1`
(see "Resource layout"). Configuration fit is not performance qualification.
The current owner-approved comparison is deliberately fixed at **N=2/k=2, pool
`4-5`, 25 msg/s, 1,000 devices and payload selector 0**, not a scaling experiment.

> **State of this repository (2026-11-07):** benchmark tooling, reviewed bounded qualification,
> and a **partially executed ordinary campaign**. Two ordinary attempts are validated; one
> setup failure is preserved and another attempt's remote artifacts await reconciliation.
> Runtime work is paused for repository handoff. No completed campaign, capacity or scalability
> claim is made. Current evidence, limitations and the exact continuation gate live in
> [`docs/status.md`](docs/status.md). Per-session narratives are archived under
> `research/archive/sessions/` and may contradict each other; `docs/status.md` wins.

Start with the [quickstart](docs/quickstart.md) if you are new, then the
[documentation index](docs/README.md),
[publication/handoff checklist](docs/operations/publication-handoff.md) and
[operator runbook](docs/operations/ordinary-campaign.md).
**Do not use `make clean` or `down -v` between retained campaign attempts.**

Only one platform can run at a time on the SUT (they share port 8404, and the platform pool cores).

| Platform | One instance = | Not replicated (by design) | LB ports |
|---|---|---|---|
| FIWARE | `orion` + `iot-agent` | MongoDB 8.0 | 1026 Orion · 4041 IoTA north · 7896 UL/HTTP |
| EdgeX 4.0.2 | `core-data` + `device-rest` | PostgreSQL, NATS, core-keeper, core-metadata | 59986 ingress · 59880 core-data · 59881 metadata |
| ThingsBoard 4.3.1.6 | `tb-core` + `tb-rule-engine` + `tb-http-transport` | PostgreSQL, Kafka, ZooKeeper, Valkey | 8080 REST + HTTP device API |

All stacks expose HAProxy stats on `:8404/stats` and Prometheus metrics on `:8404/metrics`. Every tier is its own
container, so per-tier CPU/memory can be monitored separately (`docker stats`, cAdvisor, ...). HAProxy discovers
replicas through Docker DNS (`server-template`, up to 16 per tier) within ~5 s.

## Build and verify locally (no benchmark servers)

The load generator and the monitoring stack run on **ramonavm1**, the platforms on **ramonavm2** (the SUT).

Requires the Go toolchain specified in `go.mod`, Python 3 and Node.js for offline
adapter tests. Docker/Compose is needed for deployment; cloning is not permission
to deploy or generate traffic. From the repository root:

```bash
go build ./cmd/...
go test -short -count=1 ./internal/... ./cmd/... ./tests/...
python3 -B -m unittest discover -s tests -p '*_test.py'
python3 -B -m unittest discover -s tests/offline -p 'test_*.py'
node --test tests/offline/*.test.mjs
bash scripts/check-preregistration.sh
```

Official execution uses the [frozen ordinary matrix](results/feasible-campaign-20261106/run-matrix.json)
and reviewed launcher, with the [dispatch contract](docs/methodology/dispatch-contract-20261106.md),
automatic per-message offline analysis and [live progress reports](docs/development/generator-progress.md).
Read the runbook first: existing deployments, review receipts and external evidence are
required. Legacy `workloads/legacy/run.sh` results retain their historical exploratory scope.

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
  ThingsBoard's base descriptors use separate JVM limits; the qualified ordinary deployment
  explicitly uses **1280 MiB core + 1280 MiB rule-engine + 768 MiB transport** per replica.
  Effective run-time resources and image identities, not this overview, govern eligibility.
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
   notification sink (`cmd/sink`, configured by `scripts/outbound.sh`); `make smoke` does not exercise it. This is a
   known gap, not a pass.

Historical smoke helpers can race cold-start initialization even after HAProxy
reports UP. A fixed sleep is **not** ordinary-campaign readiness. Use the actual
bounded backend, initialized-ingest-route and all-device replica gates in the
[operator runbook](docs/operations/ordinary-campaign.md); do not measure merely
because a service ping succeeds or 30 seconds elapsed.

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

## Historical qualification summary (2026-10-29)

The following is historical, not the current campaign status. See [`docs/status.md`](docs/status.md)
for the narrowed release, preserved failures and remaining official attempts.

- All three stacks boot and smoke-test on the SUT (ramonavm2, 8 vCPU / 16 GB) and are exercised end to end from
  the load-generator host with the Yaade collections in `yaade/`; boot fixes are listed in the compose headers.
- ThingsBoard (k=2, 1280 MiB, qualified under load 50/200 r/s), the generator ceiling (≤ 3200 r/s loopback,
  recommended campaign max ≤ 1600), the FIWARE history pairing (Orion 4.5.0 → QL 1.0.0, 2048 MiB adopted) and the
  EdgeX disk+WAL method are live-verified — all as **qualification**, not campaign, evidence.
- The main benchmark campaign is **not** executed; per-claim state, evidence and limitations:
  [`docs/status.md`](docs/status.md) (authoritative), `research/qualification/qualification-blockers.md` (row-level record).

## Known limitations / open decisions

- **ThingsBoard partitions.** Kafka topic partition counts cap how many core/rule-engine replicas can do useful
  work. The defaults (10) are above N_max = 4.
- **ThingsBoard install** can load demo data (`TB_LOAD_DEMO=true`) so smoke tests have a tenant.
  The retained campaign does not delete existing data; report actual deployment state.
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
deploy/<platform>/compose.yaml   the stack (header lists every deviation from the official compose)
deploy/<platform>/.env.example   versions, resource layout, tuning knobs (copied to .env by make)
deploy/<platform>/haproxy/       LB config
deploy/<platform>/scripts/       smoke + provisioning helpers
deploy/edgex/config/core-data.yaml   official core-data config + NATS QueueGroup
deploy/fiware/mongo/init-indexes.js  entity indexes from Orion's perf-tuning guide
deploy/thingsboard/tb-transports/    the HTTP transport image's own default config, mounted at /config
deploy/fiware/fiware-history-candidate/  explicit EXPERIMENT: isolated FIWARE history tier (see docs/architecture/fiware-history-design.md)
yaade/                    Yaade collections (provision → send → read back) and the scripts that build/run them
workloads/k6/             open-model generator (gen.js + adapters) run by benchctl (docs/methodology/generator.md)
workloads/plans/          generator plans (smoke, calibration, Poisson)
workloads/legacy/         first-generation run.sh + *.js — exploratory harness, kept with documented limits
cmd/, internal/           Go: benchctl, fixture-target, plan/runner/report/reconcile/arrivals
tests/                    feasibility probes and Docker integration tests
docs/                     status (authoritative), architecture, methodology, qualification, operations, development, archive
evidence/                 integrity manifests + sha256 list + source excerpts for results/**
research/                 preregistration archive, protocols, findings
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
