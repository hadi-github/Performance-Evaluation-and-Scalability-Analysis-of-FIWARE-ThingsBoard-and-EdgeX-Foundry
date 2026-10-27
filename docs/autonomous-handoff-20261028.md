# Autonomous session handoff (auto-20261028, close-out 2026-10-28 ~15:15Z)

## Recommendations produced this session (each PROPOSED, with evidence)

1. **ThingsBoard memory (B6):** adopt container limits **1280 MiB** for tb-core/tb-rule-engine (transport
   unchanged 512 MiB) — clears the >95% stop condition at k≤3 (k=3 total 13,056 MiB ≤ 14,848 budget; k=4
   16,128 MiB does NOT fit). 1536 MiB is the headroom variant but fits only k≤2. Heap stays 512m (unchanged;
   the container limit, not the heap, was the binding constraint — measured: JVM heap+nonheap 548–641 MiB,
   off-heap/page-cache makes up the rest of the ~1.0 GiB working set). Evidence:
   `results/auto-20261028/pilot/` (V1/V2 CSVs, strict intervals, cgroup notes),
   `docs/pilot-20261028-tb-memory.md`.
2. **Maximum campaign rate (B10):** **none issued** — the bracket [50 r/s VALID, 100 r/s INVALID] is bounded
   by the PLATFORM at the current adopted configuration (k=2, 1024 MiB, ~2,400-device registry), not by the
   generator; the generator-only ceiling (null receiver) remains unmeasured. The sink was validated loss-free
   to ≥876/s (loopback). After the memory/k decisions land, re-run the bracket at the adopted configuration;
   the 2× rule (S3/D34/D37) then defines the campaign rate. Evidence: `results/auto-20261028/ceiling/`.
3. **FIWARE supporting-tier budget (B4):** proposed addition = timescale 1,536 + quantumleap 512 =
   2,048 MiB on the infra cpuset (no new cores); fits all planned N (N=4 total 9,472 MiB ≤ 14,848).
   Measured idle footprints recorded. Evidence: `results/auto-20261028/budget/fiware-supporting-tier.md`.
4. **Clock-error budget (B11):** the +330 ms median was PROBE BIAS (SSH session geometry), not clock offset;
   v2 probe prepared (interval-intersection + NTP self-reports); live windows pending. Conditional budget:
   cross-host claims need U ≤ P/10 where P is the owner-set SLO precision; no SLO invented. Primary latency
   claims are same-clock and unaffected. Evidence: `results/auto-20261028/clock/clock-diagnosis.md`.

## Server state at close-out
- SUT (`ramona-vm-2`): TB k=2 stack at ADOPTED 1024 MiB limits (pilot settings removed), registry 2,413
  devices (2,013 prior + 100 pilot-v1 + 100 pilot-v2 + ~100 bench-* from the ceiling steps, minus k6
  teardown deletions — exact count re-checkable via postgres); EdgeX database container stopped again after
  the B14 check (stack was down from isolation); FIWARE candidate/baseline stopped (isolation state);
  exporters running (cpuset 0).
- Load host (`ramona-vm-q`): monitoring + yaade untouched; sink instance RUNNING on :18090 writing
  `/root/ceiling-sink-auto20261028.ndjson` (session-owned; stop with `pkill -f "sink -addr"`, keep the
  ndjson as evidence); three ceiling run dirs under the candidate workloads results.
- Rollback commands in `docs/autonomous-handoff.md` (auto-20261027) remain valid for the stacks; the TB
  memory rollback already applied (adopted limits active).

## Failures / limitations / gaps
- Network dropouts on the workstation path (hotspot) cost ~40 min and forced SUT-local telemetry for the
  pilot phases (recorded deviation; the official load-host generator path was used for the ceiling steps).
- Generator-only ceiling NOT run (B10's generator-side half open).
- B11 live windows not completed (probe v2 ready).
- cgroup memory.peak for the pilot variants was reset by the post-pilot recreate (sampled maxima from the
  monitors are the retained evidence).
- D29a WAL live verification (pg-counters overlay run) still pending from auto-20261027.
