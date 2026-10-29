# EdgeX per-run disk and WAL report (D29a)

**Recorded 2026-10-26 (Prompt 5).** Authorization: D29a Text + Impl/checks. Reporting only: no verdict, durability,
batching or database setting changed.

Command: `benchctl resources -run DIR [-prom-query URL -node HOST:9100] [-pg-counters FILE] [-query-step-s 5] [-out FILE]`
Code: `internal/diskmetrics` (formulas copied from `scripts/diag/analyze.py`); command in `cmd/benchctl/main.go`.
Without `-out` the JSON goes to stdout, so no evidence directory is written by accident.

## 1. What is reported, per measured step

Two groups, never merged:

| Group | Field | Source | Meaning |
|---|---|---|---|
| host disk (node-exporter) | `disk_write_await_ms` per device | `rate(node_disk_write_time_seconds_total)/rate(node_disk_writes_completed_total)` | mean time a write spends on the device, **all writes** (not only PostgreSQL WAL) |
| host disk | `disk_writes_per_s` per device | `rate(node_disk_writes_completed_total)` | completed writes per second on the device |
| PostgreSQL WAL (diagnostic overlay) | `wal_sync_ms_per_sync` | Δ`wal_sync_time` / Δ`wal_sync` | mean fdatasync time per WAL sync, cluster-wide |
| PostgreSQL WAL | `wal_sync_time_share_of_window` | Δ`wal_sync_time` / window | summed sync time over window time |
| PostgreSQL WAL | `commits_per_sync` | Δ`xact_commit` / Δ`wal_sync` | commits per WAL sync (group-commit ratio) |
| PostgreSQL WAL | `commits_per_s`, `wal_syncs_per_s` | Δ counter / window | rates |

`commits_per_s` (already in the diagnostic result) is kept separately. `commits_per_sync` is a new field and is not a
relabelled rate.

Missing data is explicit. Each block has `status` (`ok` or `unavailable`) and a `reason`. An undefined ratio, such as
`wal_sync_ms_per_sync` with zero syncs, or `disk_write_await_ms` on a device with zero writes, is **absent**, not zero.

## 2. Sampling and windows

- **node-exporter**: queried with Prometheus `query_range`, step `-query-step-s` (default 5 s), over each measured
  step window. Each device's value is the mean of the finite samples. The PromQL uses `rate(...[30s])`, as the
  diagnostic does. Prometheus's own `rate()` extrapolation applies.
- **pg-counters**: the diagnostic collector samples every ~10 s (`sample_interval_s` 10). Counters are differenced at
  the samples **nearest** the window edges, not at the window edges. The retained runs show edge errors of 1.6 s to 4.6 s
  (`edge_error_ms`), and that error is part of each rate's uncertainty. The output reports it.
- Window = the step's scenario start and duration (`k6-summary.json`, `resolved.json`). Warm-up and sweep steps are
  reported separately.

## 3. Counter limitations

- **Overlay only.** `pg-counters` exists only in the diagnostic overlay, which needs `track_wal_io_timing`. Ordinary
  EdgeX runs do not collect it, so their WAL block is `unavailable` with that reason. The overlay is not enabled in
  the main compose by this change.
- **Cluster-wide.** WAL and commit counters are cluster totals, not per core-data replica or per transaction.
- **Resets.** A change in `stats_reset` inside the window, or any cumulative counter that decreases, makes the block
  `unavailable`. A reset is never silently differenced.
- **Concurrent flushers.** `wal_sync_time` sums time across backends. With several concurrent flushers the share can
  exceed 1. It is a summed-time ratio, **not** utilization of the disk or of a core.
- **Sample alignment.** Counters are sampled, not read atomically with the window. Edge errors are reported.

## 4. Verification performed (2026-10-26)

Offline only. No platform was contacted. Raw evidence hashes were checked before and after.

| Check | Result |
|---|---|
| `go test ./internal/diskmetrics` (12 tests: WAL known answer, undefined ratios, multi-line parse, non-finite averages, Prometheus known answer and error, reset and decreasing-counter guards, retained-run reproduction) | all pass |
| Retained EdgeX DIAG WAL recomputed from raw `pg-counters.ndjson` vs committed `results/diag/<run>.diag.json` | 5 runs identical (max relative difference 0.00 across window, commits/s, syncs/s, ms/sync and share): `k1-r200`, `k1-r200-nosync`, `k4-r500b`, `k4-r500`, `k4-r500-nosync` |
| Two retained runs with an empty `pg-counters` file (`k4-obs-nosink-r200`, `k4-obs-sink-r200`) | `unavailable`, consistent with the retained result having no `postgres` block |
| Retained disk fields (`disk_write_await_ms`, `disk_writes_per_s`) | **not reproduced.** The Prometheus responses were not retained, so only the formulas and the query shape can be checked. Recorded as a limitation, not a pass. |
| `DIAG-edgex-k4-r500` disk fields (empty in the committed result) | the new code reports `unavailable` with a reason, not zero |

Reproduce:

```bash
go test ./internal/diskmetrics
benchctl resources -run results/diag/DIAG-edgex-k4-r500b \
  -pg-counters results/diag/sut/DIAG-edgex-k4-r500b/pg-counters.ndjson -out /tmp/opencode/resources.json
```

## 5. Interpretation restrictions

- Host await is an **average device service time per write** over the window. It is **not** per-transaction commit
  latency and must not be presented as one.
- WAL `ms/sync` and host await are different quantities and are never substituted for each other.
- Neither disk nor WAL figures decide a verdict. The VALID/FAIL/INVALID rules are unchanged.
- Results across different SUT disks are not comparable without naming the device and its characteristics.

## 6. Not done here (requires authorization)

- No `pg_stat_statements` or `track_wal_io_timing` change in the main EdgeX compose (separate change with its own validation).
- No live Prometheus query against the SUT. The `-prom-query` path has been exercised only with a local fake server in tests.
- Ordinary runs do not yet start the pg-counters overlay. Adding it is a platform configuration change and is not authorized.
