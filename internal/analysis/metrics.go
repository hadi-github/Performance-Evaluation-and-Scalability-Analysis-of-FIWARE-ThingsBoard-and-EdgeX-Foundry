package analysis

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"iotbench/internal/arrivals"
	"iotbench/internal/audit"
	"iotbench/internal/cpubound"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func put(s *Step, name string, v float64, unit, dependency, basis string) {
	var value *float64
	if finite(v) {
		value = &v
	}
	m := Metric{Value: value, Unit: unit, Claim: dependency, Basis: basis}
	switch dependency {
	case "ack_latency":
		m.Endpoint = "scheduled_arrival_to_http_response"
		m.ClockDomain = "generator realtime epoch milliseconds"
	case "http_duration":
		m.Endpoint = "actual_http_send_to_response"
		m.ClockDomain = "k6 native per-request duration"
	case "delivery_latency":
		m.Endpoint = "scheduled_arrival_to_first_sink_receipt"
		m.ClockDomain = "explicit shared load-host realtime attestation required"
	case "history":
		m.Endpoint = "identity_retrievable_in_history_snapshot"
		m.ClockDomain = "snapshot collection time; store ts is not commit/visibility time"
	case "sink":
		m.Endpoint = "sink_receipt_by_declared_deadline"
		m.ClockDomain = "sink realtime epoch milliseconds"
	case "cpu_cost":
		m.Endpoint = "nominal_phase_cgroup_counter_delta"
		m.ClockDomain = "counter endpoints require explicit phase clock alignment"
	}
	s.Metrics[name] = m
}

func addBaseMetrics(s *Step, st plan.Step, ss report.StepSummary, r *plan.Resolved) {
	for name, v := range map[string]float64{"workload.devices": float64(r.Devices), "workload.offered_rate": st.Rate, "workload.measurement_seconds": float64(st.DurationS), "outcomes.scheduled": float64(report.PlannedSends(r, st)), "outcomes.started": ss.Attempted, "outcomes.acknowledged": ss.AckSuccess, "outcomes.explicit_failure": ss.ExplicitFailure, "outcomes.transport_unknown": ss.Unknown, "outcomes.dropped_iterations": ss.Dropped, "outcomes.early": ss.Early} {
		unit := "messages"
		switch name {
		case "workload.devices":
			unit = "devices"
		case "workload.offered_rate":
			unit = "messages/s"
		case "workload.measurement_seconds":
			unit = "s"
		}
		put(s, name, v, unit, "offered_load", "resolved plan and exact per-step k6 counters")
	}
	put(s, "outcomes.achieved_rate", (ss.Attempted-ss.DupInjected)/float64(st.DurationS), "messages/s", "offered_load", "attempted sends / nominal measurement duration; not observed throughput")
	if ss.Attempted > 0 {
		put(s, "outcomes.unsuccessful_ratio", (ss.ExplicitFailure+ss.Unknown)/ss.Attempted, "ratio", "offered_load", "(explicit failures + transport unknown) / attempted")
	}
	for name, v := range map[string]float64{"lag.p50_ms": ss.SendLag.P50, "lag.p99_ms": ss.SendLag.P99, "lag.max_ms": ss.SendLag.Max} {
		if ss.SendLag.Count > 0 {
			put(s, name, v, "ms", "offered_load", "per-step k6 Trend; not merged from minute p99s")
		}
	}
	s.Claims["ack_latency"] = dependent(s.Claims["offered_load"])
	if s.Claims["offered_load"].Status == Pass {
		if ss.AckLatency.Count > 0 {
			s.Claims["ack_latency"] = claim(Pass, "exact per-step k6 Trend of successful scheduled-to-HTTP-response observations")
		} else {
			s.Claims["ack_latency"] = claim(NotAssessed, "no acknowledged latency samples")
		}
	}
	if ss.AckLatency.Count > 0 {
		put(s, "latency.ack.p50_ms", ss.AckLatency.P50, "ms", "ack_latency", "per-step successful acknowledgements only; scheduled arrival → HTTP response")
		put(s, "latency.ack.p99_ms", ss.AckLatency.P99, "ms", "ack_latency", "per-step successful acknowledgements only; never durable storage latency")
	}
	s.Claims["http_duration"] = claim(NotAssessed, "complete actual-send-to-response distribution is not in the exception ledger or summary schema")
	s.Claims["durable_latency"] = claim(NotAssessed, "no per-message durable-commit timestamps")
	s.Claims["service_slo"] = claim(NotAssessed, "no adopted complete service-SLO procedure/configuration supplied; exploratory rules are not substituted")
}

