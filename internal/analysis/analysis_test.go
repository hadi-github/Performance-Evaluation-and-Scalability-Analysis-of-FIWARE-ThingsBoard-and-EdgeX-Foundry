package analysis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"iotbench/internal/audit"
	"iotbench/internal/plan"
	"iotbench/internal/reconcile"
	"iotbench/internal/report"
)

func fixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "input")
	if e := Synthetic(p, "SYNTHETIC-test", 2); e != nil {
		t.Fatal(e)
	}
	return p
}
func load[T any](t *testing.T, d, name string) T {
	t.Helper()
	var v T
	b, e := os.ReadFile(filepath.Join(d, name))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func save(t *testing.T, d, name string, v any) {
	t.Helper()
	if e := writeJSON(filepath.Join(d, name), v); e != nil {
		t.Fatal(e)
	}
}
func text(t *testing.T, d, name, value string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(d, name), []byte(value), 0644); e != nil {
		t.Fatal(e)
	}
}
func analyse(t *testing.T, d string) *Result {
	t.Helper()
	r, e := Analyze(Options{Run: d, Evidence: filepath.Join(d, "evidence.json"), Policy: filepath.Join(d, "profiles.json")})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func profileStatus(t *testing.T, r *Result, id string) Status {
	t.Helper()
	for _, p := range r.Steps[0].Profiles {
		if p.Profile.ID == id {
			return p.Status
		}
	}
	t.Fatal("profile missing", id)
	return ""
}
func modifySummary(t *testing.T, d string, f func(*report.Summary)) {
	t.Helper()
	s := load[report.Summary](t, d, "k6-summary.json")
	f(&s)
	save(t, d, "k6-summary.json", s)
}
func modifyEvidence(t *testing.T, d string, f func(*Evidence)) {
	t.Helper()
	e := load[Evidence](t, d, "evidence.json")
	f(&e)
	save(t, d, "evidence.json", e)
}
func rewriteRows(t *testing.T, d, name string, f func([]map[string]any) []map[string]any) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(d, name))
	if e != nil {
		t.Fatal(e)
	}
	list, e := rows[map[string]any](b)
	if e != nil {
		t.Fatal(e)
	}
	var out strings.Builder
	for _, v := range f(list) {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		out.Write(b)
		out.WriteByte('\n')
	}
	text(t, d, name, out.String())
}

func TestSyntheticPassReadOnlyAndRepeatable(t *testing.T) {
	d := fixture(t)
	before := treeHash(t, d)
	r := analyse(t, d)
	for _, name := range []string{"offered_load", "comparability"} {
		if r.Claims[name].Status != Pass {
			t.Fatalf("%s: %+v", name, r.Claims[name])
		}
	}
	for _, name := range []string{"ack_latency", "delivery_latency", "sink", "history", "cpu_cost"} {
		if r.Steps[0].Claims[name].Status != Pass {
			t.Fatalf("%s: %+v", name, r.Steps[0].Claims[name])
		}
	}
	for _, id := range []string{"quick_delivery", "saved_data_reliability", "high_device_count"} {
		if profileStatus(t, r, id) != Pass {
			t.Fatal(id)
		}
	}
	if r.Steps[0].Claims["durable_latency"].Status != NotAssessed || r.CampaignVerdict != "NOT_ASSESSED" {
		t.Fatal("invented durable/campaign claim")
	}
	if v := *r.Steps[0].Metrics["cpu.seconds_per_message"].Value; v != .15 {
		t.Fatal(v)
	}
	out := filepath.Join(t.TempDir(), "report")
	if e := Write(out, r, d); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, treeHash(t, d)) {
		t.Fatal("evidence changed")
	}
	r2 := analyse(t, d)
	b, _ := json.Marshal(r)
	b2, _ := json.Marshal(r2)
	if string(b) != string(b2) {
		t.Fatal("analysis not deterministic")
	}
	if e := Write(out, r, d); e == nil {
		t.Fatal("overwrote existing output")
	}
	if e := Write(filepath.Join(d, "forbidden"), r, d); e == nil {
		t.Fatal("wrote inside input evidence")
	}
}

