package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"iotbench/internal/cpubound"
)

// Sample continuously, including setup, so phase boundaries are selected from the
// generator's recorded scenario timestamps rather than guessed from launch time.
// The command must return a complete, single snapshot in cpu-snapshot.sh format.
func captureCPU(ctx context.Context, command, dir string) {
	f, err := os.Create(filepath.Join(dir, "cpu-snapshots.jsonl"))
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for ctx.Err() == nil {
		start := time.Now().UTC()
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(cctx, "bash", "-c", command)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, e := cmd.Output()
		cancel()
		status := "OK"
		if e != nil {
			status = e.Error()
		}
		_ = enc.Encode(map[string]any{"request_utc": start, "response_utc": time.Now().UTC(), "status": status, "stderr": stderr.String(), "snapshot_tsv": string(out)})
		select {
		case <-ctx.Done():
			return
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// Select a pair that encloses every phase using load-host request/response
// brackets. SUT snapshot wall clocks are not compared to load-host scenario times.
// The actual counter-snapshot instant lies within each request/response bracket.
func finishCPU(dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, "k6-summary.json"))
	if err != nil {
		return err
	}
	var summary struct {
		Steps []struct {
			Index     int     `json:"index"`
			T0        float64 `json:"scenario_start_ms"`
			Attempted int64   `json:"attempted"`
		} `json:"steps"`
	}
	if err = json.Unmarshal(b, &summary); err != nil {
		return err
	}
	b, err = os.ReadFile(filepath.Join(dir, "resolved.json"))
	if err != nil {
		return err
	}
	var resolved struct {
		Steps []struct {
			Index    int    `json:"index"`
			Phase    string `json:"phase"`
			Duration int    `json:"duration_s"`
		} `json:"steps"`
	}
	if err = json.Unmarshal(b, &resolved); err != nil {
		return err
	}
	b, err = os.ReadFile(filepath.Join(dir, "cpu-snapshots.jsonl"))
	if err != nil {
		return err
	}
	type snapshot struct {
		Text     string    `json:"snapshot_tsv"`
		Status   string    `json:"status"`
		Request  time.Time `json:"request_utc"`
		Response time.Time `json:"response_utc"`
		Rows     []cpubound.SnapshotRecord
	}
	var samples []snapshot
	for _, line := range strings.Split(string(b), "\n") {
		var s snapshot
		if json.Unmarshal([]byte(line), &s) != nil || s.Status != "OK" {
			continue
		}
		s.Rows, err = cpubound.Parse(strings.NewReader(s.Text))
		if err != nil || len(s.Rows) == 0 {
			continue
		}
		if !s.Request.IsZero() && !s.Response.IsZero() && !s.Response.Before(s.Request) {
			samples = append(samples, s)
		}
	}
	var reports []map[string]any
	for _, step := range resolved.Steps {
		var t0 float64
		var n int64
		for _, s := range summary.Steps {
			if s.Index == step.Index {
				t0 = s.T0 / 1000
				n = s.Attempted
			}
		}
		end := t0 + float64(step.Duration)
		startTime := time.Unix(0, int64(t0*float64(time.Second)))
		endTime := time.Unix(0, int64(end*float64(time.Second)))
		before, after := -1, -1
		for i, s := range samples {
			if !s.Response.After(startTime) && (before < 0 || s.Response.After(samples[before].Response)) {
				before = i
			}
			if !s.Request.Before(endTime) && (after < 0 || s.Request.Before(samples[after].Request)) {
				after = i
			}
		}
		entry := map[string]any{"step": step.Index, "phase": step.Phase, "phase_start_load_epoch_seconds": t0,
			"phase_end_load_epoch_seconds": end, "status": "UNAVAILABLE",
			"alignment_clock": "load-host CLOCK_REALTIME for request/response brackets and k6 scenario timestamps; SUT wall clock not compared"}
		if t0 > 0 && before >= 0 && after >= 0 {
			pre, post := samples[before], samples[after]
			prefix := fmt.Sprintf("cpu-step-%02d", step.Index)
			if err = os.WriteFile(filepath.Join(dir, prefix+"-before.tsv"), []byte(pre.Text), 0644); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, prefix+"-after.tsv"), []byte(post.Text), 0644); err != nil {
				return err
			}
			preRequest, preResponse := float64(pre.Request.UnixNano())/1e9, float64(pre.Response.UnixNano())/1e9
			postRequest, postResponse := float64(post.Request.UnixNano())/1e9, float64(post.Response.UnixNano())/1e9
			entry["before_capture_request_epoch_seconds"] = preRequest
			entry["before_capture_response_epoch_seconds"] = preResponse
			entry["after_capture_request_epoch_seconds"] = postRequest
			entry["after_capture_response_epoch_seconds"] = postResponse
			entry["before_boundary_skew_bounds_seconds"] = []float64{t0 - preResponse, t0 - preRequest}
			entry["after_boundary_skew_bounds_seconds"] = []float64{postRequest - end, postResponse - end}
			entry["counter_interval_duration_bounds_seconds"] = []float64{postRequest - preResponse, postResponse - preRequest}
			entry["report"] = cpubound.Diff(pre.Rows, post.Rows, &cpubound.Denominator{Count: n, Source: "k6-summary.json", Definition: "attempted HTTP sends in this phase; CPU delta interval is bounded by load-host request/response brackets", Present: true}, "SUT counters are compared between snapshots whose request/response intervals bracket this phase. SUT wall timestamps are not aligned to load-host scenario timestamps; no exact-phase or cross-host latency claim is made.")
			entry["status"] = "CAPTURED"
		}
		reports = append(reports, entry)
	}
	return writeJSON(filepath.Join(dir, "cpu-boundary.json"), reports)
}
