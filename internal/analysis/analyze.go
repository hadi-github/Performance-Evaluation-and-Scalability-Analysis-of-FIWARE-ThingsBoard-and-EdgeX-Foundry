package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"iotbench/internal/audit"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func Analyze(o Options) (*Result, error) {
	if err := guardProtocolIntent(o.Run); err != nil {
		return nil, err
	}
	r := &Result{SchemaVersion: 1, Claims: map[string]Claim{}, Provenance: map[string]json.RawMessage{}, CampaignVerdict: "NOT_ASSESSED", MetricInterpretation: LegacyMetricInterpretation}
	r.Limitations = []string{
		"No durable-commit latency producer exists; history is retrievable visibility, not commit timing.",
		"For legacy summaries without per-message success rows, success identities are inferred from aggregate outcomes and the planned ledger; protocol-bound runs validate the per-message outcome ledger.",
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
		completeLedger := false
		if protocolMode {
			var ledgerMode string
			completeLedger = json.Unmarshal(protocolManifest["outcome_ledger_interpretation"], &ledgerMode) == nil && ledgerMode == "complete-dispatch-outcome-v1"
		}
		if err = validateLedger(&resolved, planned, recs, completeLedger); err != nil {
			blockers = append(blockers, err.Error())
		}
		if protocolMode && !ev.Synthetic {
			if !completeLedger {
				blockers = append(blockers, "protocol-bound manifest lacks complete dispatch/outcome ledger interpretation")
			} else if e := validateCompleteOutcomeLedger(&resolved, &sum, recs); e != nil {
				blockers = append(blockers, e.Error())
			}
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
		}
		if protocolMode {
			if json.Unmarshal(raw, &synthetic) != nil || synthetic != ev.Synthetic {
				blockers = append(blockers, "protocol manifest synthetic/live provenance is missing, malformed, or contradicts evidence specification")
			}
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
		if in.Status == Available {
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
