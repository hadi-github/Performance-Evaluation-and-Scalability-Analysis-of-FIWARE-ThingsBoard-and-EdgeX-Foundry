# Finding: why EdgeX cannot enter the W1–W4 batch — two blockers, one decision

Status as of 2026-11-10: **OPEN, awaiting an owner decision.** Nothing in the W1–W4
batch has been launched. This note records the state of the EdgeX blocker so the
decision can be made on measured numbers rather than on the projection that
prompted the original block.

It supersedes the loose statement in `w1-readiness-after-b0.md` that "all EdgeX W
cells are blocked on the cap". That is wrong on two counts, and both are corrected
below with live measurement.

## What is actually verified live (2026-11-10)

| Quantity | Value | How measured |
|---|---:|---|
| Retained EdgeX devices | **10,192** | `GET /api/v3/device/all?limit=1` → `totalCount` |
| `MaxResultCount` — core-metadata | **16,384** | `GET /api/v3/config` → `Service.MaxResultCount` |
| `MaxResultCount` — device-rest | **16,384** | same, on the driver's own API |
| Profile GET cost | **48 ms** | 100 sequential `GET /deviceprofile/name/bench-gen-v1` = **4,797 ms** |
| device-rest readiness route | **405** (ready) | `GET /api/v3/resource/__bench_readiness__/reading` via ingress |

The 48 ms/GET figure is a fresh live measurement, consistent with the ~46 ms
recorded during the ordinal-15 investigation. It is the load-bearing input to
blocker B.

## Blocker A — inventory headroom, and where it actually binds

The gate (``deploy/edgex/scripts/provision-then-scale.sh:194``) is:

```bash
if (( retained + reserve > metadata_limit || retained + reserve > driver_limit )); then
  exit 4   # before any traffic
fi
```

with `reserve` = `EDGEX_INVENTORY_RESERVE`, which the W launcher sets to **6,000**.
Applying that to the real retained count, accumulating 1,000 new devices per
attempt because inventory is deliberately preserved:

| Cell | Retained | + reserve 6,000 | vs cap 16,384 | Result |
|---|---:|---:|---:|---|
| **W1, rep 1** | 10,192 | 16,192 | **+192 margin** | **PASSES** |
| W1, rep 2 | 11,192 | 17,192 | −808 | blocked |
| W1, rep 3 | 12,192 | 18,192 | −1,808 | blocked |
| W2 (2,000 devices) | 13,192 | 19,192 | −2,808 | blocked |
| W3 | 15,192 | 21,192 | −4,808 | blocked |
| W4 | 16,192 | 22,192 | −5,808 | blocked |

**So the correct statement is not "all EdgeX is blocked."** One EdgeX attempt — W1
repetition 1 — clears the inventory gate by 192 devices. Every subsequent EdgeX
attempt fails it.

That single attempt is not usable as a W1 EdgeX repetition either, because a
workload needs five repetitions to be comparable and the five are not
interchangeable. Running rep 1 and declaring EdgeX partially measured would
produce a 1-of-5 cell whose remaining four attempts are known-inapplicable. That
is a reporting decision, not something to do silently.

## Blocker B — the readiness gate, which binds *first*

The gate order matters and is often misstated. In `provision-then-scale.sh`, the
readiness wait runs at ~line 160; the inventory arithmetic runs at line 194. So
**readiness is upstream of the inventory check**: a cap bump alone would not let
an EdgeX run through.

`wait_driver_ready()` requires the POST-only resource route to answer **405**
rather than 404/503, which happens only after `device-sdk-go` finishes
`InitCache`. In v4.0.2, `internal/cache/init.go:36` fetches the *single shared
profile* once per device, sequentially:

```
InitCache
  └─ for each device  (N iterations)
       └─ GET /api/v3/deviceprofile/name/<profile>      <- same profile, every time
```

At 48 ms/GET (measured above):

| Retained devices | Projected InitCache | vs 180 s default deadline |
|---:|---:|---|
| 10,192 | **≈ 489 s** | **2.7× over** |
| 11,192 | ≈ 537 s | 3.0× over |
| 35,192 | ≈ 1,689 s | 9.4× over |

This is why ordinal 15 failed: zero traffic, `workload_launch_attempted: false`,
and the documented root cause is this O(N) fetch, not the cap.

Note the invalidation caveat, stated so nobody overreads the table: the
projection assumes roughly linear scaling, which is what the code implies but is
only measured at N = 10,192. The *direction and magnitude* are solid; the exact
second counts are projections.

