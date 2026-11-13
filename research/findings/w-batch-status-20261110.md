# W batch status — 2026-11-10 (post-qualification)

> **SUPERSEDED by `generator-pool-amendment-20261110.md` on the W1 verdict.**
> The pool problem described here was real but was a framework artifact, not a
> platform limit. The owner authorised a rate-derived pool; ThingsBoard then
> qualified `VALID` with 0 drops. W1 is no longer pool-blocked. The EdgeX,
> W2/W3/W4 and W3-FIWARE sections below still stand.

**Where things stand after running the three W1-release qualifications that were
authorized.** This supersedes the "load host is down" and "VU pool is a caveat"
framings in earlier notes; both were wrong and are corrected here.

---

## 1. Headline

- **W1 was not launchable under the frozen 150/150 pool** — but that pool was a
  generator constant, not a platform limit, and has since been amended. Two
  full-duration 50/s qualifications ran under v1 and both failed the
  pre-registered zero-drop gate. **No proposed attempt was spent.** Under the
  amended pool, ThingsBoard qualified `VALID` with 0 drops.
- **W2/W3/W4 were not attempted.** They are not blocked by the VU pool (they run
  at 25/s, where the pool cannot bind by arithmetic), but each has its own
  configuration gate, and the `other_configuration_headroom` clause is ambiguous
  in a way only the owner can settle.
- **EdgeX W runs remain blocked and deliberately not run**, per the standing
  instruction to document and leave that open until the final decision.
- **60 of 60 proposed attempts are still `BLOCKED_PENDING_VARIANT_RELEASE`.**
  `scope.json` `execution_authorized` remains `false`.

## 2. What was actually run

Three qualification IDs were staged under a hash-bound owner receipt
(`owner-w1-qualification-20261110c`). Qualifications are **not** among the 60
proposed attempts and never count toward them.

| Run ID | Platform | Outcome | Drops | Peak VUs | Evidence |
|---|---|---|---:|---:|---|
| `w1-qual-tb-20261110c` | ThingsBoard | `INVALID_VU_POOL_BOUND` | 134 | 150/150 | present |
| `w1-qual-fw-20261110c` | FIWARE | `INVALID_VU_POOL_BOUND` | 65 | 149/150 | present |
| `w1-qual-ex-20261110c` | EdgeX | **not run** (see §6) | — | — | none |

Two earlier generations were consumed and preserved rather than reused:

| Run ID | Outcome | Why |
|---|---|---|
| `w1-qual-tb-20261110` | `NOT_ASSESSED_PREFLIGHT_REFUSAL` | stack-isolation guard refused; 0 offered |
| `w1-qual-tb-20261110b` | `NOT_ASSESSED_HARNESS_TRANSPORT_TIMEOUT` | one SSH poll through the jump exceeded its socket timeout while TB was healthy (`login=200`); 0 offered |

The transport timeout was a real harness defect and was fixed narrowly in the W
launcher only (the frozen B0 launcher was not touched): the readiness poll now
retries a `TimeoutExpired` inside the same deadline, and still fails closed if the
deadline expires. Covered by regression tests.

## 3. Why W1 fails, in one table

The pool binds when `rate x latency > 150`, i.e. when latency exceeds
`150 / rate`. At 50/s that threshold is **3.0 s**.

| Platform | warm-up latency vs 3.0 s | Drops | Where | Measurement |
|---|---|---:|---|---|
| ThingsBoard | p99 4.86 s, max 5.00 s | 134 | whole warm-up, immediate | clean: 30,000/30,000, p99 212 ms |
| FIWARE | peak p99 5.00 s | 65 | single 30 s window at t=210–240 s | clean: 30,000/30,000, p99 1,022 ms |

Both platforms are fine in steady state. The failure is confined to warm-up on
both, and neither failed in measurement.

**The mechanisms differ, and the FIWARE one is not explained.** ThingsBoard bound
immediately (send lag 2,682 ms at p50 from the first minute). FIWARE bound
transiently (send lag p50 stayed at 2 ms; one 30 s stall pushed response p50 to
2,347 ms). All FIWARE containers show `RestartCount 0` and `OOMKilled false`; no
restart occurred. **The trigger is recorded as unidentified, not guessed.**

## 4. The pool was a frozen policy — now amended (superseded)

> **This section described the state before the 2026-11-10 amendment.** The pool
> was frozen in `dispatch-integrity.py` and could not be raised as tuning. It has
> since been replaced by a rate-derived rule under a recorded policy revision
> (`generator-pool-amendment-20261110.md`): 150 at 25/s (B0-identical), 300 at 50/s.

`scripts/dispatch-integrity.py` used to raise `frozen VU allocation is 150/150` on
any deviation. Raising it changes frozen workload policy and B0 comparability, so
it was escalated rather than tuned. That escalation was answered: the rule is now
derived from `rate x timeout` with 10% headroom, chosen so the 25/s value is
unchanged.

Three honest options, all requiring owner input:

1. **Report W1 as not-qualifiable under frozen policy** on both platforms. Do not
   launch. Conservative and evidence-faithful.
2. **Amend the pool** to `>= rate x timeout` under a recorded policy revision,
   then re-qualify under new generation IDs.
3. **Lower W1's rate** to <=30/s so `rate x 5 s <= 150`. This redefines the
   experiment and must be explicit.

Re-running the same 50/s plan against the same 150 pool is not one of the options;
it is a foregone INVALID.

## 5. A second, separate gate ambiguity

`qualification-gates.json` `other_configuration_headroom` says:

> "Qualify applicable 50/s headroom for D=2000, padding=1024 or N=1/k=2..."

