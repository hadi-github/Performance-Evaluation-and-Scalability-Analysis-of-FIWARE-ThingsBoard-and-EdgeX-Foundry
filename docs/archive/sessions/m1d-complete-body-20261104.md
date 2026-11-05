# M1d bounded receiver complete-body receipt contract — 2026-11-04

**Ready for independent DeepSeek Pro review. Offline code and test work only; no live evidence,
Docker/k6 execution, SUT, adapter, deployment, runtime capture, clock qualification, campaign
qualification or storage proof is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`); it does not expose a separate
effort control. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. Pre-edit baseline packet:
`results/m1d-baseline-20261104-022833+0330/`. Review packet:
`results/m1d-complete-body-20261104-024117+0330/`. No commit or push was made.

This packet supplements the preserved M0b/M1a/M1b/M1c packets and does not replace them. It does
not modify retained evidence, `docs/status.md`, the protocol definition, the D44 profile policy,
the existing canonical payload encoders, or the metric/threshold definitions.

## What changed

| File | Purpose |
|---|---|
| `internal/receipt/receipt.go` | New shared, versioned receiver semantics constants: `complete-body-read-before-parse-v1`, endpoint label, sink timestamp field name, capture schema 2 / legacy schema 1. Explicitly outside the frozen protocol definition. |
| `internal/sink/sink.go` | Bounded 4 MiB complete-body reader; new `receipt_complete_body_read_ms` and `receipt_capture_semantics` record fields; incomplete-body diagnostics; unchanged legacy `recv_ms`, received/duplicate/overflow accounting. |
| `internal/sink/sink_test.go` | Controlled-reader proof of timestamp ordering; failed/truncated/oversized body rejection with no record and counters; exact-bound acceptance; legacy-record distinction; concurrent unique/duplicate requests. |
| `internal/analysis/protocol_binding.go` | Raw capture validation by schema version; schema 2 requires an explicit non-null semantics marker; schema 1 remains readable legacy only. |
| `internal/analysis/protocol_cohort.go` | New primary metrics require schema 2 plus the exact complete-body semantics and endpoint; schema 1 is NOT_ASSESSED and produces no new primary metrics. |
| `internal/analysis/types.go` | Capture semantics field and read-only protocol provenance for the capture schema/semantics. |
| `internal/analysis/protocol_synthetic.go` | The explicitly synthetic protocol fixture emits schema 2 with the new marker; it is not a relabel of retained evidence. |
| `internal/analysis/protocol_cohort_test.go` | Legacy schema, missing/wrong semantics and raw-schema rejection tests; provenance migration test. |
| `tests/offline/pipeline_test.go` | Offline M1b fixture emits schema 2 and exercises the same strictness through the real prepare/finish/analyze path. |
| `cmd/sink/main.go` | Log includes the incomplete-body diagnostic; exit status still depends only on overflow. |
| `docs/archive/sessions/m1d-complete-body-20261104.md` | This packet. |

## Receiver contract and legacy distinction

The sink now records two receiver-side semantics on every new record:

```json
{
  "recv_ms": 1700000000000.125,
  "receipt_complete_body_read_ms": 1700000000000.750,
  "receipt_capture_semantics": "complete-body-read-before-parse-v1",
  "platform": "...",
  "run_id": "...",
  "seq": 1,
  "dup": false
}
```

- `receipt_complete_body_read_ms` is taken **immediately after** `io.ReadAll` returns without error
  and the read length is confirmed within the 4 MiB bound, and **before** JSON parsing, identity
  extraction, deduplication or record emission. No timestamp is taken after parsing.
- Unit/domain: epoch milliseconds as `float64` with sub-millisecond resolution, in the same
  shared-load-realtime wall-clock domain as the existing `recv_ms`. It is a same-host/local
  observation. No monotonic-clock, cross-host offset, end-to-end, store or durable-commit
  qualification is claimed.
- `recv_ms` is unchanged: the request-arrival wall clock taken before the body read. Existing
  records and reruns that contain only `recv_ms` remain legacy; they are never silently
  reinterpreted as complete-body receipts.
- The new semantics marker is versioned (`...-v1`). New records are distinguished from legacy
  records by the presence of both fields; `Record.HasCompleteBodyReceipt()` returns false for
  legacy records.
- A single HTTP body may carry several identities (a batch). All records extracted from that body
  share the request-level complete-body receipt time. That is not a per-message parse time and not
  a per-message platform or store delivery time. Conditional latency availability is therefore
  explicit rather than inferred.

### Failed, truncated and oversized bodies

The reader is bounded at `MaxBodyBytes = 4 MiB` and reads at most `MaxBodyBytes+1` bytes. It emits a
record only when the entire body was read successfully and fit the bound. A read error, a short or
truncated body, or a body larger than the bound:

- emits **no** receipt record and does not fabricate an identity or complete-body timestamp;
- increments `sink_incomplete_body_total`, with `sink_body_read_error_total` and
  `sink_oversized_body_total` as the reason breakdown;
- also increments the existing `sink_unparsed_total` health count, because the request produced no
  `(run_id, seq)` identity;
- leaves `sink_received_total`, duplicate accounting and overflow accounting unchanged.

Overflow and process exit semantics are preserved: records still go through the bounded channel,
overflow is counted, and `cmd/sink` exits non-zero only when overflow occurred. Incomplete-body
counters are diagnostic, not a silent success and not an overflow substitute.

## Capture schema migration

The capture document remains outside the frozen protocol contract, so the protocol definition hash
is unchanged. The version lives in `internal/receipt` and in `protocol-receipts.json`:

| Schema | Status | Meaning |
|---|---|---|
| 1 | Legacy, read-only | Pre-M1d capture. Readable and retained, but it lacks the complete-body-before-parse marker and cannot produce the new primary delivery metrics. |
| 2 | Current | Requires `receipt_capture_semantics=complete-body-read-before-parse-v1`, the exact `response_complete_body_read` endpoint, `complete=true`, `body_read_complete=true`, and explicit source/clock metadata. |

The analyzer validates the raw document before typed decoding (required fields, unsupported schema
versions, null semantics, duplicate keys, case aliases). Missing fields or capture errors leave the
protocol-delivery claim `NOT_ASSESSED` with no primary percentile/count metrics. A schema-1 capture
is explicitly reported as legacy in provenance and is likewise `NOT_ASSESSED` for the new primary
metrics. An unsupported semantics marker is not upgraded by default.

The explicitly synthetic M1a/M1b fixtures were migrated to schema 2 because they are generated test
artifacts, not retained real captures. No retained evidence or rerun was relabeled.

## Tests

`internal/sink/sink_test.go` covers:

- a controlled reader and injected clock prove the complete-body timestamp is taken after the final
  body bytes and before the parse hook, with exactly two clock readings;
- read error, `io.ErrUnexpectedEOF` truncation and `MaxBodyBytes+1` oversize cases produce no NDJSON
  record, no received/overflow increment, exact incomplete/read-error/oversized counters and no
  legacy-health regression;
- a body exactly at the 4 MiB bound is accepted;
- a legacy JSON record without the new fields stays legacy, while a new record is recognized;
- concurrent unique requests and concurrent duplicate identities preserve consistent first/duplicate
  accounting and complete-body fields (also run under `-race`).

`internal/analysis/protocol_cohort_test.go` and `tests/offline/pipeline_test.go` cover schema-1
legacy refusal, missing/wrong semantics, raw missing/null/unsupported-schema rejection, and the new
version provenance, including through the real M1b offline prepare/finish/analyze path.

## Validation

Commands and exits are recorded in the review packet:

| Command | Exit |
|---|---|
| `go test ./internal/receipt ./internal/sink ./internal/analysis ./tests/offline -count=1` | 0 |
| `go test -race ./internal/sink ./internal/analysis ./tests/offline -count=1` | 0 |
| `go test ./cmd/... ./internal/... ./tests/... -short -count=1` | 0 |
| `git diff --check` | 0 |
| `go vet ./internal/receipt ./internal/sink ./internal/analysis ./tests/offline ./cmd/sink` | 0 |

`-short` skips the Docker-backed integration execution. The known archive-recursion failure of
unrestricted `go test ./...` under retained `results/` source snapshots was not invoked and those
snapshots were not edited. No live sink, SUT, Docker, k6, adapter or deployment was started.

## Preserved hashes and limits

- Protocol definition SHA-256:
  `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` (unchanged).
- D44 profile SHA-256:
  `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` (unchanged).
- Retained `results/` tree before M1d (excluding `results/m1d-*`):
  `01b7aa4478bdb133a3d2492fe355b39f7b5271fc3924ace76f24c101ed3fce3a`
  (1,966 files, recorded in the baseline manifest).
- Retained `results/` tree after M1d (same exclusion set):
  `01b7aa4478bdb133a3d2492fe355b39f7b5271fc3924ace76f24c101ed3fce3a`
  (1,966 files; identical).

No protocol definition, D44 profile, metric threshold, `docs/status.md`, deployment, Docker/k6
execution, SUT, live adapter, runtime sink process, clock model or storage claim was changed or
exercised. The working tree contains the pre-existing M1a/M1b/M1c uncommitted work; it was
preserved. Real git metadata is dirty, so provenance/comparability remains `NOT_ASSESSED` by design.
Passing offline tests are a contract/implementation check, not runtime receiver qualification.
