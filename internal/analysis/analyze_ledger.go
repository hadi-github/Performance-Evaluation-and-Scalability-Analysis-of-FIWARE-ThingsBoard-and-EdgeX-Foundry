package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"

	"iotbench/internal/audit"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func validatePlan(r *plan.Resolved) error {
	if r.SchemaVersion != 1 || r.RunID == "" || r.Devices <= 0 || len(r.Steps) == 0 {
		return fmt.Errorf("missing/unsupported resolved schema, identity, devices or steps")
	}
	if r.Arrivals.Model != "constant" && r.Arrivals.Model != "thinning" {
		return fmt.Errorf("unsupported arrival model")
	}
	if !finite(r.SendProb) || r.SendProb <= 0 || r.SendProb > 1 {
		return fmt.Errorf("invalid arrival selection probability")
	}
	if r.Health.MinAchievedRatio <= 0 || r.Health.MinAchievedRatio > 1 || r.Health.SendLagP99Ms <= 0 || r.Health.EarlyToleranceMs < 0 {
		return fmt.Errorf("missing or invalid adopted health gates")
	}
	var seq int64
	var end int
	for i, s := range r.Steps {
		if s.Index != i || s.Name == "" || !finite(s.Rate) || s.Rate <= 0 || !finite(s.TickRate) || s.TickRate <= 0 || s.DurationS <= 0 || s.Ticks <= 0 || s.SeqOffset != seq || s.StartS < end || float64(s.Ticks) != s.TickRate*float64(s.DurationS) {
			return fmt.Errorf("inconsistent resolved schedule at step %d", i)
		}
		if r.Arrivals.Model == "constant" && (s.TickRate != s.Rate || r.SendProb != 1) {
			return fmt.Errorf("constant-arrival rate/probability contradict executor")
		}
		seq += s.Ticks
		end = s.StartS + s.DurationS
	}
	if r.TotalTicks != seq || r.DurationS != end {
		return fmt.Errorf("resolved total ticks/duration contradict steps")
	}
	return nil
}

func validateSummary(raw []byte, r *plan.Resolved, s *report.Summary) error {
	if s.SchemaVersion != 2 || s.RunID != r.RunID || len(s.Steps) != len(r.Steps) {
		return fmt.Errorf("summary schema, run identity or step coverage mismatch")
	}
	var structure struct {
		Steps []map[string]json.RawMessage `json:"steps"`
	}
	_ = json.Unmarshal(raw, &structure)
	var dropped float64
	for i, x := range s.Steps {
		for _, key := range []string{"index", "name", "scenario_start_ms", "attempted", "ack_success", "explicit_failure", "unknown", "early", "beyond_plan", "thinned", "dup_injected", "dropped_iterations", "http_reqs", "send_lag_ms", "ack_latency_ms"} {
			if value, ok := structure.Steps[i][key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return fmt.Errorf("summary step %d missing %s", i, key)
			}
		}
		if x.Index != r.Steps[i].Index || x.Name != r.Steps[i].Name || !finite(x.ScenarioStartMs) || x.ScenarioStartMs <= 0 {
			return fmt.Errorf("inconsistent scenario identity/timestamps")
		}
		for _, key := range []string{"send_lag_ms", "ack_latency_ms"} {
			var stats map[string]json.RawMessage
			_ = json.Unmarshal(structure.Steps[i][key], &stats)
			for _, field := range []string{"count", "p50", "p99", "max"} {
				if value, ok := stats[field]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return fmt.Errorf("summary step %d missing %s.%s", i, key, field)
				}
			}
		}
		if i > 0 && x.ScenarioStartMs <= s.Steps[i-1].ScenarioStartMs {
			return fmt.Errorf("non-increasing scenario timestamps")
		}
		for _, n := range []float64{x.Attempted, x.AckSuccess, x.ExplicitFailure, x.Unknown, x.Early, x.BeyondPlan, x.Thinned, x.DupInjected, x.Dropped, x.HTTPReqs, x.SendLag.Count, x.AckLatency.Count} {
			if !integer(n) {
				return fmt.Errorf("negative, fractional or non-finite counter in step %d", i)
			}
		}
		if x.SendLag.Count != x.Attempted || x.AckLatency.Count != x.AckSuccess {
			return fmt.Errorf("latency observation counts contradict outcomes in step %d", i)
		}
		if x.HTTPReqs != x.Attempted || x.DupInjected > 0 || r.Arrivals.Model == "constant" && x.Thinned > 0 {
			return fmt.Errorf("HTTP sends, injected duplicates or thinning contradict producer in step %d", i)
		}
		if x.Dropped == 0 && int64(x.Attempted) != report.PlannedSends(r, r.Steps[i]) {
			return fmt.Errorf("exact planned-identity set cannot be inferred from attempted count in step %d", i)
		}
		for _, t := range []report.TrendStats{x.SendLag, x.AckLatency} {
			if t.Count > 0 && (!finite(t.P50) || !finite(t.P99) || !finite(t.Max) || t.P50 > t.P99 || t.P99 > t.Max) {
				return fmt.Errorf("invalid percentile distribution in step %d", i)
			}
		}
		dropped += x.Dropped
	}
	if dropped != s.DroppedTotal {
		return fmt.Errorf("summary total dropped counter contradicts per-step counters")
	}
	return nil
}

