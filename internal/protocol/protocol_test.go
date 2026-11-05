package protocol

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const definitionPath = "../../research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json"

func loadTest(t *testing.T) Loaded {
	t.Helper()
	p, e := LoadVerified(definitionPath, filepath.Join("..", ".."))
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestStrictDecodeAndSourceByteHashBinding(t *testing.T) {
	b, e := os.ReadFile(definitionPath)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Decode(append(append([]byte(nil), b...), []byte(` {"schema_version":1}`)...)); e == nil {
		t.Fatal("accepted trailing JSON")
	}
	var v map[string]any
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	v["surprise"] = true
	unknown, _ := json.Marshal(v)
	if _, e = Decode(unknown); e == nil {
		t.Fatal("accepted unknown field")
	}
	p := loadTest(t)
	d, err := p.Definition()
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := os.ReadFile("../../research/protocols/iotbench-telemetry-v0.1.0-pilot.1.json.sha256")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = LoadPinned(definitionPath, strings.TrimSpace(string(pinned))); err != nil {
		t.Fatal(err)
	}
	changed := append([]byte(nil), b...)
	changed = append(changed, ' ')
	if HashBytes(changed) == p.SHA256() {
		t.Fatal("source-byte hash ignored whitespace")
	}
	if _, err = LoadPinned(definitionPath, HashBytes(changed)); err == nil {
		t.Fatal("accepted mismatched pinned source hash")
	}
	if e = CheckEligibility(p, Binding{d.ProtocolID, HashBytes(changed), EngineeringPilot}, nil, EvidenceReferences{}); e == nil {
		t.Fatal("accepted mismatched protocol hash")
	}
}

func TestPilotCampaignFailClosedAndExplicitParameters(t *testing.T) {
	p := loadTest(t)
	d, err := p.Definition()
	if err != nil {
		t.Fatal(err)
	}
	b := Binding{ProtocolID: d.ProtocolID, ProtocolSHA256: p.SHA256(), ExecutionClass: EngineeringPilot}
	if e := CheckEligibility(p, b, nil, EvidenceReferences{}); e == nil {
		t.Fatal("pilot accepted unspecified numerical settings")
	}
	rate := 400.0
	measurement, warmup := 60, 10
	cutoff := int64(1700000000000)
	settings := &PilotParameters{RatePerSecond: &rate, MeasurementSeconds: &measurement, WarmupSeconds: &warmup, DrainCutoffUnixMS: &cutoff}
	if e := CheckEligibility(p, b, settings, EvidenceReferences{}); e != nil {
		t.Fatal(e)
	}
	b.ExecutionClass = Campaign
	if e := CheckEligibility(p, b, settings, EvidenceReferences{[]string{"q"}, []string{"c"}, []string{"p"}}); e == nil {
		t.Fatal("preliminary pending protocol became campaign eligible")
	}
	// Mutating the caller-visible Definition copy cannot alter the source-bound Loaded value.
	mutated, err := p.Definition()
	if err != nil {
		t.Fatal(err)
	}
	mutated.FreezeStatus = "frozen"
	mutated.Policies.PendingCalibration = nil
	if e := CheckEligibility(p, b, settings, EvidenceReferences{[]string{"looks-qualified"}, []string{"looks-calibrated"}, []string{"looks-profiled"}}); e == nil {
		t.Fatal("caller mutation bypassed preliminary campaign rejection")
	}
	for _, rate := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, -1} {
		bad := rate
		params := *settings
		params.RatePerSecond = &bad
		if e := CheckEligibility(p, Binding{d.ProtocolID, p.SHA256(), EngineeringPilot}, &params, EvidenceReferences{}); e == nil {
			t.Fatalf("accepted pilot rate %v", rate)
		}
	}
}

