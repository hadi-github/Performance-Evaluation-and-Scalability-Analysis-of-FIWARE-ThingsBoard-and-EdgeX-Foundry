package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/report"
)

func progressPlan() *plan.Resolved {
	return &plan.Resolved{RunID: "progress-test", TotalTicks: 4, Steps: []plan.Step{
		{Index: 0, Phase: "warmup", Ticks: 1, SeqOffset: 0},
		{Index: 1, Phase: "measurement", Ticks: 3, SeqOffset: 1},
	}}
}

func progressLine(seq int64, outcome string) []byte {
	step, phase, tick := 1, "measurement", seq-1
	if seq == 0 {
		step, phase, tick = 0, "warmup", 0
	}
	r, _ := json.Marshal(report.Record{Rec: "msg", RunID: "progress-test", Seq: seq,
		Step: step, Phase: phase, Tick: tick, Outcome: outcome})
	b, _ := json.Marshal(map[string]string{"source": "console", "msg": string(r)})
	return append(b, '\n')
}

func TestProgressCountsPhasesAndSeparatesACKFromDelivery(t *testing.T) {
	p := newProgressObserver(progressPlan())
	for seq, outcome := range []string{"ack_success", "explicit_failure", "unknown", "ack_success"} {
		p.consume(progressLine(int64(seq), outcome))
	}
	p.consume(progressLine(3, "ack_success"))
	p.consume(progressLine(5, "ack_success")) // out of plan; cannot inflate completion
	p.consume([]byte(`{"source":"console","msg":"secret setup credential"}`))
	p.consume(bytes.ReplaceAll(progressLine(0, "ack_success"), []byte("progress-test"), []byte("other-run")))
	d := t.TempDir()
	var out, log bytes.Buffer
	code := 1
	if err := p.emit(d, &out, &log, &code); err != nil {
		t.Fatal(err)
	}
	s := p.snapshot
	if s.Completed != 4 || s.Percent != 100 || s.Responses != 5 || s.ACK != 3 || s.Failure != 1 || s.Unknown != 1 || s.Duplicates != 1 || s.Invalid != 2 {
		t.Fatalf("incorrect accounting: %+v", s)
	}
	if s.Steps[0].ACK != 1 || s.Steps[1].Completed != 3 || s.State != "generator_exited" || *s.ExitCode != 1 {
		t.Fatalf("incorrect phase/state: %+v", s)
	}
	md, err := os.ReadFile(filepath.Join(d, "generator-send-report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"ACK is not sink/history delivery", "not a campaign success verdict", "k6 exit code: 1", "measurement (step 1)"} {
		if !strings.Contains(string(md), text) {
			t.Errorf("missing %q in report", text)
		}
	}
	if strings.Contains(out.String()+log.String()+string(md), "secret") {
		t.Fatal("raw diagnostic leaked into progress report")
	}
}

func TestProgressIncrementalPartialLinesAndNoRawMutation(t *testing.T) {
	d := t.TempDir()
	path := filepath.Join(d, "k6-log.ndjson")
	p := newProgressObserver(progressPlan())
	first, second := progressLine(0, "ack_success"), progressLine(1, "unknown")
	partial := append(append([]byte{}, first...), second[:len(second)/2]...)
	if err := os.WriteFile(path, partial, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.read(path); err != nil {
		t.Fatal(err)
	}
	if p.snapshot.Completed != 1 || p.offset != int64(len(first)) {
		t.Fatalf("partial line counted: %+v", p.snapshot)
	}
	if err := p.read(path); err != nil {
		t.Fatal(err)
	}
	if p.snapshot.Responses != 1 {
		t.Fatal("incremental reader counted the first line twice")
	}
	full := append(append([]byte{}, first...), second...)
	if err := os.WriteFile(path, full, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, log bytes.Buffer
	if err := p.emit(d, &out, &log, nil); err != nil {
		t.Fatal(err)
	}
	if p.snapshot.Completed != 2 || p.snapshot.Unknown != 1 || p.snapshot.Percent != 50 {
		t.Fatalf("wrong partial progress: %+v", p.snapshot)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, full) {
		t.Fatal("observer modified original log")
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.emit(d, &out, &log, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.snapshot.ReadError, "truncated") {
		t.Fatal("truncation not surfaced")
	}
}

func TestProgressSetupAbortNeverInventsSends(t *testing.T) {
	d := t.TempDir()
	exit := make(chan int, 1)
	exit <- 108
	var out bytes.Buffer
	observeProgress(context.Background(), d, progressPlan(), &out, exit)
	b, err := os.ReadFile(filepath.Join(d, "generator-progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s progressSnapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Completed != 0 || s.ACK != 0 || s.Percent != 0 || s.LatestPhase != "none" || s.State != "generator_exited" || *s.ExitCode != 108 {
		t.Fatalf("setup abort misreported: %+v", s)
	}
	if !strings.Contains(out.String(), "completed=0/4") {
		t.Fatal("missing console progress")
	}
	before, _ := os.ReadFile(filepath.Join(d, "generator-progress.log"))
	observeProgress(context.Background(), d, progressPlan(), &out, exit)
	after, _ := os.ReadFile(filepath.Join(d, "generator-progress.log"))
	if !bytes.Equal(before, after) {
		t.Fatal("existing progress log overwritten")
	}
}

func TestProgressMissingOrNullIdentityCannotBecomeFirstTick(t *testing.T) {
	for _, key := range []string{"run_id", "seq", "rate_step", "phase", "tick", "outcome"} {
		for _, null := range []bool{false, true} {
			t.Run(key+fmt.Sprint(null), func(t *testing.T) {
				var envelope map[string]string
				if err := json.Unmarshal(progressLine(0, "ack_success"), &envelope); err != nil {
					t.Fatal(err)
				}
				var fields map[string]any
				if err := json.Unmarshal([]byte(envelope["msg"]), &fields); err != nil {
					t.Fatal(err)
				}
				delete(fields, key)
				if null {
					fields[key] = nil
				}
				msg, _ := json.Marshal(fields)
				envelope["msg"] = string(msg)
				line, _ := json.Marshal(envelope)
				p := newProgressObserver(progressPlan())
				p.consume(line)
				if p.snapshot.Completed != 0 || p.snapshot.ACK != 0 || p.snapshot.Invalid != 1 {
					t.Fatalf("missing/null identity counted: %+v", p.snapshot)
				}
			})
		}
	}
}
