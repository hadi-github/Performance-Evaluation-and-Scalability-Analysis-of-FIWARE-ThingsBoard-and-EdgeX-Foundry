# Finding: ThingsBoard in batch B0 — five cells, four measured, three failure mechanisms

Observed 2026-11-06 to 2026-11-09. Batch B0 attempted the five ThingsBoard cells its frozen
matrix defines. **Four produced a valid measurement and one did not**, and the five failures
are not one mechanism: four are a ThingsBoard delivery gap, three are harness/pre-traffic
faults. This document exists to keep those apart, because conflating them would both
overstate what ThingsBoard did wrong and understate how strong its actual signal is.

The platform defect — the first-contact egress stall — is what fails the
`dispatch-integrity-v1` policy on ordinals 6, 8, 10 (attempt A2) and 14. It is **not**
random flakiness: it reproduces with a recognisable shape every time.

> **Revision note (2026-11-10).** Three claims in the earlier draft were wrong and have
> been corrected against the raw sink/history exports: (a) the backlog does *not* settle to
> ≤0.75 s by offset ~50 s — three of four runs still show 1.5–2.0 s outliers immediately
> after the drain point; (b) the `Timeout to process` burst window is 20:27:44.963–20:28:27.614,
> not "20:27:38–20:28:30"; (c) r01 was described as "never offered traffic" without saying
> *why* — the workload was launched ~27 s into a ~215 s boot and HAProxy answered 503. None
> of these changes a verdict.

> **Draft correction (retained).** An earlier version explained the gap as the rule-engine
> tier "lazily initialising its rule-chain runtime on first contact". The platform logs
> disprove that for the process as a whole: the rule-engine JVM was fully started at
> **20:23:58**, three minutes and forty seconds *before* the generator's first tick, and had
> already processed 199 messages with zero timeouts. That claim has been removed. What
> survives is a narrower and better-evidenced statement — the *telemetry rule-chain path*
> (not the JVM) is cold at first contact — and the internal reason for the 31–39 s cost
> remains unidentified. See "Root cause, stated honestly".

## One-line summary

At the first telemetry of a run, the ThingsBoard rule-engine → sink delivery path stalls.
**No message at all reaches the sink for 30.9–39.3 s** — verified identical across all four
runs: zero receipts before t=30 s, and in r02/r03/r04b zero before t=31 s. Then the path
flushes its accumulated backlog in a decaying burst and tracks the schedule exactly. The
generator has already started a fixed 25/s schedule and mostly gets HTTP ACKs, so the stall
appears as 126–200 missing identities — always on the leading edge of the warmup, never in
the measurement window.

## All five cells: state and why each one failed

This is the complete ThingsBoard account for batch B0. Five cells were attempted, one per
ordinal. Four produced a valid measurement; one did not. Across those five cells the record
contains **seven run IDs** (ordinal 10 needed three), and the failures are **not one
mechanism** — they split into three, and only one of them is ThingsBoard's own behaviour.

| Ordinal | Run | Result | Offered | Why it failed |
|---:|---|---|---:|---|
| 1 | `disp-tb-r01-20261106` | `NOT_ASSESSED_SETUP_FAILURE` | **0** | **Harness.** Workload launched ~27 s after app containers started, against a ~215 s boot. HAProxy frontend was up but no backend was, so the first k6 login got `503 No server is available`. No traffic, no measurement. |
| 6 | `disp-tb-r02-20261106` | `VALID_PLATFORM_FAILURE_OR_BACKLOG` | 22,500 | **Platform.** First-contact egress stall: first sink arrival at 30.9 s, 200 warm-up identities never delivered. |
| 8 | `disp-tb-r03-20261106` | `VALID_PLATFORM_FAILURE_OR_BACKLOG` | 22,500 | **Platform.** Same stall: 39.3 s, 160 missing. *(Separately, its in-band history export failed — an evidence defect, not the cause of the verdict.)* |
| 10 | `disp-tb-r04-20261106` → `r04a` → `r04b` | `NOT_ASSESSED` → `NOT_ASSESSED` → `VALID_PLATFORM_FAILURE_OR_BACKLOG` | 0 → 0 → 22,500 | **Harness, then platform.** `r04`: diagnostic helper on `:18085` had died (`curl` exit 7, connection refused). `r04a`: helper alive but its aggregate `/snapshot` returned 503 (`empty project snapshot: fiware`) because it snapshots all platforms and the inactive ones fail it. `r04b` finally measured — and hit the **platform** stall: 39.1 s, 144 missing. |
| 14 | `disp-tb-r05-20261106` | `VALID_PLATFORM_FAILURE_OR_BACKLOG` | 22,500 | **Platform.** Same stall: 31.0 s, 126 missing. |

