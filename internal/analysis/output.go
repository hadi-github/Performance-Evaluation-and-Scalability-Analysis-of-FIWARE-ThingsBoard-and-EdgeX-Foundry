package analysis

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"iotbench/internal/protocol"
)

// Write creates a NEW output directory outside every input run. It never invokes
// runner.Finish/check, modifies manifests, or repairs evidence.
func Write(out string, r *Result, run string) error {
	if err := newDirectory(out, []string{run}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(out, "analysis.json"), r); err != nil {
		return err
	}
	var text strings.Builder
	fmt.Fprintf(&text, "# Run analysis: %s\n\nSynthetic: **%t**. Campaign: **NOT ASSESSED / NO-GO**.\n\n", r.RunID, r.Synthetic)
	fmt.Fprintf(&text, "Platform: `%s`; seed: `%d`; interpretation: `%s`.\n\n", r.Platform, r.Seed, r.Interpretation)
	fmt.Fprintf(&text, "Metric interpretation: `%s`.\n\n", r.MetricInterpretation)
	if r.Protocol != nil {
		fmt.Fprintf(&text, "Protocol provenance: ID `%s`; protocol SHA-256 `%s`; profile `%s` SHA-256 `%s`; execution class `%s`. Eligibility: %s.\n\n", r.Protocol.Binding.ProtocolID, r.Protocol.ProtocolSHA256, r.Protocol.ProfilePath, r.Protocol.ProfileSHA256, r.Protocol.Binding.ExecutionClass, r.Protocol.Eligibility)
	} else {
		fmt.Fprint(&text, "Protocol provenance: **none (legacy interpretation)**.\n\n")
	}
	if r.FaultTarget != nil {
		fmt.Fprintf(&text, "Offline fault target: eligible **%t**, policy `%s`, dry run **%t**.\n\n", r.FaultTarget.Eligible, r.FaultTarget.PolicyVersion, r.FaultTarget.DryRun)
		for _, reason := range r.FaultTarget.RefusalReasons {
			fmt.Fprintf(&text, "- Fault target refused: %s\n", reason)
		}
	}
	if r.FaultEvent != nil {
		fmt.Fprintf(&text, "Manual fault event: status `%s`, qualified **%t**, dry run **%t**.\n\n", r.FaultEvent.Status, r.FaultEvent.Qualified, r.FaultEvent.DryRun)
	}
	if r.FaultCapacity != nil {
		c := r.FaultCapacity
		fmt.Fprintf(&text, "Offline capacity observation: status `%s`; lower %s; upper %s. This is not a campaign capacity claim. Stop reason: %s.\n\n", c.Status, boundText(c.LowerBound), boundText(c.UpperBound), c.StopReason)
	}
	for _, key := range sortedKeys(r.Claims) {
		c := r.Claims[key]
		fmt.Fprintf(&text, "- **%s: %s** — %s\n", key, c.Status, strings.Join(c.Reasons, "; "))
	}
	for _, s := range r.Steps {
		fmt.Fprintf(&text, "\n## %s — %d devices, %g messages/s\n\nWindow: %.3f–%.3f epoch milliseconds (scheduled cohort, end exclusive).\n\n", s.Name, s.Devices, s.OfferedRate, s.StartMS, s.EndMS)
		for _, key := range sortedKeys(s.Claims) {
			c := s.Claims[key]
			fmt.Fprintf(&text, "- **%s: %s** — %s\n", key, c.Status, strings.Join(c.Reasons, "; "))
		}
		fmt.Fprintln(&text, "\n| Metric | Value | Unit | Claim | Basis |\n|---|---:|---|---|---|")
		for _, name := range sortedKeys(s.Metrics) {
			m := s.Metrics[name]
			v := "unavailable"
			if m.Value != nil {
				v = fmt.Sprintf("%g", *m.Value)
			}
			fmt.Fprintf(&text, "| %s | %s | %s | %s | %s |\n", name, v, m.Unit, s.Claims[m.Claim].Status, m.Basis)
		}
		fmt.Fprintln(&text, "\n### Legacy profile assessments (not protocol-cohort verdicts)")
		for _, p := range s.Profiles {
			fmt.Fprintf(&text, "\n- **%s: %s** — %s\n", p.Profile.Name, p.Status, strings.Join(p.Reasons, "; "))
			for _, check := range p.SensitivityChecks {
				threshold := "unavailable"
				if check.Requirement.Threshold != nil {
					threshold = fmt.Sprint(*check.Requirement.Threshold)
				}
				fmt.Fprintf(&text, "  - Sensitivity only (not primary): %s %s %s %s — %s\n", check.Requirement.Metric, check.Requirement.Operator, threshold, check.Requirement.Unit, check.Status)
			}
		}
	}
	fmt.Fprintln(&text, "\n## Limitations")
	for _, l := range r.Limitations {
		fmt.Fprintf(&text, "- %s\n", l)
	}
	fmt.Fprintln(&text, "\n## Input provenance\n\n| Role | File | Availability | SHA-256 |\n|---|---|---|---|")
	for _, in := range r.Inputs {
		fmt.Fprintf(&text, "| %s | %s | %s | %s |\n", in.Role, in.Path, in.Status, in.SHA256)
	}
	if err := os.WriteFile(filepath.Join(out, "report.md"), []byte(text.String()), 0644); err != nil {
		return err
	}
	return checksums(out)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

func canonicalPath(path string) string {
	p := absolute(path)
	if x, e := filepath.EvalSymlinks(p); e == nil {
		return x
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(canonicalPath(parent), filepath.Base(p))
}

func newDirectory(out string, inputs []string) error {
	if out == "" {
		return fmt.Errorf("new output directory required")
	}
	o := canonicalPath(out)
	for _, in := range inputs {
		if in == "" {
			continue
		}
		p := canonicalPath(in)
		if o == p || strings.HasPrefix(o, p+string(os.PathSeparator)) {
			return fmt.Errorf("output must be outside input evidence: %s", in)
		}
	}
	if _, e := os.Lstat(out); !os.IsNotExist(e) {
		return fmt.Errorf("output already exists or cannot be inspected: %s", out)
	}
	if e := os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		return e
	}
	return os.Mkdir(out, 0755)
}

func checksums(dir string) error {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	var text strings.Builder
	for _, f := range entries {
		if f.IsDir() || f.Name() == "SHA256SUMS" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, f.Name()))
		if e != nil {
			return e
		}
		h := sha256.Sum256(b)
		fmt.Fprintf(&text, "%x  %s\n", h, f.Name())
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(text.String()), 0644)
}

