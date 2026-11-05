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

const m2eTestdata = "testdata/m2e"

func m2eRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(m2eTestdata, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func m2eObservationsRaw(t *testing.T, platform string) []byte {
	t.Helper()
	return m2eRead(t, "synthetic-observations-"+platform+"-pass.json")
}

func m2ePlanRaw(t *testing.T, platform string) []byte {
	t.Helper()
	return mustMarshal(t, m2dBuild(t, platform))
}

func m2eBuild(t *testing.T, platform string) *HealthEvidenceDryRun {
	t.Helper()
	r, err := BuildHealthEvidenceDryRun(m2ePlanRaw(t, platform), m2eObservationsRaw(t, platform))
	if err != nil {
		t.Fatalf("build %s health evidence: %v", platform, err)
	}
	return r
}

func m2eDecodeFixture(t *testing.T, platform string) *SyntheticServiceObservations {
	t.Helper()
	o, err := DecodeSyntheticServiceObservations(m2eObservationsRaw(t, platform))
	if err != nil {
		t.Fatalf("decode %s observations: %v", platform, err)
	}
	return o
}

func m2eMutatedObservations(t *testing.T, platform string, mutate func(*SyntheticServiceObservations)) []byte {
	t.Helper()
	o := m2eDecodeFixture(t, platform)
	mutate(o)
	return mustMarshal(t, o.Canonical())
}

func m2eResultByService(t *testing.T, r *HealthEvidenceDryRun, service string) HealthServiceResult {
	t.Helper()
	for _, svc := range r.Services {
		if svc.Service == service {
			return svc
		}
	}
	t.Fatalf("service %q not found in health evidence", service)
	return HealthServiceResult{}
}

func m2eRemoveObservation(o *SyntheticServiceObservations, service string) {
	out := o.Observations[:0]
	for _, obs := range o.Observations {
		if obs.Service != service {
			out = append(out, obs)
		}
	}
	o.Observations = out
}

func TestM2ESyntheticObservationsFixturesAndStrictDecode(t *testing.T) {
	for _, platform := range []string{"thingsboard", "fiware", "edgex"} {
		t.Run(platform, func(t *testing.T) {
			raw := m2eObservationsRaw(t, platform)
			o, err := DecodeSyntheticServiceObservations(raw)
			if err != nil {
				t.Fatal(err)
			}
			if o.SchemaVersion != SyntheticObservationsSchemaVersion ||
				o.Interpretation != SyntheticObservationsInterpretation {
				t.Fatalf("observation document identity is wrong: %+v", o)
			}
			if !strings.HasPrefix(o.Source, SyntheticSourcePrefix) {
				t.Fatalf("source is not synthetic-labeled: %q", o.Source)
			}
			if !strings.HasPrefix(o.RunTag, SyntheticRunTagPrefix) {
				t.Fatalf("run tag is not synthetic-labeled: %q", o.RunTag)
			}
			canonical, err := o.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			hash, err := o.CanonicalSHA256()
			if err != nil {
				t.Fatal(err)
			}
			if hash != SHA256Bytes(canonical) {
				t.Fatalf("canonical hash %s is not SHA-256(canonical JSON) %s", hash, SHA256Bytes(canonical))
			}
			if SHA256Bytes(raw) == hash {
				t.Fatal("raw and canonical hashes unexpectedly coincide")
			}
			for i := 1; i < len(o.Canonical().Observations); i++ {
				prev := o.Canonical().Observations[i-1].Service
				cur := o.Canonical().Observations[i].Service
				if prev >= cur {
					t.Fatalf("canonical observations are not sorted: %q then %q", prev, cur)
				}
			}
			roundTrip, err := DecodeSyntheticServiceObservations(canonical)
			if err != nil {
				t.Fatalf("canonical observations do not round-trip: %v", err)
			}
			again, err := roundTrip.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again, canonical) {
				t.Fatal("round-tripped observations are not canonical")
			}
		})
	}

	base := m2eObservationsRaw(t, "thingsboard")

	t.Run("empty observations allowed", func(t *testing.T) {
		raw := m2eMutatedObservations(t, "thingsboard", func(o *SyntheticServiceObservations) {
			o.Observations = []ServiceObservation{}
			o.RunID = "m2e-synthetic-empty-20261105-01"
			o.RunTag = "m2e-synthetic-empty"
		})
		o, err := DecodeSyntheticServiceObservations(raw)
		if err != nil {
			t.Fatalf("explicit empty observations were rejected: %v", err)
		}
		if len(o.Observations) != 0 {
			t.Fatalf("observations = %d, want 0", len(o.Observations))
		}
	})

	strict := []struct {
		name string
		raw  []byte
	}{
		{"case alias", bytes.Replace(base, []byte(`"observations":`), []byte(`"Observations":`), 1)},
		{"unknown top-level key", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			m["unexpected"] = true
			b, _ := json.Marshal(m)
			return b
		}()},
		{"duplicate JSON key", bytes.Replace(base, []byte(`"run_id":`), []byte(`"run_id":"duplicate","run_id":`), 1)},
		{"explicit null", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			m["observations"] = nil
			b, _ := json.Marshal(m)
			return b
		}()},
		{"trailing JSON", append(append([]byte(nil), base...), []byte("{}")...)},
		{"missing observations", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			delete(m, "observations")
			b, _ := json.Marshal(m)
			return b
		}()},
		{"bad schema version", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			m["schema_version"] = 2
			b, _ := json.Marshal(m)
			return b
		}()},
		{"non-synthetic source", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			m["source"] = "caller-supplied observations"
			b, _ := json.Marshal(m)
			return b
		}()},
		{"non-synthetic run tag", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			m["run_tag"] = "other-tag"
			b, _ := json.Marshal(m)
			return b
		}()},
		{"run id does not match tag/date", func() []byte {
			var m map[string]any
			if err := json.Unmarshal(base, &m); err != nil {
				t.Fatal(err)
			}
			m["run_id"] = "wrong-run-id"
			b, _ := json.Marshal(m)
			return b
		}()},
	}
	for _, tc := range strict {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeSyntheticServiceObservations(tc.raw); err == nil {
				t.Fatalf("strict decoder accepted %s", tc.name)
			}
		})
	}

	semantic := []struct {
		name   string
		mutate func(*SyntheticServiceObservations)
	}{
		{"duplicate service", func(o *SyntheticServiceObservations) {
			o.Observations = append(o.Observations, o.Observations[0])
		}},
		{"invalid service name", func(o *SyntheticServiceObservations) {
			o.Observations[0].Service = "bad service!"
		}},
		{"negative replicas", func(o *SyntheticServiceObservations) {
			o.Observations[0].Replicas = -1
		}},
		{"invalid health", func(o *SyntheticServiceObservations) {
			o.Observations[0].Health = "starting"
		}},
	}
	for _, tc := range semantic {
		t.Run(tc.name, func(t *testing.T) {
			o := m2eDecodeFixture(t, "thingsboard")
			tc.mutate(o)
			if err := o.Validate(); err == nil {
				t.Fatalf("semantic validation accepted %s", tc.name)
			}
			if _, err := DecodeSyntheticServiceObservations(mustMarshal(t, o)); err == nil {
				t.Fatalf("decoder accepted %s", tc.name)
			}
		})
	}
}

