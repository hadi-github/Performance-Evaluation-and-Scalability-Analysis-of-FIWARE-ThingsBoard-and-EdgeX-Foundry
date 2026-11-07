package analysis

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/receipt"
)

func protocolFixture(t *testing.T) (string, Options) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "synthetic-run")
	protocolPath, err := filepath.Abs("../../research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err = SyntheticProtocolCohort(dir, "SYNTHETIC-protocol-test", protocolPath, root); err != nil {
		t.Fatal(err)
	}
	return dir, Options{Run: dir, Evidence: filepath.Join(dir, "evidence.json"), Policy: filepath.Join(dir, "profiles.json"), ProtocolPath: protocolPath, ProtocolRoot: root}
}
func analyzeProtocolFixture(t *testing.T, o Options) *Result {
	t.Helper()
	r, e := Analyze(o)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func measurementStep(t *testing.T, r *Result) Step {
	t.Helper()
	for _, s := range r.Steps {
		if s.Phase == "measurement" {
			return s
		}
	}
	t.Fatal("measurement step missing")
	return Step{}
}
func metricValue(t *testing.T, s Step, name string) float64 {
	t.Helper()
	m, ok := s.Metrics[name]
	if !ok || m.Value == nil {
		t.Fatalf("metric %s unavailable: %+v", name, m)
	}
	return *m.Value
}
func saveCapture(t *testing.T, dir string, c ProtocolReceiptCapture) {
	t.Helper()
	if e := writeJSON(filepath.Join(dir, "protocol-receipts.json"), c); e != nil {
		t.Fatal(e)
	}
}
func readCapture(t *testing.T, dir string) ProtocolReceiptCapture {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(dir, "protocol-receipts.json"))
	if e != nil {
		t.Fatal(e)
	}
	var c ProtocolReceiptCapture
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	return c
}

func TestProtocolScheduledCohortSyntheticHandCalculatedVerticalSlice(t *testing.T) {
	d, o := protocolFixture(t)
	r := analyzeProtocolFixture(t, o)
	s := measurementStep(t, r)
	if r.MetricInterpretation != ProtocolMetricInterpretation || r.Protocol == nil || r.Protocol.ProtocolSHA256 == "" || r.Protocol.ProfileSHA256 == "" {
		t.Fatalf("protocol provenance missing: %+v", r.Protocol)
	}
	if r.Protocol.ReceiptCaptureSchemaVersion != receipt.SchemaVersion || r.Protocol.ReceiptCaptureSemantics != receipt.CaptureSemantics {
		t.Fatalf("receipt capture version provenance missing: %+v", r.Protocol)
	}
	if s.Claims["protocol_delivery"].Status != Pass {
		t.Fatalf("protocol claim: %+v; offered load: %+v", s.Claims["protocol_delivery"], s.Claims["offered_load"])
	}
	if got := s.Metrics["protocol.delivery.p99_ms"].ClockDomain; got != "synthetic:epoch-ms" {
		t.Fatalf("metric clock domain=%q", got)
	}
	for name, want := range map[string]float64{"protocol.scheduled_cohort.scheduled": 5, "protocol.scheduled_cohort.observed": 3, "protocol.scheduled_cohort.missing": 2, "protocol.scheduled_cohort.duplicate_extra_receipts": 1, "protocol.delivery.p50_ms": 100, "protocol.delivery.p95_ms": 600, "protocol.delivery.p99_ms": 600, "protocol.delivery.attainment_250_count": 2, "protocol.delivery.attainment_250_ratio": .4, "protocol.delivery.attainment_500_count": 2, "protocol.delivery.attainment_500_ratio": .4, "protocol.delivery.attainment_1000_count": 3, "protocol.delivery.attainment_5000_count": 3, "protocol.receipts.error": 1, "protocol.receipts.unknown": 1} {
		if got := metricValue(t, s, name); got != want {
			t.Fatalf("%s=%g want %g", name, got, want)
		}
	}
	if _, ok := s.Metrics["latency.ack.p99_ms"]; !ok {
		t.Fatal("legacy acknowledged-only metric was removed")
	}
	out := filepath.Join(t.TempDir(), "analysis")
	if e := Write(out, r, d); e != nil {
		t.Fatal(e)
	}
	reportBytes, e := os.ReadFile(filepath.Join(out, "report.md"))
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(reportBytes), "protocol.delivery.p99_ms") || !strings.Contains(string(reportBytes), "scheduled minus unique first receipts") {
		t.Fatal("report omitted protocol metric basis/denominator")
	}
}

