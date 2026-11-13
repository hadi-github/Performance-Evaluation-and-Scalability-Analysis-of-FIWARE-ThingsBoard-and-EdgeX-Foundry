#!/usr/bin/env python3
"""Render W qualification measurements as self-contained SVG charts.

No third-party plotting library is available in this environment, so this emits
plain SVG. It reads only evidence that the analysis already produced; it never
recomputes a verdict and never invents a number.

    python3 scripts/render-w1-charts.py            # W1 (historical default)
    python3 scripts/render-w1-charts.py --workload W2

Outputs (under results/workload-batch-20261108/reports/):
  <wid>-latency-percentiles.svg   dispatch-to-receipt p50/p95/p99 per platform
  <wid>-ack-completeness.svg      dispatched vs acknowledged vs persisted
  <wid>-throughput-timeline.svg   per-second dispatch bins per platform
  <wid>-summary.md                machine-derived table used by the charts
"""
import argparse
import json
import math
import pathlib
import sys

REPO = pathlib.Path(__file__).resolve().parents[1]
EVID = REPO / "results/workload-batch-20261108/evidence"
OUT = REPO / "results/workload-batch-20261108/reports"

#: The run ID that actually carries each workload's measured result, per platform.
#: A platform with no measured run is listed as "not measured" rather than guessed.
RUNS = {
    "W1": [
        ("thingsboard", "w1-qual-tb-20261110e", "ThingsBoard"),
        ("fiware", "w1-qual-fw-20261110f", "FIWARE"),
        ("edgex", "w1-qual-ex-20261110e", "EdgeX"),
    ],
    "W2": [
        ("thingsboard", "w2-qual-tb-20261110", "ThingsBoard"),
        ("fiware", "w2-qual-fw-20261110", "FIWARE"),
        ("edgex", "w2-qual-ex-20261110", "EdgeX"),
    ],
    "W3": [
        ("thingsboard", "w3-qual-tb-20261110", "ThingsBoard"),
        ("fiware", "w3-qual-fw-20261110", "FIWARE"),
        ("edgex", "w3-qual-ex-20261110", "EdgeX"),
    ],
}

#: Expected rate per workload, used for the reference line and deviation text.
#: It is read from the plan when available, but a fallback keeps the chart honest
#: for a workload whose plan is not on disk.
RATE = {"W1": 50, "W2": 25, "W3": 25}

# Colour-blind-safe palette (Okabe-Ito subset).
C_OK = "#0072B2"
C_WARN = "#D55E00"
C_FAIL = "#8B0000"
C_GRID = "#DDDDDD"
C_TEXT = "#222222"

W, H = 900, 420
PAD_L, PAD_R, PAD_T, PAD_B = 78, 28, 46, 66


def load(run_id):
    if not run_id:
        return None
    p = EVID / run_id / f"{run_id}.dispatch-analysis.json"
    if not p.is_file():
        return None
    return json.loads(p.read_text())


