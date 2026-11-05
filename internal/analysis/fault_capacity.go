package analysis

// M1f top-level offline fault/capacity evidence document and deterministic
// serialization. This extends the existing internal/analysis output conventions;
// it is not a second reporting pipeline. The reducer never populates itself from
// Analyze and never claims campaign capacity by default.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FaultCapacityEvidence struct {
	SchemaVersion int                 `json:"schema_version"`
	Synthetic     bool                `json:"synthetic"`
	Inventory     *FaultInventory     `json:"inventory,omitempty"`
	Target        *FaultTargetRequest `json:"target,omitempty"`
	Event         *FaultEvent         `json:"event,omitempty"`
	Capacity      *CapacityInput      `json:"capacity,omitempty"`
}

type FaultCapacityReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Synthetic     bool                   `json:"synthetic"`
	Target        *FaultTargetResolution `json:"fault_target,omitempty"`
	Event         *FaultReduction        `json:"fault_event,omitempty"`
	Capacity      *CapacityResult        `json:"capacity,omitempty"`
	Limitations   []string               `json:"limitations"`
}

// DecodeFaultCapacityEvidence strictly decodes the versioned evidence document.
// Unknown fields, duplicate keys, trailing JSON and explicit nulls are rejected.
func DecodeFaultCapacityEvidence(b []byte) (*FaultCapacityEvidence, error) {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, err
	}
	if err := requireJSONFields(top, "fault capacity evidence", "schema_version", "synthetic"); err != nil {
		return nil, err
	}
	for key := range top {
		switch key {
		case "schema_version", "synthetic", "inventory", "target", "event", "capacity":
		default:
			return nil, fmt.Errorf("fault capacity evidence has unsupported field %q", key)
		}
	}
	if err := rejectJSONNulls(top, "fault capacity evidence"); err != nil {
		return nil, err
	}
	ev := &FaultCapacityEvidence{}
	if err := decode(b, ev, true); err != nil {
		return nil, err
	}
	if ev.SchemaVersion != FaultTargetSchemaVersion {
		return nil, fmt.Errorf("unsupported fault capacity evidence schema_version %d", ev.SchemaVersion)
	}
	if raw, ok := top["inventory"]; ok {
		inv, err := DecodeFaultInventory(raw)
		if err != nil {
			return nil, fmt.Errorf("inventory: %w", err)
		}
		ev.Inventory = inv
	}
	if raw, ok := top["target"]; ok {
		req, err := DecodeFaultTargetRequest(raw)
		if err != nil {
			return nil, fmt.Errorf("target: %w", err)
		}
		ev.Target = req
	}
	if raw, ok := top["event"]; ok {
		event, err := DecodeFaultEvent(raw)
		if err != nil {
			return nil, fmt.Errorf("event: %w", err)
		}
		ev.Event = event
	}
	if raw, ok := top["capacity"]; ok {
		capacity, err := DecodeCapacityInput(raw)
		if err != nil {
			return nil, fmt.Errorf("capacity: %w", err)
		}
		ev.Capacity = capacity
	}
	if ev.Inventory == nil && ev.Event == nil && ev.Capacity == nil {
		return nil, fmt.Errorf("fault capacity evidence must supply at least one of inventory, event or capacity")
	}
	return ev, nil
}

// ReduceFaultCapacityEvidence resolves/reduces each supplied section. It is pure
// and file-free; writing is a separate explicit step.
func ReduceFaultCapacityEvidence(ev *FaultCapacityEvidence) (*FaultCapacityReport, error) {
	if ev == nil {
		return nil, fmt.Errorf("nil fault capacity evidence")
	}
	if ev.SchemaVersion != FaultTargetSchemaVersion {
		return nil, fmt.Errorf("unsupported fault capacity evidence schema_version %d", ev.SchemaVersion)
	}
	if ev.Inventory != nil && ev.Target == nil {
		return nil, fmt.Errorf("inventory supplied without an explicit target request")
	}
	if ev.Target != nil && ev.Inventory == nil {
		return nil, fmt.Errorf("target request supplied without an inventory")
	}
	report := &FaultCapacityReport{
		SchemaVersion: FaultTargetSchemaVersion,
		Synthetic:     ev.Synthetic,
		Limitations: []string{
			"Offline M1f foundation only: no Docker client, stop command, fault scheduling, live observation or campaign qualification was performed.",
			"Capacity bounds are descriptive explicit-window observations and are not a campaign C_SLO unless a separate authorized procedure adopts them.",
			"The degraded-topology reference is declared from N and one intended replica; it is not inferred from transient unavailability or used as capacity evidence.",
		},
	}
	if ev.Target != nil {
		resolution := ResolveFaultTarget(*ev.Inventory, *ev.Target)
		report.Target = &resolution
	}
	if ev.Event != nil {
		reduction := ReduceFaultEvent(*ev.Event)
		report.Event = &reduction
	}
	if ev.Capacity != nil {
		capacity := ReduceCapacity(*ev.Capacity)
		report.Capacity = &capacity
	}
	return report, nil
}

