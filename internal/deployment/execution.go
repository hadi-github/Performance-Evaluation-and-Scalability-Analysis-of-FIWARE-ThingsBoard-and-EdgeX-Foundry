package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// This file adds the M2d render-only deployment execution plan. It is a pure
// document renderer: it decodes already-approved M2/M2b documents, derives a
// deterministic runbook, and writes JSON/Markdown only into a new output
// directory. It exposes no executor, lifecycle method, Docker client, SUT
// access, shell invocation or command execution path.

const (
	// ExecutionPlanSchemaVersion versions the M2d execution plan.
	ExecutionPlanSchemaVersion = 1
	// ExecutionPlanInterpretation names the exact render-only runbook semantics.
	ExecutionPlanInterpretation = "deployment-execution-plan-v1"
	// ExecutionPlanMode is the only admitted mode. The plan describes what a
	// future separately authorized execution would do; it never does it.
	ExecutionPlanMode = "offline-render-only"
	// ExecutionStatusNotExecuted is the only admitted step status. Even a
	// read-only row is a rendered document row, not a deployment action.
	ExecutionStatusNotExecuted ExecutionStatus = "NOT EXECUTED"
	// ReadOnlyStepMarker marks a benign in-process read/validate/render row.
	ReadOnlyStepMarker = "NOT EXECUTED"
	// ResourceGuardScope states that resource values are descriptor declarations,
	// not live measurements or an observed inventory.
	ResourceGuardScope = "descriptor_declaration_not_live_inventory"
	// ResourceObservedSource is the explicit absence of a live resource observation.
	ResourceObservedSource = "no_live_observation"
	// EvidenceChecksumAlgorithm is the packaging specification algorithm.
	EvidenceChecksumAlgorithm = "SHA-256"
)

// ExecutionStatus is the explicit runbook execution state. M2d admits only
// NOT EXECUTED: an execution-bearing row is blocked, and a read-only row is
// rendered in-process without external I/O beyond the declared inputs/output.
type ExecutionStatus string

// ExecutionStepMode distinguishes a benign in-process render row from an
// execution-bearing row that must remain blocked.
type ExecutionStepMode string

const (
	// StepModeReadOnly is a pure read/validate/render/attest row. It is not an
	// execution capability.
	StepModeReadOnly ExecutionStepMode = "read_only_render"
	// StepModeExecutionBlocked is a row that would change platform state in a
	// future authorization. M2d renders it only and never admits it.
	StepModeExecutionBlocked ExecutionStepMode = "execution_blocked"
)

// operationIDPattern admits structured operation identifiers, not commands.
// M2d deliberately emits no runnable platform command strings.
var operationIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)

// ExecutionInputs records the content identity of the documents that produced
// this plan. It records hashes only, never an absolute working-directory path.
type ExecutionInputs struct {
	DescriptorRawSHA256              string `json:"descriptor_raw_sha256"`
	DescriptorCanonicalSHA256        string `json:"descriptor_canonical_sha256"`
	InventoryRawSHA256               string `json:"inventory_raw_sha256"`
	InventoryCanonicalSHA256         string `json:"inventory_canonical_sha256"`
	OrchestrationPlanRawSHA256       string `json:"orchestration_plan_raw_sha256"`
	OrchestrationPlanCanonicalSHA256 string `json:"orchestration_plan_canonical_sha256"`
}

// ExecutionStep is one ordered runbook row. PlanStage references the approved
// orchestration v2 stage; the step's actions and evidence roles are copied from
// that stage rather than re-declared.
type ExecutionStep struct {
	Order                 int               `json:"order"`
	Step                  string            `json:"step"`
	PlanStage             StageName         `json:"plan_stage"`
	Purpose               string            `json:"purpose"`
	Mode                  ExecutionStepMode `json:"mode"`
	PermissibleActions    []Action          `json:"permissible_actions"`
	BlockedActions        []Action          `json:"blocked_actions,omitempty"`
	ExecutionBlockedHere  bool              `json:"execution_blocked_here"`
	Marker                string            `json:"marker"`
	ExecutionStatus       ExecutionStatus   `json:"execution_status"`
	BlockedReason         string            `json:"blocked_reason,omitempty"`
	RequiredAuthorization string            `json:"required_authorization,omitempty"`
	OperationIDs          []string          `json:"operation_ids"`
	ExpectedEvidence      []string          `json:"expected_evidence,omitempty"`
}

