package deployment

import "regexp"

// The M2d render-only deployment execution plan is a pure
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