func treeHash(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	e := filepath.WalkDir(dir, func(p string, f os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !f.IsDir() {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			out[p] = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}

func TestClaimFailureMatrix(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, string)
		claim  string
		want   Status
	}{
		{"drops", func(t *testing.T, d string) {
			modifySummary(t, d, func(s *report.Summary) { s.Steps[0].Dropped = 1; s.DroppedTotal = 1 })
		}, "offered_load", Invalid},
		{"send-lag", func(t *testing.T, d string) {
			modifySummary(t, d, func(s *report.Summary) { s.Steps[0].SendLag.P99 = 11; s.Steps[0].SendLag.Max = 12 })
		}, "offered_load", Invalid},
		{"missing-outcome-file", func(t *testing.T, d string) {
			if e := os.Remove(filepath.Join(d, "records.ndjson")); e != nil {
				t.Fatal(e)
			}
		}, "offered_load", Invalid},
		{"missing-failure-record", func(t *testing.T, d string) {
			modifySummary(t, d, func(s *report.Summary) {
				s.Steps[0].AckSuccess = 9
				s.Steps[0].ExplicitFailure = 1
				s.Steps[0].AckLatency.Count = 9
			})
		}, "offered_load", Invalid},
		{"missing-history", func(t *testing.T, d string) {
			if e := os.Remove(filepath.Join(d, "history.ndjson")); e != nil {
				t.Fatal(e)
			}
		}, "history", NotAssessed},
		{"partial-history", func(t *testing.T, d string) { modifyEvidence(t, d, func(e *Evidence) { e.History.Complete = false }) }, "history", NotAssessed},
		{"missing-acknowledged", func(t *testing.T, d string) {
			rewriteRows(t, d, "history.ndjson", func(r []map[string]any) []map[string]any { return r[:9] })
		}, "history", Fail},
		{"duplicate", func(t *testing.T, d string) {
			rewriteRows(t, d, "history.ndjson", func(r []map[string]any) []map[string]any { return append(r, r[0]) })
		}, "history", Fail},
		{"missing-offset-by-duplicate", func(t *testing.T, d string) {
			rewriteRows(t, d, "history.ndjson", func(r []map[string]any) []map[string]any { return append(r[:9], r[0]) })
		}, "history", Fail},
		{"malformed-identity", func(t *testing.T, d string) {
			rewriteRows(t, d, "history.ndjson", func(r []map[string]any) []map[string]any { delete(r[0], "run_id"); return r })
		}, "history", Invalid},
		{"missing-seq-not-zero", func(t *testing.T, d string) {
			rewriteRows(t, d, "history.ndjson", func(r []map[string]any) []map[string]any { delete(r[0], "seq"); return r })
		}, "history", Invalid},
		{"missing-clock", func(t *testing.T, d string) { modifyEvidence(t, d, func(e *Evidence) { e.SinkClock = "" }) }, "delivery_latency", NotAssessed},
		{"missing-cpu", func(t *testing.T, d string) {
			if e := os.Remove(filepath.Join(d, "cpu-after.tsv")); e != nil {
				t.Fatal(e)
			}
		}, "cpu_cost", NotAssessed},
		{"cpu-reset", func(t *testing.T, d string) {
			b, e := os.ReadFile(filepath.Join(d, "cpu-after.tsv"))
			if e != nil {
				t.Fatal(e)
			}
			text(t, d, "cpu-after.tsv", strings.Replace(string(b), "2000000", "100", 1))
		}, "cpu_cost", Invalid},
		{"cpu-enclosing-window", func(t *testing.T, d string) {
			b, e := os.ReadFile(filepath.Join(d, "cpu-after.tsv"))
			if e != nil {
				t.Fatal(e)
			}
			text(t, d, "cpu-after.tsv", strings.ReplaceAll(string(b), "1700000001", "1700000002"))
		}, "cpu_cost", NotAssessed},
		{"zero-ack-denominator", func(t *testing.T, d string) {
			modifyEvidence(t, d, func(e *Evidence) { e.CPU[0].Denominator = "unknown-denominator" })
		}, "cpu_cost", NotAssessed},
		{"missing-observation-deadline", func(t *testing.T, d string) { modifyEvidence(t, d, func(e *Evidence) { e.ObservationDeadlineMS = 0 }) }, "history", NotAssessed},
		{"history-window-incomplete", func(t *testing.T, d string) {
			modifyEvidence(t, d, func(e *Evidence) { e.History.EndMS = e.ObservationDeadlineMS - 1 })
		}, "history", NotAssessed},
		{"no-independent-history", func(t *testing.T, d string) { modifyEvidence(t, d, func(e *Evidence) { e.History.Path = e.Sink.Path }) }, "history", Invalid},
		{"wrong-summary-id", func(t *testing.T, d string) { modifySummary(t, d, func(s *report.Summary) { s.RunID = "other" }) }, "offered_load", Invalid},
		{"generator-exit", func(t *testing.T, d string) {
			m := load[map[string]any](t, d, "manifest.json")
			m["k6_exit_code"] = 108
			save(t, d, "manifest.json", m)
		}, "offered_load", Invalid},
		{"contradictory-health", func(t *testing.T, d string) { save(t, d, "health.json", map[string]any{"valid": false}) }, "offered_load", Invalid},
		{"optional-cpu-malformed", func(t *testing.T, d string) { text(t, d, "gen-cpu.ndjson", "not-json\n") }, "offered_load", Pass},
		{"sink-recording-error", func(t *testing.T, d string) {
			text(t, d, "sink-status.txt", "sink_overflow_total 1\n")
			modifyEvidence(t, d, func(e *Evidence) { e.Sink.Diagnostics = "sink-status.txt"; e.Sink.DiagnosticsRunLocal = true })
		}, "sink", Invalid},
		{"history-unrecoverable", func(t *testing.T, d string) {
			text(t, d, "export.log", "history-ids fiware: unrecoverable=1\n")
			modifyEvidence(t, d, func(e *Evidence) { e.History.Diagnostics = "export.log"; e.History.DiagnosticsRunLocal = true })
		}, "history", Invalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture(t)
			tc.mutate(t, d)
			r := analyse(t, d)
			c := r.Claims[tc.claim]
			if tc.claim != "offered_load" {
				c = r.Steps[0].Claims[tc.claim]
			}
			if c.Status != tc.want {
				t.Fatalf("want %s, got %+v", tc.want, c)
			}
			if tc.claim != "offered_load" && r.Claims["offered_load"].Status != Pass {
				t.Fatal("secondary missing/failure invalidated offered load")
			}
		})
	}
}

