# Open-model load generator (k6 + Go)

The generator that drives telemetry into the platforms under test (FIWARE, ThingsBoard, EdgeX, and the reference
ingester when it exists). Correctness of the measurement comes first. Wherever k6 forced a compromise, it is listed
in "Compromises forced by k6" below.

```bash
# on the load-generator host (ramonavm1), after scripts/deploy-generator.sh
bin/benchctl run -plan workloads/plans/<plan>.json -cpuset 0-1     # -> results/runs/<run_id>/, exit 0 = VALID
bin/benchctl check     -run results/runs/<run_id>                   # recompute the verdict offline
bin/benchctl reconcile -run results/runs/<run_id> -store <store>    # identities vs what the endpoint stored
bin/benchctl resolve   -plan <plan>                                 # explicit steps, seq offsets
go test ./internal/... && go test ./tests/integration -v           # unit + fault-injection tests (Docker)
```

## Model

- **Open model only.** Every step is a k6 `constant-arrival-rate` scenario (one scenario per step, consecutive
  `startTime`s). One iteration sends at most one HTTP request. There are no sleeps and no retries: k6 never
  retries, and the generator never does either.
- **Message identity.** In step *s*, iteration *i* (`exec.scenario.iterationInTest`) is
  `seq = s.seq_offset + i` and `scheduled_send_ms = scenario start + i × 1000 / tick_rate`.
  - `iterationInTest` counts **started** iterations, so this mapping is exact **only when no iteration was
    dropped**. A step with `dropped_iterations > 0` is INVALID, and reconciliation refuses such runs
    (docs/k6-feasibility.md, A1/A2).
  - With enough VUs but a slowed generator, the counter stayed gap-free and the lag showed up as send lag
    (feasibility case C1).
- **Planned slots per step = rate × duration exactly.** k6 sometimes runs one extra iteration exactly at the step
  boundary (measured: r·d or r·d+1). That iteration is not sent; it is counted as `beyond_plan`. A step that
  executes fewer than r·d ticks is INVALID (`ticks_complete`).
- **Logical message**, identical on every platform: `run_id, seq, scheduled_send_ms, temperature, humidity,
  pressure` (plus the device credential).
  - Device = `seq mod D`, where D (`devices`) is independent of the rate; per-device interval D/λ is in the health
    report.
  - Field values come from `hash(seed, seq)`, so a payload can be regenerated offline.
- **Adapters** (`workloads/k6/lib/adapters.js`) only wrap the logical message:

  | Platform | Request | On-wire bytes/request (HTTP, measured) | Stored as |
  |---|---|---|---|
  | fixture / reference | `POST /ingest` JSON incl. `device_id` | 332 (203-byte body) | one record |
  | ThingsBoard | `POST /api/v1/<token>/telemetry` JSON | 308 (145-byte body) | 6 `ts_kv` rows (one per key), same ts |
  | FIWARE | `POST /iot/d?k=&i=` UltraLight `t\|h\|p\|q(seq)\|r(run_id)\|s(sched)` | 289 (77-byte body) | entity attributes, **latest value only** (no history yet) |
  | EdgeX | `POST device-rest /api/v3/resource/<dev>/reading` JSON, `Object` resource | (see calibration) 139-byte body | one reading with an object value |

  The bodies exceed the ~100 B target because identity fields are never truncated (`run_id` is ~35 characters).

## Outcomes and records

`http.setResponseCallback(expectedStatuses(success, 207))` plus an explicit `classify()` per adapter:

| Outcome | Rule |
|---|---|
| `ack_success` | the adapter's success status (200 for all four). A 207 batch answer counts only if every item's `statusCode` is 2xx |
| `explicit_failure` | any other HTTP response (incl. a 207 with a non-2xx item) |
| `unknown` | no HTTP response: timeout (explicit `request_timeout_ms`), reset, any transport error (status 0) |
| `not_sent` | dropped iteration. These runs are INVALID, so `not_sent` only appears as a count |

- **EdgeX:** on the ingestion path, device-rest answers **200** (accepted) or **404** (unknown device/resource), not
  207 (checked live). 207 batch answers come from the provisioning APIs, which the adapter's setup parses item by
  item. The send path parses a 207 too, should one ever appear.
- **What is recorded per message:** only non-success outcomes and early sends, as JSON lines in
  `records.ndjson`, with fields `run_id, seq, device_id, rate_step, phase, tick, scheduled_send_ms,
  actual_send_ms, outcome, http_status, error_code, response_ms, detail, early`. Every other seq of the plan is
  `ack_success` by construction.
