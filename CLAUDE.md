# iot-bench-deploy — project instructions

These rules are non-negotiable. They override convenience, speed, and any
conflicting prose or subagent instruction. If a rule blocks a task, stop and ask
the owner; do not work around it.

## 1. Active Queue Freeze

Whenever `drive-fixed-point-matrix.py` is running
(`pgrep -f drive-fixed-point-matrix.py`), NEVER edit, format, or commit any file
bound by the active authorization receipt
`workloads/campaigns/fixed-points-w1-w4/authorization/owner-w-matrix3-20261116i.json`.

Run this check before every edit, format, or commit:

```bash
pgrep -af '[d]rive-fixed-point-matrix\.py'          # any output => ACTIVE
tmux ls 2>/dev/null | grep -E '^(wmatrix|wsuper|tbsink):'
pgrep -af '[s]upervise-fixed-point-queue\.py'
```

The bracketed `[d]` form prevents the check from matching its own command line.
Any doubt, unreadable process table, or failing command means ACTIVE.

### Active receipts

- `owner-w-matrix3-20261116i.json` — the receipt named by this file; newest.
- `owner-w-matrix3-20261116h.json` — still named by the running code at
  `scripts/drive-fixed-point-matrix.py:37` and `scripts/queue-watchdog.sh:43`.
  Verified 2026-11-16: both receipts have identical `authorized_run_ids`,
  `bindings_sha256`, and `authorized_plan_sha256`, and both pass `--check`.

Treat the union of both receipts' bound paths as bound. If they ever diverge,
freeze the union, do not choose one silently, and report the difference.

### Bound set

The receipts bind these paths (current contents; re-derive before relying on the
list after any receipt or control-file change):

- `workloads/campaigns/fixed-points-w1-w4/scope.json`
- `workloads/campaigns/fixed-points-w1-w4/run-matrix.json`
- `scripts/run-workload-campaign.py`
- `scripts/aggregate-runs.py`
- `scripts/stage-w1-qualification.py`
- `scripts/stage-fixed-point-variants.py`
- `scripts/dispatch-integrity.py`
- `scripts/pool_policy.py`
- `results/feasible-campaign-20261106/run-feasible-campaign.py`
- every `workloads/campaigns/fixed-points-w1-w4/qualification/**/index.json`
- every plan named in `authorized_plan_sha256` (24 plans)
- both receipt files themselves

If a receipt key is unfamiliar or `--check` fails, freeze all of `scripts/`,
`workloads/campaigns/fixed-points-w1-w4/`, and the B0 launcher until resolved.

### While ACTIVE

- Do not edit, format (`gofmt -w`, `make fmt`), or commit any bound path.
- Do not edit, overwrite, backfill, or "fix" a receipt. A drifted tree gets a new
  receipt from the owner, never a silent edit.
- Do not rewire code from receipt `h` to `i`; defer that to an idle queue.
- Do not run `make clean` or `docker compose down -v`.
- Do not run `make check`, `make test-node`, or other build/test workloads; they
  belong to an idle queue.
- Read-only inspection is allowed: `pgrep`, `tmux ls`/`capture-pane`, `git status`,
  `df`, census, and the receipt `--check` commands below.
- If an edit is requested, report the active receipt and the bound path instead.

### Receipt verification (read-only)

```bash
IOTBENCH_ENV_FILE=/nonexistent python3 scripts/record-w-authorization.py \
  --receipt-id owner-w-matrix3-20261116h --check
IOTBENCH_ENV_FILE=/nonexistent python3 scripts/record-w-authorization.py \
  --receipt-id owner-w-matrix3-20261116i --check
```

A drift result means a bound file changed. Stop and report; never repair it by
editing the receipt. Recording a new receipt is permitted only on an explicit
verbatim owner instruction, with exact run IDs and a new receipt ID.

## 2. Data hygiene

### Ordinal 21 — exclude

`w4dp-tb-r02-20261108` (W4/thingsboard repetition 2) is a **harness defect**:
the ThingsBoard root rule chain was missing the `bench sink` TbRestApiCallNode,
while ThingsBoard itself stored 22,500/22,500 messages. Exclude it from every
platform comparison table, chart, repetition set, and pooled statistic; never
present it as a platform finding.

