package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// This file adds the M2b runtime artifact packaging schema. It is a pure
// document contract and read-only verifier: it never creates a run directory,
// writes an artifact, rotates a ledger or contacts a service.

const (
	// ArtifactManifestSchemaVersion versions the runtime artifact manifest.
	ArtifactManifestSchemaVersion = SchemaVersion
	// ArtifactManifestInterpretation names the exact manifest semantics.
	ArtifactManifestInterpretation = "runtime-artifact-manifest-v1"
	// ArtifactRunDirectoryPrefix is the only admitted run root prefix.
	ArtifactRunDirectoryPrefix = "results/"
)

// ArtifactOrigin distinguishes artifacts produced by this run from artifacts
// that must remain byte-identical from a prior run.
type ArtifactOrigin string

const (
	ArtifactThisRun        ArtifactOrigin = "this_run"
	ArtifactPreservedPrior ArtifactOrigin = "preserved_prior"
)

// Artifact role taxonomy. A new role requires an explicit schema revision; the
// decoder will not silently accept an unknown category.
const (
	ArtifactRoleDescriptor        = "descriptor"
	ArtifactRoleInventory         = "inventory"
	ArtifactRoleOrchestrationPlan = "orchestration_plan"
	ArtifactRoleCompose           = "compose"
	ArtifactRoleEnv               = "env"
	ArtifactRoleMakefile          = "makefile"
	ArtifactRoleProtocol          = "protocol"
	ArtifactRoleProfile           = "profile"
	ArtifactRoleRunManifest       = "run_manifest"
	ArtifactRoleK6Summary         = "k6_summary"
	ArtifactRoleSinkLedger        = "sink_ledger"
	ArtifactRoleAdapterLedger     = "adapter_ledger"
	ArtifactRoleReceiptLedger     = "receipt_ledger"
	ArtifactRoleLog               = "log"
	ArtifactRoleMonitoringExport  = "monitoring_export"
	ArtifactRoleCPUSnapshot       = "cpu_snapshot"
	ArtifactRoleClockBound        = "clock_bound"
	ArtifactRoleSHA256SUMS        = "sha256sums"
	ArtifactRolePacket            = "packet"
	ArtifactRoleOther             = "other"
)

var validArtifactRoles = map[string]bool{
	ArtifactRoleDescriptor: true, ArtifactRoleInventory: true,
	ArtifactRoleOrchestrationPlan: true, ArtifactRoleCompose: true,
	ArtifactRoleEnv: true, ArtifactRoleMakefile: true,
	ArtifactRoleProtocol: true, ArtifactRoleProfile: true,
	ArtifactRoleRunManifest: true, ArtifactRoleK6Summary: true,
	ArtifactRoleSinkLedger: true, ArtifactRoleAdapterLedger: true,
	ArtifactRoleReceiptLedger: true, ArtifactRoleLog: true,
	ArtifactRoleMonitoringExport: true, ArtifactRoleCPUSnapshot: true,
	ArtifactRoleClockBound: true, ArtifactRoleSHA256SUMS: true,
	ArtifactRolePacket: true, ArtifactRoleOther: true,
}

