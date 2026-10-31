package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCPUBoundarySelectionUsesScenarioTimes(t *testing.T) {
	d := t.TempDir()
	if err := writeJSON(filepath.Join(d, "resolved.json"), map[string]any{"steps": []map[string]any{{"index": 1, "phase": "sweep", "duration_s": 60}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(d, "k6-summary.json"), map[string]any{"steps": []map[string]any{{"index": 1, "scenario_start_ms": 10000, "attempted": 1000}}}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(d, "cpu-snapshots.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	e := json.NewEncoder(f)
	for _, s := range []string{"9.9\tapp\tid\t1000000\tstart\t4\n", "70.2\tapp\tid\t3000000\tstart\t4\n"} {
		if err = e.Encode(map[string]string{"status": "OK", "snapshot_tsv": s}); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	if err = finishCPU(d); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(d, "cpu-boundary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result []struct {
		Status     string  `json:"status"`
		BeforeSkew float64 `json:"before_skew_seconds"`
		Report     struct {
			Total float64 `json:"total_cpu_seconds"`
			Per   float64 `json:"total_cpu_seconds_per_message"`
		} `json:"report"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Status != "CAPTURED" || result[0].Report.Total != 2 || result[0].Report.Per != 0.002 || result[0].BeforeSkew <= 0 {
		t.Fatalf("unexpected report: %s", b)
	}
}
