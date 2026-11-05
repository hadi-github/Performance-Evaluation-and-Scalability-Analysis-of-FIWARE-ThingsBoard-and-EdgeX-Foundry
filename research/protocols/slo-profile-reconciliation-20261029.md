# SLO-profile reconciliation — TELEM-500 and the deadline sensitivity set

Decision note, dated **2026-10-29**, recorded under the 2026-10-29 working authorization
("SLO profiles remain pending. Do not invent thresholds or produce service-SLO PASS/FAIL verdicts").
The preregistration archive is unchanged; bytes verify with `scripts/check-preregistration.sh`.

## 1. What the preregistration records (unchanged)

`research/preregistration/preregistration-archive/original/slo_notes.txt` (verbatim, not edited here):

- Deadline set: **TELEM-250 / TELEM-500 / TELEM-1000 / TELEM-5000** (T in {250, 500, 1000, 5000} ms);
  `fingerprint_notes.txt` names **TELEM-500 primary**.
- Basis recorded in the preregistration for 500 ms: *"3GPP TS 22.369, Table 6.3-1, smart-grid sensor
  data collection, max end-to-end latency 1 s. The platform is allotted 50 % of it. The 50 % split is
  a researcher assumption, not a 3GPP requirement."*

## 2. Reconciliation with the current status

1. The four deadline values continue to exist **only inside the archive** as preregistered *candidates*.
   No SLO profile is adopted for the benchmark; no TELEM-x gate is active in any harness or runner
   (`benchctl audit -deadline-ms` is a reporting/qualification parameter, not a verdict rule).
2. The preregistered acceptance rule (p99(L_delivery) ≤ T, error rate, missing/dup counters at the sink
   and store) remains **dormant**: evaluations against those thresholds are not run and would not be
   published as campaign verdicts until the owner ratifies at least one profile.
3. Measured intervals that happen to be inside a TELEM-x value are reported as facts
   ("within 1000 ms: N identities"), never labelled PASS; this matches the audit's own label semantics
   (`internal/audit` labels `"qualified … not a platform verdict"`).
4. Status pages and blockers previously marking any deadline number as an adopted threshold are
   corrected by interpretation, not by editing archived or historical text; this note is the
   interpretation record.

## 3. External attribution — UNVERIFIED

- The reference to **3GPP TS 22.369, Table 6.3-1** in the preregistration has **not been checked
  against the cited source** during any session recorded here. Until that check, the attribution is
  treated as **unverified** and no downstream statement may quote the table as support.
- The 50 % split (platform share of the 1 s end-to-end figure) is, by the preregistration's own words,
  a researcher assumption. It is not re-justified here and confers no adoption.

## 4. What would resolve this

Genuinely new (owner-level) decisions still owed before any SLO-based campaign evaluation:

1. Confirm or supersede the four registered deadlines and the TELEM-500 primary choice;
2. Verify (or replace) the external basis after checking the cited source;
3. Decide what the components of an adopted C_SLO are (see `docs/methodology/methodology-reconciliation.md`
   O5/O6 for related open owner decisions: overload anchoring D1 and the uncertainty method D2).

Until those decisions exist, the current status stands exactly as authorized: **SLO profiles pending;
measurement verification and qualification proceed without them.**

## Addendum — owner mappings approved 2026-11-01

The owner has now explicitly approved the three named mappings recorded in
[D44](../../docs/architecture/design-decisions.md#d44--owner-profile-mappings-2026-11-01):
Quick delivery (500 ms scheduled→first-sink p99 primary, 250/1000/5000 sensitivities),
Reliability of saved data (zero acknowledged identities missing from independent history,
zero duplicate stored events in the documented scope/window), and High number of devices
(W2 100-vs-10,000 at matched ~50%/~75% C_SLO(500,1) rates, N=1; no device-capacity cutoff).
The dated [offline policy](offline-profiles-20261101.json) implements those mappings.
This supersedes the pending-mapping status above, not the archived texts, external-source
verification gap or missing reference/capture/qualification evidence. It does not adopt a
complete campaign C_SLO or authorize runs. **Campaign remains NO-GO.**
