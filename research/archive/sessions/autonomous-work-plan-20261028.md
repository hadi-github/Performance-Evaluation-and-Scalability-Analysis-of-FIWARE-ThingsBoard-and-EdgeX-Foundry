# Autonomous session plan (auto-20261028, started 2026-10-28 ~20:35 local / 17:35Z window per host clocks)

**Session ID:** `auto-20261028` · **Baseline HEAD:** `2bff5171ed8f21e99ef740111b841b2895ce1593` (clean tree).
Prior session (`auto-20261027`) records preserved untouched; this session writes `-20261028` suffixed docs.

## Owner authorization (this session; recorded summary — full prompt text held by the owner)

- Access both servers; implement fixes; local tests; **bounded live qualification tests**.
- Stop/recreate intended benchmark services to isolate tests or apply **temporary pilot configurations**,
  preserving volumes and existing data.
- **Forbidden:** deleting volumes, resetting databases, removing existing devices, changing platform versions,
  touching `yaade`, pushing commits, starting the main campaign.
- Temporary pilot settings recorded separately from adopted benchmark settings; no asking on routine choices;
  blocked research decisions documented and independent work continues.
- Explicitly authorized: **TB memory-sizing pilot** (bounded container-limit increases, JVM heap unchanged
  initially), **labelled generator-ceiling exploration** (no owner max-rate needed), **clock diagnosis**,
  remaining B12–B14 checks, D27 populated-data verification in an isolated namespace, FIWARE budget doc.

## Host/service state at session start (verified 2026-10-28)

- SUT `ramona-vm-2` (87.107.191.107): 31 containers — TB k=2 stack (`tb-*`), EdgeX stack (`edgex-*`),
  FIWARE candidate (`fiwhc-*`), FIWARE baseline (`fiware-*`), exporters (`exporters-*`). All from
  auto-20261027, volumes intact (2,013 devices in TB registry: 1,000 f1 + 1,000 f2 + 13 probe).
- Load `ramona-vm-q` (87.107.191.32): monitoring stack (prometheus/cadvisor/grafana/node-exporter, 5 d) +
  `yaade` (untouched). Candidate generator dir `/root/iot-bench-cand-auto20261027/workloads` with the two
  VALID smoke results.
- Evidence manifests re-verified: 758/758 historical + 26/26 auto-20261027.

## Queue

| # | Task | Acceptance | State |
|---|---|---|---|
| 0 | Session records + baseline + manifest verify | files + 758/26 OK | VERIFIED |
| 1 | Isolation prep per live test (stop unrelated stacks, record layout, fit check) | transcript per test | READY |
| 2 | TB memory-sizing pilot (V1 1.25g, V2 1.5g; heap unchanged; written protocol; strict monitoring) | per-variant evidence + recommendation + k-fit table | READY |
| 3 | Generator-ceiling exploration (sink-first, then full path, bounded doubling steps, bracket + confirm) | bracketed sustainable rate + ≤½ recommendation (labelled) | READY |
| 4 | Clock diagnosis (decompose probe, repeat windows, claim classification, conditional budget) | decomposition + windows + budget | READY |
| 5 | B14 EdgeX `describe` live | column dump + verdict (+fix if needed) | READY |
| 6 | D27 populated-data verification (isolated namespace, seeded duplicate/incomplete) | exact detection counts | READY |
| 7 | FIWARE supporting-tier budget (live numbers, proposed-only changes) | budget table | READY |
| 8 | B15/B17 inspection from repo records | documented | READY |
| 9 | Close-out: blockers, handoff, manifests, commits, stable server state | 4 recommendations recorded | READY |

Rules: sequential live tests; per-op timeouts; BLOCKED after ~30 min/3 failed attempts; evidence under
`results/auto-20261028/`; preserve every prior record; deadline 3 h from start (last 30 min close-out).
