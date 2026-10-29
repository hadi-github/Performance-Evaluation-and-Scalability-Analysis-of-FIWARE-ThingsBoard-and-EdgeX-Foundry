# Evidence reproducibility: `docs/operations/diagnostics.md`

**Status: documentation only (2026-10-25).** No number in `docs/operations/diagnostics.md` was recreated, and the generated
report was not edited. `results/**` is untouched. Decision-log entries and benchmark definitions are unchanged.

## 1. Question

Can `docs/operations/diagnostics.md` be regenerated from the inputs retained in this checkout?

## 2. Answer

**In this working copy, yes.** `scripts/diag/report.py` runs to completion and reproduces the committed 328-line
report, with one exception: the commit string embedded in the header (line 3) and footer (line 328). The committed
report was generated at `8338b82`, and `report.py` stamps the current `HEAD` (`73dea4d`) via
`git rev-parse --short HEAD` (`scripts/diag/report.py:95`). No result number changes.

**From a fresh clone, no.** `.gitignore` ignores `results/**`; `git ls-files results` returns only
`results/exploratory/EXPL-tb-k4b.amendment.json`. Of the 76 files (47 MB uncompressed) that the generator reads, 75
are not versioned. A fresh clone therefore has the code but not the evidence.

Consequences:

- Do not hand-edit `docs/operations/diagnostics.md` and do not retype its numbers from memory.
- To regenerate, restore the input set in §4 (or the package in §5) and run the generator at a recorded commit.
- If the inputs are unavailable, record the gap; do not substitute numbers.

## 3. Verification performed (2026-10-25)

| Check | Result |
|---|---|
| `python3 scripts/diag/report.py > /tmp/opencode/diagnostics.regen.md` in this checkout | rc 0, no stderr, 328 lines |
| Diff against `docs/operations/diagnostics.md` | only the two commit strings (`8338b82` → `73dea4d`) |
| Clean clone of `HEAD` (tracked files only) plus the 76-file allowlist | clone has no `results/` evidence apart from the tracked amendment |
| Generator run in that clone at `HEAD` | byte-identical to the in-place regeneration; SHA256 `d7b3531e1ec0a107b574423bd76503751a77153f57376a7c5774e7184ebcd666` |
| Same clone checked out at `8338b82`, same 76 files, generator run | **byte-identical to `docs/operations/diagnostics.md`**; SHA256 `5b5b053147a86d9d71e6bf1ef5f893fc9f1d09c0a8fb808a5c060014b4511ed4` |

The two hashes differ only because of the commit stamp. The regenerated content is otherwise identical. The three
generator/helper scripts and the plan fallback are byte-identical between `8338b82` and `HEAD`
(`git diff --stat 8338b82..HEAD -- scripts/diag/report.py scripts/demand-fit.py scripts/diag/cpuprof-summary.py
workloads/plans/diag-fiware-k4-r100.json` is empty).

## 4. Exact input set

Generator and helpers (tracked in git):

- `scripts/diag/report.py` — the generator; writes stdout and embeds `git rev-parse --short HEAD`.
- `scripts/demand-fit.py` — called by `report.py` for the four exploratory runs.
- `scripts/diag/cpuprof-summary.py` — called by `report.py` for the FIWARE CPU profile.
- `workloads/plans/diag-fiware-k4-r100.json` — fallback source for the device count
  (`scripts/diag/report.py:88`).

Retained results (76 files, 47 MB uncompressed):

| Group | Files | Rule |
|---|---|---|
| Exploratory runs | 19 | For `EXPL-edgex-k4`, `EXPL-edgex-k1`, `EXPL-tb-k4b`, `EXPL-fiware-k4`: `explore.json`, `k6-summary.json`, `resolved.json`, and `<run>.cpu.tsv`; plus `EXPL-edgex-k4.history-final.ndjson`, `EXPL-edgex-k1.history.ndjson`, and the tracked `EXPL-tb-k4b.amendment.json` |
| DIAG summaries | 17 | `<run>.diag.json` for the 17 DIAG ids listed below |
| DIAG per-run inputs | 33 | `explore.json` for 14 runs, `resolved.json` for 15 runs, `.history.ndjson` for 4 runs |
| SUT captures | 7 | See the list below |

The 17 DIAG ids with `<run>.diag.json` inputs:

```
DIAG-edgex-k1-r200                  DIAG-fiware-k4-r100
DIAG-edgex-k1-r200-nosync           DIAG-fiware-k4-r100-cpuprof
DIAG-edgex-k4-obs-nosink-r200       DIAG-fiware-k4-r100-cpuprof2
DIAG-edgex-k4-obs-sink-r200         DIAG-fiware-k4-r100-cpuprof3
DIAG-edgex-k4-r500                  DIAG-fiware-obs-nosink-r50
DIAG-edgex-k4-r500-nosync           DIAG-fiware-obs-sink-r50
DIAG-edgex-k4-r500b                 DIAG-tb-idle
                                    DIAG-tb-idle2
                                    DIAG-tb-obs-nosink-r100
                                    DIAG-tb-obs-sink-r100
```

`explore.json` inputs exist for the EdgeX runs except `DIAG-edgex-k4-r500`, and for the FIWARE and ThingsBoard
runs; `resolved.json` exists for the EdgeX runs including `r500`, and for FIWARE and ThingsBoard. Four runs have a
history export: `DIAG-edgex-k4-obs-{sink,nosink}-r200` and `DIAG-tb-obs-{sink,nosink}-r100`.

