# Qualification continuation from 9c7fc5f — 2026-10-31

**Main campaign: NO-GO. Session blocked by SUT network reachability, not by an SLO decision.**
No push, database reset, device deletion, volume removal or platform-version change was performed.

## Repository and provenance

Initial checkout: clean `chore/ci-and-release` at `9c7fc5f`; no newer work needed reconciliation.
The new controlled receiver and six 1,000-device plans were checkpointed at **b67fe86** before
building/deploying. Both hosts received a fresh tree at `/root/iot-qual-v3-20261031`, not an overwrite
of the old verification tree. New manifests correctly name
`b67fe860199e7479427cec5036091e6318ea4f6a`, `dirty=false`, and retain executed k6 script hashes and
image digest. Old stale-GIT_REV manifests are untouched.

Deployed static binary SHA-256:

```
70dd430f9258567607979f9153a4c2324349d39c83e88d4904743717f2880d96  benchctl
4e67e2daca0e1158d1f5491ccb01d4a4bf508b5d68213b94860b16fcdf0dc4d4  sink
```

Evidence root: **`results/qualification-next-20261031/`**. Existing rehearsal and previous
qualification evidence was not modified.

## 1. Actual adapters / 1,000-device headroom attempts

The unmodified ThingsBoard/FIWARE/EdgeX adapters in `workloads/k6/lib/adapters.js` were selected by
the plans, not replaced by the fixture adapter. `scripts/adapter-receiver.py` implements the exact
setup/ingress API subset used by those adapters:

- ThingsBoard login, synthetic bearer authorization, device creation, credential retrieval,
  telemetry URL token lookup, and telemetry JSON decoding;
- FIWARE service-group/device provisioning, API-key/device lookup, and UltraLight decoding;
- EdgeX existing-profile lookup, device lookup/provisioning with per-item 207 status, readiness
  probe handling, and Object-reading JSON decoding.

This reproduces serialization, URLs, request/header construction, setup API calls, credential
selection, response parsing and keep-alive behavior. It **does not** implement actual platform
authentication/authorization, database persistence, rule processing, platform queues, notifications
or historical visibility. The EdgeX controlled profile already exists; real profile creation is
not emulated. All credentials are synthetic. Receipt timestamps use SUT CLOCK_REALTIME; raw body,
path, run_id, seq and logical device identity are recorded before the optional response delay.
Output uses create-exclusive mode and flushes each record; recording errors return HTTP 507 and
are counted. Each receiver is source-IP restricted to the load host and loopback.

Isolation: all running `tb`, `edgex`, `fiware`, `fiwhc` containers and `diag-pgcnt-test` were stopped
before tests. The four existing SUT exporters remained running. Monitoring/Grafana/yaade and the
original ceiling sink on the load host were retained. New receivers were pinned to SUT cores 6/7,
ports 18083/18084 (immediate/700 ms). Generator: load host cores 0-1, GOGC=400, HTTP/1.1 keep-alive,
no retries, Prometheus remote-write active, normal outcome logging. Plans use 1,000 round-robin
logical devices, 15 s warm-up at 100/s and 120 s measurement at 400/s; delayed plans use 700 VUs.

| Run (`adapter1000/` subdirectory) | Observed result | Qualification |
|---|---|---|
| `P3-thingsboard-immediate-20261031` | 48,000/48,000 measured sends acknowledged, zero drops/failures/unknowns; lag p99 3.5 ms, scheduled-to-ack p99 7 ms | **Generator VALID**; receiver identity reconciliation unavailable until SUT ledger is retrieved |
| `P3-thingsboard-delay700-20261031` | 45,122/48,000 measured attempts, 43,022 acknowledgements, 2,100 unknown outcomes, 3,278 drops; lag p99 6,511 ms | **INVALID**; network failure during run; no capacity claim |
| `P3-fiware-immediate-20261031` | Setup request timed out; zero measured sends | **INVALID**, infrastructure/setup, not platform failure |
| `P3-fiware-delay700-20261031` | Setup aborted; zero sends | **INVALID**, infrastructure/setup |
| `P3-edgex-immediate-20261031` | Setup GET failed with no route to host; zero sends | **INVALID**, infrastructure/setup |
| `P3-edgex-delay700-20261031` | Setup aborted; zero sends | **INVALID**, infrastructure/setup |

The delayed TB run's unknown outcomes **may have been recorded at the receiver before the client
timed out**. Do not classify them as lost; retrieve the receipt ledger and retain this relationship.
Dropped iterations make the exact planned-seq mapping unavailable for that run. All INVALID
bundles are preserved, and none are relabelled platform FAIL.

### Reachability failure and bounded retries

Node/cAdvisor were last scraped successfully around **18:48:35 UTC** and down around **18:48:40 UTC**.
The load host remained reachable directly; its route to `87.107.191.107` reported **no route to
host**. Direct workstation→SUT SSH timed out; reversing the jump through the load host also failed.
Three later bounded SSH attempts at 18:58:14, 18:58:49 and 18:59:24 UTC timed out. No further
performance workload was launched after identifying the infrastructure failure.