func dependent(load Claim) Claim {
	if load.Status == Pass {
		return claim(NotAssessed, "required evidence not established")
	}
	if load.Status == Invalid {
		return claim(Invalid, "offered-load prerequisite failed")
	}
	return claim(NotAssessed, "offered-load prerequisite unavailable")
}

func sourceStatus(s *Step, src Source, deadline float64, readErr error) Claim {
	if s.Claims["offered_load"].Status != Pass {
		return dependent(s.Claims["offered_load"])
	}
	if readErr != nil {
		return claim(NotAssessed, "observation file unavailable; not treated as empty")
	}
	if !src.Complete || src.Source == "" {
		return claim(NotAssessed, "capture completeness or source is not attested")
	}
	if deadline <= 0 || !finite(deadline) || deadline < s.EndMS {
		return claim(NotAssessed, "observation/drain deadline missing or before measured phase end")
	}
	if !finite(src.StartMS) || src.StartMS <= 0 || !finite(src.EndMS) || src.StartMS > s.StartMS || src.EndMS < deadline || src.EndMS < src.StartMS {
		return claim(NotAssessed, "capture does not cover the declared observation window")
	}
	return claim(Pass, "complete identity capture declared through observation deadline; not proof of permanent loss or durable commit")
}

func analyseObservations(s *Step, rd reader, ev Evidence, r *plan.Resolved, sum *report.Summary, recs []report.Record, planned []audit.Planned, rule *BacklogRule) {
	sb, se := rd.read("sink", ev.Sink.Path)
	hb, he := rd.read("history", ev.History.Path)
	s.Claims["sink"] = sourceStatus(s, ev.Sink, ev.ObservationDeadlineMS, se)
	s.Claims["history"] = sourceStatus(s, ev.History, ev.ObservationDeadlineMS, he)
	for _, point := range []struct {
		name   string
		source Source
	}{{"sink", ev.Sink}, {"history", ev.History}} {
		if point.source.Diagnostics == "" {
			continue
		}
		b, e := rd.read(point.name+"_diagnostics", point.source.Diagnostics)
		if e != nil {
			s.Claims[point.name] = claim(NotAssessed, "declared capture diagnostics unavailable")
			continue
		}
		pattern := `(?m)^sink_(overflow|unparsed|recording_errors)_total\s+([0-9.]+)`
		if point.name == "history" {
			pattern = `(unrecoverable)=([0-9]+)`
		}
		for _, match := range regexp.MustCompile(pattern).FindAllStringSubmatch(string(b), -1) {
			n, _ := strconv.ParseFloat(match[2], 64)
			if n > 0 {
				if point.source.DiagnosticsRunLocal {
					s.Claims[point.name] = claim(Invalid, "run-local capture diagnostics report "+match[1]+"; observation completeness cannot establish a platform failure")
				} else {
					s.Claims[point.name] = claim(NotAssessed, "nonzero cumulative diagnostics without a run-local baseline; completeness unestablished")
				}
			}
		}
	}
	resolve := func(p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(rd.base, p)
		}
		return canonicalPath(p)
	}
	if ev.Sink.Path != "" && ev.History.Path != "" && resolve(ev.Sink.Path) == resolve(ev.History.Path) {
		s.Claims["sink"] = claim(Invalid, "sink and history reference the same input; independent observation not established")
		s.Claims["history"] = s.Claims["sink"]
	}
	type observation struct {
		RunID  string   `json:"run_id"`
		Seq    *int64   `json:"seq"`
		Recv   *float64 `json:"recv_ms"`
		Device string   `json:"device"`
		TS     *float64 `json:"ts"`
	}
	srows, sparse := rows[observation](sb)
	hrows, hparse := rows[observation](hb)
	if se == nil && sparse != nil {
		s.Claims["sink"] = claim(Invalid, "malformed sink NDJSON: "+sparse.Error())
	}
	if he == nil && hparse != nil {
		s.Claims["history"] = claim(Invalid, "malformed history NDJSON: "+hparse.Error())
	}
	var sinks []audit.SinkRecord
	var histories []audit.HistoryRecord
	malSink, malHist, late := 0, 0, 0
	cohort := map[int64]bool{}
	allPlanned := map[int64]bool{}
	for _, p := range planned {
		allPlanned[p.Seq] = true
		if p.Step == s.Name {
			cohort[p.Seq] = true
		}
	}
	for _, x := range srows {
		if x.RunID == "" || x.Seq == nil || x.Recv == nil || !finite(*x.Recv) {
			malSink++
			continue
		}
		if x.RunID == r.RunID && allPlanned[*x.Seq] && !cohort[*x.Seq] {
			continue
		}
		if ev.ObservationDeadlineMS > 0 && *x.Recv > ev.ObservationDeadlineMS {
			if x.RunID == r.RunID {
				late++
			}
			continue
		}
		sinks = append(sinks, audit.SinkRecord{RunID: x.RunID, Seq: *x.Seq, RecvMs: *x.Recv})
	}
	for _, x := range hrows {
		if x.RunID == "" || x.Seq == nil {
			malHist++
			continue
		}
		if x.RunID == r.RunID && allPlanned[*x.Seq] && !cohort[*x.Seq] {
			continue
		}
		h := audit.HistoryRecord{RunID: x.RunID, Seq: *x.Seq, Device: x.Device}
		if x.TS != nil {
			if !finite(*x.TS) {
				malHist++
				continue
			}
			h.HasTs = true
			h.TsMs = *x.TS
		}
		histories = append(histories, h)
	}
	if malSink > 0 && se == nil {
		s.Claims["sink"] = claim(Invalid, "malformed/missing sink identities or receipt timestamps; no sequence-only repair")
	}
	if malHist > 0 && he == nil {
		s.Claims["history"] = claim(Invalid, "malformed/missing history identities; no sequence-only repair")
	}
	if s.Claims["history"].Status == Pass && (ev.History.CollectedAtMS < ev.ObservationDeadlineMS || !finite(ev.History.CollectedAtMS)) {
		s.Claims["history"] = claim(NotAssessed, "history snapshot collection time missing or precedes deadline")
	}
	if se == nil {
		put(s, "sink.malformed_rows", float64(malSink), "rows", "sink", "raw capture parsing")
	}
	if he == nil {
		put(s, "history.malformed_rows", float64(malHist), "rows", "history", "raw capture parsing")
	}
	if se == nil && ev.ObservationDeadlineMS > 0 {
		put(s, "sink.late_occurrences", float64(late), "occurrences", "sink", "observations after declared deadline; preserved in original input, excluded from deadline set")
	}
	if s.Claims["offered_load"].Status != Pass {
		s.Claims["delivery_latency"] = dependent(s.Claims["offered_load"])
		analyseBacklog(s, rd, ev)
		return
	}
	// Reuse audit on exactly one measured cohort, never mix warm-up or sweep rates.
	rc := *r
	sc := *sum
	for _, st := range r.Steps {
		if st.Index == s.Index {
			rc.Steps = []plan.Step{st}
			rc.TotalTicks = st.Ticks
			rc.DurationS = st.DurationS
		}
	}
	for _, ss := range sum.Steps {
		if ss.Index == s.Index {
			sc.Steps = []report.StepSummary{ss}
		}
	}
	a := audit.Analyse(&rc, &sc, recs, sinks, histories, he == nil, audit.Options{MaxExamples: 20})
	for _, point := range []struct {
		name string
		set  audit.SetSummary
	}{{"sink", a.Sink}, {"history", a.History}} {
		if (point.name == "sink" && se != nil) || (point.name == "history" && he != nil) {
			continue
		}
		for name, v := range map[string]int{"unique": point.set.Unique, "missing_acknowledged": point.set.Missing, "duplicate_identities": point.set.DuplicateSeqs, "duplicate_extra_copies": point.set.DuplicateCopies, "unexpected_identities": point.set.Orphans, "other_run_occurrences": point.set.OtherRunIDs} {
			put(s, point.name+"."+name, float64(v), "messages", point.name, "identity join on (run_id, seq), selected scheduled cohort; absence at declared observation is not demonstrated permanent loss")
		}
		if s.Claims[point.name].Status == Pass {
			if point.set.Missing > 0 || point.set.DuplicateCopies > 0 || point.set.Orphans > 0 {
				s.Claims[point.name] = claim(Fail, "complete capture shows missing acknowledged, duplicate or unexpected cohort identities at the stated observation point")
			} else if a.AckSuccess == 0 {
				s.Claims[point.name] = claim(NotAssessed, "no acknowledged identities to establish successful delivery/persistence correctness")
			}
		}
	}
	if he == nil {
		put(s, "history.acknowledged_found", float64(a.AckSuccess-int64(a.History.Missing)), "messages", "history", "acknowledged identities independently found in the selected history snapshot")
		devices := map[string]bool{}
		allNamed := true
		for _, h := range histories {
			if h.RunID == r.RunID && cohort[h.Seq] {
				if h.Device == "" {
					allNamed = false
				} else {
					devices[h.Device] = true
				}
			}
		}
		if allNamed {
			put(s, "workload.observed_history_devices", float64(len(devices)), "devices", "history", "distinct device fields in independently retrieved selected-cohort history; configured devices alone do not establish exercised devices")
		}
	}
	for _, point := range []string{"sink", "history"} {
		observed := map[int64]bool{}
		if point == "sink" {
			for _, x := range sinks {
				if x.RunID == r.RunID {
					observed[x.Seq] = true
				}
			}
		} else {
			for _, x := range histories {
				if x.RunID == r.RunID {
					observed[x.Seq] = true
				}
			}
		}
		unknown, failed := 0, 0
		for _, x := range recs {
			if cohort[x.Seq] && observed[x.Seq] {
				if x.Outcome == "unknown" {
					unknown++
				}
				if x.Outcome == "explicit_failure" {
					failed++
				}
			}
		}
		if point == "sink" && se == nil || point == "history" && he == nil {
			put(s, point+".observed_despite_unknown", float64(unknown), "messages", point, "unknown HTTP outcomes are retained, not automatically classified as lost")
			put(s, point+".observed_despite_failure", float64(failed), "messages", point, "HTTP classification and downstream observation are independent")
		}
	}
	// A valid capture can show failure; FAIL is evidence, unlike INVALID/unavailable.
	s.Claims["delivery_latency"] = claim(NotAssessed, "complete acknowledged sink cohort and qualified clock relationship required")
	clockOK := ev.SinkClock == "shared_load_realtime" && ev.ClockSource != ""
	outcome := map[int64]string{}
	for _, x := range recs {
		outcome[x.Seq] = x.Outcome
	}
	first := map[int64]float64{}
	for _, x := range sinks {
		if x.RunID == r.RunID {
			if old, ok := first[x.Seq]; !ok || x.RecvMs < old {
				first[x.Seq] = x.RecvMs
			}
		}
	}
	var lat []float64
	badClock := false
	for _, p := range planned {
		if p.Step != s.Name {
			continue
		}
		if o := outcome[p.Seq]; o != "" && o != "ack_success" {
			continue
		}
		if end, ok := first[p.Seq]; ok {
			v := end - p.SchedMs
			lat = append(lat, v)
			if v < -r.Health.EarlyToleranceMs {
				badClock = true
			}
		}
	}
	if clockOK && s.Claims["sink"].Status == Pass && len(lat) > 0 {
		s.Claims["delivery_latency"] = claim(Pass, "first receipt minus scheduled arrival on explicitly attested shared load-host realtime; complete acknowledged cohort")
	}
	if badClock {
		s.Claims["delivery_latency"] = claim(Invalid, "receipt precedes scheduled send beyond adopted early tolerance; clocks/timestamps inconsistent")
	}
	if len(lat) > 0 {
		sort.Float64s(lat)
		put(s, "latency.delivery.p50_ms", arrivals.Quantile(lat, .5), "ms", "delivery_latency", "first receipt − scheduled; acknowledged observed cohort; unavailable as full-cohort percentile when censored")
		put(s, "latency.delivery.p99_ms", arrivals.Quantile(lat, .99), "ms", "delivery_latency", "raw first-receipt observations, existing arrivals.Quantile estimator")
	}
	put(s, "latency.delivery.observed_count", float64(len(lat)), "messages", "sink", "only acknowledged observations at/before deadline")
	analyseBacklog(s, rd, ev)
	evaluateGrowth(s, rd, ev, rule, planned, outcome, first, clockOK)
}