type ComparisonRow struct {
	RunID                string            `json:"run_id"`
	MetricInterpretation string            `json:"metric_interpretation"`
	ProtocolBinding      *protocol.Binding `json:"protocol_binding,omitempty"`
	ProfileSHA256        string            `json:"profile_sha256,omitempty"`
	Platform             string            `json:"platform"`
	Step                 int               `json:"step"`
	Devices              int               `json:"devices"`
	Rate                 float64           `json:"offered_rate"`
	Configuration        Configuration     `json:"configuration"`
	Profile              Profile           `json:"profile"`
	ProfileVerdict       Status            `json:"profile_verdict"`
	Claims               map[string]Claim  `json:"claims"`
	Metrics              map[string]Metric `json:"metrics"`
	Eligible             bool              `json:"eligible_for_profile_comparison"`
	Group                string            `json:"group"`
	Reasons              []string          `json:"reasons"`
	SensitivityChecks    []Check           `json:"sensitivity_checks,omitempty"`
}

type Group struct {
	Key                 string    `json:"key"`
	PassingRates        []float64 `json:"tested_passing_rates"`
	FailingRates        []float64 `json:"tested_failing_rates"`
	HighestObservedPass *float64  `json:"highest_observed_passing_rate"`
	HighestAllLowerPass *float64  `json:"highest_rate_with_all_lower_tested_points_passing"`
	DistinctRuns        int       `json:"distinct_run_ids"`
	Capacity            Claim     `json:"campaign_capacity"`
}

