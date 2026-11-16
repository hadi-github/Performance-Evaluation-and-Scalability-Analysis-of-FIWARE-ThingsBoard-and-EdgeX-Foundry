// Validation of the M2e health-evidence report.
package deployment

import (
	"errors"
	"fmt"
	"strings"
)

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
