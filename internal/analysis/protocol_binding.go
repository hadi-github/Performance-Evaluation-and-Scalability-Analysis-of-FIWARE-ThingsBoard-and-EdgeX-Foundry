package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"

	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/receipt"
)

// guardProtocolIntent runs before any legacy early return. Surviving protocol metadata
// means a missing or invalid resolved binding is an error, never a legacy downgrade.
func guardProtocolIntent(run string) error {
	intent := false
	for _, name := range []string{"plan.json", "resolved.json", "manifest.json"} {
		b, err := os.ReadFile(filepath.Join(run, name))
		if err != nil {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			if bytes.Contains(b, []byte("protocol_")) || bytes.Contains(b, []byte("interpretation")) {
				return fmt.Errorf("%s contains malformed protocol metadata", name)
			}
			continue
		}
		for _, key := range []string{"protocol_binding", "protocol_pilot_parameters", "receipt_capture_interpretation", "metric_interpretation"} {
			if _, ok := fields[key]; ok {
				intent = true
			}
		}
		if raw, ok := fields["runtime_metric_interpretation"]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || (value != "" && value != LegacyMetricInterpretation) {
				return errors.New("unsupported runtime metric interpretation")
			}
		}
		if raw, ok := fields["receipt_capture_interpretation"]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || (value != "" && value != ProtocolMetricInterpretation) {
				return errors.New("unsupported receipt capture interpretation")
			}
		}
		if raw, ok := fields["metric_interpretation"]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || (value != LegacyMetricInterpretation && value != ProtocolMetricInterpretation) {
				return errors.New("unsupported metric interpretation")
			}
		}
	}
	if _, err := os.Stat(filepath.Join(run, "protocol-receipts.json")); err == nil {
		intent = true
	}
	if !intent {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(run, "resolved.json"))
	if err != nil {
		return errors.New("protocol evidence exists but resolved.json is unavailable; legacy fallback refused")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) != nil {
		return errors.New("protocol evidence exists but resolved.json is malformed; legacy fallback refused")
	}
	for _, key := range []string{"protocol_binding", "protocol_pilot_parameters"} {
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("protocol evidence exists but resolved.json %s is missing; legacy fallback refused", key)
		}
	}
	return nil
}

func validateProtocolCaptureRaw(b []byte) error {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return err
	}
	var schemaVersion int
	if err := json.Unmarshal(top["schema_version"], &schemaVersion); err != nil {
		return fmt.Errorf("schema_version: %w", err)
	}
	required := []string{"schema_version", "metric_interpretation", "protocol_binding", "synthetic", "complete", "endpoint", "source", "body_read_complete", "clock_domain", "clock_source", "coverage_start_ms", "coverage_end_ms", "observation_cutoff_ms", "receipts"}
	switch schemaVersion {
	case receipt.SchemaVersion:
		required = append(required, "receipt_capture_semantics")
	case receipt.LegacySchemaVersion:
		// Schema 1 remains readable as a legacy capture. It cannot claim the new primary metrics.
	default:
		return fmt.Errorf("unsupported receipt capture schema_version %d", schemaVersion)
	}
	for _, key := range required {
		if _, ok := top[key]; !ok {
			return fmt.Errorf("missing required capture field %q", key)
		}
	}
	if raw, ok := top["receipt_capture_semantics"]; ok && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("receipt_capture_semantics must not be null")
	}
	allowedTop := map[string]bool{"schema_version": true, "metric_interpretation": true, "protocol_binding": true, "synthetic": true, "complete": true, "endpoint": true, "source": true, "body_read_complete": true, "receipt_capture_semantics": true, "clock_domain": true, "clock_source": true, "coverage_start_ms": true, "coverage_end_ms": true, "observation_cutoff_ms": true, "receipts": true}
	for key := range top {
		if !allowedTop[key] {
			return fmt.Errorf("unsupported or incorrectly cased capture field %q", key)
		}
	}
	var binding map[string]json.RawMessage
	if err := json.Unmarshal(top["protocol_binding"], &binding); err != nil {
		return fmt.Errorf("protocol_binding: %w", err)
	}
	for _, key := range []string{"protocol_id", "protocol_sha256", "execution_class"} {
		if _, ok := binding[key]; !ok {
			return fmt.Errorf("protocol_binding missing exact field %q", key)
		}
	}
	for key := range binding {
		if key != "protocol_id" && key != "protocol_sha256" && key != "execution_class" {
			return fmt.Errorf("unsupported or incorrectly cased protocol_binding field %q", key)
		}
	}
	var receipts []json.RawMessage
	if err := json.Unmarshal(top["receipts"], &receipts); err != nil || receipts == nil {
		return errors.New("receipts must be an explicit JSON array; null is invalid")
	}
	for i, raw := range receipts {
		var row map[string]json.RawMessage
		if err := json.Unmarshal(raw, &row); err != nil {
			return fmt.Errorf("receipt %d: %w", i, err)
		}
		for _, key := range []string{"run_id", "seq", "device_id", "phase", "scheduled_send_ms", "observed_ms", "outcome"} {
			value, ok := row[key]
			if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return fmt.Errorf("receipt %d missing/null required field %q", i, key)
			}
		}
		for key := range row {
			if key != "run_id" && key != "seq" && key != "device_id" && key != "phase" && key != "scheduled_send_ms" && key != "observed_ms" && key != "outcome" {
				return fmt.Errorf("receipt %d has unsupported or incorrectly cased field %q", i, key)
			}
		}
	}
	return nil
}

