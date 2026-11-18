#!/usr/bin/env python3
"""Aggregate independent run analyses without pooling message-level percentiles.

Input is one or more evidence roots laid out as
    <root>/<run_id>/<run_id>.dispatch-analysis.json
    <root>/<run_id>/run-result.json
(also accepts explicit JSON paths). Emits runs.csv and summary.md.

Per-run percentiles stay per-run. The summary reports min/median/max across
latency-assessed runs of a platform and keeps invalid or not-assessed attempts
in the matrix, never averaged or replaced.
"""
import argparse
import csv
import json
import pathlib
import statistics
import sys

PLATFORM_BY_PREFIX = {"tb": "thingsboard", "fw": "fiware", "ex": "edgex"}
FIELDS = [
    "platform", "run_id", "workload_id", "run_kind", "phase", "scheduled", "received_by_cutoff", "missing_by_cutoff",
    "dispatch_p50_ms", "dispatch_p95_ms", "dispatch_p99_ms", "scheduled_p99_ms",
    "duplicates_extra", "late_rows", "outcome_unknown", "outcome_failure",
    "unknown_but_delivered", "acked_but_history_missing",
    "history_present", "history_missing", "history_duplicate_extra",
    "malformed_bodies", "truncated_bodies", "oversized_bodies",
    "measurement_valid", "capture_valid", "latency_assessed", "platform_result",
    "history_status", "workload_valid", "workload_findings",
    "resource_qualified", "sut_resource_qualified", "post_run_defect", "campaign_qualified", "run_result_finish",
    "recovery_classification", "workload_lock_retained", "setup_planned_messages", "setup_offered_messages",
    "harness_defect", "harness_defect_reason",
]


def harness_defect_of(evidence_dir):
    """Return the exclusion reason for a run the bench, not the platform, broke.

    `w4dp-tb-r02-20261108` carries `VALID_PLATFORM_FAILURE_OR_BACKLOG` in its
    run-result, but ThingsBoard ingested all 22,500 messages and the missing hop
    was our own outbound rule-chain node. Keeping it in the W4/thingsboard set
    would inflate that cell to four runs and mix a harness artefact into a
    platform comparison, so it is flagged here and kept out of every repetition
    set and pooled statistic. It stays visible in the CSV as a consumed ID.

    A resolved defect (one that was recovered on disk, e.g. `w2dp-ex-r04`) is
    deliberately NOT excluded: its evidence was re-analysed and is a real
    platform result.
    """
    sidecar = pathlib.Path(evidence_dir) / "harness-defect.json"
    if not sidecar.is_file():
        return None
    defect = json.loads(sidecar.read_text())
    if defect.get("platform_finding") is not False:
        return None
    if defect.get("outcome") != "HARNESS_DEFECT_EXCLUDE_FROM_REPETITIONS":
        return None
    detail = defect.get("defect") if isinstance(defect.get("defect"), dict) else {}
    parts = [defect.get("outcome")]
    if detail.get("component"):
        parts.append("component " + detail["component"])
    if detail.get("missing_rule_node"):
        parts.append("missing rule node " + detail["missing_rule_node"])
    narrative = (defect.get("references") or {}).get("defect_narrative")
    if narrative:
        parts.append("narrative " + narrative)
    return "; ".join(parts)



WORKLOAD_BY_PREFIX = {"w1dp": "W1", "w2dp": "W2", "w3dp": "W3", "w4dp": "W4"}


def workload_of(run_id):
    """W fixed-point attempts are named w{N}dp-{tb|fw|ex}-r{NN}-YYYYMMDD.

    Never inferred from the platform prefix alone: a W1 ThingsBoard attempt and
    the W2 ThingsBoard attempt share a platform but must never be pooled.

    Qualifications are their own workload bucket. They reproduce a W shape but
    are not one of the 60 proposed attempts, so pooling them with W1 would both
    inflate the attempt count and contaminate the fixed-point statistics.
    """
    parts = (run_id or "").split("-")
    if len(parts) >= 2 and parts[0] in WORKLOAD_BY_PREFIX:
        return WORKLOAD_BY_PREFIX[parts[0]]
    if parts and parts[0] == "w1" and len(parts) > 1 and parts[1] == "qual":
        return "W1-QUALIFICATION"
    return None


