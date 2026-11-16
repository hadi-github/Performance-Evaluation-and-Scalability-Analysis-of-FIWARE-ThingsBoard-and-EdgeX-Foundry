package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"iotbench/internal/diskmetrics"
	"iotbench/internal/plan"
	"iotbench/internal/report"
)

// runResources writes the D29a per-run report for an EdgeX run: host/device disk statistics (node-exporter) and, when
// the diagnostic overlay ran, PostgreSQL WAL counters. Missing sources are reported with a reason, never as zero. The
// output is informational and is not read by any verdict rule. Without -out the JSON goes to stdout, so no evidence
// directory is written by accident.
func runResources(dir, promBase, node, pgPath string, stepS int, outPath string) {
	var r plan.Resolved
	if err := report.ReadJSON(filepath.Join(dir, "resolved.json"), &r); err != nil {
		die(2, "%v", err)
	}
	var sum report.Summary
	if err := report.ReadJSON(filepath.Join(dir, "k6-summary.json"), &sum); err != nil {
		die(2, "%v", err)
	}
	if r.Adapter != "edgex" {
		die(2, "resources: D29a reporting applies to EdgeX runs (adapter %q)", r.Adapter)
	}
	if stepS <= 0 {
		die(2, "resources: -query-step-s must be positive")
	}
	var samples []diskmetrics.PGSample
	if pgPath != "" {
		s, skipped, err := diskmetrics.ReadPGCounters(pgPath)
		if err != nil {
			die(2, "%v", err)
		}
		samples = s
		if skipped > 0 {
			fmt.Fprintf(os.Stderr, "warning: %d pg-counters record(s) could not be parsed and were skipped\n", skipped)
		}
	}
	out := diskmetrics.Resources{SchemaVersion: diskmetrics.SchemaVersion, RunID: r.RunID, Platform: "edgex",
		Scope: "EdgeX per-run disk and WAL measurements (D29a); informational only, not a verdict input"}
	for i, st := range r.Steps {
		start := float64(st.StartS) * 1000
		if i < len(sum.Steps) && sum.Steps[i].ScenarioStartMs > 0 {
			start = sum.Steps[i].ScenarioStartMs
		} else if sum.SetupEndMs > 0 {
			start = sum.SetupEndMs + float64(st.StartS)*1000
		}
		end := start + float64(st.DurationS)*1000
		out.Steps = append(out.Steps, diskmetrics.StepResources{
			Name: st.Name, Phase: st.Phase, StartMs: start, EndMs: end,
			Disk: diskmetrics.Disk(promBase, node, stepS, start, end, nil),
			WAL:  diskmetrics.WAL(samples, start, end),
		})
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		die(2, "resources: %v", err)
	}
	if outPath == "" {
		fmt.Println(string(b))
		return
	}
	if err := os.WriteFile(outPath, append(b, '\n'), 0o644); err != nil {
		die(2, "%v", err)
	}
	fmt.Printf("== resources %s: %d steps written to %s\n", out.RunID, len(out.Steps), outPath)
	for _, s := range out.Steps {
		disk := "host disk " + s.Disk.Status
		if s.Disk.Status != "ok" {
			disk += " (" + s.Disk.Reason + ")"
		}
		wal := "postgres WAL " + s.WAL.Status
		if s.WAL.Status != "ok" {
			wal += " (" + s.WAL.Reason + ")"
		}
		fmt.Printf("  %-18s %s; %s\n", s.Name, disk, wal)
	}
}
