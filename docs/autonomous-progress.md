# Autonomous progress (auto-20261027)

Authorization: see `docs/autonomous-work-plan.md` (recorded verbatim there; live per-task status below).
State machine: READY / IN_PROGRESS / IMPLEMENTED_UNVERIFIED / VERIFIED / BLOCKED / DEFERRED.

## Task log

### Task 0 — session records + baseline — VERIFIED (10:09Z)
Baseline HEAD 73dea4d5…, 9 tracked modified + 26 untracked; diff at `results/auto-20261027/baseline-tracked.diff`
(456 lines), untracked inventory `baseline-untracked.txt`. Session state `docs/autonomous-state.json`.

### Task 1 — preflight Gates A–C — VERIFIED (10:17Z)
- Mapping (from /etc/hosts + SSH, NOT inferred): SUT 87.107.191.107 = alias `ramonavm2` = uname `ramona-vm-2`;
  LOAD 87.107.191.32 = alias `ramonavm1` = uname `ramona-vm-q`.
- SUT: Ubuntu 24.04, kernel 6.8.0-31, 8 CPUs (1 thread/core), 16 GB RAM, 26 GB free on /, NTP sync OK
  (timedatectl), swap: 1.9 G swapfile (472 M used on load host), swappiness 60 — recorded as a known deviation
  (memory limits are hard, `memswap_limit` set in compose).
- Docker 29.9.1 + Compose v5.6.0 on both. **SUT: zero containers running, no collision.** Prior benchmark
  artefacts preserved untouched: `/root/iot-bench-deploy` (stale copy, no git, no D28 config), `/root/calib|diag|expl`,
  leftover docker volumes (kept, not used).
- Load host: 4 CPUs, 8 GB, disk 24 % used, NTP active. **Running: our monitoring stack**
  (prometheus v3.13.4, cadvisor, grafana 12.4.13, node-exporter — images via arvancloud mirror) **+ unrelated
  `yaade` container — untouched, listed for collision avoidance.** Listener note: node_exporter binds 127.0.0.1:9100,
  prometheus 127.0.0.1:9090 on the load host.
- Local pins present on SUT: thingsboard/tb-node 4.3.1.6 + transports + js-executor, postgres:16, kafka 4.0.0
  (image list), valkey 8.0, mongo 8.0, EdgeX 4.0.2 set, monitoring exporters.
- Transcripts: `results/auto-20261027/preflight-sut-gateA.txt`, `preflight-load-gateA.txt`.

### Task 2 — D28 live verification — VERIFIED (10:30Z)  [B7: technical evidence now LIVE]
Candidate deployed to a **new dedicated dir** (preserves prior copy):
`/root/iot-bench-deploy-cand-auto20261027` (GIT_REV: `local-dirty-2da151b788b2a3f5 (base 73dea4d5)`; shipped
tree hash of tarball prefix `2da151b788b2a3f5`, 226 files; benchctl linux binary `/root/.../bin/benchctl`).
Stack started k=1 (`N=1 make install && make up`), schema already present in existing TB volumes (kept).

§5 checklist results (all commands with outputs above under `results/auto-20261027/d28-* | spool`):
1. `/config/thingsboard.conf` + `/config/logback.xml` inside tb-core match vendored SHA-256 exactly ✔
2. Entrypoint exact as designed (`export TB_SERVICE_ID=$$HOSTNAME && exec start-tb-node.sh`, unescaped verified) ✔;
   `TB_SERVICE_ID` is NOT visible in a `docker exec` shell (expected — entrypoint shell only) — proven instead by
   JVM args expanding it to the container hostname (`-XX:HeapDumpPath=/var/log/thingsboard/4faae0cebb00-heapdump.bin`). ✔
3. Effective proc-1 JVM flags: `-Xms512m -Xmx512m` (heap unchanged, as designed), `+ExitOnOutOfMemoryError`,
   `+HeapDumpOnOutOfMemoryError`, `-Dlogging.config=/config/logback.xml`, G1 set. No heap change recorded. ✔
