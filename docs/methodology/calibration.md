# Generator calibration and Poisson prototype (pilot)

Status: **pilot evidence, 2026-10-22.** It measures the k6 generator (`docs/methodology/generator.md`) on the real load host.
It decides nothing about the platforms.

## Setup

| | |
|---|---|
| Generator host | ramonavm1: 4 vCPU Intel Xeon E5-2680 v3 (VMware), 8 GB, Ubuntu 24.04, kernel cgroup v2 |
| k6 | v1.8.1, `docker.arvancloud.ir/grafana/k6@sha256:23f2279054c3e01535455c4d92914a625df12aeb631ea0e3ea7a43f96bcbc843` |
| Code | commits `36a1b46` .. `c08b14b` (each manifest records its own SHA + `k6_scripts_sha256`) |
| Target | `cmd/fixture-target` on ramonavm2 (pinned to cores 4-7, answers 200 immediately, stores every request), same /24 LAN, RTT 0.3 ms. Port 18080 open to ramonavm1 only |
| Generator cores | `--cpuset-cpus 0-1` (2 cores) or `0-2` (3 cores); Prometheus + node-exporter on core 3; **Grafana stopped during runs** |
| Plan defaults | 100 devices, 2 s request timeout, keep-alive, histogram edges 0..10000 ms, gates: 0 drops, all ticks, achieved >= 0.99, send-lag p99 <= 10 ms, no early sends |

Every number below comes from `results/runs/<run_id>/{health.json,manifest.json}` on ramonavm1. Recompute any
verdict with `bin/benchctl check -run <dir>`.

## 1. Ceiling ramp, default Go GC (2 cores, 200 VUs, 45 s steps): `calibration-ramp-20261022t000701z-af9e`

| Rate | Sends | Dropped | Send lag p50 / p99 / max (ms, bucket) | Core busy mean (max) | Verdict |
|---|---|---|---|---|---|
| 500 | 22500/22500 | 0 | <=1 / <=20 / <=50 | 39% (76%) | INVALID (lag) |
| 1000 | 45000/45000 | 0 | <=1 / <=20 / <=75 | 51% (82%) | INVALID (lag) |
| 2000 | 89186/90000 | 815 | <=300 / <=500 / <=500 | 83% (99%) | INVALID |
| 3000 .. 10000 | | 7,673 .. 244,062 | seconds | 96-100% | INVALID |

**With the default GC, 2 cores fail the 10 ms send-lag gate at every rate tested.** The median is ~1 ms, so the
p99 comes from short stalls. k6 memory grew to 1.4 GB over the run.

## 2. Garbage collector and cores at 1000/s (45 s, one step each): `calibration-fixed-*`

| Cores | `GOGC` | Lag p99 / max | Ack p99 | Core busy mean | k6 memory | Verdict |
|---|---|---|---|---|---|---|
| 2 | default (100) | <=15 / <=75 ms | <=50 ms | 51-52% | 150 MB | INVALID |
| 2 | 400 | <=7 / <=20 ms | <=20 ms | 55% | 361 MB | VALID |
| 3 | default | <=5 / <=30 ms | <=10 ms | 45-50% | 147 MB | VALID |
| 3 | 400 | **<=2** / <=20 ms | **<=5** ms | 33-36% | 357 MB | VALID |

The stalls are Go garbage collection in k6. `GOGC=400` (collect less often, larger heap) removes most of them; a
third core removes more. **Cost: about one core per 1,000 req/s** on this CPU.

## 3. Ceilings with `GOGC=400` (45 s steps)

| Cores | Run | Last VALID step | First failing step |
|---|---|---|---|
| 2 (0-1) | `calibration-ramp-2c-20261022t001839z-40e1` | **1000/s** (lag p99 <= 5 ms) | 1250/s: lag p99 <= 15 ms, no drops; drops from 1750/s |
| 3 (0-2) | `calibration-ramp-3c-20261022t002232z-f015` | **2000/s** (lag p99 <= 10 ms, *at the limit*) | 2500/s: 174 drops, lag p99 <= 75 ms |

**Generator ceiling: ~1,000 req/s on 2 cores, ~1,500-2,000 req/s on 3 cores** (2,000 is borderline).
The maximum planned platform rate is not fixed yet. If it exceeds ~1,500/s, the 2x headroom rule of calibration
(§9 of the task) cannot be met with k6 on this host, and a Go generator or a larger load host is needed.

## 4. Ten minutes at 1500/s, 3 cores, `GOGC=400`: `calibration-long-20261022t002826z-8934`

Ten 60 s steps (to see drift):

| Measure | Result |
|---|---|
| send lag p99 per minute | 1-7 ms (all <= 10) |
| ack latency p99 per minute | 5-15 ms |
| core busy (3 generator cores) | ~55% mean, flat |
| core 3 (Prometheus) | ~10-12% |
| k6 memory | 2.33 GB (minute 1) → 2.61 GB (minute 10) |

No drift. **One step-boundary loss:** minute 7 executed 89,999 of 90,000 ticks with **0 dropped iterations
reported**. The missing message was seq 629,999, the last tick of the step: its timer fired after the scenario
deadline. Fixed in `c08b14b`: every executor now runs 1 s past its step, and only `i < rate x duration` is sent.

## 5. Boundary guard check, 30 x 10 s at 1000/s, 3 cores, `GOGC=400`

