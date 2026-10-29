// Package explore applies the exploratory capacity check's pass rule (docs/methodology/exploratory-capacity.md), declared
// before any data on 2026-10-22 and not to be changed after seeing results.
//
// A step PASSES when all of these hold:
//   - generator health (health.json): 0 dropped iterations, achieved >= 99%, send-lag p99 <= 10 ms, complete ticks,
//     no early sends. Otherwise the step is INVALID (generator), not a platform failure;
//   - unsuccessful responses (explicit failures + unknown) / attempted <= 0.1%;
//   - latency: the ack p99 of every minute <= 1 s, and p99(min 3) <= max(1.5 x p99(min 1), p99(min 1) + 50 ms);
//   - outstanding work (sent - reached sink, unique IDs): average over the last 10 s of minute 3 <= average over the
//     last 10 s of minute 1 + rate x 1 s.
//
// "Sent by t" counts the planned seqs scheduled <= t. That is valid because the send-lag gate bounds dispatch to
// <= 10 ms after the schedule. "Reached the sink by t" is the first sink receipt of each unique (run_id, seq).
// Both use the load host's clock. Evaluation stops at the first step that does not pass; later steps are
// "not evaluated". The warm-up step is not evaluated.
package explore

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

// Declared constants of the pass rule.
const (
	MaxUnsuccessfulRatio = 0.001
	MaxMinuteP99Ms       = 1000.0
	TrendFactor          = 1.5
	TrendSlackMs         = 50.0
	OutstandingWindowS   = 10.0
	OutstandingSlackS    = 1.0
)

// StorePoint is one sample of a cumulative stored-message counter (D23: goodput at persistence), e.g. the
// postgres-exporter series pg_stat_user_tables_n_tup_ins of EdgeX's core_data.reading table.
type StorePoint struct {
	TMs   float64
	Count float64
}

// SinkObs is one sink observation (first receipts are derived here).
type SinkObs struct {
	RunID  string
	Seq    int64
	RecvMs float64
}

// Snapshot is one container's line of cpu-snapshot.sh.
type Snapshot struct {
	TimeS     float64
	Container string
	ID        string
	UsageUsec int64
	StartedAt string
	Cpuset    string
}

type MinuteStats struct {
	Minute int      `json:"minute"`
	Count  float64  `json:"count"`
	P99Ms  report.F `json:"ack_p99_ms"`
}

type ContainerCPU struct {
	Container string  `json:"container"`
	Cpuset    string  `json:"cpuset"`
	Cores     int     `json:"cores_allocated"`
	CPUSec    float64 `json:"cpu_seconds"`
	D         float64 `json:"D_cpu_s_per_msg"`
	B         float64 `json:"B_msgs_per_s"` // cores / D
	Complete  bool    `json:"complete"`
}

type PoolBound struct {
	Cpuset     string   `json:"cpuset"`
	Cores      int      `json:"cores"`
	Containers []string `json:"containers"`
	SumD       float64  `json:"sum_D"`
	Bound      float64  `json:"bound_msgs_per_s"`
}

type Step struct {
	Index          int            `json:"index"`
	Name           string         `json:"name"`
	Rate           float64        `json:"rate"`
	Verdict        string         `json:"verdict"` // PASS | FAIL | INVALID | not evaluated | warm-up
	Reasons        []string       `json:"reasons"`
	Attempted      float64        `json:"attempted"`
	AckSuccess     float64        `json:"ack_success"`
	Unsuccessful   float64        `json:"unsuccessful_ratio"`
	SendLagP99Ms   report.F       `json:"send_lag_p99_ms"`
	Minutes        []MinuteStats  `json:"minutes"`
	OutstandingM1  report.F       `json:"outstanding_avg_last10s_min1"`
	OutstandingM3  report.F       `json:"outstanding_avg_last10s_min3"`
	OutstandingMin []report.F     `json:"outstanding_avg_last10s_by_minute"`
	CPU            []ContainerCPU `json:"cpu,omitempty"`
	Pools          []PoolBound    `json:"pool_bounds,omitempty"`
	MinB           float64        `json:"utilization_law_bound,omitempty"`
	MinBContainer  string         `json:"bottleneck_container,omitempty"`
	// D23: the same rule with outstanding work measured at persistence instead of the sink (only with a store series)
	VerdictPersist        string     `json:"verdict_persistence,omitempty"`
	ReasonsPersist        []string   `json:"reasons_persistence,omitempty"`
	OutstandingPersistMin []report.F `json:"outstanding_persistence_avg_last10s_by_minute,omitempty"`
}

