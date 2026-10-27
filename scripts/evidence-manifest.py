#!/usr/bin/env python3
"""Non-destructive baseline evidence manifest (D33 / Prompt 4 deliverable 5).

Reads results/** only. Never writes into results/. Produces:
  docs/evidence-integrity-manifest.csv     one row per file with size and SHA-256
  docs/evidence-integrity-manifest.md      per-run summary, gaps, reproduction notes
  docs/evidence-integrity.sha256           sha256sum -c compatible list (repo-relative)

Run from the repository root:  python3 scripts/evidence-manifest.py
"""
from __future__ import annotations

import csv
import hashlib
import json
import os
import subprocess
import sys
from datetime import datetime, timezone

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RESULTS = os.path.join(ROOT, "results")
DOCS = os.path.join(ROOT, "docs")

REQUIRED_IN_RUN = {
    "manifest.json", "plan.json", "resolved.json", "k6-summary.json", "health.json", "records.ndjson",
}
REQUIRED_EXTERNAL = {
    "run.txt", "sink.ndjson", "history.ndjson", "history-final.ndjson", "persist.tsv",
    "gate.json", "check.json", "diag.json", "amendment.json",
}
SUPPORTING_EXTERNAL = {"explore.json", "gen-cpu.ndjson", "cpu.tsv", "prepare.log", "k6-stdout.log"}
RAW_PREFIXES = ("k6-log.ndjson", "k6-messages.log", "k6.json.gz", "target.ndjson")


def rel(path: str) -> str:
    return os.path.relpath(path, ROOT).replace(os.sep, "/")


