// Decoding, evaluation and derivation of the M2e health-evidence dry run.
package deployment

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

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
