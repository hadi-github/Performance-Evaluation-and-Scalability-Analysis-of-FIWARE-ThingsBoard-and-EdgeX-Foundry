package report

import (
	"encoding/json"
	"math"
	"testing"

	"iotbench/internal/plan"
)

func TestBoundaryOutcomesRemainExplicitFailureSubsets(t *testing.T) {
	r := &plan.Resolved{
		Devices: 1, VUs: plan.VUs{Max: 1}, Arrivals: plan.Arrivals{Model: "constant"},
		Health: plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2},
		Steps:  []plan.Step{{Index: 0, Name: "m", Phase: "measurement", Rate: 1, DurationS: 5, Ticks: 5}},
	}
	s := &Summary{Steps: []StepSummary{{
		Index: 0, Attempted: 5, AckSuccess: 2, ExplicitFailure: 3, Unknown: 0,
		IncompleteResponse: 2, OversizedResponse: 1, HTTPReqs: 5, DataSentBytes: 500,
		SendLag: TrendStats{Count: 5, P99: 1}, AckLatency: TrendStats{Count: 2, P99: 1},
	}}}
	recs := []Record{
		{Rec: "msg", Seq: 1, Step: 0, Outcome: "explicit_failure", Detail: "incomplete_response:body_3_declared_10"},
		{Rec: "msg", Seq: 2, Step: 0, Outcome: "explicit_failure", Detail: "oversized_response:content_length_4194305_gt_4194304"},
		{Rec: "msg", Seq: 3, Step: 0, Outcome: "explicit_failure"},
	}
	if subtype, ok := BoundarySubtype(recs[0].Detail); !ok || subtype != "incomplete_response" {
		t.Fatalf("incomplete boundary detail not recognized: %q %v", subtype, ok)
	}
	if subtype, ok := BoundarySubtype(recs[1].Detail); !ok || subtype != "oversized_response" {
		t.Fatalf("oversized boundary detail not recognized: %q %v", subtype, ok)
	}
	if _, ok := BoundarySubtype(recs[2].Detail); ok {
		t.Fatal("legacy explicit failure relabelled as a boundary outcome")
	}
	h := EvaluateInputs(r, s, recs, nil)
	if !h.Valid {
		t.Fatalf("boundary counters changed legacy accounting: %+v", h)
	}
	got := h.Steps[0].Outcomes
	if got["incomplete_response"] != 2 || got["oversized_response"] != 1 || got["explicit_failure"] != 3 {
		t.Fatalf("boundary outcomes not surfaced as explicit-failure subsets: %+v", got)
	}
}

func TestFromTrend(t *testing.T) {
	q := fromTrend(TrendStats{Count: 100, P50: 1.5, P99: 7.25, Max: 30})
	if q.Count != 100 || q.P50 != 1.5 || q.P99 != 7.25 || q.Max != 30 {
		t.Fatalf("got %+v", q)
	}
	q = fromTrend(TrendStats{})
	if q.Count != 0 || !math.IsNaN(float64(q.P99)) {
		t.Fatalf("empty: %+v", q)
	}
	b, err := json.Marshal(q)
	if err != nil || string(b) != `{"count":0,"p50":null,"p99":null,"max":null}` {
		t.Fatalf("json %s %v", b, err)
	}
}