4. Log dirs are owned/writable by the thingsboard user; core (`4faae0cebb00/`) and rule-engine (`f5d02f230ad1/`)
   per-service dirs + per-service `*-gc.log` both present. At k=1 replica-dir uniqueness is by hostname;
   k≥2 check deferred to F2 (Task 5). ✔
5. Both thingsboard.log files written; zero logback appender errors in stdout. ✔
6. OOMKilled=false, RestartCount=0, Status=running for both nodes after ~15 min uptime. ✔
7. Health + low-rate telemetry smoke: core boot marker `Started ThingsboardServerApplication in 237.5 s`;
   HAProxy `/` returns 200; REST login 200; device create 200; **HTTP device API telemetry write 200 and
   `telemetry-stored: 1`** (rule chain processed + persisted). ✔
Rollback note (unchanged if reverted): prior `/root/iot-bench-deploy` remains intact; candidate stack currently
runs from the new dir; `docker compose down --remove-orphans` in the candidate dir returns to the pre-session
state (no volumes created deleted).

### Task 5 preconditions observed during Task 2 (recorded for B6)
- k=1 boot: kafka healthy, TB core fully booted in 237.5 s (first boot with existing schema), rule-engine running.
- F1 arithmetic fit and idle-RSS observation still to be recorded (Task 5a above), before any fixed-load.

### Task 3 — D27 read-only registry audit + B14 live-checked — DONE as sub-steps of Task 4 (12:20Z)
- **B14 ThingsBoard branch:** 2-message labelled telemetry smoke through the device API (device `b14-dev`), then
  `history-ids-audit.sh thingsboard b14-auto-20261027` returned **2/2 rows with device name and ts (ms)**.
  Pinned-schema columns confirmed live (`ts_kv`: entity_id/ts/str_v/long_v/dbl_v; `device`: name).
- **B14 EdgeX branch:** NOT run — the EdgeX stack was not deployed this session; the `describe` acceptance needs a live
  postgres core_data schema. Recorded as remaining scope for B14.
- **D27 audit (read-only, baseline registry live):** brought up the baseline FIWARE stack from the candidate tree
  (`/root/iot-bench-deploy-cand-auto20261027/fiware-baseline`, project `fiware`, ITS OWN persisted `mongo-data`
  volume; LB stats port moved to 18404 and Orion/agent ports to 11027/14041/17896 so the two HAProxy instances do
  not collide). `device-index.sh audit < live export` → **`{"total":0, "incomplete":0, "duplicate_groups":0}`**
  — the live registry is EMPTY at rest (prior runs delete their devices in teardown), so the audit passes but no
  duplicate can exist by population alone; duplicate detection remains covered by the offline tests only.
  No index created; no writes to any registry.

## Next action
Task 5b (F2 k=2) then Task 6; Task 3 (D27) already executed by Task 4 (see Task 4 records).

### Task 5a — ThingsBoard feasibility F1 (k=1, tracked defaults) — REACHED-STOP-CONDITION (12:35Z) [B6 advanced: F1 evidence obtained, k=4 still unqualified]
Run sits inside the D28 candidate stack at `/root/iot-bench-deploy-cand-auto20261027/thingsboard` (same session, ARANDOM device registry retained; volumes untouched). Status flags: OOMKilled=false / RestartCount=0 for all 8 TB containers throughout.

Evidence (`results/auto-20261027/f1/`):
- `f1-monitor1-raw.csv` (legacy writer v1; 640 valid samples) — monitored 10:54:31→11:03:24 covering
  **idle-before-provisioning (also ~19 min boot idleness before that)** together with the **provisioning
  window interleaved/overlapped** (3 attempts; two early SSH-timeout runs were half-executed; the completion run
  reported `provisioned:0 failed:1000` because ~1,000 devices already existed from those attempts). Device
  registration is count-verified in postgres: **1,000 devices** named `f1-auto20261027-0000..0999`.
- `f1-monitor2-raw.csv` (writer-v2, 344 valid samples, 0 gaps) — clean post-provisioning idle window
  11:15:24→11:21:01, no provisioning during it.
