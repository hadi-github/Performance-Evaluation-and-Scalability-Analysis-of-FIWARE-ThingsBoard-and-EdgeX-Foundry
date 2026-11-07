# Feasible campaign execution plan — 2026-11-06

**Status:** owner-approved. Primary latency stays in the offline per-message join; Grafana support for it is optional. This plan must not be re-scoped after seeing campaign outcomes.

**Current handoff (2026-11-07):** runtime work is paused at owner request for
repository/documentation commits. Direct load-host SSH fails, but strict-key
jump access via the SUT works; that route is not integrated into the launcher.
Entry 4's remote artifacts exist but its outcome/continuity/cleanup remain
unreconciled after a later load-host reboot. No automatic retry or new workload.
Use `docs/status.md` and `docs/operations/ordinary-campaign.md` for the current
continuation gates; the checkpoints below retain their chronological context.

**Scope:** bounded ordinary batch — 25 msg/s, 1,000 devices, canonical payload selector 0, N=2, application pool `4-5` (k=2), 300 s warm-up, 600 s measurement, five independent repetitions per platform (ThingsBoard, FIWARE, EdgeX), `GOGC=400`, required monitoring enabled, open-loop constant arrivals. Not a capacity, maximum-throughput or scalability study. Manual replica stops remain a separate operator batch.

## Priority 0 — required before any campaign launch

### P0.1 Automatic offline primary analysis

Add `scripts/post-run-analysis.sh RUN_ID RESULTS_DIR PLATFORM`:

1. Refuse missing run dir, `resolved.json`, `k6-summary.json`, `manifest.json`, `records.ndjson`, or capture window.
2. Export history while the platform is still up:
   - ThingsBoard: `TB_PROJECT=tbq20261105`
   - FIWARE: `FWHC_CONTAINER=fiwhc-timescale-1`
   - EdgeX: `edgex-database-1`
   - Failure is recorded; never fabricate an empty success.
3. Write load-host evidence (`hostname`, boot ID, kernel, sink PID/start ticks, k6 container IDs).
4. Invoke `scripts/analyze-dispatch-run.py` with run, sink, window, history, clock samples, host evidence, resource evidence, sampler status, gen CPU; emit JSON/MD/CSV and an analysis status file.
5. Missing clock/sampler evidence must produce `latency_assessed=false`, not a crash or fabricated latency.
6. Idempotent: refuse overwrite unless explicit resume/force.

**Verification:** fixture run produces per-message dispatch p99; missing window fails; missing history is recorded; missing clock yields not-assessed latency.

### P0.2 Generic capture wrapper for all platforms

Add `scripts/run-capture.sh PLAN RUN_ID RESULTS_DIR PLATFORM`:

- starts `generator-diagnostic-sampler.py` and waits for readiness;
- runs `run-protocol-capture.sh` with `SINK_PORT=18091`, `GENERATOR_CPUSET=0-1`;
- forwards calibrated `GOGC=400`, Prometheus remote write, optional Grafana annotation and CPU snapshot command;
- waits for the sampler, then runs `post-run-analysis.sh` even when capture exits non-zero;
- preserves all artifacts and cancellation cleanup.

Keep the ThingsBoard-specific `run-generator-diagnostic.sh` for its existing diagnostic path.

**Verification:** shell test with a fake benchctl/sink proves flags and post-run analysis are invoked.

### P0.3 Batch launch action

Implement `launch` in `results/feasible-campaign-20261106/run-feasible-campaign.py`:

- require `--execute-reviewed --sol-review`, platform, run ID and plan;
- local exclusive lock and refusal of existing evidence;
- read-only check + activate platform;
- stage exact reviewed bytes with SHA-256 verification;
- run `run-capture.sh` under a timeout longer than the wrapper watchdog;
- fetch all artifacts, then `finish` (stop owned stacks, volumes preserved);
- write `run-result.json` with platform, plan/hash, policy, analysis path and validity status.

**Decision:** commit the reviewed measurement-contract milestone, or stage exact reviewed bytes as `qualify-endpoint.py` does. `deploy-generator.sh` refuses a dirty tree.

### P0.4 Sink counter attribution

Distinguish capture failure from platform behavior:

