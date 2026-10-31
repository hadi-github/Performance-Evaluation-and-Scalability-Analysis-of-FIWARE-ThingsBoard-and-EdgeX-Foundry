package cpubound

import (
	"math"
	"strings"
	"testing"
)

func snap(epoch, name, id, usage, started, cpuset string) string {
	return strings.Join([]string{epoch, name, id, usage, started, cpuset}, "\t") + "\n"
}

func parse(t *testing.T, s string) []SnapshotRecord {
	t.Helper()
	r, err := Parse(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func byName(t *testing.T, rep *Report, name string) ContainerCost {
	t.Helper()
	for _, c := range rep.Containers {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("container %s not in report %v", name, rep.Containers)
	return ContainerCost{}
}

func denom() *Denominator {
	return &Denominator{Count: 1000, Source: "test", Definition: "known-answer identities", Present: true}
}

// exact counter delta: 1,000,000 µs x 2 endpoints -> 1.0 CPU-s per container, 2.0 total.
func TestKnownCounterValuesProduceExactDelta(t *testing.T) {
	before := parse(t, snap("10.0", "app-1", "id1", "1005000", "S1", "4")+
		snap("10.0", "db-1", "id2", "500000", "S1", "5"))
	after := parse(t, snap("70.0", "app-1", "id1", "10050800", "S1", "4")+
		snap("70.0", "db-1", "id2", "1800000", "S1", "5"))
	rep := Diff(before, after, denom(), "")
	if !rep.AllOK || rep.TotalCPUSeconds == nil || math.Abs(*rep.TotalCPUSeconds-10.3458) > 1e-9 { // 9.0458 + 1.3
		t.Fatalf("total %v allOK %v", rep.TotalCPUSeconds, rep.AllOK)
	}
	c := byName(t, rep, "app-1")
	if c.DeltaSeconds == nil || math.Abs(*c.DeltaSeconds-9.0458) > 1e-9 || c.Status != OK {
		t.Fatalf("delta %+v", c)
	}
	if rep.TotalCPUSecondsPerMessage == nil || math.Abs(*rep.TotalCPUSecondsPerMessage-10.3458/1000.0) > 1e-12 {
		t.Fatalf("per-message %v", rep.TotalCPUSecondsPerMessage)
	}
	if rep.Denominator == nil || rep.Denominator.Definition == "" || rep.Denominator.Source == "" {
		t.Fatalf("denominator must be named: %+v", rep.Denominator)
	}
	// pre/post values are preserved with their units
	if c.UsageBeforeUsec == nil || *c.UsageBeforeUsec != 1005000 || c.UsageAfterUsec == nil || *c.UsageAfterUsec != 10050800 {
		t.Fatalf("+%v", c)
	}
}

func TestRestartedContainerIsUnavailableNotZero(t *testing.T) {
	before := parse(t, snap("1", "app-1", "idA", "1000", "S1", ""))
	after := parse(t, snap("61", "app-1", "idA", "1200", "S2", "4")) // same id, new StartedAt => restarted
	rep := Diff(before, after, nil, "")
	c := byName(t, rep, "app-1")
	if c.Status != Restarted || c.DeltaSeconds != nil {
		t.Fatalf("restarted must be recognised and have no delta: %+v", c)
	}
	if rep.AllOK || rep.TotalCPUSeconds != nil || rep.TotalCPUSecondsPerMessage != nil {
		t.Fatal("no aggregate CPU cost may be claimed from a restarted interval")
	}
	if rep.Denominator != nil && rep.Denominator.Present {
		// denominator recorded but cost still absent
		if rep.TotalCPUSecondsPerMessage != nil {
			t.Fatal("per-message cost must stay absent on restarted counters")
		}
	}
	if rep.Note == "" || !strings.Contains(rep.Note, "not zero") {
		t.Fatalf("note must forbid a zero value: %q", rep.Note)
	}
}

func TestRewoundCounterIsUnavailable(t *testing.T) {
	before := parse(t, snap("1", "app-1", "idA", "9000", "S1", ""))
	after := parse(t, snap("61", "app-1", "idA", "100", "S1", "")) // counter reset without restart
	rep := Diff(before, after, nil, "")
	c := byName(t, rep, "app-1")
	if c.Status != Rewound || c.DeltaSeconds != nil {
		t.Fatalf("rewound must be recognised: %+v", c)
	}
	if rep.AllOK {
		t.Fatal("rewound counters must block the aggregate claim")
	}
}

func TestReplacementIDCannotProduceCost(t *testing.T) {
	before := parse(t, snap("1", "app", "old-id", "100", "same", "4"))
	after := parse(t, snap("61", "app", "new-id", "9000", "same", "4"))
	r := Diff(before, after, denom(), "")
	c := byName(t, r, "app")
	if r.AllOK || r.TotalCPUSeconds != nil || c.Status != Restarted || c.DeltaSeconds != nil {
		t.Fatalf("replacement container must invalidate cost: %+v", r)
	}
	if c.IDAfter != "new-id" || c.EpochBefore != "1" || c.EpochAfter != "61" {
		t.Fatalf("boundary provenance missing: %+v", c)
	}
}

func TestReversedWindowAndUnnamedDenominator(t *testing.T) {
	b := parse(t, snap("61", "app", "id", "100", "S", "4"))
	a := parse(t, snap("1", "app", "id", "900", "S", "4"))
	r := Diff(b, a, denom(), "")
	if r.AllOK || r.TotalCPUSeconds != nil || byName(t, r, "app").Status != InvalidWindow {
		t.Fatal("reversed interval must be unavailable")
	}
	b[0].EpochS = "0.5"
	r = Diff(b, a, &Denominator{Count: 100, Present: true}, "")
	if !r.AllOK || r.TotalCPUSecondsPerMessage != nil {
		t.Fatal("unnamed denominator must not produce a cost claim")
	}
}

func TestInvalidSnapshotRejected(t *testing.T) {
	for name, text := range map[string]string{
		"negative":        snap("1", "app", "id", "-1", "S", "4"),
		"missing_started": snap("1", "app", "id", "1", "", "4"),
		"nan_time":        snap("NaN", "app", "id", "1", "S", "4"),
		"duplicate":       snap("1", "app", "id", "1", "S", "4") + snap("1", "app", "id2", "2", "S", "4"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(text)); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestTierTotals(t *testing.T) {
	b := parse(t, "1\tapp\ta\t1000000\tS\t4\torion\tapplication\n1\tdb\tb\t1000000\tS\t2-3\ttimescale\tsupporting\n")
	a := parse(t, "2\tapp\ta\t3000000\tS\t4\torion\tapplication\n2\tdb\tb\t4000000\tS\t2-3\ttimescale\tsupporting\n")
	r := Diff(b, a, denom(), "")
	if r.CPUSecondsByTier["application"] != 2 || r.CPUSecondsByTier["supporting"] != 3 || *r.TotalCPUSeconds != 5 {
		t.Fatalf("bad tier totals: %+v", r)
	}
}

func TestMissingContainerOrCounterIsUnavailable(t *testing.T) {
	// container disappears between boundaries
	before := parse(t, snap("1", "app-1", "idA", "1000", "S1", ""))
	after := parse(t, snap("61", "other", "idB", "5000", "S2", ""))
	rep := Diff(before, after, nil, "")
	if len(rep.Containers) != 2 {
		t.Fatalf("both sides must be reported: %+v", rep.Containers)
	}
	if c := byName(t, rep, "app-1"); c.Status != MissingAfter {
		t.Fatalf("%+v", c)
	}
	if c := byName(t, rep, "other"); c.Status != MissingBefore {
		t.Fatalf("%+v", c)
	}
	if rep.AllOK {
		t.Fatal("missing endpoints must block the aggregate cost claim")
	}
	// counter unreadable on one side ("NA")
	before = parse(t, snap("1", "app-1", "idA", "NA", "S1", ""))
	after = parse(t, snap("61", "app-1", "idA", "2000", "S1", ""))
	rep = Diff(before, after, nil, "")
	if c := byName(t, rep, "app-1"); c.Status != NoUsageBefore || c.DeltaSeconds != nil {
		t.Fatalf("%+v", c)
	}
	if rep.TotalCPUSeconds != nil {
		t.Fatal("NA counter must not manufacture a delta")
	}
}

func TestEmptyResultAndNoDenominatorOmitsCost(t *testing.T) {
	before := parse(t, snap("1", "app-1", "idA", "1000", "S1", ""))
	after := parse(t, snap("61", "app-1", "idA", "21000", "S1", ""))
	rep := Diff(before, after, &Denominator{}, "no audit available")
	if !rep.AllOK || rep.TotalCPUSecondsPerMessage != nil {
		t.Fatal("without a denominator the per-message cost must be omitted, not estimated")
	}
	if rep.TotalCPUSeconds == nil || *rep.TotalCPUSeconds != 0.020 {
		t.Fatalf("delta itself is still a measured fact: %v", rep.TotalCPUSeconds)
	}
	if before[0].HasUsage != true {
		t.Fatal("parse must flag present counters")
	}
}

func TestFreshContainerCountsAsMissingBefore(t *testing.T) {
	before := parse(t, snap("1", "app-1", "idA", "1000", "S1", ""))
	after := parse(t, snap("61", "app-1", "idA", "21000", "S1", "")+
		snap("61", "sidecar", "idC", "3000", "S1", ""))
	rep := Diff(before, after, nil, "")
	if c := byName(t, rep, "sidecar"); c.Status != MissingBefore || c.UsageBeforeUsec != nil {
		t.Fatalf("%+v", c)
	}
	if rep.AllOK {
		t.Fatal("a container seen only after the interval blocks the total claim")
	}
}