- `f1-artifacts.intervals.csv` — parsed with validated patterns (no positional assumptions) plus explicit
  malformed/marker preservation (`f1-artifacts.malformed.txt` captures each). Two sustained >95% intervals on
  `tb-tb-core-1`, both **QUALIFY (≥60 s)**:
  1. **10:56:19–11:03:24, 425 s (64 samples), peak 993.7/1024 MiB (97.04%)** — spans the provisioning window.
  2. **11:15:24–11:20:53, 329 s (43 samples), peak 996.3/1024 MiB (97.30%)** — post-provisioning idle, clean.
  A single 97% sample never satisfies the sustained condition; both intervals were computed from elapsed
  timestamps with continuity broken at gaps > 8 s.
- `f1-cgroup-peaks.txt` — lifetime cgroup `memory.peak` (hard counters, distinct from sampled peaks):
  tb-core **1004.4 MiB** of 1024, rule-engine 899.5/1024, transport 412.2/512, kafka 915.7/1280, postgres 325.7/1536.
  Scope protected: these counters accumulate since container start (~10:16Z), so they include boot-time pressure.

**F1 stage outcome per `docs/thingsboard-feasibility-plan.md`:** the documented stop condition
("any OOM kill, container restart, or a limit > 95% for 60 s") is **REACHED** — tb-core sustained >95% for
minutes at idle with a 1,000-device registry. This reproduces the D11 history live. Not an OOM kill, and no
restart occurred. Per plan this records infeasibility at k=1+tracked defaults; it does NOT automatically block
the campaign and must be presented to the owner unchanged (allowed: the plan documents the deviation process).

### Task 4 — FIWARE pair-test + isolated history candidate — VERIFIED (12:05Z) [B2 narrowed→live-passed on the candidate; B3 live-paired; B5 implemented + replay; B4 measured below]
Candidate: `/root/iot-bench-deploy-cand-auto20261027/fiware-history-candidate` (project `fiwhc`; fresh volumes
`fwc-mongo-data`, `fwc-ts-data`; compose committed as `fiware/fiware-history-candidate/compose.yaml`).
Images observed (pinned/tag verified at runtime): `orchestracities/quantumleap:1.0.0`, `timescale/timescaledb-postgis:2.3.0-pg13`,
`fiware/orion:4.5.0` (pre-cached), `mongo:8.0` — **Orion stays 4.5.0 (owner B3 decision honoured)**.
Live results (each logged above; results spool: /tmp on the SUT + this file):
- **4-payload probe** (direct to QL, `openiot`/`/`): a 200, exact duplicate b → second identical row 200;
  c (same time_index) → independent row 200; d (type mismatch) → pg8000 ProgrammingError 22P02 → **full
  original notification JSON preserved in `__original_ngsi_entity__`** (with `failedBatchID`), and the HTTP
  response was still "Notification successfully processed" (200). No collapse anywhere: 4 rows total.