- `overflow_total > 0` → capture loss, measurement invalid.
- `oversized_body_total > 0` → bounded sink limit exceeded, measurement unqualified.
- `body_read_error_total > 0` → incomplete/truncated delivery; report as platform/transport truncation, no receipt, conditional latency only.
- `unparsed_total - incomplete_body_total > 0` → platform malformed/empty body; valid platform correctness failure.
- Never let unfavorable platform behavior become automatic measurement invalidity.

**Verification:** fixtures for overflow (invalid), malformed platform body (valid platform failure), truncated body (conditional), and healthy capture.

### P0.5 One bounded end-to-end acceptance

Run the TB 50 msg/s, 30 s warm-up + 120 s measurement qualification with the new path. Accept only if:

- 1:1 planned/request records, zero dropped iterations;
- sink counters reconcile, history exported;
- dispatch-to-receipt p50/p95/p99 emitted per phase;
- invalid vs valid-platform-failure classification is correct;
- export artifacts are produced if the plotting dependency is available.

## Priority 1 — required for the 15-run batch

### P1.1 FIWARE parity

- `activate fiware` starts `fiware-baseline` + `fiwhc`; verify QL subscription posts to `:18091/fiware`.
- `history-ids.sh fiware` reads `fiwhc-timescale-1`.
- Separate 50 msg/s headroom qualification for FIWARE; TB evidence does not transfer.

### P1.2 EdgeX parity

- `activate edgex` provisions all intended run-ID devices at N=1, then scales to N=2.
- Verify app export posts to `:18091/edgex`; history reads `edgex-database-1`.
- Separate 50 msg/s headroom qualification; `finish` retains the provision gate result.

### P1.3 Repetition aggregation

Add `scripts/aggregate-runs.py`:

- consumes per-run `dispatch-analysis.json` + `run-result.json`;
- emits `runs.csv` and `summary.md` with per-run dispatch p99, received/scheduled, missing/late/duplicate/unknown, history missing, workload-integrity checks and resource qualification;
- reports min/median/max across five runs per platform;
- never pools message-level percentiles; invalid attempts stay in the matrix.

Run after each platform block.

## Priority 2 — optional, must not delay the campaign

### P2.1 Sink scrape and panels

Live Prometheus is running a stale configuration (last reload 22 October); the on-disk config has the sink job but the runtime does not. Campaign sinks use `18091`, the job targets `18090`.

- add `127.0.0.1:18091` (keep `18090` if needed);
- sync config/rules, `scripts/monitoring.sh check`, then `reload`;
- verify sink target `up` and add received/duplicates/overflow/unparsed panels (counters only).

### P2.2 Chart export

`matplotlib` is not installed on the load host or workstation. Create a pinned venv or run export on a machine with it and a Prometheus tunnel. Use `scripts/export-run.py` + `verify-run-export.py`; derive the run window from `k6-summary.json`/`resolved.json`, never from the run name.

### P2.3 Dashboard coverage and documentation

Optional generator memory/GOGC/OOM panels (sampler JSON, not Prometheus), phase annotations, and removal of “live acceptance pending” after P0.5.

## Execution order

1. P0.1, P0.4 (analysis + attribution).
2. P0.2, P0.3 (capture + launch).
3. P0.5 (TB acceptance).
4. P1.1, P1.2 (platform parity + headroom).
5. 15 ordinary runs with automatic analysis.
6. P1.3 aggregation/report per platform block.
7. P2 in parallel or after the batch.

## Authority rules

- Authoritative verdicts and percentiles come from raw records and the offline join, never from Grafana samples.
- Missing remains missing; late receipts are not rewritten as on-time; duplicate receipts are retained.
- Charts are diagnostic. Windowed percentiles are never averaged.
- Historical verdicts and the old scheduled-to-receipt interpretation remain unchanged.
- No hardware upgrade, replacement framework, maximum-capacity claim or automated fault injection.

## Execution checkpoint

### Verification cadence (owner update)

For small fixes, use focused unit tests, offline reanalysis of existing captures,
or a short isolated live probe. Do not repeat a full qualification solely to
verify a small change. Frozen ordinary repetitions remain 300 s warm-up + 600 s
measurement; short probes are diagnostics, never campaign repetitions.

