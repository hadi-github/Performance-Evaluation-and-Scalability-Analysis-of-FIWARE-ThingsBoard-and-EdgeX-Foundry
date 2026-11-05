package analysis

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testImage(name string) string { return name + "@sha256:" + strings.Repeat("a", 64) }

func targetInventory(platformService string) FaultInventory {
	image := testImage("example/" + platformService)
	return FaultInventory{
		SchemaVersion: FaultTargetSchemaVersion,
		Project:       "bench",
		Services: []FaultInventoryService{{
			Project: "bench", Service: platformService, Replicas: 2, Image: image,
			Containers: []FaultInventoryContainer{
				{Project: "bench", Service: platformService, ContainerID: "container-0", Image: image, Replica: 0, StartedAtMS: 1000, Healthy: true, Labels: map[string]string{"app": platformService}},
				{Project: "bench", Service: platformService, ContainerID: "container-1", Image: image, Replica: 1, StartedAtMS: 1001, Healthy: true, Labels: map[string]string{"app": platformService}},
			},
		}},
	}
}

func targetRequest(platform, service string) FaultTargetRequest {
	started := 1000.0
	return FaultTargetRequest{Platform: platform, Project: "bench", Service: service, Replica: 0, Replicas: 2, ContainerID: "container-0", Image: testImage("example/" + service), StartedAtMS: &started, Labels: map[string]string{"app": service}, DryRun: true}
}

func TestFaultTargetAllowlistedPlatformsResolveExactlyOne(t *testing.T) {
	for _, tc := range []struct{ platform, service string }{
		{"thingsboard", "tb-http-transport"},
		{"fiware", "iot-agent"},
		{"edgex", "device-rest"},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			res := ResolveFaultTarget(targetInventory(tc.service), targetRequest(tc.platform, tc.service))
			if !res.Eligible || res.Target == nil {
				t.Fatalf("allowlisted target refused: %+v", res)
			}
			got := res.Target
			if got.Platform != tc.platform || got.Service != tc.service || got.ContainerID != "container-0" || got.Replica != 0 || got.Replicas != 2 || got.StartedAtMS != 1000 {
				t.Fatalf("unexpected identity: %+v", got)
			}
			if !res.DryRun || len(res.RefusalReasons) != 0 {
				t.Fatalf("dry-run/refusal mismatch: %+v", res)
			}
			a, _ := json.Marshal(res)
			b, _ := json.Marshal(res)
			if !bytes.Equal(a, b) {
				t.Fatal("fault target JSON is not deterministic")
			}
		})
	}
}

func TestFaultTargetRefusals(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FaultInventory, *FaultTargetRequest)
		want   string
	}{
		{"wrong project", func(_ *FaultInventory, r *FaultTargetRequest) { r.Project = "other" }, "does not match requested project"},
		{"stateful", func(inv *FaultInventory, r *FaultTargetRequest) {
			r.Service = "postgres"
			inv.Services[0].Service = "postgres"
			for i := range inv.Services[0].Containers {
				inv.Services[0].Containers[i].Service = "postgres"
			}
		}, "not the allowlisted"},
		{"haproxy", func(inv *FaultInventory, r *FaultTargetRequest) {
			r.Service = "haproxy"
			inv.Services[0].Service = "haproxy"
			for i := range inv.Services[0].Containers {
				inv.Services[0].Containers[i].Service = "haproxy"
			}
		}, "not the allowlisted"},
		{"monitoring", func(inv *FaultInventory, r *FaultTargetRequest) {
			r.Service = "prometheus"
			inv.Services[0].Service = "prometheus"
			for i := range inv.Services[0].Containers {
				inv.Services[0].Containers[i].Service = "prometheus"
			}
		}, "not the allowlisted"},
		{"ambiguous", func(inv *FaultInventory, _ *FaultTargetRequest) {
			inv.Services[0].Containers = append(inv.Services[0].Containers, FaultInventoryContainer{Project: "bench", Service: inv.Services[0].Service, ContainerID: "container-duplicate", Image: testImage("example/dup"), Replica: 0, StartedAtMS: 1002, Healthy: true, Labels: map[string]string{}})
		}, "ambiguous target"},
		{"missing", func(inv *FaultInventory, r *FaultTargetRequest) {
			r.Replica = 1
			inv.Services[0].Containers = inv.Services[0].Containers[:1]
		}, "no inventory container matches"},
		{"duplicate id", func(inv *FaultInventory, _ *FaultTargetRequest) {
			inv.Services[0].Containers[1].ContainerID = "container-0"
		}, "duplicate container id"},
		{"unexpected id", func(_ *FaultInventory, r *FaultTargetRequest) { r.ContainerID = "unexpected" }, "unexpected container id"},
		{"N invalid", func(_ *FaultInventory, r *FaultTargetRequest) { r.Replicas = 0 }, "N is invalid"},
		{"replica outside N", func(_ *FaultInventory, r *FaultTargetRequest) { r.Replica = 2 }, "outside declared N"},
		{"not dry run", func(_ *FaultInventory, r *FaultTargetRequest) { r.DryRun = false }, "dry-run resolution is mandatory"},
		{"cross-project container", func(inv *FaultInventory, _ *FaultTargetRequest) { inv.Services[0].Containers[0].Project = "other" }, "belongs to project"},
		{"unhealthy", func(inv *FaultInventory, _ *FaultTargetRequest) { inv.Services[0].Containers[0].Healthy = false }, "not healthy"},
		{"missing started", func(inv *FaultInventory, _ *FaultTargetRequest) { inv.Services[0].Containers[0].StartedAtMS = 0 }, "started_at_ms"},
		{"label mismatch", func(_ *FaultInventory, r *FaultTargetRequest) { r.Labels["missing"] = "true" }, "expected label"},
		{"image mismatch", func(_ *FaultInventory, r *FaultTargetRequest) { r.Image = testImage("other") }, "image mismatch"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := targetInventory("iot-agent")
			req := targetRequest("fiware", "iot-agent")
			tc.mutate(&inv, &req)
			res := ResolveFaultTarget(inv, req)
			if res.Eligible || res.Target != nil {
				t.Fatalf("refused target reported eligible: %+v", res)
			}
			found := false
			for _, reason := range res.RefusalReasons {
				if strings.Contains(reason, tc.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected refusal containing %q, got %v", tc.want, res.RefusalReasons)
			}
		})
	}
}

