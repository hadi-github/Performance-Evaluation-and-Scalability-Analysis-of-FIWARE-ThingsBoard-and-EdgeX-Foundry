package arrivals

import (
	"math"
	"testing"

	"iotbench/internal/plan"
)

func resolved(model string, factor float64) *plan.Resolved {
	r := &plan.Resolved{Plan: plan.Plan{Seed: 5, Arrivals: plan.Arrivals{Model: model, TickFactor: factor}}, SendProb: 1}
	tick := 100.0
	if model == "thinning" {
		tick, r.SendProb = 100*factor, 1/factor
	}
	r.Steps = []plan.Step{{Name: "s", Rate: 100, TickRate: tick, DurationS: 600, Ticks: int64(tick * 600)}}
	return r
}

func TestConstantArrivals(t *testing.T) {
	rep := Analyse(resolved("constant", 0), nil)[0]
	if rep.Planned.CV > 1e-9 || math.Abs(rep.Planned.MeanMs-10) > 1e-9 || rep.Planned.KSvsExp < rep.Planned.KSCrit01 {
		t.Fatalf("constant: %+v (must have CV 0 and fail the exponential KS test)", rep.Planned)
	}
}

func TestThinningApproximatesPoisson(t *testing.T) {
	rep := Analyse(resolved("thinning", 10), nil)[0]
	if math.Abs(rep.Planned.CV-math.Sqrt(0.9)) > 0.03 || math.Abs(rep.Planned.MeanMs-10) > 0.3 {
		t.Fatalf("thinning: %+v (expect CV ~ sqrt(1-p) = 0.949, mean 10 ms)", rep.Planned)
	}
}
