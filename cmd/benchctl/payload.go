// Stored-payload validation: the independent check W3 requires.
//
// W3's gate is not throughput. It is "exactly 1024 extra ASCII logical padding
// bytes ... with live sink/history preservation and independent stored-payload
// validation". An identity-only history export cannot answer that: a platform can
// store the right (run_id, seq) and drop or truncate the padding.
//
// protocol.ValidateStored already implements the strict comparison, but until now it
// was reachable only from Go tests. This subcommand exposes it over files:
//
//	benchctl payload-validate -plan P -run-id ID \
//	    -ledger RUN/records.ndjson -stored stored-payload.ndjson [-out FILE]
//
// The expected payload is reconstructed from the plan's seed and the message's seq
// through the same contract the generator uses (protocol.BuildPayloadOffset), then
// compared against the canonical fields the platform actually stored. Padding is
// never copied from the store into the expectation, so a platform that stored a
// truncated padding fails instead of confirming itself.
//
// Exit status: 0 = every stored row validated, 1 = any row failed or nothing was
// validated, 2 = usage or I/O error.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"

	benchhash "iotbench/internal/hash"
	"iotbench/internal/plan"
	"iotbench/internal/protocol"
)

// storedRow is the canonical projection scripts/history-payload.sh emits: the eight
// logical fields, read out of the platform's own store.
type storedRow struct {
	Seq uint32 `json:"seq"`
}

// ledgerRow is the subset of a complete generator ledger record needed to rebuild the
// expectation. The ledger is the generator's record, not the platform's store, so
// taking identity/timing from it keeps the padding comparison independent.
type ledgerRow struct {
	Seq             *uint32          `json:"seq"`
	DeviceID        *json.RawMessage `json:"device_id"`
	RunID           *string          `json:"run_id"`
	ScheduledSendMS *float64         `json:"scheduled_send_ms"`
	Outcome         string           `json:"outcome"`
	Rec             string           `json:"rec"`
}

func payloadValidateCommand(args []string) {
	fs := flag.NewFlagSet("payload-validate", flag.ExitOnError)
	planPath := fs.String("plan", "", "plan JSON (its seed and payload.size_offset_bytes define the expectation)")
	runID := fs.String("run-id", "", "run id; also the device-name prefix (bench-<run_id>-NNNNN)")
	ledgerPath := fs.String("ledger", "", "generator complete ledger (records.ndjson)")
	storedPath := fs.String("stored", "", "stored-payload NDJSON from scripts/history-payload.sh")
	outPath := fs.String("out", "", "write the report here (stdout otherwise)")
	maxExamples := fs.Int("max-examples", 10, "cap on example seqs listed per failure kind")
	_ = fs.Parse(args)

	if *planPath == "" || *runID == "" || *ledgerPath == "" || *storedPath == "" {
		die(2, "payload-validate requires -plan, -run-id, -ledger and -stored")
	}

	report, err := validatePayloads(*planPath, *runID, *ledgerPath, *storedPath, *maxExamples)
	if err != nil {
		if pe, ok := err.(payloadError); ok {
			die(pe.code, "%s", pe.msg)
		}
		die(2, "%v", err)
	}
	emitJSON(report, *outPath)
	if report["verdict"] != "VALID" {
		os.Exit(1)
	}
}

type payloadError struct {
	code int
	msg  string
}

func (e payloadError) Error() string { return e.msg }

