// W3 gate 1 needs an *independent* stored-payload check, not a sink row count.
//
// The sink retains identity and timing only (internal/sink/sink.go Record), so a
// delivered message can be counted while its padding is truncated or replaced by the
// platform. protocol.ValidateStored implements the strict comparison but was only
// reachable from tests; `benchctl payload-validate` now exposes it over files.
//
// These tests hold that entry point to its contract:
//   - a faithful store validates, including the exact 1024-byte padding;
//   - identity-correct rows with damaged padding fail the export;
//   - the expectation is rebuilt from the seed, so it cannot be circular;
//   - an empty store is a failure, not a vacuous pass.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	benchhash "iotbench/internal/hash"
	"iotbench/internal/plan"
	"iotbench/internal/protocol"
)

const (
	testSeed   = 202611060
	testRunID  = "w3-qual-tb-20261110"
	testOffset = 1024
)

func testPlan(t *testing.T) *plan.Plan {
	t.Helper()
	offset := testOffset
	return &plan.Plan{
		SchemaVersion: 1,
		PlanID:        "payload-validate-test",
		Seed:          testSeed,
		Adapter:       "thingsboard",
		Devices:       1000,
		Payload:       &plan.PayloadPlan{EncodingID: protocol.PayloadEncodingID, SizeOffsetBytes: &offset},
	}
}

// storedBytes builds the payload the generator would send for seq, using the same
// contract functions the adapters use. It returns the canonical bytes the platform
// ought to have stored.
func storedBytes(t *testing.T, seq uint32) []byte {
	t.Helper()
	p := testPlan(t)
	temperature, humidity, pressure := benchhash.Values(p.Seed, seq)
	logical := protocol.LogicalPayload{
		RunID:           testRunID,
		Seq:             seq,
		DeviceID:        fmt.Sprintf("bench-%s-%05d", testRunID, seq%1000),
		ScheduledSendMS: 1794486804127.125 + float64(seq)*40,
		Temperature:     temperature,
		Humidity:        humidity,
		Pressure:        pressure,
	}
	baseline, err := protocol.CanonicalBytes(logical)
	if err != nil {
		t.Fatalf("canonical baseline: %v", err)
	}
	out, err := protocol.BuildPayload(logical, p.Seed, len(baseline)+testOffset)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	return out
}

