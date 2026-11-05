# M1e bounded adapter payload controls and response boundary semantics — 2026-11-04

**Ready for independent DeepSeek Pro review. Offline code and tests only; no live evidence,
Docker/k6 execution, SUT, platform, deployment, runtime capture, storage proof, clock
qualification or campaign qualification is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`); it does not expose a separate
effort control. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. Pre-edit baseline packet:
`results/m1e-baseline-20261104-031903+0330/`. Review packet:
`results/m1e-adapter-payload-20261104-035029+0330/`. Post-review F1/F2 fix packet:
`results/m1e-fix-20261104-044749+0330/`. No commit or push was made.

This packet supplements the preserved M0b/M1a/M1b/M1c/M1d packets and does not replace them. It
does not modify retained evidence, `docs/status.md`, the protocol definition, the D44 profile
policy, the sink complete-body contract, any metric threshold, or the existing canonical payload
encoders.

## What changed

All payload encoding still goes through the existing `workloads/k6/lib/payload.js`
(`canonicalPayloadBytes`, `buildPayload`, `validateStoredPayload`). No second encoder, padding
algorithm or size grid was introduced.

| File | Purpose |
|---|---|
| `internal/protocol/payload.go` | Names the existing encoding id as `PayloadEncodingID`; no byte, field-order, salt or validation semantics changed. |
| `internal/plan/plan.go` | New optional `payload` plan control. Absence preserves the legacy adapter wire bytes. Present requires the exact encoding id and an explicit integer `size_offset_bytes`; no implicit size point. |
| `internal/plan/plan_test.go` | Accepts explicit 0/1024/4096 offsets and rejects wrong encoding, missing, negative, fractional, too-large or unknown payload fields; absence remains legacy. |
| `workloads/k6/lib/adapter_payload.js` | Adapter-native projection of the already-encoded canonical object. Fixture/ThingsBoard/EdgeX use the canonical JSON body; FIWARE maps the same fields to UltraLight attributes. Reports logical bytes, adapter body bytes and HTTP-head overhead separately. |
| `workloads/k6/lib/response_semantics.js` | Shared response classification with distinct `incomplete_response` and `oversized_response` outcomes at the adapter boundary. No retry is added. |
| `workloads/k6/lib/adapters.js` | All four adapters use the projection. FIWARE provisions the `d`/`padding` Text alias only when payload is explicitly selected. |
| `workloads/k6/gen.js` | Counts boundary outcomes distinctly while keeping `explicit_failure` the full accounting bucket; summary carries logical/adapter/HTTPS byte accounting. |
| `internal/report/report.go`, `report_test.go` | Sample byte-accounting fields and boundary outcome fields; diagnostics are subsets of explicit failure, so legacy accounting is unchanged. |
| `internal/fixture/fixture.go`, `fixture_test.go` | Bounded fixture capture records the actual padding field when present; legacy records are unchanged and are not relabelled. |
| `scripts/history-payload.sh`, `tests/offline/test_history_payload.py` | New read-only exporter of actual stored logical fields, including padding, for all three platforms. Legacy `history-ids*.sh` output is unchanged. |
| `scripts/adapter-receiver.py`, `tests/adapter_receiver_test.py` | Local controlled receiver records the actual FIWARE padding alias in addition to the existing fields. |
| `tests/offline/adapter-payload.test.mjs`, `adapter_payload_test.go` | Node/Go offline proof of native projections, actual stored-field validation, opt-in legacy bytes, size failures, determinism, timestamp/device boundaries, a loopback HTTP fixture and response-boundary classification. |
| `docs/archive/sessions/m1e-adapter-payload-20261104.md` | This packet. |

## Explicit plan control (opt-in only)

A plan selects the logical payload with:

```json
"payload": {
  "encoding_id": "logical-json-ascii-v1",
  "size_offset_bytes": 1024
}
```

- If `payload` is absent, the adapters emit exactly their historical bodies. This is the default
  for every existing plan and is not silently upgraded.
- If `payload` is present, both fields are mandatory. The encoding id must be exact; the offset must
  be an explicit non-negative integer. Missing, negative, fractional, too-large or unknown fields
  fail plan decode/validation. There is no fallback to 0, 1024 or 4096.
- `0`, `1024` and `4096` are exercised as the contract's proposed engineering points
  (`size_points_status=proposed_not_qualified`), not as a frozen scientific grid. An explicit even
  offset is accepted only when the plan's worst-case canonical baseline plus that offset stays
  within the encoder's 1 MiB bound; odd offsets and below-baseline targets fail rather than being
  rounded or truncated.
- Target size is always `len(unpadded canonical body) + size_offset_bytes`. Plan resolution derives
  the maximum valid offset from the actual canonical baseline helper (run id, longest generated
  device id, maximum seq and longest permitted timestamp token), so an offset cannot resolve and
  then fail on the first encode. The canonical helper still enforces the 1 MiB body bound and
  hex-padding parity.

## Platform field mapping

| Adapter | Native request body | Stored native fields | Padding retrieval |
|---|---|---|---|
| fixture | canonical JSON object (all eight fields) to `/ingest` | fixture NDJSON record includes `run_id`, `seq`, `device_id`, `scheduled_send_ms` and, when supplied, the actual `padding` | actual `padding` record field |
| ThingsBoard | canonical JSON object to `/api/v1/<token>/telemetry` | one `ts_kv` row per telemetry key, including `run_id`, `seq`, `scheduled_send_ms`, `temperature`, `humidity`, `pressure`, `padding`; `device_id` is also carried by the body and device name is stored by the device registry | `scripts/history-payload.sh thingsboard` joins the actual `padding` key on the same `(entity_id, ts)` |
| FIWARE | UltraLight 2.0 text: `t|...|h|...|p|...|q|...|r|...|s|...|d|<padding>` | IoT Agent/Orion attributes; explicit provisioning adds `d` → `name=padding`, `type=Text` only in payload mode | `scripts/history-payload.sh fiware` selects the actual typed `padding` column; missing typed rows are reported, never counted as payload evidence |
| EdgeX | canonical JSON object to device-rest `/api/v3/resource/<device>/reading` | existing `Object` device resource `reading`; `core_data.reading.objectvalue` contains the whole object, including `padding` | `scripts/history-payload.sh edgex` selects `objectvalue->>'padding'` from the actual stored object |

The canonical fields are preserved verbatim: `run_id`, `seq`, `device_id`, the exact
`scheduled_send_ms` token, `temperature`, `humidity`, `pressure` and `padding`. The FIWARE device
identity is carried by the URL/entity name rather than a body attribute; the projection compares it
to the canonical `device_id`. No field is rounded, reordered or regenerated by the mapping. Explicit
payload mode also requires the generated device name to satisfy the canonical identifier bound
(1–128 ASCII identifier characters); an over-long name fails at setup instead of being truncated.

## Payload size and wire accounting

For every explicit payload vector the adapters and `gen.js` report three separate quantities:

1. **logical canonical bytes** — the exact `logical-json-ascii-v1` body from the existing helper;
2. **adapter serialized body bytes** — the platform-native request body;
3. **HTTP request-head overhead** — request line plus selected headers, excluding the body.

For the JSON-native adapters the body equals the canonical logical bytes. FIWARE text is its own
mapping and can be smaller or larger than the canonical JSON body; the signed native expansion is
reported, not hidden. `adapter_body_bytes` is not presented as a scientific payload-size result.
The `sample_request` in `k6-summary.json` carries `logical_bytes`, `adapter_body_bytes`,
`http_overhead_bytes`, `payload_encoding`, `payload_size_offset_bytes` and `native_mapping` only for
explicit payload plans; legacy samples remain unchanged.

## Adapter/export boundary semantics

The sink remains the complete-body observation point and its M1d schema/metrics are untouched.
Adapters do not claim receipts and add no receipt fields.

`response_semantics.js` classifies two boundary failures distinctly:

- `oversized_response`: the declared response `Content-Length` exceeds 4 MiB;
- `incomplete_response`: a body/read error, or a captured body shorter than its declared length.

Both increment their own per-step counters and are also included in `explicit_failure` so the
legacy invariant `attempted = ack_success + explicit_failure + unknown` remains true. They are never
counted as `ack_success`. A 207 batch is checked at the boundary before item parsing: an oversized
or truncated batch is a boundary failure, not a successful batch. No retry behavior was added or
changed. These are adapter diagnostics, not a replacement for sink counter evidence.

The non-success ledger record uses the legacy outcome `explicit_failure`; the boundary subtype stays
in `detail` with a stable `incomplete_response:` or `oversized_response:` prefix followed by the
existing `content_length...`, `body...` or `transport...` diagnostic. Readers therefore never need a
new outcome category, and old ledgers remain unchanged. Reconciliation counts these records as
explicit failures and reports a boundary failure that is absent from the store as
`boundary_failure_not_stored` (clean=false); a stored boundary failure remains in
`persisted_despite_failure`.

## Post-review fixes (F1/F2)

- **F1:** boundary classifications no longer enter the ledger outcome field. `gen.js` uses
  `ledgerOutcome()` to write legacy `explicit_failure` and keeps the subtype plus original
  diagnostic in `detail`; summary subtype counters are unchanged. `reconcile.Run` counts the record
  as an explicit failure, rejects unknown outcome categories instead of silently dropping them, and
  reports an unstored boundary failure as `boundary_failure_not_stored` so `clean` cannot appear true.
- **F2:** `plan.Resolve` now derives the maximum offset from the canonical baseline helper using the
  plan's run id, longest generated device id and maximum seq plus the longest permitted timestamp
  token. An offset at that maximum resolves and encodes; the next even offset fails at resolution.
  The generic `size_offset_bytes` ceiling and parity checks remain, and the encoder constants are not
  duplicated in the plan package.

## Tests

`tests/offline/adapter-payload.test.mjs` (executed by `adapter_payload_test.go` when Node is present)
covers:

- all four adapters at explicit offsets 0, 1024 and 4096;
- exact canonical logical bytes, native body bytes and HTTP overhead accounting;
- actual stored export projections for each platform, including the actual padding field;
- omitted, truncated and changed padding failing `validateStoredPayload` even when `run_id`, `seq`
  and `device_id` still match;
- below-baseline, odd, fractional, negative and beyond-maximum targets failing closed;
- deterministic same seed/seq bytes, different padding per sequence, exact timestamp tokens, and
  minimum/maximum/punctuated/invalid device identifiers;
- 50-message aggregate logical bytes and per-message fixed-offset padding lengths;
- a loopback HTTP fixture server receiving and validating the actual wire bodies;
- normal success, explicit failure, transport-unknown, oversized response, short body, body-read
  error and 207 batch classification;
- the ledger mapping keeps the legacy outcome vocabulary and carries boundary subtypes in `detail`.

`internal/fixture/fixture_test.go` proves the local fixture captures a 4096-byte actual padding
value and leaves a legacy record without a padding field. `tests/offline/test_history_payload.py`
proves all three platform exporters emit the actual stored padding (including explicit null when
missing) and do not turn query failures or absent FIWARE tables into successful empty captures.
`internal/plan/plan_test.go` covers explicit plan validation and the encoder-accurate maximum offset.
`internal/report/report_test.go`, `internal/reconcile/reconcile_test.go` and
`internal/analysis/analysis_test.go` cover the ledger boundary mapping: writer-shaped boundary
records pass `validateLedger`, reconcile keeps
`planned = ack_success + explicit_failure + unknown`, an unstored boundary failure is not clean,
stored boundary failures are reported as persisted, raw boundary category strings are rejected, and
end-to-end analysis does not become INVALID solely because of a boundary record.

## Validation

Commands and exits are recorded in the review packet:

| Command | Exit |
|---|---|
| `go test ./internal/plan ./internal/protocol ./internal/report ./internal/reconcile ./internal/analysis ./internal/fixture ./tests/offline -count=1` | 0 |
| `node tests/offline/adapter-payload.test.mjs` | 0 |
| `python3 -m unittest discover -s tests/offline -v` | 0 |
| `python3 tests/adapter_receiver_test.py` | 0 |
| `go test ./cmd/... ./internal/... ./tests/... -short -count=1` | 0 |
| `go test -race ./internal/plan ./internal/protocol ./internal/report ./internal/reconcile ./internal/analysis ./internal/fixture ./tests/offline -count=1` | 0 |
| `go vet` on changed packages | 0 |
| `git diff --check` | 0 |
| `gofmt -l` on changed Go files | 0 |

`-short` skips the Docker-backed integration execution. The known archive-recursion failure of
unrestricted `go test ./...` under retained `results/` source snapshots was not invoked and those
snapshots were not edited. No Docker, k6, live adapter, sink process, SUT, deployment or runtime
capture was started.

## Preserved hashes and limits

- Protocol definition SHA-256:
  `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` (unchanged).
- D44 profile SHA-256:
  `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` (unchanged).
- Retained `results/` tree before M1e (excluding `results/m1e-*`):
  `50e49df6c79cfc714e1a3495fac7ff2ecb94a0548ab22b05ef9020c75856a3a5`
  (2,001 files, recorded in the baseline manifest).
- Retained `results/` tree after M1e (same exclusion set): recorded in the review packet and
  expected identical.

No protocol definition, D44 profile, metric threshold, `docs/status.md`, deployment, Docker/k6
execution, SUT, live adapter, sink process, clock model or storage claim was changed or exercised.
The working tree contains the pre-existing M1a–M1d uncommitted work; it was preserved. Real git
metadata is dirty, so provenance/comparability remains `NOT_ASSESSED` by design. Passing offline
tests are a contract/implementation check, not runtime producer or storage qualification.
