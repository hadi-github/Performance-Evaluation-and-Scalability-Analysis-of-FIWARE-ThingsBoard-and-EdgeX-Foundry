# Methodological reconciliation: original pre-registration versus adopted operations

**Status: dated amendment record (2026-10-26).** This document explains every material difference between the
pre-registered specification in `docs/preregistration/` and the operational definitions adopted in
`docs/design-decisions.md`. It is **not** a new pre-registration, it does not change a hypothesis or a threshold,
and it does not retroactively claim that the revised wording was the original.

Authority order:

1. `docs/preregistration/original/` — the original text, byte-for-byte.
2. `docs/design-decisions.md` D23, D26, D31–D42 — the adopted operational decisions.
3. This document — the reconciliation between 1 and 2.

If this document and an original file disagree about what the original said, the original file wins.

---

## 1. Summary

| ID | Topic | Original | Adopted | Decision | Claim effect |
|---|---|---|---|---|---|
| S1 | Meaning of the scaling factor | N = app-tier replicas, 1 pinned core each; E(N) = C(N)/(N·C(1)) | k = shared app-tier CPU-core budget; replicas recorded separately; efficiency normalized against k | D26, D35 | E is per allocated core, not per replica; topology must accompany every result |
| S2 | Goodput and persistence | Stored exactly once and within budget; stop at sink receipt | Four distinct observation points; correctness-qualified goodput requires sink delivery **and** retrievable history; D23 verdict preserved | D23, D36 | No per-message durable-commit claim; aggregate proxies are labelled |
| S3 | Generator headroom | 2× the highest planned rate | 2× retained; D24's 1.5× superseded for this gate, not erased | D24, D34, D37 | A rate qualified under 1.5× is not qualified under the adopted rule |
| S4 | ThingsBoard scaling range | N ∈ {1,2,4}; fallback N ∈ {1,2,3} if N=4 does not fit | Target k ∈ {1,2,4}; fallback only after measurement and a recorded deviation | D32, D38 | A fallback is a reported deviation, never a silent substitution |
| S5 | Experimental protocol | 1,000 devices, open-loop, 5-min warm-up, sweep, 10-min confirm, ≥5 runs, uncertainty | Preserved; exploratory 120 s/180 s runs remain exploratory | D39 | Five runs are a benchmark verdict, not a reliability guarantee |

---

## 2. S1 — Scaling semantics

### Original wording

`docs/preregistration/original/rqs.md` §0:

> **Resource budget N** — App-tier replicas, **1 pinned core each**, N ∈ {1, 2, 4}. The stateful tier is **fixed at
> 3 pinned cores** for all N.

> **E(N)** — Scaling efficiency: C_SLO(N) / (N × C_SLO(1)).

`docs/preregistration/original/fingerprint_notes.txt` §2 repeats the model: `N` app cores, `N` replicas per
replicated tier, `total service cores = B_s + N`.

### Adopted wording

`docs/design-decisions.md` D26 and D35:

- **k** is the number of CPU cores in the shared application-tier cpuset pool.
- Replica counts are recorded separately for every tier.
- All replicas of a tier share the same cpuset; they are not exclusively pinned one core each.
- Resource-normalized efficiency is computed against **k**, and the actual replica topology is reported alongside
  every scaling result.

Operationally: **E(k) = C_SLO(k) / (k × C_SLO(1))**, with the replica count printed next to every point.

### Justification

The deployed stacks pass one `${POOL_CPUSET}` to every replica of a tier (`fiware/compose.yaml`,
`edgex/compose.yaml`, `thingsboard/compose.yaml`). At k = 4 with four replicas, four containers share four CPUs;
the scheduler does not reserve one core per replica. Reporting "N replicas = N cores" would therefore describe a
topology that was not deployed. D26 records what is actually run.

### Effect on the research claim

- RQ-B's question must be restated as "how much does capacity grow when the application-tier **CPU budget** grows
  from 1 to 2 to 4 cores, with the stateful tier fixed?" The replica count is a reported configuration variable.
- E(2) and E(4) remain comparable across platforms **only if** the replica topology is also reported.
- The original N-based E values cannot be reconstructed from a shared-cpuset run without re-running with exclusive
  pinning. No such re-run exists.
- **This is a dated amendment.** It must be cited as such (2026-10-26, D35) in every report. It was not the
  original pre-registration.

### Open consequence

If a reader or reviewer requires the original replica-based definition, the only honest options are (a) run an
exclusive-pinning variant as a separate, labelled experiment, or (b) report the original N-based E as
**unavailable**. Neither is authorized by this document.

---

## 3. S2 — Goodput, delivery and persistence

### Original wording

