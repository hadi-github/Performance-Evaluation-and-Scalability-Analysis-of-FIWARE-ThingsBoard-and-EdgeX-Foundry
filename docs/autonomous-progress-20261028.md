# Autonomous progress (auto-20261028)

Started 2026-10-27T20:35 local / 12:35Z. Deadline 15:35Z. Authorization: see `docs/autonomous-work-plan-20261028.md`.

## Task log

### Task 0 — records + baseline — VERIFIED
HEAD 2bff517, clean tree; manifests re-verified 758/758 + 26/26.

### Task 1 — isolation prep — VERIFIED (with a later access incident, see below)
- Stopped on the SUT (volumes preserved): EdgeX stack, FIWARE candidate (`fiwhc`), FIWARE baseline.
- Kept: TB k=2 stack (2,013 devices intact) + SUT exporters (overhead: 4 containers, 128m-class limits,
  cpuset 0 — recorded in `results/auto-20261028/isol/tb-pilot-layout.txt` with image digests and limits).
- **Access incident:** after the isolation step the workstation's network path changed (wlan0 via a
  172.20.10.x hotspot gateway; RTT collapsed to 0.13 ms with a middlebox answering TCP on all ports without
  banners). SSH to BOTH hosts failed ~12:45–13:03Z; a background watcher restored access at ~13:03Z.
  Recurring intermittency followed (load host unreachable from the workstation while the SUT→load-host path
  stayed healthy at 0.4 ms).

### Task 2 — TB memory-sizing pilot — IN PROGRESS (V1 executed; V2 pending access)
Protocol: `docs/pilot-20261028-tb-memory.md` (variants, windows, stop conditions, k-fit arithmetic).
- **V1 executed** (tb-core/tb-rule-engine → 1280 MiB, heap unchanged, k=2 topology restored after two
  compose-variable mistakes — `N=` does not reach compose, `TB_REPLICAS=` does; recorded for the runbook):
  - boot: both cores `Started ThingsboardServerApplication`, OOMKilled=false, RestartCount=0 on all four
    app-tier containers.
  - idle (2,013-device registry): cores 917/945 MiB of 1280 = **71.6%/73.8%**; rule-engines 66.6%/68.8%.
    The >95% sustained condition is cleared at idle by a wide margin (V0 reference: 96–97%).
  - JVM decomposition (actuator `/actuator/prometheus`): core-1 heap 395.0 + nonheap 246.2 = **641.2 MiB**;
    core-2 heap 293.7 + nonheap 254.6 = **548.4 MiB**. The remaining ~280–400 MiB of the cgroup footprint is
    off-heap (direct buffers ≤128 MiB configured, page cache, thread stacks) — the container limit, not the
    heap, is the binding constraint, exactly as hypothesized.
  - bounded telemetry (SUT-local deviation, 100 `pilot-v1-*` devices + 120 sends at ~2 req/s; load host
    unreachable from the workstation): post-load cores 976/980 MiB = **76.3%/76.6%** — still far below 95%.
  - monitor CSV + strict interval analysis: pending fetch (access intermittent at close of V1).
- **V2 (1536/1536/768)**: pending access restoration.

### Task 3 — generator-ceiling exploration — BLOCKED on load-host access
Protocol written (`docs/protocol-20261028-generator-ceiling.md`): sink-first capacity check, then bounded
doubling steps with the official run.sh, bracket + confirm, recommendation ≤ ½ sustainable. The generator
must run on the load host (co-locating it with the SUT would invalidate the datum — recorded as a hard rule
in the protocol). Blocked while the workstation↔load-host path is down.

### Task 4 — clock diagnosis — OFFLINE PART DONE; live windows pending
`results/auto-20261028/clock/clock-diagnosis.md`: the +330 ms median is **probe geometry bias** (the remote
`date` runs near the END of a full SSH session while the correction assumes the midpoint; ½×666 ms ≈ 330 ms),
the ~830 ms spread is SSH session-duration variance; both hosts are NTP-synchronized so the true offset is
likely ms-scale. v2 probe prepared (persistent ControlMaster + interval-intersection + NTP self-reports).
Claim classification: primary latency is same-clock (unaffected); cross-host comparisons stay unqualified.
Error budget stated conditionally (U ≤ P/10) without inventing an SLO.

### Tasks 5–8 — prepared/blocked or done offline
- B15/B17 inspected in the repo records: both are owner-decision items (O5/O6 in
  `docs/methodology-reconciliation.md`); no independent work exists — documented, not inferred.
