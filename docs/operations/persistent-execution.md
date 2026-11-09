# Disconnect-safe ordinary execution

The owner authorized a tmux controller, a small persistence probe, then the
remaining original ordinary entries. The controller/probe are **not** automatic
retry or boot-resume services. This note supplements the dated campaign status
and preserves the earlier publication handoff as historical context.

Related navigation: [current status](../status.md),
[ordinary-campaign runbook](ordinary-campaign.md),
[troubleshooting](troubleshooting.md) and [quickstart](../quickstart.md).

## Recovered attempt and diagnostic proof

The original `disp-fw-r02-20261106` completed remotely. Independent raw-artifact
review accepted all 22,500 request/sink/history identities and measurement
dispatch p99 **122.3681640625 ms**. History was exported at recovery, not proven
visible by the original cutoff. Owned services were stopped and cleanup verified
at recovery; later reboot/lock absence is not historical cleanup proof. The
original failed history assessment and all raw artifacts remain preserved.

The separate `probe-detach-20261108a1` diagnostic sent **625 messages** to a
load-host loopback fixture: 25/s, 5 s warm-up + 20 s measurement, ten device
identities, payload 0, fixed 150 VUs and `GOGC=400`. No IoT platform traffic,
provisioning, qualification or extra ordinary repetition was performed.

The launching SSH connection closed at observed **2026-11-07 21:09:08 UTC**.
A new connection at **21:09:26 UTC** witnessed tmux and k6 active with **208/625**
ACKs. The wrapper exited **0**, all 625 request/receiver identities reconciled,
dispatch integrity passed and no fixture listener/k6 container remained.
Run-ID dates are labels, not substitutes for recorded timestamps.

Original probe output retains `reviewed_for_continuation=false`. A separate
reviewed receipt is required before ordinary entry 5. Probe artifacts and review
receipts are private evidence under
`results/feasible-campaign-20261106/persistence-probe-20261108a1/`, not Git data.

## Persistent controller

`scripts/continue-ordinary-campaign.py` runs inside tmux on the **SUT**, pinned to
CPU 0 outside the application pool. It uses the frozen original matrix starting
at entry 5, `disp-ex-r02-20261106`, through entry 15. Every next cell requires a
successful launcher result, explicit empty fetch-error list, qualified workload,
capture/clock/resources/history, verified cleanup and released workload lock.
Any failure pauses for review; no automatic replacement or resumed attempt ID.

The release binds the controller/launcher, frozen scope/matrix/remaining plans,
staged measurement scripts and k6 sources, source-bound prebuilt binaries and
reviewed probe receipt. Inputs are checked before the first stack mutation and
between cells. Go need not be installed on benchmark hosts: prebuilt tools are
compiled on the workstation and verified against the exact Go source and binary
hashes before staging or execution. No k6 image replacement is implied.

The supervisor writes atomic `controller-state.json`, including PID, host boot
ID, timestamps, current run and completed IDs. A disconnect does not kill tmux;
a reboot does. A stale RUNNING file is not liveness or successful cleanup.

Private controller root:
`/root/iot-feasible-controller-20261107-1810` on `ramonavm2`.
First controller session: `iot-feasible-ordinary-20261108`, started at observed
2026-11-07 21:22 UTC after the final conditional release. PID 4078364 was
independently reconnected to and its CPU affinity verified as `0`. Treat these
as recorded startup observations; check live state/process identity rather than
assuming the original PID remains active forever.
The first controller paused on a platform result and was preserved. After
independent review, a new session `iot-feasible-ordinary-r07-20261108` continues
from original entry 7 using `state-r07/` and `controller-r07.log`. Consult current
[status](../status.md) before attaching or continuing. A missing tmux session
can mean the controller exited normally or paused after failure. Read state,
log and exit code; do not recreate it as an automatic attempt retry.

```sh
# View status; does not stop or start traffic.
ssh ramonavm2 'cat /root/iot-feasible-controller-20261107-1810/state-r07/controller-state.json'

# Attach while running; detach using Ctrl-b, then d, NOT Ctrl-C.
ssh -t ramonavm2 'tmux attach -t iot-feasible-ordinary-r07-20261108'
```

The root controller log and state survive session exit. Per-message generator
progress lives on the load host inside each run's directory; see the
[progress guide](../development/generator-progress.md). HTTP ACK is not confirmed
sink/history delivery. Aggregate summaries never pool per-message percentiles.

To request a pause **after the active cell finishes**, create the controller's
active state directory's `STOP_AFTER_CURRENT` file (`state-r07/` for the current
controller). This is an operator mutation, not a view command;
it does not immediately stop traffic. Do not kill tmux or reboot as a substitute
for owned workload cleanup. Removing the stop file does not automatically resume
an exited controller; reconcile state and obtain the applicable review first.

## SSH authority and cleanup

All SSH/SCP operations share `IOT_BENCH_SSH_CONFIG`, strict previously trusted
host keys and explicit identities. The controller uses native SUT → load access
and SUT loopback access; measured telemetry is not proxied through SSH.

A newly generated dedicated **root-capable** credential is held only in the
private controller directory. Source restrictions allow it from the SUT to the
load host and loopback to the SUT; forwarding/agent/PTY features are disabled.
These restrictions **do not constrain root commands**. The original general-
purpose workstation private key was not copied. No private key/config belongs
in Git or the public evidence archive. Revoke this specific dedicated credential
after the campaign and final evidence retrieval; never remove unrelated keys.

Local regression coverage: `tests/persistent_campaign_test.py` protects shared
transport, prebuilt source/binary validation before mutation, receipt/source/plan
binding, serial success, stop files, failure pause, no reuse and strict fetch/
cleanup gates. `tests/aggregate_runs_test.py` now selects a unique canonical
analysis over nested preserved versions and fails closed on ambiguity.

This machinery does not validate unresolved results by itself or expand the
bounded claims. TB repetition 1 remains an unreplaced zero-traffic setup failure;
phase CPU cost, capacity/scalability and automated fault claims remain excluded.