// validatePayloads is the whole check, with no process-exit side effects, so the
// verdict logic can be exercised directly by tests as well as by the CLI.
func validatePayloads(planPath, runID, ledgerPath, storedPath string, maxExamples int) (map[string]any, error) {
	p, err := plan.Load(planPath)
	if err != nil {
		return nil, payloadError{2, fmt.Sprintf("load plan: %v", err)}
	}
	if p.Payload == nil || p.Payload.SizeOffsetBytes == nil {
		return nil, payloadError{2, "plan has no explicit payload.size_offset_bytes; there is nothing to validate"}
	}
	offset := *p.Payload.SizeOffsetBytes
	if p.Payload.EncodingID != protocol.PayloadEncodingID {
		return nil, payloadError{2, fmt.Sprintf("plan payload encoding %q is not %s",
			p.Payload.EncodingID, protocol.PayloadEncodingID)}
	}

	ledger, err := loadLedger(ledgerPath)
	if err != nil {
		return nil, payloadError{2, fmt.Sprintf("load ledger: %v", err)}
	}
	stored, err := loadStored(storedPath)
	if err != nil {
		return nil, payloadError{2, fmt.Sprintf("load stored export: %v", err)}
	}
	if len(stored) == 0 {
		return nil, payloadError{1, "stored export is empty; no payload was preserved, so W3 gate 1 is not met"}
	}

	type failure struct {
		Seq    uint32 `json:"seq"`
		Reason string `json:"reason"`
	}
	failures := []failure{}
	validated := 0
	missingLedger := []uint32{}
	paddingLengths := map[int]int{}
	baselineSizes := map[int]int{}
	targetSizes := map[int]int{}
	totalDelta := 0

	for _, row := range stored {
		rec, ok := ledger[row.Seq]
		if !ok {
			missingLedger = append(missingLedger, row.Seq)
			continue
		}
		expected, baselineBytes, targetBytes, err := expectation(p, runID, rec)
		if err != nil {
			failures = append(failures, failure{Seq: row.Seq, Reason: err.Error()})
			continue
		}
		paddingLengths[len(expected.Padding)]++
		baselineSizes[baselineBytes]++
		targetSizes[targetBytes]++
		totalDelta += len(expected.Padding)
		if err := protocol.ValidateStored(expected, row.bytes); err != nil {
			failures = append(failures, failure{Seq: row.Seq, Reason: err.Error()})
			continue
		}
		validated++
	}

	report := map[string]any{
		"schema_version":               1,
		"check":                        "payload-validate",
		"plan_id":                      p.PlanID,
		"run_id":                       runID,
		"seed":                         p.Seed,
		"encoding_id":                  p.Payload.EncodingID,
		"size_offset_bytes":            offset,
		"stored_rows":                  len(stored),
		"ledger_rows_dispatched":       len(ledger),
		"validated":                    validated,
		"failed":                       len(failures),
		"missing_from_ledger":          len(missingLedger),
		"padding_lengths":              paddingLengths,
		"baseline_bytes":               baselineSizes,
		"target_bytes":                 targetSizes,
		"expected_padding_total_bytes": totalDelta,
	}
	if len(failures) > 0 {
		if len(failures) > maxExamples {
			report["failure_examples"] = failures[:maxExamples]
		} else {
			report["failure_examples"] = failures
		}
	}
	if len(missingLedger) > 0 {
		ex := missingLedger
		if len(ex) > maxExamples {
			ex = ex[:maxExamples]
		}
		report["missing_from_ledger_examples"] = ex
	}

	// The gate is exact: every stored row must validate, and the padding must be the
	// declared size on every row. A pass with a single non-conforming row is not a pass.
	verdict := "VALID"
	if validated != len(stored) || len(missingLedger) > 0 {
		verdict = "INVALID"
	}
	if offset > 0 && len(paddingLengths) == 1 {
		for n := range paddingLengths {
			if n != offset {
				verdict = "INVALID"
			}
		}
	}
	report["verdict"] = verdict

	return report, nil
}

