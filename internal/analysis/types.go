// Package analysis orchestrates existing offline evidence calculators with explicit
// claim prerequisites. It never writes into a run or contacts a service.
package analysis

import (
	"encoding/json"

	"iotbench/internal/cpubound"
	"iotbench/internal/protocol"
	"iotbench/internal/report"
)

type Status string

const (
	LegacyMetricInterpretation   = "legacy-acknowledgement-only-v1"
	ProtocolMetricInterpretation = "protocol-scheduled-cohort-v1"
)

const (
	Pass        Status = "PASS"
	Fail        Status = "FAIL"
	Invalid     Status = "INVALID"
	NotAssessed Status = "NOT_ASSESSED"
)

type Claim struct {
	Status  Status   `json:"status"`
	Reasons []string `json:"reasons"`
}

func claim(s Status, reason string) Claim { return Claim{s, []string{reason}} }

// Source describes an independently supplied observation, not an inferred empty file.
// Complete and Source are explicit capture attestations, recorded rather than assumed.
type Source struct {
	Path                string  `json:"path"`
	Complete            bool    `json:"complete"`
	Source              string  `json:"source"`
	StartMS             float64 `json:"start_ms"`
	EndMS               float64 `json:"end_ms"`
	CollectedAtMS       float64 `json:"collected_at_ms,omitempty"`
	Diagnostics         string  `json:"diagnostics_path,omitempty"`
	DiagnosticsRunLocal bool    `json:"diagnostics_run_local,omitempty"`
}

type Configuration struct {
	ID              string            `json:"id"`
	CPUAllocationK  int               `json:"application_cpu_budget_k"`
	Replicas        map[string]int    `json:"replicas"`
	Images          map[string]string `json:"platform_image_digests"`
	EffectiveSHA256 string            `json:"effective_configuration_sha256"`
	EffectivePath   string            `json:"effective_configuration_path"`
}

type Evidence struct {
	SchemaVersion         int           `json:"schema_version"`
	Synthetic             bool          `json:"synthetic"`
	Interpretation        string        `json:"interpretation"`
	Configuration         Configuration `json:"configuration"`
	ObservationDeadlineMS float64       `json:"observation_deadline_ms"`
	Sink                  Source        `json:"sink"`
	History               Source        `json:"history"`
	SinkClock             string        `json:"sink_clock"` // "shared_load_realtime", with a source, or unavailable
	ClockSource           string        `json:"clock_source"`
	CPU                   []CPUInput    `json:"cpu"`
	Backlog               Source        `json:"backlog"`
	W2Reference           *W2Reference  `json:"w2_reference,omitempty"`
}

// A reference is supplied evidence, never estimated from the input reports.
type W2Reference struct {
	Capacity   *float64      `json:"capacity_messages_per_second"`
	Definition string        `json:"definition"`
	Source     string        `json:"source"`
	Path       string        `json:"path"`
	SHA256     string        `json:"sha256"`
	RatePoints []W2RatePoint `json:"rate_points,omitempty"`
}

type W2RatePoint struct {
	Fraction float64 `json:"fraction"`
	Rate     float64 `json:"offered_rate_messages_per_second"`
	Source   string  `json:"source"`
}

type DeviceComparisonSpec struct {
	Populations        []int     `json:"populations"`
	ReferenceFractions []float64 `json:"reference_fractions"`
	ApplicationCPU     int       `json:"application_cpu_budget_k"`
	ReplicasPerService int       `json:"replicas_per_service"`
}

type CPUInput struct {
	Step               int      `json:"step"`
	Before             string   `json:"before"`
	After              string   `json:"after"`
	ExpectedContainers []string `json:"expected_containers"`
	Denominator        string   `json:"denominator"` // offered | acknowledged | persisted_acknowledged
	Clock              string   `json:"clock"`       // shared_load_realtime or unavailable
	ClockSource        string   `json:"clock_source"`
}

