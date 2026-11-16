# Test catalogue and verification guide

This catalogue covers new/updated measurement, capture, campaign-safety and
progress regressions in the current handoff. Tests are engineering evidence,
not official repetitions or proof of platform capacity. See [status](../status.md)
for live evidence and [publication checks](../operations/publication-handoff.md).

## Commands and safety boundaries

From the repository root, with the Go toolchain in `go.mod`, Python 3 and Node.js:

```sh
make test-short   # fast Go suite; skips Docker integration
make test         # full Go suite
make check        # CI-equivalent Go gate: build, vet, gofmt, full tests
make test-python  # both offline Python discovery commands
make test-node    # offline adapter regressions
```

The underlying commands, used by CI:

```sh
# Short mode skips Docker integration; never contacts benchmark hosts.
go test -short -count=1 ./internal/... ./cmd/... ./tests/...
go test -race -count=1 ./internal/runner ./cmd/benchctl
python3 -B -m unittest discover -s tests -p '*_test.py'
python3 -B -m unittest discover -s tests/offline -p 'test_*.py'
node --test tests/offline/*.test.mjs
bash tests/capture_thingsboard_file_logs_test.sh
bash scripts/check-preregistration.sh
```

One Python file: `python3 -B -m unittest tests.capture_cutoff_test -v`
(substitute its module name). One Go group:
`go test ./internal/runner -run TestProgress -count=1 -v`.

**Docker integration is not opt-in by environment flag:** without `-short`,
`tests/integration/generator_test.go` automatically runs if Docker is available,
otherwise skips. It starts a local in-process fixture and pinned k6 containers
with synthetic faults; it does not deploy real platforms or contact benchmark
hosts. It can pull the pinned image if absent and takes roughly a couple of
minutes. Deliberately choose when to run `go test -count=1 ./tests/integration -v`.

## Python regressions (`tests/`)

These use synthetic files, temporary directories, local fixtures and/or stubbed
subprocesses. SSH, Docker, firewall and service commands in launcher/capture
safety tests are mocked, not executed against benchmark hosts.

