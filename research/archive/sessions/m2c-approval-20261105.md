# M2c approval — 2026-11-05

**APPROVED for scoped M2c offline render/verify validation.** No live, Docker, SUT, deployment,
lifecycle, load or campaign authorization is granted.

## Scope verified

- Existing `benchctl deployment-validate` was exercised with synthetic descriptor/inventory:
  valid fixture `PASS`/`live_validation=NOT_ASSESSED` exit 0; controlled mismatch `FAIL` exit 1
  with inspectable output; case-alias descriptor exit 2 with no output directory.
- Existing orchestration v2 and artifact-manifest APIs were canonicalized and rendered
  deterministically; computed canonical hashes matched recorded evidence.
- `ArtifactManifest.Verify` returned 4/4 PASS, FAIL for tampered hash and bytes, UNAVAILABLE for a
  missing file, and `NOT_IMPLEMENTED`/`NOT_ASSESSED` for prior-manifest binding.
- Every fixture carried explicit `SYNTHETIC M2c` labels; no live observation, storage claim or
  path collision was introduced.
- No production interface, CLI semantic or lifecycle action was added. The implementation delta
  was test-only plus fixtures and documentation.
- Focused/scoped/race/vet/build/format/diff checks passed; patch round-tripped 9/9.

## Review provenance

Reviewer: `opencode-go/deepseek-v4-pro#high`, session `ses_e557e06aefferRFlsPQc9HpYVD`.
Implementation: DeepSeek V4.1 Flash, requested `#high`. Packet
[`m2c-render-verify-20261105.md`](m2c-render-verify-20261105.md); evidence
`results/m2c-render-verify-20261105-085543+0330/`.

## Integrity

- Protocol `7176b1fa…24ee` and D44 profile `1b6aa76d…1f8` remain unchanged.
- Retained results excluding `results/m1f-*`, `m2-*`, `m2b-*`, `m2c-*` remain 2,136 files;
  canonical aggregate `8e25006644…25ec` is reproducible.
- Prior M1f/M2/M2b artifacts: 103 references, 0 problems.
- No unrestricted full-suite claim; Docker integration was skipped by `-short`.

## Nonblocking findings

- F1 (LOW): packet retains the correct canonical aggregate hash, but its per-cycle retained
  manifest used locale-aware sorting (`ec6db6f0…`). The underlying files were verified unchanged
  and the canonical hash remains reproducible. Future cycles use `LC_ALL=C sort`.
- F2 (LOW): packet text grep produced a false missing-clause exit; packet content and hashes are
  self-consistent. No content defect.
- F3 (INFO): synthetic ThingsBoard replica shape is not reachable via the single replica knob; it
  is explicitly synthetic, internally consistent and fabricates no service. No action.

Next offline milestone: render-only execution plan/runbook. No live execution or lifecycle action
is authorized by this approval.
