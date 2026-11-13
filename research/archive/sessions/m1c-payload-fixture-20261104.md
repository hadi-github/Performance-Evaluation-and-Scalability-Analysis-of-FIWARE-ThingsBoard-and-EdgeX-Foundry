# M1c offline payload-producer golden fixture — 2026-11-04

**Ready for independent DeepSeek Pro review. Synthetic offline engineering fixture only; no live
evidence, runtime capture, payload-storage proof, clock qualification, deployment or campaign
qualification is claimed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies
the model as DeepSeek V4.1 Flash (`opencode-go/deepseek-v4.1-flash`); it does not expose a
separate effort control. Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on
`chore/ci-and-release`. Pre-edit baseline:
`results/m1c-baseline-20261104-015538+0330/`. Review packet:
`results/m1c-payload-fixture-20261104-020102+0330/`. No commit or push was made.

This packet supplements the preserved M0b/M1a/M1b packets and does not replace them. It does not
modify retained evidence, `docs/status.md`, the protocol definition, the D44 profile policy, the
existing canonical encoders, or the analysis metric schema.

## What changed

All changes are new offline fixture/test files. No production adapter, sink, payload, capture,
clock or metric-schema code was added.

| File | Purpose |
|---|---|
| `tests/offline/payload-cohort-golden.json` | Shared synthetic golden vectors: 5 cohort messages, 8 edge-case messages, 3 declared size points each (39 vectors), exact baseline bytes and SHA-256 expectations. |
| `tests/offline/payload-cohort-golden.mjs` | Independent Node execution of the real `workloads/k6/lib/payload.js` / `hash.js` against the same golden vectors, including stored-export validation and corruption rejection. |
| `internal/protocol/payload_cohort_test.go` | Go tests using the real `CanonicalBytes`, `BuildPayload`, `BuildPayloadOffset`, `ValidateStored`; exact golden parity, size bounds/parity, timestamp identity, padding reconstruction, and actual stored-export corruption rejection. |
| `tests/offline/payload_cohort_test.go` | Go/Node byte-and-hash parity, M1b `PrepareOfflineFixture` -> `Finish` -> `Analyze` cohort mapping, separately named payload verification result, and proof that corrupt payload verification does not remove latency observations. |
| `docs/archive/sessions/m1c-payload-fixture-20261104.md` | This packet document. |

The existing canonical encoders remain the only encoding implementations. The golden JSON contains
test vectors, not an alternative encoder; Go and Node both rebuild every vector through their real
encoder before comparing bytes and hashes.

## Golden cohort and size points

The fixture is deterministic: run ID `SYNTHETIC-m1c-offline-payload`, seed `17`, fixed telemetry
`temperature=20`, `humidity=40`, `pressure=1000`, and fixed scheduled epoch times.

The five `cohort` messages map exactly to the existing M1b synthetic resolved measurement step
(`s01-measurement-r1`, devices=5, warmup 2 s then measurement 5 s at 1/s): seq `2..6` map to
`device-2`, `device-3`, `device-4`, `device-0`, `device-1`, scheduled at
`1700000002000..1700000006000` ms. The integration test reconstructs that mapping through the real
`plan.Resolve`/`audit.PlannedMessages` path rather than assuming it.

The eight `edge_cases` cover seq widths and identity/timestamp boundaries: seq `0`, `9`, `10`,
`9999`, `10000`, `uint32` maximum `4294967295`; shortest valid device `d`; maximum 128-character
device; allowed punctuation (`_ . : -`); and timestamps `0`, `0.001`, `1`, `1048576.001`,
`1700000000000.001`, and `9007199254740991`.

The declared logical size points are the contract's proposed engineering offsets `0`, `1024`,
`4096` bytes (`size_points_status=proposed_not_qualified`). They are **not** a silently frozen
scientific grid. Target size is always `unpadded baseline length + offset`; the native contract
requires the target to be at least the baseline and to have the same parity. The tests verify:

- too-small targets (baseline minus 1/2 bytes) fail;
- odd increments (baseline plus 1, offset 1) fail;
- offsets beyond the 1 MiB maximum fail;
- valid 1024/4096 offsets produce exactly the declared length and SHA-256;
- timestamps with more than three effective decimal places, unsafe range, or underflow are rejected,
  and no encoder path rounds them;
- padding is reconstructed from the existing versioned `Mix(seed,seq,salt,block)` and matches the
  emitted lowercase hex exactly;
- timestamp identity is preserved in the canonical token (including the maximum safe integer).

For every vector, the golden file records the exact unpadded canonical bytes, baseline SHA-256,
target length, full-body SHA-256, padding length and padding SHA-256. The first cohort baseline can
be hand-checked directly in the JSON; `sha256sum` over that string reproduces
`d2a4678fe9b13f48036c66a4d03d3f3fcd6dd40397b1453cee9afb893baf9724`.

## Stored-export validation

`internal/protocol/payload_cohort_test.go` writes the cohort payloads to a synthetic NDJSON export
file, reads those actual bytes back, and validates every `seq` against its expected logical payload
with `ValidateStored`. The Node script performs the same validation through
`validateStoredPayload`. The following corruptions fail closed in both paths:

- altered numeric field;
- omitted required field (`padding`);
- truncated JSON;
- wrong JSON type (`seq` as string);
- explicit `null`;
- duplicate JSON key.

The M1b integration test additionally rejects duplicate stored identities and identities outside the
declared cohort. These are validation exercises over synthetic bytes, not proof of any real store.

## M1b integration and separation

`TestOfflinePayloadCohortMapsToM1bAndStaysSeparate` uses the real M1b offline path:

1. `runner.PrepareOfflineFixture` (strict plan decode, `plan.Resolve`, protocol definition/profile
   verification, binding eligibility, manifest construction) and `runner.Finish`;
2. `audit.PlannedMessages` to reconstruct the measurement identities from the resolved plan;
3. `protocol.BuildPayload` for the stored export and `protocol.ValidateStored` for verification;
4. `analysis.Analyze` before and after writing payload artifacts.

The payload result is written as `payload-verification.json` with
`metric_interpretation=separate-payload-verification-v1`, `synthetic=true`, protocol/profile/plan
provenance, per-identity PASS status, `observations_removed=0`, and an explicit statement that
payload verification never deletes or rewrites latency/receipt observations. It is not added to the
analysis metric schema. The test proves the protocol/latency result is byte-for-byte unchanged before
and after payload verification, and remains unchanged even when a stored export is corrupted and
payload verification reports FAIL. Corrupt payload findings therefore stay separately named rather
than silently removing identifiable latency observations.

## Validation

Commands and exits are recorded in the review packet:

| Command | Exit |
|---|---|
| `go test ./internal/protocol ./tests/offline -count=1` | 0 |
| `node tests/offline/payload-cohort-golden.mjs` | 0 |
| `go test ./cmd/... ./internal/... ./tests/... -short -count=1` | 0 |
| `git diff --check` | 0 |
| `go vet ./internal/protocol ./tests/offline` | 0 |

`-short` skips the Docker-backed integration execution. The known archive-recursion failure of
unrestricted `go test ./...` under retained `results/` source snapshots was not invoked and those
snapshots were not edited. The JS parity Go test executes Node when present (Node v26.11.1 here) and
skips with an explicit message only if Node is unavailable; the packet also records a direct Node
run.

## Preserved hashes and limits

- Protocol definition SHA-256:
  `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`
- D44 profile SHA-256:
  `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`
- Retained results tree before M1c (same path set re-hashed after):
  `340c9772d696089d0e13dd5d0478e0aaffb36ba7064b407ec8844f4aca713759`
  (1,939 files, recorded in the baseline manifest).

No protocol definition, D44 profile, hash, amendment, metric schema, `docs/status.md`, deployment,
Docker/k6 execution, SUT, live adapter, sink, runtime capture, clock model or payload-storage claim
was changed or exercised. The offline fixture's git metadata is real and the working tree is dirty;
provenance/comparability therefore remains `NOT_ASSESSED` by design. Passing payload byte parity in
a synthetic fixture is an encoder/fixture check, not runtime production or storage evidence.
