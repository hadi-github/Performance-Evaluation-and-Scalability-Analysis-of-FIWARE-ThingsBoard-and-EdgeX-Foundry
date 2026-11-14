# Finding: `w4dp-tb-r01-20261108` was aborted twice before traffic; two harness defects and an offline load host

**Status:** W4 is **blocked on infrastructure**. No matrix row was consumed. No
measurement was produced. The attempt is preserved, not replaced.
**Date:** 2026-11-14

## What happened

The W4 batch was launched under `owner-w4-runnable-20261114c` for the 11 runnable
cells. On the **first** cell it stopped twice, both times before the generator
started:

| # | Trigger | Result | Truth |
|---|---|---|---|
| 1 | Launcher's remote-evidence guard | `remote evidence already exists for w4dp-tb-r01-20261108; never overwrite a run` | **False.** The remote capture-window file never existed. A transient ssh exit **255** was read as exit **1**. |
| 2 | A workstation shell wrapper (`timeout 300`) | Launcher SIGKILLed mid-setup | The launcher's own budget is 5200 s; the wrapper's 300 s cap killed it. SIGKILL skips the `finally` that releases the load lock. |

Neither event opened a traffic window.

## Zero-traffic proof

| Check | Value |
|---|---|
| `haproxy_backend_http_requests_total{proxy="tb-http-transport"}` | **0** |
| Every `tb-http-transport` backend server | **0** requests |
| `w4dp*` artifacts anywhere on the load host | **0** |
| Remote `*.capture-window.json` | never existed |
| Local `run-result.json` / `launch-ssh.json` / `cleanup-verification.json` | **absent** |

The 2 requests on the `tb-core` backend are the launcher's own readiness probes,
not workload telemetry; none reached the transport ingest path.

Under the campaign's own rule — an attempt is *"consumed exactly once, by exactly
one traffic window"* — `w4dp-tb-r01-20261108` is **unconsumed**.

The precedent is `w1-qual-ex-20261110e`: also killed by external infrastructure
during activation, pre-traffic, with a leaked lock. It received a
`lock-recovery.json` and **the same ID** went on to record a `VALID`
qualification.

## Defect 1 — a transport failure is not "evidence exists"

`test ! -e` over ssh is **three-valued**:

| ssh exit | meaning |
|---:|---|
| 0 | path is absent |
| 1 | path is present |
| 255 | the transport itself failed (DNS, refused, jump-host hiccup, killed connection) |

The launcher tested the raw return code:

```python
if ssh(LOAD, f"test ! -e {REMOTE_RUNS}/{run_id}.capture-window.json", check=False).returncode:
    raise SystemExit(f"remote evidence already exists for {run_id}; never overwrite a run")
```

so `255` and `1` were the same branch. A jump-host hiccup therefore refused a run
that had never started.

**Fixed** with `remote_absent(host, path)`, which returns `True` on 0, `False` on
1, and **raises** on anything else. Both the evidence check and the lock
acquisition use it.

## Defect 2 — an empty evidence directory poisoned the retry

The order was: create the local evidence dir, *then* ask the load host. When the
guard refused (incorrectly), it left an empty `evidence/<run_id>/` behind, and the
relaunch then failed with `refusing existing evidence directory`.

**Fixed** by asking the load host *before* creating any local state.

## Defect 3 — the launch gate checked 3 of 13 bindings

The authorization receipt records 13 tree bindings. `verify_receipt` and
`verify_receipt_for_switch` hardcoded three and **silently ignored the rest**,
including `b0_launcher` — the process that actually executes the workload. A
changed B0 launcher could therefore run under a stale approval.

**Fixed** with `verify_tree_bindings()`, which enforces *every* binding the
receipt records. It now reports `b0_launcher, launcher` drift on the old receipt,
which is exactly what should have happened.

## Not fixed — SIGKILL leaks the load lock

