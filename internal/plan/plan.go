// Package plan defines the workload plan (JSON, strict) and resolves it into the explicit step list that the k6
// generator executes. The resolved plan is the single source of truth for seq numbering: step s owns
// seq_offset .. seq_offset+ticks-1, where ticks = rate x duration exactly (k6 itself may run one extra boundary
// iteration per step; the generator does not send it).
package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
)

const SchemaVersion = 1

type Plan struct {
	SchemaVersion int        `json:"schema_version"`
	PlanID        string     `json:"plan_id"`
	Purpose       string     `json:"purpose"`
	PilotOnly     bool       `json:"pilot_only"`
	Seed          uint32     `json:"seed"`
	Adapter       string     `json:"adapter"` // fixture | thingsboard | fiware | edgex
	Target        Target     `json:"target"`
	Devices       int        `json:"devices"`
	RequestTimeMs int        `json:"request_timeout_ms"`
	Connection    Connection `json:"connection"`
	Arrivals      Arrivals   `json:"arrivals"`
	VUs           VUs        `json:"vus"`
	Schedule      Schedule   `json:"schedule"`
	Health        Health     `json:"health"`
	HistogramMs   []float64  `json:"histogram_ms,omitempty"` // obsolete (latency is now exact k6 Trends); accepted and ignored
	Cleanup       bool       `json:"cleanup"`
	TestFaults    TestFaults `json:"test_faults"`
	Deviations    []string   `json:"deviations,omitempty"` // procedure deviations, copied into the run manifest
}

// Target: base URL(s) of the system under test. Credentials never live here; adapters read them from env.
type Target struct {
	BaseURL     string `json:"base_url"`               // fixture/reference, thingsboard (:8080)
	NorthURL    string `json:"north_url,omitempty"`    // fiware IoT Agent provisioning (:4041)
	SouthURL    string `json:"south_url,omitempty"`    // fiware UltraLight HTTP (:7896)
	MetadataURL string `json:"metadata_url,omitempty"` // edgex core-metadata (:59881)
	IngressURL  string `json:"ingress_url,omitempty"`  // edgex device-rest (:59986)
	// ExpectBatch207: fixture only; parse 207 batch bodies like the EdgeX adapter (classification tests).
	ExpectBatch207 bool `json:"expect_batch_207,omitempty"`
}

type Connection struct {
	KeepAlive bool `json:"keep_alive"` // k6: noConnectionReuse = !keep_alive; one connection per VU
}

type Arrivals struct {
	Model      string  `json:"model"`       // constant | thinning
	TickFactor float64 `json:"tick_factor"` // thinning: ticks run at tick_factor x rate, each sent with p = 1/tick_factor
}

type VUs struct {
	PreAllocated int `json:"pre_allocated"`
	Max          int `json:"max"`
}

type Schedule struct {
	Type string `json:"type"` // steps | steady | sweep | refine | overload

	Steps []StepSpec `json:"steps,omitempty"` // type steps

	WarmupS   int     `json:"warmup_s,omitempty"`   // steady, sweep, refine, overload: optional warm-up at the first rate
	Rate      float64 `json:"rate,omitempty"`       // steady
	DurationS int     `json:"duration_s,omitempty"` // steady

	StartRate float64 `json:"start_rate,omitempty"` // sweep
	Factor    float64 `json:"factor,omitempty"`     // sweep (e.g. 1.5)
	NSteps    int     `json:"n_steps,omitempty"`    // sweep, refine
	StepS     int     `json:"step_s,omitempty"`     // sweep, refine: duration of each step

	Lo float64 `json:"lo,omitempty"` // refine: last passing rate
	Hi float64 `json:"hi,omitempty"` // refine: first failing rate

	Capacity float64     `json:"capacity,omitempty"` // overload: baseline capacity (req/s)
	Phases   []PhaseSpec `json:"phases,omitempty"`   // overload
}

type StepSpec struct {
	Phase     string  `json:"phase"`
	Rate      float64 `json:"rate"`
	DurationS int     `json:"duration_s"`
	VUs       int     `json:"vus,omitempty"` // per-step VU pool (pre-allocated = max); default: plan vus
}

type PhaseSpec struct {
	Phase     string  `json:"phase"`
	Fraction  float64 `json:"fraction"`
	DurationS int     `json:"duration_s"`
}

