# Post-processing plan — W1–W4 fixed-point campaign

**Status:** specification only. No code change is authorized by this document.
**Authored:** 2026-11-16, while the W1–W4 queue is **ACTIVE**.
**Audience:** whoever implements the post-processor on an idle queue.

> **Superseded in part — 2026-11-18.** Commit `92ca31e` implemented §9 items 1–3
> and 8 and fixed the hard-coded repetition sentence: `scripts/aggregate-runs.py`
> is now 357 lines, with `harness_defect_of()` at line 36 and the defect check
> inside `load_run()` (line 144, `campaign_qualified` at 156–161). The census in
> §1 and the drift-acceptance bullet in §11 no longer describe the tree. The body
> below is retained as the original specification, not as a description of current
> code; re-read the file before implementing the remaining steps (4–7).

This plan describes how to turn `results/workload-batch-20261108/` into campaign
tables. It does **not** authorize editing any receipt-bound file. Read `CLAUDE.md`
first; where this document and `CLAUDE.md` differ, `CLAUDE.md` wins.

---

## 0. Constraints that outrank convenience

1. **`scripts/aggregate-runs.py` is receipt-bound.** In
   `workloads/campaigns/fixed-points-w1-w4/authorization/owner-w-matrix3-20261116i.json`
   the `bindings_sha256` object has 13 keys and the one for this script is
   **`aggregator`**; `authorized_plan_sha256` has 24 plans. `CLAUDE.md` §1 names
   `scripts/aggregate-runs.py` in the bound set. While the queue is ACTIVE it may
   be read, never modified, formatted, or committed. Both `h` and `i` receipts
   verified clean (`no drift`, 24 run IDs) at authoring time.
2. **Evidence is immutable.** Never delete or mutate
   `results/workload-batch-20261108/evidence/**`, `driver-log.jsonl` (including
   the `.pre-repair` copy), or `supervisor-log.jsonl` (`CLAUDE.md` §3).
3. **Derived output goes to a new, uniquely named directory.** Never overwrite an
   existing aggregate, report, `summary.md`, or `runs.csv`; never use `--force`.
4. **`results/**` is gitignored** except narrow unignores. Never stage outputs.
5. **Markdown and CSV only.** There is no LaTeX generator.
6. **Message-level percentiles are never pooled** across runs or messages. Report
   per-run values and min/median/max across eligible repetitions.

---

## 1. Current campaign state (closing census, 2026-11-19)

`scripts/census-w-matrix.py --expect 3` is the exclusion-authoritative census
(exit 1 = cells short). At authoring time (2026-11-16) only 2 cells had 3 reps;
the campaign has since closed at **12/12 cells with 3 eligible repetitions**
(census exit 0):

| Cell | n | Status | Repetitions |
|---|---:|---|---|
| W1/edgex | 3 | OK | r2@o23, r3@o31, r4@o42 |
| W1/fiware | 3 | OK | r2@o22, r3@o33, r4@o41 |
| W1/thingsboard | 3 | OK | r2@o24, r6@o61, r7@o62 |
| W2/edgex | 3 | OK | r2@o13, r3@o36, r4@o44 |
| W2/fiware | 3 | OK | r1@o4, r2@o15, r3@o35 |
| W2/thingsboard | 3 | OK | r3@o34, r6@o63, r9@o68 |
| W3/edgex | 3 | OK | r1@o7, r2@o18, r3@o26 |
| W3/fiware | 3 | OK | r2@o17, r4@o48, r5@o56 |
| W3/thingsboard | 3 | OK | r3@o27, r8@o69, r9@o70 |
| W4/edgex | 3 | OK | r1@o12, r3@o28, r4@o39 |
| W4/fiware | 3 | OK | r1@o11, r2@o19, r3@o30 |
| W4/thingsboard | 3 | OK | r3@o29, r4@o37, r5@o60 |

- **Ordinal 21** (`w4dp-tb-r02-20261108`) is excluded as a harness defect.
- **Ordinals 22 and 24** are retained as `VALID_PLATFORM_FAILURE_OR_BACKLOG`.
- **22 unusable attempts** are spent and excluded from denominators, including
  **ordinal 32** `w1dp-tb-r03-20261108` (`NOT_ASSESSED_NO_ANALYSIS`,
  `launcher-error.json`). Ordinal 32 did **not** leave W1/thingsboard short: that
  cell closed via r2@o24, r6@o61, r7@o62, so no backfill is required.
