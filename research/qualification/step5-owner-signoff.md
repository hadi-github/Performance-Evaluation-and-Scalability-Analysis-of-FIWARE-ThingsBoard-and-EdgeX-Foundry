# Step 5 owner sign-off and experimental scope freeze

**Status: DECISIONS RECORDED 2026-10-26 — see §0. The proposal text of revision 1 is preserved.**

Prepared 2026-10-26 from repository state `73dea4d5493280125c7d330356a90433d78889d2`. The working tree already
contained five modified documentation files (`README.md`, `docs/methodology/generator-status.md`, `docs/methodology/metrics-dictionary.md`,
`docs/development/tooling-interfaces.md`, `monitoring/README.md`) and three untracked files (`evidence/reproducibility.md`,
`docs/handoff-review-2026-10-25.md`, `docs/qualification/step5-decision-packet.md`) before this sign-off was written.

**Revision 2 (2026-10-26).** The project owner delegated selection of the next decisions to ChatGPT on 2026-10-26.
The selected dispositions and authorization levels are recorded in §0 and in the append-only entries D27–D30,
D25-a, D11-a, D31–D41 of `docs/design-decisions.md`. The selections were made by ChatGPT under that delegation;
the owner did not manually tick the boxes in §8, and no initials or signatures are forged. Revision 1's proposal
text remains below as the historical record; §0 is the operative decision record.

No Deploy or Runs level was authorized for any Step 5 candidate. The Step 5 packet itself remains a proposal
document. Nothing in this revision deploys, restarts, tunes, runs or commits anything.

Evidence labels used throughout:

- **[V]** verified in this checkout, in retained evidence, or in a pinned upstream source named in the text.
- **[P]** proposal, recommendation or plan; not adopted.
- **[G]** unverified assumption, estimate or gap; must not be reported as a result.

A recommendation in this document, an unchecked box, a blank field, or the approval of an adjacent row is **not**
owner approval. Only the specific authorization levels recorded in §0 and in the decision log authorize work.

---

## 0. Recorded decisions (2026-10-26)

Provenance: **selected by ChatGPT under the owner's explicit delegation dated 2026-10-26.** This is not a manual
owner signature and must not be described as one.

| ID | Disposition recorded | Text | Impl/checks | Deploy | Runs | Decision-log entry |
|---|---|---|---|---|---|---|
| D30 | APPROVED — corrected observation-point table; unsupported-claims column normative | Yes | — | — | — | D30 |
| D28 | APPROVED — official configuration with writable log volume; no memory-limit change; no OOM-recovery claim | Yes | Yes | No | No | D28 |
| D29a | APPROVED — EdgeX disk-latency reporting | Yes | Yes | No | No | D29a |
| D25-a | APPROVED — narrow upstream-backport clarification | Yes | — | — | — | D25-a |
| D27 | APPROVED — conditional FIWARE unique-index backport as an explicit A-equivalent exception | Yes | Yes | No | No | D27 |
| D11-a | APPROVED — historical D11 addendum | Yes | — | — | — | D11-a |
| D1-a | NO CHANGE | — | — | — | — | none (no change) |
| D29b | DEFERRED — extra EdgeX replicas | — | — | — | — | none (deferred) |
| D29c | KEPT DIAG-ONLY — asynchronous commit | No change | — | — | — | none (no change) |
| D29d | DEFERRED — `commit_delay` experimentation | — | — | — | — | none (deferred) |
| Orion `-reqPoolSize` | DEFERRED | — | — | — | — | none (deferred) |
| ThingsBoard queue intervals | DEFERRED | — | — | — | — | none (deferred) |

Research-critical selections recorded in the same delegation:

| ID | Selection | Decision-log entry |
|---|---|---|
| D9 | QuantumLeap + TimescaleDB, matching the pre-registered architecture; local design only | D31 |
| D11 | Feasibility-first k=1/k=2; budget retained; k=4 unqualified; fallback conditional | D32 |
| Measurement correctness | Minimum integrity set + conservative reporting restrictions until it qualifies | D33 |
| Generator qualification | Strict 2× headroom; k6 retained; reopen D24 if k6 fails | D34 |
| S1 | D26 semantics: k = shared app-tier CPU budget; replicas reported separately | D35 |
| S2 | Correctness-qualified goodput clarified without rewriting D23 | D36 |
| S3 | 2× headroom; D24's 1.5× reference clarified, not erased | D37 |
| S4 | Target k ∈ {1,2,4}; k ∈ {1,2,3} only as a measured, separately recorded deviation | D38 |
| S5 | Original main protocol preserved; exploratory durations stay exploratory | D39 |
| Pre-registration | Read-only archive with hashes; differences documented separately | D40 |
| Provenance | Delegated selection recorded; no manual signature implied | D41 |

**Deploy and Runs: not authorized for any Step 5 candidate.** Every row above is Text and/or local Impl/checks only.
The sign-off's §8.1 blank sheet is retained as revision 1's proposal; §8.3 records the actual, limited authorizations.

---

## 1. Executive decision matrix

Every level is a separate authorization. Ticking one does not tick the next. The scope of each level is defined in
the packet §9.2–§9.4 and is reproduced for each candidate in §4 below.

| ID | Candidate | Recommended disposition (not an approval) | Text | Impl/checks | Deploy | Runs | Gate role |
|---|---|---|---|---|---|---|---|
| D27 | FIWARE IoT Agent device-registry index | Approve as explicit A-equivalent upstream-backport exception, conditional on D25-a and a clean duplicate/provisioning check | `[ ]` | `[ ]` | `[ ]` | `[ ]` | FIWARE deployment correctness; not history |
| D28 | ThingsBoard official `/config` mounts and logging | Approve as Category A deployment-defect fix; choose official conf + log volume or reviewed stdout-only logback | `[ ]` | `[ ]` | `[ ]` | `[ ]` | ThingsBoard deployment correctness |
| D29a | EdgeX disk-write-latency reporting | Approve reporting only; no verdict or platform change | `[ ]` | `[ ]` | — | — | EdgeX bottleneck evidence |
| D29b | Extra EdgeX core-data replicas as topology ablation | Defer; diagnose only if the owner wants a labelled topology ablation | `[ ]` | `[ ]` | `[ ]` | `[ ]` | Optional diagnosis; not qualification |
| D29c | Preserve `synchronous_commit=off` as diagnostics-only | Keep DIAG-only; reject for qualification | `[ ]` (no change) | — | — | — | Durability guardrail |
| D29d | Separate durable `commit_delay` experiment | Defer separately; do not conflate with D29c | `[ ]` | `[ ]` | `[ ]` | `[ ]` | Optional diagnosis |
| D30 | Corrected ack/sink/storage observation-point table | Approve the corrected table; unsupported-claims column normative | `[ ]` | — | — | — | Reporting and claim discipline |
| — | Orion `-reqPoolSize` tuning | Defer; 4 per replica is an unvalidated aggregate-oversubscription choice | `[ ]` | `[ ]` | `[ ]` | `[ ]` | Optional tuning |
| — | ThingsBoard queue poll intervals | Defer; requires a Category A ruling plus a named A/B | `[ ]` | `[ ]` | `[ ]` | `[ ]` | Optional tuning |
| D25-a | Clarify upstream-backport authorization | Approve if D27 is approved; text only | `[ ]` | — | — | — | Enables an honest D27 |
| D11-a | Record the historical D11 edit | Approve the append-only addendum | `[ ]` | — | — | — | Governance |
| D1-a | Optional historical D1 correction note | No change required; optional addendum | `[ ]` | — | — | — | Governance |

### 1.1 Research-critical decisions outside the Step 5 packet

These are not D27–D30 candidates, but they block a publication-grade run more directly than optional tuning does.
The options are listed without preselecting one.

| ID | Decision | Options (mark one only if the owner decides) | Recommended path (not an approval) | Blocks |
|---|---|---|---|---|
| D9 | FIWARE historical persistence | `[ ]` Add QuantumLeap + TimescaleDB as planned · `[ ]` Alternative durable history writer · `[ ]` Defer FIWARE and exclude it from cross-platform claims · `[ ]` Stop | Add QuantumLeap + TimescaleDB, validate `(run_id, seq)` row independence, count it in the supporting tier | FIWARE qualification; cross-platform durability and capacity claims |
| D11 | ThingsBoard four-core feasibility | `[ ]` Prove a k=4, N=4 configuration inside 16 GiB · `[ ]` Use k ∈ {1,2,3} for ThingsBoard and report the deviation · `[ ]` Keep k=4 with fewer replicas · `[ ]` Larger SUT · `[ ]` Defer ThingsBoard scaling | Authorise a bounded k=1/k=2 memory-feasibility check first; do not call k=4 qualified | ThingsBoard E(4), scaling claims and part of the fingerprint |
| M | Measurement correctness | `[ ]` Implement ack→sink identity join, clock-offset qualification and TB collision audit · `[ ]` Restrict claims and run · `[ ]` Both | Both: implement the joins and keep the claim restrictions until validated | Delivery, durability, cross-host latency and loss claims |
| G | Generator qualification | `[ ]` Qualify 2-core and 3-core layouts with sink and monitoring · `[ ]` Cap experiments to qualified rates · `[ ]` Build the Go generator | Qualify the layouts and cap rates; build the Go generator only if a planned rate exceeds the qualified 2× headroom | High-rate, Poisson and overload experiments |
| S | Spec/repository interpretation | `[ ]` k means shared cores and replicas are reported separately · `[ ]` Restore one exclusive core per replica · `[ ]` Other | Confirm the k-versus-N interpretation before the first run | E(N), E(2), E(4) and every scaling statement |
| H | Headroom rule | `[ ]` 2× governs · `[ ]` D24's 1.5× governs · `[ ]` Other | 2× governs; amend D24 explicitly if the owner chooses otherwise | Generator qualification and maximum planned rate |

---

## 2. Minimum decisions required to unblock the next implementation milestone

The next milestone is **not a run**. It is the set of local, non-deploying changes that make later runs defensible.
Priority is correctness and reproducibility over optional throughput tuning.

1. **D30 Text.** Adopt the corrected observation-point table. Every later report depends on naming its ack, sink
   and storage points and on not claiming an ack→sink join that does not exist.
2. **D28 Text + Impl/checks.** Approve the Category A deployment-defect fix and vendor the pinned configuration
   locally. This is the lowest-risk correction with direct evidence of a broken-OOM behaviour.
3. **D29a Text + Impl/checks.** Adopt the EdgeX disk-write reporting fields. It changes no platform and no verdict.
4. **D25-a Text + D27 Text + Impl/checks.** Approve the backport exception and implement the index locally, with a
   duplicate audit that stops and reports. Deploy is a later, separate tick and requires a clean audit.
5. **D11-a Text.** Record the historical D11 edit. D1-a Text is optional and changes nothing substantive.
6. **Authorize the local measurement-integrity work** (not a packet item): ack→sink identity join, clock-offset
   qualification design, and the ThingsBoard same-millisecond collision audit. These are implementation gaps, not
   tuning choices, and they do not touch the SUT.
7. **D9 decision in principle.** Approve the history architecture or an alternative now, because it is the longest
   lead item and it changes the FIWARE platform definition.
8. **D11 feasibility decision.** Authorise a bounded memory-feasibility check for ThingsBoard and decide the k-set
   consequence before any ThingsBoard scaling run.
