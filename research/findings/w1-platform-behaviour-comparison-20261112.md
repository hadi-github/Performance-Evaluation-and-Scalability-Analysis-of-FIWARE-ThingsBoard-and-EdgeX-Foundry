# What W1's 50/s workload did to each platform

**Date:** 2026-11-12
**Workload:** W1 — 1,000 devices, 50 messages/s, 300 s warm-up + 600 s measurement,
payload `logical-json-ascii-v1` with `size_offset_bytes=0`, 5,000 ms request timeout,
300-VU rate-derived pool.

A side-by-side account of the same offered load against three platforms, written as a
reference for interpreting platform behaviour rather than as a capacity claim. **All
three runs are single observations.** None is replicated, and none supports a
capacity statement.

---

## 1. The one table

| | ThingsBoard | FIWARE | EdgeX |
|---|---:|---:|---:|
| Dispatched (measurement) | 30,000 | 30,000 | 30,000 |
| Acknowledged < 5 s | 30,000 | **16,519** | 30,000 |
| Timed out (`unknown`) | 0 | **13,481** | 0 |
| Explicit failures | 0 | 0 | 0 |
| Dropped (generator) | 0 | 0 | 0 |
| Missing at cutoff | 0 | **20** | 0 |
| Persisted to history | 30,000 | 29,980 | 30,000 |
| p50 dispatch→receipt | 109.8 ms | 3,455.6 ms | **4.6 ms** |
| p95 | 245.2 ms | 23,544.9 ms | 6.7 ms |
| p99 | 347.4 ms | **35,338.0 ms** | **8.5 ms** |
| Verdict | `VALID_PLATFORM_SUCCESS` | `VALID_PLATFORM_FAILURE_OR_BACKLOG` | `VALID_PLATFORM_SUCCESS` |

The p99 spread is **~4,150×** between EdgeX (8.5 ms) and FIWARE (35.3 s).

## 2. ThingsBoard — absorbed it, with a queue-shaped tail

- Everything arrived. All 30,000 acknowledged inside the timeout; all 30,000 persisted.
- The distribution is the interesting part: p50 110 ms, p95 245 ms, p99 347 ms. A tail
  roughly **3× the median** is the signature of a service that queues under load and
  drains steadily rather than one that saturates.
- Never close to the 5 s timeout. No unknowns, no missing identities.
- **Caveat: warm start.** The stack had just served the `d` generation. Warm-up ack p99
  was 625 ms versus 3,969 ms cold. This says nothing about cold-start behaviour at 50/s.

**Reading:** comfortably inside its envelope at this shape. The tail shape suggests
headroom was still being consumed, but one run cannot quantify it.

## 3. FIWARE — did not absorb it; it fell behind and never caught up

- **13,481 of 30,000** measurement sends exceeded the 5 s timeout — 45%.
- **13,461 of those 13,481 eventually arrived anyway.** They were not lost at the
  platform; they arrived *late*.
- p99 dispatch→receipt was **35.3 s against a 5 s timeout — about 7×.**
- 20 identities never arrived before the cutoff, and the same 20 are missing from
  history. Those are genuinely lost.
- Zero explicit failures, zero generator drops, no malformed bodies.

**Reading:** this is the distinctive "backlog, not loss" signature. A platform that
drops under load shows `failure`/connection errors. FIWARE showed **zero failures** and
kept accepting — it simply could not keep up, so a request queue grew without bound and
requests timed out while still queued. The measured p99 of ~35 s is roughly how far
behind the queue was by the end.

**Generous reading:** with a much longer timeout, this same traffic might have been
recorded as slow-but-complete. **That is not what the frozen shape asks for** — the
cell is defined with a 5,000 ms timeout, and 45% of requests breached it.

**Important negative could have fooled a naive check:** the wrapper printed `VALID`
because `dropped_iterations` was 0. Zero drops is a *generator* statement, not a
platform one. The platform verdict comes from the dispatch-integrity analysis.

## 4. EdgeX — absorbed it with almost no queueing at all

- Everything arrived, everything persisted, nothing timed out.
- p50 4.6 ms, p95 6.7 ms, p99 8.5 ms — a **~1.8× tail from median**, which is the
  shape of a system responding directly with little queueing.
- Setup gate: both replicas accepted all 1,000 devices on their **first** readiness
  attempt, zero repair restarts, zero core-metadata panics.
- Zero `dropped_iterations`, zero saturation, all 10 generator-resource checks true.

**Reading:** at this shape EdgeX was not stressed. A sub-9 ms p99 against a 5 s timeout
means the timeout never entered the picture. This says the workload is comfortably
inside its envelope; it does **not** establish where the envelope ends.

## 5. Why the three differ so much (hypotheses, not findings)

Offered to guide the next experiment, explicitly flagged as unverified:

1. **Ingest path length.** EdgeX `device-rest` takes an HTTP POST and hands it to NATS;
   the response completes early. FIWARE's path is Orion → subscription notification →
   IoT Agent in the measurement loop's contract, which is materially more work per
   message.
2. **What "ack" means per adapter.** The endpoint under test is
   `application_dispatch_to_response_complete_body_read`. The three platforms do
   different amounts of *durable* work before returning that body. Comparing their
   latencies compares different amounts of committed work — a cross-platform latency
   ranking is therefore **not** a like-for-like performance ranking.
3. **FIWARE's subscription fan-out.** W1 has one sink subscription plus the history
   subscription. A doubling of notifications per message would compound exactly the way
   the observed backlog did.

None of these is established by this batch. They are the obvious next things to
isolate.

## 6. What these runs do not support

- **No capacity claim.** One run per platform, one shape.
- **No cross-platform ranking.** Different ack semantics; see §5.2.
- **No cold-start statement** for ThingsBoard.
- **No replication.** W cells need five repetitions to be comparable; these are
  qualifications, deliberately not fixed-point results.
- **No claim that FIWARE is unusable.** It is a claim about 50/s at *this* shape with a
  5 s timeout, and nothing more.

## 7. Artifacts
- `results/workload-batch-20261108/reports/w1-summary.md` — the machine-derived table
- `results/workload-batch-20261108/reports/w1-latency-percentiles.svg`
- `results/workload-batch-20261108/reports/w1-ack-completeness.svg`
- `results/workload-batch-20261108/reports/w1-throughput-timeline.svg` — per-second 50/s integrity
- Regenerate with `python3 scripts/render-w1-charts.py`

## 8. Related

- `w1-qualified-thingsboard-20261111.md`
- `w1-fiware-fails-at-50s-20261111.md`
- `w1-edgex-qualified-20261112.md`
- `edgex-readiness-measured-20261111.md`