But `scope.json` specifies W2/W3/W4 at **25 messages/second**, not 50/s. At 25/s
the pool cannot bind (25 x 5.0 s = 125 < 150), so under the old fixed pool the
literal 50/s clause was either inert or unresolvable depending on how it is read:

- **If it means "the rate those variants run at"** → inert; the real gates are
  D=2000 provisioning, padding=1024 fidelity, and N=1/k=2 cpusets.
- **If it means "50/s evidence for those configurations"** → under the old fixed
  pool this was unresolvable; **under the amended rate-derived pool 50/s is now
  qualifiable (300 VUs)**, so this reading is no longer a hard block, just extra
  qualification work.

`scope.json` and all 60 frozen plans (25/s) still contradict the literal wording,
so **the reading remains an owner decision** — it is just no longer fatal. See
`research/findings/w1-headroom-gate-is-unresolvable.md`.

## 6. EdgeX — documented, left open as instructed

Not run. Two independent blockers, both measured previously:

- **Blocker B (binds first):** `wait_driver_ready()` needs the POST-only route to
  return 405, which happens only after `device-sdk-go` `InitCache` completes. That
  path is O(N) sequential profile fetches; at 10,192 retained devices it is
  ≈489 s against a **180 s** deadline (2.7x over, projected — measured only at
  N=10,192).
- **Blocker A (binds from rep 2):** the 16,384 `MaxResultCount` cap.

W1 rep 1 would fit the inventory cap by +192, but not the readiness deadline, so
rep 1 fails at blocker B before inventory is even consulted. This is why a cap
bump alone cannot help.

Three options are laid out in `research/findings/edgex-w-batch-blocker.md`;
recommendation there is **Option 2 (run EdgeX out of this batch) now, Option 1
(cap bump + init fix, frozen batch-wide) later.** Left for the owner's final
decision, per instruction.

## 7. What is genuinely still actionable

| Item | State | Blocker |
|---|---|---|
| W1 TB / FIWARE attempts | **not launchable** | frozen 150/150 pool |
| W2 all platforms | not attempted | D=2000 provisioning gate + §5 ambiguity |
| W3 FIWARE | not attempted | Orion subscription omits `padding`; no evidence path |
| W3 TB | not attempted | §5 ambiguity resolved either way |
| W4 all platforms | not attempted | N=1/k=2 effective-cpuset verification |
| EdgeX W1–W4 | not run | blockers A/B, owner decision |

The only work that can advance without an owner decision is **preparation**:
staging plans and fixing harness defects. Launching more W1 50/s qualifications
would spend IDs on a foregone result.

## 8. Harness fixes made in this stretch

- **Readiness poll resilience** (`scripts/run-workload-campaign.py`): transient
  SSH timeouts retried within the deadline; still fails closed. 3 regression tests.
- **Consumed-ID guard** (`locate()`): an already-consumed qualification ID is
  refused and reports its successor. Prevents silent evidence overwrite.
- **Evidence-derived index status** (`stage-w1-qualification.py --refresh`): row
  status comes from the run's own result record, not from what the generator hoped.
  `--check` now validates real invariants (plan shape unchanged, status agrees with
  evidence, consumed rows keep their evidence) and was proven to catch a tampered
  row claiming a consumed ID was relaunchable.
- **Aggregator qualification bucketing** (`scripts/aggregate-runs.py`):
  qualifications are their own `W1-QUALIFICATION` workload and never pool with W1
  attempts; `load_results_only` now sets `workload_id`. Verified **0 substantive
  changes** to existing B0 rows.
- **Stale lock recovery**: both qualification runs retained
  `/run/iot-bench-workload.lock` by design (analysis failure skips `finish`). Both
  were released after verified ownership; one `lock-recovery.json` per run.

All 19 test modules pass.

## 9. Corrections to earlier documents

- **"The load host is down"** — false. `ramonavm1` is reachable via
  `ProxyJump ramonavm2`; direct connections fail. It was measured, not assumed.
- **"The VU pool is a minor caveat / only a TB risk"** — false twice over. It is
  the binding gate, and it binds on FIWARE too. The earlier "FIWARE passes"
  inference came from B0's **30 s** qualification (peak 56 VUs) — the exact
  30 s-warm-up trap the analysis warns about.
- **`vus_max_allocated: 300`** in summaries is the sum of two 150-VU scenario
  pools, not available capacity. `vus_peak` is the real number.

## 10. Decisions needed from the owner

1. ~~**W1 under frozen pool:** accept the negative, amend the pool, or lower the
   rate.~~ **DECIDED 2026-11-10 — the pool was amended** to a rate-derived rule
   (`generator-pool-amendment-20261110.md`). ThingsBoard re-qualified `VALID` with
   0 drops. Still outstanding: re-qualify FIWARE and settle EdgeX.
2. **`other_configuration_headroom`:** which reading of the 50/s clause is
   authoritative for W2–W4. No longer fatal (50/s is now qualifiable) but still
   ambiguous against the frozen 25/s plans.
3. **EdgeX:** Option 1, 2, or 3 — and whether W1 rep 1 should run as a labelled
   pilot (it will fail at blocker B; that is expected, not informative).
4. **The batch release itself:** all 60 rows remain `BLOCKED_PENDING_VARIANT_RELEASE`
   with `execution_authorized: false`. The pool amendment clears one blocker; it is
   not a release.

No proposed attempt is consumed while these are open.

## Related

- `research/findings/w1-vu-pool-binds-both-platforms.md` — full measurement reports
- `research/findings/w1-vu-pool-headroom-risk.md` — the advance prediction and tree
- `research/findings/w1-headroom-gate-is-unresolvable.md` — the structural argument
- `research/findings/edgex-w-batch-blocker.md` — EdgeX gates and options
- `workloads/campaigns/fixed-points-w1-w4/qualification/index.json` — consumed IDs