9. **Specification pinning.** Approve a read-only copy of the pre-registered research questions, SLO profile and
   fingerprint definitions inside the repository (currently they live only in the external research notes).

Optional tuning is **not** in the minimum set: D29b, D29d, Orion `-reqPoolSize` and ThingsBoard queue intervals can
remain deferred without blocking a valid k=1 pilot.

## 3. Current state and evidence base

| Item | State | Consequence for this sign-off |
|---|---|---|
| Git HEAD | `73dea4d` [V] | This document is written against that checkout. |
| Working tree | Five modified docs and three untracked docs, as listed in the header [V] | Preserved. No existing change was edited. |
| Decision log | D1–D26 only; D11 and D9 still open [V] | Nothing after D26 is a log entry. D27–D30 remain proposals. |
| Step 5 packet | 806 lines, status PROPOSED [V] | The packet is authoritative for the four-level model and §9.2–§9.4 definitions. |
| Evidence | `results/**` is gitignored except one tracked amendment; 30 run directories, 22 VALID and 8 INVALID [V] | A fresh clone does not contain the raw evidence. The evidence package is a stage-2 prerequisite. |
| Pre-registered research questions | Present in `~/catStuff/wrk/iot-performance-evaluation/research/notes/rqs.md`, outside the repository [V] | The repository does not contain the SLO/fingerprint text. It must be pinned before a publication-grade run. |
| Remote SUT/load hosts | Last attempt on 2026-10-25 was application-unreachable; no banner or HTTP data [V, per handoff] | Remote deployment state cannot be confirmed from this checkout and was not touched. |
| FIWARE history | No QuantumLeap, TimescaleDB or other historical store; `scripts/history-ids.sh fiware` exits 3 [V] | FIWARE cannot yet satisfy an independent history verification. |
| ThingsBoard k=4 | Only a 4-core pool with **one replica per tier** ran; four replicas at k=4 never ran [V] | Do not describe `EXPL-tb-k4b` as a four-replica result. |
| Ack→sink identity join | Not implemented; sink figures are counts from NDJSON [V] | Any per-message delivery claim is unsupported. |
| Clock offset | No measured ramonavm1↔ramonavm2 offset [G] | Cross-host latency is unqualified. |
| Generator+sink layout | A 3-generator-core layout with sink and monitoring together has not been measured [V] | The 2× headroom gate is not qualified. |

### 3.1 What is already adopted (and therefore not reopened here)

D1–D26 are the decision log. This sign-off does not rewrite them. In particular:

- **D23** defines goodput as durably stored and delivered, evaluated at persistence for EdgeX, with the sink kept
  as the latency observation point. This is already adopted. The sign-off preserves the verdict rule and narrows
  only the **claim** that the evidence can support (§4.7 and §5.3).
- **D24** defers the Go generator and reopens it if a tuned capacity exceeds 450 msg/s. This sign-off treats the
  prompt's 2× headroom rule as stricter and flags the conflict; it does not amend D24.
- **D25** permits official-documentation tuning or deployment-defect fixes, with Category A meaning unchanged ack,
  durability, delivery and batching semantics. D25-a is the proposed clarification.
- **D26** defines k as the number of application cpuset cores and records replicas separately. This conflicts with
  the external research questions, which define N as application-tier replicas with one pinned core each. The
  conflict is listed in §6.4 (S1) and is not resolved silently.

---

## 4. Step 5 candidate decisions

Each candidate follows the same eight-part structure. The final level table is the **only** authorization record for
that candidate; everything before it is analysis. All boxes are unchecked.

### 4.1 D27 — FIWARE IoT Agent device-registry index

**1. Verified current behavior and evidence.**
[V] iotagent-ul 3.7.0 pins iotagent-node-lib 4.7.0, whose `lib/model/Device.js` declares no index. The 4.8.0 source
declares the unique index `{service, subservice, apikey, id}`; iotagent-ul 3.7.0 is the latest release, so no
released UL agent ships it. [V] The MongoDB profiler measured 1.98 `COLLSCAN` operations per message on
`iotagentul.devices`, examining about 2,501 documents per query at 1,000 provisioned devices. [V] The IoT Agent
carries about 50% of its busy time in mongoose/mongodb/bson frames; the binding pool fit is about 131 msg/s. [G] The
4.8.0 changelog/PR `#1576` is not independently verified; the code at the tag is.

**2. Recommended disposition.**
[P] **Approve as an explicit A-equivalent upstream-backport exception**, conditional on D25-a and on a clean
duplicate pre-check plus a passing provisioning check. It is not ordinary Category A under D25 as written, because
its source is upstream library code rather than the platform's official deployment documentation, and because the
application's `findOrCreate` path could turn duplicates into write errors.

**3. Alternatives and risks.**
[P] Alternatives: reject and leave the COLLSCAN in place; defer FIWARE qualification; amend D25 generally to admit
upstream backports; add a **non-unique** index (fixes the scan but is not the upstream fix and needs its own
approval); or change the agent's registry access path (larger, not proposed). Risks: index creation fails on
existing duplicate tuples; missing fields are treated as null by MongoDB; concurrent provisioning can raise
duplicate-key errors; results before and after the change are not directly comparable and must be recorded as a
deviation.

**4. Benefit and experimental impact.**
[P] Both per-message lookups become index lookups, so MongoDB CPU per message stops growing with the device count.
Capacity may rise toward the IoT Agent app-pool limit (about 131 msg/s), not without bound. It does not create
history, does not fix Node CPU, and does not replace D9. Every later FIWARE run must record the index as a
deviation.

**5. Dependencies and prerequisites.**
[P] D25-a Text or a one-off exception recorded in D27 Text. A duplicate audit over the exact key
`{service, subservice, apikey, id}` that **stops and reports** on conflict; no deletion, merge or identity rewrite.
A provisioning-path check after creation. D9 is independent for the index itself but is required before FIWARE
history can be claimed.

**6. Validation required after implementation.**
[P] Before creation: count duplicate groups and report offending identities; if any exist, stop. After creation:
`explain` both per-message lookups and confirm `IXSCAN`, no `COLLSCAN`; exercise the agent's create/update path and
check for duplicate-key errors or failed registrations; then run a fixed-load FIWARE smoke at the same offered rate
and k as the baseline, three repetitions, checking ack outcomes, collector errors, MongoDB CPU per message, and the
sink/history outcome. Record the index and result in the run manifest `deviations`. Until D9 is resolved, "history"
in that smoke is sink-only, not history.

**7. Rollback.**
[P] First revert or disable the index-creation step in the init path so a restart cannot recreate it. Then drop the
exact index identified by its key spec `{service:1, subservice:1, apikey:1, id:1}` (record the generated name), and
verify it is absent after restart. Do not delete or rewrite registry data.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Record D27 as an explicit A-equivalent backport; append D25-a only if D25-a Text is also ticked; no rewrite of D25. | `[ ]` |
| Impl/checks | Add the exact unique index to the init path for `iotagentul.devices`; add the stopping duplicate audit; run `explain` and the provisioning checks; add the deviation text. No other FIWARE change. | `[ ]` |
| Deploy | Run the init path on the SUT; restart the agent only as needed; verify the index and a clean start. No other FIWARE compose or `.env` change. | `[ ]` |
| Runs | Fixed-load FIWARE smoke at the same offered rate and k as the baseline, three repetitions; ack, errors, MongoDB CPU per message, sink/history; record the deviation. | `[ ]` |

### 4.2 D28 — ThingsBoard official configuration mounts and logging

**1. Verified current behavior and evidence.**
[V] `deploy/thingsboard/compose.yaml` mounts `/config:ro` only on `tb-http-transport` (line 225). `tb-core` and
`tb-rule-engine` run the image's built-in configuration. [V] The official v4.3.1.6 `docker/tb-node/conf/thingsboard.conf`
sets `ExitOnOutOfMemoryError`, and the official compose mounts that directory at `/config`. [V] In `EXPL-tb-k4b`
the rule engine raised `OutOfMemoryError: Java heap space` 74 times without restarting, and stalled with about
193,700 messages left in Kafka. [V] Effective heaps are 512 MiB in tb-core and tb-rule-engine, 256 MiB in the
transport; container limits in that run were 1536/1536/768 MiB. [G] No controlled OOM test exists; the claim that
the restart policy recovers the JVM is a source-based expectation.

**2. Recommended disposition.**
[P] **Approve as a Category A deployment-defect fix.** Choose **official conf + writable log volume** as the primary
variant, because it restores the official launcher behaviour (`ExitOnOutOfMemoryError`, GC logs, heap dumps, official
logback) and keeps the logs inspectable. A reviewed stdout-only logback is the lower-disk alternative.

**3. Alternatives and risks.**
[P] Alternatives: stdout-only logback (no `/var/log/thingsboard` volume, but must still vendor the official conf for
the JVM flags); reject or defer (the broken-OOM behaviour remains); mount the official conf read-only without a log
volume (official logback may fail to write). Risks: log volume disk growth; restart-on-OOM changes platform
behaviour under overload and must be reported; official flags may interact with our `JAVA_OPTS`; no controlled OOM
test proves recovery.

**4. Benefit and experimental impact.**
[P] An OOM exits and restarts instead of leaving a live but broken JVM. Logs become readable for diagnosis. It does
not increase heap, does not fix the k=4 memory feasibility problem, and does not change steady-state throughput. If
it changes OOM behaviour, that is a platform behaviour change and must be reported as a deviation.

**5. Dependencies and prerequisites.**
[P] None for the mount itself. D11 is independent. A controlled OOM test is a separate DIAG and is not authorized
by this row. The chosen logback/log-volume variant is an owner/implementation choice, not settled by the upstream
source.

**6. Validation required after implementation.**
[P] `docker compose config` and a hash check of the vendored files; verify the mounts on both services; verify
effective JVM flags include the official OOM flag; verify logs are written and readable; run a baseline-load smoke
checking ack, sink and storage. Do not claim OOM recovery from a smoke.

**7. Rollback.**
[P] Remove the `/config` mounts and the log volume, restore the prior logging configuration, and restart tb-core and
tb-rule-engine. No stored telemetry is changed.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Record the chosen variant and the deviation. | `[ ]` |
| Impl/checks | Vendor the pinned v4.3.1.6 `thingsboard.conf` and `logback.xml`; mount `/config:ro` on tb-core and tb-rule-engine; add the chosen writable log volume or reviewed stdout-only logback; run `docker compose config` and hash checks. | `[ ]` |
| Deploy | Restart tb-core and tb-rule-engine; verify mounts, effective JVM flags and readable logs. | `[ ]` |
| Runs | Baseline-load smoke checking ack, sink and storage. A controlled OOM test is a separate DIAG and is not authorized by this row. | `[ ]` |

### 4.3 D29a — EdgeX disk-write-latency reporting

**1. Verified current behavior and evidence.**
[V] EdgeX persists one serial transaction per event per replica; the durable run at k=4 persisted 402.0 events/s at
500 offered, with the WAL sync path busy 94% of the time, 4.17 ms per sync and 4.41 ms disk write await. [V] With
`synchronous_commit=off` (DIAG only) it persisted 494.5/s, with 14.66 ms disk await and a 3% WAL sync share. [V]
`docs/methodology/metrics-dictionary.md` reports disk activity only at host/device level; there is no per-transaction latency.

