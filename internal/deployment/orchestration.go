package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// This file adds the M2b offline orchestration blueprint schema. It is a pure
// document contract: it never contacts Docker, a SUT, a platform, a lifecycle
// endpoint, a monitoring service or a load generator, and it exposes no
// execution path.

const (
	// OrchestrationSchemaVersion versions the M2b orchestration plan. Version 2
	// adds explicit lifecycle, activation and healthcheck-presence declarations;
	// version 1 remains a rejected predecessor rather than being reinterpreted.
	OrchestrationSchemaVersion = 2
	// OrchestrationInterpretation names the exact plan semantics.
	OrchestrationInterpretation = "deployment-orchestration-plan-v2"
	// OrchestrationMode is the only admitted mode. A plan is a blueprint, not a
	// runnable deployment description.
	OrchestrationMode = "offline-blueprint-only"
	// OrchestrationAuthorization records that execution is deferred to a separate
	// reviewed and explicitly authorized milestone.
	OrchestrationAuthorization = "separate-review-and-authorization-required"
	// ExecutionBlockedMarker is the literal marker a future execution gate must
	// replace only after separate authorization. It is documentation metadata, not
	// a callable action.
	ExecutionBlockedMarker = "EXECUTION_BLOCKED_HERE"
)

// StageName identifies one versioned orchestration stage. The stage sequence is
// fixed and every platform declares the same read-only state machine.
type StageName string

const (
	StagePreDeployInventory StageName = "pre_deploy_inventory"
	StageProvisionOnSUT     StageName = "provision_on_sut"
	StageDryRunRender       StageName = "dry_run_render"
	StageApply              StageName = "apply"
	StageValidateHealth     StageName = "validate_health"
	StageEvidence           StageName = "evidence"
	StageRollback           StageName = "rollback"
)

// stageOrder is the only admitted stage sequence.
var stageOrder = []StageName{
	StagePreDeployInventory,
	StageProvisionOnSUT,
	StageDryRunRender,
	StageApply,
	StageValidateHealth,
	StageEvidence,
	StageRollback,
}

// Action is the explicit orchestration taxonomy. Safe actions can inspect,
// compare, render or attest. Blocked actions change platform state and are
// never admitted as permissible in this schema.
type Action string

const (
	ActionRead         Action = "Read"
	ActionValidate     Action = "Validate"
	ActionRenderDryRun Action = "RenderDryRun"
	ActionAttest       Action = "Attest"

	ActionApply   Action = "Apply"
	ActionScale   Action = "Scale"
	ActionRestart Action = "Restart"
	ActionStop    Action = "Stop"
	ActionFault   Action = "Fault"
	ActionLoad    Action = "Load"
	ActionReset   Action = "Reset"
	ActionDelete  Action = "Delete"
)

// actionRank gives deterministic ordering without relying on map iteration.
var actionRank = map[Action]int{
	ActionRead: 0, ActionValidate: 1, ActionRenderDryRun: 2, ActionAttest: 3,
	ActionApply: 4, ActionScale: 5, ActionRestart: 6, ActionStop: 7,
	ActionFault: 8, ActionLoad: 9, ActionReset: 10, ActionDelete: 11,
}

// IsSafeAction reports whether a belongs to the read-only taxonomy.
func IsSafeAction(a Action) bool {
	switch a {
	case ActionRead, ActionValidate, ActionRenderDryRun, ActionAttest:
		return true
	default:
		return false
	}
}

// IsBlockedAction reports whether a changes platform state and is therefore
// never permissible in an offline blueprint.
func IsBlockedAction(a Action) bool {
	switch a {
	case ActionApply, ActionScale, ActionRestart, ActionStop,
		ActionFault, ActionLoad, ActionReset, ActionDelete:
		return true
	default:
		return false
	}
}

// ServiceLifecycle distinguishes a long-running Compose service from a one-shot
// job. A one-shot job is successful when it completes with exit code 0; it is
// not expected to remain running and must not be attested as a persistent
// service.
type ServiceLifecycle string