| Run | VU pool | Lost ticks | Steps failing the lag gate | k6 memory peak |
|---|---|---|---|---|
| `calibration-boundaries-20261022t004029z-8e73` | 200 | **0 of 30 boundaries** | 3 of 30 (steps 0, 10, 21: lag p99 15-20 ms, cores to ~95%) | **5.2 GB** |
| `calibration-boundaries-v50-20261022t005108z-fe0d` | 50 | **0** | **0 of 30** (worst p99 <= 10 ms, cores <= 83%) | **1.56 GB** |

The guard works: 0 lost ticks over 60 boundaries in the two runs. Memory grows with VU pool × number of steps:
every VU runs the init code, which creates a Counter per (step, histogram bucket), so 30 steps make ~1,700 metric
objects per VU. With an oversized pool (200), the GC cycles of that 5.2 GB heap caused the lag spikes. With 50 VUs
the same plan is VALID in every step.

**VU sizing rule (pilot):** `pre_allocated = max = ceil(rate × p99.9 acknowledgement latency × 2)`, with at least
~20 and at most what the step count allows in memory. Size it from a short run's ack-latency histogram. For
platforms with 100-200 ms acknowledgement tails at 1000/s that is 200-400 VUs: re-check memory with the real step
count before a sweep.

## 6. Send time: JS vs k6 sample time: `calibration-sendtimes-20261022t004547z-a4ba` (500/s, full JSON output)

| Source | p50 | p99 | max |
|---|---|---|---|
| JS `Date.now()` before the request (histogram, bucket upper edges) | <= 1 ms | <= 2 ms | <= 50 ms |
| k6 sample time − `http_req_duration` − scheduled (ns, `benchctl sendtimes`) | 1.377 ms | 3.224 ms | 34.0 ms |

The JS send time and k6's own dispatch time agree within ~1-2 ms. That difference is the ms truncation plus the
time between `Date.now()` and the socket write. The full JSON output for 22,500 requests was 4.25 MB gzipped
(13 samples/request): affordable for calibration runs, not for 10-minute production runs.

## 7. Poisson prototype: Bernoulli thinning (seeded `hash(seed, seq)`), 3 cores, `GOGC=400`

| Run | λ | Tick rate | Sent / planned | Dropped | Lag p99 | Core busy mean (max) | Verdict |
|---|---|---|---|---|---|---|---|
| `poisson-thin10-20261022t004640z-e17d` step 0 | 500/s | 5000/s (p = 0.1) | 30163/30163 | 0 | <= 10 ms | 46% (72%) | VALID (at the limit) |
| same, step 1 | 1000/s | 10000/s | 60221/60221 sent, 600000 ticks | **4307** | <= 500 ms | 75% (88%) | INVALID |
| `poisson-thin4-20261022t004847z-81b4` | 1000/s | 4000/s (p = 0.25) | 60249/60249 | 0 | <= 15 ms | 51% (68%) | INVALID (lag) |

- **Reproducibility:** "sent / planned" compares the messages k6 sent with the set `benchctl expected` recomputes
  offline from the seed. They match exactly in every step.
- **Timing accuracy:** see "Arrival statistics" below.
- **Cost:** thinning makes k6 run 4-10 iterations per message, so the generator ceiling for Poisson arrivals is
  about **λ ≈ 500/s on 3 cores** (tick factor 10), against ~1500-2000/s for constant arrivals.

**Recommendation:** keep constant arrivals as the default (as agreed). Thinning is usable only up to ~500/s on this
host, or with a coarser tick factor (×4 approximates Poisson less well: CV = √(1−p) = 0.87 instead of 0.95).
A Go generator would remove this cost.

### Arrival statistics (`benchctl arrivals`, received side from the fixture store)

| Run / step | Planned gaps: CV, KS vs exponential (1% critical) | Received at the target: CV, KS | Receipt − scheduled p50 / p99 |
|---|---|---|---|
| thin10, λ = 500 | 0.957, 0.095 (0.009) | 1.073, 0.043 | 1.9 / 12.2 ms |
| thin10, λ = 1000 (INVALID run) | 0.953, 0.095 (0.007) | 1.035, 0.055 | 241 / 434 ms |
| thin4, λ = 1000 | 0.862, 0.221 (0.007) | 1.058, 0.040 | 1.9 / 19.1 ms |
| constant, λ = 500 (`calibration-sendtimes`) | 0.000, 0.632 (0.011) | **0.391**, 0.357 | 2.2 / 4.5 ms |

- **The planned thinning schedule behaves exactly as designed:** CV = √(1−p) (0.949 for p = 0.1, 0.866 for p = 0.25).
- **It is still formally rejected as exponential at these sample sizes, because KS ≈ p.** Gaps are whole multiples
  of the tick period, and a gap of exactly one tick has probability p. The departure from Poisson is a time grid of
  1/tick_rate (0.2 ms at λ = 500 ×10, 0.25 ms at λ = 1000 ×4). It is small next to ms-scale latencies, but it is
  not zero, and it must be stated.
- **The process the target sees is not the schedule.** Constant arrivals reach the fixture with CV 0.39 (generator
  and network jitter), and thinned schedules with CV slightly above 1. "Constant arrivals" describes the schedule,
  not the arrival process at the platform.
- **Reproducibility:** in every step, the received seq set equals the offline set from `benchctl expected`.
