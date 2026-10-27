# Qualification blockers (before any deployment or experimental run)

**Recorded 2026-10-26; reassessed 2026-10-27 (session auto-20261027, owner authorization for this session,
server access + live checks + candidate deploys on the pre-registered hosts).** Overall: **NO-GO for the main
experimental campaign** (unchanged); qualification-scoped tests authorized for this session were executed and
are recorded per row. Every row must still be resolved, or explicitly accepted by the owner as a recorded
deviation, before the gate it blocks.
Columns: **Gate** — the action this blocks (**Deploy**, **Smoke**, **Run**, or **Claim** = a report statement).
**Type** — **T** technical, **S** scientific or methodological, **O** owner decision or authorization.

| ID | Blocker | Gate | Type | Current state (evidence) | To clear it |
|---|---|---|---|---|---|
| B1 | SUT (`87.107.191.107`) and load host (`87.107.191.32`) unreachable from this environment | Deploy, Run | T | All pre-registered host targets reachable via configured SSH aliases (Gates A–C transcripts `results/auto-20261027/preflight-*.txt`); identity mapping verified not inferred (SUT=87.107.191.107 = `ramona-vm-2` alias ramonavm2; load=87.107.191.32 = `ramona-vm-q` alias ramonavm1); both NTP-synced; SUT had 0 running containers pre-session; load host runs the project monitoring stack + unrelated `yaade` (untouched). | Re-verify host identity/clock before any future platform change (granted and live-verified this session). |

| B2 | QuantumLeap data model and upsert/same-timestamp behaviour unverified | Deploy (FIWARE) | T, S | **LIVE-PASSED on the isolated candidate 2026-10-27** (project `fiwhc`, fresh volumes, pinned QL 1.0.0): four-payload probe reproduced the source reading — one row per notification; duplicate and same-timestamp rows stored independently (no collapse); failed insert's original JSON preserved in `__original_ngsi_entity__` with `failedBatchID`; identity retrievable from that JSON. | Rerun the probe if any pinned image version changes; keep this as the record for the full-stack deployment using the same images. |

| B3 | QuantumLeap tested Orion range (2.4.2–3.3.1) excludes the pinned Orion 4.5.0 | Deploy (FIWARE) | T, O | **LIVE-PAIRED 2026-10-27 (owner B3 choice: keep Orion 4.5.0):** Orion 4.5.0 subscription → QL 1.0.0 works end-to-end: 10/10 labelled updates stored with exact identities/timestamps; deliberate drop (QL down) → acknowledged-but-not-stored; subscription stays `active`. One live low-rate data point, not an upstream guarantee (tested set still ends at 3.3.1). | Before the campaign: higher rates, restart flows and full-stack paths under this pairing must be qualified in the dedicated stages. |

| B4 | Memory and CPU placement of QuantumLeap and TimescaleDB inside the fixed supporting tier | Deploy (FIWARE) | T, O | **ADOPTED by the owner 2026-10-29:** FIWARE history addition = TimescaleDB 1536 + QuantumLeap 512 = 2,048 MiB on the infra cpuset (fits all N; N=4 total 9,472 ≤ 14,848). **Validated under load 2026-10-29:** 3,001/3,001 identities through Orion→QL at 50 r/s, zero missing/duplicates/unrecoverable; timescale 137/1536 MiB, QL 77/512 MiB under load. Higher-rate adequacy still open. Owner decision assigns the candidate allocations into the adopted fixed budget (measured input above). | Higher-rate adequacy validation. |

| B5 | No FIWARE history identity export; `history-ids.sh fiware` exits 3 | Claim (FIWARE store-side), Smoke | T | **IMPLEMENTED + LIVE-REPLAYED 2026-10-27:** `scripts/history-ids.sh fiware` (+ `fiware describe`): per-tenant schema, dynamic table discovery, error-column recovery + explicit unrecoverable-row markers (never silent drops). Replay 24 planned identities → rows 24, distinct 23, missing exactly {11} (deliberate drop), duplicate exactly seq5 (2 copies), zero beyond-plan; restart visibility: full QL+DB restart preserved 15/15 rows. | Exercise the export through the full-stack (iot-agent) path under a real run before store-side claims. |

