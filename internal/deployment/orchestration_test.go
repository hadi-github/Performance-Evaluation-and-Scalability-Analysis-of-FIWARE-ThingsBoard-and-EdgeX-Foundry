package deployment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func validOrchestrationPlan() OrchestrationPlan {
	stateMachine := append([]StageName(nil), stageOrder...)
	return OrchestrationPlan{
		SchemaVersion:    OrchestrationSchemaVersion,
		Interpretation:   OrchestrationInterpretation,
		Mode:             OrchestrationMode,
		Authorization:    OrchestrationAuthorization,
		ExecutionBlocked: true,
		Stages: []OrchestrationStage{
			{
				Stage:              StagePreDeployInventory,
				Purpose:            "Read and validate the declared descriptor, inventory and unchanged stack files without contacting anything.",
				PermissibleActions: []Action{ActionRead, ActionValidate, ActionAttest},
				ExpectedEvidence:   []string{"descriptor", "inventory", "compose", "env", "makefile", "protocol", "profile"},
			},
			{
				Stage:                StageProvisionOnSUT,
				Purpose:              "Reference the future provisioned replica set. Provisioning is not performed by M2b.",
				PermissibleActions:   []Action{ActionValidate, ActionAttest},
				BlockedActions:       []Action{ActionApply, ActionScale},
				ExecutionBlockedHere: true,
				ExpectedEvidence:     []string{"orchestration_plan"},
			},
			{
				Stage:              StageDryRunRender,
				Purpose:            "Render the deterministic plan and attestation from caller-supplied documents.",
				PermissibleActions: []Action{ActionRead, ActionValidate, ActionRenderDryRun, ActionAttest},
				ExpectedEvidence:   []string{"orchestration_plan"},
			},
			{
				Stage:                StageApply,
				Purpose:              "Reference only: a future, separately authorized deployment would be applied here.",
				PermissibleActions:   []Action{ActionAttest},
				BlockedActions:       []Action{ActionApply, ActionScale, ActionRestart, ActionStop},
				ExecutionBlockedHere: true,
				ExpectedEvidence:     []string{"orchestration_plan"},
			},
			{
				Stage:              StageValidateHealth,
				Purpose:            "Compare caller-supplied health and inventory attestations with the declaration.",
				PermissibleActions: []Action{ActionRead, ActionValidate, ActionAttest},
				ExpectedEvidence:   []string{"inventory", "monitoring_export", "log"},
			},
			{
				Stage:              StageEvidence,
				Purpose:            "Package hashes, logs and preserved ledgers without rewriting any prior artifact.",
				PermissibleActions: []Action{ActionRead, ActionValidate, ActionAttest},
				ExpectedEvidence:   []string{"run_manifest", "k6_summary", "sink_ledger", "adapter_ledger", "receipt_ledger", "sha256sums", "packet"},
			},
			{
				Stage:                StageRollback,
				Purpose:              "Reference only: restoration rules and the interrupt exception; no restoration is executed.",
				PermissibleActions:   []Action{ActionRead, ActionValidate, ActionAttest},
				BlockedActions:       []Action{ActionRestart, ActionStop, ActionReset, ActionDelete},
				ExecutionBlockedHere: true,
				ExpectedEvidence:     []string{"run_manifest", "log"},
			},
		},
		Platforms: []PlatformBlueprint{
			{
				Platform:     "thingsboard",
				Project:      "tb",
				StateMachine: append([]StageName(nil), stateMachine...),
				ApplicationServices: []ServiceExpectation{
					persistentDefault("tb-core", 1, HealthcheckNone),
					persistentDefault("tb-rule-engine", 1, HealthcheckNone),
					persistentDefault("tb-http-transport", 1, HealthcheckNone),
				},
				SupportingServices: []ServiceExpectation{
					persistentDefault("zookeeper", 1, HealthcheckNone),
					persistentDefault("kafka", 1, HealthcheckDeclared),
					persistentDefault("postgres", 1, HealthcheckDeclared),
					persistentDefault("valkey", 1, HealthcheckDeclared),
					persistentDefault("haproxy", 1, HealthcheckNone),
					oneShotOptional("tb-install"),
				},
				HealthChecks: []string{
					"explicit healthchecks (kafka, postgres, valkey) must report healthy; unhealthy is FAIL and missing health evidence is UNAVAILABLE",
					"services with healthcheck:none (tb-core, tb-rule-engine, tb-http-transport, zookeeper, haproxy) require running plus functional evidence, never a synthetic healthy value",
					"tb-install is one_shot behind the install profile: not started by default and NOT_ASSESSED unless activated; when activated it must exit 0",
					"declared replica counts match observed contiguous replicas for persistent default-start services",
					"haproxy stats endpoint readable read-only",
				},
				AllowedEvidenceRoles:     []string{"descriptor", "inventory", "compose", "env", "makefile", "protocol", "profile", "run_manifest", "monitoring_export", "log", "sha256sums", "packet"},
				RequiredAttestationRoles: []string{"descriptor", "inventory", "run_manifest", "sha256sums", "packet"},
				MissingEvidenceStatus:    Unavailable,
			},
			{
				Platform:     "fiware",
				Project:      "fiware",
				StateMachine: append([]StageName(nil), stateMachine...),
				ApplicationServices: []ServiceExpectation{
					persistentDefault("orion", 1, HealthcheckNone),
					persistentDefault("iot-agent", 1, HealthcheckNone),
				},
				SupportingServices: []ServiceExpectation{
					persistentDefault("mongo", 1, HealthcheckDeclared),
					oneShotDefault("mongo-init"),
					persistentDefault("haproxy", 1, HealthcheckNone),
				},
				HealthChecks: []string{
					"mongo explicit healthcheck must report healthy; unhealthy is FAIL and missing health evidence is UNAVAILABLE",
					"mongo-init is a default-start one_shot job: successful completion (exit 0) is required, running/healthy is not applicable, and a missing exit code is UNAVAILABLE",
					"orion, iot-agent and haproxy have healthcheck:none and require running plus functional evidence, never a synthetic healthy value",
					"declared replica counts match observed contiguous replicas",
				},
				AllowedEvidenceRoles:     []string{"descriptor", "inventory", "compose", "env", "makefile", "protocol", "profile", "run_manifest", "monitoring_export", "log", "sha256sums", "packet"},
				RequiredAttestationRoles: []string{"descriptor", "inventory", "run_manifest", "sha256sums", "packet"},
				MissingEvidenceStatus:    NotAssessed,
			},
			{
				Platform:     "edgex",
				Project:      "edgex",
				StateMachine: append([]StageName(nil), stateMachine...),
				ApplicationServices: []ServiceExpectation{
					persistentDefault("core-data", 1, HealthcheckNone),
					persistentDefault("device-rest", 1, HealthcheckNone),
				},
				SupportingServices: []ServiceExpectation{
					persistentDefault("database", 1, HealthcheckDeclared),
					persistentDefault("nats", 1, HealthcheckDeclared),
					persistentDefault("core-keeper", 1, HealthcheckNone),
					oneShotDefault("core-common-config-bootstrapper"),
					persistentDefault("core-metadata", 1, HealthcheckNone),
					persistentDefault("haproxy", 1, HealthcheckNone),
					persistentOptional("app-http-export"),
					persistentOptional("ui"),
				},
				HealthChecks: []string{
					"database and nats explicit healthchecks must report healthy; unhealthy is FAIL and missing health evidence is UNAVAILABLE",
					"core-common-config-bootstrapper is a default-start one_shot job: successful completion (exit 0) is required, running/healthy is not applicable, and a missing exit code is UNAVAILABLE",
					"core-keeper, core-metadata, core-data, device-rest and haproxy have healthcheck:none and require running plus functional evidence, never a synthetic healthy value",
					"app-http-export and ui are optional_profile declarations and are NOT_ASSESSED unless explicitly activated",
					"declared replica counts match observed contiguous replicas",
				},
				AllowedEvidenceRoles:     []string{"descriptor", "inventory", "compose", "env", "makefile", "protocol", "profile", "run_manifest", "monitoring_export", "log", "sha256sums", "packet"},
				RequiredAttestationRoles: []string{"descriptor", "inventory", "run_manifest", "sha256sums", "packet"},
				MissingEvidenceStatus:    Unavailable,
			},
		},
	}
}