def sha256(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def load(path: str):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return None


def kind_of(name: str) -> str:
    base = name.split("/")[-1]
    if base in REQUIRED_IN_RUN or any(base.endswith("." + s) for s in REQUIRED_EXTERNAL):
        return "required"
    if base in SUPPORTING_EXTERNAL or base.endswith(".explore.json"):
        return "supporting"
    if any(base.endswith(s) for s in RAW_PREFIXES) or base.endswith(".log"):
        return "raw"
    if base == "case.env" or base == "target-stats.json":
        return "required"
    return "supporting"


def decisions_for(run_id: str, platform: str) -> str:
    tags = []
    if platform == "fiware":
        tags += ["D9 open", "D27 conditional"]
    if platform == "thingsboard":
        tags += ["D11 open"]
    if platform == "edgex":
        tags += ["D23", "D29a"]
    if run_id.startswith("EXPL-"):
        tags.append("D24")
    if run_id.startswith("DIAG-"):
        tags.append("D23 diagnostic")
    return "; ".join(tags) if tags else "—"


def main() -> int:
    if not os.path.isdir(RESULTS):
        print("results/ not found", file=sys.stderr)
        return 2
    head = subprocess.run(["git", "rev-parse", "HEAD"], cwd=ROOT, capture_output=True, text=True).stdout.strip()
    rows = []
    runs = []
    collections = []

    # feasibility is handled below: its cases have no benchctl manifest and a different record shape.
    suites = [d for d in sorted(os.listdir(RESULTS))
              if os.path.isdir(os.path.join(RESULTS, d)) and d != "feasibility"]
    for suite in suites:
        suite_dir = os.path.join(RESULTS, suite)
        top = sorted(os.listdir(suite_dir))
        for name in sorted(os.listdir(suite_dir)):
            run_dir = os.path.join(suite_dir, name)
            if not os.path.isdir(run_dir):
                continue
            man_path = os.path.join(run_dir, "manifest.json")
            manifest = load(man_path) if os.path.exists(man_path) else None
            files = []
            for dirpath, _, names in os.walk(run_dir):
                for n in sorted(names):
                    files.append(os.path.join(dirpath, n))
            if manifest is None:
                # Shared log/support directory (e.g. diag/sut): not a run, no verdict, no required-file gap.
                collections.append({
                    "suite": suite, "name": name, "dir": rel(run_dir), "files": len(files),
                    "bytes": sum(os.path.getsize(f) for f in files),
                })
                for f in sorted(files):
                    rows.append({
                        "suite": suite, "run_id": f"(collection) {name}", "kind": "supporting",
                        "path": rel(f), "bytes": os.path.getsize(f), "sha256": sha256(f),
                        "verdict": "N/A", "git": "",
                    })
                continue
            run_id = manifest.get("run_id", name)
            verdict = manifest.get("verdict", "UNVERIFIED")
            git = (manifest.get("git") or {}).get("sha", "")
            valid = (manifest.get("health") or {}).get("valid")
            platform = run_id.split("-")[1].lower() if "-" in run_id else suite
            # sibling evidence lives in the suite root and starts with "<run_id>."
            prefix = run_id + "."
            for n in top:
                p = os.path.join(suite_dir, n)
                if os.path.isfile(p) and n.startswith(prefix):
                    files.append(p)
            files = sorted(set(files))
            present_required = 0
            missing = []
            for base_name in sorted(REQUIRED_IN_RUN):
                if not os.path.exists(os.path.join(run_dir, base_name)):
                    missing.append(base_name)
                else:
                    present_required += 1
            for suffix in sorted(REQUIRED_EXTERNAL):
                hit = any(os.path.basename(f) == run_id + "." + suffix for f in files)
                if hit:
                    present_required += 1
            explored = os.path.exists(os.path.join(run_dir, "explore.json"))
            runs.append({
                "suite": suite, "run_id": run_id, "dir": rel(run_dir), "verdict": verdict,
                "git": git[:12], "health": valid, "files": len(files), "manifest": True,
                "bytes": sum(os.path.getsize(f) for f in files),
                "missing": missing, "explored": explored, "platform": platform,
                "decisions": decisions_for(run_id, platform),
            })
            for f in files:
                rows.append({
                    "suite": suite, "run_id": run_id, "kind": kind_of(os.path.basename(f)),
                    "path": rel(f), "bytes": os.path.getsize(f), "sha256": sha256(f),
                    "verdict": verdict, "git": git[:12],
                })

    # feasibility cases have no benchctl manifest
    if os.path.isdir(os.path.join(RESULTS, "feasibility")):
        for case in sorted(os.listdir(os.path.join(RESULTS, "feasibility"))):
            case_dir = os.path.join(RESULTS, "feasibility", case)
            if not os.path.isdir(case_dir):
                continue
            files = []
            for dirpath, _, names in os.walk(case_dir):
                for n in sorted(names):
                    files.append(os.path.join(dirpath, n))
            runs.append({
                "suite": "feasibility", "run_id": case, "dir": rel(case_dir), "verdict": "UNVERIFIED",
                "git": "", "health": None, "files": len(files), "manifest": False,
                "bytes": sum(os.path.getsize(f) for f in files), "missing": [],
                "explored": False, "platform": "generator", "decisions": "M0 feasibility; no manifest",
            })
            for f in sorted(files):
                rows.append({
                    "suite": "feasibility", "run_id": case, "kind": kind_of(os.path.basename(f)),
                    "path": rel(f), "bytes": os.path.getsize(f), "sha256": sha256(f),
                    "verdict": "UNVERIFIED", "git": "",
                })

    # aggregate / unattached evidence
    attached = {r["path"] for r in rows}
    for dirpath, _, names in os.walk(RESULTS):
        for n in sorted(names):
            p = os.path.join(dirpath, n)
            if rel(p) in attached:
                continue
            suite = rel(p).split("/")[1] if rel(p).startswith("results/") else "other"
            rows.append({
                "suite": suite, "run_id": "(unattached)", "kind": kind_of(n),
                "path": rel(p), "bytes": os.path.getsize(p), "sha256": sha256(p),
                "verdict": "UNVERIFIED", "git": "",
            })

    rows.sort(key=lambda r: (r["suite"], r["run_id"], r["path"]))
    os.makedirs(DOCS, exist_ok=True)
    with open(os.path.join(DOCS, "evidence-integrity-manifest.csv"), "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=["suite", "run_id", "kind", "path", "bytes", "sha256", "verdict", "git"])
        w.writeheader()
        w.writerows(rows)
    with open(os.path.join(DOCS, "evidence-integrity.sha256"), "w", encoding="utf-8") as f:
        for r in sorted(rows, key=lambda x: x["path"]):
            f.write(f"{r['sha256']}  {r['path']}\n")

    total_bytes = sum(r["bytes"] for r in rows)
    generated = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    explored = [r for r in runs if r["explored"]]
    manifest_runs = [r for r in runs if r["manifest"]]
    feasibility = [r for r in runs if r["suite"] == "feasibility"]
    invalid = [r for r in runs if r["verdict"] == "INVALID"]
    valid = [r for r in runs if r["verdict"] == "VALID"]

    with open(os.path.join(DOCS, "evidence-integrity-manifest.md"), "w", encoding="utf-8") as f:
        f.write("# Baseline evidence-integrity manifest\n\n")
        f.write(f"**Generated:** {generated} from `HEAD` `{head}`. **Read-only audit:** no file under `results/**` "
                "was created, edited, moved or deleted.\n\n")
        f.write("This manifest lists the retained evidence needed to reproduce the exploratory and diagnostic "
                "findings, its hashes, the run verdict and the related decisions. The machine-readable form is "
                "`docs/evidence-integrity-manifest.csv`; the bulk-verification list is "
                "`docs/evidence-integrity.sha256`.\n\n")
        f.write("## Totals\n\n")
        f.write(f"- Files: **{len(rows)}**\n- Bytes: **{total_bytes:,}** ({total_bytes/1024/1024:.1f} MiB)\n")
        f.write(f"- Runs with a `benchctl` manifest: **{len(manifest_runs)}** "
                f"({len(valid)} VALID, {len(invalid)} INVALID)\n")
        f.write(f"- Feasibility cases without a manifest: **{len(feasibility)}** (UNVERIFIED)\n")
        f.write(f"- Runs with `explore.json` analysis: **{len(explored)}**\n")
        f.write(f"- Shared log/support collections (not runs): **{len(collections)}**\n\n")

        f.write("## Per-run inventory\n\n")
        f.write("| Suite | Run | Verdict | Git | Health | Files | MiB | Required gaps | Decisions |\n")
        f.write("|---|---|---|---|---|---|---|---|---|\n")
        for r in sorted(runs, key=lambda x: (x["suite"], x["run_id"])):
            gaps = ", ".join(r["missing"]) if r["missing"] else ("no explore.json" if not r["explored"] and r["suite"] != "feasibility" else "—")
            health = "—" if r["health"] is None else ("valid" if r["health"] else "invalid")
            f.write(f"| {r['suite']} | `{r['run_id']}` | {r['verdict']} | `{r['git'] or '—'}` | {health} | "
                    f"{r['files']} | {r['bytes']/1024/1024:.1f} | {gaps} | {r['decisions']} |\n")

        f.write("\n## Supporting collections (not runs)\n\n")
        f.write("| Suite | Directory | Files | MiB | Role |\n|---|---|---|---|---|\n")
        for c in sorted(collections, key=lambda x: (x["suite"], x["name"])):
            f.write(f"| {c['suite']} | `{c['dir']}` | {c['files']} | {c['bytes']/1024/1024:.1f} | "
                    "Host/container logs and captured diagnostics; no run manifest, so no verdict. |\n")

        f.write("\n## Missing or unverified evidence\n\n")
        f.write("| Item | State | Consequence |\n|---|---|---|\n")
        f.write("| `results/feasibility/*` | No `benchctl` manifest, git revision or health verdict | Generator "
                "feasibility findings are pilot evidence only; they cannot be promoted to a qualified run without "
                "a manifest. |\n")
        f.write("| FIWARE runs | No history export (no QuantumLeap) | No store-side reconciliation; D9/D31 open. |\n")
        f.write("| `EXPL-tb-k4b` | Memory deviation lives in `EXPL-tb-k4b.amendment.json`, not in the manifest | "
                "The amendment is retained and hashed; the original manifest is unchanged. |\n")
        f.write("| Runs without `explore.json` | Analysis not generated or superseded | The raw summary, health and "
                "records remain; re-running `benchctl explore` is possible but was not done here. |\n")
        f.write("| `results/diag/DIAG-edgex-k4-r500` | Superseded by `r500b` (first run lacked exporters) | "
                "Retained; do not use as the primary point. |\n")
        f.write("| `results/exploratory/sut-logs`, `results/diag/sut` | Host/container logs, no run manifest | "
                "Supporting diagnostics only. |\n")
        f.write("| Unattached files | See rows with `(unattached)` in the CSV | Each still has a hash; none was moved. |\n\n")

        f.write("## Reproduction map\n\n")
        f.write("| Finding | Primary evidence | Reproduce with |\n|---|---|---|\n")
        f.write("| Diagnostics report (`docs/diagnostics.md`) | `results/diag/**` | `scripts/diag/report.py` "
                "(reproduces the report except the embedded commit) |\n")
        f.write("| Exploratory capacity (`docs/exploratory-capacity.md`) | `results/exploratory/**` | "
                "`benchctl explore` per run with its sink/history/CPU files |\n")
        f.write("| Generator calibration (`docs/calibration.md`) | `results/feasibility/**` | Re-run the feasibility "
                "cases; no manifest exists, so the raw files are the record |\n")
        f.write("| TB memory deviation (`EXPL-tb-k4b`) | `results/exploratory/EXPL-tb-k4b.amendment.json` + "
                "`sut-logs/tb-prep.*` | Compare `tb-prep.log` memory check with the manifest |\n")
        f.write("| EdgeX WAL / persistence | EdgeX DIAG sink, history, persist and `diag.json` files | "
                "`benchctl explore` with the persistence series |\n\n")

        f.write("## Handling rules\n\n")
        f.write("- Do **not** move, rename, rewrite or delete any file under `results/` to package this evidence. "
                "The manifest is additive.\n")
        f.write(f"- Do **not** stage the {total_bytes/1024/1024:.0f} MiB of raw evidence in Git automatically. If an "
                "archive is needed, create a tar outside the repository or a release asset, and verify it against "
                "`docs/evidence-integrity.sha256`.\n")
        f.write("- A pattern scan found no credential values in the scanned logs, but raw evidence is still not "
                "automatically publishable; review before any external release.\n")
        f.write("- A fresh clone does not contain `results/**` (`.gitignore`), so the manifest proves integrity only "
                "when the files are present.\n\n")
        f.write("## Verification\n\n")
        f.write("```bash\nsha256sum -c docs/evidence-integrity.sha256\n```\n")

    print(f"wrote docs/evidence-integrity-manifest.md, .csv ({len(rows)} files) and evidence-integrity.sha256")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