func validFaultEvent(replicas int) FaultEvent {
	service := "iot-agent"
	stopCompletion, grace := 1010.0, 30.0
	unavailableStart, unavailableEnd := 1010.0, 1030.0
	lbRemoval := 1005.0
	restart, rejoin := 1040.0, 1045.0
	return FaultEvent{
		SchemaVersion: FaultEventSchemaVersion, RunID: "RUN-fault", ProtocolSHA256: strings.Repeat("7", 64),
		Platform: "fiware", Project: "bench", Service: service, ContainerID: "container-0", ContainerImage: testImage("example/iot-agent"),
		Replica: 0, Replicas: replicas, ServiceReplicas: map[string]int{service: replicas},
		StopRequestMS: 1000, StopCompletionMS: &stopCompletion, Signal: "SIGTERM", GraceMS: &grace,
		ObservedUnavailabilityStartMS: &unavailableStart, ObservedUnavailabilityEndMS: &unavailableEnd,
		LBRemovalMS: &lbRemoval, RestartRequestMS: &restart, HealthyRejoinMS: &rejoin,
		UnexpectedTransitions:        []FaultTransition{},
		OfferedRateMessagesPerSecond: 100, WindowStartMS: 900, WindowEndMS: 1100,
		ClockDomain: "synthetic:shared-epoch-ms", ClockSource: "synthetic:operator fixture",
		EventSource: FaultEventSourceOperatorManual, DryRun: false,
	}
}

func TestFaultEventQualifiedStopRejoinAndN1Outage(t *testing.T) {
	red := ReduceFaultEvent(validFaultEvent(2))
	if red.Status != FaultQualified || !red.Qualified {
		t.Fatalf("valid event did not qualify: %+v", red)
	}
	if red.Transient == nil || !red.Transient.StopObserved || !red.Transient.HealthyRejoinObserved {
		t.Fatalf("transient stop/rejoin not separated: %+v", red.Transient)
	}
	if red.Transient.StopDurationMS == nil || *red.Transient.StopDurationMS != 10 {
		t.Fatalf("stop duration not hand-computable: %+v", red.Transient.StopDurationMS)
	}
	if red.Transient.UnavailabilityDurationMS == nil || *red.Transient.UnavailabilityDurationMS != 20 {
		t.Fatalf("unavailability duration: %+v", red.Transient.UnavailabilityDurationMS)
	}
	if red.Transient.RejoinAfterRestartMS == nil || *red.Transient.RejoinAfterRestartMS != 5 {
		t.Fatalf("rejoin-after-restart duration: %+v", red.Transient.RejoinAfterRestartMS)
	}
	top := red.DegradedTopology
	if top == nil || top.Classification != "replica_degradation" || top.RemainingReplicas != 1 || top.DegradationMeasured || top.InferredFromTransientWindow {
		t.Fatalf("degraded topology reference wrong: %+v", top)
	}

	one := ReduceFaultEvent(validFaultEvent(1))
	if one.Status != FaultQualified || one.DegradedTopology.Classification != "N=1_service_outage" || one.DegradedTopology.RemainingReplicas != 0 {
		t.Fatalf("N=1 outage not labelled: %+v", one.DegradedTopology)
	}
}