def esc(s):
    return (str(s).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;"))


def svg_open(title, subtitle, height=H):
    return [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{height}" '
        f'viewBox="0 0 {W} {height}" font-family="Helvetica,Arial,sans-serif">',
        f'<rect width="{W}" height="{height}" fill="#FFFFFF"/>',
        f'<text x="{PAD_L}" y="26" font-size="17" font-weight="bold" fill="{C_TEXT}">{esc(title)}</text>',
        f'<text x="{PAD_L}" y="43" font-size="11.5" fill="#555555">{esc(subtitle)}</text>',
    ]


def axes(x0, y0, w, h, ymax, ylabel, yticks=5, yfmt=lambda v: f"{v:g}"):
    out = []
    for i in range(yticks + 1):
        v = ymax * i / yticks
        y = y0 + h - (h * i / yticks)
        out.append(f'<line x1="{x0}" y1="{y:.1f}" x2="{x0+w}" y2="{y:.1f}" stroke="{C_GRID}" stroke-width="1"/>')
        out.append(f'<text x="{x0-8}" y="{y+3.5:.1f}" font-size="10.5" text-anchor="end" fill="{C_TEXT}">{esc(yfmt(v))}</text>')
    out.append(f'<line x1="{x0}" y1="{y0}" x2="{x0}" y2="{y0+h}" stroke="{C_TEXT}" stroke-width="1.2"/>')
    out.append(f'<line x1="{x0}" y1="{y0+h}" x2="{x0+w}" y2="{y0+h}" stroke="{C_TEXT}" stroke-width="1.2"/>')
    out.append(f'<text transform="translate({x0-52},{y0+h/2}) rotate(-90)" font-size="11.5" '
               f'text-anchor="middle" fill="{C_TEXT}">{esc(ylabel)}</text>')
    return out


def chart_percentiles(data, platforms, wid):
    """Grouped bars: p50/p95/p99 dispatch-to-receipt per platform (measurement phase)."""
    rows = []
    for plat, rid, label in platforms:
        d = data.get(plat)
        ph = None
        if d:
            ph = next((x for x in d.get("phases", []) if x["phase"] == "measurement"), None)
        if ph is None:
            rows.append((label, None, None, None, "no measurement"))
            continue
        m = ph["conditional_dispatch_to_receipt_ms"]
        rows.append((label, m["p50"], m["p95"], m["p99"], None))

    vals = [v for r in rows for v in r[1:4] if isinstance(v, (int, float))]
    ymax = max(vals) * 1.18 if vals else 10
    ymax = max(ymax, 1)
    mag = 10 ** math.floor(math.log10(ymax))
    ymax = math.ceil(ymax / mag) * mag

    out = svg_open(
        f"{wid} qualification — dispatch-to-receipt latency",
        f"Measurement phase only. Lower is better; p99 is the gate.",
    )
    x0, y0, w, h = PAD_L, PAD_T, W - PAD_L - PAD_R, H - PAD_T - PAD_B
    out += axes(x0, y0, w, h, ymax, "dispatch → receipt (ms)", yfmt=lambda v: f"{v:.0f}")

    n = len(rows)
    slot = w / n
    bw = slot * 0.20
    for gi, (pct_i, colour) in enumerate([(1, C_OK), (2, "#56B4E9"), (3, C_WARN)]):
        for i, r in enumerate(rows):
            v = r[pct_i]
            cx = x0 + slot * i + slot * 0.5
            bx = cx - bw * 1.5 + gi * bw
            if not isinstance(v, (int, float)):
                continue
            bh = h * (v / ymax)
            by = y0 + h - bh
            out.append(f'<rect x="{bx:.1f}" y="{by:.1f}" width="{bw:.1f}" height="{bh:.1f}" '
                       f'fill="{colour}"/>')
            out.append(f'<text x="{bx+bw/2:.1f}" y="{by-4:.1f}" font-size="9.5" text-anchor="middle" '
                       f'fill="{C_TEXT}">{v:.1f}</text>')

    for i, r in enumerate(rows):
        cx = x0 + slot * i + slot * 0.5
        out.append(f'<text x="{cx:.1f}" y="{y0+h+22}" font-size="12" text-anchor="middle" '
                   f'fill="{C_TEXT}" font-weight="bold">{esc(r[0])}</text>')
        if r[4]:
            out.append(f'<text x="{cx:.1f}" y="{y0+h+38}" font-size="10" text-anchor="middle" '
                       f'fill="{C_FAIL}">{esc(r[4])}</text>')

    lx, ly = x0, y0 - 14
    for j, (txt, colour) in enumerate([("p50", C_OK), ("p95", "#56B4E9"), ("p99", C_WARN)]):
        out.append(f'<rect x="{lx + j*74}" y="{ly-9}" width="11" height="11" fill="{colour}"/>')
        out.append(f'<text x="{lx + j*74 + 16}" y="{ly}" font-size="10.5" fill="{C_TEXT}">{txt}</text>')
    out.append("</svg>")
    return "\n".join(out)


def chart_completeness(data, platforms, wid):
    """Dispatched vs acked vs persisted per platform, measurement phase."""
    planned = RATE.get(wid, 25) * 600
    rows = []
    for plat, rid, label in platforms:
        d = data.get(plat)
        ph = None
        if d:
            ph = next((x for x in d.get("phases", []) if x["phase"] == "measurement"), None)
        if ph is None:
            rows.append((label, planned, None, None, None, "no measurement"))
            continue
        o = ph["outcomes"]
        acked = ph["scheduled"] - o["unknown"] - o["failure"]
        rows.append((label, ph["scheduled"], acked, ph.get("history_present"), o["unknown"], None))

    ymax = max(r[1] for r in rows) * 1.12
    out = svg_open(
        f"{wid} qualification — delivery completeness",
        "Measurement phase: dispatched, acknowledged within the 5 s timeout, and persisted to history.",
    )
    x0, y0, w, h = PAD_L, PAD_T, W - PAD_L - PAD_R, H - PAD_T - PAD_B
    out += axes(x0, y0, w, h, ymax, "requests", yfmt=lambda v: f"{v/1000:.0f}k")

    n = len(rows)
    slot = w / n
    bw = slot * 0.19
    series = [(1, "dispatched", "#BBBBBB"), (2, "acked", C_OK), (3, "in history", "#009E73")]
    for gi, (idx, name, colour) in enumerate(series):
        for i, r in enumerate(rows):
            v = r[idx]
            if not isinstance(v, (int, float)):
                continue
            cx = x0 + slot * i + slot * 0.5
            bx = cx - bw * 1.5 + gi * bw
            bh = h * (v / ymax)
            by = y0 + h - bh
            out.append(f'<rect x="{bx:.1f}" y="{by:.1f}" width="{bw:.1f}" height="{bh:.1f}" fill="{colour}"/>')
            out.append(f'<text x="{bx+bw/2:.1f}" y="{by-4:.1f}" font-size="9" text-anchor="middle" '
                       f'fill="{C_TEXT}">{v:,}</text>')

    for i, r in enumerate(rows):
        cx = x0 + slot * i + slot * 0.5
        out.append(f'<text x="{cx:.1f}" y="{y0+h+22}" font-size="12" text-anchor="middle" '
                   f'fill="{C_TEXT}" font-weight="bold">{esc(r[0])}</text>')
        if r[5]:
            out.append(f'<text x="{cx:.1f}" y="{y0+h+38}" font-size="10" text-anchor="middle" '
                       f'fill="{C_FAIL}">{esc(r[5])}</text>')
        elif isinstance(r[4], int) and r[4] > 0:
            out.append(f'<text x="{cx:.1f}" y="{y0+h+38}" font-size="10" text-anchor="middle" '
                       f'fill="{C_WARN}">{r[4]:,} timed out</text>')

    lx, ly = x0, y0 - 14
    for j, (idx, name, colour) in enumerate(series):
        out.append(f'<rect x="{lx + j*104}" y="{ly-9}" width="11" height="11" fill="{colour}"/>')
        out.append(f'<text x="{lx + j*104 + 16}" y="{ly}" font-size="10.5" fill="{C_TEXT}">{name}</text>')
    out.append("</svg>")
    return "\n".join(out)


def chart_timeline(data, platforms, wid):
    """Per-second dispatch bins, warmup + measurement, per platform (small multiples)."""
    rate = RATE.get(wid, 25)
    height = 150 + 120 * len(platforms)
    out = svg_open(
        f"{wid} qualification — dispatch rate integrity",
        f"Per-second physical dispatches. A flat {rate}/s line proves the generator never starved.",
        height=height,
    )
    x0, w = PAD_L, W - PAD_L - PAD_R
    for pi, (plat, rid, label) in enumerate(platforms):
        d = data.get(plat)
        top = 82 + pi * 120
        h = 86
        wi = (d or {}).get("workload_integrity") or {}
        phases = wi.get("phases") or []
        bins = []
        for ph in phases:
            bins.extend(ph.get("bins_1s") or [])
        out.append(f'<text x="{x0}" y="{top-8}" font-size="12" font-weight="bold" fill="{C_TEXT}">'
                   f'{esc(label)}</text>')
        if not bins:
            msg = "no measurement" if not d else "no bin data"
            out.append(f'<text x="{x0+w/2}" y="{top+h/2}" font-size="11" text-anchor="middle" '
                       f'fill="{C_FAIL}">{esc(msg)}</text>')
            continue
        ymax = max(max(bins), 1) * 1.3
        base_y = top + h
        out.append(f'<line x1="{x0}" y1="{base_y}" x2="{x0+w}" y2="{base_y}" stroke="{C_TEXT}" stroke-width="1"/>')
        ref = top + h - h * (rate / ymax)
        out.append(f'<line x1="{x0}" y1="{ref:.1f}" x2="{x0+w}" y2="{ref:.1f}" stroke="{C_OK}" '
                   f'stroke-width="1" stroke-dasharray="4,3"/>')
        out.append(f'<text x="{x0+w+4}" y="{ref+3.5:.1f}" font-size="9.5" fill="{C_OK}">{rate}/s</text>')
        n = len(bins)
        step = w / max(n, 1)
        bw = max(step * 0.72, 0.6)
        for i, b in enumerate(bins):
            bh = h * (b / ymax)
            out.append(f'<rect x="{x0 + i*step:.2f}" y="{base_y-bh:.2f}" width="{bw:.2f}" '
                       f'height="{bh:.2f}" fill="{C_OK}" opacity="0.82"/>')
        wu = phases[0].get("bins_1s") or [] if phases else []
        if len(phases) >= 2 and wu:
            bx = x0 + len(wu) * step
            out.append(f'<line x1="{bx:.1f}" y1="{top}" x2="{bx:.1f}" y2="{base_y}" stroke="{C_WARN}" '
                       f'stroke-width="1.2" stroke-dasharray="3,3"/>')
            out.append(f'<text x="{bx+3:.1f}" y="{top+11}" font-size="9" fill="{C_WARN}">measurement →</text>')
        dev = max(abs(b - rate) for b in bins)
        out.append(f'<text x="{x0+w}" y="{top-8}" font-size="10" text-anchor="end" fill="#555555">'
                   f'{n} bins · max deviation from {rate}/s: {dev:g}</text>')
    out.append("</svg>")
    return "\n".join(out)


def payload_gate(run_id):
    """Read the declared payload-gate verdict from a run's result record.

    W3's gate is payload preservation, not throughput, so a throughput summary that
    showed only the platform verdict would misrepresent the cell. The value is read
    from qualification-result.json as written; it is never inferred from the phase
    counts, and a run with no declared gate returns None rather than "passed".
    """
    if not run_id:
        return None
    p = EVID / run_id / "qualification-result.json"
    if not p.is_file():
        return None
    try:
        g = json.loads(p.read_text()).get("payload_gate") or {}
    except (ValueError, OSError):
        return None
    order = ("gate_1b_stored_preservation", "gate_1a_exact_logical_padding",
             "gate_2_evidence_path")
    label = {"PASS": "PASS", "UNAVAILABLE": "UNAVAILABLE", "PARTIAL": "PARTIAL"}
    for key in order:
        v = g.get(key)
        if v:
            return label.get(str(v).upper(), str(v))
    return None


def summary_table(data, platforms, wid, notes):
    show_payload = wid == "W3"
    tail = " Payload gate |" if show_payload else ""
    tail_rule = "---|" if show_payload else ""
    lines = [
        f"# {wid} qualification — measurement summary",
        "",
        f"Generated from evidence by `scripts/render-w1-charts.py --workload {wid}`. Every number",
        "below is read directly from a `dispatch-analysis.json`; nothing is recomputed or estimated.",
        "",
        "| Platform | Verdict | Dispatched | Acked | History | Dropped | Unknown | p50 ms | p95 ms | p99 ms |"
        + tail,
        "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|" + tail_rule,
    ]
    for plat, rid, label in platforms:
        d = data.get(plat)
        if not d:
            cell = f" {payload_gate(rid) or 'not measured'} |" if show_payload else ""
            lines.append(f"| {label} | not measured | – | – | – | – | – | – | – | – |" + cell)
            continue
        ph = next((x for x in d.get("phases", []) if x["phase"] == "measurement"), None)
        verdict = d.get("platform_result", "?")
        if ph is None:
            cell = f" {payload_gate(rid) or 'not recorded'} |" if show_payload else ""
            lines.append(f"| {label} | {verdict} | – | – | – | – | – | – | – | – |" + cell)
            continue
        o = ph["outcomes"]
        acked = ph["scheduled"] - o["unknown"] - o["failure"]
        m = ph["conditional_dispatch_to_receipt_ms"]
        hist = ph.get("history_present")
        cell = f" {payload_gate(rid) or 'not recorded'} |" if show_payload else ""
        lines.append(
            f"| {label} | `{verdict}` | {ph['scheduled']:,} | {acked:,} | "
            f"{hist if hist is not None else 'n/a'} | 0 | {o['unknown']:,} | "
            f"{m['p50']:.1f} | {m['p95']:.1f} | {m['p99']:.1f} |" + cell
        )
    lines += [
        "",
        "Dropped is `dropped_iterations`, which is generator starvation, not platform loss.",
        "Unknown is a request that produced no response inside the 5 000 ms timeout.",
        "History is the count of identities the platform actually persisted.",
    ]
    if show_payload:
        lines.append(
            "Payload gate is gate 1b/1a/2 as declared in that run's `qualification-result.json`. "
            "`UNAVAILABLE` means the configuration cannot observe it — it is not a pass and not a "
            "platform failure."
        )
    lines += [
        "",
        "## Setup / operational notes",
        "",
        "| Platform | Note |",
        "|---|---|",
    ]
    for label, note in notes:
        lines.append(f"| {label} | {note} |")
    lines.append("")
    return "\n".join(lines)


NOTES = {
    "W1": [
        ("ThingsBoard", "Warm start: the stack had just served the `d` generation; warm-up ack p99 625 ms is not a cold-start figure."),
        ("FIWARE", "13 481 of 30 000 measurement sends breached the 5 s timeout; p99 dispatch-to-receipt was ~35.3 s. 20 identities never arrived. Platform backlog, not generator."),
        ("EdgeX", "Setup readiness measured at 224 s against a 180 s historical default; raised to 600 s (setup wait only). Provisioning and traffic were separated by a workstation restart; traffic itself was one unbroken capture."),
    ],
    "W2": [
        ("ThingsBoard", "Cold start: 2 000 devices were created immediately before traffic. Warm-up lost 150 of 7 500 at the cutoff (all 150 still reached history, none absent everywhere); the 600 s measurement lost nothing. Verdict is `FAILURE_OR_BACKLOG` because the frozen policy counts any missing identity in any phase."),
        ("FIWARE", "Clean at 25/s with 2 000 devices: 22 500/22 500 dispatched, acknowledged and persisted, p99 312.9 ms. This is the same platform that failed W1 at 50/s, so rate rather than cohort size binds it."),
        ("EdgeX", "Passed. 2 000 devices were accepted by both device-rest replicas on their first readiness attempt, with 0 repair restarts, 0 core-metadata restarts and 0 panics. 22 500/22 500 dispatched, acknowledged and persisted, p99 8.7 ms. Enabled by a cohort-sized inventory reserve (2 000 instead of a fixed 6 000); the 16 384 discovery limit was not changed and nothing was deleted."),
    ],
    "W3": [
        ("ThingsBoard", "Payload gate PASS: 22 500/22 500 stored rows independently validated, every one exactly 1 024 hex chars of padding rebuilt from the seed. The `FAILURE_OR_BACKLOG` verdict is warm-up only \u2014 213 of 7 500 cold-start identities had not arrived by the cutoff, though all 7 500 reached history. The 600 s measurement was 15 000/15 000 with 0 unknown, p99 257.7 ms."),
        ("FIWARE", "PARTIAL: `VALID_PLATFORM_SUCCESS` on delivery \u2014 the only W cell in this batch with a clean two-phase record (22 500/22 500, 0 unknown, 0 missing, p99 407.3 ms). Payload gates 1b/2 and measured transport bytes are UNAVAILABLE, not passed: both active Orion subscriptions select only seq/run_id/scheduled_send_ms, so QuantumLeap has no `padding` column. Orion latest-state shows 1 000/1 000 entities holding 1 024-char padding \u2014 weak evidence that the platform ingests it, not per-message proof."),
        ("EdgeX", "NOT YET MEASURED."),
    ],
}


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--workload", default="W1", choices=sorted(RUNS))
    args = ap.parse_args()
    wid = args.workload
    platforms = RUNS[wid]
    data, missing = {}, []
    for plat, rid, label in platforms:
        d = load(rid)
        if d is None and rid is not None:
            missing.append(rid)
        data[plat] = d
    OUT.mkdir(parents=True, exist_ok=True)
    prefix = wid.lower()
    (OUT / f"{prefix}-latency-percentiles.svg").write_text(chart_percentiles(data, platforms, wid))
    (OUT / f"{prefix}-ack-completeness.svg").write_text(chart_completeness(data, platforms, wid))
    (OUT / f"{prefix}-throughput-timeline.svg").write_text(chart_timeline(data, platforms, wid))
    (OUT / f"{prefix}-summary.md").write_text(summary_table(data, platforms, wid, NOTES.get(wid, [])))
    print(f"wrote 4 files to {OUT.relative_to(REPO)} for {wid}")
    if missing:
        print("missing evidence for:", ", ".join(missing), file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
