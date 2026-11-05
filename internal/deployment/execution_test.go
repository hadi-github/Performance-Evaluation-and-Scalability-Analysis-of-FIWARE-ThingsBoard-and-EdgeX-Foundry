package deployment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func m2dReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func m2dPlanBytes(t *testing.T) []byte {
	t.Helper()
	return m2dReadFile(t, filepath.Join("testdata", "orchestration-plan-v2.json"))
}

func m2dInputs(t *testing.T, platform string) ([]byte, []byte) {
	t.Helper()
	switch platform {
	case "thingsboard":
		return m2dReadFile(t, filepath.Join("testdata", "m2c", "synthetic-descriptor-thingsboard.json")),
			m2dReadFile(t, filepath.Join("testdata", "m2c", "synthetic-inventory-thingsboard.json"))
	case "fiware":
		return m2dReadFile(t, filepath.Join("testdata", "m2d", "descriptor-fiware.json")),
			m2dReadFile(t, filepath.Join("testdata", "m2d", "inventory-fiware.json"))
	case "edgex":
		return m2dReadFile(t, filepath.Join("testdata", "m2d", "descriptor-edgex.json")),
			m2dReadFile(t, filepath.Join("testdata", "m2d", "inventory-edgex.json"))
	default:
		t.Fatalf("unknown M2d platform fixture %q", platform)
		return nil, nil
	}
}

func m2dBuild(t *testing.T, platform string) *ExecutionPlan {
	t.Helper()
	descriptor, inventory := m2dInputs(t, platform)
	p, err := BuildExecutionPlan(descriptor, inventory, m2dPlanBytes(t))
	if err != nil {
		t.Fatalf("build %s execution plan: %v", platform, err)
	}
	return p
}

type m2dExpectedResources struct {
	appServices    int
	appReplicas    int
	appMemoryBytes int64
	supServices    int
	supReplicas    int
	supMemoryBytes int64
}

var m2dResourceExpectations = map[string]m2dExpectedResources{
	// M2c ThingsBoard descriptor: tb-http-transport is declared with 2 replicas.
	"thingsboard": {appServices: 3, appReplicas: 4, appMemoryBytes: 3221225472,
		supServices: 5, supReplicas: 5, supMemoryBytes: 4026531840},
	// FIWARE descriptor: orion, iot-agent; mongo, mongo-init, haproxy.
	"fiware": {appServices: 2, appReplicas: 2, appMemoryBytes: 1073741824,
		supServices: 3, supReplicas: 3, supMemoryBytes: 2684354560},
	// EdgeX descriptor: core-data, device-rest; database, nats, core-keeper,
	// core-common-config-bootstrapper, core-metadata, haproxy. Optional-profile
	// app-http-export and ui are not in the default resource declaration.
	"edgex": {appServices: 2, appReplicas: 2, appMemoryBytes: 1073741824,
		supServices: 6, supReplicas: 6, supMemoryBytes: 2818572288},
}