- TB 50/s qualification captured: `endpoint-qualification/`. Offline assessment
  is in `endpoint-qualification/runs/tb-dispatch-endpoint-qual-20261106.dispatch-analysis.json`.
  All 7,500 dispatched identities reached sink and history. Measurement cohort:
  6,000/6,000 received, dispatch-to-complete-body p99 457.039 ms. Workload,
  capture, clock coverage and generator resources pass the implemented checks.
- Acceptance automation initially failed on missing host evidence. Analysis was
  recovered using `endpoint-qualification/reconstructed-hosts.json`, explicitly
  marked as reconstructed from retained pre-run hostname/boot ID evidence and a
  matching post-run host check. Original failure status remains preserved.
  This is not proof that the revised generic wrapper has passed live acceptance.
- Generic capture now records owned sink PID/start ticks and load-host identity
  while the sink is alive; offline analysis uses that evidence on retrieval.
- Load host cannot SSH to SUT. Workstation-side history export and reanalysis
  remain required before finishing/stopping a platform. Export failure is not
  reinterpreted as all history messages missing.
- Prometheus runtime now includes ports 18090/18091 and FIWARE history-tier
  labels. Port 18091 being DOWN between runs is expected.
- FIWARE rendered frozen configuration passes; services are stopped. Setup now
  verifies/updates an existing sink subscription to the requested URL and rejects
  ambiguous duplicate subscriptions. Separate FIWARE/EdgeX headroom runs and
  the 15 ordinary campaign runs are still pending.
- Launcher failure-path hardening and tests are in progress. Do not describe
  helper/unit tests as live launch acceptance or invent a Sol review receipt.
- The bounded FIWARE qualification was started through the revised launcher
  after the self-review recorded in `launcher-review.md`. Inspect its run result
  before ordinary FIWARE traffic. The self-review is not an independent review.
- Optional workstation plotting setup attempted in `/tmp/opencode/iot-bench-plots`:
  matplotlib installation failed because the package index supplied no suitable
  Pillow distribution. No change to campaign validity or launch dependencies;
  chart export remains deferred.
- The original FIWARE 50/s qualification completed as
  `VALID_PLATFORM_FAILURE_OR_BACKLOG`: all 7,500 HTTP requests returned 400,
  sink/history both empty. Capture, workload and generator resources passed;
  there is no observed latency percentile for this attempt.
- Isolated probes identified `|d|` (bare empty padding) as an UltraLight parse
  error. `|d|""` returns 200 and stores the actual empty Text value; evidence is
  in `fiware-padding-probe.json` and `fiware-empty-encoding-probe.json`.
  The explicit FIWARE adapter now quotes only empty padding; logical canonical
  bytes/selector zero and legacy/nonempty encoding remain unchanged.
- Follow-up qualification `fw-dispatch-endpoint-qual-20261106-encodingfix1`
  was started with the same 50/s plan. It is separately retained, not replacement
  of the failed attempt. Neither qualification enters ordinary replicate
  statistics. Current aggregate: `aggregate/summary.md`.
- FIWARE encoding-fix follow-up: all 7,500 requests acknowledged and received at
  the sink; measurement dispatch p99 597.148 ms. History has 6,766 unique IDs,
  734 missing IDs and 6,007 duplicate extra rows. A later read-only export agrees.
  Resource qualification is incomplete because one active sampler row has no
  cgroup data. No full rerun was performed for the analyzer null-handling fix.
- Recovered analysis is retained alongside the original automatic failure;
  `evidence/fw-dispatch-endpoint-qual-20261106-encodingfix1/recovery-result.json`
  records the history/resource limitations. Owned FIWARE services were stopped
  with volumes preserved and its retained workload lock was released after
  recovery. Ordinary FIWARE qualification is **not approved** yet.
