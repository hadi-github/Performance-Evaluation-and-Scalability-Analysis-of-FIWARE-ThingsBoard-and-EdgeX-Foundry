# Measurement verification follow-up — 2026-10-31

Continuation of `96c4fdb` / `verify-20261029.md`. **Main campaign remains NO-GO. No push.**
All evidence below is repository-relative. Raw results remain outside Git under the existing
publication policy; the dated amendment index and source changes are versioned.

## Claim-level readiness

| Claim | State | Evidence / limit |
|---|---|---|
| Original rehearsal message chains | Preserved | All 42 original evidence files match their original hashes. The pre-existing checksum manifest also lists a deleted temporary `SHA256SUMS.tmp`; that manifest defect is reported, not silently rewritten. `results/verification-20261031/original-rehearsal-integrity.json` |
| CPU boundary collection in authoritative runner | Implemented; tested live on a controlled receiver | `benchctl run -cpu-snapshot-command ...` retains raw captures, command request/response times and explicit per-phase bracketing skew. See `docs/development/tooling-interfaces.md`. It does not retrofit counters into the old rehearsals. |
| Counter deltas, resets/restarts, unavailable data | Unit-tested | Exact deltas; changed StartedAt; changed ID even with identical StartedAt; decreasing counters; missing containers; missing/negative counters; duplicate identities; reverse intervals; unnamed denominators. Missing/invalid cost is absent, never zero. |
| Exact platform phase CPU cost | Unavailable for old rehearsals | No endpoint snapshots were captured then. Automatic capture currently reports actual enclosing capture intervals, not exact nominal phase intervals. Full platform membership and cross-host phase-clock alignment must be qualified in a future bounded run. |
| Three CSV/PNG/SVG/offline exports | Complete | `results/verification-20261031/reports-final/{REH-TB-V2,REH-EDGEX-V1,REH-FIWARE-V4}/index.html`; 36 PNG/SVG pairs, exact UTC query windows, expressions, raw responses, CSV and per-export checksums. 10,302 finite CSV values verified against raw responses; no query errors. |
| Missing-data treatment | Verified | Each export has 15 OK queries and 4 NO_DATA queries. Failure/unknown/drop time series can be absent when those counters were never emitted; Kafka lag is absent even in TB's window and must not be interpreted as healthy zero. Synthetic error/all-NaN/gap tests pass. |
| Generator → receiver, cross-host 400/s | Verified for the stated fixture scope | Immediate and 700 ms delayed runs: 120 measured seconds each, 48,000 sends each, zero drops/early sends, complete outcome accounting. Warm-up included: 51,000 exact identities per valid run, no receiver anomalies. |
| Receiver/network path headroom | Demonstrated at 800/s, 60 measured seconds | Same cross-host path and logical telemetry serialization, monitoring and recording enabled. 48,000 measured sends, zero drops, lag p99 2.8 ms; 51,000/51,000 receiver identities including warm-up. Not a theoretical link-capacity claim. |
| Delayed-response VU sensitivity | Verified | 100-VU negative control INVALID with 31,201 drops; 700-VU case VALID with lag p99 1.5 ms and scheduled-to-ack p99 704 ms. No gate weakened. |
| Full campaign generator envelope | Not yet qualified | These controlled-receiver plans use the fixture adapter and 100 round-robin device identities, not 1,000-device platform provisioning or all three platform-specific adapters. Do not extrapolate this result into an unconditional 200/s campaign authorization. |
| FIWARE D27 populated registry | Read-only audit complete | 100 registry rows, zero incomplete keys and zero duplicate groups; independent empty-string/null check also clean. No index creation or device deletion. |
| TELEM profiles | Pending owner adoption | Dated reconciliation: `research/protocols/slo-profile-reconciliation-20261029.md`. TELEM-500 and 250/1000/5000 ms sensitivities remain preregistered candidates. 3GPP source attribution remains unverified. Archive byte check passes. |
| FIWARE history path | Confirmed at rehearsal scope | Full UL ingress through baseline HAProxy/IoT Agent/Orion, with independent sink and QL/Timescale notification branches. See `docs/architecture/fiware-benchmark-path.md`; full path checked at 25/s, store-only probe at 50/s. No higher-rate compatibility/adequacy claim. |

