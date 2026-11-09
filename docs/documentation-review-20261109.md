# Documentation review — 2026-11-09

This review covers the tracked documentation tree plus the local documentation
handbook. It is a navigation and maintenance review, not a claim about current
runtime state or experimental validity.

## Scope checked

- Root `README.md` and `CONTRIBUTING.md`.
- `docs/README.md`, the consolidated design/method pages, operations,
  development, methodology, qualification and archive material.
- `DOCUMENTING.md` for the repository's documentation rules.

The tree currently contains more than 90 files under `docs/` and roughly 1 MB of
documents. The archive accounts for a large part of that volume.

## What the existing documents cover

| Area | Existing coverage |
|---|---|
| Root `README.md` | Project overview, local build/smoke checks, resource layout, design decisions, limitations and layout |
| `docs/README.md` | Reading order, authoritative-status pointer, evidence boundary and directory map |
| `docs/status.md` | Current campaign checkpoint, historical qualification record, blockers, contradictions, evidence inventory and owner inputs |
| `docs/architecture.md` | Two-host architecture, fixed CPU budgets, HAProxy path, platform paths, generator/adapters, evidence and monitoring |
| `docs/methodology.md` | Observation/claim separation, timing endpoints, identity accounting, validity, windows, percentiles, CPU, protocol provenance, B0/W1–W4, payload size and repetitions |
| `docs/research-guide.md` | Research questions, contributions, setup, procedure, protocol evolution, claim-to-evidence mapping and publication limits |
| `docs/operations/` | Ordinary-campaign runbook, persistent execution, diagnostics, publication handoff, review/export and server access |
| `docs/development/` | Tooling interfaces, offline analysis, test catalogue, deployment foundation, packaging and generator progress |
| `docs/methodology/` | Detailed contracts and reconciliation records, including the dispatch contract and metrics dictionary |
| `docs/qualification/` | Blockers, D28/D29a checks, readiness checklist, Step-5 decision packet and owner sign-off |
| `docs/archive/sessions/` | Verbatim historical session narratives and state snapshots |
| `CONTRIBUTING.md` | Ground rules, local verification and pull-request expectations |

The material is technically strong where it exists. It preserves distinctions
between dispatch, wire transmission, HTTP acknowledgement, sink receipt and
history read-back. It also preserves failed attempts and explicit limits rather
than relabelling them as successes.

## Findings

1. **The main reading path is too large and fragmented.** A new engineer must
   discover commands, evidence paths and safety rules across many pages.
2. **Current and historical material are mixed.** `docs/status.md` is the
   authoritative status page, but it is long and contains dated qualifications,
   amendments and historical observations alongside the current checkpoint.
3. **There is no compact onboarding page, glossary or troubleshooting entry.**
   Terms such as B0, W1–W4, TB/FW/EX, N, k, QL, receipt and capture window are
   defined across several documents.
4. **There is no single command entry point.** The safe build/test checks,
   campaign controls, status checks and evidence commands live in different
   pages.
5. **The newest setup failure has no reader-facing troubleshooting path.** The
   ordinal-11 EdgeX failure is recorded in evidence and the campaign log, but not
   explained as a reusable failure class.
6. **The proposal and paper material is valuable and must remain.** The
   architecture, methodology, research guide, qualification records, frozen
   protocols and historical status records are inputs to later publication and
   cannot be replaced by a short onboarding page.

## Decisions

- Keep `docs/status.md` as the authoritative current-status record.
- Keep `docs/architecture.md`, `docs/methodology.md`, `docs/research-guide.md`,
  qualification records, frozen protocols and the archive.
- Add a small navigation layer rather than rewriting the scientific record.
- Treat the archive and qualification packets as historical/paper material, not
  as the first page a new engineer must read.
- Do not remove or relabel historical attempts, proposals or caveats.
- Do not use this review to authorize traffic, deployment, cleanup or publication.

## Initial fixes

This review is paired with a first small documentation pass:

- `docs/quickstart.md` — the shortest safe path through the repository.
- `docs/glossary.md` — shared terms and their canonical references.
- `docs/operations/troubleshooting.md` — known failure classes and safe next
  actions, starting with the EdgeX readiness failure.
- `docs/README.md` — a compact new-engineer reading path and a clear pointer to
  the material retained for the paper.

Further work should improve the existing pages in place rather than creating a
second documentation tree.

## Second pass and verification

The follow-up pass kept the same structure and tightened the entry points:

- `docs/README.md` now distinguishes the authoritative status page from the
  first page for a new engineer, and links the evidence index.
- `docs/quickstart.md` links the evidence index from its navigation table.
- `docs/operations/ordinary-campaign.md` and
  `docs/operations/persistent-execution.md` cross-link the quickstart, glossary
  and troubleshooting pages.
- Root `README.md` points new readers at the quickstart before the detailed
  documentation index.

Checks run locally for this documentation pass:

- A read-only local Markdown check over every changed page found no missing
  files or anchors. The check was an ad hoc script, not a repository tool, so it
  is not presented as a reusable command.
- A repository-wide local link scan found only expected unmatched targets inside
  the quoted upstream excerpts in `evidence/source-excerpts.md` and inside
  archived raw-result snapshots. Those are quoted/archival content, not
  navigation links, and were not rewritten.
- `git diff --check` returned no output.
- `bash scripts/check-preregistration.sh` returned `rc=0`.

No deployment, workload, history export or remote mutation was performed by
this documentation work.