type Requirement struct {
	Metric    string   `json:"metric"`
	Operator  string   `json:"operator"` // <= | >= | ==
	Threshold *float64 `json:"threshold"`
	Unit      string   `json:"unit"`
	Source    string   `json:"source"`
}

type Profile struct {
	ID               string                `json:"id"`
	Name             string                `json:"name"`
	Source           string                `json:"source"`
	Requirements     []Requirement         `json:"requirements"`
	Sensitivities    []Requirement         `json:"sensitivities,omitempty"`
	DeviceComparison *DeviceComparisonSpec `json:"device_comparison,omitempty"`
}

type Policy struct {
	SchemaVersion  int          `json:"schema_version"`
	Synthetic      bool         `json:"synthetic"`
	Interpretation string       `json:"interpretation"`
	Profiles       []Profile    `json:"profiles"`
	BacklogRule    *BacklogRule `json:"backlog_rule,omitempty"`
}

type BacklogRule struct {
	Method          string `json:"method"`
	IndependentBins bool   `json:"independent_bins_assumption"`
	Calibration     string `json:"calibration"`
	Source          string `json:"source"`
}

func DefaultPolicy() Policy {
	source := "docs/architecture/design-decisions.md#d44--owner-profile-mappings-2026-11-01"
	q := func(metric, op, unit string, value float64) Requirement {
		return Requirement{Metric: metric, Operator: op, Threshold: &value, Unit: unit, Source: source}
	}
	return Policy{SchemaVersion: 1, Interpretation: "claim-gates-v1", Profiles: []Profile{
		{ID: "quick_delivery", Name: "Quick delivery", Source: source, Requirements: []Requirement{q("latency.delivery.p99_ms", "<=", "ms", 500)}, Sensitivities: []Requirement{q("latency.delivery.p99_ms", "<=", "ms", 250), q("latency.delivery.p99_ms", "<=", "ms", 1000), q("latency.delivery.p99_ms", "<=", "ms", 5000)}},
		{ID: "saved_data_reliability", Name: "Reliability of saved data", Source: source, Requirements: []Requirement{q("history.missing_acknowledged", "==", "messages", 0), q("history.duplicate_extra_copies", "==", "messages", 0)}},
		{ID: "high_device_count", Name: "High number of devices", Source: source, Requirements: []Requirement{}, DeviceComparison: &DeviceComparisonSpec{Populations: []int{100, 10000}, ReferenceFractions: []float64{.5, .75}, ApplicationCPU: 1, ReplicasPerService: 1}},
	}}
}