**2. Recommended disposition.**
[P] **Approve reporting.** Add `disk_write_await_ms` and `disk_writes_per_s` to every EdgeX result, and add WAL
`ms/sync`, `wal_sync_share` and `commits/s` only where the diagnostic pgstat overlay exists. No verdict change, no
main-compose change.

**3. Alternatives and risks.**
[P] Alternatives: report only disk await and writes per second (simpler, loses the WAL-share explanation); add a
per-transaction instrumentation (more accurate, more work, not official); reject (weakens the RQ-C bottleneck
evidence). Risks: host-level averages can be mistaken for per-transaction latency; the 5 s scrape and 10 s counter
resolution must be stated.

**4. Benefit and experimental impact.**
[P] Makes the EdgeX persistence limit visible next to the capacity verdict and supports the bottleneck analysis. It
does not change capacity, durability or any pass/fail rule.

**5. Dependencies and prerequisites.**
[P] None. It is independent of D29b, D29c and D29d.

**6. Validation required after implementation.**
[P] A desk check against the retained EdgeX DIAG runs: the fields are present and agree with the recorded evidence;
the verdict is unchanged. No new run is required.

**7. Rollback.**
[P] Revert the result/report-generation change. No platform or evidence is changed.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Adopt the reporting fields as requirements. | `[ ]` |
| Impl/checks | Extend EdgeX result/report generation with `disk_write_await_ms` and `disk_writes_per_s`; WAL fields only where the pgstat overlay exists; no verdict change. | `[ ]` |
| Deploy | — (no platform change; the existing exporter/diagnostic overlay is unchanged). | — |
| Runs | — (no dedicated run; the reporting applies to runs authorized elsewhere). | — |

### 4.4 D29b — Extra EdgeX core-data replicas as a topology ablation

**1. Verified current behavior and evidence.**
[V] One core-data replica persists one transaction per event with one goroutine; group commit at four replicas
shares a sync among about 1.8 commits, while a single replica cannot share. [V] The proposed topology is eight
core-data replicas on the existing four cores. No eight-replica run exists. Memory and connection costs are
estimates from compose limits, not measurements. [V] D26 records replicas separately from k, so this is a topology
change, not a k change.

**2. Recommended disposition.**
[P] **Defer.** If the owner wants it, approve only a separately labelled **topology ablation**, never a qualification
configuration.

**3. Alternatives and risks.**
[P] Alternatives: reject; run two replicas per core instead of eight; keep four replicas and accept the WAL ceiling.
Risks: eight Go services on four cores can oversubscribe CPU; memory and PostgreSQL connections grow; NATS queue
group distribution may change; the result is not comparable with the qualification configuration.

**4. Benefit and experimental impact.**
[P] Tests whether more concurrent commits raise persisted throughput at the same disk. It informs RQ-C. It changes
the replica topology, so it cannot be used as an E(N) point.

**5. Dependencies and prerequisites.**
[P] Owner acceptance of the topology label; the existing four-core pool; a recorded memory and connection budget;
no other platform change during the ablation.

**6. Validation required after implementation.**
[P] The labelled k=4 ablation only: record replicas, memory, connections, persisted rate, CPU and OOM events, and
compare with the four-replica baseline. Do not treat it as a qualification run.

**7. Rollback.**
[P] Restore the replica count and limits, restart, and verify the original topology.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Record the change as a labelled topology ablation, not a qualification configuration. | `[ ]` |
| Impl/checks | Compose replica and limit changes; record memory and connection budget. | `[ ]` |
| Deploy | Restart with eight core-data replicas on the existing four cores. | `[ ]` |
| Runs | The labelled k=4 ablation only; not a qualification run. | `[ ]` |

### 4.5 D29c — Preserve `synchronous_commit=off` as diagnostics-only

**1. Verified current behavior and evidence.**
[V] The DIAG runs show 402.0 → 494.5 persisted events/s at k=4 and 136.0 → 199.9 at k=1 when synchronous commit is
turned off. [V] D25 forbids durability-weakening settings in `compose.yaml` or `.env.example`; the async option exists
only as a diagnostic overlay. [V] Async commit can lose acknowledged rows on a crash.

**2. Recommended disposition.**
[P] **Keep DIAG-only; reject for qualification.** This is a disposition that records no change. It does not approve
a future D25 amendment or a crash test.

**3. Alternatives and risks.**
[P] Alternatives: reject entirely (loses the diagnostic explanation of the WAL ceiling); defer with no change;
approve a future amendment plus a crash test (not authorized here). Risk of any production use: acknowledged data
can be lost on a crash, which violates the durability requirement.

**4. Benefit and experimental impact.**
[P] The diagnostic result explains the mechanism without weakening the qualification configuration. It is not a
capacity result and must never appear as one.

**5. Dependencies and prerequisites.**
[P] None. A future amendment and crash test would be separate decisions.

**6. Validation required after implementation.**
[P] None; the existing DIAG evidence is unchanged. Do not re-run it as part of this row.

**7. Rollback.**
[P] Not applicable: no change is authorized.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | — (no change; no future-amendment approval). | — |
| Impl/checks | — | — |
| Deploy | — | — |
| Runs | — | — |

### 4.6 D29d — Separate durable `commit_delay` experiment

**1. Verified current behavior and evidence.**
[V] `commit_delay` is a PostgreSQL group-commit delay, distinct from `synchronous_commit=off`. No `commit_delay`
run exists. [G] Its interaction with the durable path and with crash behaviour is not measured here. It must not be
combined with D29c or used in the qualification configuration.

**2. Recommended disposition.**
[P] **Defer separately.** If approved, authorise only a separate durable group-commit test on a test-only overlay.

**3. Alternatives and risks.**
[P] Alternatives: reject; approve the separate durable test; combine with async commit (rejected, because it weakens
durability). Risks: confusing a latency improvement with a durability change; adding latency; an unmeasured crash
outcome.

**4. Benefit and experimental impact.**
[P] Tests whether group commit raises throughput without disabling durability. It is a diagnostic, not a
qualification configuration.

**5. Dependencies and prerequisites.**
[P] A test-only compose overlay, not the main compose or `.env`; a crash/kill test if durability is claimed.

**6. Validation required after implementation.**
[P] Measure latency and throughput on the test stack; if durability is claimed, include a crash/kill test that
shows the acknowledged rows survive. Record every changed variable.

**7. Rollback.**
[P] Remove the overlay and restart the test stack.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Record a separate durable group-commit test; no durability weakening. | `[ ]` |
| Impl/checks | Test-only compose overlay; not main compose or `.env`. | `[ ]` |
| Deploy | Only the test stack. | `[ ]` |
| Runs | The separate durable group-commit test, distinct from D29c. | `[ ]` |

### 4.7 D30 — Corrected acknowledgment, sink and storage observation-point table

**1. Verified current behavior and evidence.**
[V] The acknowledgement points differ: EdgeX at `device-rest`, ThingsBoard at the HTTP transport after the message
is accepted for asynchronous processing, FIWARE after the Orion entity write. [V] The EdgeX export app-service
subscribes upstream of core-data; the ThingsBoard sink runs after the Save Timeseries callback; FIWARE keeps only the
latest Orion state and has no history. [V] There is no implemented ack→sink identity join: sink figures are counts
from NDJSON, and `reconcile` joins only the ledger to stored history. [V] The ThingsBoard adapter sends no `ts`;
`JsonConverter` supplies `System.currentTimeMillis()` before queueing; `SqlInsertTsRepository` upserts on
`(entity_id, key, ts)`. [V] The D23 persistence series is a table-level, unfiltered insert counter. The packet §5
contains the corrected table, including the unsupported-claims column.

**2. Recommended disposition.**
[P] **Approve the corrected table.** Adopt packet §5 as the D30 text; the unsupported-claims column is normative for
all reports. Documentation only.

**3. Alternatives and risks.**
[P] Alternatives: defer (reports remain ambiguous about ack, delivery and storage); reject (no replacement text);
amend specific cells (owner may, but must not reintroduce a per-message commit or ack→sink claim). Risk: a table that
is treated as a guarantee rather than as a description of what the evidence can establish.

**4. Benefit and experimental impact.**
[P] Forces every report to name its observation point, clock and identity join, and prevents three recurring errors:
treating HTTP 200 as durability, treating a sink receipt as storage, and comparing ack latency across platforms as
if the ack points were the same. No runtime, durability or verdict change.

**5. Dependencies and prerequisites.**
[P] None. D30 does not imply approval of D29c or of any platform change.

**6. Validation required after implementation.**
[P] A desk check against one retained run per platform: every row must agree with the named evidence file, with
`README.md`'s ack statement, with `docs/methodology/metrics-dictionary.md` and with D23, and no cell may claim a per-message
commit time, an ack→sink identity join, or an unqualified cross-host latency.

**7. Rollback.**
[P] Revert the adopted text. No platform or evidence is changed.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Adopt packet §5 as the D30 text; the unsupported-claims column is normative. | `[ ]` |
| Impl/checks | — (documentation only). | — |
| Deploy | — | — |
| Runs | — (validation is a desk check against retained runs). | — |

### 4.8 Orion `-reqPoolSize` tuning

**1. Verified current behavior and evidence.**
[V] `ORION_REQ_POOL_SIZE` defaults to 0 in `deploy/fiware/compose.yaml` and `.env.example`; the diagnostic table suggests 4.
[V] Every Orion replica receives the **same** `${POOL_CPUSET}`; Docker restricts the allowed CPUs but does not
partition them or reserve one core per replica. At k=4 that is four CPUs shared by four replicas; at k=1 it is one
CPU. [V] Orion 4.5.0 `perf_tuning.md` recommends a pool close to the number of available cores and warns that more
threads than cores usually reduces performance. [G] The guidance is written per process and does not say how to
count processes sharing a cpuset. No A/B exists; Orion costs about 1 ms CPU per message; the FIWARE limit is the IoT
Agent app pool.

**2. Recommended disposition.**
[P] **Defer.** The proposed value 4 is not established for the shared-cpuset topology. It is an unvalidated
aggregate-oversubscription choice at k=4 and a clearer per-process overrun at k=1. Do not describe it as a proven
violation of the guidance.

**3. Alternatives and risks.**
[P] Alternatives: a topology-aware test with 0, 1 and 4 per replica at k=4; one per replica at k=4; reject. Risks:
up to 16 pool threads across four replicas on four cores; thread memory; a small change that is mistaken for a
capacity fix.

**4. Benefit and experimental impact.**
[P] Avoids thread creation/destruction per request and uses `epoll()`; likely small here. It does not address the
binding limit and does not create history.

**5. Dependencies and prerequisites.**
[P] A Category A ruling in the Text row before any implementation or run; no other FIWARE change during the A/B.
If D27 is also approved, run the Orion A/B after the index change to avoid confounding.

**6. Validation required after implementation.**
[P] Fixed-k A/B of 0, 1 and 4 per replica, three repetitions; measure ack latency, CPU per replica, errors and
throughput; record the aggregate pool size, the shared cpuset and the per-process visible CPU count. Verify the
effective flag with `docker inspect` or `ps`.

**7. Rollback.**
[P] Set `ORION_REQ_POOL_SIZE=0` and restart.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Record the chosen value and the shared-cpuset/aggregate interpretation. | `[ ]` |
| Impl/checks | Set `ORION_REQ_POOL_SIZE` in compose or `.env`; record the deviation; no other Orion change. | `[ ]` |
| Deploy | Restart the Orion replicas. | `[ ]` |
| Runs | Fixed-k A/B of 0, 1 and 4 per replica; record aggregate threads, shared cpuset and per-process visible CPUs. | `[ ]` |