type Comparison struct {
	SchemaVersion     int             `json:"schema_version"`
	Rows              []ComparisonRow `json:"rows"`
	Groups            []Group         `json:"groups"`
	Warnings          []string        `json:"warnings"`
	DeviceComparisons []DeviceStudy   `json:"device_comparisons,omitempty"`
}

// Compare does not pool percentiles or promote points into campaign capacity.
// Only the explicit W2 study matches different device counts, in separate cells.
func Compare(results []*Result) Comparison {
	c := Comparison{SchemaVersion: 1, Warnings: []string{"Groups match platform, devices, CPU budget/actual replicas, configuration, images, source, script hash, seed, connection model, interpretation and profile requirements. Offered rate varies only within an otherwise matching group.", "Distinct run IDs are not proof of independent experimental replications. Windows within one run are never counted as independent runs.", "Highest observed passing rate is descriptive, not maximum capacity or a campaign-qualified C_SLO. No statistical significance is claimed."}}
	seen := map[string]bool{}
	for _, r := range results {
		duplicate := seen[r.RunID]
		seen[r.RunID] = true
		if len(r.Steps) == 0 {
			c.Warnings = append(c.Warnings, "report has no evaluable measured steps: "+r.RunID)
		}
		for _, s := range r.Steps {
			for _, p := range s.Profiles {
				configurationKey := r.Configuration
				configurationKey.EffectivePath = ""
				qualifiers := map[string]any{}
				if e := r.CaptureSpecification; e != nil {
					qualifiers["sink_clock"] = e.SinkClock
					qualifiers["observation_after_phase_ms"] = e.ObservationDeadlineMS - s.EndMS
					qualifiers["history_collection_after_phase_ms"] = e.History.CollectedAtMS - s.EndMS
					for _, cpu := range e.CPU {
						if cpu.Step == s.Index {
							qualifiers["cpu_denominator"] = cpu.Denominator
							qualifiers["cpu_membership"] = cpu.ExpectedContainers
							qualifiers["cpu_clock"] = cpu.Clock
						}
					}
				}
				if r.Policy != nil && r.Policy.BacklogRule != nil {
					rule := *r.Policy.BacklogRule
					rule.Calibration = ""
					qualifiers["backlog_rule"] = rule
					for _, in := range r.Inputs {
						if in.Role == "backlog_calibration" {
							qualifiers["backlog_calibration_sha256"] = in.SHA256
						}
					}
				}
				keyBody, _ := json.Marshal(struct {
					Platform             string
					Devices              int
					Synthetic            bool
					Seed                 uint32
					Configuration        Configuration
					Provenance           map[string]json.RawMessage
					Interpretation       string
					MetricInterpretation string
					ProtocolBinding      *protocol.Binding
					ProfileSHA256        string
					Profile              Profile
					Phase                string
					DurationMS           float64
					Qualifiers           map[string]any
				}{r.Platform, s.Devices, r.Synthetic, r.Seed, configurationKey, r.Provenance, r.Interpretation, r.MetricInterpretation, comparisonProtocolBinding(r), func() string {
					if r.Protocol != nil {
						return r.Protocol.ProfileSHA256
					}
					return ""
				}(), p.Profile, s.Phase, s.EndMS - s.StartMS, qualifiers})
				h := sha256.Sum256(keyBody)
				key := hex.EncodeToString(h[:])
				eligible := r.Claims["comparability"].Status == Pass && s.Claims["offered_load"].Status == Pass && (p.Status == Pass || p.Status == Fail) && !duplicate
				if r.MetricInterpretation == ProtocolMetricInterpretation && s.Claims["protocol_delivery"].Status != Pass {
					eligible = false
				}
				reasons := append([]string{}, p.Reasons...)
				if r.Claims["comparability"].Status != Pass {
					reasons = append(reasons, "provenance insufficient for comparison")
				}
				if duplicate {
					reasons = append(reasons, "duplicate run input; excluded, not an independent repetition")
				}
				if r.MetricInterpretation == ProtocolMetricInterpretation && s.Claims["protocol_delivery"].Status != Pass {
					reasons = append(reasons, "protocol delivery claim is not PASS")
				}
				claims := map[string]Claim{}
				for k, v := range s.Claims {
					claims[k] = v
				}
				claims["comparability"] = r.Claims["comparability"]
				profileHash := ""
				if r.Protocol != nil {
					profileHash = r.Protocol.ProfileSHA256
				}
				c.Rows = append(c.Rows, ComparisonRow{RunID: r.RunID, MetricInterpretation: r.MetricInterpretation, ProtocolBinding: comparisonProtocolBinding(r), ProfileSHA256: profileHash, Platform: r.Platform, Step: s.Index, Devices: s.Devices, Rate: s.OfferedRate, Configuration: r.Configuration, Profile: p.Profile, ProfileVerdict: p.Status, Claims: claims, Metrics: s.Metrics, Eligible: eligible, Group: key, Reasons: reasons, SensitivityChecks: p.SensitivityChecks})
			}
		}
	}
	groups := map[string][]ComparisonRow{}
	for _, r := range c.Rows {
		if r.Profile.DeviceComparison != nil {
			continue
		}
		groups[r.Group] = append(groups[r.Group], r)
	}
	for _, key := range sortedKeys(groups) {
		rows := groups[key]
		runs := map[string]bool{}
		pass, fail := map[float64]bool{}, map[float64]bool{}
		g := Group{Key: key, Capacity: claim(NotAssessed, "campaign qualification, independent repetitions and adopted complete SLO procedure are not established by these reports")}
		for _, r := range rows {
			runs[r.RunID] = true
			if r.Eligible {
				if r.ProfileVerdict == Pass {
					pass[r.Rate] = true
				} else {
					fail[r.Rate] = true
				}
			}
		}
		g.DistinctRuns = len(runs)
		for rate := range pass {
			g.PassingRates = append(g.PassingRates, rate)
		}
		for rate := range fail {
			g.FailingRates = append(g.FailingRates, rate)
		}
		sort.Float64s(g.PassingRates)
		sort.Float64s(g.FailingRates)
		if len(g.PassingRates) > 0 {
			v := g.PassingRates[len(g.PassingRates)-1]
			g.HighestObservedPass = &v
		}
		for _, rate := range g.PassingRates {
			all := true
			for _, r := range rows {
				if r.Rate <= rate && (!r.Eligible || r.ProfileVerdict != Pass) {
					all = false
				}
			}
			if all {
				v := rate
				g.HighestAllLowerPass = &v
			}
		}
		c.Groups = append(c.Groups, g)
	}
	c.DeviceComparisons = compareDevices(results)
	if len(c.DeviceComparisons) > 0 {
		c.Warnings = append(c.Warnings, "W2 alone matches 100-vs-10000 populations at fixed aggregate reference rates; design validity is not a performance or device-capacity verdict.")
	}
	return c
}

