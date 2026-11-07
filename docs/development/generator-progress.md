# Live generator progress and send report

New `benchctl run` builds automatically observe the **existing** k6 outcome log
for protocol-bound and `dispatch-integrity-v1` plans. No new per-message logging
is added to k6, and no rate, seed, duration, request timing or validity gate changes.
Historical compact/error-only ledgers are not treated as complete send ledgers.

Every 10 seconds, and once after k6 exits, the runner writes:

| File inside the run directory | Purpose |
|---|---|
| `generator-progress.log` | Timestamped live totals; suitable for `tail -F`. |
| `generator-progress.json` | Latest snapshot, atomically replaced, including per-phase totals. |
| `generator-send-report.md` | Small report of completed/planned ticks, ACKs, explicit failures and unknowns, by phase. |

The same progress line is printed to benchctl's stdout. In the campaign capture
wrapper this goes to `<run_id>.wrapper-run.log`. The launcher retrieves the run
directory, including the three progress files, with the remaining evidence.
Raw setup/error messages and payloads are not copied into these reports.

Illustrative progress line (not a measurement):

```text
[generator progress] example state=observing_completed_responses latest-phase=measurement completed=12500/22500 (55.6%) responses=12500 ACK=12498 failure=1 unknown=1 duplicates=0 invalid=0
```

## Watching a future campaign run

On the load host, use the run directory printed by benchctl:

```sh
tail -F /path/to/results/RUN_ID/generator-progress.log
cat /path/to/results/RUN_ID/generator-send-report.md
```

For this campaign, from the workstation **only if direct load-host access has
been restored** (replace `RUN_ID`):

```sh
ssh ramonavm1 'tail -F /root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106/runs/RUN_ID/generator-progress.log'
```

For the verified jump-host route, use:

```sh
ssh -J ramonavm2 -o HostName=87.107.191.32 -o HostKeyAlias=ramonavm1 \
  -o StrictHostKeyChecking=yes -o BatchMode=yes ramonavm1 \
  'tail -F /root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106/runs/RUN_ID/generator-progress.log'
```

See the [runbook](../operations/ordinary-campaign.md) before changing any
launcher transport. These viewing commands do not enable jump transport in the
launcher itself.

Ctrl-C stops the log viewer, **not the workload**. This is read-only observation.

## Interpretation and limitations

- Counts are **observed completed responses**, not immediate dispatch counters:
  requests still in flight or buffered log lines are not yet included.
- Completion percent is unique completed planned ticks / planned ticks, across
  warm-up and measurement. Duplicate response records do not inflate it. For
  thinning/fault plans it is not an estimate of expected accepted sends.
- ACK means the adapter classified the HTTP response as successful. It does
  **not** prove arrival at the sink or storage in history. Offline identity
  reconciliation remains authoritative for delivery and latency.
- `latest_completed_phase` names the most recently logged response's phase, not
  an asserted current scheduler phase. A delayed warm-up response can arrive
  while measurement is running.
- The observer reports timestamps, invalid records and read errors. Its files
  are diagnostics, never inputs to campaign validity gates. Zero observed
  responses alone does not establish zero dispatches.
- `generator_exited` records the actual k6 exit code, including setup failures.
  Neither that state nor 100% means drain, late observation, history export,
  offline analysis or cleanup has finished. Those retain their existing outputs.
- If benchctl is killed or its host restarts, the snapshot may remain stale in
  an observing state: check `updated_utc` and actual processes/artifacts. Do not
  infer that the workload stopped from a stale progress file.
- Observation runs outside k6, but writes to the filesystem/stdout synchronously
  and the runner waits for its final snapshot. Pathological blocked I/O can
  delay finalization; this is not a hard nonblocking or real-time guarantee.

This feature is locally tested only; it is not retroactively present in already
launched runs. The owner-paused campaign, including unresolved FIWARE repetition
2, must be reconciled before any future deployment or launch.