type Result struct {
	RunID             string  `json:"run_id"`
	Steps             []Step  `json:"steps"`
	HighestPassRate   float64 `json:"highest_passing_rate"`
	FirstFailRate     float64 `json:"first_failing_rate"`
	FirstFailVerdict  string  `json:"first_failing_verdict"`
	SinkUnique        int     `json:"sink_unique"`
	SinkDuplicates    int     `json:"sink_duplicates"`
	SinkOtherRun      int     `json:"sink_other_run_ids"`
	Planned           int64   `json:"planned_messages"`
	ReachedSinkOfPlan int     `json:"planned_reached_sink"`
	// D23 persistence evaluation (present only with a store series)
	Persist *PersistSummary `json:"persistence,omitempty"`
}

type PersistSummary struct {
	Source            string  `json:"source"`
	Basis             string  `json:"basis"`
	Points            int     `json:"points"`
	MedianSpacingMs   float64 `json:"median_sample_spacing_ms"` // time resolution of the stored count
	Baseline          float64 `json:"baseline_count_at_warmup_start"`
	StoredAtLastPoint float64 `json:"stored_at_last_point"`
	LastPointMs       float64 `json:"last_point_ms"`
	HighestPassRate   float64 `json:"highest_passing_rate"`
	FirstFailRate     float64 `json:"first_failing_rate"`
	FirstFailVerdict  string  `json:"first_failing_verdict"`
}

func cpusetCores(cs string) int {
	if cs == "" || cs == "none" {
		return 0
	}
	n := 0
	for _, part := range strings.Split(cs, ",") {
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			a, _ := strconv.Atoi(lo)
			b, _ := strconv.Atoi(hi)
			n += b - a + 1
		} else {
			n++
		}
	}
	return n
}

// nearest returns, per container, the snapshot closest to t (seconds).
func nearest(snaps []Snapshot, t float64) map[string]Snapshot {
	best := map[string]Snapshot{}
	for _, s := range snaps {
		if b, ok := best[s.Container]; !ok || math.Abs(s.TimeS-t) < math.Abs(b.TimeS-t) {
			best[s.Container] = s
		}
	}
	return best
}

