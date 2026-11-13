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
	"strconv"
	"strings"

	"iotbench/internal/arrivals"
	"iotbench/internal/audit"
	"iotbench/internal/cpubound"
	"iotbench/internal/diskmetrics"
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
		die(2, "usage: benchctl resolve|run|check|reconcile|expected|audit|cpu|analyze|compare|analyze-demo|fault-capacity|deployment-validate [flags]")
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

// runCPU builds the CPU-boundary artifact of the run bundle:
//
//	benchctl cpu -before before.tsv -after after.tsv [-audit audit.json | -messages N -messages-definition ...]
//	[-out FILE] [-note ...]
//
// Snapshots come from monitoring/exporters/cpu-snapshot.sh run on the SUT immediately before and after the
// measurement interval. The denominator is named explicitly (who supplies N and its definition); the per-message
// cost is absent, not zero, whenever the counters or the denominator are unavailable.
func runCPU(beforePath, afterPath, outPath string, messages int64, messagesDef, messagesSrc, auditPath, note string) {
	if beforePath == "" || afterPath == "" {
		die(2, "cpu: -before and -after snapshot files are required")
	}
	bf, err := os.Open(beforePath)
	if err != nil {
		die(2, "%v", err)
	}
	before, err := cpubound.Parse(bf)
	bf.Close()
	if err != nil {
		die(2, "%s: %v", beforePath, err)
	}
	af, err := os.Open(afterPath)
	if err != nil {
		die(2, "%v", err)
	}
	after, err := cpubound.Parse(af)
	af.Close()
	if err != nil {
		die(2, "%s: %v", afterPath, err)
	}
	var denom *cpubound.Denominator
	switch {
	case messages > 0:
		denom = &cpubound.Denominator{Count: messages, Source: messagesSrc,
			Definition: messagesDef, Present: true}
	case auditPath != "":
		b, err := os.ReadFile(auditPath)
		if err != nil {
			die(2, "%v", err)
		}
		var a struct {
			Qualified *struct {
				Identities int `json:"identities"`
			} `json:"correctness_qualified"`
		}
		if err := json.Unmarshal(b, &a); err != nil {
			die(2, "%s: %v", auditPath, err)
		}
		if a.Qualified == nil || a.Qualified.Identities <= 0 {
			// unavailable stays unavailable: the cost claim does not get a made-up denominator
			denom = &cpubound.Denominator{}
			note += "; denominator from audit is unavailable"
		} else {
			denom = &cpubound.Denominator{Count: int64(a.Qualified.Identities), Source: auditPath,
				Definition: "correctness-qualified identities (acknowledged AND at the sink AND retrievable from history, measured window)", Present: true}
		}
	}
	rep := cpubound.Diff(before, after, denom, note)
	out := os.Stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			die(2, "%v", err)
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		die(2, "%v", err)
	}
}

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
