// Validation of the M2d execution-plan invariants.
package deployment

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks the plan after decoding or direct construction. It rejects any
// attempt to unblock an execution-bearing step, any missing/reordered/duplicate
// step, any fabricated resource observation and any evidence claim.
func (p ExecutionPlan) Validate() error {
	if p.SchemaVersion != ExecutionPlanSchemaVersion {
		return fmt.Errorf("execution plan: unsupported schema_version %d", p.SchemaVersion)
	}
	if p.Interpretation != ExecutionPlanInterpretation {
		return fmt.Errorf("execution plan: interpretation must be %q", ExecutionPlanInterpretation)
	}
	if p.Mode != ExecutionPlanMode {
		return fmt.Errorf("execution plan: mode must be %q", ExecutionPlanMode)
	}
	if p.Authorization != OrchestrationAuthorization {
		return fmt.Errorf("execution plan: authorization must be %q", OrchestrationAuthorization)
	}
	if !p.ExecutionBlocked {
		return errors.New("execution plan: execution_blocked must be true; M2d admits no executable plan")
	}
	if !platformAllowlisted(p.Platform) {
		return fmt.Errorf("execution plan: platform %q is outside the allowlist", p.Platform)
	}
	if want := platformProject[p.Platform]; p.Project != want {
		return fmt.Errorf("execution plan: platform %q must use project %q, got %q", p.Platform, want, p.Project)
	}
	if p.PlanSchemaVersion != OrchestrationSchemaVersion {
		return fmt.Errorf("execution plan: plan_schema_version %d is not the approved v%d", p.PlanSchemaVersion, OrchestrationSchemaVersion)
	}
	if p.PlanInterpretation != OrchestrationInterpretation {
		return fmt.Errorf("execution plan: plan_interpretation must be %q", OrchestrationInterpretation)
	}
	if err := validateStageMachine(p.PlanStageOrder); err != nil {
		return fmt.Errorf("execution plan: %w", err)
	}
	if err := validateExecutionInputs(p.Inputs); err != nil {
		return err
	}
	if len(p.Steps) != len(executionStepSpecs) {
		return fmt.Errorf("execution plan: steps must contain exactly the %d documented runbook rows", len(executionStepSpecs))
	}
	for i, spec := range executionStepSpecs {
		if err := validateExecutionStep(p.Steps[i], i, spec, p.Authorization); err != nil {
			return err
		}
	}
	if err := validateExpectedServices(p.ExpectedServices); err != nil {
		return err
	}
	if err := validateResourceGuard(p.ResourceGuard); err != nil {
		return err
	}
	if err := validateEvidenceChecklist(p.EvidenceChecklist); err != nil {
		return err
	}
	evidenceRoles := map[string]bool{}
	for _, item := range p.EvidenceChecklist.Roles {
		evidenceRoles[item.Role] = true
	}
	for _, step := range p.Steps {
		for _, role := range step.ExpectedEvidence {
			if !evidenceRoles[role] {
				return fmt.Errorf("execution plan: step %q expects evidence role %q absent from the checklist", step.Step, role)
			}
		}
	}
	if len(p.Limitations) == 0 {
		return errors.New("execution plan: limitations must be a non-empty explicit array")
	}
	for _, limitation := range p.Limitations {
		if strings.TrimSpace(limitation) == "" {
			return errors.New("execution plan: limitations contains an empty value")
		}
	}
	return nil
}

func validateExecutionInputs(inputs ExecutionInputs) error {
	for label, hash := range map[string]string{
		"descriptor_raw_sha256":               inputs.DescriptorRawSHA256,
		"descriptor_canonical_sha256":         inputs.DescriptorCanonicalSHA256,
		"inventory_raw_sha256":                inputs.InventoryRawSHA256,
		"inventory_canonical_sha256":          inputs.InventoryCanonicalSHA256,
		"orchestration_plan_raw_sha256":       inputs.OrchestrationPlanRawSHA256,
		"orchestration_plan_canonical_sha256": inputs.OrchestrationPlanCanonicalSHA256,
	} {
		if !sha256Pattern.MatchString(hash) {
			return fmt.Errorf("execution plan: inputs.%s must be 64 lower-case hex characters", label)
		}
	}
	return nil
}

