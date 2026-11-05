package deployment

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// m2cTestdata is the caller-supplied synthetic fixture set for the M2c offline
// end-to-end validation. Every fixture names itself synthetic; none is a live
// observation and no Docker, SUT, observer or lifecycle action is involved.
const m2cTestdata = "testdata/m2c"

func m2cRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(m2cTestdata, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func m2cDescriptor(t *testing.T) *Descriptor {
	t.Helper()
	d, err := DecodeDescriptor(m2cRead(t, "synthetic-descriptor-thingsboard.json"))
	if err != nil {
		t.Fatalf("decode synthetic descriptor: %v", err)
	}
	return d
}

func m2cInventory(t *testing.T, name string) *Inventory {
	t.Helper()
	inv, err := DecodeInventory(m2cRead(t, name))
	if err != nil {
		t.Fatalf("decode synthetic inventory %s: %v", name, err)
	}
	return inv
}

func m2cPlan(t *testing.T) *OrchestrationPlan {
	t.Helper()
	p, err := DecodeOrchestrationPlan(m2cRead(t, "synthetic-orchestration-plan-v2.json"))
	if err != nil {
		t.Fatalf("decode synthetic orchestration plan: %v", err)
	}
	return p
}

func m2cManifest(t *testing.T) *ArtifactManifest {
	t.Helper()
	m, err := DecodeArtifactManifest(m2cRead(t, "synthetic-artifact-manifest.json"))
	if err != nil {
		t.Fatalf("decode synthetic artifact manifest: %v", err)
	}
	return m
}

// TestM2CSyntheticDeploymentValidateRoundTrip validates the existing approved
// descriptor/inventory surface with caller-supplied synthetic documents and a
// controlled mismatch. It asserts the synthetic label is carried in the
// document provenance and that live validation remains NOT_ASSESSED.
func TestM2CSyntheticDeploymentValidateRoundTrip(t *testing.T) {
	descriptorBytes := m2cRead(t, "synthetic-descriptor-thingsboard.json")
	inventoryBytes := m2cRead(t, "synthetic-inventory-thingsboard.json")
	d := m2cDescriptor(t)
	inv := m2cInventory(t, "synthetic-inventory-thingsboard.json")

	if !strings.Contains(d.Provenance.Source, "SYNTHETIC M2c") {
		t.Fatalf("descriptor provenance does not carry the synthetic label: %q", d.Provenance.Source)
	}
	if !strings.Contains(inv.Source, "SYNTHETIC M2c") {
		t.Fatalf("inventory source does not carry the synthetic label: %q", inv.Source)
	}
	// The fixture declares the unchanged approved protocol binding and D44
	// profile hashes. This is a declaration, not a live re-read.
	if d.Protocol.BindingSHA256 != "7176b1faed9f5d1b08720f8c2e38256418c4cef65b1182e6157c9b276d0d24ee" {
		t.Fatalf("synthetic descriptor protocol binding changed: %s", d.Protocol.BindingSHA256)
	}
	if d.Protocol.ProfileSHA256 != "1b6aa76d5156eeb1561b0d02641b6f01d3ca6a3447904182a3c219b5930ed1f8" {
		t.Fatalf("synthetic descriptor D44 profile changed: %s", d.Protocol.ProfileSHA256)
	}

	descCanonical, err := d.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	invCanonical, err := inv.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	report, err := Validate(*d, *inv, ValidateOptions{
		DescriptorSHA256:          SHA256Bytes(descriptorBytes),
		DescriptorCanonicalSHA256: descCanonical,
		InventorySHA256:           SHA256Bytes(inventoryBytes),
		InventoryCanonicalSHA256:  invCanonical,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Attestation != Pass {
		t.Fatalf("attestation = %s, want PASS", report.Attestation)
	}
	if report.LiveValidation != NotAssessed {
		t.Fatalf("live_validation = %s, want NOT_ASSESSED", report.LiveValidation)
	}
	if report.DryRun != true {
		t.Fatal("report is not marked dry-run")
	}
	if report.Observer.Status != NotAssessed {
		t.Fatalf("observer status = %s, want NOT_ASSESSED", report.Observer.Status)
	}
	for _, svc := range report.Services {
		if svc.Status != Pass {
			t.Fatalf("service %s status = %s, want PASS: %+v", svc.Service, svc.Status, svc.Checks)
		}
	}
	if len(report.Services) != 8 {
		t.Fatalf("validated %d services, want the 8 current ThingsBoard persistent services", len(report.Services))
	}
	if report.Totals.RequiredApplicationReplicas != 4 || report.Totals.ObservedApplicationReplicas != 4 {
		t.Fatalf("application replicas = %d/%d, want 4/4", report.Totals.RequiredApplicationReplicas, report.Totals.ObservedApplicationReplicas)
	}
	if report.Totals.RequiredSupportingReplicas != 5 || report.Totals.ObservedSupportingReplicas != 5 {
		t.Fatalf("supporting replicas = %d/%d, want 5/5", report.Totals.RequiredSupportingReplicas, report.Totals.ObservedSupportingReplicas)
	}

	rendered := RenderMarkdown(report)
	if !strings.Contains(rendered, "**Live validation: NOT_ASSESSED**") {
		t.Fatalf("rendered report does not mark live validation NOT_ASSESSED:\n%s", rendered)
	}
	if !strings.Contains(rendered, "SYNTHETIC M2c") {
		t.Fatalf("rendered report does not carry the synthetic provenance label:\n%s", rendered)
	}
	if again := RenderMarkdown(report); again != rendered {
		t.Fatal("rendered deployment report is not deterministic")
	}

	// Controlled mismatch: the same descriptor against a synthetic inventory
	// whose tb-core image, health and state differ. This must be FAIL, not an
	// input error and not a silent PASS.
	mismatch := m2cInventory(t, "synthetic-inventory-thingsboard-mismatch.json")
	bad, err := Validate(*d, *mismatch, ValidateOptions{
		DescriptorSHA256:          SHA256Bytes(descriptorBytes),
		DescriptorCanonicalSHA256: descCanonical,
		InventorySHA256:           SHA256Bytes(m2cRead(t, "synthetic-inventory-thingsboard-mismatch.json")),
		InventoryCanonicalSHA256:  mustInvCanonical(t, mismatch),
	})
	if err != nil {
		t.Fatalf("controlled mismatch produced an input error instead of a FAIL report: %v", err)
	}
	if bad.Attestation != Fail {
		t.Fatalf("controlled mismatch attestation = %s, want FAIL", bad.Attestation)
	}
	if bad.LiveValidation != NotAssessed {
		t.Fatalf("controlled mismatch live_validation = %s, want NOT_ASSESSED", bad.LiveValidation)
	}
}

func mustInvCanonical(t *testing.T, inv *Inventory) string {
	t.Helper()
	h, err := inv.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// TestM2CSyntheticOrchestrationPlanRoundTrip decodes, canonicalizes and renders
// the existing approved orchestration v2 surface. The fixture carries a
// synthetic label in every stage purpose and in each platform's health checks.
func TestM2CSyntheticOrchestrationPlanRoundTrip(t *testing.T) {
	raw := m2cRead(t, "synthetic-orchestration-plan-v2.json")
	if !bytes.Contains(raw, []byte("SYNTHETIC M2c")) {
		t.Fatal("orchestration fixture does not carry an explicit synthetic label")
	}
	p := m2cPlan(t)
	if !p.ExecutionBlocked {
		t.Fatal("synthetic orchestration plan is not execution-blocked")
	}
	if p.SchemaVersion != OrchestrationSchemaVersion || p.Interpretation != OrchestrationInterpretation {
		t.Fatalf("plan schema=%d interpretation=%s", p.SchemaVersion, p.Interpretation)
	}
	for _, stage := range p.Stages {
		for _, action := range stage.PermissibleActions {
			if !IsSafeAction(action) {
				t.Fatalf("stage %s admits non-safe action %s", stage.Stage, action)
			}
		}
		if !strings.HasPrefix(stage.Purpose, "SYNTHETIC M2c") {
			t.Fatalf("stage %s purpose lacks the synthetic label: %q", stage.Stage, stage.Purpose)
		}
	}
	for _, platform := range p.Platforms {
		if !strings.HasPrefix(platform.HealthChecks[0], "SYNTHETIC M2c") {
			t.Fatalf("platform %s lacks the synthetic health-check label", platform.Platform)
		}
	}

	canonicalJSON, err := p.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	canonicalHash, err := p.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := SHA256Bytes(canonicalJSON), canonicalHash; got != want {
		t.Fatalf("canonical hash %s is not the SHA-256 of canonical JSON %s", want, got)
	}
	again, err := DecodeOrchestrationPlan(raw)
	if err != nil {
		t.Fatal(err)
	}
	againJSON, err := again.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	againHash, err := again.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalJSON, againJSON) || againHash != canonicalHash {
		t.Fatal("orchestration canonical bytes are not deterministic across decodes")
	}

	rendered := RenderOrchestrationPlanMarkdown(p)
	if !strings.Contains(rendered, "SYNTHETIC M2c") {
		t.Fatalf("rendered orchestration plan does not carry the synthetic label:\n%s", rendered)
	}
	if !strings.Contains(rendered, ExecutionBlockedMarker) {
		t.Fatalf("rendered orchestration plan does not mark execution blocked:\n%s", rendered)
	}
	if !strings.Contains(rendered, "No Docker client call") {
		t.Fatalf("rendered orchestration plan does not state the lifecycle exclusion:\n%s", rendered)
	}
	if againRendered := RenderOrchestrationPlanMarkdown(p); againRendered != rendered {
		t.Fatal("rendered orchestration plan is not deterministic")
	}
}

// TestM2CSyntheticArtifactManifestRoundTrip exercises canonicalization, render,
// and the approved Verify surface against a scratch directory. It covers
// success, a tampered hash, a tampered byte count, a missing file and a prior
// manifest that must remain NOT_IMPLEMENTED/NOT_ASSESSED.
func TestM2CSyntheticArtifactManifestRoundTrip(t *testing.T) {
	raw := m2cRead(t, "synthetic-artifact-manifest.json")
	if !bytes.Contains(raw, []byte("m2c-synthetic")) {
		t.Fatal("artifact manifest fixture does not carry the synthetic run tag/identifier")
	}
	m := m2cManifest(t)
	canonicalJSON, err := m.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	canonicalHash, err := m.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := SHA256Bytes(canonicalJSON), canonicalHash; got != want {
		t.Fatalf("canonical hash %s is not the SHA-256 of canonical JSON %s", want, got)
	}
	if again, err := DecodeArtifactManifest(raw); err != nil {
		t.Fatal(err)
	} else {
		againHash, err := again.CanonicalSHA256()
		if err != nil {
			t.Fatal(err)
		}
		if againHash != canonicalHash {
			t.Fatalf("manifest canonical hash not deterministic: %s != %s", againHash, canonicalHash)
		}
	}
	rendered := RenderArtifactManifestMarkdown(m)
	if !strings.Contains(rendered, "Prior-manifest binding is NOT_IMPLEMENTED") {
		t.Fatalf("manifest render does not state the prior-manifest non-implementation:\n%s", rendered)
	}
	if again := RenderArtifactManifestMarkdown(m); again != rendered {
		t.Fatal("rendered artifact manifest is not deterministic")
	}

	root := t.TempDir()
	m2cPopulatePackagingScratch(t, root, m)
	results, err := m.Verify(root)
	if err != nil {
		t.Fatalf("verify matching synthetic artifacts: %v", err)
	}
	if len(results) != len(m.Artifacts) {
		t.Fatalf("verified %d artifacts, want %d", len(results), len(m.Artifacts))
	}
	for _, v := range results {
		if v.Status != Pass {
			t.Fatalf("artifact %s %s status = %s, want PASS", v.Role, v.Path, v.Status)
		}
	}

	t.Run("tampered hash fails", func(t *testing.T) {
		tampered := *m
		tampered.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
		tampered.Artifacts[0].SHA256 = strings.Repeat("f", 64)
		got, err := tampered.Verify(root)
		if err == nil {
			t.Fatal("tampered hash was accepted")
		}
		if v := m2cFindVerification(got, tampered.Artifacts[0].Path); v == nil || v.Status != Fail {
			t.Fatalf("tampered hash not reported as FAIL: %v %+v", err, v)
		}
	})

	t.Run("tampered completeness fails", func(t *testing.T) {
		tampered := *m
		tampered.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
		tampered.Artifacts[1].Bytes++
		got, err := tampered.Verify(root)
		if err == nil {
			t.Fatal("tampered byte count was accepted")
		}
		if v := m2cFindVerification(got, tampered.Artifacts[1].Path); v == nil || v.Status != Fail {
			t.Fatalf("tampered byte count not reported as FAIL: %v %+v", err, v)
		}
	})

	t.Run("missing file is unavailable", func(t *testing.T) {
		missing := *m
		missing.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
		missing.Artifacts[2].Path = ArtifactRunRoot(m.RunID) + "/absent-plan.json"
		got, err := missing.Verify(root)
		if err == nil {
			t.Fatal("missing artifact was accepted")
		}
		if v := m2cFindVerification(got, missing.Artifacts[2].Path); v == nil || v.Status != Unavailable {
			t.Fatalf("missing artifact not reported as UNAVAILABLE: %v %+v", err, v)
		}
	})

	t.Run("prior manifest remains not implemented", func(t *testing.T) {
		if PriorManifestVerificationNotImplemented != "NOT_IMPLEMENTED" {
			t.Fatalf("prior-manifest marker = %q, want NOT_IMPLEMENTED", PriorManifestVerificationNotImplemented)
		}
		priorRaw := m2cRead(t, "synthetic-prior-artifact-manifest.json")
		prior, err := DecodeArtifactManifest(priorRaw)
		if err != nil {
			t.Fatalf("decode synthetic prior manifest: %v", err)
		}
		// The synthetic prior manifest deliberately declares a non-matching hash
		// and byte count for the same path. Verify must not read it.
		priorPath := prior.Artifacts[0].Path
		if prior.Artifacts[0].SHA256 == m.Artifacts[3].SHA256 {
			t.Fatal("synthetic prior manifest was expected to declare a different hash")
		}
		write := func(rel string, b []byte) {
			t.Helper()
			p := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, b, 0644); err != nil {
				t.Fatal(err)
			}
		}
		write(filepath.ToSlash(filepath.Join(filepath.Dir(priorPath), "artifact-manifest.json")), priorRaw)
		if _, err := m.Verify(root); err != nil {
			t.Fatalf("Verify consulted the prior manifest instead of ignoring it: %v", err)
		}
	})
}

func m2cPopulatePackagingScratch(t *testing.T, root string, m *ArtifactManifest) {
	t.Helper()
	content := map[string][]byte{
		"descriptor.json":         m2cRead(t, "synthetic-descriptor-thingsboard.json"),
		"inventory.json":          m2cRead(t, "synthetic-inventory-thingsboard.json"),
		"orchestration-plan.json": m2cRead(t, "synthetic-orchestration-plan-v2.json"),
		"prior-note.txt":          m2cRead(t, "synthetic-prior-note.txt"),
	}
	for _, artifact := range m.Artifacts {
		name := filepath.Base(artifact.Path)
		b, ok := content[name]
		if !ok {
			t.Fatalf("no synthetic scratch content for artifact %s", artifact.Path)
		}
		p := filepath.Join(root, filepath.FromSlash(artifact.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func m2cFindVerification(results []ArtifactVerification, path string) *ArtifactVerification {
	for i := range results {
		if results[i].Path == path {
			return &results[i]
		}
	}
	return nil
}

// TestM2CWriteCanonicalOutputs is an evidence helper, not a production path. It
// is skipped unless M2C_CANONICAL_OUT names a new directory. It writes only the
// already-computed canonical bytes and deterministic renders; it performs no
// Docker, SUT, lifecycle, load or observer action.
func TestM2CWriteCanonicalOutputs(t *testing.T) {
	out := os.Getenv("M2C_CANONICAL_OUT")
	if out == "" {
		t.Skip("M2C_CANONICAL_OUT not set")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	d := m2cDescriptor(t)
	inv := m2cInventory(t, "synthetic-inventory-thingsboard.json")
	p := m2cPlan(t)
	m := m2cManifest(t)

	type output struct {
		name string
		body []byte
	}
	descJSON, err := d.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	invJSON, err := inv.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	planJSON, err := p.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := m.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	outputs := []output{
		{"descriptor.canonical.json", descJSON},
		{"inventory.canonical.json", invJSON},
		{"orchestration-plan.canonical.json", planJSON},
		{"orchestration-plan.md", []byte(RenderOrchestrationPlanMarkdown(p))},
		{"artifact-manifest.canonical.json", manifestJSON},
		{"artifact-manifest.md", []byte(RenderArtifactManifestMarkdown(m))},
	}
	var hashes strings.Builder
	for _, o := range outputs {
		if err := os.WriteFile(filepath.Join(out, o.name), o.body, 0644); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&hashes, "%s  %s\n", SHA256Bytes(o.body), o.name)
	}
	fmt.Fprintf(&hashes, "%s  synthetic-descriptor-thingsboard.json (raw input)\n", SHA256Bytes(m2cRead(t, "synthetic-descriptor-thingsboard.json")))
	fmt.Fprintf(&hashes, "%s  synthetic-inventory-thingsboard.json (raw input)\n", SHA256Bytes(m2cRead(t, "synthetic-inventory-thingsboard.json")))
	fmt.Fprintf(&hashes, "%s  synthetic-orchestration-plan-v2.json (raw input)\n", SHA256Bytes(m2cRead(t, "synthetic-orchestration-plan-v2.json")))
	fmt.Fprintf(&hashes, "%s  synthetic-artifact-manifest.json (raw input)\n", SHA256Bytes(m2cRead(t, "synthetic-artifact-manifest.json")))
	descHash, _ := d.CanonicalSHA256()
	invHash, _ := inv.CanonicalSHA256()
	planHash, _ := p.CanonicalSHA256()
	manifestHash, _ := m.CanonicalSHA256()
	fmt.Fprintf(&hashes, "%s  descriptor canonical SHA-256\n", descHash)
	fmt.Fprintf(&hashes, "%s  inventory canonical SHA-256\n", invHash)
	fmt.Fprintf(&hashes, "%s  orchestration plan canonical SHA-256\n", planHash)
	fmt.Fprintf(&hashes, "%s  artifact manifest canonical SHA-256\n", manifestHash)
	if err := os.WriteFile(filepath.Join(out, "canonical-sha256.txt"), []byte(hashes.String()), 0644); err != nil {
		t.Fatal(err)
	}
	// Prove the recorded canonical bytes round-trip through the strict decoders.
	if _, err := DecodeDescriptor(descJSON); err != nil {
		t.Fatalf("canonical descriptor does not round-trip: %v", err)
	}
	if _, err := DecodeOrchestrationPlan(planJSON); err != nil {
		t.Fatalf("canonical plan does not round-trip: %v", err)
	}
	if _, err := DecodeArtifactManifest(manifestJSON); err != nil {
		t.Fatalf("canonical manifest does not round-trip: %v", err)
	}
}

// TestM2CSyntheticInvalidFixtureRejected proves the deliberately invalid
// synthetic fixture fails closed through the existing strict decoder.
func TestM2CSyntheticInvalidFixtureRejected(t *testing.T) {
	raw := m2cRead(t, "synthetic-descriptor-invalid-case-alias.json")
	if _, err := DecodeDescriptor(raw); err == nil || !strings.Contains(err.Error(), "non-case-exact key") {
		t.Fatalf("invalid synthetic descriptor was not rejected with a strict-key error: %v", err)
	}
}

// TestM2CWritePackagingEvidence is an evidence helper, not a production path.
// It is skipped unless M2C_PACKAGING_OUT names a new directory. It exercises the
// approved Verify surface against caller-supplied synthetic scratch trees and
// records the expected success/failure outcomes. It never writes into an
// existing evidence input.
func TestM2CWritePackagingEvidence(t *testing.T) {
	out := os.Getenv("M2C_PACKAGING_OUT")
	if out == "" {
		t.Skip("M2C_PACKAGING_OUT not set")
	}
	m := m2cManifest(t)
	var summary strings.Builder
	for _, name := range []string{"success", "tampered-hash", "tampered-completeness", "missing", "prior"} {
		root := filepath.Join(out, "packaging-scratch", name)
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
		m2cPopulatePackagingScratch(t, root, m)
		switch name {
		case "success":
			results, err := m.Verify(root)
			if err != nil {
				t.Fatalf("%s verify: %v", name, err)
			}
			fmt.Fprintf(&summary, "%s: PASS (%d/%d artifacts verified)\n", name, len(results), len(m.Artifacts))
		case "tampered-hash":
			tampered := *m
			tampered.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
			tampered.Artifacts[0].SHA256 = strings.Repeat("f", 64)
			results, err := tampered.Verify(root)
			if err == nil {
				t.Fatalf("%s: tampered hash was accepted", name)
			}
			fmt.Fprintf(&summary, "%s: expected FAIL reached: %v (status=%s)\n", name, err, m2cFindVerification(results, tampered.Artifacts[0].Path).Status)
		case "tampered-completeness":
			tampered := *m
			tampered.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
			tampered.Artifacts[1].Bytes++
			results, err := tampered.Verify(root)
			if err == nil {
				t.Fatalf("%s: tampered byte count was accepted", name)
			}
			fmt.Fprintf(&summary, "%s: expected FAIL reached: %v (status=%s)\n", name, err, m2cFindVerification(results, tampered.Artifacts[1].Path).Status)
		case "missing":
			planPath := filepath.Join(root, filepath.FromSlash(m.Artifacts[2].Path))
			if err := os.Remove(planPath); err != nil {
				t.Fatal(err)
			}
			results, err := m.Verify(root)
			if err == nil {
				t.Fatalf("%s: missing artifact was accepted", name)
			}
			fmt.Fprintf(&summary, "%s: expected UNAVAILABLE reached: %v (status=%s)\n", name, err, m2cFindVerification(results, m.Artifacts[2].Path).Status)
		case "prior":
			priorRaw := m2cRead(t, "synthetic-prior-artifact-manifest.json")
			prior, err := DecodeArtifactManifest(priorRaw)
			if err != nil {
				t.Fatal(err)
			}
			priorPath := filepath.Join(root, filepath.FromSlash(prior.Artifacts[0].Path))
			if err := os.WriteFile(filepath.Join(filepath.Dir(priorPath), "artifact-manifest.json"), priorRaw, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Verify(root); err != nil {
				t.Fatalf("%s: Verify consulted the prior manifest: %v", name, err)
			}
			fmt.Fprintf(&summary, "%s: PASS; prior-manifest binding remains %s / NOT_ASSESSED, Verify ignored the prior manifest\n", name, PriorManifestVerificationNotImplemented)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "packaging-verification.txt"), []byte(summary.String()), 0644); err != nil {
		t.Fatal(err)
	}
}