type Health struct {
	MinAchievedRatio float64 `json:"min_achieved_ratio"` // per step, attempted / planned sends (>= 0.99)
	SendLagP99Ms     float64 `json:"send_lag_p99_ms"`    // per step, p99 send lag limit (10)
	EarlyToleranceMs float64 `json:"early_tolerance_ms"` // a send this much before its scheduled time is a violation
	MaxCoreBusy      float64 `json:"max_core_busy"`      // per generator core, 0 = report only
}

// TestFaults inject generator-side faults for the automated tests. Never set in experiment plans.
type TestFaults struct {
	DuplicateSeqs []int64 `json:"duplicate_seqs,omitempty"` // send these seqs twice (downstream duplicate detection)
	EarlyEvery    int64   `json:"early_every,omitempty"`    // shift every n-th seq's scheduled time 50 ms later (early-send check)
}

// Step is one resolved, explicit step.
type Step struct {
	Index     int     `json:"index"`
	Name      string  `json:"name"` // s03-sweep-r338: k6 scenario name
	Phase     string  `json:"phase"`
	Rate      float64 `json:"rate"`      // offered messages/s
	TickRate  float64 `json:"tick_rate"` // executor rate (= rate for constant arrivals)
	DurationS int     `json:"duration_s"`
	StartS    int     `json:"start_s"`    // offset from the test start
	Ticks     int64   `json:"ticks"`      // = tick_rate x duration_s
	SeqOffset int64   `json:"seq_offset"` // first seq of this step
	VUs       int     `json:"vus,omitempty"`
}

type Resolved struct {
	Plan
	RunID      string  `json:"run_id"`
	Steps      []Step  `json:"steps"`
	TotalTicks int64   `json:"total_ticks"`
	DurationS  int     `json:"total_duration_s"`
	SendProb   float64 `json:"send_probability"` // 1 for constant arrivals
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// runIDRe also allows upper case (exploratory runs are named EXPL-...).
var runIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// Load reads a plan with strict decoding: unknown fields and trailing data are errors.
func Load(path string) (*Plan, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(b)
}

func Decode(b []byte) (*Plan, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p Plan
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}
	if dec.More() {
		return nil, errors.New("plan: trailing data after the JSON object")
	}
	return &p, nil
}

func (p *Plan) validate() error {
	var errs []string
	bad := func(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }
	if p.SchemaVersion != SchemaVersion {
		bad("schema_version must be %d", SchemaVersion)
	}
	if !nameRe.MatchString(p.PlanID) {
		bad("plan_id must match %s", nameRe)
	}
	switch p.Adapter {
	case "fixture", "thingsboard":
		if p.Target.BaseURL == "" {
			bad("target.base_url is required for adapter %s", p.Adapter)
		}
	case "fiware":
		if p.Target.NorthURL == "" || p.Target.SouthURL == "" || p.Target.BaseURL == "" {
			bad("fiware needs target.base_url (Orion), north_url and south_url")
		}
	case "edgex":
		if p.Target.MetadataURL == "" || p.Target.IngressURL == "" || p.Target.BaseURL == "" {
			bad("edgex needs target.base_url (core-data), metadata_url and ingress_url")
		}
	default:
		bad("adapter must be fixture, thingsboard, fiware or edgex")
	}
	if p.Devices < 1 {
		bad("devices must be >= 1")
	}
	if p.RequestTimeMs < 1 {
		bad("request_timeout_ms must be set explicitly (k6's default is never used)")
	}
	switch p.Arrivals.Model {
	case "constant":
		if p.Arrivals.TickFactor != 0 {
			bad("arrivals.tick_factor is only valid for model thinning")
		}
	case "thinning":
		if p.Arrivals.TickFactor <= 1 {
			bad("arrivals.tick_factor must be > 1 for thinning")
		}
	default:
		bad("arrivals.model must be constant or thinning")
	}
	if p.VUs.PreAllocated < 1 || p.VUs.Max < p.VUs.PreAllocated {
		bad("vus: need 1 <= pre_allocated <= max")
	}
	h := p.Health
	if h.MinAchievedRatio <= 0 || h.MinAchievedRatio > 1 || h.SendLagP99Ms <= 0 || h.EarlyToleranceMs < 0 {
		bad("health: min_achieved_ratio in (0,1], send_lag_p99_ms > 0 and early_tolerance_ms >= 0 are required")
	}
	if len(errs) > 0 {
		return errors.New("plan invalid:\n  " + strings.Join(errs, "\n  "))
	}
	return nil
}

