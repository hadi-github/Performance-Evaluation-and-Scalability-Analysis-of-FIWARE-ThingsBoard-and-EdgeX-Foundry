package deployment

import (
	"fmt"
	"sort"
	"strings"
)

// Check is one explicit, deterministic comparison result. Expected and Observed
// are strings because the report is an evidence document, not a typed API for
// arithmetic. A check never carries a fabricated live value.
type Check struct {
	Name     string `json:"check"`
	Status   Status `json:"status"`
	Expected string `json:"expected,omitempty"`
	Observed string `json:"observed,omitempty"`
	Reason   string `json:"reason"`
}

// ContainerResult describes an observed container without adding health or
// state information that the inventory did not explicitly attest.
type ContainerResult struct {
	ContainerID string `json:"container_id"`
	Replica     int    `json:"replica"`
	Image       string `json:"image"`
	Digest      string `json:"digest,omitempty"`
	StartedAtMS int64  `json:"started_at_ms"`
	Health      string `json:"health"`
	State       string `json:"state"`
}

// ServiceResult is the per-service attestation.
type ServiceResult struct {
	Service          string            `json:"service"`
	Class            ServiceClass      `json:"class"`
	Status           Status            `json:"status"`
	RequiredReplicas int               `json:"required_replicas"`
	ObservedReplicas int               `json:"observed_replicas"`
	Checks           []Check           `json:"checks"`
	Containers       []ContainerResult `json:"containers,omitempty"`
}

// Totals reports the declared and observed application/supporting replica
// counts. The statuses are explicit and do not turn a missing service into a
// successful zero.
type Totals struct {
	RequiredApplicationReplicas int    `json:"required_application_replicas"`
	ObservedApplicationReplicas int    `json:"observed_application_replicas"`
	ApplicationStatus           Status `json:"application_status"`
	RequiredSupportingReplicas  int    `json:"required_supporting_replicas"`
	ObservedSupportingReplicas  int    `json:"observed_supporting_replicas"`
	SupportingStatus            Status `json:"supporting_status"`
}

// ObserverReport describes the optional read-only observer. When no observer is
// supplied it is NOT_ASSESSED and carries no synthetic replica count.
type ObserverReport struct {
	Status          Status `json:"status"`
	Name            string `json:"name,omitempty"`
	Reason          string `json:"reason"`
	InventorySHA256 string `json:"inventory_sha256,omitempty"`
}

// ValidateOptions carries provenance hashes, the optional observer attestation
// and the optional evidence manifest. An omitted observer is NOT_ASSESSED.
type ValidateOptions struct {
	DescriptorSHA256          string
	DescriptorCanonicalSHA256 string
	InventorySHA256           string
	InventoryCanonicalSHA256  string
	Observer                  ObserverReport
	Evidence                  *EvidenceManifest
}

// Report is the versioned dry-run attestation. LiveValidation is always
// NOT_ASSESSED in M2 because no live observer implementation exists.
type Report struct {
	SchemaVersion             int               `json:"schema_version"`
	Interpretation            string            `json:"interpretation"`
	DescriptorInterpretation  string            `json:"descriptor_interpretation"`
	InventoryInterpretation   string            `json:"inventory_interpretation"`
	DryRun                    bool              `json:"dry_run"`
	LiveValidation            Status            `json:"live_validation"`
	Platform                  string            `json:"platform"`
	Project                   string            `json:"project"`
	DescriptorSHA256          string            `json:"descriptor_sha256,omitempty"`
	DescriptorCanonicalSHA256 string            `json:"descriptor_canonical_sha256"`
	InventorySHA256           string            `json:"inventory_sha256,omitempty"`
	InventoryCanonicalSHA256  string            `json:"inventory_canonical_sha256"`
	Protocol                  ProtocolBinding   `json:"protocol"`
	Provenance                Provenance        `json:"provenance"`
	InventorySource           string            `json:"inventory_source"`
	InventoryCapturedAtMS     *int64            `json:"inventory_captured_at_ms,omitempty"`
	Observer                  ObserverReport    `json:"observer"`
	Attestation               Status            `json:"attestation"`
	Totals                    Totals            `json:"totals"`
	Services                  []ServiceResult   `json:"services"`
	Checks                    []Check           `json:"checks"`
	Evidence                  *EvidenceManifest `json:"evidence,omitempty"`
	Limitations               []string          `json:"limitations"`
}

