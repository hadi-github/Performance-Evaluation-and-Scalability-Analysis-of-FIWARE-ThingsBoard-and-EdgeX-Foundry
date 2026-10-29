# Step 5 decision packet

**PROPOSED — awaiting owner decisions.** Nothing in this packet is adopted, applied, tuned or run. It creates no
decision-log entry, changes no compose file or plan, and authorizes no experiment. Statements are tagged:

- **[V]** verified in this checkout, in a pinned upstream tag, or in retained evidence named below;
- **[P]** proposal or recommendation;
- **[G]** gap or claim that could not be verified.

- Checkout: `73dea4d5493280125c7d330356a90433d78889d2` (2026-10-22 23:10 +0330).
- Working tree at packet creation: 5 modified documentation files (`README.md`, `docs/methodology/generator-status.md`,
  `docs/methodology/metrics-dictionary.md`, `docs/development/tooling-interfaces.md`, `monitoring/README.md`) and 2 untracked review files
  (`evidence/reproducibility.md`, `docs/handoff-review-2026-10-25.md`). This packet is the third untracked file.
- Decision source of truth: `docs/design-decisions.md` (D1–D26). **D27–D30 do not exist there**; they are proposals
  in `docs/operations/diagnostics.md` only. The Step 5 proposal table is `docs/operations/diagnostics.md:294-313`.
- Raw evidence root: `results/**` is gitignored; the only tracked result file is
  `results/exploratory/EXPL-tb-k4b.amendment.json`. A fresh clone cannot regenerate the diagnostics.

Nothing below changes D23's measurement rule. D23 goodput remains a persistence-based rate verdict; it is not
per-message committed-visibility latency (see §5.4).

## 1. What the owner is deciding

| ID | Candidate | Recommended disposition |
|---|---|---|
| D27 | FIWARE IoT Agent device-registry index | **Approve as an explicit upstream-backport exception**, not as ordinary Category A, subject to D25-a and the provisioning-validation gate (§3.3, §4.1) |
| D28 | ThingsBoard tb-core/rule-engine `/config` mount | **Approve as a Category A deployment-defect fix**, with the logback/log-volume detail resolved (§4.2) |
| D29a | EdgeX disk-write-latency reporting | **Approve** (reporting only; no platform change) |
| D29b | EdgeX extra core-data replicas per core | **Defer**; if pursued, run only as a labelled topology ablation, not as a qualification configuration |
| D29c | EdgeX `synchronous_commit=off` | **Reject for qualification; keep existing DIAG-only use**; do not infer approval from DIAG results |
| D29d | EdgeX `commit_delay` | **Defer separately**; it is not the same durability choice as `synchronous_commit=off` |
| D30 | Observation points per platform | **Approve the corrected table in §5**, replacing the missing “README table in Step 5” |
| — | Orion `-reqPoolSize` | **Defer**; 4 per replica is an unvalidated aggregate-oversubscription risk at 4 replicas sharing a 4-core cpuset, not a proven violation of per-process guidance |
| — | ThingsBoard queue poll intervals | **Defer**; the knobs are documented but upstream recommends no value, and the Category A question is unresolved |
| D25-a | Clarify upstream backports | **Approve append-only clarification** if D27 is approved |
| D11-a | Record the historical D11 edit | **Approve append-only addendum** |
| D1-a | Record the historical D1 correction | **Not required**; D1 already carries a dated correction and no dependent run used the old text. Optional wording in §6.2 |

## 2. Current state and evidence base

### 2.1 Verified implementation state [V]

- `deploy/fiware/mongo/init-indexes.js` creates only Orion **entity** indexes. It creates no index on the IoT Agent
  `devices` collection. The agent registry is `iotagentul` (`deploy/fiware/compose.yaml:109-112`).
- `deploy/fiware/compose.yaml:66-90`: Orion runs `-reqPoolSize ${ORION_REQ_POOL_SIZE:-0}` on `${POOL_CPUSET}`, with
  `${FIWARE_REPLICAS}` replicas. Every replica receives the **same** cpuset; Docker does not partition it. The
  `scripts/resources.mk` comment “one core per instance” describes the sizing intent (N pool cores for N instances),
  not exclusive per-replica pinning. `deploy/fiware/.env.example:25` sets `ORION_REQ_POOL_SIZE=0`.
- `deploy/thingsboard/compose.yaml:172-213`: `tb-core` and `tb-rule-engine` have **no** `/config` mount and no
  `/var/log/thingsboard` mount. Only `tb-http-transport` mounts `./tb-transports/http/conf:/config:ro`
  (`deploy/thingsboard/compose.yaml:218-225`).
- `deploy/edgex/compose.yaml:193-249`: `core-data` and `device-rest` both use `replicas: ${EDGEX_REPLICAS:-1}` and the
  same `${POOL_CPUSET}`; each has a 512 MiB limit (`deploy/edgex/.env.example`). `scripts/resources.mk` defines N pool
  cores and the comment “one core per instance”.
- `scripts/diag/analyze.py:175-181` computes `disk_write_await_ms` and `disk_writes_per_s` from node-exporter on the
  SUT. `scripts/diag/report.py:398` renders a “disk write await (ms)” column in the EdgeX diagnostic table.
  `monitoring/rules/bench.yml` contains no disk-latency alert/recording rule.
- `deploy/edgex/diag/compose.diag-nosync.yaml` is the only file setting `synchronous_commit=off`; its header says DIAG only
  and “must never reach compose.yaml or .env.example (D25)”.
- `README.md:119`: “TB and EdgeX acknowledge before persistence; the IoT Agent acknowledges after the Orion
  update. Ingress latency is not comparable across platforms.”
- `scripts/history-ids.sh`: EdgeX reads `core_data.reading`; ThingsBoard joins `ts_kv` on `(entity_id, ts)`; FIWARE
  exits 3 (“no history”). `benchctl reconcile -store` and `benchctl explore -history` provide the joins
  (`docs/development/tooling-interfaces.md:61-72`).
- ThingsBoard HTTP telemetry carries no `ts` (`workloads/k6/lib/adapters.js:94-104`). The pinned transport documents
  that the current server time is then used (`DeviceApiController.java`, `TS_PAYLOAD`), and
  `JsonConverter.convertToTelemetryProto(JsonElement)` supplies `System.currentTimeMillis()` at conversion, before
  queueing. This is the server-side conversion time, not the DB commit time. The SQL timeseries writer uses
  `INSERT … ON CONFLICT (entity_id, key, ts) DO UPDATE` (`SqlInsertTsRepository.java`), so two rows for one
  entity/key/millisecond overwrite rather than coexist; our database type is SQL (`deploy/thingsboard/compose.yaml:43`).
- `internal/sink/sink.go:24-31`: a sink record is `(recv_ms, platform, run_id, seq, dup)`; `recv_ms` is the sink
  process's wall clock. `internal/explore/explore.go:117-121` reports `sink_unique`, `sink_duplicates`,
  `sink_other_run_ids` and `planned_reached_sink`. These are computed only from sink NDJSON and planned sequence
  counts. `internal/reconcile` joins the ack/outcome ledger to **stored** identities (history), not to sink
  identities; no implemented ack→sink identity join exists.

### 2.2 Source verification [V/G]