func validateExecutionStep(step ExecutionStep, index int, spec executionStepSpec, authorization string) error {
	context := fmt.Sprintf("execution plan step %d (%s)", index+1, spec.Step)
	if step.Order != index+1 {
		return fmt.Errorf("%s: order must be %d, got %d", context, index+1, step.Order)
	}
	if step.Step != spec.Step {
		return fmt.Errorf("%s: step identifier must be %q, got %q", context, spec.Step, step.Step)
	}
	if step.PlanStage != spec.Stage {
		return fmt.Errorf("%s: plan_stage must be %q, got %q", context, spec.Stage, step.PlanStage)
	}
	if step.Mode != spec.Mode {
		return fmt.Errorf("%s: mode must be %q, got %q", context, spec.Mode, step.Mode)
	}
	if strings.TrimSpace(step.Purpose) == "" {
		return fmt.Errorf("%s: purpose is required", context)
	}
	if len(step.PermissibleActions) == 0 {
		return fmt.Errorf("%s: permissible_actions must be non-empty", context)
	}
	seenSafe := map[Action]bool{}
	for _, action := range step.PermissibleActions {
		if !IsSafeAction(action) {
			return fmt.Errorf("%s: action %q is not in the read-only taxonomy", context, action)
		}
		if seenSafe[action] {
			return fmt.Errorf("%s: duplicate permissible action %q", context, action)
		}
		seenSafe[action] = true
	}
	seenBlocked := map[Action]bool{}
	for _, action := range step.BlockedActions {
		if !IsBlockedAction(action) {
			return fmt.Errorf("%s: blocked action %q is not a lifecycle action", context, action)
		}
		if seenSafe[action] {
			return fmt.Errorf("%s: action %q cannot be both permissible and blocked", context, action)
		}
		if seenBlocked[action] {
			return fmt.Errorf("%s: duplicate blocked action %q", context, action)
		}
		seenBlocked[action] = true
	}
	if len(step.OperationIDs) == 0 {
		return fmt.Errorf("%s: operation_ids must be non-empty", context)
	}
	seenOperations := map[string]bool{}
	for _, id := range step.OperationIDs {
		if !operationIDPattern.MatchString(id) {
			return fmt.Errorf("%s: operation identifier %q is not a structured operation id", context, id)
		}
		if seenOperations[id] {
			return fmt.Errorf("%s: duplicate operation identifier %q", context, id)
		}
		seenOperations[id] = true
	}
	seenEvidence := map[string]bool{}
	for _, role := range step.ExpectedEvidence {
		if role == "" || strings.TrimSpace(role) != role {
			return fmt.Errorf("%s: expected_evidence role %q must be non-empty without surrounding whitespace", context, role)
		}
		if seenEvidence[role] {
			return fmt.Errorf("%s: duplicate expected_evidence role %q", context, role)
		}
		seenEvidence[role] = true
	}
	if step.ExecutionStatus != ExecutionStatusNotExecuted {
		return fmt.Errorf("%s: execution_status must be %q, got %q", context, ExecutionStatusNotExecuted, step.ExecutionStatus)
	}
	switch spec.Mode {
	case StepModeExecutionBlocked:
		if !step.ExecutionBlockedHere {
			return fmt.Errorf("%s: execution_blocked_here must be true; %s", context, ExecutionBlockedMarker)
		}
		if len(step.BlockedActions) == 0 {
			return fmt.Errorf("%s: execution step must reference at least one blocked action", context)
		}
		if step.Marker != ExecutionBlockedMarker {
			return fmt.Errorf("%s: marker must be %q", context, ExecutionBlockedMarker)
		}
		if strings.TrimSpace(step.BlockedReason) == "" {
			return fmt.Errorf("%s: blocked_reason is required", context)
		}
		if step.RequiredAuthorization == "" || step.RequiredAuthorization != authorization {
			return fmt.Errorf("%s: required_authorization must be %q", context, authorization)
		}
		switch spec.Step {
		case "start_apply_reference":
			if !seenBlocked[ActionApply] {
				return fmt.Errorf("%s: apply step must reference Apply as a blocked action", context)
			}
		case "provision_reference":
			if !seenBlocked[ActionApply] && !seenBlocked[ActionScale] {
				return fmt.Errorf("%s: provisioning step must reference Apply or Scale as a blocked action", context)
			}
		case "rollback_reference":
			if !seenBlocked[ActionRestart] && !seenBlocked[ActionStop] && !seenBlocked[ActionReset] && !seenBlocked[ActionDelete] {
				return fmt.Errorf("%s: rollback step must reference a blocked restoration action", context)
			}
		}
	case StepModeReadOnly:
		if step.ExecutionBlockedHere {
			return fmt.Errorf("%s: read-only step cannot be execution_blocked_here", context)
		}
		if len(step.BlockedActions) != 0 {
			return fmt.Errorf("%s: read-only step cannot carry blocked actions", context)
		}
		if step.BlockedReason != "" {
			return fmt.Errorf("%s: read-only step cannot carry a blocked_reason", context)
		}
		if step.RequiredAuthorization != "" {
			return fmt.Errorf("%s: read-only step cannot carry required_authorization", context)
		}
		if step.Marker != ReadOnlyStepMarker {
			return fmt.Errorf("%s: read-only marker must be %q", context, ReadOnlyStepMarker)
		}
	default:
		return fmt.Errorf("%s: unknown mode %q", context, spec.Mode)
	}
	return nil
}