func TestM2DBuildAllPlatformFixturesDeterministic(t *testing.T) {
	wantSteps := []string{
		"preflight_validate",
		"compose_render_guard_reference",
		"provision_reference",
		"start_apply_reference",
		"health_validate",
		"evidence_package",
		"rollback_reference",
	}
	blockedSteps := map[string]bool{
		"provision_reference":   true,
		"start_apply_reference": true,
		"rollback_reference":    true,
	}
	requiredEvidence := []string{"descriptor", "inventory", "packet", "run_manifest", "sha256sums"}

	planBytes := m2dPlanBytes(t)
	plan, err := DecodeOrchestrationPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}

	for _, platform := range []string{"thingsboard", "fiware", "edgex"} {
		t.Run(platform, func(t *testing.T) {
			descriptor, inventory := m2dInputs(t, platform)
			p1, err := BuildExecutionPlan(descriptor, inventory, planBytes)
			if err != nil {
				t.Fatal(err)
			}
			p2, err := BuildExecutionPlan(descriptor, inventory, planBytes)
			if err != nil {
				t.Fatal(err)
			}
			if !p1.ExecutionBlocked || p1.Mode != ExecutionPlanMode || p1.Interpretation != ExecutionPlanInterpretation {
				t.Fatalf("plan-level execution boundary is wrong: %+v", p1)
			}
			if len(p1.Steps) != len(wantSteps) {
				t.Fatalf("step count = %d, want %d", len(p1.Steps), len(wantSteps))
			}
			for i, step := range p1.Steps {
				if step.Order != i+1 || step.Step != wantSteps[i] {
					t.Fatalf("step %d = %d/%s, want %d/%s", i, step.Order, step.Step, i+1, wantSteps[i])
				}
				if step.ExecutionStatus != ExecutionStatusNotExecuted {
					t.Fatalf("step %s status = %s, want %s", step.Step, step.ExecutionStatus, ExecutionStatusNotExecuted)
				}
				if blockedSteps[step.Step] {
					if !step.ExecutionBlockedHere || step.Marker != ExecutionBlockedMarker {
						t.Fatalf("execution step %s is not internally blocked: %+v", step.Step, step)
					}
					if len(step.BlockedActions) == 0 || step.BlockedReason == "" ||
						step.RequiredAuthorization != OrchestrationAuthorization {
						t.Fatalf("execution step %s lacks blocked reason/authorization: %+v", step.Step, step)
					}
				} else {
					if step.ExecutionBlockedHere || len(step.BlockedActions) != 0 ||
						step.Marker != ReadOnlyStepMarker || step.BlockedReason != "" || step.RequiredAuthorization != "" {
						t.Fatalf("read-only step %s is mislabeled: %+v", step.Step, step)
					}
				}
			}
			if !hasAction(p1.Steps[2].BlockedActions, ActionApply) && !hasAction(p1.Steps[2].BlockedActions, ActionScale) {
				t.Fatal("provision row does not reference Apply or Scale as blocked")
			}
			if !hasAction(p1.Steps[3].BlockedActions, ActionApply) {
				t.Fatal("start/apply row does not reference Apply as blocked")
			}
			if !hasAnyAction(p1.Steps[6].BlockedActions, ActionRestart, ActionStop, ActionReset, ActionDelete) {
				t.Fatal("rollback row does not reference a blocked restoration action")
			}

			// Expected services are derived from the orchestration v2 blueprint.
			bp := mustBlueprint(t, plan, platform)
			wantServices := len(bp.ApplicationServices) + len(bp.SupportingServices)
			if len(p1.ExpectedServices) != wantServices {
				t.Fatalf("expected services = %d, want %d from orchestration v2", len(p1.ExpectedServices), wantServices)
			}
			byName := map[string]ExpectedService{}
			for _, svc := range p1.ExpectedServices {
				byName[svc.Service] = svc
			}
			for _, svc := range bp.ApplicationServices {
				assertExpectedService(t, byName[svc.Service], svc, Application)
			}
			for _, svc := range bp.SupportingServices {
				assertExpectedService(t, byName[svc.Service], svc, Supporting)
			}
			assertExpectedServicesSorted(t, p1.ExpectedServices)

			// Resource guard is a descriptor declaration, not an observation.
			want := m2dResourceExpectations[platform]
			g := p1.ResourceGuard
			if g.Scope != ResourceGuardScope {
				t.Fatalf("resource guard scope = %q", g.Scope)
			}
			if g.Declared.ApplicationServices != want.appServices ||
				g.Declared.ApplicationReplicas != want.appReplicas ||
				g.Declared.ApplicationMemoryBytes != want.appMemoryBytes ||
				g.Declared.SupportingServices != want.supServices ||
				g.Declared.SupportingReplicas != want.supReplicas ||
				g.Declared.SupportingMemoryBytes != want.supMemoryBytes {
				t.Fatalf("declared resource summary = %+v, want %+v", g.Declared, want)
			}
			if g.Declared.TotalServices != want.appServices+want.supServices ||
				g.Declared.TotalReplicas != want.appReplicas+want.supReplicas ||
				g.Declared.TotalMemoryBytes != want.appMemoryBytes+want.supMemoryBytes {
				t.Fatalf("declared total resource summary = %+v", g.Declared)
			}
			if g.Observed.Status != NotAssessed || g.Observed.Source != ResourceObservedSource {
				t.Fatalf("observed resource state is not explicitly not-assessed: %+v", g.Observed)
			}

			// Packaging checklist and non-implementation status.
			e := p1.EvidenceChecklist
			if e.PackagingInterpretation != ArtifactManifestInterpretation ||
				e.PriorManifestVerification != string(PriorManifestVerificationNotImplemented) ||
				e.PriorManifestBindingStatus != NotAssessed ||
				e.LiveValidation != NotAssessed {
				t.Fatalf("evidence checklist metadata is wrong: %+v", e)
			}
			if len(e.Roles) != 17 {
				t.Fatalf("evidence role count = %d, want 17", len(e.Roles))
			}
			if len(e.RoleTaxonomy) != len(validArtifactRoles) {
				t.Fatalf("role taxonomy count = %d, want %d", len(e.RoleTaxonomy), len(validArtifactRoles))
			}
			required := 0
			for _, item := range e.Roles {
				if item.Status != NotAssessed || item.Reason == "" {
					t.Fatalf("evidence item is not an explicit checklist row: %+v", item)
				}
				if item.Required {
					required++
					if !containsString(requiredEvidence, item.Role) {
						t.Fatalf("unexpected required evidence role %q", item.Role)
					}
				}
			}
			if required != len(requiredEvidence) {
				t.Fatalf("required evidence count = %d, want %d", required, len(requiredEvidence))
			}

			// Deterministic canonical bytes, hash and Markdown.
			c1, err := p1.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			c2, err := p2.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(c1, c2) {
				t.Fatal("canonical JSON bytes differ across builds")
			}
			h1, err := p1.CanonicalSHA256()
			if err != nil {
				t.Fatal(err)
			}
			h2, err := p2.CanonicalSHA256()
			if err != nil {
				t.Fatal(err)
			}
			if h1 != h2 || h1 != SHA256Bytes(c1) {
				t.Fatalf("canonical hash = %s/%s, SHA-256(bytes)=%s", h1, h2, SHA256Bytes(c1))
			}
			md1 := RenderExecutionPlanMarkdown(p1)
			md2 := RenderExecutionPlanMarkdown(p2)
			if md1 != md2 {
				t.Fatal("Markdown render is not deterministic")
			}
			if !strings.Contains(md1, ExecutionBlockedMarker) || !strings.Contains(md1, "NOT EXECUTED") {
				t.Fatalf("Markdown does not carry blocked/not-executed markers:\n%s", md1)
			}
			if !strings.Contains(md1, "Prior-manifest verification: `NOT_IMPLEMENTED`") ||
				!strings.Contains(md1, "live validation: **NOT_ASSESSED**") {
				t.Fatalf("Markdown does not carry NOT_IMPLEMENTED/NOT_ASSESSED:\n%s", md1)
			}
			assertStepSection(t, md1, "start_apply_reference", true)
			assertStepSection(t, md1, "provision_reference", true)
			assertStepSection(t, md1, "rollback_reference", true)
			assertStepSection(t, md1, "preflight_validate", false)
			assertStepSection(t, md1, "compose_render_guard_reference", false)
			assertStepSection(t, md1, "health_validate", false)
			assertStepSection(t, md1, "evidence_package", false)

			// The pretty and canonical bytes round-trip through the strict decoder.
			for _, raw := range [][]byte{c1, mustMarshal(t, p1)} {
				decoded, err := DecodeExecutionPlan(raw)
				if err != nil {
					t.Fatalf("execution plan does not round-trip: %v", err)
				}
				again, err := decoded.CanonicalJSON()
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(again, c1) {
					t.Fatal("round-tripped execution plan is not canonical")
				}
			}
		})
	}
}