func TestFaultEventUnexpectedTransitionInvalid(t *testing.T) {
	ev := validFaultEvent(2)
	ev.UnexpectedTransitions = []FaultTransition{{AtMS: 1025, ContainerID: "container-1", Kind: "restart"}}
	red := ReduceFaultEvent(ev)
	if red.Status != FaultEventInvalid || red.Qualified || !red.UnexpectedExtraTransitions {
		t.Fatalf("extra transition was not invalid/visible: %+v", red)
	}
}

func TestFaultEventDryRunCannotQualify(t *testing.T) {
	ev := validFaultEvent(2)
	ev.DryRun = true
	red := ReduceFaultEvent(ev)
	if red.Status != FaultEventDryRun || red.Qualified {
		t.Fatalf("dry run qualified a fault: %+v", red)
	}
}

func TestFaultEventOrderingViolation(t *testing.T) {
	ev := validFaultEvent(2)
	bad := 990.0
	ev.StopCompletionMS = &bad
	red := ReduceFaultEvent(ev)
	if red.Status != FaultEventInvalid || red.Qualified {
		t.Fatalf("out-of-order event accepted: %+v", red)
	}
	ev = validFaultEvent(2)
	ev.ObservedUnavailabilityStartMS = nil
	ev.ObservedUnavailabilityEndMS = nil
	bad = 1005.0
	ev.StopCompletionMS = &bad
	red = ReduceFaultEvent(ev)
	if red.Status != FaultUnavailable {
		t.Fatalf("missing unavailability start should be unavailable: %+v", red)
	}
}

func TestFaultEventMissingStagesAreNotSuccess(t *testing.T) {
	ev := validFaultEvent(2)
	ev.StopCompletionMS = nil
	red := ReduceFaultEvent(ev)
	if red.Status != FaultUnavailable || red.Qualified {
		t.Fatalf("missing stop completion became success: %+v", red)
	}

	ev = validFaultEvent(2)
	ev.ObservedUnavailabilityEndMS = nil
	ev.HealthyRejoinMS = nil
	red = ReduceFaultEvent(ev)
	if red.Status != FaultRightCensored || red.Qualified {
		t.Fatalf("missing rejoin was not right-censored: %+v", red)
	}
	if red.DegradedTopology == nil || red.DegradedTopology.InferredFromTransientWindow {
		t.Fatalf("censored event lost its declared topology reference: %+v", red.DegradedTopology)
	}

	ev = validFaultEvent(2)
	ev.RestartRequestMS = nil
	red = ReduceFaultEvent(ev)
	if red.Status != FaultUnavailable || red.Qualified {
		t.Fatalf("missing restart request became success: %+v", red)
	}
}

func testPredicate() CapacityPredicate {
	return CapacityPredicate{Version: CapacityPredicateVersion, ID: "hand-computed", P99MaxMS: 100, AttainmentMinRatio: .99, MissingMax: 0, AchievedRateMinRatio: 1.0, MinSamples: 10, Source: "unit test explicit predicate"}
}

func testSearch() CapacitySearchPolicy {
	return CapacitySearchPolicy{Version: CapacitySearchVersion, MonotonicityAssumed: true, MinConfirmations: 1, Source: "unit test explicit search"}
}

func noFaultWindow(id string, rate float64, pass bool) CapacityWindow {
	p99 := 50.0
	if !pass {
		p99 = 150
	}
	return CapacityWindow{ID: id, RunID: "RUN-" + id, OfferedRate: rate, Fault: false, Recovery: "none", WindowStartMS: 1000, WindowEndMS: 2000, Scheduled: 100, Observed: 100, Missing: 0, P99MS: p99, AttainmentRatio: 1, AchievedRate: rate, Backlog: 0, Samples: 100}
}

func faultWindow(id string, rate float64, recovery string, recoveredAt *float64) CapacityWindow {
	w := noFaultWindow(id, rate, true)
	w.Fault = true
	w.Recovery = recovery
	w.RecoveredAtMS = recoveredAt
	return w
}

func TestCapacityBracketedHandComputed(t *testing.T) {
	in := CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{
		noFaultWindow("w10", 10, true), noFaultWindow("w20a", 20, true), noFaultWindow("w20b", 20, true), noFaultWindow("w40a", 40, false), noFaultWindow("w40b", 40, false),
	}}
	r := ReduceCapacity(in)
	if r.Status != CapacityBracketed || r.LowerBound == nil || *r.LowerBound != 20 || r.UpperBound == nil || *r.UpperBound != 40 {
		t.Fatalf("bracket arithmetic wrong: %+v", r)
	}
	if len(r.Confirmations) != 3 {
		t.Fatalf("confirmation counts missing: %+v", r.Confirmations)
	}
	for _, c := range r.Confirmations {
		if c.Rate == 20 && (c.Pass != 2 || c.Fail != 0) {
			t.Fatalf("rate 20 confirmation wrong: %+v", c)
		}
		if c.Rate == 40 && (c.Fail != 2 || c.Pass != 0) {
			t.Fatalf("rate 40 confirmation wrong: %+v", c)
		}
	}
	if len(r.BoundEvidence) != 2 || r.BoundEvidence[0].Bound != "lower" || r.BoundEvidence[1].Bound != "upper" {
		t.Fatalf("bound evidence wrong: %+v", r.BoundEvidence)
	}
}

