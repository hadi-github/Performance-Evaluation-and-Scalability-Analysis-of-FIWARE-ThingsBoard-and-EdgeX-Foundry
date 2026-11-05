package offline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"iotbench/internal/audit"
	"iotbench/internal/protocol"
	"iotbench/internal/report"
)

// M1c golden vectors are synthetic engineering fixtures. They exercise the real canonical
// encoders on both sides; the expected bytes are test data, not a second source of truth.
type payloadGolden struct {
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
	Cohort    []payloadGoldenMessage `json:"cohort"`
	EdgeCases []payloadGoldenMessage `json:"edge_cases"`
}

type payloadGoldenMessage struct {
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

func (g payloadGolden) logical(m payloadGoldenMessage) protocol.LogicalPayload {
	return protocol.LogicalPayload{RunID: g.RunID, Seq: m.Seq, DeviceID: m.DeviceID, ScheduledSendMS: m.ScheduledSendMS,
		Temperature: g.Telemetry.Temperature, Humidity: g.Telemetry.Humidity, Pressure: g.Telemetry.Pressure}
}

func loadPayloadGolden(t *testing.T) payloadGolden {
	t.Helper()
	b, err := os.ReadFile("payload-cohort-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g payloadGolden
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	return g
}

func payloadSHA(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type payloadBuilt struct {
	Name          string
	Seq           uint32
	OffsetBytes   int
	Length        int
	SHA256        string
	PaddingSHA256 string
}

func buildGoldenPayloads(t *testing.T, g payloadGolden) map[string]payloadBuilt {
	t.Helper()
	out := map[string]payloadBuilt{}
	for _, m := range append(append([]payloadGoldenMessage(nil), g.Cohort...), g.EdgeCases...) {
		p := g.logical(m)
		base, err := protocol.CanonicalBytes(p)
		if err != nil {
			t.Fatal(err)
		}
		if string(base) != m.ExpectedBaseline {
			t.Fatalf("%s canonical baseline differs from golden", m.Name)
		}
		for _, size := range m.Sizes {
			target := len(base) + size.OffsetBytes
			built, err := protocol.BuildPayload(p, g.Seed, target)
			if err != nil {
				t.Fatalf("%s/%d: %v", m.Name, size.OffsetBytes, err)
			}
			var stored protocol.LogicalPayload
			if err := json.Unmarshal(built, &stored); err != nil {
				t.Fatal(err)
			}
			entry := payloadBuilt{Name: m.Name, Seq: m.Seq, OffsetBytes: size.OffsetBytes, Length: len(built),
				SHA256: payloadSHA(built), PaddingSHA256: payloadSHA([]byte(stored.Padding))}
			if entry.Length != size.TargetBytes || entry.SHA256 != size.SHA256 || entry.PaddingSHA256 != size.PaddingSHA256 {
				t.Fatalf("%s/%d Go build differs from golden: %+v size=%+v", m.Name, size.OffsetBytes, entry, size)
			}
			out[fmt.Sprintf("%s/%d", m.Name, size.OffsetBytes)] = entry
		}
	}
	return out
}

type payloadNodeResult struct {
	Status   string `json:"status"`
	Messages []struct {
		Name          string `json:"name"`
		Seq           uint32 `json:"seq"`
		OffsetBytes   int    `json:"offset_bytes"`
		Length        int    `json:"length"`
		SHA256        string `json:"sha256"`
		PaddingSHA256 string `json:"padding_sha256"`
	} `json:"messages"`
	Stored *struct {
		Path        string `json:"path"`
		OffsetBytes int    `json:"offset_bytes"`
		Lines       int    `json:"lines"`
		Validated   int    `json:"validated"`
	} `json:"stored"`
}

func runPayloadNode(t *testing.T, args ...string) payloadNodeResult {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available; JS/Go parity is exercised when Node is installed")
	}
	cmd := exec.Command(node, append([]string{"payload-cohort-golden.mjs"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node payload golden failed: %v\n%s", err, out)
	}
	var result payloadNodeResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		t.Fatalf("node output is not JSON: %v\n%s", err, out)
	}
	return result
}

func writeCohortStoredExport(t *testing.T, g payloadGolden, offset int) (string, map[uint32]protocol.LogicalPayload) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload-stored-export.ndjson")
	var export strings.Builder
	expected := map[uint32]protocol.LogicalPayload{}
	for _, m := range g.Cohort {
		p := g.logical(m)
		target := len(m.ExpectedBaseline) + offset
		stored, err := protocol.BuildPayload(p, g.Seed, target)
		if err != nil {
			t.Fatal(err)
		}
		var logical protocol.LogicalPayload
		if err := json.Unmarshal(stored, &logical); err != nil {
			t.Fatal(err)
		}
		p.Padding = logical.Padding
		expected[m.Seq] = p
		export.Write(stored)
		export.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(export.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, expected
}

type payloadVerification struct {
	SchemaVersion        int    `json:"schema_version"`
	MetricInterpretation string `json:"metric_interpretation"`
	Label                string `json:"label"`
	Synthetic            bool   `json:"synthetic"`
	Status               string `json:"status"`
	RunID                string `json:"run_id"`
	ProtocolID           string `json:"protocol_id"`
	ProtocolSHA256       string `json:"protocol_sha256"`
	ProfileSHA256        string `json:"profile_sha256"`
	PlanSHA256           string `json:"plan_sha256"`
	EncodingID           string `json:"encoding_id"`
	SizeOffsetBytes      int    `json:"size_offset_bytes"`
	Validated            int    `json:"validated"`
	Cohort               []struct {
		Seq         uint32  `json:"seq"`
		DeviceID    string  `json:"device_id"`
		ScheduledMS float64 `json:"scheduled_send_ms"`
		Status      string  `json:"status"`
	} `json:"cohort"`
	ObservationsRemoved      int    `json:"observations_removed"`
	LatencyObservationEffect string `json:"latency_observation_effect"`
	Note                     string `json:"note"`
}

func verifyPayloadStoredExport(g payloadGolden, path string, expected map[uint32]protocol.LogicalPayload, binding protocol.Binding, profileSHA, planSHA string, offset int) (payloadVerification, error) {
	v := payloadVerification{SchemaVersion: 1, MetricInterpretation: "separate-payload-verification-v1", Label: g.Label,
		Synthetic: true, Status: "PASS", RunID: g.RunID, ProtocolID: binding.ProtocolID, ProtocolSHA256: binding.ProtocolSHA256,
		ProfileSHA256: profileSHA, PlanSHA256: planSHA, EncodingID: g.Provenance.EncodingID, SizeOffsetBytes: offset,
		ObservationsRemoved: 0, LatencyObservationEffect: "none; payload verification never deletes or rewrites identifiable latency/receipt observations",
		Note: "Synthetic offline engineering verification only; not live evidence, runtime capture, storage proof, or clock qualification."}
	raw, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	seen := map[uint32]bool{}
	for i, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var identity struct {
			Seq uint32 `json:"seq"`
		}
		if err := json.Unmarshal(line, &identity); err != nil {
			v.Status = "FAIL"
			return v, fmt.Errorf("stored export line %d: %w", i, err)
		}
		p, ok := expected[identity.Seq]
		if !ok {
			v.Status = "FAIL"
			return v, fmt.Errorf("stored export line %d: seq %d is not in the synthetic cohort", i, identity.Seq)
		}
		if seen[identity.Seq] {
			v.Status = "FAIL"
			return v, fmt.Errorf("stored export line %d: duplicate stored identity seq %d", i, identity.Seq)
		}
		seen[identity.Seq] = true
		if err := protocol.ValidateStored(p, line); err != nil {
			v.Status = "FAIL"
			return v, fmt.Errorf("stored export line %d seq %d: %w", i, identity.Seq, err)
		}
		v.Validated++
		v.Cohort = append(v.Cohort, struct {
			Seq         uint32  `json:"seq"`
			DeviceID    string  `json:"device_id"`
			ScheduledMS float64 `json:"scheduled_send_ms"`
			Status      string  `json:"status"`
		}{Seq: identity.Seq, DeviceID: p.DeviceID, ScheduledMS: p.ScheduledSendMS, Status: "PASS"})
	}
	if v.Validated != len(expected) {
		v.Status = "FAIL"
		return v, fmt.Errorf("stored export validated %d of %d expected cohort identities", v.Validated, len(expected))
	}
	return v, nil
}

func TestPayloadCohortNodeParityAndStoredExport(t *testing.T) {
	g := loadPayloadGolden(t)
	goBuilt := buildGoldenPayloads(t, g)
	storedPath, expected := writeCohortStoredExport(t, g, 4096)
	node := runPayloadNode(t, "--stored", storedPath, "--stored-offset", "4096")
	if node.Status != "ok" {
		t.Fatalf("node status=%q", node.Status)
	}
	if len(node.Messages) != len(goBuilt) {
		t.Fatalf("node messages=%d want %d", len(node.Messages), len(goBuilt))
	}
	for _, entry := range node.Messages {
		key := fmt.Sprintf("%s/%d", entry.Name, entry.OffsetBytes)
		want, ok := goBuilt[key]
		if !ok {
			t.Fatalf("node returned unknown vector %s", key)
		}
		if entry.Seq != want.Seq || entry.Length != want.Length || entry.SHA256 != want.SHA256 || entry.PaddingSHA256 != want.PaddingSHA256 {
			t.Fatalf("JS/Go parity mismatch %s: node=%+v go=%+v", key, entry, want)
		}
	}
	if node.Stored == nil || node.Stored.OffsetBytes != 4096 || node.Stored.Lines != len(g.Cohort) || node.Stored.Validated != len(g.Cohort) {
		t.Fatalf("node stored export result: %+v", node.Stored)
	}
	raw, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var identity struct {
			Seq uint32 `json:"seq"`
		}
		if err := json.Unmarshal(line, &identity); err != nil {
			t.Fatal(err)
		}
		if err := protocol.ValidateStored(expected[identity.Seq], line); err != nil {
			t.Fatalf("Go ValidateStored rejected actual export seq %d: %v", identity.Seq, err)
		}
	}
	// Corrupted actual stored bytes must fail rather than being silently discarded.
	corrupt := bytes.Replace(raw, []byte(`"temperature":20`), []byte(`"temperature":21`), 1)
	corruptPath := filepath.Join(t.TempDir(), "corrupt.ndjson")
	if err := os.WriteFile(corruptPath, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := verifyPayloadStoredExport(g, corruptPath, expected, protocol.Binding{ProtocolID: g.Provenance.ProtocolID, ProtocolSHA256: g.Provenance.ProtocolSHA256}, g.Provenance.ProfileSHA256, strings.Repeat("0", 64), 4096)
	if err == nil || v.Status != "FAIL" {
		t.Fatalf("corrupt stored export was not reported: %+v %v", v, err)
	}
}

func TestOfflinePayloadCohortMapsToM1bAndStaysSeparate(t *testing.T) {
	g := loadPayloadGolden(t)
	f := newOfflineFixture(t, g.RunID, true)
	var sum report.Summary
	readJSON(t, filepath.Join(f.dir, "k6-summary.json"), &sum)
	planned := audit.PlannedMessages(f.resolved, &sum)
	var stepName string
	for _, step := range f.resolved.Steps {
		if step.Phase == "measurement" {
			stepName = step.Name
			break
		}
	}
	if stepName != g.CohortStep.Name {
		t.Fatalf("measurement step %q differs from golden %q", stepName, g.CohortStep.Name)
	}
	var measurement []audit.Planned
	for _, p := range planned {
		if p.Step == stepName {
			measurement = append(measurement, p)
		}
	}
	if len(measurement) != len(g.Cohort) {
		t.Fatalf("M1b measurement identities=%d want %d", len(measurement), len(g.Cohort))
	}
	for i, m := range g.Cohort {
		p := measurement[i]
		if int(p.Seq) != int(m.Seq) || p.Device != int(m.Seq%5) || p.SchedMs != m.ScheduledSendMS || fmt.Sprintf("device-%d", p.Device) != m.DeviceID {
			t.Fatalf("cohort mapping %d differs: planned=%+v golden=%+v", i, p, m)
		}
	}

	before := analyzeFixture(t, f)
	storedPath, expected := writeCohortStoredExport(t, g, 4096)
	v, err := verifyPayloadStoredExport(g, storedPath, expected, f.binding, g.Provenance.ProfileSHA256, f.fixture.PlanSHA256, 4096)
	if err != nil || v.Status != "PASS" || v.Validated != len(g.Cohort) || v.ObservationsRemoved != 0 {
		t.Fatalf("payload verification result: %+v %v", v, err)
	}
	writeJSON(t, filepath.Join(f.dir, "payload-verification.json"), v)
	// The payload export is intentionally not an analysis input; the protocol/latency result is unchanged.
	after := analyzeFixture(t, f)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("payload verification changed the protocol/latency analysis result")
	}
	s := measurementStep(t, after)
	for name, want := range map[string]float64{
		"protocol.scheduled_cohort.scheduled": 5, "protocol.scheduled_cohort.observed": 3, "protocol.scheduled_cohort.missing": 2,
		"protocol.delivery.p99_ms": 600, "protocol.delivery.attainment_500_count": 2,
	} {
		if got := metric(t, s, name); got != want {
			t.Fatalf("%s=%g want %g", name, got, want)
		}
	}
	for name := range s.Metrics {
		if strings.Contains(strings.ToLower(name), "payload") {
			t.Fatalf("payload verification was retrofitted into the metric schema as %q", name)
		}
	}
	if after.Protocol == nil || after.Protocol.ProtocolSHA256 != g.Provenance.ProtocolSHA256 || after.Protocol.ProfileSHA256 != g.Provenance.ProfileSHA256 {
		t.Fatalf("protocol provenance changed: %+v", after.Protocol)
	}
	// A corrupt payload remains a separately reported failure and removes no latency observation.
	raw, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatal(err)
	}
	corruptPath := filepath.Join(t.TempDir(), "corrupt-export.ndjson")
	if err = os.WriteFile(corruptPath, bytes.Replace(raw, []byte(`"temperature":20`), []byte(`"temperature":21`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	corrupt, err := verifyPayloadStoredExport(g, corruptPath, expected, f.binding, g.Provenance.ProfileSHA256, f.fixture.PlanSHA256, 4096)
	if err == nil || corrupt.Status != "FAIL" || corrupt.Validated >= len(g.Cohort) {
		t.Fatalf("corrupt payload verification did not fail closed: %+v %v", corrupt, err)
	}
	still := analyzeFixture(t, f)
	if !reflect.DeepEqual(before, still) {
		t.Fatal("corrupt payload verification removed or rewrote identifiable latency observations")
	}
}