func validateExpectedServices(services []ExpectedService) error {
	if len(services) == 0 {
		return errors.New("execution plan: expected_services must be a non-empty explicit array")
	}
	seen := map[string]bool{}
	for i, svc := range services {
		context := fmt.Sprintf("execution plan expected service %d (%s)", i, svc.Service)
		if svc.Service == "" || strings.TrimSpace(svc.Service) != svc.Service {
			return fmt.Errorf("%s: service name is required", context)
		}
		if seen[svc.Service] {
			return fmt.Errorf("%s: duplicate service", context)
		}
		seen[svc.Service] = true
		if svc.Tier != Application && svc.Tier != Supporting {
			return fmt.Errorf("%s: tier %q must be application or supporting", context, svc.Tier)
		}
		if svc.Replicas <= 0 {
			return fmt.Errorf("%s: replicas must be positive", context)
		}
		switch svc.Lifecycle {
		case LifecyclePersistent:
		case LifecycleOneShot:
			if svc.Replicas != 1 {
				return fmt.Errorf("%s: one_shot service must declare exactly one replica", context)
			}
		default:
			return fmt.Errorf("%s: lifecycle %q is not persistent or one_shot", context, svc.Lifecycle)
		}
		switch svc.Activation {
		case ActivationDefaultStart, ActivationOptionalProfile:
		default:
			return fmt.Errorf("%s: activation %q is not default_start or optional_profile", context, svc.Activation)
		}
		switch svc.Healthcheck {
		case HealthcheckDeclared, HealthcheckNone:
		default:
			return fmt.Errorf("%s: healthcheck %q is not declared or none", context, svc.Healthcheck)
		}
		want := healthModesFor(ServiceExpectation{
			Service: svc.Service, Replicas: svc.Replicas, Lifecycle: svc.Lifecycle,
			Activation: svc.Activation, Healthcheck: svc.Healthcheck,
		})
		if !sameStringSet(svc.HealthModes, want) {
			return fmt.Errorf("%s: health_modes %v do not match the derived modes %v", context, svc.HealthModes, want)
		}
		for _, mode := range svc.HealthModes {
			if !allowedHealthModes[mode] {
				return fmt.Errorf("%s: unknown health mode %q", context, mode)
			}
		}
		if i > 0 {
			prev := services[i-1]
			if tierRank[prev.Tier] > tierRank[svc.Tier] || (prev.Tier == svc.Tier && prev.Service > svc.Service) {
				return fmt.Errorf("%s: expected services must be sorted by tier then service", context)
			}
		}
	}
	return nil
}