### 4.9 ThingsBoard queue poll intervals

**1. Verified current behavior and evidence.**
[V] The pinned v4.3.1.6 configuration uses 25 ms poll intervals for transport request/response, core and rule-engine
queues, with 10 core partitions and `max_poll_records` 8192; the repository has no overrides. [V] Idle Kafka used
0.658 cores, 389 requests/s and 63 consumer-member rows in `DIAG-tb-idle2`. [V] The configuration documents the
knobs but no official recommended value was found; adding the interval to latency is the expected effect. No A/B
exists.

**2. Recommended disposition.**
[P] **Defer.** If the owner wants it, first rule whether a documented-but-unrecommended knob can be Category A, then
authorise a named A/B. It is not a qualification prerequisite.

**3. Alternatives and risks.**
[P] Alternatives: reject; approve after the Category A ruling; combine several queue families (rejected, because it
confounds the result). Risks: higher latency; a Category A classification that is not supported by an official
recommendation; interaction with Kafka consumer lag and with D28 if both are deployed together.

**4. Benefit and experimental impact.**
[P] May lower idle Kafka/JVM CPU and free headroom. It does not change durability or delivery if only the interval
changes, but it can add latency and must be measured.

**5. Dependencies and prerequisites.**
[P] Category A ruling in Text; one knob family at a time; if D28 is approved, deploy D28 first and re-baseline to
avoid confounding.

**6. Validation required after implementation.**
[P] Fixed-load A/B at, for example, 100/s and 500/s with one knob family, three repetitions; idle/loaded CPU, ack
and sink latency, throughput and Kafka consumer lag; record the value and the deviation. Do not combine with other
ThingsBoard changes.

**7. Rollback.**
[P] Unset the variables and restart; the defaults return.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Record the Category A ruling and the named knob family/value. | `[ ]` |
| Impl/checks | Set the named `TB_QUEUE_*_POLL_INTERVAL_MS` variables; record the deviation. | `[ ]` |
| Deploy | Restart the ThingsBoard services. | `[ ]` |
| Runs | Fixed-load A/B, one knob family at a time; idle/loaded CPU, latency, throughput and consumer lag. | `[ ]` |

### 4.10 D25-a — Upstream-backport authorization clarification

**1. Verified current behavior and evidence.**
[V] D25's Category A has two limbs: (a) the platform's official documentation recommends the change, or (b) it fixes
a defect in our deployment. [V] D27's index is upstream library code plus a changelog claim; it is neither limb by
itself, and our deployment is not misconfigured. [P] The packet §6.3 proposes a narrow clarification: a backport may
be A-equivalent only if the owner records an explicit exception, the change leaves ack point, durability, delivery
guarantee and batching semantics unchanged, and the application's own write/provisioning path is validated not to
gain new errors.

**2. Recommended disposition.**
[P] **Approve if D27 is approved.** Text only; it authorizes no platform work.

**3. Alternatives and risks.**
[P] Alternatives: reject and defer D27; amend D25 more broadly (wider precedent than needed); rely on a one-off
exception recorded inside D27 (works, but leaves no reusable rule). Risk: a broad backport exception could admit
changes that do alter semantics; the proposed wording is deliberately narrow.

**4. Benefit and experimental impact.**
[P] Allows the FIWARE fix to be classified honestly without pretending it is ordinary Category A. No runtime effect.

**5. Dependencies and prerequisites.**
[P] The D27 disposition. D25-a Text is a prerequisite for D27 Impl/checks, Deploy and Runs unless a one-off exception
is recorded in D27 Text.

**6. Validation required after implementation.**
[P] Read the appended row against D27 and the D25 text; confirm it does not rewrite D25 and does not authorize a
specific backport by itself.

**7. Rollback.**
[P] The log is append-only; a correction must itself be an append-only row. Do not rewrite D25.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Append the packet §6.3 row; no rewrite of D25. | `[ ]` |
| Impl/checks | — | — |
| Deploy | — | — |
| Runs | — | — |

### 4.11 D11-a — Historical D11 edit

**1. Verified current behavior and evidence.**
[V] D11 was edited in place in `c689191` (2026-10-22 16:48:13 +0330, 13:18 UTC). Before the edit it ended at
"Postponed by the user ("decide later"); pilot uses N=1." The edit added the exploratory OOM result and the N=1
memory decision (tb-core/rule-engine 1536m, transport 768m, heap unchanged; N=4 still open). [V] `EXPL-tb-k4` ran
09:40:41–09:41:39 UTC before the edit; its tb-core OOM at 1 GB motivated the text. `EXPL-tb-k4b` ran
14:31:47–14:56:22 UTC after the edit with that memory setting; its manifest git `671262e` predates `c689191`, so the
manifest lacks the deviation, and `results/exploratory/EXPL-tb-k4b.amendment.json` records it. [V] `EXPL-tb-k4b` is
INVALID on generator health; it is not a passing capacity result.

**2. Recommended disposition.**
[P] **Approve the append-only addendum.** It records the in-place edit without rewriting D11 and without changing
D11's substantive status.

**3. Alternatives and risks.**
[P] Alternatives: reject (the in-place edit stays unrecorded); rewrite D11 (forbidden by append-only policy); add a
broader governance row (more than needed). Risk: the addendum could be read as retroactively approving the memory
change; the proposed text says it records, not approves, and leaves D11 open.

**4. Benefit and experimental impact.**
[P] Restores an audit trail for a historical edit. No runtime effect.

**5. Dependencies and prerequisites.**
[P] None.

**6. Validation required after implementation.**
[P] Check the row against the git history and the amendment; confirm no existing row is rewritten and no approval is
implied.

**7. Rollback.**
[P] Append-only: a correction must be another row. Do not rewrite D11.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | Append the packet §6.1 row; no rewrite of D11. | `[ ]` |
| Impl/checks | — | — |
| Deploy | — | — |
| Runs | — | — |

### 4.12 D1-a — Optional historical correction note

**1. Verified current behavior and evidence.**
[V] D1 was corrected in place in `72a18f7`, before any dependent run. The old text said the prior notes did not
exist; the corrected text records their location and the search-depth error. [V] The correction is already dated in
the D1 row.

**2. Recommended disposition.**
[P] **No change required.** Add the optional note only if the owner wants a separate audit trail.

**3. Alternatives and risks.**
[P] Alternatives: add the optional row; leave D1 as is. Risk: none substantive; the note could add noise to the log.

**4. Benefit and experimental impact.**
[P] Audit trail only. No runtime effect.

**5. Dependencies and prerequisites.**
[P] None.

**6. Validation required after implementation.**
[P] If added, check the text against the git history and confirm it is append-only.

**7. Rollback.**
[P] Append-only: a correction must be another row.

**8. Exact authorized scope if approved.**

| Level | Authorized scope | Box |
|---|---|---|
| Text | If chosen, append the packet §6.2 optional row; otherwise no action. | `[ ]` |
| Impl/checks | — | — |
| Deploy | — | — |
| Runs | — | — |

## 5. Unresolved research-critical blockers

The Step 5 tuning decisions do not resolve these. Each blocker is classified as an **implementation gap**, a
**measurement limitation**, or a **claim that must change** if the gap is not closed.

### 5.1 D9 — FIWARE historical persistence

**Verified current state.**
[V] `deploy/fiware/compose.yaml` contains only `mongo`, `mongo-init`, `orion`, `iot-agent` and `haproxy`; there is no
QuantumLeap, TimescaleDB, CrateDB or other history backend. Orion keeps only the latest entity state. The sink
receives notifications after the Orion write, but that is a delivery observation, not history.
[V] `scripts/history-ids.sh fiware` exits 3 with "FIWARE-no-history keeps no history: not reconcilable".
[V] `docs/development/tooling-interfaces.md` marks the FIWARE reconciliation reader as absent, and the metrics dictionary has no
FIWARE history quantity.
[V] The pre-registered research notes plan `IoTA → Orion → subscription #1 → QuantumLeap (TimescaleDB)` and
`subscription #2 → sink`, running in parallel. [G] No version, data model, memory allocation or query interface for
that plan exists in this checkout.

**Why this blocks research validity.**
The study's common operation is "receive identifiable telemetry and retain its history" and its durability check is
"stored count equals sent count, zero duplicates" on all three platforms. Without an independent FIWARE history,
FIWARE cannot be reconciled, cannot contribute a persistence-based goodput verdict, and cannot support a
cross-platform claim that all three platforms persisted every reading. Sink counts are not a substitute: they show
notification delivery, not stored history, and they are not joined to acknowledged identities.

**Requirements for every `(run_id, seq)` identity.**

| # | Requirement | Why it is required |
|---|---|---|
| R1 | One independently addressable history row per logical message, carrying `run_id`, `seq`, device, `scheduled_send_ms` and the payload fields | The reconciliation unit is the identity, not an aggregate count. |
| R2 | Independence from Orion's latest-state entity document | Updating or deleting the entity must not remove history. |
| R3 | A uniqueness rule on `(run_id, seq)`, or an audit query that reports duplicates | Notification retries must not silently collapse or double-count messages. |
| R4 | A query/export by `run_id` returning every `(run_id, seq)` after the run and after a restart | The existing EdgeX/ThingsBoard readers define the interface; FIWARE needs an equivalent. |
| R5 | A server-side receipt or insert timestamp, distinct from the client's scheduled time | Needed for auditing and for any storage-side latency statement. A per-message commit time is a stronger requirement and is not currently available on any platform. |
| R6 | Retention for the campaign and a clean wipe between runs | Cold start and DB wipe are part of the protocol. |
| R7 | CPU and memory counted in the FIWARE platform, inside the fixed supporting-tier budget | QuantumLeap and TimescaleDB are part of the system under test, not an external observer. |
| R8 | Detectable failure: a notification drop appears as an acknowledged-but-not-stored identity | Otherwise a short history silently looks complete. |
| R9 | Both subscriptions provisioned and verified before the run, with the known cache-refresh delay | The exploratory run needed a 65 s wait after subscription creation. |
| R10 | No equivalence with Orion latest state, MongoDB operation counters, sink receipt counts or Kafka lag | These are different quantities and cannot prove history. |

**Assessment of the planned QuantumLeap/TimescaleDB architecture.**
[P] The architecture is the right shape if, and only if, QuantumLeap writes one row per notification and the
identity attributes are stored as queryable columns. [G] QuantumLeap's default data model is a table per entity type
with one row per notification and attribute columns; whether the pinned version enforces a uniqueness rule on
`(run_id, seq)`, or upserts by entity and time, must be verified before approval. If it upserts, two messages of one
entity in the same timestamp can collapse, and the architecture would repeat the ThingsBoard problem. TimescaleDB
compression or retention must not drop rows during the campaign. The added services must fit the 16 GiB SUT and the
fixed three-core supporting tier; if they do not, either the memory allocation or the supporting-tier accounting
must change, and that is an owner decision, not an implementation detail.

**Alternatives.**

