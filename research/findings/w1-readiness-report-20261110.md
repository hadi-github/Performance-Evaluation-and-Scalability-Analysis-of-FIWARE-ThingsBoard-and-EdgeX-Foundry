# W1 readiness report — 2026-11-10

> **SUPERSEDED IN PART.** Blocker 1 below ("the load host is down") was **wrong**
> and has been corrected. `ramonavm1` is reachable via `ProxyJump ramonavm2`;
> direct connections fail, which is what produced the false reading. The
> qualifications have since been run and both platforms failed the VU-pool gate.
> **Read `research/findings/w-batch-status-20261110.md` for current state.**
> This page is retained for its EdgeX and W3 findings, which still stand.

**No W1 traffic has been launched.** This report records what was built, what
was verified, and the two hard blockers that make launching impossible right
now. It does not mark W1 launch-cleared and it does not change
`execution_authorized`, which remains `false`.

## Bottom line

| Question | Answer |
|---|---|
| Is the W launcher built? | **Yes** — `scripts/run-workload-campaign.py`, 60 rows / 0 problems |
| Is W-aware aggregation built? | **Yes** — `scripts/aggregate-runs.py`, 0 B0 cells changed |
| Is the W1 50/s qualification staged? | **Yes** — three plans, 45,000 identities each, verified |
| Has the qualification run? | **Yes** — TB and FIWARE both `INVALID_VU_POOL_BOUND` (see correction above) |
| Can W1 run today? | **No** — the frozen 150/150 VU pool binds at 50/s on both platforms tested |
| Is W1 EdgeX runnable at all? | **No** — readiness gate needs ~489 s of profile fetches against a 180 s deadline |
| Is W3 FIWARE runnable? | **No** — the subscription never delivers `padding` |

## Blocker 1 (RETRACTED): "the load host is down"

This section was wrong. `ramonavm1` (87.107.191.32) is **up and reachable** via a
jump through the SUT. The original evidence was real but misinterpreted:

```
ping 87.107.191.32      -> 100% packet loss      (ICMP blocked, not down)
</dev/tcp/87.107.191.32/22 -> CLOSED             (direct route blocked)
ssh ramonavm1           -> Connection timed out  (direct route blocked)
from ramonavm2: getent hosts ramonavm1 -> no DNS (name, not host)
```

The host responds normally with
`ssh -F <config> ramonavm1` where the config sets `ProxyJump ramonavm2` and
`HostName 87.107.191.32`. **Lesson recorded rather than quietly deleted: an
ICMP/direct-route failure is not proof a host is down, and "no DNS from the SUT"
only says the name is unresolved.**

## Blocker 2 (EdgeX, measured): readiness cost is O(retained devices)

Measured live today, not projected:

| Quantity | Value |
|---|---:|
| Retained EdgeX devices | 10,192 |
| `MaxResultCount` (metadata and driver) | 16,384 |
| Profile GET cost | **48 ms** (100 sequential GETs = 4,797 ms) |
| Projected `InitCache` at 10,192 | **≈ 489 s** inside a **180 s** deadline |

Gate order matters: readiness (~line 160) runs **before** the inventory check
(line 194), so a cap bump alone would not admit an EdgeX run. Also correcting a
prior overstatement: **W1 rep 1 clears the inventory gate by 192 devices**
(10,192 + 6,000 reserve = 16,192 vs 16,384); rep 2 and every later attempt do
not. One attempt is not five comparable repetitions.

The live container currently answers the readiness route with 405, but it was
allowed unbounded time outside the gate's deadline, so that observation does not
settle the question. The 22 `panic:` entries in its log are all dated
2026-11-06 (B0 era) and are historical, not current.

Full account: `edgex-w-batch-blocker.md`. **This is left open for your decision**
as instructed: raise the cap (batch-wide, with a comparability note) or run
EdgeX out of the batch.

## Blocker 3 (W3 FIWARE, confirmed today)

The Orion subscription delivers `attrs: [seq, run_id, scheduled_send_ms,
temperature]`. The 1,024-byte `padding` field is never in that list, so the W3
claim has no evidence path. `scripts/history-payload.sh` can already read a
stored `padding` column, but the field never reaches storage via this
subscription. This needs a reviewed payload-preserving subscription, which the
gate explicitly says must not be changed for W3 alone.

## Blocker 4 (honest, governance): authorization is not yet recorded

`scope.json` says `execution_authorized: false` and the matrix says
`PROPOSED_NOT_LAUNCH_CLEARED`. Your instruction authorizes the work, but I will
not flip a fail-closed flag myself — that is exactly the self-authorization the
B0 discipline exists to prevent. It should be raised as an explicit
owner-authorization receipt, and I have left it untouched.

## The real W1 risk, quantified: the 150-VU pool

This is the finding most likely to decide whether W1 succeeds, and the earlier
note had it backwards ("unlikely to bind").

At 25/s, every measured ThingsBoard cell peaked at **125 VUs — exactly
`25/s × 5 s`** — with warm-up first-minute p99 of 1,925–2,154 ms and maxima at
4,967–4,995 ms. The pool was fully saturated and never dropped only because
125 < 150.

At 50/s the same behaviour demands `50 × 5 = 250` VUs against a 150 pool: a
100-VU shortfall, in the phase where the platform is slowest. Sustained
throughput from a 150 pool is `150 / latency`, so 3 s of latency is the exact
ceiling for 50/s.

