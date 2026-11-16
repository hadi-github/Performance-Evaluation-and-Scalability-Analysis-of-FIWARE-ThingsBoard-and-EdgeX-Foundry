// Canonicalisation, packaging and Markdown rendering of the M2e health-evidence report.
package deployment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Canonical returns a deep copy with deterministic ordering. Execution step
// order and both limitation lists are preserved exactly.
func (r HealthEvidenceDryRun) Canonical() HealthEvidenceDryRun {
	out := r
	out.ExecutionPlan = r.ExecutionPlan.Canonical()
	out.Services = make([]HealthServiceResult, len(r.Services))
	for i, svc := range r.Services {
		out.Services[i] = svc
		out.Services[i].Expected = svc.Expected
		out.Services[i].Expected.HealthModes = append([]string(nil), svc.Expected.HealthModes...)
		sort.Strings(out.Services[i].Expected.HealthModes)
		out.Services[i].Observation = cloneServiceObservation(svc.Observation)
	}
	sort.SliceStable(out.Services, func(i, j int) bool {
		if out.Services[i].Expected.Tier != out.Services[j].Expected.Tier {
			return tierRank[out.Services[i].Expected.Tier] < tierRank[out.Services[j].Expected.Tier]
		}
		return out.Services[i].Service < out.Services[j].Service
	})
	out.ArtifactManifest = r.ArtifactManifest.Canonical()
	out.ArtifactVerification = make([]ArtifactVerification, len(r.ArtifactVerification))
	copy(out.ArtifactVerification, r.ArtifactVerification)
	sort.SliceStable(out.ArtifactVerification, func(i, j int) bool {
		return out.ArtifactVerification[i].Path < out.ArtifactVerification[j].Path
	})
	out.Limitations = append([]string(nil), r.Limitations...)
	return out
}

// CanonicalJSON is the deterministic compact canonical representation used for
// semantic hashing and for the canonical output file.
func (r HealthEvidenceDryRun) CanonicalJSON() ([]byte, error) {
	return json.Marshal(r.Canonical())
}

