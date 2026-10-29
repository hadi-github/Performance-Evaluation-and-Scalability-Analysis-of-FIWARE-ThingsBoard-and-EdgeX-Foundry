# Hypotheses (written and committed before the runs that test them)

Each entry: the hypothesis, the run(s) that test it, the expected outcome, and what would refute it. Results go in
`docs/operations/diagnostics.md`; an entry here is never edited after its runs start (corrections are appended).

## Diagnostics round 1 (2026-10-22), runs `DIAG-*`, before any tuning

Basis: `docs/methodology/exploratory-capacity.md`, the D23 persistence re-evaluation and the demand fits
(`scripts/demand-fit.py`), and the source reading in `evidence/source-excerpts.md`.

### EdgeX

**H-E1: the persistence ceiling is the commit latency of one serial transaction per event per replica.**
core-data persists from one goroutine per replica, one transaction per event (INSERT event, COPY readings,
COMMIT), with no knob for concurrency or batching (edgex-go v4.0.2, excerpts). Throughput per replica is then
1 / (time per event): ~7.4 ms at k = 1 (~135/s), ~10 ms at k = 4 (~100/s per replica, 4 replicas).
- Runs: `DIAG-edgex-k1-r200`, `DIAG-edgex-k4-r500` (durable, `pg_stat_statements` + `track_wal_io_timing`), then
  `DIAG-edgex-k1-r200-nosync`, `DIAG-edgex-k4-r500-nosync` (`synchronous_commit=off`, DIAG only).
- Expected (durable): commits/s ≈ persisted events/s; one busy connection per core-data replica; WAL sync time per
  commit is a large share (≥ 30 %) of the time per event; `pg_stat_activity` samples of the core-data connections
  dominated by WAL wait events (`WALSync`/`WALWrite`) or client round-trip waits (`ClientRead`), not CPU.
- Expected (`synchronous_commit=off`): persisted throughput rises by ≥ 1.5x (k = 1 keeps up with 200/s; k = 4
  keeps up with 500/s).
- Refuted if: `synchronous_commit=off` raises the persisted rate by < 1.2x. Then commit latency is not the
  cause, and the round trips / per-event Postgres and Go work are.

**H-E2: the acknowledged-but-lost readings at k = 1 (EXPL-edgex-k1) are NATS slow-consumer drops at the default
64 MiB pending limit.** go-mod-messaging v4.0.3 sets no pending limits; nats.go's defaults are 500,000 messages /
64 MiB per subscription. Drops began when ~72k events were pending.
- Run: any EdgeX DIAG run: NATS `in_bytes / in_msgs` over the step.
- Expected: ~0.9 KB per event envelope (64 MiB / ~72k ≈ 930 B), i.e. the byte limit is hit before the message limit.
- Refuted if: the mean envelope is < 0.5 KB or > 1.5 KB (then the 72k coincidence is not the byte limit).

### ThingsBoard

**H-T1: Kafka's ~0.95 cores at any rate is idle polling by many consumers with 25 ms poll intervals.**
`thingsboard.yml` v4.3.1.6: core, rule-engine, transport and notification queues poll every 25 ms, with 10
partitions each and one consumer per partition.
- Run: `DIAG-tb-idle` (5 min, no load, after start-up), Kafka JMX request metrics (DIAG-only JMX port).
- Expected: Kafka uses ≥ 0.7 cores with no load; Fetch requests ≥ 1,000/s; ≥ 40 consumer group members.
- Refuted if: idle Kafka < 0.3 cores (then the flat ~1 core comes from load-dependent work).

**H-T2: the rule-engine OOM is heap size, not container memory.** `-Xmx512m` inside a 1.5 GiB container.
- Run: `DIAG-tb-obs-sink-r100`: effective JVM flags and GC logs.
- Expected: effective `MaxHeapSize` = 512 MiB in all three JVMs; old-generation occupancy after GC grows with
  load; GC time is a measurable share (≥ 2 %) of wall time already at 100 msg/s in tb-rule-engine.
- Refuted if: `MaxHeapSize` differs from 512 MiB (flags not applied).

**H-T3: Valkey's ~2 ms CPU per message comes from several cache commands per message.**
- Run: `DIAG-tb-obs-sink-r100`: `INFO commandstats` before/after ÷ messages.
- Expected: ≥ 3 Valkey commands per message.
- Refuted if: < 1 command per message (then Valkey CPU is not per-message work).

### FIWARE

**H-F1: the IoT Agent does ≥ 2 MongoDB reads per measure (device registry and configuration group, no cache).**
iotagent-node-lib 4.7.0 has no cache; iotagent-ul 3.7.0 `retrieveDevice` looks up the device by id and then the
group.
- Run: `DIAG-fiware-k4-r100` with the MongoDB profiler for 60 s.
- Expected: ≥ 2 IoT Agent DB operations per message, plus ≥ 1 Orion entity write per message.
- Refuted if: < 1 IoT Agent DB operation per message.

**H-F2: MongoDB's ~9 ms CPU per message comes from at least one unindexed or poorly selective query.**
- Run: same profile.
- Expected: a COLLSCAN, or a plan with `docsExamined / nreturned` ≫ 1, on a per-message query.
- Refuted if: every per-message query is an IXSCAN with `docsExamined / nreturned` ≤ 2.

**H-F3: there is no duplicate processing in FIWARE.**
- Run: same run: Orion incoming update count (all replicas) vs messages sent over the same window.
- Expected: updates = messages sent (± provisioning requests).

**H-F4: the IoT Agent OOM is backlog growth in one Node process per container, not process count.**
- Run: process model check (`docker top`, `nproc`, Node's CPU count) and RSS under load.
- Expected: one node process per container (`IOTA_MULTI_CORE=false`); RSS well below 512 MB at 100 msg/s.

### Observer overhead (Step 4)

**H-O: the sink path costs CPU of the same order as the platform's own per-message work on EdgeX, and less on the
others.** Runs at ~50 % of the exploratory capacity, with and without the sink path:
`DIAG-edgex-k4-obs-{sink,nosink}-r200`, `DIAG-tb-obs-{sink,nosink}-r100`, `DIAG-fiware-obs-{sink,nosink}-r50`.
- Expected: EdgeX export service 1.6 ms/msg ± 25 % (fit); ThingsBoard REST node adds 0.3-2 ms/msg in
  tb-rule-engine; FIWARE subscription adds 0.3-2 ms/msg (Orion + MongoDB).

## Appended 2026-10-22, before run `DIAG-fiware-k4-r100-cpuprof` (after the FIWARE profile results)

**H-F5: the IoT Agent's Node CPU per measure (~22 ms per measure handled by a replica, plus ~0.15 cores per
replica at idle) is dominated by the per-measure registry round trips through mongoose (two device lookups and the
`oldCtxt` save, iotagent-node-lib 4.7.0 `ngsiService.js:74-76`, `entities-NGSI-v2.js:659-662`,
`deviceRegistryMongoDB.js:400-440`), not by UL parsing or the NGSI request to Orion.**
- Run: `DIAG-fiware-k4-r100-cpuprof`: V8 CPU profile (inspector, 60 s) of `fiware-iot-agent-1` at a steady
  100 msg/s; no MongoDB profiler.
- Expected: ≥ 30 % of non-idle samples in mongoose / mongodb driver / bson frames (self + children, by module path);
  < 20 % in the UL parser and the HTTP client to Orion together.
- Refuted if: mongoose/mongodb/bson < 15 % of non-idle samples.
