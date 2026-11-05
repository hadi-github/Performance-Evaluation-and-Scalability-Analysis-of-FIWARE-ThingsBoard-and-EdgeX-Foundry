package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/receipt"
	"iotbench/internal/report"
)

// SyntheticProtocolCohort writes static, explicitly synthetic artifacts; it starts no generator or service.
func SyntheticProtocolCohort(dir, runID, protocolPath, protocolRoot string) error {
	if err := Synthetic(dir, runID, 5); err != nil {
		return err
	}
	loaded, err := protocol.LoadVerified(protocolPath, protocolRoot)
	if err != nil {
		return err
	}
	d, err := loaded.Definition()
	if err != nil {
		return err
	}
	rate := 1.0
	measurement, warmup := 5, 2
	start := int64(1700000000000)
	cutoff := start + 12000
	binding := protocol.Binding{ProtocolID: d.ProtocolID, ProtocolSHA256: loaded.SHA256(), ExecutionClass: protocol.EngineeringPilot}
	pilot := &protocol.PilotParameters{RatePerSecond: &rate, MeasurementSeconds: &measurement, WarmupSeconds: &warmup, DrainCutoffUnixMS: &cutoff}
	p := plan.Plan{SchemaVersion: 1, PlanID: "synthetic-protocol-cohort", Purpose: "STATIC SYNTHETIC protocol cohort; not an experiment", PilotOnly: true, Seed: 17, Adapter: "fixture", Target: plan.Target{BaseURL: "http://synthetic.invalid"}, Devices: 5, RequestTimeMs: 1000, Connection: plan.Connection{KeepAlive: true}, Arrivals: plan.Arrivals{Model: "constant"}, VUs: plan.VUs{PreAllocated: 5, Max: 5}, Schedule: plan.Schedule{Type: "steps", Steps: []plan.StepSpec{{Phase: "warmup", Rate: 1, DurationS: 2}, {Phase: "measurement", Rate: 1, DurationS: 5}}}, Health: plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2}, TestFaults: plan.TestFaults{}, ProtocolBinding: &binding, ProtocolPilot: pilot}
	r, err := p.Resolve(runID)
	if err != nil {
		return err
	}
	if err = writeJSON(filepath.Join(dir, "plan.json"), p); err != nil {
		return err
	}
	if err = writeJSON(filepath.Join(dir, "resolved.json"), r); err != nil {
		return err
	}
	measurementStart := float64(start + 2000)
	stepSummary := []report.StepSummary{
		{Index: 0, Name: r.Steps[0].Name, ScenarioStartMs: float64(start), Attempted: 2, AckSuccess: 2, HTTPReqs: 2, SendLag: report.TrendStats{Count: 2, P50: 1, P99: 1, Max: 1}, AckLatency: report.TrendStats{Count: 2, P50: 20, P99: 20, Max: 20}},
		{Index: 1, Name: r.Steps[1].Name, ScenarioStartMs: measurementStart, Attempted: 5, AckSuccess: 1, ExplicitFailure: 1, Unknown: 3, HTTPReqs: 5, SendLag: report.TrendStats{Count: 5, P50: 1, P99: 1, Max: 1}, AckLatency: report.TrendStats{Count: 1, P50: 100, P99: 100, Max: 100}},
	}
	sum := report.Summary{SchemaVersion: 2, RunID: runID, Steps: stepSummary, SetupEndMs: float64(start), TestRunDurationMs: 7000, VUsPeak: 5, VUsMaxAllocated: 5}
	if err = writeJSON(filepath.Join(dir, "k6-summary.json"), sum); err != nil {
		return err
	}
	planBytes, err := os.ReadFile(filepath.Join(dir, "plan.json"))
	if err != nil {
		return err
	}
	planHash := sha256.Sum256(planBytes)
	manifest := map[string]any{"schema_version": 1, "run_id": runID, "plan_id": p.PlanID, "pilot_only": true, "plan_sha256": hex.EncodeToString(planHash[:]), "resolved_plan": r, "k6_exit_code": 0, "verdict": "VALID", "git": map[string]string{"sha": strings.Repeat("a", 40), "dirty": "false"}, "k6": map[string]string{"digest": "synthetic/k6@sha256:" + strings.Repeat("b", 64)}, "k6_scripts_sha256": strings.Repeat("c", 64), "generator_cpuset": "synthetic", "connection_model": map[string]any{"http": "HTTP/1.1", "keep_alive": true}, "synthetic": true, "protocol_binding": binding, "protocol_pilot_parameters": pilot, "runtime_metric_interpretation": "legacy-acknowledgement-only-v1", "receipt_capture_interpretation": ProtocolMetricInterpretation}
	if err = writeJSON(filepath.Join(dir, "manifest.json"), manifest); err != nil {
		return err
	}
	var records strings.Builder
	for _, entry := range []struct {
		seq     int64
		outcome string
		latency float64
	}{{3, "explicit_failure", 600}, {4, "unknown", 20}, {5, "unknown", 25}, {6, "unknown", 30}} {
		scheduled := measurementStart + float64(entry.seq-2)*1000
		record := report.Record{Rec: "msg", RunID: runID, Seq: entry.seq, Device: int(entry.seq % 5), Step: 1, Phase: "measurement", Tick: entry.seq - 2, SchedMs: scheduled, ActualMs: scheduled + 1, Outcome: entry.outcome, Status: 500, RespMs: scheduled + entry.latency}
		b, e := json.Marshal(record)
		if e != nil {
			return e
		}
		records.Write(b)
		records.WriteByte('\n')
	}
	if err = os.WriteFile(filepath.Join(dir, "records.ndjson"), []byte(records.String()), 0644); err != nil {
		return err
	}
	// Maintain ordinary legacy observation artifacts, but the protocol cohort metric below consumes only the explicit new capture.
	var sink, history strings.Builder
	for _, seq := range []int64{2, 3, 4} {
		scheduled := measurementStart + float64(seq-2)*1000
		for name, dst := range map[string]*strings.Builder{"sink.ndjson": &sink, "history.ndjson": &history} {
			row := map[string]any{"run_id": runID, "seq": seq, "recv_ms": scheduled + []float64{100, 600, 20}[seq-2]}
			if name == "history.ndjson" {
				row["device"] = fmt.Sprintf("device-%d", seq%5)
				row["ts"] = scheduled + 1
			}
			b, e := json.Marshal(row)
			if e != nil {
				return e
			}
			dst.Write(b)
			dst.WriteByte('\n')
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "sink.ndjson"), []byte(sink.String()), 0644); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "history.ndjson"), []byte(history.String()), 0644); err != nil {
		return err
	}
	ev := loadSyntheticEvidence(dir)
	ev.ObservationDeadlineMS = float64(cutoff)
	ev.Sink.StartMS = float64(start)
	ev.Sink.EndMS = float64(cutoff)
	ev.History.StartMS = float64(start)
	ev.History.EndMS = float64(cutoff)
	ev.History.CollectedAtMS = float64(cutoff)
	if err = writeJSON(filepath.Join(dir, "evidence.json"), ev); err != nil {
		return err
	}
	capture := ProtocolReceiptCapture{SchemaVersion: receipt.SchemaVersion, MetricInterpretation: ProtocolMetricInterpretation, Binding: binding, Synthetic: true, Complete: true, Endpoint: receipt.Endpoint, Source: "synthetic:static fixture body capture", BodyReadComplete: true, ReceiptCaptureSemantics: receipt.CaptureSemantics, ClockDomain: "synthetic:epoch-ms", ClockSource: "synthetic:fixed epoch schedule", CoverageStartMS: measurementStart, CoverageEndMS: float64(cutoff), ObservationCutoffMS: float64(cutoff), Receipts: []ProtocolReceipt{
		{RunID: runID, Seq: 0, DeviceID: "device-0", Phase: "warmup", ScheduledSendMS: float64(start), ObservedMS: float64(start + 50), Outcome: protocol.OutcomeOK},
		{RunID: runID, Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: measurementStart, ObservedMS: measurementStart + 100, Outcome: protocol.OutcomeOK},
		{RunID: runID, Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: measurementStart, ObservedMS: measurementStart + 150, Outcome: protocol.OutcomeOK},
		{RunID: runID, Seq: 3, DeviceID: "device-3", Phase: "measurement", ScheduledSendMS: measurementStart + 1000, ObservedMS: measurementStart + 1600, Outcome: protocol.OutcomeError},
		{RunID: runID, Seq: 4, DeviceID: "device-4", Phase: "measurement", ScheduledSendMS: measurementStart + 2000, ObservedMS: measurementStart + 2020, Outcome: protocol.OutcomeUnknown},
	}}
	return writeJSON(filepath.Join(dir, "protocol-receipts.json"), capture)
}

func loadSyntheticEvidence(dir string) Evidence {
	var ev Evidence
	b, _ := os.ReadFile(filepath.Join(dir, "evidence.json"))
	_ = json.Unmarshal(b, &ev)
	return ev
}