- **Orion→QL pair-test** (Orion 4.5.0 subscription → http://quantumleap:8668/v2/notify, normalized):
  10/10 labelled updates stored with identity columns and exact timestamps; subscription `active`.
  Deliberate-drop test: QL stopped during seq=10 → 204 accepted by Orion, **not stored** after restart.
- **Restart visibility**: full restart of QL + Timescale container → 15/15 rows preserved, health `pass`.
- **Identity replay** (24 planned, via the real Orion path): 24 rows, 23 distinct, **missing exactly {11},
  duplicate exactly seq5 (2 copies)**, zero beyond-plan — reconciliation at identity level, not count level.
- **`scripts/history-ids.sh fiware` implemented** (per-tenant schemas, dynamic table discovery, error-column
  recovery for integer seqs within stored JSON, explicit `{"history_export_row_unrecoverable": …}` stderr
  marker + count for the rest). Live-verified: `pair-e2e` 10/10; `replay-auto20261027` 23/24 with named
  missing and duplicate; `pair-automust-d` → marker row, count=1.
- **`scripts/history-ids-audit.sh fiware describe|export` implemented** (B14): describe verified live on the
  candidate (`mtopeniot.etdevice` columns: entity_id text, time_index timestamp, seq double precision,
  run_id text, `__original_ngsi_entity__` jsonb).
- Bootstrap caveat recorded: QL 1.0.0 image has python but no psql; its shipped `quantumleap-db-setup.py`
  therefore cannot run in it; the bootstrap SQL was executed word-for-word from the pinned image's own
  template inside the timescale container (evidence above); the compose one-shot now sits behind a `bootstrap`
  profile so it cannot re-run and is documented.
- **Resource observation (B4 input, candidate, running idle):** services running with candidate limits
  (timescale 1536m, quantumleap 512m on infra cpuset 2-3; orion 512m pool core 4; mongo 2g). `docker stats`
  snapshot recorded in close-out; the supporting-tier fit question stays OPEN (B4) until an owner decision:
  the candidate fits within the 16GB host and adds no borrowed cores, but its two new services share the infra
  cpuset unchanged.

### Task 5b — ThingsBoard feasibility F2 (k=2, tracked defaults) — VERIFIED (feasibility result obtained, 12:30Z) [B6 F2 evidence obtained]
Stack recreated at k=2 (`N=2 POOL_CPUSET=4-5 make check|up`) with the SAME existing volumes and ALL pre-existing
device data (nothing reset: **F1's 1,000 devices intentionally retained** → F2 starts with 1,000 devices already
registered and adds its own 1,000, so F1 and F2 are NOT identical fresh-state comparisons; the effect of the larger
registry is part of the measurement). Final device counts (verified in postgres): total 2,013 =
1,000 `f1-auto20261027-*` (F1 batch, dol Elon retained) + 1,000 `f2-auto20261027-*` (1000/1000 distinct names,
998 direct 201-creates; the first F2 provisioning attempt ran while cores were still booting and was rejected
1000/1000 at the load balancer (HTTP 400), leaving `f2_count=0`; the second run after the full boot marker
("Started ThingsboardServerApplication" on both cores) created the rest; two curl-connection failures (code 000)
also occurred during early seconds (recorded, no identity affected: count still 1,000/1,000 distinct).
Live evidence:
- Identity/duplicate rule (D28 §5 item 7 at k≥2): each TB replica writes its OWN hostname-named log dir and
  GC log (`e67017e9400c`, `8d51074c34ec`, `209777c4bbb0`); no shared paths. ✔
- **Stop condition NOT triggered at k=2:** both cores ride 95–96.8% continuously in 8–41 s fragments; the strict
  analyzer found 39 short fragments and **zero qualifying ≥ 60 s intervals** (`f2-intervals.csv`: maximum
  fragment 41 s). OOMKilled=false / RestartCount=0 on all six application-tier containers; cgroup peaks
  (lifetime counters): tb-core-1 **1021.8/1024 MiB** (99.8% of the limit!), tb-core-2 1003.5/1024,
  rule-engine-1 884.8/1024, rule-engine-2 908.8/1024, transports 408/407 of 512. F2 does NOT trip the documented
  infeasibility signal, but both cores again sit near the tracked-default limit, matching D11's pattern; this is a
  feasibility observation, not an OOM.
- Bounded fixed-load smoke (official generator path, k=2, 100 devices provisioned by k6):
  `thingsboard-n1-r2-20261027T114603Z` (results retained on the load host at
  `/root/iot-bench-cand-auto20261027/workloads/results/…`): 2.02 req/s vs target 2 (offered≈achieved), 0.0% errors,
  **stored == sent (182 verified)**, p95(steady)=92.58 ms, k6 exit 0. Run verdict **VALID**; SUT oom=0
  (advisory: `containers per service` map empty — cAdvisor metadata was not populated during this run; the OOM
  and restarts list is from the same check and non-empty values were still captured — recorded as an
  observation gap, not hidden).
- Sampling integrity: monitor-v2 writer used with the same validation rules; 9 s sampling steps logged
  as GAP lines (continuity breaks); no hidden drops. CSV + interval artifacts in `results/auto-20261027/f2/`.
Conclusion for `docs/qualification-blockers.md` B6: F1 (k=1) is a **reached-stop-condition** record (evidence above);
F2 (k=2) shows **no limiting OOM/restart and no ≥60 s >95% interval**, with a VALID bounded smoke and a retained,
reconciled device registry. k>2 stays out of scope (F3/k=4 not authorized).