| Claim | Verdict | How verified |
|---|---|---|
| iotagent-ul 3.7.0 pins iotagent-node-lib 4.7.0 | [V] | `package.json` at tag 3.7.0 fetched from `raw.githubusercontent.com`; also `evidence/source-excerpts.md:605-617` |
| iotagent-ul 3.7.0 is the latest release | [V] | GitHub releases API (`/releases/latest` → tag `3.7.0`, published 2025-02-03) |
| iotagent-node-lib 4.7.0 `Device.js` declares no index | [V] | tag 4.7.0 `lib/model/Device.js` fetched; also `evidence/source-excerpts.md:661-698` |
| iotagent-node-lib 4.8.0 declares `{service, subservice, apikey, id}` unique | [V] | tag 4.8.0 `lib/model/Device.js` fetched; also `evidence/source-excerpts.md:715-728` |
| 4.8.0 change log records “Add: index for Device model … (#1576)” | [G] | The excerpt's quoted code block is empty (`evidence/source-excerpts.md:730-737`); a direct fetch of the tag's `CHANGES_NEXT_RELEASE` returned no body. The prose line is retained, but the changelog/PR number is not independently verified here. The code itself is verified. |
| D28 official v4.3.1.6 compose mounts `./tb-node/conf:/config` for tb-core and rule-engine | [V] | official `docker/docker-compose.yml` fetched; also `evidence/source-excerpts.md:1035-1063` |
| D28 official start script sources `/config/thingsboard.conf` when `/config` exists | [V] | `msa/tb-node/docker/start-tb-node.sh` fetched; also `evidence/source-excerpts.md:1085-1100` |
| `ExitOnOutOfMemoryError` is in the official `docker/tb-node/conf/thingsboard.conf` | [V] | file fetched; also `evidence/source-excerpts.md:1065-1083` |
| Official `docker/tb-node/conf` contains both `thingsboard.conf` and `logback.xml` | [V] | GitHub contents API at tag v4.3.1.6 |
| Orion `-reqPoolSize` default is 0; official guidance is pool ≈ available CPU cores and not above them | [V] | `evidence/source-excerpts.md:907-934` (perf_tuning.md lines 280-300) |
| ThingsBoard queue poll defaults are 25 ms; core partitions 10; no repo overrides | [V] | `evidence/source-excerpts.md:994-1033`; `docs/operations/diagnostics.md:188` |
| A ThingsBoard telemetry payload without `ts` is stored at current server time | [V] | `DeviceApiController.java` (`TS_PAYLOAD`, `postTelemetry`) at v4.3.1.6; `JsonConverter.convertToTelemetryProto(JsonElement)` defaults to `System.currentTimeMillis()` |
| ThingsBoard overwrites rows with the same `(entity_id, key, ts)` | [V] | `dao/.../insert/sql/SqlInsertTsRepository.java` at v4.3.1.6: `INSERT … ON CONFLICT (entity_id, key, ts) DO UPDATE`; `deploy/thingsboard/compose.yaml:43` uses `DATABASE_TS_TYPE: sql` |
| Ack identities are joined to sink identities | **No** [V] | `cmd/benchctl/main.go` `explore -sink` builds sink observations from NDJSON; `internal/explore` counts sink unique/planned reached; `internal/reconcile` joins only the ledger to stored identities. No sink↔ledger join is implemented. |
| The D23 persistence series is a table-level insert counter | [V] | `scripts/persist-series.sh` default `pg_stat_user_tables_n_tup_ins{relname="reading",schemaname="core_data"}`; 5 s scrape; PostgreSQL flush delay; unfiltered by `run_id` |

## 3. Category A wording tension

### 3.1 The actual D25 rule [V]

`docs/design-decisions.md:33` says, verbatim:

> A platform gets only (a) tuning its own official documentation recommends, or (b) fixes for defects in our
> deployment. Category A: leaves ack point, durability, delivery guarantee and batching semantics unchanged.
> Category B: changes any of these; needs the user's approval. Timebox: one working day of tuning per platform.
> Every change records its official source (URL + section, or repo file + tag), reason and expected effect, and goes
> into the manifest `deviations` of every later run. Durability-weakening settings (e.g. `synchronous_commit=off`)
> only in `DIAG-*` runs, labelled, never in `compose.yaml` / `.env.example`.

Two separate tests follow from this text:

1. **Eligibility**: the change must be (a) recommended by the platform's official documentation, **or** (b) a fix for
   a defect in our deployment.
2. **Category**: if eligible, it is Category A only if it leaves ack point, durability, delivery guarantee and
   batching semantics unchanged; otherwise it is Category B and needs approval.

### 3.2 The stricter paraphrase that must not replace D25 [V/G]

`docs/handoff-review-2026-10-25.md` §5.3 records an earlier agent-prompt paraphrase: “Category A only if the change
is supported by the platform's official documentation”, omitting limb (b). That paraphrase is **not** D25. The
original prompt text is not retained in the repository [G]; the repository record of the paraphrase is the handoff
review. No future packet should silently use the paraphrase to disqualify a deployment-defect fix, and no packet
should use D25 to admit an upstream backport that is neither (a) nor (b).

### 3.3 Classification of D27 and D28

| Candidate | Limb (a) official recommendation? | Limb (b) our deployment defect? | Semantics claim | Classification |
|---|---|---|---|---|
| D27 index | **No.** The source is upstream library code plus a changelog/PR claim, not official deployment documentation. | **Not strictly.** The missing index is an upstream 4.7.0 omission; our deployment is not misconfigured. | **Supported only conditionally:** ack point, write concern, delivery path and batching are intended to stay the same, and the index changes query planning. That claim holds only if index creation succeeds on the existing data and the agent’s provisioning writes tolerate the new unique constraint; concurrent `findOrCreate` can turn duplicates into write errors. Provisioning validation is a condition, not an assumption. | **Not ordinary Category A.** It is an **upstream backport**. Needs an explicit owner exception (recommended: D25-a) before it can be recorded as A-equivalent, plus the provisioning-validation gate. |
| D28 `/config` | **Yes**, the official v4.3.1.6 compose and launcher do this. | **Yes**, our compose omitted the mount that the official deployment uses. | **Yes** [V]: no ack/durability/delivery/batching change; it adds JVM flags and logging config. | **Category A (defect fix)**. |

D27 must not be called “Category A” without the exception. Calling it B would be wrong because B is defined by
changed semantics, and D27 is intended to change none. The honest choices are: (i) approve an explicit A-equivalent
backport exception; (ii) reject/defer; or (iii) amend D25. Option (i) is recommended, with D25-a recording the rule,
but “semantics unchanged” is limited to the supported claims above and is conditional on provisioning validation.

## 4. Candidate inventory

### 4.1 D27 — FIWARE IoT Agent registry index

**Exact proposed change [P].** In `deploy/fiware/mongo/init-indexes.js`, after the Orion entity indexes, create on the IoT
Agent registry database (`iotagentul`, or `IOTA_MONGO_DB`) the index:

```js
db.getSiblingDB("iotagentul").devices.createIndex(
  { service: 1, subservice: 1, apikey: 1, id: 1 },
  { unique: true }
);
```

The index key order must match upstream 4.8.0 exactly. The init container must receive or derive the same DB name
the agent uses; `deploy/fiware/compose.yaml:112` sets `IOTA_MONGO_DB: iotagentul`.

**Supporting source [V/G].** Upstream `iotagent-node-lib@4.8.0 lib/model/Device.js` adds the unique index; 4.7.0 does
not. `iotagent-ul@3.7.0` is the latest release and pins 4.7.0 (GitHub releases API, published 2025-02-03), so the
index is absent from every released UL agent. The 4.8.0 changelog/PR `#1576` claim is partially unverified [G]
(empty excerpt block; direct changelog fetch empty), but the code at the tag is verified. Diagnostics observed 1.98
`COLLSCAN` operations per message on `iotagentul.devices` (`docs/operations/diagnostics.md:147, 298`).

**Expected benefit [P].** Both per-message device lookups become index lookups. MongoDB CPU per message stops growing
with device count. The binding pool should then remain the IoT Agent app pool (fit ≈ 131 msg/s), so capacity should
rise only toward that app-pool limit, not without bound.

**Uncertainty [G].** (i) The changelog/PR is not independently verified. (ii) Existing registry data may contain
duplicate `(service, subservice, apikey, id)` tuples, including documents with missing fields (Mongo unique indexes
treat missing as null); a unique index creation will fail while they exist, and resolving them is not authorized
here. (iii) The application may already rely on `findOrCreate` races that a unique index turns into duplicate-key
write errors under concurrent provisioning. (iv) The measured 131 msg/s app-pool fit is from diagnostics, not a
post-index run.

**Duplicate pre-check [P].** Before creating the index, count duplicate groups with the exact key
`{service, subservice, apikey, id}` and report the count and offending identities. If any conflict exists, **stop and
report**; do not delete documents, merge identities, rewrite `id`/`apikey` fields, or otherwise “clean” the registry.
The index is not created in that state. The same rule applies if creation fails because of a conflict discovered at
create time.

**Durability / resource / comparability.** The supported claim is narrower than “semantics unchanged”: the configured
write concern, ack point, delivery path and batching are intended to stay the same, and the index changes query
planning only, **provided** creation succeeds on the existing data and the agent’s provisioning writes tolerate the
unique constraint. Durability is not strengthened: the index is not a commit or journal guarantee. Index maintenance
adds write cost on provisioning and disk space on the small `devices` collection. Results after the change are not
directly comparable with pre-change FIWARE runs; every later run must record the deviation (D25).

**Validation needed [P].** Before creating: run the duplicate pre-check above. After creating: `explain` the two
per-message lookups and confirm `IXSCAN` (no `COLLSCAN`); exercise the agent’s own device create/update/provisioning
path and check for duplicate-key errors or failed registrations; if any appear, stop and report rather than changing
agent behavior. Then run a fixed-load FIWARE smoke at the same offered rate and k as the baseline, 3 repetitions,
checking ack outcomes, collector errors, MongoDB CPU per message, and the sink/history outcome. Record the index and
its validation result in the run manifest `deviations`.

**Rollback [P].** First revert or disable the index-creation step in the init path, so a restart cannot recreate it.
Then drop the exact index created by this change, identified by its key spec `{service:1, subservice:1, apikey:1, id:1}`
(record the actual generated name), and verify it is absent after restart. Do not delete or rewrite registry data.

**Recommendation [P].** **Approve as an explicit upstream-backport exception**, conditional on D25-a and on a clean
duplicate pre-check plus provisioning validation. It has a clear mechanism, but it is not ordinary Category A under
D25 as written, and “semantics unchanged” is limited to the supported claims above.

**Owner choice.** See the §9 form (disposition and authorization levels; no level is authorized by this packet).

---

### 4.2 D28 — ThingsBoard tb-core/rule-engine `/config`

**Exact proposed change [P].** Vendor the official v4.3.1.6 `docker/tb-node/conf` files into the repository
(`deploy/thingsboard/tb-node/conf/thingsboard.conf` and `deploy/thingsboard/tb-node/conf/logback.xml`), and mount that directory
read-only at `/config` for **both** `tb-core` and `tb-rule-engine`:

```yaml
volumes:
  - ./tb-node/conf:/config:ro
```

The official compose does exactly this for `tb-core1`/`tb-core2` and `tb-rule-engine1`/`tb-rule-engine2`
(`evidence/source-excerpts.md:1035-1063`). Because the launcher sets
`LOGGING_CONFIG=/config/logback.xml` whenever `/config` exists, the mount must include `logback.xml`; mounting only
`thingsboard.conf` is incomplete. The official `logback.xml` writes to
`/var/log/thingsboard/${TB_SERVICE_ID}/thingsboard.log`; our compose currently has no such mount. The implementation
must therefore either add a writable per-service log volume or use a reviewed stdin/stdout-only logback. The first
option keeps the official files byte-identical; the second is a deviation from the official conf and must be
recorded as such.

**Supporting source [V].** Official `docker/docker-compose.yml` v4.3.1.6 mounts `./tb-node/conf:/config` for tb-core
and rule-engine; official `start-tb-node.sh` sources `/config/thingsboard.conf` and sets
`LOGGING_CONFIG=/config/logback.xml`; official `thingsboard.conf` contains
`-XX:+ExitOnOutOfMemoryError` and `-XX:+HeapDumpOnOutOfMemoryError`. Our stack mounts `/config` only for the HTTP
transport, so tb-core/rule-engine use the image's built-in conf without the exit-on-OOM flag
(`docs/operations/diagnostics.md:190`).

**Expected benefit [P].** An OOM exits the JVM instead of leaving a broken process; the restart policy can then
restart it. The official logback and GC/heap-dump flags come along. No ack point, durability, delivery guarantee or
batching semantics change.

**Uncertainty [G].** (i) The official `thingsboard.conf` also appends `-Dplatform=deb`, `-Dinstall.data_dir=...`, GC
logging, heap-dump path and GC flags; these are not individually evaluated for our containers. (ii) The official
`logback.xml` writes files under `/var/log/thingsboard`; without a writable mount, logging may fail or degrade.
(iii) The official compose leaves `JAVA_OPTS` empty while ours sets `-Xms/-Xmx`; the conf appends to the existing
env, so heap sizing is preserved, but this interaction needs a startup check. (iv) A controlled OOM test has not
been run (and must not be run as part of this packet).

**Durability / resource / comparability.** Durability/delivery unchanged. Resource effect: GC logs (10 × 10 MB),
heap dumps on OOM, and log volume disk; no additional CPU or memory budget. Throughput should be unchanged; logging
level/config could shift CPU slightly. Any later run must record the conf version and mount as a deviation.

**Validation needed [P].** `docker compose config`; start both services; verify the effective JVM command contains
`ExitOnOutOfMemoryError` (e.g. `/proc/1/cmdline` or startup log), verify `/config/thingsboard.conf` and
`logback.xml` are the pinned files (hash), verify logs are written/readable, and run a smoke at the same load as the
baseline checking ack/sink/storage counts. The OOM behavior itself needs a separate, explicitly authorized
small-heap DIAG test.

**Rollback [P].** Remove the mounts and vendored files, restart the services. The image's built-in conf returns.

**Recommendation [P].** **Approve as Category A (defect fix)**, but treat the logback/log-volume detail as part of
the implementation review; do not apply a half-mount.

**Owner choice.** See the §9 form (disposition and authorization levels). The logback/log-volume variant is chosen
there; no level is authorized by this packet.

---

### 4.3 D29 — EdgeX persistence: three separable decisions

D29 is not one decision. The diagnostic finding is a fact; the three responses below have different categories and
different risks.

#### D29a — report disk-write latency with every EdgeX result

**Exact proposed change [P].** Carry the existing host disk fields into every EdgeX result, not only DIAG runs:
`disk_write_await_ms` per device and `disk_writes_per_s` from the SUT node-exporter. Where the diagnostic
`pg_stat_statements`/`track_wal_io_timing` overlay is already enabled, also carry the WAL-side `ms_per_sync`,
`wal_sync_share` and `commits_per_s`. Store them in the run's result/manifest (for example under `resources.disk`)
and render them beside the persistence rate. Do not change the verdict rule. Do **not** enable
`pg_stat_statements` in the main EdgeX compose as part of this reporting change; that would be a separate
configuration change with its own validation.

**Supporting source [V].** `scripts/diag/analyze.py:175-181` already computes the host disk fields; `report.py:398`
already renders “disk write await (ms)” in the diagnostic table. The metrics dictionary already lists disk activity
as host-level, with “no per-container or per-transaction latency”
(`docs/methodology/metrics-dictionary.md`, Host/container resources).

**Expected benefit [P].** A reader can see that EdgeX's ceiling is WAL flush/disk latency, not CPU, and can compare
runs on the same SUT disk. It makes the D29 finding reproducible without rerunning the diagnostic analysis.

**Uncertainty [G].** The node-exporter metric is host/device-level and includes all writes, not only PostgreSQL WAL;
`disk_write_await_ms = rate(write_time)/rate(writes_completed)` is an average over the scrape window (5 s steps in
the diagnostic). It must not be presented as per-transaction latency. The WAL `ms/sync` and the host await are
different quantities; report both, never one as the other.

**Durability / resource / comparability.** No platform change; no durability effect; negligible storage. It improves
comparability by naming the disk context, but does not make runs on different disks comparable.

**Validation needed [P].** Recompute one retained EdgeX run from its Prometheus inputs and compare with
`results/diag/<run>.diag.json`; confirm the same values appear in the result file and the report.

**Rollback [P].** Remove the reporting fields; no runtime rollback needed.

**Recommendation [P].** **Approve** as reporting work (not a tuning category).

**Owner choice.** See the §9 form (disposition and authorization levels; no level is authorized by this packet).

#### D29b — more core-data replicas than pool cores

**Exact proposed change [P].** At k = 4, run 8 `core-data` replicas (2 per pool core) while keeping 4 `device-rest`
replicas and the fixed 4-core `${POOL_CPUSET}`, to get more concurrent commits and more group-commit sharing. This is
the diagnostic table's “8 at k = 4” option. It requires a separate core-data replica variable (or an equivalent
compose override), because `deploy/edgex/compose.yaml` currently drives both `core-data` and `device-rest` from the single
`EDGEX_REPLICAS` value. A distinct alternative is k = 8 / N = 8 on an 8-core pool; that is a new k, not “more
replicas per core”, and is not the same comparison.

