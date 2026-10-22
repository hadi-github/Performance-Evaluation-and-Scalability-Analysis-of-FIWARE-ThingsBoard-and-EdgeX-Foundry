package explore

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

const t0 = 1_000_000.0 // ms

// fixture: warm-up (60 s at 10/s), then steps at rate 10/s for 180 s each.
func setup(nSteps int) (*plan.Resolved, *report.Summary, *report.Health) {
	r := &plan.Resolved{RunID: "EXPL-test"}
	s := &report.Summary{}
	h := &report.Health{}
	var seq int64
	start := 0
	add := func(phase string, dur int) {
		i := len(r.Steps)
		st := plan.Step{Index: i, Name: phase, Phase: phase, Rate: 10, TickRate: 10, DurationS: dur, StartS: start,
			Ticks: int64(10 * dur), SeqOffset: seq}
		r.Steps = append(r.Steps, st)
		s.Steps = append(s.Steps, report.StepSummary{Index: i, ScenarioStartMs: t0 + float64(start)*1000,
			Attempted: float64(st.Ticks), AckSuccess: float64(st.Ticks)})
		hs := report.StepHealth{Index: i, Checks: []report.Check{{Name: "dropped_iterations", Pass: true, Gate: true}}}
		for m := 1; m <= int(math.Ceil(float64(dur)/60)); m++ {
			hs.AckByMinute = append(hs.AckByMinute, report.Quantiles{Minute: m, Count: 600, P99: 100})
		}
		h.Steps = append(h.Steps, hs)
		seq += st.Ticks
		start += dur
	}
	add("warmup", 60)
	for k := 0; k < nSteps; k++ {
		add("sweep", 180)
	}
	r.TotalTicks = seq
	return r, s, h
}

// every planned message reaches the sink delayMs(seq) after its scheduled time
func sinkFor(r *plan.Resolved, sum *report.Summary, delayMs func(seq int64) float64) []SinkObs {
	var out []SinkObs
	for i, st := range r.Steps {
		for k := int64(0); k < st.Ticks; k++ {
			seq := st.SeqOffset + k
			sched := sum.Steps[i].ScenarioStartMs + float64(k)*1000/st.TickRate
			out = append(out, SinkObs{RunID: r.RunID, Seq: seq, RecvMs: sched + delayMs(seq)})
		}
	}
	return out
}

func TestPassWithConstantDelay(t *testing.T) {
	r, s, h := setup(2)
	res := Evaluate(r, s, h, sinkFor(r, s, func(int64) float64 { return 200 }), nil)
	if res.Steps[0].Verdict != "warm-up" || res.Steps[1].Verdict != "PASS" || res.Steps[2].Verdict != "PASS" {
		t.Fatalf("%+v", res.Steps)
	}
	// 200 ms in flight at 10/s: exactly 2 outstanding at every instant (no bias from schedule-aligned sampling)
	if o := float64(res.Steps[1].OutstandingM1); math.Abs(o-2) > 1e-9 || math.Abs(float64(res.Steps[1].OutstandingM3)-o) > 1e-9 {
		t.Fatalf("outstanding %v / %v", res.Steps[1].OutstandingM1, res.Steps[1].OutstandingM3)
	}
	if res.HighestPassRate != 10 || res.SinkUnique != int(r.TotalTicks) {
		t.Fatalf("%+v", res)
	}
}

func TestOutstandingGrowthFails(t *testing.T) {
	r, s, h := setup(2)
	// sink delay grows 20 ms per second of step 1: about 13 outstanding at the end of minute 1, about 37 at the end of
	// minute 3, a growth of ~24, well above rate x 1 s = 10
	off := r.Steps[1].SeqOffset
	res := Evaluate(r, s, h, sinkFor(r, s, func(seq int64) float64 {
		if seq < off || seq >= off+r.Steps[1].Ticks {
			return 100
		}
		return 100 + float64(seq-off)*2 // +2 ms per message = +20 ms per second
	}), nil)
	st := res.Steps[1]
	if st.Verdict != "FAIL" || !strings.Contains(strings.Join(st.Reasons, ";"), "outstanding work grows") {
		t.Fatalf("%+v", st)
	}
	if res.Steps[2].Verdict != "not evaluated" || res.FirstFailRate != 10 {
		t.Fatalf("stop rule: %+v", res.Steps[2])
	}
}

func TestOutstandingGrowthWithinSlackPasses(t *testing.T) {
	r, s, h := setup(1)
	off := r.Steps[1].SeqOffset
	// +500 ms more delay in minute 3 than minute 1: ~5 more outstanding, within rate x 1 s = 10
	res := Evaluate(r, s, h, sinkFor(r, s, func(seq int64) float64 {
		if seq >= off+1200 && seq < off+1800 {
			return 600
		}
		return 100
	}), nil)
	if res.Steps[1].Verdict != "PASS" {
		t.Fatalf("%+v", res.Steps[1])
	}
}

