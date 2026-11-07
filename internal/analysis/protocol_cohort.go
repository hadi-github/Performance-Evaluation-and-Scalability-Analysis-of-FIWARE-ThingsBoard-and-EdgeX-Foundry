package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"iotbench/internal/audit"
	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/receipt"
	"iotbench/internal/report"
)

func putProtocolMetric(s *Step, name string, value *float64, unit, basis string) {
	s.Metrics[name] = Metric{Value: value, Unit: unit, Claim: "protocol_delivery", Endpoint: "scheduled_measurement_to_first_complete_body_receipt", ClockDomain: "capture-declared epoch milliseconds", Basis: basis}
}
func protocolValue(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	x := v
	return &x
}

func analyseProtocolCohort(s *Step, capture *ProtocolReceiptCapture, captureErr error, provenance *ProtocolProvenance, pilot protocol.PilotParameters, r *plan.Resolved, sum *report.Summary, planned []audit.Planned, synthetic bool, manifest map[string]json.RawMessage) {
	var expected []audit.Planned
	allBySeq := map[int64]audit.Planned{}
	phaseByName := map[string]string{}
	for _, st := range r.Steps {
		phaseByName[st.Name] = st.Phase
	}
	for _, p := range planned {
		allBySeq[p.Seq] = p
		if p.Step == s.Name && phaseByName[p.Step] == "measurement" {
			expected = append(expected, p)
		}
	}
	scheduled := float64(len(expected))
	putProtocolMetric(s, "protocol.scheduled_cohort.scheduled", protocolValue(scheduled), "messages", "planned measurement identities in this exact resolved step; half-open scheduled phase [start,end)")
	s.Claims["protocol_delivery"] = claim(NotAssessed, "protocol receipt capture unavailable")
	if captureErr != nil || capture == nil {
		return
	}
	if s.Claims["offered_load"].Status != Pass {
		s.Claims["protocol_delivery"] = dependent(s.Claims["offered_load"])
		return
	}
	if r.TotalTicks >= 1<<32 || len(r.TestFaults.DuplicateSeqs) > 0 || r.TestFaults.EarlyEvery > 0 {
		s.Claims["protocol_delivery"] = claim(Invalid, "protocol cohort identity mapping is not valid under generator overflow/fault injection")
		return
	}
	switch capture.SchemaVersion {
	case receipt.SchemaVersion, receipt.DispatchSchemaVersion:
		if capture.ReceiptCaptureSemantics != receipt.CaptureSemantics {
			s.Claims["protocol_delivery"] = claim(NotAssessed, "complete-body receipt capture semantics marker is missing or unsupported")
			return
		}
	case receipt.LegacySchemaVersion:
		s.Claims["protocol_delivery"] = claim(NotAssessed, "legacy receipt capture schema 1 has no complete-body-before-parse semantics; new primary metrics are unavailable")
		return
	default:
		s.Claims["protocol_delivery"] = claim(Invalid, "unsupported receipt capture schema version")
		return
	}
	interpretation := ProtocolMetricInterpretation
	if capture.SchemaVersion == receipt.DispatchSchemaVersion {
		interpretation = DispatchMetricInterpretation
	}
	if capture.MetricInterpretation != interpretation || capture.Binding != provenance.Binding {
		s.Claims["protocol_delivery"] = claim(Invalid, "receipt capture interpretation or protocol binding mismatch")
		return
	}
	manifestIsSynthetic := manifestSynthetic(manifest)
	if capture.Synthetic != synthetic || capture.Synthetic != manifestIsSynthetic {
		s.Claims["protocol_delivery"] = claim(Invalid, "receipt capture, analyzer evidence specification, and manifest disagree on synthetic/live provenance")
		return
	}
	endpoint := receipt.Endpoint
	if capture.SchemaVersion == receipt.DispatchSchemaVersion {
		endpoint = "application_dispatch_to_response_complete_body_read"
	}
	if !capture.Complete || !capture.BodyReadComplete || capture.Endpoint != endpoint || capture.Source == "" || capture.ClockDomain == "" || capture.ClockSource == "" {
		s.Claims["protocol_delivery"] = claim(NotAssessed, "complete-body receipt capture endpoint, completeness, source, or clock metadata missing")
		return
	}
	if !finiteTime(capture.CoverageStartMS) || !finiteTime(capture.CoverageEndMS) || capture.CoverageStartMS <= 0 {
		s.Claims["protocol_delivery"] = claim(Invalid, "receipt capture coverage bounds are invalid")
		return
	}
	var phaseStart, phaseEnd float64
	phaseSet := false
	for i, st := range r.Steps {
		if st.Phase != "measurement" {
			continue
		}
		if i >= len(sum.Steps) || sum.Steps[i].Index != i || !finiteTime(sum.Steps[i].ScenarioStartMs) {
			s.Claims["protocol_delivery"] = claim(Invalid, "measurement scenario start missing or mismatched")
			return
		}
		start := sum.Steps[i].ScenarioStartMs
		end := start + float64(st.DurationS)*1000
		if !phaseSet || start < phaseStart {
			phaseStart = start
		}
		if !phaseSet || end > phaseEnd {
			phaseEnd = end
		}
		phaseSet = true
	}
	expectedCutoff := float64(0)
	if pilot.DrainCutoffUnixMS != nil && pilot.DrainSecondsAfterMeasurement == nil {
		expectedCutoff = float64(*pilot.DrainCutoffUnixMS)
	} else if pilot.DrainCutoffUnixMS == nil && pilot.DrainSecondsAfterMeasurement != nil {
		expectedCutoff = phaseEnd + float64(*pilot.DrainSecondsAfterMeasurement)*1000
	}
	if !phaseSet || capture.ObservationCutoffMS != expectedCutoff || capture.CoverageEndMS < capture.ObservationCutoffMS {
		s.Claims["protocol_delivery"] = claim(Invalid, "receipt capture coverage/cutoff contradicts the declared pilot drain policy")
		return
	}
	if !phaseSet || capture.CoverageStartMS > phaseStart || capture.ObservationCutoffMS < phaseEnd {
		s.Claims["protocol_delivery"] = claim(NotAssessed, "capture does not cover the full half-open measurement phase and drain cutoff")
		return
	}
	if capture.Synthetic {
		if !strings.HasPrefix(capture.Source, "synthetic:") || !strings.HasPrefix(capture.ClockSource, "synthetic:") || !strings.HasPrefix(capture.ClockDomain, "synthetic:") {
			s.Claims["protocol_delivery"] = claim(Invalid, "synthetic capture/clock metadata is not explicitly labeled synthetic")
			return
		}
	} else if strings.HasPrefix(capture.Source, "synthetic:") || strings.HasPrefix(capture.ClockSource, "synthetic:") || strings.HasPrefix(capture.ClockDomain, "synthetic:") {
		s.Claims["protocol_delivery"] = claim(Invalid, "live capture contains synthetic source or clock metadata")
		return
	}
	stepBySeq := map[int64]audit.Planned{}
	for _, p := range expected {
		stepBySeq[p.Seq] = p
	}
	type receiptSet struct{ rows []ProtocolReceipt }
	sets := map[int64]receiptSet{}
	late := 0
	for _, receipt := range capture.Receipts {
		p, ok := allBySeq[receipt.Seq]
		expectedDevice := fmt.Sprintf("device-%d", p.Device)
		if !capture.Synthetic {
			expectedDevice = fmt.Sprintf("bench-%s-%05d", r.RunID, p.Device)
		}
		if !ok || receipt.RunID != r.RunID || receipt.DeviceID != expectedDevice || receipt.Phase != phaseByName[p.Step] || !finiteTime(receipt.ScheduledSendMS) || !finiteTime(receipt.ObservedMS) || math.Abs(receipt.ScheduledSendMS-p.SchedMs) > .0011 || receipt.ObservedMS < p.SchedMs || !protocol.ValidateOutcome(receipt.Outcome, false) || receipt.Outcome == protocol.OutcomeNotSent {
			s.Claims["protocol_delivery"] = claim(Invalid, fmt.Sprintf("invalid receipt identity, phase, schedule, clock, or outcome at seq %d", receipt.Seq))
			return
		}
		if receipt.ObservedMS > capture.ObservationCutoffMS {
			if _, isMeasurement := stepBySeq[receipt.Seq]; isMeasurement {
				late++
			}
			continue
		}
		set := sets[receipt.Seq]
		set.rows = append(set.rows, receipt)
		sets[receipt.Seq] = set
	}
	var latencies []float64
	duplicates, errorsSeen, unknownSeen := 0, 0, 0
	for seq, p := range stepBySeq {
		set := sets[seq]
		if len(set.rows) == 0 {
			continue
		}
		if len(set.rows) > 1 {
			duplicates += len(set.rows) - 1
		}
		firstTime := set.rows[0].ObservedMS
		for _, row := range set.rows[1:] {
			if row.ObservedMS < firstTime {
				firstTime = row.ObservedMS
			}
		}
		firstSet := false
		var first ProtocolReceipt
		for _, row := range set.rows {
			if row.ObservedMS != firstTime {
				continue
			}
			if !firstSet {
				first, firstSet = row, true
				continue
			}
			if row.Outcome != first.Outcome {
				s.Claims["protocol_delivery"] = claim(Invalid, fmt.Sprintf("ambiguous tied first receipt outcomes at seq %d", seq))
				return
			}
		}
		latency := first.ObservedMS - p.SchedMs
		if capture.SchemaVersion == receipt.DispatchSchemaVersion {
			if !finiteTime(first.ActualDispatchMS) || first.ActualDispatchMS < p.SchedMs-float64(r.Health.EarlyToleranceMs) || first.ActualDispatchMS > first.ObservedMS {
				s.Claims["protocol_delivery"] = claim(Invalid, fmt.Sprintf("invalid dispatch timestamp/clock step at seq %d", seq))
				return
			}
			latency = first.ObservedMS - first.ActualDispatchMS
		}
		if !finiteTime(latency) || latency < 0 {
			s.Claims["protocol_delivery"] = claim(Invalid, fmt.Sprintf("invalid first receipt latency at seq %d", seq))
			return
		}
		latencies = append(latencies, latency)
		switch first.Outcome {
		case protocol.OutcomeError:
			errorsSeen++
		case protocol.OutcomeUnknown:
			unknownSeen++
		}
	}
	counts := protocol.Counts(len(expected), len(latencies))
	putProtocolMetric(s, "protocol.scheduled_cohort.observed", protocolValue(float64(counts.Observed)), "messages", "unique first receipts through inclusive cutoff, all normalized outcomes")
	putProtocolMetric(s, "protocol.scheduled_cohort.missing", protocolValue(float64(counts.Missing)), "messages", "scheduled minus unique first receipts by cutoff; not exact NOT_SENT attribution")
	putProtocolMetric(s, "protocol.scheduled_cohort.duplicate_extra_receipts", protocolValue(float64(duplicates)), "receipts", "extra receipt rows for already observed scheduled identities; separately reported")
	putProtocolMetric(s, "protocol.scheduled_cohort.late_excluded_receipts", protocolValue(float64(late)), "receipts", "receipt rows strictly after inclusive cutoff; excluded from primary cohort")
	putProtocolMetric(s, "protocol.receipts.error", protocolValue(float64(errorsSeen)), "messages", "first observed receipt classified ERROR; request outcome ledger remains unchanged")
	putProtocolMetric(s, "protocol.receipts.unknown", protocolValue(float64(unknownSeen)), "messages", "first observed receipt classified UNKNOWN; request outcome ledger remains unchanged")
	for _, pc := range []struct {
		p    int
		name string
	}{{50, "p50"}, {95, "p95"}, {99, "p99"}} {
		putProtocolMetric(s, "protocol.delivery."+pc.name+"_ms", protocol.NearestRank(latencies, pc.p), "ms", "conditional nearest-rank percentile of unique first observed receipt latencies from application dispatch, including OK/ERROR/UNKNOWN; not full-cohort latency")
	}
	for _, threshold := range []float64{250, 500, 1000, 5000} {
		count, ratio := protocol.Attainment(counts.Scheduled, latencies, threshold)
		suffix := fmt.Sprintf("%d", int(threshold))
		putProtocolMetric(s, "protocol.delivery.attainment_"+suffix+"_count", protocolValue(float64(count)), "messages", fmt.Sprintf("unique first receipts at or below %g ms; denominator is all scheduled measurement identities", threshold))
		putProtocolMetric(s, "protocol.delivery.attainment_"+suffix+"_ratio", protocolValue(ratio), "ratio", fmt.Sprintf("scheduled denominator; %d/%d at or below %g ms", count, counts.Scheduled, threshold))
	}
	for name, metric := range s.Metrics {
		if strings.HasPrefix(name, "protocol.") {
			metric.ClockDomain = capture.ClockDomain
			if capture.SchemaVersion == receipt.DispatchSchemaVersion {
				metric.Endpoint = "application_dispatch_to_response_complete_body_read"
			}
			s.Metrics[name] = metric
		}
	}
	if capture.Synthetic {
		s.Claims["protocol_delivery"] = claim(Pass, "complete synthetic response-body capture and declared synthetic clock metadata validated; conditional nearest-rank latency includes ERROR/UNKNOWN receipts; missing delivery is reported separately and does not invalidate observed p99")
	} else {
		s.Claims["protocol_delivery"] = claim(Pass, "complete live response-body capture and declared live clock metadata validated; conditional nearest-rank latency includes ERROR/UNKNOWN receipts; missing delivery is reported separately and does not invalidate observed p99")
	}
}

func manifestSynthetic(m map[string]json.RawMessage) bool {
	var v bool
	return json.Unmarshal(m["synthetic"], &v) == nil && v
}
