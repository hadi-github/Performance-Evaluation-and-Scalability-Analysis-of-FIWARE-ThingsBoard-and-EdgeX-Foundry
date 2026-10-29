# Source manifest — pre-registration archive

**Status:** read-only archive of the original specifications. Created 2026-10-26 (D40).
**Rule:** the files under `original/` are byte-for-byte copies. They are **not** edited to match later
implementation decisions. Differences are explained in `docs/methodology/methodology-reconciliation.md`.

Source directory: `~/catStuff/wrk/iot-performance-evaluation/research/notes/`
Source dates below are the file modification times on the source host (`+0330`) and, where the document declares
one, the version/date in its own text.

| Archive file | Role | Bytes | SHA-256 | Source date (mtime) | Declared in text |
|---|---|---|---|---|---|
| `original/rqs.md` | Research questions, RQ-E/A/B/C/D, shared definitions, protocol | 10,658 | `6fa7e555338e516aa4e6861222f4448116bd1858264a279b67fc14ef322001d6` | 2026-10-16 16:26:55 | "v2, locked Oct 16, 2026" |
| `original/decisions.md` | Framing, RQs, SLO profile, time budget, semantics, hardware, QuantumLeap | 2,477 | `c4078513818aaa4b6eea8177975c79d86384edb52e15e3f18578039ce9bf1571` | 2026-10-16 16:15:23 | decisions v3 (implied) |
| `original/slo_notes.txt` | SLO-TELEM: terms, variants, acceptance rule, backlog, validity, gates | 7,188 | `1290bb4b55e1da000f4e5f2758bb3f70f612d2eb6895454d3277860fc2399da4` | 2026-10-22 19:48:51 | "v0.2, 2026-10-22" |
| `original/slo_components.txt` | SLO component table and criteria | 1,694 | `a1c0f9f293f7ca6b9efb0249a721bdc33760f7df48937f77364caaf016f087c2` | 2026-10-22 19:48:12 | (companion to SLO-TELEM) |
| `original/workloads.txt` | Workload model, W1–W4, payload, overload, generator requirements, gates | 8,384 | `3417304a504857782f663f15062ae46b0af5a1c85f23d092f5c21ed1b8bfe46f` | 2026-10-22 20:26:12 | "candidate v0.1, 2026-10-22" |
| `original/fingerprint_notes.txt` | Fingerprint definitions, resource model, search protocol, overload, open decisions | 9,588 | `81dbc950fa1124e6dd27e066a2ce3ecb48ef2a0af79c19bb8ae7880144fea3c5` | 2026-10-22 20:07:59 | "candidate v0.1, 2026-10-22" |
| `original/fingerprint_metrics.txt` | Fingerprint metric table (short form) | 1,666 | `4f7be910716245083726d8c647e189b943eb01b5706a9e76d0a4cc2ae5dc4099` | 2026-10-22 20:07:35 | (companion to fingerprint notes) |
| `original/metrics_notes.txt` | Metrics specification: control variables, raw record, measured/monitored | 11,046 | `3726ea20ec6a5357fee29e19248bd71890369969d94d16930b898d37c754617e` | 2026-10-22 21:15:11 | "candidate v0.2, 2026-10-22" |
| `original/metrics_list.txt` | Metrics short list | 2,914 | `147121dd11eeb729bfa5f886b1705d3c580b6f2954c69aed69723363acd79f46` | 2026-10-22 21:15:42 | "v0.2, 2026-10-22" |

`SHA256SUMS` is the machine-checkable form of the same hashes. Verify from this directory:

```bash
cd research/preregistration/preregistration-archive && sha256sum -c SHA256SUMS
# or from the repository root:
scripts/check-preregistration.sh
```

## Not included

| Source | Reason |
|---|---|
| `reading-fingerprint.md`, `reading-slo-profiles.md` | Reading lists, not specification text. They contain no threshold or definition used by the experiment. |
| `iot-30-day-pan.md` | Project plan, not a research definition. |
| `hello.md` or other working notes | Not part of the five requested specification classes. |

## Source gaps

None of the nine requested source classes is missing. The archive contains the research questions, SLO profile,
workload, fingerprint and metrics definitions, plus the framing/decisions summary that the RQ text references.

Known provenance limits, recorded rather than repaired:

- The source files have no embedded commit or version-control history; the mtimes and declared versions above are
  the only dates available.
- `decisions.md` does not declare a version string in its text; it is dated by its mtime and its content alignment
  with `rqs.md` v2.
- The source directory is outside the implementation repository. The archive is the in-repo authority from
  2026-10-26 onward; the external directory remains the original location.