func TestCapacityAllPassLowerBound(t *testing.T) {
	r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{
		noFaultWindow("w10", 10, true), noFaultWindow("w20", 20, true),
	}})
	if r.Status != CapacityLowerBoundOnly || r.LowerBound == nil || *r.LowerBound != 20 || r.UpperBound != nil {
		t.Fatalf("all-pass lower bound wrong: %+v", r)
	}
	if !strings.Contains(r.StopReason, "no valid failing point") {
		t.Fatalf("stop reason does not explain missing upper point: %q", r.StopReason)
	}
}

func TestCapacityInvalidUpperCannotBracket(t *testing.T) {
	bad := noFaultWindow("w40", 40, false)
	bad.Samples = 50 // does not cover the observed cohort
	r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{
		noFaultWindow("w10", 10, true), noFaultWindow("w20", 20, true), bad,
	}})
	if r.Status != CapacityLowerBoundOnly || r.LowerBound == nil || *r.LowerBound != 20 || r.UpperBound != nil {
		t.Fatalf("invalid failing point established an upper bound: %+v", r)
	}
	if r.Windows[2].Validity != "invalid" || r.Windows[2].Exact {
		t.Fatalf("invalid upper point not marked invalid/inexact: %+v", r.Windows[2])
	}
}

func TestCapacityContradictoryRepeatVisible(t *testing.T) {
	r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{
		noFaultWindow("w10", 10, true), noFaultWindow("w20pass", 20, true), noFaultWindow("w20fail", 20, false), noFaultWindow("w40", 40, false),
	}})
	if len(r.Contradictions) == 0 || !strings.Contains(r.Contradictions[0], "rate 20") {
		t.Fatalf("contradictory repetitions not visible: %+v", r.Contradictions)
	}
	if r.Status != CapacityBracketed || *r.LowerBound != 10 || *r.UpperBound != 40 {
		t.Fatalf("contradictory rate entered a bound: %+v", r)
	}
}

func TestCapacityNoPassingRateAndInvalidOnly(t *testing.T) {
	r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{noFaultWindow("w10", 10, false)}})
	if r.Status != CapacityNoPassingRate || r.LowerBound != nil || r.UpperBound != nil {
		t.Fatalf("no-pass result wrong: %+v", r)
	}
	bad := noFaultWindow("w10", 10, true)
	bad.Scheduled, bad.Observed, bad.Missing, bad.Samples = 0, 0, 0, 0
	r = ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{bad}})
	if r.Status != CapacityUnresolved || r.LowerBound != nil {
		t.Fatalf("zero denominator established a bound: %+v", r)
	}
}

func TestCapacityRecoveryRightCensoring(t *testing.T) {
	recovered := 1500.0
	r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{
		faultWindow("low-fault", 5, "not_recovered", nil),
		noFaultWindow("w10", 10, true),
		faultWindow("failover", 20, "failover", nil),
		faultWindow("rejoin", 30, "rejoin", &recovered),
		faultWindow("nonrecovery", 40, "not_recovered", nil),
	}})
	if r.Recovery.Status != "right_censored" || r.Recovery.RightCensored != 2 || r.Recovery.FailoverObserved != 1 || r.Recovery.RejoinObserved != 1 {
		t.Fatalf("recovery separation wrong: %+v", r.Recovery)
	}
	if len(r.Recovery.CensoredReferences) != 2 || r.Recovery.CensoredReferences[0].Exact {
		t.Fatalf("censored reference incorrectly exact: %+v", r.Recovery.CensoredReferences)
	}
	if r.Status != CapacityLowerBoundOnly || r.LowerBound == nil || *r.LowerBound != 10 {
		t.Fatalf("fault/recovery windows entered capacity bounds: %+v", r)
	}
	for _, w := range r.Windows {
		if w.Fault && w.UsedForCapacity {
			t.Fatalf("fault window used for capacity: %+v", w)
		}
	}
}

