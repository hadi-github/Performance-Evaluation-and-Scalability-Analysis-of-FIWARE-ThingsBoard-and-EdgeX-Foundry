# M1f independent review — 2026-11-05

**CHANGES_REQUIRED.** M1f is not approved. No live, deployment or campaign qualification is
granted.

Review session: `ses_e575a9429ffeYSYmg0XdMc2a9w`; requested route
`opencode-go/kimi-k3.1-pro#low`. The harness identified Kimi K3.1 Pro; effort control is exposed
and recorded as `low`. Implementation packet `m1f-fault-capacity-20261104.md`; review evidence
`results/m1f-fault-capacity-20261105-013824+0330/`; scratch probes
`/tmp/opencode/m1f-review-probes`.

Earlier attempts to route the review to Astra were blocked externally:
- `ses_e575c9738ffe6zc8Xui599YNGA`: subscription-sharing limit before review started.
- `ses_e575b9ae9ffem60Vz9hZsGuKVV`: insufficient account funds before review started.
- `ses_e575af71bffeWysMvgshSTYqM2`: subscription-sharing limit before review started.

## Findings

| ID | Severity | Evidence | Required correction / acceptance |
|---|---|---|---|
| M1F-R1 | HIGH | `capacity_rates` retains caller order; the passing-prefix and upper-bound searches run on unsorted input. `[pass20,fail10,fail40]` produced `bracketed` with its pass above its fail. Sorted non-monotonic input `[10p,20f,30p,40f]` also returned `bracketed` despite the contradiction. | Sort no-fault rate inputs before all bound calculations; make results permutation invariant; confirmed pass-above-fail contradictions must not yield `bracketed` or an unsupported upper bound. Retain contradictions visibly. Every permutation of the probe input must produce the same non-bracketed result; existing passing/non-monotonic cases remain green. |
| M1F-R2 | HIGH | `fault_target.go` trusts each row's self-declared `compose_service` rather than validating the row against its enclosing service. A spoofed nested service row and an inventory with missing/`>=N` replica indices were accepted. | Validate each container's project/service against its enclosing service; resolve only from the uniquely matched allowlisted service; validate the complete declared replica set as unique indices `[0,N)`. Nested-service spoof probes and incomplete/incorrect replica sets must be refused without emitting a target. |
| M1F-R3 | HIGH | Only `stop_request_ms` is checked against the declared window; a healthy rejoin outside the window still qualified. | Validate every supplied stage timestamp against `window_start_ms/window_end_ms`. Events with stages outside the declared window cannot qualify or produce recovery claims; test each stage individually. |
| M1F-R4 | MEDIUM | Grace was treated as a non-positive timestamp so `grace_ms=0` returned INVALID, despite being a valid duration. Missing stop completion still asserted `Transient.StopObserved=true`. | Treat grace as a finite duration `>=0`, separately from timestamps. Derive stop observations only from explicit stop-completion evidence or provide separate request/observation semantics. Zero-grace valid events qualify; negative/non-finite remain invalid; request-only events do not report an observed stop. |
| M1F-R5 | MEDIUM | The packet claims `incremental.patch`, `final-files.sha256` and `evidence-manifest.json`, but none exists in the packet. | Preserve the original packet unchanged; add a corrected supplemental packet with actual final-file hashes, incremental diff, manifest, and verified patch round-trip under the new packet directory. |

## Verified positively

- The three allowlisted mappings and ordinary refusal tests pass; no Docker client, stop execution,
  scheduler or fault-execution field was added.
- Event source is restricted to `operator/manual`; unexpected transitions invalidate; dry runs can
  never qualify; N=1 is labelled separately; degraded topology is declaration-based.
- Fault windows do not create capacity bounds; failover/rejoin are separate; non-recovery is
  right-censored with `exact=false`; invalid points do not directly establish bounds.
- Predicate, search policy, thresholds and confirmation counts are explicit with no default hybrid
  or 5-of-5 rule.
- CLI output is file-only with a new directory and always shows
  `Campaign capacity: **NOT CLAIMED**` unless explicitly set.
- Normal `Analyze` does not attach optional fault/capacity fields by default.
- Focused tests, race tests, vet, formatting, diff checks, CLI JSON parsing and CLI checksums pass.
- Protocol SHA-256 remains `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee`;
  D44 profile remains `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8`.
- Retained results excluding `results/m1f-*` reproduce exactly: 2,136 files,
  SHA-256 `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`.
- All nine recorded command exit codes are `0`; no false unrestricted-suite claim.

## Next gate

DeepSeek Flash corrects R1–R5 within offline code only and adds a supplemental evidence packet
with reproducible hashes and patch. Then an independent reviewer must re-verify. M1f needs
approval before M2 instrumentation/deployment work. The proposed M2 is a versioned, read-only
instrumentation/deployment foundation, not a fault or campaign run.