- Final short FIWARE verification, after removing the duplicate active history
  route and correcting PID-0 lifecycle handling, passed end to end:
  `evidence/fw-dispatch-endpoint-qual-20261106-finalshort1/run-result.json`.
  500 warm-up + 1,500 measurement messages: all acknowledged, at sink and in
  history, zero missing/duplicates. Workload/capture/clock/resource checks pass;
  measurement p99 737.803 ms. Automatic history reanalysis, platform finish and
  workload-lock release pass. This short check verifies the repairs, not the
  frozen 300/600 s ordinary repetitions or a new full-duration headroom run.
- EdgeX final verification is still pending. Pretraffic finalshort1 failed at
  replica discovery (900/1,000 on replica 2). The legacy remote helper contained
  a destructive `down -v` and was not executed; the safe local helper is staged
  under a new verified hash-derived name instead.
- Common-config discovery limit now reports 8,192 in both metadata/device-rest
  live APIs (`edgex-discovery-effective.json`), preserving existing cohorts.
  Private SERVICE_MAXRESULTCOUNT was ineffective; only the common bootstrapper
  ALL_SERVICES override is used. Images and hard resource limits are unchanged.
- EdgeX finalshort2 failed during SDK startup/provisioning. Finalshort3 failed
  a prematurely started gate, but a later isolated check of that same replica
  and cohort returned 1,000/1,000 HTTP 200. These setup failures remain retained;
  they are not campaign traffic or successful qualifications.
- The readiness helper now retains individual response codes and permits at
  most three bounded readiness checks before declaring setup failure. Every
  replica must still accept all 1,000 devices before workload traffic. The
  separately retained finalshort4 check is running with the same short plan.
- The discovery limit must cover all retained cohorts before the ordinary
  batch; do not delete historical devices to fit the limit. Additional diagnostic
  cohorts count toward this inventory.
- EdgeX finalshort4 passed both all-device gates, then the launcher exposed a
  wrong NATS monitoring URL. The correct check is container-local port 8222,
  verified live; no host port publication was added. Its passed setup was
  resumed without provisioning a replacement cohort, preserving
  `pretraffic-run-result.json` and rechecking staged hashes and metadata state.
- Resumed finalshort4 delivered/stored all 2,000 workload identities, zero
  missing/duplicates, with measurement p99 9.128 ms and qualified resources.
  Its conservative failure verdict is retained for 16 additional unparsed sink
  requests from the adapter's negative-sequence setup probes.
- Fully preprovisioned EdgeX runs now receive a run-ID-bound gate flag, still
  verify every device exists, and omit only the redundant 16 k6 setup probes.
  Legacy mode and malformed-body checks remain unchanged. Four offline-stub
  setup-contract checks pass. Separate finalshort5 is verifying the clean path.
- Latest final software regression: production Go, integration and offline Go
  suites pass; 63 Python tests pass with one optional plotting test skipped;
  eight additional offline Python tests pass; native-payload vectors and EdgeX
  preprovision setup checks pass. Optional chart environment remains blocked
  by unavailable Pillow distribution; it is not part of primary analysis.
- Finalshort5 stopped before traffic: cold-start device-rest repeatedly panics
  in the pinned device-sdk-go v4.0.2 `profileCache.ForName` callback, while
  metadata device validation times out. Logs are retained under
  `evidence/ex-dispatch-endpoint-qual-20261106-finalshort5/`.
  No new workload or favorable replacement result was produced. The prior
  2,000-message EdgeX workload remains valid evidence, with its setup-probe
  anomaly verdict unchanged; cold-start/setup reliability is **not fixed**.
- After saving logs, only the named benchmark projects were stopped to end the
  restart loop; volumes and all retained devices/history remain intact. No
  further retry cohort should be added merely to work around this failure. Investigate a
  reproducible deployment-only startup workaround before another final test;
  do not silently patch/upgrade the pinned platform image or reset its data.
- Overall final acceptance is **NOT GREEN** while EdgeX startup remains
  unreliable. The 15 ordinary campaign runs have not begun. FIWARE short final
  acceptance and the software regressions pass; plotting remains optional.
- At the owner's explicit request, finalshort6 retried the unchanged short
  EdgeX plan once. It again failed before workload traffic: batch-0 device
  creation timed out, with a device-rest startup panic and metadata validation
  timeouts captured in that attempt's startup logs. The original results remain
  preserved. Named benchmark services were stopped afterward without deleting
  volumes. Final acceptance remains **NOT GREEN**.