// Validate compares a validated descriptor with a validated caller-supplied
// inventory. It is pure and file-free. Cross-project inventories are reported
// as FAIL, not rejected as an I/O error, so the evidence remains inspectable.
func Validate(d Descriptor, inv Inventory, opts ValidateOptions) (*Report, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}
	dc, err := d.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	ic, err := inv.CanonicalSHA256()
	if err != nil {
		return nil, err
	}
	if opts.DescriptorCanonicalSHA256 != "" && opts.DescriptorCanonicalSHA256 != dc {
		return nil, fmt.Errorf("descriptor canonical SHA-256 mismatch: supplied %s, computed %s", opts.DescriptorCanonicalSHA256, dc)
	}
	if opts.InventoryCanonicalSHA256 != "" && opts.InventoryCanonicalSHA256 != ic {
		return nil, fmt.Errorf("inventory canonical SHA-256 mismatch: supplied %s, computed %s", opts.InventoryCanonicalSHA256, ic)
	}
	opts.DescriptorCanonicalSHA256 = dc
	opts.InventoryCanonicalSHA256 = ic
	if opts.Evidence != nil {
		if err := opts.Evidence.Validate(); err != nil {
			return nil, err
		}
		canonical := opts.Evidence.Canonical()
		opts.Evidence = &canonical
	}
	observer := opts.Observer
	if observer.Status == "" {
		observer = ObserverReport{
			Status: NotAssessed,
			Reason: "no read-only observer implementation supplied; observer evidence is absent, not zero",
		}
	}
	if !validStatus(observer.Status) {
		return nil, fmt.Errorf("observer status %q is not an explicit M2 state", observer.Status)
	}
	if observer.Reason == "" {
		return nil, fmt.Errorf("observer status %s requires an explicit reason", observer.Status)
	}

	report := &Report{
		SchemaVersion:             SchemaVersion,
		Interpretation:            ValidationInterpretation,
		DescriptorInterpretation:  DescriptorInterpretation,
		InventoryInterpretation:   InventoryInterpretation,
		DryRun:                    true,
		LiveValidation:            NotAssessed,
		Platform:                  d.Platform,
		Project:                   d.Project,
		DescriptorSHA256:          opts.DescriptorSHA256,
		DescriptorCanonicalSHA256: opts.DescriptorCanonicalSHA256,
		InventorySHA256:           opts.InventorySHA256,
		InventoryCanonicalSHA256:  opts.InventoryCanonicalSHA256,
		Protocol:                  d.Protocol,
		Provenance:                d.Provenance.Canonical(),
		InventorySource:           inv.Source,
		InventoryCapturedAtMS:     inv.CapturedAtMS,
		Observer:                  observer,
		Evidence:                  opts.Evidence,
		Limitations: []string{
			"Dry-run attestation only: no deployment, lifecycle action, Docker client call, SUT access, health probe, log read or load was performed.",
			"Live validation is NOT_ASSESSED because M2 provides no live observer implementation; the result is limited to the supplied documents.",
			"Attestation PASS means the supplied inventory matches the supplied descriptor, not that any service exists or is reachable.",
			"Images, digests, cpusets, memory, configuration checksums and container health/state are caller attestations, not independently observed facts.",
			"Protocol and profile hashes are recorded from the descriptor; this package does not re-read or re-verify those files.",
		},
	}

	declared := map[string]DescriptorService{}
	for _, svc := range d.Canonical().Services {
		declared[svc.Service] = svc
	}
	observed := map[string]InventoryService{}
	for _, svc := range inv.Canonical().Services {
		observed[svc.Service] = svc
	}

	// Global checks are fixed-order and avoid map iteration in output.
	report.Checks = append(report.Checks, compareProject(d.Project, inv.Project))
	report.Checks = append(report.Checks, compareUnexpectedServices(declared, observed))
	report.Checks = append(report.Checks, compareEffectiveConfiguration(d.EffectiveConfiguration, inv.EffectiveConfiguration))
	report.Checks = append(report.Checks, Check{
		Name:     "protocol_binding_declared",
		Status:   Pass,
		Expected: "valid protocol binding and profile SHA-256 declarations",
		Observed: "both declared; external files not re-read",
		Reason:   "descriptor pins binding_sha256 and profile_sha256; the hashes are declarations, not live verification",
	})
	report.Checks = append(report.Checks, Check{
		Name:     "observer",
		Status:   observer.Status,
		Expected: "read-only observer",
		Observed: observer.Name,
		Reason:   observer.Reason,
	})
	report.Checks = append(report.Checks, Check{
		Name:     "live_validation",
		Status:   NotAssessed,
		Expected: "live read-only observation",
		Observed: "unavailable in M2",
		Reason:   "no Docker or live observer implementation exists; the report must not infer live behavior",
	})

	for _, svc := range d.Canonical().Services {
		obs, ok := observed[svc.Service]
		report.Services = append(report.Services, validateService(svc, obs, ok, inv.Project))
	}

	report.Totals = totalsFor(report.Services)
	report.Attestation = aggregate(report.Checks, report.Services)
	return report, nil
}