func TestM2DRejectsUnblockedOrMislabeledExecution(t *testing.T) {
	base := m2dBuild(t, "thingsboard")
	cases := []struct {
		name   string
		mutate func(*ExecutionPlan)
	}{
		{"plan execution_blocked false", func(p *ExecutionPlan) { p.ExecutionBlocked = false }},
		{"apply execution_blocked_here false", func(p *ExecutionPlan) { p.Steps[3].ExecutionBlockedHere = false }},
		{"apply mode read-only", func(p *ExecutionPlan) { p.Steps[3].Mode = StepModeReadOnly }},
		{"apply blocked actions empty", func(p *ExecutionPlan) { p.Steps[3].BlockedActions = nil }},
		{"apply marker read-only", func(p *ExecutionPlan) { p.Steps[3].Marker = ReadOnlyStepMarker }},
		{"apply status executed", func(p *ExecutionPlan) { p.Steps[3].ExecutionStatus = "EXECUTED" }},
		{"apply reason empty", func(p *ExecutionPlan) { p.Steps[3].BlockedReason = "" }},
		{"apply authorization missing", func(p *ExecutionPlan) { p.Steps[3].RequiredAuthorization = "" }},
		{"provision execution_blocked_here false", func(p *ExecutionPlan) { p.Steps[2].ExecutionBlockedHere = false }},
		{"provision blocked actions empty", func(p *ExecutionPlan) { p.Steps[2].BlockedActions = nil }},
		{"rollback execution_blocked_here false", func(p *ExecutionPlan) { p.Steps[6].ExecutionBlockedHere = false }},
		{"rollback blocked actions empty", func(p *ExecutionPlan) { p.Steps[6].BlockedActions = nil }},
		{"read-only step marked blocked", func(p *ExecutionPlan) { p.Steps[0].ExecutionBlockedHere = true }},
		{"read-only step carries blocked action", func(p *ExecutionPlan) { p.Steps[0].BlockedActions = []Action{ActionApply} }},
		{"read-only step uses blocked marker", func(p *ExecutionPlan) { p.Steps[0].Marker = ExecutionBlockedMarker }},
		{"read-only step carries authorization", func(p *ExecutionPlan) { p.Steps[0].RequiredAuthorization = OrchestrationAuthorization }},
		{"step order swapped", func(p *ExecutionPlan) { p.Steps[0], p.Steps[1] = p.Steps[1], p.Steps[0] }},
		{"step removed", func(p *ExecutionPlan) { p.Steps = p.Steps[:6] }},
		{"expected service health mode invented", func(p *ExecutionPlan) { p.ExpectedServices[0].HealthModes = []string{"healthy"} }},
		{"observed resource claimed passive", func(p *ExecutionPlan) { p.ResourceGuard.Observed.Status = Pass }},
		{"prior manifest verification claimed", func(p *ExecutionPlan) { p.EvidenceChecklist.PriorManifestVerification = "PASS" }},
		{"live validation claimed", func(p *ExecutionPlan) { p.EvidenceChecklist.LiveValidation = Pass }},
		{"required evidence fabricated", func(p *ExecutionPlan) { p.EvidenceChecklist.Roles[0].Status = Pass }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base.Canonical()
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("tampered plan was accepted: %+v", candidate)
			}
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeExecutionPlan(raw); err == nil {
				t.Fatal("strict decode accepted the tampered plan")
			}
		})
	}
}