## Live tests and exact scope

Load host `87.107.191.32`, k6 pinned to cores `0-1`, `GOGC=400`, remote-write to
`http://127.0.0.1:9090/api/v1/write`. Outbound path is real host networking to SUT
`87.107.191.107:18080` (immediate) or `:18081` (700 ms delay). Receiver containers run on
SUT cores 6 and 7 respectively, 256 MiB each; OS-side capture helper is pinned to core 0.
Existing benchmark services and exporters stayed running; this is not an otherwise-idle-host result.
Payload is the actual fixture adapter's JSON with run_id, global seq, device_id,
scheduled_send_ms and three sensor values. Connection model is one in-flight HTTP/1.1 request
per VU, keep-alive, no retries. Each receiver retains every receipt to NDJSON and flushes on SIGTERM.

| Run ID (under `results/verification-20261031/qual400-20261031/`) | Measured phase | Result |
|---|---|---|
| `Q400-IMMEDIATE-20261031T1805Z` | 400/s × 120 s | VALID; 48,000 acknowledged; lag p99 2 ms; ack p99 3.5 ms |
| `Q400-DELAY-UNDERSIZED-20261031T1810Z` | 400/s × 120 s, 100 VUs | INVALID; 17,200 attempted, 31,201 drops; retained, not a platform FAIL |
| `Q400-DELAY-SIZED-20261031T1810Z` | 400/s × 120 s, 700 VUs | VALID; 48,000 acknowledged; lag p99 1.5 ms; ack p99 704 ms |
| `Q800-RECEIVER-CPU-20261031` | 800/s × 60 s | VALID; 48,000 acknowledged; lag p99 2.8 ms; ack p99 6.5 ms |

All include 30 s warm-up at 100/s. No valid run has early sends, missing outcome accounting,
receiver duplicates, missing identities or unexpected identities. Receiver logs show zero overflow;
the 100-VU invalid control is deliberately not assigned an exact planned identity reconciliation.
Monitoring queries/raw responses for each run are retained under its `monitoring/` subdirectory.

Receiver CPU capture in the 800/s run: 21,306,745 → 37,185,270 usage_usec = **15.878525 CPU-s**.
Same full container ID and StartedAt at both endpoints; no counter rewind. Capture interval:
`1793470527.877607545`–`1793470588.777254572` epoch seconds. Nominal measured phase:
`1793470528.209`–`1793470588.209`. Before/after skew: 0.331393 / 0.568254 s. Denominator is
explicitly **48,000 attempted measured-step HTTP sends**, giving 0.000330802604 CPU-s/send over
the enclosing capture interval. This is receiver cost only, not total SUT/platform CPU cost;
cross-host clock alignment was not newly qualified. Counter deltas themselves use only SUT counters.

## Provenance caution

The load-host deployment's `GIT_REV` marker still named `204d0c8` during the tests. It is **stale**
and must not be treated as the complete source identity of the new CPU runner. Original manifests
are preserved unchanged. `deployed-binaries.sha256` identifies the exact locally built receiver and
CPU-enabled benchctl binaries; per-run manifests retain the hashes of the k6 scripts actually used.
The final source commit includes additional validation/metadata fixes made after the live CPU smoke.
This limitation must be carried if the exploratory qualification data are reused.

## Export reproduction and checks

Exact times are from each successful rehearsal's `k6-summary.json` scenario_start_ms, not its name:

| Run | Measurement UTC start | Measurement UTC end |
|---|---|---|
| REH-TB-V2 | 2026-10-29T12:57:39.940Z | 2026-10-29T12:59:39.940Z |
| REH-EDGEX-V1 | 2026-10-29T13:15:27.214Z | 2026-10-29T13:17:27.214Z |
| REH-FIWARE-V4 | 2026-10-29T14:21:17.126Z | 2026-10-29T14:23:17.126Z |

