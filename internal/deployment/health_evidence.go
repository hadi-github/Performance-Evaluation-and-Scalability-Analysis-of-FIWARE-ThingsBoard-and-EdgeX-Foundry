package deployment

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// The M2e offline synthetic health-evidence dry run is a pure
// document evaluator and a new-directory evidence writer: it compares
// caller-supplied synthetic service observations with the approved M2d execution
// plan. It never contacts Docker, a SUT, a platform, a lifecycle endpoint, a
// monitoring service or a load generator, and it exposes no executor, observer
// implementation or lifecycle method.

const (
	// SyntheticObservationsSchemaVersion versions the synthetic observation
	// document.
	SyntheticObservationsSchemaVersion = 1
	// SyntheticObservationsInterpretation names the exact synthetic observation
	// semantics.
	SyntheticObservationsInterpretation = "synthetic-service-observations-v1"
	// HealthEvidenceSchemaVersion versions the health-evidence dry-run report.
	HealthEvidenceSchemaVersion = 1
	// HealthEvidenceInterpretation names the exact health-evidence dry-run
	// semantics.
	HealthEvidenceInterpretation = "deployment-health-evidence-dry-run-v1"
	// HealthEvidenceMode is the only admitted mode.
	HealthEvidenceMode = "offline-synthetic-dry-run"
	// HealthEvidenceSyntheticLabel prominently labels every emitted report. A
	// synthetic comparison is never a live qualification.
	HealthEvidenceSyntheticLabel = "SYNTHETIC M2e — OFFLINE DRY RUN"
	// SyntheticSourcePrefix is the required leading label of the observation
	// document source.
	SyntheticSourcePrefix = "SYNTHETIC M2e"
	// SyntheticRunTagPrefix is the required leading label of the observation run
	// tag and therefore of the packaged run identifier.
	SyntheticRunTagPrefix = "m2e-synthetic"
	// HealthEvidenceExecutionPlanInputName is the copied execution-plan artifact.
	HealthEvidenceExecutionPlanInputName = "execution-plan.input.json"
	// HealthEvidenceObservationsInputName is the copied observation artifact.
	HealthEvidenceObservationsInputName = "synthetic-service-observations.json"
	// HealthEvidenceReportName is the pretty JSON report.
	HealthEvidenceReportName = "health-evidence-dry-run.json"
	// HealthEvidenceCanonicalName is the compact canonical JSON report.
	HealthEvidenceCanonicalName = "health-evidence-dry-run.canonical.json"
	// HealthEvidenceMarkdownName is the Markdown report.
	HealthEvidenceMarkdownName = "health-evidence-dry-run.md"
)

var validObservedHealth = map[string]bool{
	"healthy":   true,
	"unhealthy": true,
	"none":      true,
	"unknown":   true,
}

// SyntheticServiceObservations is the strict caller-supplied synthetic
// observation document. It reuses ServiceObservation unchanged. The document is
// never collected by this package; every value is supplied by the caller and is
// labeled synthetic.
type SyntheticServiceObservations struct {
	SchemaVersion  int                  `json:"schema_version"`
	Interpretation string               `json:"interpretation"`
	Source         string               `json:"source"`
	RunID          string               `json:"run_id"`
	RunDate        string               `json:"run_date"`
	RunTag         string               `json:"run_tag"`
	Observations   []ServiceObservation `json:"observations"`
}

// serviceObservationShape is the shared exact-key shape of one observation.
var serviceObservationShape = jsonShape{
	object: map[string]jsonShape{
		"service":             scalarValueShape,
		"present":             scalarValueShape,
		"replicas":            scalarValueShape,
		"running":             scalarValueShape,
		"health":              scalarValueShape,
		"exit_code":           scalarValueShape,
		"functional_evidence": scalarValueShape,
	},
	optional: map[string]bool{"exit_code": true},
}

var syntheticObservationsShape = jsonShape{
	object: map[string]jsonShape{
		"schema_version": scalarValueShape,
		"interpretation": scalarValueShape,
		"source":         scalarValueShape,
		"run_id":         scalarValueShape,
		"run_date":       scalarValueShape,
		"run_tag":        scalarValueShape,
		"observations":   {element: &serviceObservationShape},
	},
}

// DecodeSyntheticServiceObservations strictly decodes a synthetic observation
// document. Duplicate keys, case aliases, unknown fields, nulls, trailing JSON,
// missing fields and malformed semantics are rejected.
func DecodeSyntheticServiceObservations(b []byte) (*SyntheticServiceObservations, error) {
	if err := validateRawDocument(b, syntheticObservationsShape, "synthetic observations"); err != nil {
		return nil, err
	}
	var o SyntheticServiceObservations
	if err := decodeStrict(b, &o); err != nil {
		return nil, fmt.Errorf("synthetic observations: %w", err)
	}
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return &o, nil
}

