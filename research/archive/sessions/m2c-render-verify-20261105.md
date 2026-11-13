# M2c render and verification packet — 2026-11-05

**Ready for independent review. Offline test and evidence only. No Docker client call, SUT access,
deployment, lifecycle action, observer implementation, load, campaign or live validation is claimed.
No production interface was added or changed.**

Requested implementation route: `opencode-go/deepseek-v4.1-flash#high`. The harness identifies the
exposed provider model as `opencode-go/deepseek-v4.1-flash` (DeepSeek V4.1 Flash) at the requested
`#high` route intent. A separate effort control was not independently exposed by the harness.
Baseline HEAD: `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` on `chore/ci-and-release`. No commit or
push was made. `docs/status.md` was not edited by this cycle; the orchestrator owns it.

Evidence directory: `results/m2c-render-verify-20261105-085543+0330/`.
Approved surfaces exercised: `benchctl deployment-validate`, `internal/deployment` descriptor /
inventory validation, `deployment-orchestration-plan-v2`, `runtime-artifact-manifest-v1` and
`ArtifactManifest.Verify`.
Preserved prior packet: `docs/archive/sessions/m2b-fix-20261105.md`, SHA-256
`07b2122d750e4736fe440e7e61929c0b982a95687da47da7eb0189074cce97d8`; prior M2b approval unchanged.

## 1. Scope

M2c is an offline end-to-end validation of the already approved M2/M2b surfaces. It adds **no** new
CLI, schema field, service, stack, network, receiver, ledger, lifecycle path or observer. The only
new source files are one test file and synthetic fixtures. The existing command and package APIs are
used as shipped.

The implementation does not deploy, provision, apply, up/down, scale, restart, stop, reset, delete,
exec, read logs, inject faults or generate load. It does not contact a SUT, Docker daemon, platform,
monitoring service, receiver or network endpoint.

## 2. Caller-supplied synthetic fixtures

All fixtures live under `internal/deployment/testdata/m2c/` and label themselves synthetic:

| Fixture | Purpose | Explicit synthetic marker |
|---|---|---|
| `synthetic-descriptor-thingsboard.json` | Descriptor for the eight current ThingsBoard **persistent** default-start services (`tb-core`, `tb-rule-engine`, `tb-http-transport`, `zookeeper`, `kafka`, `postgres`, `valkey`, `haproxy`), with the real default images, replica shape (`tb-http-transport` x2) and the unchanged protocol / D44 declarations. | `provenance.source` starts `SYNTHETIC M2c caller-supplied fixture` |
| `synthetic-inventory-thingsboard.json` | Matching caller-supplied observed inventory. No Docker, SUT or observer was read. | `source` starts `SYNTHETIC M2c caller-supplied inventory` |
| `synthetic-inventory-thingsboard-mismatch.json` | Controlled mismatch: `tb-core` reports a wrong image, `unhealthy` health and `exited` state, with explicit reasons. | same synthetic source label plus `SYNTHETIC M2c controlled mismatch` reasons |
| `synthetic-descriptor-invalid-case-alias.json` | Controlled invalid input: `platform` replaced by `PLATFORM`. | derived from the synthetic descriptor |
| `synthetic-orchestration-plan-v2.json` | Existing v2 plan shape covering all current services and the optional / one-shot services. Every stage purpose and every platform health-check list begins with an explicit `SYNTHETIC M2c` label. | stage purposes and health checks |
| `synthetic-artifact-manifest.json` | Synthetic `runtime-artifact-manifest-v1` with run tag `m2c-synthetic` and run ID `m2c-synthetic-20261105-fixture`; declares synthetic descriptor, inventory, plan and preserved-prior placeholder. | run tag / run ID `m2c-synthetic` |
| `synthetic-prior-artifact-manifest.json` | Synthetic prior manifest that deliberately declares a non-matching hash and byte count for the same prior path. | run tag `m2c-synthetic-prior` |
| `synthetic-prior-note.txt` | Synthetic preserved-prior placeholder bytes. | body starts `SYNTHETIC M2c preserved-prior placeholder` |

No real prior-run artifact, ledger, packet, SUT observation or external data is referenced. The
protocol binding `7176b1fa…d24ee` and D44 profile `1b6aa76d…d1f8` are recorded **from the synthetic
descriptor as declarations**; the package did not re-read, mutate or re-verify those files.