`docs/preregistration/original/rqs.md` §0:

> **Goodput G** — Messages/s that are stored exactly once **and** arrive within the platform time budget.

> **Stop** — the sink receives the message through the platform's outbound path.

`docs/preregistration/original/slo_notes.txt` §1 is stricter about what a sink receipt proves:

> It does not measure time to durable commit... A sink receipt does not prove the historical record was written,
> and vice versa.

### Adopted definitions

`docs/design-decisions.md` D23 (already adopted) defines goodput as durably stored and delivered, evaluates EdgeX's
outstanding-work criterion and reconciliation at persistence, and keeps the sink as the latency observation point.
D36 adds the following append-only clarification. Five quantities are now named separately:

| # | Quantity | Evidence | May be reported as |
|---|---|---|---|
| 1 | HTTP request acceptance | 2xx outcome in the generator ledger | ack rate; not delivery, not storage |
| 2 | Unique sink delivery | first `(run_id, seq)` receipt in the sink stream | delivery, with the sink as the observation point |
| 3 | Historical storage identity | `(run_id, seq)` retrievable from the platform history store | stored identity; not commit latency |
| 4 | **Correctness-qualified goodput** | identity present in (1) acknowledged **and** (2) sink **and** (3) history, within the SLO latency condition | the correctness-qualified measure |
| 5 | Aggregate persistence-rate proxy | cumulative table counters, e.g. EdgeX `n_tup_ins` | a rate/goodput proxy only |

**Correctness-qualified goodput** is therefore: the count of individually verified message identities that are
acknowledged, uniquely delivered to the required sink observation, retrievable from historical storage, and whose
delivery latency is within the applicable deadline, divided by the measured window. It is reported as a count-based
rate and is **not** a substitute for the D23 pass/fail verdict.

### What does not change

- D23's verdict rule and its EdgeX persistence evaluation are preserved.
- The pass/fail criteria in the pre-registered SLO (loss, errors, p99, non-growing backlog) are unchanged.
- Sink-only and history-only counts are still reported, but never merged into a durability claim.

### What may not be claimed

- **No per-message commit latency.** None of the three platforms exposes a per-message durable-commit timestamp in
  this setup. History proves retrievability, not the time of a durable commit.
- **No ack→sink identity join** before D33's implementation passed qualification. The implementation now exists
  locally; until it is exercised on a qualified run, delivery remains an aggregate statement.
- **No "sink receipt = stored"** inference. FIWARE and EdgeX notify the sink in parallel with persistence; the sink
  can receive a message that the store later fails to keep.

### Effect on the research claim

The original "goodput" and the adopted "correctness-qualified goodput" are not numerically identical. Reports must
state which one is being shown. Where history is unavailable (FIWARE before D31 is deployed), only quantities 1, 2
and 5 can be reported, and quantity 4 is **unavailable**, not estimated.

---

## 4. S3 — Generator headroom

### Original wording

`docs/preregistration/original/workloads.txt` §6:

> Headroom (W1): qualified rate >= 2 x highest planned lambda.

`docs/preregistration/original/slo_notes.txt` §9 G1 repeats: generator + sink sustain ≥ 2× the maximum planned λ
against a null endpoint, with all validity checks passing.

### Conflicting adopted text

`docs/design-decisions.md` D24 (2026-10-22) says the generator "must reach ~1.5x the largest capacity".

### Adopted resolution

D34 and D37: **2× governs**. D24's 1.5× wording remains in the log as history; it is superseded for the
generator-headroom gate effective 2026-10-26.

### Consequences

- A layout measured only to 1.5× is not qualified.
- The 2× test must include the sink and monitoring running as they will during the experiment, not a null endpoint
  alone.
- If k6 cannot reach 2×, the measured limit is reported and D24's Go-generator question is reopened. The benchmark
  rate is not silently reduced, and the rule is not weakened.
- The original W3 overload headroom is 2.4×; it is a separate requirement and is not reduced by this resolution.

---

## 5. S4 — ThingsBoard scaling range

### Original wording

`docs/preregistration/original/rqs.md` RQ-B:

> **Known constraint:** ThingsBoard at N = 4 may not fit in 16 GB. Cap the JVM heap at ~1 GB per tb-node. If it
> still doesn't fit, use N ∈ {1, 2, 3} for ThingsBoard and report this.

### Adopted position

D32 and D38:

- The target remains **k ∈ {1,2,4}**.
- No k=4/N=4 configuration is qualified.
- The k ∈ {1,2,3} fallback may be **proposed** only after a measured feasibility check; it requires a separately
  recorded deviation and must not be adopted by implication.
- The hardware budget (8 vCPU / 16 GB) and the supporting-tier accounting are unchanged.

