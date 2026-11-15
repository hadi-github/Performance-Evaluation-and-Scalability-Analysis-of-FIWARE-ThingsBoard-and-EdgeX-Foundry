# Owner amendment: EdgeX discovery cap 16,384 → 50,000 and a deduplicated profile fetch

**Date:** 2026-11-15
**Status:** IMPLEMENTED, opt-in, **not yet executed**. No run ID was consumed.
**Authority:** owner instruction, verbatim: *"yes do this, make edgex runable and
measurable"* — approving the previously proposed fix (raise the cap **and** fix the
O(devices) profile fetch) as a separately recorded amendment rather than an edit to
already-frozen bytes.

---

## 1. What this amendment is for

W4 needs five EdgeX repetitions. Four of them are blocked, and the two blockers are
different in kind:

| Blocker | Effect | Where it binds |
|---|---|---|
| `MaxResultCount: 16384` | admits only 1 of 5 EdgeX reps (needs ≥ 41,192) | pre-run inventory gate |
| O(devices) profile fetch | readiness exceeds its deadline at W-scale inventory | readiness gate, **before** provisioning |

The second one binds **first**. That ordering is why the cap bump alone unblocks
nothing, and why this amendment changes both. This was the finding in
`edgex-ordinal-15-readiness-failure.md`; the arithmetic for the first is in
`w4-edgex-inventory-blocks-four-repetitions-20261114.md`.

## 2. What changed — and what deliberately did not

**Added (new files; nothing was rewritten):**

| File | Purpose | sha256 |
|---|---|---|
| `deploy/edgex/compose.feasible-discovery-amended.yaml` | cap → 50,000 + amended image + `build: !reset null` | `28021d16…` |
| `deploy/edgex/amendment/device-sdk-dedup-profile-fetch.patch` | the dedup patch (12 lines) | `7b8f4f6c…` |
| `deploy/edgex/amendment/Dockerfile.device-rest` | builds device-rest from the patched tree | `489c4f74…` |
| `deploy/edgex/amendment/sdk_dedup_profile_fetch_test.go` | behavioural build gate | `b48dc36d…` |
| `deploy/edgex/scripts/build-patched-device-rest.sh` | staging + build + verify | `328ff910…` |
| `tests/edgex_discovery_amendment_test.py` | 26 tests pinning the invariants | — |

**Modified (opt-in gated; flag-off behaviour is byte-identical):**

| File | Change | sha256 |
|---|---|---|
| `results/feasible-campaign-20261106/run-feasible-campaign.py` | single override resolver, cap pinned both directions, live render applies the override | `86d86838…` |
| `deploy/edgex/scripts/provision-then-scale.sh` | gate records retained/reserve/cap that admitted the run | `83751c6f…` |
| `scripts/run-w4-campaign.py` | skip list consulted only when the flag is off | `ddf70627…` |

**Left byte-identical (required):**

- `deploy/edgex/compose.feasible-discovery.yaml` — sha256
  `0893a6b861c800a264ae4612e4d3e45dc13deb2082ac42640125af8993b0aa1a`, unchanged.
  B0 review bindings verify against these exact bytes; a test asserts the hash.
- `scope.json`, `run-matrix.json`, `qualification-gates.json`, and all 60 campaign
  plans. Read-only, never written by any of the above.
- The ingested→sink path. Neither change touches it: the cap clamps list-query
  `limit` only, and the dedup patch runs once at cache init.

## 3. Why 50,000

```
retained 35,192 + reserve 6,000 = 41,192 required
16,384                          = short by 24,808
50,000                          = clears it with 8,808 headroom
```

50,000 is EdgeX's own documented example value (`edgex-go` v4.0.2
`openapi/core-metadata.yaml`), not a number tuned to a measurement. Stock EdgeX
ships 1,024. The value never enters the ingest→sink path, so it cannot make any
run faster or slower — it is a query clamp, not a performance parameter.

## 4. The dedup patch, and why it needed a build rather than a config

`device-sdk-go` v4.0.2 `internal/cache/init.go` fetches the profile **once per
device**, sequentially, before `driver.Start` registers the ingest route:

```go
for _, d := range devices {
    res, err := dpc.DeviceProfileByName(context.Background(), d.ProfileName)
    ...
    profiles = append(profiles, dtos.ToDeviceProfileModel(res.Profile))
}
```

The benchmark provisions **one** shared profile (`bench-gen-v1`) for the whole
cohort, and the resulting `profileCache` is keyed by profile name — so duplicate
appends always collapsed to a single entry anyway. The fetches were pure waste.
At the measured ~46 ms each this is ~468 s at 10,192 devices against a bounded
readiness deadline.

The patch adds a `seenProfile` set, making it O(profiles). It is the projection of
the existing cache keys, so **the cache contents are identical**; only the number
of round-trips changes.

**Upstream `main` still has the loop**, so no released version fixes this. It is not
reachable from configuration, a compose override, or a device profile setting.

### The trap that would have made this silently useless

Upstream's `Makefile` builds with `-mod=readonly`, which **ignores `vendor/`** and
resolves modules from the module cache. A patch written into `vendor/` under
`-mod=readonly` compiles, runs, and still fetches once per device — an invisible
no-op that looks exactly like success. Verified on this workstation: with
`GOPROXY=off`, `-mod=readonly` cannot even resolve `klauspost/compress`, while
`-mod=vendor` builds the patched tree offline.

The amendment therefore builds with `-mod=vendor` explicitly **and** gates the
build on a behavioural test, because an exit code cannot distinguish the two cases.

## 5. The amend path, verified in both directions

This is the part that matters most, and it was tested rather than assumed.

