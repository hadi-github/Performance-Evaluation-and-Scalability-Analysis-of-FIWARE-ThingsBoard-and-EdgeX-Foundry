package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/protocol"
)

func offlineTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func offlineProtocolPlan(t *testing.T, root string, class protocol.ExecutionClass) (*plan.Plan, []byte) {
	t.Helper()
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
	binding := protocol.Binding{ProtocolID: definition.ProtocolID, ProtocolSHA256: loaded.SHA256(), ExecutionClass: class}
	pilot := &protocol.PilotParameters{RatePerSecond: &rate, MeasurementSeconds: &measurement, WarmupSeconds: &warmup, DrainCutoffUnixMS: &cutoff}
	p := &plan.Plan{
		SchemaVersion: 1, PlanID: "offline-protocol-fixture", Purpose: "SYNTHETIC OFFLINE ENGINEERING FIXTURE; not a live run",
		PilotOnly: true, Seed: 17, Adapter: "fixture", Target: plan.Target{BaseURL: "http://synthetic.invalid"},
		Devices: 5, RequestTimeMs: 1000, Connection: plan.Connection{KeepAlive: true},
		Arrivals: plan.Arrivals{Model: "constant"}, VUs: plan.VUs{PreAllocated: 5, Max: 5},
		Schedule: plan.Schedule{Type: "steps", Steps: []plan.StepSpec{
			{Phase: "warmup", Rate: rate, DurationS: warmup},
			{Phase: "measurement", Rate: rate, DurationS: measurement},
		}},
		Health:          plan.Health{MinAchievedRatio: .99, SendLagP99Ms: 10, EarlyToleranceMs: 2},
		Deviations:      []string{"synthetic offline fixture; no generator, Docker, SUT or service was executed"},
		ProtocolBinding: &binding, ProtocolPilot: pilot,
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return p, append(b, '\n')
}

func writeOfflinePlan(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareOfflineFixtureUsesRealPlanResolveAndProtocolMetadata(t *testing.T) {
	root := offlineTestRoot(t)
	p, b := offlineProtocolPlan(t, root, protocol.EngineeringPilot)
	planPath := writeOfflinePlan(t, b)
	results := t.TempDir()
	f, err := PrepareOfflineFixture(Options{PlanPath: planPath, RepoRoot: root, ResultsDir: results, RunID: "SYNTHETIC-offline-runner"})
	if err != nil {
		t.Fatal(err)
	}
	if f.Dir != filepath.Join(results, "SYNTHETIC-offline-runner") {
		t.Fatalf("fixture dir %s", f.Dir)
	}
	var manifest Manifest
	readJSON(t, filepath.Join(f.Dir, "manifest.json"), &manifest)
	var resolved plan.Resolved
	readJSON(t, filepath.Join(f.Dir, "resolved.json"), &resolved)
	planBytes, err := os.ReadFile(filepath.Join(f.Dir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(planBytes)
	if manifest.PlanSHA256 != hex.EncodeToString(sum[:]) || f.PlanSHA256 != manifest.PlanSHA256 {
		t.Fatalf("plan hash mismatch: %s %s %x", manifest.PlanSHA256, f.PlanSHA256, sum)
	}
	if !manifest.Synthetic || !manifest.OfflineFixture || manifest.K6["executed"] != "false" || manifest.K6["mode"] != "offline-engineering-fixture" {
		t.Fatalf("offline/synthetic metadata missing: %+v", manifest)
	}
	if manifest.RuntimeMetricInterpretation != LegacyMetricInterpretation {
		t.Fatalf("runtime interpretation %q", manifest.RuntimeMetricInterpretation)
	}
	if !reflect.DeepEqual(manifest.ProtocolBinding, p.ProtocolBinding) || !reflect.DeepEqual(manifest.ProtocolPilot, p.ProtocolPilot) {
		t.Fatalf("manifest protocol metadata does not match plan: %+v %+v", manifest.ProtocolBinding, manifest.ProtocolPilot)
	}
	if manifest.Resolved == nil || !reflect.DeepEqual(manifest.Resolved, &resolved) || !reflect.DeepEqual(f.Resolved, &resolved) {
		t.Fatal("manifest/fixture resolved plan differs from resolved.json")
	}
	if manifest.RunID != "SYNTHETIC-offline-runner" || manifest.PlanID != p.PlanID || !manifest.PilotOnly {
		t.Fatalf("identity mismatch: %+v", manifest)
	}
	if err = validateProtocolManifest(&manifest); err != nil {
		t.Fatalf("fresh offline manifest rejected: %v", err)
	}
	entries, err := os.ReadDir(f.Dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"manifest.json", "plan.json", "resolved.json"}) {
		t.Fatalf("offline preparation wrote unexpected artifacts: %v", names)
	}
}

func TestProtocolManifestMetadataFailsClosed(t *testing.T) {
	root := offlineTestRoot(t)
	_, b := offlineProtocolPlan(t, root, protocol.EngineeringPilot)
	f, err := PrepareOfflineFixture(Options{PlanPath: writeOfflinePlan(t, b), RepoRoot: root, ResultsDir: t.TempDir(), RunID: "SYNTHETIC-offline-runner"})
	if err != nil {
		t.Fatal(err)
	}
	clone := func(t *testing.T) *Manifest {
		t.Helper()
		var m Manifest
		readJSON(t, filepath.Join(f.Dir, "manifest.json"), &m)
		return &m
	}
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{"missing binding", func(m *Manifest) { m.ProtocolBinding = nil }, "contradicts resolved plan"},
		{"altered binding", func(m *Manifest) { m.ProtocolBinding.ProtocolSHA256 = strings.Repeat("0", 64) }, "contradicts resolved plan"},
		{"missing pilot", func(m *Manifest) { m.ProtocolPilot = nil }, "contradicts resolved plan"},
		{"altered pilot", func(m *Manifest) { v := 9; m.ProtocolPilot.MeasurementSeconds = &v }, "contradicts resolved plan"},
		{"missing runtime interpretation", func(m *Manifest) { m.RuntimeMetricInterpretation = "" }, "explicitly labeled legacy"},
		{"upgraded runtime interpretation", func(m *Manifest) { m.RuntimeMetricInterpretation = "protocol-scheduled-cohort-v1" }, "cannot claim protocol receipt instrumentation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := clone(t)
			tc.mutate(m)
			err := validateProtocolManifest(m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPrepareOfflineFixtureRejectsCampaignBeforeWritingRun(t *testing.T) {
	root := offlineTestRoot(t)
	_, b := offlineProtocolPlan(t, root, protocol.Campaign)
	results := t.TempDir()
	_, err := PrepareOfflineFixture(Options{PlanPath: writeOfflinePlan(t, b), RepoRoot: root, ResultsDir: results, RunID: "SYNTHETIC-campaign"})
	if err == nil || !strings.Contains(err.Error(), "campaign ineligible") {
		t.Fatalf("campaign plan accepted: %v", err)
	}
	if entries, _ := os.ReadDir(results); len(entries) != 0 {
		t.Fatalf("campaign attempt wrote a run directory: %v", entries)
	}
}

func TestPrepareOfflineFixtureRejectsReusedRunDirectory(t *testing.T) {
	root := offlineTestRoot(t)
	_, b := offlineProtocolPlan(t, root, protocol.EngineeringPilot)
	results := t.TempDir()
	planPath := writeOfflinePlan(t, b)
	opts := Options{PlanPath: planPath, RepoRoot: root, ResultsDir: results, RunID: "SYNTHETIC-offline-runner"}
	if _, err := PrepareOfflineFixture(opts); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareOfflineFixture(opts); err == nil || !strings.Contains(err.Error(), "never reused") {
		t.Fatalf("reused run directory accepted: %v", err)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}