| B6 | ThingsBoard k=1/k=2 feasibility not executed; k=4 unqualified (D11 open) | Run (ThingsBoard) | T, S, O | **1280 MiB qualified under load 2026-10-29 (owner candidate decision):** 50 r/s VALID, **200 r/s VALID twice** (0 dropped, stored==sent, p99 330 ms–1.47 s), 400 r/s INVALID (saturation, p95 = the 10 s timeout) at k=2; memory 76–79% under load, zero OOM/restarts. Budget table: k=1 6,912 ✓, k=2 9,984 ✓, k=3 13,056 ✓, k=4 16,128 ✗ (overflow 1,280 MiB; driver = the raised per-replica limits vs the 768 MiB headroom the old limits left). Owner options O1–O5 recorded; O4 (cap k=3 per the adopted S4 fallback) is the measured-consistent choice. F1/F2 records preserved. The k=1 deviation decision now has a measured basis: adopt 1280 MiB (fits k≤3) or 1536 MiB (k≤2), or keep 1024 with the recorded stop-condition. | Owner records the k decision (O1-O5). |


| B7 | ~~not implemented~~ → **implemented locally; was awaiting live checks** | Deploy (ThingsBoard) | T | **LIVE-VERIFIED 2026-10-27** (authorized controlled restart): `/config` hashes match in-container; entrypoint exported `TB_SERVICE_ID` from hostname; effective JVM flags unchanged heap + ExitOnOutOfMemoryError + /config/logback; writable per-replica log dirs and GC logs; smoke `telemetry-stored:1`; per-replica log isolation confirmed at k=2 (each replica unique hostname dir). | Only an intentional isolated OOM/kill probe remains, on a disposable test container (NOT the retained deployment; unauthorized this session). |

| B8 | D27 duplicate audit implemented/offline-tested; live registry empty at rest | Claim (FIWARE index), Run | T, O | **Audited read-only 2026-10-27** on the live restored baseline registry (`fiware-mongo-1`, persisted volume): total=0, incomplete=0, duplicate_groups=0 — registry empty at rest (prior sessions delete their provisioning in teardown), so no duplicate could surface by population; detection logic covered by the offline (`fiware/scripts/test-device-index.sh`). No writes, no index created. | Re-run the live audit when a registry holds provisioned data (next real qualification stage). |

| B9 | ~~not implemented~~ → **implemented; disk fields needing live verification** | Claim (EdgeX bottleneck), Run | T | **LIVE-VERIFIED 2026-10-27** on a bounded generator run (`edgex-n1-r2-20261027T115425Z`, 2 req/s VALID): `benchctl resources` queried live Prometheus (ladder instance 87.107.191.107:9100), raw await/writes responses retained, recomputation matches per-device output to floating precision (max rel diff 1.9e-16); postgres_wal stays explicitly `unavailable` (no diagnostic overlay in a generator run); zero-write devices yield NO await (absent, not zero). | Repeat on a diagnostic-overlay run to live-verify the WAL block; the disk gate is covered. |

| B10 | Generator qualification at 2× the maximum planned rate not measured | Run (any rate above the qualified level) | T, S | **Generator-only ceiling measured 2026-10-29 (loopback):** 3200 r/s sustainable (two consecutive VALID passes, zero drops, all messages received by the sink — the sink is thereby qualified AT 3200 r/s through the real path); 6400 INVALID (97.9% achieved, p99 459 ms, loopback saturation). **Recommended max campaign rate ≤ 1600 r/s** (½ × 3200), subject to the cross-host path and the platform's qualified rate (200 r/s confirmed at k=2/1280 — the PLATFORM binds first at this configuration). Platform-bracket correction: the 2026-10-28 100/s INVALIDs were VU starvation; with proper VU sizing 100 r/s and 200 r/s are VALID at 1280 MiB. | Generator-side done; the remaining half is the cross-host path + platform-rate qualification. |