Last available telemetry: SUT MemAvailable 15,342,989,312 bytes; OOM-kill counter remained 34;
load1 0.79. These samples do not prove what happened after telemetry stopped. Do not attribute
the outage to platform saturation, receiver overload, OOM, or a provider fault without additional
evidence. Raw `up`, memory, OOM and load responses are under `network-diagnostic/`; connection
errors are preserved in `network-failure-load.txt` and k6 logs.

**200/s ceiling: not newly qualified.** The prior 100-device fixture results retain their scope.
The new immediate TB generator result alone cannot establish the common headroom requirement.
No lower common ceiling can be derived from an infrastructure outage; retain the approved 200/s
as conditional, not authorized for the main campaign.

## 2. CPU capture work

Implemented/tested locally, not yet deployed or live-qualified on the platforms:

- Extended snapshot TSV with compose service and explicit application/supporting tier. Snapshot
  wall time is taken separately per container; original six-column snapshots remain readable.
- `internal/cpubound` now reports CPU-seconds by tier as well as total and the existing explicitly
  named per-message denominator. Counter delta remains `(after_usec-before_usec)/1e6`.
- `scripts/cpu-capture-server.py` is a read-only, source-restricted helper exposing `/snapshot`,
  `/inventory` and `/clock` for operator-fixed projects. FIWARE must include **fiware + fiwhc**, with
  only the required candidate QL/Timescale services running; candidate Orion/Mongo are not the
  benchmark ingestion path and should remain stopped during a full-path test.
- `scripts/clock-bound.py` measures SUT-minus-load realtime offset intervals from the load host,
  retaining paired wall-clock and monotonic RTT samples. It does not invent a timing threshold.
- `scripts/verify-cpu-boundary.py` independently recomputes integer microsecond deltas, checks
  exact inventory membership/IDs/StartedAt, and compares per-tier and total tool outputs.

Known-answer/reset/changed-ID/missing-container tests from the previous implementation still
pass, and a new independent application/supporting sum test passes. Exact **phase-specific** CPU
cost remains unavailable when captures only enclose the phase. Preserve the actual capture interval,
clock domain, skew and offset uncertainty; do not describe it as an exact phase interval.

No new platform CPU run was started: reachability failed before that stage. There is no new
platform CPU-cost number or retrospective backfill of old rehearsals.

## 3. Kafka visibility

Observed root cause before the outage: exporter container **absent**, despite an existing Kafka
scrape target and cached `danielqsj/kafka-exporter:v1.9.0` image. Target health was down, port 9308
connection refused (`kafka/targets-before.json`). The existing dashboard expression uses
`kafka_consumergroup_lag` gated by `up{job="kafka"} == 1`; it correctly returned NO_DATA.

Added a targeted `make kafka` exporter target that starts only the Kafka exporter, without
recreating always-on monitoring or removing other exporters. **Not executed while the SUT was
unreachable.** On recovery, with TB isolated and Kafka healthy:

```bash
cd /root/iot-bench-deploy-cand-auto20261027/monitoring/exporters
docker compose -f compose.yaml -f compose.thingsboard.yaml up -d --no-deps kafka-exporter
curl -fsS http://127.0.0.1:9308/metrics
```

Then verify Prometheus target UP, discover real topic/group/partition labels, compare lag with
Kafka consumer-group committed/end offsets under bounded traffic and through drain. Unit is
**records/offset distance per consumer group/topic/partition**, summed by group/topic in the
dashboard, not end-to-end pending messages. Kafka visibility remains **unavailable**, not healthy
zero, until that live check. Sink/history identity deficits can independently describe observations
but are not a substitute measurement of Kafka backlog; a flat queue alone does not prove health.

Also fixed source monitoring coverage: include `fiwhc` cAdvisor/container-info series as FIWARE,
and classify EdgeX `app-http-export` as application tier, consistent with its resource allocation.
`promtool` accepts the configuration and rules. These changes are **not deployed**, and historical
series are not relabelled/backfilled.

## 4. FIWARE history extension and scaling

No new full-stack FIWARE rate test could start after SUT access failed. Existing scope remains:
full HAProxy→IoT Agent→Orion→sink/QL-history reconciliation at 25/s, store-side probe at 50/s.
Above 50/s is still unqualified; no rate boundary is inferred from the failed controlled-receiver
setup. Orion 4.5.0 / QL 1.0.0 compatibility remains the measured local scope documented earlier,
not an upstream support or high-rate adequacy claim.

`resource-fit-rendered.jsonl` retains sanitized effective compose resource/image-tag renders
for k=1,2,3, with TB core/rule memory explicitly 1280 MiB. `resource-fit-summary.json` includes
the additional FIWARE history 2048 MiB, conservative 896 MiB monitoring allowance and 1536 MiB
OS reserve against recorded host MemTotal. All nine combinations fit **arithmetically**.
This does **not** establish image-digest, startup, heap or smoke qualification at those scales.