| File | What it protects |
|---|---|
| `analyze_dispatch_run_test.py` | Per-message dispatch percentiles; UNKNOWN versus missing; earliest duplicate receipt; negative delays/clock gaps; fresh GOGC/cgroup/effective-CPU/ancestor-quota evidence; platform malformed/truncated output versus sink overflow/capture loss; failed history exports stay unassessed. |
| `dispatch_integrity_test.py` | Constant arrivals pass, bursts fail; frozen 50/s headroom bins; legacy plans cannot be silently reinterpreted as dispatch-policy runs. |
| `protocol_receipts_test.py` | Full-body first receipts, identity/device/schedule and summary accounting; schema-3 dispatch marker; actual measurement-relative cutoff and quiet/late coverage; nonzero exits, fractional counts, missing/duplicate records and manifest mismatches fail closed. |
| `capture_cutoff_test.py` | Executes the actual embedded cutoff program: valid measurement epoch gives the fixed deadline; zero/null/negative/nonfinite/unstarted epochs cannot manufacture a cutoff. |
| `run_capture_test.py` | Fake sampler/capture/analyzer orchestration; environment, cpuset, port, monitoring and GOGC forwarding; analysis after capture failure; sampler failure propagation; missing window stops analysis. |
| `post_run_analysis_test.py` | Automatic analysis/status output; no overwrite by default; missing host proof never attested from the analysis workstation; missing clocks stay unassessed; duplicate-only/empty delivery are retained failures; history absence stays separate from sink success. |
| `launch_campaign_test.py` | Frozen IDs/seeds/rates/devices/payload/windows/VUs; qualification versus ordinary scope; both TB N=2 backends; image/resource/continuity gates; unsafe resume disabled; staged helper hashes; export-profile shutdown; retrieval and history/cleanup/finish/unlock ordering; failures retain locks/results; post-run defect overrides success without deleting latency. |
| `aggregate_runs_test.py` | Per-run summaries without pooling message percentiles; qualification excluded from ordinary statistics; resource/history/post-run failures stay visible; overwrite guard; proven zero-traffic setup recovery is not platform loss; contradictory recovery sidecars fail. |
| `edgex_driver_ready_test.py` | Initialized POST-only ingest route rather than early ping; wait through startup; timeout fails closed before provisioning; gate order. |
| `outbound_route_test.py` | Orion sink subscriptions: patch an old route, preserve a correct route, reject ambiguous candidates. |
| `sink_firewall_test.py` | Mock iptables failure/absence: retain ownership marker when removal cannot be verified; marker cleanup alone is not successful sink shutdown. Never edits real firewall rules. |
| `adapter_receiver_test.py` | Local receiver decodes every adapter wire format and preserves identities; canonical TB payload/device ID and 20-character token handling. |
| `export_run_test.py` | Synthetic Prometheus export: fractional timestamps, gaps, NaN, absent series/query errors remain unavailable, not zero; optional plotting checks can skip when dependencies are missing. |
| `resume_safe_provision_test.py` | Preparation preserves existing storage; guards against destructive reset commands. |
| `qualification_packaging_test.py` | Fresh-clone copy of the historical one-off qualification script: default invocation labels itself unsupported and launches nothing; execution with the ignored companion absent fails before output creation or remote access. |
| `fixed_point_variants_test.py` | Offline W1–W4 staging: exactly one treatment field changes, all platforms receive five proposed attempts, shared HTTP/VUs/seeds and 300/600-second phases match B0, W4 keeps k=2/pool 4–5, IDs are unique and width-matched, message counts/interleaving/hash bindings are correct, B0 stays byte-identical, overwrite/drift is refused. No hosts or traffic. |

`tests/offline/test_history_export.py` and `test_history_payload.py` exercise
history parsing/normalization, including the FIWARE candidate, using fixtures
and mocked commands. Their `test_*.py` naming requires the separate discovery
command above.

## Go regressions in `internal/`

| File | Coverage added/updated in this handoff |
|---|---|
| `internal/runner/progress_test.go` | Incremental/partial NDJSON; per-phase ACK/failure/unknown counts; duplicate records cannot inflate unique completion; foreign/out-of-plan and missing/null identity fields rejected; raw logs untouched; truncation surfaced; zero-response setup abort records exit 108 without invented sends; existing progress log preserved; ACK/delivery caveats; raw secret-bearing diagnostics excluded. |
| `internal/runner/cpu_boundaries_test.go` | Warm-up/measurement snapshots selected using load-host request/response brackets, not SUT epoch subtraction; known CPU deltas/attempt denominators and boundary-skew intervals. Arithmetic, not exact live phase alignment. |
| `internal/analysis/protocol_cohort_test.go` | Hand-calculated dispatch endpoint and clock labels; dispatch versus scheduled latency; synthetic/live manifest/evidence mismatch invalidates claims. |
| `internal/analysis/analysis_test.go` | Success rows accepted only in complete-ledger mode; compact-ledger semantics and boundary failure categories remain strict. |
| `internal/plan/plan_test.go` | Explicit supported dispatch policy permits diagnostic-only lag; unsupported policy rejected; positive relative cutoff preserved; dual absolute/relative cutoffs rejected. |
| `internal/protocol/protocol_test.go` | Declared pilot relative drain/quiet/late eligibility; rejects dual/nonpositive cutoffs; no implicit campaign eligibility. |
| `internal/sink/sink_test.go` | Native EdgeX/Orion envelopes retain per-identity device/schedule metadata; malformed timestamp handling and actual sink-record output using local HTTP test requests. |
| `cmd/benchctl/payload_validate_test.go` | The W3 stored-payload gate. `benchctl payload-validate` compares the platform's own stored payload against an expectation rebuilt from the plan seed — never from the store — so a faithful store validates (including the exact 1,024-byte padding) while identity-correct rows with damaged or truncated padding fail. An empty store is a failure, not a vacuous pass. Tests also hold the exit-status contract: 0 all rows validated, 1 any row failed or nothing validated, 2 usage/I/O error. |

