# FIWARE benchmark path with the QuantumLeap/TimescaleDB history candidate

Recorded 2026-10-29 after the `REH-FIWARE-V4` rehearsal (`results/rehearsals-20261029/`). Supersedes
the "candidate only" reading of `fiware-history-design.md` for the ingesion path question;
that design document's own status line ("local design only, not deployed") refers to its
proposed optimal layout, several points of which remain undone (that design's direct
`orion→both-subscriptions` wiring, with agents inside the history-candidate stack, is still NOT the
adopted layout).

## 1. The path that is live and verified

```
generator (k6/gen.js on the load host)
  └─ HTTP :7896  →  HAProxy (baseline, `deploy/fiware/haproxy`)        ← measured path stays behind the LB
       └─ IoT Agent UL (baseline, one replica; registry in baseline MongoDB)
            └─ Orion (baseline, :1026, "openiot" service)
                 ├─ subscription "bench QL"      → QuantumLeap 1.0.0 (fiwhc candidate, :8668)
                 │                                  → TimescaleDB (fiwhc, adopted 1536+512 MiB tier)
                 └─ subscription "bench sink"    → external observation sink (scripts/outbound.sh fiware)
```

- Ingest is exercised through the **full benchmark path**: the generator communicates only with
  HAProxy UL :7896 (and provisions through the IoT Agent's north port :4041); Orion is never written
  to directly by the generator. Verified live in `REH-FIWARE-V4`: 3,000/3,000 measurement-window
  identities reconciled at all three observation points (ack, sink, Timescale export), 0 missing,
  0 duplicates, 0 orphans, delivery p99 313 ms at 25 msg/s.
- History retrieval is the read-only `scripts/history-ids.sh fiware` export against
  `fiwhc-timescale-1` (database `quantumleap`); it produces one identity row per stored QL row with
  `device` and `ts`, so the same-millisecond collision analysis runs on FIWARE history the way it
  does on ThingsBoard.

## 2. Compatibilities (each measured once live)

| Pairing | Evidence |
|---|---|
| Baseline Orion 4.5.0 → IoT Agent UL (ingest) | 2026-10-27 stack verification + every rehearsal use |
| Baseline Orion 4.5.0 → QuantumLeap 1.0.0 notifications | `REH-FIWARE-V4` (subscriptions registered and delivering) |
| fiwhc Orion 4.5.0 → fiwhc QL 1.0.0 (isolated pairing) | B2/B3 (2026-10-29, deliberate drop preserved) |
| QL 1.0.0 insert semantics | per-notification rows, **no upsert**, same-timestamp duplicates keep separate rows (source review `docs/architecture/fiware-history-design.md` section 3) |
| History tier budget | adopted 2026-10-29: Timescale 1536 MiB + QuantumLeap 512 MiB inside the fixed supporting tier (B4) |

## 3. Baseline vs candidate — what each stack is

- **`fiware` baseline (`deploy/fiware/`, project `fiware`)**: the *ingestion* benchmark path
  (HAProxy, IoT Agent, Orion, MongoDB). Its persistence contract remains "latest value only" — it is
  not a history store, and the rehearsal never claims it is.
- **`fiware-history-candidate` (`deploy/fiware/fiware-history-candidate/`, project `fiwhc`)**: the
  *history* pair (QuantumLeap + TimescaleDB, volumes `fwc-*`). It is **downstream** of the baseline
  Orion via the "bench QL" subscription; removing or starting it changes history coverage, never the
  ingest path. Its volumes are preserved and only its orion/quantumleap services are started/stopped.

## 4. Higher-rate qualification limits (measured ceiling of current knowledge)

| Statement | Basis |
|---|---|
| Full-stack identity reconciliation is exact at **25 msg/s** (120 s measurement window) | `REH-FIWARE-V4` |
| Store-side identity reconciliation (fiwhc export) exact at **50 msg/s** (probe path `hist-load-20261029`, 3,001/3,001) | `results/auto-20261029/fiware/` |
| History delivery p99 313 ms at 25 msg/s; QL/Timescale instantaneous memory 77/512 and 137/1536 MiB under the 50 msg/s probe | rehearsal + probe evidence |
| **Nothing above 50 msg/s is qualified for the FIWARE history path.** The rehearsal sweep must be repeated at the target rate (200 msg/s ceiling per the current authorization) with the same identity reconciliation before the FIWARE history tier is claimed at campaign rates | — |

Known open questions for that extension, in the order they are most likely to bind:

1. Orion→QL notification throughput and its p99 under 200 msg/s (protocol is HTTP 1.1 per device update);
2. Timescale insert batch latency/throughput (synch mode, no Redis offload in the confirmed configuration);
3. memory of both candidate services at 200 msg/s against the adopted 1536/512 MiB split;
4. the 65 s subscription-cache refresh when Orion is scaled to multiple replicas (only matters
   if the FIWARE baseline runs at k > 1 during a campaign cell; registration trimmed accordingly).