- A repetition not on disk is *pending*, not a pass.

Re-run the census at implementation time; do not hard-code this table.

---

## 2. Sources of truth and how they join

| Concern | Authoritative source | Notes |
|---|---|---|
| ordinal, workload, platform, repetition, expected message counts | `workloads/campaigns/fixed-points-w1-w4/run-matrix.json` → `experimental_runs[]` | Receipt-bound; read-only. |
| per-run outcome and gates | `evidence/<run_id>/run-result.json` | May be absent. |
| per-phase latency and traffic integrity | `evidence/<run_id>/<run_id>.dispatch-analysis.json` | Percentiles are precomputed here. |
| capture window / sink counters | `evidence/<run_id>/<run_id>.capture-window.json` | Duplicated inside dispatch-analysis. |
| raw run artifacts | `evidence/<run_id>/<run_id>/` | `health.json`, `gen-cpu.ndjson`, `cpu-snapshots.jsonl`, `k6-summary.json`, `records.ndjson`, `manifest.json`. |
| exclusion disposition | `evidence/<run_id>/harness-defect.json` | Present only where a defect was filed. |

**Join key:** `run_id` (e.g. `w3dp-ex-r01-20261108`). Never infer ordinal,
workload, or repetition from the platform prefix; `w1dp-tb-*` and `w2dp-tb-*`
share a platform and must never be pooled.

`experimental_runs[]` entry shape (verified):

```json
{
  "ordinal": 1,
  "workload_id": "W1",
  "platform": "thingsboard",
  "repetition": 1,
  "run_id": "w1dp-tb-r01-20261108",
  "seed": 202611061,
  "plan": "plans/w1dp-tb-r01-20261108.json",
  "plan_sha256": "…",
  "expected_warmup_messages": 15000,
  "expected_measurement_messages": 30000,
  "deployment": { … },
  "execution_status": "…"
}
```

---

## 3. Evidence manifest actually on disk

Per-run directory `evidence/<run_id>/`, with these files observed across runs
(counts are how many of the 43 evidence directories carry them):

| File | Count | Role |
|---|---:|---|
| `run-result.json` | 41 | Gate booleans, `platform_result`, `finish`. |
| `staged-source.json`, `.sha256`, `.tar.gz` | 43 | Source provenance. |
| `resolved.json` | 43 | Resolved deployment. |
| `plan-overlay.json` / `plan.overlay.json` | 29 each | Per-run plan overlay. |
| `launch-ssh.json`, `launch.log`, `launch.pid`, `launch-exit-code` | 30 | Launch evidence. |
| `effective-resources-before.json`, `-after.json` | 33 / 30 | Configuration and quota, **not** utilization. |
| `cleanup-verification.json` | 31 | Cleanup gate. |
| `reanalysis-with-history.json` | 30 | History join. |
| `history-status.json`, `*.history.ndjson`, `*.history.log` | 30 | History evidence. |
| `<run_id>.dispatch-analysis.json` / `.csv` / `.md` | 30 | Percentiles, phases, sink, integrity. |
| `<run_id>.capture-window.json` | 30 | Capture bounds and sink counters. |
| `<run_id>.sink.ndjson`, `.sink.ndjson.log` | 30 | Per-message sink receipts. |
| `<run_id>.generator-diagnostics.jsonl` (+ `.status.json`) | 30 | Generator cgroup samples, k6 metrics, Kafka lag. |
| `<run_id>.capture-window.json`, `sampler-ready.json`, exit-code files, logs | 30 | Wrapper evidence. |
| `<run_id>/health.json` | 30 | k6 step windows, outcomes, peak memory. |
| `<run_id>/gen-cpu.ndjson` | 30 | Generator CPU/memory time series. |
| `<run_id>/cpu-snapshots.jsonl` | 30 | **SUT** cAdvisor/Prometheus snapshots (see §7.4). |
| `cpu-snapshot-pretraffic.tsv` | 9 | One pre-traffic snapshot, **not** a time series. |
| `tb-haproxy-before.prom` | 9 | Pre-run Prometheus text sample, **not** a time series. |
| `edgex-gate*.json` | 9 | EdgeX SUT gate probes. |
| `launcher-error.json` | 10 | Attempts that never launched. |
| `lock-recovery.json`, `recovery-result.json`, `payload-validation.json` | 1–4 | Recovery evidence. |
| `harness-defect.json`, `HARNESS-DEFECT.md` | 1 | Ordinal 21 only. |

