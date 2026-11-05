package analysis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func approvedAnalysis(t *testing.T, d string) *Result {
	t.Helper()
	r, e := Analyze(Options{Run: d, Evidence: filepath.Join(d, "evidence.json")})
	if e != nil {
		t.Fatal(e)
	}
	return r
}

func TestOwnerPolicyFileMatchesDefaults(t *testing.T) {
	b, e := os.ReadFile("../../research/protocols/offline-profiles-20261101.json")
	if e != nil {
		t.Fatal(e)
	}
	var policy Policy
	if e = decode(b, &policy, true); e != nil {
		t.Fatal(e)
	}
	if e = validatePolicy(policy); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(policy, DefaultPolicy()) {
		t.Fatal("approved policy and built-in defaults diverged")
	}
}

func TestOwnerDeliveryPrimaryAndSensitivities(t *testing.T) {
	for _, tc := range []struct {
		latency float64
		primary Status
		strict  Status
	}{{300, Pass, Fail}, {500, Pass, Fail}, {700, Fail, Fail}, {10, Pass, Pass}} {
		t.Run(fmt.Sprint(tc.latency), func(t *testing.T) {
			d := fixture(t)
			rewriteRows(t, d, "sink.ndjson", func(rows []map[string]any) []map[string]any {
				for _, row := range rows {
					row["recv_ms"] = 1700000000000 + row["seq"].(float64)*100 + tc.latency
				}
				return rows
			})
			r := approvedAnalysis(t, d)
			if profileStatus(t, r, "quick_delivery") != tc.primary {
				t.Fatal("sensitivity changed primary verdict")
			}
			p := r.Steps[0].Profiles[0]
			if len(p.SensitivityChecks) != 3 || p.SensitivityChecks[0].Status != tc.strict || p.SensitivityChecks[1].Status != Pass || p.SensitivityChecks[2].Status != Pass {
				t.Fatal(p)
			}
			if e := Write(filepath.Join(t.TempDir(), "report"), r, d); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestOwnerMissingEvidenceAndWindow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(string)
		profile string
	}{
		{"history", func(d string) {
			if e := os.Remove(filepath.Join(d, "history.ndjson")); e != nil {
				t.Fatal(e)
			}
		}, "saved_data_reliability"},
		{"window", func(d string) { modifyEvidence(t, d, func(e *Evidence) { e.ObservationDeadlineMS = 0 }) }, "saved_data_reliability"},
		{"missing-capture-start", func(d string) { modifyEvidence(t, d, func(e *Evidence) { e.History.StartMS = 0 }) }, "saved_data_reliability"},
		{"sink", func(d string) {
			if e := os.Remove(filepath.Join(d, "sink.ndjson")); e != nil {
				t.Fatal(e)
			}
		}, "quick_delivery"},
		{"clock", func(d string) { modifyEvidence(t, d, func(e *Evidence) { e.SinkClock = "" }) }, "quick_delivery"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture(t)
			tc.mutate(d)
			r := approvedAnalysis(t, d)
			if profileStatus(t, r, tc.profile) != NotAssessed || r.Claims["offered_load"].Status != Pass {
				t.Fatal(r.Steps[0].Profiles)
			}
		})
	}
	for _, kind := range []string{"missing", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			d := fixture(t)
			rewriteRows(t, d, "history.ndjson", func(rows []map[string]any) []map[string]any {
				if kind == "missing" {
					return rows[1:]
				}
				return append(rows, rows[0])
			})
			r := approvedAnalysis(t, d)
			if profileStatus(t, r, "saved_data_reliability") != Fail || profileStatus(t, r, "quick_delivery") != Pass {
				t.Fatal(r.Steps[0].Profiles)
			}
		})
	}
	d := fixture(t)
	r := approvedAnalysis(t, d)
	if r.Claims["w2_reference"].Status != NotAssessed || !strings.Contains(r.Claims["w2_reference"].Reasons[0], "capacity_messages_per_second") {
		t.Fatal("missing reference not identified")
	}
}

func w2Fixture(t *testing.T, devices, rate int) *Result {
	t.Helper()
	d := fixture(t)
	r := load[plan.Resolved](t, d, "resolved.json")
	r.Devices = devices
	r.RunID = fmt.Sprintf("SYNTHETIC-W2-%d-%d", devices, rate)
	r.TotalTicks = int64(rate)
	r.Steps[0].Ticks = int64(rate)
	r.Steps[0].Rate = float64(rate)
	r.Steps[0].TickRate = float64(rate)
	save(t, d, "resolved.json", r)
	m := load[map[string]any](t, d, "manifest.json")
	m["run_id"] = r.RunID
	m["resolved_plan"] = r
	save(t, d, "manifest.json", m)
	modifySummary(t, d, func(s *report.Summary) {
		s.RunID = r.RunID
		s.Steps[0].Attempted = float64(rate)
		s.Steps[0].AckSuccess = float64(rate)
		s.Steps[0].HTTPReqs = float64(rate)
		s.Steps[0].SendLag.Count = float64(rate)
		s.Steps[0].AckLatency.Count = float64(rate)
	})
	var sink, history strings.Builder
	for i := 0; i < rate; i++ {
		fmt.Fprintf(&sink, "{\"run_id\":%q,\"seq\":%d,\"recv_ms\":%.3f}\n", r.RunID, i, 1700000000000+float64(i)*1000/float64(rate)+10)
		fmt.Fprintf(&history, "{\"run_id\":%q,\"seq\":%d,\"device\":\"device-%d\"}\n", r.RunID, i, i%devices)
	}
	text(t, d, "sink.ndjson", sink.String())
	text(t, d, "history.ndjson", history.String())
	reference := []byte("SYNTHETIC ONLY reference C_SLO(500,1) = 20000 messages/s\n")
	text(t, d, "reference.txt", string(reference))
	capacity := 20000.0
	modifyEvidence(t, d, func(e *Evidence) {
		e.CPU = nil
		e.W2Reference = &W2Reference{Capacity: &capacity, Definition: "C_SLO(500,1)", Source: "synthetic test only", Path: "reference.txt", SHA256: fmt.Sprintf("%x", sha256.Sum256(reference))}
	})
	return approvedAnalysis(t, d)
}

