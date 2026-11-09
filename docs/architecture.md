# System design

This guide explains the benchmark's architecture and the reasons behind it.
It summarizes the existing design; it does not introduce a new protocol or
qualify an untested configuration. For execution progress and evidence limits,
read [current status](status.md).

## 1. What the benchmark compares

The system compares HTTP telemetry processing in FIWARE, ThingsBoard and EdgeX
under explicitly recorded resource budgets. It observes external delivery and
independently retrieved history rather than treating an HTTP acknowledgement
as successful end-to-end processing.

The bounded ordinary comparison uses 25 messages/s, 1,000 logical devices,
N=2/k=2, a 300-second warm-up and 600-second measurement. These are
project-selected experimental settings, not universal IoT requirements.
The [frozen scope](../results/feasible-campaign-20261106/frozen-scope.json)
defines the configuration; this is not a capacity sweep or cluster-scaling study.

## 2. Two hosts, separate responsibilities

```text
Load and measurement host                     System under test (SUT)

benchctl + k6 ---- HTTP ---------------------> HAProxy
                                                |
                                                v
                                             application services
                                                |          |
                                                v          v
external sink <---- outbound HTTP -------- delivery path   history backend
      |                                                    |
      +---- receipt ledger          read-only history export+
                  \                         /
                   +--- offline analysis --+
```

The SUT has 8 vCPUs and 16 GB RAM. The separate load host runs the generator,
receipt sink and monitoring. Keeping them off the SUT avoids charging generator
and measurement CPU to a platform. It does not eliminate measurement overhead:
the load host must also be qualified and its resource use retained.

Generator and sink share the load-host clock. This avoids subtracting SUT and
load-host clocks for the primary latency, but clock discontinuities and capture
completeness still require checks. Administrative SSH and history retrieval are
control operations, not the measured telemetry path.

## 3. Resource budget is not replica count

| SUT cores | Responsibility |
|---|---|
| `0` | Host services, Docker and monitoring/exporter work |
| `1` | HAProxy |
| `2-3` | Supporting services: databases, brokers and other infrastructure |
| `4` onward | Shared application CPU pool |

**k** is the number of cores in the application pool. **N** describes the
deployment's replica setting; actual replicas must also be recorded per service.
The ordinary configuration ties N=2 to k=2, pool `4-5`, but the concepts are not
interchangeable. Multiple application containers share that pool; each replica
does not own an exclusive core.

Containers have explicit memory limits and swap limits. Equal application CPU
does not imply equal memory, database cost or delivery guarantees. Report the
effective limits and topology, including HAProxy and supporting services, rather
than claiming identical platform internals. The ordinary ThingsBoard deployment
uses 1280 MiB core, 1280 MiB rule-engine and 768 MiB transport per replica;
base example settings alone do not establish the effective run configuration.

Only one platform runs at a time. This avoids competition for the shared CPU
pool and ports. It also means results describe controlled single-SUT behavior,
not physical multi-node horizontal scaling.

## 4. Why HAProxy is present at every configuration

HAProxy provides the ingress endpoint and replica discovery. Keeping it present
even at N=1 avoids comparing a direct single-instance path with a load-balanced
multi-instance path. Its hop and resource cost remain part of the design.

This controls one source of variation; it does not make the three platforms'
acknowledgement, queueing or persistence semantics equivalent.

## 5. Platform paths and deliberate differences

### FIWARE: current context plus explicit history

```text
HTTP -> HAProxy -> IoT Agent UL -> Orion
                                  |-- subscription -> external sink
                                  +-- subscription -> QuantumLeap -> TimescaleDB
```

Configured versions are Orion 4.5.0, IoT Agent UL 3.7.0 and MongoDB 8.0, with
QuantumLeap 1.0.0 and TimescaleDB/PostGIS 2.3.0-pg13 for history. Orion's current
entity state is not an event history. QuantumLeap/TimescaleDB is therefore a
named, separately accounted downstream history tier, not an inference from the
baseline MongoDB store. Cygnus is not the selected history component.

Sink delivery and history processing are parallel subscriptions: either may
succeed without proving the other. The isolated history Compose file also
contains its own pairing-test services; it must not be mistaken for instructions
to run a second complete ingestion stack during the ordinary comparison.

The local Orion/QuantumLeap pairing is measured compatibility, not an upstream
support guarantee. See the [FIWARE path and compatibility record](architecture/fiware-benchmark-path.md)
and [history design](architecture/fiware-history-design.md).

### ThingsBoard: queued processing and configured rule-chain export

```text
HTTP -> HAProxy -> HTTP transport -> queued core/rule processing
                                      -> Save Timeseries -> success route -> sink
```

ThingsBoard is pinned to 4.3.1.6. Its supporting services include PostgreSQL,
Kafka, ZooKeeper and Valkey. The repository records the 4.3.x licence scope as
the reason for not treating a newer release as an automatic replacement.

The configured rule-chain success route and independent history export are
different observations. An ingress HTTP response is not a per-message durable
commit signal. Release-specific claims must use the pinned configuration and
source, not assume newer online documentation applies unchanged.

### EdgeX: competing consumers and a parallel export path

```text
HTTP -> HAProxy -> device-rest -> core NATS -> core-data -> PostgreSQL
                                      +-> application HTTP export -> sink
```

EdgeX 4.0.2 services use the official `include_nats_messaging` build flag without
platform source changes. Core-data queue groups provide competing consumers
instead of every replica independently persisting the same event. Core NATS is
not JetStream and provides no durable message backlog.

The export application and core-data observe separate bus paths. Sink receipt
does not prove database visibility. The ordinary runbook also documents
provision-at-one-replica preparation before scaling; starting replicas is not
enough to establish that all devices are usable on every intended backend.

## 6. Generator, adapters and identity

The k6 generator uses an open constant-arrival model: target arrivals are not
reduced just because platform responses get slower. Enough VUs, zero dropped
iterations and valid actual dispatch distribution are still required; an
open-model configuration alone does not prove the intended load occurred.

Adapters translate a reproducible logical message into each platform's HTTP
format. Identity is the pair `(run_id, seq)`, with device and schedule metadata.
This permits exact joins between planned requests, outcomes, receipts and
history. Equal row counts alone cannot detect a missing message offset by a
duplicate. Canonical content also does not imply identical native wire sizes;
encoding and transport overhead must be reported separately.

See [generator details](methodology/generator.md) and the
[measurement guide](methodology.md) for accounting and endpoint definitions.

## 7. Evidence, monitoring and design limits

Prometheus and Grafana explain operating conditions; dashboard curves are not
substitutes for identity ledgers or exact CPU counter boundaries. Offline
analysis retains missing, late, duplicate and unknown outcomes instead of
silently filtering them from a latency chart.

Source, tests and frozen controls belong in Git. Raw run evidence and private
review receipts require separately retained, checksummed bundles. A successful
clone or unit test does not reproduce a scientific result without those inputs.

This design does not by itself establish maximum capacity, crash durability,
safety-critical timeliness, exact wire-to-wire latency or cluster scalability.
See the [operator runbook](operations/ordinary-campaign.md),
[publication boundary](operations/publication-handoff.md) and
[historical decision log](architecture/design-decisions.md) for the detailed
controls and rationale. Existing core documents remain preserved as references.