func persistentDefault(service string, replicas int, healthcheck HealthcheckDeclaration) ServiceExpectation {
	return ServiceExpectation{
		Service: service, Replicas: replicas, Lifecycle: LifecyclePersistent,
		Activation: ActivationDefaultStart, Healthcheck: healthcheck,
	}
}

func oneShotDefault(service string) ServiceExpectation {
	return ServiceExpectation{
		Service: service, Replicas: 1, Lifecycle: LifecycleOneShot,
		Activation: ActivationDefaultStart, Healthcheck: HealthcheckNone,
	}
}

func oneShotOptional(service string) ServiceExpectation {
	return ServiceExpectation{
		Service: service, Replicas: 1, Lifecycle: LifecycleOneShot,
		Activation: ActivationOptionalProfile, Healthcheck: HealthcheckNone,
	}
}

func persistentOptional(service string) ServiceExpectation {
	return ServiceExpectation{
		Service: service, Replicas: 1, Lifecycle: LifecyclePersistent,
		Activation: ActivationOptionalProfile, Healthcheck: HealthcheckNone,
	}
}

func marshalPlan(t *testing.T, p OrchestrationPlan) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOrchestrationPlanValidAndDeterministic(t *testing.T) {
	plan := validOrchestrationPlan()
	b := marshalPlan(t, plan)
	decoded, err := DecodeOrchestrationPlan(b)
	if err != nil {
		t.Fatalf("decode valid plan: %v", err)
	}
	hash, err := decoded.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Fatalf("canonical hash length %d", len(hash))
	}
	rendered := RenderOrchestrationPlanMarkdown(decoded)
	for _, want := range []string{
		"Deployment orchestration blueprint (offline)",
		ExecutionBlockedMarker,
		"Execution blocked: true",
		"pre_deploy_inventory -> provision_on_sut -> dry_run_render -> apply -> validate_health -> evidence -> rollback",
		"Missing evidence semantics: **UNAVAILABLE**",
		"Missing evidence semantics: **NOT_ASSESSED**",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render missing %q\n%s", want, rendered)
		}
	}
	if again := RenderOrchestrationPlanMarkdown(decoded); again != rendered {
		t.Fatal("render is not deterministic")
	}
}