// expectation rebuilds the payload the generator intended for this message.
//
// It derives the padding from (seed, seq) through protocol.BuildPayloadOffset — the
// same contract the adapters use — and takes identity/timing from the generator's own
// ledger. It never reads padding out of the platform's store.
func expectation(p *plan.Plan, runID string, rec ledgerRow) (protocol.LogicalPayload, int, int, error) {
	if rec.Seq == nil {
		return protocol.LogicalPayload{}, 0, 0, fmt.Errorf("ledger row has no seq")
	}
	if rec.ScheduledSendMS == nil {
		return protocol.LogicalPayload{}, 0, 0, fmt.Errorf("ledger row has no scheduled_send_ms")
	}
	index, err := deviceIndex(rec)
	if err != nil {
		return protocol.LogicalPayload{}, 0, 0, err
	}
	deviceID := fmt.Sprintf("bench-%s-%05d", runID, index)
	temperature, humidity, pressure := benchhash.Values(p.Seed, *rec.Seq)
	logical := protocol.LogicalPayload{
		RunID:           runID,
		Seq:             *rec.Seq,
		DeviceID:        deviceID,
		ScheduledSendMS: *rec.ScheduledSendMS,
		Temperature:     temperature,
		Humidity:        humidity,
		Pressure:        pressure,
		Padding:         "",
	}
	baseline, err := protocol.CanonicalBytes(logical)
	if err != nil {
		return protocol.LogicalPayload{}, 0, 0, err
	}
	if _, err := protocol.BuildPayloadOffset(logical, p.Seed, *p.Payload.SizeOffsetBytes); err != nil {
		return protocol.LogicalPayload{}, 0, 0, err
	}
	target := len(baseline) + *p.Payload.SizeOffsetBytes
	// Recompute through the contract so the expected padding is the canonical one.
	built, err := protocol.BuildPayload(logical, p.Seed, target)
	if err != nil {
		return protocol.LogicalPayload{}, 0, 0, err
	}
	var expected protocol.LogicalPayload
	if err := json.Unmarshal(built, &expected); err != nil {
		return protocol.LogicalPayload{}, 0, 0, err
	}
	return expected, len(baseline), target, nil
}

// deviceIndex reads the device slot from a ledger record, which is a JSON number.
func deviceIndex(rec ledgerRow) (int, error) {
	if rec.DeviceID == nil {
		return 0, fmt.Errorf("ledger row has no device_id")
	}
	raw := bytes.TrimSpace(*rec.DeviceID)
	if len(raw) == 0 || raw[0] == '"' {
		return 0, fmt.Errorf("ledger device_id is not a numeric slot: %s", raw)
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil {
		return 0, fmt.Errorf("ledger device_id %s is not an integer slot", raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("ledger device_id %d is negative", n)
	}
	return n, nil
}

// loadLedger returns every message the generator dispatched, whatever its outcome.
//
// An earlier version kept only `ack_success` rows. That was wrong for W3: on
// ThingsBoard the 370 warm-up messages whose acknowledgement timed out during the
// cold-start ramp were nevertheless accepted and stored by the platform, so the
// stored export contains them. Dropping them from the expectation made a correct
// store look like it held 370 unexplained rows.
//
// Payload preservation and delivery are different questions. A row is included here
// because the generator has an identity+timing record for it; whether the platform
// acknowledged it in time is the dispatch analysis's business, not this check's.
func loadLedger(path string) (map[uint32]ledgerRow, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[uint32]ledgerRow{}
	for i, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var rec ledgerRow
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("ledger line %d: %w", i+1, err)
		}
		if rec.Rec != "" && rec.Rec != "msg" {
			continue
		}
		if rec.Seq == nil {
			continue
		}
		out[*rec.Seq] = rec
	}
	return out, nil
}

type storedEntry struct {
	Seq   uint32
	bytes []byte
}

func loadStored(path string) ([]storedEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := []storedEntry{}
	for i, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var head storedRow
		if err := json.Unmarshal(line, &head); err != nil {
			return nil, fmt.Errorf("stored line %d: %w", i+1, err)
		}
		out = append(out, storedEntry{Seq: head.Seq, bytes: append([]byte(nil), bytes.TrimSpace(line)...)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

func emitJSON(v any, path string) {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		die(2, "%v", err)
	}
	if path == "" {
		fmt.Println(string(b))
		return
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		die(2, "write report: %v", err)
	}
}
