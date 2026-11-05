package deployment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func hx(c byte) string { return strings.Repeat(string(c), 64) }

func fixtureDescriptor() Descriptor {
	return Descriptor{
		SchemaVersion: SchemaVersion,
		Platform:      "thingsboard",
		Project:       "tb",
		Allocation: Allocation{
			ApplicationCPUSets: []string{"4-7"},
			SupportingCPUSets:  []string{"1", "2-3"},
		},
		Protocol: ProtocolBinding{
			BindingSHA256: hx('a'),
			ProfileSHA256: hx('b'),
			ProfilePath:   "research/protocols/profile.json",
		},
		Provenance: Provenance{
			Source:        "deploy/thingsboard/compose.yaml",
			ComposePath:   "deploy/thingsboard/compose.yaml",
			ComposeSHA256: hx('c'),
			Documents: []EvidenceDocument{{
				Role: "compose", Path: "deploy/thingsboard/compose.yaml", SHA256: hx('c'),
			}},
		},
		EffectiveConfiguration: &EffectiveConfiguration{Path: "config/effective.yaml", SHA256: hx('d')},
		Services: []DescriptorService{
			{Service: "tb-http-transport", Class: Application, Replicas: 2, CPUSet: "4-5", MemoryBytes: 536870912, Image: "thingsboard/tb-http-transport:4.3.1.6", Digest: "sha256:" + hx('e')},
			{Service: "tb-core", Class: Application, Replicas: 1, CPUSet: "6", MemoryBytes: 1073741824, Image: "thingsboard/tb-node:4.3.1.6", Digest: "sha256:" + hx('f')},
			{Service: "postgres", Class: Supporting, Replicas: 1, CPUSet: "2-3", MemoryBytes: 1610612736, Image: "postgres:16", Digest: "sha256:" + hx('1')},
			{Service: "haproxy", Class: Supporting, Replicas: 1, CPUSet: "1", MemoryBytes: 268435456},
		},
	}
}