func TestOrchestrationPlanCanonicalOrderInvariance(t *testing.T) {
	plan := validOrchestrationPlan()
	plan.Stages[0].PermissibleActions = []Action{ActionAttest, ActionRead, ActionValidate}
	plan.Platforms[0].ApplicationServices[0], plan.Platforms[0].ApplicationServices[2] =
		plan.Platforms[0].ApplicationServices[2], plan.Platforms[0].ApplicationServices[0]
	plan.Platforms[0], plan.Platforms[2] = plan.Platforms[2], plan.Platforms[0]
	first, err := validOrchestrationPlan().CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	second, err := plan.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical hash changed with declaration order: %s != %s", first, second)
	}
}

func TestDecodeOrchestrationPlanRejectsUnsafeTaxonomy(t *testing.T) {
	plan := validOrchestrationPlan()
	plan.Stages[0].PermissibleActions = append(plan.Stages[0].PermissibleActions, ActionApply)
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "not in the read-only taxonomy") {
		t.Fatalf("unsafe action accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	plan.Stages[0].BlockedActions = []Action{ActionLoad}
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "execution_blocked_here=true") {
		t.Fatalf("blocked action without marker accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	plan.Stages[0].BlockedActions = []Action{ActionRead}
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "not a lifecycle action") {
		t.Fatalf("safe action in blocked list accepted or wrong error: %v", err)
	}
}

func TestDecodeOrchestrationPlanRequiresApplyBlock(t *testing.T) {
	plan := validOrchestrationPlan()
	for i := range plan.Stages {
		if plan.Stages[i].Stage == StageApply {
			plan.Stages[i].ExecutionBlockedHere = false
			plan.Stages[i].BlockedActions = nil
		}
	}
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "apply must be marked") {
		t.Fatalf("apply without marker accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	for i := range plan.Stages {
		if plan.Stages[i].Stage == StageApply {
			plan.Stages[i].BlockedActions = []Action{ActionScale}
		}
	}
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "reference Apply") {
		t.Fatalf("apply without Apply blocked reference accepted or wrong error: %v", err)
	}
}