### Effect on the claim

A ThingsBoard result at k ≤ 3 is a **reported deviation**, not a three-platform k=4 comparison. E(4) for
ThingsBoard is unavailable unless a k=4 configuration is measured. The pre-registered RQ-B wording already
anticipated this; the deviation record makes it explicit rather than silent.

---

## 6. S5 — Experimental protocol

### Preserved protocol

From `docs/preregistration/original/rqs.md` §"Experimental protocol" and `workloads.txt`:

| Element | Preserved value |
|---|---|
| Devices | 1,000 logical devices, all provisioned before measurement |
| Workload | Open-loop, constant-rate HTTP telemetry (`W1`) |
| Warm-up | 5 minutes at 20% of the previous C_SLO estimate |
| Coarse sweep | Geometric steps ×1.5, 3 minutes each, until the SLO breaks |
| Refine | Linear steps between the last pass and the first fail |
| Confirmation | 10-minute steady state at the candidate C_SLO |
| Repetitions | ≥5 independent runs, randomized order; continue up to 10 until the 95% bootstrap CI of C_SLO is within ±5% |
| Latency reporting | Per-run p50/p95/p99 with explicit uncertainty |
| Primary deadline | TELEM-500 (500 ms platform budget) |
| Sensitivity | 250 ms, 1,000 ms, 5,000 ms, post hoc on the same data |
| Validity | Zero dropped iterations; achieved rate within ±1%; sink and generator healthy |

### Clarifications

- **Five passing runs do not prove a 95% reliability guarantee.** `fingerprint_notes.txt` states the statistical
  meaning explicitly: 5/5 gives a one-sided 95% Clopper–Pearson lower bound of about 0.55. It is a conservative
  benchmark verdict, not proof of reliable compliance and not ±5% precision.
- **The ±5% figure is a stopping rule for the CI of C_SLO**, not a guarantee that any five runs achieve it.
- **The exploratory 120 s step and 180 s confirmation durations are not the protocol.** They remain exploratory and
  must be labelled as such. Confirmation cells must use the 10-minute window unless a dated deviation is recorded.
- **Sensitivity is re-analysis, not extra runs.** The sweep must extend far enough to locate the 5,000 ms knee.

### Divergences that remain operational, not methodological

These are already recorded in the repository and do not change the research definitions:

| Divergence | Record | Effect |
|---|---|---|
| All replicas share one cpuset (S1) | D26/D35 | Topology reported with every scaling point |
| HAProxy always present and part of the supporting tier | README, `scripts/resources.mk` | Constant across platforms; not a treatment |
| Single stateful instance, not replicated | README | Bottleneck location is a finding, not a defect |
| HTTP only | Workload specification | MQTT is out of scope |
| Official images with documented deviations | Each `compose.yaml` header | Deviations are listed per platform |
| EdgeX NATS-enabled build | `edgex/README` / compose header | Official build flag, no source change |
| k6 image via a registry mirror | `docs/generator.md`, manifests | Digest pinned; mirror is not a different generator |

---

## 7. Other differences between the original and the repository

| # | Original | Repository / adopted | Where recorded |
|---|---|---|---|
| O1 | Telemetry stop = sink receipt | D23 adds a persistence-based verdict for EdgeX; sink stays as latency point | D23, D36 |
| O2 | Durability check after every run on `ts_kv`, QuantumLeap, EdgeX | FIWARE history not implemented yet; D31 selects the architecture | D31, §5.1 of the sign-off |
| O3 | HAProxy placement open in the fingerprint notes (D3) | HAProxy on the LB cpuset, counted in the supporting tier | README, `scripts/resources.mk` |
| O4 | Stateful tier "3 pinned cores" | Deployed supporting tier includes HAProxy and per-platform services; the accounting is fixed at 3,840 MiB and three cores | `scripts/check-resources.sh`, sign-off §5.2 |
| O5 | Overload anchor open (D1: 1.2·C_ref or 1.2·X_max) | Not frozen by this stage; remains an open decision for the overload campaign | fingerprint notes §9 |
| O6 | Capacity uncertainty method open (D2) | 5/5 + pass fractions adopted implicitly by the protocol; the full-search alternative is not adopted | D39, fingerprint notes §9 |

---

## 8. What this document does not authorize

- It does not authorize deployment, restart, tuning, live traffic or a benchmark run.
- It does not change a threshold, a hypothesis, an acceptance criterion, the hardware budget or a durability rule.
- It does not make the original text match the implementation.
- It does not convert an unavailable measurement into an estimate.

**Every report must cite the amendment IDs it relies on: S1/D35, S2/D36, S3/D37, S4/D38, S5/D39.**