func TestCapacityInsufficientSamplesAndExplicitViolation(t *testing.T) {
	bad := faultWindow("failover-low", 20, "failover", nil)
	bad.Samples = 5
	r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{noFaultWindow("w10", 10, true), bad}})
	if r.Recovery.Status != "invalid" || r.Recovery.Invalid != 1 {
		t.Fatalf("insufficient samples not invalid: %+v", r.Recovery)
	}
	pass := noFaultWindow("pass", 10, true)
	fail := noFaultWindow("fail", 20, false)
	r = ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{pass, fail}})
	if r.Windows[0].ObservedViolation || !r.Windows[1].ObservedViolation {
		t.Fatalf("observed violation not explicit: %+v", r.Windows)
	}
	mixed := noFaultWindow("mixed", 10, true)
	mixed.Recovery = "failover"
	r = ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{mixed}})
	if r.Windows[0].Validity != "invalid" || r.Status != CapacityUnresolved {
		t.Fatalf("fault/no-fault recovery separation not enforced: %+v", r)
	}
}

func validEvidenceDocument(t *testing.T) []byte {
	t.Helper()
	start := 1000.0
	inv := targetInventory("iot-agent")
	req := targetRequest("fiware", "iot-agent")
	event := validFaultEvent(2)
	capacity := CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: []CapacityWindow{
		noFaultWindow("w10", 10, true), noFaultWindow("w20", 20, true), noFaultWindow("w40", 40, false),
		faultWindow("rejoin", 30, "rejoin", &start),
	}}
	ev := FaultCapacityEvidence{SchemaVersion: FaultTargetSchemaVersion, Synthetic: true, Inventory: &inv, Target: &req, Event: &event, Capacity: &capacity}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFaultCapacityEvidenceStrictDecode(t *testing.T) {
	b := validEvidenceDocument(t)
	ev, err := DecodeFaultCapacityEvidence(b)
	if err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	if ev.Inventory == nil || ev.Target == nil || ev.Event == nil || ev.Capacity == nil {
		t.Fatalf("nested sections not decoded: %+v", ev)
	}
	report, err := ReduceFaultCapacityEvidence(ev)
	if err != nil {
		t.Fatal(err)
	}
	if report.Target == nil || !report.Target.Eligible || report.Event == nil || report.Event.Status != FaultQualified || report.Capacity == nil {
		t.Fatalf("evidence reduction wrong: %+v", report)
	}

	var top map[string]any
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	top["unknown"] = true
	withUnknown, _ := json.Marshal(top)
	if _, err := DecodeFaultCapacityEvidence(withUnknown); err == nil || !strings.Contains(err.Error(), "unsupported field") {
		t.Fatalf("unknown top-level field accepted: %v", err)
	}

	var event map[string]any
	_ = json.Unmarshal(b, &event)
	evMap := event["event"].(map[string]any)
	evMap["stop_completion_ms"] = nil
	nullEvent, _ := json.Marshal(event)
	if _, err := DecodeFaultCapacityEvidence(nullEvent); err == nil || !strings.Contains(err.Error(), "must not be null") {
		t.Fatalf("explicit null accepted: %v", err)
	}

	evMap = event["event"].(map[string]any)
	evMap["stop_completion_ms"] = 1010.0
	evMap["extra"] = 1
	unknownEvent, _ := json.Marshal(event)
	if _, err := DecodeFaultCapacityEvidence(unknownEvent); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown nested field accepted: %v", err)
	}

	var capacity map[string]any
	_ = json.Unmarshal(b, &capacity)
	win := capacity["capacity"].(map[string]any)["windows"].([]any)[0].(map[string]any)
	delete(win, "samples")
	missingWindow, _ := json.Marshal(capacity)
	if _, err := DecodeFaultCapacityEvidence(missingWindow); err == nil || !strings.Contains(err.Error(), "samples") {
		t.Fatalf("missing required window field accepted: %v", err)
	}

	dup := append(append([]byte{}, b[:len(b)-1]...), []byte(`,"synthetic":true}`)...)
	if _, err := DecodeFaultCapacityEvidence(dup); err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("duplicate key accepted: %v", err)
	}
	if _, err := DecodeFaultCapacityEvidence(append(b, []byte(`{}`)...)); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing JSON accepted: %v", err)
	}
}