### Stored-payload validation (`benchctl payload-validate`)

The sink retains identity and timing only, so a sink row count cannot establish that a
platform preserved the payload. This subcommand is the independent check the W3 gate
requires:

```sh
benchctl payload-validate -plan P -run-id ID \
    -ledger RUN/records.ndjson -stored stored-payload.ndjson [-out FILE]
```

- `-ledger` is the **generator's** complete ledger, and `-stored` is the platform's own
  exported store (`scripts/history-payload.sh`). Taking identity and timing from the
  generator and payload bytes from the platform keeps the comparison non-circular.
- The expectation is rebuilt from the plan's `seed` and the message's `seq` through the
  same contract the generator uses. Padding is never copied out of the store into the
  expectation.
- A run where the store cannot be exported at all yields **unavailable**, not a pass.
  FIWARE's frozen subscriptions are exactly this case; do not present it as validated.

This validator proves *stored payload fidelity*. It does not measure wire bytes —
nothing in this harness captures the network — so it cannot be cited as a transport-byte
result.

Existing Go suites in `tests/offline/` also cover adapter payload golden vectors,
cohort arithmetic and CLI synthetic pipelines. Those fixtures are not live
storage/deployment evidence.

## JavaScript and shell probes

| File | Command / boundary |
|---|---|
| `tests/offline/adapter-payload.test.mjs` | `node --test tests/offline/adapter-payload.test.mjs`; deterministic payload offsets/accounting and response-boundary outcomes, including empty FIWARE padding. No real k6/platform connection. |
| `tests/offline/edgex-preprovision.test.mjs` | `node --test tests/offline/edgex-preprovision.test.mjs`; stubbed k6/HTTP verifies matching preprovisioned cohort skips provisioning and mismatched settings fail. |
| `tests/offline/fixed-point-padding.test.mjs` | `node --test tests/offline/fixed-point-padding.test.mjs`; all three real adapter constructors carry exactly 1,024 extra ASCII logical padding bytes. Checks native-body differences (JSON +1,024, FIWARE +1,022) separately from actual wire bytes; no HTTP/k6/platform invocation. |
| `tests/capture_thingsboard_file_logs_test.sh` | `bash tests/capture_thingsboard_file_logs_test.sh`; fake Docker checks hostname-specific current/rotated log capture and overwrite refusal. Creates its temporary parent (`TMPDIR` or `/tmp/opencode`); no real platforms. |
| `tests/k6-timing-origin-probe.js` | Optional `k6 run tests/k6-timing-origin-probe.js`; elapsed-clock probe with deliberate setup/sleeps, no HTTP requests. Not part of frozen workload; elapsed marks do not prove wall-clock dispatch accuracy. |

## Recorded handoff checks (2026-11-07)

- Full current-code Go suite passed, including local Docker fixture integration.
- Runner/CLI race tests passed.
- Root Python discovery: 85 tests run, one skipped; no failures.
- Offline history Python: eight tests passed.
- Node adapter suites: both files passed.
- Original preregistration byte checks passed.
- Fresh staged-source export, with ignored raw evidence absent: short Go,
  Python and Node suites passed. The shell regression also passed with a
  previously absent configured temporary parent. Build/vet, shell/Python syntax,
  static D28 checks and actual N=2 Compose/resource renders passed locally.

These are workstation checks. Skips remain skips. They do not resolve FIWARE
repetition 2 or qualify a new resource configuration. Re-run against the exact
release commit, inspect output and record failures; never edit historical raw
evidence to make a test or claim pass.