Anything absent or unreadable is **`UNAVAILABLE`** — never `0`, never a pass.

---

## 4. Schemas

### 4.1 `run-result.json` — real top-level keys

`schema_version, platform, run_id, run_kind, plan, plan_sha256, workload_policy,
staged_source_manifest, history_retrieved, reanalysis_with_history,
analysis_retrieved, fetch_errors, cleanup_verified, workload_lock_retained,
workload_launch_attempted, sut_resource_qualified, post_run_defect,
post_run_check_qualified, measurement_valid, capture_valid, latency_assessed,
platform_result, generator_resource_qualified, history_result, post_run_check,
finish`

**There is no top-level `ordinal`, `workload_id`, `repetition`, or `traffic_sent`.**
Take those from `run-matrix.json` (and, where present, `harness-defect.json`).
`traffic_sent` exists only inside `harness-defect.json`.

### 4.2 `<run_id>.dispatch-analysis.json` — keys used here

- `clock_evidence` (`status`, `errors`, `sample_count`, `host_identity`)
- `host_evidence` (`generator_host_id`, `sink_host_id`, `same_load_host_asserted`,
  `hostname`, `kernel`, `sink_process`)
- `capture_window` — same object as `<run_id>.capture-window.json`
- `workload_integrity` (`valid`, `findings`, `phases`, `policy`)
- `phases[]` — see below
- `sink` (`rows`, `unique_identities`, `duplicate_extra_receipts`,
  `late_rows_after_cutoff`, `missing_by_cutoff_total`, `platform_anomalies`)
- `history_status`, `history_assessed`, `history_result`
- `measurement_errors`, `measurement_valid`, `capture_valid`, `clock_errors`,
  `latency_assessed`, `platform_result`
- `resource_evidence` (`checks`, `qualified`, `saturation_flag`,
  `cgroup_throttling_observed`, `sampler_count`, `sampler_status`, `reasons`)
- `saturation_flag`, `generator_resource_qualified`

`phases[]` entry (verified):

```json
{
  "phase": "measurement",
  "scheduled": 15000,
  "received_by_cutoff": 15000,
  "missing_by_cutoff": 0,
  "conditional_dispatch_to_receipt_ms": {"p50": 5.007…, "p95": 7.611…, "p99": 12.347…},
  "conditional_scheduled_to_receipt_ms": {"p50": 8.643…, "p95": 11.397…, "p99": 16.247…},
  "outcomes": {"unknown": 0, "failure": 0, "unknown_but_delivered": 0,
               "acknowledged_but_history_missing": 0},
  "history_present": 15000,
  "history_missing": 0,
  "history_duplicate_extra": 0
}
```

### 4.3 `<run_id>.capture-window.json` — keys

`schema_version, run_id, capture_start_ms, capture_end_ms, observation_cutoff_ms,
complete, quiet_interval{required_seconds,start_ms,end_ms,passed},
late_observation{required_seconds,start_ms,end_ms,late_receipt_rows},
sink_metrics{received_total,duplicates_total,unparsed_total,overflow_total,
incomplete_body_total,body_read_error_total,oversized_body_total},
generator_exit_code, capture_recording_complete, sink_health_counters_zero,
quiet_interval_passed, health_attribution, platform_anomalies`

### 4.4 `health.json` (inside `<run_id>/`)

`valid, invalid_reasons, warnings, steps[], vus_peak, vus_max, records_total,
k6_mem_peak_bytes`. Each `steps[]` entry has `index, name, phase, rate,
duration_s, window_start_ms, planned_ticks, planned_sends, ticks_executed, sends,
achieved_ratio, outcomes{ack_success,explicit_failure,incomplete_response,
not_sent,oversized_response,unknown}, send_lag_ms{count,p50,p99,max},
ack_latency_ms{count,p50,p99,max}, ack_latency_ms_by_minute[]`.