// rejectDuplicateJSONKeys rejects duplicate keys at every nesting level before typed decoding.
func rejectDuplicateJSONKeys(b []byte) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var value func() error
	value = func() error {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if err = value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		case '[':
			for d.More() {
				if err := value(); err != nil {
					return err
				}
			}
			_, err = d.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := d.Token(); err == nil {
		return errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// prepareProtocolBinding distinguishes legacy artifacts from explicitly bound artifacts.
// A new-interpretation marker, binding metadata, or receipt capture may never fall back to legacy.
func prepareProtocolBinding(o Options, rd reader, resolved *plan.Resolved) (*ProtocolProvenance, []byte, map[string]json.RawMessage, error) {
	manifestPath := filepath.Join(o.Run, "manifest.json")
	manifestBytes, manifestErr := os.ReadFile(manifestPath)
	manifest := map[string]json.RawMessage{}
	if manifestErr == nil {
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			return nil, nil, nil, fmt.Errorf("manifest malformed: %w", err)
		}
	}
	marker := ""
	if raw, ok := manifest["receipt_capture_interpretation"]; ok {
		if err := json.Unmarshal(raw, &marker); err != nil {
			return nil, nil, nil, errors.New("manifest receipt_capture_interpretation malformed")
		}
	}
	_, captureErr := os.Stat(filepath.Join(o.Run, "protocol-receipts.json"))
	manifestHasBinding := manifest["protocol_binding"] != nil || manifest["protocol_pilot_parameters"] != nil
	if raw, ok := manifest["runtime_metric_interpretation"]; ok {
		var interpretation string
		if json.Unmarshal(raw, &interpretation) != nil || (interpretation != "" && interpretation != LegacyMetricInterpretation) {
			return nil, nil, nil, errors.New("manifest contains an unsupported runtime metric interpretation")
		}
	}
	if resolved.ProtocolBinding == nil {
		if marker == ProtocolMetricInterpretation || manifestHasBinding || captureErr == nil {
			return nil, nil, nil, errors.New("artifact claims protocol interpretation/capture but resolved plan has no protocol binding")
		}
		return nil, nil, nil, nil
	}
	if marker != "" && marker != ProtocolMetricInterpretation {
		return nil, nil, nil, fmt.Errorf("unsupported receipt capture interpretation %q", marker)
	}
	if manifestErr != nil {
		return nil, nil, nil, fmt.Errorf("protocol-bound analysis requires manifest.json: %w", manifestErr)
	}
	manifestBytesRead, readErr := rd.read("manifest", manifestPath)
	if readErr != nil {
		return nil, nil, nil, fmt.Errorf("manifest: %w", readErr)
	}
	if !reflect.DeepEqual(manifestBytes, manifestBytesRead) {
		return nil, nil, nil, errors.New("manifest changed during protocol-binding validation")
	}
	manifestBytes = manifestBytesRead
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, nil, nil, fmt.Errorf("manifest malformed: %w", err)
	}
	if o.ProtocolPath == "" || o.ProtocolRoot == "" {
		return nil, nil, nil, errors.New("protocol-bound analysis requires explicit -protocol and -protocol-root")
	}
	planBytes, err := rd.read("plan", filepath.Join(o.Run, "plan.json"))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("protocol-bound analysis requires plan.json: %w", err)
	}
	p, err := plan.Decode(planBytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("plan.json: %w", err)
	}
	if !reflect.DeepEqual(*p, resolved.Plan) {
		return nil, nil, nil, errors.New("plan.json and resolved.json plan metadata disagree")
	}
	rebuilt, err := p.Resolve(resolved.RunID)
	if err != nil || !reflect.DeepEqual(rebuilt, resolved) {
		return nil, nil, nil, errors.New("resolved.json is inconsistent with plan.json")
	}
	var sum [32]byte
	sum = sha256.Sum256(planBytes)
	var planHash string
	if json.Unmarshal(manifest["plan_sha256"], &planHash) != nil || planHash != hex.EncodeToString(sum[:]) {
		return nil, nil, nil, errors.New("manifest plan_sha256 is missing or inconsistent")
	}
	for key, want := range map[string]string{"run_id": resolved.RunID, "plan_id": resolved.PlanID} {
		var got string
		if json.Unmarshal(manifest[key], &got) != nil || got != want {
			return nil, nil, nil, fmt.Errorf("manifest %s is missing or inconsistent", key)
		}
	}
	var manifestBinding protocol.Binding
	if json.Unmarshal(manifest["protocol_binding"], &manifestBinding) != nil || !reflect.DeepEqual(&manifestBinding, resolved.ProtocolBinding) {
		return nil, nil, nil, errors.New("manifest protocol_binding is missing or contradicts plan/resolved metadata")
	}
	var manifestPilot protocol.PilotParameters
	if json.Unmarshal(manifest["protocol_pilot_parameters"], &manifestPilot) != nil || !reflect.DeepEqual(&manifestPilot, resolved.ProtocolPilot) {
		return nil, nil, nil, errors.New("manifest protocol pilot parameters are missing or contradictory")
	}
	var runtimeInterpretation string
	if json.Unmarshal(manifest["runtime_metric_interpretation"], &runtimeInterpretation) != nil || runtimeInterpretation != "legacy-acknowledgement-only-v1" {
		return nil, nil, nil, errors.New("runner runtime must explicitly retain legacy acknowledgement-only interpretation; instrumentation is not upgraded")
	}
	if raw, ok := manifest["resolved_plan"]; !ok {
		return nil, nil, nil, errors.New("manifest resolved_plan missing")
	} else {
		var mplan plan.Resolved
		if json.Unmarshal(raw, &mplan) != nil || !reflect.DeepEqual(&mplan, resolved) {
			return nil, nil, nil, errors.New("manifest resolved_plan contradicts resolved.json")
		}
	}
	loaded, err := protocol.LoadVerified(o.ProtocolPath, o.ProtocolRoot)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load verified protocol/profile: %w", err)
	}
	if err = protocol.CheckEligibility(loaded, *resolved.ProtocolBinding, resolved.ProtocolPilot, protocol.EvidenceReferences{}); err != nil {
		return nil, nil, nil, fmt.Errorf("protocol binding eligibility: %w", err)
	}
	definition, err := loaded.Definition()
	if err != nil {
		return nil, nil, nil, err
	}
	provenance := &ProtocolProvenance{Binding: *resolved.ProtocolBinding, ProtocolPath: o.ProtocolPath, ProtocolSHA256: loaded.SHA256(), ProfilePath: definition.Profiles.Path, ProfileSHA256: definition.Profiles.SHA256, MetricInterpretation: ProtocolMetricInterpretation, Eligibility: "engineering pilot metadata gate passed; not runtime instrumentation or capture attestation"}
	if resolved.ProtocolBinding.ExecutionClass == protocol.Campaign {
		return nil, nil, nil, errors.New("campaign execution is categorically ineligible under preliminary pilot protocol")
	}
	return provenance, manifestBytes, manifest, nil
}

func finiteTime(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
