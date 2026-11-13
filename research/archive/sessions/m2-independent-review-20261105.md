# M2 independent review — 2026-11-05

**CHANGES_REQUIRED.** M2 is not approved. No live, Docker, SUT, deployment, lifecycle, load or
campaign qualification is granted.

Reviewer session `ses_e571c8d75ffeX9L9u6K8zmADKh`, requested `opencode-go/kimi-k3.1-pro#low`.
Harness identified Kimi K3.1 Pro and exposed effort `low`. Implementation packet
[`m2-deployment-foundation-20261105.md`](m2-deployment-foundation-20261105.md); implementation
evidence `results/m2-deployment-foundation-20261105-024722+0330/`; baseline
`results/m2-baseline-20261105-023722+0330/`. Scratch probes under
`/tmp/opencode/m2-review-probes`.

## Finding

### M2-R1 — HIGH — case-insensitive JSON aliases bypass strict exact-key validation

`internal/deployment/schema.go:241–254` (`decodeStrict`) relies on
`encoding/json.Decoder.DisallowUnknownFields`, which does not reject case-variant keys. The
separate duplicate-key pass compares exact names only. Independent probes produced:

| Mutation | Result |
|---|---|
| descriptor `platform` → `PLATFORM` | exit `0`, `PASS` |
| nested service `service` → `SERVICE` | exit `0`, `PASS` |
| inventory `project` → `PROJECT` | exit `0`, `PASS` |
| container `state` → `STATE` | exit `0`, `PASS` |
| descriptor containing both `platform:"thingsboard"` and `PLATFORM:"fiware"` | exit `0`, `PASS`, final `platform=fiware` |

Exact uninterpreted interpretation and duplicate rejection are therefore not currently
enforced. The evidence-manifest decoder is identified as similarly affected.

**Required fix:** validate every object, including nested objects and the evidence manifest,
against the exact JSON tag set before typed struct decoding, with case-sensitive duplicate-key
detection. Do not rely on `DisallowUnknownFields` alone. Add descriptor, inventory, nested-object
and evidence-manifest regressions for case aliases and exact-key-plus-case-alias duplicates.
Re-run focused tests, scoped `-short`, race, vet, build and CLI smoke. Produce a new
implementation/evidence packet with a real incremental patch and hash manifest. Preserve the
existing packet without overwriting it.

## Verified clean

- `Observer` is read-only with only `Observe`; no package init, hidden live probe or lifecycle
  method. `FakeObserver` is file-backed only.
- At desscriptor/inventory mismatch, live observation stays `NOT_ASSESSED`; no generic success.
- CLI requires a new output directory, exits `0` only on `PASS`, and produces deterministic
  non-self-referential JSON/Markdown/`SHA256SUMS`.
- Focused tests pass and the patch round-trips 9/9 from the recorded pre-edit sources
  (`406ab86d…`).
- The 371-file source snapshot hashes to `054a9ce27b83…`; retained results excluding
  `results/m1f-*` and `results/m2-*` hash identically to 2,136 entries (`8e25006644…`).
- Protocol SHA-256 stays `7176b1faed9f…`; D44 stays `1b6aa76d…`. No status edits by the reviewer.
- At review time the only mismatch in the source snapshot was `docs/status.md` (`5f9155d7…ae2`
  recorded vs `b9d28dd5…7ef` current), explained by the orchestrator's status/task update; no
  status edit occurred during the review.

## Next

Fix M2-R1 offline and resubmit. After approval, the proposed next bounded milestone is an offline
deployment orchestration blueprint or runtime artifact packaging specification, still without live
execution.
