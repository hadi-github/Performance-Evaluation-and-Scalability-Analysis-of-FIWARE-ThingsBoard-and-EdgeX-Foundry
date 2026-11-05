package analysis

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func trace(t *testing.T, d string, count int, mutate func(int, map[string]any)) {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	for i := 0; i < count; i++ {
		data := map[string]any{"time": time.UnixMilli(1700000000000 + int64(i)*100 + 20).UTC().Format(time.RFC3339Nano), "value": 20.0, "tags": map[string]string{"name": "send", "run_id": "SYNTHETIC-test"}, "metadata": map[string]string{"seq": fmt.Sprint(i), "sched": fmt.Sprint(1700000000000 + int64(i)*100)}}
		if mutate != nil {
			mutate(i, data)
		}
		if err := json.NewEncoder(z).Encode(map[string]any{"type": "Point", "metric": "http_req_duration", "data": data}); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "k6.json.gz"), b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNativeHTTPTrace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		count  int
		mutate func(int, map[string]any)
		want   Status
	}{
		{"complete-ms", 10, nil, Pass},
		{"partial", 9, nil, NotAssessed},
		{"missing-metadata", 10, func(i int, d map[string]any) { delete(d, "metadata") }, NotAssessed},
		{"negative", 10, func(i int, d map[string]any) { d["value"] = -1 }, Invalid},
		{"missing-value", 10, func(i int, d map[string]any) { delete(d, "value") }, Invalid},
		{"wrong-run", 10, func(i int, d map[string]any) { d["tags"].(map[string]string)["run_id"] = "other" }, Invalid},
		{"duplicate", 10, func(i int, d map[string]any) {
			if i == 9 {
				d["metadata"].(map[string]string)["seq"] = "0"
			}
		}, Invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := fixture(t)
			trace(t, d, tc.count, tc.mutate)
			r := analyse(t, d)
			s := r.Steps[0]
			if s.Claims["http_duration"].Status != tc.want {
				t.Fatal(s.Claims["http_duration"])
			}
			if r.Claims["offered_load"].Status != Pass {
				t.Fatal("optional trace invalidated offered load")
			}
			m, ok := s.Metrics["latency.http.p99_ms"]
			if tc.want == Pass {
				if !ok || *m.Value != 20 {
					t.Fatal("native milliseconds rescaled", m)
				}
			} else if ok {
				t.Fatal("incomplete trace emitted a percentile")
			}
		})
	}
	t.Run("truncated-gzip", func(t *testing.T) {
		d := fixture(t)
		trace(t, d, 10, nil)
		p := filepath.Join(d, "k6.json.gz")
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(p, b[:len(b)-6], 0644); e != nil {
			t.Fatal(e)
		}
		if analyse(t, d).Steps[0].Claims["http_duration"].Status != Invalid {
			t.Fatal("gzip checksum/truncation ignored")
		}
	})
}

func TestZeroDenominator(t *testing.T) {
	d := fixture(t)
	modifySummary(t, d, func(s *report.Summary) {
		s.Steps[0].AckSuccess = 0
		s.Steps[0].Unknown = 10
		s.Steps[0].AckLatency = report.TrendStats{}
	})
	var b bytes.Buffer
	for i := int64(0); i < 10; i++ {
		r := report.Record{Rec: "msg", RunID: "SYNTHETIC-test", Seq: i, Device: int(i % 2), Step: 0, Phase: "measurement", Tick: i, SchedMs: 1700000000000 + float64(i)*100, ActualMs: 1700000000001 + float64(i)*100, RespMs: 1700000000020 + float64(i)*100, Outcome: "unknown"}
		if e := json.NewEncoder(&b).Encode(r); e != nil {
			t.Fatal(e)
		}
	}
	text(t, d, "records.ndjson", b.String())
	r := analyse(t, d)
	if r.Claims["offered_load"].Status != Pass || r.Steps[0].Claims["cpu_cost"].Status != NotAssessed {
		t.Fatal(r.Claims, r.Steps[0].Claims)
	}
	if _, ok := r.Steps[0].Metrics["cpu.seconds_per_message"]; ok {
		t.Fatal("zero denominator became cost")
	}
}

