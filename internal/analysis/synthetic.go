package analysis

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

// Synthetic creates tiny STATIC evidence, never starts a generator or service.
// Hash-like values below are explicit fake identifiers, not real image digests.
func Synthetic(dir, id string, devices int) error {
	if err := os.Mkdir(dir, 0755); err != nil {
		return err
	}
	r := plan.Resolved{Plan: plan.Plan{SchemaVersion: 1, PlanID: "synthetic", Purpose: "SYNTHETIC offline analysis fixture; not an experiment", PilotOnly: true, Seed: 17, Adapter: "fiware", Devices: devices, RequestTimeMs: 1000, Connection: plan.Connection{KeepAlive: true}, Arrivals: plan.Arrivals{Model: "constant"}, VUs: plan.VUs{PreAllocated: 5, Max: 5}, Health: plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2}}, RunID: id, TotalTicks: 10, DurationS: 1, SendProb: 1, Steps: []plan.Step{{Index: 0, Name: "s00-measure-r10", Phase: "measurement", Rate: 10, TickRate: 10, DurationS: 1, Ticks: 10}}}
	t0 := 1700000000000.0
	sum := report.Summary{SchemaVersion: 2, RunID: id, Steps: []report.StepSummary{{Index: 0, Name: r.Steps[0].Name, ScenarioStartMs: t0, Attempted: 10, AckSuccess: 10, HTTPReqs: 10, SendLag: report.TrendStats{Count: 10, P50: 1, P99: 2, Max: 3}, AckLatency: report.TrendStats{Count: 10, P50: 20, P99: 30, Max: 40}}}}
	manifest := map[string]any{"schema_version": 1, "run_id": id, "plan_id": r.PlanID, "resolved_plan": r, "k6_exit_code": 0, "verdict": "VALID", "git": map[string]string{"sha": strings.Repeat("a", 40), "dirty": "false"}, "k6": map[string]string{"digest": "synthetic/k6@sha256:" + strings.Repeat("b", 64)}, "k6_scripts_sha256": strings.Repeat("c", 64), "generator_cpuset": "synthetic", "connection_model": map[string]any{"http": "HTTP/1.1", "keep_alive": true}, "synthetic": true}
	ev := Evidence{SchemaVersion: 1, Synthetic: true, Interpretation: "claim-gates-v1", Configuration: Configuration{ID: "synthetic-config-not-deployed", CPUAllocationK: 1, Replicas: map[string]int{"application": 1}, Images: map[string]string{"application": "synthetic/application@sha256:" + strings.Repeat("d", 64)}, EffectiveSHA256: strings.Repeat("e", 64)}, ObservationDeadlineMS: t0 + 2000, Sink: Source{Path: "sink.ndjson", Complete: true, Source: "synthetic complete capture", StartMS: t0, EndMS: t0 + 2000}, History: Source{Path: "history.ndjson", Complete: true, Source: "synthetic history export", StartMS: t0, EndMS: t0 + 2000, CollectedAtMS: t0 + 2000}, SinkClock: "shared_load_realtime", ClockSource: "synthetic common epoch-ms clock", CPU: []CPUInput{{Step: 0, Before: "cpu-before.tsv", After: "cpu-after.tsv", ExpectedContainers: []string{"app", "db"}, Denominator: "acknowledged", Clock: "shared_load_realtime", ClockSource: "synthetic exact endpoint timestamps"}}}
	policy := DefaultPolicy()
	config := []byte("{\"synthetic\":true,\"configuration\":\"example-only\"}\n")
	ev.Configuration.EffectivePath = "effective.json"
	ev.Configuration.EffectiveSHA256 = fmt.Sprintf("%x", sha256.Sum256(config))
	if e := os.WriteFile(filepath.Join(dir, "effective.json"), config, 0644); e != nil {
		return e
	}
	policy.Synthetic = true
	// Legacy tiny demo policy is deliberately separate from the approved W2 study.
	policy.Profiles[0].Sensitivities = nil
	policy.Profiles[2].DeviceComparison = nil
	q := func(metric, op, unit string, t float64) Requirement {
		return Requirement{Metric: metric, Operator: op, Threshold: &t, Unit: unit, Source: "SYNTHETIC EXAMPLE ONLY; not an adopted research threshold"}
	}
	policy.Profiles[0].Source = "synthetic example"
	policy.Profiles[0].Requirements = []Requirement{q("latency.delivery.p99_ms", "<=", "ms", 100)}
	policy.Profiles[1].Source = "synthetic example"
	policy.Profiles[1].Requirements = []Requirement{q("history.missing_acknowledged", "==", "messages", 0), q("history.duplicate_extra_copies", "==", "messages", 0)}
	policy.Profiles[2].Source = "synthetic example"
	policy.Profiles[2].Requirements = []Requirement{q("workload.observed_history_devices", ">=", "devices", 2), q("workload.offered_rate", ">=", "messages/s", 10)}
	for name, value := range map[string]any{"resolved.json": r, "k6-summary.json": sum, "manifest.json": manifest, "evidence.json": ev, "profiles.json": policy} {
		if e := writeJSON(filepath.Join(dir, name), value); e != nil {
			return e
		}
	}
	if e := os.WriteFile(filepath.Join(dir, "records.ndjson"), nil, 0644); e != nil {
		return e
	}
	var sink, history strings.Builder
	for seq := 0; seq < 10; seq++ {
		s, _ := json.Marshal(map[string]any{"run_id": id, "seq": seq, "recv_ms": t0 + float64(seq)*100 + 10})
		sink.Write(s)
		sink.WriteByte('\n')
		h, _ := json.Marshal(map[string]any{"run_id": id, "seq": seq, "device": fmt.Sprintf("device-%d", seq%devices), "ts": t0 + float64(seq)*100 + 5})
		history.Write(h)
		history.WriteByte('\n')
	}
	for name, text := range map[string]string{"sink.ndjson": sink.String(), "history.ndjson": history.String(), "cpu-before.tsv": "1700000000\tapp\tapp-id\t1000000\tstart\t4\tapp\tapplication\n1700000000\tdb\tdb-id\t2000000\tstart\t2-3\tdb\tsupporting\n", "cpu-after.tsv": "1700000001\tapp\tapp-id\t2000000\tstart\t4\tapp\tapplication\n1700000001\tdb\tdb-id\t2500000\tstart\t2-3\tdb\tsupporting\n"} {
		if e := os.WriteFile(filepath.Join(dir, name), []byte(text), 0644); e != nil {
			return e
		}
	}
	return nil
}