const (
	// LifecyclePersistent is a service that must remain running for the run.
	LifecyclePersistent ServiceLifecycle = "persistent"
	// LifecycleOneShot is a Compose job (usually restart: "no") that must finish
	// successfully. Its expected terminal state is a successful exit, not
	// "running".
	LifecycleOneShot ServiceLifecycle = "one_shot"
)

// ServiceActivation distinguishes services started by the unqualified Compose
// project from services gated behind an opt-in profile.
type ServiceActivation string

const (
	// ActivationDefaultStart is started by an unqualified Compose invocation,
	// subject only to the existing depends_on conditions.
	ActivationDefaultStart ServiceActivation = "default_start"
	// ActivationOptionalProfile is not started by default; it is gated behind a
	// Compose profile and is NOT_ASSESSED unless explicitly activated.
	ActivationOptionalProfile ServiceActivation = "optional_profile"
)

// HealthcheckDeclaration records whether the unchanged Compose stack declares
// an explicit healthcheck for the service. It is not an observed health state.
type HealthcheckDeclaration string

const (
	// HealthcheckDeclared means the Compose service has an explicit healthcheck;
	// healthy and unhealthy are meaningful states and unhealthy is a FAIL.
	HealthcheckDeclared HealthcheckDeclaration = "declared"
	// HealthcheckNone means the Compose service has no healthcheck. "none" is a
	// property, not a failed check; running/functional evidence is required
	// instead of a synthetic healthy value.
	HealthcheckNone HealthcheckDeclaration = "none"
)

// ServiceExpectation declares one Compose service and its expected replica
// count plus lifecycle, activation and healthcheck-presence semantics. It is a
// blueprint declaration, not an observation.
type ServiceExpectation struct {
	Service     string                 `json:"service"`
	Replicas    int                    `json:"replicas"`
	Lifecycle   ServiceLifecycle       `json:"lifecycle"`
	Activation  ServiceActivation      `json:"activation"`
	Healthcheck HealthcheckDeclaration `json:"healthcheck"`
}

// ObservedHealth is a caller-supplied health attestation used by the pure
// service predicate. ObservedHealthNone records that no Compose healthcheck
// exists; it is never treated as a failed check and never becomes "healthy".
type ObservedHealth string

const (
	ObservedHealthy    ObservedHealth = "healthy"
	ObservedUnhealthy  ObservedHealth = "unhealthy"
	ObservedHealthNone ObservedHealth = "none"
	ObservedUnknown    ObservedHealth = "unknown"
)

// ServiceObservation is a caller-supplied, offline service attestation. This
// package never collects one from Docker, a SUT or a live endpoint; the caller
// supplies it and the predicate only compares it with a declaration.
type ServiceObservation struct {
	Service            string         `json:"service"`
	Present            bool           `json:"present"`
	Replicas           int            `json:"replicas"`
	Running            bool           `json:"running"`
	Health             ObservedHealth `json:"health"`
	ExitCode           *int           `json:"exit_code,omitempty"`
	FunctionalEvidence bool           `json:"functional_evidence"`
}

// PlatformBlueprint declares the expected state machine and evidence contract
// for one platform. Project and service names are the existing Compose names;
// this file adds no stack topology.
type PlatformBlueprint struct {
	Platform                 string               `json:"platform"`
	Project                  string               `json:"project"`
	StateMachine             []StageName          `json:"state_machine"`
	ApplicationServices      []ServiceExpectation `json:"application_services"`
	SupportingServices       []ServiceExpectation `json:"supporting_services"`
	HealthChecks             []string             `json:"health_checks"`
	AllowedEvidenceRoles     []string             `json:"allowed_evidence_roles"`
	RequiredAttestationRoles []string             `json:"required_attestation_roles"`
	MissingEvidenceStatus    Status               `json:"missing_evidence_status"`
}

// OrchestrationStage binds one stage to its permissible actions and its
// explicitly blocked actions. A stage that names any blocked action must carry
// ExecutionBlockedHere. The apply stage must always do so.
type OrchestrationStage struct {
	Stage                StageName `json:"stage"`
	Purpose              string    `json:"purpose"`
	PermissibleActions   []Action  `json:"permissible_actions"`
	BlockedActions       []Action  `json:"blocked_actions,omitempty"`
	ExecutionBlockedHere bool      `json:"execution_blocked_here"`
	ExpectedEvidence     []string  `json:"expected_evidence,omitempty"`
}

