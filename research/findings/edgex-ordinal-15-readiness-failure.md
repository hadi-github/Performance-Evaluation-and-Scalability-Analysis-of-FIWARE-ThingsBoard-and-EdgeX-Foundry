# Finding: ordinal 15 `disp-ex-r05-20261106` — device-rest never registers its ingest route

Zero-traffic setup failure, EdgeX, 2026-11-10. The workload never launched
(`workload_launch_attempted: false`, `platform_result: NOT_ASSESSED_NO_ANALYSIS`).
The attempt is preserved and was **not** retried under the same ID.

This is the last cell of the frozen B0 matrix, and it fails on a **measured**
scaling mechanism rather than the inventory cap that was expected to end the
batch.

## Symptom

```
device-rest ingest route not initialized before readiness deadline (180s):
    http://172.21.0.9:59986 (last status: 404)
replica edgex-device-rest-1 did not expose its ingest route; bounded restart 1/1
device-rest ingest route not initialized before readiness deadline (180s):
    http://172.21.0.9:59986 (last status: 404)
```

The helper exited 4. The repair path from revision 2 fired exactly once (its
budget) and the route still did not appear. The launcher recorded a setup failure.

Failure occurred **before** device provisioning, so:
- retained inventory stayed at **10,192** (no `bench-disp-ex-r05-*` devices exist),
- no run directory was created on the SUT,
- **zero traffic**, `measurement_valid: null`.

## Timeline (UTC)

| Time | Event |
|---|---|
| 08:39:28 | controller starts ordinal 15, single-replica boot |
| 08:40:09 | core-metadata starts; device-rest-1 starts 08:40:09, ping passes |
| 08:40:10–14 | device-rest logs 5 `core-metadata service not healthy` WARNs, then **silence** |
| 08:43:21 | device-rest-1 container restarted (`StartedAt` 08:43:21) |
| 08:43:22 | config re-loaded from keeper; last log line emitted |
| 08:43:22 → 08:46:24 | **~182 s of complete log silence** |
| 08:46:24 | readiness deadline (180 s) expires; helper writes dump, exits 4 |
| 08:46:35 | controller → `PAUSED_REQUIRES_REVIEW` |
| 08:47:56 | route answers **405** — i.e. it became ready ~4.5 min after start |

The route *did* eventually register. It simply took far longer than the deadline.

## Root cause — the per-device profile fetch

`device-sdk-go` v4.0.2 `internal/cache/init.go`:

```go
deviceRes, err := dc.DevicesByServiceName(ctx, instanceName, 0, -1)   // line 27
devices := make([]models.Device, len(deviceRes.Devices))
newDeviceCache(devices, dic)

profiles := make([]models.DeviceProfile, 0, len(devices))
for _, d := range devices {                                          // line 36
    res, err := dpc.DeviceProfileByName(context.Background(), d.ProfileName)
    if err != nil { return err }
    profiles = append(profiles, dtos.ToDeviceProfileModel(res.Profile))
}
```

The benchmark provisions **one shared profile** (`bench-gen-v1`) for the whole
cohort, but this loop issues **one sequential HTTP GET per device**, with no
deduplication. Cache init therefore costs O(N) round-trips before
`driver.Start` can register the POST ingest route — and the readiness probe
correctly reports 404 until then.

### Measured, not inferred

100 sequential `GET /api/v3/deviceprofile/name/bench-gen-v1` against the live SUT:

```
100 GETs = 4625 ms  ->  ~46 ms each
projected 10192 devices:  468 s per replica
projected 35192 devices: 1618 s per replica
deadline:                 180 s
```

So at the current retained inventory the cache load alone is projected at ~2.6×
the deadline; at full W retention it is projected at ~9×. This matches the
observed behaviour exactly: the route appeared at ~4.5 min, well past 180 s.