### 4.5 Resource series

- **`<run_id>/gen-cpu.ndjson`** — one record per sample:
  `ms` (float epoch ms), `core_busy` (array, one fraction per CPU), `k6_mem_bytes`.
- **`<run_id>.generator-diagnostics.jsonl`** — one record per sample:
  `load_realtime_ns`, `load_monotonic_before_ns/after_ns`, `sampler_elapsed_ns`,
  `sampling_interval_target_ns`, `sampling_overrun`, `container_id`,
  `container_oom_killed`, `container_restarts`, `cpu_stat{path, cpu_stat{usage_usec,
  user_usec, system_usec, nr_periods, nr_throttled, throttled_usec, nr_bursts,
  burst_usec}, cpu_max, cpuset_cpus_effective}`, `ancestor_constraints`,
  `cpu_pressure`, `k6_metrics`, `kafka_offsets_and_lag`, `socket_states`,
  `thread_schedstat`, `collection_errors`. It carries **no memory field**.
- **`<run_id>/cpu-snapshots.jsonl`** — SUT snapshots:
  `request_utc, response_utc, snapshot_tsv, status, stderr`. A successful record
  has a non-empty 8-column TSV string:
  `epoch_s, container, container_id, cumulative_cpu_usec, container_start_ts,
  cpuset, service, type`. Column 4 is cumulative CPU microseconds.
- **`cpu-snapshot-pretraffic.tsv`** — 8 rows (one per SUT container), same 8
  columns, a single pre-traffic snapshot.
- **`*.prom`** — Prometheus text exposition at one point in time.

---

## 5. Identity and eligibility

- **Cell** = `(workload_id, platform)` from `run-matrix.json`.
- **Repetition** = `run-matrix.json` → `repetition`.
- **Row** = one `(run_id, phase)` pair from `dispatch-analysis.json`.
- **Eligible run** must satisfy every clause in §6.
- **Unusable attempt** has no usable analysis (`NOT_ASSESSED_NO_ANALYSIS`, a
  `launcher-error.json`, or absent evidence). It stays in the audit matrix and is
  excluded from every denominator and aggregate.

### 5.1 Ordinal 21 — exclude

`w4dp-tb-r02-20261108`. `harness-defect.json` has
`outcome: "HARNESS_DEFECT_EXCLUDE_FROM_REPETITIONS"`, `platform_finding: false`,
`traffic_sent: true`, and the reproduction shows `history_rows_present: 22500`
with `sink_rows: 0`. It is a harness defect, never a platform finding. Predicate:

```python
def is_harness_defect(defect):
    return defect is not None and defect.get("platform_finding") is False
```

Exclude from every table, chart, repetition set and pooled statistic. Keep it in a
separate *exclusion ledger* so the count is auditable.

### 5.2 Ordinals 22 and 24 — retain

- `w1dp-fw-r02-20261108` (ordinal 22): one malformed body in 45,000.
- `w1dp-tb-r02-20261108` (ordinal 24): warm-up-only first-contact egress stall;
  measurement phase and history clean.

Both have `platform_result: VALID_PLATFORM_FAILURE_OR_BACKLOG`,
`capture_valid: true`, `measurement_valid: true`. Never filter by `exit == 0`, and
never treat them as invalid. Use the `VALID_` spelling for this batch.

---

## 6. Validity gate — exact field mapping

| Gate | Field(s) and required value |
|---|---|
| Platform result | `platform_result ∈ {VALID_PLATFORM_SUCCESS, VALID_PLATFORM_FAILURE_OR_BACKLOG}` |
| Capture | `capture_valid is true` |
| Measurement | `measurement_valid is true` |
| Workload integrity | `workload_integrity.valid is true` |
| Generator resource | `generator_resource_qualified is true` (and `resource_evidence.qualified is true`) |
| SUT resource | `sut_resource_qualified is true` |
| History | `history_assessed is true` and `history_status == "OK"` and `history_result == "VALID_HISTORY_SUCCESS"` and `run-result.history_retrieved is true` |
| Post-run | `post_run_defect is not true`; for EdgeX additionally `post_run_check_qualified is true` |
| Cleanup | `cleanup_verified is true` (report `false` as a failed attempt, not a platform failure) |
| Harness defect | no `harness-defect.json` with `platform_finding: false` |