func TestM2EAllPlatformsSyntheticPass(t *testing.T) {
	wantServices := map[string][]string{
		"thingsboard": {
			"tb-core", "tb-http-transport", "tb-rule-engine",
			"haproxy", "kafka", "postgres", "tb-install", "valkey", "zookeeper",
		},
		"fiware": {"iot-agent", "orion", "haproxy", "mongo", "mongo-init"},
		"edgex": {
			"core-data", "device-rest",
			"app-http-export", "core-common-config-bootstrapper", "core-keeper",
			"core-metadata", "database", "haproxy", "nats", "ui",
		},
	}
	for _, platform := range []string{"thingsboard", "fiware", "edgex"} {
		t.Run(platform, func(t *testing.T) {
			planRaw := m2ePlanRaw(t, platform)
			obsRaw := m2eObservationsRaw(t, platform)
			r1, err := BuildHealthEvidenceDryRun(planRaw, obsRaw)
			if err != nil {
				t.Fatal(err)
			}
			r2, err := BuildHealthEvidenceDryRun(planRaw, obsRaw)
			if err != nil {
				t.Fatal(err)
			}
			if r1.SchemaVersion != HealthEvidenceSchemaVersion ||
				r1.Interpretation != HealthEvidenceInterpretation ||
				r1.Mode != HealthEvidenceMode ||
				r1.Synthetic != HealthEvidenceSyntheticLabel {
				t.Fatalf("report identity is wrong: %+v", r1)
			}
			if r1.Platform != platform {
				t.Fatalf("platform = %q, want %q", r1.Platform, platform)
			}
			if r1.HealthStatus != Pass {
				t.Fatalf("health_status = %s, want PASS", r1.HealthStatus)
			}
			if r1.EvidenceStatus != NotAssessed {
				t.Fatalf("pure evidence_status = %s, want NOT_ASSESSED", r1.EvidenceStatus)
			}
			if r1.DryRunStatus != NotAssessed {
				t.Fatalf("pure dry_run_status = %s, want NOT_ASSESSED", r1.DryRunStatus)
			}
			if r1.LiveValidation != NotAssessed || r1.ObserverStatus != NotAssessed ||
				r1.PriorManifestVerification != string(PriorManifestVerificationNotImplemented) ||
				r1.PriorManifestBindingStatus != NotAssessed {
				t.Fatalf("status boundaries are wrong: %+v", r1)
			}
			var got []string
			for _, svc := range r1.Services {
				got = append(got, svc.Service)
				if svc.Status != Pass {
					t.Fatalf("service %s status = %s (%s), want PASS", svc.Service, svc.Status, svc.Reason)
				}
			}
			if !equalStrings(got, wantServices[platform]) {
				t.Fatalf("services = %v, want %v", got, wantServices[platform])
			}
			if r1.Inputs.ExecutionPlanRawSHA256 != SHA256Bytes(planRaw) ||
				r1.Inputs.ObservationsRawSHA256 != SHA256Bytes(obsRaw) ||
				r1.Inputs.ExecutionPlanBytes != int64(len(planRaw)) ||
				r1.Inputs.ObservationsBytes != int64(len(obsRaw)) {
				t.Fatalf("input identity is wrong: %+v", r1.Inputs)
			}
			if len(r1.ArtifactManifest.Artifacts) != 2 {
				t.Fatalf("manifest artifacts = %d, want 2", len(r1.ArtifactManifest.Artifacts))
			}
			if len(r1.ExecutionPlan.Steps) != 7 {
				t.Fatalf("nested execution steps = %d, want 7", len(r1.ExecutionPlan.Steps))
			}
			blocked := 0
			for _, step := range r1.ExecutionPlan.Steps {
				if step.ExecutionStatus != ExecutionStatusNotExecuted {
					t.Fatalf("step %s status = %s, want NOT EXECUTED", step.Step, step.ExecutionStatus)
				}
				if step.Mode == StepModeExecutionBlocked {
					blocked++
					if !step.ExecutionBlockedHere || step.Marker != ExecutionBlockedMarker {
						t.Fatalf("step %s is not blocked", step.Step)
					}
				}
			}
			if blocked != 3 {
				t.Fatalf("blocked execution rows = %d, want 3", blocked)
			}
			if !r1.ExecutionPlan.ExecutionBlocked {
				t.Fatal("nested execution plan is not blocked")
			}
			c1, err := r1.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			c2, err := r2.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(c1, c2) {
				t.Fatal("canonical report bytes differ across builds")
			}
			h1, err := r1.CanonicalSHA256()
			if err != nil {
				t.Fatal(err)
			}
			h2, err := r2.CanonicalSHA256()
			if err != nil {
				t.Fatal(err)
			}
			if h1 != h2 || h1 != SHA256Bytes(c1) {
				t.Fatalf("canonical hash = %s/%s, SHA(bytes)=%s", h1, h2, SHA256Bytes(c1))
			}
			pretty, err := r1.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeHealthEvidenceDryRun(pretty)
			if err != nil {
				t.Fatalf("pretty report does not round-trip: %v", err)
			}
			decodedCanonical, err := decoded.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decodedCanonical, c1) {
				t.Fatal("round-tripped report is not canonical")
			}
			if _, err := DecodeHealthEvidenceDryRun(c1); err != nil {
				t.Fatalf("canonical report does not round-trip: %v", err)
			}
			md := r1.RenderMarkdown()
			if md != r2.RenderMarkdown() {
				t.Fatal("Markdown render is not deterministic")
			}
			for _, needle := range []string{
				HealthEvidenceSyntheticLabel,
				"not live qualification",
				ExecutionBlockedMarker,
				"NOT EXECUTED",
				"health_status",
			} {
				if needle == "health_status" {
					continue
				}
				if !strings.Contains(md, needle) {
					t.Fatalf("Markdown lacks %q:\n%s", needle, md)
				}
			}
			if !strings.Contains(md, "Health status: **PASS**") ||
				!strings.Contains(md, "Evidence: NOT_ASSESSED") {
				t.Fatalf("Markdown does not carry health/evidence status:\n%s", md)
			}
		})
	}
}

