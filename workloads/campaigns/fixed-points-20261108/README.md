# W1–W4 fixed-point batch preparation

**Owner-approved configurations; five attempts per workload/platform proposed;
variants not launch-cleared.** This batch does not modify or replace B0. Start
with the [workload methodology](../../../docs/methodology.md#8-workload-configurations-b0-and-the-separate-w1w4-batch)
for the questions, settings, endpoint and limitations.

## Controls and inputs

| File | Meaning |
|---|---|
| `scope.json` | B0 source hashes, the four one-factor configurations and shared controls; `execution_authorized=false`. |
| `run-matrix.json` | All 60 proposed experimental entries, workload/platform/attempt IDs, matching B0 seeds, expected counts, intended deployment and plan hashes. |
| `plans/*.json` | Strict generator-schema inputs: no unsupported replica/CPU fields hidden in a generator plan. Deployment requirements are explicit matrix sidecars. |
| `offline-resolution.json` | Actual `benchctl resolve` outputs for all 60 plans, with the binary digest; **not observed runtime/deployment evidence**. |
| `qualification-gates.json` | Common and workload-specific prerequisites, including retained EdgeX inventory, logical versus wire bytes and N=1/k=2. |

Five proposed attempts × four workloads × three platforms = **60 attempts**.
Each W1 attempt plans 45,000 messages; each other attempt 22,500. The batch plans
1,687,500 messages before any separate diagnostics/qualification. Interleaving
rotates W1→W2→W3→W4 across repetitions and rotates all three platforms within
each workload group. The matrix, not an operator's convenient platform block,
defines the order. Failed attempts remain attempts; no automatic replacements.

New IDs use `w1dp-`, `w2dp-`, `w3dp-` or `w4dp-`, with the same string width as
B0's `disp-` prefix. Existing B0 IDs and artifacts are untouched. Archive/result
group keys must be `(workload_id, platform)` with run IDs below those groups.

## Blocked cases before any variant launch

| Case | Offline support | Still required |
|---|---|---|
| All 12 workload/platform cells | Plans resolve; one-factor fields, message counts, shared controls and unique IDs tested. | Confirm proposed repetition count; implement/review a separate workload-aware launcher and result grouping. Do not feed these plans to the frozen B0 supervisor or append to its matrix. |
| W1, all platforms | 50/s plans resolve to 15,000 warm-up + 30,000 measurement messages. The evaluator now explicitly accepts 100/s with unchanged identity/timing/loss/bin/gap/lag checks. | Full 300/600-second 50/s generator/capture validation and applicable 2× headroom; qualify the 100/s diagnostic path with matching HTTP/recording conditions. Source support is not capability evidence. |
| W2, all platforms | 2,000-device plans resolve at 25/s. | Live provisioning/resource checks and all intended devices recognized by every active replica; matching headroom evidence. |
| EdgeX, new batch | Inventory projection includes 25,000 new experimental-cohort devices. | Live retained count, numeric metadata/driver limits, memory and whole-batch reserve. Current 16,384 is below projected 36,192 after B0 plus experiments, **before** new qualification cohorts/margin. Preserve everything; do not reset history or raise a setting only for W2. |
| W3, all platforms | Encoder test proves exactly 1,024 extra logical padding bytes through each adapter. | Live delivered/stored padding validation, larger-payload capture/headroom, and separately scoped actual transport/wire measurement. FIWARE sink/history subscriptions currently omit padding and sink records do not retain it; a reviewed payload-preserving evidence path is needed with common-batch/B0 comparability disclosed. Computed header lengths are not measured wire bytes. |
| W4, all platforms | Sidecars explicitly require N=1/k=2, pool 4–5 and unchanged HAProxy path. | Effective container/backend counts, kernel CPU/resource checks, stable per-replica images/limits and matching N=1/k=2 path/headroom qualification. B0's N=2 gates do not clear this. |

The EdgeX counts are **projections**, not a fresh live inventory. Historical
qualification/device cohorts contribute to the retained count. Before the new
batch, freeze a common discovery configuration with the actual full inventory,
all planned qualification cohorts and margin. Any operational change relative
to B0 needs comparability review and disclosure. No new limit, image or resource
change is applied by this preparation.

## Offline reproduction (no servers, Docker or traffic)

Host/working directory: a workstation, repository root. Inputs: this checkout's
frozen B0 controls, Python 3, Node.js 22+ for adapter tests, and the Go toolchain
declared in `go.mod` if building a resolver. Do not reuse a historical experimental
ID to generate traffic.

```sh
# Strictly read-only controls check; no binary or output writes required.
python3 scripts/stage-fixed-point-variants.py --check

python3 -B -m unittest tests.fixed_point_variants_test -v
node --test tests/offline/fixed-point-padding.test.mjs
```

For actual offline resolution with your own build, create a local temporary
parent first and generate into a **new** output directory. A different binary
digest can make retained validation differ; never overwrite its provenance to
hide that difference:

```sh
mkdir -p /tmp/opencode
go build -o /tmp/opencode/benchctl-variants ./cmd/benchctl
python3 scripts/stage-fixed-point-variants.py --out /path/to/NEW/proposal \
  --benchctl /tmp/opencode/benchctl-variants
```

Expected: 60 strict JSON plans, scope/matrix/gates and offline resolution with
`deployment_verified_live=false`. The generator refuses an existing output
directory; `--check` never creates files. Adding `--benchctl /path/to/benchctl`
to `--check` verifies an already-present resolution with that exact binary and
fails if it is missing or differs. Failures
stop preparation, not B0. These commands test source/configuration semantics;
they do not establish that a platform sustains the workload.

Checks actually performed during preparation: all 60 plans resolved using the
retained source-bound prebuilt benchctl; four Python staging tests and the
Node padding test passed; frozen B0 hashes remained unchanged. No W1–W4
diagnostic, qualification or experimental traffic was launched.
