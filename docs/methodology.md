# Measurement and interpretation guide

This guide explains how to read the benchmark's measurements. It is a summary,
not a replacement for the versioned [dispatch contract](methodology/dispatch-contract-20261106.md),
frozen plans or [current evidence status](status.md). Historical runs retain
their original policy and verdicts.

## 1. Four observations, four different claims

| Observation | What it establishes | What it does not establish |
|---|---|---|
| HTTP acknowledgement | The configured ingress returned its success response | External delivery, query-visible history or durable commit |
| Sink receipt | The outbound path delivered a complete body from which identity can be recovered | Historical storage or database commit time |
| Independent history read-back | That identity is query-visible at the recorded observation | Its first visibility time or crash durability |
| Durable commit | Requires a separately defined backend-specific instrument | No uniform per-message commit timestamp is available here |

The separation matters because FIWARE and EdgeX deliver to the sink on paths
parallel to history persistence. Even where a configured success route follows
a storage operation, its callback must not be advertised as a universal fsync
or replication guarantee. See the detailed
[observation-point record](methodology/measurement-integrity.md).

## 2. Timing endpoints

For explicitly marked `dispatch-integrity-v1` runs, define:

```text
dispatch delay = first complete-body sink receipt - application dispatch
send lag       = application dispatch - planned scheduled send
scheduled delay = first complete-body sink receipt - planned scheduled send
```

Application dispatch is sampled with JavaScript `Date.now()` immediately before
the HTTP invocation, after payload construction. Receipt is sampled on the
same load host after the bounded complete body read and before parsing.
The primary endpoint excludes pre-dispatch preparation/scheduling delay, but
does not isolate network transmission or a single server component.

The JavaScript timestamp has 1 ms representation resolution. Fractional sink
timestamps do not confer sub-millisecond dispatch accuracy. Retain host identity,
clock provenance and realtime/monotonic checks. Same-host placement avoids
cross-host subtraction, not every clock failure. Negative joined delays and
unresolved clock discontinuities are measurement problems, not negative platform
latency. Cross-host component timing needs its own qualified uncertainty.

Older scheduled-to-sink results are not interchangeable with dispatch-to-sink
results. The prospective withdrawal of the 10 ms send-lag p99 rejection rule
does not repair a historical invalid run. A numeric deadline must identify its
endpoint and policy before it can be used as an acceptance criterion.

## 3. Identity and complete accounting

Use `(run_id, seq)` to join planned identities, individual HTTP outcomes, first
sink receipts and independent history rows. Retain device and schedule metadata
to detect inconsistent joins. Do not fill a missing receipt using an ACK, a log
message or a history row.

For the current policy, every planned identity needs a complete request/outcome
record. Classify responses as success, explicit failure or unknown transport
outcome. A timeout can still be followed by a sink receipt or stored row; it
must remain UNKNOWN in the HTTP ledger even when delivery is later proven.

Reconciliation reports at least:

- Missing deliveries by cutoff and receipts first seen after cutoff
- Duplicate receipts and unexpected identities
- Acknowledged identities absent from independently retrieved history
- UNKNOWN outcomes that nevertheless reached the sink or history
- History duplicates and unavailable/failed exports

A missing export is not an empty successful export. Matching counts are not
matching identity sets. Missing data remains unavailable, not zero-filled.

## 4. Workload validity is separate from platform success

An intended constant rate is not sufficient evidence of actual offered load.
The current policy requires zero dropped iterations, exact identity accounting,
at least 99% achieved physical offered rate and declared dispatch-distribution,
gap and lag-growth bounds. The [contract](methodology/dispatch-contract-20261106.md)
contains the frozen numeric limits for 25/s ordinary and 50/s qualification runs.
Do not tune those limits after observing platform results.

Generator/receiver headroom is qualified at twice the ordinary rate with the
relevant adapter, recording and resource settings. Monitoring remains enabled.
Retain generator CPU, memory, throttling, VUs and sampler coverage; an ACK-rate
chart alone cannot prove the offered workload or receiver capacity.

| Situation | Interpretation |
|---|---|
| Healthy full capture; platform returns errors or fails to deliver | Potentially valid platform failure; retain the evidence |
| Sink overflow, incomplete request ledger or incompatible clocks | Measurement invalid or affected claim unassessed |
| Missing history export | History unassessed; not evidence of zero loss |
| Proven setup failure with zero offered messages | No measured platform delivery result |
| Interrupted controller | Outcome unresolved until retained artifacts and cleanup are checked |

Malformed platform output and local parser/capture failures require attribution:
neither all errors nor all unparsed bodies automatically mean instrumentation
failure. Reporting an unfavorable valid outcome is part of the benchmark.

## 5. Windows, cutoffs and percentiles