func TestProfileThresholdAndEndpointSemantics(t *testing.T) {
	t.Run("threshold missing", func(t *testing.T) {
		d := fixture(t)
		p := load[Policy](t, d, "profiles.json")
		p.Profiles[0].Requirements[0].Threshold = nil
		save(t, d, "profiles.json", p)
		r := analyse(t, d)
		if profileStatus(t, r, "quick_delivery") != NotAssessed {
			t.Fatal("invented threshold")
		}
	})
	t.Run("endpoint missing", func(t *testing.T) {
		d := fixture(t)
		p := load[Policy](t, d, "profiles.json")
		p.Profiles[0].Requirements[0].Metric = "latency.durable.p99_ms"
		save(t, d, "profiles.json", p)
		if profileStatus(t, analyse(t, d), "quick_delivery") != NotAssessed {
			t.Fatal("used acknowledgement as durable persistence")
		}
	})
	t.Run("valid measured failure", func(t *testing.T) {
		d := fixture(t)
		p := load[Policy](t, d, "profiles.json")
		v := 1.0
		p.Profiles[0].Requirements[0].Threshold = &v
		save(t, d, "profiles.json", p)
		r := analyse(t, d)
		if r.Claims["offered_load"].Status != Pass || profileStatus(t, r, "quick_delivery") != Fail {
			t.Fatal("SLO failure confused with generator invalidity")
		}
	})
	t.Run("no policy", func(t *testing.T) {
		d := fixture(t)
		r, e := Analyze(Options{Run: d, Evidence: filepath.Join(d, "evidence.json")})
		if e != nil {
			t.Fatal(e)
		}
		if profileStatus(t, r, "quick_delivery") != Pass || profileStatus(t, r, "saved_data_reliability") != Pass || profileStatus(t, r, "high_device_count") != NotAssessed {
			t.Fatal("approved defaults or W2 prerequisites not respected")
		}
	})
	t.Run("explicit empty observation", func(t *testing.T) {
		d := fixture(t)
		text(t, d, "history.ndjson", "")
		r := analyse(t, d)
		if r.Steps[0].Claims["history"].Status != Fail || *r.Steps[0].Metrics["history.missing_acknowledged"].Value != 10 {
			t.Fatal("complete empty export confused with absent export")
		}
	})
}

