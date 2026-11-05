package offline

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"iotbench/internal/analysis"
	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/receipt"
	"iotbench/internal/report"
	"iotbench/internal/runner"
)

type offlineFixture struct {
	dir      string
	fixture  *runner.OfflineFixture
	binding  protocol.Binding
	pilot    protocol.PilotParameters
	resolved *plan.Resolved
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, value); err != nil {
		t.Fatal(err)
	}
}

func newOfflineFixture(t *testing.T, runID string, protocolBound bool) *offlineFixture {
	t.Helper()
	root := repoRoot(t)
	var binding *protocol.Binding
	var pilot *protocol.PilotParameters
	planID := "offline-legacy-fixture"
	purpose := "SYNTHETIC OFFLINE ENGINEERING FIXTURE; legacy acknowledgement path; not live evidence"
	if protocolBound {
		planID = "offline-protocol-fixture"
		purpose = "SYNTHETIC OFFLINE ENGINEERING FIXTURE through real plan/manifest path; not live evidence"
		protocolPath := filepath.Join(root, "research", "protocols", "iotbench-telemetry-v0.1.0-pilot.1.json")
		loaded, err := protocol.LoadVerified(protocolPath, root)
		if err != nil {
			t.Fatal(err)
		}
		definition, err := loaded.Definition()
		if err != nil {
			t.Fatal(err)
		}
		rate, measurement, warmup := 1.0, 5, 2
		cutoff := int64(1700000012000)
		b := protocol.Binding{ProtocolID: definition.ProtocolID, ProtocolSHA256: loaded.SHA256(), ExecutionClass: protocol.EngineeringPilot}
		pl := protocol.PilotParameters{RatePerSecond: &rate, MeasurementSeconds: &measurement, WarmupSeconds: &warmup, DrainCutoffUnixMS: &cutoff}
		binding, pilot = &b, &pl
	}
	p := &plan.Plan{
		SchemaVersion: 1, PlanID: planID, Purpose: purpose, PilotOnly: true, Seed: 17, Adapter: "fixture",
		Target: plan.Target{BaseURL: "http://synthetic.invalid"}, Devices: 5, RequestTimeMs: 1000,
		Connection: plan.Connection{KeepAlive: true}, Arrivals: plan.Arrivals{Model: "constant"},
		VUs: plan.VUs{PreAllocated: 5, Max: 5},
		Schedule: plan.Schedule{Type: "steps", Steps: []plan.StepSpec{
			{Phase: "warmup", Rate: 1, DurationS: 2},
			{Phase: "measurement", Rate: 1, DurationS: 5},
		}},
		Health:          plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2},
		Deviations:      []string{"synthetic offline engineering fixture; no generator, Docker, SUT or service was executed"},
		ProtocolBinding: binding, ProtocolPilot: pilot,
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeJSON(t, planPath, p)
	f, err := runner.PrepareOfflineFixture(runner.Options{PlanPath: planPath, RepoRoot: root, ResultsDir: t.TempDir(), RunID: runID})
	if err != nil {
		t.Fatal(err)
	}
	out := &offlineFixture{dir: f.Dir, fixture: f, resolved: f.Resolved}
	if binding != nil {
		out.binding, out.pilot = *binding, *pilot
	}
	writeFixtureArtifacts(t, out, protocolBound)
	if err = runner.Finish(f.Dir, f.Manifest); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeFixtureArtifacts(t *testing.T, f *offlineFixture, protocolBound bool) {
	t.Helper()
	r := f.resolved
	runID := r.RunID
	start := float64(1700000000000)
	measurementStart := start + float64(r.Steps[0].DurationS)*1000
	cutoff := start + 12000
	if protocolBound {
		cutoff = float64(*f.pilot.DrainCutoffUnixMS)
	}
	sum := report.Summary{SchemaVersion: 2, RunID: runID, SetupEndMs: start, TestRunDurationMs: 7000, VUsPeak: 5, VUsMaxAllocated: 5,
		Steps: []report.StepSummary{
			{Index: 0, Name: r.Steps[0].Name, ScenarioStartMs: start, Attempted: 2, AckSuccess: 2, HTTPReqs: 2,
				SendLag: report.TrendStats{Count: 2, P50: 1, P99: 1, Max: 1}, AckLatency: report.TrendStats{Count: 2, P50: 20, P99: 20, Max: 20}},
			{Index: 1, Name: r.Steps[1].Name, ScenarioStartMs: measurementStart, Attempted: 5, AckSuccess: 1, ExplicitFailure: 1, Unknown: 3, HTTPReqs: 5,
				SendLag: report.TrendStats{Count: 5, P50: 1, P99: 1, Max: 1}, AckLatency: report.TrendStats{Count: 1, P50: 100, P99: 100, Max: 100}},
		}}
	writeJSON(t, filepath.Join(f.dir, "k6-summary.json"), sum)

	var records strings.Builder
	for _, entry := range []struct {
		seq     int64
		outcome string
		latency float64
	}{{3, "explicit_failure", 600}, {4, "unknown", 20}, {5, "unknown", 25}, {6, "unknown", 30}} {
		scheduled := measurementStart + float64(entry.seq-2)*1000
		record := report.Record{Rec: "msg", RunID: runID, Seq: entry.seq, Device: int(entry.seq % 5), Step: 1, Phase: "measurement", Tick: entry.seq - 2,
			SchedMs: scheduled, ActualMs: scheduled + 1, Outcome: entry.outcome, Status: 500, RespMs: scheduled + entry.latency}
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		records.Write(b)
		records.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(f.dir, "records.ndjson"), []byte(records.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	var sink, history strings.Builder
	for _, seq := range []int64{2, 3, 4} {
		scheduled := measurementStart + float64(seq-2)*1000
		for name, dst := range map[string]*strings.Builder{"sink.ndjson": &sink, "history.ndjson": &history} {
			row := map[string]any{"run_id": runID, "seq": seq, "recv_ms": scheduled + []float64{100, 600, 20}[seq-2]}
			if name == "history.ndjson" {
				row["device"] = "device-" + string(rune('0'+seq%5))
				row["ts"] = scheduled + 1
			}
			b, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			dst.Write(b)
			dst.WriteByte('\n')
		}
	}
	for name, text := range map[string]string{"sink.ndjson": sink.String(), "history.ndjson": history.String()} {
		if err := os.WriteFile(filepath.Join(f.dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	effective := []byte("{\"synthetic\":true,\"configuration\":\"offline engineering fixture\"}\n")
	if err := os.WriteFile(filepath.Join(f.dir, "effective.json"), effective, 0o644); err != nil {
		t.Fatal(err)
	}
	sumEffective := sha256.Sum256(effective)
	ev := analysis.Evidence{
		SchemaVersion: 1, Synthetic: true, Interpretation: "claim-gates-v1",
		Configuration: analysis.Configuration{ID: "synthetic-offline-fixture", CPUAllocationK: 1,
			Replicas: map[string]int{"application": 1}, Images: map[string]string{"application": "synthetic/application@sha256:" + strings.Repeat("d", 64)},
			EffectiveSHA256: hex.EncodeToString(sumEffective[:]), EffectivePath: "effective.json"},
		ObservationDeadlineMS: cutoff,
		Sink:                  analysis.Source{Path: "sink.ndjson", Complete: true, Source: "synthetic offline fixture capture", StartMS: start, EndMS: cutoff},
		History:               analysis.Source{Path: "history.ndjson", Complete: true, Source: "synthetic offline fixture history", StartMS: start, EndMS: cutoff, CollectedAtMS: cutoff},
		SinkClock:             "shared_load_realtime", ClockSource: "synthetic common epoch-ms clock",
	}
	writeJSON(t, filepath.Join(f.dir, "evidence.json"), ev)

	if !protocolBound {
		return
	}
	capture := analysis.ProtocolReceiptCapture{
		SchemaVersion: receipt.SchemaVersion, MetricInterpretation: analysis.ProtocolMetricInterpretation, Binding: f.binding,
		Synthetic: true, Complete: true, Endpoint: receipt.Endpoint, BodyReadComplete: true,
		ReceiptCaptureSemantics: receipt.CaptureSemantics,
		Source:                  "synthetic:offline engineering fixture body capture", ClockDomain: "synthetic:epoch-ms",
		ClockSource: "synthetic:fixed epoch schedule", CoverageStartMS: measurementStart, CoverageEndMS: cutoff,
		ObservationCutoffMS: cutoff,
		Receipts: []analysis.ProtocolReceipt{
			{RunID: runID, Seq: 0, DeviceID: "device-0", Phase: "warmup", ScheduledSendMS: start, ObservedMS: start + 50, Outcome: protocol.OutcomeOK},
			{RunID: runID, Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: measurementStart, ObservedMS: measurementStart + 100, Outcome: protocol.OutcomeOK},
			{RunID: runID, Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: measurementStart, ObservedMS: measurementStart + 150, Outcome: protocol.OutcomeOK},
			{RunID: runID, Seq: 3, DeviceID: "device-3", Phase: "measurement", ScheduledSendMS: measurementStart + 1000, ObservedMS: measurementStart + 1600, Outcome: protocol.OutcomeError},
			{RunID: runID, Seq: 4, DeviceID: "device-4", Phase: "measurement", ScheduledSendMS: measurementStart + 2000, ObservedMS: measurementStart + 2020, Outcome: protocol.OutcomeUnknown},
		},
	}
	writeJSON(t, filepath.Join(f.dir, "protocol-receipts.json"), capture)
}

func analyzeFixture(t *testing.T, f *offlineFixture) *analysis.Result {
	t.Helper()
	root := repoRoot(t)
	r, err := analysis.Analyze(analysis.Options{
		Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"),
		ProtocolPath: filepath.Join(root, "research", "protocols", "iotbench-telemetry-v0.1.0-pilot.1.json"), ProtocolRoot: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func measurementStep(t *testing.T, r *analysis.Result) analysis.Step {
	t.Helper()
	for _, s := range r.Steps {
		if s.Phase == "measurement" {
			return s
		}
	}
	t.Fatal("measurement step missing")
	return analysis.Step{}
}

func metric(t *testing.T, s analysis.Step, name string) float64 {
	t.Helper()
	m, ok := s.Metrics[name]
	if !ok || m.Value == nil {
		t.Fatalf("metric %s unavailable: %+v", name, m)
	}
	return *m.Value
}

func TestOfflineProtocolPipelinePreservesMainCohortAndProvenance(t *testing.T) {
	f := newOfflineFixture(t, "SYNTHETIC-offline-pipeline", true)
	// Prove the fixture was produced by the real plan -> resolve -> runner manifest path.
	planBytes, err := os.ReadFile(filepath.Join(f.dir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(planBytes)
	var manifest runner.Manifest
	readJSON(t, filepath.Join(f.dir, "manifest.json"), &manifest)
	if manifest.PlanSHA256 != hex.EncodeToString(sum[:]) || manifest.PlanSHA256 != f.fixture.PlanSHA256 {
		t.Fatalf("plan hash flow mismatch: %s %s %x", manifest.PlanSHA256, f.fixture.PlanSHA256, sum)
	}
	if !manifest.Synthetic || !manifest.OfflineFixture || manifest.K6["executed"] != "false" {
		t.Fatalf("fixture not explicitly synthetic/offline: %+v", manifest)
	}
	if manifest.RuntimeMetricInterpretation != analysis.LegacyMetricInterpretation {
		t.Fatalf("runtime interpretation %q", manifest.RuntimeMetricInterpretation)
	}
	if !reflect.DeepEqual(manifest.ProtocolBinding, &f.binding) || !reflect.DeepEqual(manifest.ProtocolPilot, &f.pilot) {
		t.Fatalf("manifest protocol metadata mismatch: %+v %+v", manifest.ProtocolBinding, manifest.ProtocolPilot)
	}
	var resolved plan.Resolved
	readJSON(t, filepath.Join(f.dir, "resolved.json"), &resolved)
	if !reflect.DeepEqual(manifest.Resolved, &resolved) || !reflect.DeepEqual(f.fixture.Resolved, &resolved) {
		t.Fatal("manifest/resolved/fixture plan disagree")
	}
	if manifest.Verdict != "SYNTHETIC_FIXTURE" || manifest.Health == nil || !manifest.Health.Valid {
		t.Fatalf("offline health finish was not exercised or implied live validity: verdict=%s health=%+v", manifest.Verdict, manifest.Health)
	}

	r := analyzeFixture(t, f)
	if r.MetricInterpretation != analysis.ProtocolMetricInterpretation || r.Protocol == nil {
		t.Fatalf("protocol analysis mode missing: %+v", r.Protocol)
	}
	if r.Protocol.ProtocolSHA256 == "" || r.Protocol.ProfileSHA256 == "" || r.Protocol.ProtocolSHA256 != f.binding.ProtocolSHA256 {
		t.Fatalf("protocol provenance mismatch: %+v", r.Protocol)
	}
	s := measurementStep(t, r)
	if s.Claims["protocol_delivery"].Status != analysis.Pass || s.Claims["offered_load"].Status != analysis.Pass {
		t.Fatalf("claims: protocol=%+v offered=%+v", s.Claims["protocol_delivery"], s.Claims["offered_load"])
	}
	for name, want := range map[string]float64{
		"protocol.scheduled_cohort.scheduled": 5, "protocol.scheduled_cohort.observed": 3, "protocol.scheduled_cohort.missing": 2,
		"protocol.scheduled_cohort.duplicate_extra_receipts": 1, "protocol.delivery.p50_ms": 100, "protocol.delivery.p95_ms": 600,
		"protocol.delivery.p99_ms": 600, "protocol.delivery.attainment_250_count": 2, "protocol.delivery.attainment_250_ratio": .4,
		"protocol.delivery.attainment_500_count": 2, "protocol.delivery.attainment_500_ratio": .4,
		"protocol.delivery.attainment_1000_count": 3, "protocol.delivery.attainment_5000_count": 3,
		"protocol.receipts.error": 1, "protocol.receipts.unknown": 1,
	} {
		if got := metric(t, s, name); got != want {
			t.Fatalf("%s=%g want %g", name, got, want)
		}
	}

	// Actual Markdown/JSON outputs, then a CLI-equivalent compare using the emitted JSON.
	out := filepath.Join(t.TempDir(), "analysis-out")
	if err = analysis.Write(out, r, f.dir); err != nil {
		t.Fatal(err)
	}
	md, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{analysis.ProtocolMetricInterpretation, "protocol.scheduled_cohort.scheduled", "protocol.delivery.p99_ms", "600", r.Protocol.ProtocolSHA256, r.Protocol.ProfileSHA256, "Legacy profile assessments"} {
		if !strings.Contains(string(md), needle) {
			t.Fatalf("report.md missing %q", needle)
		}
	}
	var emitted analysis.Result
	readJSON(t, filepath.Join(out, "analysis.json"), &emitted)
	if emitted.MetricInterpretation != r.MetricInterpretation || emitted.Protocol == nil || emitted.Protocol.ProtocolSHA256 != r.Protocol.ProtocolSHA256 {
		t.Fatalf("analysis.json provenance mismatch: %+v", emitted.Protocol)
	}
	emittedStep := measurementStep(t, &emitted)
	for _, name := range []string{"protocol.scheduled_cohort.scheduled", "protocol.scheduled_cohort.observed", "protocol.scheduled_cohort.missing", "protocol.delivery.p99_ms", "protocol.delivery.attainment_500_count"} {
		if metric(t, emittedStep, name) != metric(t, s, name) {
			t.Fatalf("analysis.json %s differs from in-memory result", name)
		}
	}

	comparison := analysis.Compare([]*analysis.Result{&emitted})
	comparisonOut := filepath.Join(t.TempDir(), "comparison-out")
	if err = analysis.WriteComparison(comparisonOut, comparison, []string{out}); err != nil {
		t.Fatal(err)
	}
	records := readCSV(t, filepath.Join(comparisonOut, "comparison.csv"))
	index := headerIndex(t, records[0])
	protocolRow := findCSV(t, records, index, "metric_interpretation", analysis.ProtocolMetricInterpretation)
	for name, want := range map[string]string{"protocol_delivery_status": "PASS", "protocol_scheduled": "5", "protocol_observed": "3", "protocol_missing": "2", "protocol_delivery_p99_ms": "600", "protocol_attainment_500_count": "2"} {
		if got := protocolRow[index[name]]; got != want {
			t.Fatalf("CSV %s=%q want %q", name, got, want)
		}
	}
	if protocolRow[index["protocol_id"]] != f.binding.ProtocolID || protocolRow[index["protocol_sha256"]] != f.binding.ProtocolSHA256 || protocolRow[index["profile_sha256"]] != r.Protocol.ProfileSHA256 {
		t.Fatal("CSV protocol provenance mismatch")
	}

	// Re-analysis must be deterministic and produce identical report bytes.
	again := analyzeFixture(t, f)
	if !reflect.DeepEqual(again, r) {
		t.Fatal("re-analysis result is not deterministic")
	}
	second := filepath.Join(t.TempDir(), "analysis-out-2")
	if err = analysis.Write(second, again, f.dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"analysis.json", "report.md"} {
		a, _ := os.ReadFile(filepath.Join(out, name))
		b, _ := os.ReadFile(filepath.Join(second, name))
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s differs between identical analyses", name)
		}
	}

	// Output path boundary: an offline report may never be written into input evidence.
	forbidden := filepath.Join(f.dir, "forbidden-output")
	if err = analysis.Write(forbidden, r, f.dir); err == nil || !strings.Contains(err.Error(), "outside input evidence") {
		t.Fatalf("output inside run accepted: %v", err)
	}
	if _, statErr := os.Stat(forbidden); !os.IsNotExist(statErr) {
		t.Fatalf("rejected output path was created: %v", statErr)
	}
}

func TestOfflineProtocolPipelineRejectsPlanResolvedManifestMismatch(t *testing.T) {
	newRun := func(t *testing.T) *offlineFixture {
		return newOfflineFixture(t, "SYNTHETIC-offline-pipeline", true)
	}
	t.Run("manifest plan hash", func(t *testing.T) {
		f := newRun(t)
		var m map[string]any
		readJSON(t, filepath.Join(f.dir, "manifest.json"), &m)
		m["plan_sha256"] = strings.Repeat("0", 64)
		writeJSON(t, filepath.Join(f.dir, "manifest.json"), m)
		if _, err := analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "plan_sha256") {
			t.Fatalf("plan hash mismatch accepted: %v", err)
		}
	})
	t.Run("resolved plan differs from plan", func(t *testing.T) {
		f := newRun(t)
		var r plan.Resolved
		readJSON(t, filepath.Join(f.dir, "resolved.json"), &r)
		r.PlanID = "different-plan-id"
		writeJSON(t, filepath.Join(f.dir, "resolved.json"), r)
		if _, err := analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "plan metadata disagree") {
			t.Fatalf("resolved plan mismatch accepted: %v", err)
		}
	})
	t.Run("missing binding", func(t *testing.T) {
		f := newRun(t)
		var m map[string]any
		readJSON(t, filepath.Join(f.dir, "manifest.json"), &m)
		delete(m, "protocol_binding")
		writeJSON(t, filepath.Join(f.dir, "manifest.json"), m)
		if _, err := analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "protocol_binding") {
			t.Fatalf("missing binding accepted: %v", err)
		}
	})
	t.Run("missing pilot", func(t *testing.T) {
		f := newRun(t)
		var m map[string]any
		readJSON(t, filepath.Join(f.dir, "manifest.json"), &m)
		delete(m, "protocol_pilot_parameters")
		writeJSON(t, filepath.Join(f.dir, "manifest.json"), m)
		if _, err := analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "pilot parameters") {
			t.Fatalf("missing pilot accepted: %v", err)
		}
	})
	t.Run("runtime interpretation upgrade", func(t *testing.T) {
		f := newRun(t)
		var m map[string]any
		readJSON(t, filepath.Join(f.dir, "manifest.json"), &m)
		m["runtime_metric_interpretation"] = analysis.ProtocolMetricInterpretation
		writeJSON(t, filepath.Join(f.dir, "manifest.json"), m)
		if _, err := analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "unsupported runtime metric interpretation") {
			t.Fatalf("runtime upgrade accepted: %v", err)
		}
	})
	t.Run("missing runtime interpretation", func(t *testing.T) {
		f := newRun(t)
		var m map[string]any
		readJSON(t, filepath.Join(f.dir, "manifest.json"), &m)
		delete(m, "runtime_metric_interpretation")
		writeJSON(t, filepath.Join(f.dir, "manifest.json"), m)
		if _, err := analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "legacy acknowledgement-only") {
			t.Fatalf("missing runtime interpretation accepted: %v", err)
		}
	})
	t.Run("campaign fails closed after consistent metadata mutation", func(t *testing.T) {
		f := newRun(t)
		var p plan.Plan
		readJSON(t, filepath.Join(f.dir, "plan.json"), &p)
		p.ProtocolBinding.ExecutionClass = protocol.Campaign
		writeJSON(t, filepath.Join(f.dir, "plan.json"), p)
		planBytes, err := os.ReadFile(filepath.Join(f.dir, "plan.json"))
		if err != nil {
			t.Fatal(err)
		}
		planHash := sha256.Sum256(planBytes)
		resolved, err := p.Resolve(p.PlanID + "-campaign")
		if err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(f.dir, "resolved.json"), resolved)
		var m map[string]any
		readJSON(t, filepath.Join(f.dir, "manifest.json"), &m)
		m["run_id"] = resolved.RunID
		m["plan_sha256"] = hex.EncodeToString(planHash[:])
		m["protocol_binding"] = resolved.ProtocolBinding
		m["protocol_pilot_parameters"] = resolved.ProtocolPilot
		m["resolved_plan"] = resolved
		writeJSON(t, filepath.Join(f.dir, "manifest.json"), m)
		if _, err = analysis.Analyze(analysis.Options{Run: f.dir, Evidence: filepath.Join(f.dir, "evidence.json"), ProtocolPath: protocolPath(t), ProtocolRoot: repoRoot(t)}); err == nil || !strings.Contains(err.Error(), "campaign ineligible") {
			t.Fatalf("campaign metadata accepted: %v", err)
		}
	})
}

func TestOfflinePipelineProtocolCaptureStrictness(t *testing.T) {
	newRun := func(t *testing.T) *offlineFixture {
		return newOfflineFixture(t, "SYNTHETIC-offline-capture-strictness", true)
	}
	cases := []struct {
		name   string
		mutate func(*analysis.ProtocolReceiptCapture)
		want   analysis.Status
	}{
		{"wrong endpoint", func(c *analysis.ProtocolReceiptCapture) { c.Endpoint = "legacy_http_ack" }, analysis.NotAssessed},
		{"incomplete body", func(c *analysis.ProtocolReceiptCapture) { c.BodyReadComplete = false }, analysis.NotAssessed},
		{"incomplete capture", func(c *analysis.ProtocolReceiptCapture) { c.Complete = false }, analysis.NotAssessed},
		{"legacy schema", func(c *analysis.ProtocolReceiptCapture) { c.SchemaVersion = 1 }, analysis.NotAssessed},
		{"missing complete-body semantics", func(c *analysis.ProtocolReceiptCapture) { c.ReceiptCaptureSemantics = "" }, analysis.NotAssessed},
		{"wrong complete-body semantics", func(c *analysis.ProtocolReceiptCapture) { c.ReceiptCaptureSemantics = "legacy-arrival-clock-v1" }, analysis.NotAssessed},
		{"wrong capture binding", func(c *analysis.ProtocolReceiptCapture) { c.Binding.ProtocolSHA256 = strings.Repeat("0", 64) }, analysis.Invalid},
		{"not sent outcome", func(c *analysis.ProtocolReceiptCapture) { c.Receipts[1].Outcome = protocol.OutcomeNotSent }, analysis.Invalid},
		{"cutoff mismatch", func(c *analysis.ProtocolReceiptCapture) { c.ObservationCutoffMS++ }, analysis.Invalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRun(t)
			var capture analysis.ProtocolReceiptCapture
			readJSON(t, filepath.Join(f.dir, "protocol-receipts.json"), &capture)
			tc.mutate(&capture)
			writeJSON(t, filepath.Join(f.dir, "protocol-receipts.json"), capture)
			r := analyzeFixture(t, f)
			s := measurementStep(t, r)
			if got := s.Claims["protocol_delivery"].Status; got != tc.want {
				t.Fatalf("claim=%s want=%s: %+v", got, tc.want, s.Claims["protocol_delivery"])
			}
			if s.Metrics["protocol.delivery.p99_ms"].Value != nil {
				t.Fatal("invalid capture produced a p99")
			}
		})
	}
	t.Run("missing capture cannot fall back", func(t *testing.T) {
		f := newRun(t)
		if err := os.Remove(filepath.Join(f.dir, "protocol-receipts.json")); err != nil {
			t.Fatal(err)
		}
		r := analyzeFixture(t, f)
		s := measurementStep(t, r)
		if r.MetricInterpretation != analysis.ProtocolMetricInterpretation || s.Claims["protocol_delivery"].Status != analysis.NotAssessed || s.Metrics["protocol.delivery.p99_ms"].Value != nil {
			t.Fatal(r.MetricInterpretation, s.Claims, s.Metrics)
		}
	})
	t.Run("missing receipts array cannot fabricate counts", func(t *testing.T) {
		f := newRun(t)
		var raw map[string]any
		readJSON(t, filepath.Join(f.dir, "protocol-receipts.json"), &raw)
		delete(raw, "receipts")
		writeJSON(t, filepath.Join(f.dir, "protocol-receipts.json"), raw)
		r := analyzeFixture(t, f)
		s := measurementStep(t, r)
		if s.Claims["protocol_delivery"].Status == analysis.Pass || s.Metrics["protocol.scheduled_cohort.observed"].Value != nil || s.Metrics["protocol.scheduled_cohort.missing"].Value != nil {
			t.Fatalf("malformed capture fabricated cohort counts: %+v", s.Metrics)
		}
	})
}

func TestOfflineMixedLegacyProtocolComparisonStaysSeparate(t *testing.T) {
	protocolRun := newOfflineFixture(t, "SYNTHETIC-offline-protocol", true)
	legacyRun := newOfflineFixture(t, "SYNTHETIC-offline-legacy", false)
	protocolResult := analyzeFixture(t, protocolRun)
	legacyResult, err := analysis.Analyze(analysis.Options{Run: legacyRun.dir, Evidence: filepath.Join(legacyRun.dir, "evidence.json")})
	if err != nil {
		t.Fatal(err)
	}
	if protocolResult.MetricInterpretation == legacyResult.MetricInterpretation {
		t.Fatal("fixtures did not exercise both interpretations")
	}
	comparison := analysis.Compare([]*analysis.Result{protocolResult, legacyResult})
	groups := map[string]bool{}
	interpretations := map[string]bool{}
	protocolRows, legacyRows := 0, 0
	for _, row := range comparison.Rows {
		groups[row.Group] = true
		interpretations[row.MetricInterpretation] = true
		switch row.MetricInterpretation {
		case analysis.ProtocolMetricInterpretation:
			protocolRows++
			if row.ProtocolBinding == nil || row.ProfileSHA256 == "" {
				t.Fatal("protocol comparison row lost binding/profile provenance")
			}
		case analysis.LegacyMetricInterpretation:
			legacyRows++
			if row.ProtocolBinding != nil || row.ProfileSHA256 != "" {
				t.Fatal("legacy comparison row was relabelled protocol")
			}
		}
	}
	if len(groups) < 2 || len(interpretations) != 2 || protocolRows == 0 || legacyRows == 0 {
		t.Fatalf("mixed comparison not separated: groups=%d interpretations=%v protocol=%d legacy=%d", len(groups), interpretations, protocolRows, legacyRows)
	}
	out := filepath.Join(t.TempDir(), "mixed-comparison")
	if err = analysis.WriteComparison(out, comparison, nil); err != nil {
		t.Fatal(err)
	}
	records := readCSV(t, filepath.Join(out, "comparison.csv"))
	index := headerIndex(t, records[0])
	seen := map[string]bool{}
	for _, row := range records[1:] {
		interp := row[index["metric_interpretation"]]
		seen[interp] = true
		switch interp {
		case analysis.ProtocolMetricInterpretation:
			if row[index["protocol_id"]] == "" || row[index["protocol_sha256"]] == "" || row[index["profile_sha256"]] == "" {
				t.Fatal("protocol CSV row missing provenance")
			}
		case analysis.LegacyMetricInterpretation:
			if row[index["protocol_id"]] != "" || row[index["protocol_sha256"]] != "" || row[index["profile_sha256"]] != "" {
				t.Fatal("legacy CSV row contains protocol provenance")
			}
		}
	}
	if !seen[analysis.ProtocolMetricInterpretation] || !seen[analysis.LegacyMetricInterpretation] {
		t.Fatalf("CSV interpretations: %v", seen)
	}
}

func protocolPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "research", "protocols", "iotbench-telemetry-v0.1.0-pilot.1.json")
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func headerIndex(t *testing.T, header []string) map[string]int {
	t.Helper()
	index := map[string]int{}
	for i, name := range header {
		index[name] = i
	}
	return index
}

func findCSV(t *testing.T, records [][]string, index map[string]int, field, value string) []string {
	t.Helper()
	for _, row := range records[1:] {
		if row[index[field]] == value {
			return row
		}
	}
	t.Fatalf("CSV row with %s=%s missing", field, value)
	return nil
}