**The behavioural gate is real.** `sdk_dedup_profile_fetch_test.go` drives the
actual `InitCache` with 50 devices sharing one profile and counts
`DeviceProfileByName` calls:

| Tree | Result |
|---|---|
| patched (vendored SDK patched) | `InitCache` issues **1** fetch — pass |
| stock v4.0.2 (`git clone --branch v4.0.2`) | `50 devices sharing one profile caused 50 fetches; want 1` — **fail** |

A full `docker build` of the unpatched tree **exits 1** at that step. So the image
cannot be built successfully without the dedup in effect.

**The image builds.** `edgex-amended/device-rest:4.0.2-p1` builds from
`codeload.github.com` v4.0.2 + the patch, with the same flags as upstream
(`include_nats_messaging`, `-s -w -bindnow`, `-trimpath`, `-buildmode=pie`,
`CGO_ENABLED=0`).

**Compose renders correctly.** With the amended override,
`docker compose config` reports `image: edgex-amended/device-rest:4.0.2-p1`,
`build: ABSENT`, `cpuset: 4`, `mem_limit: 536870912`,
`MAXRESULTCOUNT: 50000`. With the frozen override it still reports
`edgex-nats/device-rest:4.0.2`, `build: PRESENT`, `MAXRESULTCOUNT: 16384`.

**The driver split is correct.** `run-w4-campaign.py --plan-only`:

| Flag | Runnable | Skipped |
|---|---:|---:|
| off (default) | 11 | 4 (`ex-r02..r05`) |
| `IOTBENCH_EDGEX_DISCOVERY_AMENDMENT=1` | 15 | 0 |

## 6. How to select it

```sh
# 1. build the amended image on the SUT (needs github.com; GO_BUILDER_BASE for a mirror)
GO_BUILDER_BASE=<your-golang-image> ./deploy/edgex/scripts/build-patched-device-rest.sh

# 2. select the amendment for a run
export IOTBENCH_EDGEX_DISCOVERY_AMENDMENT=1
```

Anything other than the exact string `1` leaves it off — a typo must not silently
depart from the frozen configuration, and a test asserts that.

## 7. Measured readiness, and what is still unmeasured

Readiness was measured at **10,192 devices: 224 s** (`edgex-readiness-measured-20261111.md`).
Projecting linearly (an **upper bound** — the same projection over-estimated by 2× at
10,192):

| Devices | Projected | Against the launcher's 600 s budget |
|---:|---:|---|
| 16,192 | 356 s | fits |
| 41,192 | 905 s | **does not fit** |

So the cap raise alone would have produced readiness timeouts at W-scale inventory,
which is precisely why both changes ship together. With dedup the cost is
O(profiles) — one fetch — so the projection no longer applies.

**What has NOT been measured:** readiness at 41,192 devices on real hardware. The
O(profiles) reasoning is source-level and the behaviour is unit-tested, but the
end-to-end timing at W-scale inventory is untested. It cannot be measured without a
SUT (see §9).

## 8. This departs from the frozen configuration — re-qualification is required

The amendment does not merely fix a bug; it changes a value the campaign froze:

- The inventory gate stops binding at W scale. A run admitted under the amendment
  can no longer be blocked by retained inventory, which is the entire point — but
  it means EdgeX cells run under the amendment are **not** directly comparable to
  B0/W1-W3 cells run under 16,384.
- `device-rest` runs a locally built image rather than the pinned upstream tag. The
  image differs only in the dedup loop, and the build is gated on a test proving it,
  but it is still a different binary.

**Consequences, stated plainly:**

1. EdgeX W4 cells run under the amendment need a **new qualification receipt**
   naming the amendment and both file hashes. The existing
   `owner-w4-runnable-20261115b` authorizes 11 IDs under the frozen configuration
   and is **not** amended by this document.
2. Results from amended cells must be reported with the amendment named, never
   pooled silently with frozen-configuration cells.
3. `docs/status.md` still describes the measured-qualification table as it stands;
   it must be updated when amended cells actually run, not now.

## 9. Why nothing has executed

**There is no SUT.** `87.107.191.107` is down. No 8 vCPU / 16 GB host exists, and
the replacement must match the pre-registered testbed exactly (the 7-core / 14 GB
option was rejected). The load host (`2.188.205.118`) is reachable and fully
verified, but a generator with no target measures nothing.

So this amendment is complete as **code, proof, and record**, and unconsumed as
**evidence**. No run ID was used. `run-w4-campaign.py check` still reports
`rows=60 problems=0`, and no matrix row moved.

## 10. Verification performed

```
python3 -m unittest discover -s tests -p '*_test.py'
    Ran 249 tests ... OK (skipped=2)          # 223 before, +26 from this amendment

python3 scripts/run-workload-campaign.py check
    rows=60 problems=0 authorized=False       # unchanged

sha256sum deploy/edgex/compose.feasible-discovery.yaml
    0893a6b861c800a264ae4612e4d3e45dc13deb2082ac42640125af8993b0aa1a  # unchanged

# behavioural gate, both directions (see §5)
patched tree   -> 1 fetch   -> PASS
stock v4.0.2   -> 50 fetches -> FAIL, docker build exits 1
```

## Related

- `research/findings/edgex-ordinal-15-readiness-failure.md` — the O(devices) defect
- `research/findings/edgex-discovery-limit-and-inventory-scaling.md` — the cap is a repo override
- `research/findings/w4-edgex-inventory-blocks-four-repetitions-20261114.md` — the blocked-cell arithmetic
- `research/findings/edgex-readiness-measured-20261111.md` — the 224 s measurement
- `research/findings/generator-pool-amendment-20261110.md` — the precedent for recording an amendment