// OrchestrationPlan is the strict M2b schema v2 blueprint. ExecutionBlocked
// must be true: the schema has no representation for an executable plan.
type OrchestrationPlan struct {
	SchemaVersion    int                  `json:"schema_version"`
	Interpretation   string               `json:"interpretation"`
	Mode             string               `json:"mode"`
	Authorization    string               `json:"authorization"`
	ExecutionBlocked bool                 `json:"execution_blocked"`
	Stages           []OrchestrationStage `json:"stages"`
	Platforms        []PlatformBlueprint  `json:"platforms"`
}

var orchestrationPlanShape = jsonShape{
	object: map[string]jsonShape{
		"schema_version":    scalarValueShape,
		"interpretation":    scalarValueShape,
		"mode":              scalarValueShape,
		"authorization":     scalarValueShape,
		"execution_blocked": scalarValueShape,
		"stages": {element: &jsonShape{
			object: map[string]jsonShape{
				"stage":                  scalarValueShape,
				"purpose":                scalarValueShape,
				"permissible_actions":    {element: &scalarValueShape},
				"blocked_actions":        {element: &scalarValueShape},
				"execution_blocked_here": scalarValueShape,
				"expected_evidence":      {element: &scalarValueShape},
			},
			optional: map[string]bool{"blocked_actions": true, "expected_evidence": true},
		}},
		"platforms": {element: &jsonShape{
			object: map[string]jsonShape{
				"platform":                   scalarValueShape,
				"project":                    scalarValueShape,
				"state_machine":              {element: &scalarValueShape},
				"application_services":       {element: &serviceExpectationShape},
				"supporting_services":        {element: &serviceExpectationShape},
				"health_checks":              {element: &scalarValueShape},
				"allowed_evidence_roles":     {element: &scalarValueShape},
				"required_attestation_roles": {element: &scalarValueShape},
				"missing_evidence_status":    scalarValueShape,
			},
		}},
	},
}

var serviceExpectationShape = jsonShape{
	object: map[string]jsonShape{
		"service":     scalarValueShape,
		"replicas":    scalarValueShape,
		"lifecycle":   scalarValueShape,
		"activation":  scalarValueShape,
		"healthcheck": scalarValueShape,
	},
}

var platformProject = map[string]string{
	"thingsboard": "tb",
	"fiware":      "fiware",
	"edgex":       "edgex",
}

// requiredDefaultStartServices lists the services the unchanged Compose stacks
// start without an opt-in profile. It includes one-shot jobs: omitting a
// dependency-required job (for example FIWARE mongo-init) would model a stack
// that the repository does not actually start. Optional-profile services are
// declarable but not required here.
var requiredDefaultStartServices = map[string][]string{
	"thingsboard": {"tb-core", "tb-rule-engine", "tb-http-transport", "zookeeper", "kafka", "postgres", "valkey", "haproxy"},
	"fiware":      {"orion", "iot-agent", "mongo", "mongo-init", "haproxy"},
	"edgex":       {"core-data", "device-rest", "database", "nats", "core-keeper", "core-common-config-bootstrapper", "core-metadata", "haproxy"},
}

