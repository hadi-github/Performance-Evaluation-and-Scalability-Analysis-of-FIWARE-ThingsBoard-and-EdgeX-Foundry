// benchctl runs and checks open-model generator runs (docs/generator.md).
//
//	benchctl resolve   -plan P [-run-id ID]                 print the resolved plan (explicit steps, seq offsets)
//	benchctl run       -plan P [-results results] [-image I] [-cpuset 0-1] [-full-json] [-env K=V ...]
//	                   [-prom-rw URL] [-grafana-url URL]   (live monitoring stream / run annotation; optional)
//	benchctl check     -run DIR                             recompute health + manifest verdict from the artifacts
//	benchctl reconcile -run DIR -store FILE                 join the outcome ledger with a fixture/ingester store
//	benchctl expected  -run DIR                             planned messages per step (thinning set from the seed)
//	benchctl arrivals  -run DIR [-store FILE]               inter-arrival statistics, planned vs received (Poisson check)
//	benchctl sendtimes -run DIR                             send lag from k6's own ns sample times (-full-json runs)
//	benchctl explore   -run DIR -sink FILE [-cpu TSV] [-history FILE]
//	                   exploratory capacity check: declared pass rule per step, CPU bounds (internal/explore)
//
// Exit status: 0 = VALID / clean, 1 = INVALID / not clean, 2 = usage or I/O error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"iotbench/internal/arrivals"
	"iotbench/internal/explore"
	"iotbench/internal/fixture"
	"iotbench/internal/plan"
	"iotbench/internal/reconcile"
	"iotbench/internal/report"
	"iotbench/internal/runner"
)

const defaultImage = "docker.arvancloud.ir/grafana/k6:1.8.1"

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func die(code int, f string, a ...any) {
	fmt.Fprintf(os.Stderr, f+"\n", a...)
	os.Exit(code)
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", " ")
	fmt.Println(string(b))
}

func repoRoot() string {
	exe, _ := os.Executable()
	for _, d := range []string{".", filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "..")} {
		if _, err := os.Stat(filepath.Join(d, "workloads", "k6", "gen.js")); err == nil {
			a, _ := filepath.Abs(d)
			return a
		}
	}
	die(2, "cannot find workloads/k6/gen.js: run benchctl from the repository root")
	return ""
}

func summarize(m *runner.Manifest) {
	fmt.Printf("== %s: %s\n", m.RunID, m.Verdict)
	for _, s := range m.Health.Steps {
		fmt.Printf("  %-28s rate %7.1f  sends %8d/%-8d  ack %8.0f fail %6.0f unknown %6.0f dropped %4.0f  lag p99 %.1f ms  ack p99 %.1f ms\n",
			s.Name, s.Rate, s.Sends, s.PlannedSends, s.Outcomes["ack_success"], s.Outcomes["explicit_failure"],
			s.Outcomes["unknown"], s.Outcomes["not_sent"], float64(s.SendLagMs.P99), float64(s.AckLatencyMs.P99))
	}
	for _, r := range m.Health.Reasons {
		fmt.Println("  INVALID: " + r)
	}
	for _, w := range m.Health.Warnings {
		fmt.Println("  warning: " + w)
	}
}

