# M1d approval — 2026-11-04

**APPROVED for scoped M1d offline complete-body receiver contract.** No deployment, live
qualification, clock qualification or campaign freeze is granted.

Reviewer session: `ses_e5c40004cffeqr4BbZeigwIZ1w`; requested route
`opencode-go/deepseek-v4-pro#high`; harness identified DeepSeek V4 Pro, effort control unexposed.
Packet: `results/m1d-complete-body-20261104-024117+0330/`; baseline:
`results/m1d-baseline-20261104-022833+0330/`; summary
[`m1d-complete-body-20261104.md`](m1d-complete-body-20261104.md).

Transcription of independent review; no live experiment.

## Verified

- HEAD/branch unchanged. All 12 changed/new file hashes matched manifest and `final-files`;
  delta contained 10 modified and 2 new files; `docs/status.md` absent.
- Retained results inventory of 1,966 files was byte-identical before/after
  (`01b7aa44…fce3a`); protocol and D44 hashes unchanged; incremental patch reconstructed the
  working delta.
- Focused tests, race tests, scoped `-short`, diff check, vet and Node parity all reproduced exit 0.
  Only expected Docker integration and Node-availability checks were skipped; no false suite claim.

## Contract behavior

- Receipt timestamp is taken after a successful bounded complete-body read and before parsing;
  a controlled clock test proved the second read saw the final body bytes, before the parse hook.
- Failed, truncated and oversized bodies produce no receipt or partial identity; diagnostics are
  explicit and existing overflow/duplicate semantics are unchanged. The 4 MiB boundary was tested.
- Batched identities share the request-level complete-body timestamp; no per-identity timestamp is
  fabricated.
- Legacy `recv_ms` and schema 1 remain readable, but cannot support new primary delivery metrics.
  New schema requires exact semantics/endpoint and rejects missing, null, case-alias, duplicate or
  unsupported fields before typed decode.
- No hidden clock attestation. Same-host wall-clock domain limitation is surfaced; cross-host and
  monotonic qualification remain unavailable.
- Retained evidence was not relabeled.

## Nonblocking observations

- Oversized/truncated request handling answers HTTP 200 by existing design; loss is observable via
  sink counters, not status or exit code. Adapter-layer signalling is deferred to M1e.
- Receipt field-name/unit constants are not compile-time linked to struct tags; values match.

Next approved scope: adapter payload controls and adapter-boundary response semantics, offline
only. No deployment or live qualification follows.