This was recorded as a *hypothesis* in
`edgex-discovery-limit-and-inventory-scaling.md` §4 ("a separate scaling cost the
cap does not cause"). Ordinal 15 is the measurement that promotes it to the
operative cause.

## What this is NOT

- **Not an inventory-cap failure.** `MaxResultCount` (16,384) never entered the
  picture: the failure happened at the readiness gate, which runs *before*
  provisioning and before the inventory arithmetic. The bump to 50,000 proposed
  for the W batch would not have prevented this.
- **Not the ordinal-11 shape.** r11 failed on the *second* replica after
  provisioning 1,000 devices. This failed on the *first* replica before any
  device was created.
- **Not a panic or crash.** `restarts=0`, `exit=0`, `oom=false`; core-metadata
  reported 0 restarts and 0 panics.
- **Not caused by the revision-2 patch.** The patch added a bounded restart and
  the failure dump, both of which worked as designed: the dump is what made this
  diagnosis possible, and it would have been unreachable before the
  `.json.failure.txt` retrieval fix.

## Diagnostics the patch delivered

The failure dump captured container state, `docker inspect` (restarts, exit, OOM,
IP), 80 lines of device-rest logs, and core-keeper/core-metadata context. Without
it the 182 s silence — the decisive evidence — would have been lost. This
validates the repair and the retrieval suffix added in the same patch.

One limitation remains: `docker restart` does not increment `RestartCount`, so
the dump shows `restarts=0` for a container that was restarted. The
`replica_repair_restarts: 1` field in the gate is the authoritative record.

## Consequences

1. **B0 EdgeX is finished with 2 valid measurements** (ordinals 3 and 5:
   `disp-ex-r01`, `disp-ex-r02`). Ordinals 7, 11 and 15 are preserved setup
   failures. The batch closes as-is; **nothing is re-run**.
2. **Ordinal 15 stays failed and unrepeated.** Its evidence directory exists and
   must not be reused or overwritten.
3. **The W EdgeX batch is blocked on a platform defect, not a config value.**
   At 35,192 retained the projection is ~1,618 s per replica. Raising
   `EDGEX_DRIVER_READY_TIMEOUT` to cover it would mean a ~30 min setup per
   attempt and still leaves two replicas each doing O(N) fetches — and it would
   be tuning under a freeze. The real fix is to stop fetching the same profile N
   times.
4. **The 50,000 cap proposal is now insufficient on its own.** It remains
   necessary for the inventory gate (41,192 required), but it is not sufficient
   for the W batch to start. Both conditions must be met.

## Fix options (not applied — post-campaign decision)

- **Deduplicate the profile fetch.** Cache by `ProfileName`; one GET per distinct
  profile instead of one per device. O(profiles) instead of O(devices). This is
  upstream `device-sdk-go` behaviour, so a local fix means either patching the
  SDK or providing the profile through config rather than a fetch loop.
- **Do not retain inventory across runs.** The cohort is what makes the cost
  grow; B0 retains by design, which is why each ordinal is slower than the last.
- **Raise the readiness deadline** *only* together with one of the above — alone
  it papers over an O(N) cost that gets worse every run.

## Reproduce

```bash
# readiness failure and the deadline
grep -E "ingest route|readiness deadline" \
  results/feasible-campaign-20261106/evidence/disp-ex-r05-20261106/launcher-error.json

# the decisive 182 s log silence
grep -E "2026-11-10T08:4[3-6]" \
  results/feasible-campaign-20261106/evidence/disp-ex-r05-20261106/edgex-gate.json.failure.txt

# the O(N) profile loop
sed -n '27,45p' /tmp/opencode/sdk/internal/cache/init.go

# zero traffic / unchanged inventory
jq '{platform_result, workload_launch_attempted, measurement_valid}' \
  results/feasible-campaign-20261106/evidence/disp-ex-r05-20261106/run-result.json
```

## Limits of this note

- The ~46 ms/GET figure is a live measurement taken *after* the failure, against
  a warm, idle metadata service. Under the concurrent cold start it may differ;
  the 182 s silence is the stronger, direct observation.
- The projection extrapolates linearly from 100 samples. It is consistent with
  the observed timing but was not measured at 10,192 or 35,192 directly.
- Whether a deduplicated fetch alone restores the 180 s budget at 35,192 devices
  is untested.
