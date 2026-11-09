# Documentation index

**Authoritative status: [`status.md`](status.md)** — claims, evidence,
limitations, resolved contradictions and pending owner inputs. Treat it as the
source of truth whenever a summary disagrees with it.

**New to the project? Start with [`quickstart.md`](quickstart.md)** for the short
safe orientation, then come back here for the detailed tree.

## New engineer path

For a compact route through the repository, read:

1. [Quickstart](quickstart.md) — what the project is and the safe local checks.
2. [Glossary](glossary.md) — shared terms and canonical references.
3. [System design](architecture.md) — the two-host design and platform paths.
4. [Measurement and interpretation](methodology.md) — what the results mean.
5. [Current status](status.md) — what is actually complete and supported.
6. [Ordinary-campaign runbook](operations/ordinary-campaign.md) and
   [persistent execution](operations/persistent-execution.md) — before touching
   a controller.
7. [Troubleshooting](operations/troubleshooting.md) — known failure classes and
   safe next actions.
8. [Evidence index](../evidence/README.md) — how raw evidence, manifests and
   checksums relate to the tracked repository.

Do not start with `archive/sessions/`, qualification decision packets or frozen
protocols. Those are historical and paper/proposal material, not onboarding.

## Material retained for the paper

The research guide, methodology, architecture references, frozen protocols,
qualification records, status history and archived sessions are retained
deliberately. They support the experimental record and later paper writing; do
not delete or rewrite them merely to shorten the onboarding path.

## Understand the design

Start with these consolidated explanations before the detailed references:

- [System design](architecture.md): two-host architecture, shared CPU budgets,
  platform paths, adapters and the reasons behind the design choices.
- [Measurement and interpretation](methodology.md): timing endpoints, identity
  accounting, validity versus platform failure, percentiles and evidence limits.
  Its [workload configuration section](methodology.md#8-workload-configurations-b0-and-the-separate-w1w4-batch)
  explains B0 and the separately staged W1–W4 fixed-point comparisons.
- [Research study guide](research-guide.md): research questions, experimental
  design, analysis, claim-to-evidence mapping and publication limitations.

These guides summarize existing contracts; they do not change thresholds,
authorize execution or replace the source documents. Core design records,
methodology contracts, protocols and test references are preserved. Historical
documents must be read with their dates and later amendments in mind.

## Operate, develop and verify

1. [Publication and repository handoff](operations/publication-handoff.md): what
   can be committed/published, remaining owner metadata and verification commands.
2. [Ordinary-campaign runbook](operations/ordinary-campaign.md): exact scope,
   interrupted-attempt recovery, serial execution and evidence/cleanup gates.
3. [Dispatch contract](methodology/dispatch-contract-20261106.md): primary
   endpoint, prospective integrity policy and scientific exclusions.
4. [Generator progress](development/generator-progress.md): live counts and the
   distinction between HTTP ACKs and confirmed delivery.
5. [Tooling interfaces](development/tooling-interfaces.md) and
   [offline analysis](development/offline-analysis.md): inputs, outputs and limits.
6. [Test catalogue](development/testing.md): regression files, commands,
   mocked/local-Docker boundaries and interpretation of a pass or skip.
7. [Persistent execution](operations/persistent-execution.md): the detached
   tmux controller, state files and continuation rules.
8. [Troubleshooting](operations/troubleshooting.md): known failure classes and
   the safe next action for each.

The dated 2026-11-05 readiness checklist and old NO-GO statements document earlier
scopes. They do not override the later bounded release, and that release does not
qualify the broader capacity/scaling/fault programme. Raw evidence is external to
Git; links into ignored result bundles are provenance references, not downloads
included in a clone. The [campaign control directory](../results/feasible-campaign-20261106/README.md)
explains the source/evidence boundary.

| Directory | Contents |
|---|---|
| `architecture/` | design decisions, the FIWARE history (QuantumLeap/TimescaleDB) design |
| `methodology/` | measurement methodology and its reconciliation, metrics dictionary, calibration, feasibility analyses, generator docs, the 2026-10-29 interpretation correction |
| `qualification/` | qualification blockers (B1–B17), ThingsBoard D28 verification, EdgeX disk/WAL (D29a), Step-5 decision packet and owner sign-off |
| `operations/` | ordinary-campaign execution, persistent controller, troubleshooting, publication handoff, diagnostics, review/export and server-access preflight |
| `development/` | tooling interfaces, offline analysis, test catalogue, generator progress and deployment notes |
| `archive/sessions/` | verbatim session narratives (progress logs, handoffs, state snapshots). May contradict each other; `status.md` wins. |

Related roots: [`../evidence/`](../evidence/) (integrity manifests), [`../research/`](../research/)
(preregistration archive — original bytes preserved, protocols, findings), legacy-run policy in
[`../workloads/README.md`](../workloads/README.md).