func validateResourceGuard(guard ResourceGuard) error {
	if guard.Scope != ResourceGuardScope {
		return fmt.Errorf("execution plan: resource_guard.scope must be %q", ResourceGuardScope)
	}
	declared := guard.Declared
	if err := validateSortedCPUSets(declared.ApplicationCPUSets, "application_cpusets"); err != nil {
		return err
	}
	if err := validateSortedCPUSets(declared.SupportingCPUSets, "supporting_cpusets"); err != nil {
		return err
	}
	if len(declared.Services) == 0 {
		return errors.New("execution plan: resource_guard.declared.services must be non-empty")
	}
	seen := map[string]bool{}
	var appServices, appReplicas, supportServices, supportReplicas int
	var appMemory, supportMemory int64
	for i, svc := range declared.Services {
		context := fmt.Sprintf("execution plan resource guard service %d (%s)", i, svc.Service)
		if svc.Service == "" {
			return fmt.Errorf("%s: service name is required", context)
		}
		if seen[svc.Service] {
			return fmt.Errorf("%s: duplicate service", context)
		}
		seen[svc.Service] = true
		if svc.Class != Application && svc.Class != Supporting {
			return fmt.Errorf("%s: class %q must be application or supporting", context, svc.Class)
		}
		if svc.Replicas <= 0 {
			return fmt.Errorf("%s: replicas must be positive", context)
		}
		if !validCPUSet(svc.CPUSet) {
			return fmt.Errorf("%s: invalid cpuset %q", context, svc.CPUSet)
		}
		allowed := declared.ApplicationCPUSets
		if svc.Class == Supporting {
			allowed = declared.SupportingCPUSets
		}
		if !cpusetSubset(svc.CPUSet, allowed) {
			return fmt.Errorf("%s: cpuset %q is not declared for class %s", context, svc.CPUSet, svc.Class)
		}
		total, err := declaredMemoryTotal(svc.MemoryBytes, svc.Replicas)
		if err != nil {
			return fmt.Errorf("%s: %w", context, err)
		}
		if svc.DeclaredTotalMemoryBytes != total {
			return fmt.Errorf("%s: declared_total_memory_bytes %d does not equal memory_bytes*replicas %d", context, svc.DeclaredTotalMemoryBytes, total)
		}
		switch svc.Class {
		case Application:
			appServices++
			appReplicas += svc.Replicas
			appMemory += total
		case Supporting:
			supportServices++
			supportReplicas += svc.Replicas
			supportMemory += total
		}
		if i > 0 {
			prev := declared.Services[i-1]
			if tierRank[prev.Class] > tierRank[svc.Class] || (prev.Class == svc.Class && prev.Service > svc.Service) {
				return fmt.Errorf("%s: declared services must be sorted by class then service", context)
			}
		}
	}
	if declared.ApplicationServices != appServices || declared.ApplicationReplicas != appReplicas || declared.ApplicationMemoryBytes != appMemory {
		return errors.New("execution plan: application resource guard totals do not match the declared services")
	}
	if declared.SupportingServices != supportServices || declared.SupportingReplicas != supportReplicas || declared.SupportingMemoryBytes != supportMemory {
		return errors.New("execution plan: supporting resource guard totals do not match the declared services")
	}
	if declared.TotalServices != appServices+supportServices || declared.TotalReplicas != appReplicas+supportReplicas || declared.TotalMemoryBytes != appMemory+supportMemory {
		return errors.New("execution plan: total resource guard values do not match the declared services")
	}
	if guard.Observed.Status != NotAssessed {
		return fmt.Errorf("execution plan: resource_guard.observed.status must be %s, got %s", NotAssessed, guard.Observed.Status)
	}
	if guard.Observed.Source != ResourceObservedSource {
		return fmt.Errorf("execution plan: resource_guard.observed.source must be %q", ResourceObservedSource)
	}
	if strings.TrimSpace(guard.Observed.Reason) == "" {
		return errors.New("execution plan: resource_guard.observed.reason is required")
	}
	return nil
}

