# M0a baseline preservation and interrupted receiver recovery — 2026-11-03

**Disposition:** evidence recovery complete; independent review required before another milestone. Main campaign remains **NO-GO**.
Scope was limited to preserving the dirty baseline, recovering the existing interrupted receiver evidence, running relevant offline tests, and updating the authoritative status. No platform restart, load campaign, protocol implementation, commit, push, or release occurred.

## Provenance and baseline preservation

- Repository: `/home/shahyar/Downloads/ramonavm2/iot-bench-deploy`, branch `chore/ci-and-release`, HEAD `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51`.
- Local capture time: `2026-11-03T02:08:28.608581+03:30`; host `artix`.
- Requested model route: `openai/gpt-6-luna#high`. Actual harness identity: OpenAI GPT-6 Luna; the harness did not expose confirmation of the separate `#high` route. No claim of routing verification is made.
- Tracked diff was copied before work to `results/m0a-20261103-020828+0330/tracked-before.patch` (SHA-256 `f0da0d534b343c0db6e8f1d8389becdc828b69de136f78935338f5464e2680c2`). All 16 pre-existing untracked source, test, policy and documentation files were byte-copied under `untracked-before/` and individually hashed in `snapshot-manifest.json`. No secrets/config credential files were copied.
- The pre-existing nine tracked modifications, untracked analysis tree, tests, and profile remain untouched. This session's intentional edits are only the appended `docs/status.md` section and this packet; new recovered evidence lives under the new dated `results/` directory.

## Host observation and bounded receiver recovery

At SUT host-local UTC `2026-11-02T22:38:39`, `ramona-vm-2` was reachable over the verified `ramonavm2` SSH alias; load host `ramona-vm-q` was reachable by the existing key at the same time. SUT reported 16,375,816 kB MemTotal and uptime 19 days, 19:01. Only the four already-running exporters were present as containers. The old receiver listeners were on 18083/18084, with PID files 3830274/3830275 and exact expected argv. Details and original-running list: `sut-pre-stop.txt` and `original-running.txt`.

Before signaling anything, both ledgers were copied into `receiver-preflush/`:

| Ledger | Rows | SHA-256 |
|---|---:|---|
| immediate | 49,500 | `2481c8557c289ba1c44e04b5e6d706c35706e8bdab6c350fc03b870a2854866c` |
| delayed | 45,222 | `4f31e456a47e165bd103d1919a9a779a04ec3b3b1aa5ef0d8c76ad6fff31466a` |

An initial process-identity guard failed closed because this session's `/proc/cmdline` parser had an incorrect NUL delimiter; it sent **no signal**. The corrected guard compared each PID file against the full expected argv, then sent SIGTERM only to those two PIDs and confirmed both exited within 20 seconds. No force-kill was used. The resulting final ledgers have the same row counts and SHA-256 values as the pre-flush copies. Final logs were also recovered (`immediate.log` SHA-256 `0525798decfaf017f57677249ceecfd4978796c459298afc960ff1bc380c021d`; `delay.log` SHA-256 `08fe7c4049fd911371c32e98c639836c8e5c32695e680b6e26929c3b1d0fcb92`). `original-running.txt` was retrieved and hashed (`dbc6224be0092cf2b946e40bcac6ee96d7269300f9ccd8bfccaebf7d87e0cb7d`). No containers were restored.

## Reconciliation

Command:

```bash
python3 scripts/verify-adapter-qualification.py \
  --runs results/qualification-next-20261031/adapter1000 \
  --receiver results/m0a-20261103-020828+0330/immediate-final.ndjson \
  --receiver results/m0a-20261103-020828+0330/delay-final.ndjson
```

Machine output: `adapter-reconciliation.json` (SHA-256 `83ff9b6c486f4946beea547f5227d52bfd0ede58101736212af8eb9eb3557aae`).

- `P3-thingsboard-immediate-20261031`: **generator VALID**, 49,500 received and unique across 1,000 devices (1,500 warm-up plus 48,000 measured), zero missing, unexpected, duplicate-extra, wrong-device, or dropped identities. This closes receiver-ledger reconciliation only; it does not qualify the actual ThingsBoard platform or campaign rate.
- `P3-thingsboard-delay700-20261031`: remains **INVALID**. It has 45,222 unique received rows over 1,000 devices, 3,278 dropped iterations, and no unexpected, duplicate-extra, or wrong-device rows. Missing is null because dropped iterations break exact planned-sequence inference. Some client unknown outcomes may have been recorded by the receiver; this evidence does not turn the run into a VALID run or infer loss.
- The four FIWARE/EdgeX setup-aborted cases remain INVALID with zero receiver rows. The verifier emits `missing=49,500` for those because it compares the full nominal plan range despite zero sends; that field is a plan-range artifact, **not** measured loss. Do not report it as observed data loss.

## Offline tests and review boundary

All commands passed:

| Command | Result |
|---|---|
| `go test ./internal/... ./cmd/...` | pass |
| `python3 tests/adapter_receiver_test.py` | pass (1 test) |
| `python3 tests/resume_safe_provision_test.py` | pass (1 test) |
| `python3 -m unittest discover -s tests/offline -v` | pass (4 tests) |
| `git diff --check` | pass |

Scope not qualified by this M0a recovery: owner scope N1/N2/N3, P/D/rate, scheduled-cohort p99, and manual single application-replica fault. Profile/statistical choices, population and rate qualification, SLO reference evidence, clock/capture completeness, platform outcomes, and the fault procedure remain open. No protocol decision is made here. The delayed run remains invalid and all original run bundles remain unchanged.

**Next action:** independent review of the evidence and baseline packet. Stop here until review; then define a separately authorized next milestone. No load campaign or platform restart is authorized by this packet.

The evidence directory's `SHA256SUMS` covers its captured files (excluding the checksum file itself); `snapshot-manifest.json` records the pre-action dirty tree and model-routing note.