| Platform | k=1 | k=2 | k=3 |
|---|---|---|---|
| ThingsBoard | Render fits; new live smoke unavailable | Render fits; previous k=2 rehearsal retained, no new smoke | Render fits; live qualification unavailable |
| FIWARE + QL/Timescale | Render fits incl. history; previous rehearsal retained | Render fits; live qualification unavailable | Render fits; live qualification unavailable |
| EdgeX | Render fits; previous rehearsal retained | Render fits; live qualification unavailable | Render fits; live qualification unavailable |

Removed `down -v` from EdgeX's qualification prepare helper. Preparation now shrinks to one
replica without discarding data, then provisions/reuses dedicated devices before scale-out. Safety
regression test passes. This fix was **not run against retained SUT devices** in this session.
The original remote helper may still contain `down -v`: **do not invoke that remote prepare
script until the patched source has been deployed and inspected.**

## 5. Evidence / analysis commands

```bash
go test ./internal/... ./cmd/...
go test -count=1 -v ./tests/integration
python3 tests/adapter_receiver_test.py
python3 tests/resume_safe_provision_test.py
python3 scripts/verify-run-export.py results/qualification-next-20261031/graphs/*
```

Two new CSV/PNG/SVG/offline reports use actual scenario timestamps, not run names, with 60 s
context. `graph-windows.json` records the VALID/INVALID generator labels and explicitly unavailable
receiver reconciliation. The four aborted setup runs have no measurement timestamps, so no
measurement graph window was invented for them.

After receiver recovery, reproducible reconciliation is:

```bash
python3 scripts/verify-adapter-qualification.py \
  --runs results/qualification-next-20261031/adapter1000 \
  --receiver RECOVERED-immediate.ndjson --receiver RECOVERED-delay.ndjson
```

Do not run that command with empty stand-ins and report the result as measured loss. Preserve
raw receiver records, including negative-seq EdgeX readiness probes (the analyzer excludes probes
from workload identities). Actual binary/script/image provenance is retained in each manifest.

## 6. Services, recovery and rollback

**Current SUT state cannot be confirmed while it is unreachable.** Last confirmed actions:

- Stopped only the existing benchmark projects tb, fiware, fiwhc and edgex, plus diag-pgcnt-test,
  preserving containers/volumes. Existing four exporters were left running.
- Started two Python controlled-receiver processes under `/root/iot-qual-v3-20261031/receiver/`,
  with PIDs recorded in `immediate.pid` and `delay.pid`, receipt paths `immediate.ndjson` and
  `delay.ndjson`, and logs with matching stems. Their last/final state is unknown, not assumed stopped.
- The new CPU helper was never started. No old qualification receiver was restarted.
- Load host remains reachable **directly** using the existing key, without the failed jump:
  `ssh -i ~/.ssh/id_ed25519_vps root@87.107.191.32`. Its monitoring and yaade remain running;
  all six new k6 runs finished. The original ceiling sink was not stopped.

On restored SUT access, before restarting any test:

1. Record hostname/uptime, Docker state, kernel/journal/network evidence around 18:48 UTC.
2. Verify each PID file against `ps -p PID -o args=`. Only if it matches the corresponding
   `/root/iot-qual-v3-20261031/scripts/adapter-receiver.py` process, send SIGTERM and wait for
   shutdown/flush. Retrieve both receipt files and logs without truncating them.
3. Retrieve `/root/iot-qual-v3-20261031/state/original-running.txt` and preserve it in evidence.
   To restore the **exact pre-test running containers**, after checking no test is in flight:
   `xargs -r docker start < /root/iot-qual-v3-20261031/state/original-running.txt`.
   This is a rollback to the pre-session service state, not permission to run performance tests
   with all stacks active. For qualification, start one platform only and keep the rest stopped.
4. Use new names/run IDs/output files for retries. Neither the stopped old Go receiver containers
   nor the new Python receipt paths should be reused. Never run `down -v` or reset a database.

## 7. Resume order and genuinely open choices

Infrastructure recovery is the immediate technical blocker. Then retrieve receiver ledgers,
diagnose the delayed-run interruption, finish the six adapter cases plus receiver/network
headroom, and only then qualify each platform's CPU captures and approved scaling configurations.
Enable/validate Kafka observing bounded TB traffic. Extend isolated full-path FIWARE runs through
the approved rates only while offered-load gates stay valid; define and preserve drain deadlines,
late observations and history-export error rows. No SLO/profile decision is needed to do this work.

Owner choices remain only the previously pending profile adoption/application profiles and related
methodology decisions. k, memory limits, common rate sweep, conditional ceiling and protocol are
unchanged. **No new capacity or campaign GO is justified by this session.**