**Supporting source [V/P].** EdgeX scales by replicas + NATS queue group; `deploy/edgex/compose.yaml` ties both tiers to
`EDGEX_REPLICAS` and `POOL_CPUSET`; `scripts/resources.mk` allocates “one core per instance”; D26 records replicas
per tier separately. The diagnostic hypothesis is that 4 replicas share ~1.8 commits per WAL sync, so more
concurrent transactions could share more (`docs/operations/diagnostics.md:218`). This is a hypothesis, not a measured result.

**Expected benefit [P].** A higher persisted rate at the same disk if group commit is the limiting mechanism and the
extra concurrency does not saturate CPU or the DB.

**Uncertainty [G].** (i) PostgreSQL already group-commits; extra replicas add context switches, NATS consumers and DB
connections (each core-data has `MaxConns=4`; 4 extra replicas add up to 16 connections). (ii) The disk remains the
serial resource; more commits may not reduce total sync time. (iii) Memory: four extra `core-data` containers at
512 MiB each add ~2 GiB of limits; the host is 16 GiB and the existing stack already uses most of it. (iv) The
fixed CPU budget is unchanged: all replicas still share 4 cores, so this is not “free” capacity.

**Does it violate one-instance-per-core?** **Yes, as written.** The design comment and `scripts/resources.mk` define
one instance per pool core; 8 core-data on 4 cores is two instances per core. It is a topology change, not a
Category A tuning knob. It also changes the replicas-per-tier value that D26 says must be recorded separately, so a
k = 4 result with 8 core-data is not directly comparable with the existing k = 4 / 4 core-data baseline. If the
single `EDGEX_REPLICAS` variable were set to 8 instead of splitting the tiers, `device-rest` would also double,
adding ~4 GiB rather than ~2 GiB and changing ingress concurrency as well.

