package plan

import (
	"strings"
	"testing"
)

const base = `{"schema_version":1,"plan_id":"t","purpose":"x","seed":1,"adapter":"fixture",
"target":{"base_url":"http://127.0.0.1:1"},"devices":10,"request_timeout_ms":1000,"connection":{"keep_alive":true},
"arrivals":{"model":"constant"},"vus":{"pre_allocated":5,"max":5},%SCHED%,
"health":{"min_achieved_ratio":0.99,"send_lag_p99_ms":10,"early_tolerance_ms":2},
"histogram_ms":[0,1,5,10,100],"cleanup":false,"test_faults":{}}`

func load(t *testing.T, sched string) (*Plan, error) {
	t.Helper()
	return Decode([]byte(strings.Replace(base, "%SCHED%", sched, 1)))
}

func TestStrictDecoding(t *testing.T) {
	if _, err := Decode([]byte(strings.Replace(base, "%SCHED%", `"schedule":{"type":"steady","rate":1,"duration_s":1},"bogus":1`, 1))); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := Decode([]byte(strings.Replace(base, "%SCHED%", `"schedule":{"type":"steady","rate":1,"duration_s":1}`, 1) + "{}")); err == nil {
		t.Fatal("trailing data accepted")
	}
}

func TestStepsAndSeqOffsets(t *testing.T) {
	p, err := load(t, `"schedule":{"type":"steps","steps":[{"phase":"warmup","rate":50,"duration_s":4},{"phase":"a","rate":7,"duration_s":3}]}`)
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Resolve("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Steps) != 2 || r.Steps[0].Ticks != 200 || r.Steps[1].Ticks != 21 || r.Steps[1].SeqOffset != 200 ||
		r.Steps[1].StartS != 4 || r.TotalTicks != 221 || r.DurationS != 7 {
		t.Fatalf("unexpected resolution: %+v", r.Steps)
	}
}

func TestSweepRefineOverload(t *testing.T) {
	cases := map[string][]float64{
		`"schedule":{"type":"sweep","start_rate":100,"factor":1.5,"n_steps":4,"step_s":2,"warmup_s":2}`:                             {100, 100, 150, 225, 338},
		`"schedule":{"type":"refine","lo":300,"hi":400,"n_steps":3,"step_s":2}`:                                                     {325, 350, 375},
		`"schedule":{"type":"overload","capacity":1000,"phases":[{"fraction":0.5,"duration_s":2},{"fraction":1.2,"duration_s":2}]}`: {500, 1200},
	}
	for sched, want := range cases {
		p, err := load(t, sched)
		if err != nil {
			t.Fatal(err)
		}
		r, err := p.Resolve("run-1")
		if err != nil {
			t.Fatalf("%s: %v", sched, err)
		}
		if len(r.Steps) != len(want) {
			t.Fatalf("%s: %d steps, want %d", sched, len(r.Steps), len(want))
		}
		for i, s := range r.Steps {
			if s.Rate != want[i] {
				t.Fatalf("%s: step %d rate %v, want %v", sched, i, s.Rate, want[i])
			}
		}
	}
}

func TestRejects(t *testing.T) {
	for _, sched := range []string{
		`"schedule":{"type":"steady","rate":0.3,"duration_s":1}`, // 0.3 slots: not an integer number of ticks
		`"schedule":{"type":"nope"}`,
		`"schedule":{"type":"refine","lo":400,"hi":300,"n_steps":1,"step_s":1}`,
	} {
		p, err := load(t, sched)
		if err != nil {
			continue
		}
		if _, err := p.Resolve("run-1"); err == nil {
			t.Fatalf("accepted %s", sched)
		}
	}
	p, _ := load(t, `"schedule":{"type":"steady","rate":1,"duration_s":1}`)
	if _, err := p.Resolve("EXPL-edgex-k4"); err != nil {
		t.Fatalf("upper-case run id rejected: %v", err)
	}
	if _, err := p.Resolve("bad id"); err == nil {
		t.Fatal("run id with a space accepted")
	}
}