var (
	artifactRunIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
	artifactDatePattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ArtifactRecord is one packaged file. Paths are repository-relative and use
// forward slashes. Bytes and SHA-256 are caller-supplied declarations, never
// inferred from an unread file.
type ArtifactRecord struct {
	Role     string         `json:"role"`
	Path     string         `json:"path"`
	SHA256   string         `json:"sha256"`
	Bytes    int64          `json:"bytes"`
	Origin   ArtifactOrigin `json:"origin"`
	Required bool           `json:"required"`
}

// PriorManifestVerificationScope records the exact scope of prior-manifest
// binding implemented by this package. It is an explicit non-implementation
// marker: ArtifactManifest.Verify does not read a prior run's manifest.
type PriorManifestVerificationScope string

const (
	// PriorManifestVerificationNotImplemented means a preserved_prior artifact
	// is checked only against the SHA-256 and byte count declared in the current
	// manifest. It is not bound to the manifest of its own prior run; that is a
	// future, separately reviewed packaging capability.
	PriorManifestVerificationNotImplemented PriorManifestVerificationScope = "NOT_IMPLEMENTED"
)

// ArtifactManifest is the strict M2b runtime artifact manifest schema v1. It
// never creates a timestamp; CreatedAtMS is only copied from the caller.
type ArtifactManifest struct {
	SchemaVersion  int              `json:"schema_version"`
	Interpretation string           `json:"interpretation"`
	RunID          string           `json:"run_id"`
	RunDate        string           `json:"run_date"`
	RunTag         string           `json:"run_tag"`
	Platform       string           `json:"platform"`
	CreatedAtMS    *int64           `json:"created_at_ms,omitempty"`
	Artifacts      []ArtifactRecord `json:"artifacts"`
}

// ArtifactVerification is the result of re-hashing one declared artifact.
type ArtifactVerification struct {
	Role          string `json:"role"`
	Path          string `json:"path"`
	Declared      string `json:"declared_sha256"`
	Actual        string `json:"actual_sha256,omitempty"`
	DeclaredBytes int64  `json:"declared_bytes"`
	ActualBytes   int64  `json:"actual_bytes,omitempty"`
	Status        Status `json:"status"`
	Reason        string `json:"reason,omitempty"`
}

var artifactManifestShape = jsonShape{
	object: map[string]jsonShape{
		"schema_version": scalarValueShape,
		"interpretation": scalarValueShape,
		"run_id":         scalarValueShape,
		"run_date":       scalarValueShape,
		"run_tag":        scalarValueShape,
		"platform":       scalarValueShape,
		"created_at_ms":  scalarValueShape,
		"artifacts": {element: &jsonShape{
			object: map[string]jsonShape{
				"role":     scalarValueShape,
				"path":     scalarValueShape,
				"sha256":   scalarValueShape,
				"bytes":    scalarValueShape,
				"origin":   scalarValueShape,
				"required": scalarValueShape,
			},
		}},
	},
	optional: map[string]bool{"created_at_ms": true},
}

// DecodeArtifactManifest strictly decodes an M2b artifact manifest using the
// same exact-key, duplicate-key and null rules as the approved M2 documents.
func DecodeArtifactManifest(b []byte) (*ArtifactManifest, error) {
	if err := validateRawDocument(b, artifactManifestShape, "artifact manifest"); err != nil {
		return nil, err
	}
	var m ArtifactManifest
	if err := decodeStrict(b, &m); err != nil {
		return nil, fmt.Errorf("artifact manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks the manifest after decoding or direct construction.
func (m ArtifactManifest) Validate() error {
	if m.SchemaVersion != ArtifactManifestSchemaVersion {
		return fmt.Errorf("artifact manifest: unsupported schema_version %d", m.SchemaVersion)
	}
	if m.Interpretation != ArtifactManifestInterpretation {
		return fmt.Errorf("artifact manifest: interpretation must be %q", ArtifactManifestInterpretation)
	}
	if !artifactRunIDPattern.MatchString(m.RunID) {
		return fmt.Errorf("artifact manifest: run_id %q is not a bounded identifier", m.RunID)
	}
	if !artifactDatePattern.MatchString(m.RunDate) {
		return errors.New("artifact manifest: run_date must be YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", m.RunDate); err != nil {
		return errors.New("artifact manifest: run_date is not a valid calendar date")
	}
	if !artifactRunIDPattern.MatchString(m.RunTag) {
		return fmt.Errorf("artifact manifest: run_tag %q is not a bounded identifier", m.RunTag)
	}
	wantPrefix := m.RunTag + "-" + strings.ReplaceAll(m.RunDate, "-", "") + "-"
	if !strings.HasPrefix(m.RunID, wantPrefix) {
		return fmt.Errorf("artifact manifest: run_id %q must start with %q (date+tag+run identifier)", m.RunID, wantPrefix)
	}
	runIdentifier := strings.TrimPrefix(m.RunID, wantPrefix)
	if runIdentifier == "" {
		return fmt.Errorf("artifact manifest: run_id %q must include a non-empty run identifier suffix after %q", m.RunID, wantPrefix)
	}
	if strings.HasPrefix(runIdentifier, "-") {
		return fmt.Errorf("artifact manifest: run_id %q repeats the date separator; the run identifier must not start with \"-\"", m.RunID)
	}
	if m.Platform != "all" && !platformAllowlisted(m.Platform) {
		return fmt.Errorf("artifact manifest: platform %q must be thingsboard, fiware, edgex or all", m.Platform)
	}
	if m.CreatedAtMS != nil && *m.CreatedAtMS < 0 {
		return errors.New("artifact manifest: created_at_ms must be non-negative when supplied")
	}
	if len(m.Artifacts) == 0 {
		return errors.New("artifact manifest: artifacts must be a non-empty explicit array")
	}
	runRoot := ArtifactRunRoot(m.RunID)
	seenPaths := map[string]bool{}
	required := 0
	for i, artifact := range m.Artifacts {
		context := fmt.Sprintf("artifact manifest artifact %d", i)
		if !validArtifactRoles[artifact.Role] {
			return fmt.Errorf("%s: role %q is not in the artifact taxonomy", context, artifact.Role)
		}
		if err := validateArtifactPath(artifact.Path); err != nil {
			return fmt.Errorf("%s: %w", context, err)
		}
		if seenPaths[artifact.Path] {
			return fmt.Errorf("%s: duplicate artifact path %q (path reuse is prohibited)", context, artifact.Path)
		}
		seenPaths[artifact.Path] = true
		if !sha256Pattern.MatchString(artifact.SHA256) {
			return fmt.Errorf("%s: sha256 must be 64 lower-case hex characters", context)
		}
		if artifact.Bytes <= 0 {
			return fmt.Errorf("%s: bytes must be positive", context)
		}
		switch artifact.Origin {
		case ArtifactThisRun:
			if !strings.HasPrefix(artifact.Path, runRoot+"/") && !strings.HasPrefix(artifact.Path, "docs/archive/sessions/") {
				return fmt.Errorf("%s: this_run artifact path must be under %s/ or docs/archive/sessions/", context, runRoot)
			}
		case ArtifactPreservedPrior:
			if strings.HasPrefix(artifact.Path, runRoot+"/") {
				return fmt.Errorf("%s: preserved_prior artifact must not be placed in the current run directory", context)
			}
			if !strings.HasPrefix(artifact.Path, "results/") && !strings.HasPrefix(artifact.Path, "docs/") {
				return fmt.Errorf("%s: preserved_prior artifact path must remain under results/ or docs/", context)
			}
		default:
			return fmt.Errorf("%s: origin %q must be this_run or preserved_prior", context, artifact.Origin)
		}
		if artifact.Path == runRoot+"/artifact-manifest.json" || artifact.Path == runRoot+"/SHA256SUMS" {
			return fmt.Errorf("%s: manifest and checksum files must not list themselves", context)
		}
		if artifact.Required {
			required++
		}
	}
	if required == 0 {
		return errors.New("artifact manifest: at least one artifact must be required")
	}
	return nil
}

// ArtifactRunRoot returns the only admitted run directory for a validated run
// identifier.
func ArtifactRunRoot(runID string) string {
	return ArtifactRunDirectoryPrefix + runID
}

// ArtifactPath builds a repository-relative path inside the run directory.
// Callers still validate the resulting manifest.
func ArtifactPath(runID, name string) (string, error) {
	if !artifactRunIDPattern.MatchString(runID) {
		return "", fmt.Errorf("artifact path: run_id %q is not a bounded identifier", runID)
	}
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("artifact path: name must be a single non-empty path element")
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", errors.New("artifact path: name must not contain control or space characters")
		}
	}
	return ArtifactRunRoot(runID) + "/" + name, nil
}

func validateArtifactPath(p string) error {
	if p == "" {
		return errors.New("path is required")
	}
	for _, r := range p {
		if unicode.IsControl(r) || unicode.IsSpace(r) || r == '\\' {
			return fmt.Errorf("path %q must use forward slashes and contain no whitespace or control characters", p)
		}
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, ":") {
		return fmt.Errorf("path %q must be relative", p)
	}
	if path.Clean(p) != p {
		return fmt.Errorf("path %q must be clean and must not contain . or .. elements", p)
	}
	for _, element := range strings.Split(p, "/") {
		if element == "" || element == "." || element == ".." {
			return fmt.Errorf("path %q must not contain empty, . or .. elements", p)
		}
	}
	return nil
}

// Canonical returns a deep copy with artifacts sorted deterministically.
func (m ArtifactManifest) Canonical() ArtifactManifest {
	out := m
	out.Artifacts = append([]ArtifactRecord(nil), m.Artifacts...)
	sort.SliceStable(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].Path < out.Artifacts[j].Path })
	return out
}

// CanonicalJSON is the deterministic compact manifest representation.
func (m ArtifactManifest) CanonicalJSON() ([]byte, error) {
	return json.Marshal(m.Canonical())
}

// CanonicalSHA256 is the semantic manifest hash.
func (m ArtifactManifest) CanonicalSHA256() (string, error) {
	b, err := m.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

// Verify re-hashes every path declared in this manifest relative to root. It
// does not discover or read a prior run's manifest, so it does not bind a
// preserved_prior artifact to its own prior manifest; that capability is
// explicitly not implemented (PriorManifestVerificationNotImplemented). It
// never writes, repairs, moves or deletes a file.
func (m ArtifactManifest) Verify(root string) ([]ArtifactVerification, error) {
	out := make([]ArtifactVerification, 0, len(m.Canonical().Artifacts))
	for _, artifact := range m.Canonical().Artifacts {
		file := filepath.Join(root, filepath.FromSlash(artifact.Path))
		v := ArtifactVerification{
			Role: artifact.Role, Path: artifact.Path,
			Declared: artifact.SHA256, DeclaredBytes: artifact.Bytes,
		}
		b, err := os.ReadFile(file)
		if err != nil {
			v.Status = Unavailable
			v.Reason = err.Error()
			out = append(out, v)
			continue
		}
		v.Actual = SHA256Bytes(b)
		v.ActualBytes = int64(len(b))
		switch {
		case v.Actual != artifact.SHA256:
			v.Status = Fail
			v.Reason = "artifact SHA-256 differs from the manifest"
		case v.ActualBytes != artifact.Bytes:
			v.Status = Fail
			v.Reason = "artifact byte count differs from the manifest"
		default:
			v.Status = Pass
			v.Reason = "artifact SHA-256 and byte count match the manifest"
		}
		out = append(out, v)
	}
	for _, v := range out {
		if v.Status != Pass {
			return out, fmt.Errorf("artifact manifest verification failed for %s %s: %s", v.Role, v.Path, v.Reason)
		}
	}
	return out, nil
}

// RenderArtifactManifestMarkdown returns a deterministic human-readable
// packaging manifest. It states the offline and no-write constraints.
func RenderArtifactManifestMarkdown(m *ArtifactManifest) string {
	if m == nil {
		return ""
	}
	c := m.Canonical()
	var b strings.Builder
	fmt.Fprintf(&b, "# Runtime artifact manifest (offline)\n\n")
	fmt.Fprintf(&b, "Run `%s`; tag `%s`; date `%s`; platform `%s`.\n\n",
		mdCell(c.RunID), mdCell(c.RunTag), mdCell(c.RunDate), mdCell(c.Platform))
	fmt.Fprintf(&b, "> This manifest declares packaged artifact identity only. It does not create a run directory, copy, move, rewrite, delete or upload any artifact, and it makes no live-system claim.\n\n")
	fmt.Fprintf(&b, "Interpretation: `%s`; schema version: `%d`.\n\n", mdCell(c.Interpretation), c.SchemaVersion)
	if c.CreatedAtMS != nil {
		fmt.Fprintf(&b, "Caller-supplied created_at_ms: `%d`.\n\n", *c.CreatedAtMS)
	} else {
		fmt.Fprintf(&b, "No manifest timestamp supplied; none was fabricated.\n\n")
	}
	fmt.Fprintf(&b, "| Role | Path | SHA-256 | Bytes | Origin | Required |\n|---|---|---|---:|---|---|\n")
	for _, artifact := range c.Artifacts {
		fmt.Fprintf(&b, "| %s | %s | `%s` | %d | %s | %t |\n",
			mdCell(artifact.Role), mdCell(artifact.Path), mdCell(artifact.SHA256),
			artifact.Bytes, artifact.Origin, artifact.Required)
	}
	fmt.Fprintf(&b, "\n## Non-goals and lifecycle exclusion\n\n")
	fmt.Fprintf(&b, "- No run directory, artifact, ledger, receiver, `.env` or compose file is created, copied, rewritten or deleted.\n")
	fmt.Fprintf(&b, "- No Docker client, SUT, deployment, lifecycle, monitoring or load action is performed.\n")
	fmt.Fprintf(&b, "- A missing artifact is UNAVAILABLE, not PASS or zero; a mismatched artifact is FAIL.\n")
	fmt.Fprintf(&b, "- Prior-manifest binding is NOT_IMPLEMENTED: Verify checks only the paths declared in this manifest.\n")
	return b.String()
}