// ExpectedService is derived from the orchestration v2 platform blueprint. Tier,
// replicas, lifecycle, activation and healthcheck are the plan's declarations.
// HealthModes is a deterministic semantic expansion, not a live observation.
type ExpectedService struct {
	Service     string                 `json:"service"`
	Tier        ServiceClass           `json:"tier"`
	Replicas    int                    `json:"replicas"`
	Lifecycle   ServiceLifecycle       `json:"lifecycle"`
	Activation  ServiceActivation      `json:"activation"`
	Healthcheck HealthcheckDeclaration `json:"healthcheck"`
	HealthModes []string               `json:"health_modes"`
}

// ResourceGuardService is one descriptor-declared service resource.
type ResourceGuardService struct {
	Service                  string       `json:"service"`
	Class                    ServiceClass `json:"class"`
	Replicas                 int          `json:"replicas"`
	CPUSet                   string       `json:"cpuset"`
	MemoryBytes              int64        `json:"memory_bytes"`
	DeclaredTotalMemoryBytes int64        `json:"declared_total_memory_bytes"`
}

// ResourceGuardDeclared is a descriptor declaration summary. Memory totals are
// memory_bytes * declared replicas, not measured usage.
type ResourceGuardDeclared struct {
	ApplicationCPUSets     []string               `json:"application_cpusets"`
	SupportingCPUSets      []string               `json:"supporting_cpusets"`
	ApplicationServices    int                    `json:"application_services"`
	ApplicationReplicas    int                    `json:"application_replicas"`
	ApplicationMemoryBytes int64                  `json:"application_memory_bytes"`
	SupportingServices     int                    `json:"supporting_services"`
	SupportingReplicas     int                    `json:"supporting_replicas"`
	SupportingMemoryBytes  int64                  `json:"supporting_memory_bytes"`
	TotalServices          int                    `json:"total_services"`
	TotalReplicas          int                    `json:"total_replicas"`
	TotalMemoryBytes       int64                  `json:"total_memory_bytes"`
	Services               []ResourceGuardService `json:"services"`
}

