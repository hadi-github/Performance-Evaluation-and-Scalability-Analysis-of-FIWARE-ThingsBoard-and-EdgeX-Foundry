# Baseline evidence-integrity manifest

**Generated:** 2026-10-27 09:59 UTC from `HEAD` `73dea4d5493280125c7d330356a90433d78889d2`. **Read-only audit:** no file under `results/**` was created, edited, moved or deleted.

This manifest lists the retained evidence needed to reproduce the exploratory and diagnostic findings, its hashes, the run verdict and the related decisions. The machine-readable form is `docs/evidence-integrity-manifest.csv`; the bulk-verification list is `docs/evidence-integrity.sha256`.

## Totals

- Files: **758**
- Bytes: **423,927,356** (404.3 MiB)
- Runs with a `benchctl` manifest: **30** (22 VALID, 8 INVALID)
- Feasibility cases without a manifest: **9** (UNVERIFIED)
- Runs with `explore.json` analysis: **19**
- Shared log/support collections (not runs): **2**

## Per-run inventory

| Suite | Run | Verdict | Git | Health | Files | MiB | Required gaps | Decisions |
|---|---|---|---|---|---|---|---|---|
| diag | `DIAG-edgex-k1-r200` | VALID | `61b6f22ff6ec` | valid | 21 | 6.2 | — | D23; D29a; D23 diagnostic |
| diag | `DIAG-edgex-k1-r200-nosync` | VALID | `61b6f22ff6ec` | valid | 21 | 6.7 | — | D23; D29a; D23 diagnostic |
| diag | `DIAG-edgex-k4-obs-nosink-r200` | VALID | `61b6f22ff6ec` | valid | 21 | 2.5 | — | D23; D29a; D23 diagnostic |
| diag | `DIAG-edgex-k4-obs-sink-r200` | VALID | `61b6f22ff6ec` | valid | 21 | 6.9 | — | D23; D29a; D23 diagnostic |
| diag | `DIAG-edgex-k4-r500` | VALID | `61b6f22ff6ec` | valid | 20 | 14.0 | no explore.json | D23; D29a; D23 diagnostic |
| diag | `DIAG-edgex-k4-r500-nosync` | VALID | `61b6f22ff6ec` | valid | 21 | 15.3 | — | D23; D29a; D23 diagnostic |
| diag | `DIAG-edgex-k4-r500b` | VALID | `61b6f22ff6ec` | valid | 21 | 14.2 | — | D23; D29a; D23 diagnostic |
| diag | `DIAG-fiware-k4-r100` | VALID | `aa5a97a582d1` | valid | 15 | 3.1 | — | D9 open; D27 conditional; D23 diagnostic |
| diag | `DIAG-fiware-k4-r100-cpuprof` | VALID | `f370025e73c5` | valid | 15 | 3.3 | — | D9 open; D27 conditional; D23 diagnostic |
| diag | `DIAG-fiware-k4-r100-cpuprof2` | INVALID | `b81f9996300e` | invalid | 15 | 3.3 | — | D9 open; D27 conditional; D23 diagnostic |
| diag | `DIAG-fiware-k4-r100-cpuprof3` | VALID | `3ea06644a10b` | valid | 15 | 0.1 | — | D9 open; D27 conditional; D23 diagnostic |
| diag | `DIAG-fiware-obs-nosink-r50` | VALID | `aa5a97a582d1` | valid | 15 | 0.1 | — | D9 open; D27 conditional; D23 diagnostic |
| diag | `DIAG-fiware-obs-sink-r50` | VALID | `aa5a97a582d1` | valid | 15 | 1.6 | — | D9 open; D27 conditional; D23 diagnostic |
| diag | `DIAG-tb-obs-nosink-r100` | VALID | `a249513baf3a` | valid | 16 | 1.3 | — | D23 diagnostic |
| diag | `DIAG-tb-obs-sink-r100` | INVALID | `a249513baf3a` | invalid | 16 | 3.6 | — | D23 diagnostic |
| exploratory | `EXPL-dryrun-090308` | VALID | `6bfa0f27c53d` | valid | 11 | 0.1 | no explore.json | D24 |
| exploratory | `EXPL-dryrun-091551` | VALID | `5277342373e7` | valid | 15 | 10.4 | — | D24 |
| exploratory | `EXPL-edgex-k1` | VALID | `671262e644e3` | valid | 20 | 30.1 | — | D23; D29a; D24 |
| exploratory | `EXPL-edgex-k4` | INVALID | `671262e644e3` | invalid | 21 | 91.3 | — | D23; D29a; D24 |
| exploratory | `EXPL-edgex-trace2` | INVALID | `7d06587c6001` | invalid | 13 | 0.0 | no explore.json | D23; D29a; D24 |
| exploratory | `EXPL-edgex-trace3` | INVALID | `7d06587c6001` | invalid | 13 | 0.0 | no explore.json | D23; D29a; D24 |
| exploratory | `EXPL-fiware-k4` | INVALID | `671262e644e3` | invalid | 15 | 138.0 | — | D9 open; D27 conditional; D24 |
| exploratory | `EXPL-fiware-trace` | VALID | `671262e644e3` | valid | 13 | 0.0 | no explore.json | D9 open; D27 conditional; D24 |
| exploratory | `EXPL-fiware-trace2` | VALID | `671262e644e3` | valid | 13 | 0.0 | no explore.json | D9 open; D27 conditional; D24 |
| exploratory | `EXPL-preflight-085930` | VALID | `8c700289832e` | valid | 10 | 0.0 | no explore.json | D24 |
| exploratory | `EXPL-tb-k4` | INVALID | `7d06587c6001` | invalid | 16 | 0.1 | no explore.json | D24 |
| exploratory | `EXPL-tb-k4b` | INVALID | `671262e644e3` | invalid | 17 | 31.7 | — | D24 |
| exploratory | `EXPL-tb-trace` | VALID | `7d06587c6001` | valid | 13 | 0.0 | no explore.json | D24 |
| exploratory | `EXPL-tb-trace2` | VALID | `671262e644e3` | valid | 13 | 0.0 | no explore.json | D24 |
| feasibility | `A0-baseline` | UNVERIFIED | `—` | — | 6 | 0.7 | — | M0 feasibility; no manifest |
| feasibility | `A1-vu-exhaustion` | UNVERIFIED | `—` | — | 6 | 0.1 | — | M0 feasibility; no manifest |
| feasibility | `A2-generator-delay` | UNVERIFIED | `—` | — | 6 | 0.5 | — | M0 feasibility; no manifest |
| feasibility | `A3-target-delay` | UNVERIFIED | `—` | — | 6 | 0.4 | — | M0 feasibility; no manifest |
| feasibility | `B0-baseline` | UNVERIFIED | `—` | — | 6 | 0.7 | — | M0 feasibility; no manifest |
| feasibility | `B1-vu-exhaustion` | UNVERIFIED | `—` | — | 6 | 0.4 | — | M0 feasibility; no manifest |
| feasibility | `B2-generator-delay` | UNVERIFIED | `—` | — | 6 | 1.9 | — | M0 feasibility; no manifest |
| feasibility | `B3-target-delay` | UNVERIFIED | `—` | — | 6 | 0.4 | — | M0 feasibility; no manifest |
| feasibility | `C1-genlag-ample-vus` | UNVERIFIED | `—` | — | 6 | 0.7 | — | M0 feasibility; no manifest |
| runs | `fixture-smoke-20261021t235415z-bf1d` | VALID | `1776c9924c35` | valid | 11 | 0.0 | no explore.json | — |

