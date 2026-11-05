package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// This file adds the M2e offline synthetic health-evidence dry run. It is a pure
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

// HealthServiceResult is one declared service compared with one synthetic
// observation. Expected is copied from the execution plan so tier and health
// modes remain visible.
type HealthServiceResult struct {
	Service     string             `json:"service"`
	Expected    ExpectedService    `json:"expected"`
	Observation ServiceObservation `json:"observation"`
	Status      Status             `json:"status"`
	Reason      string             `json:"reason"`
}

// HealthEvidenceInputs records the content identity of both raw inputs. It
// contains hashes and byte counts only, never a filesystem path.
type HealthEvidenceInputs struct {
	ExecutionPlanRawSHA256       string `json:"execution_plan_raw_sha256"`
	ExecutionPlanCanonicalSHA256 string `json:"execution_plan_canonical_sha256"`
	ExecutionPlanBytes           int64  `json:"execution_plan_bytes"`
	ObservationsRawSHA256        string `json:"observations_raw_sha256"`
	ObservationsCanonicalSHA256  string `json:"observations_canonical_sha256"`
	ObservationsBytes            int64  `json:"observations_bytes"`
}

// HealthEvidenceDryRun is the strict M2e report. It carries the nested execution
// plan unchanged, the synthetic service evaluation, the declared artifact
// manifest and the writer's actual artifact verification.
type HealthEvidenceDryRun struct {
	SchemaVersion              int                    `json:"schema_version"`
	Interpretation             string                 `json:"interpretation"`
	Mode                       string                 `json:"mode"`
	Synthetic                  string                 `json:"synthetic"`
	Platform                   string                 `json:"platform"`
	Project                    string                 `json:"project"`
	ObservationSource          string                 `json:"observation_source"`
	Inputs                     HealthEvidenceInputs   `json:"inputs"`
	ExecutionPlan              ExecutionPlan          `json:"execution_plan"`
	Services                   []HealthServiceResult  `json:"services"`
	HealthStatus               Status                 `json:"health_status"`
	ArtifactManifest           ArtifactManifest       `json:"artifact_manifest"`
	ArtifactVerification       []ArtifactVerification `json:"artifact_verification"`
	EvidenceStatus             Status                 `json:"evidence_status"`
	DryRunStatus               Status                 `json:"dry_run_status"`
	LiveValidation             Status                 `json:"live_validation"`
	ObserverStatus             Status                 `json:"observer_status"`
	PriorManifestVerification  string                 `json:"prior_manifest_verification"`
	PriorManifestBindingStatus Status                 `json:"prior_manifest_binding_status"`
	Limitations                []string               `json:"limitations"`
}

var healthEvidenceInputsShape = jsonShape{
	object: map[string]jsonShape{
		"execution_plan_raw_sha256":       scalarValueShape,
		"execution_plan_canonical_sha256": scalarValueShape,
		"execution_plan_bytes":            scalarValueShape,
		"observations_raw_sha256":         scalarValueShape,
		"observations_canonical_sha256":   scalarValueShape,
		"observations_bytes":              scalarValueShape,
	},
}

var healthExpectedServiceShape = jsonShape{
	object: map[string]jsonShape{
		"service":      scalarValueShape,
		"tier":         scalarValueShape,
		"replicas":     scalarValueShape,
		"lifecycle":    scalarValueShape,
		"activation":   scalarValueShape,
		"healthcheck":  scalarValueShape,
		"health_modes": {element: &scalarValueShape},
	},
}

var healthServiceResultShape = jsonShape{
	object: map[string]jsonShape{
		"service":     scalarValueShape,
		"expected":    healthExpectedServiceShape,
		"observation": serviceObservationShape,
		"status":      scalarValueShape,
		"reason":      scalarValueShape,
	},
}

var artifactVerificationShape = jsonShape{
	object: map[string]jsonShape{
		"role":            scalarValueShape,
		"path":            scalarValueShape,
		"declared_sha256": scalarValueShape,
		"actual_sha256":   scalarValueShape,
		"declared_bytes":  scalarValueShape,
		"actual_bytes":    scalarValueShape,
		"status":          scalarValueShape,
		"reason":          scalarValueShape,
	},
	optional: map[string]bool{"actual_sha256": true, "actual_bytes": true, "reason": true},
}