| Option | What it means | Main risk |
|---|---|---|
| A. QuantumLeap + TimescaleDB | The pre-registered plan; two Orion subscriptions, one to history and one to the sink | Version/data-model verification; memory and CPU accounting; notification-path loss |
| B. Small Go history writer + PostgreSQL/TimescaleDB | Subscribe to Orion, write one row per `(run_id, seq)` with an explicit unique constraint | Deviates from the pre-registered architecture; new code and its own validation |
| C. Defer FIWARE and exclude it from cross-platform claims | Run EdgeX and ThingsBoard only, labelled incomplete | Weakens the three-platform comparison and the paper's scope |
| D. Stop | Do not run FIWARE | No FIWARE result |

**Recommended decision and validation plan (not an implementation).**
[P] Choose A, with B as the fallback if the pinned QuantumLeap cannot guarantee independent rows. The plan:

1. Owner decision on A or B (Text), plus the memory/CPU placement in the supporting tier.
2. Pin the QuantumLeap and TimescaleDB versions and record their images.
3. Add the services and the history subscription; keep the sink subscription separate and parallel.
4. Implement `scripts/history-ids.sh fiware` against the history store, returning `(run_id, seq)` for a run.
5. Replay a known set of N messages: require N rows, N distinct `(run_id, seq)`, zero duplicates, zero missing, and
   no dependence on the latest entity state.
6. Inject a duplicate notification and a dropped notification: the first must be countable, the second must appear
   as acknowledged-but-not-stored.
7. Restart the history service and the database; repeat the query. Wipe and cold-start; repeat.
8. Measure the added CPU per message and the memory working set; confirm the fixed supporting-tier budget.
9. Only then authorize a FIWARE smoke and, later, qualification runs.

**Stop conditions.** Rows collapse or are overwritten; the query returns latest state only; any acknowledged message
cannot be accounted for; the added services exceed the memory/CPU budget; the history path cannot be distinguished
from the sink path. Stop and report; do not delete or rewrite data.

**Blocking scope.** FIWARE qualification and every cross-platform durability, capacity or fingerprint claim. An
EdgeX/ThingsBoard k=1 pilot may proceed only if it is explicitly labelled "FIWARE absent" and is not used for a
three-platform claim. [P] A publication-grade first run should not be declared until D9 is resolved for all three
platforms.

### 5.2 D11 — ThingsBoard four-core feasibility

**Verified current state and the arithmetic.**
[V] The SUT is 8 vCPU / 16 GB. `scripts/check-resources.sh` reserves 1,536 MiB, so the declared memory budget is
14,848 MiB (16 GiB minus the reserve). The supporting tier is fixed at 3,840 MiB: PostgreSQL 1,536 + Kafka 1,280 +
ZooKeeper 512 + Valkey 256 + HAProxy 256. ThingsBoard's three application services are `tb-core`, `tb-rule-engine`
and `tb-http-transport`; `TB_REPLICAS` scales all three together.

[V] The exploratory ThingsBoard run used a **four-core pool with one replica per tier**, not four replicas. Its
memory override was 1,536 / 1,536 / 768 MiB, heap unchanged at 512 / 512 / 256 MiB. It booted and ran steps up to
200 msg/s; at 700 and 1,000 msg/s the rule engine raised `OutOfMemoryError: Java heap space` 74 times without
restarting and left about 193,700 messages in Kafka. The run is INVALID on generator health, not a passing capacity
result. [V] The earlier `EXPL-tb-k4` at 1 GiB tb-core was OOM-killed during device provisioning, before load.

| Configuration | Replicas per tier | App MiB | Infra MiB | Total MiB | Against 14,848 MiB budget | Verified to run? |
|---|---|---|---|---|---|---|
| k=4 pool, exploratory 1536/1536/768 | 1 | 3,840 | 3,840 | 7,680 | fits | **Yes**: booted and ran; overall verdict INVALID on generator; heap OOM at 700/1000 |
| k=4 pool, tracked defaults 1024/1024/512 | 1 | 2,560 | 3,840 | 6,400 | fits | **No**: tb-core OOM at 1 GiB during provisioning |
| k=2 pool, exploratory 1536/1536/768 | 2 | 7,680 | 3,840 | 11,520 | fits | **No** |
| k=3 pool, mixed 1536/1024/512 | 3 | 9,216 | 3,840 | 13,056 | fits | **No** |
| k=4 pool, tracked defaults 1024/1024/512 | 4 | 10,240 | 3,840 | 14,080 | fits on paper (768 MiB margin) | **No**; tb-core OOM at 1 GiB, and no four-replica run exists |
| k=4 pool, exploratory 1536/1536/768 | 4 | 15,360 | 3,840 | 19,200 | **over by 4,352 MiB** | **No**; cannot fit in 16 GiB |
| k=3 pool, exploratory 1536/1536/768 | 3 | 11,520 | 3,840 | 15,360 | over by 512 MiB | **No** |

**Separate the three questions.**

- **Arithmetic fit.** The tracked defaults at k=4/N=4 total 14,080 MiB, which is inside the 14,848 MiB budget but
  leaves only 2,304 MiB to physical RAM for OS, Docker and page cache. The exploratory values at k=4/N=4 total
  19,200 MiB and cannot fit. A k=3 mixed configuration at 13,056 MiB fits arithmetically; a k=2 exploratory
  configuration at 11,520 MiB fits. These are arithmetic statements only.
- **Verified to run.** Only the k=4 pool with **one replica** and the exploratory memory values has been observed to
  boot and run. The tracked defaults have not been verified even at one replica, because tb-core OOM'd at 1 GiB.
  No four-replica configuration has been run.
- **Unqualified.** Everything else: k=4/N=4 on defaults or on any raised-memory mix; k=2; k=3; any JVM-resized
  variant; any claim that the k=4 four-replica point is feasible.

**Why JVM tuning cannot be assumed to solve it.**
[P] The heap is already 512 MiB in a 1 GiB or 1.5 GiB container; the JVM also needs roughly 350–400 MiB outside the
heap (metaspace, code cache, direct memory, stacks, GC structures). Raising the heap requires raising the container
limit, which the k=4 budget does not have; lowering non-heap reserves risks a different OOM. The k=4b evidence shows
that raising the container from 1 GiB to 1.5 GiB did **not** prevent the heap OOM at 700/1,000 msg/s. No official
ThingsBoard heap-sizing guidance for Docker was found in the diagnostics. A JVM change is therefore a separate
experiment with its own validation, not a fix that can be assumed.

**Decision options.**

| Option | Consequence | Status |
|---|---|---|
| A. Prove a k=4, N=4 configuration inside 16 GiB | Preserves the pre-registered k set and one-replica-per-core interpretation | Unproven; the arithmetic leaves almost no room if tb-core must exceed 1 GiB |
| B. Use k ∈ {1,2,3} for ThingsBoard and report the deviation | The pre-registered RQ-B known constraint explicitly allows this | Methodological change; needs owner approval |
| C. Keep k=4 but use fewer than four replicas per tier | Fits memory, but changes what E(4) means | Conflicts with "one replica per core"; needs owner approval |
| D. Larger SUT | Violates the fixed hardware budget | Not recommended |
| E. Defer ThingsBoard scaling | Keeps k=1 only | Weakens RQ-B and the fingerprint |

**Recommended path (not an approval).**
[P] Do **not** approve k=4 for ThingsBoard on arithmetic. First authorise a bounded k=1/k=2 memory-feasibility check
that uses `docker compose config --format json | scripts/check-resources.sh`, boots the stack, provisions 1,000
devices, measures idle and loaded memory, and records OOM events. If no k=4/N=4 configuration can be verified, bring
the choice between B and C back to the owner. B is the cleaner methodological fallback because it is already named
in the pre-registered known constraints; C must be reported as a different scaling configuration.

**Validation required before any ThingsBoard scaling claim.**
[P] Fit check; boot; provisioning of 1,000 devices; idle and loaded memory working set; OOM events and restarts;
fixed-load k=1; then k=2; then k=4 only if a configuration fits and boots. Record the exact memory values, replicas
and heap flags. Do not infer feasibility from arithmetic and do not call a 4-core/1-replica run a four-replica
result.

**Blocking scope.** ThingsBoard E(2)/E(4), the ThingsBoard part of the fingerprint, and every statement that
ThingsBoard scales to four replicas on this host. It does not block an EdgeX k=1 pilot.

### 5.3 Measurement correctness

These are gaps between what the study claims to measure and what the current instrumentation can establish. They are
not tuning decisions and cannot be fixed by approving a Step 5 row.

| # | Gap | Verified state | Claim that is unsupported today | What would close it |
|---|---|---|---|---|
| M1 | No ack→sink identity join | [V] `internal/explore` counts sink NDJSON records; `internal/reconcile` joins the ledger only to stored history; no code matches an acknowledged `(run_id, seq)` to a sink record | "Every acknowledged message was delivered to the sink"; per-message delivery latency | A join on `(run_id, seq)` between the acknowledged set, the sink stream and stored history, with an explicit unmatched-set report |
| M2 | No per-message commit or visibility evidence | [V] EdgeX uses a table-level `n_tup_ins` counter with a 5 s scrape; ThingsBoard has no per-message commit marker; FIWARE has no history at all | "Durably stored" as a per-message property; commit latency | A storage-side insert/commit timestamp per identity, or a restart-visibility check on a sampled identity set; state the weaker interpretation otherwise |
| M3 | Duplicate and retry auditing is incomplete | [V] `reconcile` computes duplicates for EdgeX/ThingsBoard history where a reader exists; the sink stream has no duplicate accounting; FIWARE has no reader | "Zero duplicates" across all observation points | Duplicate counts reported separately for sink, history and storage; FIWARE reader (D9) |
| M4 | ThingsBoard same-millisecond collisions | [V] The adapter sends no `ts`; the server timestamps at JSON conversion before queueing; SQL upserts on `(entity_id, key, ts)` | "Every message is stored" and "zero loss" at high rate | An audit that compares sent identities per device per millisecond against stored rows; if collisions occur, report the loss as a platform property and re-scope the loss claim |
| M5 | EdgeX persistence counter is table-level and unfiltered | [V] `pg_stat_user_tables_n_tup_ins{relname="reading",schemaname="core_data"}`, 5 s scrape, database flush interval | Per-run, per-message stored counts from the live series | Use the history reader for verdicts and the live series only as a rate/goodput proxy |
| M6 | Ack, delivery and storage are different quantities across platforms | [V] EdgeX acks at `device-rest`; ThingsBoard acks at the HTTP transport before asynchronous processing; FIWARE acks after the Orion write | Comparing ack latency across platforms as if it were the same point | D30 text plus per-platform reporting that names the point; no cross-platform ack comparison |
| M7 | Cross-host latency is unqualified | [V] k6 computes ack latency from `response - scheduled_send_ms` using load-host `Date.now()`; the sink stamps with `time.Now()` and defaults to the load host; [G] no measured ramonavm1↔ramonavm2 offset | Any statement that a cross-host delta of a few milliseconds is real | Measure the offset and its drift, report the uncertainty, and widen any threshold that the offset can consume |
| M8 | D23 intent versus evidence | [V] D23 defines goodput at persistence; the live persistence series is a proxy; per-message durability is not instrumented | "D23 goodput" as a per-message durability measure | Keep the D23 verdict rule, but label the live series a rate/goodput proxy until M2 closes |
| M9 | Wall-clock scheduling and send-lag inclusion | [V] `scheduled_send_ms` is a plan-relative wall clock; ack latency includes send lag and is not `http_req_duration` | Treating ack latency as pure server time | Report send lag separately and gate on its p99 (already a generator gate) |

