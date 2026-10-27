# Pre-registration archive

**Status: read-only historical record. Do not edit the files in `original/`.**

This directory pins the original research specification that predates the implementation decisions. It was created
on 2026-10-26 under D40 so that every later report can be checked against the thresholds and definitions that were
fixed before the first data existed.

| Path | Contents |
|---|---|
| `original/` | Byte-for-byte copies of the nine source files |
| `SHA256SUMS` | SHA-256 of every archived file, verifiable with `sha256sum -c` |
| `SOURCE-MANIFEST.md` | Source paths, source dates, roles, declared versions, gaps |

## Rules

1. **No edits.** The archived files are historical. Corrections, clarifications and operational decisions go into
   `docs/design-decisions.md` and `docs/methodology-reconciliation.md`, never into `original/`.
2. **No silent replacement.** If the source is revised, add a new dated archive directory; do not overwrite this one.
3. **No reconstruction.** A source that cannot be located is reported BLOCKED. It is not recreated from memory or
   from secondary documentation.
4. **Differences are explicit.** The gap between the original wording and the adopted operational decisions is
   listed in `docs/methodology-reconciliation.md` (S1–S5 and the protocol record).

## Verification

```bash
scripts/check-preregistration.sh
```

Expected: every line prints `OK`, exit status 0. A mismatch means either the archive or the manifest was modified;
investigate before using the archive as authority.

## Reading order

1. `original/rqs.md` — research questions and the pre-registered hypotheses.
2. `original/slo_notes.txt` + `original/slo_components.txt` — SLO terms, acceptance rule, gates.
3. `original/workloads.txt` — workload model, generator requirements, gates.
4. `original/fingerprint_notes.txt` + `original/fingerprint_metrics.txt` — fingerprint metrics.
5. `original/metrics_notes.txt` + `original/metrics_list.txt` — measured vs monitored metrics.
6. `original/decisions.md` — the framing summary that ties the above together.
7. `docs/methodology-reconciliation.md` — what changed after the originals were written, and why.

## Known limits

- `original/` is read-only by file permission in this checkout; Git does not preserve that permission bit, so the
  rule is also stated here.
- The source files are plain text/markdown with no internal version control. The mtimes in `SOURCE-MANIFEST.md` are
  the only file-level dates.
- The archive contains specification text only. It does not contain the exploratory or diagnostic results, which
  are listed in `docs/evidence-integrity-manifest.md`.