## 3. Existing CLI validation

The built binary was used unchanged:

```sh
benchctl deployment-validate -descriptor synthetic-descriptor-thingsboard.json \
  -inventory synthetic-inventory-thingsboard.json -out NEWDIR
```

| Run | Output | Exit | Result |
|---|---|---:|---|
| `cli-smoke-valid` | new `cli-smoke-valid/` | 0 | `attestation=PASS`, `live_validation=NOT_ASSESSED`, `dry_run=true`, observer `NOT_ASSESSED`, all 8 services `PASS`, application replicas 4/4, supporting 5/5 |
| `cli-smoke-mismatch` | new `cli-smoke-mismatch/` | 1 | controlled mismatch writes an inspectable report with `attestation=FAIL`, `live_validation=NOT_ASSESSED`, `tb-core` non-`PASS` |
| invalid `PLATFORM` fixture | no output directory created | 2 | `descriptor: $: unknown or non-case-exact key "PLATFORM"` |

The valid report and Markdown render both state `Live validation: NOT_ASSESSED`; the report repeats
that no deployment, Docker call, SUT access or live observer exists. Each output directory was new
and outside the fixture inputs. Logs and `.exit` files are in the evidence directory.

## 4. Canonicalization and deterministic render

The existing package APIs were used to decode, canonicalize and render the plan and manifest. The
recorded canonical files under `canonical/` are:

| Output | SHA-256 |
|---|---|
| `descriptor.canonical.json` | `d0237c7bef98b714fc86380ce4f9f34af4ccaf54df0a89820c37c5a932b71b36` |
| `inventory.canonical.json` | `86d836e542008537ba43f1e98c063e1da67a30def6f7f68ee4fb24ca2af11afd` |
| `orchestration-plan.canonical.json` | `7835a5ec81266782747422ef9e37f046e5cb8fa6b70b7cc0afb3a2ec427d4c78` |
| `orchestration-plan.md` | `b6b544759fb0710f2a617d5e27b01278dd1a1d4877166024d8ce1fd37f26e437` |
| `artifact-manifest.canonical.json` | `6bf0eaf7318db905ce2572c341216b0b0fc5a92ffc68005a4ca7b419ad3cfabe` |
| `artifact-manifest.md` | `0bcaed9b97b25f38bb4ed468689d20ac051b7b300dda4d8cbc70c7135d0d2f5d` |

Raw synthetic input hashes:

| Input | SHA-256 |
|---|---|
| `synthetic-descriptor-thingsboard.json` | `91e0dd8ebd79cf980a6d86a6a03459881805de27b5913b12a2838c8f88438681` |
| `synthetic-inventory-thingsboard.json` | `f751636685bfb2563d4b6b95c44f6c1e7bdd2bb6663fa91bdf599f36ecb9cdae` |
| `synthetic-orchestration-plan-v2.json` | `9a5ff516b0274afb21f32fd238eb9cf779c8beb6b5b6323ec78041bf3cceec8e` |
| `synthetic-artifact-manifest.json` | `57629f47d8b29fa54b9eef09e23204f22c0f7f73504bfcd6f6306999a845ec9a` |

Each canonical hash equals the SHA-256 of the recorded canonical bytes. Repeated decoding and
rendering produce byte-identical canonical JSON and Markdown. The canonical plan and manifest were
round-tripped back through `DecodeOrchestrationPlan` / `DecodeArtifactManifest`; the canonical
descriptor round-tripped through `DecodeDescriptor`. The rendered plan states the offline mode,
`EXECUTION_BLOCKED_HERE`, the lifecycle exclusion and the synthetic label. The rendered manifest
states that no directory or artifact is created and that prior-manifest binding is
`NOT_IMPLEMENTED`.

## 5. Synthetic packaging verification matrix

`ArtifactManifest.Verify` was run against new caller-supplied synthetic scratch trees under
`packaging-scratch/`. The manifest declares four synthetic artifacts: descriptor, inventory,
orchestration plan and a preserved-prior placeholder.

