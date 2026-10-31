// Package cpubound turns two boundary snapshots of cumulative cgroup CPU-usage counters (as produced by
// monitoring/exporters/cpu-snapshot.sh on the SUT) into the CPU-cost artifact of the authoritative run bundle:
// per-container CPU-seconds as a counter delta, with restart/reset/missing-counter handling.
//
// Semantics (docs/methodology/measurement-integrity.md, "exact CPU cost requires cgroup endpoint counters"):
//   - a delta is a measured fact only when the same container id was observed at both boundaries with the same
//     StartedAt (no restart), both usage counters are present, and the counter did not go backwards;
//   - RESTARTED, REWOUND, MISSING_BEFORE, MISSING_AFTER and NO_COUNTER containers are reported unavailable —
//     never zero — and the container contributes no CPU-seconds;
//   - "cost per message" carries an explicitly named denominator (who supplies the count and its definition).
//     With no denominator the cost is absent, not estimated.
package cpubound

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// SnapshotRecord is one line of a cpu-snapshot.sh snapshot:
// epoch_s \t container_name \t container_id \t usage_usec \t StartedAt \t cpuset
type SnapshotRecord struct {
	EpochS    string
	Name      string
	ID        string
	StartedAt string
	Cpuset    string
	UsageUsec int64 // present only when the kernel counter was readable
	HasUsage  bool  // false when the counter read failed (reported NO_COUNTER)
	Service   string
	Tier      string
}

type Status = string

const (
	OK            = "ok"
	NoUsageBefore = "no_counter_before"
	NoUsageAfter  = "no_counter_after"
	MissingBefore = "missing_before"
	MissingAfter  = "missing_after"
	Restarted     = "restarted" // StartedAt differs: the counter was reset by the daemon
	Rewound       = "rewound"   // after < before: the counter was reset without a restart
	InvalidWindow = "invalid_window"
)

// Denominator is the explicitly named message count used for cost-per-message.
type Denominator struct {
	Count      int64  `json:"count"`
	Source     string `json:"source"`     // e.g. "explicit", "benchctl audit -history"
	Definition string `json:"definition"` // e.g. "correctness-qualified identities, measured window"
	Present    bool   `json:"-"`          // false when no denominator was supplied
}

// ContainerCost is one container's reconciliation between the two snapshots.
type ContainerCost struct {
	Name            string   `json:"container"`
	ID              string   `json:"container_id"`
	StartedAt       string   `json:"started_at"` // when it matches; when it does not, the after value
	StartedAtBefore string   `json:"started_at_before,omitempty"`
	StartedAtAfter  string   `json:"started_at_after,omitempty"`
	UsageBeforeUsec *int64   `json:"usage_before_usec,omitempty"`
	UsageAfterUsec  *int64   `json:"usage_after_usec,omitempty"`
	DeltaSeconds    *float64 `json:"cpu_seconds,omitempty"` // counter delta, only when status == ok
	Status          Status   `json:"status"`
	Cpuset          string   `json:"cpuset,omitempty"`
	IDAfter         string   `json:"container_id_after,omitempty"`
	EpochBefore     string   `json:"snapshot_before_epoch_seconds,omitempty"`
	EpochAfter      string   `json:"snapshot_after_epoch_seconds,omitempty"`
	Service         string   `json:"service,omitempty"`
	Tier            string   `json:"tier,omitempty"`
}

// Report is the CPU-cost artifact. Available cost-per-message requires every container ok (that is, no
// omitted counter anywhere); per-container costs are reported individually so a partial failure stays
// visible rather than silently zeroing.
type Report struct {
	Denominator               *Denominator       `json:"denominator,omitempty"` // nil = no denominator supplied
	Containers                []ContainerCost    `json:"containers"`
	AllOK                     bool               `json:"counters_ok"`
	TotalCPUSeconds           *float64           `json:"total_cpu_seconds,omitempty"` // sum of ok deltas
	TotalCPUSecondsPerMessage *float64           `json:"total_cpu_seconds_per_message,omitempty"`
	Note                      string             `json:"note,omitempty"`
	CPUSecondsByTier          map[string]float64 `json:"cpu_seconds_by_tier,omitempty"`
}