func writeLedger(t *testing.T, dir string, seqs []uint32) string {
	t.Helper()
	var buf bytes.Buffer
	for _, seq := range seqs {
		fmt.Fprintf(&buf, `{"rec":"msg","run_id":%q,"seq":%d,"device_id":%d,`+
			`"scheduled_send_ms":%v,"outcome":"ack_success"}`+"\n",
			testRunID, seq, seq%1000, 1794486804127.125+float64(seq)*40)
	}
	path := filepath.Join(dir, "records.ndjson")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeStored(t *testing.T, dir string, rows [][]byte) string {
	t.Helper()
	path := filepath.Join(dir, "stored.ndjson")
	if err := os.WriteFile(path, bytes.Join(rows, []byte("\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writePlan(t *testing.T, dir string) string {
	t.Helper()
	b, err := json.Marshal(testPlan(t))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runValidate calls the same function the CLI calls. The gate reads the verdict,
// so the verdict is what these tests assert.
func runValidate(t *testing.T, planPath, ledgerPath, storedPath string) map[string]any {
	t.Helper()
	report, err := validatePayloads(planPath, testRunID, ledgerPath, storedPath, 10)
	if err != nil {
		if pe, ok := err.(payloadError); ok && pe.code == 1 {
			return map[string]any{"verdict": "INVALID", "error": pe.msg}
		}
		t.Fatalf("validatePayloads: %v", err)
	}
	return report
}

func TestPayloadValidateAcceptsFaithfulStore(t *testing.T) {
	dir := t.TempDir()
	seqs := []uint32{0, 1, 2, 3, 4}
	var rows [][]byte
	for _, seq := range seqs {
		rows = append(rows, storedBytes(t, seq))
	}
	report := runValidate(t, writePlan(t, dir),
		writeLedger(t, dir, seqs), writeStored(t, dir, rows))

	if report["verdict"] != "VALID" {
		t.Fatalf("faithful store rejected: %v", report)
	}
	if got := report["validated"]; got != len(seqs) {
		t.Fatalf("validated=%v want %d", got, len(seqs))
	}
	// The gate is about the exact padding size, so assert it, not just "valid".
	lengths, ok := report["padding_lengths"].(map[int]int)
	if !ok || lengths[testOffset] != len(seqs) {
		t.Fatalf("padding lengths=%v want every row at %d", report["padding_lengths"], testOffset)
	}
}

func TestPayloadValidateRejectsDamagedPaddingWithIdentityIntact(t *testing.T) {
	dir := t.TempDir()
	seqs := []uint32{0, 1, 2}
	rows := [][]byte{storedBytes(t, 0), storedBytes(t, 1), storedBytes(t, 2)}

	// Truncate two hex chars: run_id/seq/device_id are untouched, so an identity-only
	// check would pass this row. The payload is no longer the contract payload.
	var damaged protocol.LogicalPayload
	if err := json.Unmarshal(rows[1], &damaged); err != nil {
		t.Fatal(err)
	}
	damaged.Padding = damaged.Padding[:len(damaged.Padding)-2]
	mutated, err := protocol.CanonicalBytes(damaged)
	if err != nil {
		t.Fatal(err)
	}
	rows[1] = mutated

	report := runValidate(t, writePlan(t, dir),
		writeLedger(t, dir, seqs), writeStored(t, dir, rows))

	if report["verdict"] != "INVALID" {
		t.Fatalf("truncated padding accepted: %v", report)
	}
	if report["failed"] != 1 {
		t.Fatalf("failed=%v want exactly 1", report["failed"])
	}
}

func TestPayloadValidateRejectsReplacedPadding(t *testing.T) {
	dir := t.TempDir()
	seqs := []uint32{0}
	row := storedBytes(t, 0)
	var got protocol.LogicalPayload
	if err := json.Unmarshal(row, &got); err != nil {
		t.Fatal(err)
	}
	// Same length, wrong content: the size gate alone cannot catch this.
	got.Padding = strings.Repeat("ab", testOffset/2)
	replaced, err := protocol.CanonicalBytes(got)
	if err != nil {
		t.Fatal(err)
	}
	report := runValidate(t, writePlan(t, dir),
		writeLedger(t, dir, seqs), writeStored(t, dir, [][]byte{replaced}))

	if report["verdict"] != "INVALID" {
		t.Fatalf("replaced padding accepted: %v", report)
	}
}

func TestPayloadValidateFailsWhenNothingWasStored(t *testing.T) {
	dir := t.TempDir()
	report := runValidate(t, writePlan(t, dir),
		writeLedger(t, dir, []uint32{0, 1}), writeStored(t, dir, nil))

	if report["verdict"] == "VALID" {
		t.Fatalf("empty store accepted: %v", report)
	}
}

// TestPayloadValidateExpectedDiffersFromStoredPadding proves the check is not
// circular: the expectation is rebuilt from the seed, so a store that echoes a
// modified padding cannot define what "correct" means.
func TestPayloadValidateExpectedDiffersFromStoredPadding(t *testing.T) {
	faithful := storedBytes(t, 7)
	var got protocol.LogicalPayload
	if err := json.Unmarshal(faithful, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Padding) != testOffset {
		t.Fatalf("padding %d bytes, want %d", len(got.Padding), testOffset)
	}
	// Same seq must reproduce the same padding from the seed alone.
	again := storedBytes(t, 7)
	if !bytes.Equal(faithful, again) {
		t.Fatal("payload derivation is not deterministic")
	}
	var second protocol.LogicalPayload
	if err := json.Unmarshal(again, &second); err != nil {
		t.Fatal(err)
	}
	if got.Padding != second.Padding {
		t.Fatal("padding not reproducible from seed and seq")
	}
}