So the tally is **4 valid measured repetitions out of 5 cells**, and the three failure
mechanisms stack like this:

| Mechanism | Cells | Owner |
|---|---|---|
| First-contact egress stall (30.9–39.3 s) | r02, r03, r04b, r05 | ThingsBoard behaviour |
| Launched into a still-booting platform | r01 | Harness (no application-readiness gate) |
| Diagnostic helper dead / aggregate snapshot 503 | r04, r04a | Harness / infrastructure |

Two conclusions follow directly, and they are the point of this document:

1. **ThingsBoard failed every time it was actually measured** — 4 for 4, same shape. That
   is a real, reproducible property of the system under test at this operating point, and
   it is the reason the policy returns `VALID_PLATFORM_FAILURE_OR_BACKLOG`.
2. **The fifth repetition was lost to the harness, not to ThingsBoard** — r01, r04 and r04a
   died before traffic, for reasons outside the platform. Those attempts are not evidence
   about ThingsBoard in either direction; they are simply the reason the count is 4 not 5.

The rest of this document is about mechanism 1, because it is the only one that is a
property of ThingsBoard and the only one that recurs.

## Why the other three failed — and why that is not ThingsBoard's fault

These are included because they are part of the state of TB in B0, and because they had been
described only loosely. None of the three is a measurement of the platform.

### Ordinal 1 (`disp-tb-r01`): launched into a platform that had not finished booting

The record says "first k6 setup login HTTP 503". The logs show why:

```
23:21:12–23:21:14  kafka, zookeeper, postgres, valkey start   (infra, "healthy")
23:21:37           tb-rule-engine / tb-core start
23:21:39–23:21:40  tb-http-transport ×2, haproxy start
23:21:46           launcher takes its pre-traffic resource snapshot  (passes)
23:22:04           k6 first login  ->  HTTP 503 "No server is available to handle this request"
```

ThingsBoard takes **215 s** to boot (measured on this same stack in r05:
`Started ThingsBoard in 215 seconds`). The workload was launched **27 s** after the
application containers started — roughly 188 s early.

The relevant detail is that the 503 came from **HAProxy**, not from ThingsBoard. The
message `No server is available to handle this request` is HAProxy's "backend has no
healthy server" page. HAProxy's frontend binds immediately; its backends only become
usable once `tb-http-transport` is actually listening. So the launcher's readiness check
was satisfied by infrastructure that was up (containers running, `:18085` answering) while
the application behind it was not ready to serve a login.

This is a **harness gap, not a platform defect**, and it was correctly repaired: the
launcher now requires both HAProxy backends to report `UP` before traffic
(`wait_tb_backends`, backed by the `tb-haproxy-before.prom` snapshot each run writes). That
gate is what makes r02–r05 trustworthy — every later TB run provably started against a
fully booted platform, with `haproxy_backend_status{proxy="tb-core",state="UP"} 1` and the
same for `tb-http-transport` in the retained snapshot. It also means r01's 503 should never
be cited as evidence about ThingsBoard.

*(Evidence note: `disp-tb-r03`'s copy of `tb-haproxy-before.prom` was present on the SUT but
missing from the local checkout; it has been synced. The aggregate was always built from SUT
evidence, so no reported number was affected.)*

**0 of 22,500 messages were offered.** The k6 scenario aborted at setup, before the warm-up
step began: `ticks_complete: 0 of 7500`, `achieved_rate: 0 of 7500`, k6 exit 108. The sink
and history ledgers are empty by construction. There is nothing to measure and nothing was
measured — not a clean run, not a failed run.

### Ordinal 10 (`disp-tb-r04`, `disp-tb-r04a`): the diagnostic helper, not the platform

Both attempts died before traffic for reasons in the surrounding instrumentation:

