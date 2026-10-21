// Package report reads a run's retained artifacts and computes the generator health verdict. Everything here is
// a pure function of files in the run directory, so a verdict can always be regenerated offline (benchctl check).
package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"iotbench/internal/hash"
	"iotbench/internal/plan"
)

// Summary is k6-summary.json, written by workloads/k6/gen.js handleSummary.
type Summary struct {
	SchemaVersion     int           `json:"schema_version"`
	RunID             string        `json:"run_id"`
	EdgesMs           []float64     `json:"histogram_edges_ms"`
	Steps             []StepSummary `json:"steps"`
	DroppedTotal      float64       `json:"dropped_iterations_total"`
	VUsPeak           float64       `json:"vus_peak"`
	VUsMaxAllocated   float64       `json:"vus_max_allocated"`
	SampleRequest     *Sample       `json:"sample_request"`
	SetupEndMs        float64       `json:"setup_end_ms"`
	TestRunDurationMs float64       `json:"test_run_duration_ms"`
}

type Sample struct {
	URLBytes  int    `json:"url_bytes"`
	BodyBytes int    `json:"body_bytes"`
	Body      string `json:"body"`
}

type StepSummary struct {
	Index           int       `json:"index"`
	Name            string    `json:"name"`
	ScenarioStartMs float64   `json:"scenario_start_ms"`
	Attempted       float64   `json:"attempted"`
	AckSuccess      float64   `json:"ack_success"`
	ExplicitFailure float64   `json:"explicit_failure"`
	Unknown         float64   `json:"unknown"`
	Early           float64   `json:"early"`
	BeyondPlan      float64   `json:"beyond_plan"`
	Thinned         float64   `json:"thinned"`
	DupInjected     float64   `json:"dup_injected"`
	Dropped         float64   `json:"dropped_iterations"`
	HTTPReqs        float64   `json:"http_reqs"`
	DataSentBytes   float64   `json:"data_sent_bytes"`
	SendLagHist     []float64 `json:"send_lag_hist"`
	AckLatencyHist  []float64 `json:"ack_latency_hist"`
}

// Record is one line of records.ndjson: a non-success message or an early send.
type Record struct {
	Rec       string  `json:"rec"`
	RunID     string  `json:"run_id"`
	Seq       int64   `json:"seq"`
	Device    int     `json:"device_id"`
	Step      int     `json:"rate_step"`
	Phase     string  `json:"phase"`
	Tick      int64   `json:"tick"`
	SchedMs   float64 `json:"scheduled_send_ms"`
	ActualMs  float64 `json:"actual_send_ms"`
	Outcome   string  `json:"outcome"`
	Status    int     `json:"http_status"`
	ErrorCode int     `json:"error_code"`
	RespMs    float64 `json:"response_ms"`
	Detail    string  `json:"detail"`
	Early     bool    `json:"early"`
}

// CPUSample is one line of gen-cpu.ndjson: per-core busy fraction over the previous interval, and k6 memory.
type CPUSample struct {
	Ms       float64   `json:"ms"`
	CoreBusy []float64 `json:"core_busy"`
	K6MemB   int64     `json:"k6_mem_bytes"`
}

type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Gate   bool   `json:"gate"` // false: reported only
	Detail string `json:"detail"`
}

type StepHealth struct {
	Index            int                `json:"index"`
	Name             string             `json:"name"`
	Phase            string             `json:"phase"`
	Rate             float64            `json:"rate"`
	DurationS        int                `json:"duration_s"`
	WindowStartMs    float64            `json:"window_start_ms"`
	PlannedTicks     int64              `json:"planned_ticks"`
	PlannedSends     int64              `json:"planned_sends"`
	TicksExecuted    int64              `json:"ticks_executed"`
	Sends            int64              `json:"sends"` // attempted minus injected duplicates
	AchievedRatio    float64            `json:"achieved_ratio"`
	Outcomes         map[string]float64 `json:"outcomes"`
	SendLagMs        Quantiles          `json:"send_lag_ms"`
	AckLatencyMs     Quantiles          `json:"ack_latency_ms"`
	PerDeviceIntervS float64            `json:"per_device_interval_s"`
	OnWireBytesAvg   float64            `json:"on_wire_bytes_per_request"`
	CoreBusyMax      []float64          `json:"generator_core_busy_max"`
	CoreBusyMean     []float64          `json:"generator_core_busy_mean"`
	K6MemPeakB       int64              `json:"k6_mem_peak_bytes"`
	Checks           []Check            `json:"checks"`
	Valid            bool               `json:"valid"`
}

// Quantiles from a bucketed histogram: each value is the upper edge of the bucket holding that quantile
// (+Inf for the overflow bucket), plus the count.
type Quantiles struct {
	Count float64 `json:"count"`
	P50   F       `json:"p50_le"`
	P99   F       `json:"p99_le"`
	Max   F       `json:"max_le"`
}

// F is a float that JSON-encodes NaN (no data) as null and +Inf (overflow bucket) as "+Inf".
type F float64