func TestNullAndMissingNestedSummary(t *testing.T) {
	for _, field := range []string{"p99", "count"} {
		for _, null := range []bool{false, true} {
			t.Run(fmt.Sprint(field, null), func(t *testing.T) {
				d := fixture(t)
				m := load[map[string]any](t, d, "k6-summary.json")
				stats := m["steps"].([]any)[0].(map[string]any)["ack_latency_ms"].(map[string]any)
				if null {
					stats[field] = nil
				} else {
					delete(stats, field)
				}
				save(t, d, "k6-summary.json", m)
				if analyse(t, d).Claims["offered_load"].Status != Invalid {
					t.Fatal("absent/null percentile defaulted to zero")
				}
			})
		}
	}
}

func TestUnsupportedSchemaAndSymlinks(t *testing.T) {
	d := fixture(t)
	modifySummary(t, d, func(s *report.Summary) { s.SchemaVersion = 99 })
	if analyse(t, d).Claims["offered_load"].Status != NotAssessed {
		t.Fatal("unsupported schema misclassified")
	}
	d = fixture(t)
	r := analyse(t, d)
	link := filepath.Join(t.TempDir(), "alias")
	if e := os.Symlink(d, link); e != nil {
		t.Fatal(e)
	}
	if e := Write(filepath.Join(link, "output"), r, d); e == nil {
		t.Fatal("output symlink bypassed evidence boundary")
	}
	alias := filepath.Join(d, "history-alias.ndjson")
	if e := os.Symlink(filepath.Join(d, "sink.ndjson"), alias); e != nil {
		t.Fatal(e)
	}
	modifyEvidence(t, d, func(e *Evidence) { e.History.Path = "history-alias.ndjson" })
	if analyse(t, d).Steps[0].Claims["history"].Status != Invalid {
		t.Fatal("same source via symlink passed independent observation gate")
	}
}

func TestWarmupExcludedFromMeasuredCohort(t *testing.T) {
	d := fixture(t)
	r := load[plan.Resolved](t, d, "resolved.json")
	r.Steps[0].Phase = "warmup"
	second := r.Steps[0]
	second.Index = 1
	second.Name = "s01-measure-r10"
	second.Phase = "measurement"
	second.StartS = 1
	second.SeqOffset = 10
	r.Steps = append(r.Steps, second)
	r.TotalTicks = 20
	r.DurationS = 2
	save(t, d, "resolved.json", r)
	m := load[map[string]any](t, d, "manifest.json")
	m["resolved_plan"] = r
	save(t, d, "manifest.json", m)
	modifySummary(t, d, func(s *report.Summary) {
		second := s.Steps[0]
		second.Index = 1
		second.Name = "s01-measure-r10"
		second.ScenarioStartMs += 1000
		s.Steps = append(s.Steps, second)
	})
	modifyEvidence(t, d, func(e *Evidence) {
		e.ObservationDeadlineMS += 1000
		e.Sink.EndMS += 1000
		e.History.EndMS += 1000
		e.History.CollectedAtMS += 1000
		e.CPU = nil
	})
	for _, file := range []string{"sink.ndjson", "history.ndjson"} {
		rewriteRows(t, d, file, func(rows []map[string]any) []map[string]any {
			out := append([]map[string]any{}, rows...)
			for _, row := range rows {
				copy := map[string]any{}
				for k, v := range row {
					copy[k] = v
				}
				copy["seq"] = copy["seq"].(float64) + 10
				key := "recv_ms"
				if file == "history.ndjson" {
					key = "ts"
				}
				copy[key] = copy[key].(float64) + 1000
				out = append(out, copy)
			}
			return out
		})
	}
	result := analyse(t, d)
	if result.Claims["offered_load"].Status != Pass || len(result.Steps) != 1 {
		t.Fatal(result.Claims, result.Steps)
	}
	s := result.Steps[0]
	if s.Index != 1 || s.Claims["history"].Status != Pass || *s.Metrics["history.unique"].Value != 10 || *s.Metrics["sink.unique"].Value != 10 || *s.Metrics["sink.unexpected_identities"].Value != 0 {
		t.Fatal("warmup mixed into measured cohort", s)
	}
}