func TestDecodeOrchestrationPlanRejectsBadPlatformAndStageOrder(t *testing.T) {
	plan := validOrchestrationPlan()
	plan.Stages[0], plan.Stages[1] = plan.Stages[1], plan.Stages[0]
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "stage 0 must be") {
		t.Fatalf("stage order accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	for i := range plan.Stages {
		if plan.Stages[i].Stage == StageProvisionOnSUT || plan.Stages[i].Stage == StageRollback {
			plan.Stages[i].ExecutionBlockedHere = false
			plan.Stages[i].BlockedActions = nil
		}
	}
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "must be marked") {
		t.Fatalf("provision/rollback without marker accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	plan.Platforms = plan.Platforms[:2]
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "must declare its state machine") {
		t.Fatalf("missing platform accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	plan.Platforms[0].Project = "fiware"
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "existing compose project") {
		t.Fatalf("project substitution accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	plan.ExecutionBlocked = false
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "execution_blocked must be true") {
		t.Fatalf("executable plan accepted or wrong error: %v", err)
	}

	plan = validOrchestrationPlan()
	plan.Platforms[0].MissingEvidenceStatus = Pass
	if _, err := DecodeOrchestrationPlan(marshalPlan(t, plan)); err == nil || !strings.Contains(err.Error(), "never PASS or FAIL") {
		t.Fatalf("PASS missing-evidence status accepted or wrong error: %v", err)
	}
}

func TestOrchestrationPlanRepresentsCurrentComposeServices(t *testing.T) {
	want := map[string][]string{
		"thingsboard": {
			"tb-core", "tb-rule-engine", "tb-http-transport",
			"zookeeper", "kafka", "postgres", "valkey", "haproxy", "tb-install",
		},
		"fiware": {
			"orion", "iot-agent",
			"mongo", "mongo-init", "haproxy",
		},
		"edgex": {
			"core-data", "device-rest",
			"database", "nats", "core-keeper", "core-common-config-bootstrapper",
			"core-metadata", "haproxy", "app-http-export", "ui",
		},
	}
	plan := validOrchestrationPlan()
	if len(plan.Platforms) != len(want) {
		t.Fatalf("platform count %d, want %d", len(plan.Platforms), len(want))
	}
	for _, platform := range plan.Platforms {
		var got []string
		for _, svc := range platform.ApplicationServices {
			got = append(got, svc.Service)
		}
		for _, svc := range platform.SupportingServices {
			got = append(got, svc.Service)
		}
		sort.Strings(got)
		expected := append([]string(nil), want[platform.Platform]...)
		sort.Strings(expected)
		if strings.Join(got, ",") != strings.Join(expected, ",") {
			t.Fatalf("%s services = %v, want %v", platform.Platform, got, expected)
		}
	}

	// The dependency-required one-shot jobs and optional installer must not be
	// forgotten or silently treated as persistent.
	assertService := func(platform, service string, lifecycle ServiceLifecycle, activation ServiceActivation, healthcheck HealthcheckDeclaration) {
		t.Helper()
		for _, p := range plan.Platforms {
			if p.Platform != platform {
				continue
			}
			for _, svc := range append(append([]ServiceExpectation{}, p.ApplicationServices...), p.SupportingServices...) {
				if svc.Service != service {
					continue
				}
				if svc.Lifecycle != lifecycle || svc.Activation != activation || svc.Healthcheck != healthcheck {
					t.Fatalf("%s/%s = %s/%s/%s, want %s/%s/%s", platform, service,
						svc.Lifecycle, svc.Activation, svc.Healthcheck, lifecycle, activation, healthcheck)
				}
				return
			}
		}
		t.Fatalf("%s service %q not declared", platform, service)
	}
	assertService("fiware", "mongo-init", LifecycleOneShot, ActivationDefaultStart, HealthcheckNone)
	assertService("edgex", "core-common-config-bootstrapper", LifecycleOneShot, ActivationDefaultStart, HealthcheckNone)
	assertService("thingsboard", "tb-install", LifecycleOneShot, ActivationOptionalProfile, HealthcheckNone)
	assertService("edgex", "app-http-export", LifecyclePersistent, ActivationOptionalProfile, HealthcheckNone)
	assertService("edgex", "ui", LifecyclePersistent, ActivationOptionalProfile, HealthcheckNone)
}

func TestServiceExpectationPredicatePersistentAndOneShot(t *testing.T) {
	healthy := persistentDefault("postgres", 1, HealthcheckDeclared)
	exitZero := 0
	exitOne := 1

	basePersistent := ServiceObservation{Service: "postgres", Present: true, Replicas: 1, Running: true}

	cases := []struct {
		name string
		exp  ServiceExpectation
		obs  ServiceObservation
		want Status
	}{
		{"explicit healthcheck healthy", healthy, withHealth(basePersistent, ObservedHealthy), Pass},
		{"explicit healthcheck unhealthy", healthy, withHealth(basePersistent, ObservedUnhealthy), Fail},
		{"explicit healthcheck missing", healthy, withHealth(basePersistent, ObservedHealthNone), Unavailable},
		{"explicit healthcheck unknown", healthy, withHealth(basePersistent, ObservedUnknown), Unavailable},
		{"persistent not running", healthy, func() ServiceObservation {
			o := withHealth(basePersistent, ObservedHealthy)
			o.Running = false
			return o
		}(), Fail},
		{"replica mismatch", healthy, func() ServiceObservation {
			o := withHealth(basePersistent, ObservedHealthy)
			o.Replicas = 2
			return o
		}(), Fail},
		{"health none with functional evidence", persistentDefault("tb-core", 1, HealthcheckNone), ServiceObservation{
			Service: "tb-core", Present: true, Replicas: 1, Running: true,
			Health: ObservedHealthNone, FunctionalEvidence: true,
		}, Pass},
		{"health none without functional evidence", persistentDefault("tb-core", 1, HealthcheckNone), ServiceObservation{
			Service: "tb-core", Present: true, Replicas: 1, Running: true,
			Health: ObservedHealthNone,
		}, Unavailable},
		{"health none is not a failed check", persistentDefault("tb-core", 1, HealthcheckNone), ServiceObservation{
			Service: "tb-core", Present: true, Replicas: 1, Running: true,
			Health: ObservedUnhealthy, FunctionalEvidence: true,
		}, Pass},
		{"one shot successful exit", oneShotDefault("mongo-init"), ServiceObservation{
			Service: "mongo-init", Present: true, Replicas: 1, Health: ObservedHealthNone, ExitCode: &exitZero,
		}, Pass},
		{"one shot failed exit", oneShotDefault("mongo-init"), ServiceObservation{
			Service: "mongo-init", Present: true, Replicas: 1, Health: ObservedHealthNone, ExitCode: &exitOne,
		}, Fail},
		{"one shot running without exit is not completion", oneShotDefault("mongo-init"), ServiceObservation{
			Service: "mongo-init", Present: true, Replicas: 1, Running: true, Health: ObservedHealthy,
		}, Unavailable},
		{"one shot missing observation", oneShotDefault("mongo-init"), ServiceObservation{}, Unavailable},
		{"optional one shot not activated", oneShotOptional("tb-install"), ServiceObservation{}, NotAssessed},
		{"optional persistent not activated", persistentOptional("ui"), ServiceObservation{}, NotAssessed},
		{"optional one shot activated and successful", oneShotOptional("tb-install"), ServiceObservation{
			Service: "tb-install", Present: true, Replicas: 1, ExitCode: &exitZero,
		}, Pass},
		{"default service missing observation", persistentDefault("orion", 1, HealthcheckNone), ServiceObservation{}, Unavailable},
	}
	for _, tc := range cases {
		got, reason := EvaluateServiceExpectation(tc.exp, tc.obs)
		if got != tc.want {
			t.Fatalf("%s: got %s (%s), want %s", tc.name, got, reason, tc.want)
		}
		if strings.TrimSpace(reason) == "" {
			t.Fatalf("%s: empty reason", tc.name)
		}
	}
}

func withHealth(o ServiceObservation, health ObservedHealth) ServiceObservation {
	o.Health = health
	return o
}

func TestDecodeOrchestrationPlanRejectsBadLifecycleSemantics(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*OrchestrationPlan)
		want   string
	}{
		{"unknown lifecycle", func(p *OrchestrationPlan) {
			p.Platforms[0].ApplicationServices[0].Lifecycle = "daemon"
		}, "lifecycle"},
		{"unknown activation", func(p *OrchestrationPlan) {
			p.Platforms[0].ApplicationServices[0].Activation = "auto"
		}, "activation"},
		{"unknown healthcheck", func(p *OrchestrationPlan) {
			p.Platforms[0].ApplicationServices[0].Healthcheck = "maybe"
		}, "healthcheck"},
		{"one shot replicas", func(p *OrchestrationPlan) {
			p.Platforms[1].SupportingServices[1].Replicas = 2
		}, "exactly one replica"},
		{"missing default-start one shot", func(p *OrchestrationPlan) {
			services := p.Platforms[1].SupportingServices
			p.Platforms[1].SupportingServices = append(services[:1], services[2:]...)
		}, "must declare the default-start service \"mongo-init\""},
		{"predecessor schema version", func(p *OrchestrationPlan) {
			p.SchemaVersion = 1
		}, "unsupported schema_version"},
	}
	for _, tc := range cases {
		plan := validOrchestrationPlan()
		tc.mutate(&plan)
		_, err := DecodeOrchestrationPlan(marshalPlan(t, plan))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s accepted or wrong error: %v", tc.name, err)
		}
	}
}

func TestOrchestrationPlanFixtureDecodes(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("source location unavailable")
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", "orchestration-plan-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := DecodeOrchestrationPlan(b)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if plan.SchemaVersion != OrchestrationSchemaVersion {
		t.Fatalf("fixture schema version %d", plan.SchemaVersion)
	}
	if _, err := plan.CanonicalSHA256(); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeOrchestrationPlanStrictKeys(t *testing.T) {
	base := string(marshalPlan(t, validOrchestrationPlan()))
	cases := map[string]string{
		"case alias":    strings.Replace(base, `"mode":`, `"MODE":`, 1),
		"unknown key":   strings.Replace(base, `{"schema_version"`, `{"unexpected":1,"schema_version"`, 1),
		"duplicate":     strings.Replace(base, `{"schema_version"`, `{"mode":"offline-blueprint-only","schema_version"`, 1),
		"null field":    strings.Replace(base, `"authorization":"`+OrchestrationAuthorization+`"`, `"authorization":null`, 1),
		"trailing JSON": base + `{}`,
	}
	for name, body := range cases {
		if _, err := DecodeOrchestrationPlan([]byte(body)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
