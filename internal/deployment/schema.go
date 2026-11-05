// Package deployment implements the M2 bounded, read-only deployment and
// instrumentation foundation. It validates caller-supplied versioned documents
// and renders dry-run attestations. It never contacts Docker, a SUT, a platform,
// a lifecycle endpoint or a monitoring service, and it exposes no lifecycle
// operation.
package deployment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const (
	// SchemaVersion versions every M2 document in this package.
	SchemaVersion = 1
	// DescriptorInterpretation names the exact descriptor semantics.
	DescriptorInterpretation = "deployment-descriptor-v1"
	// InventoryInterpretation names the exact observed-inventory semantics.
	InventoryInterpretation = "observed-inventory-v1"
	// ValidationInterpretation names the exact dry-run attestation semantics.
	ValidationInterpretation = "deployment-attestation-v1"
	// EvidenceManifestInterpretation names the evidence manifest semantics.
	EvidenceManifestInterpretation = "deployment-evidence-manifest-v1"
)

// Status is an explicit assessment state. A generic boolean success is never
// used because missing evidence and a failed comparison are different results.
type Status string

const (
	Pass        Status = "PASS"
	Fail        Status = "FAIL"
	Unavailable Status = "UNAVAILABLE"
	NotAssessed Status = "NOT_ASSESSED"
)

// ServiceClass separates the measured application tier from everything that
// supports it. The distinction is declared, never inferred from a service name.
type ServiceClass string

const (
	Application ServiceClass = "application"
	Supporting  ServiceClass = "supporting"
)