- Finalshort6 regression checks: Go production/integration/offline tests pass;
  Python 63 passed + one optional plotting skip; eight offline Python tests,
  four EdgeX setup-contract checks and all 12 native-payload vectors pass.

## Latest repair and verified outcome

The cold-start crash was traced to the pinned SDK source: ping and metadata
callbacks start before profile-cache initialization completes. The helper now
waits for the POST-only ingest route to be registered (non-mutating GET returns
405), with a bounded deadline, before provisioning and before replica gates.
This replaces the unsafe ping-plus-fixed-sleep assumption. No SDK patch or
image upgrade was needed.

Finalshort7 verified this startup repair and stored all 2,000 messages, but
exported none. The separate exporter lifecycle bug was then identified:
plain Compose stop omitted the export-profile service, retaining an old NATS
connection across cold starts. Setup now explicitly stops/restarts that exporter;
finish/switch include the export profile. Earlier failures remain unchanged.

**Finalshort8 passes the complete short cold-start/end-to-end check.**
Evidence: `evidence/ex-dispatch-endpoint-qual-20261106-finalshort8/run-result.json`.

- Same 1,000 devices, N=2, pool 4-5, payload selector 0, 50/s short diagnostic.
- 500 warm-up + 1,500 measurement: all acknowledged, at sink and in history.
- Zero missing/duplicate workload identities, malformed/truncated/oversized
  bodies, late receipts, metadata panics or metadata restarts.
- Measurement dispatch-to-complete-body p99: 8.831 ms (conditional on receipts;
  all 1,500 measurement messages were received).
- Workload, capture, clock and generator-resource checks pass.
- Automatic history export/reanalysis, gate retrieval, platform finish and
  workload-lock release pass. A final live check confirms no running EdgeX
  services (including exporter), no k6, no campaign sink and no workload lock.
- Final Python regression: 68 passed, one optional plotting skip; eight offline
  Python tests and four EdgeX setup-contract checks pass. Production/integration/
  offline Go suites passed before these shell-orchestration-only repairs.

This closes the diagnosed EdgeX startup/export issues for the tested short
scope. It is not evidence that the 15 frozen ordinary repetitions have run or
that maximum capacity/scalability was established. Optional plotting remains
deferred. Check retained device inventory against the discovery limit before
  the ordinary batch; historical devices and volumes must not be deleted.

## Independent review follow-up (ordinary batch still paused)

The owner authorized an independent reviewer agent. Actual review record:
`independent-review-findings.json`, session `ses_e4df139d2ffezK66fc5LL133Hx`.
Its initial verdict was NOT READY, not authorization for ordinary traffic.

Repairs now under follow-up review:

- Duplicate-only sink defects yield valid platform failure while keeping
  first-receipt percentiles and complete raw accounting.
- Missing capture-time host provenance is explicitly unassessed. Offline
  analysis no longer invents generator/sink boot IDs from its current machine.
  Capture process proof or explicitly reviewed reconstruction is required.
- Generator effective/requested CPU set and container/ancestor CPU quotas are
  checked, including missing-evidence and restrictive-quota cases.
- Launcher verifies actual Docker identity, image ID, replicas, health,
  memory/swap/CPU constraints, live kernel cgroups and pre/post resource
  continuity against rendered reviewed configuration. Resource snapshots omit
  environment secrets and are retained per run.
- Pretraffic resume is disabled; the old incomplete guarantees cannot be used
  for another launch. Prior resumed evidence remains preserved as executed.
- Post-run metadata defects propagate to run-result and aggregation. Conditional
  diagnostic latency stays visible, but resource-unqualified or post-defective
  attempts do not enter campaign-qualified ordinary replicate statistics.
- Authoritative retained EdgeX count is 5,192. Updated discovery bound 16,384
  covers that inventory plus 1,000 qualification and 5,000 ordinary devices.
  Provisioning now checks both runtime discovery limits before creating devices;
  it reserves the remaining 6,000-device budget without deleting old cohorts.

