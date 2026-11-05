package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	benchhash "iotbench/internal/hash"
)

// payloadCohortGolden is a synthetic engineering test vector, not a second encoder. Expected
// bytes are compared against the real BuildPayload/CanonicalBytes implementation on both sides.
type payloadCohortGolden struct {
	SchemaVersion int    `json:"schema_version"`
	Label         string `json:"label"`
	Provenance    struct {
		ProtocolID       string   `json:"protocol_id"`
		ProtocolSHA256   string   `json:"protocol_sha256"`
		ProfileSHA256    string   `json:"profile_sha256"`
		EncodingID       string   `json:"encoding_id"`
		FieldOrder       []string `json:"field_order"`
		PaddingSalt      string   `json:"padding_salt"`
		SizePointsStatus string   `json:"size_points_status"`
		Note             string   `json:"note"`
	} `json:"provenance"`
	Seed      uint32 `json:"seed"`
	RunID     string `json:"run_id"`
	Telemetry struct {
		Temperature int `json:"temperature"`
		Humidity    int `json:"humidity"`
		Pressure    int `json:"pressure"`
	} `json:"telemetry"`
	SizeOffsets []int `json:"size_offsets_bytes"`
	CohortStep  struct {
		Index int    `json:"index"`
		Name  string `json:"step_name"`
		Phase string `json:"phase"`
		Note  string `json:"note"`
	} `json:"cohort_step"`
	Cohort    []payloadCohortMessage `json:"cohort"`
	EdgeCases []payloadCohortMessage `json:"edge_cases"`
}

type payloadCohortMessage struct {
	Name                   string  `json:"name"`
	Seq                    uint32  `json:"seq"`
	DeviceID               string  `json:"device_id"`
	ScheduledSendMS        float64 `json:"scheduled_send_ms"`
	TimestampToken         string  `json:"timestamp_token"`
	ExpectedBaseline       string  `json:"expected_baseline"`
	ExpectedBaselineSHA256 string  `json:"expected_baseline_sha256"`
	Sizes                  []struct {
		OffsetBytes   int    `json:"offset_bytes"`
		TargetBytes   int    `json:"target_bytes"`
		SHA256        string `json:"sha256"`
		PaddingLength int    `json:"padding_length"`
		PaddingSHA256 string `json:"padding_sha256"`
	} `json:"sizes"`
}

func loadPayloadCohortGolden(t *testing.T) payloadCohortGolden {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "tests", "offline", "payload-cohort-golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g payloadCohortGolden
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	return g
}