func TestM2DRejectsMissingIncompleteAmbiguousInputs(t *testing.T) {
	descriptor, inventory := m2dInputs(t, "thingsboard")
	planBytes := m2dPlanBytes(t)
	if _, err := BuildExecutionPlan(nil, nil, nil); err == nil {
		t.Fatal("nil inputs were accepted")
	}
	if _, err := BuildExecutionPlan([]byte("{"), inventory, planBytes); err == nil {
		t.Fatal("invalid descriptor was accepted")
	}
	if _, err := BuildExecutionPlan(descriptor, []byte("{}"), planBytes); err == nil {
		t.Fatal("invalid inventory was accepted")
	}
	if _, err := BuildExecutionPlan(descriptor, inventory, []byte("{}")); err == nil {
		t.Fatal("invalid orchestration plan was accepted")
	}

	plan, err := DecodeOrchestrationPlan(planBytes)
	if err != nil {
		t.Fatal(err)
	}
	missingPlatform := *plan
	missingPlatform.Platforms = append([]PlatformBlueprint(nil), plan.Platforms[:2]...)
	if _, err := BuildExecutionPlan(descriptor, inventory, mustMarshal(t, missingPlatform)); err == nil {
		t.Fatal("plan missing a platform was accepted")
	}
	duplicatePlatform := *plan
	duplicatePlatform.Platforms = append(append([]PlatformBlueprint(nil), plan.Platforms...), plan.Platforms[0])
	if _, err := BuildExecutionPlan(descriptor, inventory, mustMarshal(t, duplicatePlatform)); err == nil {
		t.Fatal("plan with a duplicate platform was accepted")
	}
	reorderedPlan := *plan
	reorderedPlan.Stages = append([]OrchestrationStage(nil), plan.Stages...)
	reorderedPlan.Stages[0], reorderedPlan.Stages[1] = reorderedPlan.Stages[1], reorderedPlan.Stages[0]
	if _, err := BuildExecutionPlan(descriptor, inventory, mustMarshal(t, reorderedPlan)); err == nil {
		t.Fatal("plan with a reordered state machine was accepted")
	}

	d, err := DecodeDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	missingService := *d
	missingService.Services = append([]DescriptorService(nil), d.Services[1:]...)
	if _, err := BuildExecutionPlan(mustMarshal(t, missingService), inventory, planBytes); err == nil {
		t.Fatal("descriptor missing a default-start service was accepted")
	}
	unknownService := *d
	unknownService.Services = append(append([]DescriptorService(nil), d.Services...), DescriptorService{
		Service: "unknown-service", Class: Supporting, Replicas: 1, CPUSet: "1", MemoryBytes: 1,
	})
	if _, err := BuildExecutionPlan(mustMarshal(t, unknownService), inventory, planBytes); err == nil {
		t.Fatal("descriptor with an unknown service was accepted")
	}

	inv, err := DecodeInventory(inventory)
	if err != nil {
		t.Fatal(err)
	}
	unknownInventory := *inv
	unknownInventory.Services = append(append([]InventoryService(nil), inv.Services...), InventoryService{
		Project: inv.Project, Service: "unknown-service", Replicas: 1, CPUSet: "1",
		Containers: []InventoryContainer{{
			Project: inv.Project, Service: "unknown-service", ContainerID: "synthetic-unknown-0",
			Replica: 0, Image: "unknown:latest", StartedAtMS: 1700000000000, Health: "none", State: "running",
		}},
	})
	if _, err := BuildExecutionPlan(descriptor, mustMarshal(t, unknownInventory), planBytes); err == nil {
		t.Fatal("inventory with an undeclared service was accepted")
	}
	_, fiwareInventory := m2dInputs(t, "fiware")
	if _, err := BuildExecutionPlan(descriptor, fiwareInventory, planBytes); err == nil {
		t.Fatal("assembled inventory from a different project was accepted")
	}

	valid := m2dBuild(t, "thingsboard")
	incomplete := valid.Canonical()
	incomplete.Steps = incomplete.Steps[:6]
	if err := incomplete.Validate(); err == nil {
		t.Fatal("execution plan with a missing step was accepted")
	}
	duplicated := valid.Canonical()
	duplicated.Steps[1] = duplicated.Steps[0]
	if err := duplicated.Validate(); err == nil {
		t.Fatal("execution plan with a duplicated step was accepted")
	}
	ambiguous := valid.Canonical()
	ambiguous.Steps[2].PlanStage = StageDryRunRender
	if err := ambiguous.Validate(); err == nil {
		t.Fatal("execution plan with an ambiguous stage mapping was accepted")
	}
}