Required full 50/s headroom runs remain pending for repaired FIWARE and EdgeX:
30 s warm-up, 120 s measurement, frozen 900 s drain, 30 s quiet/late windows.
They are not small-change repair reruns or ordinary repetitions. Candidate IDs:
`fw-dispatch-endpoint-qual-20261106-reviewedheadroom1` and
`ex-dispatch-endpoint-qual-20261106-reviewedheadroom1`. Obtain follow-up review
before launching them; short success evidence is not relabeled as full-duration
qualification. No ordinary campaign run has started.

### Independent follow-up and qualification execution

The same independent reviewer returned CONDITIONAL clearance for the two exact
full headroom IDs only, not for ordinary traffic. Actual clearance record:
`independent-review-qualification-clearance.json`. The reviewer independently
reproduced the healthy baseline and negative duplicate/CPU/host cases and
confirmed resume is disabled. Two additional findings (missing-history
aggregation and incomplete resolved qualification field checks) were fixed with
focused regressions before launch. Configured Docker healthchecks get a bounded
startup wait, not an automatic workload retry.

FIWARE `fw-dispatch-endpoint-qual-20261106-reviewedheadroom1` is running through
the reviewed launcher using the full frozen plan. Do not launch another workload
until it returns and cleanup/evidence are checked. Next is the separately
identified full EdgeX qualification, serially. Neither short diagnostics nor
this conditional clearance release the ordinary batch.

The generated `run-matrix.json` binds all 15 ordinary IDs/plans/seeds to the
frozen scope SHA-256 and interleaved order. All 15 plans have been resolved and
verified at 25/s with 300 s warm-up and 600 s measurement; the obsolete ORD IDs
in operational instructions were corrected. No ordinary run has started.

### Ordinary batch release and first attempt

The independent reviewer has accepted full EdgeX qualification after raw-ledger,
clock/resource/hash/window and cleanup checks, and returned CONDITIONAL technical
release for the exact 15-cell matrix. Record:
`independent-review-ordinary-release.json`. FIWARE full qualification is likewise
accepted (`independent-review-fiware-qualification.json`). TB's reused full
qualification retains its explicit reconstructed-provenance limitation.

Ordinary attempt 1, `disp-tb-r01-20261106`, has been launched under that release.
Frozen settings remain 25/s, 1,000 devices, selector 0, N=2/k=2/pool 4-5,
300 s warm-up + 600 s measurement, GOGC=400, monitoring and offline analysis.
Wait for its actual run-result and verify evidence/cleanup before the next
matrix entry, `disp-fw-r01-20261106`. No automatic workload retry or replacement.

As requested by the release review, subsequent EdgeX activations retain numeric
runtime inventory/metadata/driver discovery API evidence alongside readiness
gates. This adds evidence files only; it does not change traffic, limits or
readiness predicates. Focused readiness/launcher tests pass. No remote service
or monitoring configuration was changed during the running TB attempt.

Current mutable progress is tracked in `campaign-progress.json`; original
failed attempts and earlier checkpoint statements remain historical records.

### Ordinary entry 1 setup failure; progression to entry 2

`disp-tb-r01-20261106` aborted on the first setup login (HTTP 503). Raw k6
summary has null setup completion, zero scenario epochs and zero attempted
telemetry; request/sink ledgers are empty. No capture window or latency was
fabricated. This is an invalid setup attempt with **0/22,500 offered/planned**,
not a measured delivery loss. Original result remains unchanged beside
`recovery-result.json`; services were stopped volume-preservingly and the owned
load lock released after zero-traffic/cleanup verification.

The generic launcher had omitted the HAProxy readiness gate used by the earlier
TB-specific qualification wrapper. Both exact N=2 backend gates are now checked
under a bounded pretraffic wait and their metrics retained. Cutoff derivation
rejects invalid/unstarted measurement epochs. Focused tests pass; a separately
identified post-failure readiness login returned 200 without retaining its token.
It does not validate or replace the failed attempt.

Independent recovery/progression clearance:
`independent-review-tb-r01-recovery.json`. Continue at original entry 2,
`disp-fw-r01-20261106`, now launched under unchanged frozen settings. No retry,
replacement ID or extra favorable repetition is permitted for TB entry 1.
Report five planned TB attempts and **at most four valid measured repetitions**
if its remaining attempts succeed. All prior stop/review conditions still apply.

