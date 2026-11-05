package deployment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validArtifactManifest() ArtifactManifest {
	runID := "m2b-blueprint-20261105-032542+0330"
	return ArtifactManifest{
		SchemaVersion:  ArtifactManifestSchemaVersion,
		Interpretation: ArtifactManifestInterpretation,
		RunID:          runID,
		RunDate:        "2026-11-05",
		RunTag:         "m2b-blueprint",
		Platform:       "all",
		Artifacts: []ArtifactRecord{
			{Role: ArtifactRoleDescriptor, Path: ArtifactRunRoot(runID) + "/descriptor.json", SHA256: strings.Repeat("a", 64), Bytes: 10, Origin: ArtifactThisRun, Required: true},
			{Role: ArtifactRoleInventory, Path: ArtifactRunRoot(runID) + "/inventory.json", SHA256: strings.Repeat("b", 64), Bytes: 20, Origin: ArtifactThisRun, Required: true},
			{Role: ArtifactRolePacket, Path: "docs/archive/sessions/m2b-blueprint-20261105.md", SHA256: strings.Repeat("c", 64), Bytes: 30, Origin: ArtifactThisRun, Required: true},
			{Role: ArtifactRoleSinkLedger, Path: "results/m1f-fault-capacity-20261105-013824+0330/sink.ndjson", SHA256: strings.Repeat("d", 64), Bytes: 40, Origin: ArtifactPreservedPrior, Required: false},
		},
	}
}

func marshalArtifactManifest(t *testing.T, m ArtifactManifest) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestArtifactManifestValidAndRender(t *testing.T) {
	m := validArtifactManifest()
	b := marshalArtifactManifest(t, m)
	decoded, err := DecodeArtifactManifest(b)
	if err != nil {
		t.Fatalf("decode valid manifest: %v", err)
	}
	hash, err := decoded.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Fatalf("canonical hash length %d", len(hash))
	}
	rendered := RenderArtifactManifestMarkdown(decoded)
	for _, want := range []string{
		"Runtime artifact manifest (offline)",
		"does not create a run directory",
		"preserved_prior",
		"m2b-blueprint-20261105-032542+0330",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render missing %q\n%s", want, rendered)
		}
	}
	if again := RenderArtifactManifestMarkdown(decoded); again != rendered {
		t.Fatal("render is not deterministic")
	}
}

func TestArtifactManifestCanonicalOrderInvariance(t *testing.T) {
	m := validArtifactManifest()
	m.Artifacts[0], m.Artifacts[3] = m.Artifacts[3], m.Artifacts[0]
	first, err := validArtifactManifest().CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.CanonicalSHA256()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical hash changed with artifact order: %s != %s", first, second)
	}
}

func TestArtifactManifestRejectsPathReuseAndTraversal(t *testing.T) {
	m := validArtifactManifest()
	m.Artifacts[1].Path = m.Artifacts[0].Path
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "duplicate artifact path") {
		t.Fatalf("path reuse accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.Artifacts[0].Path = ArtifactRunRoot(m.RunID) + "/../descriptor.json"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("traversal accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.Artifacts[0].Path = "/abs/descriptor.json"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "relative") {
		t.Fatalf("absolute path accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.Artifacts[3].Path = ArtifactRunRoot(m.RunID) + "/old-sink.ndjson"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "must not be placed in the current run directory") {
		t.Fatalf("prior artifact in new run accepted or wrong error: %v", err)
	}
}

func TestArtifactManifestRejectsBadIdentityRoleAndSelfReference(t *testing.T) {
	m := validArtifactManifest()
	m.RunID = "other-run"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "must start with") {
		t.Fatalf("run id without date+tag prefix accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.RunDate = "2026-02-30"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "valid calendar date") {
		t.Fatalf("invalid date accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.Artifacts[0].Role = "executable"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "artifact taxonomy") {
		t.Fatalf("unknown role accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.Artifacts = append(m.Artifacts, ArtifactRecord{
		Role: ArtifactRoleOrchestrationPlan, Path: ArtifactRunRoot(m.RunID) + "/artifact-manifest.json",
		SHA256: strings.Repeat("e", 64), Bytes: 1, Origin: ArtifactThisRun, Required: true,
	})
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "must not list themselves") {
		t.Fatalf("self-referential manifest accepted or wrong error: %v", err)
	}
}

func TestArtifactManifestRejectsEmptyRunIdentifierSuffix(t *testing.T) {
	m := validArtifactManifest()
	m.RunID = "m2b-blueprint-20261105-"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "non-empty run identifier suffix") {
		t.Fatalf("exact prefix without run identifier accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	m.RunID = "m2b-blueprint-20261105--extra"
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err == nil || !strings.Contains(err.Error(), "repeats the date separator") {
		t.Fatalf("repeated date separator accepted or wrong error: %v", err)
	}

	m = validArtifactManifest()
	oldRoot := ArtifactRunRoot(m.RunID)
	m.RunID = "m2b-blueprint-20261105-x"
	for i := range m.Artifacts {
		m.Artifacts[i].Path = strings.Replace(m.Artifacts[i].Path, oldRoot, ArtifactRunRoot(m.RunID), 1)
	}
	if _, err := DecodeArtifactManifest(marshalArtifactManifest(t, m)); err != nil {
		t.Fatalf("non-empty run identifier suffix rejected: %v", err)
	}
}

func TestArtifactManifestVerify(t *testing.T) {
	root := t.TempDir()
	m := validArtifactManifest()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(m.Artifacts[0].Path, "descriptor")
	write(m.Artifacts[1].Path, "inventory")
	write(m.Artifacts[2].Path, "packet")
	write(m.Artifacts[3].Path, "old-sink")
	for i := range m.Artifacts {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(m.Artifacts[i].Path)))
		if err != nil {
			t.Fatal(err)
		}
		m.Artifacts[i].SHA256 = SHA256Bytes(b)
		m.Artifacts[i].Bytes = int64(len(b))
	}
	if _, err := m.Verify(root); err != nil {
		t.Fatalf("verify matching artifacts: %v", err)
	}

	mismatch := m
	mismatch.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
	mismatch.Artifacts[0].SHA256 = strings.Repeat("f", 64)
	results, err := mismatch.Verify(root)
	if err == nil {
		t.Fatal("mismatch was not reported")
	}
	if got := findArtifactVerification(results, m.Artifacts[0].Path); got == nil || got.Status != Fail {
		t.Fatalf("mismatch not reported as FAIL: %v %+v", err, got)
	}

	missing := m
	missing.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
	missing.Artifacts[1].Path = ArtifactRunRoot(m.RunID) + "/absent.json"
	results, err = missing.Verify(root)
	if err == nil {
		t.Fatal("missing artifact was not reported")
	}
	if got := findArtifactVerification(results, missing.Artifacts[1].Path); got == nil || got.Status != Unavailable {
		t.Fatalf("missing artifact not reported as UNAVAILABLE: %v %+v", err, got)
	}
}

