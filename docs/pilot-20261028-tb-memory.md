# ThingsBoard memory-sizing pilot protocol (auto-20261028, TEMPORARY pilot — not adopted settings)

**Status: pilot only.** F1's stop-condition record (k=1, 425 s + 329 s >95% at 1 GiB) and F2's result (k=2,
no ≥60 s >95% interval, cores riding ~96–97%) are preserved as-is and are NOT reinterpreted here.

## Question

Is the binding constraint the **container limit** (1 GiB) rather than the **JVM heap** (`-Xmx512m`)?
Reasoning to test: the JVM's fixed non-heap parts (MaxMetaspaceSize 256m + ReservedCodeCache 96m +
MaxDirectMemorySize 128m + thread stacks + GC structures) plus the 512m heap sum to ≈ 0.95–1.0 GiB — exactly
the observed steady-state. If so, raising the container limit **without touching the heap** should drop the
utilization percentage proportionally and remove the >95% sustained condition, while the heap behaviour
(and therefore GC pressure) stays identical.

## Variants (temporary, pilot-only; heap and all other settings unchanged)

| Variant | tb-core | tb-rule-engine | transport | Applied by |
|---|---|---|---|---|
| V0 (reference, already recorded) | 1024 MiB | 1024 MiB | 512 MiB | F1/F2 evidence — not re-run |
| V1 | 1280 MiB | 1280 MiB | 512 MiB | `TB_CORE_MEM=1280m TB_RULE_ENGINE_MEM=1280m docker compose up -d --force-recreate tb-core tb-rule-engine` |
| V2 | 1536 MiB | 1536 MiB | 768 MiB | additionally `TB_HTTP_TRANSPORT_MEM=768m … --force-recreate` on all three services |

Registry: **reused as-is** (2,013 devices from auto-20261027; no new provisioning for the pilot). If any
variant needs new devices, a unique prefix (`pilot-vN-`) and a fixed recorded count with a resume-safe guard
would be used — not expected here.

## Observation windows per variant (monitor v2 writer, 4 s samples, gaps logged)

1. **Recreate+boot** — monitor starts BEFORE `up --force-recreate`; ends at the boot marker
   (`Started ThingsboardServerApplication` on both cores) + 60 s.
2. **Idle with populated registry** — 5 min, no load.
3. **Bounded telemetry** — official generator path from the load host:
   `run.sh thingsboard 2 1m --devices 100 --warmup 30s` (creates its own `bench-<testid>` devices and
   deletes them in teardown; run dir is new/unique).
4. **Post-load idle** — 3 min.

JVM decomposition (where instrumentation permits): TB exposes actuator Prometheus metrics
(`METRICS_ENABLED=true`); sampled every ~15 s during windows 2–4: `jvm_memory_used_bytes{area=heap}` and
`area=nonheap`, plus container cgroup `memory.peak` (lifetime counter) and docker-stats current.

## Stop conditions (per variant, same semantics as the stage plan)

Any OOM kill, any container restart, or a sustained >95%-of-limit interval ≥ 60 s (strict analyzer:
elapsed-time based, ≥16 samples at 4 s spacing, continuity broken at gaps > 8 s) → the variant's result is
recorded and the sequence stops escalating at that variant. Between variants the stack returns to the next
variant's limits via `--force-recreate` (volumes and data preserved throughout).

## Fit arithmetic (for the recommendation; host budget 14,848 MiB = 16 GiB − 1,536 reserve)

Per replica = core + rule-engine + transport. Infra tier fixed at 3,840 MiB.

| Variant | per replica | k=1 | k=2 | k=3 | k=4 |
|---|---|---|---|---|---|
| V0 1024/1024/512 | 2,560 | 6,400 ✓ | 8,960 ✓ | 11,520 ✓ | 14,080 ✓ (768 headroom — the D11 problem) |
| V1 1280/1280/512 | 3,072 | 6,912 ✓ | 9,984 ✓ | 13,056 ✓ | 16,128 ✗ over budget |
| V2 1536/1536/768 | 3,840 | 7,680 ✓ | 11,520 ✓ | 15,360 ✗ | 18,944 ✗ |

So a raised-limit recommendation can fit **k ≤ 3 (V1)** or **k ≤ 2 (V2)**; k=4 does not fit either variant —
that constraint is reported, not silently adopted.

## Outputs

Per variant: monitor CSV + `.gaps`, strict interval analysis, cgroup peaks, JVM heap/non-heap samples,
telemetry smoke result (VALID/INVALID + p95), OOM/restart states. Recommendation at the end of the session
handoff, labelled **proposed**, with the k-fit constraint.
