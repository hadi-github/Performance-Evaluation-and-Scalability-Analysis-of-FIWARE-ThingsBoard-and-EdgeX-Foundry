package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"iotbench/internal/explore"
	"iotbench/internal/reconcile"
	"iotbench/internal/report"
)

func readJSON(path string, v any) json.RawMessage {
	b, err := os.ReadFile(path)
	if err != nil {
		die(2, "%v", err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		die(2, "%s: %v", path, err)
	}
	return b
}

// readStoreSeries reads "time_ms<TAB>count" lines ('#' starts a comment).
func readStoreSeries(path string) ([]explore.StorePoint, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []explore.StorePoint
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) < 2 {
			return nil, fmt.Errorf("%s: bad line %q", path, l)
		}
		t, err1 := strconv.ParseFloat(f[0], 64)
		c, err2 := strconv.ParseFloat(f[1], 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("%s: bad line %q", path, l)
		}
		out = append(out, explore.StorePoint{TMs: t, Count: c})
	}
	return out, nil
}

func readSnapshots(path string) ([]explore.Snapshot, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []explore.Snapshot
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Split(l, "\t")
		if len(f) < 6 {
			continue
		}
		t, err1 := strconv.ParseFloat(f[0], 64)
		u, err2 := strconv.ParseInt(f[3], 10, 64)
		if err1 != nil || err2 != nil {
			continue // NA usage (container gone mid-snapshot)
		}
		out = append(out, explore.Snapshot{TimeS: t, Container: f[1], ID: f[2], UsageUsec: u, StartedAt: f[4], Cpuset: f[5]})
	}
	return out, nil
}

func runExplore(dir, sinkPath, cpuPath, historyPath, gatePath, checkPath, persistPath string) {
	r, sum, h, err := report.Evaluate(dir)
	if err != nil || sum == nil {
		die(2, "run %s: %v (summary present: %v)", dir, err, sum != nil)
	}
	type sinkRow struct {
		RunID  string  `json:"run_id"`
		Seq    int64   `json:"seq"`
		RecvMs float64 `json:"recv_ms"`
	}
	rows, err := report.ReadNDJSON[sinkRow](sinkPath)
	if err != nil {
		die(2, "%v", err)
	}
	var obs []explore.SinkObs
	for _, x := range rows {
		obs = append(obs, explore.SinkObs{RunID: x.RunID, Seq: x.Seq, RecvMs: x.RecvMs})
	}
	snaps, err := readSnapshots(cpuPath)
	if err != nil {
		die(2, "%v", err)
	}
	store, err := readStoreSeries(persistPath)
	if err != nil {
		die(2, "%v", err)
	}
	res := explore.Evaluate(r, sum, h, obs, snaps, store)
	if res.Persist != nil {
		res.Persist.Source = persistPath
	}
	out := map[string]any{"explore": res}
	if historyPath != "" {
		stored, err := readStore(historyPath)
		if err != nil {
			die(2, "%v", err)
		}
		if rec, err := reconcile.Run(dir, stored); err != nil {
			out["history_reconciliation"] = "refused: " + err.Error()
		} else {
			out["history_reconciliation"] = rec
		}
	}
	// run-level verdicts declared with the EdgeX procedure: a failed setup gate makes the run INVALID; a platform defect
	// seen after the run (core-metadata panic/restart) makes it FAIL, whatever the per-step results
	runVerdict := ""
	if gatePath != "" {
		var g struct {
			Pass    bool     `json:"pass"`
			Reasons []string `json:"reasons"`
		}
		raw := readJSON(gatePath, &g)
		out["setup_gate"] = raw
		if !g.Pass {
			runVerdict = "INVALID (setup failure): " + strings.Join(g.Reasons, "; ")
		}
	}
	if checkPath != "" {
		var c struct {
			Defect  bool   `json:"defect"`
			Verdict string `json:"verdict"`
		}
		raw := readJSON(checkPath, &c)
		out["platform_check"] = raw
		if c.Defect && runVerdict == "" {
			runVerdict = c.Verdict
		}
	}
	if runVerdict != "" {
		out["run_verdict"] = runVerdict
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		die(2, "explore.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "explore.json"), append(b, '\n'), 0o644); err != nil {
		die(2, "%v", err)
	}
	if runVerdict != "" {
		fmt.Printf("== RUN VERDICT %s: %s\n", res.RunID, runVerdict)
	}

	fmt.Printf("== explore %s: highest passing %v msg/s, first non-pass %v msg/s (%s); sink: %d of %d planned reached, %d duplicate receipts\n",
		res.RunID, res.HighestPassRate, res.FirstFailRate, res.FirstFailVerdict, res.ReachedSinkOfPlan, res.Planned, res.SinkDuplicates)
	for _, s := range res.Steps {
		var mins []string
		for _, m := range s.Minutes {
			mins = append(mins, fmt.Sprintf("%.0f", m.P99Ms))
		}
		var outs []string
		for _, o := range s.OutstandingMin {
			outs = append(outs, fmt.Sprintf("%.1f", o))
		}
		fmt.Printf("  %-26s %-13s unsucc %.4f%%  lag p99 %5.1f ms  ack p99/min [%s] ms  outstanding/min [%s]",
			s.Name, s.Verdict, s.Unsuccessful*100, s.SendLagP99Ms, strings.Join(mins, " "), strings.Join(outs, " "))
		if s.MinBContainer != "" {
			fmt.Printf("  bound %.0f msg/s (%s)", s.MinB, s.MinBContainer)
		}
		if s.VerdictPersist != "" {
			var po []string
			for _, o := range s.OutstandingPersistMin {
				po = append(po, fmt.Sprintf("%.1f", o))
			}
			fmt.Printf("\n  %-26s persistence: %-13s outstanding/min [%s]", "", s.VerdictPersist, strings.Join(po, " "))
		}
		fmt.Println()
		for _, reason := range s.Reasons {
			fmt.Println("      - " + reason)
		}
	}
	if p := res.Persist; p != nil {
		fmt.Printf("  persistence (D23): highest passing %v msg/s, first non-pass %v msg/s (%s); %d points, median spacing %.0f ms, stored at last point %.0f\n",
			p.HighestPassRate, p.FirstFailRate, p.FirstFailVerdict, p.Points, p.MedianSpacingMs, p.StoredAtLastPoint)
	}
	if rec, ok := out["history_reconciliation"].(*reconcile.Result); ok {
		fmt.Printf("  history: planned %d, stored unique %d, ack_lost %d, persisted_despite_unknown %d, duplicates %d, orphans %d\n",
			rec.Planned, rec.StoredUnique, len(rec.AckLost), len(rec.PersistedDespiteUnknown), len(rec.Duplicates), len(rec.Orphans))
	} else if v, ok := out["history_reconciliation"]; ok {
		fmt.Printf("  history: %v\n", v)
	}
}