func validateLedger(r *plan.Resolved, planned []audit.Planned, recs []report.Record, allowSuccessfulRows bool) error {
	bySeq := map[int64]audit.Planned{}
	for _, p := range planned {
		bySeq[p.Seq] = p
	}
	seen := map[int64]bool{}
	for _, rc := range recs {
		p, ok := bySeq[rc.Seq]
		if !ok || seen[rc.Seq] || rc.RunID != r.RunID || rc.Rec != "msg" || rc.Device != int(rc.Seq%int64(r.Devices)) || rc.Step < 0 || rc.Step >= len(r.Steps) || r.Steps[rc.Step].Name != p.Step || rc.Phase != r.Steps[rc.Step].Phase || rc.Tick != rc.Seq-r.Steps[rc.Step].SeqOffset {
			return fmt.Errorf("outcome ledger has duplicate, malformed, unexpected or mismatched identity seq %d", rc.Seq)
		}
		seen[rc.Seq] = true
		if rc.Outcome != "ack_success" && rc.Outcome != "explicit_failure" && rc.Outcome != "unknown" {
			return fmt.Errorf("unknown outcome category at seq %d", rc.Seq)
		}
		if !finite(rc.SchedMs) || !finite(rc.ActualMs) || !finite(rc.RespMs) || math.Abs(rc.SchedMs-p.SchedMs) > .0011 || rc.RespMs < rc.ActualMs {
			return fmt.Errorf("inconsistent outcome timestamps at seq %d", rc.Seq)
		}
		early := rc.ActualMs-rc.SchedMs < -r.Health.EarlyToleranceMs
		if early != rc.Early || !allowSuccessfulRows && rc.Outcome == "ack_success" && !rc.Early {
			return fmt.Errorf("outcome ledger early/success record contradicts producer schema at seq %d", rc.Seq)
		}
	}
	if len(r.TestFaults.DuplicateSeqs) > 0 || r.TestFaults.EarlyEvery > 0 {
		return fmt.Errorf("generator fault injection active; not an unmodified offered-load measurement")
	}
	return nil
}

// validateCompleteOutcomeLedger is the stricter protocol-bound mode. The v1 pilot
// contract relies on one full outcome per offered slot; thinning/implicit NOT_SENT
// attribution is not supported by this ledger and must fail closed.
func validateCompleteOutcomeLedger(r *plan.Resolved, summary *report.Summary, recs []report.Record) error {
	if r.Arrivals.Model != "constant" || len(r.TestFaults.DuplicateSeqs) != 0 || r.TestFaults.EarlyEvery != 0 {
		return fmt.Errorf("complete protocol outcome ledger requires constant arrivals and no generator fault injection")
	}
	if r.TotalTicks < 0 || int64(len(recs)) != r.TotalTicks {
		return fmt.Errorf("complete dispatch/outcome ledger has %d rows for %d planned ticks", len(recs), r.TotalTicks)
	}
	if len(summary.Steps) != len(r.Steps) {
		return fmt.Errorf("complete dispatch/outcome ledger cannot reconcile %d summary steps with %d plan steps", len(summary.Steps), len(r.Steps))
	}
	type counts struct{ ack, failure, unknown int64 }
	byStep := make(map[int]counts, len(r.Steps))
	seen := make(map[int64]bool, len(recs))
	for _, row := range recs {
		if row.Seq < 0 || row.Seq >= r.TotalTicks || seen[row.Seq] {
			return fmt.Errorf("complete dispatch/outcome ledger has duplicate or out-of-range seq %d", row.Seq)
		}
		seen[row.Seq] = true
		c := byStep[row.Step]
		switch row.Outcome {
		case "ack_success":
			c.ack++
		case "explicit_failure":
			c.failure++
		case "unknown":
			c.unknown++
		default:
			return fmt.Errorf("complete dispatch/outcome ledger has unsupported outcome %q at seq %d", row.Outcome, row.Seq)
		}
		byStep[row.Step] = c
	}
	for i, step := range r.Steps {
		ss := summary.Steps[i]
		c := byStep[step.Index]
		if c.ack != int64(ss.AckSuccess) || c.failure != int64(ss.ExplicitFailure) || c.unknown != int64(ss.Unknown) {
			return fmt.Errorf("complete dispatch/outcome ledger counts contradict summary for step %s", step.Name)
		}
		if int64(ss.Attempted) != step.Ticks || int64(ss.Dropped) != 0 || int64(ss.Thinned) != 0 {
			return fmt.Errorf("complete dispatch/outcome ledger has incomplete scheduled coverage for step %s", step.Name)
		}
	}
	return nil
}
