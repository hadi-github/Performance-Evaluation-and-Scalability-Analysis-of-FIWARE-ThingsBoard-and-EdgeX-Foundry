package analysis

// M1f bounded fault-target resolution. This file is deliberately pure: it reads a
// caller-supplied, already-verified inventory document and never opens a Docker
// socket, issues a stop command, or schedules an unattended fault. The only
// supported operation is a dry-run resolution of ONE application replica from the
// versioned ingress allowlist.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

const (
	// FaultTargetSchemaVersion versions the inventory/request wire document.
	FaultTargetSchemaVersion = 1
	// FaultTargetPolicyVersion names the exact allowlist semantics below. A caller may
	// not infer a target from a different policy generation.
	FaultTargetPolicyVersion = "fault-target-allowlist-v1"
)

// faultTargetAllowlist is the complete set of admissible application replicas. It
// intentionally does not admit stateful stores, HAProxy, monitoring exporters or a
// whole-stack selector. Any other platform/service pair is refused.
var faultTargetAllowlist = map[string]string{
	"thingsboard": "tb-http-transport",
	"fiware":      "iot-agent",
	"edgex":       "device-rest",
}

// FaultInventory is a previously verified project/service/replica/container
// inventory. It is an input artifact, not a live Docker result. Containers carry
// both their project and compose service so a mismatched or duplicated row can be
// refused instead of silently accepted.
type FaultInventory struct {
	SchemaVersion int                     `json:"schema_version"`
	Project       string                  `json:"project"`
	Services      []FaultInventoryService `json:"services"`
}

type FaultInventoryService struct {
	Project    string                    `json:"project"`
	Service    string                    `json:"compose_service"`
	Replicas   int                       `json:"replicas"`
	Image      string                    `json:"image"`
	Containers []FaultInventoryContainer `json:"containers"`
}

type FaultInventoryContainer struct {
	Project     string            `json:"project"`
	Service     string            `json:"compose_service"`
	ContainerID string            `json:"container_id"`
	Image       string            `json:"image"`
	Replica     int               `json:"replica"`
	StartedAtMS float64           `json:"started_at_ms"`
	Healthy     bool              `json:"healthy"`
	Labels      map[string]string `json:"labels"`
}

