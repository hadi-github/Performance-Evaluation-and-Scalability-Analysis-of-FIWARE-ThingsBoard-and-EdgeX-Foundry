package reconcile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

const testRunID = "SYNTHETIC-reconcile-boundary"

func writeRun(t *testing.T, detail string, outcome string) string {
	t.Helper()
	d := t.TempDir()
	r := plan.Resolved{
		Plan: plan.Plan{SchemaVersion: 1, PlanID: "boundary", Purpose: "synthetic", Seed: 17, Adapter: "fixture",
			Devices: 2, RequestTimeMs: 1000, Connection: plan.Connection{KeepAlive: true},
			Arrivals: plan.Arrivals{Model: "constant"}, VUs: plan.VUs{PreAllocated: 5, Max: 5},
			Health: plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2}},
		RunID: testRunID, TotalTicks: 10, DurationS: 1, SendProb: 1,
		Steps: []plan.Step{{Index: 0, Name: "s00-measure-r10", Phase: "measurement", Rate: 10, TickRate: 10, DurationS: 1, Ticks: 10}},
	}
	t0 := 1700000000000.0
	sum := report.Summary{SchemaVersion: 2, RunID: testRunID, Steps: []report.StepSummary{{
		Index: 0, Name: r.Steps[0].Name, ScenarioStartMs: t0, Attempted: 10, AckSuccess: 9, ExplicitFailure: 1, HTTPReqs: 10,
		SendLag: report.TrendStats{Count: 10, P50: 1, P99: 2, Max: 3}, AckLatency: report.TrendStats{Count: 9, P50: 20, P99: 30, Max: 40},
	}}}
	rc := report.Record{Rec: "msg", RunID: testRunID, Seq: 9, Device: 1, Step: 0, Phase: "measurement", Tick: 9,
		SchedMs: t0 + 900, ActualMs: t0 + 901, RespMs: t0 + 920, Outcome: outcome, Detail: detail}
	for name, value := range map[string]any{"resolved.json": r, "k6-summary.json": sum} {
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(d, name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := json.Marshal(rc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(d, "records.ndjson"), append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	return d
}

func stored(seqs ...int64) []Stored {
	var out []Stored
	for _, seq := range seqs {
		out = append(out, Stored{RunID: testRunID, Seq: seq})
	}
	return out
}

func allStored() []Stored {
	var out []Stored
	for seq := int64(0); seq < 10; seq++ {
		out = append(out, Stored{RunID: testRunID, Seq: seq})
	}
	return out
}

func TestBoundaryFailureRetainsAccountingAndCleanliness(t *testing.T) {
	for _, tc := range []struct{ name, detail string }{
		{"incomplete", "incomplete_response:body_3_declared_10"},
		{"incomplete transport", "incomplete_response:transport_response_body_read"},
		{"oversized", "oversized_response:content_length_4194305_gt_4194304"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := writeRun(t, tc.detail, "explicit_failure")
			missing, err := Run(d, stored(0, 1, 2, 3, 4, 5, 6, 7, 8))
			if err != nil {
				t.Fatal(err)
			}
			if missing.Planned != missing.AckSuccess+missing.ExplicitFailure+missing.Unknown || missing.ExplicitFailure != 1 ||
				missing.Clean || len(missing.BoundaryNotStored) != 1 || missing.BoundaryNotStored[0] != 9 {
				t.Fatalf("unstored boundary failure not explicit: %+v", missing)
			}
			if len(missing.PersistedDespiteFailure) != 0 {
				t.Fatalf("unstored boundary failure counted as persisted: %+v", missing)
			}
			present, err := Run(d, allStored())
			if err != nil {
				t.Fatal(err)
			}
			if present.Planned != present.AckSuccess+present.ExplicitFailure+present.Unknown || present.ExplicitFailure != 1 ||
				!present.Clean || len(present.BoundaryNotStored) != 0 || len(present.PersistedDespiteFailure) != 1 || present.PersistedDespiteFailure[0] != 9 {
				t.Fatalf("stored boundary failure not clean or not persisted: %+v", present)
			}
		})
	}
}

func TestLegacyExplicitFailureNotStoredStaysClean(t *testing.T) {
	d := writeRun(t, "", "explicit_failure")
	res, err := Run(d, stored(0, 1, 2, 3, 4, 5, 6, 7, 8))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Clean || len(res.BoundaryNotStored) != 0 || res.ExplicitFailure != 1 {
		t.Fatalf("legacy explicit failure semantics changed: %+v", res)
	}
}

func TestRawBoundaryCategoryIsRejectedNotDropped(t *testing.T) {
	for _, outcome := range []string{"incomplete_response", "oversized_response"} {
		d := writeRun(t, "body_3_declared_10", outcome)
		if _, err := Run(d, allStored()); err == nil {
			t.Fatalf("raw boundary category %q silently accepted", outcome)
		}
	}
}