func TestM2DWriteNewDirectoryEnforcement(t *testing.T) {
	p := m2dBuild(t, "thingsboard")
	root := t.TempDir()
	out := filepath.Join(root, "out")
	if err := WriteExecutionPlan(out, p, nil); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	wantNames := []string{
		"SHA256SUMS",
		"deployment-execution-plan.canonical.json",
		"deployment-execution-plan.json",
		"deployment-execution-plan.md",
	}
	if !equalStrings(names, wantNames) {
		t.Fatalf("output files = %v, want %v", names, wantNames)
	}
	canonicalBytes, err := os.ReadFile(filepath.Join(out, "deployment-execution-plan.canonical.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantCanonical, err := p.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalBytes, wantCanonical) {
		t.Fatal("written canonical JSON differs from the canonical render")
	}
	hash, err := p.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if SHA256Bytes(canonicalBytes) != hash {
		t.Fatal("written canonical JSON hash differs from CanonicalSHA256")
	}
	md, err := os.ReadFile(filepath.Join(out, "deployment-execution-plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(md) != RenderExecutionPlanMarkdown(p) {
		t.Fatal("written Markdown differs from the deterministic render")
	}
	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) != 3 {
		t.Fatalf("SHA256SUMS lines = %d, want 3:\n%s", len(lines), sums)
	}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed SHA256SUMS line %q", line)
		}
		body, err := os.ReadFile(filepath.Join(out, fields[1]))
		if err != nil {
			t.Fatal(err)
		}
		if SHA256Bytes(body) != fields[0] {
			t.Fatalf("SHA256SUMS mismatch for %s", fields[1])
		}
	}

	if err := WriteExecutionPlan(out, p, nil); err == nil {
		t.Fatal("existing output directory was accepted")
	}
	inputDir := filepath.Join(root, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteExecutionPlan(filepath.Join(inputDir, "out"), p, []string{inputDir}); err == nil {
		t.Fatal("output under an input evidence tree was accepted")
	}
	if err := WriteExecutionPlan(inputDir, p, []string{inputDir}); err == nil {
		t.Fatal("output equal to an input path was accepted")
	}
	if err := WriteExecutionPlan(filepath.Join(root, "nil"), nil, nil); err == nil {
		t.Fatal("nil execution plan was accepted")
	}
}