def platform_of(run_id, result):
    if result.get("platform"):
        return result["platform"]
    parts = (run_id or "").split("-")
    if len(parts) >= 2 and parts[0] == "disp":
        return PLATFORM_BY_PREFIX.get(parts[1])
    if len(parts) >= 2 and parts[0] in WORKLOAD_BY_PREFIX:
        return PLATFORM_BY_PREFIX.get(parts[1])
    if '-dispatch-endpoint-qual-' in (run_id or ''):
        return PLATFORM_BY_PREFIX.get(parts[0])


def run_kind(run_id):
    if '-finalshort' in (run_id or ''):
        return 'short-verification'
    if '-dispatch-endpoint-qual-' in (run_id or ''):
        return 'qualification'
    if workload_of(run_id) == 'W1-QUALIFICATION':
        return 'qualification'
    if workload_of(run_id):
        return 'workload'
    return 'ordinary'


def discover(paths):
    grouped = {}
    for raw in paths:
        p = pathlib.Path(raw)
        if p.is_file():
            candidates = [p] if p.name.endswith(".dispatch-analysis.json") else []
        elif p.is_dir():
            candidates = sorted(p.rglob("*.dispatch-analysis.json"))
        else:
            raise SystemExit(f"input does not exist: {p}")
        for analysis in candidates:
            run_id = analysis.name[:-len(".dispatch-analysis.json")]
            grouped.setdefault(run_id, set()).add(analysis.resolve())
    found = {}
    for run_id, candidates in grouped.items():
        if len(candidates) == 1:
            found[run_id] = next(iter(candidates))
            continue
        canonical = [p for p in candidates if p.parent.name == run_id]
        # Preserved/recovered versions below a canonical attempt are retained,
        # not additional repetitions. Never choose one by alphabetical order.
        if len(canonical) == 1 and all(p == canonical[0] or canonical[0].parent in p.parents for p in candidates):
            found[run_id] = canonical[0]
        else:
            raise SystemExit('ambiguous analyses for '+run_id+'; provide one explicit canonical input')
    return found


def load_run(analysis_path):
    a = json.loads(analysis_path.read_text())
    run_id = a.get("run_id") or analysis_path.name[:-len(".dispatch-analysis.json")]
    result_path = analysis_path.parent / "run-result.json"
    result = json.loads(result_path.read_text()) if result_path.is_file() else {}
    load = a.get("workload_integrity") or {}
    sink = a.get("sink") or {}
    anomalies = sink.get("platform_anomalies") or {}
    platform = platform_of(run_id,result)
    defect_reason = harness_defect_of(analysis_path.parent)
    post_ok = result.get('post_run_defect') is not True
    if platform == 'edgex': post_ok = post_ok and result.get('post_run_check_qualified') is True
    qualified = (a.get('measurement_valid') is True and a.get('capture_valid') is True and
                 a.get('latency_assessed') is True and a.get('generator_resource_qualified') is True and
                 load.get('valid') is True and result.get('sut_resource_qualified') is True and
                 a.get('history_assessed') is True and a.get('history_status') == 'OK' and
                 result.get('history_retrieved') is True and post_ok and
                 defect_reason is None)
    base = {
        "platform": platform_of(run_id, result),
        "run_id": run_id,
        "workload_id": workload_of(run_id),
        "run_kind": run_kind(run_id),
        "measurement_valid": a.get("measurement_valid"),
        "capture_valid": a.get("capture_valid"),
        "latency_assessed": a.get("latency_assessed"),
        "platform_result": 'VALID_PLATFORM_FAILURE_POST_RUN_DEFECT' if result.get('post_run_defect') is True else a.get("platform_result"),
        "history_status": a.get("history_status"),
        "workload_valid": load.get("valid"),
        "workload_findings": ";".join(load.get("findings") or []),
        "resource_qualified": a.get("generator_resource_qualified"),
        "sut_resource_qualified": result.get('sut_resource_qualified'),
        "post_run_defect": result.get('post_run_defect'),
        "campaign_qualified": qualified,
        "harness_defect": defect_reason is not None,
        "harness_defect_reason": defect_reason,
        "run_result_finish": result.get("finish"),
        "workload_lock_retained": result.get('workload_lock_retained'),
        "duplicates_extra": sink.get("duplicate_extra_receipts"),
        "late_rows": sink.get("late_rows_after_cutoff"),
        "malformed_bodies": anomalies.get("malformed_bodies"),
        "truncated_bodies": anomalies.get("truncated_bodies"),
        "oversized_bodies": anomalies.get("oversized_bodies"),
    }
    rows = []
    for phase in a.get("phases") or []:
        dispatch = phase.get("conditional_dispatch_to_receipt_ms") or {}
        scheduled = phase.get("conditional_scheduled_to_receipt_ms") or {}
        outcomes = phase.get("outcomes") or {}
        row = dict(base)
        row.update({
            "phase": phase.get("phase"),
            "scheduled": phase.get("scheduled"),
            "received_by_cutoff": phase.get("received_by_cutoff"),
            "missing_by_cutoff": phase.get("missing_by_cutoff"),
            "dispatch_p50_ms": dispatch.get("p50"),
            "dispatch_p95_ms": dispatch.get("p95"),
            "dispatch_p99_ms": dispatch.get("p99"),
            "scheduled_p99_ms": scheduled.get("p99"),
            "outcome_unknown": outcomes.get("unknown"),
            "outcome_failure": outcomes.get("failure"),
            "unknown_but_delivered": outcomes.get("unknown_but_delivered"),
            "acked_but_history_missing": outcomes.get("acknowledged_but_history_missing"),
            "history_present": phase.get("history_present"),
            "history_missing": phase.get("history_missing"),
            "history_duplicate_extra": phase.get("history_duplicate_extra"),
        })
        rows.append(row)
    return rows


