// Package audit reconciles a run's acknowledged identities with the messages observed at the external sink and
// with the identities independently retrievable from the platform's history store.
//
// It is the implementation of the measurement-integrity set selected on 2026-10-26 (D33): acknowledged -> sink,
// acknowledged -> history, duplicates at each observation point, ambiguous outcomes, and ThingsBoard-style
// same-millisecond collision risk. It does not invent a per-message commit timestamp: storage is established by a
// retrievable identity, not by a durable-commit time.
//
// The sets are exact only when the generator reported no dropped iterations (D15). With drops, the acknowledged
// set is a lower bound and the result carries that limitation.
package audit

import (
	"fmt"
	"math"
	"sort"

	"iotbench/internal/arrivals"
	"iotbench/internal/hash"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

// SinkRecord is one observation at the external sink (cmd/sink NDJSON).
type SinkRecord struct {
	RunID  string  `json:"run_id"`
	Seq    int64   `json:"seq"`
	RecvMs float64 `json:"recv_ms"`
	Dup    bool    `json:"dup"`
}

// HistoryRecord is one identity retrievable from the platform's history store. Device and TsMs are optional: they
// are present only when the export can supply them, and are required for collision analysis.
type HistoryRecord struct {
	RunID  string  `json:"run_id"`
	Seq    int64   `json:"seq"`
	Device string  `json:"device,omitempty"`
	TsMs   float64 `json:"ts,omitempty"`
	HasTs  bool    `json:"-"`
}

// Options controls the report. DeadlineMs is the SLO deadline used for the within-deadline count; it is not a
// verdict. MaxExamples caps every example list so a failed run does not produce an unbounded report.
type Options struct {
	DeadlineMs  float64
	MaxExamples int
}

// Planned is one planned message identity and its schedule.
type Planned struct {
	Seq     int64   `json:"seq"`
	Device  int     `json:"device"`
	Step    string  `json:"step"`
	SchedMs float64 `json:"scheduled_send_ms"`
}

// SetSummary is the accounting at one observation point.
type SetSummary struct {
	Available       bool    `json:"available"`
	Unique          int     `json:"unique_identities"`
	Missing         int     `json:"missing_acknowledged"`
	MissingSeqs     []int64 `json:"missing_seqs,omitempty"`
	DuplicateSeqs   int     `json:"sequences_with_extra_copies"`
	DuplicateCopies int     `json:"extra_copies"`
	Duplicates      []int64 `json:"duplicate_seqs,omitempty"`
	Orphans         int     `json:"orphan_identities"`
	OrphanSeqs      []int64 `json:"orphan_seqs,omitempty"`
	OtherRunIDs     int     `json:"identities_with_other_run_id"`
	Ambiguous       []int64 `json:"ambiguous_outcome_but_observed,omitempty"`
}

// StepQualified is the correctness-qualified accounting for one plan step. A sweep's aggregate quantiles mix
// different offered rates; the per-step table is the reporting unit.
type StepQualified struct {
	Name           string  `json:"name"`
	Phase          string  `json:"phase"`
	Rate           float64 `json:"rate_per_s"`
	WindowMs       float64 `json:"window_ms"`
	Planned        int     `json:"planned_identities"`
	Identities     int     `json:"qualified_identities"`
	WithinDeadline int     `json:"identities_within_deadline"`
	P50Ms          float64 `json:"delivery_latency_p50_ms"`
	P95Ms          float64 `json:"delivery_latency_p95_ms"`
	P99Ms          float64 `json:"delivery_latency_p99_ms"`
}

// Qualified is the identity-verified set: acknowledged, observed at the sink, and retrievable from history.
type Qualified struct {
	Available       bool            `json:"available"`
	Identities      int             `json:"identities"`
	WindowMs        float64         `json:"measured_window_ms"`
	RatePerS        float64         `json:"identity_qualified_rate_proxy_per_s"`
	DeadlineMs      float64         `json:"deadline_ms"`
	WithinDeadline  int             `json:"identities_within_deadline"`
	P50Ms           float64         `json:"delivery_latency_p50_ms"`
	P95Ms           float64         `json:"delivery_latency_p95_ms"`
	P99Ms           float64         `json:"delivery_latency_p99_ms"`
	ByStep          []StepQualified `json:"by_step,omitempty"`
	NotWithinReason string          `json:"note,omitempty"`
}

// Collision is the ThingsBoard-style same-millisecond overwrite analysis. PlannedPairs is a risk bound computed
// from the schedule; StoredSameTsGroups is evidence from the history export.
type Collision struct {
	Available          bool     `json:"available"`
	Basis              string   `json:"basis,omitempty"`
	PlannedSameMsPairs int      `json:"planned_same_ms_pairs_per_device"`
	StoredSameTsGroups int      `json:"stored_same_timestamp_groups"`
	Examples           []string `json:"examples,omitempty"`
}

type Result struct {
	RunID       string            `json:"run_id"`
	Planned     int64             `json:"planned_identities"`
	AckSuccess  int64             `json:"acknowledged_identities"`
	AckFailure  int64             `json:"explicit_failure_identities"`
	Unknown     int64             `json:"unknown_outcome_identities"`
	DroppedIter int64             `json:"generator_dropped_iterations"`
	Sink        SetSummary        `json:"sink"`
	History     SetSummary        `json:"history"`
	Qualified   *Qualified        `json:"correctness_qualified,omitempty"`
	Collisions  *Collision        `json:"timestamp_collisions,omitempty"`
	Clean       bool              `json:"clean"`
	Labels      map[string]string `json:"labels"`
	Limitations []string          `json:"limitations,omitempty"`
}

func limit(v []int64, n int) []int64 {
	if n <= 0 {
		n = 20
	}
	if len(v) <= n {
		return v
	}
	return v[:n]
}

func limits(v []string, n int) []string {
	if n <= 0 {
		n = 20
	}
	if len(v) <= n {
		return v
	}
	return v[:n]
}

// PlannedMessages reproduces the planned identities and schedule from the resolved plan and the run summary's
// scenario starts. It mirrors the outstanding-work time base of internal/explore.
func PlannedMessages(r *plan.Resolved, sum *report.Summary) []Planned {
	var out []Planned
	threshold := hash.Threshold(r.SendProb)
	devices := r.Devices
	if devices <= 0 {
		devices = 1
	}
	for i, st := range r.Steps {
		t0 := stepStart(r, sum, i)
		period := 1000 / st.TickRate
		for k := int64(0); k < st.Ticks; k++ {
			seq := st.SeqOffset + k
			if r.Arrivals.Model == "thinning" && !hash.Selected(r.Seed, uint32(seq), threshold) {
				continue
			}
			out = append(out, Planned{
				Seq:     seq,
				Device:  int(seq % int64(devices)),
				Step:    st.Name,
				SchedMs: t0 + float64(k)*period,
			})
		}
	}
	return out
}

// stepStart returns the absolute (epoch ms) start of step i, using the summary's scenario start when present and
// the plan's relative start otherwise.
func stepStart(r *plan.Resolved, sum *report.Summary, i int) float64 {
	st := r.Steps[i]
	if sum != nil && i < len(sum.Steps) && sum.Steps[i].ScenarioStartMs > 0 {
		return sum.Steps[i].ScenarioStartMs
	}
	if sum != nil && sum.SetupEndMs > 0 {
		return sum.SetupEndMs + float64(st.StartS)*1000
	}
	return float64(st.StartS) * 1000
}

// measuredWindow returns the absolute scheduled window of the non-warm-up steps. If there is no such step, it
// returns the whole plan.
func measuredWindow(r *plan.Resolved, sum *report.Summary) (float64, float64, bool) {
	var lo, hi float64
	set := false
	for i, st := range r.Steps {
		if st.Phase == "warmup" {
			continue
		}
		a := stepStart(r, sum, i)
		b := a + float64(st.DurationS)*1000
		if !set || a < lo {
			lo = a
		}
		if !set || b > hi {
			hi = b
		}
		set = true
	}
	if !set {
		return 0, float64(r.DurationS) * 1000, false
	}
	return lo, hi, true
}

// Analyse performs the reconciliation. hasHistory distinguishes "no history source" from "an empty history".
func Analyse(r *plan.Resolved, sum *report.Summary, records []report.Record, sink []SinkRecord, history []HistoryRecord, hasHistory bool, opts Options) *Result {
	if opts.MaxExamples <= 0 {
		opts.MaxExamples = 20
	}
	planned := PlannedMessages(r, sum)
	res := &Result{RunID: r.RunID, Planned: int64(len(planned)), Labels: map[string]string{
		"acknowledged": "measured fact: planned identity with an ack_success outcome (no drop)",
		"sink":         "measured fact: first receipt of each (run_id, seq) at the external sink",
		"history":      "measured fact conditional on the exporter: identity retrievable from the platform history store",
		"qualified":    "derived set (acknowledged AND sink AND history); not a platform verdict",
		"rate":         "proxy: identity count / resolved measured window; not a C_SLO and not a pass/fail rule",
		"collision":    "planned pairs are a risk bound; stored same-timestamp groups are direct evidence of overwrite",
		"missing":      "a missing sink receipt means not observed at the sink, and a missing history identity means not retrievable; neither is proof of platform loss, and no history export means nothing is confirmed stored",
		"ambiguous":    "an outcome other than ack_success is ambiguous: a delivered or stored identity with such an outcome is listed, never counted as acknowledged or qualified",
	}}
	bySeq := make(map[int64]Planned, len(planned))
	for _, p := range planned {
		bySeq[p.Seq] = p
	}

	// Outcomes: records.ndjson holds non-success outcomes and early sends (D18). A seq without a record is
	// ack_success. Duplicate outcome records are their own limitation.
	outcome := map[int64]string{}
	seenOutcome := map[int64]bool{}
	dupOutcome := 0
	for _, rc := range records {
		if seenOutcome[rc.Seq] {
			dupOutcome++
		}
		seenOutcome[rc.Seq] = true
		if rc.Outcome != "ack_success" {
			outcome[rc.Seq] = rc.Outcome
		}
	}

	// Sink: first receipt per seq, extra observations, orphans, other runs.
	sinkFirst := map[int64]float64{}
	sinkCopies := map[int64]int{}
	for _, s := range sink {
		if s.RunID != r.RunID {
			res.Sink.OtherRunIDs++
			continue
		}
		if _, ok := bySeq[s.Seq]; !ok {
			res.Sink.Orphans++
			if len(res.Sink.OrphanSeqs) < opts.MaxExamples {
				res.Sink.OrphanSeqs = append(res.Sink.OrphanSeqs, s.Seq)
			}
			continue
		}
		if t, ok := sinkFirst[s.Seq]; !ok || s.RecvMs < t {
			sinkFirst[s.Seq] = s.RecvMs
		}
		sinkCopies[s.Seq]++
	}
	res.Sink.Available = true
	res.Sink.Unique = len(sinkFirst)
	for seq, n := range sinkCopies {
		if n > 1 {
			res.Sink.DuplicateSeqs++
			res.Sink.DuplicateCopies += n - 1
			if len(res.Sink.Duplicates) < opts.MaxExamples {
				res.Sink.Duplicates = append(res.Sink.Duplicates, seq)
			}
		}
	}

	// History: unique retrievable identities, extra rows, orphans, other runs.
	histFirst := map[int64]HistoryRecord{}
	histCopies := map[int64]int{}
	if hasHistory {
		for _, h := range history {
			if h.RunID != r.RunID {
				res.History.OtherRunIDs++
				continue
			}
			if _, ok := bySeq[h.Seq]; !ok {
				res.History.Orphans++
				if len(res.History.OrphanSeqs) < opts.MaxExamples {
					res.History.OrphanSeqs = append(res.History.OrphanSeqs, h.Seq)
				}
				continue
			}
			if _, ok := histFirst[h.Seq]; !ok {
				histFirst[h.Seq] = h
			}
			histCopies[h.Seq]++
		}
		res.History.Available = true
		res.History.Unique = len(histFirst)
		for seq, n := range histCopies {
			if n > 1 {
				res.History.DuplicateSeqs++
				res.History.DuplicateCopies += n - 1
				if len(res.History.Duplicates) < opts.MaxExamples {
					res.History.Duplicates = append(res.History.Duplicates, seq)
				}
			}
		}
	}

	// Acknowledged sets and per-point missing/ambiguous accounting.
	for _, p := range planned {
		o := outcome[p.Seq]
		switch o {
		case "", "ack_success":
			res.AckSuccess++
			if _, ok := sinkFirst[p.Seq]; !ok {
				res.Sink.Missing++
				if len(res.Sink.MissingSeqs) < opts.MaxExamples {
					res.Sink.MissingSeqs = append(res.Sink.MissingSeqs, p.Seq)
				}
			}
			if hasHistory {
				if _, ok := histFirst[p.Seq]; !ok {
					res.History.Missing++
					if len(res.History.MissingSeqs) < opts.MaxExamples {
						res.History.MissingSeqs = append(res.History.MissingSeqs, p.Seq)
					}
				}
			}
		case "explicit_failure":
			res.AckFailure++
		case "unknown":
			res.Unknown++
		default:
			res.Unknown++
		}
		if o != "" && o != "ack_success" {
			if _, ok := sinkFirst[p.Seq]; ok {
				res.Sink.Ambiguous = append(res.Sink.Ambiguous, p.Seq)
			}
			if hasHistory {
				if _, ok := histFirst[p.Seq]; ok {
					res.History.Ambiguous = append(res.History.Ambiguous, p.Seq)
				}
			}
		}
	}
	res.Sink.Ambiguous = limit(res.Sink.Ambiguous, opts.MaxExamples)
	res.History.Ambiguous = limit(res.History.Ambiguous, opts.MaxExamples)
	sort.Slice(res.Sink.MissingSeqs, func(i, j int) bool { return res.Sink.MissingSeqs[i] < res.Sink.MissingSeqs[j] })
	sort.Slice(res.History.MissingSeqs, func(i, j int) bool { return res.History.MissingSeqs[i] < res.History.MissingSeqs[j] })

	// Correctness-qualified set: acknowledged, at the sink, retrievable in history, with a delivery latency.
	if hasHistory {
		q := &Qualified{Available: true, DeadlineMs: opts.DeadlineMs}
		lo, hi, ok := measuredWindow(r, sum)
		q.WindowMs = hi - lo
		if !ok {
			q.WindowMs = float64(r.DurationS) * 1000
		}
		var lat []float64
		for _, p := range planned {
			o := outcome[p.Seq]
			if o != "" && o != "ack_success" {
				continue
			}
			t, ok := sinkFirst[p.Seq]
			if !ok {
				continue
			}
			if _, ok := histFirst[p.Seq]; !ok {
				continue
			}
			if ok && q.WindowMs > 0 && (p.SchedMs < lo || p.SchedMs >= hi) {
				continue
			}
			q.Identities++
			lat = append(lat, t-p.SchedMs)
			if opts.DeadlineMs > 0 && t-p.SchedMs <= opts.DeadlineMs {
				q.WithinDeadline++
			}
		}
		if len(lat) > 0 {
			sort.Float64s(lat)
			q.P50Ms = arrivals.Quantile(lat, 0.50)
			q.P95Ms = arrivals.Quantile(lat, 0.95)
			q.P99Ms = arrivals.Quantile(lat, 0.99)
		}
		if q.WindowMs > 0 {
			q.RatePerS = float64(q.WithinDeadline) / (q.WindowMs / 1000)
		}
		if opts.DeadlineMs <= 0 {
			q.NotWithinReason = "no deadline supplied: the within-deadline count is unavailable"
		}
		for _, st := range r.Steps {
			sw := StepQualified{Name: st.Name, Phase: st.Phase, Rate: st.TickRate, WindowMs: float64(st.DurationS) * 1000}
			var slat []float64
			for _, p := range planned {
				if p.Step != st.Name {
					continue
				}
				sw.Planned++
				if o := outcome[p.Seq]; o != "" && o != "ack_success" {
					continue
				}
				t, ok := sinkFirst[p.Seq]
				if !ok {
					continue
				}
				if _, ok := histFirst[p.Seq]; !ok {
					continue
				}
				sw.Identities++
				slat = append(slat, t-p.SchedMs)
				if opts.DeadlineMs > 0 && t-p.SchedMs <= opts.DeadlineMs {
					sw.WithinDeadline++
				}
			}
			if len(slat) > 0 {
				sort.Float64s(slat)
				sw.P50Ms = arrivals.Quantile(slat, 0.50)
				sw.P95Ms = arrivals.Quantile(slat, 0.95)
				sw.P99Ms = arrivals.Quantile(slat, 0.99)
			}
			q.ByStep = append(q.ByStep, sw)
		}
		rates := map[float64]bool{}
		for _, st := range r.Steps {
			if st.Phase != "warmup" {
				rates[st.TickRate] = true
			}
		}
		if len(rates) > 1 {
			res.Limitations = append(res.Limitations, "aggregate qualified quantiles and the rate proxy span steps at different offered rates; read by_step for per-rate results")
		}
		res.Qualified = q
	}

	// ThingsBoard-style collision analysis.
	c := &Collision{}
	if hasHistory {
		c.Available = true
		c.Basis = "per-device same-millisecond key (ThingsBoard ts_kv is keyed by entity, key, ts)"
		// planned risk: consecutive messages of the same device whose scheduled times share a millisecond
		byDevice := map[int][]Planned{}
		for _, p := range planned {
			byDevice[p.Device] = append(byDevice[p.Device], p)
		}
		for _, v := range byDevice {
			sort.Slice(v, func(i, j int) bool { return v[i].SchedMs < v[j].SchedMs })
			for i := 1; i < len(v); i++ {
				if int64(math.Floor(v[i-1].SchedMs)) == int64(math.Floor(v[i].SchedMs)) {
					c.PlannedSameMsPairs++
					if len(c.Examples) < opts.MaxExamples {
						c.Examples = append(c.Examples, fmt.Sprintf("planned device %d seq %d/%d in the same ms (%.3f)", v[i].Device, v[i-1].Seq, v[i].Seq, v[i].SchedMs))
					}
				}
			}
		}
		// stored evidence: one device with more than one distinct seq at the same stored timestamp
		groups := map[string]map[int64]struct{}{}
		for _, h := range history {
			if h.RunID != r.RunID || h.Device == "" || !h.HasTs {
				continue
			}
			key := fmt.Sprintf("%s|%d", h.Device, int64(math.Round(h.TsMs)))
			if groups[key] == nil {
				groups[key] = map[int64]struct{}{}
			}
			groups[key][h.Seq] = struct{}{}
		}
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if len(groups[k]) > 1 {
				c.StoredSameTsGroups++
				if len(c.Examples) < opts.MaxExamples {
					var seqs []int64
					for s := range groups[k] {
						seqs = append(seqs, s)
					}
					sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
					c.Examples = append(c.Examples, fmt.Sprintf("stored %s holds %d seqs", k, len(seqs)))
				}
			}
		}
		c.Examples = limits(c.Examples, opts.MaxExamples)
	}
	res.Collisions = c

	// Limitations are explicit; an absent producer is never replaced by an invented number.
	if sum != nil {
		res.DroppedIter = int64(sum.DroppedTotal)
		if sum.DroppedTotal > 0 {
			res.Limitations = append(res.Limitations, fmt.Sprintf("generator dropped %.0f iterations: the acknowledged set is a lower bound", sum.DroppedTotal))
		}
	} else {
		res.Limitations = append(res.Limitations, "no k6 summary: the acknowledged set is not exact")
	}
	if !hasHistory {
		res.Limitations = append(res.Limitations, "history not supplied: store missing/duplicates and correctness-qualified goodput are unavailable")
	} else if c.StoredSameTsGroups == 0 && c.PlannedSameMsPairs == 0 && len(history) > 0 {
		// collision analysis was possible; no limitation
	}
	if hasHistory {
		withTs := 0
		for _, h := range history {
			if h.HasTs && h.Device != "" {
				withTs++
			}
		}
		if withTs == 0 && len(history) > 0 {
			res.Collisions.Available = false
			res.Limitations = append(res.Limitations, "history export lacks device/timestamp fields: timestamp-collision analysis unavailable")
		}
	}
	if dupOutcome > 0 {
		res.Limitations = append(res.Limitations, fmt.Sprintf("%d duplicate outcome record(s): the outcome map keeps one per seq", dupOutcome))
	}
	res.Limitations = append(res.Limitations, "no per-message durable-commit timestamp exists; history is a retrievable-identity check, not commit latency")

	// Clean needs the history evidence: without an export, nothing can be confirmed stored, so the run is not clean.
	res.Clean = hasHistory && res.Sink.Missing == 0 && res.History.Missing == 0 &&
		res.Sink.DuplicateCopies == 0 && res.History.DuplicateCopies == 0 &&
		res.Sink.Orphans == 0 && res.History.Orphans == 0
	return res
}