func main() {
	if len(os.Args) < 2 {
		die(2, "usage: benchctl resolve|run|check|reconcile|expected [flags]")
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	planPath := fs.String("plan", "", "plan JSON")
	runID := fs.String("run-id", "", "run id (default: <plan_id>-<utc>-<random>)")
	results := fs.String("results", "results", "results directory")
	image := fs.String("image", defaultImage, "k6 image")
	cpuset := fs.String("cpuset", "0-1", "generator cores")
	fullJSON := fs.Bool("full-json", false, "also write k6's full JSON output (calibration only)")
	promRW := fs.String("prom-rw", "", "Prometheus remote-write URL for live monitoring (e.g. http://127.0.0.1:9090/api/v1/write)")
	grafana := fs.String("grafana-url", "", "post a run annotation to this Grafana (password: env BENCH_GRAFANA_PASSWORD)")
	dir := fs.String("run", "", "run directory")
	store := fs.String("store", "", "store file: NDJSON with run_id, seq (and optional stored); fixture or history export")
	sinkFile := fs.String("sink", "", "sink NDJSON (cmd/sink)")
	cpuFile := fs.String("cpu", "", "CPU snapshot TSV (monitoring/exporters/cpu-snapshot.sh --loop)")
	history := fs.String("history", "", "history export NDJSON (scripts/history-ids.sh)")
	gate := fs.String("gate", "", "explore: setup gate JSON; pass=false makes the run INVALID (edgex/scripts/provision-then-scale.sh)")
	check := fs.String("check", "", "explore: post-run platform check JSON; defect=true makes the run FAIL (platform defect)")
	var env multi
	fs.Var(&env, "env", "KEY=VALUE passed to k6 (repeatable; names are recorded, values are not)")
	_ = fs.Parse(args)

	switch cmd {
	case "resolve":
		p, err := plan.Load(*planPath)
		if err != nil {
			die(2, "%v", err)
		}
		id := *runID
		if id == "" {
			id = p.PlanID + "-resolve"
		}
		r, err := p.Resolve(id)
		if err != nil {
			die(2, "%v", err)
		}
		printJSON(r)
	case "run":
		if *planPath == "" {
			die(2, "-plan is required")
		}
		d, m, err := runner.Run(runner.Options{PlanPath: *planPath, RepoRoot: repoRoot(), ResultsDir: *results, RunID: *runID,
			Image: *image, Cpuset: *cpuset, FullJSON: *fullJSON, Env: env, PromRW: *promRW, GrafanaURL: *grafana})
		if err != nil {
			die(2, "run %s: %v", d, err)
		}
		summarize(m)
		fmt.Println("  results: " + d)
		if m.Verdict != "VALID" {
			os.Exit(1)
		}
	case "check":
		var m runner.Manifest
		if err := report.ReadJSON(filepath.Join(*dir, "manifest.json"), &m); err != nil {
			die(2, "%v", err)
		}
		if err := runner.Finish(*dir, &m); err != nil {
			die(2, "%v", err)
		}
		summarize(&m)
		if m.Verdict != "VALID" {
			os.Exit(1)
		}
	case "reconcile":
		stored, err := readStore(*store)
		if err != nil {
			die(2, "%v", err)
		}
		res, err := reconcile.Run(*dir, stored)
		if err != nil {
			die(2, "%v", err)
		}
		b, _ := json.MarshalIndent(res, "", " ")
		_ = os.WriteFile(filepath.Join(*dir, "reconcile.json"), append(b, '\n'), 0o644)
		fmt.Printf("== reconcile %s: planned %d, ack %d, failure %d, unknown %d, stored %d | ack_lost %d, persisted_despite_unknown %d, persisted_despite_failure %d, duplicates %d (+%d copies), orphans %d, other run ids %d -> clean=%v\n",
			res.RunID, res.Planned, res.AckSuccess, res.ExplicitFailure, res.Unknown, res.StoredUnique, len(res.AckLost),
			len(res.PersistedDespiteUnknown), len(res.PersistedDespiteFailure), len(res.Duplicates), res.DuplicateCopies,
			len(res.Orphans), res.OtherRunIDs, res.Clean)
		if !res.Clean {
			os.Exit(1)
		}
	case "expected":
		var r plan.Resolved
		if err := report.ReadJSON(filepath.Join(*dir, "resolved.json"), &r); err != nil {
			die(2, "%v", err)
		}
		for _, s := range r.Steps {
			fmt.Printf("%s\tticks %d\tplanned_sends %d\tseq %d..%d\n", s.Name, s.Ticks, report.PlannedSends(&r, s),
				s.SeqOffset, s.SeqOffset+s.Ticks-1)
		}
	case "arrivals":
		var r plan.Resolved
		if err := report.ReadJSON(filepath.Join(*dir, "resolved.json"), &r); err != nil {
			die(2, "%v", err)
		}
		var recv []arrivals.Received
		if *store != "" {
			recs, err := fixture.ReadRecords(*store)
			if err != nil {
				die(2, "%v", err)
			}
			for _, x := range recs {
				if x.RunID == r.RunID && x.Seq >= 0 {
					recv = append(recv, arrivals.Received{Seq: x.Seq, RecvMs: x.RecvMs, Sched: x.SchedMs})
				}
			}
		}
		rep := arrivals.Analyse(&r, recv)
		b, _ := json.MarshalIndent(rep, "", " ")
		_ = os.WriteFile(filepath.Join(*dir, "arrivals.json"), append(b, '\n'), 0o644)
		for _, s := range rep {
			fmt.Printf("%-24s lambda %7.1f tick %8.1f | planned: n %7d mean %7.3f ms CV %.3f KS %.4f (1%% crit %.4f)",
				s.Name, s.Lambda, s.TickRate, s.Planned.N, s.Planned.MeanMs, s.Planned.CV, s.Planned.KSvsExp, s.Planned.KSCrit01)
			if s.Received != nil {
				fmt.Printf(" | received: n %7d CV %.3f KS %.4f | receipt-scheduled p50 %.2f p99 %.2f ms",
					s.Received.N, s.Received.CV, s.Received.KSvsExp, s.TimingErrP50, s.TimingErrP99)
			}
			fmt.Println()
		}
	case "sendtimes":
		lags, err := arrivals.SendTimes(filepath.Join(*dir, "k6.json.gz"))
		if err != nil {
			die(2, "%v", err)
		}
		s := arrivals.SortedCopy(lags)
		fmt.Printf("authoritative send lag (k6 sample time - http_req_duration - scheduled): n %d  p50 %.3f  p99 %.3f  max %.3f  min %.3f ms\n",
			len(s), arrivals.Quantile(s, 0.5), arrivals.Quantile(s, 0.99), arrivals.Quantile(s, 1), arrivals.Quantile(s, 0))
	case "explore":
		runExplore(*dir, *sinkFile, *cpuFile, *history, *gate, *check)
	default:
		die(2, "unknown command %q", cmd)
	}
}

// readStore reads identities from an NDJSON file with run_id and seq; a "stored" field, if present, must be true.
func readStore(path string) ([]reconcile.Stored, error) {
	type row struct {
		RunID  string `json:"run_id"`
		Seq    *int64 `json:"seq"`
		Stored *bool  `json:"stored"`
	}
	rows, err := report.ReadNDJSON[row](path)
	if err != nil {
		return nil, err
	}
	var out []reconcile.Stored
	for _, r := range rows {
		if r.Seq == nil || *r.Seq < 0 || (r.Stored != nil && !*r.Stored) {
			continue
		}
		out = append(out, reconcile.Stored{RunID: r.RunID, Seq: *r.Seq})
	}
	return out, nil
}

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

func runExplore(dir, sinkPath, cpuPath, historyPath, gatePath, checkPath string) {
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
	res := explore.Evaluate(r, sum, h, obs, snaps)
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
	b, _ := json.MarshalIndent(out, "", " ")
	_ = os.WriteFile(filepath.Join(dir, "explore.json"), append(b, '\n'), 0o644)
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
		fmt.Println()
		for _, reason := range s.Reasons {
			fmt.Println("      - " + reason)
		}
	}
	if rec, ok := out["history_reconciliation"].(*reconcile.Result); ok {
		fmt.Printf("  history: planned %d, stored unique %d, ack_lost %d, persisted_despite_unknown %d, duplicates %d, orphans %d\n",
			rec.Planned, rec.StoredUnique, len(rec.AckLost), len(rec.PersistedDespiteUnknown), len(rec.Duplicates), len(rec.Orphans))
	} else if v, ok := out["history_reconciliation"]; ok {
		fmt.Printf("  history: %v\n", v)
	}
}