## Supporting collections (not runs)

| Suite | Directory | Files | MiB | Role |
|---|---|---|---|---|
| diag | `results/diag/sut` | 207 | 14.3 | Host/container logs and captured diagnostics; no run manifest, so no verdict. |
| exploratory | `results/exploratory/sut-logs` | 12 | 0.0 | Host/container logs and captured diagnostics; no run manifest, so no verdict. |

## Missing or unverified evidence

| Item | State | Consequence |
|---|---|---|
| `results/feasibility/*` | No `benchctl` manifest, git revision or health verdict | Generator feasibility findings are pilot evidence only; they cannot be promoted to a qualified run without a manifest. |
| FIWARE runs | No history export (no QuantumLeap) | No store-side reconciliation; D9/D31 open. |
| `EXPL-tb-k4b` | Memory deviation lives in `EXPL-tb-k4b.amendment.json`, not in the manifest | The amendment is retained and hashed; the original manifest is unchanged. |
| Runs without `explore.json` | Analysis not generated or superseded | The raw summary, health and records remain; re-running `benchctl explore` is possible but was not done here. |
| `results/diag/DIAG-edgex-k4-r500` | Superseded by `r500b` (first run lacked exporters) | Retained; do not use as the primary point. |
| `results/exploratory/sut-logs`, `results/diag/sut` | Host/container logs, no run manifest | Supporting diagnostics only. |
| Unattached files | See rows with `(unattached)` in the CSV | Each still has a hash; none was moved. |

## Reproduction map

| Finding | Primary evidence | Reproduce with |
|---|---|---|
| Diagnostics report (`docs/diagnostics.md`) | `results/diag/**` | `scripts/diag/report.py` (reproduces the report except the embedded commit) |
| Exploratory capacity (`docs/exploratory-capacity.md`) | `results/exploratory/**` | `benchctl explore` per run with its sink/history/CPU files |
| Generator calibration (`docs/calibration.md`) | `results/feasibility/**` | Re-run the feasibility cases; no manifest exists, so the raw files are the record |
| TB memory deviation (`EXPL-tb-k4b`) | `results/exploratory/EXPL-tb-k4b.amendment.json` + `sut-logs/tb-prep.*` | Compare `tb-prep.log` memory check with the manifest |
| EdgeX WAL / persistence | EdgeX DIAG sink, history, persist and `diag.json` files | `benchctl explore` with the persistence series |

## Handling rules

- Do **not** move, rename, rewrite or delete any file under `results/` to package this evidence. The manifest is additive.
- Do **not** stage the 404 MiB of raw evidence in Git automatically. If an archive is needed, create a tar outside the repository or a release asset, and verify it against `docs/evidence-integrity.sha256`.
- A pattern scan found no credential values in the scanned logs, but raw evidence is still not automatically publishable; review before any external release.
- A fresh clone does not contain `results/**` (`.gitignore`), so the manifest proves integrity only when the files are present.

## Verification

```bash
sha256sum -c docs/evidence-integrity.sha256
```
