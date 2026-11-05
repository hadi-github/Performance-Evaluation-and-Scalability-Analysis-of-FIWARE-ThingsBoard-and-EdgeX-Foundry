package deployment

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// EvidenceManifestSchemaVersion versions the standalone evidence manifest.
const EvidenceManifestSchemaVersion = SchemaVersion

// EvidenceManifest lists caller-supplied documents and their SHA-256 values. It
// never invents a capture timestamp: CapturedAtMS is omitted unless the caller
// explicitly supplies one.
type EvidenceManifest struct {
	SchemaVersion  int                `json:"schema_version"`
	Interpretation string             `json:"interpretation"`
	Documents      []EvidenceDocument `json:"documents"`
	CapturedAtMS   *int64             `json:"captured_at_ms,omitempty"`
}

// EvidenceVerification is the result of re-hashing one declared document.
type EvidenceVerification struct {
	Role     string `json:"role"`
	Path     string `json:"path"`
	Declared string `json:"declared_sha256"`
	Actual   string `json:"actual_sha256,omitempty"`
	Status   Status `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

// NewEvidenceManifest builds a manifest from already-computed documents. It
// validates paths/roles/hashes and sorts the documents deterministically. It
// does not add a timestamp.
func NewEvidenceManifest(docs []EvidenceDocument) (*EvidenceManifest, error) {
	m := &EvidenceManifest{
		SchemaVersion:  EvidenceManifestSchemaVersion,
		Interpretation: EvidenceManifestInterpretation,
		Documents:      append([]EvidenceDocument(nil), docs...),
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	m.Documents = m.Canonical().Documents
	return m, nil
}

// EvidenceFromBytes computes one document entry from caller-supplied bytes.
func EvidenceFromBytes(role, path string, b []byte) EvidenceDocument {
	return EvidenceDocument{Role: role, Path: path, SHA256: SHA256Bytes(b)}
}

// EvidenceFromFile reads one explicit file and computes its entry. It is a
// read-only integrity helper, not an observer.
func EvidenceFromFile(role, path string) (EvidenceDocument, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EvidenceDocument{}, err
	}
	return EvidenceFromBytes(role, path, b), nil
}

// DecodeEvidenceManifest strictly decodes a caller-supplied manifest.
func DecodeEvidenceManifest(b []byte) (*EvidenceManifest, error) {
	if err := validateRawDocument(b, evidenceManifestShape, "evidence manifest"); err != nil {
		return nil, err
	}
	var m EvidenceManifest
	if err := decodeStrict(b, &m); err != nil {
		return nil, fmt.Errorf("evidence manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks the manifest after decoding or direct construction.
func (m EvidenceManifest) Validate() error {
	if m.SchemaVersion != EvidenceManifestSchemaVersion {
		return fmt.Errorf("evidence manifest: unsupported schema_version %d", m.SchemaVersion)
	}
	if m.Interpretation != EvidenceManifestInterpretation {
		return fmt.Errorf("evidence manifest: interpretation must be %q", EvidenceManifestInterpretation)
	}
	if m.CapturedAtMS != nil && *m.CapturedAtMS < 0 {
		return errors.New("evidence manifest: captured_at_ms must be non-negative when supplied")
	}
	if len(m.Documents) == 0 {
		return errors.New("evidence manifest: documents must be a non-empty explicit array")
	}
	if err := validateEvidenceDocuments(m.Documents, "evidence manifest"); err != nil {
		return err
	}
	return nil
}

// Canonical returns a deep copy with documents sorted by role, path and hash.
func (m EvidenceManifest) Canonical() EvidenceManifest {
	out := m
	out.Documents = append([]EvidenceDocument(nil), m.Documents...)
	sort.SliceStable(out.Documents, func(i, j int) bool {
		a, b := out.Documents[i], out.Documents[j]
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.SHA256 < b.SHA256
	})
	return out
}

// CanonicalJSON is the deterministic compact manifest representation.
func (m EvidenceManifest) CanonicalJSON() ([]byte, error) {
	return json.Marshal(m.Canonical())
}

// Verify re-hashes every declared document relative to root (or directly when
// the declared path is absolute). It never writes or repairs a document.
func (m EvidenceManifest) Verify(root string) ([]EvidenceVerification, error) {
	var out []EvidenceVerification
	for _, doc := range m.Canonical().Documents {
		path := doc.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		b, err := os.ReadFile(path)
		v := EvidenceVerification{Role: doc.Role, Path: doc.Path, Declared: doc.SHA256}
		if err != nil {
			v.Status = Unavailable
			v.Reason = err.Error()
			out = append(out, v)
			continue
		}
		v.Actual = SHA256Bytes(b)
		if v.Actual == doc.SHA256 {
			v.Status = Pass
			v.Reason = "document SHA-256 matches the manifest"
		} else {
			v.Status = Fail
			v.Reason = "document SHA-256 differs from the manifest"
		}
		out = append(out, v)
	}
	for _, v := range out {
		if v.Status != Pass {
			return out, fmt.Errorf("evidence manifest verification failed for %s %s: %s", v.Role, v.Path, v.Reason)
		}
	}
	return out, nil
}