The 7 SUT captures:

```
results/diag/sut/DIAG-edgex-k4-obs-sink-r200/containers.tsv
results/diag/sut/DIAG-fiware-k4-r100-cpuprof3/iot-agent-1.cpuprofile
results/diag/sut/DIAG-tb-idle2/tb-jvm.txt
results/diag/sut/DIAG-tb-obs-nosink-r100/tb-rulechain-without-sink.json
results/diag/sut/DIAG-tb-obs-sink-r100/tb-rulechain-with-sink.json
results/diag/sut/fiware-static/DIAG-fiware-idle-60s.txt
results/diag/sut/fiware-static/DIAG-fiware-procmodel.txt
```

These paths are the read set of `scripts/diag/report.py` and its two helpers, checked by running the generator
against exactly this set in a clean clone (§3). The run-id sets and the `os.path.exists` guards in the generator
determine the exact names; the packaged `SHA256SUMS` is the authoritative list. Do not infer additional inputs.

## 5. Smallest safe evidence package

**Contents (uncompressed 47 MB):**

1. The 76 results files in §4, rooted at `results/`.
2. `scripts/diag/report.py`, `scripts/demand-fit.py`, `scripts/diag/cpuprof-summary.py`.
3. `workloads/plans/diag-fiware-k4-r100.json`.
4. The expected output `docs/operations/diagnostics.md` and its SHA256.
5. A `META.json` recording: source `HEAD`, generation commit `8338b82`, date, command, Python version, and the
   verified diff result (only the commit stamp).
6. A sorted `SHA256SUMS` over every packaged file.

**Excluded, deliberately:** every `.env` (including `results/feasibility/*/case.env`), `*.key`, `*.pem`, tokens,
credentials, `.git/`, monitoring credentials, and any log not read by the generator. A scan of the 76 files found
no credential values; the only `apikey` match is a MongoDB index key name in
`results/diag/DIAG-fiware-k4-r100.diag.json`, not a secret.

**Packaging sketch (deterministic, no secrets):**

```bash
# evidence-files.txt: the sorted relative paths from §4, one per line
tar --sort=name --mtime='UTC 2026-10-22' --owner=0 --group=0 --numeric-owner \
    -czf diagnostics-evidence-2026-10-25.tgz -T evidence-files.txt
xargs -a evidence-files.txt sha256sum > SHA256SUMS
```

**Storage:** outside git (artifact store or encrypted disk). `results/**` is gitignored on purpose and run output
must not be committed; only `*.amendment.json` files are versioned.

**Reproduction check:** extract into a checkout at `8338b82`, run
`python3 scripts/diag/report.py > /tmp/diagnostics.md`, and compare. At `8338b82` the output is byte-identical. At
a later commit, expect exactly two lines to differ (the commit stamp in the header and footer); no result changes.

## 6. Limits of the package

- The package reproduces the report from **derived** inputs (`explore.json`, `.diag.json`, history NDJSON, SUT
  captures). It does not re-derive those from raw Prometheus scrapes or platform databases. That stronger replay
  would additionally need the collector scripts (`scripts/diag/*.sh`), `scripts/diag/analyze.py`, the run manifests,
  and access to the SUT for the collector captures; it is not part of the smallest package.
- `report.py` is not hermetic: it stamps ambient `git HEAD` and invokes the two Python helpers. Pin the checkout
  commit in `META.json`.
- `EXPL-edgex-k4` has both `history.ndjson` and `history-final.ndjson`; the generator prefers the `-final` export
  (`scripts/diag/report.py:80-81`). The package includes the file actually read.
- Some runs legitimately have no `explore.json` (the two idle runs and `DIAG-edgex-k4-r500`); the generator guards
  those reads with `os.path.exists` (`scripts/diag/report.py:63,68`). Their absence is not a missing input.
- If any §4 file is lost, the report cannot be regenerated faithfully. Do not fill the gap from memory.

## 7. `findings/` paths

- The top-level `findings/` directory is **empty**, untracked (git cannot track an empty directory), and referenced
  nowhere in the repository.
- The canonical path is `docs/findings/`, which contains `research/findings/edgex-device-validation-panic.md`; that file is cited by
  `edgex/scripts/provision-then-scale.sh:2`, `docs/methodology/exploratory-capacity.md:102`, and the EdgeX plans.
- The empty top-level directory has no effect on a clone or on regeneration. It was **not deleted** here (no
  deletion authorization); the owner may remove it or ignore it.

## 8. Remaining drift and open items

- **Not corrected (outside the docs-only scope):** `monitoring/grafana/build_dashboards.py:114,279` still emits
  "Unavailable: sink receipts" and "producer not implemented yet" into the generated dashboard, although the sink
  exists and its panel queries the live sink counters. Fixing it is a dashboard-generator change followed by
  `make dashboards`, not a documentation edit. The committed dashboard JSON is generated by that script and must not
  be hand-edited.
- **Open methodology items are unchanged** and remain in `docs/handoff-review-2026-10-25.md`: D27 Category A
  eligibility, D30's missing observation-point table, and the historical in-place D11 edit. This document does not
  decide them.

## 9. What this document does not do

No platform setting, compose file, threshold, decision-log entry, manifest, raw result, or generated report was
changed. No data or volume was deleted, no stack was started or stopped, no credential was read or printed, and
nothing was committed or published.
