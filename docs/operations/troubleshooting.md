# Troubleshooting

Read [current status](../status.md) first. A status summary is a navigation aid;
the state file, launcher result and retained evidence are the evidence.

## General rule

Preserve the failed attempt. Do not retry it under the same ID, do not resume an
old controller state, and do not remove volumes or raw evidence to make the next
run look clean. Classify the failure before changing anything.

## EdgeX: device-rest ingest route not initialized before readiness deadline

**Observed form**

```text
device-rest ingest route not initialized before readiness deadline: http://172.21.0.13:59986
```

**Stage reached**

The launcher completed initial provisioning at one replica and accepted devices
on the first `edgex-device-rest` replica, then scaled to N=2 and started the
export service. The readiness gate failed while waiting for the second
device-rest replica's HTTP ingest route.

**What it means**

- The failure occurred before workload launch.
- `run-result.json` should show `workload_launch_attempted: false`.
- There may be no `generator-progress.json`, because the generator never started.
- This is a setup/readiness failure, not a valid platform backlog result.

**What to inspect**

- `launcher-error.json` — complete pre-traffic error and command context.
- `run-result.json` — launch/measurement/cleanup fields.
- `edgex-gate-retrieval.json` — which gate artifacts were retrievable.
- `edgex-gate-statuses/` — per-replica HTTP status evidence.
- `edgex-gate.json.*` — retained inventory, metadata and driver discovery.

Do not invent a measurement or a platform verdict when the workload did not
start. Preserve `NOT_ASSESSED_NO_ANALYSIS` if that is what the result records.

**Safe next action**

Verify ownership of the affected EdgeX project, stop only the owned project
without removing volumes, confirm no load lock or competing generator remains,
record the failure, and create a fresh reviewed continuation for the next
planned ordinal. Do not blind-retry the failed ID.

## No generator-progress file

The progress file is produced after the generator starts. Its absence is normal
for a pre-traffic setup failure and does not prove that the run is still active.
Check `run-result.json` and `launcher-error.json` before making any status claim.

## Controller is PAUSED_REQUIRES_REVIEW

Inspect `controller-state.json`, the controller log, `run-result.json`, active
owned projects and the load-host lock. Do not resume the old state directory.
Create a new reviewed continuation with a fresh state directory after the failed
attempt is preserved.

## History export failed

Preserve the original failure. A later export can establish retrievability at
the later time, but not visibility by the original cutoff. Do not relabel the
run or invent a persistence-latency claim.

## Load lock is present

Do not kill the owner or recursively delete the lock. Identify the exact process,
project and run that own it, then follow the reviewed cleanup procedure. A stale
lock is not permission to start another workload.

## SSH or status path fails

Use the reviewed private SSH configuration, preserve host-key checking and avoid
copying private keys into examples. If the direct workstation path fails, use
the documented SUT-to-load path only after verifying the transport is the
reviewed one; the administrative path must not proxy measured telemetry.

## Related pages

- [Ordinary-campaign runbook](ordinary-campaign.md)
- [Persistent execution](persistent-execution.md)
- [Current status](../status.md)
- [Measurement guide](../methodology.md)