**Durability / resource / comparability.** Durability semantics unchanged if `synchronous_commit` stays on. Resource
effect: +~2 GiB memory, +DB connections, same 4 cores. Comparability: broken unless the new topology is part of the
configuration label and the baseline is not mixed with it.

**Validation needed [P].** Only as a labelled topology ablation: fixed k = 4, 4 vs 8 core-data, 3 repetitions at the
same offered rates, measuring persisted/s, WAL sync share, commits/sync, per-container CPU, memory, connection counts
and errors. It must not be presented as a qualification configuration or as Category A.

**Rollback [P].** Restore 4 core-data replicas (or the separate topology variable), restart; drop extra connections.

**Recommendation [P].** **Defer.** If the owner wants the experiment, authorize it as an explicitly labelled
topology ablation, not as Step 5 tuning. Do not call it Category A.

**Owner choice.** See the §9 form (disposition and authorization levels; no level is authorized by this packet).

#### D29c — `synchronous_commit=off`

**Exact proposed change [P].** The only existing implementation is
`deploy/edgex/diag/compose.diag-nosync.yaml:24`, layered only for `DIAG-*-nosync` runs. A qualification change would move
`synchronous_commit=off` into the main EdgeX database configuration or `.env.example`; D25 explicitly forbids that.
No such change is proposed here.

**Supporting source [V].** PostgreSQL 16 documentation 20.5.1 / 30.4, cited in the diagnostic file header; DIAG
results: durable k = 4 402.0/s vs async 494.5/s; k = 1 136.0/s vs 199.9/s; both async runs pass the offered load
(`docs/operations/diagnostics.md:209-218`). The async run also shows higher disk await (14.66 ms at k = 4) because commits are
batched; the throughput gain is not free.

**Expected benefit [P].** Higher persisted throughput and less WAL sync wait.

**Uncertainty / risk [V].** Asynchronous commit can lose acknowledged rows on a crash. That is a durability change,
so it is Category B by D25. The DIAG result proves only that the mechanism explains the ceiling; it does **not**
authorize a qualification configuration, and no crash-loss test exists.

**Resource / comparability.** Same CPU/memory budget; disk write pattern changes. Any async run is not comparable
with durable runs and must be labelled separately.

**Validation needed [P].** If the owner ever considers it: an explicit D25 amendment, a crash/kill test that
quantifies acknowledged-but-lost rows, separate verdict labels, and a manifest deviation. Not part of this packet.

**Rollback [P].** Remove the DIAG overlay; the main compose remains untouched.

**Recommendation [P].** **Reject for qualification; keep the existing DIAG-only use.** Do not infer approval from the
DIAG pass.

**Owner choice.** See the §9 form. This row records no change; approval of a future D25 amendment is not given here.

#### D29d — `commit_delay`

**Exact proposed change [P].** No change is proposed. `commit_delay` (with `commit_siblings`) delays a commit to
allow group commit but still waits for the synchronous WAL flush; it does **not** weaken durability the way
`synchronous_commit=off` does.

**Why separate [V/P].** The diagnostic table groups “`synchronous_commit=off` / `commit_delay`” under B. That is
imprecise: the durability argument applies to async commit, not to `commit_delay`. `commit_delay` changes latency
and batching, and there is no official recommendation for this workload.

**Recommendation [P].** **Defer separately.** If pursued, measure it as a latency/group-commit experiment at fixed
durable settings, not as an async-commit substitute.

**Owner choice.** See the §9 form (disposition and authorization levels; no level is authorized by this packet).

---

### 4.4 D30 — observation points

**Exact proposed change [P].** Replace the missing “README table in Step 5” with the table in §5 and adopt it as the
D30 text. D30 is documentation: it defines what ack, sink receipt and storage-reader evidence each establish. It
does not change a platform, a verdict rule or D23.

**Why the original wording is blocked [V].** `docs/operations/diagnostics.md:313` says “per the README table in Step 5”; no such
table exists in this checkout (`docs/handoff-review-2026-10-25.md` §3). Approving D30 as written would approve a
reference to nothing.

**Benefit [P].** Prevents three recurring errors: treating an HTTP 200 as durability; treating a sink receipt as a
storage commit; and comparing ack latency across platforms as if the ack points were the same.

**Uncertainty [G].** The EdgeX device-rest handler is not in `evidence/source-excerpts.md`; its ack point is
established by `README.md:119` and the architecture, not by a pinned source excerpt. The ThingsBoard path is now
pinned: `DeviceApiController.postTelemetry` returns 200 through `HttpOkCallback` after the transport service accepts
the message; `JsonConverter.convertToTelemetryProto` assigns `System.currentTimeMillis()` when the payload has no
`ts`; `SqlInsertTsRepository` upserts on `(entity_id,key,ts)`. The sink’s “after save” ordering is source-backed, but
the exact per-message DB commit time is not exposed. **Acknowledged identities are not joined to sink identities**:
sink figures are aggregates over sink NDJSON, and `reconcile` joins only to stored history. An ack-but-not-sink claim
is therefore an inference from counts/health, not a per-identity join.

**Durability / resource / comparability.** No runtime, durability, CPU or memory effect: documentation only. It
improves comparability by forcing every report to name its observation point and clock basis; it does **not** make
cross-host latency comparable and does not change D23's persistence verdict.

**Validation needed [P].** Apply the table to one retained run per platform and check every row against the named
evidence file; confirm it agrees with `README.md:119`, `docs/methodology/metrics-dictionary.md` and D23; confirm no cell claims a
per-message commit time, an ack→sink identity join, or a cross-host latency that the evidence does not support; and
confirm the ThingsBoard `ts`/overwrite statements against `DeviceApiController`, `JsonConverter` and
`SqlInsertTsRepository` at the pinned tag.

**Rollback [P].** Revert the documentation change; no runtime rollback exists because nothing is deployed.

**Recommendation [P].** **Approve the corrected table** as documentation, with the unsupported-claims column
treated as normative.

**Owner choice.** See the §9 form (Text only; no level is authorized by this packet).

---

### 4.5 Orion request pool size

**Exact proposed change [P].** Set `ORION_REQ_POOL_SIZE` to a non-zero value; the diagnostic table suggests `4`
(`docs/operations/diagnostics.md:299`). Current default is `0` (`deploy/fiware/compose.yaml:80-82`, `.env.example:25`). At k = 4 there
are 4 Orion replicas; at k = 1 there is 1 replica.

**Topology fact [V].** Every Orion replica is given the **same** cpuset, `${POOL_CPUSET}`: at k = 4 that is 4 CPUs
shared by all 4 replicas, and at k = 1 it is 1 CPU. Docker’s `cpuset` restricts the allowed CPUs; it does not
partition them or reserve one core per replica. The `scripts/resources.mk` comment “one core per instance” describes
the sizing intent (N pool cores for N instances), not exclusive per-replica pinning.

**Supporting source [V].** Orion 4.5.0 `perf_tuning.md` lines 280-300: default 0 means a thread per request; a pool
is recommended under high load; “the pool should be sized with a value equal or close to number of available CPU
cores. If you set `-reqPoolSize` to a value higher than number of CPU cores then you'll most probably experience
performance decrease.”