func TestFaultCapacityDeterministicSerializationAndWrite(t *testing.T) {
	ev, err := DecodeFaultCapacityEvidence(validEvidenceDocument(t))
	if err != nil {
		t.Fatal(err)
	}
	report, err := ReduceFaultCapacityEvidence(ev)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.MarshalIndent(report, "", "  ")
	b, _ := json.MarshalIndent(report, "", "  ")
	if !bytes.Equal(a, b) {
		t.Fatal("report JSON is not deterministic")
	}
	dir := t.TempDir()
	out1 := filepath.Join(dir, "one")
	out2 := filepath.Join(dir, "two")
	if err := WriteFaultCapacity(out1, report, nil); err != nil {
		t.Fatal(err)
	}
	if err := WriteFaultCapacity(out2, report, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fault-capacity.json", "fault-capacity.md", "SHA256SUMS"} {
		x, err := os.ReadFile(filepath.Join(out1, name))
		if err != nil {
			t.Fatal(err)
		}
		y, err := os.ReadFile(filepath.Join(out2, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(x, y) {
			t.Fatalf("%s differs across runs", name)
		}
	}
	md, _ := os.ReadFile(filepath.Join(out1, "fault-capacity.md"))
	if !strings.Contains(string(md), "Campaign capacity: **NOT CLAIMED**") || !strings.Contains(string(md), "Degraded topology reference") {
		t.Fatalf("markdown lost required limitation: %s", md)
	}
	if err := WriteFaultCapacity(out1, report, nil); err == nil {
		t.Fatal("existing output directory was reused")
	}
}

func permutationsOfIndices(n int) [][]int {
	var out [][]int
	var rec func(prefix []int, used []bool)
	rec = func(prefix []int, used []bool) {
		if len(prefix) == n {
			out = append(out, append([]int{}, prefix...))
			return
		}
		for i := 0; i < n; i++ {
			if used[i] {
				continue
			}
			used[i] = true
			rec(append(prefix, i), used)
			used[i] = false
		}
	}
	rec([]int{}, make([]bool, n))
	return out
}

// TestCapacityPermutationInvariantContradiction is the review probe:
// [pass20, fail10, fail40] must produce the same non-bracketed result for every
// caller ordering because a passing rate sits above a failing rate.
func TestCapacityPermutationInvariantContradiction(t *testing.T) {
	base := []CapacityWindow{
		noFaultWindow("pass20", 20, true),
		noFaultWindow("fail10", 10, false),
		noFaultWindow("fail40", 40, false),
	}
	for _, perm := range permutationsOfIndices(len(base)) {
		windows := make([]CapacityWindow, len(base))
		for i, idx := range perm {
			windows[i] = base[idx]
		}
		r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: windows})
		if r.Status != CapacityUnresolved {
			t.Fatalf("permutation %v: expected unresolved non-bracket, got %s (lower=%v upper=%v)", perm, r.Status, r.LowerBound, r.UpperBound)
		}
		if r.LowerBound != nil || r.UpperBound != nil {
			t.Fatalf("permutation %v: contradiction established a bound: lower=%v upper=%v", perm, r.LowerBound, r.UpperBound)
		}
		if len(r.Contradictions) != 1 || !strings.Contains(r.Contradictions[0], "passing rate 20 lies above failing rate 10") {
			t.Fatalf("permutation %v: contradiction not visible: %v", perm, r.Contradictions)
		}
	}
}

// Sorted [pass10, fail20, pass30, fail40] must not yield bracketed and must not
// emit an unsupported upper bound. Every permutation must agree.
func TestCapacityPassAboveFailBlocksBracket(t *testing.T) {
	base := []CapacityWindow{
		noFaultWindow("pass10", 10, true),
		noFaultWindow("fail20", 20, false),
		noFaultWindow("pass30", 30, true),
		noFaultWindow("fail40", 40, false),
	}
	for _, perm := range permutationsOfIndices(len(base)) {
		windows := make([]CapacityWindow, len(base))
		for i, idx := range perm {
			windows[i] = base[idx]
		}
		r := ReduceCapacity(CapacityInput{SchemaVersion: CapacitySchemaVersion, Predicate: testPredicate(), Search: testSearch(), Windows: windows})
		if r.Status == CapacityBracketed {
			t.Fatalf("permutation %v: pass-above-fail contradiction produced a clean bracket: %+v", perm, r)
		}
		if r.Status != CapacityLowerBoundOnly || r.LowerBound == nil || *r.LowerBound != 10 || r.UpperBound != nil {
			t.Fatalf("permutation %v: expected lower-bound-only at 10 with no upper, got status=%s lower=%v upper=%v", perm, r.Status, r.LowerBound, r.UpperBound)
		}
		found := false
		for _, c := range r.Contradictions {
			if strings.Contains(c, "passing rate 30 lies above failing rate 20") {
				found = true
			}
		}
		if !found {
			t.Fatalf("permutation %v: pass-above-fail contradiction not visible: %v", perm, r.Contradictions)
		}
	}
}

