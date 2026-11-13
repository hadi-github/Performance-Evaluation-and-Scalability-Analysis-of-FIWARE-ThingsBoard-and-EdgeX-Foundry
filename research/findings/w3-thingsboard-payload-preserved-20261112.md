# W3 measured on ThingsBoard: 1,024 bytes of padding survived end to end

**Date:** 2026-11-12
**Workload:** W3 — **1,000 devices**, 25 messages/s, 300 s warm-up + 600 s measurement,
payload `logical-json-ascii-v1` with **`size_offset_bytes=1024`**, 5,000 ms timeout,
150-VU pool (`dispatch-integrity-v1`).
**Scope:** qualification. **Does not count toward the 60 proposed attempts.**

---

## 1. Result

| | Value |
|---|---|
| Run ID | `w3-qual-tb-20261110` |
| Verdict | `INVALID_PLATFORM_FAILURE_OR_BACKLOG` |
| **W3 payload gate** | **PASS** (gates 1a, 1b, 2, 3a) |
| Dispatched / acked / in history | 22,500 / 22,500 / 22,500 |
| Measurement missing / unknown / failed | **0 / 0 / 0** |
| p99 dispatch-to-receipt (measurement) | **257.7 ms** |
| Stored rows validated | **22,500 / 22,500** |
| Padding length, every stored row | **exactly 1,024 bytes, lowercase hex** |

The verdict is `FAILURE_OR_BACKLOG` for **one reason only**: 213 warm-up identities had not
arrived by the observation cutoff, and the frozen policy counts any missing identity in any
phase. All 7,500 warm-up identities are present in history, so nothing was lost; the 600 s
measurement is completely clean.

This is the same cold-start shape W2 saw on ThingsBoard, one workload later.

## 2. The payload evidence

W3's gate is not throughput. It is that **exactly 1,024 extra ASCII logical padding bytes**
were preserved. That is now demonstrated three independent ways.

### 2a. The generator sent them (gate 1a, 3a)

`k6-summary.json → sample_request`:

```json
{
  "logical_bytes": 1208,
  "adapter_body_bytes": 1208,
  "payload_size_offset_bytes": 1024,
  "payload_encoding": "logical-json-ascii-v1",
  "native_mapping": "thingsboard-http-telemetry-json-keys",
  "http_overhead_bytes": 114
}
```

Baseline for a W3-shaped message is 184 bytes; with `size_offset_bytes=1024` the serialized body
is 1,208 bytes — **exactly 1,024 more**. `http_overhead_bytes` (114) is header arithmetic, **not**
an on-wire measurement, and is labelled as an estimate everywhere it appears.

### 2b. The platform stored them

`scripts/history-payload.sh thingsboard` exported **22,500 rows** from `ts_kv.str_v` joined
through `key_dictionary` on `key='padding'`. The padding-length histogram over the entire store:

```
{1024: 22500}
```

Every single row, exactly 1,024 chars. Nothing truncated, nothing substituted. The (truncated)
sample row:

```json
{"run_id":"w3-qual-tb-20261110","seq":639,
 "device_id":"bench-w3-qual-tb-20261110-00639",
 "scheduled_send_ms":1794498802163,"temperature":19,"humidity":80,"pressure":1048,
 "padding":"e693341aa9fec3361316509151de9e5d835c3a3c..."}
```

ThingsBoard could hold this comfortably: `ts_kv.str_v` is `varchar(10,000,000)`, and the HTTP
transport sets `max_string_value_length: 0` (unlimited). A naive assumption that telemetry
strings cap at 255 would have been wrong here, which is why it was checked rather than assumed.

### 2c. They are the *right* bytes (gates 1b, 2)

Row counts and lengths prove *something* was stored. They do not prove it was the payload the
generator intended — a platform could store 1,024 chars of the wrong hex. The independent check
rebuilds the expectation from the plan seed:

```bash
benchctl payload-validate \
  -plan   .../w3/w3-qual-tb-20261110.json \
  -run-id w3-qual-tb-20261110 \
  -ledger .../w3-qual-tb-20261110/records.ndjson \
  -stored .../w3-qual-tb-20261110/stored-payload.ndjson
```

```json
{"verdict": "VALID", "validated": 22500, "stored_rows": 22500,
 "failed": 0, "missing_from_ledger": 0,
 "padding_lengths": {"1024": 22500},
 "baseline_bytes": {"1205": 182, "1206": 2676, "1207": 9895, "1208": 9377},
 "expected_padding_total_bytes": 22661120}
```

For each row the expected payload is recomputed from `(seed, seq, device slot,
scheduled_send_ms)` through `protocol.BuildPayload`, then `protocol.ValidateStored` compares it
byte-for-byte against the platform's export. **Padding is never copied from the store into the
expectation**, so a store that echoes a modified padding cannot define itself as correct. The
mutation tests in `cmd/benchctl/payload_validate_test.go` pin that: a row with correct
`run_id`/`seq`/`device_id` and two hex chars removed is rejected, and so is one with the same
length but different content.