**The problem with “4” as written [P].** The guidance is written per Orion process, and each process sees the whole
shared cpuset. At k = 4, 4 pool threads per process is at, not above, the process’s visible 4 CPUs; the risk is
aggregate: up to 16 request-pool threads across 4 replicas contend for the same 4 cores. At k = 1, 4 threads on a
1-CPU cpuset is above the process’s visible core count and is closer to a literal reading of the warning. The
official text does not say whether to count per process or across processes on a shared cpuset, so the packet cannot
claim that 4 per replica **violates** the guidance. It can claim that 4 per replica is an unvalidated
aggregate-oversubscription choice at k = 4, and a clearer per-process overrun at k = 1. A topology-aware aggregate
would be 1 per replica at k = 4 (4 total) and 1 at k = 1; that is an owner interpretation, not an upstream rule.

**Expected benefit [P].** Avoid thread creation/destruction per request and use `epoll()`; likely small here because
Orion costs ~1 ms CPU per message and the FIWARE limit is the IoT Agent app pool (fit ≈ 131 msg/s). Not a capacity
fix.

**Uncertainty [G].** No measured A/B in this checkout; the effect on our 4-replica shared-cpuset topology is unknown;
the official guidance does not address multiple processes sharing a cpuset; and 4 per replica is above the visible
CPU count at k = 1.

**Durability / resource / comparability.** No durability/delivery change. Thread pool memory/stack per thread; CPU
scheduling changes. A run with a non-zero pool is a new configuration and must record the deviation.

**Validation needed [P].** Compare 0, 1 per replica and 4 per replica at fixed k and offered rate, 3 repetitions;
measure ack latency, CPU per replica, errors and throughput; record the aggregate pool size, the shared cpuset, and
the per-process visible CPU count. Use `docker inspect`/`ps` to verify the effective flag per replica.

**Rollback [P].** Set `ORION_REQ_POOL_SIZE=0` and restart.

**Recommendation [P].** **Defer.** The proposed value 4 is not established for the 4-replica shared-cpuset topology.
If the owner wants it, authorize a topology-aware test with 0, 1 and 4 per replica at k = 4 (and note the k = 1
per-process overrun), recorded as a deviation. Do not describe 4 per replica as a proven guidance violation.

**Owner choice.** See the §9 form (disposition and authorization levels; no level is authorized by this packet).

---

### 4.6 ThingsBoard queue poll intervals

**Exact proposed change [P].** Raise one or more queue poll intervals from the 25 ms defaults, e.g.
`TB_QUEUE_TRANSPORT_REQUEST_POLL_INTERVAL_MS`, `TB_QUEUE_TRANSPORT_RESPONSE_POLL_INTERVAL_MS`,
`TB_QUEUE_CORE_POLL_INTERVAL_MS`, `TB_QUEUE_RULE_ENGINE_POLL_INTERVAL_MS`. The diagnostic table gives no value and
says “needs your call whether ‘documented knob’ counts as (a)”.

**Supporting source [V].** `thingsboard.yml` v4.3.1.6 documents the knobs and defaults (25 ms; core partitions 10);
no repository override exists (`evidence/source-excerpts.md:994-1033`; `docs/operations/diagnostics.md:188`). **No official
recommended value was found** [G].

**Expected benefit [P].** Fewer idle Kafka polls/JVM wakeups, lower idle CPU, possibly less noise in the CPU budget.
Cost: up to the interval added to queue latency; at low offered rates the effect on throughput may be small.

**Category question [P].** Limb (a) requires official documentation to **recommend** the change. A documented knob
with no recommended value is not the same thing. Limb (b) does not apply (nothing is broken). The change also
touches latency/batching timing, so it is at least B-adjacent. It is therefore **not Category A as written**; it
needs an owner interpretation/exception or should be deferred.

**Uncertainty [G].** No measured idle/loaded A/B in this checkout; the interaction with Kafka partitions, consumer
groups and the 25 ms defaults is not characterized; a longer poll interval may increase end-to-end latency more than
it saves CPU.

**Resource / comparability.** Lower CPU is the goal; no memory change. Any change alters latency and makes runs
non-comparable with the 25 ms baseline; record every changed variable in `deviations`.

**Validation needed [P].** Fixed-load A/B (e.g. 100/s and 500/s) with one knob family at a time, 3 repetitions,
measuring idle/loaded CPU, ack latency, sink latency, throughput and Kafka consumer lag; do not combine with other
TB changes. The value must be chosen by the test, not asserted as official.

**Rollback [P].** Unset the variables and restart; defaults return.

**Recommendation [P].** **Defer.** If the owner wants it, first rule whether a documented-but-unrecommended knob can
be Category A; then authorize a named A/B, not a production change.

**Owner choice.** See the §9 form (disposition and authorization levels; no level is authorized by this packet).

## 5. D30 observation table (proposed text)

**Status: [P] proposed wording; facts are [V] unless marked [G].**

| Platform | What HTTP acknowledgement establishes | What sink receipt establishes | What the storage reader establishes | Timestamp source / clock requirement | Identity, loss and duplicate audits | Unsupported claims |
|---|---|---|---|---|---|---|
| **EdgeX** | `device-rest` accepted the request and acknowledged it. [V] `README.md:119` records “acknowledge before persistence”; the exact handler/publish ordering is not in the pinned excerpts [G]. It does **not** establish that `core-data` persisted the event. | The export app-service subscribes to the NATS event stream **upstream of / in parallel with** `core-data`; a receipt means the export saw the event, possibly before or without storage. The sink is not a durability timestamp. | `core_data.reading` rows via `scripts/history-ids.sh edgex`; full `(run_id, seq)` identity per row. It establishes presence at query time, not the commit time. | **Ack:** k6 `scheduled_send_ms` (load-host `Date.now()`-based wall clock, from the scenario start) → `response = Date.now()` immediately after `http.post` returns; latency is `response - scheduled_send_ms`, includes send lag, 1 ms resolution, **not** `http_req_duration` and **not** monotonic. **Sink:** `recv_ms = time.Now().UnixNano()/1e6` (sink-host wall clock); the sink runs on the load host by default, and `benchctl explore -sink` subtracts the load-host planned time, so that path is same-host by default. Any load-host↔SUT storage comparison is **cross-host and unqualified**; no clock offset was measured [G]. | Generator outcomes and health gates; sink `sink_unique`/`sink_duplicates`/`sink_other_run_ids` and `planned_reached_sink` are **counts only** (no ack→sink identity join); `benchctl reconcile -store`/`explore -history` joins the ledger to **stored** identities (`ack_lost`, `persisted_despite_*`, duplicates, orphans). NATS drops appear as ack-but-not-stored. No per-message commit timestamp. | “200 OK means stored”; “sink receipt means persisted”; “an identity-level ack→sink join exists”; “the table insert counter proves commits or durability”; “cumulative persistence rate is per-message commit latency”; any cross-host latency without a measured offset. |
| **ThingsBoard** | `DeviceApiController.postTelemetry` returns 200 through `HttpOkCallback` after the transport service accepts the message for asynchronous processing [V]. It does **not** establish a DB write. | The rule chain is `Save Timeseries` → `bench sink`; `Success` fires from the save callback after the SQL batch insert returned (`docs/operations/diagnostics.md:28`; source excerpts `TbMsgTimeseriesNode`/`TelemetryNodeCallback`/`JpaSqlTimeseriesDao`/`TbSqlBlockingQueue`). Receipt means the save callback succeeded, not that a per-message commit timestamp exists or that the row is queryable/retained. | `ts_kv` rows joined on `(entity_id, ts)` by `scripts/history-ids.sh thingsboard`; identity is reconstructable. The adapter sends no `ts`, so the server uses `System.currentTimeMillis()` at JSON conversion in the HTTP transport [V] — server wall clock, 1 ms, **before queueing**, not the DB commit time. `SqlInsertTsRepository` upserts on `(entity_id, key, ts)`, so two messages of one device in the same millisecond overwrite rather than coexist; they appear missing, not as a lost write. It establishes presence at query time, not commit time. | **Ack:** load-host `scheduled_send_ms` → `response` wall clock (same k6 path as EdgeX; includes send lag, not `http_req_duration`). **Sink:** load-host schedule → sink `recv_ms` after the save callback, not a per-message DB commit time. The DB `ts` is SUT-side; joining it to the load-host schedule is **cross-host and unqualified** [G]. | Generator outcomes/health; sink counts only (no ack→sink identity join); stored-identity reconciliation via `history-ids.sh` + `reconcile`; Kafka consumer lag is **not** persistence evidence. Same-millisecond key collisions limit the loss audit and can make a stored message appear missing; duplicate rows collapse by key. | “HTTP 200 means stored”; “sink success means durable per-message commit”; “history row count proves every message”; “a same-ms overwrite proves loss”; “Kafka lag zero means persisted”; an ack→sink identity join; cross-host latency. |
| **FIWARE** | The IoT Agent accepted the measure after a successful Orion update. Orion writes the entity to MongoDB with write concern **1 = acknowledged (applied in memory, not journaled)** and only then calls `processSubscriptions`; on write failure it returns without notifying. It does **not** establish durability, history, or sink receipt. | Orion sends a notification after the entity write; a sink receipt means the notification was delivered. Orion keeps only the **latest state**; the notification path is not a history store and can be lost. It is not a durability timestamp. | **No history reader.** `scripts/history-ids.sh fiware` exits 3 (“FIWARE-no-history”); D9 is open. Only the latest entity state exists. A storage reader cannot establish complete historical delivery or per-message loss. | **Ack:** load-host `scheduled_send_ms` → `response` wall clock after the Orion update (same k6 path; includes send lag). **Sink:** load-host schedule → sink `recv_ms` after the notification. No per-message Orion/Mongo commit time is captured; any SUT-side timestamp comparison is **cross-host and unqualified** [G]. | Generator outcomes/health; sink counts only (no ack→sink identity join); **no storage reconciliation**. Duplicates can be counted at the sink; loss after ack cannot be audited from latest state. Mongo operation counters are not per-message audits. | “Ack means durably stored”; “sink receipt means history”; “latest entity state proves every message arrived”; “Mongo op counts are delivered messages”; an ack→sink identity join; any completeness claim; cross-host latency. |