`aggregate-runs.py:load_run()` already computes most of this at lines 117–123. The
implementation must extend it with the harness-defect check and the matrix join,
not duplicate the logic.

---

## 7. Metrics

### 7.1 Latency (per run, never pooled)

Headline latency is the **measurement-phase** `conditional_dispatch_to_receipt_ms`
— dispatch to complete-body receipt, the platform service time. Report `p50`,
`p95`, `p99` exactly as precomputed in the analysis file; do **not** recompute from
`*.sink.ndjson` and do not assume an interpolation method.

Track `conditional_scheduled_to_receipt_ms` as a secondary metric (it includes
generator scheduling lag and must not replace the headline).

Per run: one row per phase; the *campaign value* is the `measurement` phase only.

Per cell: for each of p50/p95/p99, report **min / median / max across eligible
repetitions**. Never pool messages or runs into one percentile.

### 7.2 Throughput

```
duration_s   = health.steps[phase == "measurement"].duration_s
throughput   = phases[measurement].received_by_cutoff / duration_s   # msg/s
```

If `health.json` is missing or the measurement step cannot be identified, report
`UNAVAILABLE`. Do not substitute the whole capture window
(`capture_end_ms - capture_start_ms`), which includes warm-up and quiet tail; if
used as a fallback, label it `throughput_capture_window` separately.

Also record the planned rate from `steps[].rate` for context.

### 7.3 Drop and anomaly rates

```
drop_rate            = phases[measurement].missing_by_cutoff / phases[measurement].scheduled
ack_failure_rate     = outcomes.failure / scheduled
unknown_rate         = outcomes.unknown / scheduled
history_missing_rate = phases[measurement].history_missing / history_present
```

Report sink health counters separately from `capture_window.sink_metrics`:
`received_total`, `duplicates_total`, `unparsed_total`, `overflow_total`,
`incomplete_body_total`, `body_read_error_total`, `oversized_body_total`, plus
`sink.platform_anomalies` (`malformed_bodies`, `truncated_bodies`,
`oversized_bodies`). A `capture_window.complete` of `false` makes the run's
message-level rates `UNAVAILABLE` for comparison purposes; the run may still be
gate-eligible if every other clause passes, but flag it.

Do not call a non-zero anomaly count a platform failure when the ordinal is
retained as `VALID_PLATFORM_FAILURE_OR_BACKLOG`; report it as a measured value.

### 7.4 Resource peaks

**Window:** measurement phase only. Bound it with
`health.steps[phase == "measurement"].window_start_ms` and
`window_start_ms + duration_s*1000`; fall back to
`[capture_start_ms, observation_cutoff_ms]` from `capture-window.json` and label
the source. Never include warm-up.

**Generator CPU** — from `gen-cpu.ndjson`: `peak_cores = max(sum(core_busy))` over
in-window samples; also report mean. Cross-check `generator-diagnostics.jsonl`
`cpu_stat.cpu_stat.usage_usec` deltas: `Δusage_usec / Δload_realtime_ns * 1e3` =
cores at each interval; take the max.

**Generator memory** — `gen-cpu.ndjson` `k6_mem_bytes`: max over window;
cross-check `health.json` `k6_mem_peak_bytes`.

**SUT CPU** — from successful `cpu-snapshots.jsonl` records (`snapshot_tsv`
non-empty). Parse the 8-column TSV, group by `container_id`, order by `epoch_s`,
and compute per interval:

```
cores = (cumulative_cpu_usec[i] - cumulative_cpu_usec[i-1])
        / ((epoch_s[i] - epoch_s[i-1]) * 1e6)
peak_cores[container] = max(cores) over intervals intersecting the measurement window
```

Require at least two in-window samples per container; otherwise `UNAVAILABLE`.