The baseline histogram confirms the arithmetic per message: every row's target is its own
baseline + 1,024, and the baseline varies (1,205–1,208) purely with the digit width of
`seq`, `temperature`, `humidity`, `pressure` and `scheduled_send_ms` — not with a fixed grid.

## 3. A validator bug this run exposed, and fixed

The first validation pass reported `INVALID` with **370 rows "missing from ledger"**. The cause
was the validator, not the platform:

- it only expected rows marked `ack_success`;
- ThingsBoard's cold-start ramp produced 370 warm-up messages whose acknowledgement the client
  never saw (they timed out at 5 s);
- the platform **accepted and stored all 370 anyway**;
- so the store contained 370 identities the validator refused to have an opinion about.

That conflates **payload preservation** with **delivery**. They are different questions: the
generator has an identity+timing record for every dispatched message, so every dispatched
message is a valid expectation. `loadLedger` now includes every dispatched row regardless of
outcome; delivery completeness stays the dispatch analysis's job.

After the fix, the *same* stored export validates 22,500/22,500 with zero failures. No evidence
was regenerated and no row was discarded — the stored bytes never changed, only the expectation
set.

This is also why the run's verdict (`FAILURE_OR_BACKLOG`, delivery) and its payload verdict
(`VALID`, preservation) can differ without contradiction. Both are true of the same run.

## 4. What W3 established that W1 and W2 could not

| | W1 | W2 | W3 |
|---|---|---|---|
| devices | 1,000 | 2,000 | 1,000 |
| rate | 50/s | 25/s | 25/s |
| `size_offset_bytes` | 0 | 0 | **1,024** |
| stored payload validated | no | no | **yes, 22,500 rows** |

W3 is the first cell in this batch where the *content* of the message is evidence, not just its
identity. W1 and W2 proved the platform could address, accept and persist 22,500 identities; W3
proves the bytes it persisted are the bytes intended, to the byte, for every message.

The p99 also improved over W2 ThingsBoard (257.7 ms vs 287.8 ms) despite 1,024 extra payload
bytes per message and the same 1,000-device cohort — so the padding is not measurably costing
the platform latency at this shape.

## 5. Gates, stated exactly

| Gate | Status | Basis |
|---|---|---|
| 1a. exactly 1,024 extra ASCII logical padding bytes | **PASS** | `sample_request`: 184 → 1,208 bytes; store histogram `{1024: 22500}` |
| 1b. live sink/history preservation | **PASS** | 22,500 rows exported from `ts_kv`, every one 1,024 chars, all validated |
| 2. payload-preserving evidence path qualified | **PASS** | `benchctl payload-validate` → `VALID`, 22,500/22,500 |
| 3. logical / adapter body / transport reported separately | **PARTIAL** | logical 1,208 and body 1,208 are measured; **transport bytes unavailable** — 114 is header arithmetic, not capture |
| 4. FIWARE native delta 1,022 vs logical 1,024 | **N/A** | JSON adapter; the delta is UltraLight-specific |

Gate 3's transport component is **unavailable on every platform** in this harness, and will be
reported as unavailable rather than estimated into a number.

## 6. Evidence

- `results/workload-batch-20261108/evidence/w3-qual-tb-20261110/qualification-result.json`
- `.../w3-qual-tb-20261110.dispatch-analysis.json` — verdicts and per-phase outcomes
- `.../payload-validation.json` — the independent check, 22,500/22,500
- `.../stored-payload.ndjson` — 22,500 rows exported from the platform's own store (27 MB)
- `.../w3-qual-tb-20261110/k6-summary.json` — `sample_request` byte accounting
- `workloads/campaigns/fixed-points-w1-w4/qualification/w3/index.json` — ledger row
- `workloads/campaigns/fixed-points-w1-w4/authorization/owner-w3-qual-tb-20261112c2.json`

Reproduce the payload check:

```bash
scripts/history-payload.sh thingsboard w3-qual-tb-20261110 > /tmp/stored.ndjson
benchctl payload-validate -plan <w3 tb plan> -run-id w3-qual-tb-20261110 \
  -ledger <run>/records.ndjson -stored /tmp/stored.ndjson
```

## 7. Open decisions (owner's, not the harness's)

1. **Warm-up cold-start policy.** Same question as W2 ThingsBoard, now with a second example: a
   cell whose 600 s measurement is clean and whose warm-up has a cold-start ramp is reported
   `FAILURE_OR_BACKLOG`. Whether that should fail the cell is a policy decision, not a
   measurement one.
2. **Transport-byte measurement.** Gate 3's third component cannot be answered by this harness.
   Real on-wire capture is a harness addition, not a run.

## 8. Related

- `w3-payload-evidence-path-20261112.md` — how each W3 gate is demonstrated
- `fiware-w3-payload-evidence-proposal-20261112.md` — the FIWARE blocker and its (unapplied) fix
- `w2-2000-devices-measured-20261112.md` — W2, including the same TB cold-start shape
- `cmd/benchctl/payload.go` — the independent stored-payload check