def load_results_only(root):
    """Keep attempts that produced no analysis in the matrix."""
    rows = []
    for result_path in sorted(pathlib.Path(root).rglob("run-result.json")):
        if (result_path.parent / (result_path.parent.name + ".dispatch-analysis.json")).exists():
            continue
        result = json.loads(result_path.read_text())
        defect_reason = harness_defect_of(result_path.parent)
        row = {field: None for field in FIELDS}
        row.update({
            "platform": result.get("platform"),
            "run_id": result.get("run_id"),
            "workload_id": workload_of(result.get("run_id")),
            "run_kind": run_kind(result.get('run_id')),
            "phase": "no-analysis",
            "measurement_valid": result.get("measurement_valid"),
            "capture_valid": result.get("capture_valid"),
            "latency_assessed": result.get("latency_assessed"),
            "platform_result": result.get("platform_result"),
            "run_result_finish": result.get("finish"),
            "workload_lock_retained": result.get('workload_lock_retained'),
            "campaign_qualified": False,
            "harness_defect": defect_reason is not None,
            "harness_defect_reason": defect_reason,
        })
        recovery_path=result_path.parent/'recovery-result.json'
        if recovery_path.is_file():
            recovery=json.loads(recovery_path.read_text())
            if recovery.get('outcome')=='NOT_ASSESSED_SETUP_FAILURE':
                run_dir=result_path.parent/result['run_id']
                ledger=run_dir/'records.ndjson'
                summary=json.loads((run_dir/'k6-summary.json').read_text())
                if (recovery.get('run_id')!=result['run_id'] or recovery.get('original_run_result_preserved') is not True or
                        recovery.get('offered_workload_messages')!=0 or recovery.get('automatic_replacement') is not False or
                        not ledger.is_file() or ledger.read_text().strip() or not summary.get('steps') or
                        any(s.get('attempted')!=0 or s.get('scenario_start_ms')!=0 for s in summary['steps'])):
                    raise ValueError('zero-traffic setup recovery contradicts raw evidence: '+str(recovery_path))
                row.update(phase='setup-failure',platform_result=recovery['outcome'],
                           recovery_classification=recovery['outcome'],
                           setup_planned_messages=recovery.get('planned_messages'),setup_offered_messages=0,
                           run_result_finish=recovery.get('finish'),
                           workload_lock_retained=recovery.get('workload_lock_retained'))
        rows.append(row)
    return rows


def fmt(value):
    return "" if value is None else value