func TestArtifactManifestVerifyDoesNotBindPriorManifests(t *testing.T) {
	if PriorManifestVerificationNotImplemented != "NOT_IMPLEMENTED" {
		t.Fatalf("prior-manifest marker = %q, want NOT_IMPLEMENTED", PriorManifestVerificationNotImplemented)
	}

	root := t.TempDir()
	priorPath := "results/prior-run-20260101-000000+0330/sink.ndjson"
	priorManifestPath := "results/prior-run-20260101-000000+0330/artifact-manifest.json"
	body := []byte("prior ledger bytes")

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
	write(priorPath, body)
	// The prior manifest deliberately declares a different hash. Verify must not
	// consult it, because prior-manifest binding is not implemented.
	write(priorManifestPath, []byte(`{"path":"`+priorPath+`","sha256":"`+strings.Repeat("f", 64)+`"}`))

	m := ArtifactManifest{
		SchemaVersion:  ArtifactManifestSchemaVersion,
		Interpretation: ArtifactManifestInterpretation,
		RunID:          "m2b-fix-20261105-120000+0330",
		RunDate:        "2026-11-05",
		RunTag:         "m2b-fix",
		Platform:       "all",
		Artifacts: []ArtifactRecord{{
			Role: ArtifactRoleSinkLedger, Path: priorPath,
			SHA256: SHA256Bytes(body), Bytes: int64(len(body)),
			Origin: ArtifactPreservedPrior, Required: true,
		}},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if _, err := m.Verify(root); err != nil {
		t.Fatalf("Verify unexpectedly consulted the prior manifest: %v", err)
	}
}

func findArtifactVerification(results []ArtifactVerification, path string) *ArtifactVerification {
	for i := range results {
		if results[i].Path == path {
			return &results[i]
		}
	}
	return nil
}

func TestArtifactManifestStrictKeys(t *testing.T) {
	base := string(marshalArtifactManifest(t, validArtifactManifest()))
	cases := map[string]string{
		"case alias":    strings.Replace(base, `"run_id":`, `"RUN_ID":`, 1),
		"unknown key":   strings.Replace(base, `{"schema_version"`, `{"unexpected":1,"schema_version"`, 1),
		"duplicate":     strings.Replace(base, `{"schema_version"`, `{"run_id":"x","schema_version"`, 1),
		"null field":    strings.Replace(base, `"platform":"all"`, `"platform":null`, 1),
		"trailing JSON": base + `[]`,
	}
	for name, body := range cases {
		if _, err := DecodeArtifactManifest([]byte(body)); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}

func TestArtifactPathHelper(t *testing.T) {
	got, err := ArtifactPath("m2b-blueprint-20261105-032542+0330", "summary.txt")
	if err != nil {
		t.Fatal(err)
	}
	if want := "results/m2b-blueprint-20261105-032542+0330/summary.txt"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	for _, bad := range []string{"../x", "a/b", "", "has space"} {
		if _, err := ArtifactPath("run-20261105-000000+0330", bad); err == nil {
			t.Fatalf("bad name %q accepted", bad)
		}
	}
}