func comparisonProtocolBinding(r *Result) *protocol.Binding {
	if r.Protocol == nil {
		return nil
	}
	b := r.Protocol.Binding
	return &b
}

func WriteComparison(out string, c Comparison, inputs []string) error {
	if e := newDirectory(out, inputs); e != nil {
		return e
	}
	if e := writeJSON(filepath.Join(out, "comparison.json"), c); e != nil {
		return e
	}
	f, e := os.Create(filepath.Join(out, "comparison.csv"))
	if e != nil {
		return e
	}
	w := csv.NewWriter(f)
	header := []string{"run_id", "platform", "step", "application_cpu_budget_k", "replicas_json", "devices", "offered_rate_messages_per_second", "profile", "requirements_json", "profile_verdict", "offered_load", "sink", "history", "ack_latency", "delivery_latency", "cpu_cost", "comparability", "eligible", "group", "reasons", "sensitivity_checks_json", "metric_interpretation", "protocol_id", "protocol_sha256", "profile_sha256", "protocol_delivery_status", "protocol_delivery_reasons", "protocol_scheduled", "protocol_observed", "protocol_missing", "protocol_delivery_p50_ms", "protocol_delivery_p95_ms", "protocol_delivery_p99_ms", "protocol_attainment_250_count", "protocol_attainment_250_ratio", "protocol_attainment_500_count", "protocol_attainment_500_ratio", "protocol_attainment_1000_count", "protocol_attainment_1000_ratio", "protocol_attainment_5000_count", "protocol_attainment_5000_ratio"}
	_ = w.Write(header)
	for _, r := range c.Rows {
		replicas, _ := json.Marshal(r.Configuration.Replicas)
		requirements, _ := json.Marshal(r.Profile.Requirements)
		row := []string{r.RunID, r.Platform, fmt.Sprint(r.Step), fmt.Sprint(r.Configuration.CPUAllocationK), string(replicas), fmt.Sprint(r.Devices), fmt.Sprint(r.Rate), r.Profile.ID, string(requirements), string(r.ProfileVerdict)}
		for _, name := range []string{"offered_load", "sink", "history", "ack_latency", "delivery_latency", "cpu_cost", "comparability"} {
			row = append(row, string(r.Claims[name].Status))
		}
		row = append(row, fmt.Sprint(r.Eligible), r.Group, strings.Join(r.Reasons, "; "))
		sensitivities, _ := json.Marshal(r.SensitivityChecks)
		row = append(row, string(sensitivities))
		row = append(row, r.MetricInterpretation)
		protocolID, protocolHash, profileHash := "", "", ""
		if r.ProtocolBinding != nil {
			protocolID, protocolHash = r.ProtocolBinding.ProtocolID, r.ProtocolBinding.ProtocolSHA256
		}
		profileHash = r.ProfileSHA256
		row = append(row, protocolID, protocolHash, profileHash, string(r.Claims["protocol_delivery"].Status), strings.Join(r.Claims["protocol_delivery"].Reasons, "; "))
		for _, name := range []string{"protocol.scheduled_cohort.scheduled", "protocol.scheduled_cohort.observed", "protocol.scheduled_cohort.missing", "protocol.delivery.p50_ms", "protocol.delivery.p95_ms", "protocol.delivery.p99_ms", "protocol.delivery.attainment_250_count", "protocol.delivery.attainment_250_ratio", "protocol.delivery.attainment_500_count", "protocol.delivery.attainment_500_ratio", "protocol.delivery.attainment_1000_count", "protocol.delivery.attainment_1000_ratio", "protocol.delivery.attainment_5000_count", "protocol.delivery.attainment_5000_ratio"} {
			value := ""
			if metric, ok := r.Metrics[name]; ok && metric.Value != nil {
				value = fmt.Sprint(*metric.Value)
			}
			row = append(row, value)
		}
		if e = w.Write(row); e != nil {
			f.Close()
			return e
		}
	}
	w.Flush()
	e = w.Error()
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if len(c.DeviceComparisons) > 0 {
		if e = writeDeviceCSV(filepath.Join(out, "device-comparison.csv"), c.DeviceComparisons); e != nil {
			return e
		}
	}
	return checksums(out)
}