func TestDispatchSchemaUsesApplicationDispatchAsPrimaryLatency(t *testing.T) {
	d, o := protocolFixture(t)
	c := readCapture(t, d)
	c.SchemaVersion = receipt.DispatchSchemaVersion
	c.MetricInterpretation = DispatchMetricInterpretation
	c.Endpoint = "application_dispatch_to_response_complete_body_read"
	for i := range c.Receipts {
		c.Receipts[i].ActualDispatchMS = c.Receipts[i].ScheduledSendMS + 5
	}
	saveCapture(t, d, c)
	b, err := os.ReadFile(filepath.Join(d, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err = json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["receipt_capture_interpretation"] = DispatchMetricInterpretation
	if err = writeJSON(filepath.Join(d, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	r := analyzeProtocolFixture(t, o)
	s := measurementStep(t, r)
	if s.Claims["protocol_delivery"].Status != Pass {
		t.Fatalf("dispatch claim: %+v", s.Claims["protocol_delivery"])
	}
	if got := metricValue(t, s, "protocol.delivery.p99_ms"); got != 595 {
		t.Fatalf("dispatch p99=%g, want 595", got)
	}
	if s.Metrics["protocol.delivery.p99_ms"].Endpoint != "application_dispatch_to_response_complete_body_read" {
		t.Fatal("dispatch endpoint not labeled")
	}
}

func TestProtocolManifestSyntheticProvenanceCannotRewriteEvidenceSpec(t *testing.T) {
	d, o := protocolFixture(t)
	b, err := os.ReadFile(o.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	var evidence Evidence
	if err = json.Unmarshal(b, &evidence); err != nil {
		t.Fatal(err)
	}
	evidence.Synthetic = false
	if err = writeJSON(o.Evidence, evidence); err != nil {
		t.Fatal(err)
	}
	var policy Policy
	readJSONTest(t, o.Policy, &policy)
	policy.Synthetic = false
	if err = writeJSON(o.Policy, policy); err != nil {
		t.Fatal(err)
	}
	r := analyzeProtocolFixture(t, o)
	if got := r.Claims["offered_load"].Status; got != Invalid {
		t.Fatalf("manifest/evidence synthetic disagreement status=%s", got)
	}
	if got := measurementStep(t, r).Claims["protocol_delivery"].Status; got != Invalid {
		t.Fatalf("protocol provenance mismatch status=%s", got)
	}
	_ = d
}

func TestProtocolCohortCutoffWarmupZeroAndOutcomeSemantics(t *testing.T) {
	t.Run("inclusive cutoff and warmup excluded", func(t *testing.T) {
		d, o := protocolFixture(t)
		c := readCapture(t, d)
		c.Receipts = append(c.Receipts, ProtocolReceipt{RunID: "SYNTHETIC-protocol-test", Seq: 1, DeviceID: "device-1", Phase: "warmup", ScheduledSendMS: c.CoverageStartMS - 1000, ObservedMS: c.CoverageStartMS - 900, Outcome: protocol.OutcomeOK}, ProtocolReceipt{RunID: "SYNTHETIC-protocol-test", Seq: 6, DeviceID: "device-1", Phase: "measurement", ScheduledSendMS: c.CoverageStartMS + 4000, ObservedMS: c.ObservationCutoffMS, Outcome: protocol.OutcomeUnknown})
		saveCapture(t, d, c)
		s := measurementStep(t, analyzeProtocolFixture(t, o))
		if metricValue(t, s, "protocol.scheduled_cohort.scheduled") != 5 || metricValue(t, s, "protocol.scheduled_cohort.observed") != 4 || metricValue(t, s, "protocol.scheduled_cohort.missing") != 1 {
			t.Fatal(s.Metrics)
		}
		if metricValue(t, s, "protocol.delivery.p99_ms") != 6000 {
			t.Fatal(s.Metrics)
		}
	})
	t.Run("after cutoff excluded", func(t *testing.T) {
		d, o := protocolFixture(t)
		c := readCapture(t, d)
		c.Receipts = append(c.Receipts, ProtocolReceipt{RunID: "SYNTHETIC-protocol-test", Seq: 6, DeviceID: "device-1", Phase: "measurement", ScheduledSendMS: c.CoverageStartMS + 4000, ObservedMS: c.ObservationCutoffMS + 1, Outcome: protocol.OutcomeOK})
		saveCapture(t, d, c)
		s := measurementStep(t, analyzeProtocolFixture(t, o))
		if metricValue(t, s, "protocol.scheduled_cohort.observed") != 3 || metricValue(t, s, "protocol.scheduled_cohort.missing") != 2 || metricValue(t, s, "protocol.scheduled_cohort.late_excluded_receipts") != 1 {
			t.Fatal(s.Metrics)
		}
	})
	t.Run("zero receipt percentile null", func(t *testing.T) {
		d, o := protocolFixture(t)
		c := readCapture(t, d)
		c.Receipts = c.Receipts[:1]
		saveCapture(t, d, c)
		s := measurementStep(t, analyzeProtocolFixture(t, o))
		if metricValue(t, s, "protocol.scheduled_cohort.observed") != 0 || metricValue(t, s, "protocol.scheduled_cohort.missing") != 5 {
			t.Fatal(s.Metrics)
		}
		for _, name := range []string{"protocol.delivery.p50_ms", "protocol.delivery.p95_ms", "protocol.delivery.p99_ms"} {
			if s.Metrics[name].Value != nil {
				t.Fatalf("zero observed %s=%v, want null", name, *s.Metrics[name].Value)
			}
		}
	})
}

func TestProtocolCohortCaptureAndIdentityFailuresFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string, *ProtocolReceiptCapture)
		want   Status
	}{
		{"wrong endpoint", func(_ string, c *ProtocolReceiptCapture) { c.Endpoint = "legacy_http_ack" }, NotAssessed},
		{"incomplete body", func(_ string, c *ProtocolReceiptCapture) { c.BodyReadComplete = false }, NotAssessed},
		{"legacy schema", func(_ string, c *ProtocolReceiptCapture) { c.SchemaVersion = 1 }, NotAssessed},
		{"missing semantics", func(_ string, c *ProtocolReceiptCapture) { c.ReceiptCaptureSemantics = "" }, NotAssessed},
		{"wrong semantics", func(_ string, c *ProtocolReceiptCapture) { c.ReceiptCaptureSemantics = "legacy-arrival-clock-v1" }, NotAssessed},
		{"wrong binding", func(_ string, c *ProtocolReceiptCapture) { c.Binding.ProtocolSHA256 = strings.Repeat("0", 64) }, Invalid},
		{"bad device mapping", func(_ string, c *ProtocolReceiptCapture) { c.Receipts[1].DeviceID = "wrong-device" }, Invalid},
		{"wrong phase boundary", func(_ string, c *ProtocolReceiptCapture) { c.Receipts[1].Phase = "warmup" }, Invalid},
		{"clock before schedule", func(_ string, c *ProtocolReceiptCapture) {
			c.Receipts[1].ObservedMS = c.Receipts[1].ScheduledSendMS - 1
		}, Invalid},
		{"not sent forbidden", func(_ string, c *ProtocolReceiptCapture) { c.Receipts[1].Outcome = protocol.OutcomeNotSent }, Invalid},
		{"unlabeled synthetic clock", func(_ string, c *ProtocolReceiptCapture) { c.ClockSource = "realtime" }, Invalid},
		{"cutoff mismatch", func(_ string, c *ProtocolReceiptCapture) { c.ObservationCutoffMS++ }, Invalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, o := protocolFixture(t)
			c := readCapture(t, d)
			tc.mutate(d, &c)
			saveCapture(t, d, c)
			r := analyzeProtocolFixture(t, o)
			s := measurementStep(t, r)
			if got := s.Claims["protocol_delivery"].Status; got != tc.want {
				t.Fatalf("claim=%s want=%s: %+v", got, tc.want, s.Claims["protocol_delivery"])
			}
			if s.Metrics["protocol.delivery.p99_ms"].Value != nil {
				t.Fatal("invalid capture produced a p99")
			}
		})
	}
	t.Run("missing capture cannot fall back", func(t *testing.T) {
		d, o := protocolFixture(t)
		if e := os.Remove(filepath.Join(d, "protocol-receipts.json")); e != nil {
			t.Fatal(e)
		}
		r := analyzeProtocolFixture(t, o)
		s := measurementStep(t, r)
		if r.MetricInterpretation != ProtocolMetricInterpretation || s.Claims["protocol_delivery"].Status != NotAssessed || s.Metrics["protocol.delivery.p99_ms"].Value != nil {
			t.Fatal(r.MetricInterpretation, s.Claims, s.Metrics)
		}
	})
	t.Run("partial legacy ledger preserves D15 invalidity", func(t *testing.T) {
		d, o := protocolFixture(t)
		if e := os.Remove(filepath.Join(d, "records.ndjson")); e != nil {
			t.Fatal(e)
		}
		r := analyzeProtocolFixture(t, o)
		s := measurementStep(t, r)
		if s.Claims["offered_load"].Status != Invalid || s.Claims["protocol_delivery"].Status != Invalid {
			t.Fatal(s.Claims)
		}
	})
}

// A schema-1 capture remains readable but must never be promoted to the schema-2 complete-body
// primary metrics by default. Its legacy arrival timestamps stay legacy.
func TestProtocolLegacyCaptureSchemaCannotClaimNewPrimaryMetrics(t *testing.T) {
	d, o := protocolFixture(t)
	c := readCapture(t, d)
	c.SchemaVersion = 1
	c.ReceiptCaptureSemantics = ""
	saveCapture(t, d, c)
	r := analyzeProtocolFixture(t, o)
	if r.Protocol == nil || r.Protocol.ReceiptCaptureSchemaVersion != 1 || r.Protocol.ReceiptCaptureSemantics != "" {
		t.Fatalf("legacy capture provenance was relabeled: %+v", r.Protocol)
	}
	s := measurementStep(t, r)
	if s.Claims["protocol_delivery"].Status != NotAssessed {
		t.Fatalf("legacy capture claim=%+v", s.Claims["protocol_delivery"])
	}
	if !strings.Contains(strings.Join(s.Claims["protocol_delivery"].Reasons, " "), "legacy") {
		t.Fatalf("legacy reason not explicit: %+v", s.Claims["protocol_delivery"])
	}
	for _, name := range []string{"protocol.scheduled_cohort.observed", "protocol.scheduled_cohort.missing", "protocol.delivery.p99_ms"} {
		if s.Metrics[name].Value != nil {
			t.Fatalf("legacy capture produced %s=%v", name, *s.Metrics[name].Value)
		}
	}
}

func TestProtocolCaptureRawSchemaRejectsMissingNullAliasesAndDuplicateKeys(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"missing receipts", func(b []byte) []byte {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(b, &m)
			delete(m, "receipts")
			v, _ := json.Marshal(m)
			return v
		}},
		{"null receipts", func(b []byte) []byte {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(b, &m)
			m["receipts"] = json.RawMessage("null")
			v, _ := json.Marshal(m)
			return v
		}},
		{"missing complete-body semantics", func(b []byte) []byte {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(b, &m)
			delete(m, "receipt_capture_semantics")
			v, _ := json.Marshal(m)
			return v
		}},
		{"null complete-body semantics", func(b []byte) []byte {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(b, &m)
			m["receipt_capture_semantics"] = json.RawMessage("null")
			v, _ := json.Marshal(m)
			return v
		}},
		{"unsupported schema version", func(b []byte) []byte {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(b, &m)
			m["schema_version"] = json.RawMessage("99")
			v, _ := json.Marshal(m)
			return v
		}},
		{"null identity", func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"device_id": "device-2"`, `"device_id": null`, 1))
		}},
		{"case alias", func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"complete": true`, `"Complete": true`, 1))
		}},
		{"duplicate key", func(b []byte) []byte {
			return []byte(strings.Replace(string(b), `"complete": true`, `"complete": true, "complete": false`, 1))
		}},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			d, o := protocolFixture(t)
			p := filepath.Join(d, "protocol-receipts.json")
			b, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(p, tc.mutate(b), 0644); e != nil {
				t.Fatal(e)
			}
			r := analyzeProtocolFixture(t, o)
			s := measurementStep(t, r)
			if s.Claims["protocol_delivery"].Status == Pass || s.Metrics["protocol.scheduled_cohort.observed"].Value != nil || s.Metrics["protocol.scheduled_cohort.missing"].Value != nil {
				t.Fatalf("malformed raw capture yielded platform counts: %+v", s)
			}
		})
	}
	t.Run("explicit empty receipts array is valid zero observation", func(t *testing.T) {
		d, o := protocolFixture(t)
		c := readCapture(t, d)
		c.Receipts = []ProtocolReceipt{}
		saveCapture(t, d, c)
		s := measurementStep(t, analyzeProtocolFixture(t, o))
		if s.Claims["protocol_delivery"].Status != Pass || metricValue(t, s, "protocol.scheduled_cohort.observed") != 0 || metricValue(t, s, "protocol.scheduled_cohort.missing") != 5 || s.Metrics["protocol.delivery.p99_ms"].Value != nil {
			t.Fatal(s.Claims, s.Metrics)
		}
	})
}

