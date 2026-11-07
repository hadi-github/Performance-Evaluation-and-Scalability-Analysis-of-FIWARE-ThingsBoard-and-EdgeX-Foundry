# Feasible ordinary batch: reviewed operational sequence

**Current-state warning (2026-11-07):** this procedure is deployment-specific,
not fresh authorization. Runtime work is paused for source/documentation
handoff. Before any continuation, recover/reconcile FIWARE repetition 2 and
establish a working reviewed SSH/SCP transport. Review receipts and raw evidence
are held outside Git. Read `docs/status.md`, `docs/operations/ordinary-campaign.md`
and this directory's `README.md`; do not rerun completed attempts or the
historical qualification executor to fill missing packaging dependencies.

This is the switch/verify procedure for `frozen-scope.json` and
`DISPATCH-20261106`. Frozen cells: 25 msg/s in both 300 s warm-up and 600 s
measurement; 1,000 devices; `logical-json-ascii-v1`, size offset 0; N=2; CPU
pool 4-5; five repetitions/platform. Shared calibrated generator settings are
CPU 0-1, sink CPU 2, monitoring CPU 3, `GOGC=400`, keep-alive, VUs 150 and
remote-write enabled. Dispatch workload validity and 2x adapter-specific
headroom are governed by the frozen dispatch contract. This is not a capacity
sweep and not cold-start IID replication.

## Local readiness commands (read-only)

From repository root, after staging the integrated-review branch's tools:

```sh
python3 results/feasible-campaign-20261106/run-feasible-campaign.py check --platform thingsboard
python3 results/feasible-campaign-20261106/run-feasible-campaign.py check --platform fiware
python3 results/feasible-campaign-20261106/run-feasible-campaign.py check --platform edgex
```

Each check asserts no k6 workload is active and port 18091 is unoccupied, then
renders effective config on `ramonavm2` (no `make check`, `.env` creation, pull,
or build). FIWARE uses source `/root/iot-bench-deploy-cand-auto20261027`,
baseline `fiware-baseline/compose.yaml` plus the history-only `fiware-history-candidate`
Timescale 1536 MiB + QL 512 MiB tier. Their effective artifacts are
`inventory/fiware-n2-effective.json`; verify app N=2, pool 4-5, QL/Timescale
images and memory, and total baseline+history 4,608+2,048 MiB. FIWARE source
preserves its existing volumes. EdgeX uses the same remote source's `edgex`
tree; `inventory/edgex-n2-effective.json` records two core-data and two
device-rest replicas plus one 512 MiB `app-http-export` (5,248 MiB platform
service allocation including infra). TB stays on existing
`/root/iot-qual-tb-20261105/deploy/thingsboard/compose.yaml`, project
`tbq20261105`; check asserts exact N=2 app resource inventory.

## After integrated Sol approval only

Every mutating command requires the integrated review receipt explicitly. Do
not pass a made-up receipt. The script's `switch` uses `docker compose stop` on
only the named owned projects (`tbq20261105`, `fiware`, `fiwhc`, `edgex`); it
never removes volumes, clears history, stops exporters, or touches the load
host's unrelated monitoring/Yaade/18090 sink. Activate exactly one platform:

```sh
python3 results/feasible-campaign-20261106/run-feasible-campaign.py switch --execute-reviewed --sol-review <receipt>
python3 results/feasible-campaign-20261106/run-feasible-campaign.py activate --platform fiware --execute-reviewed --sol-review <receipt>
python3 results/feasible-campaign-20261106/run-feasible-campaign.py verify --platform fiware
```

Use `fiware`, `edgex`, or `thingsboard` in turn. FIWARE starts the baseline N=2
stack and only the existing fiwhc Timescale/QL history pair, creates/verifies
the `openiot` normalized-sensor subscription to
`http://87.107.191.107:8668/v2/notify`, and reuses `scripts/outbound.sh fiware`
for sink delivery and the approved 65 s Orion subscription cache refresh.
EdgeX requires `.env` already present, uses its existing
`scripts/provision-then-scale.sh prepare 2 1000 RUN_ID GATE.json` flow for each
distinct run cohort (provision at one replica, scale to N=2, export profile,
validate every replica); the launcher wraps Compose up with `--no-build --pull
never`. This avoids the known metadata panic when adding devices at N>1. TB's
new endpoint qualification is performed separately by the parent qualification
tool; do not rerun N=1. Each platform-specific bounded path/headroom gate must
pass before its ordinary repetitions.

The parent supplies/stages the dispatch-integrity run executor and frozen run
plans separately. Do not launch ordinary work from this file until those bytes
and the analyzer are integrated and reviewed. Runs must be interleaved by
repetition across platform blocks, unique run/device IDs, serially, with the
existing 900 s drain ceiling plus distinct 30 s quiet and 30 s late-observation
attestation. After each run retain sink/request/history evidence, run the
integrated analysis/review, and only then continue. At each between-run switch,
after artifacts are safely retained, use:

```sh
python3 results/feasible-campaign-20261106/run-feasible-campaign.py finish --platform edgex --run-id <run-id> --execute-reviewed --sol-review <receipt>
```

For EdgeX this also runs the helper's post-run panic/restart check. The launcher
refuses mutation without review flags and refuses to act if a k6 container or
port 18091 is occupied. It does not implement automated workload retry or
overwrite. Never use `down -v`, `clean`, pull/build, or clear platform history.

The fixed interleaved identity sequence is generated in `run-matrix.json`, bound
to `frozen-scope.json` by SHA-256. Never reuse an ID. Qualification clearance
alone is not ordinary-batch clearance.

| Order | Platform | Run ID |
|---:|---|---|
| 1 | ThingsBoard | `disp-tb-r01-20261106` |
| 2 | FIWARE | `disp-fw-r01-20261106` |
| 3 | EdgeX | `disp-ex-r01-20261106` |
| 4 | FIWARE | `disp-fw-r02-20261106` |
| 5 | EdgeX | `disp-ex-r02-20261106` |
| 6 | ThingsBoard | `disp-tb-r02-20261106` |
| 7 | EdgeX | `disp-ex-r03-20261106` |
| 8 | ThingsBoard | `disp-tb-r03-20261106` |
| 9 | FIWARE | `disp-fw-r03-20261106` |
| 10 | ThingsBoard | `disp-tb-r04-20261106` |
| 11 | EdgeX | `disp-ex-r04-20261106` |
| 12 | FIWARE | `disp-fw-r04-20261106` |
| 13 | FIWARE | `disp-fw-r05-20261106` |
| 14 | ThingsBoard | `disp-tb-r05-20261106` |
| 15 | EdgeX | `disp-ex-r05-20261106` |

After independent ordinary-batch release, use the matrix's order: `switch`,
then `launch --platform PLATFORM --run-id RUN_ID --plan plans/RUN_ID.json`
with the required review flags and a repository-relative plan path. `launch`
performs activation, readiness/resource checks, capture, automatic offline
analysis, history retrieval/reanalysis and finish. Do not run activation twice.
Retain and inspect `run-result.json` after each attempt; no automatic replacement
of invalid attempts or valid platform failures. Stop for unresolved ownership,
cleanup or instrumentation failures, not merely an unfavorable platform result.
