# Evidence

This directory holds the integrity layer over the raw benchmark data that stays out of Git (see `.gitignore`):
the machine-readable manifest, the bulk-verification list, the reproduction notes and the pinned upstream source
excerpts. Raw run data (≈ 416 MB) is **not** published in Git — reviewed bundles ship to a separate research
archive with checksums and an index entry linking bundle → source state (backlog item in
`docs/status.md` §5). Known-answer fixtures deliberately kept in Git live under `tests/`.

## Verifying

```bash
sha256sum -c evidence/evidence-integrity.sha256       # every listed results/** file must verify
python3 scripts/check-preregistration.sh              # preregistration archive bytes (research/preregistration/)
```

- `evidence-integrity-manifest.csv` — one row per evidence file: run, verdict, size, SHA-256, status.
- `evidence-integrity-manifest.md` — per-run summary, gaps, reproduction notes (human-readable mirror).
- `evidence-integrity.sha256` — `sha256sum -c` compatible list (repo-relative paths), maintained by
  `scripts/evidence-manifest.py`. It is regenerated only by that tool from the current tree; edits are recorded as
  a new dated entry in the manifest, never as a silent rewrite.
- `reproducibility.md` — what a fresh clone can and cannot reproduce, and why.
- `source-excerpts.md` — pinned upstream source excerpts (ThingsBoard/EdgeX/FIWARE versions, with line ranges),
  produced by `scripts/source-excerpts.sh`; treat as upstream attribution material.

## Path mapping for historical records

Historical manifests and the preregistration archive were recorded with the pre-2026-10-29 repository layout
(platform dirs at the root). Content hashes were never rewrited — only paths changed. Mapping:

| Historical path | Current path |
|---|---|
| `edgex/` | `deploy/edgex/` |
| `thingsboard/` | `deploy/thingsboard/` |
| `fiware/` | `deploy/fiware/` |
| `docs/preregistration/` | `research/preregistration/preregistration-archive/` |
| `docs/evidence-integrity-*.{csv,md,sha256}` | `evidence/` |
| `docs/evidence/source-excerpts.md` | `evidence/source-excerpts.md` |
| `docs/evidence-reproducibility.md` | `evidence/reproducibility.md` |
| `docs/protocol-20261028-generator-ceiling.md` | `research/protocols/generator-ceiling-20261028.md` |
| `docs/findings/`, root `MONITORING_FINDINGS.md` | `research/findings/` |
| `docs/autonomous-*.md`, `docs/autonomous-state*.json` etc. | `research/archive/sessions/` (verbatim) |
| legacy `workloads/*.js`, `run.sh`, `sink.js`, `lib/` | `workloads/legacy/` |
