package runner

// Progress is diagnostic only. Tail the existing complete outcome log outside
// k6; never add work to its dispatch/response timing path or alter verdicts.
import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

type progressStep struct {
	Index     int    `json:"index"`
	Phase     string `json:"phase"`
	Planned   int64  `json:"planned_ticks"`
	Completed int64  `json:"unique_completed_ticks"`
	Responses int64  `json:"responses_observed"`
	ACK       int64  `json:"ack_success"`
	Failure   int64  `json:"explicit_failure"`
	Unknown   int64  `json:"unknown"`
}

type progressSnapshot struct {
	SchemaVersion int            `json:"schema_version"`
	RunID         string         `json:"run_id"`
	UpdatedUTC    string         `json:"updated_utc"`
	ElapsedS      float64        `json:"observer_elapsed_s"`
	State         string         `json:"state"`
	LatestPhase   string         `json:"latest_completed_phase"`
	Planned       int64          `json:"planned_ticks"`
	Completed     int64          `json:"unique_completed_ticks"`
	Percent       float64        `json:"completed_tick_percent"`
	Responses     int64          `json:"responses_observed"`
	ACK           int64          `json:"ack_success"`
	Failure       int64          `json:"explicit_failure"`
	Unknown       int64          `json:"unknown"`
	Duplicates    int64          `json:"duplicate_response_records"`
	Invalid       int64          `json:"invalid_records"`
	ReadError     string         `json:"read_error,omitempty"`
	ExitCode      *int           `json:"k6_exit_code,omitempty"`
	Steps         []progressStep `json:"steps"`
	Semantics     string         `json:"semantics"`
}

type progressObserver struct {
	resolved *plan.Resolved
	offset   int64
	seen     map[int64]bool
	started  time.Time
	snapshot progressSnapshot
}

func newProgressObserver(r *plan.Resolved) *progressObserver {
	p := &progressObserver{resolved: r, seen: make(map[int64]bool), started: time.Now()}
	p.snapshot = progressSnapshot{SchemaVersion: 1, RunID: r.RunID, Planned: r.TotalTicks,
		State: "waiting_for_completed_responses", LatestPhase: "none",
		Semantics: "Diagnostic completed-response counts from the existing k6 log; in-flight requests are not counted. ACK is not sink/history delivery. Percent is unique completed ticks/planned ticks, not elapsed campaign progress. Generator exit does not establish capture, drain, analysis or cleanup success. Missing/buffered records can make counts lag; offline analysis remains authoritative."}
	for _, s := range r.Steps {
		p.snapshot.Steps = append(p.snapshot.Steps, progressStep{Index: s.Index, Phase: s.Phase, Planned: s.Ticks})
	}
	return p
}

func (p *progressObserver) consume(line []byte) {
	var envelope struct {
		Source string `json:"source"`
		Msg    string `json:"msg"`
	}
	if json.Unmarshal(line, &envelope) != nil {
		p.snapshot.Invalid++
		return
	}
	if envelope.Source != "console" {
		return // setup/error messages may contain credentials: never copy them
	}
	var r report.Record
	if json.Unmarshal([]byte(envelope.Msg), &r) != nil || r.Rec != "msg" {
		return
	}
	// Zero-valued decoded fields must not turn a missing/null identity into
	// the first planned tick (seq=0, step=0, tick=0).
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(envelope.Msg), &fields) != nil {
		p.snapshot.Invalid++
		return
	}
	for _, key := range []string{"run_id", "seq", "rate_step", "phase", "tick", "outcome"} {
		value, exists := fields[key]
		if !exists || string(value) == "null" {
			p.snapshot.Invalid++
			return
		}
	}
	for i, s := range p.resolved.Steps {
		if r.RunID != p.resolved.RunID || r.Step != s.Index {
			continue
		}
		if r.Phase != s.Phase || r.Tick < 0 || r.Tick >= s.Ticks || r.Seq != s.SeqOffset+r.Tick ||
			(r.Outcome != "ack_success" && r.Outcome != "explicit_failure" && r.Outcome != "unknown") {
			break
		}
		x := &p.snapshot.Steps[i]
		x.Responses++
		p.snapshot.Responses++
		switch r.Outcome {
		case "ack_success":
			x.ACK++
			p.snapshot.ACK++
		case "explicit_failure":
			x.Failure++
			p.snapshot.Failure++
		case "unknown":
			x.Unknown++
			p.snapshot.Unknown++
		}
		if p.seen[r.Seq] {
			p.snapshot.Duplicates++
		} else {
			p.seen[r.Seq] = true
			x.Completed++
			p.snapshot.Completed++
		}
		p.snapshot.LatestPhase = r.Phase
		p.snapshot.State = "observing_completed_responses"
		return
	}
	p.snapshot.Invalid++
}

