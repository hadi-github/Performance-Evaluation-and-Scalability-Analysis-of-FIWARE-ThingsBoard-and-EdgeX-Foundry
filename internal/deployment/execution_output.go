// Canonicalisation, packaging and Markdown rendering of the M2d execution plan.
package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