func TestNormativeContractMutationsAndDuplicateKeysRejected(t *testing.T) {
	original, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"omit serialization", func(m map[string]any) { delete(m["payload"].(map[string]any), "serialization") }},
		{"null numeric rules", func(m map[string]any) { m["payload"].(map[string]any)["numeric_rules"] = nil }},
		{"omit padding", func(m map[string]any) { delete(m["payload"].(map[string]any), "padding") }},
		{"change padding salt", func(m map[string]any) { m["payload"].(map[string]any)["padding_salt"] = "0xBAD" }},
		{"omit stored validation", func(m map[string]any) { delete(m["payload"].(map[string]any), "stored_validation") }},
		{"omit complete body policy", func(m map[string]any) {
			delete(m["receipt_capture"].(map[string]any), "response_complete_body_read_before_parse")
		}},
		{"omit capture field", func(m map[string]any) { delete(m["receipt_capture"].(map[string]any), "sink_change") }},
		{"omit receipt capture object", func(m map[string]any) { delete(m, "receipt_capture") }},
		{"capture null", func(m map[string]any) { m["receipt_capture"].(map[string]any)["sink_change"] = nil }},
		{"extra capture key", func(m map[string]any) { m["receipt_capture"].(map[string]any)["unexpected"] = "value" }},
		{"extra delivery outcome", func(m map[string]any) {
			x := m["delivery_metric"].(map[string]any)
			x["includes_outcomes"] = append(x["includes_outcomes"].([]any), "BOGUS")
		}},
		{"extra normalized state", func(m map[string]any) {
			x := m["outcomes"].(map[string]any)
			x["normalized_states"] = append(x["normalized_states"].([]any), "BOGUS")
		}},
		{"field order single joined element", func(m map[string]any) {
			m["payload"].(map[string]any)["field_order"] = []any{"run_id,seq,device_id,scheduled_send_ms,temperature,humidity,pressure,padding"}
		}},
		{"field order reordered", func(m map[string]any) {
			m["payload"].(map[string]any)["field_order"] = []any{"seq", "run_id", "device_id", "scheduled_send_ms", "temperature", "humidity", "pressure", "padding"}
		}},
		{"root case alias collision", func(m map[string]any) { m["SCHEMA_VERSION"] = m["schema_version"] }},
		{"nested case alias collision", func(m map[string]any) { m["receipt_capture"].(map[string]any)["SINK_CHANGE"] = nil }},
		{"policy source contradiction", func(m map[string]any) {
			m["policies"].(map[string]any)["warmup_seconds"].(map[string]any)["source"] = "invented#source"
		}},
		{"policy extra multiplier", func(m map[string]any) {
			m["policies"].(map[string]any)["warmup_seconds"].(map[string]any)["multiplier"] = 2
		}},
		{"null pending calibration item", func(m map[string]any) { a := m["policies"].(map[string]any)["pending_calibration"].([]any); a[0] = nil }},
		{"nested unknown policy key", func(m map[string]any) {
			m["policies"].(map[string]any)["warmup_seconds"].(map[string]any)["invented"] = "x"
		}},
		{"omit explicit null percentile", func(m map[string]any) { delete(m["delivery_metric"].(map[string]any), "zero_observed_percentile") }},
		{"non-null zero percentile", func(m map[string]any) { m["delivery_metric"].(map[string]any)["zero_observed_percentile"] = 0 }},
		{"omit explicit false policy", func(m map[string]any) {
			delete(m["policies"].(map[string]any)["separately_labeled_search_proposals"].(map[string]any), "optional_favorable_stopping")
		}},
	}
	for _, alias := range []string{"SCHEMA_VERSION", "Schema_Version", "Sink_Change", "SINK_CHANGE"} {
		var m map[string]any
		if e := json.Unmarshal(original, &m); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(alias, "SCHEMA") || strings.Contains(alias, "Schema") {
			m[alias] = m["schema_version"]
		} else {
			m["receipt_capture"].(map[string]any)[alias] = "out_of_scope"
		}
		b, _ := json.Marshal(m)
		if _, e := Decode(b); e == nil {
			t.Fatalf("accepted alias/collision %q", alias)
		}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if e := json.Unmarshal(original, &m); e != nil {
				t.Fatal(e)
			}
			tc.mutate(m)
			b, _ := json.Marshal(m)
			if _, e := Decode(b); e == nil {
				t.Fatal("mutated normative contract accepted")
			}
		})
	}
	for _, duplicate := range []string{`"schema_version":1,"schema_version":1`, `"sink_change":"out_of_scope","sink_change":"out_of_scope"`, `"run_id":"first","run_id":"pilot-1"`} {
		var raw string
		if strings.Contains(duplicate, "schema_version") {
			raw = strings.Replace(string(original), `"schema_version": 1`, duplicate, 1)
		} else if strings.Contains(duplicate, "sink_change") {
			raw = strings.Replace(string(original), `"sink_change": "out_of_scope"`, duplicate, 1)
		} else {
			raw = `{"run_id":"first","run_id":"pilot-1"}`
		}
		if strings.Contains(duplicate, "run_id") {
			if err := rejectDuplicateJSONKeys([]byte(raw)); err == nil {
				t.Fatal("duplicate payload key accepted")
			}
		} else if _, err := Decode([]byte(raw)); err == nil {
			t.Fatalf("duplicate contract key accepted: %s", duplicate)
		}
	}
	var canonical map[string]any
	if err := json.Unmarshal(original, &canonical); err != nil {
		t.Fatal(err)
	}
	for _, path := range contractLeafPaths(canonical, "$") {
		if path == "$.delivery_metric.zero_observed_percentile" {
			continue
		}
		t.Run("null-leaf-"+strings.ReplaceAll(path, ".", "_"), func(t *testing.T) {
			var copy map[string]any
			b, _ := json.Marshal(canonical)
			if e := json.Unmarshal(b, &copy); e != nil {
				t.Fatal(e)
			}
			setContractPathNull(copy, strings.TrimPrefix(path, "$."))
			raw, _ := json.Marshal(copy)
			if _, e := Decode(raw); e == nil {
				t.Fatalf("accepted null at %s", path)
			}
		})
	}
}