- Evidence: `results/workload-batch-20261108/evidence/w4dp-tb-r02-20261108/harness-defect.json`
  (`platform_finding: false`, `HARNESS_DEFECT_EXCLUDE_FROM_REPETITIONS`).
- Valid W4/thingsboard repetitions are ordinals **29, 37, 60**
  (`w4dp-tb-r03/r04/r05`). A repetition not yet on disk is pending, not a pass.

### Ordinals 22 and 24 — include

Ordinals 22 (`w1dp-fw-r02-20261108`, W1/fiware) and 24
(`w1dp-tb-r02-20261108`, W1/thingsboard) are genuine platform results and must
stay in their cells as `VALID_PLATFORM_FAILURE_OR_BACKLOG`.

- Ordinal 22: one malformed body in 45,000.
- Ordinal 24: warm-up-only first-contact egress stall; the measurement phase and
  history are clean.
- Never filter either out by `exit == 0`. Use the `VALID_` spelling for this
  batch; older documents' `INVALID_PLATFORM_FAILURE_OR_BACKLOG` is not a filter.

### Validity gate

A run is usable only when all of the following hold:

- `platform_result` is `VALID_PLATFORM_SUCCESS` or
  `VALID_PLATFORM_FAILURE_OR_BACKLOG`;
- `capture_valid` is true and `measurement_valid` is true;
- the workload, generator-resource, SUT-resource, history, and post-run gates
  pass;
- no `harness-defect.json` with `platform_finding: false` applies.

`exit == 0` is not a verdict. Missing or unreadable evidence is `UNAVAILABLE`,
never zero and never a pass.

### Tooling caveats

- `scripts/census-w-matrix.py` is the exclusion-authoritative census.
- `scripts/aggregate-runs.py` does not read `harness-defect.json` and still says
  "five independent repetitions"; do not use it unmodified for a platform
  comparison. Pre-filter ordinal 21 or wait for an authorized fix.
- `scripts/aggregate-runs.py` is receipt-bound: do not edit it while the queue is
  active. The same applies to every path in the bound set.
- Publication output is Markdown and CSV only; there is no LaTeX generator.
- Never pool message-level percentiles across runs; report per-run values and
  min/median/max across eligible runs.

## 3. Evidence preservation

NEVER delete or mutate `results/workload-batch-20261108/evidence/**` or
`results/workload-batch-20261108/driver-log.jsonl` (including its `.pre-repair`
copy) or `supervisor-log.jsonl`. Prior run directories, manifests, ledgers, and
checksums are immutable.

- Missing evidence stays `UNAVAILABLE`; absent is not zero and not success.
- Write derived outputs to a new, uniquely named directory. Never overwrite an
  existing aggregate, report, `summary.md`, or `runs.csv`.
- Never use `--force` against an existing output directory.
- `results/**` is gitignored except narrow unignores. Never stage evidence.

## 4. Build and Test Gate

Always verify Go changes with `make check` and then `make test-node` before
claiming completion; both must exit 0. Report Docker or integration skips as
skips, never as passes.

Run build/test workloads only when the queue is idle. If idle cannot be
established, report the change as unverified instead of running the gate.

For Python or shell changes, run `make test-python` and the relevant static
checks when possible.

## 5. Sub-agents

- `queue-ops` — read-only queue, receipt, tmux, census, and host-health checks.
- `data-analyst` — evidence-only post-processing, exclusions, aggregates,
  Markdown/CSV charts and tables.
- `go-architect` — Go decomposition and refactors, only on an idle queue, with
  test parity.

Every agent reads this file first. When instructions conflict, the stricter rule
wins.

## 6. Pre-change checklist

1. `pgrep -af '[d]rive-fixed-point-matrix\.py'` and the tmux/supervisor checks.
2. Run both receipt `--check` commands.
3. Confirm the target path is not in the bound set.
4. Make the change; if it is Go, wait for idle then run `make check` and
   `make test-node`.
5. Re-run both `--check` commands. Any drift means stop and report.