func (f F) MarshalJSON() ([]byte, error) {
	switch {
	case math.IsNaN(float64(f)):
		return []byte("null"), nil
	case math.IsInf(float64(f), 1):
		return []byte(`"+Inf"`), nil
	}
	return json.Marshal(float64(f))
}

func (f *F) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "null":
		*f = F(math.NaN())
		return nil
	case `"+Inf"`:
		*f = F(math.Inf(1))
		return nil
	}
	var v float64
	err := json.Unmarshal(b, &v)
	*f = F(v)
	return err
}

type Health struct {
	Valid        bool         `json:"valid"`
	Reasons      []string     `json:"invalid_reasons"`
	Warnings     []string     `json:"warnings"`
	Steps        []StepHealth `json:"steps"`
	VUsPeak      float64      `json:"vus_peak"`
	VUsMax       int          `json:"vus_max"`
	RecordsTotal int          `json:"records_total"`
	K6MemPeakB   int64        `json:"k6_mem_peak_bytes"`
}

func ReadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func ReadNDJSON[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

func quantiles(edges, hist []float64) Quantiles {
	q := Quantiles{}
	for _, c := range hist {
		q.Count += c
	}
	if q.Count == 0 {
		q.P50, q.P99, q.Max = F(math.NaN()), F(math.NaN()), F(math.NaN())
		return q
	}
	edge := func(b int) F {
		if b < len(edges) {
			return F(edges[b])
		}
		return F(math.Inf(1))
	}
	at := func(p float64) F {
		need, cum := p*q.Count, 0.0
		for b, c := range hist {
			cum += c
			if cum >= need-1e-9 {
				return edge(b)
			}
		}
		return edge(len(hist) - 1)
	}
	q.P50, q.P99 = at(0.50), at(0.99)
	for b := len(hist) - 1; b >= 0; b-- {
		if hist[b] > 0 {
			q.Max = edge(b)
			break
		}
	}
	return q
}

// PlannedSends is the number of messages step s is meant to send: every tick for constant arrivals, the
// seed-selected ticks for thinning (reproducible offline from the seed).
func PlannedSends(r *plan.Resolved, s plan.Step) int64 {
	if r.Arrivals.Model != "thinning" {
		return s.Ticks
	}
	t := hash.Threshold(r.SendProb)
	n := int64(0)
	for seq := s.SeqOffset; seq < s.SeqOffset+s.Ticks; seq++ {
		if hash.Selected(r.Seed, uint32(seq), t) {
			n++
		}
	}
	return n
}

// Evaluate computes the health verdict of a run directory.
func Evaluate(dir string) (*plan.Resolved, *Summary, *Health, error) {
	var r plan.Resolved
	if err := ReadJSON(filepath.Join(dir, "resolved.json"), &r); err != nil {
		return nil, nil, nil, err
	}
	var s Summary
	if err := ReadJSON(filepath.Join(dir, "k6-summary.json"), &s); err != nil {
		return &r, nil, &Health{Valid: false, Reasons: []string{"no k6 summary: " + err.Error()}}, nil
	}
	recs, err := ReadNDJSON[Record](filepath.Join(dir, "records.ndjson"))
	if err != nil {
		return nil, nil, nil, err
	}
	cpu, err := ReadNDJSON[CPUSample](filepath.Join(dir, "gen-cpu.ndjson"))
	if err != nil {
		return nil, nil, nil, err
	}
	h := &Health{Valid: true, VUsPeak: s.VUsPeak, VUsMax: r.VUs.Max, RecordsTotal: len(recs)}
	for _, c := range cpu {
		h.K6MemPeakB = max(h.K6MemPeakB, c.K6MemB)
	}

	// records per step: non-success outcomes and early sends
	type recCount struct{ nonSuccess, early int64 }
	byStep := map[int]*recCount{}
	seen := map[int64]int{}
	for _, rc := range recs {
		c := byStep[rc.Step]
		if c == nil {
			c = &recCount{}
			byStep[rc.Step] = c
		}
		if rc.Outcome != "ack_success" {
			c.nonSuccess++
		}
		if rc.Early {
			c.early++
		}
		seen[rc.Seq]++
	}
	dupSeqs := map[int64]bool{}
	for _, q := range r.TestFaults.DuplicateSeqs {
		dupSeqs[q] = true
	}
	for seq, n := range seen {
		if n > 1 && !dupSeqs[seq] {
			h.Reasons = append(h.Reasons, fmt.Sprintf("seq %d has %d records (each seq must have exactly one outcome)", seq, n))
		}
	}

	if len(s.Steps) != len(r.Steps) {
		h.Reasons = append(h.Reasons, fmt.Sprintf("summary has %d steps, plan has %d", len(s.Steps), len(r.Steps)))
	}
	for i, st := range r.Steps {
		if i >= len(s.Steps) {
			break
		}
		ss := s.Steps[i]
		sh := StepHealth{Index: st.Index, Name: st.Name, Phase: st.Phase, Rate: st.Rate, DurationS: st.DurationS,
			PlannedTicks: st.Ticks, PlannedSends: PlannedSends(&r, st), Valid: true}
		sh.WindowStartMs = ss.ScenarioStartMs
		if sh.WindowStartMs == 0 {
			sh.WindowStartMs = s.SetupEndMs + float64(st.StartS)*1000
		}
		sh.Sends = int64(ss.Attempted - ss.DupInjected)
		sh.TicksExecuted = sh.Sends + int64(ss.Thinned)
		if sh.PlannedSends > 0 {
			sh.AchievedRatio = float64(sh.Sends) / float64(sh.PlannedSends)
		}
		sh.Outcomes = map[string]float64{"ack_success": ss.AckSuccess, "explicit_failure": ss.ExplicitFailure,
			"unknown": ss.Unknown, "not_sent": ss.Dropped}
		sh.SendLagMs = quantiles(s.EdgesMs, ss.SendLagHist)
		sh.AckLatencyMs = quantiles(s.EdgesMs, ss.AckLatencyHist)
		sh.PerDeviceIntervS = float64(r.Devices) / st.Rate
		if ss.HTTPReqs > 0 {
			sh.OnWireBytesAvg = ss.DataSentBytes / ss.HTTPReqs
		}
		// generator CPU and memory inside the step window
		end := sh.WindowStartMs + float64(st.DurationS)*1000
		var n int
		for _, c := range cpu {
			if c.Ms <= sh.WindowStartMs || c.Ms > end {
				continue
			}
			if sh.CoreBusyMax == nil {
				sh.CoreBusyMax = make([]float64, len(c.CoreBusy))
				sh.CoreBusyMean = make([]float64, len(c.CoreBusy))
			}
			for k, b := range c.CoreBusy {
				if k < len(sh.CoreBusyMax) {
					sh.CoreBusyMax[k] = max(sh.CoreBusyMax[k], b)
					sh.CoreBusyMean[k] += b
				}
			}
			sh.K6MemPeakB = max(sh.K6MemPeakB, c.K6MemB)
			n++
		}
		for k := range sh.CoreBusyMean {
			sh.CoreBusyMean[k] /= float64(n)
		}

		add := func(name string, pass, gate bool, detail string, a ...any) {
			sh.Checks = append(sh.Checks, Check{Name: name, Pass: pass, Gate: gate, Detail: fmt.Sprintf(detail, a...)})
			if gate && !pass {
				sh.Valid = false
			}
		}
		add("dropped_iterations", ss.Dropped == 0, true, "%.0f dropped (must be 0: seq/scheduled-time mapping needs it)", ss.Dropped)
		add("ticks_complete", sh.TicksExecuted == st.Ticks, true, "%d of %d planned ticks executed", sh.TicksExecuted, st.Ticks)
		add("achieved_rate", sh.AchievedRatio >= r.Health.MinAchievedRatio, true,
			"%d of %d planned sends (%.4f, limit %.2f)", sh.Sends, sh.PlannedSends, sh.AchievedRatio, r.Health.MinAchievedRatio)
		add("send_lag_p99", float64(sh.SendLagMs.P99) <= r.Health.SendLagP99Ms, true, "p99 <= %v ms (limit %v ms; p50 <= %v, max <= %v)",
			sh.SendLagMs.P99, r.Health.SendLagP99Ms, sh.SendLagMs.P50, sh.SendLagMs.Max)
		add("no_early_sends", ss.Early == 0, true, "%.0f sends more than %v ms before their scheduled time", ss.Early, r.Health.EarlyToleranceMs)
		outSum := ss.AckSuccess + ss.ExplicitFailure + ss.Unknown
		add("outcome_accounting", outSum == ss.Attempted, true, "ack %.0f + failure %.0f + unknown %.0f = %.0f attempted",
			ss.AckSuccess, ss.ExplicitFailure, ss.Unknown, ss.Attempted)
		rc := byStep[st.Index]
		if rc == nil {
			rc = &recCount{}
		}
		add("record_completeness", rc.nonSuccess == int64(ss.ExplicitFailure+ss.Unknown) && rc.early == int64(ss.Early), true,
			"%d non-success records for %.0f non-success outcomes, %d early records for %.0f early sends",
			rc.nonSuccess, ss.ExplicitFailure+ss.Unknown, rc.early, ss.Early)
		if r.Health.MaxCoreBusy > 0 {
			worst := 0.0
			for _, b := range sh.CoreBusyMax {
				worst = max(worst, b)
			}
			add("generator_core_busy", worst <= r.Health.MaxCoreBusy, true, "busiest core %.2f (limit %.2f)", worst, r.Health.MaxCoreBusy)
		}
		if !sh.Valid {
			h.Valid = false
			for _, c := range sh.Checks {
				if c.Gate && !c.Pass {
					h.Reasons = append(h.Reasons, fmt.Sprintf("step %s: %s: %s", st.Name, c.Name, c.Detail))
				}
			}
		}
		h.Steps = append(h.Steps, sh)
	}
	if s.VUsPeak >= float64(r.VUs.Max) {
		h.Warnings = append(h.Warnings, fmt.Sprintf("peak VUs %.0f reached vus.max %d: no headroom left", s.VUsPeak, r.VUs.Max))
	}
	if len(h.Reasons) > 0 {
		h.Valid = false
	}
	sort.Strings(h.Warnings)
	return &r, &s, h, nil
}