- FIWARE budget: `results/auto-20261028/budget/fiware-supporting-tier.md` (measured idle numbers; PROPOSED
  additions 2,048 MiB on infra; fits N=1..4 with the platform tier; TimescaleDB-under-load validation still
  owed before adoption).
- EdgeX describe (B14) + D27 populated-data (isolated `d27test` namespace, script ready:
  `results/auto-20261028/isol/d27-populated.sh`): pending access.

### Task 2 (continued) — pilot V2 + recommendation — VERIFIED
- **V2 (1536/1536/768)**: boot clean (OOMKilled=false, RestartCount=0 on all six app-tier containers);
  post-telemetry cores 960/986 MiB of 1536 = **62.5%/64.2%**; strict analyzer over 1,209 samples
  (21:51:46→22:06:25, boot+idle+telemetry+post-idle): **zero >95% samples of any length**.
- **Pilot recommendation (PROPOSED, not adopted):** the steady-state working set at this registry size is
  ~0.96–1.0 GiB per tb-core/rule-engine regardless of the container limit (JVM heap 512m fixed + nonheap
  ~250 MiB + off-heap/page-cache ~200–250 MiB). The container limit must exceed that by a safe margin:
  - **1280 MiB (V1) clears the >95% stop condition** (idle 71–74%, post-load 76–77%) and fits k ≤ 3
    (3,072 MiB/replica → k=3 total 13,056 ≤ 14,848; k=4 = 16,128 ✗).
  - **1536 MiB (V2)** adds ~10 points of headroom (62–64%) but fits only k ≤ 2 (3,840/replica →
    k=3 = 15,360 ✗).
  - **k=4 does not fit either variant** — the constraint is reported, per the stage plan; any k=4 attempt
    needs an owner decision on the total budget, not a silent limit change.
  - Stack restored to the ADOPTED 1024 MiB limits after the pilot (temporary settings removed; evidence
    retained). The adopted-limit state showed the expected ~96–97% band again (F1/F2 record unchanged).

### Task 3 — generator-ceiling exploration — EXECUTED (bracket obtained; generator-only ceiling still open)
Protocol: `docs/protocol-20261028-generator-ceiling.md`. Evidence: `results/auto-20261028/ceiling/`.
- **Phase 0 (sink, loopback):** zero loss/unparsed/overflow at every rate the driver produced (≥876/s) —
  the receiver is not the bottleneck at campaign rates.
- **Phase 1 (full path, k6 on the load host → TB k=2 @ adopted limits):** 100 r/s INVALID (dropped
  iterations; loss on the first attempt, zero loss with MAX_VUS=300 but 1.1% dropped — VU starvation from
  platform latency tails); **50 r/s VALID** (stored==sent, 0 drops). Bracket: **[50 pass, 100 fail]**.
- **Reading:** the binding constraint at this configuration is the PLATFORM, not the generator; the
  generator-only ceiling (null/echo receiver) was not run, so B10's generator-side half stays open. No
  max-campaign-rate recommendation is issued from a platform-limited datum (the 2× rule needs the QUALIFIED
  platform rate once the memory/k decisions land).

### Task 5 — EdgeX describe (B14) — VERIFIED (with a fix)
Live schema: `core_data.reading(event_id uuid, device_info_id integer, origin bigint, value text,
binaryvalue bytea, objectvalue jsonb, numeric_value numeric)` — **no `devicename` column**; the export SQL
was fixed to join `core_data.device_info` (id, devicename, …) via `reading.device_info_id`; regression:
export runs clean against the live DB (0 rows for an unknown run, rc=0). B14 now covered on all three
platforms.

### Task 6 — D27 populated-data verification — VERIFIED (isolated namespace)
Seeded in the candidate mongo under service `d27test` (throwaway iot-agent container, then direct mongosh
after the agent's image proved curl-less): 1 complete device + a deliberate duplicate copy + 1
incomplete-key row. Audit output: `{"total":3, "incomplete":1, "duplicate_groups":1,
duplicate_examples:[["d27test","/","d27k1","d27dev1"], count 2]}` — both conditions detected exactly.
Namespace cleaned afterwards (0 rows); throwaway container removed; NO index created anywhere.

### Access incidents (recorded)
Two network dropouts on the workstation path (hotspot gateway answering TCP without banners): ~12:45–13:03Z
and ~13:15–13:49Z; a third period with only the load host unreachable (worked around via
`ssh -J ramonavm2 -i ~/.ssh/id_ed25519_vps root@87.107.191.32`). All live work was executed through these
windows; nothing was lost.
