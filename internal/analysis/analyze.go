package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"iotbench/internal/audit"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

type reader struct {
	result *Result
	base   string
}

func (r reader) read(role, path string) ([]byte, error) {
	if path != "" && !filepath.IsAbs(path) {
		path = filepath.Join(r.base, path)
	}
	b, err := os.ReadFile(path)
	in := Input{Role: role, Path: path, Status: "AVAILABLE", Bytes: len(b)}
	if err != nil {
		in.Status = "UNAVAILABLE"
		in.Error = err.Error()
	} else {
		h := sha256.Sum256(b)
		in.SHA256 = hex.EncodeToString(h[:])
	}
	r.result.Inputs = append(r.result.Inputs, in)
	return b, err
}

func decode(b []byte, v any, strict bool) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if strict {
		d.DisallowUnknownFields()
	}
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON or corrupt input")
	}
	return nil
}

func rows[T any](b []byte) ([]T, error) {
	var out []T
	for i, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var v T
		if err := decode(line, &v, false); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func finite(v float64) bool  { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func integer(v float64) bool { return finite(v) && v >= 0 && math.Trunc(v) == v }

func Analyze(o Options) (*Result, error) {
	if err := guardProtocolIntent(o.Run); err != nil {
		return nil, err
	}
	r := &Result{SchemaVersion: 1, Claims: map[string]Claim{}, Provenance: map[string]json.RawMessage{}, CampaignVerdict: "NOT_ASSESSED", MetricInterpretation: LegacyMetricInterpretation}
	r.Limitations = []string{
		"No durable-commit latency producer exists; history is retrievable visibility, not commit timing.",
		"Success identities are inferred from complete aggregate outcomes and the planned ledger, not individual success records.",
		"Per-step k6 trend percentiles are not mergeable: exact run-wide acknowledgement p99 is unavailable without raw samples.",
		"Observation completeness, clock relationship and configuration sidecars are explicit caller attestations, not independently verified capture facts.",
		"This offline analysis does not qualify the campaign, establish permanent loss, or adopt application thresholds.",
	}
	r.Claims["offered_load"] = claim(NotAssessed, "mandatory generator artifacts not yet validated")
	r.Claims["comparability"] = claim(NotAssessed, "configuration and provenance unavailable")
	rd := reader{r, o.Run}
	policy := DefaultPolicy()
	ev := Evidence{SchemaVersion: 1, Interpretation: "claim-gates-v1"}
	if o.Evidence != "" {
		b, e := rd.read("evidence_spec", absolute(o.Evidence))
		if e != nil {
			return nil, e
		}
		if e = decode(b, &ev, true); e != nil {
			return nil, fmt.Errorf("evidence spec: %w", e)
		}
	}
	if o.Policy != "" {
		// Decode an explicit policy independently, not over nested default fields.
		policy = Policy{}
		b, e := rd.read("profile_policy", absolute(o.Policy))
		if e != nil {
			return nil, e
		}
		if e = decode(b, &policy, true); e != nil {
			return nil, fmt.Errorf("policy: %w", e)
		}
	}
	if ev.SchemaVersion != 1 || policy.SchemaVersion != 1 || ev.Interpretation == "" || policy.Interpretation != ev.Interpretation {
		return nil, fmt.Errorf("unsupported or conflicting schema/interpretation")
	}
	if err := validatePolicy(policy); err != nil {
		return nil, err
	}
	r.Synthetic = ev.Synthetic
	r.CaptureSpecification = &ev
	r.Policy = &policy
	r.Interpretation = ev.Interpretation
	r.Configuration = ev.Configuration
	knownProfiles := map[string]bool{}
	for _, p := range policy.Profiles {
		knownProfiles[p.ID] = true
	}
	for _, p := range DefaultPolicy().Profiles {
		if !knownProfiles[p.ID] {
			policy.Profiles = append(policy.Profiles, p)
		}
	}
	cpuSteps := map[int]bool{}
	for _, in := range ev.CPU {
		if cpuSteps[in.Step] {
			return nil, fmt.Errorf("duplicate CPU input for step %d", in.Step)
		}
		cpuSteps[in.Step] = true
	}
	if policy.Synthetic && !ev.Synthetic {
		return nil, fmt.Errorf("synthetic thresholds cannot assess evidence not labelled synthetic")
	}
	b, err := rd.read("resolved_plan", "resolved.json")
	if err != nil {
		r.Claims["offered_load"] = claim(NotAssessed, "resolved.json unavailable")
		return r, nil
	}
	var resolved plan.Resolved
	if err = decode(b, &resolved, false); err != nil {
		r.Claims["offered_load"] = claim(Invalid, "malformed resolved plan: "+err.Error())
		return r, nil
	}
	if resolved.SchemaVersion != 0 && resolved.SchemaVersion != 1 {
		r.Claims["offered_load"] = claim(NotAssessed, "resolved plan schema unsupported by this analyzer")
		return r, nil
	}
	r.RunID = resolved.RunID
	r.Platform = resolved.Adapter
	r.Seed = resolved.Seed
	var warmup []plan.Step
	for _, st := range resolved.Steps {
		if st.Phase == "warmup" {
			warmup = append(warmup, st)
		}
	}
	workload, _ := json.Marshal(map[string]any{"arrivals": resolved.Arrivals, "connection": resolved.Connection, "request_timeout_ms": resolved.RequestTimeMs, "warmup": warmup})
	r.Provenance["workload_parameters"] = workload
	if err = validatePlan(&resolved); err != nil {
		r.Claims["offered_load"] = claim(Invalid, err.Error())
		return r, nil
	}
	protocolInfo, protocolManifestBytes, protocolManifest, bindErr := prepareProtocolBinding(o, rd, &resolved)
	if bindErr != nil {
		return nil, bindErr
	}
	protocolMode := protocolInfo != nil
	if protocolMode {
		r.Protocol = protocolInfo
		r.MetricInterpretation = ProtocolMetricInterpretation
		r.Limitations = append(r.Limitations, "Protocol receipt timestamps are same-host shared-load-realtime wall-clock observations; no monotonic, cross-host offset, end-to-end delivery or durable-commit qualification is claimed.")
	}
	b, err = rd.read("generator_summary", "k6-summary.json")
	if err != nil {
		r.Claims["offered_load"] = claim(NotAssessed, "k6 summary unavailable")
		return r, nil
	}
	var sum report.Summary
	if err = decode(b, &sum, false); err != nil {
		r.Claims["offered_load"] = claim(Invalid, "malformed summary: "+err.Error())
		return r, nil
	}
	if sum.SchemaVersion != 0 && sum.SchemaVersion != 2 {
		r.Claims["offered_load"] = claim(NotAssessed, "generator summary schema unsupported by this analyzer")
		return r, nil
	}
	summaryIssue := validateSummary(b, &resolved, &sum)
	if summaryIssue != nil && (sum.SchemaVersion != 2 || len(sum.Steps) != len(resolved.Steps)) {
		r.Claims["offered_load"] = claim(Invalid, summaryIssue.Error())
		return r, nil
	}
	b, ledgerErr := rd.read("outcome_ledger", "records.ndjson")
	recs, parseErr := rows[report.Record](b)
	if parseErr == nil && ledgerErr == nil {
		objects, _ := rows[map[string]json.RawMessage](b)
		for i, object := range objects {
			for _, key := range []string{"rec", "run_id", "seq", "device_id", "rate_step", "phase", "tick", "scheduled_send_ms", "actual_send_ms", "response_ms", "outcome", "early"} {
				if value, ok := object[key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					parseErr = fmt.Errorf("line %d: missing/null %s", i+1, key)
					break
				}
			}
			if parseErr != nil {
				break
			}
		}
	}
	var blockers []string
	if summaryIssue != nil {
		blockers = append(blockers, summaryIssue.Error())
	}
	if ledgerErr != nil {
		blockers = append(blockers, "required outcome ledger is missing; absence cannot establish successful outcomes")
	}
	if parseErr != nil {
		blockers = append(blockers, "malformed outcome ledger: "+parseErr.Error())
	}
	planned := audit.PlannedMessages(&resolved, &sum)
	var protocolCapture *ProtocolReceiptCapture
	var protocolCaptureErr error
	if protocolMode {
		captureBytes, captureErr := rd.read("protocol_receipt_capture", "protocol-receipts.json")
		protocolCaptureErr = captureErr
		if captureErr == nil {
			var capture ProtocolReceiptCapture
			if e := validateProtocolCaptureRaw(captureBytes); e != nil {
				protocolCaptureErr = fmt.Errorf("malformed protocol receipt capture: %w", e)
			} else if e := decode(captureBytes, &capture, true); e != nil {
				protocolCaptureErr = fmt.Errorf("malformed protocol receipt capture: %w", e)
			} else {
				protocolCapture = &capture
				r.Protocol.ReceiptCaptureSchemaVersion = capture.SchemaVersion
				r.Protocol.ReceiptCaptureSemantics = capture.ReceiptCaptureSemantics
			}
		}
	}
	if ledgerErr == nil && parseErr == nil {
		if err = validateLedger(&resolved, planned, recs); err != nil {
			blockers = append(blockers, err.Error())
		}
	}
	cb, cpuErr := rd.read("generator_cpu", "gen-cpu.ndjson")
	cpuSamples, cpuParseErr := rows[report.CPUSample](cb)
	if resolved.Health.MaxCoreBusy > 0 && (cpuErr != nil || cpuParseErr != nil || len(cpuSamples) == 0) {
		blockers = append(blockers, "required generator CPU samples unavailable or malformed")
	}
	if cpuParseErr != nil {
		cpuSamples = nil
		r.Limitations = append(r.Limitations, "optional generator CPU diagnostic malformed; not used to manufacture zero CPU")
	}
	var mb []byte
	var manifestErr error
	manifest := map[string]json.RawMessage{}
	if protocolMode {
		mb, manifest = protocolManifestBytes, protocolManifest
	} else {
		mb, manifestErr = rd.read("manifest", "manifest.json")
		if manifestErr == nil {
			if err = decode(mb, &manifest, false); err != nil {
				blockers = append(blockers, "malformed manifest: "+err.Error())
			}
		}
	}
	if raw, ok := manifest["synthetic"]; ok {
		var synthetic bool
		if json.Unmarshal(raw, &synthetic) == nil && synthetic {
			r.Synthetic = true
			ev.Synthetic = true
		}
	}
	for _, k := range []string{"git", "k6", "k6_scripts_sha256", "generator_cpuset", "connection_model"} {
		if v, ok := manifest[k]; ok {
			r.Provenance[k] = v
		}
	}
	for key, want := range map[string]string{"run_id": resolved.RunID, "plan_id": resolved.PlanID} {
		if manifestErr == nil {
			var got string
			if decode(manifest[key], &got, false) != nil || got != want {
				blockers = append(blockers, "manifest "+key+" contradicts resolved plan")
			}
		}
	}
	if raw, ok := manifest["resolved_plan"]; ok {
		var mplan plan.Resolved
		if decode(raw, &mplan, false) != nil || !reflect.DeepEqual(mplan, resolved) {
			blockers = append(blockers, "manifest resolved_plan contradicts input plan")
		}
	}
	if manifestErr == nil {
		var exit *int
		if decode(manifest["k6_exit_code"], &exit, false) != nil || exit == nil {
			blockers = append(blockers, "manifest k6 exit status missing")
		} else if *exit != 0 {
			blockers = append(blockers, fmt.Sprintf("generator exited with code %d", *exit))
		}
	}
	if parseErr == nil {
		h := report.EvaluateInputs(&resolved, &sum, recs, cpuSamples)
		r.Health = h
		if !h.Valid {
			blockers = append(blockers, h.Reasons...)
		}
		var retained string
		if raw, ok := manifest["verdict"]; ok && json.Unmarshal(raw, &retained) == nil && retained == "INVALID" && h.Valid {
			blockers = append(blockers, "retained INVALID manifest contradicts recomputed passing health; review rather than overwrite evidence")
		}
	}
	if hb, he := rd.read("retained_health", "health.json"); he == nil {
		var saved struct {
			Valid *bool `json:"valid"`
		}
		if decode(hb, &saved, false) != nil || saved.Valid == nil {
			blockers = append(blockers, "retained health record malformed")
		} else if !*saved.Valid && r.Health != nil && r.Health.Valid {
			blockers = append(blockers, "retained invalid health contradicts recomputed health")
		}
	}
	if len(blockers) > 0 {
		r.Claims["offered_load"] = Claim{Invalid, blockers}
	} else if manifestErr != nil {
		r.Claims["offered_load"] = claim(NotAssessed, "manifest unavailable; generator exit/setup status not established")
	} else {
		r.Claims["offered_load"] = claim(Pass, "existing generator health gates and ledger identity/accounting checks passed")
	}
	r.Claims["comparability"] = provenanceClaim(r, manifest, ev.Configuration)
	if ev.Configuration.EffectivePath != "" {
		b, e := rd.read("effective_configuration", ev.Configuration.EffectivePath)
		if e != nil {
			r.Claims["comparability"] = claim(NotAssessed, "effective configuration file unavailable")
		} else {
			h := sha256.Sum256(b)
			if hex.EncodeToString(h[:]) != ev.Configuration.EffectiveSHA256 {
				r.Claims["comparability"] = claim(Invalid, "effective configuration hash mismatch")
			}
		}
	} else {
		r.Claims["comparability"] = claim(NotAssessed, "effective configuration artifact not supplied")
	}
	r.Claims["w2_reference"] = validateW2Reference(rd, ev.W2Reference)
	for _, st := range resolved.Steps {
		if st.Phase == "warmup" {
			continue
		}
		var ss report.StepSummary
		for _, x := range sum.Steps {
			if x.Index == st.Index {
				ss = x
			}
		}
		s := Step{Index: st.Index, Name: st.Name, Phase: st.Phase, Devices: resolved.Devices, OfferedRate: st.Rate, StartMS: ss.ScenarioStartMs, EndMS: ss.ScenarioStartMs + float64(st.DurationS)*1000, Claims: map[string]Claim{}, Metrics: map[string]Metric{}}
		s.Claims["offered_load"] = r.Claims["offered_load"]
		s.Claims["w2_reference"] = r.Claims["w2_reference"]
		addBaseMetrics(&s, st, ss, &resolved)
		httpSamples(&s, rd, resolved.RunID, planned)
		analyseObservations(&s, rd, ev, &resolved, &sum, recs, planned, policy.BacklogRule)
		if protocolMode {
			analyseProtocolCohort(&s, protocolCapture, protocolCaptureErr, protocolInfo, *resolved.ProtocolPilot, &resolved, &sum, planned, ev.Synthetic, manifest)
		}
		analyseCPU(&s, rd, ev)
		for _, profile := range policy.Profiles {
			a := assess(profile, &s)
			s.Profiles = append(s.Profiles, a)
			if profile.ID == "service_slo" {
				s.Claims["service_slo"] = serviceClaim(a)
			}
		}
		r.Steps = append(r.Steps, s)
	}
	for _, in := range r.Inputs {
		if in.Status == "AVAILABLE" {
			b, e := os.ReadFile(in.Path)
			h := sha256.Sum256(b)
			if e != nil || hex.EncodeToString(h[:]) != in.SHA256 {
				bad := claim(Invalid, "input changed during analysis: "+in.Path)
				r.Claims["offered_load"] = bad
				r.Claims["comparability"] = bad
				for i := range r.Steps {
					for k := range r.Steps[i].Claims {
						r.Steps[i].Claims[k] = bad
					}
					for j := range r.Steps[i].Profiles {
						r.Steps[i].Profiles[j].Claim = bad
					}
				}
			}
		}
	}
	return r, nil
}

func absolute(path string) string {
	p, e := filepath.Abs(path)
	if e != nil {
		return path
	}
	return p
}

func validatePlan(r *plan.Resolved) error {
	if r.SchemaVersion != 1 || r.RunID == "" || r.Devices <= 0 || len(r.Steps) == 0 {
		return fmt.Errorf("missing/unsupported resolved schema, identity, devices or steps")
	}
	if r.Arrivals.Model != "constant" && r.Arrivals.Model != "thinning" {
		return fmt.Errorf("unsupported arrival model")
	}
	if !finite(r.SendProb) || r.SendProb <= 0 || r.SendProb > 1 {
		return fmt.Errorf("invalid arrival selection probability")
	}
	if r.Health.MinAchievedRatio <= 0 || r.Health.MinAchievedRatio > 1 || r.Health.SendLagP99Ms <= 0 || r.Health.EarlyToleranceMs < 0 {
		return fmt.Errorf("missing or invalid adopted health gates")
	}
	var seq int64
	var end int
	for i, s := range r.Steps {
		if s.Index != i || s.Name == "" || !finite(s.Rate) || s.Rate <= 0 || !finite(s.TickRate) || s.TickRate <= 0 || s.DurationS <= 0 || s.Ticks <= 0 || s.SeqOffset != seq || s.StartS < end || float64(s.Ticks) != s.TickRate*float64(s.DurationS) {
			return fmt.Errorf("inconsistent resolved schedule at step %d", i)
		}
		if r.Arrivals.Model == "constant" && (s.TickRate != s.Rate || r.SendProb != 1) {
			return fmt.Errorf("constant-arrival rate/probability contradict executor")
		}
		seq += s.Ticks
		end = s.StartS + s.DurationS
	}
	if r.TotalTicks != seq || r.DurationS != end {
		return fmt.Errorf("resolved total ticks/duration contradict steps")
	}
	return nil
}

func validateSummary(raw []byte, r *plan.Resolved, s *report.Summary) error {
	if s.SchemaVersion != 2 || s.RunID != r.RunID || len(s.Steps) != len(r.Steps) {
		return fmt.Errorf("summary schema, run identity or step coverage mismatch")
	}
	var structure struct {
		Steps []map[string]json.RawMessage `json:"steps"`
	}
	_ = json.Unmarshal(raw, &structure)
	var dropped float64
	for i, x := range s.Steps {
		for _, key := range []string{"index", "name", "scenario_start_ms", "attempted", "ack_success", "explicit_failure", "unknown", "early", "beyond_plan", "thinned", "dup_injected", "dropped_iterations", "http_reqs", "send_lag_ms", "ack_latency_ms"} {
			if value, ok := structure.Steps[i][key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return fmt.Errorf("summary step %d missing %s", i, key)
			}
		}
		if x.Index != r.Steps[i].Index || x.Name != r.Steps[i].Name || !finite(x.ScenarioStartMs) || x.ScenarioStartMs <= 0 {
			return fmt.Errorf("inconsistent scenario identity/timestamps")
		}
		for _, key := range []string{"send_lag_ms", "ack_latency_ms"} {
			var stats map[string]json.RawMessage
			_ = json.Unmarshal(structure.Steps[i][key], &stats)
			for _, field := range []string{"count", "p50", "p99", "max"} {
				if value, ok := stats[field]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return fmt.Errorf("summary step %d missing %s.%s", i, key, field)
				}
			}
		}
		if i > 0 && x.ScenarioStartMs <= s.Steps[i-1].ScenarioStartMs {
			return fmt.Errorf("non-increasing scenario timestamps")
		}
		for _, n := range []float64{x.Attempted, x.AckSuccess, x.ExplicitFailure, x.Unknown, x.Early, x.BeyondPlan, x.Thinned, x.DupInjected, x.Dropped, x.HTTPReqs, x.SendLag.Count, x.AckLatency.Count} {
			if !integer(n) {
				return fmt.Errorf("negative, fractional or non-finite counter in step %d", i)
			}
		}
		if x.SendLag.Count != x.Attempted || x.AckLatency.Count != x.AckSuccess {
			return fmt.Errorf("latency observation counts contradict outcomes in step %d", i)
		}
		if x.HTTPReqs != x.Attempted || x.DupInjected > 0 || r.Arrivals.Model == "constant" && x.Thinned > 0 {
			return fmt.Errorf("HTTP sends, injected duplicates or thinning contradict producer in step %d", i)
		}
		if x.Dropped == 0 && int64(x.Attempted) != report.PlannedSends(r, r.Steps[i]) {
			return fmt.Errorf("exact planned-identity set cannot be inferred from attempted count in step %d", i)
		}
		for _, t := range []report.TrendStats{x.SendLag, x.AckLatency} {
			if t.Count > 0 && (!finite(t.P50) || !finite(t.P99) || !finite(t.Max) || t.P50 > t.P99 || t.P99 > t.Max) {
				return fmt.Errorf("invalid percentile distribution in step %d", i)
			}
		}
		dropped += x.Dropped
	}
	if dropped != s.DroppedTotal {
		return fmt.Errorf("summary total dropped counter contradicts per-step counters")
	}
	return nil
}

func validateLedger(r *plan.Resolved, planned []audit.Planned, recs []report.Record) error {
	bySeq := map[int64]audit.Planned{}
	for _, p := range planned {
		bySeq[p.Seq] = p
	}
	seen := map[int64]bool{}
	for _, rc := range recs {
		p, ok := bySeq[rc.Seq]
		if !ok || seen[rc.Seq] || rc.RunID != r.RunID || rc.Rec != "msg" || rc.Device != int(rc.Seq%int64(r.Devices)) || rc.Step < 0 || rc.Step >= len(r.Steps) || r.Steps[rc.Step].Name != p.Step || rc.Phase != r.Steps[rc.Step].Phase || rc.Tick != rc.Seq-r.Steps[rc.Step].SeqOffset {
			return fmt.Errorf("outcome ledger has duplicate, malformed, unexpected or mismatched identity seq %d", rc.Seq)
		}
		seen[rc.Seq] = true
		if rc.Outcome != "ack_success" && rc.Outcome != "explicit_failure" && rc.Outcome != "unknown" {
			return fmt.Errorf("unknown outcome category at seq %d", rc.Seq)
		}
		if !finite(rc.SchedMs) || !finite(rc.ActualMs) || !finite(rc.RespMs) || math.Abs(rc.SchedMs-p.SchedMs) > .0011 || rc.RespMs < rc.ActualMs {
			return fmt.Errorf("inconsistent outcome timestamps at seq %d", rc.Seq)
		}
		early := rc.ActualMs-rc.SchedMs < -r.Health.EarlyToleranceMs
		if early != rc.Early || rc.Outcome == "ack_success" && !rc.Early {
			return fmt.Errorf("outcome ledger early/success record contradicts producer schema at seq %d", rc.Seq)
		}
	}
	if len(r.TestFaults.DuplicateSeqs) > 0 || r.TestFaults.EarlyEvery > 0 {
		return fmt.Errorf("generator fault injection active; not an unmodified offered-load measurement")
	}
	return nil
}

func provenanceClaim(r *Result, m map[string]json.RawMessage, c Configuration) Claim {
	var git, k6 map[string]string
	var scripts string
	_ = json.Unmarshal(m["git"], &git)
	_ = json.Unmarshal(m["k6"], &k6)
	_ = json.Unmarshal(m["k6_scripts_sha256"], &scripts)
	var reasons []string
	if !hexLength(git["sha"], 40) && !hexLength(git["sha"], 64) || git["dirty"] != "false" {
		reasons = append(reasons, "clean source commit not established")
	}
	if !imageDigest(k6["digest"]) || !hexLength(scripts, 64) {
		reasons = append(reasons, "generator image digest or executed script hash unavailable")
	}
	if c.ID == "" || !hexLength(c.EffectiveSHA256, 64) || c.CPUAllocationK <= 0 || len(c.Replicas) == 0 || len(c.Images) == 0 {
		reasons = append(reasons, "effective configuration hash, CPU budget, actual replicas or platform images missing")
	}
	for _, n := range c.Replicas {
		if n <= 0 {
			reasons = append(reasons, "invalid replica count")
		}
	}
	for _, image := range c.Images {
		if !imageDigest(image) {
			reasons = append(reasons, "platform image digest unavailable")
		}
	}
	if len(reasons) > 0 {
		return Claim{NotAssessed, reasons}
	}
	return claim(Pass, "provenance fields present; comparison still requires matching workload, versions, configuration, profile and endpoint")
}

func hexLength(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func imageDigest(s string) bool {
	parts := strings.Split(s, "@sha256:")
	return len(parts) == 2 && parts[0] != "" && hexLength(parts[1], 64)
}