var healthEvidenceShape = jsonShape{
	object: map[string]jsonShape{
		"schema_version":                scalarValueShape,
		"interpretation":                scalarValueShape,
		"mode":                          scalarValueShape,
		"synthetic":                     scalarValueShape,
		"platform":                      scalarValueShape,
		"project":                       scalarValueShape,
		"observation_source":            scalarValueShape,
		"inputs":                        healthEvidenceInputsShape,
		"execution_plan":                executionPlanShape,
		"services":                      {element: &healthServiceResultShape},
		"health_status":                 scalarValueShape,
		"artifact_manifest":             artifactManifestShape,
		"artifact_verification":         {element: &artifactVerificationShape},
		"evidence_status":               scalarValueShape,
		"dry_run_status":                scalarValueShape,
		"live_validation":               scalarValueShape,
		"observer_status":               scalarValueShape,
		"prior_manifest_verification":   scalarValueShape,
		"prior_manifest_binding_status": scalarValueShape,
		"limitations":                   {element: &scalarValueShape},
	},
}

// DecodeHealthEvidenceDryRun strictly decodes an M2e report using the same
// exact-key, duplicate-key, null and trailing-JSON rules as the approved M2
// documents, then validates every derived value.
func DecodeHealthEvidenceDryRun(b []byte) (*HealthEvidenceDryRun, error) {
	if err := validateRawDocument(b, healthEvidenceShape, "health evidence dry run"); err != nil {
		return nil, err
	}
	var r HealthEvidenceDryRun
	if err := decodeStrict(b, &r); err != nil {
		return nil, fmt.Errorf("health evidence dry run: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

func healthServiceExpectation(svc ExpectedService) ServiceExpectation {
	return ServiceExpectation{
		Service:     svc.Service,
		Replicas:    svc.Replicas,
		Lifecycle:   svc.Lifecycle,
		Activation:  svc.Activation,
		Healthcheck: svc.Healthcheck,
	}
}

func evaluateHealthService(expected ExpectedService, observation ServiceObservation) (Status, string) {
	return EvaluateServiceExpectation(healthServiceExpectation(expected), observation)
}

func buildHealthEvidenceServices(plan *ExecutionPlan, observations []ServiceObservation) []HealthServiceResult {
	byService := map[string]ServiceObservation{}
	for _, obs := range observations {
		byService[obs.Service] = cloneServiceObservation(obs)
	}
	out := make([]HealthServiceResult, 0, len(plan.ExpectedServices))
	for _, expected := range plan.Canonical().ExpectedServices {
		observation, ok := byService[expected.Service]
		if !ok {
			// An omitted observation is explicitly represented; presence,
			// replicas, exit code and functional evidence are never invented.
			observation = ServiceObservation{Service: expected.Service, Health: ObservedUnknown}
		}
		status, reason := evaluateHealthService(expected, observation)
		out = append(out, HealthServiceResult{
			Service:     expected.Service,
			Expected:    expected,
			Observation: observation,
			Status:      status,
			Reason:      reason,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Expected.Tier != out[j].Expected.Tier {
			return tierRank[out[i].Expected.Tier] < tierRank[out[j].Expected.Tier]
		}
		return out[i].Service < out[j].Service
	})
	return out
}

func buildHealthEvidenceManifest(o *SyntheticServiceObservations, platform string, executionPlanRaw, observationsRaw []byte) (ArtifactManifest, error) {
	planPath, err := ArtifactPath(o.RunID, HealthEvidenceExecutionPlanInputName)
	if err != nil {
		return ArtifactManifest{}, err
	}
	obsPath, err := ArtifactPath(o.RunID, HealthEvidenceObservationsInputName)
	if err != nil {
		return ArtifactManifest{}, err
	}
	manifest := ArtifactManifest{
		SchemaVersion:  ArtifactManifestSchemaVersion,
		Interpretation: ArtifactManifestInterpretation,
		RunID:          o.RunID,
		RunDate:        o.RunDate,
		RunTag:         o.RunTag,
		Platform:       platform,
		Artifacts: []ArtifactRecord{
			{
				Role: ArtifactRoleOrchestrationPlan, Path: planPath,
				SHA256: SHA256Bytes(executionPlanRaw), Bytes: int64(len(executionPlanRaw)),
				Origin: ArtifactThisRun, Required: true,
			},
			{
				Role: ArtifactRoleOther, Path: obsPath,
				SHA256: SHA256Bytes(observationsRaw), Bytes: int64(len(observationsRaw)),
				Origin: ArtifactThisRun, Required: true,
			},
		},
	}
	if err := manifest.Validate(); err != nil {
		return ArtifactManifest{}, err
	}
	return manifest, nil
}

// BuildHealthEvidenceDryRun evaluates caller-supplied synthetic observations
// against an approved M2d execution plan. It is pure and file-free: it reads no
// file, probes no environment and contacts no Docker daemon, SUT or service.
func BuildHealthEvidenceDryRun(executionPlanRaw, observationsRaw []byte) (*HealthEvidenceDryRun, error) {
	plan, err := DecodeExecutionPlan(executionPlanRaw)
	if err != nil {
		return nil, err
	}
	observations, err := DecodeSyntheticServiceObservations(observationsRaw)
	if err != nil {
		return nil, err
	}
	expected := map[string]bool{}
	for _, svc := range plan.ExpectedServices {
		expected[svc.Service] = true
	}
	for _, obs := range observations.Observations {
		if !expected[obs.Service] {
			return nil, fmt.Errorf("synthetic observations: service %q is not declared by the execution plan", obs.Service)
		}
	}
	planCanonicalSHA256, err := plan.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	observationsCanonicalSHA256, err := observations.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	manifest, err := buildHealthEvidenceManifest(observations, plan.Platform, executionPlanRaw, observationsRaw)
	if err != nil {
		return nil, err
	}
	canonicalPlan := plan.Canonical()
	services := buildHealthEvidenceServices(&canonicalPlan, observations.Observations)
	statuses := make([]Status, 0, len(services))
	for _, svc := range services {
		statuses = append(statuses, svc.Status)
	}
	healthStatus := aggregateStatus(statuses)
	evidenceStatus := NotAssessed
	result := &HealthEvidenceDryRun{
		SchemaVersion:     HealthEvidenceSchemaVersion,
		Interpretation:    HealthEvidenceInterpretation,
		Mode:              HealthEvidenceMode,
		Synthetic:         HealthEvidenceSyntheticLabel,
		Platform:          plan.Platform,
		Project:           plan.Project,
		ObservationSource: observations.Source,
		Inputs: HealthEvidenceInputs{
			ExecutionPlanRawSHA256:       SHA256Bytes(executionPlanRaw),
			ExecutionPlanCanonicalSHA256: planCanonicalSHA256,
			ExecutionPlanBytes:           int64(len(executionPlanRaw)),
			ObservationsRawSHA256:        SHA256Bytes(observationsRaw),
			ObservationsCanonicalSHA256:  observationsCanonicalSHA256,
			ObservationsBytes:            int64(len(observationsRaw)),
		},
		ExecutionPlan:              canonicalPlan,
		Services:                   services,
		HealthStatus:               healthStatus,
		ArtifactManifest:           manifest,
		ArtifactVerification:       []ArtifactVerification{},
		EvidenceStatus:             evidenceStatus,
		DryRunStatus:               aggregateStatus([]Status{healthStatus, evidenceStatus}),
		LiveValidation:             NotAssessed,
		ObserverStatus:             NotAssessed,
		PriorManifestVerification:  string(PriorManifestVerificationNotImplemented),
		PriorManifestBindingStatus: NotAssessed,
		Limitations: []string{
			"SYNTHETIC M2e offline dry run: health results compare only caller-supplied synthetic service observations with the approved execution-plan declaration. No Docker, SUT, platform, observer, lifecycle, deployment, fault or load action is performed.",
			"A PASS health or dry-run status means only that the supplied synthetic observations are internally consistent with the declared plan. It is not live qualification, campaign success, reachability evidence or proof that any service exists.",
			"The nested execution plan is preserved unchanged: the ordered steps, three EXECUTION_BLOCKED_HERE rows and all seven NOT EXECUTED statuses remain in force.",
			"No observation, presence, replica, exit code, functional evidence, artifact hash, timestamp, environment value or working-directory path is inferred, probed or fabricated.",
			"Artifact verification is limited to the two declared raw inputs in this new run directory. Prior-manifest binding is NOT_IMPLEMENTED and no preserved_prior artifact is declared.",
		},
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

func expectedServiceEqual(a, b ExpectedService) bool {
	if a.Service != b.Service || a.Tier != b.Tier || a.Replicas != b.Replicas ||
		a.Lifecycle != b.Lifecycle || a.Activation != b.Activation || a.Healthcheck != b.Healthcheck {
		return false
	}
	return sameStringSet(a.HealthModes, b.HealthModes)
}

func validateHealthObservation(obs ServiceObservation) error {
	if !serviceNamePattern.MatchString(obs.Service) {
		return fmt.Errorf("service %q is not a valid compose service name", obs.Service)
	}
	if obs.Replicas < 0 {
		return fmt.Errorf("service %q has negative replicas", obs.Service)
	}
	if !validObservedHealth[string(obs.Health)] {
		return fmt.Errorf("service %q has unsupported health %q", obs.Service, obs.Health)
	}
	return nil
}

func validateHealthEvidenceManifest(manifest ArtifactManifest, platform string) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if manifest.Platform != platform {
		return fmt.Errorf("health evidence: artifact_manifest.platform %q does not match report platform %q", manifest.Platform, platform)
	}
	if !strings.HasPrefix(manifest.RunTag, SyntheticRunTagPrefix) {
		return fmt.Errorf("health evidence: artifact_manifest.run_tag must begin with %q", SyntheticRunTagPrefix)
	}
	if manifest.CreatedAtMS != nil {
		return errors.New("health evidence: artifact_manifest must not carry a timestamp")
	}
	if len(manifest.Artifacts) != 2 {
		return fmt.Errorf("health evidence: artifact_manifest must declare exactly two this_run artifacts, got %d", len(manifest.Artifacts))
	}
	planPath, err := ArtifactPath(manifest.RunID, HealthEvidenceExecutionPlanInputName)
	if err != nil {
		return err
	}
	obsPath, err := ArtifactPath(manifest.RunID, HealthEvidenceObservationsInputName)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, artifact := range manifest.Artifacts {
		if artifact.Origin != ArtifactThisRun {
			return fmt.Errorf("health evidence: artifact %q must be this_run", artifact.Path)
		}
		if !artifact.Required {
			return fmt.Errorf("health evidence: artifact %q must be required", artifact.Path)
		}
		switch artifact.Role {
		case ArtifactRoleOrchestrationPlan:
			if artifact.Path != planPath {
				return fmt.Errorf("health evidence: orchestration_plan artifact path must be %q", planPath)
			}
		case ArtifactRoleOther:
			if artifact.Path != obsPath {
				return fmt.Errorf("health evidence: other artifact path must be %q", obsPath)
			}
		default:
			return fmt.Errorf("health evidence: artifact role %q is not admitted in M2e", artifact.Role)
		}
		if seen[artifact.Role] {
			return fmt.Errorf("health evidence: duplicate artifact role %q", artifact.Role)
		}
		seen[artifact.Role] = true
	}
	if !seen[ArtifactRoleOrchestrationPlan] || !seen[ArtifactRoleOther] {
		return errors.New("health evidence: exactly the orchestration_plan and other artifacts are required")
	}
	return nil
}

func validateHealthEvidenceVerifications(manifest ArtifactManifest, verifications []ArtifactVerification) (Status, error) {
	if len(verifications) == 0 {
		return NotAssessed, nil
	}
	artifacts := manifest.Canonical().Artifacts
	if len(verifications) != len(artifacts) {
		return "", fmt.Errorf("health evidence: artifact_verification must contain one row per manifest artifact, got %d for %d", len(verifications), len(artifacts))
	}
	sorted := append([]ArtifactVerification(nil), verifications...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	statuses := make([]Status, 0, len(sorted))
	for i, artifact := range artifacts {
		v := sorted[i]
		if v.Path != artifact.Path || v.Role != artifact.Role || v.Declared != artifact.SHA256 || v.DeclaredBytes != artifact.Bytes {
			return "", fmt.Errorf("health evidence: verification row %d does not match manifest artifact %q", i, artifact.Path)
		}
		if !validStatus(v.Status) {
			return "", fmt.Errorf("health evidence: verification %q has invalid status %q", v.Path, v.Status)
		}
		if strings.TrimSpace(v.Reason) == "" {
			return "", fmt.Errorf("health evidence: verification %q requires a reason", v.Path)
		}
		switch v.Status {
		case Pass:
			if v.Actual != v.Declared || v.ActualBytes != v.DeclaredBytes {
				return "", fmt.Errorf("health evidence: verification %q claims PASS with a different hash or byte count", v.Path)
			}
		case Fail:
			if v.Actual == "" || (v.Actual == v.Declared && v.ActualBytes == v.DeclaredBytes) {
				return "", fmt.Errorf("health evidence: verification %q claims FAIL without a mismatch", v.Path)
			}
		case Unavailable:
		case NotAssessed:
			return "", fmt.Errorf("health evidence: verification %q cannot be NOT_ASSESSED", v.Path)
		}
		statuses = append(statuses, v.Status)
	}
	return aggregateStatus(statuses), nil
}

// Validate checks the report after decoding or direct construction. Health
// statuses and reasons are recomputed with the same predicate; artifact
// verification, evidence status, dry-run status and the execution boundary are
// checked against their declarations.
func (r HealthEvidenceDryRun) Validate() error {
	if r.SchemaVersion != HealthEvidenceSchemaVersion {
		return fmt.Errorf("health evidence: unsupported schema_version %d", r.SchemaVersion)
	}
	if r.Interpretation != HealthEvidenceInterpretation {
		return fmt.Errorf("health evidence: interpretation must be %q", HealthEvidenceInterpretation)
	}
	if r.Mode != HealthEvidenceMode {
		return fmt.Errorf("health evidence: mode must be %q", HealthEvidenceMode)
	}
	if r.Synthetic != HealthEvidenceSyntheticLabel {
		return fmt.Errorf("health evidence: synthetic label must be %q", HealthEvidenceSyntheticLabel)
	}
	if !platformAllowlisted(r.Platform) {
		return fmt.Errorf("health evidence: platform %q is outside the allowlist", r.Platform)
	}
	if want := platformProject[r.Platform]; r.Project != want {
		return fmt.Errorf("health evidence: platform %q must use project %q, got %q", r.Platform, want, r.Project)
	}
	if !strings.HasPrefix(r.ObservationSource, SyntheticSourcePrefix) {
		return fmt.Errorf("health evidence: observation_source must begin with %q", SyntheticSourcePrefix)
	}
	for label, hash := range map[string]string{
		"execution_plan_raw_sha256":       r.Inputs.ExecutionPlanRawSHA256,
		"execution_plan_canonical_sha256": r.Inputs.ExecutionPlanCanonicalSHA256,
		"observations_raw_sha256":         r.Inputs.ObservationsRawSHA256,
		"observations_canonical_sha256":   r.Inputs.ObservationsCanonicalSHA256,
	} {
		if !sha256Pattern.MatchString(hash) {
			return fmt.Errorf("health evidence: inputs.%s must be 64 lower-case hex characters", label)
		}
	}
	if r.Inputs.ExecutionPlanBytes <= 0 || r.Inputs.ObservationsBytes <= 0 {
		return errors.New("health evidence: input byte counts must be positive")
	}
	if err := r.ExecutionPlan.Validate(); err != nil {
		return err
	}
	if r.ExecutionPlan.Platform != r.Platform || r.ExecutionPlan.Project != r.Project {
		return errors.New("health evidence: nested execution plan platform/project does not match the report")
	}
	planCanonicalSHA256, err := r.ExecutionPlan.CanonicalSHA256()
	if err != nil {
		return err
	}
	if planCanonicalSHA256 != r.Inputs.ExecutionPlanCanonicalSHA256 {
		return errors.New("health evidence: nested execution plan does not match execution_plan_canonical_sha256")
	}
	if err := validateHealthEvidenceManifest(r.ArtifactManifest, r.Platform); err != nil {
		return err
	}
	expectedServices := r.ExecutionPlan.Canonical().ExpectedServices
	if len(r.Services) != len(expectedServices) {
		return fmt.Errorf("health evidence: services must contain exactly the %d expected plan services", len(expectedServices))
	}
	recomputed := make([]Status, 0, len(r.Services))
	for i, expected := range expectedServices {
		result := r.Services[i]
		context := fmt.Sprintf("health evidence service %d (%s)", i, expected.Service)
		if result.Service != expected.Service {
			return fmt.Errorf("%s: service name does not match the execution plan", context)
		}
		if !expectedServiceEqual(result.Expected, expected) {
			return fmt.Errorf("%s: expected declaration does not match the execution plan", context)
		}
		if err := validateHealthObservation(result.Observation); err != nil {
			return fmt.Errorf("%s: %w", context, err)
		}
		if result.Observation.Service != expected.Service {
			return fmt.Errorf("%s: observation service %q does not match", context, result.Observation.Service)
		}
		status, reason := evaluateHealthService(expected, result.Observation)
		if result.Status != status {
			return fmt.Errorf("%s: status %q does not match the recomputed %q", context, result.Status, status)
		}
		if result.Reason != reason {
			return fmt.Errorf("%s: reason does not match the recomputed reason", context)
		}
		if i > 0 {
			prev := r.Services[i-1]
			if tierRank[prev.Expected.Tier] > tierRank[result.Expected.Tier] ||
				(prev.Expected.Tier == result.Expected.Tier && prev.Service > result.Service) {
				return fmt.Errorf("%s: services must be sorted by tier then service", context)
			}
		}
		recomputed = append(recomputed, status)
	}
	if want := aggregateStatus(recomputed); r.HealthStatus != want {
		return fmt.Errorf("health evidence: health_status %s does not match the service aggregate %s", r.HealthStatus, want)
	}
	evidenceStatus, err := validateHealthEvidenceVerifications(r.ArtifactManifest, r.ArtifactVerification)
	if err != nil {
		return err
	}
	if r.EvidenceStatus != evidenceStatus {
		return fmt.Errorf("health evidence: evidence_status %s does not match the artifact aggregate %s", r.EvidenceStatus, evidenceStatus)
	}
	if want := aggregateStatus([]Status{r.HealthStatus, r.EvidenceStatus}); r.DryRunStatus != want {
		return fmt.Errorf("health evidence: dry_run_status %s does not match the health+evidence aggregate %s", r.DryRunStatus, want)
	}
	if r.LiveValidation != NotAssessed {
		return fmt.Errorf("health evidence: live_validation must be %s, got %s", NotAssessed, r.LiveValidation)
	}
	if r.ObserverStatus != NotAssessed {
		return fmt.Errorf("health evidence: observer_status must be %s, got %s", NotAssessed, r.ObserverStatus)
	}
	if r.PriorManifestVerification != string(PriorManifestVerificationNotImplemented) {
		return fmt.Errorf("health evidence: prior_manifest_verification must be %q", PriorManifestVerificationNotImplemented)
	}
	if r.PriorManifestBindingStatus != NotAssessed {
		return fmt.Errorf("health evidence: prior_manifest_binding_status must be %s", NotAssessed)
	}
	if len(r.Limitations) == 0 {
		return errors.New("health evidence: limitations must be a non-empty explicit array")
	}
	for _, limitation := range r.Limitations {
		if strings.TrimSpace(limitation) == "" {
			return errors.New("health evidence: limitations contains an empty value")
		}
	}
	return nil
}

// Canonical returns a deep copy with deterministic ordering. Execution step
// order and both limitation lists are preserved exactly.
func (r HealthEvidenceDryRun) Canonical() HealthEvidenceDryRun {
	out := r
	out.ExecutionPlan = r.ExecutionPlan.Canonical()
	out.Services = make([]HealthServiceResult, len(r.Services))
	for i, svc := range r.Services {
		out.Services[i] = svc
		out.Services[i].Expected = svc.Expected
		out.Services[i].Expected.HealthModes = append([]string(nil), svc.Expected.HealthModes...)
		sort.Strings(out.Services[i].Expected.HealthModes)
		out.Services[i].Observation = cloneServiceObservation(svc.Observation)
	}
	sort.SliceStable(out.Services, func(i, j int) bool {
		if out.Services[i].Expected.Tier != out.Services[j].Expected.Tier {
			return tierRank[out.Services[i].Expected.Tier] < tierRank[out.Services[j].Expected.Tier]
		}
		return out.Services[i].Service < out.Services[j].Service
	})
	out.ArtifactManifest = r.ArtifactManifest.Canonical()
	out.ArtifactVerification = make([]ArtifactVerification, len(r.ArtifactVerification))
	copy(out.ArtifactVerification, r.ArtifactVerification)
	sort.SliceStable(out.ArtifactVerification, func(i, j int) bool {
		return out.ArtifactVerification[i].Path < out.ArtifactVerification[j].Path
	})
	out.Limitations = append([]string(nil), r.Limitations...)
	return out
}

// CanonicalJSON is the deterministic compact canonical representation used for
// semantic hashing and for the canonical output file.
func (r HealthEvidenceDryRun) CanonicalJSON() ([]byte, error) {
	return json.Marshal(r.Canonical())
}

// CanonicalSHA256 is the semantic report hash.
func (r HealthEvidenceDryRun) CanonicalSHA256() (string, error) {
	b, err := r.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

// Marshal returns deterministic pretty JSON with a trailing newline. It adds no
// timestamp, environment value or live claim.
func (r *HealthEvidenceDryRun) Marshal() ([]byte, error) {
	if r == nil {
		return nil, errors.New("nil health evidence dry run")
	}
	b, err := json.MarshalIndent(r.Canonical(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func observationExitCode(o ServiceObservation) string {
	if o.ExitCode == nil {
		return "—"
	}
	return fmt.Sprintf("%d", *o.ExitCode)
}

// RenderMarkdown returns a deterministic human-readable report. It prominently
// labels the document synthetic and limits PASS to supplied synthetic
// consistency.
func (r *HealthEvidenceDryRun) RenderMarkdown() string {
	if r == nil {
		return ""
	}
	c := r.Canonical()
	var b strings.Builder
	fmt.Fprintf(&b, "# Health evidence dry run — %s\n\n", HealthEvidenceSyntheticLabel)
	fmt.Fprintf(&b, "**Synthetic: %s. Health: %s. Evidence: %s. Dry run: %s. Live validation: %s.**\n\n",
		mdCell(c.Synthetic), c.HealthStatus, c.EvidenceStatus, c.DryRunStatus, c.LiveValidation)
	fmt.Fprintf(&b, "> This is a **synthetic, offline dry run**. It compares caller-supplied synthetic service observations with an approved execution-plan declaration. A PASS here means only that the supplied synthetic observations are internally consistent with that declaration. It is **not live qualification**, campaign success, reachability evidence or proof that any service exists.\n\n")
	fmt.Fprintf(&b, "Interpretation: `%s`; mode: `%s`; schema version: `%d`. Platform: `%s`; project: `%s`.\n\n",
		mdCell(c.Interpretation), mdCell(c.Mode), c.SchemaVersion, mdCell(c.Platform), mdCell(c.Project))
	fmt.Fprintf(&b, "Observation source: `%s`.\n\n", mdCell(c.ObservationSource))

	fmt.Fprintf(&b, "## Input identity\n\n")
	fmt.Fprintf(&b, "| Input | Raw SHA-256 | Canonical SHA-256 | Bytes |\n|---|---|---|---:|\n")
	fmt.Fprintf(&b, "| execution plan | `%s` | `%s` | %d |\n",
		mdCell(c.Inputs.ExecutionPlanRawSHA256), mdCell(c.Inputs.ExecutionPlanCanonicalSHA256), c.Inputs.ExecutionPlanBytes)
	fmt.Fprintf(&b, "| synthetic observations | `%s` | `%s` | %d |\n\n",
		mdCell(c.Inputs.ObservationsRawSHA256), mdCell(c.Inputs.ObservationsCanonicalSHA256), c.Inputs.ObservationsBytes)

	fmt.Fprintf(&b, "## Health results\n\n")
	fmt.Fprintf(&b, "| Service | Tier | Expected replicas | Present | Observed replicas | Running | Health | Exit code | Functional | Status | Reason |\n|---|---|---:|---|---:|---|---|---:|---|---|---|\n")
	for _, svc := range c.Services {
		fmt.Fprintf(&b, "| %s | %s | %d | %t | %d | %t | %s | %s | %t | %s | %s |\n",
			mdCell(svc.Service), mdCell(string(svc.Expected.Tier)), svc.Expected.Replicas,
			svc.Observation.Present, svc.Observation.Replicas, svc.Observation.Running,
			mdCell(string(svc.Observation.Health)), observationExitCode(svc.Observation),
			svc.Observation.FunctionalEvidence, svc.Status, mdCell(svc.Reason))
	}
	fmt.Fprintf(&b, "\nHealth status: **%s** (FAIL > UNAVAILABLE > NOT_ASSESSED > PASS).\n\n", c.HealthStatus)

	fmt.Fprintf(&b, "## Nested execution plan (unchanged)\n\n")
	fmt.Fprintf(&b, "Mode: `%s`; execution blocked: `%t`; authorization: `%s`.\n\n",
		mdCell(c.ExecutionPlan.Mode), c.ExecutionPlan.ExecutionBlocked, mdCell(c.ExecutionPlan.Authorization))
	fmt.Fprintf(&b, "| # | Step | Mode | Marker | Status |\n|---:|---|---|---|---|\n")
	for _, step := range c.ExecutionPlan.Steps {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			step.Order, mdCell(step.Step), mdCell(string(step.Mode)), mdCell(step.Marker), step.ExecutionStatus)
	}
	fmt.Fprintf(&b, "\n> Three execution-bearing rows remain `%s` and all seven rows remain `%s`. No row was executed.\n\n",
		ExecutionBlockedMarker, ExecutionStatusNotExecuted)

	fmt.Fprintf(&b, "## Artifact manifest and verification\n\n")
	fmt.Fprintf(&b, "Run `%s`; tag `%s`; date `%s`; platform `%s`.\n\n",
		mdCell(c.ArtifactManifest.RunID), mdCell(c.ArtifactManifest.RunTag),
		mdCell(c.ArtifactManifest.RunDate), mdCell(c.ArtifactManifest.Platform))
	fmt.Fprintf(&b, "| Role | Path | SHA-256 | Bytes | Origin | Required |\n|---|---|---|---:|---|---|\n")
	for _, artifact := range c.ArtifactManifest.Canonical().Artifacts {
		fmt.Fprintf(&b, "| %s | %s | `%s` | %d | %s | %t |\n",
			mdCell(artifact.Role), mdCell(artifact.Path), mdCell(artifact.SHA256),
			artifact.Bytes, artifact.Origin, artifact.Required)
	}
	fmt.Fprintf(&b, "\n")
	if len(c.ArtifactVerification) == 0 {
		fmt.Fprintf(&b, "Artifact verification: **%s** (no writer verification has run).\n\n", c.EvidenceStatus)
	} else {
		fmt.Fprintf(&b, "| Role | Path | Declared SHA-256 | Actual SHA-256 | Status | Reason |\n|---|---|---|---|---|---|\n")
		for _, v := range c.ArtifactVerification {
			fmt.Fprintf(&b, "| %s | %s | `%s` | `%s` | %s | %s |\n",
				mdCell(v.Role), mdCell(v.Path), mdCell(v.Declared), mdCell(v.Actual), v.Status, mdCell(v.Reason))
		}
		fmt.Fprintf(&b, "\nEvidence status: **%s** (aggregated from actual artifact verification).\n\n", c.EvidenceStatus)
	}

	fmt.Fprintf(&b, "## Status boundaries\n\n")
	fmt.Fprintf(&b, "- Live validation: **%s**.\n", c.LiveValidation)
	fmt.Fprintf(&b, "- Observer: **%s**.\n", c.ObserverStatus)
	fmt.Fprintf(&b, "- Prior-manifest verification: `%s`.\n", mdCell(c.PriorManifestVerification))
	fmt.Fprintf(&b, "- Prior-manifest binding: **%s**.\n", c.PriorManifestBindingStatus)
	fmt.Fprintf(&b, "- Dry-run status combines only health and artifact evidence: **%s**.\n\n", c.DryRunStatus)

	fmt.Fprintf(&b, "## Limitations\n\n")
	for _, limitation := range c.Limitations {
		fmt.Fprintf(&b, "- %s\n", mdCell(limitation))
	}
	fmt.Fprintf(&b, "\n## Non-goals and lifecycle exclusion\n\n")
	fmt.Fprintf(&b, "- No Docker client call, SUT access, deployment, lifecycle action, observer implementation, fault, load, reset or delete.\n")
	fmt.Fprintf(&b, "- No live health, replica, exit code, functional evidence, artifact or monitoring value is collected or fabricated.\n")
	fmt.Fprintf(&b, "- No prior artifact is copied, rewritten, deleted or bound to its own prior manifest.\n")
	fmt.Fprintf(&b, "- No timestamp, environment probe or absolute working-directory path is included.\n")
	return b.String()
}

func healthEvidenceCore(r *HealthEvidenceDryRun) *HealthEvidenceDryRun {
	out := r.Canonical()
	out.ArtifactVerification = []ArtifactVerification{}
	out.EvidenceStatus = NotAssessed
	out.DryRunStatus = aggregateStatus([]Status{out.HealthStatus, out.EvidenceStatus})
	return &out
}

// WriteHealthEvidenceDryRun creates a NEW results/<run_id> output directory,
// writes copies of the exact raw inputs, verifies them through the existing
// ArtifactManifest.Verify, records the actual verification, and writes the
// final JSON, canonical JSON, Markdown, manifest and SHA256SUMS. It never writes
// outside that directory. A health FAIL is a valid report, not a writer error.
func WriteHealthEvidenceDryRun(out string, r *HealthEvidenceDryRun, executionPlanRaw, observationsRaw []byte, inputs []string) error {
	if r == nil {
		return errors.New("nil health evidence dry run")
	}
	built, err := BuildHealthEvidenceDryRun(executionPlanRaw, observationsRaw)
	if err != nil {
		return err
	}
	wantCore, err := healthEvidenceCore(built).CanonicalJSON()
	if err != nil {
		return err
	}
	gotCore, err := healthEvidenceCore(r).CanonicalJSON()
	if err != nil {
		return err
	}
	if !bytes.Equal(wantCore, gotCore) {
		return errors.New("health evidence: report does not match the supplied raw inputs")
	}
	cleanOut := filepath.Clean(out)
	runID := built.ArtifactManifest.RunID
	expectedParent := strings.TrimSuffix(ArtifactRunDirectoryPrefix, "/")
	if filepath.Base(cleanOut) != runID || filepath.Base(filepath.Dir(cleanOut)) != expectedParent {
		return fmt.Errorf("health evidence: output must end with results/%s", runID)
	}
	verificationRoot := filepath.Dir(filepath.Dir(cleanOut))
	if err := newDirectory(out, inputs); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceExecutionPlanInputName), executionPlanRaw, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceObservationsInputName), observationsRaw, 0644); err != nil {
		return err
	}
	verifications, verifyErr := built.ArtifactManifest.Verify(verificationRoot)
	built.ArtifactVerification = verifications
	statuses := make([]Status, 0, len(verifications))
	for _, v := range verifications {
		statuses = append(statuses, v.Status)
	}
	built.EvidenceStatus = aggregateStatus(statuses)
	if len(verifications) == 0 {
		built.EvidenceStatus = NotAssessed
	}
	built.DryRunStatus = aggregateStatus([]Status{built.HealthStatus, built.EvidenceStatus})
	if err := built.Validate(); err != nil {
		return err
	}
	pretty, err := built.Marshal()
	if err != nil {
		return err
	}
	canonical, err := built.CanonicalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceReportName), pretty, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceCanonicalName), canonical, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceMarkdownName), []byte(built.RenderMarkdown()), 0644); err != nil {
		return err
	}
	manifestJSON, err := json.MarshalIndent(built.ArtifactManifest.Canonical(), "", "  ")
	if err != nil {
		return err
	}
	manifestJSON = append(manifestJSON, '\n')
	if err := os.WriteFile(filepath.Join(out, "artifact-manifest.json"), manifestJSON, 0644); err != nil {
		return err
	}
	if err := checksums(out); err != nil {
		return err
	}
	// Preserve the report even when verification found a problem, then surface
	// the verification error to the caller.
	return verifyErr
}