The table describes what the named evidence can establish, not guarantees the platforms make. The unsupported-claims
column is normative for reports.

### 5.1 Reading the table

- **Ack is not persistence** on all three platforms. The ack points differ: EdgeX at `device-rest`, ThingsBoard at
  the HTTP transport, FIWARE after the Orion update. `README.md:119` already states this; the table makes the
  consequence explicit.
- **Sink receipt is not storage** for EdgeX (upstream of persistence) and FIWARE (notification, latest state only).
  For ThingsBoard it is after the save callback, but still not a per-message committed-visibility timestamp.
- **Only EdgeX and ThingsBoard have a storage reader**; FIWARE has none (D9). Even there, the reader proves presence
  at query time, not commit time or durability.
- **Counts are not joins.** `planned_reached_sink` and the sink unique/duplicate counts are computed from sink NDJSON
  against planned sequence numbers; they are not joined to the ack/outcome ledger. `reconcile` joins only to stored
  identities. An “acknowledged but not at the sink” statement is an inference from counts and health, not a
  per-identity result.
- **The persistence counter is not a commit log.** It is a table-level cumulative insert counter; §5.4 states its
  window, sampling and conversion limits.
- **No cross-host latency is qualified.** The only same-host latency is the load-host ack/send path (and the sink by
  default, because it runs on the load host). Any latency that joins a load-host schedule to a SUT-side DB/Orion
  timestamp, or that compares the two VMs' clocks, needs a measured clock offset first. None exists [G].

### 5.2 Required fields for future latency reports

Every latency figure must state: (i) the observation point (ack / sink / storage), (ii) the clock of each endpoint,
(iii) whether the endpoints are on the same host, and (iv) the resolution of the source (per-message, sampled
counter, 5 s scrape). A rate figure must additionally state (v) the counter query, observation window and sampling
interval. A figure missing these fields is not comparable and must not be placed in a cross-platform table.

### 5.3 Loss/duplicate audit matrix

| Audit | EdgeX | ThingsBoard | FIWARE |
|---|---|---|---|
| Planned vs attempted vs ack | yes (k6 + health) | yes | yes |
| Ack but not reached sink | **counts/inference only** (`planned_reached_sink`, sink unique; no identity join) | **counts/inference only** (no identity join) | **counts/inference only** (no identity join) |
| Ack but not stored | yes, identity join (`history_reconciliation.ack_lost`) | yes, identity join, subject to same-ms overwrite | **no** (no history) |
| Stored identity duplicates | yes (history join) | limited: duplicates at `(entity,key,ts)` overwrite and collapse | **no** |
| Stored but ack unknown/failed | yes (`persisted_despite_*`) | yes | **no** |
| Per-message commit/visibility time | **no** | **no** | **no** |
| Commit/WAL/fsync durability evidence | **no** | **no** | **no** |

### 5.4 D23 goodput vs per-message committed visibility

D23 (`docs/design-decisions.md:31`) defines goodput as “durably stored and delivered”, evaluated at persistence,
with the sink kept as the latency observation point and a persistence-based verdict reported next to the sink-based
one. The **intended guarantee** is durability plus delivery. The **retained evidence** is weaker than that
guarantee.

Implementation: `verdict_persistence`/`outstanding_persistence_*` in `internal/explore/explore.go:105-123,245`, fed by
a cumulative stored-count series from `scripts/persist-series.sh`. For EdgeX the default series is
`pg_stat_user_tables_n_tup_ins{relname="reading",schemaname="core_data"}`, scraped every 5 s plus PostgreSQL's
statistics-flush delay (busy backends flush at most once per second). `explore -persist` subtracts the counter value
at the warm-up start, interpolates linearly between samples, and evaluates outstanding work over the same 10 s
windows. The export window runs from 5 min before warm-up to `DRAIN_S` (default 3600 s) after the last step, capped
at now; a counter decrease before warm-up restarts the series, and one after warm-up ends it.

Limits of that evidence:

- **Table-level and unfiltered.** `n_tup_ins` counts every insert into `core_data.reading`, not just this run. Probe
  readings, other traffic and other runs inside the window are included; the series carries no `run_id`.
- **Inserts, not commits.** The counter counts inserted tuples at execution. It does not show transaction outcome,
  WAL flush, `fsync`, replication, or reader visibility. A rolled-back or later-overwritten insert is not a durable
  stored message.
- **Duplicates count.** Re-inserts and duplicate deliveries increment the counter; the conversion from “rows” to
  “logical messages” relies on the workload sending one Object reading per request, which is true for the adapter but
  is not enforced by the counter.
- **Sampling and window.** 5 s scrapes plus flush delay mean short bursts and the first/last seconds are interpolated;
  the baseline and `DRAIN_S` choice affect the rate. Counter resets are handled only by the script's before/after
  warm-up rule.
- **No per-message identity or timestamp.** The history reader gives `(run_id, seq)` presence at query time; the
  persistence series cannot say which acknowledged messages were stored, or when a commit became visible.

D23's **verdict rule** is unchanged: it remains the declared rate/outstanding-work rule evaluated at persistence, and
`verdict_persistence` is reported next to the sink-based verdict. What must not change is the claim: the persistence
verdict is a **rate/goodput proxy**, not proof of per-message durability. `synchronous_commit=off` DIAG results
(D29c) do not supply durability evidence and do not authorize a main-compose change.

Rules for reports:

- Do not call `persisted_per_s_in_window` a latency.
- Do not difference cumulative counts to manufacture a per-message p99.
- Do not compare a D23 rate verdict with an ack p99 or a sink latency as if they were the same quantity.
- State the counter query, window and sample spacing; note that duplicates and other traffic are included.
- Keep the sink-based verdict and the persistence-based verdict labelled separately, as D23 requires.

## 6. Governance remediation (proposed wording only)

### 6.1 D11: dated addendum for the historical edit

**Facts [V].** D11 was edited in place in `c689191` (2026-10-22 16:48:13 +0330 = 13:18 UTC). Before the edit, D11
ended at “Postponed by the user (“decide later”); pilot uses N=1.” The edit added the exploratory OOM result and the
user's memory decision (N=1 exploratory: tb-core/rule-engine 1536m, transport 768m, heap unchanged; N=4 still open).

The chronology is:

- `EXPL-tb-k4` ran 2026-10-22 09:40:41–09:41:39 UTC (manifest git `7d06587`), **before** the edit. Its tb-core was
  OOM-killed at 1 GB during provisioning. It motivated the added text; it did not depend on the added text. Its
  manifest lists no memory deviation.