// CanonicalSHA256 is the semantic report hash.
func (r HealthEvidenceDryRun) CanonicalSHA256() (string, error) {
	b, err := r.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

// Marshal returns deterministic pretty JSON with a trailing newline. It adds no
// timestamp, environment value or live claim.
func (r *HealthEvidenceDryRun) Marshal() ([]byte, error) {
	if r == nil {
		return nil, errors.New("nil health evidence dry run")
	}
	b, err := json.MarshalIndent(r.Canonical(), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func observationExitCode(o ServiceObservation) string {
	if o.ExitCode == nil {
		return "—"
	}
	return fmt.Sprintf("%d", *o.ExitCode)
}

// RenderMarkdown returns a deterministic human-readable report. It prominently
// labels the document synthetic and limits PASS to supplied synthetic
// consistency.
func (r *HealthEvidenceDryRun) RenderMarkdown() string {
	if r == nil {
		return ""
	}
	c := r.Canonical()
	var b strings.Builder
	fmt.Fprintf(&b, "# Health evidence dry run — %s\n\n", HealthEvidenceSyntheticLabel)
	fmt.Fprintf(&b, "**Synthetic: %s. Health: %s. Evidence: %s. Dry run: %s. Live validation: %s.**\n\n",
		mdCell(c.Synthetic), c.HealthStatus, c.EvidenceStatus, c.DryRunStatus, c.LiveValidation)
	fmt.Fprintf(&b, "> This is a **synthetic, offline dry run**. It compares caller-supplied synthetic service observations with an approved execution-plan declaration. A PASS here means only that the supplied synthetic observations are internally consistent with that declaration. It is **not live qualification**, campaign success, reachability evidence or proof that any service exists.\n\n")
	fmt.Fprintf(&b, "Interpretation: `%s`; mode: `%s`; schema version: `%d`. Platform: `%s`; project: `%s`.\n\n",
		mdCell(c.Interpretation), mdCell(c.Mode), c.SchemaVersion, mdCell(c.Platform), mdCell(c.Project))
	fmt.Fprintf(&b, "Observation source: `%s`.\n\n", mdCell(c.ObservationSource))

	fmt.Fprintf(&b, "## Input identity\n\n")
	fmt.Fprintf(&b, "| Input | Raw SHA-256 | Canonical SHA-256 | Bytes |\n|---|---|---|---:|\n")
	fmt.Fprintf(&b, "| execution plan | `%s` | `%s` | %d |\n",
		mdCell(c.Inputs.ExecutionPlanRawSHA256), mdCell(c.Inputs.ExecutionPlanCanonicalSHA256), c.Inputs.ExecutionPlanBytes)
	fmt.Fprintf(&b, "| synthetic observations | `%s` | `%s` | %d |\n\n",
		mdCell(c.Inputs.ObservationsRawSHA256), mdCell(c.Inputs.ObservationsCanonicalSHA256), c.Inputs.ObservationsBytes)

	fmt.Fprintf(&b, "## Health results\n\n")
	fmt.Fprintf(&b, "| Service | Tier | Expected replicas | Present | Observed replicas | Running | Health | Exit code | Functional | Status | Reason |\n|---|---|---:|---|---:|---|---|---:|---|---|---|\n")
	for _, svc := range c.Services {
		fmt.Fprintf(&b, "| %s | %s | %d | %t | %d | %t | %s | %s | %t | %s | %s |\n",
			mdCell(svc.Service), mdCell(string(svc.Expected.Tier)), svc.Expected.Replicas,
			svc.Observation.Present, svc.Observation.Replicas, svc.Observation.Running,
			mdCell(string(svc.Observation.Health)), observationExitCode(svc.Observation),
			svc.Observation.FunctionalEvidence, svc.Status, mdCell(svc.Reason))
	}
	fmt.Fprintf(&b, "\nHealth status: **%s** (FAIL > UNAVAILABLE > NOT_ASSESSED > PASS).\n\n", c.HealthStatus)

	fmt.Fprintf(&b, "## Nested execution plan (unchanged)\n\n")
	fmt.Fprintf(&b, "Mode: `%s`; execution blocked: `%t`; authorization: `%s`.\n\n",
		mdCell(c.ExecutionPlan.Mode), c.ExecutionPlan.ExecutionBlocked, mdCell(c.ExecutionPlan.Authorization))
	fmt.Fprintf(&b, "| # | Step | Mode | Marker | Status |\n|---:|---|---|---|---|\n")
	for _, step := range c.ExecutionPlan.Steps {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			step.Order, mdCell(step.Step), mdCell(string(step.Mode)), mdCell(step.Marker), step.ExecutionStatus)
	}
	fmt.Fprintf(&b, "\n> Three execution-bearing rows remain `%s` and all seven rows remain `%s`. No row was executed.\n\n",
		ExecutionBlockedMarker, ExecutionStatusNotExecuted)

	fmt.Fprintf(&b, "## Artifact manifest and verification\n\n")
	fmt.Fprintf(&b, "Run `%s`; tag `%s`; date `%s`; platform `%s`.\n\n",
		mdCell(c.ArtifactManifest.RunID), mdCell(c.ArtifactManifest.RunTag),
		mdCell(c.ArtifactManifest.RunDate), mdCell(c.ArtifactManifest.Platform))
	fmt.Fprintf(&b, "| Role | Path | SHA-256 | Bytes | Origin | Required |\n|---|---|---|---:|---|---|\n")
	for _, artifact := range c.ArtifactManifest.Canonical().Artifacts {
		fmt.Fprintf(&b, "| %s | %s | `%s` | %d | %s | %t |\n",
			mdCell(artifact.Role), mdCell(artifact.Path), mdCell(artifact.SHA256),
			artifact.Bytes, artifact.Origin, artifact.Required)
	}
	fmt.Fprintf(&b, "\n")
	if len(c.ArtifactVerification) == 0 {
		fmt.Fprintf(&b, "Artifact verification: **%s** (no writer verification has run).\n\n", c.EvidenceStatus)
	} else {
		fmt.Fprintf(&b, "| Role | Path | Declared SHA-256 | Actual SHA-256 | Status | Reason |\n|---|---|---|---|---|---|\n")
		for _, v := range c.ArtifactVerification {
			fmt.Fprintf(&b, "| %s | %s | `%s` | `%s` | %s | %s |\n",
				mdCell(v.Role), mdCell(v.Path), mdCell(v.Declared), mdCell(v.Actual), v.Status, mdCell(v.Reason))
		}
		fmt.Fprintf(&b, "\nEvidence status: **%s** (aggregated from actual artifact verification).\n\n", c.EvidenceStatus)
	}

	fmt.Fprintf(&b, "## Status boundaries\n\n")
	fmt.Fprintf(&b, "- Live validation: **%s**.\n", c.LiveValidation)
	fmt.Fprintf(&b, "- Observer: **%s**.\n", c.ObserverStatus)
	fmt.Fprintf(&b, "- Prior-manifest verification: `%s`.\n", mdCell(c.PriorManifestVerification))
	fmt.Fprintf(&b, "- Prior-manifest binding: **%s**.\n", c.PriorManifestBindingStatus)
	fmt.Fprintf(&b, "- Dry-run status combines only health and artifact evidence: **%s**.\n\n", c.DryRunStatus)

	fmt.Fprintf(&b, "## Limitations\n\n")
	for _, limitation := range c.Limitations {
		fmt.Fprintf(&b, "- %s\n", mdCell(limitation))
	}
	fmt.Fprintf(&b, "\n## Non-goals and lifecycle exclusion\n\n")
	fmt.Fprintf(&b, "- No Docker client call, SUT access, deployment, lifecycle action, observer implementation, fault, load, reset or delete.\n")
	fmt.Fprintf(&b, "- No live health, replica, exit code, functional evidence, artifact or monitoring value is collected or fabricated.\n")
	fmt.Fprintf(&b, "- No prior artifact is copied, rewritten, deleted or bound to its own prior manifest.\n")
	fmt.Fprintf(&b, "- No timestamp, environment probe or absolute working-directory path is included.\n")
	return b.String()
}

func healthEvidenceCore(r *HealthEvidenceDryRun) *HealthEvidenceDryRun {
	out := r.Canonical()
	out.ArtifactVerification = []ArtifactVerification{}
	out.EvidenceStatus = NotAssessed
	out.DryRunStatus = aggregateStatus([]Status{out.HealthStatus, out.EvidenceStatus})
	return &out
}

// WriteHealthEvidenceDryRun creates a NEW results/<run_id> output directory,
// writes copies of the exact raw inputs, verifies them through the existing
// ArtifactManifest.Verify, records the actual verification, and writes the
// final JSON, canonical JSON, Markdown, manifest and SHA256SUMS. It never writes
// outside that directory. A health FAIL is a valid report, not a writer error.
func WriteHealthEvidenceDryRun(out string, r *HealthEvidenceDryRun, executionPlanRaw, observationsRaw []byte, inputs []string) error {
	if r == nil {
		return errors.New("nil health evidence dry run")
	}
	built, err := BuildHealthEvidenceDryRun(executionPlanRaw, observationsRaw)
	if err != nil {
		return err
	}
	wantCore, err := healthEvidenceCore(built).CanonicalJSON()
	if err != nil {
		return err
	}
	gotCore, err := healthEvidenceCore(r).CanonicalJSON()
	if err != nil {
		return err
	}
	if !bytes.Equal(wantCore, gotCore) {
		return errors.New("health evidence: report does not match the supplied raw inputs")
	}
	cleanOut := filepath.Clean(out)
	runID := built.ArtifactManifest.RunID
	expectedParent := strings.TrimSuffix(ArtifactRunDirectoryPrefix, "/")
	if filepath.Base(cleanOut) != runID || filepath.Base(filepath.Dir(cleanOut)) != expectedParent {
		return fmt.Errorf("health evidence: output must end with results/%s", runID)
	}
	verificationRoot := filepath.Dir(filepath.Dir(cleanOut))
	if err := newDirectory(out, inputs); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceExecutionPlanInputName), executionPlanRaw, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceObservationsInputName), observationsRaw, 0644); err != nil {
		return err
	}
	verifications, verifyErr := built.ArtifactManifest.Verify(verificationRoot)
	built.ArtifactVerification = verifications
	statuses := make([]Status, 0, len(verifications))
	for _, v := range verifications {
		statuses = append(statuses, v.Status)
	}
	built.EvidenceStatus = aggregateStatus(statuses)
	if len(verifications) == 0 {
		built.EvidenceStatus = NotAssessed
	}
	built.DryRunStatus = aggregateStatus([]Status{built.HealthStatus, built.EvidenceStatus})
	if err := built.Validate(); err != nil {
		return err
	}
	pretty, err := built.Marshal()
	if err != nil {
		return err
	}
	canonical, err := built.CanonicalJSON()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceReportName), pretty, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceCanonicalName), canonical, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, HealthEvidenceMarkdownName), []byte(built.RenderMarkdown()), 0644); err != nil {
		return err
	}
	manifestJSON, err := json.MarshalIndent(built.ArtifactManifest.Canonical(), "", "  ")
	if err != nil {
		return err
	}
	manifestJSON = append(manifestJSON, '\n')
	if err := os.WriteFile(filepath.Join(out, "artifact-manifest.json"), manifestJSON, 0644); err != nil {
		return err
	}
	if err := checksums(out); err != nil {
		return err
	}
	// Preserve the report even when verification found a problem, then surface
	// the verification error to the caller.
	return verifyErr
}