**Minimum instrumentation set before a publication-grade run.** [P]

1. Identity join: acknowledged set ↔ sink stream ↔ stored history, per `(run_id, seq)`, with unmatched counts by
   direction and by platform.
2. Storage-side audit: either a per-message insert/commit timestamp or a documented restart-visibility check on a
   sampled set, reported at the interpretation level the evidence supports.
3. Duplicate accounting at every observation point, reported separately.
4. A ThingsBoard collision audit (sent identities per device per millisecond versus stored rows).
5. A measured cross-host clock offset with drift, recorded per campaign, not per run only.
6. Report fields for the ack/sink/storage distinction, aligned with D30.

**Claim restrictions if the owner chooses not to implement them.** [P] State that delivery and durability are
established in aggregate by counts and inference, not per message; do not report a per-message e2e latency that
depends on an unmeasured clock offset; do not compare ack latency across platforms; report the ThingsBoard
same-millisecond collision risk as a known limitation. These restrictions are honest but weaker; they do not
substitute for M1 and M2 in a durability claim.

**Options for decision.** `[ ]` Implement the minimum set · `[ ]` Restrict claims and proceed · `[ ]` Both ·
`[ ]` Defer and stop before publication-grade runs.

### 5.4 Generator qualification

**Verified state.**

- [V] The open-loop k6 generator reaches about 1,000 req/s on two pinned cores, about 1,500–2,000 req/s on three
  cores (2,000 is borderline), and about 500 req/s in Poisson mode on three cores.
- [V] Memory growth in the generator process is real: 200 VUs × 30 steps reached about 5.2 GB; 50 VUs used about
  1.56 GB.
- [V] Delayed-response and concurrency behaviour is documented in `docs/methodology/k6-feasibility.md`: k6 has wall-clock-only
  timing, millisecond resolution, and a bounded iteration model; the planned arrival identity comes from
  `iterationInTest`, with a step-boundary guard.
- [V] No run has measured the three-generator-core layout with the sink and monitoring running together on the load
  host, so the 2× headroom rule is not qualified for the full experimental layout.
- [G] There is no measured generator ceiling with the sink writing NDJSON at the highest planned rate.

**What "qualified" must mean.** [P] A layout is qualified for a rate when, at that rate and for the planned duration,
all generator gates pass: zero dropped iterations, all ticks present, achieved/planned ≥ 0.99, send-lag p99 within
its gate, no early sends, and CPU and memory inside the host envelope — **while** the sink and monitoring run as they
will during the experiment, and with the 2× headroom rule satisfied.

**Known generator-limited experiments.** [V] Any planned rate above the qualified ceiling in the actual layout.

- The exploratory EdgeX k=1 failure at the 200 msg/s step and the ThingsBoard rule-engine stall at 700/1,000 msg/s
  are confounded by generator health in at least one run; they cannot be cited as pure platform ceilings without a
  generator re-qualification at those rates.
- `EXPL-tb-k4b` is INVALID on generator health and must not be used as a capacity point.
- Poisson experiments at rates near the 500 msg/s prototype ceiling are not qualified for the sampling rates the
  design may need.

**Do not claim full qualification.** [P] The current evidence supports "the generator is qualified for low-rate,
constant-arrival smoke and pilot workloads in a two- or three-core layout without the sink", and nothing stronger.

**Required qualification work.** [P]

1. Measure the exact experimental layout (generator cores, sink, Prometheus, exporters) at the highest planned rate.
2. Measure the 2× rule: run at 2× the planned maximum and confirm all gates pass with margin.
3. Measure the Poisson generator at the planned sampling rate and confirm the same gates.
4. Measure the delayed-response and high-concurrency cases named in `docs/methodology/k6-feasibility.md`.
5. Record CPU, memory, dropped iterations, send lag, and the sink's write rate in the same manifest fields.
6. Publish a qualified rate table per platform and per k, and cap every plan at the qualified ceiling.

**Fallback.** [P] If the qualified rate is below a planned rate, either cap the experiment (methodological change,
owner decision) or build the Go generator (D24 reopen). Do not run an unqualified generator and call the platform
result a ceiling.

**Options for decision.** `[ ]` Qualify the full layout at planned maximums · `[ ]` Cap experiments to the qualified
rate · `[ ]` Build the Go generator · `[ ]` Combination (state which).

## 6. Experimental scope freeze

This section states what this sign-off treats as fixed, what remains conditional, and which choices must be made
before the first main run. Approving a row in §4 does **not** change anything below; changing a frozen element is a
separate owner decision and must be reported explicitly.

### 6.1 Frozen elements

| # | Frozen element | Exact statement | Basis |
|---|---|---|---|
| F1 | Three platforms | FIWARE, ThingsBoard and EdgeX Foundry, at the pinned versions in `README.md` | Pre-registered scope |
| F2 | Device count | 1,000 provisioned devices per platform | Pre-registered workload |
| F3 | Workload model | Open-loop, constant-rate HTTP telemetry; the generator schedules arrivals independently of responses | Pre-registered; D23 |
| F4 | Latency budget | The platform time budget is **500 ms** (P1 = 1 s end-to-end, 50% to the platform: TELEM-500) | Pre-registered SLO profile |
| F5 | Sensitivity set | Re-analysis at 250 ms, 1,000 ms and 5,000 ms from the same data; no new runs | Pre-registered |
| F6 | Scaling factor | k ∈ {1, 2, 4} application-tier cores, **subject to feasibility** for each platform (see §5.2) | Pre-registered; D26 |
| F7 | Supporting-tier accounting | Fixed, counted inside the SUT budget, identical across platforms: PostgreSQL/MongoDB, queue/broker, registry/cache, HAProxy | Pre-registered; `scripts/check-resources.sh` |
| F8 | History verification | Independent of the platform's latest-state view; every platform must expose an identity-addressable history for `(run_id, seq)` | Pre-registered; D9 |
| F9 | Repetitions and uncertainty | At least five repetitions per cell for confirmation, reported with a 95% CI; pilot cells may be fewer but must be labelled | Pre-registered protocol |
| F10 | Experiment families | Capacity, scaling, bottleneck location, overload and recovery | Pre-registered RQs |
| F11 | Reporting frame | Conventional metrics (throughput, latency, loss, errors, CPU) reported alongside the SLO/fingerprint view | Pre-registered contribution |
| F12 | Verdict rule | D23 goodput (durably stored and delivered, evaluated at persistence for EdgeX) with zero loss, errors ≤ 0.1%, p99 within budget, non-growing backlog | Adopted D23 |
| F13 | Durability requirement | No durability-weakening setting in the qualification configuration | Adopted D25 |
| F14 | Hardware budget | SUT 8 vCPU / 16 GB with the 1,536 MiB reserve; load host 4 vCPU / 8 GB | Adopted |
| F15 | Observation points | Per D30: ack, sink and storage are distinct and named in every report | Proposed D30 |

### 6.2 Conditional elements

| # | Element | Condition | If the condition fails |
|---|---|---|---|
| C1 | k=4 for ThingsBoard | A configuration inside the memory budget is verified to boot and run | Fall back to k ∈ {1,2,3} for ThingsBoard and report the deviation, or stop scaling claims (§5.2) |
| C2 | FIWARE participation in cross-platform claims | A verified independent history exists (§5.1) | Run FIWARE labelled "history absent" and exclude it from durability/capacity comparisons |
| C3 | Any rate above the qualified generator ceiling | The full experimental layout passes the generator gates at 2× the planned rate | Cap the rate, or build the Go generator (D24 reopen) (§5.4) |
| C4 | Per-message delivery and durability claims | The identity join and storage audit exist (§5.3) | Report aggregate counts and inference only, with explicit limitations |
| C5 | Cross-host latency statements | A measured clock offset with drift | Report same-host quantities only, or widen thresholds by the measured bound |
| C6 | Overload and recovery experiments | A valid capacity point exists for that platform and k | Run overload only relative to a measured C_SLO, never relative to an estimate |
| C7 | Fingerprint components E(2), E(4), L_knee | Two or more valid scaling points per platform | Report the available components and mark the rest unavailable |
| C8 | Five repetitions per confirmation cell | Host availability and time budget | Label the cell as pilot, not confirmation |
| C9 | D27 index in the qualification configuration | Duplicate audit clean and provisioning path validated | Deploy FIWARE without the index and report the COLLSCAN limitation |
| C10 | D28 official configuration | Chosen variant is vendored, hashed and mounted | Keep the current configuration and report the broken-OOM behaviour |

### 6.3 Decisions required before the first main run