func validateService(svc DescriptorService, obs InventoryService, present bool, inventoryProject string) ServiceResult {
	res := ServiceResult{
		Service:          svc.Service,
		Class:            svc.Class,
		RequiredReplicas: svc.Replicas,
	}
	checks := []Check{}
	if !present {
		imageStatus, imageObserved := NotAssessed, "not declared"
		imageReason := "descriptor does not declare an image"
		digestStatus, digestObserved := NotAssessed, "not declared"
		digestReason := "descriptor does not declare a digest"
		if svc.Image != "" {
			imageStatus, imageObserved = Fail, "absent"
			imageReason = "the declared image cannot be compared because the service is missing"
		}
		if svc.Digest != "" {
			digestStatus, digestObserved = Fail, "absent"
			digestReason = "the declared digest cannot be compared because the service is missing"
		}
		checks = append(checks,
			Check{Name: "service_present", Status: Fail, Expected: svc.Service, Observed: "absent", Reason: "required service is missing from the supplied inventory"},
			Check{Name: "replicas", Status: Fail, Expected: fmt.Sprint(svc.Replicas), Observed: "0", Reason: "a missing service is not a successful zero-replica observation"},
			Check{Name: "health", Status: Fail, Expected: "healthy", Observed: "unavailable", Reason: "no observed service exists to attest health"},
			Check{Name: "state", Status: Fail, Expected: "running", Observed: "unavailable", Reason: "no observed service exists to attest state"},
			Check{Name: "image", Status: imageStatus, Expected: svc.Image, Observed: imageObserved, Reason: imageReason},
			Check{Name: "digest", Status: digestStatus, Expected: svc.Digest, Observed: digestObserved, Reason: digestReason},
			Check{Name: "cpuset", Status: Fail, Expected: svc.CPUSet, Observed: "unavailable", Reason: "no observed service exists to attest a cpuset"},
			Check{Name: "topology", Status: Fail, Expected: fmt.Sprintf("%d containers with replicas [0,%d)", svc.Replicas, svc.Replicas), Observed: "absent", Reason: "realized topology is unknown because the service is missing"},
		)
		res.Status = aggregateRequired(checks)
		res.Checks = checks
		return res
	}

	res.ObservedReplicas = len(obs.Containers)
	for _, c := range sortedContainers(obs) {
		res.Containers = append(res.Containers, ContainerResult{
			ContainerID: c.ContainerID,
			Replica:     c.Replica,
			Image:       c.Image,
			Digest:      c.Digest,
			StartedAtMS: c.StartedAtMS,
			Health:      c.Health,
			State:       c.State,
		})
	}
	checks = append(checks,
		Check{Name: "service_present", Status: Pass, Expected: svc.Service, Observed: obs.Service, Reason: "enclosing service matches the descriptor"},
		compareReplicas(svc.Replicas, res.ObservedReplicas),
		compareHealth(obs),
		compareState(obs),
		compareImage(svc.Image, obs),
		compareDigest(svc.Digest, obs),
		compareCPUSet(svc.CPUSet, obs.CPUSet),
		compareTopology(svc, obs, inventoryProject),
	)
	res.Status = aggregateRequired(checks)
	res.Checks = checks
	return res
}

