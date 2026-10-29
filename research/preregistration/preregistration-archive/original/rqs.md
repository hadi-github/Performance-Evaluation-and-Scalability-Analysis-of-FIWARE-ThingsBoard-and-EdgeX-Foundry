# Research Questions (v2, locked Oct 16, 2026)

Framing: a JSys Tool/Benchmark paper. It proposes an SLO-based scalability fingerprint and compares it against conventional IoT benchmark metrics. FIWARE, ThingsBoard and EdgeX are case studies.

Every hypothesis below is **pre-registered**: its thresholds are fixed before any data exists. Changing them later means reporting the change explicitly.

---

## 0. Shared definitions

| Term | Definition |
|---|---|
| Offered rate λ | Messages/s sent by the generator at a fixed rate (constant or Poisson), independent of how the system responds. This is an open model. |
| Goodput G | Messages/s that are stored exactly once **and** arrive within the platform time budget. |
| SLO profile p | An end-to-end latency L_p from a standard, plus loss = 0 and errors ≤ 0.1%. |
| Platform time budget | **50% of L_p**, pre-registered. Sensitivity analysis at 25% and 100% uses the same data (re-analysis only). |
| Percentile | p99 of platform latency, measured per run. |
| Resource budget N | App-tier replicas, **1 pinned core each**, N ∈ {1, 2, 4}. The stateful tier is **fixed at 3 pinned cores** for all N. |
| C_SLO(p, N) | Highest λ that, over a 10-minute steady state, keeps p99 ≤ budget, loss = 0, errors ≤ 0.1%, and a non-growing backlog. |
| L_knee | λ at which p99 first exceeds 2× its low-load value. |
| G_peak | Maximum goodput observed across the sweep, even when the SLO is violated. |
| E(N) | Scaling efficiency: C_SLO(N) / (N × C_SLO(1)). |
| Fingerprint F | (C_SLO, E₂, E₄, L_knee, G_peak, R_overload, T_recovery), reported per platform per profile. |
| Run | One independent execution: cold start, DB wipe, warm-up, measurement. **The run is the statistical unit**, not the request. |

### SLO profiles
| ID | Source | End-to-end | Platform budget (50%) |
|---|---|---|---|
| P1 smart grid | 3GPP TS 22.369, Table 6.3-1 | 1 s | **500 ms** |
| P2 | TBD (suggested: smart city, TS 22.369, 10 s) | — | — |
| P3 | TBD (optional: AMI, TR 22.867, 3 s) | — | — |

### Telemetry semantics (what is measured)
- **Start:** the generator's send timestamp, carried in the payload together with a sequence number.
- **Stop:** the sink receives the message through the platform's outbound path:
  - ThingsBoard: `Save Timeseries` → on Success → `REST API Call` → sink. This fires only after persistence.
  - FIWARE: IoTA → Orion. Subscription #1 goes to QuantumLeap (TimescaleDB), subscription #2 goes to the sink. These run in parallel.
  - EdgeX: device-rest → message bus → core-data (persistence ON). In parallel, the bus feeds an app-service HTTP export → sink.
- **Durability check after every run:** stored count == sent count, with 0 duplicates. Check ThingsBoard `ts_kv`, the QuantumLeap table and EdgeX core-data.
- **Payload:** identical across platforms. ~100 B: 3 numeric fields, a timestamp and a sequence number. HTTP first; MQTT only if time allows.

### Testbed
- **SUT** (8 cores / 16 GB, platforms only, one platform at a time):
  - 1 core: OS + Docker + exporters
  - 3 cores: stateful tier
  - 4 cores: app replicas
- **Load machine** (4 cores / 8 GB):
  - generator: 2 cores
  - sink: 1 core
  - monitoring: 1 core
  - The generator and sink share one clock.

### Experimental protocol (common to all RQs)
1. Cold start, DB wipe, health gate passes for **every** replica.
2. Warm-up: 5 minutes at 20% of the previous C_SLO estimate.
3. Step-up rate sweep: geometric steps (×1.5), 3 minutes each, until the SLO breaks.
4. Refine: linear steps between the last pass and the first fail.
5. Confirm: 10-minute steady state at the candidate C_SLO.
6. Repeat: **≥ 5 runs**, randomized order. Continue up to 10 runs until the 95% bootstrap CI of C_SLO is within **± 5%**.
7. Load-generator check: the generator's own CPU stays < 70% and the achieved rate is ≥ 99% of the offered rate. Otherwise the run is invalid.

---

## RQ-E (headline): Conventional metrics vs fingerprint

**Question:** Do conventional IoT benchmark metrics lead to the same conclusions as the SLO-based fingerprint?

**Why this is a contribution:** If the methods disagree, current IoT benchmarking practice produces misleading conclusions. That is a methodological finding that generalizes beyond these three platforms.

**Conventional metrics** (computed from the **same runs**, so no extra experiments):
- throughput at a fixed request volume (device × message counts)
- average response time
- per-node CPU%
- error rate at fixed volume levels (low, medium, high)

**Hypothesis H-E (pre-registered):** The two methods disagree on **at least one** of the following:
1. The platform ranking by "performance": conventional average latency or throughput, vs C_SLO(P1, N=1).
2. The scaling verdict: falling per-node CPU% suggests improved efficiency, while E(N) < 0.75.
3. The stability verdict: "0% errors at high volume" while the run fails the C_SLO steady-state criterion.

**Decision rule:** H-E is supported if at least one disagreement holds for at least one platform, outside the 95% CIs.