// WriteFaultCapacity creates a new output directory and writes the deterministic
// JSON report, a Markdown reading aid and SHA256SUMS.
func WriteFaultCapacity(out string, report *FaultCapacityReport, inputs []string) error {
	if report == nil {
		return fmt.Errorf("nil fault capacity report")
	}
	if err := newDirectory(out, inputs); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(out, "fault-capacity.json"), report); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "fault-capacity.md"), []byte(faultCapacityMarkdown(report)), 0644); err != nil {
		return err
	}
	return checksums(out)
}

func faultCapacityMarkdown(r *FaultCapacityReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Fault/capacity reduction (offline)\n\nSynthetic: **%t**. Campaign capacity: **NOT CLAIMED**.\n\n", r.Synthetic)
	if r.Target != nil {
		fmt.Fprintf(&b, "## Fault target\n\nEligible: **%t**; policy `%s`; dry run: **%t**.\n\n", r.Target.Eligible, r.Target.PolicyVersion, r.Target.DryRun)
		if r.Target.Target != nil {
			t := r.Target.Target
			fmt.Fprintf(&b, "Target: `%s/%s/%s` container `%s`, replica %d/%d, image `%s`, started `%g`.\n\n", t.Platform, t.Project, t.Service, t.ContainerID, t.Replica, t.Replicas, t.Image, t.StartedAtMS)
		}
		for _, reason := range r.Target.RefusalReasons {
			fmt.Fprintf(&b, "- Refused: %s\n", reason)
		}
		b.WriteString("\n")
	}
	if r.Event != nil {
		fmt.Fprintf(&b, "## Fault event\n\nStatus: **%s**; qualified: **%t**; dry run: **%t**.\n\n", r.Event.Status, r.Event.Qualified, r.Event.DryRun)
		if r.Event.Transient != nil {
			t := r.Event.Transient
			fmt.Fprintf(&b, "Transient: stop observed **%t**, rejoin observed **%t**, LB removal observed **%t**.\n\n", t.StopObserved, t.HealthyRejoinObserved, t.LBRemovalObserved)
		}
		if r.Event.DegradedTopology != nil {
			d := r.Event.DegradedTopology
			fmt.Fprintf(&b, "Degraded topology reference: `%s`; remaining replicas %d/%d (down replica %d); measured **%t**; inferred from transient window **%t**.\n\n", d.Classification, d.RemainingReplicas, d.Replicas, d.DownReplica, d.DegradationMeasured, d.InferredFromTransientWindow)
		}
		for _, reason := range r.Event.Reasons {
			fmt.Fprintf(&b, "- %s\n", reason)
		}
		b.WriteString("\n")
	}
	if r.Capacity != nil {
		c := r.Capacity
		fmt.Fprintf(&b, "## Capacity observation\n\nStatus: **%s**. Lower: %s. Upper: %s.\n\nStop reason: %s.\n\n", c.Status, boundText(c.LowerBound), boundText(c.UpperBound), c.StopReason)
		fmt.Fprintln(&b, "| Window | Rate | Fault | Validity | Outcome | Violation | Used for capacity |")
		fmt.Fprintln(&b, "|---|---:|---|---|---|---|---|")
		for _, w := range c.Windows {
			fmt.Fprintf(&b, "| %s | %g | %t | %s | %s | %t | %t |\n", w.ID, w.OfferedRate, w.Fault, w.Validity, w.Outcome, w.ObservedViolation, w.UsedForCapacity)
		}
		b.WriteString("\n")
	}
	fmt.Fprintln(&b, "## Limitations")
	for _, l := range r.Limitations {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	return b.String()
}

func boundText(v *float64) string {
	if v == nil {
		return "not established"
	}
	return fmt.Sprintf("%g messages/s", *v)
}
