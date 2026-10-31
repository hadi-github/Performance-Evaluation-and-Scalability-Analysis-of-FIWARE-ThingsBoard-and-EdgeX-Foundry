package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// Select a pair enclosing each measured step. CPU cost describes the ACTUAL
// capture interval, never a falsely exact nominal measurement interval. Preserve
// all captures and report boundary skew so the reader can judge contamination.
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
		Text        string `json:"snapshot_tsv"`
		Status      string `json:"status"`
		Rows        []cpubound.SnapshotRecord
		First, Last float64
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
		valid := true
		for i, row := range s.Rows {
			t, e := strconv.ParseFloat(row.EpochS, 64)
			if e != nil {
				valid = false
				break
			}
			if i == 0 || t < s.First {
				s.First = t
			}
			if i == 0 || t > s.Last {
				s.Last = t
			}
		}
		if valid {
			samples = append(samples, s)
		}
	}
	var reports []map[string]any
	for _, step := range resolved.Steps {
		if step.Phase == "warmup" {
			continue
		}
		var t0 float64
		var n int64
		for _, s := range summary.Steps {
			if s.Index == step.Index {
				t0 = s.T0 / 1000
				n = s.Attempted
			}
		}
		end := t0 + float64(step.Duration)
		before, after := -1, -1
		for i, s := range samples {
			if s.Last <= t0 && (before < 0 || s.Last > samples[before].Last) {
				before = i
			}
			if s.First >= end && (after < 0 || s.First < samples[after].First) {
				after = i
			}
		}
		entry := map[string]any{"step": step.Index, "measurement_start_epoch_seconds": t0, "measurement_end_epoch_seconds": end, "status": "UNAVAILABLE"}
		if t0 > 0 && before >= 0 && after >= 0 {
			pre, post := samples[before], samples[after]
			prefix := fmt.Sprintf("cpu-step-%02d", step.Index)
			if err = os.WriteFile(filepath.Join(dir, prefix+"-before.tsv"), []byte(pre.Text), 0644); err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(dir, prefix+"-after.tsv"), []byte(post.Text), 0644); err != nil {
				return err
			}
			entry["capture_start_epoch_seconds"] = pre.First
			entry["capture_end_epoch_seconds"] = post.Last
			entry["before_skew_seconds"] = t0 - pre.First
			entry["after_skew_seconds"] = post.Last - end
			entry["report"] = cpubound.Diff(pre.Rows, post.Rows, &cpubound.Denominator{Count: n, Source: "k6-summary.json", Definition: "attempted HTTP sends in this measured step; CPU interval includes explicitly reported boundary skew", Present: true}, "CPU delta covers the capture interval, not precisely the nominal phase. No retrospective counters are inferred.")
			entry["status"] = "CAPTURED"
		}
		reports = append(reports, entry)
	}
	return writeJSON(filepath.Join(dir, "cpu-boundary.json"), reports)
}