func TestUnknownOutcomesRemainRelatedToObservations(t *testing.T) {
	d := fixture(t)
	modifySummary(t, d, func(s *report.Summary) {
		s.Steps[0].AckSuccess = 9
		s.Steps[0].Unknown = 1
		s.Steps[0].AckLatency.Count = 9
	})
	rc := report.Record{Rec: "msg", RunID: "SYNTHETIC-test", Seq: 9, Device: 1, Step: 0, Phase: "measurement", Tick: 9, SchedMs: 1700000000900, ActualMs: 1700000000901, RespMs: 1700000001901, Outcome: "unknown", Status: 0}
	b, _ := json.Marshal(rc)
	text(t, d, "records.ndjson", string(b)+"\n")
	r := analyse(t, d)
	if r.Claims["offered_load"].Status != Pass {
		t.Fatal(r.Claims["offered_load"])
	}
	for _, point := range []string{"sink", "history"} {
		if *r.Steps[0].Metrics[point+".observed_despite_unknown"].Value != 1 {
			t.Fatal("lost unknown/downstream relationship")
		}
	}
}

func TestBoundaryLedgerRecordsPassValidationAndAnalysis(t *testing.T) {
	for _, detail := range []string{
		"incomplete_response:body_3_declared_10",
		"incomplete_response:transport_response_body_read",
		"oversized_response:content_length_4194305_gt_4194304",
	} {
		t.Run(detail, func(t *testing.T) {
			d := fixture(t)
			modifySummary(t, d, func(s *report.Summary) {
				s.Steps[0].AckSuccess = 9
				s.Steps[0].ExplicitFailure = 1
				s.Steps[0].AckLatency.Count = 9
			})
			rc := report.Record{Rec: "msg", RunID: "SYNTHETIC-test", Seq: 9, Device: 1, Step: 0, Phase: "measurement", Tick: 9,
				SchedMs: 1700000000900, ActualMs: 1700000000901, RespMs: 1700000001920, Outcome: "explicit_failure", Detail: detail}
			b, _ := json.Marshal(rc)
			text(t, d, "records.ndjson", string(b)+"\n")
			r := load[plan.Resolved](t, d, "resolved.json")
			sum := load[report.Summary](t, d, "k6-summary.json")
			if err := validateLedger(&r, audit.PlannedMessages(&r, &sum), []report.Record{rc}); err != nil {
				t.Fatalf("boundary ledger record rejected: %v", err)
			}
			result := analyse(t, d)
			if result.Claims["offered_load"].Status != Pass {
				t.Fatalf("boundary ledger record invalidated offered load: %+v", result.Claims["offered_load"])
			}
		})
	}
}

