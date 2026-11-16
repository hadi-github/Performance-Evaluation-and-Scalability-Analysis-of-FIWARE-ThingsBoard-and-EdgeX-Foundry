package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"iotbench/internal/audit"
	"iotbench/internal/report"
)

// runAudit implements the measurement-integrity reconciliation selected on 2026-10-26 (D33). It never writes into
// the run directory unless -out names a path, so preserved evidence cannot be modified by accident.
func runAudit(dir, sinkPath, historyPath, outPath string, deadlineMs float64, maxExamples int, strict bool) {
	if sinkPath == "" {
		die(2, "audit: -sink is required")
	}
	r, sum, health, err := report.Evaluate(dir)
	if err != nil {
		die(2, "run %s: %v", dir, err)
	}
	if sum == nil || health == nil || !health.Valid {
		die(1, "audit: offered-load validity not established; use analyze for claim-level diagnostic output")
	}
	for _, path := range []string{filepath.Join(dir, "records.ndjson"), sinkPath} {
		if _, err := os.Stat(path); err != nil {
			die(2, "audit: required input unavailable: %v", err)
		}
	}
	if historyPath != "" {
		if _, err := os.Stat(historyPath); err != nil {
			die(2, "audit: history unavailable, not an empty export: %v", err)
		}
	}
	recs, err := report.ReadNDJSON[report.Record](filepath.Join(dir, "records.ndjson"))
	if err != nil {
		die(2, "%v", err)
	}
	sinkRows, err := report.ReadNDJSON[audit.SinkRecord](sinkPath)
	if err != nil {
		die(2, "%v", err)
	}
	var hist []audit.HistoryRecord
	hasHistory := historyPath != ""
	if hasHistory {
		type row struct {
			RunID  string   `json:"run_id"`
			Seq    int64    `json:"seq"`
			Device string   `json:"device"`
			Ts     *float64 `json:"ts"`
		}
		rows, err := report.ReadNDJSON[row](historyPath)
		if err != nil {
			die(2, "%v", err)
		}
		for _, x := range rows {
			h := audit.HistoryRecord{RunID: x.RunID, Seq: x.Seq, Device: x.Device}
			if x.Ts != nil {
				h.TsMs, h.HasTs = *x.Ts, true
			}
			hist = append(hist, h)
		}
	}
	res := audit.Analyse(r, sum, recs, sinkRows, hist, hasHistory, audit.Options{DeadlineMs: deadlineMs, MaxExamples: maxExamples})
	b, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		die(2, "audit.json: %v", err)
	}
	if outPath != "" {
		if err := os.WriteFile(outPath, append(b, '\n'), 0o644); err != nil {
			die(2, "%v", err)
		}
	}

	fmt.Printf("== audit %s: planned %d, acknowledged %d, failure %d, unknown %d\n", res.RunID, res.Planned, res.AckSuccess, res.AckFailure, res.Unknown)
	fmt.Printf("  sink:    unique %d, missing %d, extra copies %d, orphans %d, other run ids %d, ambiguous %d\n",
		res.Sink.Unique, res.Sink.Missing, res.Sink.DuplicateCopies, res.Sink.Orphans, res.Sink.OtherRunIDs, len(res.Sink.Ambiguous))
	if hasHistory {
		fmt.Printf("  history: unique %d, missing %d, extra copies %d, orphans %d, other run ids %d, ambiguous %d\n",
			res.History.Unique, res.History.Missing, res.History.DuplicateCopies, res.History.Orphans, res.History.OtherRunIDs, len(res.History.Ambiguous))
	} else {
		fmt.Println("  history: not supplied (store-side correctness unavailable)")
	}
	if q := res.Qualified; q != nil {
		fmt.Printf("  qualified: %d identities, %d within %.0f ms, latency p50 %.1f / p95 %.1f / p99 %.1f ms, rate proxy %.1f/s over %.0f s\n",
			q.Identities, q.WithinDeadline, q.DeadlineMs, q.P50Ms, q.P95Ms, q.P99Ms, q.RatePerS, q.WindowMs/1000)
		for _, s := range q.ByStep {
			fmt.Printf("    %-18s %-8s rate %4.0f/s: planned %6d, qualified %6d, within %6d, p50 %9.1f / p95 %9.1f / p99 %9.1f ms\n",
				s.Name, s.Phase, s.Rate, s.Planned, s.Identities, s.WithinDeadline, s.P50Ms, s.P95Ms, s.P99Ms)
		}
	}
	if c := res.Collisions; c != nil && hasHistory {
		if c.Available {
			fmt.Printf("  collisions: planned same-ms pairs %d, stored same-timestamp groups %d\n", c.PlannedSameMsPairs, c.StoredSameTsGroups)
		} else {
			fmt.Println("  collisions: unavailable (history export lacks device/ts)")
		}
	}
	for _, l := range res.Limitations {
		fmt.Println("  limitation: " + l)
	}
	if outPath == "" {
		fmt.Println(string(b))
	}
	if strict && !res.Clean {
		os.Exit(1)
	}
}