- **`r04`** — the read-only capture helper on `:18085` was not listening at all. The
  launcher's `curl` returned exit 7 (`Failed to connect ... Couldn't connect to server`).
  Confirmed by the recovery sidecar as "SUT inventory endpoint port 18085 unavailable".
- **`r04a`** — the helper had been restarted and was listening, but its aggregate
  `/snapshot` endpoint returned **503: `empty project snapshot: fiware`**. The helper
  snapshots ThingsBoard *plus* FIWARE, FIWARE history and EdgeX in one call; with only
  ThingsBoard active, the inactive projects made the aggregate call fail. The launcher
  aborted deliberately rather than proceed without the CPU boundary snapshots.

Neither is a statement about ThingsBoard. Neither was offered traffic
(`workload_launch_attempted: false`, 0 messages). A third attempt, `r04b`, was separately
authorized as `TB-R04-A2` with an explicit review receipt, and *that* one ran and produced
the platform failure described above. It is worth being precise that `r04b` is a
**supplement to ordinal 10, not a retry of a ThingsBoard failure** — the two prior attempts
never reached the platform.

## What ordinal 8's evidence status actually is

Ordinal 8 measured validly, but its post-run verification was incomplete, and that must not
be smoothed over:

| Check | Status | Consequence |
|---|---|---|
| In-band history export | `EXPORT_FAILED` | Sink-vs-history reconciliation unavailable *in band* |
| Late history retrieval (2026-11-09) | `OK_LATE_RETRIEVAL`, 22,500 rows, `sha256 d58f07c1…` | Reconciliation **was** completed offline: `history_missing: 0` in both phases |
| Post-run `effective-resources` snapshot | 180 s timeout on `docker compose config` | No after-snapshot comparable to ordinal 7's |
| `sut_resource_qualified` | `False` | `campaign_qualified: False` |

So ordinal 8's *numbers* are sound and independently reconciled — the 160 missing
identities are provably all present in history. What failed is the campaign's
*qualification* of the run: it cannot be called campaign-qualified with a missing
post-run resource snapshot. It counts as a valid measurement but not as a qualified cell.

The 180 s `docker compose config` timeout is the same failure class as ordinal 7's EdgeX
evidence hole: a post-run snapshot that never got written. It is the harness's weakest
point, not a platform property.

## The signature (all four runs)

| Run | Date | First sink arrival | Missing | Unknown | ACKed | Warmup p99 | Measurement p99 | Measurement |
|---|---|---:|---:|---:|---:|---:|---:|---|
| `disp-tb-r02-20261106` | Nov 6 | **30.9 s** | 200 | 36 | 7464 | 33787 ms | 203 ms | 15000/15000, 0 missing |
| `disp-tb-r03-20261106` | Nov 7 | **39.3 s** | 160 | 134 | 7366 | 38550 ms | 218 ms | 15000/15000, 0 missing |
| `disp-tb-r04b-20261109` | Nov 9 | **39.1 s** | 144 | 73 | 7427 | 41488 ms | 215 ms | 15000/15000, 0 missing |
| `disp-tb-r05-20261106` | Nov 9 | **31.0 s** | 126 | 37 | 7463 | 33757 ms | 281 ms | 15000/15000, 0 missing |

Five constants, in every run:

1. **The gap is 30.9–39.3 s** and it is *absolute*: no message reaches the sink at all before
   it. Measured per run: 0 receipts before t=30 s in all four; before t=31 s, 4 of 22,300 in
   r02, 0 of 22,340 in r03, 0 of 22,356 in r04b, 1 of 22,374 in r05.
2. **100 % of missed identities are in warmup.** Not one measurement-period tick has been
   missed in any TB run — measurement is 15,000/15,000 with 0 missing in all four. The
   missing identities are confined to the first 9.9–23.1 s of scheduled warm-up traffic.
3. **They were never delivered at all — not "delivered late".** No message arrives after the
   900 s schedule ends: the last receipt is 0.0–0.1 s after the last scheduled send in every
   run. So the 200/160/144/126 are genuinely absent from the sink stream, which is why they
   are counted as missing rather than as backlog.
4. **`history_missing` is 0, and every sink-missing identity is present in history.** Verified
   against the raw history exports: 200/200, 160/160, 144/144, 126/126. The platform
   persisted everything. This is a *delivery-timing* failure, not data loss.
4. **The backlog fully drains, but not in one step.** Lag decays monotonically from the
   stall — 44 → 35 → 27 → 19 → 11 → 3 s across the first minute (r02; the others are
   within a few seconds of this curve). p99.9 lag falls below 1 s at scheduled offset
   **48–63 s** depending on run, and the last sub-second lag residue clears by ~1.5–2 s.
   After that the run is flat: measurement-phase p99 is 203–281 ms and the final message
   is delivered within 0.1 s of its scheduled time in all four runs.
   *(An earlier draft said "lag drops below 0.5 s by offset ~50 s and then holds at
   ≤0.75 s". That is wrong for three of the four runs — r02, r03 and r04b still show
   1.5–2.0 s outliers just after the drain point. The corrected statement is the one
   above; the important fact is unchanged — it recovers and never recurs.)*
5. **Generator dispatch was on time.** `workload_integrity` records 25 dispatches in every
   1 s bin from the first second, `dispatch_spill: 0`, `valid: true`. The gap is downstream
   of the generator, not in it.

## What the platform logs actually show (`disp-tb-r05-20261106`)

Times are UTC, reconstructed from `docker inspect` and `docker logs --timestamps`.

```
20:19:43  postgres, kafka start
20:20:09  tb-core-1 starts
20:20:10  tb-rule-engine-1 starts
20:20:12  tb-http-transport-1/2, haproxy start
20:23:56  Tomcat started on port 8080
20:23:58  "Started ThingsBoard in 215 seconds"        <- rule engine fully booted
20:25:56  consumer stats: totalMsgs=112, successful=112, timeoutMsgs=0
20:26:57  consumer stats: totalMsgs=199, successful=199, timeoutMsgs=0
          ~3 min 40 s of idle, fully booted platform after 20:23:58
