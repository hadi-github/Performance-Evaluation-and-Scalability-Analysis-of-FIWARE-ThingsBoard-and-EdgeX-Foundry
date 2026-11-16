// Package protocol holds the preliminary, executable telemetry contract.
// Existing plans/analyzers are intentionally not consumers yet.
package protocol

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
	"strings"
)

type Definition struct {
	SchemaVersion  int              `json:"schema_version"`
	ProtocolID     string           `json:"protocol_id"`
	Lifecycle      string           `json:"lifecycle"`
	FreezeStatus   string           `json:"freeze_status"`
	Profiles       ProfileReference `json:"profiles"`
	Design         Design           `json:"design"`
	Delivery       DeliveryMetric   `json:"delivery_metric"`
	Outcomes       Outcomes         `json:"outcomes"`
	Payload        PayloadContract  `json:"payload"`
	ReceiptCapture ReceiptCapture   `json:"receipt_capture"`
	Policies       Policies         `json:"policies"`
}

type ReceiptCapture struct {
	ResponseCompleteBodyReadBeforeParse string `json:"response_complete_body_read_before_parse"`
	SinkChange                          string `json:"sink_change"`
}

type ProfileReference struct {
	PolicyID      string `json:"policy_id"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	Compatibility string `json:"compatibility"`
}
type Design struct {
	DevicePopulations           []int                     `json:"device_populations"`
	BaselineDevices             int                       `json:"baseline_devices"`
	ApplicationCPUBudgetK       []int                     `json:"application_cpu_budget_k"`
	ReplicaCountsN              []int                     `json:"replica_counts_n"`
	ReplicaMappingQualification string                    `json:"replica_mapping_qualification"`
	ReplicaMaps                 map[string]map[string]any `json:"replica_maps"`
}
type DeliveryMetric struct {
	PrimaryPopulation                            string   `json:"primary_population"`
	PercentileMethod                             string   `json:"percentile_method"`
	Percentiles                                  []int    `json:"percentiles"`
	IncludesOutcomes                             []string `json:"includes_outcomes"`
	IncludesDrainArrivalsUntil                   string   `json:"includes_drain_arrivals_until"`
	ConditionalOnObservedDelivery                bool     `json:"conditional_on_observed_delivery"`
	ScheduledAttainmentThresholdsMS              []int    `json:"scheduled_attainment_thresholds_ms"`
	PrimaryThresholdMS                           int      `json:"primary_threshold_ms"`
	MissingLatency                               string   `json:"missing_latency"`
	ZeroObservedPercentile                       any      `json:"zero_observed_percentile"`
	AckOnlySinkLatency                           string   `json:"ack_only_sink_latency"`
	ScheduledToResponseLatency                   string   `json:"scheduled_to_response_latency"`
	CaptureInvalidityIsNotPlatformMissing        bool     `json:"capture_invalidity_is_not_platform_missing"`
	IdentifiablePayloadCorruptionRemainsObserved bool     `json:"identifiable_payload_corruption_remains_observed"`
}
type Outcomes struct {
	NormalizedStates                          []string `json:"normalized_states"`
	CoverageIsSeparate                        bool     `json:"coverage_is_separate"`
	NotSentRule                               string   `json:"not_sent_rule"`
	DroppedIterationsAreDistinct              bool     `json:"dropped_iterations_are_distinct"`
	IterationIdentity                         string   `json:"iteration_identity"`
	ZeroDropRequiredForValidity               bool     `json:"zero_drop_required_for_validity"`
	ExactPerSlotLedgerUnderDroppedInvalidRuns string   `json:"exact_per_slot_ledger_under_dropped_invalid_runs"`
}
type PayloadContract struct {
	EncodingID               string   `json:"encoding_id"`
	ContentType              string   `json:"content_type"`
	FieldOrder               []string `json:"field_order"`
	Serialization            string   `json:"serialization"`
	NumericRules             string   `json:"numeric_rules"`
	Padding                  string   `json:"padding"`
	PaddingSalt              string   `json:"padding_salt"`
	MaximumBytes             int      `json:"maximum_bytes"`
	ProposedSizeOffsetsBytes []int    `json:"proposed_size_offsets_bytes"`
	SizePointsStatus         string   `json:"size_points_status"`
	StoredValidation         string   `json:"stored_validation"`
}
type PolicySetting struct {
	Status     string   `json:"status"`
	Source     string   `json:"source,omitempty"`
	Value      *int     `json:"value,omitempty"`
	Multiplier *float64 `json:"multiplier,omitempty"`
}
type SearchProposal struct {
	Status                    string  `json:"status"`
	CoarseMultiplier          float64 `json:"coarse_multiplier"`
	CoarseDurationSeconds     int     `json:"coarse_duration_seconds"`
	BisectionMaxFraction      float64 `json:"bisection_max_fraction"`
	EndpointRepetitions       int     `json:"endpoint_repetitions"`
	OptionalFavorableStopping bool    `json:"optional_favorable_stopping"`
	HybridSearch              string  `json:"hybrid_search"`
}
type Policies struct {
	StrictTwoXHeadroom               PolicySetting  `json:"strict_two_x_headroom"`
	WarmupSeconds                    PolicySetting  `json:"warmup_seconds"`
	ConfirmationSeconds              PolicySetting  `json:"confirmation_seconds"`
	MinimumIndependentRepetitions    PolicySetting  `json:"minimum_independent_repetitions"`
	PendingCalibration               []string       `json:"pending_calibration"`
	SeparatelyLabeledSearchProposals SearchProposal `json:"separately_labeled_search_proposals"`
}

// Loaded is source-bound and immutable outside this package. Accessors decode fresh copies.
type Loaded struct {
	source      []byte
	sha256      string
	sourcePath  string
	profileRoot string
}

// Load reads the definition exactly once and binds the run contract to the hash of those source bytes.
func Load(path string) (Loaded, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, err
	}
	_, err = Decode(b)
	if err != nil {
		return Loaded{}, err
	}
	return Loaded{source: append([]byte(nil), b...), sha256: HashBytes(b), sourcePath: path}, nil
}

// LoadPinned rejects validly decoded but byte-different definitions when a run was bound to a prior hash.
func LoadPinned(path, expectedSHA256 string) (Loaded, error) {
	p, err := Load(path)
	if err != nil {
		return Loaded{}, err
	}
	if p.sha256 != expectedSHA256 {
		return Loaded{}, fmt.Errorf("protocol source hash mismatch: expected %s got %s", expectedSHA256, p.sha256)
	}
	return p, nil
}

// LoadVerified also checks the dated profile bytes against the pinned profile hash.
func LoadVerified(path, repositoryRoot string) (Loaded, error) {
	p, err := Load(path)
	if err != nil {
		return Loaded{}, err
	}
	d, err := p.Definition()
	if err != nil {
		return Loaded{}, err
	}
	if err = VerifyProfile(repositoryRoot, d.Profiles); err != nil {
		return Loaded{}, err
	}
	p.profileRoot = repositoryRoot
	return p, nil
}

func (p Loaded) Definition() (Definition, error) {
	if len(p.source) == 0 || HashBytes(p.source) != p.sha256 {
		return Definition{}, errors.New("protocol loaded source integrity failure")
	}
	return Decode(p.source)
}
func (p Loaded) SHA256() string     { return p.sha256 }
func (p Loaded) SourcePath() string { return p.sourcePath }

func HashBytes(source []byte) string { sum := sha256.Sum256(source); return hex.EncodeToString(sum[:]) }

func Decode(source []byte) (Definition, error) {
	if err := rejectDuplicateJSONKeys(source); err != nil {
		return Definition{}, fmt.Errorf("protocol: %w", err)
	}
	if err := validateExactSchemaKeys(source); err != nil {
		return Definition{}, fmt.Errorf("protocol: %w", err)
	}
	zeroPercentile, err := rawJSONPath(source, "delivery_metric", "zero_observed_percentile")
	if err != nil || !bytes.Equal(bytes.TrimSpace(zeroPercentile), []byte("null")) {
		return Definition{}, errors.New("protocol: delivery_metric.zero_observed_percentile must be explicitly null")
	}
	optionalStop, err := rawJSONPath(source, "policies", "separately_labeled_search_proposals", "optional_favorable_stopping")
	if err != nil || !bytes.Equal(bytes.TrimSpace(optionalStop), []byte("false")) {
		return Definition{}, errors.New("protocol: optional_favorable_stopping must be explicitly false")
	}
	dec := json.NewDecoder(bytes.NewReader(source))
	dec.DisallowUnknownFields()
	var d Definition
	if err := dec.Decode(&d); err != nil {
		return Definition{}, fmt.Errorf("protocol: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Definition{}, errors.New("protocol: trailing JSON value")
		}
		return Definition{}, fmt.Errorf("protocol: trailing data: %w", err)
	}
	if err := d.Validate(); err != nil {
		return Definition{}, err
	}
	return d, nil
}

// rejectDuplicateJSONKeys checks every object before encoding/json can collapse duplicate members.
func rejectDuplicateJSONKeys(source []byte) error {
	dec := json.NewDecoder(bytes.NewReader(source))
	first, err := dec.Token()
	if err != nil {
		return err
	}
	if err = walkJSONToken(dec, first); err != nil {
		return err
	}
	if _, err = dec.Token(); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}
func walkJSONToken(dec *json.Decoder, token json.Token) error {
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = true
			value, err := dec.Token()
			if err != nil {
				return err
			}
			if err = walkJSONToken(dec, value); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("malformed JSON object")
		}
	case '[':
		for dec.More() {
			value, err := dec.Token()
			if err != nil {
				return err
			}
			if err = walkJSONToken(dec, value); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("malformed JSON array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

// validateExactSchemaKeys is deliberately separate from encoding/json's struct decoder,
// which accepts case-insensitive aliases for tagged fields. It validates every protocol object.
func validateExactSchemaKeys(source []byte) error {
	var root any
	if err := json.Unmarshal(source, &root); err != nil {
		return err
	}
	objects := map[string][]string{
		"$":                              {"schema_version", "protocol_id", "lifecycle", "freeze_status", "profiles", "design", "delivery_metric", "outcomes", "payload", "receipt_capture", "policies"},
		"profiles":                       {"policy_id", "path", "sha256", "compatibility"},
		"design":                         {"device_populations", "baseline_devices", "application_cpu_budget_k", "replica_counts_n", "replica_mapping_qualification", "replica_maps"},
		"delivery_metric":                {"primary_population", "percentile_method", "percentiles", "includes_outcomes", "includes_drain_arrivals_until", "conditional_on_observed_delivery", "scheduled_attainment_thresholds_ms", "primary_threshold_ms", "missing_latency", "zero_observed_percentile", "ack_only_sink_latency", "scheduled_to_response_latency", "capture_invalidity_is_not_platform_missing", "identifiable_payload_corruption_remains_observed"},
		"outcomes":                       {"normalized_states", "coverage_is_separate", "not_sent_rule", "dropped_iterations_are_distinct", "iteration_identity", "zero_drop_required_for_validity", "exact_per_slot_ledger_under_dropped_invalid_runs"},
		"payload":                        {"encoding_id", "content_type", "field_order", "serialization", "numeric_rules", "padding", "padding_salt", "maximum_bytes", "proposed_size_offsets_bytes", "size_points_status", "stored_validation"},
		"receipt_capture":                {"response_complete_body_read_before_parse", "sink_change"},
		"policies":                       {"strict_two_x_headroom", "warmup_seconds", "confirmation_seconds", "minimum_independent_repetitions", "pending_calibration", "separately_labeled_search_proposals"},
		"policies.strict_two_x_headroom": {"status", "source", "multiplier"},
		"policies.warmup_seconds":        {"status", "source", "value"},
		"policies.confirmation_seconds":  {"status", "source", "value"},
		"policies.minimum_independent_repetitions":     {"status", "source", "value"},
		"policies.separately_labeled_search_proposals": {"status", "coarse_multiplier", "coarse_duration_seconds", "bisection_max_fraction", "endpoint_repetitions", "optional_favorable_stopping", "hybrid_search"},
	}
	return validateSchemaNode(root, "$", objects)
}

func validateSchemaNode(node any, path string, objects map[string][]string) error {
	switch v := node.(type) {
	case map[string]any:
		if allowed, ok := objects[path]; ok {
			set := make(map[string]bool, len(allowed))
			for _, k := range allowed {
				set[k] = true
			}
			for k := range v {
				if !set[k] {
					return fmt.Errorf("unknown or non-case-exact field %s.%s", path, k)
				}
			}
			for _, k := range allowed {
				if _, ok := v[k]; !ok {
					return fmt.Errorf("missing required field %s.%s", path, k)
				}
			}
		} else if path == "design.replica_maps" {
			allowed := []string{"thingsboard", "fiware", "edgex"}
			set := map[string]bool{}
			for _, k := range allowed {
				set[k] = true
			}
			for k := range v {
				if !set[k] {
					return fmt.Errorf("unknown replica system %s", k)
				}
			}
			for _, k := range allowed {
				if _, ok := v[k]; !ok {
					return fmt.Errorf("missing replica system %s", k)
				}
			}
		} else if strings.HasPrefix(path, "design.replica_maps.") {
			var allowed []string
			switch path {
			case "design.replica_maps.thingsboard":
				allowed = []string{"tb-core", "tb-rule-engine", "tb-http-transport"}
			case "design.replica_maps.fiware":
				allowed = []string{"orion", "iot-agent"}
			case "design.replica_maps.edgex":
				allowed = []string{"core-data", "device-rest", "app-http-export"}
			default:
				return fmt.Errorf("unknown replica map %s", path)
			}
			set := map[string]bool{}
			for _, k := range allowed {
				set[k] = true
			}
			for k := range v {
				if !set[k] {
					return fmt.Errorf("unknown replica mapping %s.%s", path, k)
				}
			}
			for _, k := range allowed {
				if _, ok := v[k]; !ok {
					return fmt.Errorf("missing replica mapping %s.%s", path, k)
				}
			}
		} else {
			return fmt.Errorf("unexpected JSON object at %s", path)
		}
		for k, value := range v {
			child := k
			if path != "$" {
				child = path + "." + k
			}
			if value == nil && child != "delivery_metric.zero_observed_percentile" {
				return fmt.Errorf("null is not allowed at %s", child)
			}
			if err := validateSchemaNode(value, child, objects); err != nil {
				return err
			}
		}
	case []any:
		for i, value := range v {
			if value == nil {
				return fmt.Errorf("null array item at %s[%d]", path, i)
			}
			if err := validateSchemaNode(value, fmt.Sprintf("%s[%d]", path, i), objects); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d Definition) Validate() error {
	if d.SchemaVersion != 1 || d.ProtocolID != "iotbench-telemetry-v0.1.0-pilot.1" {
		return errors.New("protocol: unsupported schema_version or protocol_id")
	}
	if d.Lifecycle != "preliminary_pilot" || d.FreezeStatus != "pending_calibration" {
		return errors.New("protocol: this definition must remain preliminary and pending_calibration")
	}
	if d.Profiles.PolicyID != "offline-profiles-20261101" || d.Profiles.Path != "research/protocols/offline-profiles-20261101.json" || d.Profiles.SHA256 != "1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8" || d.Profiles.Compatibility != "D44 meanings and thresholds are unchanged; reference is explicit and immutable" {
		return errors.New("protocol: explicit profile path, policy_id, and SHA-256 are required")
	}
	if _, err := hex.DecodeString(d.Profiles.SHA256); err != nil {
		return errors.New("protocol: profile SHA-256 must be lowercase hex")
	}
	if d.Design.BaselineDevices != 1000 || !intsEqual(d.Design.DevicePopulations, []int{100, 1000, 10000}) || !intsEqual(d.Design.ApplicationCPUBudgetK, []int{1, 2, 3}) || !intsEqual(d.Design.ReplicaCountsN, []int{1, 2, 3}) || d.Design.ReplicaMappingQualification != "proposal_not_qualified" {
		return errors.New("protocol: unsupported or qualified design mapping")
	}
	if len(d.Design.ReplicaMaps) != 3 || !serviceMap(d.Design.ReplicaMaps["thingsboard"], map[string]any{"tb-core": "N", "tb-rule-engine": "N", "tb-http-transport": "N"}) || !serviceMap(d.Design.ReplicaMaps["fiware"], map[string]any{"orion": "N", "iot-agent": "N"}) || !serviceMap(d.Design.ReplicaMaps["edgex"], map[string]any{"core-data": "N", "device-rest": "N", "app-http-export": float64(1)}) {
		return errors.New("protocol: replica maps must be explicit and match N mapping proposal")
	}
	if d.Delivery.PercentileMethod != "nearest_rank" || !intsEqual(d.Delivery.Percentiles, []int{50, 95, 99}) || d.Delivery.PrimaryThresholdMS != 500 || !intsEqual(d.Delivery.ScheduledAttainmentThresholdsMS, []int{250, 500, 1000, 5000}) || d.Delivery.ZeroObservedPercentile != nil || !d.Delivery.ConditionalOnObservedDelivery || !d.Delivery.CaptureInvalidityIsNotPlatformMissing || !d.Delivery.IdentifiablePayloadCorruptionRemainsObserved {
		return errors.New("protocol: delivery metric semantics incomplete or changed")
	}
	if d.Delivery.PrimaryPopulation != "unique_first_observed_receipts_for_scheduled_half_open_measurement_cohort" || d.Delivery.IncludesDrainArrivalsUntil != "explicit_observation_cutoff" || !stringsEqual(d.Delivery.IncludesOutcomes, []string{"OK", "ERROR", "UNKNOWN"}) || d.Delivery.MissingLatency != "not_finite_and_excluded_from_delivery_percentile; report scheduled_observed_missing_separately" || d.Delivery.AckOnlySinkLatency != "separate_metric" || d.Delivery.ScheduledToResponseLatency != "separate_metric" {
		return errors.New("protocol: receipt cohort and companion metric semantics incomplete")
	}
	if !stringsEqual(d.Outcomes.NormalizedStates, []string{"OK", "ERROR", "UNKNOWN", "NOT_SENT"}) || !d.Outcomes.CoverageIsSeparate || d.Outcomes.NotSentRule != "positive_identified_non_submission_evidence_only" || !d.Outcomes.DroppedIterationsAreDistinct || !d.Outcomes.ZeroDropRequiredForValidity || d.Outcomes.IterationIdentity != "D15 CAR iterationInTest" || d.Outcomes.ExactPerSlotLedgerUnderDroppedInvalidRuns != "unsatisfied_capability" {
		return errors.New("protocol: outcome contract incomplete")
	}
	if d.Payload.EncodingID != "logical-json-ascii-v1" || !stringsEqual(d.Payload.FieldOrder, []string{"run_id", "seq", "device_id", "scheduled_send_ms", "temperature", "humidity", "pressure", "padding"}) || d.Payload.MaximumBytes != PayloadMaximumBytes || !intsEqual(d.Payload.ProposedSizeOffsetsBytes, []int{0, 1024, 4096}) || d.Payload.SizePointsStatus != "proposed_not_qualified" {
		return errors.New("protocol: unsupported payload contract")
	}
	if d.Payload.ContentType != "application/json; charset=utf-8" || d.Payload.Serialization != "compact UTF-8 JSON; ASCII keys/values; no insignificant whitespace; field order normative" || d.Payload.NumericRules != "seq is uint32; scheduled_send_ms is finite nonnegative decimal <= 9007199254740991 with at most 3 effective fractional decimal digits and is never rounded by the encoder; stored timestamp token must equal shortest canonical numeric serialization except exact numeric-zero spellings; decimal exponent magnitude above 10000 is rejected; stored seq, temperature, humidity, pressure use canonical integer JSON tokens only; negative zero normalizes to zero; temperature 15..45, humidity 30..80, pressure 950..1150; preserve supplied values" || d.Payload.Padding != "lowercase ASCII hex from versioned deterministic Mix(seed,seq,salt,block); one ASCII byte per hex character; body size grows in even-byte increments from its unpadded baseline and must match baseline parity" || d.Payload.PaddingSalt != "0x50414431" || d.Payload.StoredValidation != "compare supplied stored bytes and decoded stored fields to expected values; a locally reconstructed payload alone is not storage evidence" {
		return errors.New("protocol: payload serialization, numeric, padding, salt, or stored-validation semantics incomplete or changed")
	}
	if d.ReceiptCapture.ResponseCompleteBodyReadBeforeParse != "proposed_amendment_not_implemented" || d.ReceiptCapture.SinkChange != "out_of_scope" {
		return errors.New("protocol: receipt capture proposal/sink boundary missing or changed")
	}
	if !stringsEqual(d.Policies.PendingCalibration, []string{"clock_error_budget", "drain_deadline", "recovery_window", "sample_counts", "backlog_epsilon", "capacity_full_predicate", "capacity_search_rule", "scientific_device_rate_grid"}) {
		return errors.New("protocol: unresolved campaign policies must remain explicit")
	}
	if d.Policies.StrictTwoXHeadroom.Status != "source_requirement_retained" || d.Policies.StrictTwoXHeadroom.Multiplier == nil || *d.Policies.StrictTwoXHeadroom.Multiplier != 2 || d.Policies.StrictTwoXHeadroom.Source != "docs/architecture/design-decisions.md#d34--generator-qualification" ||
		d.Policies.WarmupSeconds.Status != "source_requirement_retained" || d.Policies.WarmupSeconds.Value == nil || *d.Policies.WarmupSeconds.Value != 300 || d.Policies.WarmupSeconds.Source != "docs/architecture/design-decisions.md#d39--s5-experimental-protocol" ||
		d.Policies.ConfirmationSeconds.Status != "source_requirement_retained" || d.Policies.ConfirmationSeconds.Value == nil || *d.Policies.ConfirmationSeconds.Value != 600 || d.Policies.ConfirmationSeconds.Source != "docs/architecture/design-decisions.md#d39--s5-experimental-protocol" ||
		d.Policies.MinimumIndependentRepetitions.Status != "source_requirement_retained" || d.Policies.MinimumIndependentRepetitions.Value == nil || *d.Policies.MinimumIndependentRepetitions.Value != 5 || d.Policies.MinimumIndependentRepetitions.Source != "docs/architecture/design-decisions.md#d39--s5-experimental-protocol" {
		return errors.New("protocol: sourced headroom/warmup/confirmation/repetition requirements changed or incomplete")
	}
	proposal := d.Policies.SeparatelyLabeledSearchProposals
	if proposal.Status != "proposal_only_not_frozen" || proposal.CoarseMultiplier != 1.5 || proposal.CoarseDurationSeconds != 180 || proposal.BisectionMaxFraction != .05 || proposal.EndpointRepetitions != 5 || proposal.OptionalFavorableStopping || proposal.HybridSearch != "not_adopted" {
		return errors.New("protocol: search proposals must remain separately labeled and unfrozen")
	}
	return nil
}

func rawJSONPath(source []byte, path ...string) (json.RawMessage, error) {
	var current map[string]json.RawMessage
	if err := json.Unmarshal(source, &current); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	for i, key := range path {
		var ok bool
		raw, ok = current[key]
		if !ok {
			return nil, fmt.Errorf("missing required field %s", strings.Join(path[:i+1], "."))
		}
		if i < len(path)-1 {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, err
			}
		}
	}
	return raw, nil
}

func VerifyProfile(root string, ref ProfileReference) error {
	clean := filepath.Clean(ref.Path)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("protocol: profile reference must remain inside repository root")
	}
	b, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		return err
	}
	if got := HashBytes(b); got != ref.SHA256 {
		return fmt.Errorf("protocol: profile hash mismatch: expected %s got %s", ref.SHA256, got)
	}
	return nil
}
func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func serviceMap(got, want map[string]any) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

type ExecutionClass string

const (
	EngineeringPilot ExecutionClass = "engineering_pilot"
	Campaign         ExecutionClass = "campaign"
)

type Binding struct {
	ProtocolID     string         `json:"protocol_id"`
	ProtocolSHA256 string         `json:"protocol_sha256"`
	ExecutionClass ExecutionClass `json:"execution_class"`
}
type PilotParameters struct {
	RatePerSecond                *float64 `json:"rate_per_second"`
	MeasurementSeconds           *int     `json:"measurement_seconds"`
	WarmupSeconds                *int     `json:"warmup_seconds"`
	DrainCutoffUnixMS            *int64   `json:"drain_cutoff_unix_ms"`
	DrainSecondsAfterMeasurement *int     `json:"drain_seconds_after_measurement"`
	QuietIntervalSeconds         *int     `json:"quiet_interval_seconds,omitempty"`
	LateObservationSeconds       *int     `json:"late_observation_seconds,omitempty"`
}
type EvidenceReferences struct {
	Qualification []string `json:"qualification"`
	Calibration   []string `json:"calibration"`
	Profile       []string `json:"profile"`
}

// CheckEligibility is metadata/evidence-reference gating only; it does not attest that referenced evidence is true.
func CheckEligibility(p Loaded, b Binding, pilot *PilotParameters, refs EvidenceReferences) error {
	if len(p.source) == 0 || HashBytes(p.source) != p.sha256 {
		return errors.New("protocol loaded source integrity failure")
	}
	d, err := Decode(p.source)
	if err != nil {
		return err
	}
	if p.profileRoot == "" {
		return errors.New("protocol profile binding was not verified")
	}
	if err = VerifyProfile(p.profileRoot, d.Profiles); err != nil {
		return err
	}
	if b.ProtocolID != d.ProtocolID || b.ProtocolSHA256 != p.sha256 {
		return errors.New("protocol binding ID/hash mismatch")
	}
	switch b.ExecutionClass {
	case EngineeringPilot:
		if pilot == nil || pilot.RatePerSecond == nil || math.IsNaN(*pilot.RatePerSecond) || math.IsInf(*pilot.RatePerSecond, 0) || *pilot.RatePerSecond <= 0 || pilot.MeasurementSeconds == nil || *pilot.MeasurementSeconds <= 0 || pilot.WarmupSeconds == nil || *pilot.WarmupSeconds < 0 {
			return errors.New("engineering pilot requires explicit numerical settings")
		}
		if (pilot.DrainCutoffUnixMS == nil) == (pilot.DrainSecondsAfterMeasurement == nil) {
			return errors.New("engineering pilot requires exactly one positive absolute or measurement-relative drain cutoff")
		}
		if pilot.DrainCutoffUnixMS != nil && *pilot.DrainCutoffUnixMS <= 0 || pilot.DrainSecondsAfterMeasurement != nil && *pilot.DrainSecondsAfterMeasurement <= 0 {
			return errors.New("engineering pilot drain cutoff must be positive")
		}
		if pilot.DrainSecondsAfterMeasurement != nil && (pilot.QuietIntervalSeconds == nil || *pilot.QuietIntervalSeconds <= 0 || pilot.LateObservationSeconds == nil || *pilot.LateObservationSeconds <= 0 || *pilot.QuietIntervalSeconds > *pilot.DrainSecondsAfterMeasurement) {
			return errors.New("measurement-relative drain requires explicit positive quiet-interval and late-observation durations")
		}
		return nil
	case Campaign:
		_ = refs // References are labels only and never attest to evidence validity.
		return errors.New("campaign ineligible: this preliminary pilot protocol is never campaign eligible")
	default:
		return errors.New("execution_class must be engineering_pilot or campaign")
	}
}