func TestRawBoundaryCategoryStillRejectedByLedgerValidation(t *testing.T) {
	for _, outcome := range []string{"incomplete_response", "oversized_response"} {
		d := fixture(t)
		rc := report.Record{Rec: "msg", RunID: "SYNTHETIC-test", Seq: 9, Device: 1, Step: 0, Phase: "measurement", Tick: 9,
			SchedMs: 1700000000900, ActualMs: 1700000000901, RespMs: 1700000001920, Outcome: outcome}
		r := load[plan.Resolved](t, d, "resolved.json")
		sum := load[report.Summary](t, d, "k6-summary.json")
		if err := validateLedger(&r, audit.PlannedMessages(&r, &sum), []report.Record{rc}); err == nil {
			t.Fatalf("raw boundary outcome %q silently accepted", outcome)
		}
	}
}

func TestLateObservationsNotRewrittenOrCalledPermanentLoss(t *testing.T) {
	d := fixture(t)
	rewriteRows(t, d, "sink.ndjson", func(r []map[string]any) []map[string]any { r[0]["recv_ms"] = 1700000003000.0; return r })
	r := analyse(t, d)
	if *r.Steps[0].Metrics["sink.late_occurrences"].Value != 1 || *r.Steps[0].Metrics["sink.missing_acknowledged"].Value != 1 {
		t.Fatal("late/deadline accounting wrong")
	}
	if r.Steps[0].Claims["delivery_latency"].Status != NotAssessed {
		t.Fatal("claimed uncensored full latency with missing observations")
	}
}

func TestComparisonSeparatesWorkloadDimensionsAndVersions(t *testing.T) {
	d := fixture(t)
	r1 := analyse(t, d)
	d2 := filepath.Join(t.TempDir(), "input")
	if e := Synthetic(d2, "SYNTHETIC-more", 10); e != nil {
		t.Fatal(e)
	}
	r2 := analyse(t, d2)
	c := Compare([]*Result{r1, r2})
	if len(c.Groups) != 6 {
		t.Fatal("equal rates/different device counts collapsed", len(c.Groups))
	}
	r3 := analyse(t, d)
	r3.RunID = "different-version"
	r3.Configuration.Images = map[string]string{"application": "synthetic/application@sha256:" + strings.Repeat("f", 64)}
	c = Compare([]*Result{r1, r3})
	if len(c.Groups) != 6 {
		t.Fatal("incompatible image versions collapsed")
	}
	c = Compare([]*Result{r1, r1})
	for _, row := range c.Rows[3:] {
		if row.Eligible {
			t.Fatal("duplicate inputs counted as repetitions")
		}
	}
	r4 := analyse(t, d)
	r4.RunID = "unknown-provenance"
	r4.Claims["comparability"] = claim(NotAssessed, "missing image digest")
	c = Compare([]*Result{r4})
	for _, row := range c.Rows {
		if row.Eligible {
			t.Fatal("unknown provenance included")
		}
	}
	for _, g := range c.Groups {
		if g.HighestObservedPass != nil {
			t.Fatal("unsupported capacity point")
		}
	}
}

func TestBacklogMathAndNoQueueProofOfStorage(t *testing.T) {
	for _, tc := range []struct{ df, want float64 }{{1, 6.3137515148}, {10, 1.8124611228}, {58, 1.6715527625}} {
		if math.Abs(studentCritical95(tc.df)-tc.want) > 1e-7 {
			t.Fatalf("t critical df %v: %v", tc.df, studentCritical95(tc.df))
		}
	}
	d := fixture(t)
	rewriteRows(t, d, "history.ndjson", func(r []map[string]any) []map[string]any { return r[1:] })
	text(t, d, "queue.json", "[{\"epoch_ms\":1700000000000,\"records\":0},{\"epoch_ms\":1700000001000,\"records\":0}]")
	modifyEvidence(t, d, func(e *Evidence) {
		e.Backlog = Source{Path: "queue.json", Complete: true, Source: "synthetic flat broker queue"}
	})
	r := analyse(t, d)
	if r.Steps[0].Claims["history"].Status != Fail || profileStatus(t, r, "saved_data_reliability") != Fail {
		t.Fatal("flat queue hid missing history identity")
	}
	if r.Steps[0].Claims["backlog"].Status != NotAssessed {
		t.Fatal("flat queue became proof of sustained processing")
	}
}