20:27:38.499  core: Queue-0 [TS] totalAdded=2 totalSaved=2 totalFailed=0  <- DB path already warm
20:27:38.524  generator sends seq 0                 <- t = 0.00 s (0.025 s later)
20:27:43.078  first AntiSamy policy load (rule-dispatcher)
20:27:44.963  first "Timeout to process [...] messages"
20:28:09      first row arrives at the sink         <- 31.0 s after seq 0
20:28:27.614  last "Timeout to process" (202 lines on this replica)
20:28:57      consumer stats: timeoutMsgs=369 (peak)
20:29:57      consumer stats: timeoutMsgs=0         <- recovered, never recurs
```

Three facts from those logs matter:

- **The engine was warm.** Boot completed at 20:23:58, 3 m 40 s before traffic, and it had
  already handled 199 messages cleanly. A cold JVM is not the explanation.
- **Ingestion and persistence are not the bottleneck.** The DB writer was already drained and
  failing nothing before the first tick, and every identity — including all 126 that never
  reached the sink — is present in history.
- **The stall is specific to the outbound telemetry path.** The only component off the SUT is
  the sink itself (`http://87.107.191.32:18091/tb`, appended to the root rule chain as the
  `bench sink` REST node). Everything upstream of that edge is demonstrably alive.

## Why the gateway accepts and then appears to lose

The path is:

```
client --HTTP--> tb-http-transport --> Kafka --> tb-rule-engine --> PostgreSQL
                                                        |
                                                        +--> REST "bench sink" node --> sink
```

The HTTP transport acknowledges once the message is enqueued to Kafka. That is why 7,463 of
7,500 warmup dispatches were ACKed with `failure: 0`: from the client's point of view nothing
failed. The shortfall is downstream, between Kafka consumption and the REST export node,
while that path is stalling. This is why the analyzer reports `measurement_valid: true`
alongside `missing_by_cutoff > 0` — both are true at once, and that combination is exactly
what `VALID_PLATFORM_FAILURE_OR_BACKLOG` means.

The missing set splits into two populations, which is itself diagnostic:

| Run | Missing | Never answered by HTTP | ACKed, never reached sink |
|---|---:|---:|---:|
| r02 | 200 | 14 | 186 |
| r03 | 160 | 72 | 88 |
| r04b | 144 | 24 | 120 |
| r05 | 126 | 11 | **115** |

The dominant population is the second: the client was told the message was accepted, and it
still never reached the export edge in time.

## What "Timeout to process" does and does not tell us

The rule-engine log prints `Timeout to process` during the stall — 202 lines on replica 1 and
200 on replica 2 in `disp-tb-r05-20261106`. They are a useful corroborating timestamp, and
they are tightly localised:

- **All 202 on replica 1 occur between 20:27:44.963 and 20:28:27.614** — a 42.7 s window
  starting 6.5 s after the first tick. Zero before traffic, zero after 20:28:27. The
  consumer stats confirm the same shape across their 60 s samples: `timeoutMsgs`
  0 → 0 → 156 → 369 → 0, i.e. the burst is visible in the 20:27:57 and 20:28:57 samples
  and gone by 20:29:57. (Those sample timestamps are *not* the burst window; the burst
  itself is the 42.7 s interval above.)
- Their `Last Rule Node` is `Message Type Switch` (52), `Save Timeseries` (48) and `bench
  sink` (1) — i.e. they are spread across the chain, not concentrated at the export node.
- They are **not** a 1:1 proxy for loss: 201 unique sequences timed out, but only **17** of
  them are among the 126 missing, and **184 timed-out messages were delivered anyway**.
  Timeout is a retry signal, not a drop.

So the timeouts localise *when* the path stalled, but they do not by themselves identify the
stage, and they must not be reported as the number of lost messages.

## Why it is not a resource problem

This matters for the classification, because it rules out "the SUT was overloaded":

- `saturation_flag: false`, `cgroup_throttling_observed: false` in all four runs.
- Generator resource qualification `true` in all four runs; 0 OOM, 0 container restarts in
  the post-run snapshots; sampler status complete, 0 discontinuities, 0 overruns.
- `scheduled → receipt` p99 *within* the measurement window is 205–283 ms (dispatch→receipt
  p99 203–281 ms) — flat and healthy for 600 s at the same 25/s.
- The platform keeps up perfectly once past first contact, with `timeoutMsgs` pinned at 0.

A four-run defect confined to the first 9.9–23.1 s of a 900 s schedule, on a platform that
had been idle and fully booted for over three minutes, is a **first-contact transient**, not
a capacity limit.

## Root cause, stated honestly

**Proven:**

- Generator dispatch is on time (25/s in every 1 s bin from t=0).
- The rule-engine process is fully booted and healthy well before traffic.
- Ingestion and persistence succeed; every sink-missing identity exists in history.
- Delivery to the sink stalls absolutely for 31–39 s, then bursts and recovers.
- The stall is confined to the first contact of the run, and to the rule-engine→sink edge.

**Refuted:** "the rule-engine JVM initialises lazily at first contact". The 215-second boot
finished 3 m 40 s before traffic, and 199 messages were processed before first contact.

**Not proven:** why the outbound telemetry path costs 31–39 s specifically. Candidates that
the current evidence cannot separate:

- first-use initialisation of the REST export node's HTTP client / connection pool toward the
  off-host sink, including the AntiSamy policy load seen at 20:27:43 (5 s after first contact);
- a Kafka consumer-processing stall or rebalance as the first telemetry partition becomes
  active, which would also explain the tightly-clustered processing timeouts;
- brief contention on the platform pool as the first burst is absorbed.

This distinction is recorded deliberately: the report must not claim a mechanism it cannot
show. What it does establish is that the cost is *deterministic enough to measure four times*
and always lands on the leading edge.

## Consequences for the campaign

So:

1. **Ordinals 6, 8, 10 (`r04b`) and 14 are correctly classified.** They are genuine,
   reproducible first-contact failures under the frozen policy. They must not be relabelled
   as successes and must not be retried.
2. **Ordinals 1, 10 (`r04`) and 10 (`r04a`) are not platform evidence.** They were never
   offered traffic and failed for harness reasons. They should be reported as setup
   failures, never as ThingsBoard losing messages.
3. **The measurement window is unaffected.** Every TB run measured cleanly at
   15,000/15,000 with 0 missing. Latency claims drawn from the measurement phase stand; no
   claim should ever be drawn from the warmup p99 (33.8–41.5 s), which measures the
   transient.
4. **ThingsBoard now has at most four valid measured repetitions** (r02, r03, r04b, r05),
   below the five the protocol targets. r01 was never offered traffic and is not replaced;
   `r04`/`r04a` supplement ordinal 10 rather than replacing anything. Missing repetitions
   are not replaced.
5. **This will recur on any future TB run** unless something changes. It is not noise that a
   retry would "get past" — every retry pays the same 31–39 s.

## What would fix it (post-campaign only)

These are proposals, not authorizations. The frozen batch binds the launcher and plans; none
of this may be applied while ordinals remain.

**Option A — accept it as the platform's measured behaviour (recommended).**
The frozen contract reports `VALID_PLATFORM_FAILURE_OR_BACKLOG` for warmup shortfall. The gap
*is* a real property of the system under test at this operating point. Reporting it, with the
caveat that the signal is a first-contact transient and that the measurement window is clean,
is the honest thing to do. The four-of-five repetition limitation is then carried into every
TB report.

