package analysis

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"time"

	"iotbench/internal/arrivals"
	"iotbench/internal/audit"
)

// Native k6 JSON http_req_duration values are milliseconds; Prometheus remote
// write time metrics use seconds. Never read a chart percentile as a raw point.
func httpSamples(s *Step, rd reader, runID string, planned []audit.Planned) {
	b, e := rd.read("native_k6_json", "k6.json.gz")
	if e != nil {
		return
	}
	if s.Claims["offered_load"].Status != Pass {
		s.Claims["http_duration"] = dependent(s.Claims["offered_load"])
		return
	}
	gz, e := gzip.NewReader(bytes.NewReader(b))
	if e != nil {
		s.Claims["http_duration"] = claim(Invalid, "corrupt native k6 gzip trace")
		return
	}
	defer gz.Close()
	want := map[int64]float64{}
	for _, p := range planned {
		if p.Step == s.Name {
			want[p.Seq] = p.SchedMs
		}
	}
	seen := map[int64]bool{}
	var values []float64
	d := json.NewDecoder(gz)
	for {
		var point struct {
			Type   string `json:"type"`
			Metric string `json:"metric"`
			Data   struct {
				Time     time.Time         `json:"time"`
				Value    *float64          `json:"value"`
				Tags     map[string]string `json:"tags"`
				Metadata map[string]string `json:"metadata"`
			} `json:"data"`
		}
		e = d.Decode(&point)
		if e == io.EOF {
			break
		}
		if e != nil {
			s.Claims["http_duration"] = claim(Invalid, "malformed native k6 JSON trace")
			return
		}
		if point.Type != "Point" || point.Metric != "http_req_duration" || point.Data.Tags["name"] != "send" {
			continue
		}
		seq, err := strconv.ParseInt(point.Data.Metadata["seq"], 10, 64)
		if err != nil {
			s.Claims["http_duration"] = claim(NotAssessed, "trace lacks per-message metadata needed for scheduled-cohort assignment")
			return
		}
		sched, err := strconv.ParseFloat(point.Data.Metadata["sched"], 64)
		expected, inCohort := want[seq]
		if !inCohort {
			continue
		}
		if err != nil || !finite(sched) || math.Abs(sched-expected) > .0011 || seen[seq] || point.Data.Tags["run_id"] != runID || point.Data.Value == nil || !finite(*point.Data.Value) || *point.Data.Value < 0 || point.Data.Time.IsZero() {
			s.Claims["http_duration"] = claim(Invalid, "duplicate, mismatched or inconsistent native HTTP sample")
			return
		}
		seen[seq] = true
		values = append(values, *point.Data.Value)
	}
	if len(seen) != len(want) || len(values) == 0 {
		s.Claims["http_duration"] = claim(NotAssessed, fmt.Sprintf("native HTTP trace incomplete: %d of %d planned sends", len(seen), len(want)))
		return
	}
	sort.Float64s(values)
	s.Claims["http_duration"] = claim(Pass, "complete native per-message HTTP-duration samples matched to scheduled cohort; milliseconds, not durable latency")
	put(s, "latency.http.p50_ms", arrivals.Quantile(values, .5), "ms", "http_duration", "native k6 actual-send-to-response diagnostic, nearest-rank estimator")
	put(s, "latency.http.p99_ms", arrivals.Quantile(values, .99), "ms", "http_duration", "native per-message values; not averaged window percentiles")
}
