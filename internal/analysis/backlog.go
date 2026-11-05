package analysis

import (
	"math"
	"sort"

	"iotbench/internal/audit"
)

// Optional explicit interpretation of the archived OLS procedure. IID-bin
// uncertainty is never selected implicitly: neither binning nor a flat queue
// establishes independence, and no calibration epsilon is invented.
func evaluateGrowth(s *Step, rd reader, ev Evidence, rule *BacklogRule, planned []audit.Planned, outcome map[int64]string, first map[int64]float64, clockOK bool) {
	if rule == nil {
		return
	}
	if !rule.IndependentBins || rule.Source == "" {
		s.Claims["backlog"] = claim(NotAssessed, "OLS independent-bin confidence assumption and procedure source must be explicit")
		return
	}
	if !clockOK || (s.Claims["sink"].Status != Pass && s.Claims["sink"].Status != Fail) {
		s.Claims["backlog"] = claim(NotAssessed, "pending-identity backlog requires a complete valid sink capture and comparable scheduled/receipt clocks")
		return
	}
	b, e := rd.read("backlog_calibration", rule.Calibration)
	if e != nil {
		s.Claims["backlog"] = claim(NotAssessed, "calibrated epsilon evidence unavailable")
		return
	}
	var cal struct {
		SchemaVersion int  `json:"schema_version"`
		Synthetic     bool `json:"synthetic"`
		Runs          []struct {
			RunID     string  `json:"run_id"`
			Platform  string  `json:"platform"`
			Valid     bool    `json:"valid"`
			Rate      float64 `json:"rate"`
			Reference float64 `json:"reference_capacity"`
			DelayMS   float64 `json:"delay_growth_ms"`
			SHA       string  `json:"source_sha256"`
		} `json:"runs"`
	}
	if decode(b, &cal, true) != nil || cal.SchemaVersion != 1 || cal.Synthetic != ev.Synthetic {
		s.Claims["backlog"] = claim(Invalid, "calibration schema or synthetic/real provenance mismatch")
		return
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	seenHashes := map[string]bool{}
	epsilon := 0.0
	for _, r := range cal.Runs {
		if !r.Valid || r.RunID == "" || seen[r.RunID] || seenHashes[r.SHA] || len(r.SHA) != 64 || !finite(r.DelayMS) || !finite(r.Rate) || !finite(r.Reference) || r.Rate <= 0 || r.Reference <= 0 || r.Rate > r.Reference*.25 {
			s.Claims["backlog"] = claim(Invalid, "calibration requires distinct valid stable-rate run attestations at <=25% reference capacity with source hashes")
			return
		}
		if r.Platform != "thingsboard" && r.Platform != "fiware" && r.Platform != "edgex" {
			s.Claims["backlog"] = claim(Invalid, "unknown calibration platform")
			return
		}
		counts[r.Platform]++
		seen[r.RunID] = true
		seenHashes[r.SHA] = true
		epsilon = math.Max(epsilon, math.Abs(r.DelayMS))
	}
	for _, p := range []string{"thingsboard", "fiware", "edgex"} {
		if counts[p] < 5 {
			s.Claims["backlog"] = claim(NotAssessed, "calibration needs at least five independent-run attestations per platform")
			return
		}
	}
	if math.Mod(s.EndMS-s.StartMS, 10000) != 0 || s.EndMS-s.StartMS < 30000 {
		s.Claims["backlog"] = claim(NotAssessed, "OLS procedure requires >=3 complete 10-second bins")
		return
	}
	y := pendingBins(s, planned, outcome, first)
	n := float64(len(y))
	xbar := (n - 1) * 5
	ybar := 0.0
	for _, v := range y {
		ybar += v / n
	}
	sxx, sxy := 0.0, 0.0
	for i, v := range y {
		x := float64(i)*10 - xbar
		sxx += x * x
		sxy += x * (v - ybar)
	}
	slope := sxy / sxx
	residual := 0.0
	for i, v := range y {
		fitted := ybar + slope*(float64(i)*10-xbar)
		residual += (v - fitted) * (v - fitted)
	}
	se := math.Sqrt(residual / (n - 2) / sxx)
	lower := slope - studentCritical95(n-2)*se
	delayMS := slope / s.OfferedRate * (s.EndMS - s.StartMS)
	growing := lower > 0 && delayMS > epsilon
	s.Claims["backlog"] = claim(Pass, "explicit OLS IID-10s-bin interpretation and supplied calibration; confidence conclusion is conditional on the declared independence assumption")
	put(s, "backlog.slope_messages_per_second", slope, "messages/s", "backlog", "OLS of 10-second means of acknowledged pending identities, not broker queue depth")
	put(s, "backlog.slope_one_sided95_lower", lower, "messages/s", "backlog", "Student-t OLS residual interval, conditional on explicitly selected IID-bin assumption")
	put(s, "backlog.delay_growth_ms", delayMS, "ms", "backlog", "beta / offered rate * measurement duration; 600-second protocol window gives archived dW")
	put(s, "backlog.calibrated_epsilon_ms", epsilon, "ms", "backlog", "maximum absolute supplied stable-run dW; calibration-file hash is in input inventory")
	v := 1.0
	if growing {
		v = 0
	}
	put(s, "backlog.non_growing", v, "indicator", "backlog", "1 iff the two-part growth criterion is not met; not proof of delivery/storage correctness")
}

func pendingBins(s *Step, planned []audit.Planned, outcome map[int64]string, first map[int64]float64) []float64 {
	type event struct {
		t     float64
		delta int
	}
	var events []event
	for _, p := range planned {
		if p.Step != s.Name {
			continue
		}
		if o := outcome[p.Seq]; o != "" && o != "ack_success" {
			continue
		}
		end := s.EndMS
		if t, ok := first[p.Seq]; ok && t < end {
			end = t
		}
		if end <= p.SchedMs {
			continue
		}
		events = append(events, event{p.SchedMs, 1}, event{end, -1})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].t < events[j].t })
	bins := make([]float64, int((s.EndMS-s.StartMS)/10000))
	pos := s.StartMS
	index, pending := 0, 0
	for i := range bins {
		end := s.StartMS + float64(i+1)*10000
		area := 0.0
		for index < len(events) && events[index].t < end {
			e := events[index]
			area += float64(pending) * (e.t - pos)
			pending += e.delta
			pos = e.t
			index++
		}
		area += float64(pending) * (end - pos)
		bins[i] = area / 10000
		pos = end
	}
	return bins
}