// Resolve validates the plan and expands its schedule into explicit steps.
func (p *Plan) Resolve(runID string) (*Resolved, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if !runIDRe.MatchString(runID) {
		return nil, fmt.Errorf("run_id must match %s", runIDRe)
	}
	specs, err := p.Schedule.expand()
	if err != nil {
		return nil, err
	}
	r := &Resolved{Plan: *p, RunID: runID, SendProb: 1}
	factor := 1.0
	if p.Arrivals.Model == "thinning" {
		factor = p.Arrivals.TickFactor
		r.SendProb = 1 / factor
	}
	var seq int64
	start := 0
	for i, s := range specs {
		if s.Rate <= 0 || s.DurationS <= 0 {
			return nil, fmt.Errorf("step %d: rate and duration must be > 0", i)
		}
		tick := s.Rate * factor
		ticks := tick * float64(s.DurationS)
		if math.Abs(ticks-math.Round(ticks)) > 1e-9 {
			return nil, fmt.Errorf("step %d: tick_rate x duration = %.4f is not an integer number of slots", i, ticks)
		}
		st := Step{
			Index: i, Phase: s.Phase, Rate: s.Rate, TickRate: tick, DurationS: s.DurationS,
			StartS: start, Ticks: int64(math.Round(ticks)), SeqOffset: seq, VUs: s.VUs,
		}
		if s.VUs < 0 {
			return nil, fmt.Errorf("step %d: vus must be >= 0", i)
		}
		st.Name = fmt.Sprintf("s%02d-%s-r%s", i, s.Phase, trimFloat(s.Rate))
		if !nameRe.MatchString(st.Name) {
			return nil, fmt.Errorf("step %d: phase %q gives an invalid scenario name", i, s.Phase)
		}
		r.Steps = append(r.Steps, st)
		seq += st.Ticks
		start += s.DurationS
	}
	r.TotalTicks = seq
	r.DurationS = start
	return r, nil
}

func trimFloat(f float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", f), "0"), ".")
	return strings.ReplaceAll(s, ".", "p")
}

func (s Schedule) expand() ([]StepSpec, error) {
	var out []StepSpec
	warm := func(rate float64) {
		if s.WarmupS > 0 {
			out = append(out, StepSpec{Phase: "warmup", Rate: rate, DurationS: s.WarmupS})
		}
	}
	switch s.Type {
	case "steps":
		if len(s.Steps) == 0 {
			return nil, errors.New("schedule.steps is empty")
		}
		out = append(out, s.Steps...)
	case "steady":
		if s.Rate <= 0 || s.DurationS <= 0 {
			return nil, errors.New("steady needs rate and duration_s")
		}
		warm(s.Rate)
		out = append(out, StepSpec{Phase: "steady", Rate: s.Rate, DurationS: s.DurationS})
	case "sweep":
		if s.StartRate <= 0 || s.Factor <= 1 || s.NSteps < 1 || s.StepS <= 0 {
			return nil, errors.New("sweep needs start_rate > 0, factor > 1, n_steps >= 1, step_s > 0")
		}
		warm(s.StartRate)
		r := s.StartRate
		for i := 0; i < s.NSteps; i++ {
			out = append(out, StepSpec{Phase: "sweep", Rate: math.Round(r), DurationS: s.StepS})
			r *= s.Factor
		}
	case "refine":
		if s.Lo <= 0 || s.Hi <= s.Lo || s.NSteps < 1 || s.StepS <= 0 {
			return nil, errors.New("refine needs 0 < lo < hi, n_steps >= 1, step_s > 0")
		}
		warm(s.Lo)
		for i := 1; i <= s.NSteps; i++ { // strictly between lo and hi
			r := s.Lo + (s.Hi-s.Lo)*float64(i)/float64(s.NSteps+1)
			out = append(out, StepSpec{Phase: "refine", Rate: math.Round(r), DurationS: s.StepS})
		}
	case "overload":
		if s.Capacity <= 0 || len(s.Phases) == 0 {
			return nil, errors.New("overload needs capacity and phases")
		}
		warm(math.Round(s.Capacity * s.Phases[0].Fraction))
		for _, ph := range s.Phases {
			if ph.Phase == "" {
				ph.Phase = fmt.Sprintf("load%d", int(math.Round(ph.Fraction*100)))
			}
			out = append(out, StepSpec{Phase: ph.Phase, Rate: math.Round(s.Capacity * ph.Fraction), DurationS: ph.DurationS})
		}
	default:
		return nil, fmt.Errorf("schedule.type %q: must be steps, steady, sweep, refine or overload", s.Type)
	}
	return out, nil
}
