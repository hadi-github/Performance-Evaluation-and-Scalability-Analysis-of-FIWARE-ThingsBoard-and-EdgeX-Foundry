# Documentation index

**Start here: [`status.md`](status.md)** — the single authoritative status page (claims, evidence, limitations,
resolved contradictions, pending owner inputs).

## Current reading path

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
| `operations/` | ordinary-campaign execution, publication handoff, diagnostics, server-access preflight |
| `development/` | tooling interfaces (benchctl, scripts) |
| `archive/sessions/` | verbatim session narratives (progress logs, handoffs, state snapshots). May contradict each other; `status.md` wins. |

Related roots: [`../evidence/`](../evidence/) (integrity manifests), [`../research/`](../research/)
(preregistration archive — original bytes preserved, protocols, findings), legacy-run policy in
[`../workloads/README.md`](../workloads/README.md).
