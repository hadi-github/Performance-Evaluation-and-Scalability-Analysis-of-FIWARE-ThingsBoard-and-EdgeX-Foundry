package audit

import (
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func baseRun(ticks int64, tickRate float64, devices int) (*plan.Resolved, *report.Summary) {
	r := &plan.Resolved{
		Plan: plan.Plan{
			Devices:  devices,
			Arrivals: plan.Arrivals{Model: "constant"},
		},
		RunID:     "R1",
		DurationS: 1,
		Steps: []plan.Step{{
			Index: 0, Name: "steady", Phase: "steady", Rate: tickRate,
			TickRate: tickRate, DurationS: 1, Ticks: ticks, StartS: 0,
		}},
	}
	sum := &report.Summary{Steps: []report.StepSummary{{ScenarioStartMs: 1_000_000}}}
	return r, sum
}

func TestAnalyseClassifiesMissingDuplicateAmbiguousDelayed(t *testing.T) {
	r, sum := baseRun(5, 10, 2)
	records := []report.Record{{Seq: 4, Outcome: "explicit_failure"}}
	sink := []SinkRecord{
		{RunID: "R1", Seq: 0, RecvMs: 1_000_050},
		{RunID: "R1", Seq: 1, RecvMs: 1_000_150},
		{RunID: "R1", Seq: 1, RecvMs: 1_000_160, Dup: true},
		{RunID: "R1", Seq: 3, RecvMs: 1_000_350},
		{RunID: "R1", Seq: 4, RecvMs: 1_000_450},
		{RunID: "R1", Seq: 9, RecvMs: 1_000_900}, // orphan
		{RunID: "OTHER", Seq: 0, RecvMs: 1_000_000},
	}
	history := []HistoryRecord{
		{RunID: "R1", Seq: 0},
		{RunID: "R1", Seq: 0}, // duplicate row
		{RunID: "R1", Seq: 1},
		{RunID: "R1", Seq: 2},
		// seq 3 missing from history
	}
	got := Analyse(r, sum, records, sink, history, true, Options{DeadlineMs: 500})

	if got.AckSuccess != 4 || got.AckFailure != 1 || got.Planned != 5 {
		t.Fatalf("ack accounting: %+v", got)
	}
	if got.Sink.Missing != 1 || len(got.Sink.MissingSeqs) != 1 || got.Sink.MissingSeqs[0] != 2 {
		t.Fatalf("sink missing: %+v", got.Sink)
	}
	if got.Sink.DuplicateCopies != 1 || got.Sink.DuplicateSeqs != 1 {
		t.Fatalf("sink duplicates: %+v", got.Sink)
	}
	if got.Sink.Orphans != 1 || got.Sink.OtherRunIDs != 1 {
		t.Fatalf("sink orphans/other: %+v", got.Sink)
	}
	if len(got.Sink.Ambiguous) != 1 || got.Sink.Ambiguous[0] != 4 {
		t.Fatalf("ambiguous delivered: %+v", got.Sink)
	}
	if got.History.Missing != 1 || got.History.MissingSeqs[0] != 3 {
		t.Fatalf("history missing: %+v", got.History)
	}
	if got.History.DuplicateCopies != 1 || got.History.DuplicateSeqs != 1 {
		t.Fatalf("history duplicates: %+v", got.History)
	}
	if got.Qualified == nil || got.Qualified.Identities != 2 || got.Qualified.WithinDeadline != 2 {
		// seq 0 and 1 are acknowledged, at the sink and in history: latencies 50 and 150 ms.
		t.Fatalf("qualified: %+v", got.Qualified)
	}
	if got.Qualified.P50Ms != 50 || got.Qualified.P99Ms != 50 {
		t.Fatalf("latency percentiles: %+v", got.Qualified)
	}
	if got.Clean {
		t.Fatal("run with missing and duplicate identities must not be clean")
	}
}

func TestCorrectnessQualifiedDeadlineFilter(t *testing.T) {
	r, sum := baseRun(3, 10, 1)
	sink := []SinkRecord{
		{RunID: "R1", Seq: 0, RecvMs: 1_000_050},
		{RunID: "R1", Seq: 1, RecvMs: 1_000_150},
		{RunID: "R1", Seq: 2, RecvMs: 1_000_600},
	}
	history := []HistoryRecord{{RunID: "R1", Seq: 0}, {RunID: "R1", Seq: 1}, {RunID: "R1", Seq: 2}}
	got := Analyse(r, sum, nil, sink, history, true, Options{DeadlineMs: 100})
	if got.Qualified == nil || got.Qualified.Identities != 3 || got.Qualified.WithinDeadline != 2 {
		t.Fatalf("deadline filter: %+v", got.Qualified)
	}
	if got.Qualified.WindowMs != 1000 {
		t.Fatalf("window: %+v", got.Qualified)
	}
}

func TestCollisionRiskAndStoredEvidence(t *testing.T) {
	// 3,000 ticks/s means 0.333 ms between messages: three consecutive messages of a device share a millisecond.
	r, sum := baseRun(4, 3000, 1)
	sink := []SinkRecord{
		{RunID: "R1", Seq: 0, RecvMs: 1_000_001},
		{RunID: "R1", Seq: 1, RecvMs: 1_000_002},
		{RunID: "R1", Seq: 2, RecvMs: 1_000_003},
		{RunID: "R1", Seq: 3, RecvMs: 1_000_004},
	}
	history := []HistoryRecord{
		{RunID: "R1", Seq: 0, Device: "d0", TsMs: 1000, HasTs: true},
		{RunID: "R1", Seq: 1, Device: "d0", TsMs: 1001, HasTs: true},
		{RunID: "R1", Seq: 2, Device: "d0", TsMs: 1000, HasTs: true},
		{RunID: "R1", Seq: 3, Device: "d0", TsMs: 1002, HasTs: true},
	}
	got := Analyse(r, sum, nil, sink, history, true, Options{DeadlineMs: 500})
	if got.Collisions == nil || !got.Collisions.Available {
		t.Fatalf("collision block missing: %+v", got.Collisions)
	}
	if got.Collisions.PlannedSameMsPairs != 2 {
		t.Fatalf("planned same-ms pairs: %+v", got.Collisions)
	}
	if got.Collisions.StoredSameTsGroups != 1 {
		t.Fatalf("stored same-ts groups: %+v", got.Collisions)
	}
}

func TestNoHistoryIsALimitationNotAnInventedResult(t *testing.T) {
	r, sum := baseRun(2, 10, 1)
	sink := []SinkRecord{{RunID: "R1", Seq: 0, RecvMs: 1_000_010}, {RunID: "R1", Seq: 1, RecvMs: 1_000_020}}
	got := Analyse(r, sum, nil, sink, nil, false, Options{DeadlineMs: 500})
	if got.History.Available {
		t.Fatal("history must be unavailable when no source is supplied")
	}
	if got.Qualified != nil {
		t.Fatal("correctness-qualified set must be absent without history")
	}
	found := false
	for _, l := range got.Limitations {
		if strings.Contains(l, "history not supplied") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing history limitation: %v", got.Limitations)
	}
}

func TestDroppedIterationsMakeAckSetALowerBound(t *testing.T) {
	r, sum := baseRun(2, 10, 1)
	sum.DroppedTotal = 3
	got := Analyse(r, sum, nil, nil, nil, false, Options{})
	found := false
	for _, l := range got.Limitations {
		if strings.Contains(l, "lower bound") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing dropped-iteration limitation: %v", got.Limitations)
	}
}

func TestEmptyHistoryExportStaysDistinguishable(t *testing.T) {
	r, sum := baseRun(1, 10, 1)
	got := Analyse(r, sum, nil, []SinkRecord{{RunID: "R1", Seq: 0, RecvMs: 1_000_000}}, []HistoryRecord{}, true, Options{})
	if !got.History.Available {
		t.Fatal("an empty export is still an available source")
	}
	if got.History.Missing != 1 {
		t.Fatalf("acked identity must be missing from an empty history: %+v", got.History)
	}
}

func TestByStepSeparatesRatesAndExcludesWarmUpFromAggregate(t *testing.T) {
	r := &plan.Resolved{
		Plan:      plan.Plan{Devices: 1, Arrivals: plan.Arrivals{Model: "constant"}},
		RunID:     "R1",
		DurationS: 2,
		Steps: []plan.Step{
			{Index: 0, Name: "warm", Phase: "warmup", Rate: 10, TickRate: 10, DurationS: 1, Ticks: 2, StartS: 0, SeqOffset: 0},
			{Index: 1, Name: "steady", Phase: "steady", Rate: 20, TickRate: 20, DurationS: 1, Ticks: 2, StartS: 1, SeqOffset: 2},
		},
	}
	sum := &report.Summary{Steps: []report.StepSummary{
		{ScenarioStartMs: 1_000_000},
		{ScenarioStartMs: 1_001_000},
	}}
	sink := []SinkRecord{
		{RunID: "R1", Seq: 0, RecvMs: 1_000_050}, // warm-up, latency 50
		{RunID: "R1", Seq: 1, RecvMs: 1_000_150}, // warm-up, latency 50
		{RunID: "R1", Seq: 2, RecvMs: 1_001_700}, // steady, latency 700: outside a 500 ms deadline
		// seq 3 never reaches the sink
	}
	history := []HistoryRecord{{RunID: "R1", Seq: 0}, {RunID: "R1", Seq: 1}, {RunID: "R1", Seq: 2}, {RunID: "R1", Seq: 3}}
	got := Analyse(r, sum, nil, sink, history, true, Options{DeadlineMs: 500})

	if got.Qualified == nil || len(got.Qualified.ByStep) != 2 {
		t.Fatalf("by_step missing: %+v", got.Qualified)
	}
	warm, steady := got.Qualified.ByStep[0], got.Qualified.ByStep[1]
	if warm.Name != "warm" || warm.Planned != 2 || warm.Identities != 2 || warm.WithinDeadline != 2 || warm.P50Ms != 50 {
		t.Fatalf("warm-up step: %+v", warm)
	}
	if steady.Name != "steady" || steady.Rate != 20 || steady.Planned != 2 || steady.Identities != 1 ||
		steady.WithinDeadline != 0 || steady.P50Ms != 700 {
		t.Fatalf("steady step: %+v", steady)
	}
	if got.Qualified.Identities != 1 || got.Qualified.WithinDeadline != 0 {
		t.Fatalf("aggregate must cover only the measured (non-warm-up) window: %+v", got.Qualified)
	}
	mixed := false
	for _, l := range got.Limitations {
		if strings.Contains(l, "different offered rates") {
			mixed = true
		}
	}
	if mixed {
		t.Fatalf("one measured rate must not raise the mixed-rate limitation: %v", got.Limitations)
	}
}

func TestNoHistoryIsNeverClean(t *testing.T) {
	r, sum := baseRun(2, 10, 1)
	sink := []SinkRecord{{RunID: "R1", Seq: 0, RecvMs: 1_000_010}, {RunID: "R1", Seq: 1, RecvMs: 1_000_020}}
	got := Analyse(r, sum, nil, sink, nil, false, Options{DeadlineMs: 500})
	if got.Sink.Missing != 0 || got.Clean {
		t.Fatalf("a complete sink with no history export must not be reported clean: %+v", got)
	}
	if got.Labels["missing"] == "" || got.Labels["ambiguous"] == "" {
		t.Fatal("missing and ambiguous labels must be present")
	}
}

func TestMixedMeasuredRatesRaiseLimitation(t *testing.T) {
	r := &plan.Resolved{
		Plan:      plan.Plan{Devices: 1, Arrivals: plan.Arrivals{Model: "constant"}},
		RunID:     "R1",
		DurationS: 2,
		Steps: []plan.Step{
			{Index: 0, Name: "a", Phase: "sweep", Rate: 10, TickRate: 10, DurationS: 1, Ticks: 2, StartS: 0, SeqOffset: 0},
			{Index: 1, Name: "b", Phase: "sweep", Rate: 20, TickRate: 20, DurationS: 1, Ticks: 2, StartS: 1, SeqOffset: 2},
		},
	}
	sum := &report.Summary{Steps: []report.StepSummary{{ScenarioStartMs: 1_000_000}, {ScenarioStartMs: 1_001_000}}}
	got := Analyse(r, sum, nil, nil, []HistoryRecord{}, true, Options{DeadlineMs: 500})
	found := false
	for _, l := range got.Limitations {
		if strings.Contains(l, "different offered rates") {
			found = true
		}
	}
	if !found {
		t.Fatalf("two measured rates must raise the mixed-rate limitation: %v", got.Limitations)
	}
	if len(got.Qualified.ByStep) != 2 {
		t.Fatalf("per-step rows are the reporting unit: %+v", got.Qualified)
	}
}

func TestUnknownOutcomeStaysAmbiguousAndUnqualified(t *testing.T) {
	r, sum := baseRun(3, 10, 1)
	records := []report.Record{{Seq: 1, Outcome: "unknown"}}
	sink := []SinkRecord{{RunID: "R1", Seq: 0, RecvMs: 1_000_010}, {RunID: "R1", Seq: 1, RecvMs: 1_000_020}}
	history := []HistoryRecord{{RunID: "R1", Seq: 0}, {RunID: "R1", Seq: 1}, {RunID: "R1", Seq: 2}}
	got := Analyse(r, sum, records, sink, history, true, Options{DeadlineMs: 500})
	if got.Unknown != 1 || got.AckSuccess != 2 {
		t.Fatalf("unknown outcome must not count as acknowledged: %+v", got)
	}
	if len(got.Sink.Ambiguous) != 1 || got.Sink.Ambiguous[0] != 1 {
		t.Fatalf("a delivered unknown outcome must be listed as ambiguous: %+v", got.Sink)
	}
	if got.Qualified.Identities != 1 {
		t.Fatalf("an unknown outcome must not enter the qualified set: %+v", got.Qualified)
	}
	if got.Clean {
		t.Fatal("an unreconciled unknown outcome must not be reported clean")
	}
}

func TestDuplicatesCannotInflateGoodput(t *testing.T) {
	r, sum := baseRun(2, 10, 1)
	plain := Analyse(r, sum,
		nil,
		[]SinkRecord{{RunID: "R1", Seq: 0, RecvMs: 1_000_010}, {RunID: "R1", Seq: 1, RecvMs: 1_000_020}},
		[]HistoryRecord{{RunID: "R1", Seq: 0}, {RunID: "R1", Seq: 1}}, true, Options{DeadlineMs: 500})
	dup := Analyse(r, sum,
		nil,
		[]SinkRecord{{RunID: "R1", Seq: 0, RecvMs: 1_000_010}, {RunID: "R1", Seq: 0, RecvMs: 1_000_011, Dup: true},
			{RunID: "R1", Seq: 0, RecvMs: 1_000_012, Dup: true}, {RunID: "R1", Seq: 1, RecvMs: 1_000_020}},
		[]HistoryRecord{{RunID: "R1", Seq: 0}, {RunID: "R1", Seq: 0}, {RunID: "R1", Seq: 1}}, true, Options{DeadlineMs: 500})
	if dup.Qualified.Identities != plain.Qualified.Identities || dup.Qualified.WithinDeadline != plain.Qualified.WithinDeadline {
		t.Fatalf("duplicate copies changed goodput: plain %+v dup %+v", plain.Qualified, dup.Qualified)
	}
	if dup.Sink.DuplicateCopies != 2 || dup.History.DuplicateCopies != 1 || dup.Clean {
		t.Fatalf("duplicates must still be counted and make the run not clean: %+v", dup)
	}
}