func TestFaultTargetNestedServiceSpoofRefused(t *testing.T) {
	image := testImage("example/iot-agent")
	inv := FaultInventory{
		SchemaVersion: FaultTargetSchemaVersion, Project: "bench",
		Services: []FaultInventoryService{
			{
				Project: "bench", Service: "postgres", Replicas: 1, Image: testImage("example/postgres"),
				Containers: []FaultInventoryContainer{{
					Project: "bench", Service: "iot-agent", ContainerID: "container-0", Image: image,
					Replica: 0, StartedAtMS: 1000, Healthy: true, Labels: map[string]string{"app": "iot-agent"},
				}},
			},
			{
				Project: "bench", Service: "iot-agent", Replicas: 2, Image: image,
				Containers: []FaultInventoryContainer{{
					Project: "bench", Service: "iot-agent", ContainerID: "container-1", Image: image,
					Replica: 1, StartedAtMS: 1001, Healthy: true, Labels: map[string]string{"app": "iot-agent"},
				}},
			},
		},
	}
	res := ResolveFaultTarget(inv, targetRequest("fiware", "iot-agent"))
	if res.Eligible || res.Target != nil {
		t.Fatalf("nested-service spoof emitted a target: %+v", res)
	}
	joined := strings.Join(res.RefusalReasons, "\n")
	if !strings.Contains(joined, "declares compose_service") {
		t.Fatalf("spoofed container service not refused: %v", res.RefusalReasons)
	}
	if !strings.Contains(joined, "missing declared replica index 0") {
		t.Fatalf("incomplete declared replica set not refused: %v", res.RefusalReasons)
	}
}

func TestFaultTargetEmptyEnclosingProjectRefused(t *testing.T) {
	// A valid allowlisted iot-agent inventory plus an unrelated service whose
	// enclosing project is empty. Project identity is not optional: both a
	// container that also omits the project and a container that declares the
	// requested project must be refused, with no emitted target.
	tests := []struct {
		name          string
		containerProj string
		wantReason    string
	}{
		{"matching empty container project", "", "declares project"},
		{"different container project", "bench", "belongs to project"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := targetInventory("iot-agent")
			inv.Services = append(inv.Services, FaultInventoryService{
				Project: "", Service: "postgres", Replicas: 1, Image: testImage("example/postgres"),
				Containers: []FaultInventoryContainer{{
					Project: tc.containerProj, Service: "postgres", ContainerID: "postgres-0",
					Image: testImage("example/postgres"), Replica: 0, StartedAtMS: 2000, Healthy: true,
					Labels: map[string]string{"app": "postgres"},
				}},
			})
			res := ResolveFaultTarget(inv, targetRequest("fiware", "iot-agent"))
			if res.Eligible || res.Target != nil {
				t.Fatalf("empty enclosing project did not refuse: %+v", res)
			}
			if len(res.RefusalReasons) == 0 {
				t.Fatal("empty enclosing project refusal recorded no reason")
			}
			joined := strings.Join(res.RefusalReasons, "\n")
			if !strings.Contains(joined, tc.wantReason) {
				t.Fatalf("expected refusal containing %q, got %v", tc.wantReason, res.RefusalReasons)
			}
			b, err := json.Marshal(res)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(b, []byte(`"target"`)) {
				t.Fatalf("refusal serialized a target: %s", b)
			}
		})
	}
}

func TestFaultTargetConsistentUnrelatedServiceAllowed(t *testing.T) {
	// A genuinely unrelated service that declares the same project and whose
	// containers agree with their enclosing service must not be overrejected.
	inv := targetInventory("iot-agent")
	inv.Services = append(inv.Services, FaultInventoryService{
		Project: "bench", Service: "postgres", Replicas: 1, Image: testImage("example/postgres"),
		Containers: []FaultInventoryContainer{{
			Project: "bench", Service: "postgres", ContainerID: "postgres-0",
			Image: testImage("example/postgres"), Replica: 0, StartedAtMS: 2000, Healthy: true,
			Labels: map[string]string{"app": "postgres"},
		}},
	})
	res := ResolveFaultTarget(inv, targetRequest("fiware", "iot-agent"))
	if !res.Eligible || res.Target == nil {
		t.Fatalf("consistent unrelated service caused overrejection: %+v", res)
	}
	if res.Target.Service != "iot-agent" || res.Target.ContainerID != "container-0" {
		t.Fatalf("unexpected target: %+v", res.Target)
	}
}

func TestFaultTargetReplicaSetMustBeCompleteAndContiguous(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FaultInventory)
		want   string
	}{
		{"duplicate replica", func(inv *FaultInventory) { inv.Services[0].Containers[1].Replica = 0 }, "duplicate replica index"},
		{"missing replica", func(inv *FaultInventory) { inv.Services[0].Containers = inv.Services[0].Containers[:1] }, "missing declared replica index 1"},
		{"negative replica", func(inv *FaultInventory) { inv.Services[0].Containers[0].Replica = -1 }, "outside declared N"},
		{"replica at N", func(inv *FaultInventory) { inv.Services[0].Containers[1].Replica = 2 }, "outside declared N"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := targetInventory("iot-agent")
			tc.mutate(&inv)
			res := ResolveFaultTarget(inv, targetRequest("fiware", "iot-agent"))
			if res.Eligible || res.Target != nil {
				t.Fatalf("incomplete replica set emitted a target: %+v", res)
			}
			found := false
			for _, reason := range res.RefusalReasons {
				if strings.Contains(reason, tc.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected refusal containing %q, got %v", tc.want, res.RefusalReasons)
			}
		})
	}
}