func copyResult(t *testing.T, r *Result) *Result {
	t.Helper()
	b, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var copy Result
	if e = json.Unmarshal(b, &copy); e != nil {
		t.Fatal(e)
	}
	return &copy
}

func TestOwnerW2MatchedComparison(t *testing.T) {
	var results []*Result
	for _, rate := range []int{10000, 15000} {
		for _, devices := range []int{100, 10000} {
			results = append(results, w2Fixture(t, devices, rate))
		}
	}
	c := Compare(results)
	if len(c.DeviceComparisons) != 1 || c.DeviceComparisons[0].DesignValidity.Status != Pass || len(c.DeviceComparisons[0].Cells) != 4 {
		t.Fatal(c.DeviceComparisons)
	}
	study := c.DeviceComparisons[0]
	if study.ProfileVerdict.Status != NotAssessed {
		t.Fatal("W2 invented device-capacity cutoff")
	}
	for _, r := range results {
		if profileStatus(t, r, "high_device_count") != NotAssessed {
			t.Fatal("single run became W2 verdict")
		}
	}
	t.Run("explicit-approximate-rate-keeps-dimensions", func(t *testing.T) {
		var copies []*Result
		for _, r := range results {
			copy := copyResult(t, r)
			copy.CaptureSpecification.W2Reference.RatePoints = []W2RatePoint{{Fraction: .5, Rate: 9999, Source: "explicit synthetic rounded rate"}}
			if copy.Steps[0].OfferedRate == 10000 {
				copy.Steps[0].OfferedRate = 9999
			}
			copies = append(copies, copy)
		}
		comparison := Compare(copies)
		if len(comparison.DeviceComparisons) != 1 || comparison.DeviceComparisons[0].DesignValidity.Status != Pass {
			t.Fatal(comparison.DeviceComparisons)
		}
		for _, cell := range comparison.DeviceComparisons[0].Cells {
			if *cell.ReferenceFraction == .5 && (cell.OfferedRate != 9999 || *cell.ActualReferenceRatio != .49995) {
				t.Fatal("rate or device dimension overwritten", cell)
			}
		}
	})
	for _, g := range c.Groups {
		for _, row := range c.Rows {
			if row.Group == g.Key && row.Profile.DeviceComparison != nil {
				t.Fatal("W2 entered capacity groups")
			}
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func([]*Result) []*Result
	}{
		{"missing-population", func(r []*Result) []*Result { return r[:3] }},
		{"duplicate-not-repetition", func(r []*Result) []*Result { return []*Result{r[0], r[0], r[2], r[3]} }},
		{"unmatched-rate", func(r []*Result) []*Result { r[0].Steps[0].OfferedRate = 9999; return r }},
		{"N-not-one", func(r []*Result) []*Result { r[0].Configuration.Replicas["application"] = 2; return r }},
		{"unobserved-population", func(r []*Result) []*Result {
			v := 99.0
			m := r[0].Steps[0].Metrics["workload.observed_history_devices"]
			m.Value = &v
			r[0].Steps[0].Metrics["workload.observed_history_devices"] = m
			return r
		}},
		{"missing-window", func(r []*Result) []*Result {
			r[0].Steps[0].Claims["history"] = claim(NotAssessed, "observation deadline missing")
			return r
		}},
		{"incompatible-image", func(r []*Result) []*Result {
			r[0].Configuration.Images["application"] = "other@sha256:" + strings.Repeat("f", 64)
			return r
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var copies []*Result
			for _, r := range results {
				copies = append(copies, copyResult(t, r))
			}
			c := Compare(tc.mutate(copies))
			for _, s := range c.DeviceComparisons {
				if s.DesignValidity.Status == Pass {
					t.Fatal("incomplete or incompatible design accepted", s)
				}
			}
		})
	}
	if e := WriteComparison(filepath.Join(t.TempDir(), "comparison"), c, nil); e != nil {
		t.Fatal(e)
	}
}

func TestW2ReferenceValidationAndExplicitApproximation(t *testing.T) {
	d := fixture(t)
	rd := reader{result: &Result{}, base: d}
	capacity := 100.0
	ref := &W2Reference{Capacity: &capacity, Definition: "C_SLO(500,1)", Source: "synthetic", Path: "ref.txt", SHA256: strings.Repeat("0", 64)}
	if validateW2Reference(rd, ref).Status != NotAssessed {
		t.Fatal("missing reference artifact accepted")
	}
	text(t, d, "ref.txt", "synthetic evidence")
	if validateW2Reference(rd, ref).Status != Invalid {
		t.Fatal("hash mismatch accepted")
	}
	ref.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("synthetic evidence")))
	ref.RatePoints = []W2RatePoint{{Fraction: .5, Rate: 49, Source: "explicit synthetic rounded rate"}}
	if validateW2Reference(rd, ref).Status != Pass {
		t.Fatal("sourced approximate rate rejected")
	}
	ref.RatePoints = append(ref.RatePoints, ref.RatePoints[0])
	if validateW2Reference(rd, ref).Status != Invalid {
		t.Fatal("duplicate rate point accepted")
	}
}
