// Package arrivals analyses the arrival process of a run: the planned send times (reproduced offline from the
// resolved plan and seed) and, if the endpoint's store is given, the arrival times the endpoint actually saw.
// For Poisson arrivals at rate lambda, inter-arrival gaps are exponential (CV = 1); constant arrivals have CV = 0;
// Bernoulli thinning with send probability p has geometric gaps in units of the tick period (CV = sqrt(1-p)).
package arrivals

import (
	"math"
	"sort"

	"iotbench/internal/hash"
	"iotbench/internal/plan"
)

type Stats struct {
	N        int     `json:"n_gaps"`
	MeanMs   float64 `json:"mean_gap_ms"`
	CV       float64 `json:"cv"`
	KSvsExp  float64 `json:"ks_vs_exponential"` // sup |F_emp - F_exp(lambda)|
	KSCrit01 float64 `json:"ks_critical_1pct"`  // 1.628 / sqrt(n)
	P99GapMs float64 `json:"p99_gap_ms"`
}

type StepReport struct {
	Name         string  `json:"name"`
	Lambda       float64 `json:"lambda"`
	TickRate     float64 `json:"tick_rate"`
	Planned      Stats   `json:"planned"`
	Received     *Stats  `json:"received,omitempty"`
	TimingErrP50 float64 `json:"receipt_minus_scheduled_p50_ms,omitempty"`
	TimingErrP99 float64 `json:"receipt_minus_scheduled_p99_ms,omitempty"`
	Matched      int     `json:"matched_messages,omitempty"`
}

// Planned returns, per step, the sorted planned send times (ms, relative to the step start) of the messages the
// plan sends.
func Planned(r *plan.Resolved, s plan.Step) (seqs []int64, times []float64) {
	t := hash.Threshold(r.SendProb)
	for i := int64(0); i < s.Ticks; i++ {
		seq := s.SeqOffset + i
		if r.Arrivals.Model == "thinning" && !hash.Selected(r.Seed, uint32(seq), t) {
			continue
		}
		seqs = append(seqs, seq)
		times = append(times, float64(i)*1000/s.TickRate)
	}
	return
}

func gapStats(times []float64, lambda float64) Stats {
	var gaps []float64
	for i := 1; i < len(times); i++ {
		gaps = append(gaps, times[i]-times[i-1])
	}
	st := Stats{N: len(gaps)}
	if len(gaps) < 2 {
		return st
	}
	sum, sq := 0.0, 0.0
	for _, g := range gaps {
		sum += g
	}
	st.MeanMs = sum / float64(len(gaps))
	for _, g := range gaps {
		sq += (g - st.MeanMs) * (g - st.MeanMs)
	}
	st.CV = math.Sqrt(sq/float64(len(gaps)-1)) / st.MeanMs
	sort.Float64s(gaps)
	st.P99GapMs = gaps[int(math.Ceil(0.99*float64(len(gaps))))-1]
	rate := lambda / 1000 // per ms
	d := 0.0
	for i, g := range gaps {
		f := 1 - math.Exp(-rate*g)
		lo, hi := float64(i)/float64(len(gaps)), float64(i+1)/float64(len(gaps))
		d = math.Max(d, math.Max(math.Abs(f-lo), math.Abs(hi-f)))
	}
	st.KSvsExp = d
	st.KSCrit01 = 1.628 / math.Sqrt(float64(len(gaps)))
	return st
}

// Received is one stored message with its receipt time at the endpoint.
type Received struct {
	Seq    int64
	RecvMs float64
	Sched  float64
}

// Analyse builds the report; recv may be nil.
func Analyse(r *plan.Resolved, recv []Received) []StepReport {
	bySeq := map[int64]Received{}
	for _, x := range recv {
		bySeq[x.Seq] = x
	}
	var out []StepReport
	for _, s := range r.Steps {
		seqs, times := Planned(r, s)
		rep := StepReport{Name: s.Name, Lambda: s.Rate, TickRate: s.TickRate, Planned: gapStats(times, s.Rate)}
		if len(recv) > 0 {
			var rt, errs []float64
			for _, q := range seqs {
				if x, ok := bySeq[q]; ok {
					rt = append(rt, x.RecvMs)
					errs = append(errs, x.RecvMs-x.Sched)
				}
			}
			sort.Float64s(rt)
			st := gapStats(rt, s.Rate)
			rep.Received, rep.Matched = &st, len(rt)
			if len(errs) > 0 {
				sort.Float64s(errs)
				rep.TimingErrP50 = errs[len(errs)/2]
				rep.TimingErrP99 = errs[int(math.Ceil(0.99*float64(len(errs))))-1]
			}
		}
		out = append(out, rep)
	}
	return out
}
