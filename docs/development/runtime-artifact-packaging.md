# Runtime artifact packaging specification (M2b)

**Status: offline specification only. No packaging command is executed.** This document defines how
a *future, separately authorized* run packages runtime artifacts without rewriting or deleting prior
evidence. It adds no run directory, receiver, ledger, Compose file or live action. It extends the
approved M2 contract and is not a new project or framework.

## 1. Scope and source of truth

The unit of packaging is one **new run identity**. A run packages:

- the documents that justified the run (descriptor, inventory, orchestration plan, protocol/profile);
- the unchanged stack declarations (Compose, `.env`, Makefile) as identity hashes;
- the produced run outputs (manifest, k6 summary, ledgers, receipts, logs, monitoring exports,
  CPU/clock evidence);
- a small set of prior artifacts that must be preserved byte-identically.

The only authoritative identity is content: SHA-256 plus byte count. File modification time, a
directory listing, an HTTP status or an operator note is never a substitute for a content hash.

## 2. Artifact taxonomy

| Role | Meaning | Typical origin |
|---|---|---|
| `descriptor` | Approved M2 deployment descriptor | this run or preserved prior |
| `inventory` | Caller-supplied observed inventory | this run or preserved prior |
| `orchestration_plan` | M2b orchestration blueprint/attestation | this run |
| `compose` | Unchanged `deploy/*/compose.yaml` | preserved prior |
| `env` | `.env.example` or operator `.env` identity | preserved prior or this run |
| `makefile` | Unchanged platform Makefile | preserved prior |
| `protocol` | Frozen protocol definition | preserved prior |
| `profile` | Frozen SLO/protocol profile | preserved prior |
| `run_manifest` | Run identity, plan, timings and declared settings | this run |
| `k6_summary` | k6 summary for the run | this run |
| `sink_ledger` | Sink receipt ledger | this run or preserved prior |
| `adapter_ledger` | Adapter-receiver receipt ledger | this run or preserved prior |
| `receipt_ledger` | Any complete-body receipt ledger | this run or preserved prior |
| `log` | Command/test/operation log | this run |
| `monitoring_export` | Read-only Prometheus/Grafana export | this run |
| `cpu_snapshot` | CPU-boundary snapshot/inventory | this run |
| `clock_bound` | Clock-bound samples | this run |
| `sha256sums` | Non-self-referential checksum list | this run or preserved prior |
| `packet` | Session/evidence packet document | this run |
| `other` | Explicitly reviewed residual category | this run or preserved prior |

A new role requires a schema revision. The decoder rejects an unknown role; it does not silently
classify it as `other`.

## 3. Run identity and path templates

Run identity is `tag + date + run identifier`. The directory name is the run identifier and is never
reused.

| Item | Template | Example |
|---|---|---|
| Run identifier | `{tag}-{YYYYMMDD}-{HHMMSS}+0330` | `m2b-blueprint-20261105-032542+0330` |
| Run directory | `results/{run_id}/` | `results/m2b-blueprint-20261105-032542+0330/` |
| Packet document | `research/archive/sessions/{tag}-{YYYYMMDD}.md` | `research/archive/sessions/m2b-blueprint-20261105.md` |
| Manifest | `results/{run_id}/artifact-manifest.json` | — |
| Checksums | `results/{run_id}/SHA256SUMS` | — |

The `run_id` must begin with `{tag}-{YYYYMMDD}-` followed by a **non-empty** run identifier. An
identifier that stops at the separator (for example `m2b-fix-20261105-`) and an identifier that
repeats the separator (`m2b-fix-20261105--x`) are rejected. The manifest records `run_id`, `run_date`
(`YYYY-MM-DD`), `run_tag` and `platform` (`thingsboard`, `fiware`, `edgex` or `all`). These are
declared identities, not captured timestamps.

Path rules:

1. Paths are repository-relative and use `/`.
2. Absolute paths, `..`, `.` elements, empty elements, backslashes, whitespace and control
   characters are rejected.
3. A `this_run` artifact is under `results/{run_id}/` or `research/archive/sessions/`.
4. A `preserved_prior` artifact is under `results/` or `docs/` and must **not** be under the current
   run directory.
5. A path may appear at most once. Path reuse is a hard failure, not a warning.
6. The manifest and `SHA256SUMS` must not list themselves. Self-referential hashing is prohibited.

## 4. Manifest schema (schema version 1)

```json
{
  "schema_version": 1,
  "interpretation": "runtime-artifact-manifest-v1",
  "run_id": "m2b-blueprint-20261105-032542+0330",
  "run_date": "2026-11-05",
  "run_tag": "m2b-blueprint",
  "platform": "all",
  "created_at_ms": 1762295722000,
  "artifacts": [
    {
      "role": "descriptor",
      "path": "results/m2b-blueprint-20261105-032542+0330/descriptor.json",
      "sha256": "<64 lower-case hex characters>",
      "bytes": 2092,
      "origin": "this_run",
      "required": true
    },
    {
      "role": "sink_ledger",
      "path": "results/m1f-fault-capacity-20261105-013824+0330/sink.ndjson",
      "sha256": "<64 lower-case hex characters>",
      "bytes": 123456,
      "origin": "preserved_prior",
      "required": false
    }
  ]
}
```

`created_at_ms` is optional and is only copied from the caller; the schema never invents it. Every
object is checked against its exact case-sensitive key set before typed decoding. Duplicate keys,
case variants, explicit nulls, unknown keys, trailing JSON, duplicate paths, a bad date, a role
outside the taxonomy and a non-positive byte count fail closed.

