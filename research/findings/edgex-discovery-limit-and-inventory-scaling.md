# Finding: EdgeX `MaxResultCount` — what it affects, what it does not, and the B0→W inventory ceiling

Investigation of `deploy/edgex/compose.feasible-discovery.yaml`
(`ALL_SERVICES_SERVICE_MAXRESULTCOUNT: "16384"`, sha256 `0893a6b8…`) against the
EdgeX **v4.0.2 source**, not against documentation or inference. Written
2026-11-10.

**Scope.** This note answers three questions: (1) does changing the value change
platform *performance*, (2) does it affect *memory handling*, and (3) what is the
real inventory ceiling for the separate W1–W4 batch. It does not authorise a
change and does not record any measurement. No run was executed for it.

## Summary

| Question | Answer |
|---|---|
| Does it change measured performance? | **No.** It is never executed on the ingest→sink path. It only clamps the `limit` argument of HTTP **list** queries. |
| Does it affect memory handling? | **Not directly.** It allocates nothing. Memory scales with the **device count** each replica caches — a count this value only *gates* (and gates silently). |
| Is 16384 an EdgeX limit? | **No.** Stock EdgeX ships `MaxResultCount: 1024`; EdgeX's own OpenAPI example uses `50000`. 16384 is a repo override, already 16× stock. |
| What does the W batch need? | **≥ 41,192** on the `core-metadata` side. 16,384 is short by 24,808. |
| Is ordinal 15 affected? | **No.** 10,192 + 6,000 = 16,192 ≤ 16,384, margin 192. It fits and must run at 16,384. |

## 1. What `MaxResultCount` actually does

Every non-test use of the field in `edgex-go` v4.0.2 is the identical single
pattern — it is the `maxLimit` argument of a list-query parser:

```go
// internal/core/metadata/controller/http/device.go:133, 234, 304
offset, limit, _, err := utils.ParseGetAllObjectsRequestQueryString(
    c, 0, math.MaxInt32, -1, config.Service.MaxResultCount)
```

`ParseGetAllObjectsRequestQueryString` rewrites a request of `limit=-1` to
`limit = maxLimit`. That is the entire semantic: **a clamp on how many rows one
HTTP list response may return.**

It is not used by the ingest, bus, storage or export paths. It never sizes an
allocation. The measured workload traverses:

| Stage | Code path | Touches the value? |
|---|---|---|
| Ingress | `POST /api/v3/resource/{name}/reading` → `validateServiceAndDeviceState` → `cache.Devices().ForName()` | No |
| Bus | NATS core → core-data | No |
| Store | PostgreSQL `core_data.reading` | No |
| Egress | app-http-export → sink | No |
| Measurement | offline dispatch→first-body sink receipt join | No |

`ForName` is an in-memory map lookup under a read lock
(`device-sdk-go` `internal/cache/devices.go:87`). No query, no limit.

**Conclusion for Q1:** a 25/s or 50/s run cannot become slower or faster because
of this value. Changing it is not a performance variable.

## 2. Why the value still matters — the silent-truncation coupling

Although the value is inert at runtime, it is a **correctness gate on startup**,
and it fails silently.

`device-sdk-go` v4.0.2 populates each replica's device cache at boot:

```go
// internal/cache/init.go:27
deviceRes, err := dc.DevicesByServiceName(context.Background(), instanceName, 0, -1)
```

`limit = -1` is clamped by `core-metadata` to `MaxResultCount`. The range check
that guards it cannot detect the resulting truncation:

```go
// edgex-go internal/pkg/utils/count.go:15
func CheckCountRange(totalCount int64, offset, limit int) (continueExec bool, err errors.EdgeX) {
	if limit == 0 || totalCount == 0 { return false, nil }
	if int64(offset) > totalCount { return false, errors.NewCommonEdgeX(... "bounds out of range" ...) }
	return true, nil
}
```

It only rejects `offset > totalCount`. A `limit` below `totalCount` is accepted
and the response is simply short. A replica whose cache load was clamped comes
up **healthy but partially blind**: devices past the clamp are absent from
`deviceMap`, so their ingested requests fail `ForName` with
`KindEntityDoesNotExist`.