func analyseCPU(s *Step, rd reader, ev Evidence) {
	s.Claims["cpu_cost"] = dependent(s.Claims["offered_load"])
	s.Claims["cpu_capture_interval"] = dependent(s.Claims["offered_load"])
	for _, in := range ev.CPU {
		if in.Step != s.Index {
			continue
		}
		bb, be := rd.read(fmt.Sprintf("cpu_before_%d", s.Index), in.Before)
		ab, ae := rd.read(fmt.Sprintf("cpu_after_%d", s.Index), in.After)
		if s.Claims["offered_load"].Status != Pass {
			return
		}
		if be != nil || ae != nil {
			s.Claims["cpu_cost"] = claim(NotAssessed, "CPU endpoint file unavailable")
			s.Claims["cpu_capture_interval"] = s.Claims["cpu_cost"]
			return
		}
		before, bErr := cpubound.Parse(bytes.NewReader(bb))
		after, aErr := cpubound.Parse(bytes.NewReader(ab))
		if bErr != nil || aErr != nil {
			s.Claims["cpu_cost"] = claim(Invalid, "invalid CPU snapshot: "+fmt.Sprint(bErr, aErr))
			s.Claims["cpu_capture_interval"] = s.Claims["cpu_cost"]
			return
		}
		if !sameMembership(before, after, in.ExpectedContainers) {
			s.Claims["cpu_cost"] = claim(Invalid, "CPU declared membership absent, duplicated or changed")
			s.Claims["cpu_capture_interval"] = s.Claims["cpu_cost"]
			return
		}
		denom := cpubound.Denominator{Source: "offline claim analysis", Definition: in.Denominator + " messages in selected nominal measured cohort"}
		key := map[string]string{"offered": "outcomes.scheduled", "acknowledged": "outcomes.acknowledged", "persisted_acknowledged": "history.acknowledged_found"}[in.Denominator]
		m, ok := s.Metrics[key]
		if ok && m.Value != nil && s.Claims[m.Claim].Status == Pass && *m.Value > 0 {
			denom.Count = int64(*m.Value)
			denom.Present = true
		}
		if s.Claims["history"].Status == Pass {
			if missing := s.Metrics["history.missing_acknowledged"].Value; missing != nil {
				ack := s.Metrics["outcomes.acknowledged"].Value
				if ack != nil {
					v := *ack - *missing
					put(s, "history.acknowledged_found", v, "messages", "history", "acknowledged identities independently found in history")
					if in.Denominator == "persisted_acknowledged" && v > 0 {
						denom.Count = int64(v)
						denom.Present = true
					}
				}
			}
		}
		cpu := cpubound.Diff(before, after, &denom, "Actual endpoint interval; phase-specific cost requires matching phase times and clock domain")
		s.CPU = cpu
		if !cpu.AllOK {
			s.Claims["cpu_cost"] = claim(Invalid, "missing/restarted/replaced/reset CPU counters")
			s.Claims["cpu_capture_interval"] = s.Claims["cpu_cost"]
			return
		}
		s.Claims["cpu_capture_interval"] = claim(Pass, "counter delta is available for each recorded container interval, not automatically the nominal phase")
		exact := in.Clock == "shared_load_realtime" && in.ClockSource != ""
		for _, b := range before {
			bt := number(b.EpochS)
			at := 0.0
			for _, a := range after {
				if a.Name == b.Name {
					at = number(a.EpochS)
				}
			}
			if math.Abs(bt*1000-s.StartMS) > .001 || math.Abs(at*1000-s.EndMS) > .001 {
				exact = false
			}
		}
		if !exact {
			cpu.TotalCPUSecondsPerMessage = nil
			s.Claims["cpu_cost"] = claim(NotAssessed, "only enclosing/misaligned counter intervals or unqualified clock domain; exact phase-specific CPU cost unavailable")
			return
		}
		if !denom.Present {
			s.Claims["cpu_cost"] = claim(NotAssessed, "explicit denominator unavailable, unsupported, invalid or zero")
			return
		}
		s.Claims["cpu_cost"] = claim(Pass, "matching phase counters, declared membership and clock domain; explicitly named denominator")
		put(s, "cpu.phase_seconds", *cpu.TotalCPUSeconds, "CPU-s", "cpu_cost", "sum of validated per-container counter deltas")
		put(s, "cpu.seconds_per_message", *cpu.TotalCPUSecondsPerMessage, "CPU-s/message", "cpu_cost", denom.Definition)
		return
	}
}