func TestMalformedRowNeverRepairsFullIdentityJoin(t *testing.T) {
	d := fixture(t)
	stored := []reconcile.Stored{}
	for i := int64(1); i < 10; i++ {
		stored = append(stored, reconcile.Stored{RunID: "SYNTHETIC-test", Seq: i})
	}
	stored = append(stored, reconcile.Stored{Seq: 0})
	r, e := reconcile.Run(d, stored)
	if e != nil {
		t.Fatal(e)
	}
	if len(r.AckLost) != 1 || r.AckLost[0] != 0 || len(r.MalformedIdentities) != 1 || r.Clean {
		t.Fatal("missing run_id repaired by seq alone", r)
	}
}

func TestCPUEnclosingDeltaDoesNotAcquirePhaseDenominator(t *testing.T) {
	d := fixture(t)
	b, e := os.ReadFile(filepath.Join(d, "cpu-after.tsv"))
	if e != nil {
		t.Fatal(e)
	}
	text(t, d, "cpu-after.tsv", strings.ReplaceAll(string(b), "1700000001", "1700000002"))
	r := analyse(t, d)
	cpu := r.Steps[0].CPU
	if cpu.TotalCPUSeconds == nil || cpu.TotalCPUSecondsPerMessage != nil || r.Steps[0].Claims["cpu_cost"].Status != NotAssessed {
		t.Fatal("enclosing interval silently treated as exact phase cost")
	}
}

func TestSummaryMissingCounterCannotDefaultToZero(t *testing.T) {
	d := fixture(t)
	m := load[map[string]any](t, d, "k6-summary.json")
	delete(m["steps"].([]any)[0].(map[string]any), "unknown")
	save(t, d, "k6-summary.json", m)
	if analyse(t, d).Claims["offered_load"].Status != Invalid {
		t.Fatal("missing counter became healthy zero")
	}
}

func TestNoAverageOfWindowPercentiles(t *testing.T) {
	d := fixture(t)
	resizeFixture(t, d, 120)
	modifySummary(t, d, func(s *report.Summary) {
		s.Steps[0].AckLatency = report.TrendStats{Count: 10, P50: 2, P99: 91.18, Max: 100}
		s.Steps[0].AckByMinute = []report.TrendStats{{Minute: 1, Count: 5, P50: 2, P99: 2, Max: 2}, {Minute: 2, Count: 5, P50: 2, P99: 96.08, Max: 100}}
	})
	r := analyse(t, d)
	if v := *r.Steps[0].Metrics["latency.ack.p99_ms"].Value; v != 91.18 || v == (2+96.08)/2.0 {
		t.Fatal("averaged window p99", v)
	}
	if _, ok := r.Provenance["run_wide_p99"]; ok {
		t.Fatal("invented mergeable run percentile")
	}
}

func resizeFixture(t *testing.T, d string, seconds int) {
	t.Helper()
	r := load[plan.Resolved](t, d, "resolved.json")
	r.DurationS = seconds
	r.Steps[0].DurationS = seconds
	r.Steps[0].Rate = 10 / float64(seconds)
	r.Steps[0].TickRate = r.Steps[0].Rate
	save(t, d, "resolved.json", r)
	m := load[map[string]any](t, d, "manifest.json")
	m["resolved_plan"] = r
	save(t, d, "manifest.json", m)
	modifyEvidence(t, d, func(e *Evidence) {
		e.ObservationDeadlineMS = 1700000000000 + float64(seconds+1)*1000
		e.Sink.EndMS = e.ObservationDeadlineMS
		e.History.EndMS = e.ObservationDeadlineMS
		e.History.CollectedAtMS = e.ObservationDeadlineMS
	})
	for _, file := range []string{"sink.ndjson", "history.ndjson"} {
		rewriteRows(t, d, file, func(rows []map[string]any) []map[string]any {
			for _, row := range rows {
				key, delay := "recv_ms", 10.0
				if file == "history.ndjson" {
					key, delay = "ts", 5
				}
				row[key] = 1700000000000 + row["seq"].(float64)*float64(seconds)*100 + delay
			}
			return rows
		})
	}
	b, e := os.ReadFile(filepath.Join(d, "cpu-after.tsv"))
	if e != nil {
		t.Fatal(e)
	}
	text(t, d, "cpu-after.tsv", strings.ReplaceAll(string(b), "1700000001", fmt.Sprint(1700000000+seconds)))
}