func TestM2EHealthSemantics(t *testing.T) {
	cases := []struct {
		name        string
		platform    string
		mutate      func(*SyntheticServiceObservations)
		service     string
		wantService Status
		wantHealth  Status
	}{
		{
			"declared unhealthy is FAIL", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[2] // mongo
				obs.Health = ObservedUnhealthy
			},
			"mongo", Fail, Fail,
		},
		{
			"declared none is UNAVAILABLE", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[2]
				obs.Health = ObservedHealthNone
			},
			"mongo", Unavailable, Unavailable,
		},
		{
			"declared unknown is UNAVAILABLE", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[2]
				obs.Health = ObservedUnknown
			},
			"mongo", Unavailable, Unavailable,
		},
		{
			"no healthcheck without functional evidence is UNAVAILABLE", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[4] // orion
				obs.FunctionalEvidence = false
			},
			"orion", Unavailable, Unavailable,
		},
		{
			"persistent not running is FAIL", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[4]
				obs.Running = false
			},
			"orion", Fail, Fail,
		},
		{
			"persistent replica mismatch is FAIL", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[4]
				obs.Replicas = 2
			},
			"orion", Fail, Fail,
		},
		{
			"one shot nonzero exit is FAIL", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[3] // mongo-init
				one := 1
				obs.ExitCode = &one
			},
			"mongo-init", Fail, Fail,
		},
		{
			"one shot missing exit is UNAVAILABLE", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[3]
				obs.ExitCode = nil
			},
			"mongo-init", Unavailable, Unavailable,
		},
		{
			"one shot running and healthy without exit is UNAVAILABLE", "fiware",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[3]
				obs.Running = true
				obs.Health = ObservedHealthy
				obs.ExitCode = nil
			},
			"mongo-init", Unavailable, Unavailable,
		},
		{
			"activated optional one shot success is PASS", "thingsboard",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[5] // tb-install
				zero := 0
				obs.ExitCode = &zero
			},
			"tb-install", Pass, Pass,
		},
		{
			"activated optional one shot failure is FAIL", "thingsboard",
			func(o *SyntheticServiceObservations) {
				obs := &o.Observations[5]
				one := 1
				obs.ExitCode = &one
			},
			"tb-install", Fail, Fail,
		},
		{
			"absent default service is UNAVAILABLE", "fiware",
			func(o *SyntheticServiceObservations) { m2eRemoveObservation(o, "orion") },
			"orion", Unavailable, Unavailable,
		},
		{
			"absent optional persistent is NOT_ASSESSED", "edgex",
			func(o *SyntheticServiceObservations) { m2eRemoveObservation(o, "ui") },
			"ui", NotAssessed, NotAssessed,
		},
		{
			"absent optional one shot is NOT_ASSESSED", "thingsboard",
			func(o *SyntheticServiceObservations) { m2eRemoveObservation(o, "tb-install") },
			"tb-install", NotAssessed, NotAssessed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := m2eMutatedObservations(t, tc.platform, tc.mutate)
			r, err := BuildHealthEvidenceDryRun(m2ePlanRaw(t, tc.platform), raw)
			if err != nil {
				t.Fatalf("semantically decodable observation was rejected: %v", err)
			}
			got := m2eResultByService(t, r, tc.service)
			if got.Status != tc.wantService {
				t.Fatalf("%s status = %s (%s), want %s", tc.service, got.Status, got.Reason, tc.wantService)
			}
			if r.HealthStatus != tc.wantHealth {
				t.Fatalf("health_status = %s, want %s", r.HealthStatus, tc.wantHealth)
			}
			if strings.TrimSpace(got.Reason) == "" {
				t.Fatalf("%s has an empty reason", tc.service)
			}
		})
	}

	t.Run("missing observation is explicit and invents nothing", func(t *testing.T) {
		r, err := BuildHealthEvidenceDryRun(m2ePlanRaw(t, "fiware"), m2eMutatedObservations(t, "fiware", func(o *SyntheticServiceObservations) {
			m2eRemoveObservation(o, "orion")
		}))
		if err != nil {
			t.Fatal(err)
		}
		got := m2eResultByService(t, r, "orion")
		if got.Observation.Present || got.Observation.Replicas != 0 ||
			got.Observation.Running || got.Observation.ExitCode != nil || got.Observation.FunctionalEvidence {
			t.Fatalf("omitted observation invented values: %+v", got.Observation)
		}
		if got.Observation.Service != "orion" || got.Observation.Health != ObservedUnknown {
			t.Fatalf("omitted observation identity is wrong: %+v", got.Observation)
		}
	})
}