Queries include 60 s of context either side. The original platform rehearsals were **not rerun**.
Prometheus was accessed through a local SSH tunnel to the load host. Rendering used local
`python:3.12-slim` with matplotlib installed into `/tmp/opencode/render-deps` from the Runflare
PyPI mirror (no server package installation). Image and dependency versions are saved alongside evidence.
The initial exports (`graphs/`) retain queried responses; `reports-final/` was rendered offline from them.

```bash
python3 scripts/verify-run-export.py results/verification-20261031/reports-final/*
python3 scripts/verify-controlled-receiver.py results/verification-20261031
go run ./cmd/benchctl check -run results/verification-20261031/qual400-20261031/Q800-RECEIVER-CPU-20261031
go test ./internal/... ./cmd/...
go test -count=1 -v ./tests/integration
bash scripts/check-preregistration.sh
```

Offline rerender, with matplotlib available, uses the same run/platform/start/end arguments as
`export.json`, `--responses-dir results/verification-20261031/graphs/<run_id>` and a **new** `--out`
directory. Metadata/query mismatches are rejected. PNGs were visually inspected for throughput,
CPU and missing-data panels; finite CSV-to-response equality and offline image dependencies are
machine-verified. No chart p99s are averaged into run p99s.

## D27 audit reproduction (read-only)

On the SUT verification tree, the actual populated audit was:

```bash
bash scripts/device-index.sh export > registry-snapshot.ndjson
bash scripts/device-index.sh audit registry-snapshot.ndjson
```

The source is `deploy/fiware/scripts/device-index.sh`; it was copied into the remote verification
tree for this read-only invocation. `audit` without an input file reads stdin and does **not** export
the live registry itself. Output and the full 100-row snapshot are in `results/verification-20261031/d27/`.

## Services and rollback

- `qual-ft-400` and `qual-ft-delay` are **stopped**, not removed. Both raw receipt files were flushed
  and retrieved. **Do not restart these containers:** fixture-target uses `os.Create`, so restarting
  with the same output path truncates that evidence. Create a new name/output path for future tests.
- The temporary CPU HTTP helper (port 18082, source-IP-restricted to the load host/loopback) was
  stopped using its recorded PID. No new receiver/helper ports remain listening on the SUT.
- Existing TB, FIWARE baseline, fiwhc candidate, EdgeX, exporters, monitoring and yaade were not
  restarted or reconfigured in this continuation. Current inventories:
  `service-state-sut.txt`, `service-state-load.txt`. Existing ceiling sink on :18090 remains running;
  rehearsal sink :18091 is stopped.
- No volumes or existing devices were removed. No platform versions changed. If reverting the
  earlier session's service starts is required, use `docker compose stop` in the corresponding
  original remote candidate directory; never `down -v`:
  `/root/iot-bench-deploy-cand-auto20261027/{edgex,fiware-baseline,fiware-history-candidate}`.
  For fiwhc, stop only `orion quantumleap` to return to the earlier retained DB state.
- Local failed renderer container `iot-export-renderer-20261031` is exited; dependencies remain in
  `/tmp/opencode/render-deps`. The successful renders used ephemeral containers.

## Next actions / owner choices

Technical (not owner decisions): qualify the complete 1,000-device/platform-adapter generator
configuration, verify full platform CPU boundary membership/clock alignment, extend FIWARE history
qualification above 50/s, close remaining scaling/monitoring gates. The current path evidence is
bounded and must retain its fixture/100-device/duration limits.

Owner choices genuinely still open: adoption/supersession of the preregistered SLO profiles and
associated unresolved overload/uncertainty decisions. The approved k, memory, rate sweep and campaign
ceiling decisions are not requested again. Collection tooling has stronger verification; SLO-based
campaign evaluation remains **NO-GO**.