The ordinary workload has 300 s warm-up and 600 s measurement, both at 25/s.
Assess warm-up separately rather than silently dropping unfavorable evidence.
Retain the declared measurement-relative 900 s drain ceiling, quiet interval
and separate late-observation window. Their exact anchoring and attestation
requirements are part of the contract and capture artifacts.

Per-run measurement p50/p95/p99 use nearest-rank quantiles of individual first
receipts by cutoff. These values are conditional on receipt. Publish the
scheduled/dispatched denominators, received count and missing fraction alongside
them: a small received subset can have low p99 while most messages are missing.
With no received messages, latency is null, not zero. Late receipts remain
missing-by-cutoff even when later observed.

Never subtract aggregate percentiles, average minute p99s into a run p99, or pool
messages across runs as independent system replications. Report per-run values
and replicate variation. Five planned attempts are not proof of a reliability
probability; failed attempts and reused deployment/database state must be disclosed.

## 6. CPU and history claims have additional limits

CPU accounting uses per-container boundary-counter differences with container
identity/restart checks and an explicitly named denominator. Prometheus rate
integrals are diagnostic, not a replacement. Remote snapshot brackets introduce
phase-alignment uncertainty; a valid counter delta is not automatically exact
measurement-phase CPU cost. Missing counters or snapshots cannot be reconstructed
from a later inventory. See the
[CPU capture contract](development/tooling-interfaces.md#cpu-boundary-snapshot-contract).

Post-run history read-back establishes visibility at that observation, not a
per-message visibility latency distribution. A source timestamp or database row
timestamp is not automatically a commit timestamp. Long-term retention and
crash recovery need separate experiments.

## 7. Protocol provenance and reading results

The archived preregistration, dated owner decisions and prospective dispatch
amendment describe different stages of the study. Preserve the originals;
explain amendments rather than retroactively changing the source requirements.
The older owner-approved 500 ms scheduled-to-sink profile and its
250/1,000/5,000 ms sensitivities do not automatically impose the same threshold
on the newer dispatch endpoint. The cited external TS 22.369 basis remains
unverified in the [reconciliation record](../research/protocols/slo-profile-reconciliation-20261029.md).
Project-selected thresholds must not be presented as standards-derived SLOs.

When reading a result, establish these in order:

1. Which frozen plan, policy, image/configuration and source bytes produced it?
2. Were the workload, capture, clock and resource checks qualified?
3. Which identities were received by cutoff, late, duplicated or missing?
4. Was independent history available and correctly reconciled?
5. Which endpoint/cohort does each percentile describe?
6. Which comparisons and claims does the tested envelope actually support?

Use the [ordinary runbook](operations/ordinary-campaign.md) for execution and
recovery, [offline analysis reference](development/offline-analysis.md) for
claim-specific outputs, [test catalogue](development/testing.md) for engineering
verification, and [publication guide](operations/publication-handoff.md) for
evidence distribution. A software test pass, a valid run, an SLO pass and a
completed campaign are distinct conclusions.

## 8. Workload configurations: B0 and the separate W1–W4 batch

The study compares fixed operating points, not maximum capacity. **B0** is the
ordinary telemetry workload already being executed. The owner approved four
additional configurations, each changing one treatment setting relative to B0.
They apply identically to ThingsBoard, FIWARE and EdgeX. Their plans are staged
in a **separate batch**; staging and offline resolution are not launch clearance
or measured results. Existing B0 identities, in-progress attempts and frozen
configuration remain unchanged.

### Settings and planned message counts

| ID | Question / changed setting | Devices | Total messages/s | Extra logical padding | Replicas per replicated service | Warm-up / measurement messages |
|---|---|---:|---:|---:|---:|---:|
| B0 | Ordinary telemetry baseline | 1,000 | 25 | 0 bytes | 2 | 7,500 / 15,000 |
| W1 | What changes at a higher fixed offered rate? | 1,000 | 50 | 0 bytes | 2 | 15,000 / 30,000 |
| W2 | What changes with more active device identities at the same total rate? | 2,000 | 25 | 0 bytes | 2 | 7,500 / 15,000 |
| W3 | What changes when each message carries more logical data? | 1,000 | 25 | 1,024 bytes | 2 | 7,500 / 15,000 |
| W4 | What changes with one replica sharing the same CPU budget? | 1,000 | 25 | 0 bytes | 1 | 7,500 / 15,000 |

All use constant open-loop arrivals, round-robin device selection, **300 s
warm-up and 600 s measurement at that workload's rate**. Rates are totals across
devices, not per-device rates. Thus the intended per-device interval is 40 s for
B0/W3/W4, 20 s for W1 and 80 s for W2. Planned load is not proof of achieved
physical dispatch or successful delivery; retain the accounting in sections 3–5.

The application pool remains **cores 4–5, budget k=2, including W4**. Replica
count N and CPU budget k are separate settings: W4 is N=1/k=2, not a one-core
experiment. Keep the HAProxy ingress hop at N=1 as well as N=2. Supporting
services, image identities and per-replica resource limits are not new treatment
variables. Fewer replicas reduce the configured aggregate application memory
limit, not necessarily observed usage; report that consequence rather than calling W4 a fixed-total-
memory or CPU-scaling comparison.

Shared HTTP/capture settings remain keep-alive, 5 s request timeout, no application
retries, 150 preallocated/max virtual users per scenario, `GOGC=400`, monitoring
enabled, generator cores 0–1, sink core 2, and load-host monitoring core 3. Retain
the measurement-relative 900 s drain ceiling, 30 s quiet interval at cutoff and
separate 30 s late observation. A virtual user is an execution slot, not a device.

### Payload size is not wire size

`payload.size_offset_bytes=1024` selects exactly **1,024 extra ASCII logical
padding bytes**, relative to the same message's canonical zero-padding form.
The same encoder feeds all adapters. Platform wrappers and automatic HTTP
headers still differ: record canonical logical bytes, actual serialized adapter
body bytes and scoped measured transport/wire bytes separately. Document whether
wire measurement includes headers, TCP/IP or retransmissions. The existing
request-head arithmetic is an estimate, not an actual packet capture.

For identical identity fields, JSON adapter bodies grow by 1,024 bytes, while
FIWARE's native UltraLight body can grow by 1,022: its zero-padding value uses
two quote characters that are replaced by nonempty padding. Logical padding
still grows by exactly 1,024. Offline tests establish this encoding behavior;
live sink/history preservation and an approved actual-wire measurement method
remain W3 gates. Never substitute regenerated expected padding for the actual
payload read back from a platform.

The implementation gap is concrete: the current FIWARE sink and QuantumLeap
history subscriptions select only identity/timing attributes, and the sink's
stored receipt record has no padding field. The existing evidence therefore
cannot prove end-to-end padding preservation, even though adapter construction
is tested. Any payload-preserving route/capture change needs review, matching
qualification and common-batch comparability treatment—not an undisclosed
W3-only instrumentation change or an edit to active B0.

### Repetitions, interleaving and evidence

B0 retains its original five attempts per platform. The **proposal** for the new
batch is five attempts per workload/platform: **60 experimental attempts**, not
60 guaranteed successes. Each repetition rotates the workload order and the
platform order within each workload. Seeds and identity widths are matched to
the corresponding B0 repetition, while every new attempt has a unique run/device
cohort. Diagnostics and qualification receive separate IDs and are not counted
as repetitions. The full proposed order is in the
[separate matrix](../workloads/campaigns/fixed-points-w1-w4/run-matrix.json).

Use the same offline application-dispatch → first complete-body receipt join
and independent history reconciliation. Archive workload ID, platform, run ID,
resolved generator settings, observed deployment/replicas/resources, source and
image identities, requests, receipts, clock/resource evidence, history and the
separate validity/platform verdicts. Group tables by **workload and platform**;
report per-run nearest-rank percentiles, their receipt denominators and missing,
late, duplicate, malformed/truncated and UNKNOWN outcomes. Do not mix W1–W4 into
B0 repetition summaries or pool messages into a single cross-run p99.

### Qualification and interpretation limits

- **W1:** prove full-duration 50/s recording/capture under the actual adapter.
  The shorter B0 50/s qualification does not establish this. The adopted 2×
  headroom rule also raises a 100/s evidence/policy question: the current
  dispatch evaluator supports only 25/50/s. Resolve it prospectively, without
  editing the active B0 policy or using a W1 outcome as its own qualification.
- **W2:** verify all 2,000 intended devices on every active replica. EdgeX must
  include retained historical cohorts in discovery-capacity and memory checks;
  no device/history deletion. The current 16,384 discovery setting is below the
  projected full new-batch population; live inventory and a reviewed common
  discovery configuration are prerequisites, not demonstrated infeasibility.
- **W3:** verify exact logical padding and actual sink/history payloads; qualify
  capture and applicable headroom for the larger payload, with byte scopes named.
- **W4:** verify exactly one active replica of each replicated service, effective
  two-core pool 4–5 and the same HAProxy path. Do not inherit an N=1 → k=1 default.

The existing B0-only launcher and platform-only aggregation are not a W1–W4
execution/reporting release. Those interfaces need separate workload-aware gates
and review. The [batch preparation guide](../workloads/campaigns/fixed-points-w1-w4/README.md)
lists the unresolved cases and safe offline reproduction commands.

Retained deployment/history state and chronology can confound comparisons:
B0 precedes the new batch, and interleaving does not remove every time effect.
Any necessary common discovery-setting amendment must be disclosed against B0,
not silently applied to W2 alone or backfilled into old runs. These experiments
can describe this deployment at the stated points; they cannot establish a
maximum device count, throughput boundary, general platform ranking or full
scalability curve.
