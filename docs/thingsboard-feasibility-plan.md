# ThingsBoard feasibility plan (D11 → D32, S4 → D38)

**Status: local arithmetic and a bounded check plan. Nothing is executed on the SUT. No k is qualified.**
Recorded 2026-10-26 under D32 (ChatGPT under the owner's delegation, D41). Hardware budget unchanged: 8 vCPU / 16 GiB.

Evidence labels: **[V]** verified in this checkout or retained evidence; **[P]** proposal; **[G]** unverified.

---

## 1. What is being decided

- Target: **k ∈ {1, 2, 4}**, where k is the shared application-tier CPU-core budget (D26, D35). At each k every
  ThingsBoard application tier runs k replicas: `tb-core`, `tb-rule-engine`, `tb-http-transport` (D26).
- **No k=4 / N=4 configuration is qualified.** [V] D11 is still open for N=4.
- Fallback k ∈ {1, 2, 3} may only be **proposed** after measured feasibility and a separately recorded deviation
  (D32, D38). It is not adopted by this document.

## 2. Memory arithmetic (local, from compose defaults and the handoff review) [V]

Source values: `thingsboard/compose.yaml` (`${TB_CORE_MEM:-1g}`, `${TB_RULE_ENGINE_MEM:-1g}`,
`${TB_HTTP_TRANSPORT_MEM:-512m}`, infra limits) and `docs/handoff-review-2026-10-25.md` (the same arithmetic,
§"ThingsBoard at N=4"). Budget = 16 GiB − 1,536 MiB reserve = **14,848 MiB** (`scripts/check-resources.sh`).

Per replica of the application tier, tracked defaults: 1,024 + 1,024 + 512 = **2,560 MiB**.
Infra tier (not replicated): Postgres 1,536 + Kafka 1,280 + ZooKeeper 512 + Valkey 256 + HAProxy 256 = **3,840 MiB**.

| k (replicas per tier) | App tier, tracked defaults | Total, tracked defaults | Headroom to 14,848 MiB | Result |
|---|---|---|---|---|
| 1 | 2,560 | 6,400 | 8,448 | fits on paper |
| 2 | 5,120 | 8,960 | 5,888 | fits on paper |
| 3 | 7,680 | 11,520 | 3,328 | fits on paper (fallback candidate) |
| 4 | 10,240 | **14,080** | **768** | fits on paper only; see §3 |
| 4, exploratory overrides (1,536 / 1,536 / 768 per replica) | 15,360 | **19,200** | −4,352 | **does not fit 16 GiB** |

The `tb-install` one-shot (1,536 MiB, `profiles: install`) is excluded, as in the handoff review.

## 3. Why "fits on paper" is not enough (D11 history)

- **[V] D11:** at the 1 GiB tracked default, `tb-core` ran at about 970 MB of 1 GB while idle; under load this is an
  OOM risk. The exploratory run `EXPL-tb-k4` was OOM-killed at 1 GiB during provisioning of 1,000 devices.
- **[V] D11-a:** the exploratory override (1,536 / 1,536 / 768 MiB, heap unchanged) was a deviation from the tracked
  defaults. It does not fit at k=4 (table above).
- **[V] D28:** the official configuration and a writable log volume are authorized, **but D28 does not authorize a
  memory-limit change and does not validate OOM recovery.** Feasibility checks therefore record OOM evidence from the
  container state, not from application logs alone.

Consequence: at k=4 the tracked defaults leave 768 MiB of host headroom and a `tb-core` limit already near its idle
RSS. Whether k=4 is feasible is a **measurement question**, not an arithmetic one, and any memory change is an owner
decision.

## 4. Bounded check plan (not authorized to run)

Each stage needs the SUT (currently unreachable, blocker B1) and a **separate run authorization**, which has not been
granted. Each stage runs at the tracked defaults and changes nothing it measures.

| Stage | Config | Setup | Stop rule (recorded, not tuned) | Records |
|---|---|---|---|---|
| F0 | local only | arithmetic (this document) | — | done |
| F1 | k=1, tracked defaults | provision 1,000 devices (not measured); idle 5 min | any OOM kill, any container restart, or a limit > 95% for 60 s | `docker inspect` OOMKilled and RestartCount per container, per-container memory peak, `docker stats` samples |
| F2 | k=2, tracked defaults | as F1 | as F1 | as F1 |
| F3 | k=4, tracked defaults | only if F1 and F2 pass **and** the owner authorizes k=4 | any OOM kill → k=4 **not feasible** at defaults | as F1, plus the host headroom at peak |
| F4 (fallback, conditional) | k=3, tracked defaults | only if F3 fails and the owner records the deviation | as F1 | as F1 |

Rules for every stage:

- No memory limit, heap setting or replica count is changed during a stage. A change is a new configuration with its
  own stage and its own deviation record.
- The stage runs the **same** provisioning and load protocol as the rest of the campaign (1,000 devices, open-loop
  HTTP), so any result is comparable. Load rates for a feasibility stage are taken from the existing pre-registered
  sweep rules, not chosen to look good.
- Results go to `results/feasibility/` or `results/diag/` with a `benchctl`-style manifest, so they are not confused
  with qualified runs.
- The generator must satisfy its own qualification (D34) before its load counts as evidence; a generator failure is
  recorded as INVALID (generator), never as a ThingsBoard OOM.

## 5. Methodological implications (S1 / S4)

- k is the CPU budget and replicas are recorded per tier (D35). At k=4 the defaults give four `tb-core` replicas
  sharing one cpuset. E(k) is normalized to k and must be reported with the replica topology.
- If only k ∈ {1, 2, 3} is feasible, the RQ-B scaling curve has **no k=4 point** for ThingsBoard. Any claim must say so.
  The fallback is a deviation from the S4 target, not a silent substitution (D38).
- A k=1 or k=2 OOM is a **platform finding under the tracked memory limits**, not a generator or measurement
  artefact. It is reported as such.

## 6. Local preparation that is in scope now

- This arithmetic table, reproducible from `thingsboard/compose.yaml` and `scripts/check-resources.sh`.
- The bounded stage definitions above.
- No script that starts or stops ThingsBoard is added in this stage. The feasibility stages reuse the existing
  `thingsboard/` Makefile and `benchctl`; they are not yet wired into a runner. [G]

## 7. Open items

- B6: F1–F3 not executed (SUT unreachable and not authorized). k=4 unqualified.
- B7: D28 official configuration and writable log volume not yet implemented in `thingsboard/compose.yaml`.
- The k=3 fallback has not been measured and has no deviation record.
