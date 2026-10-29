# Generator task: status and summary (2026-10-22)

The open-model k6 load generator (`docs/methodology/generator.md`), its Go controller `benchctl`, the fault fixture and the
automated tests are built and verified. Calibration and the Poisson prototype are measured (`docs/methodology/calibration.md`).
The decisions taken are in `docs/design-decisions.md` (D15-D22).

## Built

| Component | Where |
|---|---|
| k6 generator: `constant-arrival-rate` per step, seq/scheduled time from `iterationInTest`, explicit outcomes, non-success records, bucketed histograms, 1 s step-boundary guard, optional Bernoulli thinning | `workloads/k6/gen.js`, `lib/{adapters,hash}.js` |
| Adapters with `run_id, seq, scheduled_send_ms` + 3 fields: fixture/reference, ThingsBoard, FIWARE, EdgeX (`Object` reading) | `workloads/k6/lib/adapters.js` |
| `benchctl`: resolve / run / check / reconcile / expected / arrivals / sendtimes | `cmd/benchctl`, `internal/{plan,runner,report,reconcile,arrivals,hash}` |
| Fault fixture: 5xx, timeout-but-stored, slowdown ramp, 207 batch failures, bounded loss-reporting store | `internal/fixture`, `cmd/fixture-target` |
| Tests: unit (plan, hash, histograms, arrivals) and Docker integration (all §10 faults + Go/JS hash parity) | `internal/*/…_test.go`, `tests/integration` |
| Plans: smoke (fixture + 3 platforms), calibration, Poisson | `workloads/plans/` |
| Deployment of an exact commit + `GIT_REV` to the load host | `scripts/deploy-generator.sh` |

## Verified (evidence)

| Claim | Evidence |
|---|---|
| Every §10 fault is classified/flagged correctly | `go test ./tests/integration -v`: 9/9 pass (timeout-but-stored → unknown + persisted; 5xx → explicit_failure; 207 item failure; VU exhaustion → INVALID + reconciliation refused; duplicate seq detected; early send → INVALID; starved generator → send-lag INVALID; hash parity) |
| Adapters work against the live platforms (N=1, 10/s, 20 s, 10 devices) | ramonavm1 `results/runs/{thingsboard,fiware,edgex}-smoke-*`: all VALID, 200/200 ack. Identity fields confirmed in ThingsBoard ts_kv, Orion entities and EdgeX core-data readings |
| Fixture run reconciles exactly | `fixture-smoke-*`: planned 3250 = ack 3250 = stored 3250, no loss/duplicates/orphans |
| Generator ceiling, GC effect, drift, boundary losses, VU memory, send-time agreement, Poisson cost and statistics | `docs/methodology/calibration.md`, with run ids |

## Main findings

1. **k6 needs `GOGC=400`.** With the default GC, 2 cores fail the 10 ms send-lag gate even at 500/s; the p99 comes
   from GC pauses.
2. **Ceiling: ~1 core per 1000 req/s.** About 1000/s on 2 cores and 1500/s (2000 borderline) on 3 cores, against an
   instant endpoint. Platforms with slower acknowledgements need more VUs, and therefore memory.
3. **k6 silently lost the last tick of a step** (r·d−1 iterations, no drop counted) in 1 of 10 steps. Fixed with a
   1 s executor guard; 0 losses over 60 boundaries since.
4. **Memory grows with VUs × steps** (Counters are created per VU in the init context): 200 VUs × 30 steps reached
   5.2 GB with GC-induced lag failures; 50 VUs gave 1.6 GB, all valid. The VU pool must be sized from measured latency.
5. **Poisson via thinning is correct and reproducible but costs 4-10× iterations:** only λ ≈ 500/s on 3 cores.
   Constant arrivals stay the default.
6. **The arrivals the target sees are not the schedule:** constant spacing arrives with CV 0.39 because of jitter.
7. **EdgeX ingestion answers 200/404, not 207**; 207 is the provisioning batch API.

## Needs a decision

- **Maximum planned platform rate.** k6 on this host passes §9's "2× max rate" only if the maximum planned rate is
  ≤ ~750/s (≤ ~1000/s if 2000 on 3 cores is accepted). Above that: a Go generator or a larger load host.
- **Core allocation on the load host:** 3 generator cores (0-2, as in `docs/methodology/calibration.md`) leave core 3 for
  Prometheus, which the sink would then share (the sink defaults to core 2 and must be re-pinned with
  `SINK_CPU=3`). The DIAG sink runs used 2 generator cores (0-1), sink on core 2 and monitoring on core 3
  (`scripts/run-with-sink.sh:13`, `scripts/sink.sh:6`, `monitoring/compose.yaml:17`); a 3-generator-core layout with
  the sink has not been measured.
- **VU-pool sizing rule** and the late handling of platforms with long acknowledgement tails (memory).
- **FIWARE history (QuantumLeap)** is still missing, so FIWARE's history cannot be audited.
- **Prior research notes** were found (`~/catStuff/wrk/iot-performance-evaluation/research/notes/`) but not yet
  checked against the decision log (D1).

## Reproduce

```bash
go test ./internal/... && go test ./tests/integration -v            # workstation, Docker
SSH_KEY=~/.ssh/id_ed25519_vps scripts/deploy-generator.sh root@87.107.191.32 --jump root@ramonavm2
# ramonavm2: /root/fixture-target -addr 0.0.0.0:18080 -out store.ndjson   (pinned: taskset -c 4-7)
# ramonavm1:
bin/benchctl run -plan workloads/plans/calibration-ramp-3c.json -results results/runs -cpuset 0-2 -env GOGC=400
bin/benchctl check -run results/runs/<run_id>
```