func Demo(out string) error {
	if err := newDirectory(out, nil); err != nil {
		return err
	}
	inputs := filepath.Join(out, "inputs")
	reports := filepath.Join(out, "reports")
	if e := os.Mkdir(inputs, 0755); e != nil {
		return e
	}
	if e := os.Mkdir(reports, 0755); e != nil {
		return e
	}
	var results []*Result
	for _, c := range []struct {
		name    string
		devices int
	}{{"valid", 2}, {"latency-failure", 2}, {"equal-rate-more-devices", 10}} {
		dir := filepath.Join(inputs, c.name)
		if e := Synthetic(dir, "SYNTHETIC-"+c.name, c.devices); e != nil {
			return e
		}
		if c.name == "latency-failure" {
			b, e := os.ReadFile(filepath.Join(dir, "sink.ndjson"))
			if e != nil {
				return e
			}
			list, e := rows[map[string]any](b)
			if e != nil {
				return e
			}
			var text strings.Builder
			for _, row := range list {
				row["recv_ms"] = row["recv_ms"].(float64) + 500
				b, _ := json.Marshal(row)
				text.Write(b)
				text.WriteByte('\n')
			}
			if e = os.WriteFile(filepath.Join(dir, "sink.ndjson"), []byte(text.String()), 0644); e != nil {
				return e
			}
		}
		r, e := Analyze(Options{Run: dir, Evidence: filepath.Join(dir, "evidence.json"), Policy: filepath.Join(dir, "profiles.json")})
		if e != nil {
			return e
		}
		if e = Write(filepath.Join(reports, c.name), r, dir); e != nil {
			return e
		}
		results = append(results, r)
	}
	return WriteComparison(filepath.Join(out, "comparison"), Compare(results), nil)
}
