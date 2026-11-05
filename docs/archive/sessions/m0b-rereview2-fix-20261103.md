# M0b second independent-review fix cycle — 2026-11-03

**Disposition:** addressed the remaining R1, R3, R4, and R6 findings; submitted for another independent review. R2, R5, and R7 remain closed with their existing regression coverage. This packet does not claim M0b acceptance. No M1 integration, live test, deployment, campaign use, or `docs/status.md` edit was performed. Earlier packets/evidence and the D44 profile remain preserved.

Requested routing: `openai/gpt-6-luna#high`; actual harness model identity was OpenAI GPT-6 Luna. The `#high` effort route remains independently unverified.

## Fixes and regressions

| Finding | Change | Adversarial validation |
|---|---|---|
| **R1 HIGH** | Added a recursive exact-case JSON schema-key gate before `encoding/json` struct decoding. Each fixed object has an exact required key set; aliases/case collisions, unknown nested keys, duplicates, and nulls are rejected (except the explicitly required `zero_observed_percentile: null`). Replica-map objects are closed and keyed by the declared systems/services. `field_order` is compared element-by-element as the exact eight-name array. Retained-policy objects are closed, source anchors and normative values are exact, proposal multipliers/status are checked, and pending-calibration strings are exact and non-null. | Table-driven independent mutations include joined-string/reordered field order, schema aliases/collisions at root and nested fields, unknown nested policy keys, contradictory invented source references, extra multiplier, null pending item, missing/null/case-aliased fields, plus generated null-at-each-object-leaf cases. Existing exact duplicate-key tests remain. |
| **R3 HIGH** | Before numeric conversion, Go and JS validate raw `seq`, `temperature`, `humidity`, and `pressure` as canonical integer JSON tokens (`0` or positive base-10 integer, no fraction/exponent/sign alias). Existing duplicate-key scanning remains. JS still only accepts raw text/bytes, never an already-parsed object. | Paired Go/JS raw-body cases reject `seq:1e-400`, fractional/rounded temperatures/humidity/pressure such as `15.0000000000000001`, and decimal/exponent aliases such as `0.0`, `15e0`, and `30.0`. Existing missing/null/duplicate vectors remain. |
| **R4 HIGH** | Decimal scale and zero are derived from coefficient digits and bounded exponent parsing, never float-underflow tests. Exponent magnitude above 10000 is rejected. Nonzero underflow/nonrepresentable raw timestamps fail; scale math is bounded before subtraction. Exact zero coefficients—including signed zero forms—normalize consistently. Updated protocol semantics and append-only amendment document the rule. | Go/JS vectors reject nonzero `1e-400`, `1e-9223372036854775808`, huge positive exponents, and extreme zero exponents; accept canonical signed-zero forms; continue to exercise epoch, valid three-decimal timestamps, max safe integer, high precision, and the prior `1048576.001` vector. |
| **R6 HIGH** | `ValidateStored` checks and returns errors from `CanonicalBytes(expected)` and `CanonicalBytes(actual)` before byte comparison. Complete encoded-body limits are therefore enforced for each side; two failed encodings can no longer compare as equal nil slices. | Tests make expected-only, actual-only, and both-sides payloads exceed the 1 MiB encoded-body maximum, including unequal identity/temperature/padding and the 1,048,703-byte stored fixture. Positive maximum/parity target cases remain. |

R2 source/profile immutability and categorical no-campaign behavior, R5 strict JS string identities, and R7 finite positive pilot-rate gates were not weakened. Their prior adversarial tests remain and were rerun with the Go suite.

## Scope and boundaries

Only the preliminary offline protocol contract/payload implementation and its Go/JS regression tests and protocol semantics/amendment are in scope. No integration into plans/adapters/analyzers/sink was added. Cohort membership, first-receipt stream selection, cutoff enforcement, and evidence-completeness attestation remain unimplemented; no complete metric pipeline or live qualification is claimed. Campaign eligibility stays fail-closed. The D44 profile is unchanged; its SHA-256 remains `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`. Updated protocol JSON SHA-256: `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`; amendment SHA-256: `b3a3a3e06d6bc4488d49f1bf3d2470c2cdd236215f484e3eb4f799025c308412`.

## Validation and evidence

The complete scoped diff from the captured pre-fix state, exact changed-file hashes, source copies, command output/exit codes, protocol hashes, and checksums are in `results/m0b-rereview2-fix-20261103-*/`. That directory also records verification of the preceding fix-cycle and older M0a/M0b evidence manifests. Integration tests that launch Docker/k6 and all live/deployment tests were intentionally not run.

Please independently review the exact-key recursion and retained-policy schema, raw JSON integer-token semantics and cross-language vectors, exponent/underflow edge cases, and all canonical encoding error paths. Stop here pending independent re-review.