var (
	platformAllowlist = map[string]bool{
		"thingsboard": true,
		"fiware":      true,
		"edgex":       true,
	}
	composeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	imageDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// EffectiveConfiguration is an optional caller-attested configuration checksum
// and path. A descriptor that omits it makes the configuration check
// NOT_ASSESSED; it never creates a default or a fabricated checksum.
type EffectiveConfiguration struct {
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256"`
}

// EvidenceDocument records one caller-supplied document and its SHA-256. It
// carries no timestamp unless a caller explicitly supplies one elsewhere.
type EvidenceDocument struct {
	Role   string `json:"role"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Provenance is the deployment declaration's origin. CreatedAtMS is optional and
// is only copied from the caller; the package never invents a timestamp.
type Provenance struct {
	Source        string             `json:"source"`
	ComposePath   string             `json:"compose_path,omitempty"`
	ComposeSHA256 string             `json:"compose_sha256,omitempty"`
	CreatedAtMS   *int64             `json:"created_at_ms,omitempty"`
	Documents     []EvidenceDocument `json:"documents,omitempty"`
}

// ProtocolBinding pins the descriptor to caller-declared protocol and profile
// hashes. This package records and shape-validates the declaration; it does not
// read the protocol file or turn the hash into a live claim.
type ProtocolBinding struct {
	BindingSHA256 string `json:"binding_sha256"`
	ProfileSHA256 string `json:"profile_sha256"`
	ProfilePath   string `json:"profile_path,omitempty"`
}

// Allocation declares the CPU sets admitted for each service class. Every
// service cpuset must belong to the list for its declared class.
type Allocation struct {
	ApplicationCPUSets []string `json:"application_cpusets"`
	SupportingCPUSets  []string `json:"supporting_cpusets"`
}

// DescriptorService is one Compose service declaration. Image and digest are
// optional; if either is declared, validation compares it.
type DescriptorService struct {
	Service     string       `json:"service"`
	Class       ServiceClass `json:"class"`
	Replicas    int          `json:"replicas"`
	CPUSet      string       `json:"cpuset"`
	MemoryBytes int64        `json:"memory_bytes"`
	Image       string       `json:"image,omitempty"`
	Digest      string       `json:"digest,omitempty"`
}

// Descriptor is the strict versioned deployment schema v1.
type Descriptor struct {
	SchemaVersion          int                     `json:"schema_version"`
	Platform               string                  `json:"platform"`
	Project                string                  `json:"project"`
	Allocation             Allocation              `json:"allocation"`
	Protocol               ProtocolBinding         `json:"protocol"`
	Provenance             Provenance              `json:"provenance"`
	Services               []DescriptorService     `json:"services"`
	EffectiveConfiguration *EffectiveConfiguration `json:"effective_configuration,omitempty"`
}

// InventoryContainer is one caller-attested container. Digest is optional; a
// declared descriptor digest plus a missing observed digest is UNAVAILABLE, not
// a match or a zero.
type InventoryContainer struct {
	Project      string `json:"project"`
	Service      string `json:"service"`
	ContainerID  string `json:"container_id"`
	Replica      int    `json:"replica"`
	Image        string `json:"image"`
	Digest       string `json:"digest,omitempty"`
	StartedAtMS  int64  `json:"started_at_ms"`
	Health       string `json:"health"`
	State        string `json:"state"`
	HealthReason string `json:"health_reason,omitempty"`
	StateReason  string `json:"state_reason,omitempty"`
}

// InventoryService is one observed Compose service. Project and Service are
// explicit because an enclosing identity is an assertion, not an optional hint.
type InventoryService struct {
	Project    string               `json:"project"`
	Service    string               `json:"service"`
	Replicas   int                  `json:"replicas"`
	CPUSet     string               `json:"cpuset,omitempty"`
	Containers []InventoryContainer `json:"containers"`
}

// Inventory is the strict versioned observed-inventory schema v1. It is only
// ever populated from a caller-supplied document or a fake observer.
type Inventory struct {
	SchemaVersion          int                     `json:"schema_version"`
	Project                string                  `json:"project"`
	Source                 string                  `json:"source"`
	CapturedAtMS           *int64                  `json:"captured_at_ms,omitempty"`
	EffectiveConfiguration *EffectiveConfiguration `json:"effective_configuration,omitempty"`
	Services               []InventoryService      `json:"services"`
}

var validHealth = map[string]bool{
	"healthy":   true,
	"unhealthy": true,
	"starting":  true,
	"none":      true,
}

var validState = map[string]bool{
	"running":    true,
	"exited":     true,
	"created":    true,
	"restarting": true,
	"paused":     true,
	"dead":       true,
}

var validEvidenceRoles = map[string]bool{
	"descriptor":              true,
	"inventory":               true,
	"observer_inventory":      true,
	"compose":                 true,
	"protocol":                true,
	"profile":                 true,
	"effective_configuration": true,
	"other":                   true,
}

// SHA256Bytes returns the lower-case SHA-256 of b.
func SHA256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// DecodeDescriptor strictly decodes a deployment descriptor. Duplicate keys,
// case-variant keys, unknown fields, nulls, trailing JSON, missing required
// fields and malformed semantics are rejected.
func DecodeDescriptor(b []byte) (*Descriptor, error) {
	if err := validateRawDocument(b, descriptorShape, "descriptor"); err != nil {
		return nil, err
	}
	var d Descriptor
	if err := decodeStrict(b, &d); err != nil {
		return nil, fmt.Errorf("descriptor: %w", err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// DecodeInventory strictly decodes a caller-supplied observed inventory.
func DecodeInventory(b []byte) (*Inventory, error) {
	if err := validateRawDocument(b, inventoryShape, "inventory"); err != nil {
		return nil, err
	}
	var inv Inventory
	if err := decodeStrict(b, &inv); err != nil {
		return nil, fmt.Errorf("inventory: %w", err)
	}
	if err := inv.Validate(); err != nil {
		return nil, err
	}
	return &inv, nil
}

// jsonShape describes the exact, case-sensitive key set admitted by one JSON
// object. object is non-nil for an object, element is non-nil for an array, and
// a shape with neither is an uninterpreted scalar whose type is enforced by the
// typed decoder. optional names keys that may be omitted; every other key is
// required. These shapes are deliberately package-local and separate from
// encoding/json's struct decoder, which accepts case-insensitive field aliases.
type jsonShape struct {
	object   map[string]jsonShape
	optional map[string]bool
	element  *jsonShape
}

var (
	scalarValueShape = jsonShape{}

	effectiveConfigurationShape = jsonShape{
		object: map[string]jsonShape{
			"path":   scalarValueShape,
			"sha256": scalarValueShape,
		},
		optional: map[string]bool{"path": true},
	}

	evidenceDocumentShape = jsonShape{
		object: map[string]jsonShape{
			"role":   scalarValueShape,
			"path":   scalarValueShape,
			"sha256": scalarValueShape,
		},
	}

	descriptorServiceShape = jsonShape{
		object: map[string]jsonShape{
			"service":      scalarValueShape,
			"class":        scalarValueShape,
			"replicas":     scalarValueShape,
			"cpuset":       scalarValueShape,
			"memory_bytes": scalarValueShape,
			"image":        scalarValueShape,
			"digest":       scalarValueShape,
		},
		optional: map[string]bool{"image": true, "digest": true},
	}

	descriptorShape = jsonShape{
		object: map[string]jsonShape{
			"schema_version": scalarValueShape,
			"platform":       scalarValueShape,
			"project":        scalarValueShape,
			"allocation": {
				object: map[string]jsonShape{
					"application_cpusets": {element: &scalarValueShape},
					"supporting_cpusets":  {element: &scalarValueShape},
				},
			},
			"protocol": {
				object: map[string]jsonShape{
					"binding_sha256": scalarValueShape,
					"profile_sha256": scalarValueShape,
					"profile_path":   scalarValueShape,
				},
				optional: map[string]bool{"profile_path": true},
			},
			"provenance": {
				object: map[string]jsonShape{
					"source":         scalarValueShape,
					"compose_path":   scalarValueShape,
					"compose_sha256": scalarValueShape,
					"created_at_ms":  scalarValueShape,
					"documents":      {element: &evidenceDocumentShape},
				},
				optional: map[string]bool{
					"compose_path": true, "compose_sha256": true,
					"created_at_ms": true, "documents": true,
				},
			},
			"services":                {element: &descriptorServiceShape},
			"effective_configuration": effectiveConfigurationShape,
		},
		optional: map[string]bool{"effective_configuration": true},
	}

	inventoryContainerShape = jsonShape{
		object: map[string]jsonShape{
			"project":       scalarValueShape,
			"service":       scalarValueShape,
			"container_id":  scalarValueShape,
			"replica":       scalarValueShape,
			"image":         scalarValueShape,
			"digest":        scalarValueShape,
			"started_at_ms": scalarValueShape,
			"health":        scalarValueShape,
			"state":         scalarValueShape,
			"health_reason": scalarValueShape,
			"state_reason":  scalarValueShape,
		},
		optional: map[string]bool{"digest": true, "health_reason": true, "state_reason": true},
	}

	inventoryServiceShape = jsonShape{
		object: map[string]jsonShape{
			"project":    scalarValueShape,
			"service":    scalarValueShape,
			"replicas":   scalarValueShape,
			"cpuset":     scalarValueShape,
			"containers": {element: &inventoryContainerShape},
		},
		optional: map[string]bool{"cpuset": true},
	}

	inventoryShape = jsonShape{
		object: map[string]jsonShape{
			"schema_version":          scalarValueShape,
			"project":                 scalarValueShape,
			"source":                  scalarValueShape,
			"captured_at_ms":          scalarValueShape,
			"effective_configuration": effectiveConfigurationShape,
			"services":                {element: &inventoryServiceShape},
		},
		optional: map[string]bool{"captured_at_ms": true, "effective_configuration": true},
	}

	evidenceManifestShape = jsonShape{
		object: map[string]jsonShape{
			"schema_version": scalarValueShape,
			"interpretation": scalarValueShape,
			"documents":      {element: &evidenceDocumentShape},
			"captured_at_ms": scalarValueShape,
		},
		optional: map[string]bool{"captured_at_ms": true},
	}
)

// validateRawDocument applies the shared raw checks before any typed decode:
// exact duplicate keys at every nesting level, no explicit nulls, and the exact
// case-sensitive key set for every object, including objects inside arrays.
func validateRawDocument(b []byte, shape jsonShape, context string) error {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return fmt.Errorf("%s: %w", context, err)
	}
	if err := rejectJSONNulls(b, context); err != nil {
		return err
	}
	if err := validateExactJSONKeys(b, shape, context); err != nil {
		return err
	}
	return nil
}

func validateExactJSONKeys(b []byte, shape jsonShape, context string) error {
	var root any
	if err := json.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("%s: %w", context, err)
	}
	if err := validateExactShape(root, shape, "$"); err != nil {
		return fmt.Errorf("%s: %w", context, err)
	}
	return nil
}

func validateExactShape(value any, shape jsonShape, path string) error {
	if shape.element != nil {
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected a JSON array", path)
		}
		for i, item := range items {
			if err := validateExactShape(item, *shape.element, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	}
	if shape.object == nil {
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: expected a JSON object", path)
	}
	keys := make([]string, 0, len(object))
	for k := range object {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		child, ok := shape.object[k]
		if !ok {
			return fmt.Errorf("%s: unknown or non-case-exact key %q", path, k)
		}
		if err := validateExactShape(object[k], child, path+"."+k); err != nil {
			return err
		}
	}
	required := make([]string, 0, len(shape.object))
	for k := range shape.object {
		if shape.optional[k] {
			continue
		}
		required = append(required, k)
	}
	sort.Strings(required)
	for _, k := range required {
		if _, ok := object[k]; !ok {
			return fmt.Errorf("%s: missing required key %q", path, k)
		}
	}
	return nil
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

// Validate checks the descriptor after decoding or after direct construction.
func (d Descriptor) Validate() error {
	if d.SchemaVersion != SchemaVersion {
		return fmt.Errorf("descriptor: unsupported schema_version %d", d.SchemaVersion)
	}
	if !platformAllowlisted(d.Platform) {
		return fmt.Errorf("descriptor: platform %q is outside the allowlist (thingsboard, fiware, edgex)", d.Platform)
	}
	if !composeNamePattern.MatchString(d.Project) {
		return fmt.Errorf("descriptor: project %q is not a valid compose project name", d.Project)
	}
	if err := validateAllocation(d.Allocation); err != nil {
		return err
	}
	if err := validateProtocol(d.Protocol); err != nil {
		return err
	}
	if err := validateProvenance(d.Provenance); err != nil {
		return err
	}
	if d.EffectiveConfiguration != nil {
		if err := validateEffectiveConfiguration("descriptor effective_configuration", *d.EffectiveConfiguration); err != nil {
			return err
		}
	}
	if len(d.Services) == 0 {
		return errors.New("descriptor: services must be a non-empty explicit array")
	}
	seen := map[string]bool{}
	for i, s := range d.Services {
		context := fmt.Sprintf("descriptor service %d", i)
		if !serviceNamePattern.MatchString(s.Service) {
			return fmt.Errorf("%s: service %q is not a valid compose service name", context, s.Service)
		}
		if seen[s.Service] {
			return fmt.Errorf("%s: duplicate service %q", context, s.Service)
		}
		seen[s.Service] = true
		if s.Class != Application && s.Class != Supporting {
			return fmt.Errorf("%s: class %q is not application or supporting", context, s.Class)
		}
		if s.Replicas <= 0 {
			return fmt.Errorf("%s: replicas must be positive", context)
		}
		if !validCPUSet(s.CPUSet) {
			return fmt.Errorf("%s: invalid cpuset %q", context, s.CPUSet)
		}
		allowed := d.Allocation.ApplicationCPUSets
		if s.Class == Supporting {
			allowed = d.Allocation.SupportingCPUSets
		}
		if !cpusetSubset(s.CPUSet, allowed) {
			return fmt.Errorf("%s: cpuset %q is not declared for class %s", context, s.CPUSet, s.Class)
		}
		if s.MemoryBytes <= 0 {
			return fmt.Errorf("%s: memory_bytes must be positive", context)
		}
		if s.Image != "" && strings.TrimSpace(s.Image) != s.Image {
			return fmt.Errorf("%s: image must not have surrounding whitespace", context)
		}
		if s.Digest != "" && !imageDigestPattern.MatchString(s.Digest) {
			return fmt.Errorf("%s: digest must be sha256:<64 lower-case hex>", context)
		}
	}
	return nil
}

// Validate checks the inventory after decoding or after direct construction.
func (inv Inventory) Validate() error {
	if inv.SchemaVersion != SchemaVersion {
		return fmt.Errorf("inventory: unsupported schema_version %d", inv.SchemaVersion)
	}
	if !composeNamePattern.MatchString(inv.Project) {
		return fmt.Errorf("inventory: project %q is not a valid compose project name", inv.Project)
	}
	if strings.TrimSpace(inv.Source) == "" {
		return errors.New("inventory: source is required")
	}
	if inv.CapturedAtMS != nil && *inv.CapturedAtMS < 0 {
		return errors.New("inventory: captured_at_ms must be non-negative")
	}
	if inv.EffectiveConfiguration != nil {
		if err := validateEffectiveConfiguration("inventory effective_configuration", *inv.EffectiveConfiguration); err != nil {
			return err
		}
	}
	if len(inv.Services) == 0 {
		return errors.New("inventory: services must be a non-empty explicit array")
	}
	seenServices := map[string]bool{}
	seenContainers := map[string]bool{}
	for i, svc := range inv.Services {
		context := fmt.Sprintf("inventory service %d", i)
		if svc.Project != inv.Project {
			return fmt.Errorf("%s: enclosing project %q does not match inventory project %q", context, svc.Project, inv.Project)
		}
		if !serviceNamePattern.MatchString(svc.Service) {
			return fmt.Errorf("%s: service %q is not a valid compose service name", context, svc.Service)
		}
		if seenServices[svc.Service] {
			return fmt.Errorf("%s: duplicate service %q", context, svc.Service)
		}
		seenServices[svc.Service] = true
		if svc.Replicas <= 0 {
			return fmt.Errorf("%s: replicas must be positive", context)
		}
		if svc.CPUSet != "" && !validCPUSet(svc.CPUSet) {
			return fmt.Errorf("%s: invalid cpuset %q", context, svc.CPUSet)
		}
		if len(svc.Containers) == 0 {
			return fmt.Errorf("%s: containers must be a non-empty explicit array", context)
		}
		if svc.Replicas != len(svc.Containers) {
			return fmt.Errorf("%s: declared replicas %d does not equal %d containers", context, svc.Replicas, len(svc.Containers))
		}
		replicas := map[int]bool{}
		for j, c := range svc.Containers {
			ccontext := fmt.Sprintf("%s container %d", context, j)
			if c.Project != inv.Project {
				return fmt.Errorf("%s: project %q does not match inventory project %q", ccontext, c.Project, inv.Project)
			}
			if c.Service != svc.Service {
				return fmt.Errorf("%s: service %q does not match enclosing service %q", ccontext, c.Service, svc.Service)
			}
			if strings.TrimSpace(c.ContainerID) == "" {
				return fmt.Errorf("%s: container_id is required", ccontext)
			}
			if seenContainers[c.ContainerID] {
				return fmt.Errorf("%s: duplicate container_id %q", ccontext, c.ContainerID)
			}
			seenContainers[c.ContainerID] = true
			if c.Replica < 0 || c.Replica >= svc.Replicas {
				return fmt.Errorf("%s: replica %d is outside [0,%d)", ccontext, c.Replica, svc.Replicas)
			}
			if replicas[c.Replica] {
				return fmt.Errorf("%s: duplicate replica %d", ccontext, c.Replica)
			}
			replicas[c.Replica] = true
			if strings.TrimSpace(c.Image) == "" {
				return fmt.Errorf("%s: image is required", ccontext)
			}
			if c.Digest != "" && !imageDigestPattern.MatchString(c.Digest) {
				return fmt.Errorf("%s: digest must be sha256:<64 lower-case hex>", ccontext)
			}
			if c.StartedAtMS <= 0 {
				return fmt.Errorf("%s: started_at_ms must be positive and explicitly supplied", ccontext)
			}
			if !validHealth[c.Health] {
				return fmt.Errorf("%s: health %q is empty or not a supported state", ccontext, c.Health)
			}
			if !validState[c.State] {
				return fmt.Errorf("%s: state %q is empty or not a supported state", ccontext, c.State)
			}
			if c.Health != "healthy" && strings.TrimSpace(c.HealthReason) == "" {
				return fmt.Errorf("%s: non-healthy health requires health_reason", ccontext)
			}
			if c.State != "running" && strings.TrimSpace(c.StateReason) == "" {
				return fmt.Errorf("%s: non-running state requires state_reason", ccontext)
			}
		}
		for r := 0; r < svc.Replicas; r++ {
			if !replicas[r] {
				return fmt.Errorf("%s: replica set is incomplete; missing replica %d", context, r)
			}
		}
	}
	return nil
}

func validateAllocation(a Allocation) error {
	if len(a.ApplicationCPUSets) == 0 {
		return errors.New("descriptor allocation: application_cpusets must be non-empty")
	}
	if len(a.SupportingCPUSets) == 0 {
		return errors.New("descriptor allocation: supporting_cpusets must be non-empty")
	}
	for label, sets := range map[string][]string{
		"application_cpusets": a.ApplicationCPUSets,
		"supporting_cpusets":  a.SupportingCPUSets,
	} {
		seen := map[string]bool{}
		for _, set := range sets {
			if !validCPUSet(set) {
				return fmt.Errorf("descriptor allocation: %s contains invalid cpuset %q", label, set)
			}
			if seen[set] {
				return fmt.Errorf("descriptor allocation: %s contains duplicate cpuset %q", label, set)
			}
			seen[set] = true
		}
	}
	return nil
}

func validateProtocol(p ProtocolBinding) error {
	if !sha256Pattern.MatchString(p.BindingSHA256) {
		return errors.New("descriptor protocol: binding_sha256 must be 64 lower-case hex characters")
	}
	if !sha256Pattern.MatchString(p.ProfileSHA256) {
		return errors.New("descriptor protocol: profile_sha256 must be 64 lower-case hex characters")
	}
	if p.ProfilePath != "" && strings.TrimSpace(p.ProfilePath) != p.ProfilePath {
		return errors.New("descriptor protocol: profile_path must not have surrounding whitespace")
	}
	return nil
}

func validateProvenance(p Provenance) error {
	if strings.TrimSpace(p.Source) == "" {
		return errors.New("descriptor provenance: source is required")
	}
	if (p.ComposePath == "") != (p.ComposeSHA256 == "") {
		return errors.New("descriptor provenance: compose_path and compose_sha256 must be supplied together")
	}
	if p.ComposeSHA256 != "" && !sha256Pattern.MatchString(p.ComposeSHA256) {
		return errors.New("descriptor provenance: compose_sha256 must be 64 lower-case hex characters")
	}
	if p.CreatedAtMS != nil && *p.CreatedAtMS < 0 {
		return errors.New("descriptor provenance: created_at_ms must be non-negative when supplied")
	}
	return validateEvidenceDocuments(p.Documents, "descriptor provenance")
}

func validateEffectiveConfiguration(context string, c EffectiveConfiguration) error {
	if !sha256Pattern.MatchString(c.SHA256) {
		return fmt.Errorf("%s: sha256 must be 64 lower-case hex characters", context)
	}
	if c.Path != "" && strings.TrimSpace(c.Path) != c.Path {
		return fmt.Errorf("%s: path must not have surrounding whitespace", context)
	}
	return nil
}

func validateEvidenceDocuments(docs []EvidenceDocument, context string) error {
	seen := map[string]bool{}
	for i, doc := range docs {
		if !validEvidenceRoles[doc.Role] {
			return fmt.Errorf("%s document %d: role %q is not supported", context, i, doc.Role)
		}
		if strings.TrimSpace(doc.Path) == "" {
			return fmt.Errorf("%s document %d: path is required", context, i)
		}
		if seen[doc.Path] {
			return fmt.Errorf("%s document %d: duplicate path %q", context, i, doc.Path)
		}
		seen[doc.Path] = true
		if !sha256Pattern.MatchString(doc.SHA256) {
			return fmt.Errorf("%s document %d: sha256 must be 64 lower-case hex characters", context, i)
		}
	}
	return nil
}

// Canonical returns a deep copy with deterministic ordering. It never mutates
// the receiver and never adds information that the caller did not supply.
func (d Descriptor) Canonical() Descriptor {
	out := d
	out.Allocation.ApplicationCPUSets = append([]string(nil), d.Allocation.ApplicationCPUSets...)
	out.Allocation.SupportingCPUSets = append([]string(nil), d.Allocation.SupportingCPUSets...)
	sort.Strings(out.Allocation.ApplicationCPUSets)
	sort.Strings(out.Allocation.SupportingCPUSets)
	out.Services = append([]DescriptorService(nil), d.Services...)
	sort.SliceStable(out.Services, func(i, j int) bool { return out.Services[i].Service < out.Services[j].Service })
	out.Provenance = d.Provenance.Canonical()
	return out
}

// Canonical returns a deep copy with provenance documents sorted
// deterministically.
func (p Provenance) Canonical() Provenance {
	out := p
	out.Documents = append([]EvidenceDocument(nil), p.Documents...)
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

// CanonicalJSON is the deterministic compact canonical representation used for
// semantic hashing.
func (d Descriptor) CanonicalJSON() ([]byte, error) {
	return json.Marshal(d.Canonical())
}

// CanonicalSHA256 is the semantic descriptor hash. It is order-invariant for
// service, allocation and evidence-document arrays.
func (d Descriptor) CanonicalSHA256() (string, error) {
	b, err := d.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

// Canonical returns a deep copy with deterministic ordering.
func (inv Inventory) Canonical() Inventory {
	out := inv
	out.Services = append([]InventoryService(nil), inv.Services...)
	for i := range out.Services {
		out.Services[i].Containers = append([]InventoryContainer(nil), out.Services[i].Containers...)
		sort.SliceStable(out.Services[i].Containers, func(a, b int) bool {
			ca, cb := out.Services[i].Containers[a], out.Services[i].Containers[b]
			if ca.Replica != cb.Replica {
				return ca.Replica < cb.Replica
			}
			return ca.ContainerID < cb.ContainerID
		})
	}
	sort.SliceStable(out.Services, func(i, j int) bool { return out.Services[i].Service < out.Services[j].Service })
	return out
}

// CanonicalJSON is the deterministic compact canonical representation used for
// semantic hashing.
func (inv Inventory) CanonicalJSON() ([]byte, error) {
	return json.Marshal(inv.Canonical())
}

// CanonicalSHA256 is the semantic inventory hash. It is order-invariant for
// service and container arrays.
func (inv Inventory) CanonicalSHA256() (string, error) {
	b, err := inv.CanonicalJSON()
	if err != nil {
		return "", err
	}
	return SHA256Bytes(b), nil
}

func platformAllowlisted(platform string) bool { return platformAllowlist[platform] }

func cpusetSubset(service string, allowed []string) bool {
	want, ok := cpuSetValues(service)
	if !ok {
		return false
	}
	for _, candidate := range allowed {
		values, ok := cpuSetValues(candidate)
		if !ok {
			continue
		}
		contained := true
		for v := range want {
			if !values[v] {
				contained = false
				break
			}
		}
		if contained {
			return true
		}
	}
	return false
}

func cpuSetValues(s string) (map[int64]bool, bool) {
	if !validCPUSet(s) {
		return nil, false
	}
	out := map[int64]bool{}
	for _, part := range strings.Split(s, ",") {
		bounds := strings.Split(part, "-")
		start, ok := parseNonNegative(bounds[0])
		if !ok {
			return nil, false
		}
		end := start
		if len(bounds) == 2 {
			end, ok = parseNonNegative(bounds[1])
			if !ok || end < start {
				return nil, false
			}
		}
		for v := start; v <= end; v++ {
			out[v] = true
		}
	}
	return out, true
}

func validCPUSet(s string) bool {
	if s == "" || strings.TrimSpace(s) != s {
		return false
	}
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			return false
		}
		bounds := strings.Split(part, "-")
		switch len(bounds) {
		case 1:
			if !allDigits(bounds[0]) {
				return false
			}
		case 2:
			if !allDigits(bounds[0]) || !allDigits(bounds[1]) {
				return false
			}
			start, ok1 := parseNonNegative(bounds[0])
			end, ok2 := parseNonNegative(bounds[1])
			if !ok1 || !ok2 || start > end {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseNonNegative(s string) (int64, bool) {
	var n int64
	for _, r := range s {
		n = n*10 + int64(r-'0')
		if n > 1<<20 {
			return 0, false
		}
	}
	return n, true
}

// rejectDuplicateJSONKeys checks every object before encoding/json can collapse
// duplicate members.
func rejectDuplicateJSONKeys(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return errors.New("malformed JSON object")
			}
		case '[':
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return errors.New("malformed JSON array")
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := dec.Token(); err == nil {
		return errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// rejectJSONNulls rejects explicit nulls at every nesting level. Optional
// evidence is represented by omission, never by null.
func rejectJSONNulls(b []byte, context string) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("%s: %w", context, err)
	}
	var walk func(any, string) error
	walk = func(v any, path string) error {
		switch x := v.(type) {
		case nil:
			return fmt.Errorf("%s: %s must not be null; omit optional evidence instead", context, path)
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if err := walk(x[k], path+"."+k); err != nil {
					return err
				}
			}
		case []any:
			for i, item := range x {
				if err := walk(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(v, "document")
}
