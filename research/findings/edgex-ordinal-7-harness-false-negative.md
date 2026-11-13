# Finding: ordinal 7 `disp-ex-r03-20261106` — a harness false negative, not a platform failure

Post-campaign analysis, 2026-11-10. No re-run. No evidence mutated. The original
verdict (`NOT_ASSESSED_NO_ANALYSIS`, zero offered messages) stands unchanged.

This attempt was long recorded as an unexplained `effective lifecycle/health
mismatch: ('edgex', 'core-data')`. Investigating it against the ordinal-15
evidence shows the most likely cause is a **defect in the benchmark harness**, not
in EdgeX.

## What the record already shows

| Fact | Value | Source |
|---|---|---|
| Readiness gate | **passed** | `edgex-gate.json` |
| Replicas ready | 2, `readiness_attempts [1, 1]` | `edgex-gate.json` |
| Devices accepted | 1000 per replica | `edgex-gate-statuses/` |
| core-metadata restarts / panics | 0 / 0 | `edgex-gate.json` |
| Traffic offered | **0 messages** | `workload_launch_attempted: false` |
| Refusal point | pre-traffic, after `activate()` | `launcher-error.json` |

So the platform came up correctly. Something *after* the readiness gate decided
the stack was unhealthy.

## The mechanism

`check_effective_containers()` (`run-feasible-campaign.py:247-249`) required, for
every container:

```python
state.get('Running') is True
state.get('OOMKilled') is False
c.get('RestartCount') == 0          # <-- the problem
state.get('Health', {}).get('Status', 'healthy') == 'healthy'
```

`RestartCount` is **cumulative for the container's entire lifetime**. Docker does
not reset it on `docker compose stop` / `up`. This campaign persists containers
and volumes across ordinals by design — every service carries
`restart: unless-stopped` — so a container can hold a restart from an *earlier*
ordinal while being perfectly healthy in the current one.

The check therefore conflated **"restarted at some point"** with **"restarting
now"**, and a long-lived container becomes progressively more likely to be
rejected the longer the campaign runs.

### Corroborating evidence from ordinal 15

The ordinal-15 failure dump, taken on the same persisted stack two ordinals
later, records:

```
name=/edgex-core-data-1 restarts=1 exit=0
```

`core-data` demonstrably accumulates a nonzero lifetime restart count on this
stack. That is exactly the condition that would trip this check.

## The gap (stated plainly)

**This is not proven.** No `effective-resources-after.json` exists for ordinal 7
— the pre-traffic exception path aborted before the snapshot was written — so
*which* of the four sub-conditions tripped cannot be established retroactively.
`RestartCount` is the most likely candidate, but it remains inference. This
analysis does **not** upgrade the original verdict, and it does not claim the
platform passed on merit.

## The fix

`check_effective_containers()` now takes an optional `restart_baseline`:

- The **pre-traffic** snapshot uses `admit_restarts=True`, adopting each
  container's current count as its baseline. Persisted containers are admitted.
- The **post-run** snapshot enforces that baseline. A count that *rises during
  the run* still fails — the real condition of interest.
- With **no baseline**, absolute zero is still required. The check is therefore
  not weakened for fresh containers or for callers that supply none.

Covered by two tests:

- `test_historical_restart_is_admitted_only_with_a_baseline` — proves both the
  admission and that a mid-run restart still raises.
- `test_restart_baseline_uses_max_count_per_service` — replicated services
  collapse to one baseline entry.

All 18 test modules pass, including the pre-existing strict-mismatch test, which
confirms the check did not become permissive by accident.

One deliberate limit: the failure message now reports the actual values
(`running=… oom=… restarts=… allowed<=… health=…`) instead of a bare mismatch
string, so a future occurrence is diagnosable without a lost snapshot.

## What this does not do

- **It does not recover ordinal 7.** The frozen matrix allows one attempt per
  cell, and `attempt_policy` forbids retry or replacement. The cell stays
  `NOT_ASSESSED_NO_ANALYSIS` with zero offered messages.
- **It does not change EdgeX's tally.** Still 2 valid measurements
  (ordinals 3 and 5) out of 5.
- **It does not affect ordinals 11 and 15.** Those are genuine platform readiness
  failures with a different, measured cause — see
  `edgex-ordinal-15-readiness-failure.md`.

Its value is narrow and worth stating exactly: the benchmark will no longer
discard a healthy, persisted stack for carrying a historical restart count.