So the hazard is not "the platform is slower". It is: **if retained inventory
ever exceeds the cap, replicas silently cache a prefix of the cohort**, and the
failure would appear as per-device ingest errors, not as a startup error.

**Which service's cap binds.** The client call is device-rest → core-metadata, so
the binding value is **`core-metadata`'s** `MaxResultCount`. device-rest's own
copy of the setting clamps device-rest's *own* list endpoints and does not govern
its cache load. Both receive the same override, so today both read 16384, and the
setup gate conservatively requires both — that is stricter than necessary and
should be understood as a conservative check, not two independent constraints.

## 3. Memory: driven by device count, not by the cap

Raising the cap allocates nothing. Memory is set by how many devices each replica
caches, and this stack caches the **whole cohort per replica**: `device-rest`
deliberately uses no NATS queue group, so every add/update system event reaches
every replica, and every replica loads all devices belonging to the service name
(`compose.yaml`, service `device-rest`).

Per cached device (`internal/cache/devices.go:50` `newDeviceCache`):

- one entry in `map[string]*models.Device`;
- one `gometrics.Gauge` registered under `LastConnected-<device>` — a metrics
  registry entry **per device**, not per service;
- one entry in the `lastConnected` map.

Order of magnitude, against the 512 MiB `EDGEX_DEVICE_REST_MEM` limit:

| Retained devices | ≈1 KiB/device | ≈2 KiB/device | ≈3 KiB/device |
|---:|---:|---:|---:|
| 10,192 (after r15) | 9.7 MB | 19.4 MB | 29.2 MB |
| 35,192 (full W batch) | 33.6 MB | 67.1 MB | 100.5 MB |

All well inside 512 MiB, with two such replicas. These are estimates from the
data structures, **not measurements**; no cgroup memory figure has been captured
for an EdgeX stack in this campaign. The cap is not what makes them large.

## 4. A separate scaling cost the cap does not cause

`InitCache` fetches the profile **inside a per-device loop** with no
deduplication, even though the benchmark provisions a single shared profile for
the whole cohort:

```go
// internal/cache/init.go:36
for _, d := range devices {
    if len(d.ProfileName) == 0 { continue }
    res, err := dpc.DeviceProfileByName(context.Background(), d.ProfileName)  // one HTTP GET per device
    if err != nil { return err }                                              // failure aborts cache init
    profiles = append(profiles, dtos.ToDeviceProfileModel(res.Profile))
}
```

That is `N` sequential HTTP GETs to core-metadata per replica at boot — 10,192
today, 35,192 at full W retention, doubled across replicas — all returning the
same profile. Two consequences worth recording:

1. **Startup time grows linearly with retained inventory**, independent of
   `MaxResultCount`. This matters because the readiness deadline is bounded
   (`EDGEX_SCALE_TIMEOUT`, 180 s) and retained inventory only ever grows across
   the campaign.
2. **A single failed profile GET aborts cache init**, so the replica never
   registers its routes — a *zero-traffic setup failure* shape, not a traffic
   failure.

This is a plausible contributing factor to the class of failure seen at ordinal 11
(second replica never exposed its ingest route; see
`edgex-r11-second-replica-readiness-failure.md`), where metadata and keeper were
concurrently degraded. It is recorded as a scaling mechanism worth watching, not
as a proven cause of that specific attempt, whose evidence showed device-rest-2
had not begun the metadata/cache bootstrap at all.

> **Update 2026-11-10 — this is no longer a hypothesis. It is the measured cause
> of ordinal 15.** `disp-ex-r05-20261106` failed at the readiness gate on the
> *first* replica, before any device was provisioned: device-rest-1 logged
> nothing for ~182 s and the route only answered 405 about 4.5 minutes after
> start, against a 180 s deadline. A live measurement on the SUT gives ~46 ms per
> profile GET, projecting **~468 s for 10,192 devices** and **~1,618 s for
> 35,192** — 2.6× and 9× the deadline. Full account:
> `edgex-ordinal-15-readiness-failure.md`. The direct consequence is that the
> inventory-cap bump argued in §6–§7 is **necessary but not sufficient** for the
> W EdgeX batch.

## 5. Is 16384 a platform limit? No

