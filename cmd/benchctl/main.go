// benchctl runs and checks open-model generator runs (docs/generator.md).
//
//	benchctl resolve   -plan P [-run-id ID]                 print the resolved plan (explicit steps, seq offsets)
//	benchctl run       -plan P [-results results] [-image I] [-cpuset 0-1] [-full-json] [-env K=V ...]
//	benchctl check     -run DIR                             recompute health + manifest verdict from the artifacts
//	benchctl reconcile -run DIR -store FILE                 join the outcome ledger with a fixture/ingester store
//	benchctl expected  -run DIR                             planned messages per step (thinning set from the seed)
//	benchctl arrivals  -run DIR [-store FILE]               inter-arrival statistics, planned vs received (Poisson check)
//	benchctl sendtimes -run DIR                             send lag from k6's own ns sample times (-full-json runs)
//
// Exit status: 0 = VALID / clean, 1 = INVALID / not clean, 2 = usage or I/O error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"iotbench/internal/arrivals"
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
		fmt.Printf("  %-28s rate %7.1f  sends %8d/%-8d  ack %8.0f fail %6.0f unknown %6.0f dropped %4.0f  lag p99<=%v ms  ack p99<=%v ms\n",
			s.Name, s.Rate, s.Sends, s.PlannedSends, s.Outcomes["ack_success"], s.Outcomes["explicit_failure"],
			s.Outcomes["unknown"], s.Outcomes["not_sent"], s.SendLagMs.P99, s.AckLatencyMs.P99)
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
	dir := fs.String("run", "", "run directory")
	store := fs.String("store", "", "store file (fixture NDJSON records)")
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
			Image: *image, Cpuset: *cpuset, FullJSON: *fullJSON, Env: env})
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
		recs, err := fixture.ReadRecords(*store)
		if err != nil {
			die(2, "%v", err)
		}
		var stored []reconcile.Stored
		for _, r := range recs {
			if r.Stored && r.Seq >= 0 {
				stored = append(stored, reconcile.Stored{RunID: r.RunID, Seq: r.Seq})
			}
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
	default:
		die(2, "unknown command %q", cmd)
	}
}
