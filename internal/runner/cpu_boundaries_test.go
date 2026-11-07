package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCPUBoundarySelectionUsesScenarioTimes(t *testing.T) {
	d := t.TempDir()
	if err := writeJSON(filepath.Join(d, "resolved.json"), map[string]any{"steps": []map[string]any{
		{"index": 0, "phase": "warmup", "duration_s": 10},
		{"index": 1, "phase": "measurement", "duration_s": 60},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(d, "k6-summary.json"), map[string]any{"steps": []map[string]any{
		{"index": 0, "scenario_start_ms": 10000, "attempted": 100},
		{"index": 1, "scenario_start_ms": 20000, "attempted": 1000},
	}}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(d, "cpu-snapshots.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	e := json.NewEncoder(f)
	captures := []struct{ request, response, tsv string }{
		{"1970-01-01T00:00:09.700Z", "1970-01-01T00:00:09.900Z", "10000000000.0\tapp\tid\t1000000\tstart\t4\n"},
		{"1970-01-01T00:00:19.700Z", "1970-01-01T00:00:19.900Z", "10000000010.0\tapp\tid\t1900000\tstart\t4\n"},
		{"1970-01-01T00:00:20.200Z", "1970-01-01T00:00:20.400Z", "10000000011.0\tapp\tid\t2000000\tstart\t4\n"},
		{"1970-01-01T00:01:20.200Z", "1970-01-01T00:01:20.400Z", "10000000080.0\tapp\tid\t3000000\tstart\t4\n"},
	}
	for _, sample := range captures {
		if err = e.Encode(map[string]string{"status": "OK", "request_utc": sample.request, "response_utc": sample.response, "snapshot_tsv": sample.tsv}); err != nil {
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
		Phase  string    `json:"phase"`
		Status string    `json:"status"`
		Before []float64 `json:"before_boundary_skew_bounds_seconds"`
		Report struct {
			Total float64 `json:"total_cpu_seconds"`
			Per   float64 `json:"total_cpu_seconds_per_message"`
		} `json:"report"`
	}
	if err = json.Unmarshal(b, &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].Phase != "warmup" || result[1].Phase != "measurement" ||
		result[0].Status != "CAPTURED" || result[1].Status != "CAPTURED" ||
		result[0].Report.Total != 1 || result[0].Report.Per != 0.01 ||
		result[1].Report.Total != 1.1 || result[1].Report.Per != 0.0011 ||
		len(result[0].Before) != 2 || result[0].Before[0] <= 0 {
		t.Fatalf("unexpected report: %s", b)
	}
}