**Reviewer attack:** "That's a strawman." **Defence:** cite prior work that reports exactly these metrics: Araujo et al. 2019, the SBRC 2020 FIWARE/Konker/ThingsBoard study, Ismail et al. 2018, and the authors' own earlier draft.

---

## RQ-A: Sustainable capacity

**Question:** What is the sustainable capacity of each platform's official deployment under each SLO profile, at N = 1?

**Why this is a contribution:** It replaces arbitrary "load levels" with a capacity that is comparable across platforms and tied to an application requirement.

**Metrics:**
- C_SLO(p, 1) with a 95% CI
- L_knee and G_peak
- curves of p99 vs λ and goodput vs λ

**Hypothesis H-A (pre-registered):** For each platform, across platform thresholds {250 ms, 500 ms, 1 s, 5 s}:

    (max C_SLO − min C_SLO) / max C_SLO < 10%

- **Rationale:** Queueing theory (λ* = μ − 1/T*) predicts < 1% spread. The 10% margin covers batching (ThingsBoard defaults to 100 ms), GC pauses and tail effects. 10% is also wider than the ±5% CI target, so the hypothesis is testable.
- **Supported:** the knee, not the threshold, determines capacity.
- **Falsified:** threshold sensitivity on platform Y. Its root cause is analysed under RQ-C.

**Reviewer attack:** "The platforms do different work." **Defence:** the common semantics above, with each platform's persistence point documented. FIWARE stores history via QuantumLeap, so all three persist every reading.

---

## RQ-B: Scaling efficiency

**Question:** How much does sustainable capacity grow when the app tier goes from 1 to 2 to 4 pinned cores, with the stateful tier fixed?

**Why this is a contribution:** It measures scale-out against a fixed hardware accounting, instead of per-node utilization percentages.

**Metrics:**
- E(2) and E(4) with CIs
- msgs/s per core (app tier only, and total SUT)
- aggregate CPU-seconds per 1,000 messages

**Hypothesis H-B (pre-registered):** For every platform, **E(2) > E(4)** and **E(4) < 0.75**.

- **Rationale:** The stateful tier is fixed at 3 cores, so by Amdahl-style reasoning the serial share caps the speed-up.

**Decision rule:** Supported if both inequalities hold outside the CIs for all 3 platforms.

**Known constraint:** ThingsBoard at N = 4 may not fit in 16 GB. Cap the JVM heap at ~1 GB per tb-node. If it still doesn't fit, use N ∈ {1, 2, 3} for ThingsBoard and report this.

**Reviewer attack:** "Pinned cores on one host are not real nodes." **Defence:** cpuset pinning, stated in threats to validity, and the network hop is constant across N.

---

## RQ-C: Bottlenecks

**Question:** Which component limits capacity at each N, and does the bottleneck move as N grows?

**Why this is a contribution:** It turns numbers into explanations, and explanations generalize.

**Evidence collected at every knee:**
- per-container CPU vs its limit
- DB operation latency and connection-pool waits
- queue depth and lag (Kafka, RabbitMQ, NATS)
- JVM GC time (ThingsBoard)
- disk IOPS and latency

**Ablation (one per platform):** Hold the app tier at N = 2.
- **Baseline:** stateful tier at 3 cores.
- **Variant:** stateful tier at 5 cores, borrowing the 2 unused app cores.

**Hypothesis H-C (pre-registered):** Replicas raise capacity only while a stateless tier is the bottleneck. Beyond that point, a stateful component (DB or bus) caps capacity.

**Decision rule:** H-C is supported for a platform if both hold:
1. At the N where E(N) drops, a stateful component is saturated: CPU ≥ 90% of its limit, or queue lag grows during steady state.
2. In the ablation, the variant raises C_SLO by ≥ 15% over the baseline. (15% is chosen to exceed CI noise by ~3×.)

**Reviewer attack:** "Correlation is not cause." **Defence:** the ablation changes one factor and shows whether capacity follows.

---

## RQ-D: Overload and recovery

**Question:** Under transient overload, does each platform degrade gracefully, and how quickly does it recover?

**Why this is a contribution:** Benchmarks usually stop at saturation. Real IoT bursts exceed it.

**Load profile** (relative to each platform's own C_SLO(P1, N)):

    0–10 min   50%  →  10–20 min   80%  →  20–25 min   120%  →  25–35 min   70%

Run at N = 1 and N = 4.

**Metrics:**
- R_overload = goodput during the 120% phase / C_SLO
- T_recovery = time from the end of overload until the SLO holds for 2 consecutive minutes (timeout: 10 minutes)
- peak backlog, lost messages, duplicates, container restarts

**Hypothesis H-D (pre-registered):** Overload behaviour depends more on the platform's queueing design than on replica count:

    |R_overload(platform X) − R_overload(platform Y)|  >  |R_overload(X, N=1) − R_overload(X, N=4)|

This should hold for the majority of platform pairs, and likewise for T_recovery.

- **"Graceful"** is defined as R_overload ≥ 0.9 and T_recovery ≤ 120 s.

**Reviewer attack:** "One overload shape isn't representative." **Defence:** the profile is illustrative and published as a file, so others can run their own.

---

## Threats to validity (per RQ, to expand in the paper)

- **RQ-A / RQ-E:**
  - The 50% budget rule is an assumption (sensitivity analysis provided).
  - 3GPP "availability" ≠ request success rate.
  - Single payload type.
- **RQ-B:** pinned cores on one host; tested versions only; ThingsBoard's possible N cap.
- **RQ-C:** the ablation varies only the stateful-tier cores, not its configuration.
- **RQ-D:** one overload shape.
- **All:** one hardware type; HTTP only unless MQTT is added; official deployment configurations as of the pinned digests.
