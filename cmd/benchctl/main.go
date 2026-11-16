// benchctl runs and checks open-model generator runs (docs/methodology/generator.md).
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
//	benchctl audit     -run DIR -sink FILE [-history FILE] [-deadline-ms 500] [-out FILE] [-strict]
//	                   acknowledged -> sink -> history identity reconciliation and collision audit (internal/audit)
//	benchctl resources -run DIR [-prom-query URL -node HOST:9100] [-pg-counters FILE] [-out FILE]
//	                   EdgeX per-run host disk and PostgreSQL WAL report, informational only (internal/diskmetrics, D29a)
//	benchctl deployment-validate -descriptor FILE -inventory FILE [-observer-fake FILE] [-evidence-manifest FILE] -out NEWDIR
//	                   offline versioned dry-run deployment attestation; no Docker/SUT/lifecycle (internal/deployment)
//	benchctl payload-validate -plan P -run-id ID -ledger records.ndjson -stored stored-payload.ndjson [-out FILE]
//	                   independent W3 stored-payload check: rebuild (seed, seq) and compare stored bytes
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

// commandList is the complete dispatch surface; keep it in sync with main.
const commandList = "resolve|run|check|reconcile|expected|arrivals|sendtimes|explore|audit|resources|cpu|" +
	"deployment-validate|payload-validate|analyze|compare|analyze-demo|fault-capacity"

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
		die(2, "usage: benchctl "+commandList+" [flags]")
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "deployment-validate" {
		deploymentCommand(args)
		return
	}
	if cmd == "payload-validate" {
		payloadValidateCommand(args)
		return
	}
	if cmd == "analyze" || cmd == "compare" || cmd == "analyze-demo" || cmd == "fault-capacity" {
		offlineCommand(cmd, args)
		return
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	planPath := fs.String("plan", "", "plan JSON")
	runID := fs.String("run-id", "", "run id (default: <plan_id>-<utc>-<random>)")
	results := fs.String("results", "results", "results directory")
	image := fs.String("image", defaultImage, "k6 image")
	cpuset := fs.String("cpuset", "0-1", "generator cores")
	fullJSON := fs.Bool("full-json", false, "also write k6's full JSON output (calibration only)")
	promRW := fs.String("prom-rw", "", "Prometheus remote-write URL for live monitoring (e.g. http://127.0.0.1:9090/api/v1/write)")
	cpuSnapshotCmd := fs.String("cpu-snapshot-command", "", "run: trusted shell command emitting a complete SUT cpu-snapshot.sh TSV; preserves captures and brackets measured steps")
	grafana := fs.String("grafana-url", "", "post a run annotation to this Grafana (password: env BENCH_GRAFANA_PASSWORD)")
	dir := fs.String("run", "", "run directory")
	store := fs.String("store", "", "store file: NDJSON with run_id, seq (and optional stored); fixture or history export")
	sinkFile := fs.String("sink", "", "sink NDJSON (cmd/sink)")
	cpuFile := fs.String("cpu", "", "CPU snapshot TSV (monitoring/exporters/cpu-snapshot.sh --loop)")
	history := fs.String("history", "", "history export NDJSON (scripts/history-ids.sh)")
	gate := fs.String("gate", "", "explore: setup gate JSON; pass=false makes the run INVALID (deploy/edgex/scripts/provision-then-scale.sh)")
	persist := fs.String("persist", "", "explore: cumulative stored-count series TSV (time_ms, count), e.g. scripts/persist-series.sh; adds verdict_persistence (D23)")
	check := fs.String("check", "", "explore: post-run platform check JSON; defect=true makes the run FAIL (platform defect)")
	auditOut := fs.String("out", "", "audit: write the JSON report here (stdout otherwise)")
	deadline := fs.Float64("deadline-ms", 500, "audit: SLO deadline for the within-deadline identity count (not a verdict)")
	maxExamples := fs.Int("max-examples", 20, "audit: cap on example seqs/lines in the report")
	strict := fs.Bool("strict", false, "audit: exit 1 when the reconciliation is not clean")
	promQuery := fs.String("prom-query", "", "resources: Prometheus base URL for node-exporter queries (e.g. http://127.0.0.1:9090)")
	node := fs.String("node", "", "resources: node-exporter instance label of the SUT (HOST:9100)")
	pgCounters := fs.String("pg-counters", "", "resources: pg-counters.ndjson from the diagnostic overlay (optional)")
	queryStep := fs.Int("query-step-s", 5, "resources: Prometheus query_range step in seconds")
	cpuBefore := fs.String("before", "", "cpu: boundary snapshot TSV before the measurement interval (cpu-snapshot.sh)")
	cpuAfter := fs.String("after", "", "cpu: boundary snapshot TSV after the measurement interval")
	cpuMessages := fs.Int64("messages", 0, "cpu: explicit denominator count (required unless -audit supplies it)")
	cpuMessagesDef := fs.String("messages-definition", "", "cpu: definition of the denominator, named verbatim (e.g. 'correctness-qualified identities, measured window')")
	cpuMessagesSrc := fs.String("messages-source", "explicit", "cpu: source label for the denominator (e.g. 'explicit', 'benchctl audit -history')")
	cpuAudit := fs.String("audit-file", "", "cpu: audit JSON whose correctness_qualified.identities is the denominator")
	cpuNote := fs.String("note", "", "cpu: free-form note carried into the report")
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
		d, m, err := runner.Run(runner.Options{CPUSnapshotCommand: *cpuSnapshotCmd, PlanPath: *planPath, RepoRoot: repoRoot(), ResultsDir: *results, RunID: *runID,
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
		runExplore(*dir, *sinkFile, *cpuFile, *history, *gate, *check, *persist)
	case "audit":
		runAudit(*dir, *sinkFile, *history, *auditOut, *deadline, *maxExamples, *strict)
	case "resources":
		runResources(*dir, *promQuery, *node, *pgCounters, *queryStep, *auditOut)
	case "cpu":
		runCPU(*cpuBefore, *cpuAfter, *auditOut, *cpuMessages, *cpuMessagesDef, *cpuMessagesSrc, *cpuAudit, *cpuNote)
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