The pool cannot simply be raised: `scripts/dispatch-integrity.py:20` raises
`frozen VU allocation is 150/150` if the resolved plan deviates. That is a policy
change, not tuning, and must be frozen before outcomes are seen.

So the qualification is designed to test it rather than argue it. Decisions are
fixed in advance:

- **0 drops, 45,000 identities, clean capture/history** → pool does not bind; W1
  may proceed on qualified platforms, recording peak VU and warm-up tail as
  headroom evidence.
- **Any `dropped_iterations > 0`** → the frozen policy is not viable at 50/s;
  W1 is **not** launched; the result is preserved as a valid negative; the pool
  question becomes an owner decision. Not retried, not silently raised.

Analysis: `w1-vu-pool-headroom-risk.md`.

## What was built today (all offline, all tested)

### 1. `scripts/run-workload-campaign.py` — the W launcher

Reuses the proven B0 machinery instead of re-implementing it:

```python
b0.REMOTE_RESULTS = '/root/iot-qual-load-20261105/feasible-workload-w-20261108'
b0.EVIDENCE        = REPO / 'results/workload-batch-20261108/evidence'
```

Only plan resolution, plan validation and evidence roots are rebound; staging,
activation, resource gates, capture, cleanup and evidence retrieval are the same
code that ran B0. W therefore cannot contaminate B0 provenance.

Actions: `check` (offline), `resolve` (offline, real `benchctl`), `launch`
(requires `--execute-reviewed`).

### 2. `scripts/stage-w1-qualification.py` — the gate plan

Three plans reproducing the W1 attempt shape **exactly** except identity:

| Run ID | Platform | Warm-up | Measurement | Seed |
|---|---|---:|---:|---:|
| `w1-qual-tb-20261110` | ThingsBoard | 15,000 | 30,000 | 202611060 |
| `w1-qual-fw-20261110` | FIWARE | 15,000 | 30,000 | 202611060 |
| `w1-qual-ex-20261110` | EdgeX | 15,000 | 30,000 | 202611060 |

The seed is deliberately outside `seed_by_repetition`, and the IDs are
`w1-qual-*` so they can never be confused with the 60 proposed attempts. Per
`qualification_policy`, none counts toward the 60.

The qualification validator is **proven to fail**: it rejects short duration,
wrong rate, a 300-VU pool, a different target, injected faults, a wrong seed,
padding drift and lost identities.

### 3. `scripts/aggregate-runs.py` — W-aware aggregation

The `all_workloads` gate requires workload-aware aggregation. It now:

- identifies `w{1..4}dp-{tb,fw,ex}-*` run IDs (previously `platform=None`);
- tags every row with `workload_id`;
- groups summary sections by **(workload, platform)**, so a W1 and a W2 attempt
  on ThingsBoard can never share a pooled statistic.

Verified behaviour-neutral: regenerating the B0 aggregate from the same evidence
changes **0** existing cells and adds 13 qualification rows that were already in
the evidence root.

### 4. Drift fixed honestly

`scripts/stage-fixed-point-variants.py --check` previously exited 1 because the
staged scope lacked the two later B0 supplemental plan hashes
(`disp-tb-r04a/r04b`). Only those two entries were missing; refreshing them makes
`--check` exit **0** while leaving `execution_authorized: false` and
`PROPOSED_NOT_LAUNCH_CLEARED` untouched.

## Verification performed

| Check | Result |
|---|---|
| `run-workload-campaign.py check` | 60 rows, 0 problems, 3 qualifications |
| All three W1 plans resolve | 15,000 + 30,000 ticks, 50/s, 900 s |
| All three qualification plans resolve | 45,000 ticks, 150/150 pool |
| `stage-fixed-point-variants.py --check` | exit 0 |
| `stage-w1-qualification.py --check` | 3 plans verified |
| Prebuilt `benchctl` matches source | `64f66385…` = `4de110dd…` |
| Test suite | **19/19 modules pass** (`export_run_test` skips 1 internally) |
| B0 aggregate regression | 0 cells changed |
| EdgeX retained inventory (live) | 10,192 |
| EdgeX profile GET (live) | 48 ms |

## Ceilings on any claim after W1

Per `scope.json` `claim_scope`: **fixed-point comparisons only** — no maximum
capacity, no scaling curve, no cold-start IID claim. W1 is a 50/s fixed point at
1,000 devices, five repetitions, on retained state.

## What happens the moment the load host returns

1. Run the 50/s qualification on ThingsBoard (15 min). Nothing else starts first.
2. Read the decision tree above. A drop count > 0 stops W1 entirely.
3. If clean, W1 on ThingsBoard and FIWARE (18 attempts, both unblocked otherwise).
4. EdgeX stays held pending your cap decision; W3 FIWARE stays held pending a
   payload-preserving subscription.

## Related

- `research/findings/w1-vu-pool-headroom-risk.md`
- `research/findings/edgex-w-batch-blocker.md`
- `research/findings/w1-readiness-after-b0.md`
- `workloads/campaigns/fixed-points-w1-w4/qualification/index.json`
- `scripts/run-workload-campaign.py`, `scripts/stage-w1-qualification.py`