func TestM2ERejectsUnknownObservationServiceAndUnblockedPlan(t *testing.T) {
	unknown := m2eMutatedObservations(t, "fiware", func(o *SyntheticServiceObservations) {
		o.Observations = append(o.Observations, ServiceObservation{
			Service: "unknown-service", Present: true, Replicas: 1, Running: true,
			Health: ObservedHealthNone, FunctionalEvidence: true,
		})
	})
	if _, err := BuildHealthEvidenceDryRun(m2ePlanRaw(t, "fiware"), unknown); err == nil {
		t.Fatal("observation for an undeclared service was accepted")
	}

	plan := m2dBuild(t, "fiware").Canonical()
	unblocked := plan
	unblocked.ExecutionBlocked = false
	if _, err := BuildHealthEvidenceDryRun(mustMarshal(t, unblocked), m2eObservationsRaw(t, "fiware")); err == nil {
		t.Fatal("unblocked execution plan was accepted")
	}
	tamperedStep := plan
	tamperedStep.Steps[3].ExecutionStatus = "EXECUTED"
	if _, err := BuildHealthEvidenceDryRun(mustMarshal(t, tamperedStep), m2eObservationsRaw(t, "fiware")); err == nil {
		t.Fatal("executed step status was accepted")
	}
	readOnlyBlocked := plan
	readOnlyBlocked.Steps[0].ExecutionBlockedHere = true
	if _, err := BuildHealthEvidenceDryRun(mustMarshal(t, readOnlyBlocked), m2eObservationsRaw(t, "fiware")); err == nil {
		t.Fatal("mislabeled read-only step was accepted")
	}
	if _, err := BuildHealthEvidenceDryRun(m2ePlanRaw(t, "fiware"), []byte("{}")); err == nil {
		t.Fatal("invalid observation document was accepted")
	}
	if _, err := BuildHealthEvidenceDryRun(nil, nil); err == nil {
		t.Fatal("nil inputs were accepted")
	}
}