- `EXPL-tb-k4b` ran 2026-10-22 14:31:47–14:56:22 UTC (manifest git `671262e`), **after** the edit, with
  tb-core/rule-engine 1536m and transport 768m. It is the dependent benchmark run (its verdict is INVALID on
  generator health, not a passing capacity result). Its manifest lists only the outbound-delivery and 1000-device
  deviations; `results/exploratory/EXPL-tb-k4b.amendment.json` records the memory deviation. The added plan
  deviation text was committed in `c689191` after `671262e` was deployed, which is why the manifest lacks it.

The decision row still has no dated amendment. D11 is still “open”.

**Proposed append-only row [P]** (do not insert into the log until the owner approves):

> | D11-a | 2026-10-25 | D11 edit record (append-only addendum) | D11 was edited in place in `c689191` (2026-10-22 16:48:13 +0330 / 13:18 UTC). Original text ended: “Postponed by the user (“decide later”); pilot uses N=1.” The edit added the exploratory OOM result and the user's N=1 exploratory memory decision (tb-core 1536m, rule-engine 1536m, transport 768m, heap unchanged; N=4 still open). `EXPL-tb-k4` (09:40:41–09:41:39 UTC) ran before the edit and its OOM motivated the text; it did not depend on it. `EXPL-tb-k4b` (14:31:47–14:56:22 UTC) ran after the edit with that memory setting and is the dependent benchmark run (verdict INVALID on generator health); its manifest git `671262e` predates `c689191`, so the manifest lacks the deviation, and `results/exploratory/EXPL-tb-k4b.amendment.json` records it. This row records the in-place edit without rewriting D11. | proposed |

**Why this is the right shape [P].** It preserves the existing row and history, records the original and added text,
names the dependent run and the amendment, and does not pretend the edit was append-only. It also leaves D11's
substantive status unchanged.

### 6.2 D1: does it need the same treatment?

**Facts [V].** D1 was corrected in place in `72a18f7` (2026-10-22 04:27 +0330), before any dependent run. The row
itself already contains “**Correction (later the same day)**” and states what was wrong (the search depth). No
dependent result used the old text.

**Conclusion [P].** D1 does **not** need a D11-style addendum for disclosure: the correction is self-dated, visible in
the row, and pre-dependency. A strict append-only audit could still add an optional `D1-a` recording the commit and
the old text, but it would add little and would itself be another log edit. Recommended: **no change**; if the owner
wants uniform treatment, the optional row is:

> | D1-a | 2026-10-25 | D1 correction record (optional append-only note) | D1 was corrected in place in `72a18f7` before any dependent run. Old text said the notes did not exist; the corrected text records the notes' location and the search-depth error. The correction is already dated in the D1 row; this note is only an audit trail. | proposed |

### 6.3 D25 clarification for upstream backports

**Proposed append-only row [P]** (only if D27 is approved in some form):

> | D25-a | 2026-10-25 | Upstream backports (clarification) | D25's Category A has two limbs: (a) official documentation recommends the change, or (b) it fixes a defect in our deployment. An upstream fix absent from the pinned release is neither by itself. A backport may be treated as A-equivalent only if the owner records it as an explicit exception, it leaves ack point, durability, delivery guarantee and batching semantics unchanged, and the application's own write/provisioning path is validated not to gain new errors. | proposed |

This does not rewrite D25; it records how the owner wants the gap handled. If the owner prefers, D27 can be approved
as a one-off exception without adding a general rule.

## 7. Unresolved evidence gaps

1. **D27 changelog/PR**: the 4.8.0 `CHANGES_NEXT_RELEASE` excerpt block is empty and a direct fetch returned no
   body; `#1576` is not independently verified. The `Device.js` code at 4.8.0 is verified.
2. **D27 data and provisioning gate**: the existing `iotagentul.devices` data has not been audited for duplicate
   `(service, subservice, apikey, id)` tuples; the audit can only report, not repair. The effect of the unique
   constraint on the agent's concurrent `findOrCreate`/provisioning path is unmeasured. Index creation is conditional
   on a clean audit and a passing provisioning check; a conflict stops the change.
3. **D28 logback/log path**: the official conf writes under `/var/log/thingsboard`; no such mount exists here. The
   exact implementation (official conf + log volume vs reviewed stdout-only logback) is an owner/implementation
   choice, not settled by the upstream source.
4. **D28 OOM behavior**: no controlled OOM test exists; the claim that the JVM restarts correctly under the compose
   restart policy is a source-based expectation, not a measured result.
5. **D29 disk metric**: node-exporter gives host/device-level averages, not per-transaction or WAL-only latency; the
   diagnostic's 5 s steps and the `synchronous_commit=off` 14.66 ms await show the metric is sensitive to write
   batching. Do not over-interpret it.
6. **D29b topology**: no 8-core-data run exists. The group-commit benefit is a hypothesis; the memory and connection
   costs are estimates from compose limits, not measured.
7. **Orion pool**: no A/B exists. The official guidance is written per process; every replica shares the same
   4-core cpuset, so 4 threads per process is at the visible core count at k = 4 (up to 16 threads across replicas
   on those 4 cores) and above it at k = 1. The guidance does not resolve multi-process shared-cpuset sizing, so
   “4 violates the guidance” is not supported; the aggregate oversubscription risk is unmeasured.
8. **TB queue values**: no official recommended value and no A/B; the Category A question is unresolved.
9. **D30 ack handlers**: the EdgeX device-rest handler excerpt is not in the pinned evidence package; its ack point is
   established by `README.md:119` and architecture. The ThingsBoard handler is now pinned (`DeviceApiController` +
   `JsonConverter`), but the exact per-message DB commit/visibility time is still not exposed.
10. **D30 ack→sink join**: no implemented identity join exists between acknowledged messages and sink receipts. Sink
    figures are counts from sink NDJSON; `reconcile` joins only to stored history. Any ack-but-not-sink statement is
    an inference, not a per-identity audit.
11. **D23 persistence evidence**: the series is a table-level, unfiltered cumulative insert counter, sampled every
    5 s plus PostgreSQL flush delay; it counts inserts, not commits, and includes duplicates, probes and other
    traffic. It cannot establish per-message durability or committed visibility. D23's verdict rule is unchanged.
12. **FIWARE history**: D9 remains open; no storage reader can prove complete historical delivery, and no
    per-message FIWARE commit/visibility latency exists.
13. **Clock offset**: no measured offset between ramonavm1 and ramonavm2. Any cross-host latency is unqualified.
14. **Remote state**: the SUT/load hosts were not reachable at the application layer on 2026-10-25 (TCP handshakes
    completed, no SSH banner/HTTP data). No remote state was changed or confirmed.
15. **D30 original referent**: the “README table in Step 5” does not exist; this packet supplies the missing table.

## 8. Verification record for this packet

- Read: `docs/handoff-review-2026-10-25.md`, `evidence/reproducibility.md`, `docs/design-decisions.md`,
  `docs/operations/diagnostics.md`, `evidence/source-excerpts.md`, `docs/methodology/metrics-dictionary.md`,
  `docs/development/tooling-interfaces.md`, `README.md`, and the implementation files cited above.
- Fetched at pinned upstream tags: iotagent-ul 3.7.0 `package.json`; iotagent-node-lib 4.7.0 and 4.8.0
  `lib/model/Device.js`; ThingsBoard v4.3.1.6 `docker/docker-compose.yml`, `docker/tb-node/conf/thingsboard.conf`,
  `docker/tb-node/conf/logback.xml`, `msa/tb-node/docker/start-tb-node.sh`,
  `common/transport/http/.../DeviceApiController.java`, `common/proto/.../adaptor/JsonConverter.java`, and
  `dao/.../sqlts/insert/sql/SqlInsertTsRepository.java`; GitHub releases API for iotagent-ul.
- Inspected retained evidence: `results/exploratory/EXPL-tb-k4/manifest.json`,
  `results/exploratory/EXPL-tb-k4b/manifest.json`, `results/exploratory/EXPL-tb-k4b/explore.json`, and
  `results/exploratory/EXPL-tb-k4b.amendment.json`, plus the implementation files cited above.
- Did not edit the decision log, generated diagnostics, code, configuration, plans or evidence; did not run an
  experiment; did not change a server; did not commit or push.

## 9. Owner decision form

This form is proposed. An unchecked box, a recommendation in this packet, or an approval of another row is **not**
owner approval. Mark a disposition and, separately, each authorization level the owner intends.

Four levels are used:

- **Text** — adopt the quoted decision/addendum text (for D25-a, D11-a and D1-a: append the quoted row; for D30:
  adopt the §5 table). It authorizes no code, configuration, deployment or run.
- **Impl/checks** — make the named local code/configuration change and run local/static/unit/dry-run checks. It
  authorizes no SUT deployment and no experiment.
- **Deploy** — apply the named change to the SUT hosts and restart the named services. It authorizes no load run.
- **Runs** — execute the named smoke, diagnostic or benchmark run and record its evidence. It authorizes no
  configuration change.

Text → Impl/checks → Deploy → Runs is an ordering, not an implication: ticking one level does not tick the next.

### 9.1 Disposition and levels

| ID | Candidate | Recommended | Disposition (mark one) | Levels (tick only what is authorized) |
|---|---|---|---|---|
| D27 | FIWARE device index | Approve as explicit A-equivalent backport, conditional on D25-a and provisioning validation | `[ ] Approve` `[ ] Reject` `[ ] Defer` | `[ ] Text` `[ ] Impl/checks` `[ ] Deploy` `[ ] Runs` |
| D28 | TB `/config` mount | Approve as Category A defect fix | `[ ] Approve official conf + log volume` `[ ] Approve stdout-only logback` `[ ] Reject` `[ ] Defer` | `[ ] Text` `[ ] Impl/checks` `[ ] Deploy` `[ ] Runs` |
| D29a | EdgeX disk reporting | Approve | `[ ] Approve` `[ ] Reject` `[ ] Defer` | `[ ] Text` `[ ] Impl/checks` |
| D29b | More core-data replicas | Defer | `[ ] Defer` `[ ] Approve labelled topology ablation` `[ ] Reject` | `[ ] Text` `[ ] Impl/checks` `[ ] Deploy` `[ ] Runs` |
| D29c | `synchronous_commit=off` | Keep DIAG-only; reject for qualification | `[ ] Keep DIAG-only (no change)` `[ ] Reject entirely` `[ ] Defer (no change)` | No levels — no change. A future D25 amendment and crash test are separate and are **not** authorized here. |
| D29d | `commit_delay` | Defer separately | `[ ] Defer` `[ ] Reject` `[ ] Approve separate durable test` | `[ ] Text` `[ ] Impl/checks` `[ ] Deploy` `[ ] Runs` |
| D30 | Observation points | Approve corrected table | `[ ] Approve §5 table` `[ ] Defer` `[ ] Reject` | `[ ] Text` only |
| — | Orion `-reqPoolSize` | Defer | `[ ] Defer` `[ ] Approve topology-aware test` `[ ] Reject` | `[ ] Text` `[ ] Impl/checks` `[ ] Deploy` `[ ] Runs` |
| — | TB queue poll intervals | Defer | `[ ] Defer` `[ ] Approve named A/B after Category A ruling` `[ ] Reject` | `[ ] Text` `[ ] Impl/checks` `[ ] Deploy` `[ ] Runs` |
| D25-a | Backport clarification | Approve if D27 approved | `[ ] Approve` `[ ] Reject` `[ ] Defer` | `[ ] Text` only |
| D11-a | D11 historical edit | Approve | `[ ] Approve` `[ ] Reject` `[ ] Defer` | `[ ] Text` only |
| D1-a | D1 historical correction | Not required | `[ ] No change` `[ ] Add optional D1-a` | `[ ] Text` only (if D1-a is added) |

### 9.2 Scope authorized at each level

| ID | Text | Impl/checks | Deploy | Runs |
|---|---|---|---|---|
| D27 | Record D27 as an explicit A-equivalent backport; append D25-a only if D25-a Text is also ticked; no rewrite of D25. | Add the exact unique index to the init path for `iotagentul.devices`; duplicate audit that **stops and reports** on conflict (no deletion, merge or identity rewrite); `explain` and agent provisioning-path checks; deviation text. | Run the init path on the SUT; restart the agent only as needed; verify the index and a clean start. No other FIWARE compose/.env change. | Fixed-load FIWARE smoke at the same offered rate and k as the baseline, 3 repetitions; ack, errors, MongoDB CPU per message, sink/history. Record the deviation. |
| D28 | Record the chosen variant and the deviation. | Vendor the pinned v4.3.1.6 `thingsboard.conf` + `logback.xml`; mount `/config:ro` on tb-core and tb-rule-engine; add the chosen writable log volume or reviewed stdout-only logback; `docker compose config` and hash check. | Restart tb-core and tb-rule-engine; verify mounts, effective JVM flags and readable logs. | Baseline-load smoke checking ack/sink/storage. A controlled OOM test is a separate DIAG and is not authorized by this row. |
| D29a | Adopt the reporting fields as requirements. | Extend EdgeX result/report generation (`disk_write_await_ms`, `disk_writes_per_s`; WAL fields only where the pgstat overlay exists); no verdict change. | — (no platform change; existing exporter/diagnostic overlay). | — (no dedicated run; the reporting applies to runs authorized elsewhere). |
| D29b | Record as a labelled topology ablation, not a qualification configuration. | Compose replica/limit changes; record memory and connection budget. | Restart with 8 core-data replicas on the existing 4 cores. | The labelled k=4 ablation only; not a qualification run. |
| D29c | — (no change; no future-amendment approval). | — | — | — |
| D29d | If approved, record a separate durable group-commit test; no durability weakening. | Test-only compose overlay; not main compose/.env. | Only the test stack. | The separate durable group-commit test, distinct from D29c. |
| D30 | Adopt §5 as the D30 text; the unsupported-claims column is normative. | — (documentation only). | — | — (validation is a desk check against retained runs). |
| — Orion | Record the chosen value and the shared-cpuset/aggregate interpretation. | Set `ORION_REQ_POOL_SIZE` in compose/.env; record the deviation; no other Orion change. | Restart the Orion replicas. | Fixed-k A/B of 0, 1 and 4 per replica; record aggregate threads, shared cpuset and per-process visible CPUs. |
| — TB queue | Record the Category A ruling and the named knob family/value. | Set the named `TB_QUEUE_*_POLL_INTERVAL_MS` variables; record the deviation. | Restart the TB services. | Fixed-load A/B, one knob family at a time; idle/loaded CPU, latency, throughput and consumer lag. |
| D25-a | Append the §6.3 row; no rewrite of D25. | — | — | — |
| D11-a | Append the §6.1 row; no rewrite of D11. | — | — | — |
| D1-a | If chosen, append the §6.2 row; otherwise no action. | — | — | — |

### 9.3 Dependencies and ordering

- A level for an ID requires the preceding levels for **that same ID** to be ticked: Impl/checks requires Text,
  Deploy requires Impl/checks, Runs requires Deploy. A level marked “—” cannot be ticked.
- D27 Impl/checks, Deploy and Runs additionally require D25-a Text or a one-off exception recorded in D27 Text.
  D27 Deploy and Runs also require the duplicate audit to be clean and the provisioning check to pass; a conflict
  stops the change and is reported, not repaired.
- D28 is independent.
- D29a, D29b, D29c and D29d are independent of each other and of D27/D28.
- D30 is text-only and independent; it does not imply approval of D29c or of any platform change.
- Orion and TB queue changes require the Category A ruling in their Text level before Impl/checks or Runs. No run is
  authorized before that ruling.
- D25-a, D11-a and D1-a are text-only and authorize no platform work.

### 9.4 What no approval here authorizes

Only the levels ticked for that ID are authorized. Nothing else is authorized or implied:

- No other row, no unticked level, no unlisted file, and no change outside the scope cell for the ticked level.
- **Text** authorizes only the quoted adoption/append. It does not authorize rewriting D25, D11, D1 or any existing
  decision row, and it does not authorize editing generated `docs/operations/diagnostics.md`.
- **Impl/checks** authorizes only the named local code/configuration and checks; it does not authorize deployment
  or an experiment.
- **Deploy** authorizes only the named SUT services/configuration and restarts; it does not authorize a run.
- **Runs** authorizes only the named run type/load; it does not authorize a configuration change, and its evidence
  is recorded under `results/`.
- No approval authorizes editing raw evidence, other existing decision rows, unrelated compose/.env files, or any
  plan or manifest beyond the named deviation entry.
- No approval authorizes a commit or push. No approval authorizes a server change outside a ticked Deploy, or an
  experiment outside a ticked Runs.
- Unchecked boxes, blank fields, this packet’s recommendations, and approvals of adjacent rows are **not** owner
  approval.

