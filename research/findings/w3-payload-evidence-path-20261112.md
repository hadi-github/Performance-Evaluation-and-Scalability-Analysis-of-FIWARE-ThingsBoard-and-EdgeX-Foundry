# W3 evidence path: how each gate is actually demonstrated

**Date:** 2026-11-12
**Workload:** W3 — 1,000 devices, 25 msg/s, 300 s warm-up + 600 s measurement,
`logical-json-ascii-v1` with `size_offset_bytes=1024`, 5,000 ms timeout, 150-VU pool.
**Scope:** qualifications. None counts toward the 60 proposed attempts.

This note exists because W3's gate is not throughput — it is **payload preservation** — and
"the run passed" is not evidence for it. Each gate below names the artefact that demonstrates
it, or states that the artefact does not exist for that platform.

---

## 1. The gate, split into what can and cannot be shown

The frozen W3 gate has four clauses. They are not equally measurable, and conflating them is
how a payload claim becomes a claim about a row count.

| # | Clause | Demonstrated by | Available on |
|---|---|---|---|
| 1a | exactly **1,024** extra ASCII logical padding bytes | generator accounting + offline encoder test | TB, FIWARE, EdgeX |
| 1b | live **sink/history preservation** | platform store export + `benchctl payload-validate` | TB, EdgeX |
| 2 | payload-preserving evidence path qualified | 1b actually running end to end | TB, EdgeX |
| 3 | logical vs serialized body vs **measured transport** bytes, separated | `k6-summary.json` for the first two; **nothing** for the third | TB, FIWARE, EdgeX (partial) |
| 4 | FIWARE native delta 1,022 while logical delta is 1,024 | offline encoder test | FIWARE |

Clause 3's third component is the important honesty case: there is **no on-wire capture** in
this harness. HTTP-header arithmetic produces an *estimate*, and it will be labelled as an
estimate in every result record. A per-byte transport claim is unavailable.

## 2. Gate 1a and 4 — the padding arithmetic (no platform needed)

`workloads/k6/lib/adapter_payload.js` derives padding from `(seed, seq)`; the Go side
(`internal/protocol/payload.go`, `BuildPayloadOffset`) implements the same contract. Parity is
already pinned by `tests/offline/adapter_payload_test.go` (Go↔Node) and by
`tests/offline/fixed-point-padding.test.mjs`, which asserts:

- `padding` is exactly 1,024 lowercase hex chars;
- `logicalBytes` grows by exactly 1,024;
- the **native body** grows by 1,024 for JSON adapters and by **1,022** for FIWARE's UltraLight
  (`d|` prefix plus the quoted-empty encoding), which is gate 4.

Reproduce:

```bash
node tests/offline/fixed-point-padding.test.mjs
```

Verified live in this session: baseline 167 B → target 1,191 B, exactly 1,024 extra ASCII
lowercase-hex bytes for a W3-shaped message.

## 3. Gate 3 — logical vs adapter body vs transport

`gen.js` records a representative request in `k6-summary.json → sample_request`:

```json
{
  "logical_bytes": 1208,
  "adapter_body_bytes": 1208,
  "http_overhead_bytes": 114,
  "payload_encoding": "logical-json-ascii-v1",
  "payload_size_offset_bytes": 1024,
  "native_mapping": "thingsboard-http-telemetry-json-keys",
  "body_bytes": 1208,
  "url_bytes": 64
}
```

`logical_bytes` and `adapter_body_bytes` are **measured serialized sizes**. `http_overhead_bytes`
is **computed from header lengths**, not captured on the wire; results say so explicitly.

## 4. Gate 1b and 2 — stored-payload validation

This is the clause that separates W3 from a throughput run, and it needs two independent inputs:

1. **The platform's own store.** `scripts/history-payload.sh <platform> <run_id>` projects the
   eight canonical fields out of the platform database. A missing field is emitted as `null` or
   the query fails — it never regenerates padding from the seed.
2. **An independent expectation.** `benchctl payload-validate` rebuilds the payload from the
   plan's seed and the ledger's `(seq, device_id, scheduled_send_ms)` through
   `protocol.BuildPayload`, then applies `protocol.ValidateStored` to the stored bytes.

The expectation is computed **from the seed**, never copied from the store, so a platform that
stored a truncated or substituted padding fails instead of confirming itself.

