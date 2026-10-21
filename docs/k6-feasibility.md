# k6 measurement-design feasibility (M0)

Status: **M0 finding, pilot only.** It decides how planned arrivals are scheduled and identified. It does not freeze
any experimental threshold. Date: 2026-10-22. Host: local workstation (8 cores, 13 GB, Docker 29.9.1). Generator and
target share one wall clock, so cross-machine clock questions are out of scope here (M3).

## Pinned runtime

| Component | Version | Evidence |
|---|---|---|
| k6 | v1.8.1 (commit 73b044f9f9, built with go1.26.4) | `docker.arvancloud.ir/grafana/k6@sha256:23f2279054c3e01535455c4d92914a625df12aeb631ea0e3ea7a43f96bcbc843` (= `grafana/k6:1.8.1` via mirror) |
| Go (harness) | go1.27.2 (`go.mod`: `go 1.27`, `toolchain go1.27.2`) | `go version` |
| xk6 / extensions | none | not needed for the findings below (see "Decision") |

## What the pinned k6 exposes to an iteration (probed, not taken from docs)

- **Wall clock:** `Date.now()` only, with **millisecond** resolution (five consecutive calls returned the same
  value). There is no `performance.now()` or `performance.timeOrigin`. `BigInt` exists, so nanosecond strings can be
  carried losslessly.
- `exec.scenario`: `name, executor, startTime (ms), progress, iterationInInstance, iterationInTest`.
  `exec.vu`: `idInInstance, idInTest, iterationInInstance, iterationInScenario, tags, metrics{tags, metadata}`.
  `exec.instance`: `currentTestRunDuration, iterationsCompleted, iterationsInterrupted, vusActive, vusInitialized`.
  **Nothing identifies the executor's scheduled tick.**
- `res.timings`: `duration, blocked, looking_up, connecting, tls_handshaking, sending, waiting, receiving`. These are
  durations only, with no absolute dispatch timestamp.
- `exec.vu.metrics.metadata.<k> = v` attaches **non-indexed** metadata to every sample the VU emits. It appears in
  `--out json` lines (`"metadata":{"iter":"0"}`) without becoming a tag, so per-message IDs can be exported without
  high-cardinality labels.
- Sample timestamps in `--out json` are **nanosecond** wall-clock values:
  - an `http_req_*` sample's time is the **end of the response**: p50 0.205 ms after the target's receipt in A0;
  - so **dispatch (start of sending) = http_req sample time − `http_req_duration`**
    (`http_req_duration` = sending + waiting + receiving; `blocked`/`connecting` happen before it);
  - **iteration start = `iterations` sample time − `iteration_duration`.**
  - `dropped_iterations` samples carry the time of each drop.
- **Volume:** each request produces 13 samples. A 10-minute run at 1,000 req/s would be about 7.8 million JSON
  lines. Event export volume and overhead are an open item (below).

## Experiments

A controllable endpoint (`cmd/fixture-target`) answers after a configurable delay and writes one NDJSON record per
request: receipt time as epoch ns (string) plus monotonic ns, and every `X-Bench-*` header. Its buffer is bounded;
an overflow is counted and makes it exit non-zero. `cmd/feasibility-analyze` joins those records with k6's JSON
output. Reproduce everything with `tests/feasibility/run-all.sh`. Raw evidence goes to `results/feasibility/<case>/`.

Each case is 10 s. Generator delay = k6 container limited to 0.1 CPU (`--cpus 0.1`). VU exhaustion = 5 VUs against a
200 ms target (capacity ~25 req/s). Target delay = 500 ms target with enough VUs.

### A: native `constant-arrival-rate` (`workloads/k6/feasibility/car-probe.js`), identity = `iterationInTest`

| Case | Planned | Started | k6 dropped | Unaccounted | `iterationInTest` | Start − (t0 + iter/rate) p50 / max |
|---|---|---|---|---|---|---|
| A0 baseline, 200/s | 2001 | 2001 | 0 | 0 | 0..2000, no gaps | 1 / 1 ms |
| A1 VU exhaustion, 100/s | 1001 | 240 | 761 | 0 | **0..239, no gaps** | **3,681 / 7,521 ms (false)** |
| A2 generator delay, 500/s | 5001 | 1268 | 3299 | **434** | 0..1267, no gaps | **4,406 / 7,766 ms (false)** |
| A3 target delay, 100/s | 1001 | 1001 | 0 | 0 | 0..1000, no gaps | 1 / 3 ms |

In A3 arrivals at the target stayed at exactly 100/s every second. A full re-run (`run-all.sh`, same day) reproduced
every qualitative result. Magnitudes vary under generator delay: A2 lost **198** arrivals in the re-run (434 in the
first run); B2 again delivered all 5,000 slots exactly once.

### B: script-owned schedule (`workloads/k6/feasibility/slot-probe.js`), identity = planned slot