// Evaluate applies the rule. health and summary come from the run directory; obs are all sink observations
// (any run); snaps the CPU snapshot series of the SUT during the run (may be empty).
//
// store, if not empty, is a cumulative stored-message counter of the platform; the rule is then also applied with
// outstanding work = sent - stored (D23), the stored count taken relative to its value at the warm-up start and
// interpolated linearly between samples.
func Evaluate(r *plan.Resolved, sum *report.Summary, h *report.Health, obs []SinkObs, snaps []Snapshot, store []StorePoint) *Result {
	res := &Result{RunID: r.RunID, Planned: r.TotalTicks}

	// first sink receipt per seq of this run
	first := map[int64]float64{}
	for _, o := range obs {
		if o.RunID != r.RunID {
			res.SinkOtherRun++
			continue
		}
		if t, ok := first[o.Seq]; ok {
			res.SinkDuplicates++
			if o.RecvMs < t {
				first[o.Seq] = o.RecvMs
			}
			continue
		}
		first[o.Seq] = o.RecvMs
	}
	res.SinkUnique = len(first)
	var recv []float64
	for seq, t := range first {
		if seq >= 0 && seq < r.TotalTicks {
			recv = append(recv, t)
			res.ReachedSinkOfPlan++
		}
	}
	sort.Float64s(recv)

	// planned send times (ms) of every seq, from each step's scenario start
	type window struct{ t0, period float64 }
	win := make([]window, len(r.Steps))
	for i, st := range r.Steps {
		t0 := float64(st.StartS) * 1000
		if i < len(sum.Steps) && sum.Steps[i].ScenarioStartMs > 0 {
			t0 = sum.Steps[i].ScenarioStartMs
		} else if sum.SetupEndMs > 0 {
			t0 = sum.SetupEndMs + float64(st.StartS)*1000
		}
		win[i] = window{t0, 1000 / st.TickRate}
	}
	// Exact time-average of sent(t) - reached(t) over [from, to): each event at time e before `to` adds
	// to - max(from, e) to the integral. Point sampling would be biased by one message whenever the sample instants
	// line up with the send schedule (they do: whole multiples of the tick period after the scenario start).
	sentArea := func(from, to float64) float64 {
		area := 0.0
		for i, st := range r.Steps {
			if to <= win[i].t0 {
				continue
			}
			last := math.Min(math.Ceil((to-win[i].t0)/win[i].period), float64(st.Ticks))     // k with sched(k) < to
			before := math.Min(math.Max(math.Ceil((from-win[i].t0)/win[i].period), 0), last) // k with sched(k) < from
			area += before * (to - from)
			for k := before; k < last; k++ {
				area += to - (win[i].t0 + k*win[i].period)
			}
		}
		return area
	}
	avgOutstanding := func(from, to float64) float64 {
		if to <= from {
			return math.NaN()
		}
		area := sentArea(from, to)
		lo := sort.SearchFloat64s(recv, from)
		hi := sort.SearchFloat64s(recv, to)
		area -= float64(lo) * (to - from)
		for _, e := range recv[lo:hi] {
			area -= to - e
		}
		return area / (to - from)
	}

	// persistence: stored(t) piecewise linear through the samples, relative to the warm-up start
	var avgPersist func(from, to float64) float64
	if len(store) > 0 {
		sort.Slice(store, func(a, b int) bool { return store[a].TMs < store[b].TMs })
		at := func(t float64) float64 {
			j := sort.Search(len(store), func(k int) bool { return store[k].TMs >= t })
			switch {
			case j == 0:
				return store[0].Count
			case j == len(store):
				return store[len(store)-1].Count
			}
			a, b := store[j-1], store[j]
			return a.Count + (b.Count-a.Count)*(t-a.TMs)/(b.TMs-a.TMs)
		}
		base := at(win[0].t0)
		var gaps []float64
		for k := 1; k < len(store); k++ {
			gaps = append(gaps, store[k].TMs-store[k-1].TMs)
		}
		sort.Float64s(gaps)
		last := store[len(store)-1]
		res.Persist = &PersistSummary{Points: len(store), Baseline: base, StoredAtLastPoint: last.Count - base, LastPointMs: last.TMs,
			Basis: "diagnostic proxy: cumulative table-level counter (e.g. pg_stat_user_tables n_tup_ins); not a per-message durability proof (D33)"}
		if len(gaps) > 0 {
			res.Persist.MedianSpacingMs = gaps[len(gaps)/2]
		}
		storedArea := func(from, to float64) float64 { // integral of stored(t) - base over [from, to)
			pts := []float64{from}
			for _, p := range store {
				if p.TMs > from && p.TMs < to {
					pts = append(pts, p.TMs)
				}
			}
			pts = append(pts, to)
			area := 0.0
			for k := 1; k < len(pts); k++ {
				area += (at(pts[k-1]) + at(pts[k]) - 2*base) / 2 * (pts[k] - pts[k-1])
			}
			return area
		}
		avgPersist = func(from, to float64) float64 {
			if to <= from {
				return math.NaN()
			}
			return (sentArea(from, to) - storedArea(from, to)) / (to - from)
		}
	}

	stopped, stoppedPersist := false, false
	for i, st := range r.Steps {
		if i >= len(h.Steps) || i >= len(sum.Steps) {
			break
		}
		hs, ss := h.Steps[i], sum.Steps[i]
		step := Step{Index: st.Index, Name: st.Name, Rate: st.Rate, Attempted: ss.Attempted, AckSuccess: ss.AckSuccess,
			SendLagP99Ms: hs.SendLagMs.P99}
		if ss.Attempted > 0 {
			step.Unsuccessful = (ss.ExplicitFailure + ss.Unknown) / ss.Attempted
		}
		for _, m := range hs.AckByMinute {
			step.Minutes = append(step.Minutes, MinuteStats{Minute: m.Minute, Count: m.Count, P99Ms: m.P99})
		}
		mins := int(math.Ceil(float64(st.DurationS) / 60))
		for m := 1; m <= mins; m++ {
			end := win[i].t0 + float64(m)*60000
			step.OutstandingMin = append(step.OutstandingMin, report.F(avgOutstanding(end-OutstandingWindowS*1000, end)))
		}
		if len(step.OutstandingMin) >= 3 {
			step.OutstandingM1, step.OutstandingM3 = step.OutstandingMin[0], step.OutstandingMin[2]
		}

		if avgPersist != nil {
			p := step // judged with the same rule, outstanding work at persistence
			p.Reasons, p.OutstandingMin = nil, nil
			for m := 1; m <= mins; m++ {
				end := win[i].t0 + float64(m)*60000
				p.OutstandingMin = append(p.OutstandingMin, report.F(avgPersist(end-OutstandingWindowS*1000, end)))
			}
			if len(p.OutstandingMin) >= 3 {
				p.OutstandingM1, p.OutstandingM3 = p.OutstandingMin[0], p.OutstandingMin[2]
			}
			step.OutstandingPersistMin = p.OutstandingMin
			switch {
			case st.Phase == "warmup":
				step.VerdictPersist = "warm-up"
			case stoppedPersist:
				step.VerdictPersist = "not evaluated"
			default:
				step.VerdictPersist = judge(&p, st, hs)
				step.ReasonsPersist = p.Reasons
				if step.VerdictPersist != "PASS" {
					stoppedPersist = true
					res.Persist.FirstFailRate, res.Persist.FirstFailVerdict = st.Rate, step.VerdictPersist
				} else {
					res.Persist.HighestPassRate = st.Rate
				}
			}
		}

		switch {
		case st.Phase == "warmup":
			step.Verdict = "warm-up"
		case stopped:
			step.Verdict = "not evaluated"
		default:
			step.Verdict = judge(&step, st, hs)
			if step.Verdict != "PASS" {
				stopped = true
				res.FirstFailRate, res.FirstFailVerdict = st.Rate, step.Verdict
			} else {
				res.HighestPassRate = st.Rate
				cpu(&step, win[i].t0, float64(st.DurationS), snaps)
			}
		}
		res.Steps = append(res.Steps, step)
	}
	return res
}