func fixtureInventory() Inventory {
	return Inventory{
		SchemaVersion:          SchemaVersion,
		Project:                "tb",
		Source:                 "caller-supplied-fixture",
		EffectiveConfiguration: &EffectiveConfiguration{Path: "config/effective.yaml", SHA256: hx('d')},
		Services: []InventoryService{
			{
				Project: "tb", Service: "tb-http-transport", Replicas: 2, CPUSet: "4-5",
				Containers: []InventoryContainer{
					{Project: "tb", Service: "tb-http-transport", ContainerID: "tb-http-0", Replica: 0, Image: "thingsboard/tb-http-transport:4.3.1.6", Digest: "sha256:" + hx('e'), StartedAtMS: 1000, Health: "healthy", State: "running"},
					{Project: "tb", Service: "tb-http-transport", ContainerID: "tb-http-1", Replica: 1, Image: "thingsboard/tb-http-transport:4.3.1.6", Digest: "sha256:" + hx('e'), StartedAtMS: 1001, Health: "healthy", State: "running"},
				},
			},
			{
				Project: "tb", Service: "tb-core", Replicas: 1, CPUSet: "6",
				Containers: []InventoryContainer{
					{Project: "tb", Service: "tb-core", ContainerID: "tb-core-0", Replica: 0, Image: "thingsboard/tb-node:4.3.1.6", Digest: "sha256:" + hx('f'), StartedAtMS: 1002, Health: "healthy", State: "running"},
				},
			},
			{
				Project: "tb", Service: "postgres", Replicas: 1, CPUSet: "2-3",
				Containers: []InventoryContainer{
					{Project: "tb", Service: "postgres", ContainerID: "postgres-0", Replica: 0, Image: "postgres:16", Digest: "sha256:" + hx('1'), StartedAtMS: 1003, Health: "healthy", State: "running"},
				},
			},
			{
				Project: "tb", Service: "haproxy", Replicas: 1, CPUSet: "1",
				Containers: []InventoryContainer{
					{Project: "tb", Service: "haproxy", ContainerID: "haproxy-0", Replica: 0, Image: "haproxy:3.2-alpine", StartedAtMS: 1004, Health: "healthy", State: "running"},
				},
			},
		},
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(mustJSON(t, v), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func serviceMaps(t *testing.T, m map[string]any) []any {
	t.Helper()
	s, ok := m["services"].([]any)
	if !ok {
		t.Fatal("fixture has no services array")
	}
	return s
}

func TestDecodeDescriptorValidAndCanonicalPermutation(t *testing.T) {
	d := fixtureDescriptor()
	raw := mustJSON(t, d)
	decoded, err := DecodeDescriptor(raw)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := decoded.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if canonical == "" {
		t.Fatal("empty canonical hash")
	}

	permuted := d
	permuted.Services = []DescriptorService{d.Services[3], d.Services[1], d.Services[0], d.Services[2]}
	permuted.Allocation.ApplicationCPUSets = []string{"4-7"}
	permuted.Allocation.SupportingCPUSets = []string{"2-3", "1"}
	permuted.Provenance.Documents = append([]EvidenceDocument(nil), d.Provenance.Documents...)
	pc, err := permuted.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if pc != canonical {
		t.Fatalf("canonical descriptor hash depends on array order: %s != %s", pc, canonical)
	}
	if SHA256Bytes(raw) == SHA256Bytes(mustJSON(t, permuted)) {
		t.Fatal("raw descriptor bytes should differ even though the canonical hash is equal")
	}
}

func TestDecodeDescriptorStrictAndSemanticRejections(t *testing.T) {
	base := asMap(t, fixtureDescriptor())
	cases := []struct {
		name string
		raw  []byte
	}{
		{"unknown field", withMap(base, func(m map[string]any) { m["surprise"] = 1 })},
		{"unknown nested field", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["surprise"] = 1 })},
		{"null optional", withMap(base, func(m map[string]any) { m["effective_configuration"] = nil })},
		{"null nested", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["image"] = nil })},
		{"unsupported platform", withMap(base, func(m map[string]any) { m["platform"] = "openstack" })},
		{"missing platform", withMap(base, func(m map[string]any) { delete(m, "platform") })},
		{"bad project", withMap(base, func(m map[string]any) { m["project"] = "bad project" })},
		{"duplicate service", withMap(base, func(m map[string]any) {
			svcs := serviceMaps(t, m)
			svcs[1].(map[string]any)["service"] = "tb-http-transport"
		})},
		{"bad class", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["class"] = "measured" })},
		{"zero replicas", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["replicas"] = 0 })},
		{"bad cpuset syntax", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["cpuset"] = "4..5" })},
		{"cpuset not allocated", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["cpuset"] = "9" })},
		{"zero memory", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["memory_bytes"] = 0 })},
		{"bad digest", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["digest"] = "sha256:abc" })},
		{"bad protocol hash", withMap(base, func(m map[string]any) { m["protocol"].(map[string]any)["binding_sha256"] = "abc" })},
		{"missing provenance source", withMap(base, func(m map[string]any) { delete(m["provenance"].(map[string]any), "source") })},
		{"bad evidence role", withMap(base, func(m map[string]any) {
			doc := m["provenance"].(map[string]any)["documents"].([]any)[0].(map[string]any)
			doc["role"] = "container_command"
		})},
		{"empty allocation", withMap(base, func(m map[string]any) { m["allocation"].(map[string]any)["application_cpusets"] = []any{} })},
		{"duplicate key", []byte(`{"schema_version":1,"schema_version":1}`)},
		{"trailing json", append(mustJSON(t, fixtureDescriptor()), []byte(` {}`)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeDescriptor(tc.raw); err == nil {
				t.Fatalf("accepted malformed descriptor: %s", tc.raw)
			}
		})
	}
}

func TestDecodeInventoryStrictAndSemanticRejections(t *testing.T) {
	base := asMap(t, fixtureInventory())
	cases := []struct {
		name string
		raw  []byte
	}{
		{"unknown field", withMap(base, func(m map[string]any) { m["surprise"] = true })},
		{"unknown nested field", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["surprise"] = true
		})},
		{"null captured timestamp", withMap(base, func(m map[string]any) { m["captured_at_ms"] = nil })},
		{"null nested", withMap(base, func(m map[string]any) {
			serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)["health"] = nil
		})},
		{"duplicate service", withMap(base, func(m map[string]any) { serviceMaps(t, m)[1].(map[string]any)["service"] = "tb-http-transport" })},
		{"duplicate container id", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[1].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["container_id"] = "tb-http-0"
		})},
		{"duplicate replica", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[1].(map[string]any)
			c["replica"] = 0
		})},
		{"replica mismatch", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["replicas"] = 3 })},
		{"incomplete replica set", withMap(base, func(m map[string]any) {
			svc := serviceMaps(t, m)[0].(map[string]any)
			svc["containers"] = svc["containers"].([]any)[:1]
			svc["replicas"] = 2
		})},
		{"service project mismatch", withMap(base, func(m map[string]any) { serviceMaps(t, m)[0].(map[string]any)["project"] = "other" })},
		{"container project mismatch", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["project"] = "other"
		})},
		{"container service mismatch", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["service"] = "tb-core"
		})},
		{"empty containers", withMap(base, func(m map[string]any) {
			svc := serviceMaps(t, m)[0].(map[string]any)
			svc["containers"] = []any{}
			svc["replicas"] = 1
		})},
		{"missing state", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			delete(c, "state")
		})},
		{"empty state", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["state"] = ""
		})},
		{"unexpected state", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["state"] = "zombie"
		})},
		{"unreasoned unhealthy", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["health"] = "unhealthy"
		})},
		{"unreasoned non-running", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["state"] = "exited"
		})},
		{"missing started at", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			delete(c, "started_at_ms")
		})},
		{"zero started at", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["started_at_ms"] = 0
		})},
		{"bad digest", withMap(base, func(m map[string]any) {
			c := serviceMaps(t, m)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)
			c["digest"] = "sha256:no"
		})},
		{"empty source", withMap(base, func(m map[string]any) { m["source"] = "" })},
		{"duplicate key", []byte(`{"schema_version":1,"schema_version":1}`)},
		{"trailing json", append(mustJSON(t, fixtureInventory()), []byte(` {}`)...)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeInventory(tc.raw); err == nil {
				t.Fatalf("accepted malformed inventory: %s", tc.raw)
			}
		})
	}
}