| Case | Expected | Observed |
|---|---|---|
| matching scratch tree | PASS | 4/4 artifacts `PASS`; SHA-256 and byte count match |
| tampered declared hash | FAIL | `descriptor` `FAIL`; `artifact SHA-256 differs from the manifest` |
| tampered declared byte count | FAIL | `inventory` `FAIL`; `artifact byte count differs from the manifest` |
| missing declared file | UNAVAILABLE | `orchestration_plan` `UNAVAILABLE`; file-not-found reason |
| synthetic prior manifest present | prior binding ignored | current manifest still `PASS`; prior-manifest binding remains `NOT_IMPLEMENTED` / `NOT_ASSESSED` |

`PriorManifestVerificationNotImplemented` is asserted to equal `NOT_IMPLEMENTED`. The synthetic prior
manifest deliberately declares a different hash and byte count for the same path; `Verify` ignores
it and never reads a prior run's manifest. Full results are in `packaging-verification.txt` and the
verbose `command-09-targeted-m2c.log` / `command-packaging.log`.

## 6. Files changed

All nine files are additions. No production, schema, CLI, Compose, `.env`, Makefile, monitoring,
receiver, ledger, workload or documentation file was modified.

| File | Pre-edit | Final SHA-256 |
|---|---|---|
| `internal/deployment/m2c_roundtrip_test.go` | absent | `06e2dc06e96ee939baf71dd1a6076bdc6cb966dd7e6ab4e95eba2cd8549d6d94` |
| `internal/deployment/testdata/m2c/synthetic-descriptor-thingsboard.json` | absent | `91e0dd8ebd79cf980a6d86a6a03459881805de27b5913b12a2838c8f88438681` |
| `internal/deployment/testdata/m2c/synthetic-inventory-thingsboard.json` | absent | `f751636685bfb2563d4b6b95c44f6c1e7bdd2bb6663fa91bdf599f36ecb9cdae` |
| `internal/deployment/testdata/m2c/synthetic-inventory-thingsboard-mismatch.json` | absent | `89192d6a1eba6963033e141c525ae6fc06c1e383912b036f7c6365d6aeb3df55` |
| `internal/deployment/testdata/m2c/synthetic-orchestration-plan-v2.json` | absent | `9a5ff516b0274afb21f32fd238eb9cf779c8beb6b5b6323ec78041bf3cceec8e` |
| `internal/deployment/testdata/m2c/synthetic-artifact-manifest.json` | absent | `57629f47d8b29fa54b9eef09e23204f22c0f7f73504bfcd6f6306999a845ec9a` |
| `internal/deployment/testdata/m2c/synthetic-prior-artifact-manifest.json` | absent | `381483411bd80221fa8843b1a68d083a59178af5f1a638dbfb853f58aeb7b294` |
| `internal/deployment/testdata/m2c/synthetic-prior-note.txt` | absent | `ff2c98c032aded08c3a4bd678f76b5a92926c3fb169b798a1cb211f964fceb09` |
| `internal/deployment/testdata/m2c/synthetic-descriptor-invalid-case-alias.json` | absent | `1e7b4f1088cc95abaac03bd0128e6f6e3ebcb8af667771d9860ddbc8f84359c1` |

`incremental.patch` contains exactly these nine files and applies to scratch trees with
`patch -p1 --fuzz=0`. It excludes this packet and the evidence directory so the packet can record
the patch hash without a circular reference.

## 7. Verification commands

All recorded commands exited `0` except the two deliberate CLI outcomes noted above. Logs and
`.exit` files are in the evidence directory.