def summarize(rows):
    lines = ["# Ordinary dispatch batch summary", "",
             "Authoritative values come from the offline per-message join. Percentiles are per run; "
             "they are never pooled across runs or messages. Invalid and not-assessed attempts remain "
             "in the matrix.", ""]
    # Group by (workload, platform). A W1 and a W2 attempt on the same platform are
    # different fixed points and must never share a section or a pooled statistic.
    groups = sorted({(r.get("workload_id"), r["platform"]) for r in rows if r["platform"]},
                    key=lambda g: (g[1], g[0] or ""))
    for workload, platform in groups:
        runs = {}
        for row in rows:
            if row["platform"] == platform and row.get("workload_id") == workload:
                runs.setdefault(row["run_id"], []).append(row)
        lines.append(f"## {platform}" + (f" — {workload}" if workload else ""))
        lines.append("")
        lines.append("| Run | Kind | Phase | Valid | Latency assessed | Campaign qualified | Platform result | p99 ms | Received/Scheduled | Missing | Unknown | History missing |")
        lines.append("|---|---|---|---|---|---|---|---:|---|---:|---:|---:|")
        for run_id in sorted(runs):
            for row in runs[run_id]:
                lines.append(
                    f"| {run_id} | {row['run_kind']} | {row['phase']} | {row['measurement_valid']} | {row['latency_assessed']} | {row['campaign_qualified']} | "
                    f"{row['platform_result']} | {fmt(row['dispatch_p99_ms'])} | "
                    f"{fmt(row['received_by_cutoff'])}/{fmt(row['scheduled'])} | "
                    f"{fmt(row['missing_by_cutoff'])} | {fmt(row['outcome_unknown'])} | "
                    f"{'not assessed' if row['history_missing'] is None else row['history_missing']} |")
        measured = sorted(row["dispatch_p99_ms"] for row in rows
                          if row["platform"] == platform and row.get("workload_id") == workload
                          and row["phase"] == "measurement"
                          and row['run_kind'] in ('ordinary', 'workload')
                          and row['campaign_qualified'] is True
                          and row["latency_assessed"] is True and isinstance(row["dispatch_p99_ms"], (int, float)))
        lines.append("")
        for row in rows:
            if row['platform']==platform and row.get('recovery_classification')=='NOT_ASSESSED_SETUP_FAILURE':
                lines.append(f"Setup failure `{row['run_id']}`: **{row['setup_offered_messages']}/{row['setup_planned_messages']} offered/planned telemetry messages**. "
                             "No measurement ran; this is not platform message loss. Original result and recovery sidecar are both preserved. "
                             f"Workload lock retained after recovery: `{row['workload_lock_retained']}`.")
        if measured:
            lines.append(f"Measurement-phase dispatch p99 across {len(measured)} latency-assessed run(s): "
                         f"min {measured[0]:.3f} ms, median {statistics.median(measured):.3f} ms, "
                         f"max {measured[-1]:.3f} ms. Target is three independent repetitions; "
                         f"a cell short of three is a reported gap and is never backfilled silently.")
        else:
            lines.append("No campaign-qualified ordinary measurement phase; diagnostic latency remains in the matrix, but no ordinary replicate percentile is reported.")
        excluded = {row['run_id'] for row in rows
                    if row["platform"] == platform and row.get("workload_id") == workload
                    and row.get("harness_defect") is True}
        if excluded:
            lines.append("")
            lines.append("Excluded from the repetition set and every pooled statistic as a bench defect "
                         "(kept in `runs.csv` as a consumed ID): " + ", ".join(sorted(excluded)) + ".")
        invalid = [row["run_id"] for row in rows if row["platform"] == platform and row["phase"] == "measurement"
                   and row["measurement_valid"] is False]
        if invalid:
            lines.append("")
            lines.append("Invalid attempts kept in the matrix: " + ", ".join(sorted(set(invalid))) + ".")
        lines.append("")
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("inputs", nargs="+", help="evidence roots or dispatch-analysis JSON files")
    ap.add_argument("--out", type=pathlib.Path, help="output directory (default: first input root)")
    ap.add_argument("--force", action="store_true", help="replace existing runs.csv/summary.md")
    args = ap.parse_args()
    out = args.out or (pathlib.Path(args.inputs[0]) if pathlib.Path(args.inputs[0]).is_dir()
                       else pathlib.Path(args.inputs[0]).parent)
    csv_path, md_path = out / "runs.csv", out / "summary.md"
    if not args.force and (csv_path.exists() or md_path.exists()):
        raise SystemExit(f"refusing to overwrite {csv_path} / {md_path} without --force")
    analyses = discover(args.inputs)
    rows = []
    for run_id in sorted(analyses):
        rows.extend(load_run(analyses[run_id]))
    for raw in args.inputs:
        if pathlib.Path(raw).is_dir():
            rows.extend(load_results_only(raw))
    rows.sort(key=lambda r: (r.get("platform") or "", r.get("run_id") or "", r.get("phase") or ""))
    out.mkdir(parents=True, exist_ok=True)
    with csv_path.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=FIELDS)
        w.writeheader()
        for row in rows:
            w.writerow({k: fmt(row.get(k)) for k in FIELDS})
    md_path.write_text(summarize(rows) + "\n")
    print(md_path)
    return 0


if __name__ == "__main__":
    sys.exit(main())