`required: true` means the artifact is necessary for the run's attestation. At least one artifact
must be required. `required: false` is a preserved convenience artifact, not an optional permission
to omit a required check.

## 5. Hashing and verification rules

1. Hash the final bytes of the file, after the final write. Do not hash while a writer may still be
   appending.
2. Use lower-case hexadecimal SHA-256 and the actual byte count.
3. Write the manifest after the artifact bytes are final. Do not include the manifest in itself.
4. Write `SHA256SUMS` after the manifest; it may list the manifest and other artifacts but not
   itself.
5. Re-hash every declared artifact during verification. A changed hash or byte count is `FAIL`; an
   unreadable/absent file is `UNAVAILABLE`.
6. Never repair a mismatch by rewriting the declared hash. Recompute only from the actual artifact
   and record the discrepancy.
7. A prior run's artifacts should eventually be verified against their own manifest, not migrated
   into the new manifest's `this_run` set. **This binding is not implemented in M2b.**

`ArtifactManifest.Verify(root)` implements only rule 5, read-only, and only for the paths declared
in the **current** manifest. It does not locate or read a prior run's manifest, so a
`preserved_prior` entry is checked against the SHA-256 and byte count in the current manifest
only. Prior-manifest binding is explicitly marked
`PriorManifestVerificationNotImplemented = NOT_IMPLEMENTED` in `internal/deployment/packaging.go`;
it is a future, separately reviewed packaging workflow. Until that workflow exists, a gate or
packet must record prior-manifest verification as `NOT_ASSESSED` or `UNAVAILABLE`, never `PASS`.
`Verify` never writes, repairs, moves or deletes a file.

## 6. Packaging output layout

A future authorized packaging run must produce a layout of this shape. The names are illustrative;
the rules above are normative.

```text
results/{run_id}/
  artifact-manifest.json        # manifest of the run; not self-listed
  SHA256SUMS                    # final checksum list; not self-listed
  orchestration-plan.json       # optional M2b plan document
  orchestration-plan.md         # deterministic rendering
  deployment-validation.json    # existing M2 dry-run attestation, if used
  deployment-validation.md
  evidence-manifest.json        # existing M2 evidence manifest, if used
  summary.txt
  command-*.log
  command-*.exit
  source-before.sha256
  source-final.sha256
  results-before-retained.sha256
  results-after-retained.sha256
  incremental.patch
  packet-verification.log
  patch-roundtrip.log
  patch-roundtrip-verify.log
  cli-smoke/                    # only if the CLI is exercised read-only
research/archive/sessions/{tag}-{YYYYMMDD}.md
```

The packaging command writes only into a **new** run directory and the agreed packet path. It must
refuse an existing directory, a path equal to an input, and a path under an input evidence tree. It
must not rotate, rename or "clean" an old run.

## 7. Retention, preservation and old receivers/ledgers

| Class | Retention rule |
|---|---|
| Prior run directories | Immutable. Never rewritten, renamed, truncated or deleted. |
| Prior `SHA256SUMS`, manifests and packets | Immutable. A new packet references them by hash. |
| Existing sink/adapter/receipt ledgers | Preserved in place; no truncation, rotation or re-creation. |
| Existing receiver code/configuration | Reused unchanged; no new receiver on an old port/path. |
| New artifacts | New directory only; new names only. |
| Derived indexes/amendments | Separate new file; never edits the raw artifact it indexes. |

If a future run needs a receiver, it must use a new output path and a new ledger. Existing receivers
and ledgers stay exactly as they were. "Continuity" means the old files still verify; it does not
mean appending to them.

## 8. Prohibited actions

The following are prohibited in M2b and in any packaging step that claims M2b semantics:

- Creating a run directory before authorization.
- Copying, moving, renaming, truncating or deleting an old artifact, ledger, receiver output or
  results directory.
- Reusing an old run identifier, manifest path, receiver port or ledger filename.
- Including the manifest or `SHA256SUMS` in itself.
- Replacing a hash with a placeholder, a timestamp, a directory listing or a success message.
- Running `docker compose`, a lifecycle action, a fault, a load or a campaign.
- Writing live values or claiming a live deployment from an offline manifest.
- Treating a missing artifact as `PASS` or zero. It is `UNAVAILABLE`.

## 9. Interrupted packaging

If packaging is interrupted before the manifest and checksums are complete, the partial new
directory must remain identifiable and must not be merged into a prior run. A future authorized
operator may mark it incomplete; M2b itself performs no cleanup, rollback or delete. The interruption
is recorded as `UNAVAILABLE`, not hidden by rewriting an old manifest.

## 10. Gate relationship

A packaging packet may be used at the M2b review gate only when:

- the run identity is new and matches the tag/date/run-id rule;
- every `this_run` artifact is under the new run directory or the packet directory;
- every `preserved_prior` artifact is checked against the current manifest's declared hash; binding
  it to its own prior manifest is **not implemented** in M2b and must be recorded
  `NOT_ASSESSED`/`UNAVAILABLE` until a future reviewed workflow supplies that check;
- the manifest and `SHA256SUMS` are non-self-referential;
- prior results excluding the current milestone remain byte-identical;
- protocol/profile hashes remain `7176b1fa…d24ee` and `1b6aa76d…d1f8`;
- no live/lifecycle claim appears in the packet.

No value is fabricated to satisfy the gate. An unavailable value stays `UNAVAILABLE` or
`NOT_ASSESSED` and blocks a `PASS` claim.