| Case | Planned | Received | Never sent (visible) | Duplicates | Scheduling delay (start − scheduled) p50 / max |
|---|---|---|---|---|---|
| B0 baseline, 200/s | 2000 | 2000 | 0 | 0 | 1 / 2 ms |
| B1 VU exhaustion, 100/s | 1000 | 945 | 55 | 0 | 14,247 / 28,516 ms (real backlog) |
| B2 generator delay, 500/s | 5000 | 5000 | 0 | 0 | 10,875 / 21,838 ms (real backlog) |
| B3 target delay, 100/s | 1000 | 1000 | 0 | 0 | 1 / 3 ms (latency is all service time) |

## Findings against the six requirements

1. **Unique slot ID and intended time independent of completion.**
   - Native CAR: only while nothing goes wrong. `iterationInTest` is a counter of **started** iterations: in A1 it was
     gap-free 0..239 while 761 iterations were dropped. Using it as a slot index produced scheduling delays of up to
     7.5 s that never happened.
   - Script-owned schedule: yes. The slot is `vu + k·VUS`, and its due time `T0 + slot/rate` is fixed in `setup()`.
2. **Started requests map to their intended slot under VU exhaustion and generator delay.**
   - Native CAR, VU exhaustion: the mapping can be rebuilt **offline** from timestamps. In A1, iteration starts
     (ns, from JSON) plus drop samples put exactly one event on each of the 1,001 ticks, every one within 0.2
     periods of its tick.
   - Native CAR, generator delay: the mapping fails. The starved executor processes ticks in bursts (the CFS
     throttle window), so starts and drops pile up together. Only 54 of 5,001 ticks got exactly one event, 273 got
     several, 4,674 got none.
   - Script-owned schedule: yes in every case.
3. **Iteration start vs dispatch.** Iteration start (ns) = `iterations` time − `iteration_duration`. The dispatch
   event is the **start of sending**: `http_req` sample time − `http_req_duration`. The JS `Date.now()` taken just
   before `http.post` is not the dispatch event: under generator starvation the request reached the target up to
   1.5 s (A2) / 2.2 s (B2) after it.
4. **Unstarted slots never disappear.**
   - Native CAR: fails under generator delay. In A2, **434 planned arrivals were neither started nor counted in
     `dropped_iterations`.**
   - Script-owned schedule: the planned set `0..N−1` is known before the run. Every slot is either received or
     explicitly missing (B1: 55, also counted by k6 as interrupted iterations).
5. **Export without per-message console logging or high-cardinality tags.** Feasible natively:
   `exec.vu.metrics.metadata` + `--out json` carries slot IDs with ns times. Each request emits 13 samples, so
   overhead and volume must be measured (open item).
6. **A slower target must not quietly turn the run into a completion-paced workload.**
   - Native CAR: satisfied (A3: 100/s held, starts on time).
   - Script-owned schedule: satisfied (B3). When the generator itself cannot keep up (B1, B2), the backlog shows up as
     scheduling delay on every affected slot, so it is never quiet.

## Decision needed (not taken silently)

**Native `constant-arrival-rate` cannot keep the planned-arrival identity under generator delay, and loses
arrivals from its own accounting.** That is a confirmatory-run blocker (spec §3). Two candidate fixes:

1. **Script-owned deterministic schedule (native k6, tested above).** `per-vu-iterations` with one iteration per
   owned slot; each iteration waits until its slot's absolute due time.
   - This is a deadline, not pacing: a due time never depends on when an earlier request finished.
   - Passes the acceptance test (VU exhaustion, generator delay and target delay give correct
     scheduled/started/missing accounting and separate scheduling delay from service latency).
   - Costs: it is not the `constant-arrival-rate` executor the spec names. Slots are bound to VUs, so a slow request
     delays only its own VU's later slots (per-VU head-of-line blocking) while other VUs may idle. The VU count must
     therefore be sized from measured latency, and a late slot is sent late (catch-up), with its delay recorded.
2. **Go xk6 module + `constant-arrival-rate`.**
   - A shared, lock-free slot dispenser that every VU claims from in due-time order: unclaimed slots stay visible,
     claims carry the true due time, and the queue is global (no per-VU head-of-line blocking). The same module
     could write events directly (smaller than `--out json`).
   - Costs: custom k6 build and maintenance. It cannot change which ticks the executor fires or drops, so dispatch
     timing is still the executor's.

**Recommendation:** adopt (1) for M1–M2. It is native, already passes the acceptance test, and needs no custom
build. Keep (2) as the fallback if the per-VU head-of-line effect or JSON-output overhead turns out to be material in
generator qualification (M3).

## Open items (to resolve in M1–M3)

- **Event export overhead and volume** of `--out json` (13 samples/request) at the target rates on ramonavm1. Is a
  filtered or custom output needed?
- **Late-slot policy:** send late (current) vs skip once lateness exceeds a limit. Either way every slot stays
  accounted; the choice changes what "offered load" means under overload and must be fixed before confirmation.
- **Sub-millisecond due times:** due times are computed exactly, but JS waits with ms granularity. The measured start
  lag was ≤2 ms in B0/B3 at 200/s; this must be qualified at the target rates.
- **VU sizing rule** (from occupancy = rate × latency tail, with headroom) and a VU cap that fits ramonavm1.
- **Native CAR is kept** for the existing quick workloads (`workloads/*.js`); their results are not confirmatory
  (they identify devices by `iterationInTest`, which is the started-iteration counter).