func TestValidatePassAndExplicitStates(t *testing.T) {
	d := fixtureDescriptor()
	inv := fixtureInventory()
	report, err := Validate(d, inv, ValidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Attestation != Pass {
		t.Fatalf("attestation = %s; checks: %+v", report.Attestation, report.Checks)
	}
	if report.LiveValidation != NotAssessed {
		t.Fatalf("live validation = %s, want NOT_ASSESSED", report.LiveValidation)
	}
	if report.Observer.Status != NotAssessed {
		t.Fatalf("missing observer = %s, want NOT_ASSESSED", report.Observer.Status)
	}
	if report.Observer.InventorySHA256 != "" || report.Observer.Name != "" {
		t.Fatal("missing observer must not fabricate an observer inventory or name")
	}
	if !report.DryRun {
		t.Fatal("report must be dry run")
	}
	if report.InventorySource != inv.Source {
		t.Fatalf("inventory source = %q, want %q", report.InventorySource, inv.Source)
	}
	if report.InventoryCapturedAtMS != nil {
		t.Fatal("report fabricated an inventory capture timestamp")
	}
	if report.Provenance.Source != d.Provenance.Source {
		t.Fatalf("report provenance = %q, want %q", report.Provenance.Source, d.Provenance.Source)
	}
	if report.Totals.RequiredApplicationReplicas != 3 || report.Totals.ObservedApplicationReplicas != 3 {
		t.Fatalf("application totals = %+v", report.Totals)
	}
	if report.Totals.RequiredSupportingReplicas != 2 || report.Totals.ObservedSupportingReplicas != 2 {
		t.Fatalf("supporting totals = %+v", report.Totals)
	}
	if report.Totals.ApplicationStatus != Pass || report.Totals.SupportingStatus != Pass {
		t.Fatalf("totals statuses = %+v", report.Totals)
	}
	for _, svc := range report.Services {
		if svc.Status != Pass {
			t.Fatalf("%s status = %s; checks: %+v", svc.Service, svc.Status, svc.Checks)
		}
	}
	// The optional image/digest declaration is NOT_ASSESSED, not PASS and not zero.
	for _, svc := range report.Services {
		if svc.Service != "haproxy" {
			continue
		}
		for _, c := range svc.Checks {
			if c.Name == "image" || c.Name == "digest" {
				if c.Status != NotAssessed {
					t.Fatalf("%s %s = %s, want NOT_ASSESSED", svc.Service, c.Name, c.Status)
				}
			}
		}
	}
}

func TestValidateMissingServiceIsFailNotZero(t *testing.T) {
	d := fixtureDescriptor()
	inv := fixtureInventory()
	inv.Services = inv.Services[:3] // drop haproxy
	report, err := Validate(d, inv, ValidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Attestation != Fail {
		t.Fatalf("attestation = %s, want FAIL", report.Attestation)
	}
	if report.Totals.RequiredSupportingReplicas != 2 || report.Totals.ObservedSupportingReplicas != 1 {
		t.Fatalf("supporting totals = %+v", report.Totals)
	}
	if report.Totals.SupportingStatus != Fail {
		t.Fatalf("supporting status = %s, want FAIL", report.Totals.SupportingStatus)
	}
	for _, svc := range report.Services {
		if svc.Service != "haproxy" {
			continue
		}
		if svc.Status != Fail || svc.ObservedReplicas != 0 {
			t.Fatalf("missing service result = %+v", svc)
		}
	}
}

func TestValidateMismatchesAndUnavailable(t *testing.T) {
	t.Run("replica declaration mismatch", func(t *testing.T) {
		d := fixtureDescriptor()
		d.Services[0].Replicas = 3
		report, err := Validate(d, fixtureInventory(), ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if report.Attestation != Fail {
			t.Fatalf("attestation = %s, want FAIL", report.Attestation)
		}
	})
	t.Run("image mismatch", func(t *testing.T) {
		d := fixtureDescriptor()
		d.Services[0].Image = "thingsboard/tb-http-transport:other"
		report, err := Validate(d, fixtureInventory(), ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-http-transport", "image", Fail)
	})
	t.Run("digest mismatch", func(t *testing.T) {
		d := fixtureDescriptor()
		d.Services[0].Digest = "sha256:" + hx('9')
		report, err := Validate(d, fixtureInventory(), ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-http-transport", "digest", Fail)
	})
	t.Run("digest missing is unavailable", func(t *testing.T) {
		inv := fixtureInventory()
		for i := range inv.Services[0].Containers {
			inv.Services[0].Containers[i].Digest = ""
		}
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-http-transport", "digest", Unavailable)
		if report.Attestation != Unavailable {
			t.Fatalf("attestation = %s, want UNAVAILABLE", report.Attestation)
		}
	})
	t.Run("digest mismatch outranks missing", func(t *testing.T) {
		inv := fixtureInventory()
		inv.Services[0].Containers[0].Digest = ""
		inv.Services[0].Containers[1].Digest = "sha256:" + hx('9')
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-http-transport", "digest", Fail)
	})
	t.Run("cpuset mismatch", func(t *testing.T) {
		inv := fixtureInventory()
		inv.Services[0].CPUSet = "4"
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-http-transport", "cpuset", Fail)
	})
	t.Run("cpuset missing is unavailable", func(t *testing.T) {
		inv := fixtureInventory()
		inv.Services[0].CPUSet = ""
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-http-transport", "cpuset", Unavailable)
		if report.Attestation != Unavailable {
			t.Fatalf("attestation = %s, want UNAVAILABLE", report.Attestation)
		}
	})
	t.Run("health and state failures", func(t *testing.T) {
		inv := fixtureInventory()
		inv.Services[1].Containers[0].Health = "unhealthy"
		inv.Services[1].Containers[0].HealthReason = "caller reports unhealthy"
		inv.Services[2].Containers[0].State = "exited"
		inv.Services[2].Containers[0].StateReason = "caller reports exited"
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertServiceCheck(t, report, "tb-core", "health", Fail)
		assertServiceCheck(t, report, "postgres", "state", Fail)
	})
	t.Run("effective configuration mismatch", func(t *testing.T) {
		inv := fixtureInventory()
		inv.EffectiveConfiguration = &EffectiveConfiguration{SHA256: hx('8')}
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertGlobalCheck(t, report, "effective_configuration", Fail)
	})
	t.Run("effective configuration missing is unavailable", func(t *testing.T) {
		inv := fixtureInventory()
		inv.EffectiveConfiguration = nil
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertGlobalCheck(t, report, "effective_configuration", Unavailable)
		if report.Attestation != Unavailable {
			t.Fatalf("attestation = %s, want UNAVAILABLE", report.Attestation)
		}
	})
}

func TestValidateUnexpectedServiceAndCrossProject(t *testing.T) {
	t.Run("unexpected service", func(t *testing.T) {
		inv := fixtureInventory()
		inv.Services = append(inv.Services, InventoryService{
			Project: "tb", Service: "rogue", Replicas: 1,
			Containers: []InventoryContainer{{
				Project: "tb", Service: "rogue", ContainerID: "rogue-0", Replica: 0,
				Image: "rogue:1", StartedAtMS: 2000, Health: "healthy", State: "running",
			}},
		})
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertGlobalCheck(t, report, "unexpected_services", Fail)
		if report.Attestation != Fail {
			t.Fatalf("attestation = %s, want FAIL", report.Attestation)
		}
	})
	t.Run("cross-project", func(t *testing.T) {
		inv := fixtureInventory()
		inv.Project = "other"
		for i := range inv.Services {
			inv.Services[i].Project = "other"
			for j := range inv.Services[i].Containers {
				inv.Services[i].Containers[j].Project = "other"
			}
		}
		report, err := Validate(fixtureDescriptor(), inv, ValidateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertGlobalCheck(t, report, "project", Fail)
		if report.Attestation != Fail {
			t.Fatalf("attestation = %s, want FAIL", report.Attestation)
		}
	})
}

func TestObserverIsReadOnlyFakeAndAbsenceIsNotAssessed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(path, mustJSON(t, fixtureInventory()), 0644); err != nil {
		t.Fatal(err)
	}
	inv, err := (FakeObserver{Path: path}).Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(fixtureDescriptor(), inv, ValidateOptions{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (FakeObserver{Path: path}).Observe(ctx); err == nil {
		t.Fatal("cancelled observer context was ignored")
	}
	if _, err := (FakeObserver{}).Observe(context.Background()); err == nil {
		t.Fatal("empty fake observer path was accepted")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"schema_version":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := (FakeObserver{Path: bad}).Observe(context.Background()); err == nil {
		t.Fatal("malformed fake observer inventory was accepted")
	}
}

func TestObserverMismatchIsFail(t *testing.T) {
	report, err := Validate(fixtureDescriptor(), fixtureInventory(), ValidateOptions{
		Observer: ObserverReport{
			Status: Fail,
			Name:   "fake-file",
			Reason: "two supplied observations differ",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertGlobalCheck(t, report, "observer", Fail)
	if report.Attestation != Fail {
		t.Fatalf("attestation = %s, want FAIL", report.Attestation)
	}
}

func TestRenderDeterministicAndWriter(t *testing.T) {
	d := fixtureDescriptor()
	inv := fixtureInventory()
	report, err := Validate(d, inv, ValidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	permutedInv := inv
	permutedInv.Services = []InventoryService{inv.Services[3], inv.Services[1], inv.Services[0], inv.Services[2]}
	permutedInv.Services[0].Containers = []InventoryContainer{inv.Services[3].Containers[0]}
	report2, err := Validate(d, permutedInv, ValidateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalReport(report2)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("JSON report depends on inventory service order")
	}
	if RenderMarkdown(report) != RenderMarkdown(report2) {
		t.Fatal("Markdown report depends on inventory service order")
	}
	markdown := RenderMarkdown(report)
	for _, want := range []string{"Live validation: NOT_ASSESSED", "No Docker client call", "never a successful zero"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Markdown missing %q", want)
		}
	}
	if !strings.Contains(string(a), `"live_validation": "NOT_ASSESSED"`) {
		t.Fatal("JSON report does not carry explicit NOT_ASSESSED live validation")
	}

	dir := t.TempDir()
	doc, err := EvidenceFromFile("inventory", filepath.Join(dir, "in.json"))
	if err == nil {
		t.Fatal("missing evidence file unexpectedly hashed")
	}
	_ = doc
	inPath := filepath.Join(dir, "in.json")
	if err := os.WriteFile(inPath, mustJSON(t, inv), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err = EvidenceFromFile("inventory", inPath)
	if err != nil {
		t.Fatal(err)
	}
	other := EvidenceFromBytes("descriptor", filepath.Join(dir, "d.json"), mustJSON(t, d))
	manifest, err := NewEvidenceManifest([]EvidenceDocument{other, doc})
	if err != nil {
		t.Fatal(err)
	}
	if err := Write("", report, manifest, []string{inPath}); err == nil {
		t.Fatal("empty output directory was accepted")
	}
	out := filepath.Join(dir, "out")
	if err := Write(out, report, manifest, []string{inPath}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deployment-validation.json", "deployment-validation.md", "evidence-manifest.json", "SHA256SUMS"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if err := Write(out, report, manifest, []string{inPath}); err == nil {
		t.Fatal("existing output directory was accepted")
	}
}

func TestEvidenceManifestIntegrity(t *testing.T) {
	dir := t.TempDir()
	descPath := filepath.Join(dir, "descriptor.json")
	invPath := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(descPath, []byte("descriptor"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invPath, []byte("inventory"), 0644); err != nil {
		t.Fatal(err)
	}
	docs := []EvidenceDocument{
		EvidenceFromBytes("descriptor", "descriptor.json", []byte("descriptor")),
		EvidenceFromBytes("inventory", "inventory.json", []byte("inventory")),
	}
	m, err := NewEvidenceManifest(docs)
	if err != nil {
		t.Fatal(err)
	}
	if m.CapturedAtMS != nil {
		t.Fatal("default manifest fabricated a timestamp")
	}
	b, err := m.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "captured_at_ms") {
		t.Fatal("default manifest serialized a captured_at_ms field")
	}
	decoded, err := DecodeEvidenceManifest(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decoded.Verify(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invPath, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := decoded.Verify(dir); err == nil {
		t.Fatal("tampered document passed verification")
	}

	ts := int64(1234)
	withTS := *m
	withTS.CapturedAtMS = &ts
	withTS.Documents = append([]EvidenceDocument(nil), m.Documents...)
	if err := withTS.Validate(); err != nil {
		t.Fatal(err)
	}
	if b, _ := withTS.CanonicalJSON(); !strings.Contains(string(b), `"captured_at_ms":1234`) {
		t.Fatalf("caller-supplied timestamp was not preserved: %s", b)
	}
	if _, err := NewEvidenceManifest([]EvidenceDocument{
		EvidenceFromBytes("descriptor", "same", []byte("a")),
		EvidenceFromBytes("inventory", "same", []byte("b")),
	}); err == nil {
		t.Fatal("duplicate manifest path accepted")
	}
	badRole := append([]EvidenceDocument(nil), m.Documents...)
	badRole[0].Role = "command"
	if _, err := NewEvidenceManifest(badRole); err == nil {
		t.Fatal("unsupported manifest role accepted")
	}
	if _, err := DecodeEvidenceManifest([]byte(`{"schema_version":1,"interpretation":"deployment-evidence-manifest-v1","documents":[],"captured_at_ms":null}`)); err == nil {
		t.Fatal("null manifest timestamp accepted")
	}
	if _, err := DecodeEvidenceManifest([]byte(`{"schema_version":1,"interpretation":"deployment-evidence-manifest-v1","documents":[],"extra":1}`)); err == nil {
		t.Fatal("unknown manifest field accepted")
	}
}

func TestDecodeRejectsCaseVariantKeysEverywhere(t *testing.T) {
	desc := asMap(t, fixtureDescriptor())
	inv := asMap(t, fixtureInventory())
	man := asMap(t, fixtureEvidenceManifest(t))

	cases := []struct {
		name   string
		raw    []byte
		decode func([]byte) error
	}{
		{"descriptor root replaced platform", withMap(desc, func(m map[string]any) {
			delete(m, "platform")
			m["PLATFORM"] = "thingsboard"
		}), decodeDescriptorBytes},
		{"descriptor root conflicting platform alias", withMap(desc, func(m map[string]any) {
			m["PLATFORM"] = "fiware"
		}), decodeDescriptorBytes},
		{"descriptor allocation key alias", withMap(desc, func(m map[string]any) {
			m["allocation"].(map[string]any)["APPLICATION_CPUSETS"] = []any{"4-7"}
		}), decodeDescriptorBytes},
		{"descriptor protocol optional key alias", withMap(desc, func(m map[string]any) {
			m["protocol"].(map[string]any)["PROFILE_PATH"] = "research/protocols/profile.json"
		}), decodeDescriptorBytes},
		{"descriptor provenance document array alias", withMap(desc, func(m map[string]any) {
			doc := m["provenance"].(map[string]any)["documents"].([]any)[0].(map[string]any)
			doc["ROLE"] = "other"
		}), decodeDescriptorBytes},
		{"descriptor service array alias", withMap(desc, func(m map[string]any) {
			serviceMaps(t, m)[0].(map[string]any)["SERVICE"] = "tb-http-transport"
		}), decodeDescriptorBytes},
		{"descriptor service array conflicting replicas alias", withMap(desc, func(m map[string]any) {
			serviceMaps(t, m)[0].(map[string]any)["REPLICAS"] = 99
		}), decodeDescriptorBytes},
		{"descriptor effective configuration alias", withMap(desc, func(m map[string]any) {
			m["effective_configuration"].(map[string]any)["SHA256"] = hx('d')
		}), decodeDescriptorBytes},
		{"inventory root replaced project", withMap(inv, func(m map[string]any) {
			delete(m, "project")
			m["PROJECT"] = "tb"
		}), decodeInventoryBytes},
		{"inventory root conflicting project alias", withMap(inv, func(m map[string]any) {
			m["PROJECT"] = "other"
		}), decodeInventoryBytes},
		{"inventory service array alias", withMap(inv, func(m map[string]any) {
			serviceMaps(t, m)[0].(map[string]any)["SERVICE"] = "tb-http-transport"
		}), decodeInventoryBytes},
		{"inventory container array state alias", withMap(inv, func(m map[string]any) {
			containerMaps(t, m, 0)[0].(map[string]any)["STATE"] = "exited"
		}), decodeInventoryBytes},
		{"inventory container array health alias", withMap(inv, func(m map[string]any) {
			containerMaps(t, m, 0)[0].(map[string]any)["HEALTH"] = "healthy"
		}), decodeInventoryBytes},
		{"inventory container optional reason alias", withMap(inv, func(m map[string]any) {
			containerMaps(t, m, 0)[0].(map[string]any)["STATE_REASON"] = "synthetic"
		}), decodeInventoryBytes},
		{"evidence manifest documents alias", withMap(man, func(m map[string]any) {
			m["DOCUMENTS"] = m["documents"]
		}), decodeEvidenceManifestBytes},
		{"evidence manifest replaced schema alias", withMap(man, func(m map[string]any) {
			delete(m, "schema_version")
			m["SCHEMA_VERSION"] = 1
		}), decodeEvidenceManifestBytes},
		{"evidence manifest document array alias", withMap(man, func(m map[string]any) {
			m["documents"].([]any)[0].(map[string]any)["SHA256"] = hx('a')
		}), decodeEvidenceManifestBytes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decode(tc.raw)
			if err == nil {
				t.Fatalf("accepted case-variant key: %s", tc.raw)
			}
			if !strings.Contains(err.Error(), "unknown or non-case-exact key") {
				t.Fatalf("error = %v, want an explicit non-case-exact key rejection", err)
			}
		})
	}
}

func TestDecodeRejectsDuplicateKeysInNestedObjectsAndArrays(t *testing.T) {
	descriptorHead := `{"schema_version":1,"platform":"thingsboard","project":"tb",` +
		`"allocation":{"application_cpusets":["1"],"supporting_cpusets":["2"]},` +
		`"protocol":{"binding_sha256":"` + hx('a') + `","profile_sha256":"` + hx('b') + `"},`
	descriptorServices := `,"services":[{"service":"app","class":"application","replicas":1,"cpuset":"1","memory_bytes":1}]}`
	inventoryHead := `{"schema_version":1,"project":"tb","source":"synthetic","services":[` +
		`{"project":"tb","service":"svc","replicas":1,"cpuset":"1","containers":[`
	inventoryTail := `]}]}`

	cases := []struct {
		name   string
		raw    []byte
		decode func([]byte) error
	}{
		{
			"descriptor root duplicate",
			[]byte(`{"schema_version":1,"schema_version":2}`),
			decodeDescriptorBytes,
		},
		{
			"descriptor nested service duplicate",
			[]byte(descriptorHead + `"provenance":{"source":"s"},"services":[{"service":"app","service":"other","class":"application","replicas":1,"cpuset":"1","memory_bytes":1}]}`),
			decodeDescriptorBytes,
		},
		{
			"descriptor provenance document array duplicate",
			[]byte(descriptorHead + `"provenance":{"source":"s","documents":[{"role":"compose","role":"protocol","path":"p","sha256":"` + hx('c') + `"}]}` + descriptorServices),
			decodeDescriptorBytes,
		},
		{
			"inventory container array duplicate",
			[]byte(inventoryHead + `{"project":"tb","service":"svc","container_id":"c","replica":0,"image":"i","started_at_ms":1,"health":"healthy","state":"running","state":"exited"}` + inventoryTail),
			decodeInventoryBytes,
		},
		{
			"inventory service array duplicate",
			[]byte(`{"schema_version":1,"project":"tb","source":"synthetic","services":[{"project":"tb","service":"svc","replicas":1,"replicas":2,"cpuset":"1","containers":[{"project":"tb","service":"svc","container_id":"c","replica":0,"image":"i","started_at_ms":1,"health":"healthy","state":"running"}]}]}`),
			decodeInventoryBytes,
		},
		{
			"evidence manifest root duplicate",
			[]byte(`{"schema_version":1,"interpretation":"deployment-evidence-manifest-v1","documents":[{"role":"descriptor","path":"p","sha256":"` + hx('a') + `"}],"documents":[{"role":"inventory","path":"q","sha256":"` + hx('b') + `"}]}`),
			decodeEvidenceManifestBytes,
		},
		{
			"evidence manifest document array duplicate",
			[]byte(`{"schema_version":1,"interpretation":"deployment-evidence-manifest-v1","documents":[{"role":"descriptor","role":"inventory","path":"p","sha256":"` + hx('a') + `"}]}`),
			decodeEvidenceManifestBytes,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.decode(tc.raw)
			if err == nil {
				t.Fatalf("accepted duplicate key: %s", tc.raw)
			}
			if !strings.Contains(err.Error(), "duplicate JSON object key") {
				t.Fatalf("error = %v, want an explicit duplicate-key rejection", err)
			}
		})
	}
}

func TestDecodeExactKeysAndKeyOrderPermutation(t *testing.T) {
	desc := fixtureDescriptor()
	descCanonical, err := desc.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	descValues := asMap(t, desc)
	for _, order := range permutationOrders(sortedKeys(descValues)) {
		raw := orderedJSONObject(t, order, descValues)
		decoded, err := DecodeDescriptor(raw)
		if err != nil {
			t.Fatalf("valid descriptor in order %v: %v", order, err)
		}
		got, err := decoded.CanonicalSHA256()
		if err != nil {
			t.Fatal(err)
		}
		if got != descCanonical {
			t.Fatalf("canonical descriptor hash depends on key order: %s != %s", got, descCanonical)
		}
	}

	inv := fixtureInventory()
	invCanonical, err := inv.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	invValues := asMap(t, inv)
	for _, order := range permutationOrders(sortedKeys(invValues)) {
		raw := orderedJSONObject(t, order, invValues)
		decoded, err := DecodeInventory(raw)
		if err != nil {
			t.Fatalf("valid inventory in order %v: %v", order, err)
		}
		got, err := decoded.CanonicalSHA256()
		if err != nil {
			t.Fatal(err)
		}
		if got != invCanonical {
			t.Fatalf("canonical inventory hash depends on key order: %s != %s", got, invCanonical)
		}
	}

	manifest := fixtureEvidenceManifest(t)
	ts := int64(1234)
	manifest.CapturedAtMS = &ts
	manifestCanonical, err := manifest.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	manifestValues := asMap(t, manifest)
	for _, order := range permutationOrders(sortedKeys(manifestValues)) {
		raw := orderedJSONObject(t, order, manifestValues)
		decoded, err := DecodeEvidenceManifest(raw)
		if err != nil {
			t.Fatalf("valid evidence manifest in order %v: %v", order, err)
		}
		got, err := decoded.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(manifestCanonical) {
			t.Fatalf("manifest decode depends on key order: %s != %s", got, manifestCanonical)
		}
		if decoded.CapturedAtMS == nil || *decoded.CapturedAtMS != ts {
			t.Fatalf("caller-supplied manifest timestamp was not preserved: %+v", decoded.CapturedAtMS)
		}
	}

	aliasValues := asMap(t, desc)
	aliasValues["PLATFORM"] = "fiware"
	for _, order := range permutationOrders(sortedKeys(aliasValues)) {
		err := decodeDescriptorBytes(orderedJSONObject(t, order, aliasValues))
		if err == nil || !strings.Contains(err.Error(), "unknown or non-case-exact key") {
			t.Fatalf("conflicting case alias in order %v: error = %v, want explicit rejection", order, err)
		}
	}
}

func fixtureEvidenceManifest(t *testing.T) EvidenceManifest {
	t.Helper()
	m, err := NewEvidenceManifest([]EvidenceDocument{
		EvidenceFromBytes("descriptor", "descriptor.json", []byte("descriptor")),
		EvidenceFromBytes("inventory", "inventory.json", []byte("inventory")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return *m
}

func decodeDescriptorBytes(b []byte) error { _, err := DecodeDescriptor(b); return err }

func decodeInventoryBytes(b []byte) error { _, err := DecodeInventory(b); return err }

func decodeEvidenceManifestBytes(b []byte) error {
	_, err := DecodeEvidenceManifest(b)
	return err
}

func containerMaps(t *testing.T, m map[string]any, service int) []any {
	t.Helper()
	containers, ok := serviceMaps(t, m)[service].(map[string]any)["containers"].([]any)
	if !ok {
		t.Fatal("fixture service has no containers array")
	}
	return containers
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func permutationOrders(keys []string) [][]string {
	ascending := append([]string(nil), keys...)
	sort.Strings(ascending)
	descending := append([]string(nil), ascending...)
	sort.Sort(sort.Reverse(sort.StringSlice(descending)))
	rotated := append(append([]string(nil), ascending[1:]...), ascending[0])
	return [][]string{ascending, descending, rotated}
}

func orderedJSONObject(t *testing.T, order []string, values map[string]any) []byte {
	t.Helper()
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range order {
		if i > 0 {
			b.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			t.Fatal(err)
		}
		valueJSON, err := json.Marshal(values[key])
		if err != nil {
			t.Fatal(err)
		}
		b.Write(keyJSON)
		b.WriteByte(':')
		b.Write(valueJSON)
	}
	b.WriteByte('}')
	return []byte(b.String())
}

func assertServiceCheck(t *testing.T, r *Report, service, check string, want Status) {
	t.Helper()
	for _, svc := range r.Services {
		if svc.Service != service {
			continue
		}
		for _, c := range svc.Checks {
			if c.Name == check {
				if c.Status != want {
					t.Fatalf("%s %s = %s, want %s (%s)", service, check, c.Status, want, c.Reason)
				}
				return
			}
		}
	}
	t.Fatalf("service %s check %s not found", service, check)
}

func assertGlobalCheck(t *testing.T, r *Report, check string, want Status) {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == check {
			if c.Status != want {
				t.Fatalf("global %s = %s, want %s (%s)", check, c.Status, want, c.Reason)
			}
			return
		}
	}
	t.Fatalf("global check %s not found", check)
}

func withMap(base map[string]any, mutate func(map[string]any)) []byte {
	b, _ := json.Marshal(base)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	mutate(m)
	out, _ := json.Marshal(m)
	return out
}