func validateSortedCPUSets(values []string, label string) error {
	if len(values) == 0 {
		return fmt.Errorf("execution plan: resource_guard.declared.%s must be non-empty", label)
	}
	seen := map[string]bool{}
	for i, value := range values {
		if !validCPUSet(value) {
			return fmt.Errorf("execution plan: resource_guard.declared.%s contains invalid cpuset %q", label, value)
		}
		if seen[value] {
			return fmt.Errorf("execution plan: resource_guard.declared.%s contains duplicate cpuset %q", label, value)
		}
		seen[value] = true
		if i > 0 && values[i-1] > value {
			return fmt.Errorf("execution plan: resource_guard.declared.%s must be sorted", label)
		}
	}
	return nil
}

func validateEvidenceChecklist(checklist EvidenceChecklist) error {
	if checklist.PackagingInterpretation != ArtifactManifestInterpretation {
		return fmt.Errorf("execution plan: evidence_checklist.packaging_interpretation must be %q", ArtifactManifestInterpretation)
	}
	if checklist.ChecksumAlgorithm != EvidenceChecksumAlgorithm {
		return fmt.Errorf("execution plan: evidence_checklist.checksum_algorithm must be %q", EvidenceChecksumAlgorithm)
	}
	if checklist.RunRootTemplate != ArtifactRunDirectoryPrefix+"{run_id}/" {
		return fmt.Errorf("execution plan: evidence_checklist.run_root_template is not the packaging run root")
	}
	if checklist.ManifestTemplate != ArtifactRunRoot("{run_id}")+"/artifact-manifest.json" {
		return fmt.Errorf("execution plan: evidence_checklist.manifest_template is not the packaging manifest path")
	}
	if checklist.ChecksumsTemplate != ArtifactRunRoot("{run_id}")+"/SHA256SUMS" {
		return fmt.Errorf("execution plan: evidence_checklist.checksums_template is not the packaging checksum path")
	}
	if checklist.PriorManifestVerification != string(PriorManifestVerificationNotImplemented) {
		return fmt.Errorf("execution plan: evidence_checklist.prior_manifest_verification must be %q", PriorManifestVerificationNotImplemented)
	}
	if checklist.PriorManifestBindingStatus != NotAssessed {
		return fmt.Errorf("execution plan: evidence_checklist.prior_manifest_binding_status must be %s", NotAssessed)
	}
	if checklist.LiveValidation != NotAssessed {
		return fmt.Errorf("execution plan: evidence_checklist.live_validation must be %s", NotAssessed)
	}
	if len(checklist.Roles) == 0 {
		return errors.New("execution plan: evidence_checklist.roles must be non-empty")
	}
	seen := map[string]bool{}
	required := 0
	for i, item := range checklist.Roles {
		if !validArtifactRoles[item.Role] {
			return fmt.Errorf("execution plan: evidence role %q is not in the packaging artifact taxonomy", item.Role)
		}
		if seen[item.Role] {
			return fmt.Errorf("execution plan: duplicate evidence role %q", item.Role)
		}
		seen[item.Role] = true
		if item.Required {
			required++
		}
		if item.Status != NotAssessed {
			return fmt.Errorf("execution plan: evidence role %q status must be %s, got %s", item.Role, NotAssessed, item.Status)
		}
		if strings.TrimSpace(item.Reason) == "" {
			return fmt.Errorf("execution plan: evidence role %q reason is required", item.Role)
		}
		if i > 0 && checklist.Roles[i-1].Role > item.Role {
			return errors.New("execution plan: evidence_checklist.roles must be sorted by role")
		}
	}
	if required == 0 {
		return errors.New("execution plan: at least one evidence role must be required")
	}
	if !equalStrings(checklist.RoleTaxonomy, sortedArtifactRoles()) {
		return errors.New("execution plan: evidence_checklist.role_taxonomy must be the sorted packaging artifact taxonomy")
	}
	return nil
}
