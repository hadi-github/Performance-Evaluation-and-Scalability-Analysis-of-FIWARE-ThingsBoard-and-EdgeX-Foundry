# M0b bounded offline protocol foundation — 2026-11-03

**Disposition:** implementation complete for independent Astra review; preliminary engineering pilot only. This is not campaign freeze or a qualification result. No deployment, sink change, live test, analyzer migration, plan integration, adapter wiring, commit, or push occurred. `docs/status.md`, the M0a packet/evidence, and the dated profile policy were not edited.

## Provenance and baseline

- Repository branch `chore/ci-and-release`, HEAD `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`.
- M0b baseline captured locally at `2026-11-03T02:14:29+03:30` on `artix`, before edits. Separate evidence directory: `results/m0b-20261103-021429+0330/`.
- The prior tracked diff was saved (`tracked-before.patch`, SHA-256 `89f030936b67173938f1292c0bb9975111ddf7bed591ba9facbeee6dd74d7638`); 17 pre-existing untracked files were copied and individually hashed in `snapshot-manifest.json`. The dated profile and prior M0a packet appear only as baseline copies, not edits.
- Requested model route: `openai/gpt-6-luna#high`. Actual harness identity was OpenAI GPT-6 Luna; separate `#high` routing confirmation was not exposed.

## Contract decisions and implementation boundary

New definition: `research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json`; its exact source-byte SHA-256 is in the adjacent `.json.sha256` sidecar. It explicitly says `preliminary_pilot` and `pending_calibration`. `internal/protocol` provides strict unknown-field/trailing-JSON rejection, deterministic SHA-256 over actual source bytes, pinned-hash and profile-hash verification, explicit `protocol_id`/hash/`execution_class` binding, and fail-closed eligibility. A campaign binding cannot pass this preliminary version. An engineering-pilot binding requires caller-supplied numeric rate, duration, warm-up, and drain cutoff; no values are silently defaulted. Evidence references are eligibility metadata only, not attestations.

The dated `research/protocols/offline-profiles-20261101.json` is byte-unchanged and referenced by path, policy ID, and SHA-256 `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`. D44 semantics remain as-is: 500 ms primary and 250/1000/5000 ms sensitivities; saved-data reliability and high-device-count comparison are not rewritten. The preliminary design lists D={100,1000,10000}, baseline 1000, and independent K={1,2,3}/N={1,2,3}. Named N-based service maps are explicitly `proposal_not_qualified`; they do not claim coordinated scaling support.

The executable metric helpers specify nearest-rank percentiles over unique first observed scheduled-cohort receipts (including drain arrivals through an explicit cutoff); missing has no finite latency, zero observed gives null p99, and scheduled/observed/missing plus scheduled-denominator threshold attainment stay separate. Ack-only and scheduled-to-response latency are separate. Capture invalidity does not become platform missing, and identifiable payload corruption remains observed. Normalized OK/ERROR/UNKNOWN/NOT_SENT states are distinct from coverage; NOT_SENT requires positive identified non-submission evidence. D15 `iterationInTest` and zero-drop validity remain. Exact per-slot NOT_SENT attribution under dropped invalid runs is explicitly an unsatisfied capability.

The canonical recommendation is `logical-json-ascii-v1`: compact ordered UTF-8 JSON with ASCII identifiers, exact supplied timestamps (at most three decimal places; encoder rejects rather than rounds), current numeric ranges from the Go/k6 Mix generator, and deterministic lowercase hex padding with versioned salt. Proposed sizes are unpadded baseline plus offsets 1024/4096; still unqualified. Go and pure k6-compatible JavaScript golden vectors agree. The validator decodes and compares actual supplied stored bytes/fields; it does not manufacture storage proof by reconstructing a local expected payload. The JS reference is not connected to any adapter.

Append-only scope record: `research/protocols/iotbench-telemetry-v0.1.0-pilot.1.amendments.md`. D34/D39 sourced 2× headroom, 300 s warm-up, 600 s confirmation, and at least five independent repetitions are retained. Clock budget, drain/recovery windows, sample counts, backlog epsilon, capacity-full/search predicates and scientific D/rate grid remain pending. A 1.5×/180 s coarse search with ≤5% bisection and five endpoint repetitions is marked proposal-only, not frozen; no optional favorable stopping and no hybrid search are adopted. Complete-body-read-before-parse is recorded as a future sink/receiver amendment, not implemented.

**Integration gate remains open:** existing `internal/plan`, `internal/analysis`, adapters, and sink do not yet consume this protocol. No claim of one-protocol execution is made. This milestone adds only the shared API/definition and a payload reference.

## Changed files (M0b additions only)

- `internal/protocol/contract.go`, `metrics.go`, `payload.go`, `protocol_test.go`
- `research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json`, `.json.sha256`, `.amendments.md`
- `workloads/k6/lib/payload.js` (pure helper; no adapter imports)
- `tests/offline/protocol-payload-golden.mjs`
- This packet: `docs/archive/sessions/m0b-protocol-foundation-20261103.md`

Per-file hashes are recorded in `results/m0b-20261103-021429+0330/SHA256SUMS`; `validation.txt` records exact commands/status. Existing dirty files are retained unchanged.

## Validation

Passed:

```text
gofmt -w internal/protocol/*.go && go test ./internal/... ./cmd/...       PASS
python3 tests/adapter_receiver_test.py                                    PASS (1 test)
python3 tests/resume_safe_provision_test.py                               PASS (1 test)
python3 -m unittest discover -s tests/offline -v                          PASS (4 tests)
node tests/offline/protocol-payload-golden.mjs                            PASS
git diff --check                                                          PASS
```

Not run: `go test ./tests/integration` (it launches Docker k6 workload/fault scenarios; intentionally excluded by the offline-only boundary). No test was skipped inside a command shown as passing above. No live tests or platform operations were attempted.

## Review requested / open gates

Please review strict parsing/hash binding, metric/outcome semantics, timestamp and cross-language byte compatibility, actual-stored-byte validation, D44 compatibility, independent N/K mapping, and campaign fail-closed behavior. In particular, confirm whether the schema is sufficiently explicit while preserving the stated pending decisions. Before any campaign integration, still require owner/calibration ratification of pending policies, evidence qualification, protocol/profile provenance in run records, shared plan/analyzer consumption, adapter payload wiring, receiver complete-body semantics, stored-history validation, and the scientific D/rate grid. Stop here pending Astra review; no broader M1 work is included.