### One honest complication

As of this writing the live container is **ready** (405) and reports
`RestartCount: 0`. That is not evidence the blocker is gone:

- `device-rest`'s log spans every incarnation; it contains **22 `panic:` entries,
  all dated 2026-11-06**, from the B0 era. Those are historical, not current.
- The container's `Created` (2026-11-06) predates its `StartedAt` (2026-11-10
  08:43), i.e. it was recreated/restarted, and `RestartCount` resets on recreate.
- It is ready *now* because it was allowed unbounded time to finish, outside the
  gate's 180 s deadline. Being ready eventually is not the same as being ready
  within the deadline; ordinal 15 already showed the difference.

So the readiness question cannot be settled by observing a settled container. It
must be settled by the gate timing the init path, which is exactly what the
generic `EDGEX_DRIVER_READY_TIMEOUT` does not survive at this inventory size.

## The decision required (not taken)

There are exactly three defensible options. I have not applied any of them,
because the scope forbids raising the limit "for W2 alone" and requires the
comparability treatment to be frozen *before* outcomes are seen.

**Option 1 — raise `MaxResultCount` to cover the whole batch, and fix the init path.**
Both, not either. A cap bump alone fails at blocker B. Full batch needs
≈ 41,192 retained at the end, so the cap must be frozen once at ≥ 41,192 for the
common configuration (not per-variant), with the B0 comparability note that the
discovery limit changed. This is the only option that yields 5/5 EdgeX
repetitions.

**Option 2 — run EdgeX out of the batch.**
Report W1–W4 for ThingsBoard and FIWARE only, and record EdgeX as
`NOT_RUN_BLOCKED_SCOPE` for every W cell with this document as the reason. Clean,
defensible, and does not pretend 1-of-5 is 5-of-5.

**Option 3 — run the one attempt that fits (W1 rep 1) and stop.**
Technically permitted by the arithmetic. Scientifically weak: one unreplicated
attempt cannot enter a five-repetition fixed-point comparison, and it leaves a
cell that looks measured but is not comparable. Only worth doing if the goal is
explicitly a pilot, labelled as such.

My recommendation is **Option 2 for this batch, Option 1 later**, because
Option 1 changes a frozen common-batch setting mid-project and requires a
dedicated qualification pass after the change — which is a batch of its own, not
an adjustment to this one.

## What is *not* blocked

For the avoidance of doubt: **ThingsBoard and FIWARE are unblocked for W1, W2 and
W4.** Their only remaining prerequisites are the 50/s full-duration
qualification and the owner-authorization receipt. Only the following are held:

| Held | Reason |
|---|---|
| W1–W4 EdgeX (all) | blocker B; blocker A from W1 rep 2 onward |
| W3 FIWARE | Orion subscription omits `padding`, so the 1,024-byte claim has no evidence path |

## Reproduction

```bash
# Live inventory and caps (read-only)
ssh ramonavm2 'curl -s "http://127.0.0.1:59881/api/v3/device/all?limit=1"' | jq .totalCount
ssh ramonavm2 'curl -s http://127.0.0.1:59881/api/v3/config'      | jq '.config.Service.MaxResultCount'
ssh ramonavm2 'curl -s http://127.0.0.1:59986/api/v3/config'      | jq '.config.Service.MaxResultCount'

# Profile GET cost (48 ms/GET expected)
ssh ramonavm2 'start=$(date +%s%N); for i in $(seq 1 100); do \
  curl -s -o /dev/null http://127.0.0.1:59881/api/v3/deviceprofile/name/bench-gen-v1; done; \
  echo $(( ($(date +%s%N)-start)/1000000 ))ms'

# Gate arithmetic as the helper computes it
python3 -c "print(10192 + 6000 > 16384)"   # W1 rep1: False (passes, +192)
python3 -c "print(11192 + 6000 > 16384)"   # W1 rep2: True  (blocked)
```

## Related

- `research/findings/edgex-ordinal-15-readiness-failure.md` — the measured
  `InitCache` root cause and the ordinal-15 failure record.
- `research/findings/edgex-discovery-limit-and-inventory-scaling.md` — the
  `MaxResultCount` research and inventory growth model.
- `research/findings/w1-readiness-after-b0.md` — superseded on the EdgeX claim by
  this document.
- `deploy/edgex/scripts/provision-then-scale.sh:52` `wait_driver_ready`,
  `:110` `ensure_replica_ready`, `:194` inventory arithmetic.