|| B11  No measured cross-host clock offset  Claim(cross-host timing)  T, O | **Corrected probe executed 2026-10-29:** interval-intersection over persistent ControlMaster, 35 pairs/leg. workstation↔SUT [1334, 1388] ms; workstation↔load [1325, 1384] ms; **inferred load↔SUT offset ∈ [−63, +50] ms** (both hosts NTP stratum-2, self-reports captured; the workstation's own clock is ~1.36 s off — never a reference). Cross-host claims remain unqualified: U=63 ms with the (proposed) U ≤ P/10 rule would need P ≥ 630 ms and no SLO exists; narrowing U needs a direct SUT→load probe (key path not authorized to create). |


| B12 | Sink capacity at the highest planned rate unmeasured | Run | T | **Observation gap 2026-10-27:** the sink was exercised inside the two VALID smokes (stored==sent in both), but its own independent capacity envelope at higher rates remains unmeasured. | Sink stress at an adopted target rate once B10 defines it. |

| B13 | Monitoring layout cost inside the generator 2× check not measured | Run | T | **Partially measured 2026-10-27:** load-host monitoring existed before the session and was untouched; SUT-side exporters (node-exporter/cadvisor/postgres-exporter/nats-exporter, 128m-limits, cpuset 0) installed for B9's checks; their own cost inside a 2× qualification has not been isolated. | Isolate exporters' cost at the qualification rate once B10 defines it. |

|| B14  ThingsBoard `device`/`ts` history export and EdgeX `core_data.reading` column names unverified  Claim(collision analysis; EdgeX store-side)  T | **EdgeX identity export proven on real data 2026-10-29:** 600 labelled readings through the gate's own contract; export reconciled exactly (600/600, 0 missing, 0 duplicates). All three platform branches now verified with non-empty data. |


| B15 | Overload anchor (D1 open) and capacity-uncertainty method (D2) open | Overload campaign | S, O | Fingerprint notes §9 (`docs/methodology-reconciliation.md` O5/O6) | Owner decision before the overload stage |
| B16 | Raw evidence not versioned; fresh clone cannot reproduce numbers | Claim (reproducibility) | T | **Maintained 2026-10-27:** pre-session evidence unchanged (758/758 manifest files re-verify with `sha256sum -c docs/evidence-integrity.sha256`); this session's live evidence separately under `results/auto-20261027/**` with its own SHA256SUMS. | Publish the evidence inventory with checksums at report time. |

| B17 | S1–S5 amendments still need validation by qualification | Claim (all scaling statements) | S | Recorded (`docs/methodology-reconciliation.md`, D35–D39); not validated experimentally | Carry the amendment IDs in every report; validate through B10 and the stage plans |

## Not blockers, but excluded from the campaign

These are deferred under Step 5 and must not be run as part of qualification: D29b extra EdgeX replicas; D29d
`commit_delay` experimentation; Orion `-reqPoolSize`; ThingsBoard queue poll intervals; D29c asynchronous commit
(DIAG-only, not a qualification setting). See D42.

## What is ready now (local only)

- Pre-registration archive, with `scripts/check-preregistration.sh` (hashes verified).
- Methodology reconciliation S1–S5 (`docs/methodology-reconciliation.md`).
- Evidence-integrity manifest (`scripts/evidence-manifest.py`; no `results/**` change).
- Measurement-integrity audit (`benchctl audit`), per-step qualified accounting, known-answer tests; `Clean`
  requires a history export; ambiguous outcomes and missing labels recorded (2026-10-27).
- D28 official ThingsBoard config vendored, hashed, mounted read-only; writable log volumes; 95 static checks
  (`scripts/check-thingsboard-config.sh`); **live-verified 2026-10-27** (`docs/thingsboard-d28-verification.md`
  §5 checks all pass at k=1 and k=2, smoke `telemetry-stored:1`).
- D29a per-run disk/WAL report (`benchctl resources`); WAL values reproduce all retained DIAG runs exactly; disk
  fields LIVE-VERIFIED against raw Prometheus responses 2026-10-27 (`docs/edgex-resources-d29a.md`,
  `results/auto-20261027/b9/`).
- D27 duplicate gate (`fiware/scripts/device-index.sh`, offline tests); LIVE read-only audit 2026-10-27 on the
  baseline registry (empty, clean).
- B6 F1/F2 executed this session with retained evidence (`results/auto-20261027/f1/`, `f2/`); k=4/F3 unauthorized.
- B9–B13 partial live-verified states recorded in the blocker table with evidence paths.
- Server-access preflight remains documented for future sessions.
- FIWARE history design with pinned-source review of QL 1.0.0: data model, no-upsert behaviour, delivery path,
  version matrix, integration sketch and smallest live test (`docs/fiware-history-design.md` §3, §8, §8.1).
- Server-access preflight checklist (read-only Gates A–C) for the day access is granted
  (`docs/server-access-preflight.md`).
- Design documents: `docs/thingsboard-feasibility-plan.md`, `docs/measurement-integrity.md`.

## Next safely executable task

The next stage is owner-decision-gated: the F1 stop-condition at k=1 needs an owner-recorded deviation (memory
increase or k) before any further ThingsBoard scaling, and full rate qualification (B10/B12/B13) needs an
owner-chosen target rate — or an authorized labelled generator-ceiling exploration first. Server access and the
session's authorization scope are recorded in `docs/autonomous-work-plan.md`.

## Session authorization (recorded 2026-10-27)

Full text under `docs/autonomous-work-plan.md` ("Owner authorization"); covers preflight, D28 controlled restart,
the D27 read-only audit, the isolated FIWARE candidate, F1–F2, bounded smokes and qualification-scoped tests for
this session only. It does NOT authorize: deleting existing data/volumes, index creation, F3/k=4, tuning, version
switches, the main experimental campaign, pushes or publishing.