func TestM2EOutputTamperingRejected(t *testing.T) {
	base := m2eBuild(t, "thingsboard")
	cases := []struct {
		name   string
		mutate func(*HealthEvidenceDryRun)
	}{
		{"service status", func(r *HealthEvidenceDryRun) { r.Services[0].Status = Fail }},
		{"service reason", func(r *HealthEvidenceDryRun) { r.Services[0].Reason = "tampered" }},
		{"service observation health", func(r *HealthEvidenceDryRun) { r.Services[0].Observation.Health = "starting" }},
		{"service observation name", func(r *HealthEvidenceDryRun) { r.Services[0].Observation.Service = "other" }},
		{"expected declaration", func(r *HealthEvidenceDryRun) { r.Services[0].Expected.Tier = Supporting }},
		{"live validation", func(r *HealthEvidenceDryRun) { r.LiveValidation = Pass }},
		{"observer", func(r *HealthEvidenceDryRun) { r.ObserverStatus = Pass }},
		{"prior verification", func(r *HealthEvidenceDryRun) { r.PriorManifestVerification = "PASS" }},
		{"prior binding", func(r *HealthEvidenceDryRun) { r.PriorManifestBindingStatus = Pass }},
		{"execution unblocked", func(r *HealthEvidenceDryRun) { r.ExecutionPlan.ExecutionBlocked = false }},
		{"execution step status", func(r *HealthEvidenceDryRun) { r.ExecutionPlan.Steps[3].ExecutionStatus = "EXECUTED" }},
		{"execution canonical hash", func(r *HealthEvidenceDryRun) {
			r.ExecutionPlan.Limitations = append(r.ExecutionPlan.Limitations, "tampered")
		}},
		{"manifest run tag", func(r *HealthEvidenceDryRun) { r.ArtifactManifest.RunTag = "tampered" }},
		{"manifest artifact role", func(r *HealthEvidenceDryRun) { r.ArtifactManifest.Artifacts[0].Role = ArtifactRoleDescriptor }},
		{"manifest preserved prior", func(r *HealthEvidenceDryRun) {
			r.ArtifactManifest.Artifacts[0].Origin = ArtifactPreservedPrior
		}},
		{"manifest timestamp", func(r *HealthEvidenceDryRun) { ms := int64(1); r.ArtifactManifest.CreatedAtMS = &ms }},
		{"verification row", func(r *HealthEvidenceDryRun) {
			r.ArtifactVerification = []ArtifactVerification{{
				Role: ArtifactRoleOrchestrationPlan, Path: "results/tampered/execution-plan.input.json",
				Declared: strings.Repeat("0", 64), DeclaredBytes: 1, Status: Pass,
				Actual: strings.Repeat("0", 64), ActualBytes: 1, Reason: "tampered",
			}}
		}},
		{"evidence aggregate", func(r *HealthEvidenceDryRun) { r.EvidenceStatus = Pass }},
		{"health aggregate", func(r *HealthEvidenceDryRun) { r.HealthStatus = Fail }},
		{"dry run aggregate", func(r *HealthEvidenceDryRun) { r.DryRunStatus = Fail }},
		{"synthetic label", func(r *HealthEvidenceDryRun) { r.Synthetic = "offline" }},
		{"empty limitations", func(r *HealthEvidenceDryRun) { r.Limitations = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base.Canonical()
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("tampered report was accepted: %+v", candidate)
			}
			if _, err := DecodeHealthEvidenceDryRun(mustMarshal(t, candidate)); err == nil {
				t.Fatalf("strict decoder accepted tampered %s", tc.name)
			}
		})
	}
}