func sortedContainers(svc InventoryService) []InventoryContainer {
	out := append([]InventoryContainer(nil), svc.Containers...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Replica != out[j].Replica {
			return out[i].Replica < out[j].Replica
		}
		return out[i].ContainerID < out[j].ContainerID
	})
	return out
}

func compareProject(declared, observed string) Check {
	c := Check{Name: "project", Expected: declared, Observed: observed}
	if declared == observed {
		c.Status = Pass
		c.Reason = "inventory project matches the descriptor project"
		return c
	}
	c.Status = Fail
	c.Reason = "cross-project inventory: the supplied inventory belongs to a different compose project"
	return c
}

func compareUnexpectedServices(declared map[string]DescriptorService, observed map[string]InventoryService) Check {
	var extras []string
	for name := range observed {
		if _, ok := declared[name]; !ok {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	c := Check{Name: "unexpected_services", Expected: "only descriptor services", Reason: "observed service set matches the descriptor"}
	if len(extras) == 0 {
		c.Status = Pass
		return c
	}
	c.Status = Fail
	c.Observed = join(extras)
	c.Reason = "inventory contains services that the descriptor does not declare"
	return c
}

func compareEffectiveConfiguration(declared, observed *EffectiveConfiguration) Check {
	c := Check{Name: "effective_configuration", Expected: "not declared", Observed: "not declared", Reason: "descriptor does not declare an effective configuration"}
	if declared == nil {
		c.Status = NotAssessed
		return c
	}
	c.Expected = declared.SHA256
	if declared.Path != "" {
		c.Expected = declared.Path + " @ " + declared.SHA256
	}
	if observed == nil {
		c.Status = Unavailable
		c.Observed = "not supplied"
		c.Reason = "descriptor declares an effective configuration but the inventory does not supply one"
		return c
	}
	c.Observed = observed.SHA256
	if observed.Path != "" {
		c.Observed = observed.Path + " @ " + observed.SHA256
	}
	if observed.SHA256 == declared.SHA256 {
		c.Status = Pass
		c.Reason = "effective configuration checksum matches the descriptor declaration"
		return c
	}
	c.Status = Fail
	c.Reason = "effective configuration checksum differs from the descriptor declaration"
	return c
}

func compareReplicas(required, observed int) Check {
	c := Check{Name: "replicas", Expected: fmt.Sprint(required), Observed: fmt.Sprint(observed)}
	if required == observed {
		c.Status = Pass
		c.Reason = "observed container count equals the declared replica count"
		return c
	}
	c.Status = Fail
	c.Reason = "observed container count does not equal the declared replica count"
	return c
}

func compareHealth(obs InventoryService) Check {
	c := Check{Name: "health", Expected: "healthy"}
	var bad []string
	for _, cc := range sortedContainers(obs) {
		if cc.Health != "healthy" {
			bad = append(bad, fmt.Sprintf("%s=%s", cc.ContainerID, cc.Health))
		}
	}
	if len(bad) == 0 {
		c.Status = Pass
		c.Observed = "all healthy"
		c.Reason = "every supplied container attests healthy health"
		return c
	}
	c.Status = Fail
	c.Observed = join(bad)
	c.Reason = "one or more supplied containers do not attest healthy health"
	return c
}

func compareState(obs InventoryService) Check {
	c := Check{Name: "state", Expected: "running"}
	var bad []string
	for _, cc := range sortedContainers(obs) {
		if cc.State != "running" {
			bad = append(bad, fmt.Sprintf("%s=%s", cc.ContainerID, cc.State))
		}
	}
	if len(bad) == 0 {
		c.Status = Pass
		c.Observed = "all running"
		c.Reason = "every supplied container attests running state"
		return c
	}
	c.Status = Fail
	c.Observed = join(bad)
	c.Reason = "one or more supplied containers do not attest running state"
	return c
}

func compareImage(declared string, obs InventoryService) Check {
	c := Check{Name: "image", Expected: declared}
	if declared == "" {
		c.Status = NotAssessed
		c.Observed = "not declared"
		c.Reason = "descriptor does not declare an image; the inventory image is not compared"
		return c
	}
	var mismatches []string
	for _, cc := range sortedContainers(obs) {
		if cc.Image != declared {
			mismatches = append(mismatches, fmt.Sprintf("%s=%s", cc.ContainerID, cc.Image))
		}
	}
	if len(mismatches) == 0 {
		c.Status = Pass
		c.Observed = declared
		c.Reason = "every supplied container attests the declared image"
		return c
	}
	c.Status = Fail
	c.Observed = join(mismatches)
	c.Reason = "observed image differs from the descriptor declaration"
	return c
}

func compareDigest(declared string, obs InventoryService) Check {
	c := Check{Name: "digest", Expected: declared}
	if declared == "" {
		c.Status = NotAssessed
		c.Observed = "not declared"
		c.Reason = "descriptor does not declare a digest; the inventory digest is not compared"
		return c
	}
	var missing []string
	var mismatches []string
	for _, cc := range sortedContainers(obs) {
		switch {
		case cc.Digest == "":
			missing = append(missing, cc.ContainerID)
		case cc.Digest != declared:
			mismatches = append(mismatches, fmt.Sprintf("%s=%s", cc.ContainerID, cc.Digest))
		}
	}
	if len(mismatches) > 0 {
		c.Status = Fail
		c.Observed = join(mismatches)
		c.Reason = "observed digest differs from the descriptor declaration"
		return c
	}
	if len(missing) > 0 {
		c.Status = Unavailable
		c.Observed = "missing for " + join(missing)
		c.Reason = "descriptor declares a digest but the inventory omits it for one or more containers; absence is UNAVAILABLE, not a match"
		return c
	}
	c.Status = Pass
	c.Observed = declared
	c.Reason = "every supplied container attests the declared digest"
	return c
}

func compareCPUSet(declared, observed string) Check {
	c := Check{Name: "cpuset", Expected: declared}
	if observed == "" {
		c.Status = Unavailable
		c.Observed = "not supplied"
		c.Reason = "descriptor declares a cpuset but the inventory does not supply one; absence is UNAVAILABLE, not a match"
		return c
	}
	c.Observed = observed
	if declared == observed {
		c.Status = Pass
		c.Reason = "observed cpuset matches the descriptor declaration"
		return c
	}
	c.Status = Fail
	c.Reason = "observed cpuset differs from the descriptor declaration"
	return c
}

func compareTopology(svc DescriptorService, obs InventoryService, inventoryProject string) Check {
	c := Check{
		Name:     "topology",
		Expected: fmt.Sprintf("%d containers for %s with replicas [0,%d)", svc.Replicas, svc.Service, svc.Replicas),
	}
	var reasons []string
	if obs.Project != inventoryProject {
		reasons = append(reasons, "enclosing service project does not match the inventory project")
	}
	if obs.Service != svc.Service {
		reasons = append(reasons, "observed service name does not match the descriptor")
	}
	if obs.Replicas != svc.Replicas {
		reasons = append(reasons, fmt.Sprintf("declared observed replicas %d != required %d", obs.Replicas, svc.Replicas))
	}
	if len(obs.Containers) != svc.Replicas {
		reasons = append(reasons, fmt.Sprintf("container count %d != required %d", len(obs.Containers), svc.Replicas))
	}
	seenReplica := map[int]bool{}
	seenID := map[string]bool{}
	for _, cc := range obs.Containers {
		if cc.Project != inventoryProject {
			reasons = append(reasons, "container project does not match the inventory project")
		}
		if cc.Service != svc.Service {
			reasons = append(reasons, "container service does not match the enclosing service")
		}
		if cc.Replica < 0 || cc.Replica >= svc.Replicas {
			reasons = append(reasons, fmt.Sprintf("container %s replica %d is out of range", cc.ContainerID, cc.Replica))
		}
		if seenReplica[cc.Replica] {
			reasons = append(reasons, fmt.Sprintf("duplicate replica %d", cc.Replica))
		}
		seenReplica[cc.Replica] = true
		if seenID[cc.ContainerID] {
			reasons = append(reasons, fmt.Sprintf("duplicate container_id %s", cc.ContainerID))
		}
		seenID[cc.ContainerID] = true
	}
	if len(reasons) == 0 {
		for r := 0; r < svc.Replicas; r++ {
			if !seenReplica[r] {
				reasons = append(reasons, fmt.Sprintf("missing replica %d", r))
			}
		}
	}
	if len(reasons) == 0 {
		c.Status = Pass
		c.Observed = fmt.Sprintf("%d containers with replicas [0,%d)", len(obs.Containers), svc.Replicas)
		c.Reason = "supplied containers form the declared contiguous replica topology"
		return c
	}
	sort.Strings(reasons)
	c.Status = Fail
	c.Observed = fmt.Sprintf("%d containers", len(obs.Containers))
	c.Reason = join(unique(reasons))
	return c
}

func totalsFor(services []ServiceResult) Totals {
	classStatus := map[ServiceClass][]Status{}
	var t Totals
	for _, svc := range services {
		classStatus[svc.Class] = append(classStatus[svc.Class], svc.Status)
		switch svc.Class {
		case Application:
			t.RequiredApplicationReplicas += svc.RequiredReplicas
			t.ObservedApplicationReplicas += svc.ObservedReplicas
		case Supporting:
			t.RequiredSupportingReplicas += svc.RequiredReplicas
			t.ObservedSupportingReplicas += svc.ObservedReplicas
		}
	}
	t.ApplicationStatus = aggregateStatus(classStatus[Application])
	t.SupportingStatus = aggregateStatus(classStatus[Supporting])
	return t
}

func aggregate(checks []Check, services []ServiceResult) Status {
	statuses := make([]Status, 0, len(checks)+len(services))
	for _, c := range checks {
		if c.Name == "live_validation" || c.Status == NotAssessed {
			continue
		}
		statuses = append(statuses, c.Status)
	}
	for _, s := range services {
		if s.Status != NotAssessed {
			statuses = append(statuses, s.Status)
		}
	}
	return aggregateStatus(statuses)
}

func aggregateRequired(checks []Check) Status {
	statuses := make([]Status, 0, len(checks))
	for _, c := range checks {
		statuses = append(statuses, c.Status)
	}
	status := aggregateStatus(statuses)
	if status == NotAssessed {
		return Pass
	}
	return status
}

func validStatus(s Status) bool {
	switch s {
	case Pass, Fail, Unavailable, NotAssessed:
		return true
	default:
		return false
	}
}

func aggregateStatus(statuses []Status) Status {
	if len(statuses) == 0 {
		return NotAssessed
	}
	sawUnavailable := false
	sawNotAssessed := false
	for _, s := range statuses {
		switch s {
		case Fail:
			return Fail
		case Unavailable:
			sawUnavailable = true
		case NotAssessed:
			sawNotAssessed = true
		}
	}
	if sawUnavailable {
		return Unavailable
	}
	if sawNotAssessed {
		return NotAssessed
	}
	return Pass
}

func join(values []string) string { return strings.Join(values, ", ") }

func unique(values []string) []string {
	out := values[:0]
	for _, v := range values {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}