// Parse reads one snapshot TSV (the header-less output of cpu-snapshot.sh). Lines with unreadable
// usage are kept with HasUsage=false so later boundaries stay nameable and any container missing from
// this snapshot is distinguishable from a container with a zero delta.
func Parse(r io.Reader) ([]SnapshotRecord, error) {
	var out []SnapshotRecord
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			return nil, fmt.Errorf("cpu-boundary: expected >=5 tab-separated fields, got %d: %q", len(f), line)
		}
		epoch, err := strconv.ParseFloat(f[0], 64)
		if err != nil || math.IsNaN(epoch) || math.IsInf(epoch, 0) || epoch <= 0 || f[1] == "" || f[2] == "" || f[4] == "" {
			return nil, fmt.Errorf("cpu-boundary: missing or invalid timestamp/container identity/StartedAt: %q", line)
		}
		if seen[f[1]] {
			return nil, fmt.Errorf("cpu-boundary: duplicate container name %q in a single snapshot", f[1])
		}
		seen[f[1]] = true
		rec := SnapshotRecord{EpochS: f[0], Name: f[1], ID: f[2], HasUsage: false}
		if f[3] != "NA" {
			v, err := strconv.ParseInt(f[3], 10, 64)
			if err != nil || v < 0 {
				return nil, fmt.Errorf("cpu-boundary: bad usage_usec %q: %v", f[3], err)
			}
			rec.HasUsage, rec.UsageUsec = true, v
		}
		if len(f) > 4 {
			rec.StartedAt = f[4]
		}
		if len(f) > 5 {
			rec.Cpuset = f[5]
		}
		if len(f) > 6 {
			rec.Service = f[6]
		}
		if len(f) > 7 {
			rec.Tier = f[7]
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Diff compares two snapshots taken around the measurement interval. Keys are container id (a restart
// means a new id, which is exactly what must break the delta). A container visible only after the interval
// is MISSING_BEFORE; only before, MISSING_AFTER.
func Diff(before, after []SnapshotRecord, denom *Denominator, note string) *Report {
	rep := &Report{Denominator: denom, Note: note}
	nameSeen := map[string]bool{}
	var names []string
	for _, r := range append(append([]SnapshotRecord{}, before...), after...) {
		if !nameSeen[r.Name] {
			nameSeen[r.Name] = true
			names = append(names, r.Name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		brec, inB := firstByName(before, name)
		arec, inA := firstByName(after, name)
		switch {
		case !inB:
			rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: arec.ID, StartedAtAfter: arec.StartedAt, UsageAfterUsec: usagePtr(arec), Status: MissingBefore, Cpuset: arec.Cpuset})
		case !inA:
			rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: brec.ID, StartedAtBefore: brec.StartedAt, UsageBeforeUsec: usagePtr(brec), Status: MissingAfter, Cpuset: brec.Cpuset})
		case !ordered(brec.EpochS, arec.EpochS):
			rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: brec.ID, Status: InvalidWindow})
		case !(brec.HasUsage && arec.HasUsage):
			status := NoUsageAfter
			if !brec.HasUsage {
				status = NoUsageBefore
			}
			rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: brec.ID, StartedAt: brec.StartedAt, UsageBeforeUsec: usagePtr(brec), UsageAfterUsec: usagePtr(arec), Status: status, Cpuset: brec.Cpuset})
		case brec.ID != arec.ID || brec.StartedAt != arec.StartedAt:
			rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: brec.ID, StartedAtBefore: brec.StartedAt, StartedAtAfter: arec.StartedAt, UsageBeforeUsec: usagePtr(brec), UsageAfterUsec: usagePtr(arec), Status: Restarted, Cpuset: brec.Cpuset})
		default:
			d := arec.UsageUsec - brec.UsageUsec
			if d < 0 {
				rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: brec.ID, StartedAt: arec.StartedAt, UsageBeforeUsec: usagePtr(brec), UsageAfterUsec: usagePtr(arec), Status: Rewound, Cpuset: brec.Cpuset})
				continue
			}
			total := float64(d) / 1e6
			rep.Containers = append(rep.Containers, ContainerCost{Name: name, ID: brec.ID, StartedAt: arec.StartedAt, UsageBeforeUsec: usagePtr(brec), UsageAfterUsec: usagePtr(arec), DeltaSeconds: &total, Status: OK, Cpuset: brec.Cpuset})
		}
	}
	for i := range rep.Containers {
		c := &rep.Containers[i]
		if b, ok := firstByName(before, c.Name); ok {
			c.EpochBefore = b.EpochS
			c.Service = b.Service
			c.Tier = b.Tier
		}
		if a, ok := firstByName(after, c.Name); ok {
			c.EpochAfter = a.EpochS
			c.IDAfter = a.ID
		}
	}
	// Counters are ok only when every container we can name contributed an ok delta. A denominator makes
	// the per-message claim available; availability of the per-message figure is about the counters.
	bad := false
	sum := 0.0
	for _, c := range rep.Containers {
		if c.Status != OK {
			bad = true
		} else {
			sum += *c.DeltaSeconds
		}
	}
	rep.AllOK = len(rep.Containers) > 0 && !bad
	if rep.AllOK {
		rep.CPUSecondsByTier = map[string]float64{}
		for _, c := range rep.Containers {
			tier := c.Tier
			if tier == "" {
				tier = "unclassified"
			}
			rep.CPUSecondsByTier[tier] += *c.DeltaSeconds
		}
		total := sum
		rep.TotalCPUSeconds = &total
		if denom != nil && denom.Present && denom.Count > 0 && denom.Definition != "" && denom.Source != "" {
			per := total / float64(denom.Count)
			rep.TotalCPUSecondsPerMessage = &per
		}
	} else {
		rep.Note = strings.TrimSpace(rep.Note + "; one or more containers are unavailable/restarted/reset at the boundaries: CPU-seconds are unmeasured for them, not zero")
	}
	return rep
}

func ordered(before, after string) bool {
	b, e1 := strconv.ParseFloat(before, 64)
	a, e2 := strconv.ParseFloat(after, 64)
	return e1 == nil && e2 == nil && a > b
}

func usagePtr(r SnapshotRecord) *int64 {
	if !r.HasUsage {
		return nil
	}
	v := r.UsageUsec
	return &v
}

func biByIDName(recs []SnapshotRecord, name string) (SnapshotRecord, bool) {
	for _, r := range recs {
		if r.Name == name {
			return r, true
		}
	}
	return SnapshotRecord{}, false
}

func firstByName(recs []SnapshotRecord, name string) (SnapshotRecord, bool) {
	return biByIDName(recs, name)
}
