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

## SSH or status path fails

Use the reviewed private SSH configuration, preserve host-key checking and avoid
copying private keys into examples. If the direct workstation path fails, use
the documented SUT-to-load path only after verifying the transport is the
reviewed one; the administrative path must not proxy measured telemetry.

### A reachable port is not a usable host

The load host has been observed to answer ICMP and **accept an SSH key**, open a
session, and then return no output for any command (including `/bin/echo`) before
going down again. Do not treat an open tcp/22 or a completed authentication as
health, and never start a multi-minute workload inside such a window: it dies
mid-setup and, because a killed launcher cannot release it, leaves the load lock
behind.

Require evidence that a command **actually ran**:

```bash
ssh -F "$IOT_BENCH_SSH_CONFIG" ramonavm1 'hostname; test -d /run && echo RUN_OK'
```

Wait for several consecutive successes spaced by a minute, not one success. If the
host came back **without** rebooting, treat any existing
`/run/iot-bench-workload.lock` as owned until proven otherwise (see above); if it
rebooted, `/run` is `tmpfs` and the lock is gone with it.

## Load lock is present

Do not kill the owner or recursively delete the lock. Identify the exact process,
project and run that own it, then follow the reviewed cleanup procedure. A stale
lock is not permission to start another workload.

A launcher killed by `SIGKILL` cannot release this lock, so it is expected after a
workstation or host crash. Verify no owner exists — no owner file in the lock
directory, no `benchctl`/`sink`/`run-capture` process, nothing listening on 18091,
and no run artifacts for the run ID — record a `lock-recovery.json`, then `rmdir`
the lock. Never `rm -rf` it. Precedent:
`results/workload-batch-20261108/evidence/w1-qual-ex-20261110e/lock-recovery.json`.

## Related pages

- [Ordinary-campaign runbook](ordinary-campaign.md)
- [Persistent execution](persistent-execution.md)
- [Current status](../status.md)
- [Measurement guide](../methodology.md)
