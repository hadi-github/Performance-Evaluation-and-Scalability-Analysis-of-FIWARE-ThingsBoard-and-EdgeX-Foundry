# Offline claim analysis

**Current authority (owner approval, 2026-11-01):** [D44](../architecture/design-decisions.md#d44--owner-profile-mappings-2026-11-01)
adopts the three profile mappings. The executable policy is
[`offline-profiles-20261101.json`](../../research/protocols/offline-profiles-20261101.json)
and is also the built-in default. See [current configuration](#owner-approved-configuration-2026-11-01).
The implementation plan and readiness review below retain their **pre-approval** findings;
their statements that mappings/thresholds await owner approval are superseded by D44, not
by a retroactive edit to archived source definitions. Campaign remains **NO-GO**.

## Implementation plan (2026-11-01)

1. Extend benchctl, reusing report/audit/cpubound rather than introducing a second calculator.
2. Inventory and hash inputs; validate identity, completeness, windows and provenance before calculation.
3. Assess offered load, endpoint-specific latency, sink/history correctness, CPU and configured profiles separately.
4. Emit JSON, Markdown and comparison CSV; keep device count separate from offered rate.
5. Exercise small synthetic evidence and read-only retained evidence; never contact services.

The three chosen application profiles are quick delivery, reliability of saved data and high
number of devices. Their names are settled by the current owner instruction. No definitions with
numerical requirements explicitly mapped to these three names were found in the repository.
Shared SLO/workload values do exist: see the [profile readiness review](#profile-readiness-review-2026-11-01)
for their exact sources, adoption status and configuration mapping. Archived P2/P3 are older
application placeholders, **not** aliases for the owner's second and third chosen profiles.
The preregistered TELEM deadlines are candidates, not automatic requirements for these profiles.
Missing requirements must remain NOT_ASSESSED. The archive is not modified.

## Commands and outputs

Run from the repository root. Every output directory must be new and outside the input
run (including symlink aliases). These commands only read local files; they do not call
`runner.Finish`, `check`, Docker, exporters, SSH, HTTP or the SUT.

```sh
go run ./cmd/benchctl analyze-demo -out /tmp/opencode/my-new-synthetic-demo
go run ./cmd/benchctl analyze \
  -run results/rehearsals-20261029/REH-FIWARE-V4 \
  -out /tmp/opencode/my-new-retained-analysis
go run ./cmd/benchctl analyze \
  -run /path/to/run -evidence /path/to/capture.json \
  -profiles /path/to/profiles.json -out /tmp/opencode/my-new-analysis
go run ./cmd/benchctl compare \
  -analysis /tmp/opencode/my-new-analysis/analysis.json \
  -analysis /tmp/opencode/another-analysis/analysis.json \
  -out /tmp/opencode/my-new-comparison
```

`analyze` emits `analysis.json`, `report.md`, and `SHA256SUMS`. `compare` emits
`comparison.json`, `comparison.csv`, and `SHA256SUMS`. The demo creates three explicitly
synthetic static runs, their analyses and a comparison: a valid example, a delivery-latency
violation and the same rate with more devices. It runs no workload. All its thresholds,
identifiers, CPU counters and digests are examples, not research evidence.

Command success means the report was written, **not** that a profile or campaign passed.
Inspect claim statuses. Malformed specifications and unsupported specification versions
are command errors; unsupported producer schemas are NOT_ASSESSED. Evidence malformed
under a supported schema is INVALID. Missing primary artifacts can produce a diagnostic
report with no evaluable steps. No missing input is replaced by an empty ledger.

## Supplementary schemas

Go definitions in `internal/analysis/types.go` are the authoritative schema. Evidence
specifications, policies, analyses and comparisons use schema version 1; input resolved
plans use version 1 and generator summaries version 2. Specifications reject unknown
fields and trailing JSON. CLI `-evidence` / `-profiles` paths are relative to the invocation
directory; paths *inside* specifications are relative to `-run` (absolute paths also work).

Pre-approval example of an explicitly unassessed policy (not the current default):

```json
{
  "schema_version": 1,
  "synthetic": false,
  "interpretation": "claim-gates-v1",
  "profiles": [
    {"id":"quick_delivery","name":"Quick delivery","source":"owner; requirements pending","requirements":[]},
    {"id":"saved_data_reliability","name":"Reliability of saved data","source":"owner; requirements pending","requirements":[]},
    {"id":"high_device_count","name":"High number of devices","source":"owner; requirements pending","requirements":[]}
  ]
}
```

Each requirement has `metric`, `operator` (`<=`, `>=`, `==`), `threshold`, `unit` and
`source`. A null threshold or missing source remains NOT_ASSESSED; unsupported metrics,
operators and mismatched units are errors. The supported metric/unit registry is
`internal/analysis/profiles.go`. Omitted profile IDs now use the D44 owner-approved defaults.
Explicit empty requirement arrays still mean NOT_ASSESSED, not an implicit PASS.
Never use synthetic policies for retained real evidence.

The evidence specification uses the same `schema_version`, `synthetic` and
`interpretation`, plus:

- `configuration`: `id`, `application_cpu_budget_k`, actual `replicas` map,
  `platform_image_digests` map, `effective_configuration_path` and
  `effective_configuration_sha256`. The hash must match the supplied artifact bytes.
- `observation_deadline_ms`: explicit absolute epoch-ms drain/observation deadline.
- `sink`, `history`: each has `path`, `complete`, `source`, `start_ms`, `end_ms`;
  history also needs `collected_at_ms` at/after the deadline. These are capture
  attestations, not independently proven completeness. An empty *available and complete*
  export differs from a missing export. Missing identities are absence at that observation,
  never demonstrated permanent loss. A later history snapshot cannot establish exactly
  when persistence first became visible.
- `sink_clock`: only `shared_load_realtime` with a nonempty `clock_source` enables
  scheduled-to-receipt latency. No cross-host offset is guessed.
- Sources optionally name `diagnostics_path` and `diagnostics_run_local`. Nonzero
  run-local sink overflow/unparsed/recording-error or history unrecoverable counts invalidate
  that capture. Nonzero cumulative counts without a local baseline leave it unassessed.
- `cpu`: list of `{step,before,after,expected_containers,denominator,clock,clock_source}`.
  Denominators are `offered`, `acknowledged`, or `persisted_acknowledged`; the latter
  requires independently validated history. Duplicate CPU step entries are errors.
- `backlog`: optional diagnostic JSON array path containing `{epoch_ms,records}` points;
  these queue counts alone cannot support sustained-growth or storage correctness claims.

The generated demo's `inputs/valid/evidence.json` and `profiles.json` provide complete
executable examples. Do not relabel them as real observations.

## Claim dependencies and arithmetic

- **PASS**: that claim's measurement prerequisites or configured requirements hold.
- **FAIL**: valid evidence violates a configured requirement, or complete identity capture
  shows missing acknowledged, duplicated or unexpected identities.
- **INVALID**: instrumentation, identity, schema or health contradictions prevent that claim.
- **NOT_ASSESSED**: an artifact, endpoint, threshold, clock or completeness prerequisite is absent.

Offered-load health reuses `report.EvaluateInputs`, formerly the arithmetic inside
`report.Evaluate`. Additional identity/accounting checks establish whether deterministic
planned identities can be used. Normal successes do not have individual outcome records:
`records.ndjson` holds exceptions/early sends, so complete aggregate accounting plus the
planned schedule is required before inferring the successful identity set. Warm-up and
other rate steps are excluded from each measured cohort. Unknown transport outcomes stay
unknown even when independently observed downstream.

Sink/history joins reuse `audit.Analyse`, keyed by **(run_id, seq)**. Equal row counts
are not correctness: one missing identity plus one duplicate still fails. Device count is
separate from offered messages/s; observed history devices are distinct nonempty device
fields, not assumed from configured population alone. Missing secondary captures do not
invalidate offered-load or acknowledgement statistics.

Achieved rate is attempted sends / nominal duration, not observed service throughput.
Unsuccessful ratio is (explicit failures + transport unknown) / attempted sends.
Acknowledgement latency is scheduled arrival → HTTP response for successful requests.
Native `k6.json.gz` optionally supplies complete per-identity `Point/http_req_duration`
samples (`name=send`, `run_id`, metadata `seq` and `sched`) in **milliseconds**. Its
HTTP duration is a separate diagnostic, not acknowledgement or durable-commit latency.
Incomplete traces emit no percentile; malformed traces invalidate only that diagnostic.
Prometheus time-series units must not be substituted for native JSON units.

Delivery latency is first sink receipt minus scheduled arrival, conditional on the declared
clock relationship and complete acknowledged cohort by the deadline. Incomplete observed
cohorts can have descriptive values but cannot establish full-cohort latency. Always read
a metric with its named claim status; the presence of a numeric value is not validity.
There is no durable-commit timing producer. Store `ts` is not a commit timestamp.

Raw HTTP/delivery distributions reuse `arrivals.Quantile` (nearest rank). Summary Trends
retain k6's own producer estimator. These estimators are **not silently harmonized**.
Neither minute p99s nor per-step p99s are averaged into a run p99; exact run-wide
acknowledgement percentiles remain unavailable without the underlying samples.

CPU calculations reuse `cpubound.Parse/Diff`: CPU-s = sum(delta usage_usec)/1e6;
CPU-s/message divides by an explicitly named positive count. Container identity, restart,
counter monotonicity, membership, cpuset, service and tier must match. Phase-specific cost
also requires explicit shared-clock evidence and endpoint times matching the phase within
0.001 ms. Enclosing intervals retain descriptive counter deltas, **not** a nominal-phase
per-message cost. CPU allocation K is not a device count or offered rate.

## Optional backlog interpretation (not adopted campaign methodology)

Nothing enables a confidence procedure by default. A policy may explicitly supply:

```json
"backlog_rule": {
  "method": "ols-iid-10s-one-sided95-v1",
  "independent_bins_assumption": true,
  "calibration": "calibration.json",
  "source": "explicit owner-approved interpretation reference"
}
```

This calculates acknowledged-pending identities using scheduled arrivals and first sink
receipts, integrates 10-second bin means, and fits OLS for at least three complete bins.
The one-sided 95% lower slope bound is beta - t(.95,n-2)*SE under the **explicit IID-bin
assumption**. Binning does not establish independence; autocorrelated bins require a
different approved method. Student-t inversion is tested against reference quantiles.
Delay growth in ms is beta / offered_rate * window_ms (the archived 600-second case
is a special case). Growth requires both lower_bound > 0 and delay_growth > epsilon.
`backlog.non_growing=1` means that two-part criterion was not met, not proof of correctness.

Calibration JSON has `schema_version`, `synthetic`, and `runs`; each run supplies
`run_id`, `platform`, `valid`, `rate`, `reference_capacity`, `delay_growth_ms`,
`source_sha256`. At least five distinct valid run/hash attestations per platform
(thingsboard, fiware, edgex), at <=25% reference capacity, are required. Epsilon is
the maximum absolute supplied stable-run delay growth. These references are attestations:
the tool hashes the calibration file but does not recover or independently reproduce
the cited experiments. Synthetic calibration tests are not qualification evidence.

An optional `service_slo` profile claim requires delivery p99, unsuccessful ratio,
sink/history missing acknowledged and duplicate-copy requirements, and backlog non-growth.
Even a configured passing service profile cannot adopt a research protocol or qualify a campaign.

## Comparisons and methodological conflicts

Groups conservatively separate platform, configured devices, K, replicas, effective config
hash, platform/generator images, source/script versions, seed, arrival/connection parameters,
warm-up, phase/duration, synthetic label, interpretation, profile requirements, clock/observation
offsets, CPU denominator/membership and backlog interpretation/calibration hash. Rate varies
within a group. Insufficient provenance, invalid load or unassessed profiles are excluded
but remain visible in CSV/JSON. Duplicate run inputs do not become independent repetitions.

Highest observed passing rate and highest rate whose lower tested points all pass are
descriptive only. Conflicting repeats or excluded lower points block the latter. Distinct
run IDs are not proof of independence. No cross-platform pooling, percentile pooling,
statistical significance, extrapolated maximum capacity or campaign C_SLO is asserted.

Conflicts deliberately remain visible: archived candidate SLOs versus missing definitions
of the three selected profiles; k6 versus nearest-rank quantiles; scheduled-to-response versus
HTTP duration versus sink receipt versus history visibility; broker queue depth versus
acknowledged-pending backlog; nominal phase versus enclosing CPU endpoints; cumulative
capture diagnostics versus run-local errors. The archived research files are not edited.

## Validation and handoff (2026-11-01)

Starting branch: `chore/ci-and-release`; starting/ending HEAD:
`36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`. Implementation remains uncommitted.
Changes are `internal/analysis/`, `cmd/benchctl/analysis.go`, CLI dispatch/audit prerequisites,
shared report arithmetic, reconciliation prerequisites/full-identity repair removal, the
integration assertion reflecting that fix, and this document.

Local test commands (no Docker-backed integration run):

```sh
go test -count=1 ./internal/analysis ./internal/report ./internal/reconcile ./cmd/benchctl
go vet ./...
go test -short -count=1 ./...
go test -race -count=1 ./internal/analysis ./internal/report ./internal/audit ./internal/cpubound ./internal/reconcile
git diff --check
```

Synthetic tests cover drops, lag, missing outcome ledgers/failure rows, missing/partial/empty
history, identity malformation, duplicates masking missing identities, unknown/late outcomes,
missing clocks/deadlines, capture errors, unsupported schemas, null/missing nested fields,
true zero denominators, reset/missing/enclosing CPU counters, thresholds/endpoints, version
and device separation, duplicate inputs, warm-up exclusion, non-averaged percentiles,
native HTTP milliseconds/incomplete/truncated traces, conditional calibration, and safe outputs.

Verified outputs: `/tmp/opencode/offline-analysis-verified-jsnxpajn/` contains a built
`benchctl`, fresh synthetic demo, analyses of REH-EDGEX-V1, REH-FIWARE-V4, REH-TB-V2,
P3-edgex-delay700-20261031 and P3-edgex-immediate-20261031, a retained comparison,
and `evidence-before.json` / `evidence-after.json`. **52 retained files hashed identically**.
JSON parsed and all output SHA256SUMS verified. The three rehearsals retained offered-load
PASS; both interrupted qualification examples remained INVALID (zero ticks / exit 108).
With no capture/configuration sidecars supplied, history/sink/CPU/profile/comparison claims
were not manufactured. Temporary paths are validation artifacts, not committed fixtures.
The final build is `benchctl-final`, its demo is `synthetic-demo-final/`, and its
`final-retained-*` analyses matched the earlier retained analyses exactly. Final CSV shape,
six distinct synthetic profile/device groups, PASS/FAIL example outcomes and retained-row
comparison exclusions were checked. All test commands above passed after the final edits.

**Main campaign remains NO-GO.** Owner inputs still needed: sourced numerical requirements
for all three profiles, adopted service/backlog interpretation, trustworthy calibration and
independence assessment, effective configuration/image/replica provenance, complete receiver
and history captures with observation windows, clock attestations and phase CPU endpoints.
Missing interrupted-run receiver ledgers remain a separate recovery blocker; do not synthesize
them or infer permanent loss. Old durable timing/CPU endpoints cannot be reconstructed.
Any recovery, operational probe, service/network change, new benchmark or push requires
separate authorization. None was performed in this task; yaade, devices and volumes were untouched.

## Profile readiness review (2026-11-01)

This review preserves the three owner-selected profiles. It does not create replacement
profiles, ratify a TELEM threshold or treat a workload name as a profile definition.
Search scope: tracked project documentation/research, workload plans and analysis defaults;
retained evidence was not changed. The latest found interpretation of numeric SLO adoption
is R1, reaffirmed on 2026-10-31 by R6. The owner's subsequent choice settles the **names**,
not an undocumented numerical mapping.

### Exact definition sources

| Ref | File and exact section | Finding |
|---|---|---|
| R1 | `research/protocols/slo-profile-reconciliation-20261029.md` §1 “What the preregistration records”, §2 “Reconciliation with the current status”, §3 “External attribution”, §4 “What would resolve this” | TELEM-500 primary and 250/1000/5000 ms sensitivities are archived candidates, not active gates; owner ratification absent; external 3GPP attribution unverified. |
| R2 | `research/preregistration/preregistration-archive/original/slo_notes.txt` §1 SCOPE, §2 TERMS, §4 VARIANTS, §5 ACCEPTANCE RULE | Delivery endpoint = **first sink receipt minus scheduled arrival**. Candidate p99 <= T (250/500/1000/5000 ms); errors <=0.001; zero missing acknowledged identities and duplicates at sink/store; no sustained backlog growth. Not time to durable commit. |
| R3 | `research/preregistration/preregistration-archive/original/workloads.txt` §1 WORKLOAD MODEL, §2 WORKLOADS, §3 PAYLOAD AND IDENTITY, §4 W2, §6 GENERATOR REQUIREMENTS, §7 RESET AND COHORT RULES | W1: 1,000 devices, adaptive aggregate-rate sweep, open-loop constant HTTP, round-robin, three numeric values plus identity/timing. W2: **100 vs 10,000** devices at fixed aggregate rates ~50%/~75% C_SLO(500,1), N=1; conditional on provisioning/reset qualification. W2 is sensitivity, not a capacity search. L1 validated, L2 candidate; final layout/overhead gate remains. |
| R4 | `docs/architecture/design-decisions.md` rows D35–D39; `docs/methodology/methodology-reconciliation.md` §6 “Preserved protocol” / “Clarifications” | Shared app-core budget k and actual replicas recorded separately; 2x generator headroom; target k={1,2,4}. Preserved W1: 300 s warm-up at 20% prior capacity estimate; coarse x1.5/180 s, linear refinement; 600 s confirmation; >=5 independent randomized repeats (up to 10 under CI stopping rule). Deadline wording here predates R1's pending-adoption clarification. |
| R5 | `research/preregistration/preregistration-archive/original/rqs.md` §0 “SLO profiles”, “Telemetry semantics”, “Experimental protocol”; `original/decisions.md` “SLO PROFILES” / “TIME BUDGET” | Older P1 smart grid: 1 s end-to-end, 50% platform share =500 ms; P2/P3 TBD. Not definitions of “Reliability of saved data” or “High number of devices”. Earlier start/send and row-count formulations differ from R2. |
| R6 | `docs/archive/sessions/verify-followup-20261031.md` §“Claim-level readiness” rows “TELEM profiles”, “Full campaign generator envelope”, “Exact platform phase CPU cost”; `docs/status.md` §8 items 2–4, §9 | Reaffirms pending numeric adoption and limited qualification scope. Old platform CPU endpoints absent; interrupted SUT ledgers unavailable. Controlled-receiver results are not platform/population qualification. |
| R7 | `research/preregistration/preregistration-archive/original/slo_notes.txt` §6 BACKLOG DECISION PROCEDURE, §9 QUALIFICATION GATES | 10 s means, OLS, one-sided 95% lower slope bound >0 AND delay growth > calibrated epsilon; >=5 stable runs/platform <=25% pilot capacity; freeze shared epsilon. Pilot must confirm 60 s drain /10 s timeout do not change verdicts. No calibrated value supplied here. |
| R8 | `research/preregistration/preregistration-archive/original/fingerprint_notes.txt` §3 CAPACITY SEARCH PROTOCOL, §9 OPEN DECISIONS | Alternate coarse doubling from 10/s, <=5% bisection, 5/5 at passing and next failing rates; uncertainty/search choices must not be silently combined with R4. |
| R9 | `research/preregistration/preregistration-archive/original/slo_components.txt` “SLO COMPONENTS”, rows “Latency statistic”, “Error rate”, “Missing at sink/storage”, “Drain period”, “HTTP client timeout” and closing note | p99 over acknowledged A, including arrivals during drain; errors=(non-2xx/timeouts/connection errors)/sent S <=0.1%; zero missing means acknowledged, **not all offered**. 60 s drain and 10 s timeout explicitly provisional. These clarify the shared candidate, not its mapping to the three names. |

### Each chosen profile versus current pipeline configuration

The authoritative defaults are `internal/analysis/types.go::DefaultPolicy`; all three
requirements arrays are empty. The numerical examples in `synthetic.go::Synthetic` are
explicitly synthetic (100 ms delivery, zero history missing/duplicates, >=2 observed devices
and >=10 messages/s): **none is an agreed research requirement**.

| Selected profile | Latest explicit definition / relevant existing values | Already implemented | Still to enter or decide |
|---|---|---|---|
| Quick delivery (`quick_delivery`) | Name settled by owner; no dated numerical definition under this name found. R1/R2 provide the latest relevant delivery candidate: scheduled→first sink receipt, p99<=T; T=500 ms primary candidate, 250/1000/5000 sensitivities; R3/R4 provide common W1 settings. | `latency.delivery.p99_ms` in ms; distinct ack and native HTTP diagnostics; explicit clock/completeness gates. | Owner must identify the authoritative mapping to this name, or confirm which existing T and ancillary criteria apply. Then enter sourced `<=` requirement(s); do not select 500 ms automatically. Device count/rate/duration for this profile are not separately defined; W1 baseline exists but is not a named-profile binding. |
| Reliability of saved data (`saved_data_reliability`) | Name settled; no separate numeric definition under this name found. R2 §5 candidate: zero missing acknowledged history identities and duplicate copies (also sink correctness, <=0.001 errors and backlog for the full shared SLO). R2 §1 explicitly excludes durable-commit timing. R5's older “stored count==sent” differs. | Full-identity history join, missing/duplicate/unexpected counters, unknown-outcome relationships; source completeness/deadline checks. | Confirm whether this name uses R2's **acknowledged** cohort, which of the shared criteria apply, and whether post-drain retrievability is the intended saved-data endpoint. If adopted, enter `history.missing_acknowledged==0`, `history.duplicate_extra_copies==0` and any other explicitly required criteria. No agreed per-message durable latency or long-term retention interval found. |
| High number of devices (`high_device_count`) | Name settled; no adopted “high” cutoff under this name found. R3 §2/§4 defines conditional W2: D={100,10,000}, lambda≈{0.5,0.75}*C_SLO(500,1), fixed aggregate rate, N=1. W1 D=1,000 is the baseline, **not** a high-device acceptance threshold. | Configured device count and observed history-device count are separate metrics; rate remains separate; comparisons do not pool different D. | Confirm whether W2 is the intended binding for this existing profile or provide its already-agreed definition. If so, 100/10,000 need no reinvention; actual rates require a qualified reference capacity, not an arbitrary number. No maximum-device-capacity search or device-count PASS cutoff is specified by W2. |

Shared W1 settings in R3/R4 are **documented**, not a request to redesign them. They belong
in future workload plans and run provenance; the analyzer reads resolved plan values rather
than scheduling workloads. `workload.devices`, `workload.offered_rate`, and
`workload.measurement_seconds` can enforce explicitly adopted numeric requirements, but
the current profile schema does not enforce warm-up, connection mode, seed, reset,
replications or uncertainty: those need plan/run qualification and campaign checks.
`workloads/plans/qual1000-thingsboard-immediate.json` explicitly declares a **pilot** with
15 s warm-up, 120 s at 400/s, 5 s request timeout and seed 20261031. Its values are already
present for that qualification only, not missing decisions or substitutes for W1/R7.

### Evidence producers and remaining tooling work

| Needed evidence | Existing producer / compatibility | Offline action completed or documented | SUT requirement |
|---|---|---|---|
| Plan, outcomes, schedule, code/generator identity | `internal/runner/runner.go::Manifest` / `Run`; `workloads/k6/gen.js`; `-full-json` optional native samples. | Analyzer already reads these. No production profile policy is auto-populated from pilot or synthetic settings. | Existing retained data can be read offline; new platform qualification requires separately authorized runs. |
| Sink identities and capture health | `scripts/run-with-sink.sh` and `scripts/sink.sh`, Go sink NDJSON/metrics. Files are often **siblings** `<run>.sink.ndjson` / `<run>.sink-metrics.txt`; source spec may use `../...`. | Documented exact adapter fields: `sink.path`, `diagnostics_path`, `diagnostics_run_local`, `complete`, `source`, capture start/end and absolute deadline. Wrapper drains until quiet (30 s, up to 15 min), not fixed 60 s; it does not emit the analyzer sidecar or a precise start/deadline attestation. Post-run status is captured before stop; completeness requires successful close/flush too. Never infer attestations from file existence. | Local import of already-retained sink files needs none; missing SUT receiver ledgers need recovery. New capture/proof of window completeness needs authorized runtime work. |
| Historical identities, devices, diagnostics | Prefer `scripts/history-ids-audit.sh` for device+ts: TB uses device join; EdgeX uses `device_info`; FIWARE delegates to `history-ids.sh`. Basic TB/EdgeX `history-ids.sh` lacks `device`, so it cannot supply `workload.observed_history_devices`. | Fixed FIWARE fail-open discovery/diagnostic query errors and missing-table success; mock-only regression tests added. Successful empty **existing** tables remain valid empty exports. Corrected stale audit header claiming FIWARE unsupported. Sidecar must record collection time and diagnostics, process exit success, capture scope and deadlines. Query success alone is not independently proven completeness; malformed/null identities still require review. | Existing exports import offline; rerunning DB exporter is SUT access. SQL changes were not live-tested; pinned-schema and full-population capture qualification remain runtime work. |
| Effective config, image digests, actual replicas and CPU budget | Compose rendering / `scripts/check-resources.sh` support intended config; runner captures generator digest/source, **not** complete effective platform config. Docker inspect on the SUT supplies actual containers/images/cpusets. | Documented assembly: preserve exact rendered config bytes, hash them into `configuration.effective_configuration_sha256`, set `effective_configuration_path`, enter separately verified K, per-service actual replicas and immutable image digests. A local Compose render is intended config, not proof of effective runtime state; protect secrets in any capture. No all-platform automatic sidecar assembler found. | Hashing/validating supplied artifacts needs none; obtaining or verifying missing effective runtime facts needs SUT access. A receiver-only inspect is not a platform inventory. |
| CPU endpoints / clock qualification | `-cpu-snapshot-command` → `cpu-snapshots.jsonl`, `cpu-step-NN-{before,after}.tsv`, `cpu-boundary.json`; `monitoring/exporters/cpu-snapshot.sh`; `scripts/verify-cpu-boundary.py`. | Existing paths can be entered in `cpu`. **Important gap:** runner selects enclosing intervals with skew; analyzer's exact-phase test requires matching endpoints within 0.001 ms and shared-load-clock attestation. Current cross-host capture/offset bounds cannot satisfy that just by relabeling them. Keep enclosing deltas descriptive. `scripts/clock-bound.py` bounds offsets but analyzer has no bounded-cross-host clock/uncertainty model. | Old absent endpoints are unrecoverable. New complete membership/counter capture and timing qualification require authorized SUT work; an exact-phase or bounded-interval estimator needs a separately specified method, not relaxed gates. |
| Calibrated backlog / capacity / uncertainty | R7 plus optional analyzer OLS IID interpretation. R8 defines campaign search/5-of-5, not the implemented descriptive comparison maximum. | Documented: need real calibration references and a justified CI model; IID is not justified by binning. Current policy metric registry has no memory threshold, p95 or goodput requirement, and no independent-run capacity uncertainty calculation. These do not block the three names; implement only if their authoritative definitions require them. | Defining/implementing a method can be offline. Producing missing calibration, qualified reference capacity and population/reset evidence needs authorized runs. |

### Conflicts and recommended resolutions (not silently applied)

1. **“Approved TELEM-500” wording vs pending adoption:** R4/D39 and
   `docs/qualification/step5-owner-signoff.md` §6.1 “Frozen elements”, row F4, call
   500 ms frozen; R1 §2 and R6 explicitly keep it pending.
   Recommendation: retain R1's pending numerical status until an append-only owner mapping
   binds values to the three selected names. Names are already chosen and are not reopened.
2. **Send timestamp/count equality vs scheduled/identity-cohort semantics:** R5 uses send
   time and sent/store counts; R2 defines scheduled arrival, acknowledged cohort and full
   identities. Recommendation: confirm the later R2 semantics for the named profiles;
   preserve unknown outcomes and report count diagnostics separately. Never use HTTP ack,
   sink receipt, store ts and durable commit interchangeably.
3. **Sweep x1.5/linear vs doubling/bisection:** R4 §6 versus R8 §3. Recommendation: freeze
   one dated campaign search protocol before execution; this does not prevent offline per-run
   analysis and must not be decided by whichever pilot plan happens to exist.
4. **Drain/timeout:** R7's pilot-confirmed 60 s /10 s versus quiet-drain wrapper and 5 s
   qualification plans. Recommendation: retain actual settings as deviations and qualify the
   intended values, not reinterpret the pilot as approval.
5. **Resources:** R5's one-core-per-replica versus D35's shared k. This conflict already has
   an explicit amendment: use D35 and record k and replica topology separately, no new choice.
6. **Storage availability:** old methodology §7/O2 and the old audit-script header claimed
   FIWARE history missing; R6 “FIWARE history path” and current SQL document an implemented
   QL/TimescaleDB exporter at rehearsal scope. Treat that as an implemented producer, **not**
   proof that every missing run export exists or that all higher rates are qualified.

### Decisions versus independent offline work

To activate named-profile PASS/FAIL, the only immediate owner information needed is the
authoritative **name→requirements/workload mapping**, or a pointer to its existing definition
outside this checkout. In particular: which delivery deadline, saved-data cohort/endpoint and
whether W2 supplies the high-device comparison. Existing candidate values above should be
confirmed/referenced rather than reinvented. Search/CI/calibration choices become necessary
before those campaign-level claims, not before reading more local evidence.

No user decision was needed for the exporter failure fix, stale-header correction or this
evidence/configuration mapping; those were completed independently. No production policy
was activated. **Campaign remains NO-GO.** No SUT access, live benchmark, network/service
change, retained-evidence modification or push was performed.

Local validation for this review:

```sh
python3 -B -m unittest discover -s tests/offline -p 'test_*.py' -v
bash -n scripts/history-ids.sh scripts/history-ids-audit.sh
bash scripts/check-preregistration.sh
go test -short -count=1 ./...
git diff --check
```

Exporter tests put a synthetic executable named `docker` first on PATH. They cover failed
table discovery, typed/recovery/diagnostic queries, missing tables, available empty tables,
successful identities and explicit unrecoverable-row diagnostics; no real Docker is called.
All commands above passed: four Python tests (including five failure-mode subtests), shell
syntax, all nine archive checksums, the full short Go suite and whitespace validation.

## Owner-approved configuration (2026-11-01)

[D44](../architecture/design-decisions.md#d44--owner-profile-mappings-2026-11-01) records
the owner's explicit approvals and links to the unchanged source specifications. No new
owner decision is needed about the three mappings. The built-in defaults and the dated
JSON policy are tested for exact agreement.

| Profile | Active configuration | Missing evidence is not a failure |
|---|---|---|
| Quick delivery | `latency.delivery.p99_ms <= 500 ms`. `sensitivities` holds 250/1000/5000 ms; `sensitivity_checks` is separate from primary `checks`/`status`, in JSON, Markdown and comparison CSV. | Complete acknowledged sink cohort through a defined absolute observation deadline and explicit shared generator/sink clock evidence. Never substitute ack or HTTP duration. |
| Reliability of saved data | `history.missing_acknowledged == 0` and `history.duplicate_extra_copies == 0`. No additional sink/error/backlog requirement is silently added to this profile. | Independent history file, complete capture/source attestation covering the selected scheduled cohort and observation deadline, plus history collection time. Missing window/capture gives NOT_ASSESSED. This measures retrievability within the stated reconciliation scope, not durable-commit latency. |
| High number of devices | `device_comparison`: populations `[100,10000]`, reference fractions `[0.5,0.75]`, application k=1 and one replica per service (N=1). No device-count threshold. | A defined C_SLO(500,1) reference and evidence, matched rates at both populations, verified configuration/workload provenance, and independent observed device population. Missing cells/reference/window/capture leave design validity NOT_ASSESSED. |

Use defaults or select the policy explicitly (new output directory required):

```sh
go run ./cmd/benchctl analyze -run /path/to/run \
  -evidence /path/to/capture.json \
  -profiles research/protocols/offline-profiles-20261101.json \
  -out /tmp/opencode/new-owner-profile-analysis
```

### W2 reference and outputs

Optional `w2_reference` in the evidence specification must supply:

- `capacity_messages_per_second`: positive defined reference value, **not inferred**
  from the input runs or their highest passing rate;
- `definition`: exactly `C_SLO(500,1)`;
- `source`: the reference's definition/qualification provenance;
- `path`, `sha256`: retained reference evidence artifact and matching byte hash.

No reference value is installed by this approval. The artifact/source is an explicit
attestation, not an independent recomputation of campaign capacity. The artifact must
carry the same real/synthetic provenance as the study; never cite the synthetic tests
as a real reference. Paths inside the evidence specification, including the reference
artifact path, are relative to the run; CLI specification paths remain relative to invocation.

By default target aggregate rates are exactly 0.5 and 0.75 times the supplied reference.
For **approximately** those rates, an explicitly sourced `rate_points` list may supply
`{fraction, offered_rate_messages_per_second, source}` for either fraction. No rounding
tolerance or rate is invented. These are caller-declared nominal points: the output also
shows the **actual** rate/reference ratio, so the declaration does not prove statistical
equivalence or independently establish that the approximation is justified. Both
populations must use exactly the same declared aggregate rate for each nominal point.

Per-run `high_device_count` remains NOT_ASSESSED because one population/run cannot
establish W2. `compare` adds `device_comparisons` in JSON and `device-comparison.csv`:

- A separate cell per run/step retaining device count, offered rate, actual reference
  ratio, metrics and their claim dependencies. Percentiles are not pooled.
- Separate studies for differing platform, source/scripts, images, effective configuration,
  seed/connection/arrival/warm-up, phase/duration, reference or observation timing.
- `design_validity=PASS` only when eligible cells exist for **all four** combinations:
  100 and 10,000 devices at both reference fractions. This checks design/evidence, not
  performance. Duplicate input runs cannot supply missing cells; k/replicas must be one;
  full independently retrieved history must show the configured population exercised.
- `profile_verdict=NOT_ASSESSED` even for a complete design: no cutoff, acceptable effect
  size or significance requirement was approved. Results are descriptive, not device capacity.
  W2 rows do not enter the regular highest-passing-rate/campaign-capacity groups.

### Still unavailable and deliberately not invented

The approval does not supply a numeric/reference qualification artifact for C_SLO(500,1),
run-specific reconciliation scope/deadline, complete sink/history exports and diagnostics,
clock attestation, effective runtime configuration/images/actual replicas, or qualified
100/10,000-device population evidence. These must be supplied in `-evidence`; missing inputs
are identified in profile/reference/design-cell reasons. Provisional drain/timeout values
are not silently adopted as observation facts. Old absent CPU endpoints and durable-commit
timestamps are not reconstructed. Full campaign search/uncertainty/backlog qualification
remains separate from the three approved mappings.

The earlier tiny `analyze-demo` uses an explicitly synthetic custom example policy, not
the owner W2 design. New file-only owner-profile tests exercise the actual defaults, the
500 ms boundary, sensitivities on both sides of the primary requirement, missing captures/
windows, missing/duplicate stored identities, a full synthetic four-cell W2 design,
missing/incompatible cells, wrong N, unmatched rates, explicit approximate rates and
reference artifact/hash validation. No workload or SUT access is involved.

Validation after D44 implementation passed:

```sh
go test -short -count=1 ./...
go vet ./...
go test -race -count=1 ./internal/analysis
python3 -B -m unittest discover -s tests/offline -p 'test_*.py' -v
bash scripts/check-preregistration.sh
git diff --check
```

Static CLI smoke outputs: `/tmp/opencode/owner-profiles-20261101-rjftggad/`.
The dated policy was applied to generated, explicitly synthetic files only: primary
delivery PASS/FAIL, all three sensitivity checks, saved-data PASS, missing-reference W2
NOT_ASSESSED, CSV shapes and output checksums were verified; synthetic input hashes
matched before/after. The four-cell W2 success and missing/mismatched cases are file-only
Go tests. Existing uncommitted work was preserved; no retained evidence was modified,
no live workload/SUT/service action or push occurred, and no commit was created.

## Fault target, manual fault event and capacity observation foundation (M1f, 2026-11-04)

This is an offline, bounded foundation, not a live fault campaign. `benchctl` still never opens
a Docker client, issues a stop command, schedules a fault, or claims campaign capacity by default.
The new code extends `internal/analysis`; it does not add a second analysis pipeline.

### Versioned fault-target allowlist

`internal/analysis/fault_target.go` resolves exactly one application replica from a supplied,
already-verified project/service/replica/container inventory. The only admitted mappings are:

| Platform | Compose service |
|---|---|
| `thingsboard` | `tb-http-transport` |
| `fiware` | `iot-agent` |
| `edgex` | `device-rest` |

Stateful stores, HAProxy, monitoring exporters and whole-stack selectors are refused. Resolution
requires dry run and verifies the project, service, replica index/N, container id, image,
started-at, healthy flag and every supplied expected label. Every container row is validated
against the service and project that enclose it, a target is selected only from the uniquely
matched allowlisted service, and the complete declared replica set must be unique and contiguous
over `[0,N)`; a self-declared `compose_service` cannot override its enclosing service. Missing,
ambiguous, cross-project, duplicate, spoofed, incomplete/out-of-range replica, unhealthy and
unexpected-id inputs are refused with deterministic sorted reasons. The output is a target identity
only on full agreement.

Project identity is an assertion, not an optional field. An empty enclosing project is not treated
as unknown and never waives consistency: every inventory service, including one unrelated to the
requested target, must declare the requested project, and every container must match its enclosing
service project exactly. This closes the residual case where a valid allowlisted service plus an
unrelated service with `project=""` could otherwise let a container row pass silently. A consistent
unrelated service (same project, containers agreeing with their enclosing service) is still allowed.

### Manual fault event schema and reducer

`FaultEvent` schema version 1 records `run_id`, `protocol_binding_sha256`, platform/project/service,
container id/image, `replica`/`replicas` and `service_replicas`, stop request/completion,
signal/grace/forced, observed unavailability, LB removal, restart request, healthy rejoin,
unexpected transitions, offered rate and window, clock domain/source, `event_source`
(`operator/manual`) and `dry_run`. Explicit nulls and unknown fields are rejected.

`ReduceFaultEvent` validates timestamp ordering where the supplied stages allow it, requires exactly
one intended replica and no unexpected transition, and distinguishes stop, rejoin, failover and
`N=1` service outage. Every supplied stage timestamp must lie inside the declared window; a stage
outside it is `INVALID` and yields no qualification or derived recovery observation. `grace_ms` is
a finite non-negative duration, not a timestamp: zero is valid and negative/non-finite is
`INVALID`. Stop observation is true only from explicit stop-completion evidence, so a request-only
event neither asserts an observed stop nor fabricates a stop duration; request and observation are
reported separately. Missing stop/clock/LB/restart evidence is `UNAVAILABLE`; missing
unavailability-end/rejoin evidence is `RIGHT_CENSORED`. Neither is success. A dry-run event is
`DRY_RUN` and can never qualify. The transient stop/rejoin observation and the declared
degradation-topology reference are separate; the topology reference is never inferred from the
transient interval and is never a measured capacity claim.

### Recovery and capacity observation reducer

`CapacityInput` schema version 1 requires an explicit versioned predicate (`p99_max_ms`,
`attainment_min_ratio`, `missing_max`, `achieved_rate_min_ratio`, optional `backlog_max`,
`min_samples`), an explicit search policy (`monotonicity_assumed`, `min_confirmations`), and
predeclared windows. Each window supplies `scheduled`/`observed`/`missing`, p99, attainment,
achieved rate, backlog and `samples`; no sample or timestamp is invented. No default 500 ms,
hybrid search or 5-of-5 rule is adopted here.

No-fault windows can establish a capacity point. Fault windows are recovery observations only:
`failover` and `rejoin` are separate; `not_recovered`, missing rejoin evidence and recovery outside
the declared window are right-censored, `exact=false`, and cannot establish a bound. Invalid points
(zero denominator, insufficient/censored samples, contradictory counts, unknown fields) are visible
but cannot establish either bound. The result status is `bracketed`, `lower_bound_only`,
`no_passing_rate` or `unresolved`, with lower/upper bounds only when the explicit policy supports
them, `bound_evidence`, `stop_reason` and per-rate confirmation counts. Bound computation sorts the
no-fault rates first, so the status and bounds are invariant to caller window order.
Contradictory valid repetitions remain visible. A confirmed passing rate above a confirmed failing
rate contradicts the declared monotonicity: it is reported and cannot produce a `bracketed` status
or an upper bound, while an independently established lower bound is preserved. Fault and no-fault
windows are separate, so a transient fault window cannot create capacity evidence.

### Offline command and files

```sh
go run ./cmd/benchctl fault-capacity -in evidence.json -out /tmp/opencode/new-fault-capacity
```

The evidence document has `schema_version`, `synthetic`, and optional `inventory` + `target`,
`event` and `capacity` sections. The command writes `fault-capacity.json`, `fault-capacity.md` and
`SHA256SUMS` into a new directory. The Markdown explicitly says capacity is **NOT CLAIMED**. The
same reductions are available as optional `fault_target`, `fault_event` and `fault_capacity` fields
on `analysis.Result`; ordinary `analyze` leaves them nil.

Tests are hand-computable and cover all three allowlisted platforms, stateful/HAProxy/monitoring/
ambiguous/missing/duplicate/unexpected-id refusals, one-replica stop/rejoin, extra transitions,
dry-run non-qualification, ordering and missing-stage censoring, `N=1` outage, bracketed and
all-pass bounds, invalid-upper rejection, contradictory repeats, no-pass, censored recovery, zero
denominators, insufficient samples, strict unknown/null rejection and deterministic JSON/Markdown
serialization.