func TestProtocolIntentNeverSilentlyFallsBackToLegacy(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, dir string)
	}{
		{"binding in plan but missing resolved", func(t *testing.T, d string) {
			if err := os.Remove(filepath.Join(d, "resolved.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"orphan pilot in resolved", func(t *testing.T, d string) {
			var r plan.Resolved
			readJSONTest(t, filepath.Join(d, "resolved.json"), &r)
			r.ProtocolBinding = nil
			save(t, d, "resolved.json", r)
		}},
		{"unknown marker", func(t *testing.T, d string) {
			var m map[string]any
			readJSONTest(t, filepath.Join(d, "manifest.json"), &m)
			m["receipt_capture_interpretation"] = "future-unknown-v9"
			save(t, d, "manifest.json", m)
		}},
		{"corrupt resolved with protocol evidence", func(t *testing.T, d string) { text(t, d, "resolved.json", "{") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, o := protocolFixture(t)
			tc.mutate(t, d)
			if _, err := Analyze(o); err == nil {
				t.Fatal("protocol evidence silently downgraded to legacy")
			}
		})
	}
}

func TestProtocolFirstReceiptSelectionIsOrderIndependent(t *testing.T) {
	run := func(t *testing.T, rows []ProtocolReceipt) Step {
		d, o := protocolFixture(t)
		c := readCapture(t, d)
		base := c.Receipts[1]
		c.Receipts = append(c.Receipts[:1], c.Receipts[2:]...)
		c.Receipts = append(c.Receipts, rows...)
		_ = base
		saveCapture(t, d, c)
		return measurementStep(t, analyzeProtocolFixture(t, o))
	}
	rows := func() []ProtocolReceipt {
		return []ProtocolReceipt{{RunID: "SYNTHETIC-protocol-test", Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: 1700000002000, ObservedMS: 1700000002200, Outcome: protocol.OutcomeError}, {RunID: "SYNTHETIC-protocol-test", Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: 1700000002000, ObservedMS: 1700000002200, Outcome: protocol.OutcomeUnknown}, {RunID: "SYNTHETIC-protocol-test", Seq: 2, DeviceID: "device-2", Phase: "measurement", ScheduledSendMS: 1700000002000, ObservedMS: 1700000002100, Outcome: protocol.OutcomeOK}}
	}
	a := rows()
	s1 := run(t, a)
	reverse := rows()
	for i, j := 0, len(reverse)-1; i < j; i, j = i+1, j-1 {
		reverse[i], reverse[j] = reverse[j], reverse[i]
	}
	s2 := run(t, reverse)
	if s1.Claims["protocol_delivery"].Status != Pass || s2.Claims["protocol_delivery"].Status != Pass || metricValue(t, s1, "protocol.delivery.p99_ms") != metricValue(t, s2, "protocol.delivery.p99_ms") || metricValue(t, s1, "protocol.receipts.error") != metricValue(t, s2, "protocol.receipts.error") {
		t.Fatal(s1.Claims, s2.Claims, s1.Metrics, s2.Metrics)
	}
	for _, row := range rows() {
		if row.ObservedMS == 1700000002100 {
			row.Outcome = protocol.OutcomeError
			rowsForTie := []ProtocolReceipt{row, {RunID: row.RunID, Seq: row.Seq, DeviceID: row.DeviceID, Phase: row.Phase, ScheduledSendMS: row.ScheduledSendMS, ObservedMS: row.ObservedMS, Outcome: protocol.OutcomeOK}}
			s := run(t, rowsForTie)
			if s.Claims["protocol_delivery"].Status != Invalid {
				t.Fatal("conflicting earliest tie accepted", s.Claims)
			}
			break
		}
	}
}

func TestProtocolProvenanceComparisonAndCSVStaySemanticallySeparate(t *testing.T) {
	d, o := protocolFixture(t)
	protocolResult := analyzeProtocolFixture(t, o)
	out := filepath.Join(t.TempDir(), "protocol-report")
	if err := Write(out, protocolResult, d); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(out, "report.md"))
	for _, needle := range []string{ProtocolMetricInterpretation, protocolResult.Protocol.ProtocolSHA256, protocolResult.Protocol.ProfileSHA256, "Legacy profile assessments"} {
		if !strings.Contains(string(md), needle) {
			t.Fatalf("markdown missing %q", needle)
		}
	}
	legacyBytes, _ := json.Marshal(protocolResult)
	var legacy Result
	if err := json.Unmarshal(legacyBytes, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.RunID += "-legacy"
	legacy.MetricInterpretation = LegacyMetricInterpretation
	legacy.Protocol = nil
	comparison := Compare([]*Result{protocolResult, &legacy})
	if len(comparison.Rows) < 2 {
		t.Fatal("comparison rows missing")
	}
	groups := map[string]bool{}
	for _, row := range comparison.Rows {
		groups[row.Group] = true
	}
	if len(groups) < 2 {
		t.Fatal("comparison grouped distinct metric interpretations/bindings")
	}
	comparisonDir := filepath.Join(t.TempDir(), "comparison")
	if err := WriteComparison(comparisonDir, comparison, nil); err != nil {
		t.Fatal(err)
	}
	csvFile, err := os.Open(filepath.Join(comparisonDir, "comparison.csv"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(csvFile).ReadAll()
	_ = csvFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	index := map[string]int{}
	for i, h := range records[0] {
		index[h] = i
	}
	for _, field := range []string{"metric_interpretation", "protocol_id", "protocol_sha256", "profile_sha256", "protocol_delivery_status", "protocol_delivery_reasons"} {
		if _, ok := index[field]; !ok {
			t.Fatalf("comparison CSV lacks %s", field)
		}
	}
	foundProtocol := false
	for _, row := range records[1:] {
		if row[index["metric_interpretation"]] == ProtocolMetricInterpretation {
			foundProtocol = true
			if row[index["protocol_delivery_status"]] != "PASS" || row[index["protocol_sha256"]] == "" || row[index["profile_sha256"]] == "" {
				t.Fatal("CSV provenance/claim mismatch", row)
			}
		}
	}
	if !foundProtocol {
		t.Fatal("protocol row absent from CSV")
	}
}

func TestPlanProtocolPilotNullAndOneSidedMetadataReturnErrors(t *testing.T) {
	_, o := protocolFixture(t)
	var p plan.Plan
	readJSONTest(t, filepath.Join(o.Run, "plan.json"), &p)
	for _, tc := range []struct {
		name    string
		binding *protocol.Binding
		pilot   *protocol.PilotParameters
	}{{"binding-only", p.ProtocolBinding, nil}, {"pilot-only", nil, p.ProtocolPilot}, {"null-pilot", p.ProtocolBinding, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := p
			candidate.ProtocolBinding = tc.binding
			candidate.ProtocolPilot = tc.pilot
			if _, err := candidate.Resolve("SYNTHETIC-null-test"); err == nil {
				t.Fatal("one-sided/null protocol metadata accepted")
			}
		})
	}
}

func TestProtocolBindingCannotDowngradeAndRequiresVerifiedProfile(t *testing.T) {
	t.Run("new marker without binding", func(t *testing.T) {
		d, o := protocolFixture(t)
		var r plan.Resolved
		readJSONTest(t, filepath.Join(d, "resolved.json"), &r)
		r.ProtocolBinding = nil
		r.ProtocolPilot = nil
		save(t, d, "resolved.json", r)
		_, e := Analyze(o)
		if e == nil || !strings.Contains(e.Error(), "legacy fallback refused") {
			t.Fatal("new interpretation downgraded", e)
		}
	})
	t.Run("missing explicit protocol paths", func(t *testing.T) {
		_, o := protocolFixture(t)
		o.ProtocolPath = ""
		if _, e := Analyze(o); e == nil {
			t.Fatal("protocol path/root omission accepted")
		}
	})
	t.Run("manifest binding mismatch", func(t *testing.T) {
		d, o := protocolFixture(t)
		var manifest map[string]any
		readJSONTest(t, filepath.Join(d, "manifest.json"), &manifest)
		binding := manifest["protocol_binding"].(map[string]any)
		binding["protocol_sha256"] = strings.Repeat("0", 64)
		save(t, d, "manifest.json", manifest)
		if _, err := Analyze(o); err == nil || !strings.Contains(err.Error(), "protocol_binding") {
			t.Fatal("manifest/plan binding mismatch accepted", err)
		}
	})
	t.Run("altered protocol bytes", func(t *testing.T) {
		_, o := protocolFixture(t)
		changed := filepath.Join(t.TempDir(), "protocol.json")
		b, err := os.ReadFile(o.ProtocolPath)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(changed, append(b, ' '), 0644); err != nil {
			t.Fatal(err)
		}
		o.ProtocolPath = changed
		if _, err = Analyze(o); err == nil || !strings.Contains(err.Error(), "ID/hash mismatch") {
			t.Fatal("altered protocol accepted", err)
		}
	})
	t.Run("changed profile bytes", func(t *testing.T) {
		_, o := protocolFixture(t)
		root := t.TempDir()
		profileRel := "research/protocols/offline-profiles-20261101.json"
		dst := filepath.Join(root, profileRel)
		if e := os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(o.ProtocolRoot, profileRel))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(dst, append(b, ' '), 0644); e != nil {
			t.Fatal(e)
		}
		o.ProtocolRoot = root
		if _, e = Analyze(o); e == nil || !strings.Contains(e.Error(), "profile hash mismatch") {
			t.Fatal("altered profile accepted", e)
		}
	})
	t.Run("campaign stays fail closed", func(t *testing.T) {
		d, o := protocolFixture(t)
		var p plan.Plan
		readJSONTest(t, filepath.Join(d, "plan.json"), &p)
		p.ProtocolBinding.ExecutionClass = protocol.Campaign
		save(t, d, "plan.json", p)
		resolved, e := p.Resolve("SYNTHETIC-protocol-test")
		if e != nil {
			t.Fatal(e)
		}
		save(t, d, "resolved.json", resolved)
		var m map[string]any
		readJSONTest(t, filepath.Join(d, "manifest.json"), &m)
		m["protocol_binding"] = p.ProtocolBinding
		m["resolved_plan"] = resolved
		pb, _ := json.MarshalIndent(p, "", "  ")
		sum := sha256.Sum256(append(pb, '\n'))
		m["plan_sha256"] = hex.EncodeToString(sum[:])
		save(t, d, "manifest.json", m)
		_, e = Analyze(o)
		if e == nil || !strings.Contains(e.Error(), "campaign ineligible") {
			t.Fatal("campaign binding accepted", e)
		}
	})
}

func readJSONTest(t *testing.T, path string, v any) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, v); e != nil {
		t.Fatal(e)
	}
}
