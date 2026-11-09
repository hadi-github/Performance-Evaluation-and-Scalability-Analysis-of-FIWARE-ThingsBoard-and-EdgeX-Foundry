# Research study guide

This page connects the implementation to a research-paper structure. It explains
the bounded ordinary study, not the full original capacity/scaling programme.
It does not introduce new hypotheses, thresholds or permission to run experiments.
Execution progress and accepted evidence remain in [status](status.md).

## 1. Research questions

The bounded comparison addresses these descriptive questions:

1. At the same offered load and application CPU budget, what delivery latency
   and missing/duplicate outcomes does each configured platform produce?
2. Do HTTP acknowledgements, external delivery and independent history read-back
   tell the same story about successful processing?
3. Which conclusions remain supported after workload and measurement validity
   checks are applied?

These questions explain the current scope; they are not a retroactive
preregistration. The original questions include capacity, scaling and overload.
Those require additional experiments and must not be answered using this single
operating point. See the [original framing](../research/preregistration/preregistration-archive/original/decisions.md).

## 2. What the project contributes

The implementation provides:

- A common logical-message contract with platform-specific HTTP adapters.
- Identity-based joins across requests, sink receipts and history exports.
- Separate assessments of offered workload, capture quality and platform outcomes.
- Frozen plans, retained failed attempts and repeatable offline analysis.

These are implemented capabilities, not a claim of literature-wide novelty.
A paper still needs a verified related-work comparison covering IoT platform
benchmarks, open-load measurement, SLO-based benchmarking and identity-based
auditing. Compare their workloads, endpoints, resources and reproducibility.
Do not use an unverified paper summary to claim this is the first such system.

## 3. Experimental setup and fairness

| Item | Bounded ordinary design |
|---|---|
| Platforms | FIWARE + QuantumLeap/TimescaleDB; ThingsBoard; EdgeX |
| Hosts | One 8-vCPU/16-GB SUT; separate load/measurement host |
| Application allocation | Shared CPU pool `4-5`, k=2; N=2 deployment setting |
| Workload | Constant open-loop HTTP, 25 messages/s, 1,000 devices |
| Payload | `logical-json-ascii-v1`, size offset 0; not a zero-byte message |
| Phases | 300 s warm-up; 600 s measurement |
| Attempt plan | Five per platform, interleaved; failures are retained |
| Primary endpoint | Application dispatch to first complete-body sink receipt |
| History endpoint | Independent identity read-back after the run |

Source: [frozen scope](../results/feasible-campaign-20261106/frozen-scope.json).
The load is a conservative project-selected operating point, not a universal
deployment requirement or a measured maximum. Warm-up, duration and attempt count
are inherited design choices, not proof of stationarity or statistical precision.

Equal CPU allocation does not make platform semantics identical. Report native
wire sizes, actual replicas, memory limits, supporting-service costs, queue/retry
settings and database configuration. Preserve the logical content rather than
claiming every adapter has identical encoding. Reproducible device assignment
and HTTP keep-alive are part of the workload, not independent experimental factors.

The paper's setup table must also give actual CPU models, OS/kernel versions,
network characteristics, image digests, effective configuration hashes and source
revision. Take them from retained run evidence, not example defaults or a later
machine inspection. See [system design](architecture.md) for paths and rationale.

## 4. Experimental procedure

1. Freeze scope, identities, seeds, source and effective configurations.
2. Qualify the matching generator/receiver/adapter path at 50/s for 2× headroom.
3. Check isolation, service readiness, resource limits and capture ownership.
4. Execute warm-up and measurement; retain complete individual request outcomes.
5. Observe through the declared drain/cutoff and late window; export history.
6. Analyze workload, clocks, capture, delivery and history separately.
7. Verify cleanup and review the result before the next original matrix entry.

Use the [operator runbook](operations/ordinary-campaign.md) for exact controls.
Do not silently replace failed attempts, change thresholds after results, or
count qualification runs as ordinary repetitions. Reused database/deployment
state must be disclosed; these are not cold-start IID repetitions.

### Protocol evolution

