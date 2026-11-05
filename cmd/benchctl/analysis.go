package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"iotbench/internal/analysis"
)

func offlineCommand(command string, args []string) {
	fs := flag.NewFlagSet(command, flag.ExitOnError)
	out := fs.String("out", "", "NEW output directory outside retained evidence (required)")
	run := fs.String("run", "", "read-only run directory")
	evidence := fs.String("evidence", "", "capture/provenance spec JSON; source paths relative to run")
	policy := fs.String("profiles", "", "explicit profile policy JSON; omitted profiles use 2026-11-01 owner-approved defaults")
	protocolPath := fs.String("protocol", "", "verified protocol JSON required for protocol-bound analysis")
	protocolRoot := fs.String("protocol-root", "", "repository root used to verify the protocol's pinned profile")
	input := fs.String("in", "", "fault-capacity: versioned offline fault/capacity evidence JSON")
	var inputs multi
	fs.Var(&inputs, "analysis", "compare: analysis.json path (repeatable)")
	_ = fs.Parse(args)
	if *out == "" {
		die(2, "-out must name a new output directory")
	}
	switch command {
	case "analyze-demo":
		if e := analysis.Demo(*out); e != nil {
			die(2, "%v", e)
		}
	case "fault-capacity":
		if *input == "" {
			die(2, "-in required")
		}
		b, e := os.ReadFile(*input)
		if e != nil {
			die(2, "%v", e)
		}
		ev, e := analysis.DecodeFaultCapacityEvidence(b)
		if e != nil {
			die(2, "%v", e)
		}
		report, e := analysis.ReduceFaultCapacityEvidence(ev)
		if e != nil {
			die(2, "%v", e)
		}
		if e = analysis.WriteFaultCapacity(*out, report, []string{*input}); e != nil {
			die(2, "%v", e)
		}
	case "analyze":
		if *run == "" {
			die(2, "-run required")
		}
		r, e := analysis.Analyze(analysis.Options{Run: *run, Evidence: *evidence, Policy: *policy, ProtocolPath: *protocolPath, ProtocolRoot: *protocolRoot})
		if e != nil {
			die(2, "%v", e)
		}
		if e = analysis.Write(*out, r, *run); e != nil {
			die(2, "%v", e)
		}
	case "compare":
		if len(inputs) == 0 {
			die(2, "at least one -analysis file required")
		}
		var results []*analysis.Result
		var dirs []string
		for _, path := range inputs {
			b, e := os.ReadFile(path)
			if e != nil {
				die(2, "%v", e)
			}
			var r analysis.Result
			if e = json.Unmarshal(b, &r); e != nil {
				die(2, "%v", e)
			}
			if r.SchemaVersion != 1 {
				die(2, "unsupported analysis schema")
			}
			results = append(results, &r)
			dirs = append(dirs, filepath.Dir(path))
		}
		if e := analysis.WriteComparison(*out, analysis.Compare(results), dirs); e != nil {
			die(2, "%v", e)
		}
	}
	fmt.Println(*out)
}
