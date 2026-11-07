# Bounded ordinary campaign: operator runbook

Read [current status](../status.md) before using any command. This is an
execution handoff, **not an instruction to start traffic now**. Runtime work is
paused for repository preparation. Official attempts require owner authorization,
the actual independent review receipt and fresh preflight checks.

## Frozen comparison

| Item | Setting |
|---|---|
| Platforms | ThingsBoard, FIWARE + QuantumLeap/Timescale, EdgeX |
| Ordinary load | Constant open-loop 25 msg/s; 1,000 devices; payload selector 0 |
| Application resources | N=2/k=2, pool `4-5`; verify actual replicas and limits |
| Workload phases | 300 s warm-up + 600 s measurement |
| Receipt observation | 900 s measurement-relative drain ceiling, 30 s quiet interval at cutoff, separate 30 s late observation |
| Load-host allocation | Generator `0-1`, sink `2`, monitoring `3`; `GOGC=400` |
| Attempts | Five planned per platform, 15 original interleaved entries |
| Primary latency | Per-message application dispatch → first complete-body sink receipt on the same load host |

Authority: [dispatch contract](../methodology/dispatch-contract-20261106.md),
[frozen scope](../../results/feasible-campaign-20261106/frozen-scope.json),
[matrix](../../results/feasible-campaign-20261106/run-matrix.json) and
[operational sequence](../../results/feasible-campaign-20261106/operational-ready.md).
Do not regenerate IDs, seeds, thresholds or plans after seeing results.

## Resume gate: resolve FIWARE repetition 2 first

The local launcher for `disp-fw-r02-20261106` was cancelled by a harness restart.
Later read-only inspection through the SUT found its request/capture/analysis
artifacts on the load host: **local cancellation did not prove remote abortion**.
They have not been recovered and fully validated locally. The load host reports
a later boot at 2026-11-07 16:07:21 UTC; absence of its `/run` lock after reboot
does not prove the original cleanup succeeded. The SUT's FIWARE services were
still running at the last inspection. Recheck everything rather than assuming
that snapshot persists.

1. Verify both authenticated host identities, boots, kernel, disk/memory/swap,
   monitoring and resource allocations. Record the access route.
2. Inventory k6, benchctl, wrappers, samplers, capture listener `18091`, owned
   containers and lock. Do not start another workload while ownership is unclear.
3. Recover entry 4's existing raw artifacts into a new recovery directory and
   retain hashes. Never overwrite original evidence or stage new source over
   the old remote source before preserving its identity.
4. Check frozen plan/source hashes, scenario epochs and actual attempted counts;
   reconcile exact request/sink/history identity sets; inspect capture/clock,
   sampler/resources and history-export status. A remote analysis file alone
   is not acceptance. Export history read-only while FIWARE remains available,
   if necessary, then use the same offline analyzer.
5. Establish before/after resource and image continuity. Missing contemporaneous
   evidence stays unavailable; a post-reboot snapshot cannot reconstruct it.
6. Record the outcome and any recovery separately, verify safe owned-process
   cleanup and volume-preserving service finish, then obtain the applicable
   independent recovery/continuation review.

Do not rerun the validated FIWARE/EdgeX repetition 1 results to obtain progress
logs. Do not replace TB repetition 1 (zero-traffic setup failure). If entry 4 is
unusable, retain it as such; do not silently add a favourable substitute.

## Access when the direct path fails

At the 2026-11-07 check, workstation → load-host TCP/22 timed out, while SUT →
load-host TCP/22 succeeded in about 9 ms over its on-link `/24` route. Both IPs
are in the same advertised `87.107.191.0/24` prefix. This supports a path/source-
dependent failure, **not a demonstrated national filtering diagnosis**. UFW was
inactive and basic iptables INPUT/OUTPUT policies were ACCEPT on the load host;
upstream/provider rules and other filtering were not excluded.

Read-only SSH works using the workstation's existing credentials and strict
host-key verification, without agent forwarding or copying keys to the SUT:

```sh
ssh -J ramonavm2 -o HostName=87.107.191.32 -o HostKeyAlias=ramonavm1 \
  -o StrictHostKeyChecking=yes -o BatchMode=yes ramonavm1 hostname
```

The explicit IP matters: `ramonavm1` is a local alias and was not resolvable on
the jump host. Supply your own approved host/key configuration out of band.
The current launcher still invokes direct SSH/SCP; **jump-host transport is not
yet integrated or tested in that launcher**. Do not launch with a known-broken
control/retrieval path. Restore direct connectivity or separately configure and
test a scoped SSH/SCP transport before execution; preserve host-key checking.
This administrative path must not proxy the measured telemetry/sink traffic.

## Execute one original matrix entry at a time

Only after recovery, transport and release gates pass, choose the next original
entry (currently entry 5, `disp-ex-r02-20261106`), not a new replacement ID.
The launcher builds locally and stages exact bytes with SHA-256 verification.
Review any post-release source change, including diagnostic progress observation,
before accepting those bytes for the remaining campaign.

```sh
# Template only: replace PLATFORM, RUN_ID and /private/review.json after approval.
python3 results/feasible-campaign-20261106/run-feasible-campaign.py launch \
  --platform PLATFORM --run-id RUN_ID \
  --plan results/feasible-campaign-20261106/plans/RUN_ID.json \
  --execute-reviewed --sol-review /private/review.json
```

The launcher activates the selected stack; do not activate twice. It gates
isolation, source hashes, images, resources and readiness, then captures,
exports history, reanalyzes, verifies cleanup and stops owned services while
preserving volumes. Retain each `run-result.json`; inspect the underlying data,
not merely its process exit status. Pause for unresolved setup, provenance,
clock/capture, integrity, resources, history, cleanup or post-run defects.
Valid platform failures remain results and require review before progression.

[Generator progress](../development/generator-progress.md) is diagnostic: HTTP
ACK is not delivery, and 100% completed ticks is not full campaign completion.

## Offline output and reporting

`scripts/post-run-analysis.sh` and `scripts/analyze-dispatch-run.py` produce
JSON/Markdown/CSV dispatch analysis with validity, delivery, history, latency,
clock and resource assessments. Retain the capture window, host proof and
request/sink/history ledgers with source hashes and original exit/status files.
Use each script's `--help`/usage for explicit input paths; missing evidence is
not a zero-valued success. Do not force-overwrite historical outputs to change
an endpoint or verdict.

```sh
# Use a new output directory unless deliberately refreshing derived aggregates.
python3 scripts/aggregate-runs.py /path/to/retained/evidence \
  --out /path/to/new-aggregate
```

Publish every original attempt, the valid measured repetition count, per-run
p99 and min/median/max summaries, plus missing/duplicate/late/UNKNOWN/history
counts. Never pool message-level percentiles across runs. Disclose TB's missing
measured repetition, its qualification's reconstructed provenance, unavailable
phase CPU cost, reused deployments/data and tested-envelope limits. No capacity,
scaling, wire-level timing, exact-phase CPU-cost or automated fault claim follows
from this bounded comparison. Raw publication requires a reviewed external
archive, immutable manifests/checksums and recorded reproduction commands.