func (g payloadCohortGolden) logical(m payloadCohortMessage) LogicalPayload {
	return LogicalPayload{RunID: g.RunID, Seq: m.Seq, DeviceID: m.DeviceID, ScheduledSendMS: m.ScheduledSendMS,
		Temperature: g.Telemetry.Temperature, Humidity: g.Telemetry.Humidity, Pressure: g.Telemetry.Pressure}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestPayloadCohortGoldenIsSyntheticAndBoundToProtocol(t *testing.T) {
	g := loadPayloadCohortGolden(t)
	if g.SchemaVersion != 1 || !strings.Contains(g.Label, "SYNTHETIC OFFLINE ENGINEERING FIXTURE ONLY") || !strings.Contains(g.Label, "not live evidence") || !strings.Contains(g.Label, "not storage proof") {
		t.Fatalf("fixture is not explicitly synthetic-only: %q", g.Label)
	}
	if g.Provenance.SizePointsStatus != "proposed_not_qualified" || g.Provenance.EncodingID != "logical-json-ascii-v1" || g.Provenance.PaddingSalt != "0x50414431" {
		t.Fatalf("payload contract provenance changed: %+v", g.Provenance)
	}
	if len(g.SizeOffsets) != 3 || g.SizeOffsets[0] != 0 || g.SizeOffsets[1] != 1024 || g.SizeOffsets[2] != 4096 {
		t.Fatalf("size points are not the proposed engineering points: %v", g.SizeOffsets)
	}
	if len(g.Cohort) != 5 || g.CohortStep.Phase != "measurement" || g.CohortStep.Index != 1 {
		t.Fatalf("M1b cohort mapping missing: %+v %d", g.CohortStep, len(g.Cohort))
	}
	for i, m := range g.Cohort {
		wantSeq := uint32(i + 2)
		if m.Seq != wantSeq || m.DeviceID != "device-"+string(rune('0'+wantSeq%5)) {
			t.Fatalf("cohort mapping %d: seq=%d device=%s", i, m.Seq, m.DeviceID)
		}
	}
	p, err := LoadVerified(filepath.Join("..", "..", "research", "protocols", "iotbench-telemetry-v0.1.0-pilot.1.json"), filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	d, err := p.Definition()
	if err != nil {
		t.Fatal(err)
	}
	if g.Provenance.ProtocolID != d.ProtocolID || g.Provenance.ProtocolSHA256 != p.SHA256() || g.Provenance.ProfileSHA256 != d.Profiles.SHA256 {
		t.Fatalf("fixture provenance does not match loaded protocol/profile: %+v", g.Provenance)
	}
	if len(g.Provenance.FieldOrder) != len(d.Payload.FieldOrder) {
		t.Fatal("field order differs")
	}
	for i := range d.Payload.FieldOrder {
		if g.Provenance.FieldOrder[i] != d.Payload.FieldOrder[i] {
			t.Fatalf("field order[%d]=%q want %q", i, g.Provenance.FieldOrder[i], d.Payload.FieldOrder[i])
		}
	}
}

func TestPayloadCohortGoldenCanonicalBuildAndPaddingReconstruction(t *testing.T) {
	g := loadPayloadCohortGolden(t)
	messages := append(append([]payloadCohortMessage(nil), g.Cohort...), g.EdgeCases...)
	if len(messages) < 10 {
		t.Fatalf("fixture needs several cohort and edge messages, got %d", len(messages))
	}
	for _, m := range messages {
		t.Run(m.Name, func(t *testing.T) {
			p := g.logical(m)
			base, err := CanonicalBytes(p)
			if err != nil {
				t.Fatal(err)
			}
			if string(base) != m.ExpectedBaseline || len(base) != len(m.ExpectedBaseline) {
				t.Fatalf("canonical bytes\n got: %s\nwant: %s", base, m.ExpectedBaseline)
			}
			if got := sha256Hex(base); got != m.ExpectedBaselineSHA256 {
				t.Fatalf("baseline sha256=%s want %s", got, m.ExpectedBaselineSHA256)
			}
			if !bytes.Contains(base, []byte(`"scheduled_send_ms":`+m.TimestampToken+`,`)) {
				t.Fatalf("canonical timestamp token %q not preserved without rounding: %s", m.TimestampToken, base)
			}
			if len(m.Sizes) != len(g.SizeOffsets) {
				t.Fatalf("sizes=%d want %d", len(m.Sizes), len(g.SizeOffsets))
			}
			for _, size := range m.Sizes {
				target := len(base) + size.OffsetBytes
				if target != size.TargetBytes {
					t.Fatalf("offset %d target=%d want %d", size.OffsetBytes, target, size.TargetBytes)
				}
				out, err := BuildPayload(p, g.Seed, target)
				if err != nil {
					t.Fatalf("offset %d: %v", size.OffsetBytes, err)
				}
				if len(out) != target || sha256Hex(out) != size.SHA256 {
					t.Fatalf("offset %d built len=%d sha=%s want len=%d sha=%s", size.OffsetBytes, len(out), sha256Hex(out), target, size.SHA256)
				}
				var stored LogicalPayload
				if err := json.Unmarshal(out, &stored); err != nil {
					t.Fatal(err)
				}
				if len(stored.Padding) != size.PaddingLength || sha256Hex([]byte(stored.Padding)) != size.PaddingSHA256 {
					t.Fatalf("offset %d padding len=%d sha=%s want len=%d sha=%s", size.OffsetBytes, len(stored.Padding), sha256Hex([]byte(stored.Padding)), size.PaddingLength, size.PaddingSHA256)
				}
				if stored.ScheduledSendMS != p.ScheduledSendMS {
					t.Fatalf("timestamp identity changed: %v vs %v", stored.ScheduledSendMS, p.ScheduledSendMS)
				}
				expected := p
				expected.Padding = stored.Padding
				if err := ValidateStored(expected, out); err != nil {
					t.Fatalf("built stored payload rejected: %v", err)
				}
				// Reconstruction uses the existing versioned Mix only; it is not a second encoder.
				var reconstructed strings.Builder
				for block := uint32(0); reconstructed.Len() < len(stored.Padding); block++ {
					v := benchhash.Mix(g.Seed, m.Seq, PayloadPaddingSalt^block)
					var raw [4]byte
					raw[0], raw[1], raw[2], raw[3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
					reconstructed.WriteString(hex.EncodeToString(raw[:]))
				}
				if got := reconstructed.String()[:len(stored.Padding)]; got != stored.Padding {
					t.Fatalf("offset %d padding reconstruction mismatch", size.OffsetBytes)
				}
			}
		})
	}
}

func TestPayloadBuilderContractBoundsAndNoRounding(t *testing.T) {
	g := loadPayloadCohortGolden(t)
	p := g.logical(g.Cohort[0])
	base, err := CanonicalBytes(p)
	if err != nil {
		t.Fatal(err)
	}
	n := len(base)
	if _, err = BuildPayload(p, g.Seed, n-1); err == nil {
		t.Fatal("accepted too-small target")
	}
	if _, err = BuildPayload(p, g.Seed, n-2); err == nil {
		t.Fatal("accepted insufficient even target")
	}
	if _, err = BuildPayload(p, g.Seed, n+1); err == nil {
		t.Fatal("accepted odd padding increment")
	}
	if _, err = BuildPayload(p, g.Seed, PayloadMaximumBytes+1); err == nil {
		t.Fatal("accepted oversized target")
	}
	if _, err = BuildPayloadOffset(p, g.Seed, 1); err == nil {
		t.Fatal("accepted odd offset")
	}
	if _, err = BuildPayloadOffset(p, g.Seed, -1); err == nil {
		t.Fatal("accepted negative offset")
	}
	if _, err = BuildPayloadOffset(p, g.Seed, PayloadMaximumBytes+1); err == nil {
		t.Fatal("accepted oversized offset")
	}
	offset, err := BuildPayloadOffset(p, g.Seed, 4096)
	if err != nil || len(offset) != n+4096 || sha256Hex(offset) != g.Cohort[0].Sizes[2].SHA256 {
		t.Fatalf("offset 4096 len=%d err=%v", len(offset), err)
	}
	for _, ts := range []float64{0.0001, 1048576.0011, 9007199254740992} {
		bad := p
		bad.ScheduledSendMS = ts
		if _, err = CanonicalBytes(bad); err == nil {
			t.Fatalf("timestamp %g was silently rounded", ts)
		}
		if _, err = BuildPayload(bad, g.Seed, n); err == nil {
			t.Fatalf("BuildPayload accepted unrepresentable timestamp %g", ts)
		}
		if _, err = BuildPayloadOffset(bad, g.Seed, 1024); err == nil {
			t.Fatalf("BuildPayloadOffset accepted unrepresentable timestamp %g", ts)
		}
	}
	negativeZero := p
	negativeZero.ScheduledSendMS = -0.0
	neg, err := CanonicalBytes(negativeZero)
	if err != nil {
		t.Fatal(err)
	}
	positiveZero := p
	positiveZero.ScheduledSendMS = 0
	pos, err := CanonicalBytes(positiveZero)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(neg, pos) || bytes.Contains(neg, []byte(`"scheduled_send_ms":-0`)) {
		t.Fatalf("negative zero was not normalized: %s", neg)
	}
}

func TestPayloadStoredExportValidatesActualBytesAndRejectsCorruption(t *testing.T) {
	g := loadPayloadCohortGolden(t)
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "payload-stored-export.ndjson")
	var export bytes.Buffer
	expected := map[uint32]LogicalPayload{}
	for _, m := range g.Cohort {
		p := g.logical(m)
		target := len(m.ExpectedBaseline) + 4096
		out, err := BuildPayload(p, g.Seed, target)
		if err != nil {
			t.Fatal(err)
		}
		var stored LogicalPayload
		if err := json.Unmarshal(out, &stored); err != nil {
			t.Fatal(err)
		}
		p.Padding = stored.Padding
		expected[m.Seq] = p
		export.Write(out)
		export.WriteByte('\n')
	}
	if err := os.WriteFile(exportPath, export.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != len(g.Cohort) {
		t.Fatalf("stored export lines=%d want %d", len(lines), len(g.Cohort))
	}
	for _, line := range lines {
		var identity struct {
			Seq uint32 `json:"seq"`
		}
		if err := json.Unmarshal(line, &identity); err != nil {
			t.Fatal(err)
		}
		if err := ValidateStored(expected[identity.Seq], line); err != nil {
			t.Fatalf("actual stored export seq %d rejected: %v", identity.Seq, err)
		}
	}
	first := g.Cohort[0]
	line := lines[0]
	mutations := map[string][]byte{
		"altered":   bytes.Replace(line, []byte(`"temperature":20`), []byte(`"temperature":21`), 1),
		"truncated": append([]byte(nil), line[:len(line)-1]...),
		"wrong":     bytes.Replace(line, []byte(`"seq":2`), []byte(`"seq":"2"`), 1),
		"null":      bytes.Replace(line, []byte(`"temperature":20`), []byte(`"temperature":null`), 1),
		"duplicate": bytes.Replace(line, []byte(`"seq":2`), []byte(`"seq":2,"seq":2`), 1),
	}
	var omitted map[string]json.RawMessage
	if err := json.Unmarshal(line, &omitted); err != nil {
		t.Fatal(err)
	}
	delete(omitted, "padding")
	mutations["omitted"], _ = json.Marshal(omitted)
	for name, mutated := range mutations {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(mutated, line) {
				t.Fatal("mutation did not change stored bytes")
			}
			if err := ValidateStored(expected[first.Seq], mutated); err == nil {
				t.Fatal("corrupt stored content accepted")
			}
		})
	}
	other := g.logical(g.Cohort[1])
	other.Padding = expected[g.Cohort[1].Seq].Padding
	if err := ValidateStored(expected[first.Seq], lines[1]); err == nil {
		t.Fatalf("stored identity mismatch accepted: %+v", other)
	}
}