func sameMembership(b, a []cpubound.SnapshotRecord, names []string) bool {
	if len(names) == 0 || len(b) != len(names) || len(a) != len(names) {
		return false
	}
	want := map[string]bool{}
	for _, name := range names {
		if name == "" || want[name] {
			return false
		}
		want[name] = true
	}
	for _, set := range [][]cpubound.SnapshotRecord{b, a} {
		for _, r := range set {
			if !want[r.Name] {
				return false
			}
		}
	}
	for _, before := range b {
		for _, after := range a {
			if before.Name == after.Name && (before.Service != after.Service || before.Tier != after.Tier || before.Cpuset != after.Cpuset) {
				return false
			}
		}
	}
	return true
}

func number(s string) float64 { var n float64; _, _ = fmt.Sscan(s, &n); return n }

// Backlog summaries are diagnostic, not the preregistered calibrated 95%-CI test.
// No calibrated eps_W / uncertainty rule is invented here.
func analyseBacklog(s *Step, rd reader, ev Evidence) {
	s.Claims["backlog"] = claim(NotAssessed, "calibrated sustained-growth procedure/uncertainty unavailable; flat queue is not processing or storage proof")
	if ev.Backlog.Path == "" {
		return
	}
	b, e := rd.read("backlog", ev.Backlog.Path)
	if e != nil {
		return
	}
	var points []struct {
		MS      float64 `json:"epoch_ms"`
		Records float64 `json:"records"`
	}
	if e = decode(b, &points, true); e != nil {
		s.Claims["backlog"] = claim(Invalid, "malformed backlog evidence")
		return
	}
	if len(points) < 2 {
		return
	}
	max := 0.0
	for i, p := range points {
		if !finite(p.MS) || !finite(p.Records) || p.Records < 0 || (i > 0 && p.MS <= points[i-1].MS) {
			s.Claims["backlog"] = claim(Invalid, "inconsistent backlog samples")
			return
		}
		if p.Records > max {
			max = p.Records
		}
	}
	put(s, "backlog.observed_max_records", max, "records", "backlog", "diagnostic queue samples; does not imply identity correctness or no loss")
	put(s, "backlog.endpoint_change_records", points[len(points)-1].Records-points[0].Records, "records", "backlog", "last sample minus first; not a sustained-growth test")
}
