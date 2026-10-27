# Generator-ceiling exploration protocol (auto-20261028, labelled EXPLORATORY — not a qualification PASS)

**Authorization:** the owner authorized this exploration explicitly for this session (no owner-selected
maximum campaign rate required). Exploratory throughput is not a qualification PASS; undefined gates are
reported, not assumed.

## Phase 0 — receiver capacity check (sink-first)

The external sink (`bin/sink`, NDJSON receipt recorder on the load host, port 18090) is the benchmark's
independent receiver. Before any generator step:

1. Start `bin/sink` fresh (session-owned instance, unique output file).
2. Drive it directly with a local HTTP loop (no k6) at escalating rates (100 → 500 → 1000 → 2000 req/s,
   20 s each) from the load host itself (loopback — isolates the sink from the network path).
3. Acceptance per step: sink keeps up (its own queue/`recv` counters show no unbounded growth), no dropped
   connections, CPU of the sink process < 50% of one core. Record per-step rates and resource use.
4. If the sink saturates below the generator's needs, record the ceiling and stop (the sink is part of the
   qualified path; D24 forbids weakening it).

## Phase 1 — full-path ceiling (k6 → SUT platform → sink)

With the TB stack (or EdgeX) running in its isolated configuration:

1. Bounded doubling steps: 25 → 50 → 100 → 200 → 400 → 800 req/s, 60–120 s each (+30 s warm-up), via the
   official `workloads/run.sh` from the load host (k6 pinned to K6_CPUSET, remote-write to the local
   Prometheus, results in new unique dirs).
2. Per step, evaluate: achieved/offered rate (≤1% deviation gate), dropped iterations (must be 0), send-lag
   p99 (schedule integrity), error rate (0%), record completeness (stored == sent), generator CPU/memory
   (cAdvisor, K6_CPUSET cores), sink CPU/memory, SUT-side headroom (container CPU/mem from the live
   Prometheus), network throughput.
3. **Stop escalating** when: any validity gate fails, OR the generator's own resources saturate (CPU ≥ 90%
   of its cpuset for >30 s), OR the receiver saturates, OR a resource-safety limit is reached.
4. **Bracket:** after the first failing step, re-run the highest passing rate once more (confirmation), and
   step down one increment to confirm the boundary is stable (two consecutive passes at N, fail at 2N).

## Recommendation rule (adopted in `docs/methodology-reconciliation.md` S3/D34/D37: 2× governs)

`max_campaign_rate = floor(demonstrated_sustainable_generator_rate / 2)`, subject to sink and monitoring
qualification at that rate. If the sustainable rate is `R` with the full path, the recommendation is
`≤ R/2`; the 2× gate itself (B10) is cleared only when a real qualification run at `2×max_campaign_rate`
passes with the sink and monitoring in place.

## Deviations to record if they occur

- Load host unreachable from the workstation (this session's hotspot path): the generator must run from the
  load host for the full path (k6 pinned there by the protocol). If only the SUT is reachable, the ceiling
  exploration is POSTPONED, not degraded — a co-located generator would measure the platform competing with
  itself and is not a valid generator-ceiling datum.