func (p *progressObserver) read(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil // setup may not have produced a log yet
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() < p.offset {
		return fmt.Errorf("outcome log truncated; progress counts are incomplete")
	}
	if _, err = f.Seek(p.offset, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if err == io.EOF {
			return nil // leave partial writes to be re-read on the next tick
		}
		if err != nil {
			return err
		}
		p.offset += int64(len(line))
		p.consume(line)
	}
}

func (p *progressObserver) emit(dir string, out io.Writer, log io.Writer, exit *int) error {
	if err := p.read(filepath.Join(dir, "k6-log.ndjson")); err != nil {
		p.snapshot.ReadError = err.Error()
	}
	s := &p.snapshot
	s.UpdatedUTC = time.Now().UTC().Format(time.RFC3339Nano)
	s.ElapsedS = time.Since(p.started).Seconds()
	if s.Planned > 0 {
		s.Percent = 100 * float64(s.Completed) / float64(s.Planned)
	}
	if exit != nil {
		s.State, s.ExitCode = "generator_exited", exit
	}
	line := fmt.Sprintf("[generator progress] %s state=%s latest-phase=%s completed=%d/%d (%.1f%%) responses=%d ACK=%d failure=%d unknown=%d duplicates=%d invalid=%d\n",
		s.RunID, s.State, s.LatestPhase, s.Completed, s.Planned, s.Percent, s.Responses, s.ACK, s.Failure, s.Unknown, s.Duplicates, s.Invalid)
	fmt.Fprint(out, line)
	fmt.Fprintf(log, "%s %s", s.UpdatedUTC, line)
	// Atomic replacement prevents readers seeing half a JSON snapshot.
	if err := writeJSON(filepath.Join(dir, "generator-progress.json.tmp"), s); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(dir, "generator-progress.json.tmp"), filepath.Join(dir, "generator-progress.json")); err != nil {
		return err
	}
	md := fmt.Sprintf("# Generator send report: %s\n\nUpdated: %s. State: **%s**.\n\nCompleted ticks: **%d/%d (%.1f%%)**. Observed responses: **%d**.\n\nACK: **%d**; explicit failure: **%d**; unknown: **%d**.\n\n| Phase | Completed/planned ticks | ACK | Failure | Unknown |\n|---|---:|---:|---:|---:|\n",
		s.RunID, s.UpdatedUTC, s.State, s.Completed, s.Planned, s.Percent, s.Responses, s.ACK, s.Failure, s.Unknown)
	for _, x := range s.Steps {
		md += fmt.Sprintf("| %s (step %d) | %d/%d | %d | %d | %d |\n", x.Phase, x.Index, x.Completed, x.Planned, x.ACK, x.Failure, x.Unknown)
	}
	md += fmt.Sprintf("\nDuplicate response records: %d; invalid records: %d.\n\n%s\n", s.Duplicates, s.Invalid, s.Semantics)
	if s.ReadError != "" {
		md += "\nObserver error: " + s.ReadError + "\n"
	}
	if exit != nil {
		md += fmt.Sprintf("\nk6 exit code: %d. This is not a campaign success verdict.\n", *exit)
	}
	return os.WriteFile(filepath.Join(dir, "generator-send-report.md"), []byte(md), 0o644)
}

// A separate context lets the runner stop CPU samplers normally, then request
// one final observation after k6 has closed its log. No workload control here.
func observeProgress(ctx context.Context, dir string, r *plan.Resolved, out io.Writer, exit <-chan int) {
	log, err := os.OpenFile(filepath.Join(dir, "generator-progress.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(out, "generator progress unavailable: %v\n", err)
		return
	}
	defer log.Close()
	p := newProgressObserver(r)
	emit := func(code *int) {
		if err := p.emit(dir, out, log, code); err != nil {
			fmt.Fprintf(out, "generator progress unavailable: %v\n", err)
		}
	}
	emit(nil)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case code := <-exit:
			emit(&code)
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			emit(nil)
		}
	}
}