**SUT memory** — **not present** in the snapshot TSV. Report `UNAVAILABLE` unless
another verified series is added. Never report `0`.

**Availability facts (verified 2026-11-16):**

- `w4dp-tb-r02-20261108`: 1,479 successful SUT snapshots, 0 errors.
- `w3dp-ex-r01-20261108`: 2,763 records, **all** `status: "exit status 22"` with
  HTTP 503 — SUT peaks `UNAVAILABLE`.
- `w4dp-fw-r01-20261108`: 2,745 records, **all** errors/empty — `UNAVAILABLE`.

So SUT CPU peaks are currently available only for ThingsBoard runs. This is an
availability fact, not a platform result, and must be surfaced as such.

**Not resource series:** `effective-resources-before/after.json` are quota and
configuration snapshots (`expected`, `containers`, `image_ids`);
`cpu-snapshot-pretraffic.tsv` is one pre-traffic row per container; `*.prom` is a
single pre-run scrape. None yield a peak.

### 7.5 Cell aggregation

Aggregate a cell **only when it has 3 eligible repetitions** (`--target-reps`,
default 3). A cell with fewer is `SHORT`: report its eligible runs individually
and emit no pooled statistic, no zero, no interpolation. A repetition not on disk
is *pending*.

For each metric report `min`, `median`, `max` across the eligible runs. Percentile
aggregation happens on per-run percentile values, never on messages.

---

## 8. Outputs

Write to a **new** directory that cannot already exist, e.g.
`results/postprocess-w1w4-<YYYYMMDDTHHMMSSZ>-<gitshortsha>/`. Refuse to proceed if
the path exists. Never `--force`. `results/**` is gitignored; never stage.

### `runs.csv` — one row per `(run_id, phase)`

`run_id, ordinal, workload_id, platform, repetition, run_kind, phase, eligible,
exclusion_reason, campaign_qualified, platform_result, capture_valid,
measurement_valid, latency_p50_ms, latency_p95_ms, latency_p99_ms,
scheduled_p99_ms, scheduled, received_by_cutoff, missing_by_cutoff, drop_rate,
throughput_msg_s, throughput_source, outcome_unknown, outcome_failure,
unknown_but_delivered, acked_but_history_missing, history_present,
history_missing, malformed_bodies, truncated_bodies, oversized_bodies,
sink_received_total, sink_duplicates_total, sink_unparsed_total,
sink_overflow_total, gen_cpu_peak_cores, gen_mem_peak_bytes,
sut_cpu_peak_cores, sut_mem_peak_bytes, resource_status, evidence_status`

### `cells.csv` — one row per cell

`workload_id, platform, eligible_reps, target_reps, cell_status,
latency_p50_min/median/max, latency_p95_min/median/max,
latency_p99_min/median/max, throughput_min/median/max, drop_rate_min/median/max,
gen_cpu_peak_min/median/max, sut_cpu_peak_status, run_ids`

### `summary.md`

Per-cell tables (eligible runs, per-run percentiles, min/median/max), an
**exclusion ledger** (ordinal 21 and every unusable attempt with its reason), and
an **`UNAVAILABLE` ledger** naming each missing series per run. State that
percentiles are per run and never pooled.

---

## 9. Implementation plan for `scripts/aggregate-runs.py`

**Do this only on an idle queue.** The file is 307 lines; the following is the
minimal change set.

Current behavior and gaps:

- `discover()` (line 80) finds `<run_id>.dispatch-analysis.json` and prefers the
  canonical nested copy; ambiguity exits.
- `load_run()` (line 108) computes `campaign_qualified` at lines 117–123 and reads
  `phases[]` at 149–170. **It never reads `harness-defect.json`.**
- `load_results_only()` (line 175) keeps attempts with no analysis.
- `summarize()` (line 222) emits `summary.md`; line 263 hard-codes "Target is five
  independent repetitions".
- `main()` (line 276) writes `runs.csv` + `summary.md`; `--out` defaults to the
  first input root (line 282) and `--force` can overwrite (line 285). Both defaults
  are unsafe here.

Proposed change:

1. `load_matrix(path) -> dict[str, dict]` — read `experimental_runs[]`, key by
   `run_id`, validate `plan_sha256`/`scope_sha256` only as a warning, never edit.