func m2eWriteReport(t *testing.T, platform string) (string, *HealthEvidenceDryRun, []byte, []byte, string) {
	t.Helper()
	root := t.TempDir()
	planRaw := m2ePlanRaw(t, platform)
	obsRaw := m2eObservationsRaw(t, platform)
	report, err := BuildHealthEvidenceDryRun(planRaw, obsRaw)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "results", report.ArtifactManifest.RunID)
	if err := WriteHealthEvidenceDryRun(out, report, planRaw, obsRaw, nil); err != nil {
		t.Fatal(err)
	}
	return root, report, planRaw, obsRaw, out
}

func TestM2EWritePackagingAndVerification(t *testing.T) {
	for _, platform := range []string{"thingsboard", "fiware", "edgex"} {
		t.Run(platform, func(t *testing.T) {
			root, _, planRaw, obsRaw, out := m2eWriteReport(t, platform)
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
				"artifact-manifest.json",
				HealthEvidenceCanonicalName,
				HealthEvidenceExecutionPlanInputName,
				HealthEvidenceMarkdownName,
				HealthEvidenceReportName,
				HealthEvidenceObservationsInputName,
			}
			sort.Strings(wantNames)
			if !equalStrings(names, wantNames) {
				t.Fatalf("output files = %v, want %v", names, wantNames)
			}
			gotPlan, err := os.ReadFile(filepath.Join(out, HealthEvidenceExecutionPlanInputName))
			if err != nil {
				t.Fatal(err)
			}
			gotObs, err := os.ReadFile(filepath.Join(out, HealthEvidenceObservationsInputName))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(gotPlan, planRaw) {
				t.Fatal("copied execution plan is not byte-identical")
			}
			if !bytes.Equal(gotObs, obsRaw) {
				t.Fatal("copied observations are not byte-identical")
			}
			writtenBytes, err := os.ReadFile(filepath.Join(out, HealthEvidenceReportName))
			if err != nil {
				t.Fatal(err)
			}
			written, err := DecodeHealthEvidenceDryRun(writtenBytes)
			if err != nil {
				t.Fatalf("written pretty report does not decode: %v", err)
			}
			if written.EvidenceStatus != Pass {
				t.Fatalf("written evidence_status = %s, want PASS", written.EvidenceStatus)
			}
			if written.DryRunStatus != Pass {
				t.Fatalf("written dry_run_status = %s, want PASS", written.DryRunStatus)
			}
			if written.HealthStatus != Pass {
				t.Fatalf("written health_status = %s, want PASS", written.HealthStatus)
			}
			if len(written.ArtifactVerification) != 2 {
				t.Fatalf("written verifications = %d, want 2", len(written.ArtifactVerification))
			}
			for _, v := range written.ArtifactVerification {
				if v.Status != Pass {
					t.Fatalf("written verification %s = %s", v.Path, v.Status)
				}
			}
			canonicalFile, err := os.ReadFile(filepath.Join(out, HealthEvidenceCanonicalName))
			if err != nil {
				t.Fatal(err)
			}
			wantCanonical, err := written.CanonicalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(canonicalFile, wantCanonical) {
				t.Fatal("written canonical JSON differs from the canonical render")
			}
			canonicalHash, err := written.CanonicalSHA256()
			if err != nil {
				t.Fatal(err)
			}
			if SHA256Bytes(canonicalFile) != canonicalHash {
				t.Fatal("written canonical hash is not SHA-256(canonical file)")
			}
			md, err := os.ReadFile(filepath.Join(out, HealthEvidenceMarkdownName))
			if err != nil {
				t.Fatal(err)
			}
			if string(md) != written.RenderMarkdown() {
				t.Fatal("written Markdown differs from the deterministic render")
			}
			for _, needle := range []string{HealthEvidenceSyntheticLabel, "not live qualification", ExecutionBlockedMarker, "NOT EXECUTED", "Evidence status: **PASS**"} {
				if !strings.Contains(string(md), needle) {
					t.Fatalf("written Markdown lacks %q", needle)
				}
			}
			manifestBytes, err := os.ReadFile(filepath.Join(out, "artifact-manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := DecodeArtifactManifest(manifestBytes)
			if err != nil {
				t.Fatalf("written manifest does not decode: %v", err)
			}
			if len(manifest.Artifacts) != 2 {
				t.Fatalf("manifest artifacts = %d, want 2", len(manifest.Artifacts))
			}
			roles := map[string]bool{}
			for _, artifact := range manifest.Artifacts {
				roles[artifact.Role] = true
				if artifact.Origin != ArtifactThisRun || !artifact.Required {
					t.Fatalf("artifact is not required this_run: %+v", artifact)
				}
				if strings.HasSuffix(artifact.Path, "artifact-manifest.json") || strings.HasSuffix(artifact.Path, "SHA256SUMS") {
					t.Fatalf("manifest lists itself: %+v", artifact)
				}
			}
			if !roles[ArtifactRoleOrchestrationPlan] || !roles[ArtifactRoleOther] {
				t.Fatalf("manifest roles = %v", roles)
			}
			verifications, err := manifest.Verify(root)
			if err != nil {
				t.Fatalf("written artifacts do not verify: %v", err)
			}
			for _, v := range verifications {
				if v.Status != Pass {
					t.Fatalf("verification %s = %s", v.Path, v.Status)
				}
			}
			sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
			if len(lines) != 6 {
				t.Fatalf("SHA256SUMS lines = %d, want 6:\n%s", len(lines), sums)
			}
			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) != 2 {
					t.Fatalf("malformed SHA256SUMS line %q", line)
				}
				if fields[1] == "SHA256SUMS" {
					t.Fatal("SHA256SUMS lists itself")
				}
				body, err := os.ReadFile(filepath.Join(out, fields[1]))
				if err != nil {
					t.Fatal(err)
				}
				if SHA256Bytes(body) != fields[0] {
					t.Fatalf("SHA256SUMS mismatch for %s", fields[1])
				}
			}
			if bytes.Contains(writtenBytes, []byte(root)) {
				t.Fatal("report leaks the absolute working directory")
			}
		})
	}
}