// Student-t inverse CDF by bisection of the regularized incomplete beta CDF.
// No normal approximation is substituted for the small-sample distribution.
func studentCritical95(df float64) float64 {
	lo, hi := 0.0, 1000.0
	for i := 0; i < 80; i++ {
		mid := (lo + hi) / 2
		cdf := 1 - .5*regularBeta(df/(df+mid*mid), df/2, .5)
		if cdf < .95 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func regularBeta(x, a, b float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	lg, _ := math.Lgamma(a + b)
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	bt := math.Exp(lg - la - lb + a*math.Log(x) + b*math.Log1p(-x))
	if x < (a+1)/(a+b+2) {
		return bt * betaFraction(a, b, x) / a
	}
	return 1 - bt*betaFraction(b, a, 1-x)/b
}

func betaFraction(a, b, x float64) float64 {
	const tiny = 1e-300
	c := 1.0
	d := 1 - (a+b)*x/(a+1)
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m <= 300; m++ {
		mf := float64(m)
		m2 := 2 * mf
		for j, aa := range []float64{mf * (b - mf) * x / ((a + m2 - 1) * (a + m2)), -(a + mf) * (a + b + mf) * x / ((a + m2) * (a + m2 + 1))} {
			d = 1 + aa*d
			if math.Abs(d) < tiny {
				d = tiny
			}
			c = 1 + aa/c
			if math.Abs(c) < tiny {
				c = tiny
			}
			d = 1 / d
			del := d * c
			h *= del
			if j == 1 && math.Abs(del-1) < 3e-14 {
				return h
			}
		}
	}
	return h
}