```bash
# per platform; run on the workstation, not the SUT
scripts/history-payload.sh thingsboard w3-qual-tb-20261110 > /tmp/tb-stored.ndjson
scripts/history-payload.sh edgex       w3-qual-ex-20261110 > /tmp/ex-stored.ndjson

benchctl payload-validate \
  -plan   workloads/campaigns/fixed-points-w1-w4/qualification/w3/w3-qual-tb-20261110.json \
  -run-id w3-qual-tb-20261110 \
  -ledger results/workload-batch-20261108/evidence/w3-qual-tb-20261110/w3-qual-tb-20261110/records.ndjson \
  -stored /tmp/tb-stored.ndjson \
  -out    results/workload-batch-20261108/evidence/w3-qual-tb-20261110/payload-validation.json
```

Exit status is the gate: `0` only when **every** stored row validates and the padding length is
exactly the declared offset on every row; `1` otherwise; `2` for usage/IO errors. An empty store
exits `1` — "nothing preserved" is never a vacuous pass.

Pinned by `cmd/benchctl/payload_validate_test.go`, including the case that matters most: a row
with correct `run_id`/`seq`/`device_id` and two hex chars removed from `padding` is **rejected**.

### Per-platform availability

| Platform | Path | Status |
|---|---|---|
| ThingsBoard | `ts_kv.str_v` via `key_dictionary`; key is `padding` | **works** — column is `varchar(10,000,000)`, so 1,024 chars fits; transport config `max_string_value_length: 0` (unlimited) |
| EdgeX | `core_data.reading.objectvalue->>'padding'` | **works** — one Object reading per message |
| FIWARE | QuantumLeap `t."padding"` column | **unavailable** — both Orion subscriptions select only `seq/run_id/scheduled_send_ms`, so no typed column is created. The export fails closed rather than inventing a value. See `fiware-w3-payload-evidence-proposal-20261112.md`. |

## 5. What FIWARE W3 measures instead

> **Measured 2026-11-12.** FIWARE W3 ran as this section describes and returned
> `VALID_PLATFORM_SUCCESS` on delivery with gates 1b/2 recorded UNAVAILABLE. The
> unavailability was then proven against the live database, not left as a code reading:
> `SELECT t."padding" FROM mtopeniot.etsensor -> ERROR: column t.padding does not exist`,
> with all 22,500 identity rows present in that same table.
> See `w3-rollup-20261112.md` and `.../w3-qual-fw-20261110/payload-gate-unavailable.json`.

Given the owner's instruction to run FIWARE and mark the rest unavailable:

- **Measured:** throughput, latency percentiles, ack completeness, history identity completeness,
  gate 1a logical padding arithmetic, gate 3a logical/body bytes, gate 4 native delta.
- **Unavailable, and named as such in the result:** gate 1b stored-payload preservation, gate 2
  payload-preserving path qualification, gate 3b measured transport bytes.
- **Weak supporting signal:** Orion latest-state holds only the *last* value per entity, so at
  1,000 devices / 22,500 messages it can show that entities carry a `padding` attribute and its
  length, but it cannot prove per-message preservation. Recorded as weak evidence, clearly
  labelled, never as gate 1b.

FIWARE's verdict is therefore reported as a **partial** measurement: the measured axes stand on
their own, and the unavailable ones are listed rather than assumed to pass.

## 6. Order of operations

1. `switch` to stop the other owned platforms (`isolated_one_platform_at_a_time`).
2. Launch one platform; capture is one continuous window.
3. Read `dispatch-analysis.json` for verdicts; **diff every recorded percentile against it**
   before writing the result — this has been transcribed wrong twice.
4. Export the store and run `benchctl payload-validate`.
5. Save both artefacts under the run's evidence directory and reference them from the result.
6. Refresh the W3 ledger and re-render charts.

## 7. Related

- `research/findings/fiware-w3-payload-evidence-proposal-20261112.md` — the (unapplied) FIWARE fix
- `internal/protocol/payload.go` — `ValidateStored`, `BuildPayload`, `BuildPayloadOffset`
- `workloads/k6/lib/adapter_payload.js` — the generator-side encoder
- `scripts/history-payload.sh` — per-platform stored-payload export
- `cmd/benchctl/payload.go` — the independent check