func contractLeafPaths(v any, path string) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			out = append(out, contractLeafPaths(value, path+"."+k)...)
		}
	case []any:
		return nil
	default:
		out = append(out, path)
	}
	return out
}
func setContractPathNull(root map[string]any, path string) {
	parts := strings.Split(path, ".")
	current := root
	for i, key := range parts {
		if i == len(parts)-1 {
			current[key] = nil
			return
		}
		current = current[key].(map[string]any)
	}
}

func TestEligibilityRechecksPinnedProfileBytes(t *testing.T) {
	root := t.TempDir()
	protocolDir := filepath.Join(root, "research", "protocols")
	if err := os.MkdirAll(protocolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	protocolBytes, err := os.ReadFile(definitionPath)
	if err != nil {
		t.Fatal(err)
	}
	profileBytes, err := os.ReadFile(filepath.Join("..", "..", "research", "protocols", "offline-profiles-20261101.json"))
	if err != nil {
		t.Fatal(err)
	}
	protocolPath := filepath.Join(protocolDir, "iotbench-telemetry-v0.1.0-pilot.1.json")
	profilePath := filepath.Join(protocolDir, "offline-profiles-20261101.json")
	if err = os.WriteFile(protocolPath, protocolBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(profilePath, profileBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadVerified(protocolPath, root)
	if err != nil {
		t.Fatal(err)
	}
	rate := 10.0
	measurement, warmup := 1, 0
	cutoff := int64(1)
	pilot := &PilotParameters{RatePerSecond: &rate, MeasurementSeconds: &measurement, WarmupSeconds: &warmup, DrainCutoffUnixMS: &cutoff}
	d, err := loaded.Definition()
	if err != nil {
		t.Fatal(err)
	}
	binding := Binding{d.ProtocolID, loaded.SHA256(), EngineeringPilot}
	if err = CheckEligibility(loaded, binding, pilot, EvidenceReferences{}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(profilePath, append(profileBytes, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if err = CheckEligibility(loaded, binding, pilot, EvidenceReferences{}); err == nil {
		t.Fatal("profile changed after load but eligibility trusted stale verification")
	}
}

func TestDesignDimensionsAndD44ProfileBinding(t *testing.T) {
	p := loadTest(t)
	definition, err := p.Definition()
	if err != nil {
		t.Fatal(err)
	}
	d := definition.Design
	if !intsEqual(d.ApplicationCPUBudgetK, []int{1, 2, 3}) || !intsEqual(d.ReplicaCountsN, []int{1, 2, 3}) {
		t.Fatal("K and N dimensions missing")
	}
	if d.ReplicaMaps["thingsboard"]["tb-core"] != "N" || d.ReplicaMaps["fiware"]["orion"] != "N" || d.ReplicaMaps["edgex"]["app-http-export"] != float64(1) {
		t.Fatalf("replica map %+v", d.ReplicaMaps)
	}
	if e := VerifyProfile(filepath.Join("..", ".."), definition.Profiles); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join("..", "..", definition.Profiles.Path))
	if e != nil {
		t.Fatal(e)
	}
	compact := bytes.ReplaceAll(b, []byte(" "), nil)
	for _, s := range []string{`"threshold":500`, `"threshold":250`, `"threshold":1000`, `"threshold":5000`, `"populations":[100,10000]`} {
		if !bytes.Contains(compact, []byte(s)) {
			t.Fatalf("D44 profile semantics changed; missing %s", s)
		}
	}
}

func TestDeliveryMetricNearestRankAndCounts(t *testing.T) {
	if *NearestRank([]float64{8, 1, 4, 2}, 50) != 2 || *NearestRank([]float64{8, 1, 4, 2}, 99) != 8 {
		t.Fatal("nearest-rank percentile mismatch")
	}
	if NearestRank(nil, 99) != nil {
		t.Fatal("zero-observed p99 must be null")
	}
	if c := Counts(5, 3); c.Scheduled != 5 || c.Observed != 3 || c.Missing != 2 {
		t.Fatalf("counts=%+v", c)
	}
	if !ValidateOutcome(OutcomeUnknown, false) || ValidateOutcome(OutcomeNotSent, false) || !ValidateOutcome(OutcomeNotSent, true) {
		t.Fatal("outcome normalization/evidence rule failed")
	}
	if n, ratio := Attainment(5, []float64{100, 600, 20}, 500); n != 2 || ratio != .4 {
		t.Fatalf("scheduled attainment %d/%g", n, ratio)
	}
}

func TestCanonicalPayloadAndStoredContentValidation(t *testing.T) {
	p := LogicalPayload{RunID: "pilot-1", Seq: 17, DeviceID: "device-17", ScheduledSendMS: 1234.125, Temperature: 20, Humidity: 40, Pressure: 1000}
	b, e := CanonicalBytes(p)
	if e != nil {
		t.Fatal(e)
	}
	want := `{"run_id":"pilot-1","seq":17,"device_id":"device-17","scheduled_send_ms":1234.125,"temperature":20,"humidity":40,"pressure":1000,"padding":""}`
	if string(b) != want {
		t.Fatalf("canonical bytes\n got: %s\nwant: %s", b, want)
	}
	if e = ValidateStored(p, b); e != nil {
		t.Fatal(e)
	}
	padded, e := BuildPayload(p, 7, 160)
	if e != nil {
		t.Fatal(e)
	}
	if want := `{"run_id":"pilot-1","seq":17,"device_id":"device-17","scheduled_send_ms":1234.125,"temperature":20,"humidity":40,"pressure":1000,"padding":"a9f0653a6090799138"}`; string(padded) != want {
		t.Fatalf("cross-language padded golden mismatch: %s", padded)
	}
	offset, e := BuildPayloadOffset(p, 7, 1024)
	if e != nil || len(offset) != len(b)+1024 {
		t.Fatalf("offset payload size=%d err=%v", len(offset), e)
	}
	if e = ValidateStored(p, b[:len(b)-4]); e == nil {
		t.Fatal("accepted truncated stored content")
	}
	altered := strings.Replace(string(b), `"temperature":20`, `"temperature":21`, 1)
	if e = ValidateStored(p, []byte(altered)); e == nil {
		t.Fatal("accepted altered stored field")
	}
	if _, e = BuildPayload(p, 7, 1024); e != nil {
		t.Fatal(e)
	}
	tooSmall := len(b) - 1
	if _, e = BuildPayload(p, 7, tooSmall); e == nil {
		t.Fatal("accepted undersized payload")
	}
	rounded := p
	rounded.ScheduledSendMS = 1234.1251
	if _, e = CanonicalBytes(rounded); e == nil {
		t.Fatal("silently rounded timestamp")
	}
}

func TestStoredPayloadRequiresAllTypedFieldsAndRejectsDuplicateKeys(t *testing.T) {
	p := LogicalPayload{RunID: "pilot-1", Seq: 0, DeviceID: "device-0", ScheduledSendMS: 0, Temperature: 15, Humidity: 30, Pressure: 950, Padding: ""}
	b, err := CanonicalBytes(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateStored(p, b); err != nil {
		t.Fatal(err)
	}
	if err = ValidateStored(p, bytes.Replace(b, []byte(`"scheduled_send_ms":0`), []byte(`"scheduled_send_ms":-0.0`), 1)); err != nil {
		t.Fatalf("negative-zero stored value should normalize consistently: %v", err)
	}
	base := string(b)
	cases := map[string]string{
		"missing zero seq":               strings.Replace(base, `"seq":0,`, "", 1),
		"null zero seq":                  strings.Replace(base, `"seq":0,`, `"seq":null,`, 1),
		"wrong type zero seq":            strings.Replace(base, `"seq":0,`, `"seq":"0",`, 1),
		"missing zero timestamp":         strings.Replace(base, `,"scheduled_send_ms":0`, "", 1),
		"null zero timestamp":            strings.Replace(base, `"scheduled_send_ms":0`, `"scheduled_send_ms":null`, 1),
		"wrong type timestamp":           strings.Replace(base, `"scheduled_send_ms":0`, `"scheduled_send_ms":"0"`, 1),
		"missing empty padding":          strings.Replace(base, `,"padding":""`, "", 1),
		"null empty padding":             strings.Replace(base, `"padding":""`, `"padding":null`, 1),
		"wrong type padding":             strings.Replace(base, `"padding":""`, `"padding":0`, 1),
		"wrong type identity":            strings.Replace(base, `"run_id":"pilot-1"`, `"run_id":0`, 1),
		"duplicate conflicting identity": strings.Replace(base, `"run_id":"pilot-1"`, `"run_id":"evil","run_id":"pilot-1"`, 1),
		"duplicate conflicting seq":      strings.Replace(base, `"seq":0`, `"seq":1,"seq":0`, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateStored(p, []byte(input)); err == nil {
				t.Fatal("invalid stored JSON accepted")
			}
		})
	}
}

func TestRawStoredNumericTokensAreExactIntegersAndRejectUnderflow(t *testing.T) {
	p := LogicalPayload{RunID: "pilot-1", Seq: 0, DeviceID: "device-0", ScheduledSendMS: 0, Temperature: 15, Humidity: 30, Pressure: 950}
	base := string(mustPayloadBytes(t, p))
	for _, tc := range []struct{ field, token string }{
		{"seq", "1e-400"}, {"temperature", "15.0000000000000001"}, {"humidity", "30.0000000000000001"}, {"pressure", "950.0000000000000001"},
		{"seq", "0.0"}, {"temperature", "15e0"}, {"humidity", "30.0"}, {"pressure", "950.0"},
	} {
		t.Run(tc.field+"="+tc.token, func(t *testing.T) {
			key := `"` + tc.field + `":`
			start := strings.Index(base, key)
			end := strings.Index(base[start+len(key):], ",")
			if end < 0 {
				end = strings.Index(base[start+len(key):], "}")
			}
			mutated := base[:start+len(key)] + tc.token + base[start+len(key)+end:]
			if err := ValidateStored(p, []byte(mutated)); err == nil {
				t.Fatalf("accepted noncanonical raw integer %s", tc.token)
			}
		})
	}
	for _, token := range []string{"1e-400", "1e-9223372036854775808", "1e999999999999999999999", "-1e-400", "0e10001", "-0e-10001"} {
		mutated := strings.Replace(base, `"scheduled_send_ms":0`, `"scheduled_send_ms":`+token, 1)
		if err := ValidateStored(p, []byte(mutated)); err == nil {
			t.Fatalf("accepted unsupported/underflow timestamp token %q", token)
		}
	}
	for _, token := range []string{"-0", "-0.0", "-0e3"} {
		mutated := strings.Replace(base, `"scheduled_send_ms":0`, `"scheduled_send_ms":`+token, 1)
		if err := ValidateStored(p, []byte(mutated)); err != nil {
			t.Fatalf("rejected signed decimal zero %q: %v", token, err)
		}
	}
}

func TestValidateStoredPropagatesOversizedCanonicalEncodingErrors(t *testing.T) {
	base := LogicalPayload{RunID: "r", Seq: 0, DeviceID: "d", ScheduledSendMS: 0, Temperature: 15, Humidity: 30, Pressure: 950}
	tooLargeExpected := base
	tooLargeExpected.Padding = strings.Repeat("a", PayloadMaximumBytes)
	validStored, err := CanonicalBytes(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateStored(tooLargeExpected, validStored); err == nil {
		t.Fatal("oversized expected payload encoding error was ignored")
	}
	actual := base
	actual.RunID = "different"
	actual.Temperature = 16
	actual.Padding = strings.Repeat("b", PayloadMaximumBytes)
	stored, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1048703 {
		t.Fatalf("regression fixture size=%d want 1048703", len(stored))
	}
	if err = ValidateStored(base, stored); err == nil {
		t.Fatal("oversized actual encoding errors were ignored and unequal payloads matched")
	}
	actualExpected := actual
	if err = ValidateStored(actualExpected, stored); err == nil {
		t.Fatal("both oversized encodings were treated as equal")
	}
}

func TestTimestampExactDecimalRulesAndNegativeZero(t *testing.T) {
	base := LogicalPayload{RunID: "pilot-1", Seq: 1, DeviceID: "device-1", Temperature: 20, Humidity: 40, Pressure: 1000}
	for _, ts := range []float64{0, 0.001, 1048576.001, 9007199254740991, -0.0} {
		p := base
		p.ScheduledSendMS = ts
		if _, err := CanonicalBytes(p); err != nil {
			t.Fatalf("valid timestamp %g rejected: %v", ts, err)
		}
	}
	for _, vector := range []struct {
		timestamp float64
		want      string
	}{
		{0, `{"run_id":"pilot-1","seq":1,"device_id":"device-1","scheduled_send_ms":0,"temperature":20,"humidity":40,"pressure":1000,"padding":""}`},
		{1048576.001, `{"run_id":"pilot-1","seq":1,"device_id":"device-1","scheduled_send_ms":1048576.001,"temperature":20,"humidity":40,"pressure":1000,"padding":""}`},
		{9007199254740991, `{"run_id":"pilot-1","seq":1,"device_id":"device-1","scheduled_send_ms":9007199254740991,"temperature":20,"humidity":40,"pressure":1000,"padding":""}`},
	} {
		v := base
		v.ScheduledSendMS = vector.timestamp
		got, e := CanonicalBytes(v)
		if e != nil || string(got) != vector.want {
			t.Fatalf("timestamp %g bytes=%s err=%v", vector.timestamp, got, e)
		}
	}
	for _, ts := range []float64{0.0001, 1048576.0011, 9007199254740992} {
		p := base
		p.ScheduledSendMS = ts
		if _, err := CanonicalBytes(p); err == nil {
			t.Fatalf("invalid timestamp %g accepted", ts)
		}
	}
	negativeZero := base
	negativeZero.ScheduledSendMS = math.Copysign(0, -1)
	positiveZero := base
	positiveZero.ScheduledSendMS = 0
	neg, err := CanonicalBytes(negativeZero)
	if err != nil {
		t.Fatal(err)
	}
	pos, err := CanonicalBytes(positiveZero)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(neg, pos) || bytes.Contains(neg, []byte(`"scheduled_send_ms":-0`)) {
		t.Fatalf("negative zero not normalized: %s", neg)
	}
	validExpected := base
	validExpected.ScheduledSendMS = 1048576.001
	validStored := mustPayloadBytes(t, validExpected)
	if err := ValidateStored(validExpected, validStored); err != nil {
		t.Fatalf("valid boundary timestamp rejected: %v", err)
	}
	invalidPrecision := strings.Replace(string(validStored), `1048576.001`, `1048576.0010000000001`, 1)
	if err := ValidateStored(validExpected, []byte(invalidPrecision)); err == nil {
		t.Fatal("rounded float hid excess raw decimal precision")
	}
}

func TestScientificShortestTimestampScaleAcrossSafeRange(t *testing.T) {
	base := LogicalPayload{RunID: "pilot-1", Seq: 9, DeviceID: "device-9", Temperature: 20, Humidity: 40, Pressure: 1000}
	valueSet := map[int64]bool{0: true, 1: true, 1700000000000: true, 1793664000000: true, 9007199254740990: true, 9007199254740991: true}
	for power := int64(1_000_000); power <= 1_000_000_000_000_000; power *= 10 {
		valueSet[power-1] = true
		valueSet[power] = true
		valueSet[power+1] = true
	}
	values := make([]string, 0, len(valueSet))
	for value := range valueSet {
		values = append(values, strconv.FormatInt(value, 10))
	}
	sort.Strings(values)
	for _, token := range values {
		t.Run(token, func(t *testing.T) {
			ts, err := strconv.ParseFloat(token, 64)
			if err != nil {
				t.Fatal(err)
			}
			p := base
			p.ScheduledSendMS = ts
			encoded, err := CanonicalBytes(p)
			if err != nil {
				t.Fatalf("valid timestamp %s rejected: %v", token, err)
			}
			needle := []byte(`"scheduled_send_ms":` + token + `,`)
			if !bytes.Contains(encoded, needle) {
				t.Fatalf("canonical bytes don't preserve timestamp %s: %s", token, encoded)
			}
			if err := ValidateStored(p, encoded); err != nil {
				t.Fatalf("stored canonical timestamp %s rejected: %v", token, err)
			}
		})
	}
	for _, token := range []string{"1000000.001", "1700000000000.001", "1793664000000.001"} {
		ts, err := strconv.ParseFloat(token, 64)
		if err != nil {
			t.Fatal(err)
		}
		p := base
		p.ScheduledSendMS = ts
		encoded, err := CanonicalBytes(p)
		if err != nil {
			t.Fatalf("valid fractional timestamp %s rejected: %v", token, err)
		}
		if !bytes.Contains(encoded, []byte(`"scheduled_send_ms":`+token+`,`)) {
			t.Fatalf("fractional timestamp not preserved: %s", encoded)
		}
		if err = ValidateStored(p, encoded); err != nil {
			t.Fatalf("fractional timestamp %s stored validation: %v", token, err)
		}
	}
}

func mustPayloadBytes(t *testing.T, p LogicalPayload) []byte {
	t.Helper()
	b, e := CanonicalBytes(p)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestPayloadPaddingMaximumParityAndIntegerBoundaries(t *testing.T) {
	p := LogicalPayload{RunID: "r", Seq: ^uint32(0), DeviceID: "d", ScheduledSendMS: 0, Temperature: 15, Humidity: 30, Pressure: 950}
	base, err := CanonicalBytes(p)
	if err != nil {
		t.Fatal(err)
	}
	maxTarget := PayloadMaximumBytes
	if (maxTarget-len(base))%2 != 0 {
		maxTarget--
	}
	maxBytes, err := BuildPayload(p, ^uint32(0), maxTarget)
	if err != nil {
		t.Fatalf("maximum feasible body rejected: %v", err)
	}
	if len(maxBytes) != maxTarget || len(maxBytes) > PayloadMaximumBytes {
		t.Fatalf("bad maximum body size %d", len(maxBytes))
	}
	if _, err = BuildPayload(p, 0, maxTarget+1); err == nil {
		t.Fatal("maximum+1 payload target accepted")
	}
	if _, err = BuildPayload(p, 0, PayloadMaximumBytes+1); err == nil {
		t.Fatal("1 MiB plus one payload target accepted")
	}
	if _, err = BuildPayload(p, 0, len(base)+1); err == nil {
		t.Fatal("unrepresentable odd padding increment accepted")
	}
	if _, err = BuildPayloadOffset(p, 0, PayloadMaximumBytes); err == nil {
		t.Fatal("offset exceeding total bound accepted")
	}
	if _, err = BuildPayloadOffset(p, 0, PayloadMaximumBytes+1); err == nil {
		t.Fatal("offset maximum+1 accepted")
	}
	if _, err = BuildPayloadOffset(p, 0, -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	reviewer := LogicalPayload{RunID: "r", Seq: 0, DeviceID: "d", ScheduledSendMS: 0, Temperature: 15, Humidity: 30, Pressure: 950}
	reviewerBase, e := CanonicalBytes(reviewer)
	if e != nil {
		t.Fatal(e)
	}
	if len(reviewerBase) != 119 {
		t.Fatalf("reviewer boundary baseline=%d want119", len(reviewerBase))
	}
	maxOffset, err := MaximumPayloadOffset(len(reviewerBase))
	if err != nil {
		t.Fatal(err)
	}
	if maxOffset%2 != 0 || maxOffset+len(reviewerBase) > PayloadMaximumBytes {
		t.Fatalf("bad maximum offset %d for baseline %d", maxOffset, len(reviewerBase))
	}
	if _, err = BuildPayloadOffset(reviewer, 0, maxOffset); err != nil {
		t.Fatalf("maximum offset rejected by encoder: %v", err)
	}
	if _, err = BuildPayloadOffset(reviewer, 0, maxOffset+2); err == nil {
		t.Fatal("maximum+2 offset accepted")
	}
	if _, err = MaximumPayloadOffset(PayloadMaximumBytes + 1); err == nil {
		t.Fatal("baseline beyond maximum accepted")
	}
	if upper, err := PayloadBaselineUpperBound("r", "d", 0); err != nil || upper < len(reviewerBase) {
		t.Fatalf("baseline upper bound %d err=%v", upper, err)
	}
	if got, e := BuildPayload(reviewer, 0, 1048575); e != nil || len(got) != 1048575 {
		t.Fatalf("1048575-byte boundary body len=%d err=%v", len(got), e)
	}
}

// The plan bound must use the contract maxima, not the three-digit numeric minima: pressure 1150
// is one canonical byte longer than 950.
func TestPayloadBaselineUpperBoundIncludesContractNumericMaxima(t *testing.T) {
	const runID, deviceID = "r", "dd"
	const worstTimestamp = 1000000000000000.6
	min := LogicalPayload{RunID: runID, Seq: 0, DeviceID: deviceID, ScheduledSendMS: worstTimestamp,
		Temperature: 15, Humidity: 30, Pressure: 950}
	max := min
	max.Temperature, max.Humidity, max.Pressure = 45, 80, 1150
	minBytes, err := CanonicalBytes(min)
	if err != nil {
		t.Fatal(err)
	}
	maxBytes, err := CanonicalBytes(max)
	if err != nil {
		t.Fatal(err)
	}
	if len(maxBytes) != len(minBytes)+1 {
		t.Fatalf("numeric maximum baseline=%d, minimum=%d; want one byte longer", len(maxBytes), len(minBytes))
	}
	upper, err := PayloadBaselineUpperBound(runID, deviceID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if upper != len(maxBytes) {
		t.Fatalf("baseline upper bound=%d, actual worst case=%d", upper, len(maxBytes))
	}
}

// MaximumPayloadOffset must leave a target that the real encoder can build at the pressure-1150
// baseline; an offset derived from the 950 baseline overflows by one byte.
func TestPayloadMaximumOffsetCoversPressure1150(t *testing.T) {
	min := LogicalPayload{RunID: "r", Seq: 0, DeviceID: "dd", ScheduledSendMS: 0,
		Temperature: 15, Humidity: 30, Pressure: 950}
	max := min
	max.Temperature, max.Humidity, max.Pressure = 45, 80, 1150
	minBytes, err := CanonicalBytes(min)
	if err != nil {
		t.Fatal(err)
	}
	maxBytes, err := CanonicalBytes(max)
	if err != nil {
		t.Fatal(err)
	}
	if len(maxBytes) != len(minBytes)+1 {
		t.Fatalf("numeric maximum baseline=%d, minimum=%d; want one byte longer", len(maxBytes), len(minBytes))
	}
	minOffset, err := MaximumPayloadOffset(len(minBytes))
	if err != nil {
		t.Fatal(err)
	}
	if len(minBytes)%2 == 0 && len(minBytes)+minOffset != PayloadMaximumBytes {
		t.Fatalf("minimum baseline %d + max offset %d = %d, want %d", len(minBytes), minOffset, len(minBytes)+minOffset, PayloadMaximumBytes)
	}
	maxOffset, err := MaximumPayloadOffset(len(maxBytes))
	if err != nil {
		t.Fatal(err)
	}
	if maxOffset%2 != 0 || len(maxBytes)+maxOffset > PayloadMaximumBytes {
		t.Fatalf("bad maximum offset %d for baseline %d", maxOffset, len(maxBytes))
	}
	if len(maxBytes)+maxOffset+2 <= PayloadMaximumBytes {
		t.Fatalf("maximum offset %d is not maximal for baseline %d", maxOffset, len(maxBytes))
	}
	out, err := BuildPayloadOffset(max, 17, maxOffset)
	if err != nil {
		t.Fatalf("accepted maximum does not encode pressure 1150: %v", err)
	}
	if len(out) != len(maxBytes)+maxOffset {
		t.Fatalf("encoded length=%d want %d", len(out), len(maxBytes)+maxOffset)
	}
	if _, err = BuildPayloadOffset(max, 17, maxOffset-2); err != nil {
		t.Fatalf("nearest lower even offset rejected: %v", err)
	}
	if _, err = BuildPayloadOffset(max, 17, maxOffset+1); err == nil {
		t.Fatal("odd offset max+1 accepted")
	}
	if _, err = BuildPayloadOffset(max, 17, maxOffset+2); err == nil {
		t.Fatal("offset max+2 accepted")
	}
	if _, err = BuildPayloadOffset(max, 17, 1); err == nil {
		t.Fatal("odd minimum offset accepted")
	}
}

// The samples used by the plan bound must include the longest shortest timestamp token: 16 integer
// digits plus a decimal point and fractional digit. All 18-byte forms are length-equivalent for the
// bound, so covering each integer-digit width is sufficient.
func TestPayloadTimestampSamplesCoverLongestCanonicalForm(t *testing.T) {
	longest := 0
	for _, timestamp := range payloadTimestampSamples {
		b, err := json.Marshal(timestamp)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > longest {
			longest = len(b)
		}
	}
	if longest != 18 {
		t.Fatalf("longest sampled timestamp token=%d bytes, want 18", longest)
	}
	for _, timestamp := range []float64{10000000000000.123, 100000000000000.12, 1000000000000000.6} {
		b, err := json.Marshal(timestamp)
		if err != nil || len(b) != longest {
			t.Fatalf("timestamp %g token=%s len=%d err=%v; want %d bytes", timestamp, b, len(b), err, longest)
		}
	}
}
