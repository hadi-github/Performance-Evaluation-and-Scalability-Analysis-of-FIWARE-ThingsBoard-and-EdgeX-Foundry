package main

import (
	"encoding/json"
	"os"

	"iotbench/internal/cpubound"
)

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
