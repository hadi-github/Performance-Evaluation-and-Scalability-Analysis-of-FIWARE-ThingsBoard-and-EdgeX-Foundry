# Handoff verification — 2026-10-25

**Status: review packet only.** No decision was adopted, no platform was changed or tuned, no experiment was run,
no raw evidence was edited, and no run output was committed. This file verifies the 25 October handoff against
this checkout. It does not create D27–D30; those remain proposals.

- Checkout: `73dea4d` (2026-10-22 23:10 +0330), working tree clean before this file.
- Decision source of truth: `docs/design-decisions.md` (D1–D26). Nothing after D26 is a log entry.
- Evidence root: `results/**` (present locally; **gitignored** — only `EXPL-tb-k4b.amendment.json` is tracked).
- Remote access attempted from the workstation; see §4. No remote state was changed.

## 1. Verification matrix

| # | Handoff claim | Verdict | Evidence |
|---|---|---|---|
| 1 | Repository contains benchctl, k6 generator, adapters, sink, fault fixture, validity gates, reconciliation, offline verdicts, resource pinning, exploratory plans, diagnostics scripts | **Confirmed** | `cmd/benchctl`, `workloads/k6/gen.js`, `workloads/k6/lib/adapters.js`, `internal/sink` + `cmd/sink`, `internal/fixture` + `cmd/fixture-target`, `internal/report`, `internal/reconcile`, `scripts/resources.mk`, `scripts/check-resources.sh`, `workloads/plans/` (39 plans), `scripts/diag/` |
| 2 | Exploratory evidence and diagnostics round 1 complete | **Confirmed as a checkpoint, not as a closed question** | `docs/exploratory-capacity.md` l.3 ("exploratory… Not paper results"); `docs/diagnostics.md` l.3 ("checkpoint before any tuning"); 30 run manifests (see §2); diagnostics keeps an 8-item "Could not verify" list (l.315–326) |
| 3 | No tuning was applied | **Confirmed for Step 5 / D27–D30 and for durability**, with two qualifications (below) | `docs/diagnostics.md` l.294 ("nothing applied"); `synchronous_commit` absent from all main compose files and only in `edgex/diag/compose.diag-nosync.yaml`; proposed TB `/config` mount absent from `thingsboard/compose.yaml` (only the transport mounts `/config`, l.225); proposed FIWARE device index absent from `fiware/mongo/init-indexes.js`; `ORION_REQ_POOL_SIZE` defaults to 0, not the proposed 4 (`fiware/compose.yaml` l.80–81) |
| 4 | Step 5 awaits owner approval | **Confirmed** | `docs/diagnostics.md` l.3, l.294; the proposal table and the four proposed entries are the approval object |
| 5 | D27–D28 are proposed Category A fixes | **Partly confirmed** | Verified wording in §3. They are **proposals in `docs/diagnostics.md`, not entries in the decision log**. D27 is labelled A; D28 is labelled "A (defect fix)". The same table contains additional Category A candidates **not** covered by D27–D28 (Orion `-reqPoolSize 4`, ThingsBoard queue poll intervals (A? needs owner call), EdgeX more core-data replicas (A topology)) |
| 6 | D29 is the EdgeX commit-latency question | **Confirmed** | Exact wording in §3; source `docs/diagnostics.md` l.312; evidence in Step 3 EdgeX and H-E1 |
| 7 | D30 is the observation-point decision | **Confirmed, but its referent is missing** | Exact wording in §3; it cites "the README table in Step 5", and no such table exists in `README.md` or `docs/` (only D23's goodput rule mentions observation points) |
| 8 | FIWARE 4.5.0 / EdgeX 4.0.2 / ThingsBoard 4.3.1.6 | **Confirmed, with a precision correction** | `fiware/.env.example` l.2–3: `ORION_VERSION=4.5.0`, `IOTA_VERSION=3.7.0`, `MONGO_VERSION=8.0` — 4.5.0 is the **Orion** version, not a FIWARE platform version. `edgex/.env.example` l.2–3: `EDGEX_VERSION=4.0.2`, `EDGEX_DEVICE_REST_VERSION=4.0.2` (built from official v4.0.2 source with `include_nats_messaging`). `thingsboard/.env.example` l.2: `TB_VERSION=4.3.1.6` |
| 9 | FIWARE has no historical time-series audit (D9) | **Confirmed, D9 still open** | `docs/design-decisions.md` l.17 (D9, "open (M4)"); `fiware/compose.yaml` services are only `mongo`, `mongo-init`, `orion`, `iot-agent`, `haproxy` — no QuantumLeap/CrateDB/timescale; `README.md` §Known limitations and `docs/generator.md` l.42 ("latest value only") |
| 10 | ThingsBoard k=4 does not fit the current memory budget (D11) | **Confirmed for the viable configuration; wording is imprecise** | `docs/design-decisions.md` l.19 (D11, open); tracked defaults 1 GiB/1 GiB/512 MiB (`thingsboard/.env.example` l.20–24); exploratory override 1536/1536/768 MiB (`results/exploratory/EXPL-tb-k4b.amendment.json`, `workloads/plans/expl-thingsboard-k*.json` deviations). Arithmetic in §2 |
| 11 | Generator qualification limits | **Confirmed** | `docs/calibration.md` l.51 ("~1,000 req/s on 2 cores, ~1,500–2,000 req/s on 3 cores"); l.110–111 (Poisson ~500/s); D21, D22, D24 in `docs/design-decisions.md` (D24 defers the Go generator, reopen only if tuned capacity > 450 msg/s) |
| 12 | One-shot pilot evidence without confidence intervals | **Confirmed structurally** | 39/39 plans have `"pilot_only": true`; no repetition/replication field in `internal/plan/plan.go`; `docs/tooling-interfaces.md` l.16 ("no capacity, CPU-cost or statistical analysis") and l.86 ("run-level intervals" still planned); `docs/diagnostics.md` l.31 ("Each fit has only 2-4 points; R² with 2 points is 1 by construction"); no CI computation in `internal/explore` or `scripts/` |
| 13 | Retained run manifests | **Confirmed locally, not in git** | `find results -name manifest.json` = 30; `git ls-files results` = 1 amendment file only. A fresh clone does not contain the run evidence |

## 2. Evidence counts and memory arithmetic

Run manifests present in this working copy:

| Directory | VALID | INVALID | Note |
|---|---|---|---|
| `results/diag` | 13 | 2 | `DIAG-fiware-k4-r100-cpuprof2`, `DIAG-tb-obs-sink-r100`; both explained in `docs/diagnostics.md` |
| `results/exploratory` | 8 | 6 | generator or platform failures, explained in `docs/exploratory-capacity.md` |
| `results/runs` | 1 | 0 | fixture smoke |
| **total** | **22** | **8** | 30 run directories |

The DIAG *run index* lists 17 ids, but only 15 have `manifest.json` (the two `DIAG-tb-idle*` entries are not
`benchctl` runs).

ThingsBoard at N=4 (MiB, limits × replicas, excluding the one-shot `install` profile):

- app tier, tracked defaults: 4 × (1024 + 1024 + 512) = 10,240
- infra: 1536 (Postgres) + 1280 (Kafka) + 512 (ZooKeeper) + 256 (Valkey) + 256 (HAProxy) = 3,840
- total 14,080 MiB = 13.75 GiB; budget 16 GiB − 1,536 MiB = 14,848 MiB
- app tier, exploratory override: 4 × (1536 + 1536 + 768) = 15,360; total 19,200 MiB = 18.75 GiB → over 16 GiB

So: **with the exploratory values, k=4 cannot fit at all**; **with the tracked defaults it fits the budget only
on paper**, and D11's operational fact is that tb-core was OOM-killed at 1 GiB during provisioning. The handoff's
phrase "current memory budget" should be read as "the budget that makes k=4 viable", not as "the tracked defaults
are over budget".

## 3. D27–D30: exact repository wording and status

These IDs are **not** in `docs/design-decisions.md`. The only repository wording is
`docs/diagnostics.md` l.308–313 ("Proposed decision-log entries"), with the proposed fixes in the table at
l.296–306. Nothing may be inferred from diagnostic evidence as a decision.

- **D27 (proposed):** FIWARE IoT Agent registry index = upstream 4.8.0 fix applied by our init script (Category A),
  recorded as a deviation in every later FIWARE run.
  - Proposed fix: create `{service:1, subservice:1, apikey:1, id:1}` (unique) in `fiware/mongo/init-indexes.js`.
  - Cited source: iotagent-node-lib 4.8.0 `lib/model/Device.js:64` and change log (#1576); not in any
    iotagent-ul release yet.
  - Gate question: the cited source is **upstream source/changelog, not the platform's official documentation**.
    Under the prompt's stricter Category A rule, the owner must decide whether this qualifies; under D25 it is a
    "fix for a defect in our deployment". Not decided here.
- **D28 (proposed):** ThingsBoard tb-core/tb-rule-engine mount the official `docker/tb-node/conf` (deployment defect).
  - Proposed fix: mount `docker/tb-node/conf` (v4.3.1.6) at `/config` for tb-core and tb-rule-engine.
  - Cited source: `docker/docker-compose.yml` (v4.3.1.6) l.42/92; `msa/tb-node/docker/start-tb-node.sh`.
  - Verified motivation: `docs/diagnostics.md` l.190 — `ExitOnOutOfMemoryError` is not set because our stack mounts
    `/config` only for the transport. Semantics unchanged → consistent with Category A.
- **D29 (proposed):** EdgeX persistence is bounded by WAL flush latency on the SUT disk; report the disk's write
  latency with every EdgeX result. Decide whether core-data replicas per core is a tuning knob (Category A) or a
  topology change.
  - This is a **question plus a reporting requirement**, not an approved change. The async-commit option
    (`synchronous_commit=off`) is Category B (durability) and exists only as `edgex/diag/compose.diag-nosync.yaml`
    + DIAG plans; D25 forbids it in `compose.yaml` / `.env.example`.
- **D30 (proposed):** Observation points per platform (ack / sink / storage), per the README table in Step 5.
  - **Blocked as written:** the referenced table does not exist in this checkout. Either the table must be written
    (documentation work) or D30's wording must be amended before approval.

Scope correction for Step 5: the proposal table is wider than D27–D28. It also proposes Orion `-reqPoolSize 4`
(A), ThingsBoard queue poll intervals (A?, explicitly "needs your call"), and EdgeX more core-data replicas at k=4
(A topology, changes replicas per tier). These are **not** covered by the handoff's D27–D28 summary.

## 4. Server reachability (attempted 2026-10-25)

| Check | Result |
|---|---|
| Local SSH client sanity | `github.com:22` returns an SSH banner and public-key denial — the workstation network/client works |
| `87.107.191.107:22` (SUT) | TCP connect completes; **no SSH banner**; `Connection timed out during banner exchange` |
| `87.107.191.32:22` (load host) | Same |
| `87.107.191.107:{1,22,8404,3000}` | All complete a TCP handshake (even port 1), no application data |
| `curl :8404/stats`, `:3000/login` | `curl: (52) Empty reply from server` |

**Conclusion: application-layer unreachable.** TCP handshake completion on arbitrary ports is not evidence that a
host is up (it is consistent with an intercepting middlebox). No remote command was executed, no service was
started/stopped, and the remote deployment state cannot be confirmed from here. The last *recorded* state
(`docs/diagnostics.md` l.328) is: no platform or collector containers on ramonavm2; ramonavm1 with monitoring only.

## 5. Governance findings

1. **The decision log is not strictly append-only in its git history.** Two rows were edited in place:
   - D1 corrected in `72a18f7` (before any dependent run; a factual correction, low risk).
   - **D11 edited in `c689191`, after `EXPL-tb-k4` had run and depended on it.** The edit added the exploratory
     OOM result and the user's memory decision. The run itself has a proper
     `results/exploratory/EXPL-tb-k4b.amendment.json`, but the decision row has no dated amendment.
   This is exactly the pattern the handoff rule forbids. Not repaired here (repairing requires an owner decision
   and would itself touch the log).
2. **D27–D30 are proposals, not decisions.** No dependent run exists, so creating amendments is not yet indicated.
   The owner should decide whether to adopt them into the log (D27, D28) or to amend their wording (D30).
3. **Category A definition tension.** The prompt limits Category A to a change supported by the platform's official
   documentation. D25 additionally permits "fixes for defects in our deployment". D28 is supported by the official
   compose/launcher and is semantics-neutral. D27 is supported by upstream code, not official docs — needs an
   explicit owner ruling, not a diagnostic inference.
4. **D30 is not actionable until its referenced table exists.** Do not approve it as written.
5. **Doc drift (unchanged by this review):** `docs/tooling-interfaces.md` l.13 says the sink is "missing" and
   `docs/metrics-dictionary.md` l.35 says "future Go sink", although `internal/sink` + `cmd/sink` exist and were
   used in 15 DIAG runs.
6. **"No tuning applied" needs its scope stated.** It is true for Step 5/D27–D30 and for durability knobs. It is
   not a claim that the stacks are stock: pre-D25 platform choices are documented in the compose headers (e.g.
   Orion `-reqMutexPolicy none`, Orion entity indexes, core NATS, locally built NATS images), and the exploratory
   runs used the D11 memory override (1536/1536/768 MiB) recorded only in the amendment and plan deviations.

## 6. Gate status

| Gate | Status |
|---|---|
| Handoff verification | **Passed with corrections** (version precision, D27–D30 status, D11 wording, D30 missing referent) |
| Step 5 (tuning) | **Closed — pending owner approval.** Nothing applied; no run may start without it |
| Category A (D27) | **Open question** — source is upstream code, not official documentation |
| Category A (D28) | **Ready for owner approval** — official source, semantics unchanged |
| D29 | **Open decision** — reporting change + A-vs-topology question; Category B async option stays DIAG-only |
| D30 | **Blocked** — referenced observation-point table does not exist |
| Remote verification | **Blocked by network** — no SSH/HTTP application response from either host |

## 7. Exact next owner action

1. Rule on D27: does an upstream code/changelog fix (iotagent-node-lib 4.8.0 index) satisfy the Category A rule,
   or must it be treated as a deployment-defect fix under D25(b)? If approved, authorize applying it to
   `fiware/mongo/init-indexes.js` as a recorded deviation.
2. Approve or reject D28 (mount the official tb-node `/config` for tb-core and tb-rule-engine).
3. Decide D29: is "more core-data replicas than pool cores" a Category A/topology change to run in Step 5, and is
   the per-result disk-write-latency report required? Keep `synchronous_commit=off` DIAG-only.
4. Resolve D30: either authorize writing the missing ack/sink/storage observation-point table (documentation only),
   or amend D30's wording before adoption.
5. Decide the Step 5 scope: the proposal table also contains Orion `-reqPoolSize 4`, ThingsBoard poll intervals,
   and EdgeX replica topology — approve or explicitly exclude each.
6. Decide whether to add a dated amendment recording the in-place D11 edit (append-only remediation), and whether
   D1's earlier correction also needs one.
7. If remote verification is required, restore access to `87.107.191.107` / `87.107.191.32` (the workstation can
   reach other SSH hosts) and then re-run the read-only checks (`docker ps`, `cat /root/iot-bench-deploy/GIT_REV`,
   platform versions). Not attempted further; no state was changed.

## 8. What this review did not do

No platform start/stop, no `make up`, no tuning, no settings change, no acceptance-threshold change, no data or
volume deletion, no credential rotation, no experiment, no run, no decision-log edit, no commit, no push, no
external contact. No secret was read or printed.