// ResourceObserved is the explicit absence of a live resource measurement. It
// carries no fabricated value and is never a successful zero.
type ResourceObserved struct {
	Status Status `json:"status"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// ResourceGuard separates descriptor declarations from the not-assessed live
// observation.
type ResourceGuard struct {
	Scope    string                `json:"scope"`
	Declared ResourceGuardDeclared `json:"declared"`
	Observed ResourceObserved      `json:"observed"`
}

// EvidenceChecklistItem is one packaging-spec role. It is a checklist row only;
// no artifact was read, verified, copied or written.
type EvidenceChecklistItem struct {
	Role     string `json:"role"`
	Required bool   `json:"required"`
	Status   Status `json:"status"`
	Reason   string `json:"reason"`
}

// EvidenceChecklist records the packaging specification metadata and the
// explicit non-implementation of prior-manifest binding.
type EvidenceChecklist struct {
	PackagingInterpretation    string                  `json:"packaging_interpretation"`
	ChecksumAlgorithm          string                  `json:"checksum_algorithm"`
	RunRootTemplate            string                  `json:"run_root_template"`
	ManifestTemplate           string                  `json:"manifest_template"`
	ChecksumsTemplate          string                  `json:"checksums_template"`
	PriorManifestVerification  string                  `json:"prior_manifest_verification"`
	PriorManifestBindingStatus Status                  `json:"prior_manifest_binding_status"`
	LiveValidation             Status                  `json:"live_validation"`
	Roles                      []EvidenceChecklistItem `json:"roles"`
	RoleTaxonomy               []string                `json:"role_taxonomy"`
}

// ExecutionPlan is the strict M2d render-only runbook. ExecutionBlocked must be
// true: the schema has no representation for an executable plan.
type ExecutionPlan struct {
	SchemaVersion      int               `json:"schema_version"`
	Interpretation     string            `json:"interpretation"`
	Mode               string            `json:"mode"`
	Authorization      string            `json:"authorization"`
	ExecutionBlocked   bool              `json:"execution_blocked"`
	Platform           string            `json:"platform"`
	Project            string            `json:"project"`
	PlanSchemaVersion  int               `json:"plan_schema_version"`
	PlanInterpretation string            `json:"plan_interpretation"`
	PlanStageOrder     []StageName       `json:"plan_stage_order"`
	Inputs             ExecutionInputs   `json:"inputs"`
	Steps              []ExecutionStep   `json:"steps"`
	ExpectedServices   []ExpectedService `json:"expected_services"`
	ResourceGuard      ResourceGuard     `json:"resource_guard"`
	EvidenceChecklist  EvidenceChecklist `json:"evidence_checklist"`
	Limitations        []string          `json:"limitations"`
}

// executionStepSpec fixes the runbook order and stage mapping. Service roles,
// health modes, actions and evidence are NOT duplicated here: they come from the
// orchestration v2 plan.
type executionStepSpec struct {
	Step          string
	Stage         StageName
	Mode          ExecutionStepMode
	Operations    []string
	BlockedReason string
}

var executionStepSpecs = []executionStepSpec{
	{
		Step:  "preflight_validate",
		Stage: StagePreDeployInventory,
		Mode:  StepModeReadOnly,
		Operations: []string{
			"descriptor.validate",
			"inventory.validate",
			"input.hash",
			"orchestration_plan.validate",
		},
	},
	{
		Step:  "compose_render_guard_reference",
		Stage: StageDryRunRender,
		Mode:  StepModeReadOnly,
		Operations: []string{
			"compose.render_dry_run_reference",
			"compose.resource_guard_reference",
		},
	},
	{
		Step:  "provision_reference",
		Stage: StageProvisionOnSUT,
		Mode:  StepModeExecutionBlocked,
		Operations: []string{
			"orchestration.provision_reference",
			"orchestration.replica_declaration_reference",
		},
		BlockedReason: "Provisioning would create or change SUT resources; this render-only plan does not provision.",
	},
	{
		Step:  "start_apply_reference",
		Stage: StageApply,
		Mode:  StepModeExecutionBlocked,
		Operations: []string{
			"orchestration.apply_reference",
		},
		BlockedReason: "Apply would create or update a deployment; this render-only plan does not apply, scale, restart or stop.",
	},
	{
		Step:  "health_validate",
		Stage: StageValidateHealth,
		Mode:  StepModeReadOnly,
		Operations: []string{
			"health.attestation_checklist",
			"inventory.observation_reference",
		},
	},
	{
		Step:  "evidence_package",
		Stage: StageEvidence,
		Mode:  StepModeReadOnly,
		Operations: []string{
			"evidence.artifact_checklist",
			"evidence.checksum_manifest_reference",
		},
	},
	{
		Step:  "rollback_reference",
		Stage: StageRollback,
		Mode:  StepModeExecutionBlocked,
		Operations: []string{
			"orchestration.rollback_reference",
			"restoration.rules_reference",
		},
		BlockedReason: "Rollback would restart, stop, reset or delete state; this render-only plan executes no restoration.",
	},
}

var (
	allowedHealthModes = map[string]bool{
		"default_start":                        true,
		"optional_profile_not_assessed":        true,
		"persistent_running":                   true,
		"one_shot_exit_code_zero":              true,
		"explicit_healthcheck_healthy":         true,
		"healthcheck_none_functional_evidence": true,
	}
	tierRank = map[ServiceClass]int{Application: 0, Supporting: 1}
)

var executionPlanShape = jsonShape{
	object: map[string]jsonShape{
		"schema_version":      scalarValueShape,
		"interpretation":      scalarValueShape,
		"mode":                scalarValueShape,
		"authorization":       scalarValueShape,
		"execution_blocked":   scalarValueShape,
		"platform":            scalarValueShape,
		"project":             scalarValueShape,
		"plan_schema_version": scalarValueShape,
		"plan_interpretation": scalarValueShape,
		"plan_stage_order":    {element: &scalarValueShape},
		"inputs": {
			object: map[string]jsonShape{
				"descriptor_raw_sha256":               scalarValueShape,
				"descriptor_canonical_sha256":         scalarValueShape,
				"inventory_raw_sha256":                scalarValueShape,
				"inventory_canonical_sha256":          scalarValueShape,
				"orchestration_plan_raw_sha256":       scalarValueShape,
				"orchestration_plan_canonical_sha256": scalarValueShape,
			},
		},
		"steps": {element: &jsonShape{
			object: map[string]jsonShape{
				"order":                  scalarValueShape,
				"step":                   scalarValueShape,
				"plan_stage":             scalarValueShape,
				"purpose":                scalarValueShape,
				"mode":                   scalarValueShape,
				"permissible_actions":    {element: &scalarValueShape},
				"blocked_actions":        {element: &scalarValueShape},
				"execution_blocked_here": scalarValueShape,
				"marker":                 scalarValueShape,
				"execution_status":       scalarValueShape,
				"blocked_reason":         scalarValueShape,
				"required_authorization": scalarValueShape,
				"operation_ids":          {element: &scalarValueShape},
				"expected_evidence":      {element: &scalarValueShape},
			},
			optional: map[string]bool{
				"blocked_actions": true, "blocked_reason": true,
				"required_authorization": true, "expected_evidence": true,
			},
		}},
		"expected_services": {element: &jsonShape{
			object: map[string]jsonShape{
				"service":      scalarValueShape,
				"tier":         scalarValueShape,
				"replicas":     scalarValueShape,
				"lifecycle":    scalarValueShape,
				"activation":   scalarValueShape,
				"healthcheck":  scalarValueShape,
				"health_modes": {element: &scalarValueShape},
			},
		}},
		"resource_guard": {
			object: map[string]jsonShape{
				"scope": scalarValueShape,
				"declared": {
					object: map[string]jsonShape{
						"application_cpusets":      {element: &scalarValueShape},
						"supporting_cpusets":       {element: &scalarValueShape},
						"application_services":     scalarValueShape,
						"application_replicas":     scalarValueShape,
						"application_memory_bytes": scalarValueShape,
						"supporting_services":      scalarValueShape,
						"supporting_replicas":      scalarValueShape,
						"supporting_memory_bytes":  scalarValueShape,
						"total_services":           scalarValueShape,
						"total_replicas":           scalarValueShape,
						"total_memory_bytes":       scalarValueShape,
						"services": {element: &jsonShape{
							object: map[string]jsonShape{
								"service":                     scalarValueShape,
								"class":                       scalarValueShape,
								"replicas":                    scalarValueShape,
								"cpuset":                      scalarValueShape,
								"memory_bytes":                scalarValueShape,
								"declared_total_memory_bytes": scalarValueShape,
							},
						}},
					},
				},
				"observed": {
					object: map[string]jsonShape{
						"status": scalarValueShape,
						"source": scalarValueShape,
						"reason": scalarValueShape,
					},
				},
			},
		},
		"evidence_checklist": {
			object: map[string]jsonShape{
				"packaging_interpretation":      scalarValueShape,
				"checksum_algorithm":            scalarValueShape,
				"run_root_template":             scalarValueShape,
				"manifest_template":             scalarValueShape,
				"checksums_template":            scalarValueShape,
				"prior_manifest_verification":   scalarValueShape,
				"prior_manifest_binding_status": scalarValueShape,
				"live_validation":               scalarValueShape,
				"roles": {element: &jsonShape{
					object: map[string]jsonShape{
						"role":     scalarValueShape,
						"required": scalarValueShape,
						"status":   scalarValueShape,
						"reason":   scalarValueShape,
					},
				}},
				"role_taxonomy": {element: &scalarValueShape},
			},
		},
		"limitations": {element: &scalarValueShape},
	},
}

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

// Canonical returns a deep copy with deterministic sorting. Step order is the
// documented runbook order and is never sorted.
func (p ExecutionPlan) Canonical() ExecutionPlan {
	out := p
	out.PlanStageOrder = append([]StageName(nil), p.PlanStageOrder...)
	out.Limitations = append([]string(nil), p.Limitations...)
	out.Steps = append([]ExecutionStep(nil), p.Steps...)
	for i := range out.Steps {
		out.Steps[i].PermissibleActions = append([]Action(nil), p.Steps[i].PermissibleActions...)
		out.Steps[i].BlockedActions = append([]Action(nil), p.Steps[i].BlockedActions...)
		out.Steps[i].OperationIDs = append([]string(nil), p.Steps[i].OperationIDs...)
		out.Steps[i].ExpectedEvidence = append([]string(nil), p.Steps[i].ExpectedEvidence...)
		sortActions(out.Steps[i].PermissibleActions)
		sortActions(out.Steps[i].BlockedActions)
		sort.Strings(out.Steps[i].OperationIDs)
		sort.Strings(out.Steps[i].ExpectedEvidence)
	}
	out.ExpectedServices = append([]ExpectedService(nil), p.ExpectedServices...)
	for i := range out.ExpectedServices {
		out.ExpectedServices[i].HealthModes = append([]string(nil), p.ExpectedServices[i].HealthModes...)
		sort.Strings(out.ExpectedServices[i].HealthModes)
	}
	sort.SliceStable(out.ExpectedServices, func(i, j int) bool {
		if out.ExpectedServices[i].Tier != out.ExpectedServices[j].Tier {
			return tierRank[out.ExpectedServices[i].Tier] < tierRank[out.ExpectedServices[j].Tier]
		}
		return out.ExpectedServices[i].Service < out.ExpectedServices[j].Service
	})
	out.ResourceGuard.Declared.ApplicationCPUSets = append([]string(nil), p.ResourceGuard.Declared.ApplicationCPUSets...)
	out.ResourceGuard.Declared.SupportingCPUSets = append([]string(nil), p.ResourceGuard.Declared.SupportingCPUSets...)
	sort.Strings(out.ResourceGuard.Declared.ApplicationCPUSets)
	sort.Strings(out.ResourceGuard.Declared.SupportingCPUSets)
	out.ResourceGuard.Declared.Services = append([]ResourceGuardService(nil), p.ResourceGuard.Declared.Services...)
	sort.SliceStable(out.ResourceGuard.Declared.Services, func(i, j int) bool {
		if out.ResourceGuard.Declared.Services[i].Class != out.ResourceGuard.Declared.Services[j].Class {
			return tierRank[out.ResourceGuard.Declared.Services[i].Class] < tierRank[out.ResourceGuard.Declared.Services[j].Class]
		}
		return out.ResourceGuard.Declared.Services[i].Service < out.ResourceGuard.Declared.Services[j].Service
	})
	out.EvidenceChecklist.Roles = append([]EvidenceChecklistItem(nil), p.EvidenceChecklist.Roles...)
	sort.SliceStable(out.EvidenceChecklist.Roles, func(i, j int) bool {
		return out.EvidenceChecklist.Roles[i].Role < out.EvidenceChecklist.Roles[j].Role
	})
	out.EvidenceChecklist.RoleTaxonomy = append([]string(nil), p.EvidenceChecklist.RoleTaxonomy...)
	sort.Strings(out.EvidenceChecklist.RoleTaxonomy)
	return out
}

// CanonicalJSON is the deterministic compact representation used for semantic
// hashing and for the canonical output file.
func (p ExecutionPlan) CanonicalJSON() ([]byte, error) {
	return json.Marshal(p.Canonical())
}

// CanonicalSHA256 is the semantic execution-plan hash.
func (p ExecutionPlan) CanonicalSHA256() (string, error) {
	b, err := p.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

// MarshalExecutionPlan returns deterministic pretty JSON with a trailing
// newline. It adds no timestamp and no live claim.
func MarshalExecutionPlan(p *ExecutionPlan) ([]byte, error) {
	if p == nil {
		return nil, errors.New("nil execution plan")
	}
	b, err := json.MarshalIndent(p.Canonical(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// WriteExecutionPlan creates a NEW output directory and writes the pretty JSON,
// canonical JSON, Markdown and SHA256SUMS. It never writes outside that
// directory and never creates a temporary state directory.
func WriteExecutionPlan(out string, p *ExecutionPlan, inputs []string) error {
	if p == nil {
		return errors.New("nil execution plan")
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := newDirectory(out, inputs); err != nil {
		return err
	}
	pretty, err := MarshalExecutionPlan(p)
	if err != nil {
		return err
	}
	canonical, err := p.CanonicalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "deployment-execution-plan.json"), pretty, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "deployment-execution-plan.canonical.json"), canonical, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "deployment-execution-plan.md"), []byte(RenderExecutionPlanMarkdown(p)), 0644); err != nil {
		return err
	}
	return checksums(out)
}

// RenderExecutionPlanMarkdown returns a deterministic human-readable runbook. It
// states the lifecycle exclusion in the document itself and never labels a
// read-only row as blocked or an execution row as executed.
func RenderExecutionPlanMarkdown(p *ExecutionPlan) string {
	if p == nil {
		return ""
	}
	c := p.Canonical()
	var b strings.Builder
	fmt.Fprintf(&b, "# Deployment execution plan (render-only, offline)\n\n")
	fmt.Fprintf(&b, "**Mode: %s. Execution blocked: %t. Authorization: %s.**\n\n",
		mdCell(c.Mode), c.ExecutionBlocked, mdCell(c.Authorization))
	fmt.Fprintf(&b, "> This is a deterministic offline runbook. It does not deploy, provision, apply, scale, restart, stop, fault, load, reset or delete anything. Every execution-bearing row is marked `%s` and `%s`. Read-only rows are rendered in-process from caller-supplied documents; no external I/O is performed beyond reading those documents and writing this output directory.\n\n",
		ExecutionBlockedMarker, ExecutionStatusNotExecuted)
	fmt.Fprintf(&b, "Interpretation: `%s`; schema version: `%d`; plan interpretation: `%s`; plan schema version: `%d`.\n\n",
		mdCell(c.Interpretation), c.SchemaVersion, mdCell(c.PlanInterpretation), c.PlanSchemaVersion)
	fmt.Fprintf(&b, "Platform: `%s`; project: `%s`.\n\n", mdCell(c.Platform), mdCell(c.Project))
	fmt.Fprintf(&b, "Plan stage order: %s.\n\n", mdCell(joinStages(c.PlanStageOrder)))
	fmt.Fprintf(&b, "> Runbook row order is the documented render-only checklist. The approved plan's state machine order remains the line above; the two orders are intentionally labelled separately.\n\n")

	fmt.Fprintf(&b, "## Input identity\n\n")
	fmt.Fprintf(&b, "| Input | Raw SHA-256 | Canonical SHA-256 |\n|---|---|---|\n")
	fmt.Fprintf(&b, "| descriptor | `%s` | `%s` |\n", mdCell(c.Inputs.DescriptorRawSHA256), mdCell(c.Inputs.DescriptorCanonicalSHA256))
	fmt.Fprintf(&b, "| inventory | `%s` | `%s` |\n", mdCell(c.Inputs.InventoryRawSHA256), mdCell(c.Inputs.InventoryCanonicalSHA256))
	fmt.Fprintf(&b, "| orchestration plan | `%s` | `%s` |\n\n", mdCell(c.Inputs.OrchestrationPlanRawSHA256), mdCell(c.Inputs.OrchestrationPlanCanonicalSHA256))

	fmt.Fprintf(&b, "## Ordered steps\n\n")
	fmt.Fprintf(&b, "| # | Step | Plan stage | Mode | Permissible | Blocked | Marker | Status |\n|---:|---|---|---|---|---|---|---|\n")
	for _, step := range c.Steps {
		blocked := joinActions(step.BlockedActions)
		if blocked == "" {
			blocked = "—"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			step.Order, mdCell(step.Step), mdCell(string(step.PlanStage)), mdCell(string(step.Mode)),
			mdCell(joinActions(step.PermissibleActions)), mdCell(blocked), mdCell(step.Marker), step.ExecutionStatus)
	}
	fmt.Fprintf(&b, "\n")
	for _, step := range c.Steps {
		fmt.Fprintf(&b, "### %d. %s — %s — %s", step.Order, mdCell(step.Step), mdCell(string(step.Mode)), mdCell(step.Marker))
		if string(step.ExecutionStatus) != step.Marker {
			fmt.Fprintf(&b, " — %s", step.ExecutionStatus)
		}
		fmt.Fprintf(&b, "\n\n")
		fmt.Fprintf(&b, "%s\n\n", mdCell(step.Purpose))
		if step.Mode == StepModeExecutionBlocked {
			fmt.Fprintf(&b, "- Blocked reason: %s\n", mdCell(step.BlockedReason))
			fmt.Fprintf(&b, "- Required authorization: `%s`\n", mdCell(step.RequiredAuthorization))
		}
		fmt.Fprintf(&b, "- Operation identifiers: %s\n", mdCell(strings.Join(step.OperationIDs, ", ")))
		if len(step.ExpectedEvidence) > 0 {
			fmt.Fprintf(&b, "- Expected evidence roles: %s\n", mdCell(strings.Join(step.ExpectedEvidence, ", ")))
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Expected services (derived from orchestration v2)\n\n")
	fmt.Fprintf(&b, "| Service | Tier | Replicas | Lifecycle | Activation | Healthcheck | Health modes |\n|---|---|---:|---|---|---|---|\n")
	for _, svc := range c.ExpectedServices {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s |\n",
			mdCell(svc.Service), mdCell(string(svc.Tier)), svc.Replicas, mdCell(string(svc.Lifecycle)),
			mdCell(string(svc.Activation)), mdCell(string(svc.Healthcheck)), mdCell(strings.Join(svc.HealthModes, ", ")))
	}
	fmt.Fprintf(&b, "\n> These are plan declarations. A one_shot service is successful only on exit code 0; a healthcheck:none service requires running plus functional evidence; an optional_profile service is NOT_ASSESSED unless activated. No live observation is implied.\n\n")

	fmt.Fprintf(&b, "## Resource guard\n\n")
	fmt.Fprintf(&b, "Scope: `%s`. Declared values come from the descriptor; observed values are explicitly not assessed.\n\n", mdCell(c.ResourceGuard.Scope))
	fmt.Fprintf(&b, "### Declared (not a live measurement)\n\n")
	fmt.Fprintf(&b, "| Class | Service | Replicas | CPU set | Memory per replica (bytes) | Declared total memory (bytes) |\n|---|---|---:|---|---:|---:|\n")
	for _, svc := range c.ResourceGuard.Declared.Services {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %d | %d |\n",
			mdCell(string(svc.Class)), mdCell(svc.Service), svc.Replicas, mdCell(svc.CPUSet), svc.MemoryBytes, svc.DeclaredTotalMemoryBytes)
	}
	fmt.Fprintf(&b, "\n")
	fmt.Fprintf(&b, "| Class | Services | Replicas | Memory bytes |\n|---|---:|---:|---:|\n")
	fmt.Fprintf(&b, "| application | %d | %d | %d |\n", c.ResourceGuard.Declared.ApplicationServices, c.ResourceGuard.Declared.ApplicationReplicas, c.ResourceGuard.Declared.ApplicationMemoryBytes)
	fmt.Fprintf(&b, "| supporting | %d | %d | %d |\n", c.ResourceGuard.Declared.SupportingServices, c.ResourceGuard.Declared.SupportingReplicas, c.ResourceGuard.Declared.SupportingMemoryBytes)
	fmt.Fprintf(&b, "| total | %d | %d | %d |\n\n", c.ResourceGuard.Declared.TotalServices, c.ResourceGuard.Declared.TotalReplicas, c.ResourceGuard.Declared.TotalMemoryBytes)
	fmt.Fprintf(&b, "Application CPU sets: %s. Supporting CPU sets: %s.\n\n",
		mdCell(strings.Join(c.ResourceGuard.Declared.ApplicationCPUSets, ", ")),
		mdCell(strings.Join(c.ResourceGuard.Declared.SupportingCPUSets, ", ")))
	fmt.Fprintf(&b, "**Observed: %s** — source `%s`. %s\n\n",
		c.ResourceGuard.Observed.Status, mdCell(c.ResourceGuard.Observed.Source), mdCell(c.ResourceGuard.Observed.Reason))

	fmt.Fprintf(&b, "## Artifact and evidence checklist\n\n")
	fmt.Fprintf(&b, "Packaging interpretation: `%s`; checksum algorithm: `%s`; run root: `%s`; manifest: `%s`; checksums: `%s`.\n\n",
		mdCell(c.EvidenceChecklist.PackagingInterpretation), mdCell(c.EvidenceChecklist.ChecksumAlgorithm),
		mdCell(c.EvidenceChecklist.RunRootTemplate), mdCell(c.EvidenceChecklist.ManifestTemplate), mdCell(c.EvidenceChecklist.ChecksumsTemplate))
	fmt.Fprintf(&b, "Prior-manifest verification: `%s`; prior-manifest binding status: **%s**; live validation: **%s**.\n\n",
		mdCell(c.EvidenceChecklist.PriorManifestVerification), c.EvidenceChecklist.PriorManifestBindingStatus, c.EvidenceChecklist.LiveValidation)
	fmt.Fprintf(&b, "| Role | Required | Status | Reason |\n|---|---|---|---|\n")
	for _, item := range c.EvidenceChecklist.Roles {
		fmt.Fprintf(&b, "| %s | %t | %s | %s |\n", mdCell(item.Role), item.Required, item.Status, mdCell(item.Reason))
	}
	fmt.Fprintf(&b, "\n> The role taxonomy is the packaging specification taxonomy. A checklist row is not an artifact observation: no artifact was read, verified, copied or written.\n\n")

	fmt.Fprintf(&b, "## Limitations\n\n")
	for _, limitation := range c.Limitations {
		fmt.Fprintf(&b, "- %s\n", mdCell(limitation))
	}
	fmt.Fprintf(&b, "\n## Non-goals and lifecycle exclusion\n\n")
	fmt.Fprintf(&b, "- No Docker client call, SUT access, provisioning, apply/up/down, scale, restart, stop, fault, load, reset or delete.\n")
	fmt.Fprintf(&b, "- No executor, lifecycle method, shell invocation or runnable platform command exists in this package or output.\n")
	fmt.Fprintf(&b, "- No live health, replica, label, volume, resource or monitoring value is claimed or fabricated.\n")
	fmt.Fprintf(&b, "- No artifact is copied, moved, rewritten, deleted or verified against a prior manifest; prior-manifest binding is `%s`.\n", PriorManifestVerificationNotImplemented)
	fmt.Fprintf(&b, "- No timestamp, working-directory path or environment probe is included.\n")
	return b.String()
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, value := range a {
		set[value] = true
	}
	if len(set) != len(a) {
		return false
	}
	for _, value := range b {
		if !set[value] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
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