Aggregation now retains the setup-failure classification, explicit zero offered
count and corrected recovery cleanup state without modifying original evidence
or inventing missing-message counts. Contradictory zero-traffic recovery
sidecars are rejected. Six focused aggregation tests pass.

### Ordinary entry 2 accepted; entry 3 continuation

`disp-fw-r01-20261106` completed with valid measurement/capture, clock-qualified
latency, qualified generator/SUT resources and successful post-run/cleanup gates.
Raw request, sink and history ledgers each contain exactly the same 22,500
unique run/sequence identities, all requests acknowledged, with zero missing,
duplicate, late or anomalous sink deliveries. The frozen 7,500 warm-up and
15,000 measurement messages were preserved. Raw nearest-rank measurement
dispatch-to-complete-body p99 independently recomputes to **107.137939453125 ms**.
History reanalysis succeeded; owned services stopped with volumes preserved and
the workload lock released. No phase CPU-cost or capacity claim is made.

Continue at original entry 3, `disp-ex-r01-20261106`, under the existing
independent ordinary release and unchanged frozen plan. TB entry 1 remains a
zero-traffic setup failure and is not replaced.

### Ordinary entry 3 accepted; entry 4 continuation

`disp-ex-r01-20261106` completed with valid measurement/capture, clock-qualified
latency, qualified generator/SUT resources and successful post-run/cleanup gates.
Raw request, sink and history ledgers reconcile exactly across 22,500 unique
run/sequence identities, all requests acknowledged, with zero missing, duplicate,
late or anomalous deliveries. Frozen scope remains 25/s, 1,000 devices, selector
0, 300 s warm-up and 600 s measurement. Raw nearest-rank measurement
dispatch-to-complete-body p99 recomputes to **7.9072265625 ms**.

Both replicas accepted all 1,000 devices; runtime metadata/driver discovery
limits were 16,384 and retained pre-provision inventory was 6,192. Metadata
start time remained unchanged with zero new panics/restarts. Owned services
stopped volume-preservingly; cleanup is verified and the load lock released.
Continue at original entry 4, `disp-fw-r02-20261106`, under the existing release,
without replacing any attempt or changing the frozen workload or policy.

### Owner-requested pause during ordinary entry 4

Owner stated they will stop FIWARE repetition 2 and continue later. No further
attempt may launch automatically. Actual interruption and cleanup are not yet
verified; the launch-in-progress record is not a claim that stopping succeeded.
Preserve all entry 4 evidence. Before resuming, inspect its final result/raw
artifacts, remote workload/capture processes, service state and workload lock.
If interrupted, retain that classification without automatic retry/replacement;
apply the existing pause/review conditions before further matrix progression.

### Owner-requested generator progress visibility (local only)

The runner now observes the existing complete k6 outcome log every 10 seconds,
outside the request-timing path. Future builds emit live completed/planned tick,
ACK/failure/unknown totals, plus per-phase JSON and a small Markdown send report.
Unique tick progress does not double-count duplicate responses; ACK is not
sink/history delivery and generator exit is not a capture/cleanup verdict.
See `docs/development/generator-progress.md`. No frozen workload, k6 script or
validity predicate was changed for this feature. Runner/CLI race tests and diff
checks pass. No deployment, workload restart or remote reconciliation was
performed; entry 4 remains unresolved and the owner pause still applies.

### Owner requested resumption; load-host access blocker — 2026-11-07

The owner now authorizes continuing the remaining original matrix. Validated
completed FIWARE/EdgeX attempts do not require rerunning for the new diagnostic
progress logging; TB repetition 1 is not replaced. Before continuation, two
bounded SSH checks to `ramonavm1` timed out. `ramonavm2` is reachable and reports
FIWARE services running (Up 8 hours), which does not establish the generator's
state or validate repetition 2. No remote mutation or new workload was performed.
Record: `resume-check-20261107.json`. Restore load-host access, then recover
entry 4's artifacts, reconcile its outcome and cleanup/lock, and apply release
conditions before original entry 5. Do not treat local launcher cancellation as
proof that remote traffic stopped or automatically retry the unresolved attempt.