// Validate checks the synthetic observation document after decoding or direct
// construction. Semantically contradictory but decodable observations are
// deliberately admitted: the evaluator reports FAIL or UNAVAILABLE instead of
// rejecting the input. An explicit empty observation array is admitted.
func (o SyntheticServiceObservations) Validate() error {
	if o.SchemaVersion != SyntheticObservationsSchemaVersion {
		return fmt.Errorf("synthetic observations: unsupported schema_version %d", o.SchemaVersion)
	}
	if o.Interpretation != SyntheticObservationsInterpretation {
		return fmt.Errorf("synthetic observations: interpretation must be %q", SyntheticObservationsInterpretation)
	}
	if !strings.HasPrefix(o.Source, SyntheticSourcePrefix) {
		return fmt.Errorf("synthetic observations: source must begin with %q", SyntheticSourcePrefix)
	}
	if !strings.HasPrefix(o.RunTag, SyntheticRunTagPrefix) {
		return fmt.Errorf("synthetic observations: run_tag must begin with %q", SyntheticRunTagPrefix)
	}
	if err := validateSyntheticRunIdentity(o); err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, obs := range o.Observations {
		context := fmt.Sprintf("synthetic observations service %d", i)
		if !serviceNamePattern.MatchString(obs.Service) {
			return fmt.Errorf("%s: service %q is not a valid compose service name", context, obs.Service)
		}
		if seen[obs.Service] {
			return fmt.Errorf("%s: duplicate service %q", context, obs.Service)
		}
		seen[obs.Service] = true
		if obs.Replicas < 0 {
			return fmt.Errorf("%s: replicas must be non-negative", context)
		}
		if !validObservedHealth[string(obs.Health)] {
			return fmt.Errorf("%s: health %q is not healthy, unhealthy, none or unknown", context, obs.Health)
		}
	}
	return nil
}

func validateSyntheticRunIdentity(o SyntheticServiceObservations) error {
	// Reuse the packaging validator rather than duplicating the run-id grammar.
	// The artifact declaration is a shape-only carrier for the run identity; its
	// hashes are never used as evidence here.
	planPath, err := ArtifactPath(o.RunID, HealthEvidenceExecutionPlanInputName)
	if err != nil {
		return fmt.Errorf("synthetic observations: run_id is invalid: %w", err)
	}
	obsPath, err := ArtifactPath(o.RunID, HealthEvidenceObservationsInputName)
	if err != nil {
		return fmt.Errorf("synthetic observations: run_id is invalid: %w", err)
	}
	declaration := ArtifactManifest{
		SchemaVersion:  ArtifactManifestSchemaVersion,
		Interpretation: ArtifactManifestInterpretation,
		RunID:          o.RunID,
		RunDate:        o.RunDate,
		RunTag:         o.RunTag,
		Platform:       "all",
		Artifacts: []ArtifactRecord{
			{
				Role: ArtifactRoleOrchestrationPlan, Path: planPath,
				SHA256: strings.Repeat("0", 64), Bytes: 1, Origin: ArtifactThisRun, Required: true,
			},
			{
				Role: ArtifactRoleOther, Path: obsPath,
				SHA256: strings.Repeat("0", 64), Bytes: 1, Origin: ArtifactThisRun, Required: true,
			},
		},
	}
	if err := declaration.Validate(); err != nil {
		return fmt.Errorf("synthetic observations: run identity is invalid: %w", err)
	}
	return nil
}

func cloneServiceObservation(o ServiceObservation) ServiceObservation {
	out := o
	if o.ExitCode != nil {
		code := *o.ExitCode
		out.ExitCode = &code
	}
	return out
}

// Canonical returns a deep copy with observations sorted by service. It never
// mutates the receiver and never adds information.
func (o SyntheticServiceObservations) Canonical() SyntheticServiceObservations {
	out := o
	out.Observations = make([]ServiceObservation, len(o.Observations))
	for i, obs := range o.Observations {
		out.Observations[i] = cloneServiceObservation(obs)
	}
	sort.SliceStable(out.Observations, func(i, j int) bool {
		return out.Observations[i].Service < out.Observations[j].Service
	})
	return out
}

// CanonicalJSON is the deterministic compact canonical representation used for
// semantic hashing.
func (o SyntheticServiceObservations) CanonicalJSON() ([]byte, error) {
	return json.Marshal(o.Canonical())
}

// CanonicalSHA256 is the semantic observation hash. It is order-invariant for
// the observation array.
func (o SyntheticServiceObservations) CanonicalSHA256() (string, error) {
	b, err := o.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}
