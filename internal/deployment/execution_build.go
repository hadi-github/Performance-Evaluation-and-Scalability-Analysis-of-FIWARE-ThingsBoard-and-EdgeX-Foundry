// Decoding and derivation of the M2d render-only execution plan.
package deployment

import (
	"fmt"
	"sort"
)

// DecodeExecutionPlan strictly decodes an M2d execution plan using the same
// exact-key, duplicate-key, null and trailing-JSON rules as the approved M2/M2b
// documents, then validates the blocked-execution invariants.
func DecodeExecutionPlan(b []byte) (*ExecutionPlan, error) {
	if err := validateRawDocument(b, executionPlanShape, "execution plan"); err != nil {
		return nil, err
	}
	var p ExecutionPlan
	if err := decodeStrict(b, &p); err != nil {
		return nil, fmt.Errorf("execution plan: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// BuildExecutionPlan derives a render-only runbook from the approved descriptor,
// caller-supplied inventory and orchestration v2 plan. It is pure and offline:
// it reads no file and contacts no service, Docker daemon or SUT.
func BuildExecutionPlan(descriptorRaw, inventoryRaw, planRaw []byte) (*ExecutionPlan, error) {
	descriptor, err := DecodeDescriptor(descriptorRaw)
	if err != nil {
		return nil, err
	}
	inventory, err := DecodeInventory(inventoryRaw)
	if err != nil {
		return nil, err
	}
	plan, err := DecodeOrchestrationPlan(planRaw)
	if err != nil {
		return nil, err
	}
	blueprint, err := blueprintForPlatform(plan, descriptor.Platform)
	if err != nil {
		return nil, err
	}
	if err := validatePlanInputs(descriptor, inventory, blueprint); err != nil {
		return nil, err
	}
	steps, err := buildExecutionSteps(plan)
	if err != nil {
		return nil, err
	}
	guard, err := buildResourceGuard(descriptor)
	if err != nil {
		return nil, err
	}
	checklist, err := buildEvidenceChecklist(plan, blueprint)
	if err != nil {
		return nil, err
	}
	descCanonical, err := descriptor.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	invCanonical, err := inventory.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	planCanonical, err := plan.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	stageOrder := make([]StageName, len(plan.Stages))
	for i, stage := range plan.Stages {
		stageOrder[i] = stage.Stage
	}
	result := &ExecutionPlan{
		SchemaVersion:      ExecutionPlanSchemaVersion,
		Interpretation:     ExecutionPlanInterpretation,
		Mode:               ExecutionPlanMode,
		Authorization:      plan.Authorization,
		ExecutionBlocked:   true,
		Platform:           descriptor.Platform,
		Project:            descriptor.Project,
		PlanSchemaVersion:  plan.SchemaVersion,
		PlanInterpretation: plan.Interpretation,
		PlanStageOrder:     stageOrder,
		Inputs: ExecutionInputs{
			DescriptorRawSHA256:              SHA256Bytes(descriptorRaw),
			DescriptorCanonicalSHA256:        descCanonical,
			InventoryRawSHA256:               SHA256Bytes(inventoryRaw),
			InventoryCanonicalSHA256:         invCanonical,
			OrchestrationPlanRawSHA256:       SHA256Bytes(planRaw),
			OrchestrationPlanCanonicalSHA256: planCanonical,
		},
		Steps:             steps,
		ExpectedServices:  buildExpectedServices(blueprint),
		ResourceGuard:     guard,
		EvidenceChecklist: checklist,
		Limitations: []string{
			"Render-only: no Docker client call, SUT access, provisioning, apply/up/down, scale, restart, stop, fault, load, reset or delete is performed.",
			"Every execution-bearing row is permanently marked EXECUTION_BLOCKED_HERE and NOT EXECUTED; no executor or lifecycle method exists in this package.",
			"Resource guard values are descriptor declarations, not live measurements. The observed resource state is NOT_ASSESSED and no live inventory was collected.",
			"Evidence checklist rows are packaging-specification declarations only. No artifact was read or verified; prior-manifest binding is NOT_IMPLEMENTED and live validation is NOT_ASSESSED.",
			"Operation identifiers are structured references, not runnable commands. No platform command, working-directory path or environment probe is included.",
		},
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

func blueprintForPlatform(plan *OrchestrationPlan, platform string) (*PlatformBlueprint, error) {
	for i := range plan.Platforms {
		if plan.Platforms[i].Platform == platform {
			return &plan.Platforms[i], nil
		}
	}
	return nil, fmt.Errorf("execution plan: platform %q is not declared by the orchestration plan", platform)
}

func validatePlanInputs(descriptor *Descriptor, inventory *Inventory, blueprint *PlatformBlueprint) error {
	if blueprint.Platform != descriptor.Platform {
		return fmt.Errorf("execution plan: descriptor platform %q does not match orchestration platform %q", descriptor.Platform, blueprint.Platform)
	}
	if blueprint.Project != descriptor.Project {
		return fmt.Errorf("execution plan: descriptor project %q does not match orchestration project %q", descriptor.Project, blueprint.Project)
	}
	if inventory.Project != descriptor.Project {
		return fmt.Errorf("execution plan: inventory project %q does not match descriptor project %q", inventory.Project, descriptor.Project)
	}
	declared := map[string]ServiceClass{}
	for _, svc := range blueprint.ApplicationServices {
		declared[svc.Service] = Application
	}
	for _, svc := range blueprint.SupportingServices {
		declared[svc.Service] = Supporting
	}
	inDescriptor := map[string]bool{}
	for _, svc := range descriptor.Services {
		class, ok := declared[svc.Service]
		if !ok {
			return fmt.Errorf("execution plan: descriptor service %q is not declared by orchestration platform %q", svc.Service, blueprint.Platform)
		}
		if svc.Class != class {
			return fmt.Errorf("execution plan: descriptor service %q class %q does not match orchestration tier %q", svc.Service, svc.Class, class)
		}
		inDescriptor[svc.Service] = true
	}
	for _, svc := range inventory.Services {
		if _, ok := declared[svc.Service]; !ok {
			return fmt.Errorf("execution plan: inventory service %q is ambiguous; it is not declared by orchestration platform %q", svc.Service, blueprint.Platform)
		}
	}
	for _, svc := range blueprint.ApplicationServices {
		if svc.Activation == ActivationDefaultStart && !inDescriptor[svc.Service] {
			return fmt.Errorf("execution plan: descriptor must declare default-start service %q for resource guard completeness", svc.Service)
		}
	}
	for _, svc := range blueprint.SupportingServices {
		if svc.Activation == ActivationDefaultStart && !inDescriptor[svc.Service] {
			return fmt.Errorf("execution plan: descriptor must declare default-start service %q for resource guard completeness", svc.Service)
		}
	}
	return nil
}

func buildExecutionSteps(plan *OrchestrationPlan) ([]ExecutionStep, error) {
	stages := map[StageName]OrchestrationStage{}
	for _, stage := range plan.Stages {
		stages[stage.Stage] = stage
	}
	steps := make([]ExecutionStep, 0, len(executionStepSpecs))
	for i, spec := range executionStepSpecs {
		stage, ok := stages[spec.Stage]
		if !ok {
			return nil, fmt.Errorf("execution plan: orchestration stage %q is missing", spec.Stage)
		}
		step := ExecutionStep{
			Order:                i + 1,
			Step:                 spec.Step,
			PlanStage:            spec.Stage,
			Purpose:              stage.Purpose,
			Mode:                 spec.Mode,
			PermissibleActions:   append([]Action(nil), stage.PermissibleActions...),
			BlockedActions:       append([]Action(nil), stage.BlockedActions...),
			ExecutionBlockedHere: stage.ExecutionBlockedHere,
			OperationIDs:         append([]string(nil), spec.Operations...),
			ExpectedEvidence:     append([]string(nil), stage.ExpectedEvidence...),
		}
		sortActions(step.PermissibleActions)
		sortActions(step.BlockedActions)
		sort.Strings(step.OperationIDs)
		sort.Strings(step.ExpectedEvidence)
		if spec.Mode == StepModeExecutionBlocked {
			if !step.ExecutionBlockedHere || len(step.BlockedActions) == 0 {
				return nil, fmt.Errorf("execution plan: step %q requires a blocked orchestration stage", spec.Step)
			}
			step.Marker = ExecutionBlockedMarker
			step.ExecutionStatus = ExecutionStatusNotExecuted
			step.BlockedReason = spec.BlockedReason
			step.RequiredAuthorization = plan.Authorization
		} else {
			if step.ExecutionBlockedHere || len(step.BlockedActions) != 0 {
				return nil, fmt.Errorf("execution plan: read-only step %q references a blocked stage or action", spec.Step)
			}
			step.Marker = ReadOnlyStepMarker
			step.ExecutionStatus = ExecutionStatusNotExecuted
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func buildExpectedServices(blueprint *PlatformBlueprint) []ExpectedService {
	out := make([]ExpectedService, 0, len(blueprint.ApplicationServices)+len(blueprint.SupportingServices))
	add := func(svc ServiceExpectation, tier ServiceClass) {
		out = append(out, ExpectedService{
			Service:     svc.Service,
			Tier:        tier,
			Replicas:    svc.Replicas,
			Lifecycle:   svc.Lifecycle,
			Activation:  svc.Activation,
			Healthcheck: svc.Healthcheck,
			HealthModes: healthModesFor(svc),
		})
	}
	for _, svc := range blueprint.ApplicationServices {
		add(svc, Application)
	}
	for _, svc := range blueprint.SupportingServices {
		add(svc, Supporting)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tier != out[j].Tier {
			return tierRank[out[i].Tier] < tierRank[out[j].Tier]
		}
		return out[i].Service < out[j].Service
	})
	for i := range out {
		sort.Strings(out[i].HealthModes)
	}
	return out
}

func healthModesFor(svc ServiceExpectation) []string {
	modes := make([]string, 0, 3)
	if svc.Activation == ActivationOptionalProfile {
		modes = append(modes, "optional_profile_not_assessed")
	} else {
		modes = append(modes, "default_start")
	}
	if svc.Lifecycle == LifecycleOneShot {
		modes = append(modes, "one_shot_exit_code_zero")
	} else {
		modes = append(modes, "persistent_running")
	}
	if svc.Healthcheck == HealthcheckDeclared {
		modes = append(modes, "explicit_healthcheck_healthy")
	} else {
		modes = append(modes, "healthcheck_none_functional_evidence")
	}
	return modes
}

func buildResourceGuard(descriptor *Descriptor) (ResourceGuard, error) {
	canonical := descriptor.Canonical()
	declared := ResourceGuardDeclared{
		ApplicationCPUSets: append([]string(nil), canonical.Allocation.ApplicationCPUSets...),
		SupportingCPUSets:  append([]string(nil), canonical.Allocation.SupportingCPUSets...),
	}
	sort.Strings(declared.ApplicationCPUSets)
	sort.Strings(declared.SupportingCPUSets)
	for _, svc := range canonical.Services {
		total, err := declaredMemoryTotal(svc.MemoryBytes, svc.Replicas)
		if err != nil {
			return ResourceGuard{}, err
		}
		resource := ResourceGuardService{
			Service:                  svc.Service,
			Class:                    svc.Class,
			Replicas:                 svc.Replicas,
			CPUSet:                   svc.CPUSet,
			MemoryBytes:              svc.MemoryBytes,
			DeclaredTotalMemoryBytes: total,
		}
		declared.Services = append(declared.Services, resource)
		declared.TotalServices++
		declared.TotalReplicas += svc.Replicas
		declared.TotalMemoryBytes += total
		switch svc.Class {
		case Application:
			declared.ApplicationServices++
			declared.ApplicationReplicas += svc.Replicas
			declared.ApplicationMemoryBytes += total
		case Supporting:
			declared.SupportingServices++
			declared.SupportingReplicas += svc.Replicas
			declared.SupportingMemoryBytes += total
		default:
			return ResourceGuard{}, fmt.Errorf("execution plan: descriptor service %q has unknown class %q", svc.Service, svc.Class)
		}
	}
	sort.SliceStable(declared.Services, func(i, j int) bool {
		if declared.Services[i].Class != declared.Services[j].Class {
			return tierRank[declared.Services[i].Class] < tierRank[declared.Services[j].Class]
		}
		return declared.Services[i].Service < declared.Services[j].Service
	})
	return ResourceGuard{
		Scope:    ResourceGuardScope,
		Declared: declared,
		Observed: ResourceObserved{
			Status: NotAssessed,
			Source: ResourceObservedSource,
			Reason: "Resource guard uses descriptor declarations only; no live inventory, Docker call, SUT probe or measurement was performed. A missing observation is NOT_ASSESSED, never a zero.",
		},
	}, nil
}

func declaredMemoryTotal(memory int64, replicas int) (int64, error) {
	if memory <= 0 || replicas <= 0 {
		return 0, fmt.Errorf("execution plan: memory_bytes and replicas must be positive")
	}
	if memory > (1<<62)/int64(replicas) {
		return 0, fmt.Errorf("execution plan: declared memory total overflows int64")
	}
	return memory * int64(replicas), nil
}

func buildEvidenceChecklist(plan *OrchestrationPlan, blueprint *PlatformBlueprint) (EvidenceChecklist, error) {
	roles := map[string]bool{}
	for _, role := range blueprint.AllowedEvidenceRoles {
		roles[role] = true
	}
	for _, stage := range plan.Stages {
		for _, role := range stage.ExpectedEvidence {
			roles[role] = true
		}
	}
	required := map[string]bool{}
	for _, role := range blueprint.RequiredAttestationRoles {
		if !roles[role] {
			return EvidenceChecklist{}, fmt.Errorf("execution plan: required attestation role %q is absent from the evidence checklist", role)
		}
		required[role] = true
	}
	names := make([]string, 0, len(roles))
	for role := range roles {
		if !validArtifactRoles[role] {
			return EvidenceChecklist{}, fmt.Errorf("execution plan: evidence role %q is not in the packaging artifact taxonomy", role)
		}
		names = append(names, role)
	}
	sort.Strings(names)
	items := make([]EvidenceChecklistItem, 0, len(names))
	for _, role := range names {
		reason := "Optional/allowed evidence role from the orchestration plan; this offline checklist did not read, verify, copy or write the artifact."
		if required[role] {
			reason = "Required attestation role from the orchestration plan; this offline checklist did not read, verify, copy or write the artifact."
		}
		items = append(items, EvidenceChecklistItem{
			Role:     role,
			Required: required[role],
			Status:   NotAssessed,
			Reason:   reason,
		})
	}
	return EvidenceChecklist{
		PackagingInterpretation:    ArtifactManifestInterpretation,
		ChecksumAlgorithm:          EvidenceChecksumAlgorithm,
		RunRootTemplate:            ArtifactRunDirectoryPrefix + "{run_id}/",
		ManifestTemplate:           ArtifactRunRoot("{run_id}") + "/artifact-manifest.json",
		ChecksumsTemplate:          ArtifactRunRoot("{run_id}") + "/SHA256SUMS",
		PriorManifestVerification:  string(PriorManifestVerificationNotImplemented),
		PriorManifestBindingStatus: NotAssessed,
		LiveValidation:             NotAssessed,
		Roles:                      items,
		RoleTaxonomy:               sortedArtifactRoles(),
	}, nil
}

func sortedArtifactRoles() []string {
	out := make([]string, 0, len(validArtifactRoles))
	for role := range validArtifactRoles {
		out = append(out, role)
	}
	sort.Strings(out)
	return out
}
