# Finding: ordinal 11 `disp-ex-r04-20261106` — second device-rest replica never exposes its ingest route

Zero-traffic setup failure, EdgeX, 2026-11-09. The workload never launched
(`workload_launch_attempted: false`, `platform_result: NOT_ASSESSED_NO_ANALYSIS`).
The attempt is preserved and was not retried under the same ID.

## Symptom

The launcher reached the post-scale readiness gate and failed on the **second**
replica:

```
device-rest ingest route not initialized before readiness deadline: http://172.21.0.13:59986
```

`edgex-device-rest-1` had already accepted 1000/1000 devices (1000 `200`s in
`edgex-gate-statuses/…edgex-device-rest-1.attempt1.http-statuses`). No statuses
file exists for `edgex-device-rest-2`, because `wait_driver_ready` failed before
the device loop ran. The gate JSON itself was never written — only the
`.driver-discovery.json`, `.metadata-discovery.json`,
`.retained-inventory.json` sidecars were produced.

## Timeline

| Time (UTC) | Event |
|---|---|
| 18:58:16 | cold start at 1 replica; core-metadata, core-data-1, device-rest-1 up |
| 18:58:18 | device-rest-1 logs 4 `core-metadata service not healthy` WARNs (metadata still warming) |
| ~18:58:2x | device-rest-1 ready; profile + 1000 devices provisioned; 1000/1000 accepted |
| 19:01:39 | scale-out: `core-data-2` created |
| 19:01:41.697 | `edgex-device-rest-2` started |
| 19:01:42.389 | device-rest-2 last log line: `listening for device service common config changes` |
| 19:01:44 | `app-http-export` started |
| 19:01:50 | keeper begins failing health-check DNS for `app-http-export` (continues to 19:11:45) |
| 19:04:53 | `launcher-error.json` written → 180 s readiness deadline expired |
| 19:11:48 | compose tears the stack down; device-rest-2 exits 0 |

## What the logs do and do not show

`WRITABLE_LOGLEVEL=WARN` means INFO logging is only emitted during bootstrap,
before the config is applied. So a short log is **normal**, not evidence of a
hang: healthy `core-data-1` also logged only 13 startup lines, and `core-data-2`
logged the same 13.

The one real asymmetry is this: healthy device-rest-1 emitted 4 **WARN**
`core-metadata` retry lines immediately after the config listeners, while
device-rest-2 emitted **zero** WARN lines. WARN is above the suppression
threshold, so a replica that reached the metadata/cache bootstrap would have
logged them. device-rest-2 therefore never reached route registration —
`driver.Start` never ran, so `GET …/resource/__bench_readiness__/reading`
never began returning 405.

## Ruled out

| Candidate | Evidence |
|---|---|
| core-metadata device-validation panic | `core-metadata` restarts 0, panics 0, started 18:58:16 and ran to shutdown |
| crash / restart loop | device-rest-2 `RestartCount 0`, `OOMKilled false`, `ExitCode 0` (compose stop) |
| OOM / memory pressure | 512 MiB limit not hit; host PSI 0.00; container never killed |
| Docker embedded-DNS incident | `server misbehaving` appears on successful days too (1595 on Nov 7, 7184 on Nov 8); all 120 Nov 9 hits target `app-http-export`, not `device-rest` |
| Retained-inventory ceiling | 9192 + 6000 reserve = 15192 < `MaxResultCount` 16384; gate passed |
| Export service | `app-http-export` was starting, not a dependency of the gate |

## Most consistent mechanism

The replica process stayed alive but never completed bootstrap, and the only
concurrent anomalies were (a) Docker DNS returning SERVFAIL for
`app-http-export` from 19:01:50 onward, and (b) core-keeper spending that same
window (120 consecutive failures) on those failing health checks instead of
serving config/cache requests. device-rest-2 pushes and reads its private
config through core-keeper (`EDGEX_OVERWRITE_CONFIG=true`), so a replica
starting into that window can block before `driver.Start`.

This is an inference from correlated timing, not a proven cause: the INFO
suppression and the absence of a failure-time log snapshot remove the decisive
evidence.

## Gap that made this hard to diagnose

`provision-then-scale.sh` does not capture the stalled replica's logs or
`docker inspect` state when `wait_driver_ready` fails. The only reason this
analysis was possible at all is that the containers survived until shutdown and
were inspected afterwards. A future occurrence could lose that evidence.

## Fix

**Do not change `deploy/edgex/scripts/provision-then-scale.sh` during the frozen
campaign.** Its SHA-256 `77c4ba8e8605262a98f529f3e76dd13657efb469c1f0cbdaeca4074a4b5575e4`
is bound in `private/continuation-review-r12.json` and is the recorded
`measurement_sources` hash. Ordinal 15 (`disp-ex-r05-20261106`) will run under
that binding; editing the file now would invalidate it.

Correct sequence:

1. **Now:** keep the failed attempt preserved and unrepeated (already done).
2. **After ordinal 15 / after the frozen batch closes:** land both changes below
   as a reviewed amendment, re-hash, and record the new binding.
3. **Never** silently retry `disp-ex-r04-20261106`.

> **Update 2026-11-10 — the sequence above was not followed, and that is now
> recorded as a defect.** The patch was landed *before* ordinal 15, which an
> independent review then rejected for exactly the reason stated here: the
> released receipt still bound `77c4ba8e…`, so the controller would have refused
> at `continue-ordinary-campaign.py:96`. The patch has since been hardened
> (helper `540d60b4…`, revision 2) and all review conditions are addressed, but
> **ordinal 15 remains unstarted and unauthorised** until a fresh receipt is
> issued against the executing tree. See
> `edgex-discovery-limit-and-inventory-scaling.md` §8. The guidance in this
> section — bind before you run — stands; what changed is that the change is now
> reviewed and the binding still has to be re-issued.

Recommended patch (post-campaign):

- **Preserve diagnostics on gate failure.** In `wait_driver_ready`, before
  returning 4, dump `docker logs --tail 200 <container>` and
  `docker inspect` (state, restarts, OOM, IPs, cpuset) into the gate directory.
  This alone would have made r11 conclusive.
- **Bound and report the pre-ping loop.** Line 101 is an unbounded
  `until curl … /ping; do sleep 2; done`. A replica that never answers `ping`
  hangs forever instead of failing within a deadline. Give it the same
  `EDGEX_DRIVER_READY_TIMEOUT` budget and a distinct error message.
- **Optional, only if independently reviewed:** one bounded
  `docker compose up -d --force-recreate --no-deps device-rest` retry of the
  stalled replica inside the gate budget, recorded as a setup step. This turns
  a transient startup stall into a recovered setup rather than a lost ordinal.
  It is a procedure change and must be disclosed, not applied silently.

## Classification

Zero-traffic setup/readiness failure. Not a platform backlog or capacity
result. Preserved; not retried. The controller was paused at ordinal 11 and
resumed only under a fresh review that treated r11 as preserved.
