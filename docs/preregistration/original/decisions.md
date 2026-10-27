FRAMING
  JSys Tool/Benchmark paper: SLO-based scalability fingerprint vs
  conventional IoT metrics; FIWARE, ThingsBoard, EdgeX as case studies.

RQs
  RQ-E (headline) conventional metrics vs fingerprint: do they disagree?
  RQ-A capacity        C_SLO(p, N=1), L_knee, G_peak
  RQ-B scaling         E(N) = C_SLO(N) / (N x C_SLO(1)), N in {1,2,4}
  RQ-C bottleneck      per-component evidence + 1 ablation per platform
  RQ-D overload        R_overload, T_recovery (50->80->120->70% of C_SLO)

SLO PROFILES
  P1 smart grid, 3GPP TS 22.369 Table 6.3-1: end-to-end 1 s
  (more profiles: TBD by Shahyar)

TIME BUDGET
  platform gets 50% of end-to-end budget (pre-registered)
  P1 -> platform p99 <= 500 ms, loss = 0, errors <= 0.1%
  sensitivity (re-analysis only): 25% and 100%

SEMANTICS
  start   = generator send timestamp, carried in the payload
  stop    = sink receives the message via the platform's outbound path
            TB:     Save Timeseries -> on Success -> REST API Call -> sink
            FIWARE: IoTA -> Orion -> subscription #1 -> QuantumLeap (history)
                                  -> subscription #2 -> sink
            EdgeX:  device-rest -> bus -> core-data (persist ON)
                                       -> app-service HTTP export -> sink
            note: FIWARE and EdgeX notify the sink in parallel with persistence
  durable = post-run store count == sent, 0 duplicates
            (TB ts_kv, QuantumLeap table, EdgeX core-data)

HARDWARE
  SUT     8 cores / 16 GB, platforms only, one platform at a time
          1 core   OS + Docker + exporters
          3 cores  stateful tier, fixed for all N
                   (DB, broker/bus, cache, LB, QuantumLeap backend)
          4 cores  app replicas, 1 core each, N in {1,2,4}, pinned via cpuset
  LOAD    4 cores / 8 GB: generator + sink + Prometheus/Grafana
          pin: generator 2 cores, sink 1, monitoring 1
          generator and sink share a clock (no cross-machine skew)

QUANTUMLEAP
  backend: TimescaleDB (lighter than CrateDB on 16 GB), in the stateful tier

  H-A (pre-registered)
  For each platform, across platform thresholds {250 ms, 500 ms, 1 s, 5 s}:
    (max C_SLO - min C_SLO) / max C_SLO < 10%
  -> supported if this holds for all 3 platforms, given 95% CIs of about +-5%
  -> falsified if any platform shows >= 10% spread
  rationale: queueing theory predicts <1%; margin covers batching, GC,
             tail effects; 10% sits above the CI width, so it is testable