func addSyntheticService(t *testing.T, d string) {
	t.Helper()
	var runs []map[string]any
	j := 0
	for _, platform := range []string{"fiware", "thingsboard", "edgex"} {
		for i := 0; i < 5; i++ {
			j++
			runs = append(runs, map[string]any{"run_id": fmt.Sprintf("SYNTHETIC-cal-%d", j), "platform": platform, "valid": true, "rate": 1, "reference_capacity": 4, "delay_growth_ms": 100, "source_sha256": fmt.Sprintf("%064x", j)})
		}
	}
	save(t, d, "calibration.json", map[string]any{"schema_version": 1, "synthetic": true, "runs": runs})
	p := load[Policy](t, d, "profiles.json")
	p.BacklogRule = &BacklogRule{Method: "ols-iid-10s-one-sided95-v1", IndependentBins: true, Calibration: "calibration.json", Source: "SYNTHETIC explicit IID interpretation, not an adopted research method"}
	service := Profile{ID: "service_slo", Name: "SYNTHETIC service SLO", Source: "synthetic example requirements"}
	for metric, threshold := range map[string]float64{"latency.delivery.p99_ms": 100, "outcomes.unsuccessful_ratio": .001, "sink.missing_acknowledged": 0, "history.missing_acknowledged": 0, "sink.duplicate_extra_copies": 0, "history.duplicate_extra_copies": 0, "backlog.non_growing": 1} {
		op := "<="
		if metric == "backlog.non_growing" {
			op = "=="
		}
		v := threshold
		service.Requirements = append(service.Requirements, Requirement{Metric: metric, Operator: op, Unit: metricUnits[metric], Threshold: &v, Source: "synthetic example"})
	}
	// Stable ordering also makes JSON output reproducible.
	sort.Slice(service.Requirements, func(i, j int) bool { return service.Requirements[i].Metric < service.Requirements[j].Metric })
	p.Profiles = append(p.Profiles, service)
	save(t, d, "profiles.json", p)
}

func TestConfiguredServiceSLOAndCalibratedBacklog(t *testing.T) {
	d := fixture(t)
	resizeFixture(t, d, 30)
	addSyntheticService(t, d)
	r := analyse(t, d)
	if r.Steps[0].Claims["backlog"].Status != Pass || r.Steps[0].Claims["service_slo"].Status != Pass {
		t.Fatalf("synthetic complete criteria should pass: %+v", r.Steps[0].Claims)
	}
	rewriteRows(t, d, "history.ndjson", func(rows []map[string]any) []map[string]any { return rows[1:] })
	r = analyse(t, d)
	if r.Claims["offered_load"].Status != Pass || r.Steps[0].Claims["service_slo"].Status != Fail || r.Steps[0].Claims["backlog"].Status != Pass {
		t.Fatal("flat pending backlog masked known missing-history violation")
	}
	p := load[Policy](t, d, "profiles.json")
	p.BacklogRule.IndependentBins = false
	save(t, d, "profiles.json", p)
	r = analyse(t, d)
	if r.Steps[0].Claims["backlog"].Status != NotAssessed {
		t.Fatal("silently selected statistical independence assumption")
	}
}

func TestProvenanceMismatchIsNotBlanketRunInvalidity(t *testing.T) {
	d := fixture(t)
	text(t, d, "effective.json", "{\"changed\":true}")
	r := analyse(t, d)
	if r.Claims["comparability"].Status != Invalid || r.Claims["offered_load"].Status != Pass {
		t.Fatal("comparability conflated with individual run validity")
	}
}