2. `load_harness_defect(evidence_dir) -> dict | None` — read
   `harness-defect.json`; return the parsed object.
3. Extend `load_run(analysis_path, matrix_entry, defect)`:
   - add `ordinal`, `workload_id`, `platform`, `repetition` from `matrix_entry`
     (fall back to the existing prefix inference only if the matrix lacks the run);
   - set `exclusion_reason = "HARNESS_DEFECT"` when
     `defect.get("platform_finding") is False`, and `eligible = false`;
   - add `eligible` and `exclusion_reason` to `FIELDS` and the row;
   - keep excluded rows in `runs.csv` for audit but out of every aggregate.
4. `load_unusable_attempts(root, matrix)` — classify runs with
   `run-result.json` but no analysis: `launcher-error.json` →
   `NOT_ASSESSED_LAUNCH_ERROR`; `platform_result == NOT_ASSESSED_NO_ANALYSIS` →
   `NOT_ASSESSED_NO_ANALYSIS`; missing evidence → `UNAVAILABLE`. Never count these
   as repetitions.
5. `load_resources(run_dir, window) -> dict` — implement §7.4, returning
   `gen_cpu_peak_cores`, `gen_mem_peak_bytes`, `sut_cpu_peak_cores`,
   `sut_mem_peak_bytes`, `resource_status` (`OK` / `PARTIAL` / `UNAVAILABLE`).
6. `aggregate_cells(eligible_rows, target_reps=3) -> list[dict]` — group by
   `(workload_id, platform)`, keep only `phase == "measurement"` and
   `eligible is true`, emit min/median/max per §7.5.
7. `main()`:
   - add `--matrix` (default the campaign `run-matrix.json`) and `--target-reps 3`;
   - make `--out` **required** (no evidence-root default) and refuse an existing
     directory; never write `runs.csv`/`summary.md` into the evidence root;
   - emit `runs.csv`, `cells.csv`, `summary.md`;
   - replace the line-263 sentence with target-aware text.
8. Do not recompute percentiles from messages. Do not touch
   `*.sink.ndjson` except for audits explicitly requested.

---

## 10. Tests (run on an idle queue)

1. Ordinal 21 → `eligible=false`, `exclusion_reason=HARNESS_DEFECT`, absent from
   `cells.csv`; still present in `runs.csv`.
2. Ordinals 22 and 24 → eligible, retained, not excluded by `exit != 0`.
3. Ordinal 32 → classified unusable, excluded from denominators.
4. Missing `run-result.json` or missing series → `UNAVAILABLE`, never `0`.
5. Two runs with distinct p99 → per-run values preserved; cell min/median/max
   correct; no pooled percentile.
6. Cell with 2 reps → `SHORT`, no aggregate; with 3 → aggregate.
7. Resource peak excludes warm-up; CPU delta math; <2 samples → `UNAVAILABLE`.
8. Overwrite refusal when the output directory exists.
9. Matrix join by `run_id` overrides prefix inference; unknown `run_id` falls back
   and warns.
10. Census parity: cell counts from the post-processor equal
    `scripts/census-w-matrix.py` for eligible cells.

---

## 11. Acceptance and non-goals

**Acceptance**

- Output directory is new and unique; no bound path changed; `git status` shows
  only the untracked output directory (gitignored) and this plan.
- Receipt state is reported, not assumed. As of 2026-11-19 every campaign
  receipt fails `--check` on `aggregator`, because `92ca31e` edited
  `scripts/aggregate-runs.py` after the queue drained. That drift is expected and
  documented (`POSTPROCESS-FINDINGS.md` item 12); `owner-w-matrix3-20261118a` is
  the authorization of record. Do not mint a receipt to force a green check and
  do not edit a receipt to silence the drift.
- Eligible cell counts and exclusion counts match the census.

**Non-goals**

- No LaTeX, no PDF.
- No pooling of message-level percentiles.
- No mutation or backfill of evidence, logs, or receipts.
- No rewiring of driver/watchdog receipt IDs (`h` → `i`) from this work.
- No change to claim behavior or to `internal/analysis` as part of post-processing.
