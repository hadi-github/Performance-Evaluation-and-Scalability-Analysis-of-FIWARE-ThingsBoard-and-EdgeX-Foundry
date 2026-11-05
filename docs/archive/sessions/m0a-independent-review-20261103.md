# M0a independent review — 2026-11-03

Decision: **APPROVED — recovery scope only**. No required fixes for milestone acceptance.
Campaign remains **NO-GO**.

Implementation: Luna session `ses_e61964396ffefqE0zMyJY4AEA4`, requested
`openai/gpt-6-luna#high`. Independent reviewer: session
`ses_e6193170affeUiLBF5ROkzdEfP`, requested `openai/gpt-6-astra#high`.
Both harnesses identified their respective model families; separate confirmation of effort
routing was not exposed. This record transcribes the independent review result, not a new test run.

## Evidence checked

- `results/m0a-20261103-020828+0330/`: all 28 checksums passed; all 16 baseline untracked
  snapshots matched their originals at review time. Eight tracked diffs were unchanged;
  `docs/status.md` preserved the baseline and appended section 11.
- All 66 original adapter-bundle checksums passed.
- `sut-pre-stop.txt:7–14` recorded expected argv, ports and PIDs 3830274/3830275;
  `sut-stop-final.txt:1–2` recorded closure following SIGTERM. Final receiver logs reported
  matching receipt totals and zero recording errors.
- Remote pre/post-stop hashes matched local preflush/final files. Recovered
  `original-running.txt` matched the retained remote listing.
- Immediate ledger independently reconciled exact identities 0–49,499: 1,500 warm-up and
  48,000 measured, 1,000 correctly mapped devices, no missing/unexpected/duplicate identities.
- Delayed ledger retained INVALID: 45,222 unique receipts, 3,278 dropped iterations,
  `missing=null`; original 2,100 unknown-outcome records preserved. No dropped-slot identities inferred.
- Reconciliation exited zero and reproduced `adapter-reconciliation.json` byte-for-byte.

## Independent verification

Reviewer reported exit status 0 for:

- `go test -json ./internal/... ./cmd/...` using offline dependency settings:
  145 test/subtest passes, six packages without tests.
- The three Python commands documented in the implementation packet: 1 + 1 + 4 tests passed.
- `git diff --check`, checksum verification and reconciliation reproduction.

Docker integration tests and `tests/export_run_test.py` were excluded. These checks do not
qualify platforms, capacity, clocks, SLOs or replica faults. Reviewer made no repository edits,
remote operations, live load or process stops.

## M0A-R1 — LOW — Audit-provenance limitation (nonblocking)

Implementation packet lines 16, 25 and 46–54 contain session-reported details without all
corresponding transcripts: earlier dual-host observation, memory value, failed guard,
bounded wait and historical test passes. Retained pre-stop evidence starts at 22:39:04 UTC,
not the narrative's 22:38:39. No post-stop container inventory independently establishes
the no-restoration statement. No evidence indicates container restoration.

Disposition: explicitly label these details session-reported in authoritative status;
preserve the original packet and evidence. Do not recreate historical evidence by rerunning
operations. Receiver identities, closure, hashes and reconciliation have corroborating
evidence, and offline tests independently reproduced. This finding is nonblocking.
