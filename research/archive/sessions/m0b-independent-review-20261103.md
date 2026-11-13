# M0b independent review — 2026-11-03

**CHANGES_REQUIRED.** Campaign freeze remains pending. This records the independent reviewer
response from session `ses_e6188ffb2ffejzM99BYHAtczBm`; it is not a new verification run.
Requested model: `openai/gpt-6-astra#high`; harness identifies GPT-6 Astra;
separate effort routing confirmation unavailable. Implementation session:
`ses_e61964396ffefqE0zMyJY4AEA4`, requested `openai/gpt-6-luna#high`.

## Blocking findings

| ID | Severity | Evidence in reviewed revision | Required fix / acceptance |
|---|---|---|---|
| M0B-R1 | HIGH | `internal/protocol/contract.go:153–214`: omitted normative payload/capture fields, extra delivery outcomes and duplicate schema keys accepted | Validate all required/normative semantics and exact outcome sets; reject duplicate keys and inappropriate nulls; use validated capture structure; adversarial mutation tests |
| M0B-R2 | HIGH | `contract.go:108–112,291–308`: mutating exported loaded definition allowed campaign eligibility under unchanged pilot source hash | Bind eligibility to validated source state; mutations/construction cannot bypass hash binding; pilot must reject campaign execution |
| M0B-R3 | HIGH | `payload.go:115–138`: absent/null zero-valued seq/timestamp/padding and conflicting duplicate run_id accepted as stored content | Require actual present typed fields; reject null/duplicate raw JSON fields; test zero-valued cases across Go/JS |
| M0B-R4 | HIGH | Go timestamp validation and JS counterpart accept `1e-11`, reject valid `1048576.001`, serialize negative zero differently | Shared exact decimal/serialization rule without multiplication-tolerance heuristic; consistent negative zero policy; adversarial boundary vectors |
| M0B-R5 | MEDIUM | `workloads/k6/lib/payload.js:8–9`: identifier regex coerces undefined/null/numeric/array inputs | Require actual strings before regex; reject invalid identity types in encoder and storage validator |
| M0B-R6 | MEDIUM | Go/JS padding validators cap hex characters at half maximum body size; declared-size target 1,048,575 fails with baseline 119 | Align padding and total-byte limits, or explicitly amend supported range; test min/parity/max/max+1 and offset/seed/sequence bounds |
| M0B-R7 | MEDIUM | `contract.go:297`: NaN and positive infinity accepted as rate | Require finite positive rate; test NaN, both infinities, zero and negative values |

## Verified scope and limitations

Reviewer independently passed evidence/changed-file checksum checks, all 17 baseline untracked
snapshot comparisons, `go test ./internal/protocol -count=1 -v`,
`node tests/offline/protocol-payload-golden.mjs`, `go test ./internal/... ./cmd/...`, and
`git diff --check`. Existing tests passing did not resolve the adversarial findings.
Probe artifacts: `/tmp/opencode/m0b-astra-review-_pxbd9lw` (scratch-only Go overlay).
No repository edits or live operations were performed by the reviewer.

D44 bytes remain unchanged. D=1,000 baseline and independent N/K dimensions are explicit;
proposed numeric policies are labeled rather than installed as campaign defaults. Metric
definitions correctly specify observed-delivery percentiles including ERROR/UNKNOWN and drain
arrivals, missing counts separately, and full-scheduled-denominator attainment.

Helpers accept already-selected latencies: cohort selection, identity validation, drain filtering
and normalization are not implemented there. Runner/analyzer/adapters/sink remain disconnected.
Invalid identity mapping must not fabricate NOT_SENT identities.

## Next gate

Luna fixes all seven findings with regression evidence; Astra independently re-reviews before
M0b acceptance. Then implement a bounded protocol-bound synthetic-run to offline-analysis slice,
including binding propagation and scheduled-cohort first receipts. A five-scheduled/three-observed
fixture with latencies 100/600/20 ms must yield p99=600 ms, missing=2 and 500-ms attainment=2/5.

## Re-review of first fix packet — 2026-11-03

**CHANGES_REQUIRED.** Same independent Astra session reviewed
`m0b-r1-r7-fix-20261103.md` and `results/m0b-r1-r7-20261103-101923+0330/`.
R2 (immutable source binding), R5 (string identities), R7 (finite positive rates) are CLOSED.
Four findings remain open; severity HIGH for each:

- **R1:** comma-joined single-element field order, case-alias JSON keys (e.g.
  `schema_version` plus `SCHEMA_VERSION`), alias/null capture fields, invented retained-policy
  sources, contradictory policy keys and null pending-calibration entries bypass validation.
  Require exact case-sensitive field names and exact array elements, typed/validated policies,
  and rejection of aliases, inappropriate nulls and contradictions.
- **R3:** JS raw stored `seq:1e-400` and numeric telemetry such as
  `temperature:15.0000000000000001` round into expected integers. Validate every raw integer
  token before conversion, with matching Go/JS rules.
- **R4:** Go accepts nonzero `scheduled_send_ms:1e-9223372036854775808` (and negative
  equivalent) as zero through exponent arithmetic overflow and float underflow. Require
  overflow-safe scale validation and coefficient-digit-based exact-zero detection.
- **R6:** Go storage comparison discards both canonical encoding errors; two different oversized
  payloads therefore compare as equal nil byte slices. Propagate expected/actual encoding errors;
  test oversized inputs separately and together, retaining both parity-dependent valid maxima.

Reviewer passed protocol tests, Node golden vectors, all internal/command Go tests, diff check,
original adversarial replay, 5,010 typed Go/JS parity cases and maximum-size hash parity at
1,048,575 and 1,048,576 bytes. All prior evidence manifests and 17 baseline snapshots verified.
Scratch-only extended probes: `/tmp/opencode/m0b-rereview-98iz4lmz/`. No repository edits/live work.
Verified protocol SHA-256: `2dcc779491c00cbf2391c45bda79522dc425ba6356a8b15b9cc3442c55f565cb`;
D44 SHA-256 remains `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`.
Luna is assigned the four remaining fixes and regression evidence before further integration.

## Re-review of second fix packet — 2026-11-03

**CHANGES_REQUIRED: only R4 remains open (HIGH).** R1/R2/R3/R5/R6/R7 are CLOSED.
The extreme-exponent bypass is fixed, but Go's new `exponent > len(m)+4` guard rejects valid
whole-number timestamps after scientific formatting. Reproduced rejected Go / accepted JS inputs:
`1000000`, `1700000000000`, `1793664000000`. A 56-value comparison found 12 mismatches,
including powers of ten from 10^6 through 10^15. Required correction: safe decimal-scale
arithmetic without the coefficient-length restriction, preserving bounded exponent parsing,
exact-zero detection and supported-range checks. Shared tests must cover whole epochs,
powers/adjacent values, canonical bytes and stored validation, plus existing adversarial cases.

Reviewer independently rejected 363 schema mutations, verified matching acceptance for 204 raw
token cases, and checked oversized expected/actual/both payload rejection. Protocol tests, JS
golden tests and diff checks passed; broader suites were not repeated. Four evidence manifests,
current changed-file hashes and reconstruction of eight changed source files from the saved
scoped patch passed. Earlier records/snapshots remain preserved. Scratch probes:
`/tmp/opencode/m0b-review2-eojvudrh/`; no repository edits/live operations by reviewer.
Protocol/sidecar hash: `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`.
D44 remains unchanged. Foundation acceptance and campaign freeze are not granted.

## Final R4 re-review — 2026-11-03

**APPROVED — scoped offline foundation. All R1–R7 CLOSED.** This supersedes the preceding
foundation verdict only; campaign freeze and live qualification remain unapproved.
Reviewed packet: `m0b-r4-fix3-20261103.md`; evidence:
`results/m0b-r4-fix3-20261103-104408+0330/`.

Independent Astra replayed all 56 timestamp values: accepted consistently with identical Go/JS
canonical bytes and successful stored validation. All 204 raw-token cases agreed; extreme
exponents, nonzero underflow and excessive precision remained rejected, exact-zero normalization
correct. Protocol unit tests, Node golden suite and diff check passed. Scoped patch reconstructed
four changed source/test files exactly; current/historical evidence manifests passed. Protocol,
amendment, previous packets, D44 and baseline snapshots remained intact; protocol hash remains
`7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`.
Scratch: `/tmp/opencode/m0b-r4-final-review-_tud7am8/`. No reviewer repository edits/live work.
Same requested Astra-high routing; independent effort confirmation remains unavailable.

Next approved implementation scope: explicit protocol binding through plan/resolved/manifest,
`LoadVerified` then `CheckEligibility` in offline analysis, and synthetic scheduled-cohort
first-receipt analysis. Preserve legacy semantics; no new live capture qualification follows.