| Source | Value |
|---|---|
| `edgex-go` v4.0.2 `cmd/core-common-config-bootstrapper/res/configuration.yaml:23` (stock `all-services`) | **1024** |
| `edgex-go` v4.0.2 `openapi/core-metadata.yaml` (documented example) | 50000 |
| This repo, `deploy/edgex/compose.feasible-discovery.yaml:6` | 16384 |

The override was added on 2026-11-07 (commit `2537afe`) with the comment
*"Images, replicas and resource limits are unchanged."* It is a deliberate
stopgap to preserve historical device cohorts, already 16× stock. Nothing in
EdgeX caps it at 16384.

## 6. Inventory arithmetic (the actual constraint)

Observed `core-metadata` retained counts from the gate sidecars:

| Ordinal | Run | Retained | +6,000 reserve vs 16,384 |
|---:|---|---:|---|
| 1 | `disp-ex-r01` | 6,192 | 12,192 pass |
| 2 | `disp-ex-r02` | 7,192 | 13,192 pass |
| 3 | `disp-ex-r03` | 8,192 | 14,192 pass |
| 4 | `disp-ex-r04` | 9,192 | 15,192 pass |
| 5 | `disp-ex-r05` (pending) | 10,192 | **16,192 pass, margin 192** |

Each EdgeX ordinal provisions 1,000 devices on a 4,192 baseline and retains them
by design. The gate is
`retained + EDGEX_INVENTORY_RESERVE(6000) <= MaxResultCount`.

W batch (5 attempts × {W1 1,000, W2 2,000, W3 1,000, W4 1,000} = 25,000 new
devices):

```
10192 + 25000 = 35192 retained
35192 + 6000  = 41192 required  ->  cap must be >= 41192
16384 shortfall = 24808
```

First blocked cell: **W1 EdgeX attempt 1** — 11,192 + 6,000 = 17,192 > 16,384.
Ordinal 15 is the last EdgeX cell that fits at the current cap.

## 7. Consequences for the campaign (not yet executed)

- **Ordinal 15 must run at 16,384, unchanged.** It fits (margin 192) and its
  review/receipt bind this file. *(It subsequently failed at the readiness gate,
  before the inventory check ran — see `edgex-ordinal-15-readiness-failure.md`.
  The cap was never the binding constraint for this attempt.)*
- **Ordinals 12–14 ran at 16,384** and are preserved. A later bump does not
  invalidate them: they were measured correctly under their own frozen
  configuration. It does mean a W-EdgeX vs B0-EdgeX comparison carries a
  disclosed configuration discontinuity, which must be stated rather than hidden.
- **Do not re-run B0 EdgeX** at a new cap to "align" them — replacing valid
  results is forbidden by the frozen scope.
- **Bump once, after ordinal 15, and freeze it as the W baseline.** `50000`
  matches EdgeX's documented example and clears 41,192 with 8,808 headroom.
- **Prefer a monitored, fail-closed memory gate** for W over a one-off
  extrapolation: sample `device-rest` cgroup `memory.peak` around setup and
  refuse to launch if projected usage approaches `EDGEX_DEVICE_REST_MEM`.
- The truncation risk in §2 means the bump must be accompanied by a real
  **whole-cohort acceptance check**, not only an inventory comparison. Today's
  gate POSTs to devices named `bench-$RUN-*`, i.e. only the current run's 1,000
  devices, so it does not currently prove that every replica cached the full
  historical cohort. That gap is benign at 10,192 and would not be at 35,192.

## Reproduce

```bash
# stock default vs repo override
grep -n MaxResultCount \
  /tmp/opencode/exgo/cmd/core-common-config-bootstrapper/res/configuration.yaml
grep -n MAXRESULTCOUNT deploy/edgex/compose.feasible-discovery.yaml

# the clamp, and its silence about truncation
grep -n MaxResultCount /tmp/opencode/exgo/internal/core/metadata/controller/http/device.go
sed -n '15,22p' /tmp/opencode/exgo/internal/pkg/utils/count.go

# cache init: limit=-1, then one profile GET per device
sed -n '27,45p' /tmp/opencode/sdk/internal/cache/init.go

# observed retained inventory
for f in results/feasible-campaign-20261106/evidence/disp-ex-r0*/edgex-gate.json.retained-inventory.json; do
  printf '%s: ' "$f"; jq -c .totalCount "$f"
done
```