| # | Command | Exit |
|--:|---|---:|
| 01 | `go test -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| 02 | `go test -short -count=1 ./cmd/... ./internal/... ./tests/...` | 0 |
| 03 | `go test -race -count=1 ./internal/deployment ./cmd/benchctl` | 0 |
| 04 | `go vet ./internal/deployment ./cmd/benchctl` | 0 |
| 05 | `go mod verify` | 0 |
| 06 | `git diff --check` | 0 |
| 07 | `gofmt -l internal/deployment cmd/benchctl/deployment.go cmd/benchctl/deployment_cli_test.go cmd/benchctl/main.go` (empty) | 0 |
| 08 | `go build -o /tmp/opencode/m2c-build/benchctl ./cmd/benchctl` | 0 |
| 09 | `go test -count=1 -v -run 'TestM2C' ./internal/deployment` | 0 |
| 10 | targeted existing packaging/orchestration tests | 0 |
| 11 | targeted existing CLI strict-key test | 0 |
| 12 | canonical output generation (`TestM2CWriteCanonicalOutputs`) | 0 |
| 13 | packaging evidence generation (`TestM2CWritePackagingEvidence`) | 0 |
| 14 | valid synthetic CLI smoke | 0 |
| 15 | controlled mismatch CLI smoke | 1 (expected `FAIL`) |
| 16 | invalid synthetic CLI smoke | 2 (expected input rejection; no output dir) |
| 17 | patch round-trip `patch -p1 --fuzz=0` | 0 (9/9 files match) |
| 18 | preserved M1f/M2/M2b evidence re-hash | 0 (103 artifacts, 0 problems) |

This is a scoped result, not an unrestricted full-suite claim. Docker integration is skipped by
`-short`; no claim is made that unrestricted `go test ./...` passes over retained historical
snapshots under `results/`. The new tests are offline and use only `t.TempDir` or explicitly named
new evidence directories.

## 8. Integrity and preservation

| Item | Value |
|---|---|
| HEAD / branch | `36f8348e5f6a8cf286cbe93b7e2a8358bcb20d51` / `chore/ci-and-release` |
| Source snapshot before edits | 386 files, `e94b6140f96d43ebf986b61625cfe25f6fe75c5e6555554e525890e4d2f19205` |
| Source snapshot after edits | recorded in `source-final.sha256` and `summary.txt` (see `summary.txt` for count/hash) |
| Retained `results/` excluding `results/m1f-*`, `results/m2-*`, `results/m2b-*`, `results/m2c-*` | 2,136 files, `8e25006644fabd6605f5e900c0af9b4fdbe8b803f820be0f811d71d439b325ec`, identical before and after |
| Protocol definition SHA-256 | `7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee` |
| D44 profile SHA-256 | `1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8` |
| `docs/status.md` before / after | `136abe1dbb99cc82c5306448b517bec2d06e0239dd8c0097a7942dcc8120cd57` / unchanged |
| Prior M2b fix packet before / after | `07b2122d750e4736fe440e7e61929c0b982a95687da47da7eb0189074cce97d8` / unchanged |
| Prior M2b approval before / after | `d9e76bdf3603a971773652fe238548754a7e526e0a853849c6089c09c1cebbb5` / unchanged |
| Prior M1f/M2/M2b evidence manifests | 103 declared artifact references re-hashed, **0 problems** |
| `incremental.patch` SHA-256 | `3f743afd2addad23142603eebdcb0ed224da61a94983a832fd42aca2c9cadfb4` (51,496 bytes) |
| Patch round-trip | exit `0`, **9/9 files** match final SHA-256 |

The pre-edit hashes manifest is empty because every changed file is an addition;
`pre-edit-absence.txt` records the absent paths. This packet and the evidence directory are excluded
from the patch. Prior M1f/M2/M2b packets, evidence, ledgers and retained results were not edited.

## 9. Hidden-action and lifecycle exclusion

A scan of the nine new files found no `os/exec`, `exec.Command`, `net/http`, `net.Dial`, HTTP call,
`syscall` or equivalent external-call path. The only Docker/SUT/lifecycle text is explicit exclusion
wording or the approved blocked-action taxonomy in the synthetic plan. No production command file
was changed by this cycle. The only writes are to new scratch/evidence directories, and tests write
to `t.TempDir` by default. See `no-hidden-lifecycle-scan.log`.

## 10. Limits and non-goals

- No live observer, Docker client, SUT/platform/sink/adapter access, deployment, provision,
  apply/up/down, scale, restart, stop, fault, load, reset, delete, exec, log read or campaign.
- `PASS` remains an attestation over caller-supplied documents; `live_validation` remains
  `NOT_ASSESSED`. The synthetic fixtures are not live deployment evidence.
- No protocol/profile hash change, stack topology change, service addition, schema field, CLI
  command or output-format change.
- No prior-manifest binding is implemented or claimed; it remains `NOT_IMPLEMENTED`/`NOT_ASSESSED`.
- No commit, push or `docs/status.md` edit. Docker integration is skipped by `-short`.

**Stop for independent review.** M2c is not approved by this packet. Any live deployment,
observation or campaign requires separate review and explicit authorization.