| Record | Meaning and affected scope |
|---|---|
| Original preregistration | Broader capacity/scaling/overload programme and inherited telemetry deadlines |
| 2026-11-01 owner mappings | Scheduled-to-sink 500 ms profile, storage-correctness criteria and device-population comparison |
| `DISPATCH-20261106` | Marked future runs use dispatch-to-complete-body receipt as primary; scheduled delay stays secondary |

The dispatch change excludes pre-dispatch scheduling/preparation from primary
latency and replaces the universal 10 ms lag rejection with declared workload
checks. It does not change historical verdicts. Preserve the
[decision log](architecture/design-decisions.md) and
[dispatch contract](methodology/dispatch-contract-20261106.md); do not transfer
an old threshold to a different endpoint without an explicit decision.

## 5. Analysis and results

Compute nearest-rank p50/p95/p99 per run from first receipts by cutoff. Publish
the received numerator, scheduled/dispatched denominators, missing fraction,
duplicates, late receipts, UNKNOWN outcomes and history findings beside latency.
Low conditional p99 alone does not establish successful delivery.

Summarize ordinary per-run p99 with min/median/max and observed replicate
variation. Do not pool messages as independent experiment repetitions. Do not
claim statistical significance, a confidence level or a reliability probability
without an appropriate declared analysis and sufficient evidence. The current
design does not establish a capacity bracket or scaling efficiency.

The paper's main result table should include **every attempted cell**:

```text
run ID | platform | validity | received/scheduled | missing/duplicates/late
       | dispatch p99 | history outcome | cleanup/recovery status
```

Keep setup failures, invalid measurements and valid platform failures distinct.
An interrupted launcher is unresolved until its artifacts are reconciled. The
campaign is incomplete in the documented handoff; consult [status](status.md)
instead of treating this guide as a final results table. Do not rank platforms
from isolated successful runs or explain a bottleneck without supporting evidence.

### Claims and required evidence

| Claim | Evidence needed | Limit |
|---|---|---|
| Message delivered | Exact request/sink identity join | By a declared cutoff |
| Message history-visible | Independent exported identity | Visible at export, not first-visible time |
| Lower latency in this comparison | Comparable valid per-run distributions and adequate repetition evidence | Tested configuration only |
| Lower CPU cost | Valid boundary counters, alignment bounds and named denominator | Unavailable where those captures are missing |
| Greater capacity or scalability | Qualified load sweep or resource-scaling experiment | Not established by this batch |

## 6. Threats to validity

- **Generalization:** one host, one fixed load and selected versions do not
  represent every deployment, cluster or workload.
- **Fairness:** equal CPU does not equal memory, durability or queue semantics.
- **Timing:** same-host clocks still need checks; 1 ms dispatch resolution is
  not wire-level or sub-millisecond accuracy.
- **Selection:** conditional latency can hide missing messages unless counts
  and unsuccessful attempts are reported.
- **Replication:** retained state and incomplete repetitions limit independence
  and uncertainty claims.
- **Instrumentation:** monitoring/recording can affect load; missing resource
  or capture evidence cannot be reconstructed after the fact.
- **Known evidence limits:** disclose reconstructed ThingsBoard qualification
  provenance and unavailable phase-aligned CPU cost where applicable.

No claim of crash durability, safety-critical timeliness, long-term retention
or per-message commit latency follows from the present observations.

## 7. Reproducibility and publication checklist

- Identify the exact source commit, dirty state, plans, seeds and policy version.
- Publish reviewed effective configurations and image identities.
- Archive request/sink/history inputs, capture/clock/resource evidence and all
  attempted outcomes with immutable checksums.
- Supply the exact offline commands and expected output/checksum references.
- Separate synthetic regression fixtures from real measurements.
- Provide a stable reviewed evidence-archive location; raw data is not in Git.
- Complete licence, citation and full-history secret review before publication.

See [testing](development/testing.md), [measurement details](methodology.md) and
[publication requirements](operations/publication-handoff.md). A software test
pass is engineering evidence, not a completed scientific experiment.