func judge(step *Step, st plan.Step, hs report.StepHealth) string {
	for _, c := range hs.Checks {
		if c.Gate && !c.Pass {
			step.Reasons = append(step.Reasons, "generator: "+c.Name+": "+c.Detail)
		}
	}
	if len(step.Reasons) > 0 {
		return "INVALID"
	}
	if step.Unsuccessful > MaxUnsuccessfulRatio {
		step.Reasons = append(step.Reasons, fmt.Sprintf("unsuccessful %.4f%% > 0.1%%", step.Unsuccessful*100))
	}
	if len(step.Minutes) < 3 {
		step.Reasons = append(step.Reasons, fmt.Sprintf("only %d minutes of latency data (need 3)", len(step.Minutes)))
	} else {
		for _, m := range step.Minutes {
			if p := float64(m.P99Ms); m.Count == 0 || math.IsNaN(p) || p > MaxMinuteP99Ms {
				step.Reasons = append(step.Reasons, fmt.Sprintf("minute %d ack p99 %.1f ms > 1 s (n=%.0f)", m.Minute, m.P99Ms, m.Count))
			}
		}
		p1, p3 := float64(step.Minutes[0].P99Ms), float64(step.Minutes[2].P99Ms)
		if limit := math.Max(TrendFactor*p1, p1+TrendSlackMs); p3 > limit {
			step.Reasons = append(step.Reasons, fmt.Sprintf("ack p99 trend: minute 3 %.1f ms > max(1.5 x %.1f, %.1f + 50) = %.1f ms", p3, p1, p1, limit))
		}
	}
	o1, o3 := float64(step.OutstandingM1), float64(step.OutstandingM3)
	if len(step.OutstandingMin) < 3 || math.IsNaN(o1) || math.IsNaN(o3) {
		step.Reasons = append(step.Reasons, "outstanding work not computable")
	} else if limit := o1 + st.Rate*OutstandingSlackS; o3 > limit {
		step.Reasons = append(step.Reasons, fmt.Sprintf("outstanding work grows: minute 3 %.1f > minute 1 %.1f + rate x 1 s = %.1f",
			o3, o1, limit))
	}
	if len(step.Reasons) > 0 {
		return "FAIL"
	}
	return "PASS"
}

// cpu computes D_i and B_i over the step from the snapshots nearest to its start and end.
func cpu(step *Step, t0Ms, durS float64, snaps []Snapshot) {
	if len(snaps) == 0 || step.AckSuccess == 0 {
		return
	}
	a, b := nearest(snaps, t0Ms/1000), nearest(snaps, t0Ms/1000+durS)
	pools := map[string]*PoolBound{}
	step.MinB = math.Inf(1)
	for name, s0 := range a {
		s1, ok := b[name]
		cc := ContainerCPU{Container: name, Cpuset: s0.Cpuset, Cores: cpusetCores(s0.Cpuset)}
		cc.Complete = ok && s1.ID == s0.ID && s1.StartedAt == s0.StartedAt && s1.UsageUsec >= s0.UsageUsec && s1.TimeS > s0.TimeS
		if cc.Complete {
			cc.CPUSec = float64(s1.UsageUsec-s0.UsageUsec) / 1e6
			cc.D = cc.CPUSec / step.AckSuccess
			if cc.D > 0 && cc.Cores > 0 {
				cc.B = float64(cc.Cores) / cc.D
				if cc.B < step.MinB {
					step.MinB, step.MinBContainer = cc.B, name
				}
			}
			p := pools[cc.Cpuset]
			if p == nil {
				p = &PoolBound{Cpuset: cc.Cpuset, Cores: cc.Cores}
				pools[cc.Cpuset] = p
			}
			p.Containers = append(p.Containers, name)
			p.SumD += cc.D
		}
		step.CPU = append(step.CPU, cc)
	}
	if math.IsInf(step.MinB, 1) {
		step.MinB = 0
	}
	sort.Slice(step.CPU, func(i, j int) bool { return step.CPU[i].Container < step.CPU[j].Container })
	for _, p := range pools {
		if p.SumD > 0 && p.Cores > 0 {
			p.Bound = float64(p.Cores) / p.SumD
		}
		sort.Strings(p.Containers)
		step.Pools = append(step.Pools, *p)
	}
	sort.Slice(step.Pools, func(i, j int) bool { return step.Pools[i].Cpuset < step.Pools[j].Cpuset })
}
