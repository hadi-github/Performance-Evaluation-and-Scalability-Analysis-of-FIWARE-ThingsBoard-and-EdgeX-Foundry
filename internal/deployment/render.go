package deployment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MarshalReport returns deterministic canonical JSON for a report. It does not
// add timestamps or any live claim.
func MarshalReport(report *Report) ([]byte, error) {
	if report == nil {
		return nil, fmt.Errorf("nil deployment report")
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// RenderMarkdown returns a deterministic, human-readable attestation. The text
// explicitly distinguishes the supplied-document attestation from live
// validation and repeats the lifecycle exclusion.
func RenderMarkdown(report *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Deployment attestation (dry run)\n\n")
	fmt.Fprintf(&b, "**Attestation: %s**. **Live validation: %s**. **Dry run: %t**.\n\n", report.Attestation, report.LiveValidation, report.DryRun)
	fmt.Fprintf(&b, "> This report is a dry-run comparison of caller-supplied documents. It does not claim that a deployment exists, that any service is reachable, or that a live validation occurred.\n\n")
	fmt.Fprintf(&b, "Interpretation: `%s`; descriptor: `%s`; inventory: `%s`; schema version: `%d`.\n\n", report.Interpretation, report.DescriptorInterpretation, report.InventoryInterpretation, report.SchemaVersion)
	fmt.Fprintf(&b, "Platform: `%s`; project: `%s`.\n\n", mdCell(report.Platform), mdCell(report.Project))
	fmt.Fprintf(&b, "Descriptor SHA-256: `%s`; canonical SHA-256: `%s`.\n\n", mdCell(report.DescriptorSHA256), mdCell(report.DescriptorCanonicalSHA256))
	fmt.Fprintf(&b, "Inventory SHA-256: `%s`; canonical SHA-256: `%s`.\n\n", mdCell(report.InventorySHA256), mdCell(report.InventoryCanonicalSHA256))

	fmt.Fprintf(&b, "## Protocol declaration\n\n")
	fmt.Fprintf(&b, "Protocol binding SHA-256: `%s`; profile SHA-256: `%s`.", mdCell(report.Protocol.BindingSHA256), mdCell(report.Protocol.ProfileSHA256))
	if report.Protocol.ProfilePath != "" {
		fmt.Fprintf(&b, " Profile path: `%s`.", mdCell(report.Protocol.ProfilePath))
	}
	fmt.Fprintf(&b, " These hashes are recorded from the descriptor and were not re-read by this report.\n\n")

	fmt.Fprintf(&b, "## Declaration provenance\n\n")
	fmt.Fprintf(&b, "- Source: `%s`\n", mdCell(report.Provenance.Source))
	if report.Provenance.ComposePath != "" {
		fmt.Fprintf(&b, "- Compose path: `%s`; SHA-256: `%s`\n", mdCell(report.Provenance.ComposePath), mdCell(report.Provenance.ComposeSHA256))
	}
	if report.Provenance.CreatedAtMS != nil {
		fmt.Fprintf(&b, "- Caller-supplied created_at_ms: `%d`\n", *report.Provenance.CreatedAtMS)
	} else {
		fmt.Fprintf(&b, "- No descriptor timestamp supplied; none was fabricated.\n")
	}
	if len(report.Provenance.Documents) > 0 {
		fmt.Fprintf(&b, "- Declared provenance documents:\n")
		for _, doc := range report.Provenance.Canonical().Documents {
			fmt.Fprintf(&b, "  - `%s` %s (`%s`)\n", mdCell(doc.Role), mdCell(doc.Path), mdCell(doc.SHA256))
		}
	}
	fmt.Fprintf(&b, "\n## Observation\n\n")
	fmt.Fprintf(&b, "- Inventory source: `%s`\n", mdCell(report.InventorySource))
	if report.InventoryCapturedAtMS != nil {
		fmt.Fprintf(&b, "- Caller-supplied captured_at_ms: `%d`\n", *report.InventoryCapturedAtMS)
	} else {
		fmt.Fprintf(&b, "- No inventory capture timestamp supplied; none was fabricated.\n")
	}
	fmt.Fprintf(&b, "\n## Observer\n\n")
	fmt.Fprintf(&b, "- Status: **%s** — %s\n", report.Observer.Status, mdCell(report.Observer.Reason))
	if report.Observer.Name != "" {
		fmt.Fprintf(&b, "- Name: `%s`\n", mdCell(report.Observer.Name))
	}
	if report.Observer.InventorySHA256 != "" {
		fmt.Fprintf(&b, "- Observer inventory SHA-256: `%s`\n", mdCell(report.Observer.InventorySHA256))
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Service results\n\n")
	fmt.Fprintf(&b, "| Service | Class | Required replicas | Observed replicas | Status |\n|---|---|---:|---:|---|\n")
	for _, svc := range report.Services {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %s |\n", mdCell(svc.Service), mdCell(string(svc.Class)), svc.RequiredReplicas, svc.ObservedReplicas, svc.Status)
	}
	fmt.Fprintf(&b, "\n")
	for _, svc := range report.Services {
		fmt.Fprintf(&b, "### %s (%s) — %s\n\n", mdCell(svc.Service), mdCell(string(svc.Class)), svc.Status)
		fmt.Fprintf(&b, "| Check | Status | Expected | Observed | Reason |\n|---|---|---|---|---|\n")
		for _, c := range svc.Checks {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", mdCell(c.Name), c.Status, mdCell(c.Expected), mdCell(c.Observed), mdCell(c.Reason))
		}
		if len(svc.Containers) > 0 {
			fmt.Fprintf(&b, "\nSupplied containers:\n\n")
			fmt.Fprintf(&b, "| Replica | Container | Image | Digest | Started at (ms) | Health | State |\n|---:|---|---|---|---:|---|---|\n")
			for _, c := range svc.Containers {
				fmt.Fprintf(&b, "| %d | %s | %s | %s | %d | %s | %s |\n", c.Replica, mdCell(c.ContainerID), mdCell(c.Image), mdCell(c.Digest), c.StartedAtMS, mdCell(c.Health), mdCell(c.State))
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Totals\n\n")
	fmt.Fprintf(&b, "| Class | Required replicas | Observed replicas | Status |\n|---|---:|---:|---|\n")
	fmt.Fprintf(&b, "| application | %d | %d | %s |\n", report.Totals.RequiredApplicationReplicas, report.Totals.ObservedApplicationReplicas, report.Totals.ApplicationStatus)
	fmt.Fprintf(&b, "| supporting | %d | %d | %s |\n", report.Totals.RequiredSupportingReplicas, report.Totals.ObservedSupportingReplicas, report.Totals.SupportingStatus)
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "## Global checks\n\n")
	fmt.Fprintf(&b, "| Check | Status | Expected | Observed | Reason |\n|---|---|---|---|---|\n")
	for _, c := range report.Checks {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", mdCell(c.Name), c.Status, mdCell(c.Expected), mdCell(c.Observed), mdCell(c.Reason))
	}
	fmt.Fprintf(&b, "\n")

	if report.Evidence != nil {
		fmt.Fprintf(&b, "## Evidence manifest\n\n")
		if report.Evidence.CapturedAtMS != nil {
			fmt.Fprintf(&b, "Caller-supplied capture timestamp: `%d` ms.\n\n", *report.Evidence.CapturedAtMS)
		} else {
			fmt.Fprintf(&b, "No capture timestamp supplied; none was fabricated.\n\n")
		}
		fmt.Fprintf(&b, "| Role | Path | SHA-256 |\n|---|---|---|\n")
		for _, doc := range report.Evidence.Canonical().Documents {
			fmt.Fprintf(&b, "| %s | %s | `%s` |\n", mdCell(doc.Role), mdCell(doc.Path), mdCell(doc.SHA256))
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Limitations\n\n")
	for _, l := range report.Limitations {
		fmt.Fprintf(&b, "- %s\n", mdCell(l))
	}
	fmt.Fprintf(&b, "\n## Non-goals and lifecycle exclusion\n\n")
	fmt.Fprintf(&b, "- No Docker client call, SUT access, deployment, apply/up/down/restart/stop/fault, load or campaign action.\n")
	fmt.Fprintf(&b, "- No live health probe, log read, exec, container stop/start or observer implementation is present.\n")
	fmt.Fprintf(&b, "- No timestamp, replica count, image digest, cpuset or configuration checksum is invented.\n")
	fmt.Fprintf(&b, "- A missing observer or missing optional declaration is NOT_ASSESSED or UNAVAILABLE, never a successful zero.\n")
	return b.String()
}

// Write creates a NEW output directory and writes the report, its Markdown
// rendering, the evidence manifest and SHA256SUMS. It never writes outside the
// specified directory and never creates a temporary state directory.
func Write(out string, report *Report, manifest *EvidenceManifest, inputs []string) error {
	if report == nil {
		return fmt.Errorf("nil deployment report")
	}
	if manifest == nil {
		return fmt.Errorf("evidence manifest is required")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if report.Evidence == nil {
		report.Evidence = manifest
	}
	if err := newDirectory(out, inputs); err != nil {
		return err
	}
	reportJSON, err := MarshalReport(report)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "deployment-validation.json"), reportJSON, 0644); err != nil {
		return err
	}
	manifestJSON, err := json.MarshalIndent(manifest.Canonical(), "", "  ")
	if err != nil {
		return err
	}
	manifestJSON = append(manifestJSON, '\n')
	if err := os.WriteFile(filepath.Join(out, "evidence-manifest.json"), manifestJSON, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "deployment-validation.md"), []byte(RenderMarkdown(report)), 0644); err != nil {
		return err
	}
	return checksums(out)
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func checksums(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var text strings.Builder
	for _, f := range entries {
		if f.IsDir() || f.Name() == "SHA256SUMS" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		fmt.Fprintf(&text, "%s  %s\n", hex.EncodeToString(h[:]), f.Name())
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(text.String()), 0644)
}

func newDirectory(out string, inputs []string) error {
	if out == "" {
		return fmt.Errorf("new output directory required")
	}
	o := canonicalPath(out)
	for _, in := range inputs {
		if in == "" {
			continue
		}
		p := canonicalPath(in)
		if o == p || strings.HasPrefix(o, p+string(os.PathSeparator)) {
			return fmt.Errorf("output must be outside input evidence: %s", in)
		}
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		return fmt.Errorf("output already exists or cannot be inspected: %s", out)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return os.Mkdir(out, 0755)
}

func canonicalPath(path string) string {
	p := absolute(path)
	if x, err := filepath.EvalSymlinks(p); err == nil {
		return x
	}
	parent := filepath.Dir(p)
	if parent == p {
		return p
	}
	return filepath.Join(canonicalPath(parent), filepath.Base(p))
}

func absolute(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return p
}