**Option B — add a pre-warm probe before the timed window.**
Send a small, bounded number of throwaway telemetry messages (distinct run id, not counted)
and wait until the first one reaches the sink or a deadline expires, then start the generator.
This primes the outbound path and the measurement would begin from steady state. It is the
cleanest scientific fix, but it **changes the procedure**, must be a reviewed protocol
amendment rather than a quiet launcher tweak, and delays first measurement until after first
contact, which must be disclosed.

**Option C — raise the warmup length.**
Also a protocol change, disallowed under the current frozen scope, and it wastes 30 s of
platform time per run rather than removing the transient.

**Explicitly not a fix:** retrying the run. The defect is deterministic; a retry produces the
same leading-edge loss and burns a repetition.

## Reproduce

```bash
cd results/feasible-campaign-20261106/evidence/disp-tb-r05-20261106
python3 - <<'PY'
import json
rows=[json.loads(l) for l in open('disp-tb-r05-20261106.sink.ndjson')]
t0=min(r['scheduled_send_ms'] for r in rows)
print('first arrival at %.1fs' % min((r['recv_ms']-t0)/1000 for r in rows))
print('receipts before t=30s:', sum(1 for r in rows if (r['recv_ms']-t0)/1000 < 30))
seen={r['seq'] for r in rows}
missing=sorted(set(range(22500))-seen)
print(len(missing),'missing; all warmup:', all(m<7500 for m in missing))
print('missing sched offsets %.2f..%.2fs' % (missing[0]/25, missing[-1]/25))
print('last receipt vs last send: %.1fs' % (
    max(r['recv_ms']-r['scheduled_send_ms'] for r in rows)/1000))
hist={json.loads(l)['seq'] for l in open('disp-tb-r05-20261106.history.ndjson')}
print('missing-but-persisted:', len(set(missing)&hist), 'of', len(missing))
PY
```

Expected: first arrival ≈31.0 s, 0 receipts before 30 s, 126 missing, all in warm-up
(offsets 0.04–9.88 s), all 126 present in history, last receipt within ~0.1 s of its send.

To check the drain shape rather than just the endpoints (this is where the earlier draft
was wrong):

```bash
python3 - <<'PY'
import json
rows=[json.loads(l) for l in open('disp-tb-r05-20261106.sink.ndjson')]
t0=min(r['scheduled_send_ms'] for r in rows)
for lo in range(0,120,10):
    xs=[(r['recv_ms']-r['scheduled_send_ms'])/1000 for r in rows
        if lo <= (r['scheduled_send_ms']-t0)/1000 < lo+10]
    print(f'{lo:3}-{lo+10:3}s  max lag {max(xs):5.1f}s')
PY
```

Expected (r05): `0:41.9  10:32.7  20:25.3  30:16.6  40:8.6  50:0.5` then flat — a monotonic
decay, not a step. r02/r03/r04b decay more slowly and still show 1.5–2.0 s outliers in the
50–63 s bucket.

To confirm the engine was warm rather than lazily initialised, on the SUT:

```bash
docker logs --since 2026-11-09T20:20:00 --until 2026-11-09T21:00:30 tbq20261105-tb-rule-engine-1 2>&1 \
  | grep -E "Started ThingsBoard in|TbRuleEngineConsumerStats"
```

Expected: `Started ThingsBoard in 215 seconds` at 20:23:58, then `timeoutMsgs = [0]` for the
two samples before traffic, then the samples that carry a nonzero `timeoutMsgs`
(156 at 20:27:57, 369 at 20:28:57), then back to `[0]` from 20:29:57 onward.

## Related

- `research/findings/edgex-r11-second-replica-readiness-failure.md` — the other pre-traffic
  defect found in this campaign (EdgeX, different mechanism).
- `scripts/analyze-dispatch-run.py:208` — where any missing/unknown in any phase sets
  `platform_failure`.
- `scripts/dispatch-integrity.py` — the `dispatch-integrity-v1` policy.
- `scripts/outbound.sh` — installs the `bench sink` REST node after `Save Timeseries` in the
  root rule chain; the off-host edge where the stall is observed.
- `scripts/history-ids.sh` — the history export used to prove the missing identities were
  persisted.
