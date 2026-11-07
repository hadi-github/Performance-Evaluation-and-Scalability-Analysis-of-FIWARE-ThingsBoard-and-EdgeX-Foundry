# Frozen bounded campaign controls

This directory intentionally mixes **versioned control files** with **ignored
local evidence**. Only the launcher/source, frozen scope/matrix/plans, plan
generator and operational documentation belong in the public Git repository.
The allowlist is in the root `.gitignore`.

- [Current project status](../../docs/status.md) is the public handoff.
- [Frozen scope](frozen-scope.json), [matrix](run-matrix.json), and `plans/` define
  the 15 original attempts. Do not regenerate or change them to replace failures.
- [Operational sequence](operational-ready.md) and [chronological plan](PLAN.md)
  document the deployment-specific runner; historical checkpoints are not fresh
  authorization or guarantees about current remote state.
- `run-feasible-campaign.py` requires the original hosts, existing deployments,
  retained data, locally held independent review receipts and owner approval.
  It is **not** a one-command installation on an arbitrary server.

Not shipped in Git: `evidence/`, `endpoint-qualification/`, `inventory/`,
`aggregate/`, mutable `campaign-progress.json`, review receipts, machine-state
snapshots, logs, binaries and locks. References to those paths in the status and
plan identify externally retained evidence; a Git clone alone cannot verify
their scientific claims. Archive publication must provide a reviewed,
checksummed bundle and stable location before claiming reproducibility.

No run is launched by cloning or running the offline tests. Never use `clean`,
`down -v`, data resets, image replacement or automatic replacement repetitions
to continue this campaign. See the handoff before any official execution.
