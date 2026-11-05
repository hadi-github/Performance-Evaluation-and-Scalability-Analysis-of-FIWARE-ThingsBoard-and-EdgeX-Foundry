package analysis

import "fmt"

var metricUnits = map[string]string{
	"workload.devices": "devices", "workload.offered_rate": "messages/s", "workload.measurement_seconds": "s",
	"workload.observed_history_devices": "devices",
	"outcomes.scheduled":                "messages", "outcomes.started": "messages", "outcomes.acknowledged": "messages",
	"outcomes.explicit_failure": "messages", "outcomes.transport_unknown": "messages", "outcomes.dropped_iterations": "messages", "outcomes.early": "messages",
	"outcomes.achieved_rate": "messages/s", "outcomes.unsuccessful_ratio": "ratio",
	"lag.p50_ms": "ms", "lag.p99_ms": "ms", "lag.max_ms": "ms",
	"latency.ack.p50_ms": "ms", "latency.ack.p99_ms": "ms", "latency.delivery.p50_ms": "ms", "latency.delivery.p99_ms": "ms",
	"latency.http.p99_ms": "ms", "latency.durable.p99_ms": "ms",
	"latency.http.p50_ms":       "ms",
	"sink.missing_acknowledged": "messages", "history.missing_acknowledged": "messages",
	"sink.duplicate_extra_copies": "messages", "history.duplicate_extra_copies": "messages",
	"sink.unexpected_identities": "messages", "history.unexpected_identities": "messages",
	"backlog.non_growing": "indicator", "backlog.observed_max_records": "records", "backlog.endpoint_change_records": "records",
	"cpu.phase_seconds": "CPU-s", "cpu.seconds_per_message": "CPU-s/message",
}

func validatePolicy(p Policy) error {
	if b := p.BacklogRule; b != nil && b.Method != "ols-iid-10s-one-sided95-v1" {
		return fmt.Errorf("unsupported backlog confidence interpretation %q", b.Method)
	}
	seen := map[string]bool{}
	for _, profile := range p.Profiles {
		if profile.ID == "" || seen[profile.ID] {
			return fmt.Errorf("profile IDs must be nonempty and unique")
		}
		seen[profile.ID] = true
		if d := profile.DeviceComparison; d != nil {
			if profile.ID != "high_device_count" || len(d.Populations) != 2 || d.Populations[0] != 100 || d.Populations[1] != 10000 || len(d.ReferenceFractions) != 2 || d.ReferenceFractions[0] != .5 || d.ReferenceFractions[1] != .75 || d.ApplicationCPU != 1 || d.ReplicasPerService != 1 {
				return fmt.Errorf("unsupported W2 design: expected 100 vs 10000, fractions 0.5/0.75, N=1")
			}
			if len(profile.Requirements) > 0 {
				return fmt.Errorf("W2 is a comparison, not a device-count acceptance cutoff")
			}
		}
		for _, q := range append(append([]Requirement{}, profile.Requirements...), profile.Sensitivities...) {
			unit, ok := metricUnits[q.Metric]
			if !ok {
				return fmt.Errorf("unsupported profile metric %q", q.Metric)
			}
			if q.Unit != unit {
				return fmt.Errorf("metric %s requires unit %s, got %s", q.Metric, unit, q.Unit)
			}
			if q.Operator != "<=" && q.Operator != ">=" && q.Operator != "==" {
				return fmt.Errorf("unsupported comparison operator %q", q.Operator)
			}
			if q.Threshold != nil && (!finite(*q.Threshold) || *q.Threshold < 0) {
				return fmt.Errorf("negative or non-finite threshold for %s", q.Metric)
			}
		}
	}
	return nil
}

func serviceClaim(a Assessment) Claim {
	required := map[string]bool{"latency.delivery.p99_ms": false, "outcomes.unsuccessful_ratio": false, "sink.missing_acknowledged": false, "history.missing_acknowledged": false, "sink.duplicate_extra_copies": false, "history.duplicate_extra_copies": false, "backlog.non_growing": false}
	for _, q := range a.Profile.Requirements {
		if _, ok := required[q.Metric]; ok {
			required[q.Metric] = true
		}
	}
	for _, name := range sortedKeys(required) {
		if !required[name] {
			return claim(NotAssessed, "service-SLO requirement not configured: "+name)
		}
	}
	return a.Claim
}

func assess(profile Profile, s *Step) Assessment {
	a := Assessment{Profile: profile, Claim: claim(Pass, "all explicitly configured requirements met; not campaign readiness")}
	for _, q := range profile.Sensitivities {
		check := assess(Profile{Source: profile.Source, Requirements: []Requirement{q}}, s)
		a.SensitivityChecks = append(a.SensitivityChecks, check.Checks...)
	}
	if profile.DeviceComparison != nil {
		a.Claim = claim(NotAssessed, "W2 requires both 100/10000-device populations at matched 50%/75% reference rates; run compare; no device-capacity cutoff")
		if c, ok := s.Claims["w2_reference"]; ok && c.Status != Pass {
			a.Claim = c
		}
		return a
	}
	if len(profile.Requirements) == 0 || profile.Source == "" {
		a.Claim = claim(NotAssessed, "profile requirements or their source are missing; no thresholds invented")
	}
	for _, q := range profile.Requirements {
		c := claim(NotAssessed, "required measurement/endpoint unavailable")
		if q.Threshold == nil || q.Source == "" {
			c = claim(NotAssessed, "threshold or threshold source missing")
		} else if m, ok := s.Metrics[q.Metric]; ok && m.Value != nil {
			dep := s.Claims[m.Claim]
			switch dep.Status {
			case Invalid:
				c = claim(Invalid, "measurement prerequisite invalid: "+m.Claim)
			case Pass, Fail:
				v, t := *m.Value, *q.Threshold
				pass := q.Operator == "<=" && v <= t || q.Operator == ">=" && v >= t || q.Operator == "==" && v == t
				if pass {
					c = claim(Pass, fmt.Sprintf("%s = %g %s %s %g", q.Metric, v, m.Unit, q.Operator, t))
				} else {
					c = claim(Fail, fmt.Sprintf("%s = %g %s violates %s %g", q.Metric, v, m.Unit, q.Operator, t))
				}
			default:
				c = claim(NotAssessed, "measurement prerequisite unavailable: "+m.Claim)
			}
		}
		a.Checks = append(a.Checks, Check{q, c})
	}
	if s.Claims["offered_load"].Status != Pass {
		a.Claim = dependent(s.Claims["offered_load"])
		return a
	}
	// A known violation falsifies a conjunction even if another requirement is
	// unavailable; an instrumentation error is never relabelled a platform FAIL.
	for _, status := range []Status{Invalid, Fail, NotAssessed} {
		var reasons []string
		for _, c := range a.Checks {
			if c.Status == status {
				reasons = append(reasons, c.Reasons...)
			}
		}
		if len(reasons) > 0 {
			a.Claim = Claim{status, reasons}
			return a
		}
	}
	return a
}
