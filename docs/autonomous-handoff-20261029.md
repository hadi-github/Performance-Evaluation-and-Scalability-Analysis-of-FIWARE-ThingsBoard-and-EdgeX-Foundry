# Autonomous session handoff (auto-20261029, close-out 2026-10-29)

## Verified results vs proposals

| Item | Status |
|---|---|
| Interpretation correction (50/s valid; 100/s = VU starvation; no platform bracket; 1280 MiB promising; clocks unverified) | applied (`docs/interpretation-correction-20261029.md`) |
| **Generator-only ceiling** | **VERIFIED: 3200 r/s sustainable (2 consecutive VALID passes), 6400 INVALID**; sink qualified AT 3200 r/s via the real path (240k msgs, 0 unparsed/overflow). **Recommended max campaign rate ≤ 1600 r/s** (½ rule), subject to cross-host path + platform rate |
| **1280 MiB TB qualification** | **VERIFIED: 200 r/s VALID twice** (0 dropped, stored==sent, p99 330 ms / 1.47 s), 400 r/s INVALID (saturation, p95 = 10 s timeout); memory 76–79% under load; zero OOM/restarts. **VU sizing lesson: yesterday's 2 s tails were VU-queuing artifacts** |
| **Memory budget k=1..4** | VERIFIED table: 6,912 / 9,984 / 13,056 ✓; **k=4 16,128 ✗ (overflow 1,280 MiB)**; driver = raised per-replica limits vs the 768 MiB headroom at old limits; owner options O1–O5 (O4 = cap k=3 per adopted S4 fallback) |
| **FIWARE history tier under load** (ADOPTED 2048 MiB) | **VERIFIED: 3,001/3,001 identities at 50 r/s** (0 missing/dups/unrecoverable); timescale 137/1536, QL 77/512 under load; higher-rate adequacy open |
| **EdgeX non-empty history** | **VERIFIED: 600/600 identities** reconciled exactly (gate-contract readings); B14 complete on all three platforms |
| **Clock probe (corrected)** | **VERIFIED bound: load↔SUT offset ∈ [−63, +50] ms** (interval-intersection, 35 pairs/leg; both hosts NTP stratum-2; the workstation's own clock is ~1.36 s off — never a reference). Cross-host claims stay **unqualified** (U=63 ms vs the unadopted U ≤ P/10 rule; no SLO exists) |

## Unresolved claims
- Cross-host timing (B11): bound recorded, claims unqualified until a direct SUT→load probe narrows U or the owner accepts the bound.
- Platform rate above 200 r/s at k=2 (400 r/s saturates): the campaign rate at k=2 is bounded by the platform, not the generator.
- k=4: does not fit at 1280 MiB (owner decision O1–O5 pending).
- FIWARE adequacy above 50 r/s; D29a WAL live verification (pg-counters overlay) still pending.

## Running services & precise stop/rollback identities
- SUT (`ramona-vm-2`, 87.107.191.107):
  - TB k=2 at **1280 MiB** (the owner's qualification candidate, left in place): containers `tb-tb-core-1/2`,
    `tb-tb-rule-engine-1/2`, `tb-tb-http-transport-1/2`, `tb-haproxy-1`, `tb-kafka-1`, `tb-postgres-1`,
    `tb-zookeeper-1`, `tb-valkey-1` (project dir `/root/iot-bench-deploy-cand-auto20261027/thingsboard`).
    Rollback to adopted 1024: `TB_REPLICAS=2 POOL_CPUSET=4-5 docker compose up -d --force-recreate tb-core tb-rule-engine` (from that dir).
  - FIWARE candidate: `fiwhc-mongo-1`, `fiwhc-timescale-1` up; `fiwhc-orion-1`, `fiwhc-quantumleap-1` STOPPED
    (start: `docker compose start orion quantumleap` in `/root/iot-bench-deploy-cand-auto20261027/fiware-history-candidate`).
  - Exporters: `exporters-{node-exporter,cadvisor,postgres-exporter,nats-exporter}-1` (cpuset 0).
  - EdgeX: stopped (volumes kept; start: `LB_STATS_PORT=28404 docker compose up -d` in `…/edgex`).
  - Device registry: 2,213 devices + the k6-created bench devices from this session's runs (k6 teardown
    deletes its own; pilot/qual prefixes `pilot-v1-*`, `pilot-v2-*` remain — session-created, owner may purge).
- Load host (`ramona-vm-q`, 87.107.191.32; reachable only via `ssh -J ramonavm2 -i ~/.ssh/id_ed25519_vps -o IdentitiesOnly=yes root@87.107.191.32`):
  - monitoring stack + `yaade` untouched.
  - **sink RUNNING**: process `bin/sink -addr 0.0.0.0:18090` (PID via `pgrep -f "bin/sink -addr"`), writing
    `/root/ceiling-sink-auto20261029.ndjson` (240k+ msgs). Stop: `kill $(pgrep -f "bin/sink -addr")` — keep
    the ndjson as evidence.
  - Ceiling/qualification run dirs under `/root/iot-bench-cand-auto20261027/workloads/results/` (11 new dirs).

## Evidence map (`results/auto-20261029/`, SHA256SUMS inside)
- `ceiling/all-summaries.txt` — 7 generator-only steps + 5 TB qualification runs (verdicts, rates, latencies).
- `qual1280/` — (monitor CSVs fetched from the SUT: /tmp/qual-1280.csv → copy at close if needed).
- `fiware/hist-export.ndjson` (3,001 rows) + `edgex/edgex-hist-export.ndjson` (600 rows).
- `clock/clock-probe-results.md` + `ws-to-sut.csv` + `ws-to-load.csv`.
- `tb-memory-budget.md` — the k=1..4 table + owner options.