| # | Decision | Why it cannot wait | Owner action |
|---|---|---|---|
| P1 | D9 architecture (A or B) | FIWARE cannot be verified without it; longest lead item | Text decision + placement in the supporting tier |
| P2 | D11 outcome (A, B, C or E) | Determines whether ThingsBoard has a k=4 point | Text decision after the feasibility check |
| P3 | Measurement minimum set (§5.3) | Determines what a run may claim | Text + Impl/checks |
| P4 | Generator qualification target and cap (§5.4) | Determines the maximum planned rate per cell | Text + Impl/checks + Runs |
| P5 | k versus N interpretation (D26 versus the pre-registered N) | Every scaling statement depends on it | Text decision; pin the specification in-repo |
| P6 | Headroom rule (2× versus D24's 1.5×) | Determines the generator gate and the maximum rate | Text decision; amend D24 explicitly if changed |
| P7 | Specification pinned in-repo | The RQ/SLO/fingerprint text lives outside the repository | Read-only copy plus a hash |
| P8 | D30 text | Every report's claim structure depends on it | Text |
| P9 | Repetition protocol and warm-up/step/confirmation durations | The external protocol and the exploratory runs differ | Text decision reconciling them |

### 6.4 Specification-versus-repository conflicts

All four conflicts were found while preparing this sign-off. None is resolved here.

| # | Conflict | External pre-registered text | Repository state | Consequence if unresolved |
|---|---|---|---|---|
| S1 | Meaning of the scaling factor | N = number of application-tier **replicas**, one pinned core each | D26: k = number of shared application **cpuset cores**, replicas recorded separately; all replicas of a tier share one cpuset | E(N), E(2) and E(4) mean different things; two papers could be written from the same data |
| S2 | Definition of goodput | Stored exactly once **and** arrives within the budget, stopping at sink receipt | D23: evaluated at persistence for EdgeX, with the sink as the latency observation point | Verdicts differ at the boundary; the same run can pass under one definition and fail under the other |
| S3 | Headroom rule | 2× the planned maximum | D24: 1.5× | A rate can be qualified under one rule and not the other |
| S4 | k range for ThingsBoard | k ∈ {1,2,3} allowed if N=4 does not fit | Prompt freezes k ∈ {1,2,4} | Either the platform set or the k set must change, with a reported deviation |
| S5 | Run protocol | 5 min warm-up at 20%, 3 min ×1.5 steps, 10 min confirmation, ≥5 runs, 95% CI ±5% | Exploratory runs used 120 s steps and 180 s confirmation | Confirmation cells are not comparable with the pre-registered protocol until reconciled |

[P] Recommended handling: pin the external text in-repo (P7), then resolve S1–S5 in one dated owner addendum rather
than piecemeal. Do not silently adopt either side.

### 6.5 Methodological alternatives if feasibility forces a change

If D9, D11 or generator qualification cannot be satisfied within the frozen budget, the owner must choose among
these. They are **not** preselected and **not** interchangeable.

| Area | Alternative | What changes | What must be reported |
|---|---|---|---|
| FIWARE history | QuantumLeap + TimescaleDB | Nothing in the comparison; adds the pre-registered architecture | Versions, memory/CPU accounting, subscription verification |
| FIWARE history | Custom history writer | The platform definition gains a component not in the official topology | Deviation, code provenance, its own validation |
| FIWARE history | Exclude FIWARE from cross-platform claims | The study becomes two-platform for durability/capacity | Explicit scope statement; no three-platform claim |
| ThingsBoard k=4 | k ∈ {1,2,3} for ThingsBoard | The k set differs by platform | Deviation; E(4) unavailable for ThingsBoard |
| ThingsBoard k=4 | Fewer replicas per tier at k=4 | The one-replica-per-core interpretation is lost | A differently defined scaling configuration |
| ThingsBoard k=4 | Larger SUT | The fixed hardware budget is violated | New budget; comparability with prior runs is lost |
| Generator | Cap the planned rate | The tested range shrinks | The cap, the qualified rate, and the affected RQs |
| Generator | Build the Go generator | The tool changes; the workload definition does not | Tool provenance, new qualification, re-baselining |
| Measurement | Restrict claims instead of instrumenting | Weaker but honest reporting | The exact claim restrictions (§5.3) |

**Rule.** No alternative may be adopted by implication from a feasibility failure. Each requires its own owner
decision and its own deviation record.

## 7. Dependency-ordered roadmap

Stages are ordered by dependency. A later stage does not become authorized because an earlier one completes; each
stage's own authorization is required. "Immediate" means it can start as soon as the owner decides and touches no
SUT. "Blocked" names what must be true first.

### 7.1 Stage 1 — Owner decisions and recording

| | |
|---|---|
| **Immediate or blocked** | Immediate, after the owner reads this document |
| **Inputs** | This sign-off; the packet §9; the external research notes |
| **Authorizations** | Text only, per candidate: the D27–D30, D25-a, D11-a, D1-a rows the owner chooses; plus the D9, D11, measurement, generator, specification and headroom decisions in §1.1 |
| **Verification** | Every ticked row is dated and initialled; adopted text matches the candidate exactly; no row is ticked by implication; `docs/design-decisions.md` receives only append-only rows |
| **Deliverables** | A recorded decision sheet; the adopted D30 table; the appended D25-a/D11-a/D1-a rows if approved; the pinned specification copy (P7) |
| **Stop conditions** | Any candidate left ambiguous; any attempt to tick a level without its prerequisite text; any edit to an existing decision row |

### 7.2 Stage 2 — Baseline and evidence integrity

| | |
|---|---|
| **Immediate or blocked** | Immediate; local only |
| **Inputs** | `evidence/reproducibility.md`; the retained `results/**`; the tracked amendment; `scripts/diag/report.py` |
| **Authorizations** | Impl/checks for documentation and packaging only; no deployment |
| **Verification** | `scripts/diag/report.py` reproduces the report apart from the embedded commit; every cited number resolves to a retained file; the evidence package is either versioned or archived with hashes; a fresh clone is documented as insufficient |
| **Deliverables** | An evidence package (or an archive with a manifest of hashes); a reproducibility note stating exactly what can and cannot be regenerated |
| **Stop conditions** | Any cited number without a retained source; any silent edit to the generated report; any attempt to regenerate evidence and present it as the original |

### 7.3 Stage 3 — Approved deployment fixes

| | |
|---|---|
| **Immediate or blocked** | Impl/checks immediate; Deploy blocked by SUT reachability and by stage 2 |
| **Inputs** | D28 (approved variant); D27 + D25-a (if approved); the pinned upstream files; the duplicate audit |
| **Authorizations** | Text and Impl/checks first; Deploy and Runs are separate ticks per candidate |
| **Verification** | Vendored file hashes match the pinned tag; `docker compose config` is clean and passes `scripts/check-resources.sh`; mounts and effective JVM flags are visible; the duplicate audit is clean before index creation; `explain` shows `IXSCAN` |
| **Deliverables** | A recorded deviation per change; a pre/post smoke comparison at the same offered rate and k; a rollback note verified once |
| **Stop conditions** | Duplicate tuples in the registry; a failed mount or a service that will not start; any change that alters ack, durability, delivery or batching semantics beyond the approved text; SUT unreachable |

### 7.4 Stage 4 — FIWARE history (D9)

| | |
|---|---|
| **Immediate or blocked** | Design and pinning immediate; deployment blocked by SUT reachability and by the memory/CPU placement decision |
| **Inputs** | The D9 decision (A or B); pinned QuantumLeap/TimescaleDB versions or the writer design; the Orion subscription model; `scripts/history-ids.sh` |
| **Authorizations** | Text; then Impl/checks; then Deploy; then Runs |
| **Verification** | The §5.1 R1–R10 checklist; replay of a known set returns N rows and N distinct identities; duplicate injection is countable; a dropped notification appears as acknowledged-but-not-stored; restart and cold-start repeats pass; added CPU/memory inside the budget |
| **Deliverables** | A FIWARE history reader returning `(run_id, seq)`; a validated subscription configuration; the measured overhead |
| **Stop conditions** | Any collapse or overwrite; a latest-state-only query; an unaccounted acknowledged message; budget overrun; inability to separate the history path from the sink path |

### 7.5 Stage 5 — Resource feasibility (D11)

| | |
|---|---|
| **Immediate or blocked** | Fit arithmetic immediate; boot and load blocked by SUT reachability and by stage 3 |
| **Inputs** | The D11 decision; `scripts/check-resources.sh`; compose memory defaults; the 1,000-device provisioning plan |
| **Authorizations** | Impl/checks for fit checks; Deploy and Runs for the bounded k=1/k=2 checks only |
| **Verification** | Each candidate configuration fits the budget on paper; boots; provisions 1,000 devices; records idle and loaded working set; records OOM events and restarts; a fixed-load check at k=1 |
| **Deliverables** | A table of configurations with fit, boot and load outcome; a recommendation between options A, B, C and E |
| **Stop conditions** | An OOM during provisioning or at a low fixed rate; a configuration that fits only by omitting a supporting service; any claim that k=4/N=4 is feasible without a boot and load observation |

### 7.6 Stage 6 — Generator, sink and oracle qualification

| | |
|---|---|
| **Immediate or blocked** | Local measurement possible on the workstation; the full layout is blocked by SUT reachability |
| **Inputs** | §5.4; `docs/methodology/calibration.md`; `docs/methodology/k6-feasibility.md`; the sink; the monitoring stack |
| **Authorizations** | Impl/checks; Runs for generator-only checks; Deploy for the load host only as separately ticked |
| **Verification** | All generator gates pass at the planned maximum and at 2× with sink and monitoring running; Poisson at the planned sampling rate; delayed-response and concurrency cases; sink write rate sustained; CPU and memory inside the host envelope |
| **Deliverables** | A qualified-rate table per layout; a statement of the exact layout each rate was qualified in; the sink's measured capacity |
| **Stop conditions** | Any dropped iteration, missing tick, achieved/planned below 0.99, send-lag breach or early send; memory exhaustion; a sink that cannot keep up at the planned rate |

### 7.7 Stage 7 — Gate calibration

| | |
|---|---|
| **Immediate or blocked** | Blocked by stages 5 and 6 for the full set; per-platform k=1 gates can be calibrated as soon as that platform's stage 3 and stage 6 are complete |
| **Inputs** | The frozen SLO (TELEM-500); the D23 verdict rule; the measurement minimum set; the qualified generator rates |
| **Authorizations** | Impl/checks for gate definitions; Runs for calibration smokes |
| **Verification** | Every gate has a named metric, producer, threshold and failure action; the gate fires correctly on a known-good and a known-bad run; the persistence proxy is labelled a proxy; the observation points match D30 |
| **Deliverables** | A gate specification per platform; a calibration record showing the gate's behaviour on retained evidence |
| **Stop conditions** | A gate with no producer; a gate that cannot distinguish pass from fail; a threshold change made after seeing data without a dated amendment |

### 7.8 Stage 8 — First valid k=1 capacity pilot

| | |
|---|---|
| **Immediate or blocked** | Blocked by stages 3, 5, 6 and 7 for the platform being piloted; FIWARE additionally blocked by stage 4 |
| **Inputs** | One platform at k=1; 1,000 devices; constant-rate HTTP; the frozen budget and repetitions |
| **Authorizations** | Deploy and Runs, per platform, separately ticked |
| **Verification** | Generator gates pass; the run is VALID on health; history reconciliation returns stored = sent with zero duplicates for that platform; the measurement set produces a reportable latency and goodput; the explanatory variables are recorded |
| **Deliverables** | One pilot cell per available platform, explicitly labelled pilot; the first trustworthy capacity estimate and its uncertainty |
| **Stop conditions** | Generator gates fail; reconciliation is incomplete; durability is unverifiable; the run's own manifest lacks a deviation; any threshold is adjusted mid-run |

### 7.9 Stage 9 — Confirmed capacity and repetitions

| | |
|---|---|
| **Immediate or blocked** | Blocked by stage 8 and by host time |
| **Inputs** | The pilot protocol reconciled with the pre-registered durations (S5); ≥5 repetitions |
| **Authorizations** | Runs per platform and k, separately ticked |
| **Verification** | Each cell reaches the pre-registered repetition count; the 95% CI is reported; outliers are explained, not deleted; the warm-up and confirmation durations match the reconciled protocol |
| **Deliverables** | Confirmed C_SLO per platform and k; the uncertainty intervals; the conventional metrics alongside |
| **Stop conditions** | Fewer repetitions than the protocol requires without a "pilot" label; an unexplained outlier; a protocol deviation found after the fact |

### 7.10 Stage 10 — Multi-configuration scaling, overload, bottleneck and recovery

| | |
|---|---|
| **Immediate or blocked** | Blocked by stage 9 and, for k>1, by the D11 outcome and the S1 k-versus-N resolution |
| **Inputs** | Confirmed k=1 points; at least one higher-k point per platform; the overload profile; the bottleneck analysis |
| **Authorizations** | Runs per configuration, separately ticked |
| **Verification** | Each scaling point is a valid run with the same protocol; E(2)/E(4) are computed from measured points only; overload is expressed relative to a measured C_SLO; recovery is measured against the 2-minute rule; the bottleneck location is supported by the diagnostic evidence |
| **Deliverables** | The scaling curve and the fingerprint components that the data supports; overload and recovery results; the bottleneck statement per platform |
| **Stop conditions** | A scaling point missing because of infeasibility (report the gap, do not extrapolate); overload run without a measured baseline; a fingerprint component computed from one point |

### 7.11 Immediate versus blocked summary

| Stage | Immediate | Blocked by |
|---|---|---|
| 1 Owner decisions | Yes | — |
| 2 Baseline and evidence integrity | Yes | — |
| 3 Approved deployment fixes (impl) | Yes | Deploy blocked by SUT reachability |
| 4 FIWARE history (design) | Yes | Deploy blocked by SUT and by the placement decision |
| 5 Resource feasibility (arithmetic) | Yes | Boot blocked by SUT and stage 3 |
| 6 Generator qualification (local) | Partly | Full layout blocked by SUT |
| 7 Gate calibration | Partly | Stages 5–6 per platform |
| 8 k=1 pilot | No | Stages 3, 5, 6, 7 (+4 for FIWARE) |
| 9 Confirmations | No | Stage 8 |
| 10 Scaling/overload | No | Stage 9 and the D11 decision |

## 8. Authorization sheet

**Nothing in this sheet is ticked. Ticking a level authorizes only the scope named for that level in §4; it does not
authorize the next level.** Use one line per ID and tick only the levels the owner intends to authorize now.

Owner: ______________________  Date (UTC): ______________  Initials: ________

| ID | Text | Impl/checks | Deploy | Runs | Notes / conditions recorded by the owner |
|---|---|---|---|---|---|
| D27 FIWARE index | `[ ]` | `[ ]` | `[ ]` | `[ ]` | |
| D28 ThingsBoard config/logging | `[ ]` | `[ ]` | `[ ]` | `[ ]` | |
| D29a EdgeX disk reporting | `[ ]` | `[ ]` | — | — | |
| D29b EdgeX topology ablation | `[ ]` | `[ ]` | `[ ]` | `[ ]` | |
| D29c `synchronous_commit=off` | `[ ]` (keep DIAG-only) | — | — | — | |
| D29d `commit_delay` experiment | `[ ]` | `[ ]` | `[ ]` | `[ ]` | |
| D30 observation-point table | `[ ]` | — | — | — | |
| Orion `-reqPoolSize` | `[ ]` | `[ ]` | `[ ]` | `[ ]` | |
| ThingsBoard queue intervals | `[ ]` | `[ ]` | `[ ]` | `[ ]` | |
| D25-a backport clarification | `[ ]` | — | — | — | |
| D11-a historical edit | `[ ]` | — | — | — | |
| D1-a optional note | `[ ]` | — | — | — | |

### 8.1 Research-critical decisions (separate block)

| Decision | Selected option (write the letter; no option is preselected) | Conditions |
|---|---|---|
| D9 FIWARE history | ______ (A / B / C / D) | Memory and CPU placement: ______________ |
| D11 ThingsBoard feasibility | ______ (A / B / C / D / E) | Feasibility check authorized: `[ ]` yes `[ ]` no |
| Measurement minimum set | ______ (implement / restrict / both / defer) | |
| Generator qualification | ______ (qualify / cap / Go generator / combination) | Qualified ceiling per layout: ______________ |
| Specification pinning (P7) | `[ ]` authorize read-only in-repo copy with hash | Source path: ______________ |
| k versus N (S1) | `[ ]` D26 shared-core meaning · `[ ]` one exclusive core per replica · `[ ]` other: ______ | |
| Headroom rule (S3) | `[ ]` 2× · `[ ]` 1.5× · `[ ]` other: ______ | D24 amendment needed: `[ ]` yes `[ ]` no |

### 8.2 Global acknowledgements

| Acknowledgement | Box |
|---|---|
| I understand that Text, Impl/checks, Deploy and Runs are separate authorizations and that ticking one does not tick the next. | `[ ]` |
| I understand that no decision in this document is adopted until it is recorded in the decision log. | `[ ]` |
| I understand that FIWARE and ThingsBoard cannot currently support the cross-platform durability claim, and that the affected results must be labelled. | `[ ]` |
| I understand that the ack→sink identity join, the per-message storage audit and the cross-host clock offset do not currently exist. | `[ ]` |
| I understand that the generator is not qualified for the full experimental layout, and that any rate above the qualified ceiling is a generator-limited result. | `[ ]` |
| I understand that the repository does not contain the pre-registered SLO/fingerprint text, and that the external notes must be pinned before publication. | `[ ]` |
| I authorize no deployment or run by signing this sheet unless the corresponding Deploy and Runs boxes are individually ticked. | `[ ]` |

### 8.3 Recorded authorizations (2026-10-26, revision 2)

Provenance: **ChatGPT, under the owner's explicit delegation dated 2026-10-26.** The owner did not manually tick
these boxes and no initials or signature are implied. This table is the operative authorization record; §8.1 and
§8.2 are retained as revision 1's blank proposal.

| ID | Text | Impl/checks | Deploy | Runs | Exact limit recorded |
|---|---|---|---|---|---|
| D30 | Yes | — | No | No | Adopt the corrected table; documentation only. |
| D28 | Yes | Yes | No | No | Vendor/mount the pinned official configuration and writable log volume locally; no memory-limit change; no OOM-recovery claim. |
| D29a | Yes | Yes | No | No | Add EdgeX disk-latency reporting; no verdict change. |
| D25-a | Yes | — | No | No | Append the narrow backport clarification; no rewrite of D25. |
| D27 | Yes | Yes | No | No | Add the index-creation step and stopping duplicate audit locally. Index creation on any registry, deployment and runs are not authorized. |
| D11-a | Yes | — | No | No | Append the historical addendum; D11 stays open. |
| D1-a | — | — | No | No | No change. |
| D29b | — | — | No | No | Deferred. |
| D29c | No change | — | No | No | Asynchronous commit remains DIAG-only. |
| D29d | — | — | No | No | Deferred. |
| Orion `-reqPoolSize` | — | — | No | No | Deferred. |
| ThingsBoard queue intervals | — | — | No | No | Deferred. |

**D27 condition (normative).** Duplicate registry keys on `{service, subservice, apikey, id}` must be audited before
any index creation. A duplicate conflict stops the operation and reports the offending identities. No deletion,
merging or rewriting of registry data is authorized.

**No Deploy or Runs level is authorized for any Step 5 candidate.** The local measurement-integrity work recorded
in D33 is a separate authorization and also excludes deployment and live traffic.

## 9. Closing status

### 9.1 Decisions recorded and items still awaiting an owner decision

Revision 1's list below is retained for history. **The operative record is §0**; all Step 5 items listed there are
recorded. The research-critical items that still need an explicit owner decision after this stage are: the D9
data-model verification result, the D11 feasibility outcome and any k-range fallback, and the generator limit if k6
fails the 2× rule.

### 9.2 Technical blockers that remain after any approval

These do not disappear when the decision sheet is signed.

1. **Remote SUT and load hosts are not reachable from this environment.** Last attempt returned no application
   response. Deployment, restart and run stages cannot be executed until access is restored. This checkout was not
   used to probe further for this document.
2. **FIWARE has no history store.** Until stage 4 completes, no FIWARE durability or capacity claim is supportable.
3. **ThingsBoard k=4/N=4 is unproven and does not fit at the exploratory memory values.** Arithmetic alone cannot
   settle it.
4. **The ack→sink→history identity join is implemented locally (D33)** and tested on synthetic known-answer
   fixtures; it has not been run against a live platform. No per-message durable-commit timestamp exists.
5. **No measured cross-host clock offset.** Cross-host deltas are unqualified; primary delivery latency uses the
   generator/sink shared host clock.
6. **The generator is not qualified for the full experimental layout**, and the sink's capacity at the highest
   planned rate is unmeasured.
7. **The pre-registered specification is pinned in-repo as of revision 2** (`research/preregistration/preregistration-archive/`), but S1–S5
   remain methodological amendments that must be carried into every report.
8. **Raw evidence is not versioned.** A manifest with hashes now exists, but a fresh clone still lacks the files.
9. **S1–S5 are recorded but not validated experimentally.** The k meaning, goodput boundary, 2× rule, ThingsBoard
   range and run protocol are reconciled in `docs/methodology/methodology-reconciliation.md`; they still need to survive the
   qualification gates.

### 9.3 Next safely executable task

Revision 2 has completed the decision recording, the pre-registration pinning, the evidence manifest and the local
measurement-integrity implementation. The next safe task is **local preparation only**:

1. Verify the pinned QuantumLeap/TimescaleDB data model, timestamp behaviour and identity query from pinned
   sources; write the D9 verification record. No FIWARE deployment.
2. Run the bounded D11 arithmetic and prepare the k=1/k=2 feasibility check scripts and manifests. No restart.
3. Bind the new `benchctl audit` command into the run procedure and document its use; re-run it on preserved
   evidence only.
4. Prepare the generator qualification plan at 2× the maximum planned rate, including the sink and monitoring
   layout. No load against a platform.
5. Do not deploy, restart, tune or run anything on the SUT.

### 9.4 Criteria for the first main run

A run may be called the first main run only when **all** of the following hold.

| # | Criterion | Evidence required |
|---|---|---|
| 1 | Decision sheet signed; every adopted row recorded in the log | The dated rows |
| 2 | Specification pinned in-repo; S1–S5 resolved by a dated addendum | The pinned text plus the addendum |
| 3 | D30 adopted; every report names its ack, sink and storage points | The adopted table |
| 4 | Measurement minimum set implemented, or claim restrictions formally adopted | The join/audit evidence, or the signed restriction |
| 5 | Generator qualified for the planned rate and layout, with 2× headroom measured | The qualified-rate table |
| 6 | Sink and monitoring sustained at the planned rate | The sink's measured capacity |
| 7 | Supporting-tier accounting fixed and inside the budget | `scripts/check-resources.sh` output for the exact configuration |
| 8 | The platform's history verification passes on a replay, including duplicate and drop injection | The replay record |
| 9 | D9 resolved for FIWARE if FIWARE is in the run; D11 resolved for any k>1 ThingsBoard point | The D9/D11 records |
| 10 | Deployment fixes applied and verified, with deviations recorded | Hashes, mounts, `explain`, smoke results |
| 11 | Cross-host clock offset measured and recorded | The offset and drift record |
| 12 | Run plan frozen with warm-up, step, confirmation and repetition counts matching the protocol | The plan file and its hash |
| 13 | The run manifest will record the git SHA, deviations, layout and qualified rate | The plan and manifest schema |

[P] If any criterion is unmet, the run is a pilot or a diagnostic and must be labelled as such. It must not be
reported as the first main run.

---

## Appendix A — Verification record for this document

| Check | Result |
|---|---|
| Only `docs/qualification/step5-owner-signoff.md` created or modified by this step | Yes; the pre-existing working-tree changes were not touched |
| Packet status still PROPOSED | Yes; `docs/qualification/step5-decision-packet.md` was not edited |
| `docs/design-decisions.md` unedited | Yes |
| Compose files, source, settings, servers, evidence unedited | Yes |
| No commit, no push | Yes |
| Every checkbox unchecked | Yes; all appear as `[ ]` |
| Four-level model preserved, ordering not implication | Yes; stated in the header, §1, §4 and §8 |
| Each candidate has behavior, disposition, alternatives, benefit, dependencies, validation, rollback, scope | Yes; §4.1–§4.12 |
| Blockers separated: D9, D11, measurement, generator | Yes; §5.1–§5.4 |
| Scope freeze distinguishes frozen, conditional and pre-run decisions | Yes; §6.1–§6.3 |
| Roadmap has inputs, authorizations, verification, deliverables, stop conditions | Yes; §7 stages 1–10 |
| Minimum decisions to unblock the next milestone present | Yes; §2 |
| Closing status includes awaiting-approval, blockers, next safe task, first-run criteria | Yes; §9.1–§9.4 |
| Verified facts distinguished from proposals and assumptions | Yes; `[V]`, `[P]`, `[G]` labels |
| No approval inferred anywhere | Yes; stated in the header, §1 and §8 |

**Document status: PROPOSED FOR OWNER REVIEW. Not approved. Not implementation-ready without the decisions in §8.**