func mustBlueprint(t *testing.T, plan *OrchestrationPlan, platform string) *PlatformBlueprint {
	t.Helper()
	for i := range plan.Platforms {
		if plan.Platforms[i].Platform == platform {
			return &plan.Platforms[i]
		}
	}
	t.Fatalf("platform %q not in plan", platform)
	return nil
}

func assertExpectedService(t *testing.T, got ExpectedService, want ServiceExpectation, tier ServiceClass) {
	t.Helper()
	if got.Service != want.Service || got.Tier != tier || got.Replicas != want.Replicas ||
		got.Lifecycle != want.Lifecycle || got.Activation != want.Activation || got.Healthcheck != want.Healthcheck {
		t.Fatalf("expected service mismatch: got %+v, want %+v tier %s", got, want, tier)
	}
	wantModes := healthModesFor(want)
	if !sameStringSet(got.HealthModes, wantModes) {
		t.Fatalf("health modes for %s = %v, want %v", got.Service, got.HealthModes, wantModes)
	}
}

func assertExpectedServicesSorted(t *testing.T, services []ExpectedService) {
	t.Helper()
	for i := 1; i < len(services); i++ {
		prev, cur := services[i-1], services[i]
		if tierRank[prev.Tier] > tierRank[cur.Tier] || (prev.Tier == cur.Tier && prev.Service > cur.Service) {
			t.Fatalf("expected services are not sorted at %d: %s then %s", i, prev.Service, cur.Service)
		}
	}
}

func assertStepSection(t *testing.T, markdown, step string, blocked bool) {
	t.Helper()
	needle := "### "
	start := strings.Index(markdown, needle)
	for start >= 0 {
		lineEnd := strings.Index(markdown[start:], "\n")
		if lineEnd < 0 {
			break
		}
		line := markdown[start : start+lineEnd]
		if strings.Contains(line, " "+step+" ") || strings.HasSuffix(line, " "+step) {
			end := strings.Index(markdown[start+lineEnd:], "\n### ")
			section := markdown[start:]
			if end >= 0 {
				section = markdown[start : start+lineEnd+end]
			}
			if blocked {
				if !strings.Contains(section, ExecutionBlockedMarker) || !strings.Contains(section, "NOT EXECUTED") {
					t.Fatalf("blocked section %s lacks markers:\n%s", step, section)
				}
			} else if strings.Contains(section, ExecutionBlockedMarker) {
				t.Fatalf("read-only section %s is mislabeled as blocked:\n%s", step, section)
			}
			return
		}
		next := strings.Index(markdown[start+len(needle):], needle)
		if next < 0 {
			break
		}
		start = start + len(needle) + next
	}
	t.Fatalf("Markdown section %s not found", step)
}

func hasAction(actions []Action, want Action) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}

func hasAnyAction(actions []Action, wants ...Action) bool {
	for _, want := range wants {
		if hasAction(actions, want) {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestM2DWriteCanonicalOutputs is an evidence helper, not a production path. It
// is skipped unless M2D_CANONICAL_OUT names a new directory. It writes only the
// deterministic renders for the three platform fixtures; it performs no Docker,
// SUT, lifecycle, load or observer action.
func TestM2DWriteCanonicalOutputs(t *testing.T) {
	out := os.Getenv("M2D_CANONICAL_OUT")
	if out == "" {
		t.Skip("M2D_CANONICAL_OUT not set")
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatalf("M2D_CANONICAL_OUT must name a new directory: %s", out)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	var summary strings.Builder
	planBytes := m2dPlanBytes(t)
	for _, platform := range []string{"thingsboard", "fiware", "edgex"} {
		descriptor, inventory := m2dInputs(t, platform)
		p, err := BuildExecutionPlan(descriptor, inventory, planBytes)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(out, platform)
		if err := WriteExecutionPlan(dir, p, nil); err != nil {
			t.Fatal(err)
		}
		canonical, err := p.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		hash, err := p.CanonicalSHA256()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&summary, "%s canonical_sha256=%s canonical_bytes=%d canonical_file=deployment-execution-plan.canonical.json\n",
			platform, hash, len(canonical))
		render := RenderExecutionPlanMarkdown(p)
		fmt.Fprintf(&summary, "%s markdown_sha256=%s markdown_bytes=%d markdown_file=deployment-execution-plan.md\n",
			platform, SHA256Bytes([]byte(render)), len(render))
	}
	if err := os.WriteFile(filepath.Join(out, "canonical-sha256.txt"), []byte(summary.String()), 0644); err != nil {
		t.Fatal(err)
	}
}