func TestFaultEventEverySuppliedStageMustBeInsideWindow(t *testing.T) {
	setters := map[string]func(*FaultEvent, float64){
		"stop_completion_ms":               func(ev *FaultEvent, v float64) { ev.StopCompletionMS = &v },
		"observed_unavailability_start_ms": func(ev *FaultEvent, v float64) { ev.ObservedUnavailabilityStartMS = &v },
		"observed_unavailability_end_ms":   func(ev *FaultEvent, v float64) { ev.ObservedUnavailabilityEndMS = &v },
		"lb_removal_ms":                    func(ev *FaultEvent, v float64) { ev.LBRemovalMS = &v },
		"restart_request_ms":               func(ev *FaultEvent, v float64) { ev.RestartRequestMS = &v },
		"healthy_rejoin_ms":                func(ev *FaultEvent, v float64) { ev.HealthyRejoinMS = &v },
	}
	for name, set := range setters {
		setter := set
		field := name
		for _, tc := range []struct {
			name string
			at   float64
		}{
			{"before", 890},
			{"after", 1105},
		} {
			tc := tc
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				ev := validFaultEvent(2)
				setter(&ev, tc.at)
				red := ReduceFaultEvent(ev)
				if red.Status != FaultEventInvalid || red.Qualified {
					t.Fatalf("%s outside window did not invalidate: %+v", field, red)
				}
				if red.Transient != nil {
					t.Fatalf("%s outside window produced a derived transient observation: %+v", field, red.Transient)
				}
				found := false
				for _, reason := range red.Reasons {
					if strings.Contains(reason, field+" lies outside the declared fault window") {
						found = true
					}
				}
				if !found {
					t.Fatalf("%s outside window not individually reported: %v", field, red.Reasons)
				}
			})
		}
	}
}

func TestFaultEventGraceIsNonNegativeDuration(t *testing.T) {
	zero := 0.0
	ev := validFaultEvent(2)
	ev.GraceMS = &zero
	red := ReduceFaultEvent(ev)
	if red.Status != FaultQualified || !red.Qualified {
		t.Fatalf("zero grace did not qualify: %+v", red)
	}
	if red.Transient == nil || red.Transient.GraceMS == nil || *red.Transient.GraceMS != 0 {
		t.Fatalf("zero grace not preserved as a duration: %+v", red.Transient)
	}

	negative := -1.0
	ev = validFaultEvent(2)
	ev.GraceMS = &negative
	red = ReduceFaultEvent(ev)
	if red.Status != FaultEventInvalid || red.Qualified {
		t.Fatalf("negative grace accepted: %+v", red)
	}

	notFinite := math.NaN()
	ev = validFaultEvent(2)
	ev.GraceMS = &notFinite
	red = ReduceFaultEvent(ev)
	if red.Status != FaultEventInvalid || red.Qualified {
		t.Fatalf("non-finite grace accepted: %+v", red)
	}
}

func TestFaultEventRequestOnlyDoesNotObserveStop(t *testing.T) {
	ev := validFaultEvent(2)
	ev.StopCompletionMS = nil
	red := ReduceFaultEvent(ev)
	if red.Status != FaultUnavailable || red.Qualified {
		t.Fatalf("request-only event did not stay unavailable: %+v", red)
	}
	if red.Transient == nil {
		t.Fatal("request-only event lost its separate transient request evidence")
	}
	if red.Transient.StopObserved {
		t.Fatalf("request-only event asserted an observed stop: %+v", red.Transient)
	}
	if !red.Transient.StopRequested {
		t.Fatalf("request-only event lost its explicit stop request: %+v", red.Transient)
	}
	if red.Transient.StopDurationMS != nil {
		t.Fatalf("request-only event fabricated a stop duration: %+v", red.Transient)
	}
}

func TestFaultTargetDecodeStrict(t *testing.T) {
	inv := targetInventory("iot-agent")
	req := targetRequest("fiware", "iot-agent")
	b, _ := json.Marshal(inv)
	if _, err := DecodeFaultInventory(b); err != nil {
		t.Fatalf("valid inventory rejected: %v", err)
	}
	q, _ := json.Marshal(req)
	if _, err := DecodeFaultTargetRequest(q); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["services"].([]any)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)["started_at_ms"] = nil
	n, _ := json.Marshal(m)
	if _, err := DecodeFaultInventory(n); err == nil {
		t.Fatal("null inventory field accepted")
	}
	m["services"].([]any)[0].(map[string]any)["containers"].([]any)[0].(map[string]any)["started_at_ms"] = 1000
	m["extra"] = 1
	n, _ = json.Marshal(m)
	if _, err := DecodeFaultInventory(n); err == nil {
		t.Fatal("unknown inventory field accepted")
	}
}