Source trees used: `edgex-go` v4.0.2 and `device-sdk-go` v4.0.2, both checked out
under `/tmp/opencode/` at their release tags.

## 8. Addendum (2026-11-10): the ordinal-15 helper patch and its review

The inventory work above is independent of the **setup-helper repair** that was
also under way for ordinal 15. They are recorded together here because both were
reviewed on the same day and both changed bytes bound by the release receipt.
This section does not change anything in §1–§7.

An independent review of `provision-then-scale.sh` revision 2 returned **REJECT**
on procedural grounds: revision 2's bytes no longer matched the hash bound in the
existing r15 receipt, so the frozen controller would abort at
`continue-ordinary-campaign.py:96` before any mutation. That is correct
behaviour, not a bug — the binding exists to prevent an un-reviewed change from
running on the last ordinal. The review also found concrete defects in the patch
itself, all now fixed:

| Defect | Effect | Resolution |
|---|---|---|
| `wait_driver_ready` read `DRIVER_TIMEOUT` from outside the extracted function text | `tests/edgex_driver_ready_test.py` failed (empty deadline) | default read inside the function via `${EDGEX_DRIVER_READY_TIMEOUT:-180}`; the now-unused top-level `DRIVER_TIMEOUT` was removed |
| `test_driver_wait_precedes_any_provisioning` searched for `wait_driver_ready` directly in the `prepare` block | test errored; the readiness-before-provisioning invariant was no longer asserted | re-pointed at `ensure_replica_ready`, plus a new assertion that no `until` loop exists in the setup path |
| Bare `ip=$(docker inspect …)` under `set -e` | aborted **before** `setup_failure_dump`, so the promised diagnostic was never written | all four substitutions now `|| true` with an explicit emptiness check |
| `curl … \| jq` and `jq -er` on evidence fetches | same abort-before-dump; and `jq -e`'s exit **4** collided with the helper's own "gate failed, exit 4" | replaced with `fetch_json()` (validates JSON, dumps, returns 1) and `jq -r '.x // empty'` with a numeric regex guard that reports a distinct reason |
| Inventory-limit failure path had no dump | a real gate failure left no diagnostics | `setup_failure_dump` added before `exit 4` |
| Non-numeric `EDGEX_REPLICA_REPAIR_RESTARTS` | bare `unbound variable`, exit 1, no dump | validated at parse time, `exit 2` with a clear message |
| `fetch_edgex_gate()` did not request `.json.failure.txt` | the new dump stayed on the SUT — unreachable evidence | suffix added, so the dump is retrieved with the other gate artifacts |
| Worst-case setup ≈ 2,390 s vs launcher `timeout=2400` | 10 s margin | raised to 5,400 s |

Verification: `bash -n` clean; no `until` loop in the setup path; all four
`ensure_replica_ready` paths exercised with stubs (already-ready → `rc=0`, no
restart; stalled-then-healed → `rc=0`, exactly one restart; never-healed →
`rc=4` with budget respected; budget 0 → no restart); all 18 test modules green,
including the two that previously failed.

The repair remains bounded to **setup only** — provisioning at one replica, the
405 not-a-device probe, the inventory gate and every measurement-path element are
unchanged. `docker restart` does not increment `RestartCount`, which the review
noted; the gate now records `helper_revision: 2` and
`replica_repair_restarts: N`, and any run with `N > 0` must carry an explicit
deviation entry in its manifest, because no earlier EdgeX run had a setup
intervention of that kind.

**Status: still not executable.** A fresh receipt binding the current helper
(`540d60b4…`, revision 2 after these fixes) must be issued, and it must describe
the **executing** tree, not this one — the SUT holds a different
`scripts/dispatch-integrity.py` (`d59876b3…`) from the local checkout
(`335e8451…`). Nothing in this section authorises ordinal 15.


## Limits of this note

- Memory figures are structural estimates; no EdgeX cgroup memory measurement
  exists in this campaign.
- The relationship between the per-device profile-GET loop and the ordinal-11
  readiness failure is a hypothesis about a mechanism, not an established cause.
- Nothing here authorises execution. The W batch remains
  `PROPOSED_NOT_LAUNCH_CLEARED` with `execution_authorized: false`.
