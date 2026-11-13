#!/usr/bin/env python3
"""Assemble a live protocol-receipt capture from complete ledgers and sink evidence.

This program is fail-closed: it writes only a new protocol-receipts.json, never infers
success from missing rows, and never upgrades an incomplete sink window.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path

SEMANTICS = "complete-body-read-before-parse-v1"
INTERPRETATION = "protocol-scheduled-cohort-v1"
OUTCOMES = {"ack_success": "OK", "explicit_failure": "ERROR", "unknown": "UNKNOWN"}
PLATFORMS = {"thingsboard": "tb", "fiware": "fiware", "edgex": "edgex"}
# k6's exec.scenario.startTime reflects executor startup, not an idealized common
# epoch. Retained run summaries show 1–2 ms inter-scenario offset variation. Permit
# up to 5 ms (2.5x observed maximum, still below the existing 10 ms send-lag gate);
# this only validates scenario scheduling metadata and never adjusts message times.
SCENARIO_OFFSET_TOLERANCE_MS = 5.0


def read_json(path):
    return json.loads(Path(path).read_text())


def ndjson(path):
    rows = []
    for line_no, line in enumerate(Path(path).read_text().splitlines(), 1):
        if not line.strip():
            continue
        try:
            rows.append(json.loads(line))
        except json.JSONDecodeError as exc:
            raise ValueError(f"{path}:{line_no}: invalid JSON: {exc}") from exc
    return rows


def finite(value, label):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
        raise ValueError(f"{label} must be finite numeric")
    return float(value)


def integer(value, label):
    if isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value) or not float(value).is_integer():
        raise ValueError(f"{label} must be an integer")
    return int(value)


def assemble(run_dir, sink_path, window_path):
    run_dir = Path(run_dir)
    resolved = read_json(run_dir / "resolved.json")
    manifest = read_json(run_dir / "manifest.json")
    summary = read_json(run_dir / "k6-summary.json")
    window = read_json(window_path)
    binding = manifest.get("protocol_binding")
    pilot = manifest.get("protocol_pilot_parameters")
    dispatch_policy = resolved.get("health", {}).get("workload_policy") in (
        "dispatch-integrity-v1", "dispatch-integrity-v2")
    if not dispatch_policy and resolved.get("health", {}).get("workload_policy") not in (None, ""):
        raise ValueError("unsupported workload policy")
    if dispatch_policy and not binding:
        raise ValueError("unbound dispatch-integrity runs use analyze-dispatch-run.py, not protocol capture assembly")
    if not binding or not pilot:
        raise ValueError("run manifest is not protocol-bound")
    absolute_cutoff = pilot.get("drain_cutoff_unix_ms")
    relative_drain = pilot.get("drain_seconds_after_measurement")
    if manifest.get("synthetic", False) or window.get("synthetic", False):
        raise ValueError("live receipt assembly refuses synthetic artifacts")
    if manifest.get("outcome_ledger_interpretation") != "complete-dispatch-outcome-v1":
        raise ValueError("manifest does not declare complete per-message dispatch/outcome records")
    if manifest.get("run_id") != resolved.get("run_id") or manifest.get("resolved_plan") != resolved:
        raise ValueError("manifest run identity or resolved plan contradicts resolved.json")
    if manifest.get("verdict") != "VALID" or type(manifest.get("k6_exit_code")) is not int or manifest["k6_exit_code"] != 0:
        raise ValueError("benchctl manifest does not attest a valid zero-exit generator run")
    exit_path = run_dir.parent / f"{resolved['run_id']}.benchctl-exit-code"
    try:
        wrapper_exit = int(exit_path.read_text().strip())
    except (OSError, ValueError) as exc:
        raise ValueError("retained wrapper benchctl exit status is missing or malformed") from exc
    if wrapper_exit != 0:
        raise ValueError("retained wrapper benchctl exit status is nonzero")
    if manifest.get("protocol_binding") != resolved.get("protocol_binding") or manifest.get("protocol_pilot_parameters") != resolved.get("protocol_pilot_parameters"):
        raise ValueError("manifest protocol binding/pilot metadata contradicts resolved plan")
    if summary.get("run_id") != resolved.get("run_id"):
        raise ValueError("k6 summary run_id contradicts resolved plan")
    if resolved.get("arrivals", {}).get("model") != "constant" or resolved.get("test_faults", {}).get("duplicate_seqs") or resolved.get("test_faults", {}).get("early_every"):
        raise ValueError("live complete capture requires constant arrivals and no generator fault injection")
    if window.get("schema_version") != 1 or window.get("run_id") != resolved.get("run_id"):
        raise ValueError("capture-window schema or run_id mismatch")
    if window.get("complete") is not True:
        raise ValueError("capture window is not explicitly complete")

    start = finite(window.get("capture_start_ms"), "capture_start_ms")
    end = finite(window.get("capture_end_ms"), "capture_end_ms")
    cutoff = finite(window.get("observation_cutoff_ms"), "observation_cutoff_ms")
    if end < start or end < cutoff:
        raise ValueError("capture window has reversed or incomplete bounds")

    metrics = window.get("sink_metrics")
    required_metrics = ("received_total", "duplicates_total", "unparsed_total", "overflow_total",
                        "incomplete_body_total", "body_read_error_total", "oversized_body_total")
    if not isinstance(metrics, dict) or any(type(metrics.get(k)) is not int for k in required_metrics):
        raise ValueError("capture window must contain all integer sink health counters")
    if any(metrics[k] != 0 for k in ("unparsed_total", "overflow_total", "incomplete_body_total", "body_read_error_total", "oversized_body_total")):
        raise ValueError("sink health counters are nonzero; capture cannot be marked complete")

    steps = resolved.get("steps", [])
    summaries = summary.get("steps", [])
    if len(steps) == 0 or len(summaries) != len(steps):
        raise ValueError("resolved plan and k6 summary step counts disagree")
    if len(resolved.get("steps", [])) != len(summary.get("steps", [])):
        raise ValueError("summary has a different number of steps than the plan")
    expected = {}
    next_seq = 0
    next_start_s = 0
    run_epoch_ms = None
    for step, ss in zip(steps, summaries):
        idx = step["index"]
        if ss.get("index") != idx or ss.get("name") != step.get("name"):
            raise ValueError(f"summary step identity mismatch at index {idx}")
        start_ms = finite(ss.get("scenario_start_ms"), f"step {idx} scenario_start_ms")
        epoch = start_ms - step.get("start_s", 0) * 1000
        if run_epoch_ms is None:
            run_epoch_ms = epoch
        elif abs(epoch - run_epoch_ms) > SCENARIO_OFFSET_TOLERANCE_MS:
            raise ValueError(f"k6 scenario_start_ms contradicts resolved start_s at step {idx}")
        rate = finite(step.get("tick_rate"), f"step {idx} tick_rate")
        ticks = step.get("ticks")
        if type(ticks) is not int or ticks < 0 or rate <= 0:
            raise ValueError(f"invalid resolved tick count/rate for step {idx}")
        if type(step.get("seq_offset")) is not int or step["seq_offset"] != next_seq:
            raise ValueError(f"resolved sequence offsets are not contiguous at step {idx}")
        duration = step.get("duration_s")
        start_s = step.get("start_s")
        if type(duration) is not int or duration <= 0 or type(start_s) is not int or start_s != next_start_s:
            raise ValueError(f"resolved step duration/start schedule is inconsistent at step {idx}")
        if rate * duration != ticks:
            raise ValueError(f"resolved tick_rate × duration contradicts ticks at step {idx}")
        next_start_s += duration
        next_seq += ticks
        if integer(ss.get("attempted"), f"step {idx} attempted") != ticks or integer(ss.get("dropped_iterations"), f"step {idx} dropped_iterations") != 0 or integer(ss.get("thinned"), f"step {idx} thinned") != 0:
            raise ValueError(f"step {idx} does not have complete attempted/no-drop coverage")
        for key in ("ack_success", "explicit_failure", "unknown"):
            integer(ss.get(key), f"step {idx} {key}")
        for tick in range(ticks):
            seq = int(step["seq_offset"]) + tick
            expected[seq] = (idx, step["phase"], tick, seq % int(resolved["devices"]), start_ms + tick * 1000 / rate)
    if next_seq != resolved.get("total_ticks"):
        raise ValueError(f"resolved total_ticks {resolved.get('total_ticks')} contradicts step total {next_seq}")
    if next_start_s != resolved.get("total_duration_s"):
        raise ValueError(f"resolved total_duration_s {resolved.get('total_duration_s')} contradicts step total {next_start_s}")

    events = {}
    for row in ndjson(run_dir / "records.ndjson"):
        seq = row.get("seq")
        if type(seq) is not int or seq in events or row.get("run_id") != resolved["run_id"]:
            raise ValueError("dispatch/outcome ledger has duplicate or malformed run identity")
        if row.get("outcome") not in OUTCOMES:
            raise ValueError(f"unsupported HTTP outcome for seq {seq}")
        if row.get("rec") != "msg":
            raise ValueError(f"non-message record in complete outcome ledger at seq {seq}")
        planned = expected.get(seq)
        if planned is None or row.get("rate_step") != planned[0] or row.get("phase") != planned[1] or row.get("tick") != planned[2] or row.get("device_id") != planned[3]:
            raise ValueError(f"dispatch identity/step/device mismatch at seq {seq}")
        scheduled = finite(row.get("scheduled_send_ms"), f"seq {seq} scheduled_send_ms")
        if abs(scheduled - planned[4]) > .0011:
            raise ValueError(f"dispatch schedule mismatch at seq {seq}")
        actual = finite(row.get("actual_send_ms"), f"seq {seq} actual_send_ms")
        response = finite(row.get("response_ms"), f"seq {seq} response_ms")
        if response < actual:
            raise ValueError(f"response timestamp precedes dispatch at seq {seq}")
        if type(row.get("early")) is not bool:
            raise ValueError(f"dispatch outcome lacks boolean early field at seq {seq}")
        early_expected = actual - scheduled < -finite(resolved.get("health", {}).get("early_tolerance_ms"), "early_tolerance_ms")
        if row["early"] != early_expected:
            raise ValueError(f"early flag contradicts dispatch timestamps at seq {seq}")
        events[seq] = row
    if set(events) != set(expected):
        raise ValueError(f"complete outcome ledger identity set does not cover planned ticks: {len(events)}/{len(expected)}")
    for step, ss in zip(steps, summaries):
        rows = [r for r in events.values() if r["rate_step"] == step["index"]]
        counts = {name: sum(r["outcome"] == name for r in rows) for name in OUTCOMES}
        for outcome in OUTCOMES:
            key = outcome
            if counts[outcome] != integer(ss.get(key), f"step {step['index']} {key}"):
                raise ValueError(f"outcome ledger {outcome} count contradicts summary for step {step['index']}")

    adapter = resolved["adapter"]
    expected_platform = PLATFORMS.get(adapter)
    if expected_platform is None:
        raise ValueError(f"unsupported adapter {adapter}")
    receipts = []
    for row in ndjson(sink_path):
        seq = row.get("seq")
        event = events.get(seq) if type(seq) is int else None
        if row.get("run_id") != resolved["run_id"] or event is None:
            raise ValueError(f"sink receipt has no matching dispatch/outcome row: {row.get('run_id')} seq={seq}")
        if row.get("platform") != expected_platform:
            raise ValueError(f"sink platform mismatch at seq {seq}: {row.get('platform')}")
        if row.get("receipt_capture_semantics") != SEMANTICS:
            raise ValueError(f"sink receipt lacks complete-body semantics at seq {seq}")
        device_id = row.get("device_id")
        expected_device = f"bench-{resolved['run_id']}-{int(event['device_id']):05d}"
        if device_id != expected_device:
            raise ValueError(f"sink device identity mismatch at seq {seq}: {device_id!r} != {expected_device!r}")
        scheduled = finite(event.get("scheduled_send_ms"), f"seq {seq} scheduled_send_ms")
        actual = finite(event.get("actual_send_ms"), f"seq {seq} actual_send_ms")
        observed = finite(row.get("receipt_complete_body_read_ms"), f"seq {seq} complete-body time")
        if observed < start or observed > end:
            raise ValueError(f"sink receipt time lies outside the attested capture window at seq {seq}")
        if observed < actual:
            raise ValueError(f"sink receipt precedes actual dispatch at seq {seq}")
        sink_scheduled = finite(row.get("scheduled_send_ms"), f"seq {seq} sink schedule")
        if abs(scheduled - sink_scheduled) > .0011:
            raise ValueError(f"sink schedule contradicts dispatch ledger at seq {seq}")
        if observed < scheduled:
            raise ValueError(f"sink receipt precedes scheduled send at seq {seq}")
        receipts.append({
            "run_id": resolved["run_id"], "seq": seq, "device_id": device_id,
            "phase": event["phase"], "scheduled_send_ms": scheduled,
            **({"actual_dispatch_ms": actual} if dispatch_policy else {}), "observed_ms": observed,
            "outcome": OUTCOMES[event["outcome"]],
        })
    if len(receipts) != metrics["received_total"]:
        raise ValueError(f"sink ledger has {len(receipts)} rows but metrics report {metrics['received_total']}")
    if relative_drain is not None:
        quiet_window = window.get("quiet_interval")
        late_window = window.get("late_observation")
        if not isinstance(quiet_window, dict) or not isinstance(late_window, dict):
            raise ValueError("relative-cutoff capture lacks explicit quiet or late-observation windows")
        quiet_start = finite(quiet_window.get("start_ms"), "quiet_interval.start_ms")
        if any(quiet_start < r["observed_ms"] <= cutoff for r in receipts):
            raise ValueError("receipt ledger contains arrivals inside the attested quiet interval")
        observed_late = sum(r["observed_ms"] > cutoff for r in receipts)
        declared_late = integer(late_window.get("late_receipt_rows"), "late_observation.late_receipt_rows")
        if observed_late != declared_late:
            raise ValueError("late-observation receipt count contradicts the complete sink ledger")
    sink_rows = ndjson(sink_path)
    identity_counts = {}
    identities_seen = set()
    repeated = 0
    for row in sink_rows:
        identity = (row.get("run_id"), row.get("seq"))
        identity_counts[identity] = identity_counts.get(identity, 0) + 1
        dup = row.get("dup")
        if type(dup) is not bool:
            raise ValueError("every sink receipt must carry a boolean dup flag")
        expected_dup = identity in identities_seen
        if dup != expected_dup:
            raise ValueError(f"sink dup flag contradicts identity order at seq {row.get('seq')}")
        if expected_dup:
            repeated += 1
        identities_seen.add(identity)
    duplicate_rows = sum(row.get("dup") is True for row in sink_rows)
    if duplicate_rows != metrics["duplicates_total"] or repeated != metrics["duplicates_total"]:
        raise ValueError("sink duplicate counter does not match receipt rows; capture completeness is not established")
    if any(count > 1 for count in identity_counts.values()):
        raise ValueError("sink file contains repeated receipt identities; preserve first receipt but capture is not campaign-clean")

    measured = [s for s in resolved["steps"] if s["phase"] == "measurement"]
    if not measured:
        raise ValueError("resolved plan has no measurement step")
    starts = {s["index"]: x.get("scenario_start_ms") for s, x in zip(steps, summaries)}
    phase_start = min(finite(starts[s["index"]], "measurement scenario_start_ms") for s in measured)
    phase_end = max(finite(starts[s["index"]], "measurement scenario_start_ms") + s["duration_s"] * 1000 for s in measured)
    if (absolute_cutoff is None) == (relative_drain is None):
        raise ValueError("pilot must declare exactly one absolute or measurement-relative drain cutoff")
    declared_cutoff = finite(absolute_cutoff, "manifest drain_cutoff_unix_ms") if absolute_cutoff is not None else phase_end + integer(relative_drain, "manifest drain_seconds_after_measurement") * 1000
    if relative_drain is not None and integer(relative_drain, "manifest drain_seconds_after_measurement") <= 0:
        raise ValueError("measurement-relative drain duration must be positive")
    if cutoff != declared_cutoff:
        raise ValueError("capture window contradicts declared drain cutoff")
    if relative_drain is not None:
        quiet_seconds = integer(pilot.get("quiet_interval_seconds"), "manifest quiet_interval_seconds")
        late_seconds = integer(pilot.get("late_observation_seconds"), "manifest late_observation_seconds")
        quiet = window.get("quiet_interval")
        late_window = window.get("late_observation")
        if quiet_seconds <= 0 or late_seconds <= 0 or not isinstance(quiet, dict) or not isinstance(late_window, dict):
            raise ValueError("relative-cutoff capture lacks explicit quiet or late-observation windows")
        quiet_start = finite(quiet.get("start_ms"), "quiet_interval.start_ms")
        quiet_end = finite(quiet.get("end_ms"), "quiet_interval.end_ms")
        late_start = finite(late_window.get("start_ms"), "late_observation.start_ms")
        late_end = finite(late_window.get("end_ms"), "late_observation.end_ms")
        if quiet.get("passed") is not True or quiet_end != cutoff or quiet_end - quiet_start != quiet_seconds * 1000:
            raise ValueError("required quiet interval is not verified immediately before cutoff")
        if late_start != cutoff or late_end != end or late_end - late_start < late_seconds * 1000:
            raise ValueError("late-observation window does not cover the declared post-cutoff interval")
    if relative_drain is not None and (quiet_start < start or quiet_start < phase_end):
        raise ValueError("verified quiet interval begins before capture coverage or measurement end")
    if start > phase_start or cutoff < phase_end:
        raise ValueError("capture does not bracket full measurement phase and observation cutoff")

    return {
        "schema_version": 3 if dispatch_policy else 2,
        "metric_interpretation": "protocol-application-dispatch-cohort-v1" if dispatch_policy else INTERPRETATION,
        "protocol_binding": binding,
        "synthetic": False,
        "complete": True,
        "endpoint": "application_dispatch_to_response_complete_body_read" if dispatch_policy else "response_complete_body_read",
        "source": f"live:cmd/sink NDJSON sha256={hashlib.sha256(Path(sink_path).read_bytes()).hexdigest()}; capture-window sha256={hashlib.sha256(Path(window_path).read_bytes()).hexdigest()}",
        "body_read_complete": True,
        "receipt_capture_semantics": SEMANTICS,
        "clock_domain": "CLOCK_REALTIME shared load-host epoch milliseconds",
        "clock_source": "live: load-host CLOCK_REALTIME; generator and sink share this host clock",
        "coverage_start_ms": start,
        "coverage_end_ms": end,
        "observation_cutoff_ms": cutoff,
        "receipts": receipts,
    }


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--run", required=True, help="existing, complete benchctl run directory")
    p.add_argument("--sink", required=True, help="flushed cmd/sink NDJSON")
    p.add_argument("--window", required=True, help="explicit capture-window attestation JSON")
    a = p.parse_args()
    target = Path(a.run) / "protocol-receipts.json"
    if target.exists():
        p.error(f"refusing to overwrite existing {target}")
    try:
        capture = assemble(a.run, a.sink, a.window)
        with target.open("x") as f:
            json.dump(capture, f, indent=2)
            f.write("\n")
    except (OSError, ValueError, KeyError, TypeError) as exc:
        p.error(str(exc))
    print(target)


if __name__ == "__main__":
    main()