// FaultTargetRequest is the operator's explicit proposal. Every non-empty identity
// field is an assertion that the inventory must match; an omitted field is not a
// replacement value. ReplicaIndex is zero-based and Replicas is the expected N.
type FaultTargetRequest struct {
	Platform    string            `json:"platform"`
	Project     string            `json:"project"`
	Service     string            `json:"service"`
	Replica     int               `json:"replica"`
	Replicas    int               `json:"replicas"`
	ContainerID string            `json:"container_id,omitempty"`
	Image       string            `json:"image,omitempty"`
	StartedAtMS *float64          `json:"started_at_ms,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	DryRun      bool              `json:"dry_run"`
}

// FaultTargetIdentity is emitted only after every supplied assertion agreed with
// exactly one container. There is no partially resolved identity.
type FaultTargetIdentity struct {
	Platform    string            `json:"platform"`
	Project     string            `json:"project"`
	Service     string            `json:"service"`
	ContainerID string            `json:"container_id"`
	Image       string            `json:"image"`
	Replica     int               `json:"replica"`
	Replicas    int               `json:"replicas"`
	StartedAtMS float64           `json:"started_at_ms"`
	Labels      map[string]string `json:"labels"`
}

type FaultTargetResolution struct {
	SchemaVersion  int                  `json:"schema_version"`
	PolicyVersion  string               `json:"policy_version"`
	DryRun         bool                 `json:"dry_run"`
	Eligible       bool                 `json:"eligible"`
	Request        FaultTargetRequest   `json:"request"`
	Target         *FaultTargetIdentity `json:"target,omitempty"`
	RefusalReasons []string             `json:"refusal_reasons"`
}

// ResolveFaultTarget resolves exactly one allowlisted application replica from a
// supplied inventory. It never mutates the inventory, calls Docker, or executes a
// stop command. Refusals are deterministic and sorted; on refusal no target
// identity is returned.
func ResolveFaultTarget(inv FaultInventory, req FaultTargetRequest) FaultTargetResolution {
	res := FaultTargetResolution{
		SchemaVersion:  FaultTargetSchemaVersion,
		PolicyVersion:  FaultTargetPolicyVersion,
		DryRun:         req.DryRun,
		Request:        req,
		RefusalReasons: []string{},
	}
	refuse := func(format string, args ...any) {
		res.RefusalReasons = append(res.RefusalReasons, fmt.Sprintf(format, args...))
	}

	if !req.DryRun {
		refuse("dry-run resolution is mandatory; this resolver never issues a stop command")
	}
	if inv.SchemaVersion != FaultTargetSchemaVersion {
		refuse("unsupported fault inventory schema_version %d", inv.SchemaVersion)
	}
	if req.Service == "" {
		refuse("explicit compose service is required")
	}
	allowed, knownPlatform := faultTargetAllowlist[req.Platform]
	if !knownPlatform {
		refuse("platform %q is outside the versioned fault-target allowlist", req.Platform)
	} else if req.Service != "" && req.Service != allowed {
		refuse("service %q is not the allowlisted %s ingress application replica (%q); stateful, HAProxy, monitoring and whole-stack targets are refused", req.Service, req.Platform, allowed)
	}
	if req.Project == "" {
		refuse("explicit project is required")
	}
	if inv.Project != req.Project {
		refuse("inventory project %q does not match requested project %q", inv.Project, req.Project)
	}
	if req.Replicas <= 0 {
		refuse("N is invalid: replicas must be a positive integer")
	}
	if req.Replica < 0 || req.Replica >= req.Replicas {
		refuse("replica index %d is outside declared N=%d", req.Replica, req.Replicas)
	}
	if len(inv.Services) == 0 {
		refuse("inventory has no services")
	}

	// Duplicate container IDs and replica indexes make the inventory ambiguous even
	// if only one of the duplicates is in the requested service. Every container row
	// is validated against the service/project that encloses it, so a row cannot
	// spoof a different compose service merely by declaring it.
	//
	// Project identity is an assertion, not an optional hint. An empty enclosing
	// project is not "unknown" and does not waive consistency: if it were skipped,
	// an unrelated service with project="" could enclose a container declaring
	// project="bench" (or an empty project) and the otherwise valid allowlisted
	// target would still resolve. Every service, related or not, must therefore
	// declare the requested project, and every container must match its enclosing
	// service project exactly, including the empty case.
	ids := map[string]int{}
	for _, svc := range inv.Services {
		if svc.Project != req.Project {
			refuse("inventory service %q declares project %q, not requested project %q", svc.Service, svc.Project, req.Project)
		}
		seenReplica := map[int]bool{}
		for _, c := range svc.Containers {
			ids[c.ContainerID]++
			if c.ContainerID == "" {
				refuse("inventory contains a container without an id")
			}
			if c.Project != svc.Project {
				refuse("container %q belongs to project %q, not its enclosing service project %q", c.ContainerID, c.Project, svc.Project)
			}
			if c.Service != svc.Service {
				refuse("container %q declares compose_service %q but is enclosed by service %q", c.ContainerID, c.Service, svc.Service)
			}
			if seenReplica[c.Replica] {
				refuse("duplicate replica index %d in service %q", c.Replica, svc.Service)
			}
			seenReplica[c.Replica] = true
			if svc.Replicas > 0 && (c.Replica < 0 || c.Replica >= svc.Replicas) {
				refuse("replica index %d in service %q is outside declared N=%d", c.Replica, svc.Service, svc.Replicas)
			}
		}
	}
	for id, n := range ids {
		if n > 1 {
			refuse("duplicate container id %q appears %d times", id, n)
		}
	}

	var matchedService *FaultInventoryService
	for i := range inv.Services {
		svc := &inv.Services[i]
		if svc.Service != req.Service {
			continue
		}
		if matchedService != nil {
			refuse("duplicate inventory service entry %q", req.Service)
			continue
		}
		matchedService = svc
	}
	if matchedService == nil {
		refuse("inventory has no service %q", req.Service)
	} else {
		if matchedService.Project != req.Project {
			refuse("service %q belongs to project %q, not %q", req.Service, matchedService.Project, req.Project)
		}
		if matchedService.Replicas != req.Replicas {
			refuse("service %q declares N=%d, request declares N=%d", req.Service, matchedService.Replicas, req.Replicas)
		}
		if matchedService.Replicas > 0 && len(matchedService.Containers) != matchedService.Replicas {
			refuse("service %q has %d containers for declared N=%d", req.Service, len(matchedService.Containers), matchedService.Replicas)
		}
		// The complete declared replica set, not merely the requested replica, must
		// be unique and contiguous over [0,N). This prevents a partial or shifted
		// inventory from resolving an otherwise plausible replica.
		if matchedService.Replicas > 0 {
			seen := map[int]bool{}
			for _, c := range matchedService.Containers {
				seen[c.Replica] = true
			}
			for i := 0; i < matchedService.Replicas; i++ {
				if !seen[i] {
					refuse("service %q is missing declared replica index %d of N=%d", req.Service, i, matchedService.Replicas)
				}
			}
		}
	}

	// Resolve only from the uniquely matched allowlisted service. Self-declared
	// container service strings were already validated against their enclosing
	// service above and are not used as a selection authority.
	var matches []FaultInventoryContainer
	if matchedService != nil {
		for _, c := range matchedService.Containers {
			if c.Replica == req.Replica {
				matches = append(matches, c)
			}
		}
	}
	switch len(matches) {
	case 0:
		refuse("no inventory container matches service %q replica %d", req.Service, req.Replica)
	case 1:
	default:
		refuse("ambiguous target: %d containers match service %q replica %d", len(matches), req.Service, req.Replica)
	}
	if len(matches) == 1 {
		c := matches[0]
		if c.Project != req.Project {
			refuse("target container %q belongs to project %q, not %q", c.ContainerID, c.Project, req.Project)
		}
		if c.ContainerID == "" {
			refuse("target container id is missing")
		}
		if req.ContainerID != "" && c.ContainerID != req.ContainerID {
			refuse("unexpected container id %q; requested %q", c.ContainerID, req.ContainerID)
		}
		if c.Image == "" {
			refuse("target container image is missing")
		}
		if req.Image != "" && c.Image != req.Image {
			refuse("image mismatch: inventory %q, requested %q", c.Image, req.Image)
		}
		if matchedService != nil && matchedService.Image != "" && matchedService.Image != c.Image {
			refuse("service image %q contradicts container image %q", matchedService.Image, c.Image)
		}
		if !c.Healthy {
			refuse("target container is not healthy in the supplied inventory")
		}
		if !finiteNumber(c.StartedAtMS) || c.StartedAtMS <= 0 {
			refuse("target container started_at_ms is missing or invalid")
		}
		if req.StartedAtMS != nil {
			if !finiteNumber(*req.StartedAtMS) || *req.StartedAtMS != c.StartedAtMS {
				refuse("started_at_ms mismatch: inventory %g, requested %g", c.StartedAtMS, *req.StartedAtMS)
			}
		}
		for k, want := range req.Labels {
			if c.Labels == nil || c.Labels[k] != want {
				refuse("expected label %q=%q is absent from the target container evidence", k, want)
			}
		}
		if len(res.RefusalReasons) == 0 {
			labels := map[string]string{}
			for k, v := range c.Labels {
				labels[k] = v
			}
			res.Target = &FaultTargetIdentity{
				Platform:    req.Platform,
				Project:     c.Project,
				Service:     c.Service,
				ContainerID: c.ContainerID,
				Image:       c.Image,
				Replica:     c.Replica,
				Replicas:    req.Replicas,
				StartedAtMS: c.StartedAtMS,
				Labels:      labels,
			}
		}
	}

	if len(res.RefusalReasons) > 0 {
		sort.Strings(res.RefusalReasons)
		// Remove exact duplicates while preserving the deterministic sorted order.
		out := res.RefusalReasons[:0]
		for _, r := range res.RefusalReasons {
			if len(out) == 0 || out[len(out)-1] != r {
				out = append(out, r)
			}
		}
		res.RefusalReasons = out
		res.Target = nil
		res.Eligible = false
		return res
	}
	res.Eligible = true
	return res
}

func finiteNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// DecodeFaultInventory strictly decodes an inventory document: duplicate keys,
// unknown fields, trailing JSON and explicit nulls are rejected. Absent optional
// evidence is represented by omission, never by null.
func DecodeFaultInventory(b []byte) (*FaultInventory, error) {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, err
	}
	if err := requireJSONFields(top, "inventory", "schema_version", "project", "services"); err != nil {
		return nil, err
	}
	if err := rejectJSONNulls(top, "inventory"); err != nil {
		return nil, err
	}
	var inv FaultInventory
	if err := decode(b, &inv, true); err != nil {
		return nil, err
	}
	var svcs []map[string]json.RawMessage
	if err := json.Unmarshal(top["services"], &svcs); err != nil {
		return nil, fmt.Errorf("inventory services must be an explicit array: %w", err)
	}
	for i, svc := range svcs {
		if err := requireJSONFields(svc, fmt.Sprintf("inventory service %d", i), "project", "compose_service", "replicas", "image", "containers"); err != nil {
			return nil, err
		}
		if err := rejectJSONNulls(svc, fmt.Sprintf("inventory service %d", i)); err != nil {
			return nil, err
		}
		var containers []map[string]json.RawMessage
		if err := json.Unmarshal(svc["containers"], &containers); err != nil {
			return nil, fmt.Errorf("inventory service %d containers must be an explicit array: %w", i, err)
		}
		for j, c := range containers {
			if err := requireJSONFields(c, fmt.Sprintf("inventory service %d container %d", i, j), "project", "compose_service", "container_id", "image", "replica", "started_at_ms", "healthy", "labels"); err != nil {
				return nil, err
			}
			if err := rejectJSONNulls(c, fmt.Sprintf("inventory service %d container %d", i, j)); err != nil {
				return nil, err
			}
		}
	}
	return &inv, nil
}

// DecodeFaultTargetRequest strictly decodes an explicit request. Dry-run and the
// service/replica/N selectors are mandatory.
func DecodeFaultTargetRequest(b []byte) (*FaultTargetRequest, error) {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, err
	}
	if err := requireJSONFields(top, "fault target request", "platform", "project", "service", "replica", "replicas", "dry_run"); err != nil {
		return nil, err
	}
	if err := rejectJSONNulls(top, "fault target request"); err != nil {
		return nil, err
	}
	var req FaultTargetRequest
	if err := decode(b, &req, true); err != nil {
		return nil, err
	}
	return &req, nil
}

func requireJSONFields(obj map[string]json.RawMessage, context string, fields ...string) error {
	for _, f := range fields {
		v, ok := obj[f]
		if !ok {
			return fmt.Errorf("%s missing required field %q", context, f)
		}
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("%s field %q must not be null", context, f)
		}
	}
	return nil
}

func rejectJSONNulls(obj map[string]json.RawMessage, context string) error {
	for key, v := range obj {
		if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			return fmt.Errorf("%s field %q must not be null; omit optional evidence instead", context, key)
		}
	}
	return nil
}

func isAllowlistedFaultService(platform, service string) bool {
	want, ok := faultTargetAllowlist[platform]
	return ok && service == want
}