- **Per-step counters** (exact, in `k6-summary.json`): attempted, each outcome, early sends, beyond-plan, thinned,
  dropped iterations, `http_reqs`, `data_sent`.
- **Per-step bucketed histograms** of send lag (`actual − scheduled`) and acknowledgement latency
  (`response − scheduled`, successes only). These use one k6 Counter per bucket, not a k6 Trend (Trends keep every
  sample in memory). The bucket edges are in the plan, and the send-lag limit must be one of them, so the p99 gate
  is exact.
- `http_req_duration` is kept only as a k6 diagnostic. It starts at actual send and is never used as SLO latency.

## Health gates (per step, in `health.json` and the manifest)

A run failing any gate is **INVALID**. It is kept, never deleted, and `benchctl check` recomputes the verdict from
the retained files.

| Check | Gate |
|---|---|
| `dropped_iterations` | must be 0 |
| `ticks_complete` | executed ticks == rate × duration |
| `achieved_rate` | sends / planned sends ≥ `min_achieved_ratio` (0.99) |
| `send_lag_p99` | p99 send lag ≤ `send_lag_p99_ms` (10 ms) |
| `no_early_sends` | no send more than `early_tolerance_ms` (2 ms) before its scheduled time |
| `outcome_accounting` | ack + failure + unknown == attempted |
| `record_completeness` | records.ndjson has exactly one record per non-success outcome and per early send; no seq has two |
| `generator_core_busy` | per-core busy ≤ `max_core_busy` if set (> 0); otherwise reported only |

Also reported: peak VUs vs `vus.max` (a warning when reached), generator CPU per core (mean and max per step,
sampled every second from `/proc/stat`), k6 memory (container cgroup), and on-wire bytes per request.

## Manifest (`manifest.json`)

- Run id, plan id and plan SHA-256; the resolved plan (seed, steps, D, timeout, VUs, histogram, health limits).
- k6 image **digest** and version.
- Git SHA and dirty flag (from the `GIT_REV` file on deployed hosts), plus `k6_scripts_sha256` of the k6 files
  actually executed.
- Generator host (CPU model, cores, kernel, memory, Docker), generator cpuset, connection model, sample payload.
- Start/end UTC, k6 exit code, health and verdict.
- Credential **names** passed via `-env`; values are never stored.

**Connection model:** HTTP/1.1, keep-alive on (`connection.keep_alive`); each k6 VU has its own connection and at
most one request in flight. A VU is not a device: devices are only the `seq mod D` credentials.

## Reconciliation (`benchctl reconcile`)

Joins the ledger (every planned seq is `ack_success` unless a record says otherwise) with the stored identities
`(run_id, seq)`. It reports:
- `ack_lost` (acknowledged but not stored);
- `persisted_despite_unknown` and `persisted_despite_failure`;
- `duplicates` (plus extra copies);
- `orphans` (stored seqs outside the plan);
- identities with another run id (e.g. EdgeX readiness probes, `<run_id>-probe`, seq −1).

The automated fault tests prove each category (`tests/integration`).

## Compromises forced by k6 (decided 2026-10-22)

1. **Planned-arrival identity.** k6 exposes no tick index: `iterationInTest` counts started iterations. Identity is
   therefore exact only at zero drops. Runs with drops are INVALID instead of being reconstructed. Under generator
   overload, k6 even loses some arrivals from `dropped_iterations` (feasibility A2).
2. **Millisecond time.** k6 JS has only `Date.now()` (ms, no `performance.now()`); `exec.scenario.startTime` is ms
   too. All fields are `*_ms`. Latency is computed from `scheduled_send_ms` (derived from seq), so the JS send time
   only feeds send lag. The authoritative send time is k6's own sample time (ns) where full JSON output is enabled
   (calibration runs only).
3. **Boundary iteration.** k6 runs r·d or r·d+1 iterations per step; the extra one is suppressed.
4. **Records through the k6 log.** k6 VU code cannot write files. Non-success records are `console.log` JSON lines
   sent to `--log-output file=` with `--log-format json`. Volume is proportional to failures; the cost under
   overload is part of calibration.
5. **Shared randomness.** `randomSeed()` gives every VU the same sequence, and which VU runs which iteration is not
   deterministic. All randomness is therefore `hash(seed, seq)` (Go and JS bit-identical, tested).

## Calibration and Poisson

See `docs/calibration.md`.