func writeDeviceCSV(path string, studies []DeviceStudy) error {
	f, e := os.Create(path)
	if e != nil {
		return e
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"study", "run_id", "step", "devices", "offered_rate_messages_per_second", "reference_fraction", "actual_rate_to_reference_ratio", "cell_validity", "design_validity", "profile_verdict", "reasons", "metrics_json", "claims_json"})
	for _, study := range studies {
		for _, cell := range study.Cells {
			fraction, ratio := "", ""
			if cell.ReferenceFraction != nil {
				fraction = fmt.Sprint(*cell.ReferenceFraction)
			}
			if cell.ActualReferenceRatio != nil {
				ratio = fmt.Sprint(*cell.ActualReferenceRatio)
			}
			metrics, _ := json.Marshal(cell.Metrics)
			claims, _ := json.Marshal(cell.Claims)
			if e = w.Write([]string{study.Key, cell.RunID, fmt.Sprint(cell.Step), fmt.Sprint(cell.Devices), fmt.Sprint(cell.OfferedRate), fraction, ratio, string(cell.Validity.Status), string(study.DesignValidity.Status), string(study.ProfileVerdict.Status), strings.Join(cell.Validity.Reasons, "; "), string(metrics), string(claims)}); e != nil {
				f.Close()
				return e
			}
		}
	}
	w.Flush()
	e = w.Error()
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