A SIGKILL cannot be trapped. The documented recovery stands: verify ownership
(no owner files, no generator/sink process, nothing on 18091, no run artifacts
for the ID), record a `lock-recovery.json`, then `rmdir` the stale lock. Callers
must not wrap the launcher in a shorter timeout than its own `--timeout`.

## The actual blocker now: the load host is offline

`ramonavm1` / `87.107.191.32` is unreachable from **both** the workstation and the
SUT:

- ping from the SUT: 100 % loss;
- ARP for `.32` on the SUT (`87.107.191.107/24`, same subnet): **INCOMPLETE** —
  the host does not answer at layer 2;
- tcp/22 from the SUT: `No route to host`;
- a full `/24` tcp/22 sweep finds `.66–.115` answering but **`.32` absent**.

So the machine is **powered off or halted**, not merely unrouted. It has no
virtualisation control surface on the SUT (`virsh`/`qm`/`pvesh` all absent), so it
cannot be started from here. With it down, no W4 workload can run — and the leaked
lock cannot even be verified or released.

### The host is unstable, not merely off

At 11:47 UTC the host briefly returned: ICMP answered and tcp/22 accepted the
connection. An SSH session then **authenticated successfully** with
`id_ed25519_vps` and opened a channel — and produced no output for any command,
including `/bin/echo`, and no output on the `sftp` subsystem. Within a few minutes
it was fully down again (100 % ICMP loss, tcp/22 closed), and it has stayed down on
every subsequent poll.

That sequence rules out "the machine is simply switched off, someone will power it
on": sshd came up far enough to complete key exchange, but the session never served
a command. It is a **crash or boot loop**, and the benchmark cannot treat a host
that authenticates and then dies as usable. Any launch attempted during such a
window would die mid-setup and, per the SIGKILL defect above, leak the load lock
again.

The monitor in `/tmp/opencode/load-host-watch.log` records every transition; the
launch is deferred until the host is continuously reachable rather than briefly
pingable.

## Resuming: what "healthy" must mean

The supervisor (`/tmp/opencode/supervise-w4.sh`) gates the launch on a health
predicate that requires an SSH **command to return output**, not just a ping or an
accepted key:

```bash
ssh -F "$SSHCFG" ramonavm1 'hostname; test -d /run && echo RUN_OK'
# healthy only if the output contains the hostname AND RUN_OK
```

It requires **8 consecutive healthy polls one minute apart** before it will launch.
A host that authenticates and then dies cannot pass that. Before launching it also
refuses if `/run/iot-bench-workload.lock` still exists, if any
`benchctl`/`sink`/`run-capture` process is already running, or if the receipt has
drifted from the tree.

Because `/run` is `tmpfs`, a completed reboot clears the leaked lock on its own; the
lock check is there for the case where the host recovered **without** rebooting, in
which case the documented ownership verification and `rmdir` are still required.

## What was preserved

- Staging for the aborted attempt, hash-verified byte-identical before the
  evidence directory was removed:
  `results/workload-batch-20261108/aborted-staging/w4dp-tb-r01-20261108-20261114T1020Z/`
  (7 files).
- Full abort record:
  `results/workload-batch-20261108/blocked/w4dp-tb-r01-20261108.aborted-attempt.json`.
- The four owned stacks were **stopped, volumes preserved** (74 volumes intact).

## What is required to resume

1. Bring `87.107.191.32` back (or supply a replacement load host).
2. Verify and release the stale `/run/iot-bench-workload.lock` with a
   `lock-recovery.json`.
3. Relaunch `w4dp-tb-r01-20261108` under `owner-w4-runnable-20261114d`, which
   re-binds the corrected launcher.

No plan, matrix, gate, rate, replica count or attempt policy was changed.

## Related

- `results/workload-batch-20261108/evidence/w1-qual-ex-20261110e/lock-recovery.json`
- `research/findings/w1-edgex-qualified-20261112.md`
- `research/findings/w4-edgex-inventory-blocks-four-repetitions-20261114.md`
- `docs/operations/troubleshooting.md` ("Load lock is present")