type Input struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	Status string `json:"status"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Metric struct {
	Value       *float64 `json:"value"`
	Unit        string   `json:"unit"`
	Claim       string   `json:"claim"`
	Endpoint    string   `json:"endpoint,omitempty"`
	ClockDomain string   `json:"clock_domain,omitempty"`
	Basis       string   `json:"basis"`
}

type Assessment struct {
	Profile Profile `json:"profile"`
	Claim
	Checks            []Check `json:"checks"`
	SensitivityChecks []Check `json:"sensitivity_checks,omitempty"`
}

type Check struct {
	Requirement Requirement `json:"requirement"`
	Claim
}

type Step struct {
	Index       int               `json:"index"`
	Name        string            `json:"name"`
	Phase       string            `json:"phase"`
	Devices     int               `json:"devices"`
	OfferedRate float64           `json:"offered_rate_messages_per_second"`
	StartMS     float64           `json:"start_ms"`
	EndMS       float64           `json:"end_ms"`
	Claims      map[string]Claim  `json:"claims"`
	Metrics     map[string]Metric `json:"metrics"`
	Profiles    []Assessment      `json:"profiles"`
	CPU         *cpubound.Report  `json:"cpu,omitempty"`
}

type Result struct {
	SchemaVersion        int                        `json:"schema_version"`
	Synthetic            bool                       `json:"synthetic"`
	RunID                string                     `json:"run_id"`
	Platform             string                     `json:"platform"`
	Seed                 uint32                     `json:"seed"`
	Configuration        Configuration              `json:"configuration"`
	Provenance           map[string]json.RawMessage `json:"provenance"`
	Interpretation       string                     `json:"interpretation"`
	Inputs               []Input                    `json:"inputs"`
	Claims               map[string]Claim           `json:"claims"`
	Steps                []Step                     `json:"steps"`
	Health               *report.Health             `json:"recomputed_health,omitempty"`
	Limitations          []string                   `json:"limitations"`
	CampaignVerdict      string                     `json:"campaign_verdict"`
	CaptureSpecification *Evidence                  `json:"capture_specification"`
	Policy               *Policy                    `json:"policy"`
	Protocol             *ProtocolProvenance        `json:"protocol,omitempty"`
	MetricInterpretation string                     `json:"metric_interpretation"`
	// M1f fault/capacity foundation. These remain nil unless a caller explicitly
	// attaches an offline reduction; Analyze never manufactures a claim from them.
	FaultTarget   *FaultTargetResolution `json:"fault_target,omitempty"`
	FaultEvent    *FaultReduction        `json:"fault_event,omitempty"`
	FaultCapacity *CapacityResult        `json:"fault_capacity,omitempty"`
}

type ProtocolProvenance struct {
	Binding              protocol.Binding `json:"binding"`
	ProtocolPath         string           `json:"protocol_path"`
	ProtocolSHA256       string           `json:"protocol_sha256"`
	ProfilePath          string           `json:"profile_path"`
	ProfileSHA256        string           `json:"profile_sha256"`
	MetricInterpretation string           `json:"metric_interpretation"`
	Eligibility          string           `json:"eligibility"`
	// Receipt capture version metadata is recorded when a capture document was present and
	// decoded. A legacy schema-1 capture is reported as legacy; it is never relabeled as schema 2.
	ReceiptCaptureSchemaVersion int    `json:"receipt_capture_schema_version,omitempty"`
	ReceiptCaptureSemantics     string `json:"receipt_capture_semantics,omitempty"`
}

type ProtocolReceiptCapture struct {
	SchemaVersion        int              `json:"schema_version"`
	MetricInterpretation string           `json:"metric_interpretation"`
	Binding              protocol.Binding `json:"protocol_binding"`
	Synthetic            bool             `json:"synthetic"`
	Complete             bool             `json:"complete"`
	Endpoint             string           `json:"endpoint"`
	Source               string           `json:"source"`
	BodyReadComplete     bool             `json:"body_read_complete"`
	// ReceiptCaptureSemantics is required by schema 2. It names the versioned complete-body-read
	// receiver contract; schema-1 captures omit it and remain legacy-only.
	ReceiptCaptureSemantics string            `json:"receipt_capture_semantics"`
	ClockDomain             string            `json:"clock_domain"`
	ClockSource             string            `json:"clock_source"`
	CoverageStartMS         float64           `json:"coverage_start_ms"`
	CoverageEndMS           float64           `json:"coverage_end_ms"`
	ObservationCutoffMS     float64           `json:"observation_cutoff_ms"`
	Receipts                []ProtocolReceipt `json:"receipts"`
}

type ProtocolReceipt struct {
	RunID           string  `json:"run_id"`
	Seq             int64   `json:"seq"`
	DeviceID        string  `json:"device_id"`
	Phase           string  `json:"phase"`
	ScheduledSendMS float64 `json:"scheduled_send_ms"`
	// ObservedMS is the complete-body receipt time in the capture's declared clock domain. For
	// schema-2 captures it must come from the versioned complete-body-before-parse semantics; for
	// schema-1 captures it remains legacy arrival data and cannot produce the new primary metrics.
	ObservedMS float64          `json:"observed_ms"`
	Outcome    protocol.Outcome `json:"outcome"`
}

type Options struct {
	Run, Evidence, Policy string
	ProtocolPath          string
	ProtocolRoot          string
}