func TestLatencyTrendBorders(t *testing.T) {
	cases := []struct {
		p1, p3 float64
		pass   bool
	}{
		{100, 150, true}, {100, 150.1, false}, // 1.5 x branch
		{20, 70, true}, {20, 70.1, false}, // +50 ms branch
		{600, 900, true}, {700, 1000.5, false}, // trend ok but minute p99 > 1 s
	}
	for _, c := range cases {
		r, s, h := setup(1)
		h.Steps[1].AckByMinute[0].P99 = report.F(c.p1)
		h.Steps[1].AckByMinute[1].P99 = report.F(c.p1)
		h.Steps[1].AckByMinute[2].P99 = report.F(c.p3)
		res := Evaluate(r, s, h, sinkFor(r, s, func(int64) float64 { return 50 }), nil)
		if (res.Steps[1].Verdict == "PASS") != c.pass {
			t.Fatalf("p1 %v p3 %v: %s %v", c.p1, c.p3, res.Steps[1].Verdict, res.Steps[1].Reasons)
		}
	}
}

func TestUnsuccessfulRatio(t *testing.T) {
	for _, c := range []struct {
		bad  float64
		pass bool
	}{{1.8, true}, {1.9, false}} { // 1800 attempted: 0.1% = 1.8
		r, s, h := setup(1)
		s.Steps[1].Unknown = c.bad
		res := Evaluate(r, s, h, sinkFor(r, s, func(int64) float64 { return 50 }), nil)
		if (res.Steps[1].Verdict == "PASS") != c.pass {
			t.Fatalf("bad %v: %s %v", c.bad, res.Steps[1].Verdict, res.Steps[1].Reasons)
		}
	}
}

func TestGeneratorFailureIsInvalid(t *testing.T) {
	r, s, h := setup(2)
	h.Steps[1].Checks = append(h.Steps[1].Checks, report.Check{Name: "send_lag_p99", Pass: false, Gate: true, Detail: "p99 25 ms"})
	res := Evaluate(r, s, h, sinkFor(r, s, func(int64) float64 { return 50 }), nil)
	if res.Steps[1].Verdict != "INVALID" || res.Steps[2].Verdict != "not evaluated" || res.FirstFailVerdict != "INVALID" {
		t.Fatalf("%+v", res.Steps)
	}
}

func TestCPUBounds(t *testing.T) {
	r, s, h := setup(1)
	ts := (t0 + 60000) / 1000 // step 1 start (s)
	snaps := []Snapshot{
		{TimeS: ts, Container: "app", ID: "a", UsageUsec: 0, StartedAt: "x", Cpuset: "4-7"},
		{TimeS: ts, Container: "db", ID: "d", UsageUsec: 0, StartedAt: "x", Cpuset: "2-3"},
		{TimeS: ts, Container: "lb", ID: "l", UsageUsec: 0, StartedAt: "x", Cpuset: "1"},
		{TimeS: ts + 180, Container: "app", ID: "a", UsageUsec: 90e6, StartedAt: "x", Cpuset: "4-7"}, // 90 CPU-s / 1800 msgs
		{TimeS: ts + 180, Container: "db", ID: "d", UsageUsec: 180e6, StartedAt: "x", Cpuset: "2-3"}, // 180 CPU-s
		{TimeS: ts + 180, Container: "lb", ID: "l2", UsageUsec: 5e6, StartedAt: "y", Cpuset: "1"},    // restarted
	}
	res := Evaluate(r, s, h, sinkFor(r, s, func(int64) float64 { return 50 }), snaps)
	st := res.Steps[1]
	// app: D = 0.05, B = 4/0.05 = 80; db: D = 0.1, B = 2/0.1 = 20 -> bottleneck db
	if st.MinBContainer != "db" || math.Abs(st.MinB-20) > 1e-9 {
		t.Fatalf("bound %v %s: %+v", st.MinB, st.MinBContainer, st.CPU)
	}
	for _, c := range st.CPU {
		if c.Container == "lb" && c.Complete {
			t.Fatal("restarted container must be incomplete")
		}
	}
}

// a minute without responses has a NaN p99; the result must still encode (EXPL-fiware-k4 wrote an empty explore.json)
func TestNaNEncodes(t *testing.T) {
	r, s, h := setup(1)
	h.Steps[1].AckByMinute[2].P99 = report.F(math.NaN())
	res := Evaluate(r, s, h, sinkFor(r, s, func(int64) float64 { return 50 }), nil)
	if res.Steps[1].Verdict != "FAIL" {
		t.Fatalf("%+v", res.Steps[1])
	}
	if _, err := json.Marshal(res); err != nil {
		t.Fatal(err)
	}
}