// DecodeOrchestrationPlan strictly decodes an M2b orchestration plan. It
// performs the same duplicate-key, null, exact-case-key and trailing-JSON
// checks as the approved M2 documents.
func DecodeOrchestrationPlan(b []byte) (*OrchestrationPlan, error) {
	if err := validateRawDocument(b, orchestrationPlanShape, "orchestration plan"); err != nil {
		return nil, err
	}
	var p OrchestrationPlan
	if err := decodeStrict(b, &p); err != nil {
		return nil, fmt.Errorf("orchestration plan: %w", err)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate checks the plan after decoding or direct construction.
func (p OrchestrationPlan) Validate() error {
	if p.SchemaVersion != OrchestrationSchemaVersion {
		return fmt.Errorf("orchestration plan: unsupported schema_version %d", p.SchemaVersion)
	}
	if p.Interpretation != OrchestrationInterpretation {
		return fmt.Errorf("orchestration plan: interpretation must be %q", OrchestrationInterpretation)
	}
	if p.Mode != OrchestrationMode {
		return fmt.Errorf("orchestration plan: mode must be %q", OrchestrationMode)
	}
	if p.Authorization != OrchestrationAuthorization {
		return fmt.Errorf("orchestration plan: authorization must be %q", OrchestrationAuthorization)
	}
	if !p.ExecutionBlocked {
		return errors.New("orchestration plan: execution_blocked must be true; M2b admits no executable plan")
	}
	if len(p.Stages) != len(stageOrder) {
		return fmt.Errorf("orchestration plan: stages must contain exactly the %d versioned stages", len(stageOrder))
	}
	for i, want := range stageOrder {
		if p.Stages[i].Stage != want {
			return fmt.Errorf("orchestration plan: stage %d must be %q, got %q", i, want, p.Stages[i].Stage)
		}
		if err := validateStage(p.Stages[i]); err != nil {
			return err
		}
	}
	if len(p.Platforms) == 0 {
		return errors.New("orchestration plan: platforms must be a non-empty explicit array")
	}
	seenPlatforms := map[string]bool{}
	for i, platform := range p.Platforms {
		if err := validatePlatformBlueprint(platform, i); err != nil {
			return err
		}
		if seenPlatforms[platform.Platform] {
			return fmt.Errorf("orchestration plan platform %d: duplicate platform %q", i, platform.Platform)
		}
		seenPlatforms[platform.Platform] = true
	}
	for _, required := range []string{"thingsboard", "fiware", "edgex"} {
		if !seenPlatforms[required] {
			return fmt.Errorf("orchestration plan: platform %q must declare its state machine", required)
		}
	}
	return nil
}

func validateStage(s OrchestrationStage) error {
	context := fmt.Sprintf("orchestration plan stage %q", s.Stage)
	if strings.TrimSpace(s.Purpose) == "" {
		return fmt.Errorf("%s: purpose is required", context)
	}
	if len(s.PermissibleActions) == 0 {
		return fmt.Errorf("%s: permissible_actions must be non-empty", context)
	}
	seenSafe := map[Action]bool{}
	for _, a := range s.PermissibleActions {
		if !IsSafeAction(a) {
			return fmt.Errorf("%s: action %q is not in the read-only taxonomy (Read, Validate, RenderDryRun, Attest)", context, a)
		}
		if seenSafe[a] {
			return fmt.Errorf("%s: duplicate permissible action %q", context, a)
		}
		seenSafe[a] = true
	}
	seenBlocked := map[Action]bool{}
	for _, a := range s.BlockedActions {
		if !IsBlockedAction(a) {
			return fmt.Errorf("%s: blocked action %q is not a lifecycle action", context, a)
		}
		if seenSafe[a] {
			return fmt.Errorf("%s: action %q cannot be both permissible and blocked", context, a)
		}
		if seenBlocked[a] {
			return fmt.Errorf("%s: duplicate blocked action %q", context, a)
		}
		seenBlocked[a] = true
	}
	if len(s.BlockedActions) > 0 && !s.ExecutionBlockedHere {
		return fmt.Errorf("%s: blocked actions require execution_blocked_here=true (%s)", context, ExecutionBlockedMarker)
	}
	if s.Stage == StageApply {
		if !s.ExecutionBlockedHere {
			return fmt.Errorf("%s: apply must be marked %s", context, ExecutionBlockedMarker)
		}
		if !seenBlocked[ActionApply] {
			return fmt.Errorf("%s: apply must reference Apply as a blocked action", context)
		}
	}
	switch s.Stage {
	case StageProvisionOnSUT:
		if !s.ExecutionBlockedHere || len(s.BlockedActions) == 0 {
			return fmt.Errorf("%s: provisioning must be marked %s with at least one blocked lifecycle reference", context, ExecutionBlockedMarker)
		}
	case StageRollback:
		if !s.ExecutionBlockedHere || len(s.BlockedActions) == 0 {
			return fmt.Errorf("%s: rollback must be marked %s with at least one blocked lifecycle reference", context, ExecutionBlockedMarker)
		}
	}
	seenEvidence := map[string]bool{}
	for _, role := range s.ExpectedEvidence {
		if strings.TrimSpace(role) != role || role == "" {
			return fmt.Errorf("%s: expected_evidence role %q must be non-empty without surrounding whitespace", context, role)
		}
		if seenEvidence[role] {
			return fmt.Errorf("%s: duplicate expected_evidence role %q", context, role)
		}
		seenEvidence[role] = true
	}
	return nil
}

func validatePlatformBlueprint(p PlatformBlueprint, index int) error {
	context := fmt.Sprintf("orchestration plan platform %d", index)
	if !platformAllowlisted(p.Platform) {
		return fmt.Errorf("%s: platform %q is outside the allowlist (thingsboard, fiware, edgex)", context, p.Platform)
	}
	if want := platformProject[p.Platform]; p.Project != want {
		return fmt.Errorf("%s: platform %q must use the existing compose project %q, got %q", context, p.Platform, want, p.Project)
	}
	if err := validateStageMachine(p.StateMachine); err != nil {
		return fmt.Errorf("%s: %w", context, err)
	}
	if err := validateServiceExpectations(p.ApplicationServices, "application_services", context); err != nil {
		return err
	}
	if err := validateServiceExpectations(p.SupportingServices, "supporting_services", context); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, svc := range append(append([]ServiceExpectation{}, p.ApplicationServices...), p.SupportingServices...) {
		if seen[svc.Service] {
			return fmt.Errorf("%s: duplicate service %q across application and supporting tiers", context, svc.Service)
		}
		seen[svc.Service] = true
	}
	for _, required := range requiredDefaultStartServices[p.Platform] {
		if !seen[required] {
			return fmt.Errorf("%s: platform %q must declare the default-start service %q from the unchanged Compose stack", context, p.Platform, required)
		}
	}
	if err := validateNonEmptyStrings(p.HealthChecks, "health_checks", context); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(p.AllowedEvidenceRoles, "allowed_evidence_roles", context); err != nil {
		return err
	}
	if err := validateNonEmptyStrings(p.RequiredAttestationRoles, "required_attestation_roles", context); err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, role := range p.AllowedEvidenceRoles {
		allowed[role] = true
	}
	for _, role := range p.RequiredAttestationRoles {
		if !allowed[role] {
			return fmt.Errorf("%s: required attestation role %q is not in allowed_evidence_roles", context, role)
		}
	}
	if p.MissingEvidenceStatus != Unavailable && p.MissingEvidenceStatus != NotAssessed {
		return fmt.Errorf("%s: missing_evidence_status must be UNAVAILABLE or NOT_ASSESSED, never PASS or FAIL", context)
	}
	return nil
}

func validateStageMachine(machine []StageName) error {
	if len(machine) != len(stageOrder) {
		return fmt.Errorf("state_machine must contain exactly the %d versioned stages", len(stageOrder))
	}
	for i, want := range stageOrder {
		if machine[i] != want {
			return fmt.Errorf("state_machine stage %d must be %q, got %q", i, want, machine[i])
		}
	}
	return nil
}

func validateServiceExpectations(services []ServiceExpectation, label, context string) error {
	if len(services) == 0 {
		return fmt.Errorf("%s: %s must be a non-empty explicit array", context, label)
	}
	seen := map[string]bool{}
	for i, svc := range services {
		if !serviceNamePattern.MatchString(svc.Service) {
			return fmt.Errorf("%s: %s[%d]: service %q is not a valid compose service name", context, label, i, svc.Service)
		}
		if svc.Replicas <= 0 {
			return fmt.Errorf("%s: %s[%d]: replicas must be positive", context, label, i)
		}
		switch svc.Lifecycle {
		case LifecyclePersistent:
		case LifecycleOneShot:
			if svc.Replicas != 1 {
				return fmt.Errorf("%s: %s[%d]: one_shot service %q must declare exactly one replica", context, label, i, svc.Service)
			}
		default:
			return fmt.Errorf("%s: %s[%d]: lifecycle %q must be persistent or one_shot", context, label, i, svc.Lifecycle)
		}
		switch svc.Activation {
		case ActivationDefaultStart, ActivationOptionalProfile:
		default:
			return fmt.Errorf("%s: %s[%d]: activation %q must be default_start or optional_profile", context, label, i, svc.Activation)
		}
		switch svc.Healthcheck {
		case HealthcheckDeclared, HealthcheckNone:
		default:
			return fmt.Errorf("%s: %s[%d]: healthcheck %q must be declared or none", context, label, i, svc.Healthcheck)
		}
		if seen[svc.Service] {
			return fmt.Errorf("%s: %s: duplicate service %q", context, label, svc.Service)
		}
		seen[svc.Service] = true
	}
	return nil
}

// EvaluateServiceExpectation compares one caller-supplied observation with one
// declared service. It is pure and offline; it performs no Docker or SUT call.
//
// Semantics:
//   - an absent optional_profile service is NOT_ASSESSED, not FAIL;
//   - an absent default_start service is UNAVAILABLE, not a successful zero;
//   - a persistent service must be running with the declared replica count;
//   - an explicit healthcheck that reports unhealthy is FAIL, while missing
//     health evidence is UNAVAILABLE;
//   - a missing healthcheck is HealthcheckNone, never a failed check: running
//     plus functional evidence is required, and its absence is UNAVAILABLE;
//   - a one-shot job is successful only when it exits with code 0; absence of an
//     exit code is UNAVAILABLE, and "running" is neither expected nor sufficient.
func EvaluateServiceExpectation(e ServiceExpectation, o ServiceObservation) (Status, string) {
	context := fmt.Sprintf("service %q", e.Service)
	if !o.Present {
		if e.Activation == ActivationOptionalProfile {
			return NotAssessed, fmt.Sprintf("%s is behind an optional Compose profile and was not activated", context)
		}
		return Unavailable, fmt.Sprintf("%s is a default-start service but no observation was supplied", context)
	}
	if o.Service != "" && o.Service != e.Service {
		return Fail, fmt.Sprintf("%s observation names service %q instead", context, o.Service)
	}
	if o.Replicas != e.Replicas {
		return Fail, fmt.Sprintf("%s observed %d replicas, expected %d", context, o.Replicas, e.Replicas)
	}
	switch e.Lifecycle {
	case LifecycleOneShot:
		if o.ExitCode == nil {
			return Unavailable, fmt.Sprintf("%s is one_shot but no completion exit code was supplied; running/healthy state is not completion evidence", context)
		}
		if *o.ExitCode != 0 {
			return Fail, fmt.Sprintf("%s one_shot job exited with code %d, expected successful exit 0", context, *o.ExitCode)
		}
		return Pass, fmt.Sprintf("%s one_shot job completed successfully with exit code 0", context)
	case LifecyclePersistent:
		if !o.Running {
			return Fail, fmt.Sprintf("%s is persistent but is not running", context)
		}
		switch e.Healthcheck {
		case HealthcheckDeclared:
			switch o.Health {
			case ObservedHealthy:
				return Pass, fmt.Sprintf("%s is running and its explicit healthcheck attests healthy", context)
			case ObservedUnhealthy:
				return Fail, fmt.Sprintf("%s is running but its explicit healthcheck attests unhealthy", context)
			default:
				return Unavailable, fmt.Sprintf("%s declares an explicit healthcheck but no healthy/unhealthy health evidence was supplied", context)
			}
		case HealthcheckNone:
			if o.FunctionalEvidence {
				return Pass, fmt.Sprintf("%s is running with no Compose healthcheck and functional evidence was supplied", context)
			}
			return Unavailable, fmt.Sprintf("%s is running with no Compose healthcheck; functional evidence is missing (health:none is not a failed check)", context)
		default:
			return Unavailable, fmt.Sprintf("%s has an unknown healthcheck declaration %q", context, e.Healthcheck)
		}
	default:
		return Unavailable, fmt.Sprintf("%s has an unknown lifecycle declaration %q", context, e.Lifecycle)
	}
}

func validateNonEmptyStrings(values []string, label, context string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s: %s must be a non-empty explicit array", context, label)
	}
	seen := map[string]bool{}
	for _, v := range values {
		if v == "" || strings.TrimSpace(v) != v {
			return fmt.Errorf("%s: %s contains an empty or whitespace-padded value", context, label)
		}
		if seen[v] {
			return fmt.Errorf("%s: %s contains duplicate value %q", context, label, v)
		}
		seen[v] = true
	}
	return nil
}

// Canonical returns a deep copy with deterministic ordering and no added
// information.
func (p OrchestrationPlan) Canonical() OrchestrationPlan {
	out := p
	out.Stages = append([]OrchestrationStage(nil), p.Stages...)
	for i := range out.Stages {
		out.Stages[i].PermissibleActions = append([]Action(nil), p.Stages[i].PermissibleActions...)
		out.Stages[i].BlockedActions = append([]Action(nil), p.Stages[i].BlockedActions...)
		out.Stages[i].ExpectedEvidence = append([]string(nil), p.Stages[i].ExpectedEvidence...)
		sortActions(out.Stages[i].PermissibleActions)
		sortActions(out.Stages[i].BlockedActions)
		sort.Strings(out.Stages[i].ExpectedEvidence)
	}
	out.Platforms = append([]PlatformBlueprint(nil), p.Platforms...)
	for i := range out.Platforms {
		platform := &out.Platforms[i]
		platform.StateMachine = append([]StageName(nil), platform.StateMachine...)
		platform.ApplicationServices = append([]ServiceExpectation(nil), platform.ApplicationServices...)
		platform.SupportingServices = append([]ServiceExpectation(nil), platform.SupportingServices...)
		platform.HealthChecks = append([]string(nil), platform.HealthChecks...)
		platform.AllowedEvidenceRoles = append([]string(nil), platform.AllowedEvidenceRoles...)
		platform.RequiredAttestationRoles = append([]string(nil), platform.RequiredAttestationRoles...)
		sortServiceExpectations(platform.ApplicationServices)
		sortServiceExpectations(platform.SupportingServices)
		sort.Strings(platform.HealthChecks)
		sort.Strings(platform.AllowedEvidenceRoles)
		sort.Strings(platform.RequiredAttestationRoles)
	}
	sort.SliceStable(out.Platforms, func(i, j int) bool { return out.Platforms[i].Platform < out.Platforms[j].Platform })
	return out
}

func sortActions(actions []Action) {
	sort.SliceStable(actions, func(i, j int) bool {
		ri, ok := actionRank[actions[i]]
		if !ok {
			return false
		}
		rj, ok := actionRank[actions[j]]
		if !ok {
			return true
		}
		return ri < rj
	})
}

func sortServiceExpectations(services []ServiceExpectation) {
	sort.SliceStable(services, func(i, j int) bool { return services[i].Service < services[j].Service })
}

// CanonicalJSON is the deterministic compact plan representation.
func (p OrchestrationPlan) CanonicalJSON() ([]byte, error) {
	return json.Marshal(p.Canonical())
}

// CanonicalSHA256 is the semantic plan hash.
func (p OrchestrationPlan) CanonicalSHA256() (string, error) {
	b, err := p.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

// RenderOrchestrationPlanMarkdown returns a deterministic human-readable
// blueprint. It states the lifecycle exclusion in the document itself.
func RenderOrchestrationPlanMarkdown(p *OrchestrationPlan) string {
	if p == nil {
		return ""
	}
	c := p.Canonical()
	var b strings.Builder
	fmt.Fprintf(&b, "# Deployment orchestration blueprint (offline)\n\n")
	fmt.Fprintf(&b, "**Mode: %s. Execution blocked: %t. Authorization: %s.**\n\n",
		mdCell(c.Mode), c.ExecutionBlocked, mdCell(c.Authorization))
	fmt.Fprintf(&b, "> This is a versioned, offline blueprint. It does not deploy, provision, scale, restart, stop, fault, load, reset, delete or otherwise touch a SUT. Any stage that names a lifecycle action is a reference marked `%s` and requires separate review and authorization.\n\n", ExecutionBlockedMarker)
	fmt.Fprintf(&b, "Interpretation: `%s`; schema version: `%d`.\n\n", mdCell(c.Interpretation), c.SchemaVersion)

	fmt.Fprintf(&b, "## Stages\n\n")
	fmt.Fprintf(&b, "| Stage | Permissible actions | Blocked reference | Marker | Expected evidence |\n|---|---|---|---|---|\n")
	for _, s := range c.Stages {
		fmt.Fprintf(&b, "| %s | %s | %s | %t | %s |\n",
			mdCell(string(s.Stage)), mdCell(joinActions(s.PermissibleActions)),
			mdCell(joinActions(s.BlockedActions)), s.ExecutionBlockedHere,
			mdCell(strings.Join(s.ExpectedEvidence, ", ")))
	}
	fmt.Fprintf(&b, "\n")
	for _, s := range c.Stages {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", mdCell(string(s.Stage)), mdCell(s.Purpose))
	}

	fmt.Fprintf(&b, "## Platform state machines\n\n")
	fmt.Fprintf(&b, "> Each service declares `lifecycle` (persistent or one_shot), `activation` (default_start or optional_profile) and `healthcheck` (declared or none). A one_shot service is successful only on exit code 0; running/healthy state is not completion evidence. `healthcheck: none` is not a failed check: running plus functional evidence is required. An optional_profile service is NOT_ASSESSED unless explicitly activated.\n\n")
	for _, platform := range c.Platforms {
		fmt.Fprintf(&b, "### %s (project `%s`)\n\n", mdCell(platform.Platform), mdCell(platform.Project))
		fmt.Fprintf(&b, "- State machine: %s\n", mdCell(joinStages(platform.StateMachine)))
		fmt.Fprintf(&b, "- Application services: %s\n", mdCell(joinServices(platform.ApplicationServices)))
		fmt.Fprintf(&b, "- Supporting services: %s\n", mdCell(joinServices(platform.SupportingServices)))
		fmt.Fprintf(&b, "- Health checks: %s\n", mdCell(strings.Join(platform.HealthChecks, "; ")))
		fmt.Fprintf(&b, "- Allowed evidence roles: %s\n", mdCell(strings.Join(platform.AllowedEvidenceRoles, ", ")))
		fmt.Fprintf(&b, "- Required attestation roles: %s\n", mdCell(strings.Join(platform.RequiredAttestationRoles, ", ")))
		fmt.Fprintf(&b, "- Missing evidence semantics: **%s** (not PASS, not zero)\n\n", platform.MissingEvidenceStatus)
	}

	fmt.Fprintf(&b, "## Non-goals and lifecycle exclusion\n\n")
	fmt.Fprintf(&b, "- No Docker client call, SUT access, provisioning, apply/up/down, scale, restart, stop, fault, load, reset/clean or delete action.\n")
	fmt.Fprintf(&b, "- No stage executes a lifecycle action. `%s` is metadata for a future separately authorized capability.\n", ExecutionBlockedMarker)
	fmt.Fprintf(&b, "- No compose file, `.env`, Makefile, monitoring script, receiver or ledger is modified or invoked.\n")
	fmt.Fprintf(&b, "- No live health, replica, label, volume or monitoring value is claimed or fabricated.\n")
	return b.String()
}

func joinActions(actions []Action) string {
	parts := make([]string, len(actions))
	for i, a := range actions {
		parts[i] = string(a)
	}
	return strings.Join(parts, ", ")
}

func joinStages(stages []StageName) string {
	parts := make([]string, len(stages))
	for i, s := range stages {
		parts[i] = string(s)
	}
	return strings.Join(parts, " -> ")
}

func joinServices(services []ServiceExpectation) string {
	parts := make([]string, len(services))
	for i, s := range services {
		parts[i] = fmt.Sprintf("%s x%d (%s, %s, healthcheck:%s)", s.Service, s.Replicas, s.Lifecycle, s.Activation, s.Healthcheck)
	}
	return strings.Join(parts, ", ")
}