func TestM2EWriterRejectionsAndHealthFailIsValid(t *testing.T) {
	platform := "thingsboard"
	root := t.TempDir()
	planRaw := m2ePlanRaw(t, platform)
	obsRaw := m2eObservationsRaw(t, platform)
	report, err := BuildHealthEvidenceDryRun(planRaw, obsRaw)
	if err != nil {
		t.Fatal(err)
	}
	runID := report.ArtifactManifest.RunID

	if err := WriteHealthEvidenceDryRun(filepath.Join(root, "notresults", runID), report, planRaw, obsRaw, nil); err == nil {
		t.Fatal("output outside results/<run_id> was accepted")
	}
	if err := WriteHealthEvidenceDryRun(filepath.Join(root, "results", "wrong-run-id"), report, planRaw, obsRaw, nil); err == nil {
		t.Fatal("output with a mismatched run id was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "notresults")); !os.IsNotExist(err) {
		t.Fatal("wrong-layout rejection created an output tree")
	}
	if err := WriteHealthEvidenceDryRun(filepath.Join(root, "results", runID), nil, planRaw, obsRaw, nil); err == nil {
		t.Fatal("nil report was accepted")
	}
	inputDir := filepath.Join(root, "input")
	if err := os.MkdirAll(inputDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteHealthEvidenceDryRun(filepath.Join(inputDir, "results", runID), report, planRaw, obsRaw, []string{inputDir}); err == nil {
		t.Fatal("output under an input evidence tree was accepted")
	}

	out := filepath.Join(root, "results", runID)
	if err := WriteHealthEvidenceDryRun(out, report, planRaw, obsRaw, nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteHealthEvidenceDryRun(out, report, planRaw, obsRaw, nil); err == nil {
		t.Fatal("existing output directory was accepted")
	}

	other, err := BuildHealthEvidenceDryRun(m2ePlanRaw(t, "fiware"), m2eObservationsRaw(t, "fiware"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteHealthEvidenceDryRun(filepath.Join(root, "results", "m2e-synthetic-pass-fiware-20261105-01"), other, planRaw, obsRaw, nil); err == nil {
		t.Fatal("report from different raw inputs was accepted")
	}

	// A health FAIL is a valid report, not a writer error.
	unhealthyRaw := m2eMutatedObservations(t, platform, func(o *SyntheticServiceObservations) {
		o.RunTag = "m2e-synthetic-fail-thingsboard"
		o.RunID = "m2e-synthetic-fail-thingsboard-20261105-01"
		for i := range o.Observations {
			if o.Observations[i].Service == "kafka" {
				o.Observations[i].Health = ObservedUnhealthy
			}
		}
	})
	failReport, err := BuildHealthEvidenceDryRun(planRaw, unhealthyRaw)
	if err != nil {
		t.Fatal(err)
	}
	if failReport.HealthStatus != Fail {
		t.Fatalf("health_status = %s, want FAIL", failReport.HealthStatus)
	}
	failOut := filepath.Join(root, "results", failReport.ArtifactManifest.RunID)
	if err := WriteHealthEvidenceDryRun(failOut, failReport, planRaw, unhealthyRaw, nil); err != nil {
		t.Fatalf("health FAIL was treated as a writer error: %v", err)
	}
	failBytes, err := os.ReadFile(filepath.Join(failOut, HealthEvidenceReportName))
	if err != nil {
		t.Fatal(err)
	}
	failWritten, err := DecodeHealthEvidenceDryRun(failBytes)
	if err != nil {
		t.Fatal(err)
	}
	if failWritten.HealthStatus != Fail || failWritten.EvidenceStatus != Pass || failWritten.DryRunStatus != Fail {
		t.Fatalf("written FAIL aggregates are wrong: health=%s evidence=%s dry=%s",
			failWritten.HealthStatus, failWritten.EvidenceStatus, failWritten.DryRunStatus)
	}
}

func TestM2EVerifyPASSThenTamperAndMissing(t *testing.T) {
	root, _, _, _, out := m2eWriteReport(t, "fiware")
	manifestBytes, err := os.ReadFile(filepath.Join(out, "artifact-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := DecodeArtifactManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	verifications, err := manifest.Verify(root)
	if err != nil {
		t.Fatalf("intact artifacts did not verify: %v", err)
	}
	for _, v := range verifications {
		if v.Status != Pass {
			t.Fatalf("intact verification %s = %s", v.Path, v.Status)
		}
	}

	if err := os.WriteFile(filepath.Join(out, HealthEvidenceExecutionPlanInputName), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	verifications, err = manifest.Verify(root)
	if err == nil {
		t.Fatal("tampered artifact verified successfully")
	}
	if len(verifications) == 0 || verifications[0].Status != Fail {
		t.Fatalf("tampered artifact status = %+v, want FAIL", verifications)
	}

	if err := os.Remove(filepath.Join(out, HealthEvidenceObservationsInputName)); err != nil {
		t.Fatal(err)
	}
	verifications, err = manifest.Verify(root)
	if err == nil {
		t.Fatal("missing artifact verified successfully")
	}
	sawUnavailable := false
	for _, v := range verifications {
		if v.Status == Unavailable {
			sawUnavailable = true
		}
	}
	if !sawUnavailable {
		t.Fatalf("missing artifact status = %+v, want UNAVAILABLE", verifications)
	}
}

// TestM2EWriteCanonicalOutputs is an evidence helper, not a production path. It
// is skipped unless M2E_CANONICAL_OUT names a new directory. It writes only the
// deterministic renders for the three platform fixtures; it performs no Docker,
// SUT, lifecycle, load or observer action.
func TestM2EWriteCanonicalOutputs(t *testing.T) {
	out := os.Getenv("M2E_CANONICAL_OUT")
	if out == "" {
		t.Skip("M2E_CANONICAL_OUT not set")
	}
	if _, err := os.Lstat(out); !os.IsNotExist(err) {
		t.Fatalf("M2E_CANONICAL_OUT must name a new directory: %s", out)
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	var summary strings.Builder
	for _, platform := range []string{"thingsboard", "fiware", "edgex"} {
		planRaw := m2ePlanRaw(t, platform)
		obsRaw := m2eObservationsRaw(t, platform)
		report, err := BuildHealthEvidenceDryRun(planRaw, obsRaw)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(out, platform, "results", report.ArtifactManifest.RunID)
		if err := WriteHealthEvidenceDryRun(dir, report, planRaw, obsRaw, nil); err != nil {
			t.Fatal(err)
		}
		writtenBytes, err := os.ReadFile(filepath.Join(dir, HealthEvidenceReportName))
		if err != nil {
			t.Fatal(err)
		}
		written, err := DecodeHealthEvidenceDryRun(writtenBytes)
		if err != nil {
			t.Fatal(err)
		}
		canonical, err := written.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		hash, err := written.CanonicalSHA256()
		if err != nil {
			t.Fatal(err)
		}
		md := written.RenderMarkdown()
		fmt.Fprintf(&summary, "%s canonical_sha256=%s canonical_bytes=%d canonical_file=%s\n",
			platform, hash, len(canonical), HealthEvidenceCanonicalName)
		fmt.Fprintf(&summary, "%s markdown_sha256=%s markdown_bytes=%d markdown_file=%s\n",
			platform, SHA256Bytes([]byte(md)), len(md), HealthEvidenceMarkdownName)
		fmt.Fprintf(&summary, "%s report_sha256=%s report_bytes=%d report_file=%s\n",
			platform, SHA256Bytes(writtenBytes), len(writtenBytes), HealthEvidenceReportName)
		fmt.Fprintf(&summary, "%s observations_raw_sha256=%s observations_canonical_sha256=%s\n",
			platform, written.Inputs.ObservationsRawSHA256, written.Inputs.ObservationsCanonicalSHA256)
		fmt.Fprintf(&summary, "%s execution_plan_raw_sha256=%s execution_plan_canonical_sha256=%s\n",
			platform, written.Inputs.ExecutionPlanRawSHA256, written.Inputs.ExecutionPlanCanonicalSHA256)
	}
	if err := os.WriteFile(filepath.Join(out, "canonical-sha256.txt"), []byte(summary.String()), 0644); err != nil {
		t.Fatal(err)
	}
}
