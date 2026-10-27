# FIWARE history design (D9 → D31)

**Status: local design only. Not deployed, not verified against a pinned QuantumLeap image, not authorized for any run.**
Recorded 2026-10-26 under D31 (selection by ChatGPT under the owner's delegation, D41). This document lists what is
verified, what is only upstream-documented, and what is still unknown. Nothing here is a result.

Evidence labels: **[V]** verified in this checkout or the retained evidence; **[U]** read from upstream documentation
(URL given) on 2026-10-26; **[P]** proposal; **[G]** unknown or unverified.

---

## 1. Requirement

From D31 and the Step 5 sign-off (§5.1):

- Path: IoT Agent → Orion → **two independent Orion subscriptions**: one to QuantumLeap (history), one to the external
  sink (latency and delivery observation). [V] `docs/preregistration/original/decisions.md` (pre-registered path).
- Each `(run_id, seq)` must be an **independently retrievable logical event**: one stored row per notification, with
  the identity queryable, not only the latest entity state.
- No custom history writer unless the owner selects fallback B (D31 forbids silent substitution).
- The added services must fit the fixed supporting-tier CPU and memory budget.

## 2. Current FIWARE stack [V]

`fiware/compose.yaml` services (verified by reading the file):

| Service | Role | Cpuset | Memory limit (default) |
|---|---|---|---|
| `mongo` | Orion and IoT Agent storage; **not** a history store | `${INFRA_CPUSET:-2-3}` | `${MONGO_MEM:-2g}` |
| `mongo-init` | one-shot index job (`restart: "no"`) | infra | 256 MiB |
| `orion` (×N) | NGSI-v2 broker, `-reqMutexPolicy none` | `${POOL_CPUSET:-4}` | `${ORION_MEM:-512m}` each |
| `iot-agent` (×N) | IoT Agent UL 3.7.0 | `${POOL_CPUSET:-4}` | `${IOTA_MEM:-512m}` each |
| `haproxy` | load balancer | `${LB_CPUSET:-1}` | `${LB_MEM:-256m}` |

Orion keeps only the latest value of an entity. **There is no history service in this stack** (D9 open, handoff
review item 9, `scripts/history-ids.sh fiware` exits 3).

Steady-state memory limits at N=4 with the defaults: 2,048 (mongo) + 4 × 512 (orion) + 4 × 512 (iot-agent) + 256
(haproxy) = **6,400 MiB**. The one-shot `mongo-init` is excluded (it does not run during a run).

## 3. Upstream QuantumLeap facts [U] — source review of the pinned release

Reviewed 2026-10-26/27 against the **pinned release `1.0.0`** of `orchestracities/ngsi-timeseries-api` (tag
`1.0.0`, commit `f8fd27b90aa2`, released 2023-12-01). Tag list checked via the GitHub API: nothing newer exists
except two back-dated `FIWARE_*` tags (`FIWARE_7.9` = release `0.7.6` of 2020, `FIWARE_7.5.1` = release `0.5` of
2018), so `1.0.0` is the latest release; `master` has later commits (last 2024-10-18) but no release. The
reviewed source files were re-fetched and checked against their git blob IDs, so the statements below are about
the release's bytes. Copies of the reviewed files (blob-ID verified) can be re-created with
`curl https://raw.githubusercontent.com/orchestracities/ngsi-timeseries-api/1.0.0/<path>`.

### 3.1 Version matrix [U]

| Component | QL `1.0.0` tested | This repo |
|---|---|---|
| Orion | **2.4.2 – 3.3.1** (CI matrix `.circleci/config.yml`; `deps.env` pins `ORION_VERSION=3.3.1`) | `fiware/compose.yaml` pins **Orion 4.5.0** |
| Timescale backend | **PostgreSQL 12–13, TimescaleDB ext 1.7.5–2.3.0, PostGIS 2.5.\*** (README §Install at tag `1.0.0`; CI matrix `["2.3.0-pg13","1.7.5-pg12"]`) | none yet (D31 selects QL + TimescaleDB) |
| Redis | 4.–`6.2.3` (`deps.env`); needed only for the work queue and the geocoding cache | — |

Discrepancy recorded: the upstream docs site (readthedocs "latest", read 2026-10-26) claims PostgreSQL 12–16 /
TimescaleDB 2.11.2–2.16.1 / PostGIS 3.\*, but the README **at tag `1.0.0`** — the release we would pin — says
PostgreSQL 12–13 / ext 1.7.5–2.3.0 / PostGIS 2.5.\*. The site documents `master`, not the release. For a 1.0.0
pin, the tested range is the README/CI range in the table.

### 3.2 Data model — verified from the pinned source [U]; not yet verified live [G]

From `src/translators/sql_translator.py` and `src/translators/timescale.py` (tag `1.0.0`):

- Schema per `Fiware-Service` (`"svc"` prefix), **table per entity type**; one column per attribute plus standard
  columns `entity_id`, `entity_type`, `timeindex`, `fiwareservicepath`, `instanceid`. New attributes extend the
  table at insert time (`alter table … add column if not exists`).
- Table creation: `create table if not exists` → `create_hypertable(..., if_not_exists => true)` → index
  `ix_<table>_eid_and_tx`, **non-unique**, on `(entity_id, timeindex desc)`. Data tables have **no primary key
  and no unique constraint**; only the metadata table has one.
- **There is no upsert.** Every notification is a plain `insert into … values …` (`_insert_entity_rows`, batched
  via `insert_splitter.py`). There is no `on conflict` clause and no dedupe key.
- **Same timestamp ⇒ two separate rows.** Two notifications of the same entity with an identical `timeindex`,
  and likewise a re-delivered notification, each insert a row; nothing collapses. Favourable for the audit:
  per-notification rows, `(run_id, seq)` counting works, duplicates stay countable.
- `timeindex` selection (priority list in `src/reporter/timex.py`): custom header attribute
  (`Fiware-TimeIndex-Attribute`) → `TimeInstant` → `timestamp` attr/metadata → `observedAt`/`modifiedAt` →
  **`datetime.now()` at notification arrival as the last resort**. Without a time attribute in the payload, row
  timestamps are arrival times, not event times; the subscription should send `(run_id, seq)` attributes **and**
  `TimeInstant` (or an `httpCustom` time-index header) so identity counting does not depend on arrival time.
- Failure path: on a `pg8000.ProgrammingError` (e.g. a value that does not fit the column type), the batch is
  **not** silently dropped: the original JSON of the batch's entities is kept in the table's
  `__original_ngsi_entity__` error column (`_insert_original_entities_in_failed_batch`) with a batch id. Such
  rows are countable in `psql`, but the identity sits inside a JSON blob, not in a column; the history export
  must account for them. Note: a single bad value can fail a whole insert batch, so the error path covers the
  batch, not only the offending row.

### 3.3 Delivery path — what an HTTP 200 means — verified from the pinned source [U]

From `src/reporter/reporter.py` and `src/wq/*` (tag `1.0.0`):

- `notify()` validates every entity (one invalid entity rejects the **whole notification**, HTTP 400), computes
  `timeindex`, then calls `InsertAction(...).enqueue()`.
- `enqueue()` branches on `WQ_OFFLOAD_WORK`, which **defaults to False**: by default the insert runs
  **synchronously in the API request thread**, and the connection is `autocommit = True`. HTTP 200 therefore
  means the rows were inserted; it does **not** mean *durable commit*. The history export stays a facts check,
  never a latency measurement — the same rule as for the EdgeX sink.
- With offloading on (`WQ_OFFLOAD_WORK=true`, requires Redis, `docker-compose.wq.yml` layout), 200 means only
  *enqueued*; workers consume later; `WQ_MAX_RETRIES` defaults to **0** (no retries),
  `WQ_RECOVER_FROM_ENQUEUEING_FAILURE` defaults to False (an enqueue failure raises, the notification is lost).
- **Design conclusion [P]: run QL 1.0.0 in the default synchronous mode.** The notify path then needs no Redis
  and no worker container. Enabling the work queue without a dedicated durability analysis stays out of scope
  (D42).
- The `1.0.0` release notes add two integration-relevant points: QuantumLeap's own subscription API was
  **removed** (#493) — subscriptions are created on the Orion side, which matches the pre-registered design of
  two independent Orion subscriptions; and a missing `Fiware-ServicePath` is now treated as `/` (#564, breaking
  change relative to ≤ 0.8.x; relevant only to manual injection, not to Orion notifications).

### 3.4 Resource needs [U] — input to placement (B4); no decision here

From `docker/docker-compose.yml`, `docker/docker-compose.wq.yml` and `timescale-container/` at tag `1.0.0`: QL
image `orchestracities/quantumleap:1.0.0` (Python/Flask) + Timescale DB (e.g. `timescale/timescaledb-postgis:2.3.0-pg13`
— the image family is deprecated upstream, but a `1.0.0` pin constrains the tested set) + one bootstrap step
(`quantumleap-db-setup`). Redis is needed only for the work queue or geocoding. Set `USE_GEOCODING=false`:
geocoding is active only if the flag is set **and** a Redis host is configured (`src/geocoding/factory.py`), so
with it unset the notify path needs no Redis and makes no external geocoding HTTP calls. QL and the Timescale DB
both need explicit, measured memory limits and cpu placement inside the fixed supporting tier; neither image is
known to be small, and measured footprints are missing. **No limit is chosen or changed by this document.**

### 3.5 Compatibility gap [V] + [U] — blocker B3, unchanged in substance

The compose file pins **Orion 4.5.0** (`ORION_VERSION:-4.5.0`). QL `1.0.0`'s tested Orion range is 2.4.2–3.3.1;
there is upstream evidence for nothing above 3.3.1, much less 4.5.0. Resolution paths (owner decision either
way):

1. **Keep Orion 4.5.0 and test the pair live:** a throwaway Orion 4.5.0 container under the existing stack
   configuration, with an NGSI-v2 subscription posting labelled payloads to a QL 1.0.0 container; verify the row
   shape and identity retention. This extends an untested pairing beyond upstream evidence and needs a live
   authorization.
2. **Downgrade OCB to 3.3.1**, which changes the platform under test (S5: a fingerprint change is a recorded
   deviation). Not chosen [P].

### 3.6 Effect on blocker B2 — narrowed, not cleared

The previously unverified part of B2 (data-model shape, upsert vs insert, same-timestamp behaviour) is now
answered by the release's own source (§3.2): one row per notification, plain inserts, no collapse. B2 remains
open **only** because reading source is not a live result: the pinned-image check (§4 step 3) is retained, and a
mismatch between the pinned image and this reading would restore B2 as a full blocker.

## 4. Verification plan (before any FIWARE deployment)

Taken from the sign-off's "recommended validation plan" and made concrete. Every step is local or offline until
step 9.

| Step | Action | Pass condition | Status |
|---|---|---|---|
| 1 | Owner decision A (QuantumLeap + TimescaleDB) or B (small writer) and supporting-tier placement | Recorded in `docs/design-decisions.md` | **D31 selects A**; placement open (B4) |
| 2 | Pin QuantumLeap and TimescaleDB images by digest; resolve the Orion 4.5.0 compatibility gap (B3) | Pins recorded; compatibility shown | **Open** (B2, B3); §3.1 gives the tested ranges |
| 3 | Verify the pinned data model and timestamp behaviour: row per notification? upsert key? same-timestamp collapse? | Written verification record with source/behaviour evidence | **Partially answered by source** (§3.2: one row per notification, plain insert, no collapse) — the pinned-image check is still required before approval (B2) |
| 4 | Implement `scripts/history-ids.sh fiware` against the history store: `(run_id, seq)` per row | Returns one row per stored notification | **Not implemented**; currently exits 3 |
| 5 | Replay N known messages: N rows, N distinct identities, zero duplicates, zero missing; no dependence on latest state | Known-answer result | **Not run** (needs stack) |
| 6 | Inject one duplicate and one dropped notification | Duplicate countable; dropped one appears as acknowledged-but-not-stored | **Not run** |
| 7 | Restart the history service and the DB; repeat the query; wipe and cold-start; repeat | Identical results | **Not run** |
| 8 | Measure added CPU per message and memory working set; confirm the supporting-tier budget | Within the fixed budget | **Not run**; B4 |
| 9 | Only then authorize a FIWARE smoke test, and later qualification runs | Separate owner authorization | **Not authorized** |

### Stop conditions (from the sign-off, unchanged)

Stop and report; do not delete or rewrite data if: rows collapse or are overwritten; the query returns latest state
only; any acknowledged message cannot be accounted for; the added services exceed the memory or CPU budget; or the
history path cannot be distinguished from the sink path.

## 5. Identity query and detection (what the audit will consume)

- Identity: `(run_id, seq)`, carried in the payload. A history export is NDJSON `{"run_id","seq"}`, optionally with
  `device` and `ts` (`benchctl audit -history` reads both; see `docs/measurement-integrity.md`).
- Missing: acknowledged identity with no history row. Duplicate: more than one row for one identity, counted
  separately from the sink's duplicates.
- Ambiguous: an identity with an unknown or failed outcome that nonetheless appears in history or at the sink.
- The export for FIWARE does not exist yet. Until step 4 passes, FIWARE results cannot carry store-side
  correctness, and correctness-qualified goodput is **unavailable** for FIWARE, not estimated.

## 6. Resource accounting (B4, owner decision)

Not decided here. The supporting tier is fixed (infra cpuset 2–3, LB core 1; `scripts/check-resources.sh` refuses
to start above the host budget minus 1,536 MiB). QuantumLeap, TimescaleDB and any extra worker need explicit
memory limits and CPU placement. Adding them inside the fixed budget may require lowering another limit, which is
an owner decision. **No memory limit is chosen or changed by this document.**

## 7. Proposed integration (local design only [P]; nothing implemented)

Under the existing `fiware/compose.yaml` layout (read, unchanged in this step):

- Two added services: `quantumleap` (image `orchestracities/quantumleap:1.0.0` pinned by digest) and
  `timescale` (PG 13 + TimescaleDB ext within the README range of §3.1, pinned by digest). Run in the default
  synchronous mode (`WQ_OFFLOAD_WORK` unset/false): no Redis service, no worker. `USE_GEOCODING` unset.
- Bootstrap: one one-shot service running `quantumleap-db-setup` against the Timescale service (modelled on
  `mongo-init`), plus a `scripts/history-ids.sh fiware` read path using `psql` against the QL schema.
- Orion side: the IoT Agent → Orion subscription for history is created by the existing provisioning script with
  `httpCustom` including `TimeInstant`/time-index and the full payload (so `(run_id, seq)` land as own columns).
- Placement: supporting tier, explicit `mem_limit`/cpusets recorded in the run manifest; B4 gate before compose
  merge.

### 7.1 Smallest live test sufficient for B2 step 3 (needs authorization)

1. Start only `quantumleap` + `timescale` + bootstrap (no Orion, no IoT Agent) on any authorized host.
2. `curl POST /v2/notify` four hand-made payloads (bypassing Orion) for one entity id/type with attributes
   `run_id`, `seq`, and an explicit time index: (a) payload 1; (b) payload 1 again — a duplicate; (c) same
   timestamp as 1 — a same-index neighbour; (d) a value whose type mismatches payload 1 (to exercise the
   failure path; note QL `1.0.0` insert batches may fail as a whole on such a value, re-storing **all** of the
   batch's entities as error-column rows — what matters is that no identity is lost, not which column it lands
   in).
3. `psql` select: every identity of (a)–(d) must be retrievable — normally four rows, but if (d) trips the
   failure path, the identity of (a)–(d) may appear in error-column rows; nothing collapses, duplicates stay
   countable. A lost identity, a collapsed timestamp, or an unaccounted row count stops the test and reopens B2.
4. Then (only if supported by the test above) the Orion 4.5.0 one-shot test of §3.5 option 1: Orion → QL end to
   end for 10 labelled updates. This probe is enough to clear or confirm B3 as a live data point; it does not
   qualify the platform (that is §4 step 5 onward).

## 8. What this document does not authorize

- No FIWARE deployment, restart or registry change.
- No index creation (D27 is a separate, separately authorized step; see `fiware/scripts/device-index.sh`).
- No custom history writer (fallback B is not selected).
- No claim that the **pinned image** behaves exactly as the pinned source reads (§3.2–§3.3 are source facts, not
  live results; §4 step 3 must still run).
- No Orion-version change, no compose merge of §7, no live test of §7.1.
