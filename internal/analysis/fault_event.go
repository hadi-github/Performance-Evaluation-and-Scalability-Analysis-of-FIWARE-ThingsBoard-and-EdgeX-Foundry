package analysis

// M1f manual fault-event schema and reducer. One event describes ONE intended
// application replica. The reducer reports a transient stop/rejoin observation and
// a separate declared-topology reference; it never turns a transient interval into
// a capacity or degradation measurement. Missing stages are right-censored or
// unavailable, never success.

import (
	"encoding/json"
	"fmt"
	"sort"
)

const (
	FaultEventSchemaVersion        = 1
	FaultEventSourceOperatorManual = "operator/manual"
	FaultEventClockRequirement     = "explicit clock domain and source required"
)

type FaultTransition struct {
	AtMS        float64 `json:"at_ms"`
	ContainerID string  `json:"container_id"`
	Kind        string  `json:"kind"`
	Detail      string  `json:"detail,omitempty"`
}

type FaultEvent struct {
	SchemaVersion   int            `json:"schema_version"`
	RunID           string         `json:"run_id"`
	ProtocolSHA256  string         `json:"protocol_binding_sha256"`
	Platform        string         `json:"platform"`
	Project         string         `json:"project"`
	Service         string         `json:"service"`
	ContainerID     string         `json:"container_id"`
	ContainerImage  string         `json:"container_image"`
	Replica         int            `json:"replica"`
	Replicas        int            `json:"replicas"`
	ServiceReplicas map[string]int `json:"service_replicas"`

	StopRequestMS    float64  `json:"stop_request_ms"`
	StopCompletionMS *float64 `json:"stop_completion_ms,omitempty"`
	Signal           string   `json:"signal"`
	GraceMS          *float64 `json:"grace_ms,omitempty"`
	Forced           bool     `json:"forced"`

	ObservedUnavailabilityStartMS *float64 `json:"observed_unavailability_start_ms,omitempty"`
	ObservedUnavailabilityEndMS   *float64 `json:"observed_unavailability_end_ms,omitempty"`
	LBRemovalMS                   *float64 `json:"lb_removal_ms,omitempty"`
	RestartRequestMS              *float64 `json:"restart_request_ms,omitempty"`
	HealthyRejoinMS               *float64 `json:"healthy_rejoin_ms,omitempty"`

	UnexpectedTransitions []FaultTransition `json:"unexpected_transitions"`

	OfferedRateMessagesPerSecond float64 `json:"offered_rate_messages_per_second"`
	WindowStartMS                float64 `json:"window_start_ms"`
	WindowEndMS                  float64 `json:"window_end_ms"`
	ClockDomain                  string  `json:"clock_domain"`
	ClockSource                  string  `json:"clock_source"`
	EventSource                  string  `json:"event_source"`
	DryRun                       bool    `json:"dry_run"`
}

type FaultQualification string

const (
	FaultQualified     FaultQualification = "QUALIFIED"
	FaultUnavailable   FaultQualification = "UNAVAILABLE"
	FaultRightCensored FaultQualification = "RIGHT_CENSORED"
	FaultEventInvalid  FaultQualification = "INVALID"
	FaultEventDryRun   FaultQualification = "DRY_RUN"
)

// FaultTransientObservation separates each event so stop and rejoin are never
// collapsed into one boolean. Durations are present only when both endpoints were
// explicitly supplied.
type FaultTransientObservation struct {
	// StopRequested records the operator's request timestamp/evidence; StopObserved
	// records explicit stop-completion evidence. They are separate so a request-only
	// event cannot assert an observed stop.
	StopRequested               bool     `json:"stop_requested"`
	StopObserved                bool     `json:"stop_observed"`
	StopRequestMS               float64  `json:"stop_request_ms"`
	StopCompletionMS            *float64 `json:"stop_completion_ms,omitempty"`
	StopDurationMS              *float64 `json:"stop_duration_ms,omitempty"`
	Signal                      string   `json:"signal"`
	GraceMS                     *float64 `json:"grace_ms,omitempty"`
	Forced                      bool     `json:"forced"`
	LBRemovalObserved           bool     `json:"lb_removal_observed"`
	LBRemovalMS                 *float64 `json:"lb_removal_ms,omitempty"`
	UnavailabilityStartMS       *float64 `json:"observed_unavailability_start_ms,omitempty"`
	UnavailabilityEndMS         *float64 `json:"observed_unavailability_end_ms,omitempty"`
	UnavailabilityDurationMS    *float64 `json:"observed_unavailability_duration_ms,omitempty"`
	RestartRequested            bool     `json:"restart_requested"`
	RestartRequestMS            *float64 `json:"restart_request_ms,omitempty"`
	HealthyRejoinObserved       bool     `json:"healthy_rejoin_observed"`
	HealthyRejoinMS             *float64 `json:"healthy_rejoin_ms,omitempty"`
	RejoinAfterRestartMS        *float64 `json:"rejoin_after_restart_ms,omitempty"`
	RejoinAfterUnavailabilityMS *float64 `json:"rejoin_after_unavailability_ms,omitempty"`
}

// FaultTopologyReference is a declaration-based reference, not a measurement. It is
// never derived from the transient unavailability interval.
type FaultTopologyReference struct {
	Project                     string `json:"project"`
	Service                     string `json:"service"`
	Replicas                    int    `json:"replicas"`
	DownReplica                 int    `json:"down_replica"`
	RemainingReplicas           int    `json:"remaining_replicas"`
	Classification              string `json:"classification"`
	Basis                       string `json:"basis"`
	DegradationMeasured         bool   `json:"degradation_measured"`
	InferredFromTransientWindow bool   `json:"inferred_from_transient_window"`
}

type FaultReduction struct {
	SchemaVersion              int                        `json:"schema_version"`
	PolicyVersion              string                     `json:"policy_version"`
	Event                      FaultEvent                 `json:"event"`
	Status                     FaultQualification         `json:"status"`
	Qualified                  bool                       `json:"qualified"`
	DryRun                     bool                       `json:"dry_run"`
	Transient                  *FaultTransientObservation `json:"transient,omitempty"`
	DegradedTopology           *FaultTopologyReference    `json:"degraded_topology_reference,omitempty"`
	UnexpectedExtraTransitions bool                       `json:"unexpected_extra_transitions"`
	Reasons                    []string                   `json:"reasons"`
	Limitations                []string                   `json:"limitations"`
}

// ReduceFaultEvent validates and reduces one manual fault event. It does not
// schedule, execute, or observe a live fault.
func ReduceFaultEvent(ev FaultEvent) FaultReduction {
	res := FaultReduction{
		SchemaVersion: FaultEventSchemaVersion,
		PolicyVersion: FaultTargetPolicyVersion,
		Event:         ev,
		DryRun:        ev.DryRun,
		Reasons:       []string{},
		Limitations: []string{
			"Manual event evidence is an operator attestation; this reducer did not observe or execute the stop/rejoin.",
			"The degraded-topology reference is declared from N and one intended replica, not measured from the transient window.",
		},
	}
	var invalid, unavailable, censored []string
	add := func(dst *[]string, format string, args ...any) { *dst = append(*dst, fmt.Sprintf(format, args...)) }

	if ev.SchemaVersion != FaultEventSchemaVersion {
		add(&invalid, "unsupported fault event schema_version %d", ev.SchemaVersion)
	}
	if ev.RunID == "" {
		add(&invalid, "run_id is missing")
	}
	if !hexLength(ev.ProtocolSHA256, 64) {
		add(&invalid, "protocol_binding_sha256 is not a 64-character hexadecimal binding")
	}
	if ev.Platform == "" || !isAllowlistedFaultService(ev.Platform, ev.Service) {
		add(&invalid, "platform/service %q/%q is outside the versioned fault-target allowlist", ev.Platform, ev.Service)
	}
	if ev.Project == "" {
		add(&invalid, "project is missing")
	}
	if ev.ContainerID == "" {
		add(&invalid, "container_id is missing")
	}
	if ev.ContainerImage == "" {
		add(&invalid, "container_image is missing")
	}
	if ev.Replicas <= 0 {
		add(&invalid, "N is invalid: replicas must be positive")
	}
	if ev.Replica < 0 || ev.Replica >= ev.Replicas {
		add(&invalid, "replica index %d is outside N=%d", ev.Replica, ev.Replicas)
	}
	if len(ev.ServiceReplicas) == 0 {
		add(&invalid, "service_replicas is missing")
	}
	for service, n := range ev.ServiceReplicas {
		if service == "" || n <= 0 {
			add(&invalid, "service_replicas contains an empty service or non-positive replica count")
		}
	}
	if n, ok := ev.ServiceReplicas[ev.Service]; !ok || n != ev.Replicas {
		add(&invalid, "service_replicas[%q]=%d does not match N=%d", ev.Service, n, ev.Replicas)
	}
	if !finiteNumber(ev.StopRequestMS) || ev.StopRequestMS <= 0 {
		add(&invalid, "stop_request_ms is missing or invalid")
	}
	if ev.Signal != "SIGTERM" && ev.Signal != "SIGKILL" {
		add(&invalid, "signal %q is unsupported; expected SIGTERM or SIGKILL", ev.Signal)
	}
	if ev.GraceMS != nil && (!finiteNumber(*ev.GraceMS) || *ev.GraceMS < 0) {
		add(&invalid, "grace_ms is negative or non-finite")
	}
	if ev.Forced && ev.GraceMS != nil && *ev.GraceMS > 0 {
		add(&invalid, "forced stop contradicts positive grace_ms")
	}
	if !finiteNumber(ev.OfferedRateMessagesPerSecond) || ev.OfferedRateMessagesPerSecond <= 0 {
		add(&invalid, "offered_rate_messages_per_second is missing or non-positive")
	}
	if !finiteNumber(ev.WindowStartMS) || !finiteNumber(ev.WindowEndMS) || ev.WindowStartMS <= 0 || ev.WindowEndMS <= ev.WindowStartMS {
		add(&invalid, "fault window start/end is missing or non-monotonic")
	} else {
		// Every supplied stage timestamp, not only the stop request, must lie inside
		// the declared window. A stage outside it is inadmissible evidence and can
		// neither qualify the event nor support a derived recovery observation.
		if finiteNumber(ev.StopRequestMS) && (ev.StopRequestMS < ev.WindowStartMS || ev.StopRequestMS > ev.WindowEndMS) {
			add(&invalid, "stop_request_ms lies outside the declared fault window")
		}
		for name, p := range map[string]*float64{
			"stop_completion_ms":               ev.StopCompletionMS,
			"observed_unavailability_start_ms": ev.ObservedUnavailabilityStartMS,
			"observed_unavailability_end_ms":   ev.ObservedUnavailabilityEndMS,
			"lb_removal_ms":                    ev.LBRemovalMS,
			"restart_request_ms":               ev.RestartRequestMS,
			"healthy_rejoin_ms":                ev.HealthyRejoinMS,
		} {
			if p != nil && finiteNumber(*p) && *p > 0 && (*p < ev.WindowStartMS || *p > ev.WindowEndMS) {
				add(&invalid, "%s lies outside the declared fault window", name)
			}
		}
	}

	for _, t := range ev.UnexpectedTransitions {
		if !finiteNumber(t.AtMS) || t.AtMS <= 0 || t.ContainerID == "" || t.Kind == "" {
			add(&invalid, "unexpected transition is missing a timestamp, container id or kind")
			continue
		}
		add(&invalid, "unexpected extra transition %q at %g on container %q", t.Kind, t.AtMS, t.ContainerID)
	}
	if len(ev.UnexpectedTransitions) > 0 {
		res.UnexpectedExtraTransitions = true
	}

	// Explicit timestamp ordering. Equality is allowed; a later stage may not start
	// before an earlier one.
	ordered := func(name string, a, b *float64) {
		if a != nil && b != nil && (!finiteNumber(*a) || !finiteNumber(*b) || *a <= 0 || *b <= 0) {
			add(&invalid, "%s contains a missing, zero or non-finite timestamp", name)
		} else if a != nil && b != nil && *a > *b {
			add(&invalid, "%s is out of order: %g follows %g", name, *a, *b)
		}
	}
	ordered("stop request/completion", floatPtr(ev.StopRequestMS), ev.StopCompletionMS)
	ordered("stop completion/unavailability start", ev.StopCompletionMS, ev.ObservedUnavailabilityStartMS)
	ordered("unavailability start/end", ev.ObservedUnavailabilityStartMS, ev.ObservedUnavailabilityEndMS)
	ordered("unavailability end/healthy rejoin", ev.ObservedUnavailabilityEndMS, ev.HealthyRejoinMS)
	ordered("stop completion/restart request", ev.StopCompletionMS, ev.RestartRequestMS)
	ordered("restart request/healthy rejoin", ev.RestartRequestMS, ev.HealthyRejoinMS)
	ordered("stop request/restart request", floatPtr(ev.StopRequestMS), ev.RestartRequestMS)
	ordered("stop request/LB removal", floatPtr(ev.StopRequestMS), ev.LBRemovalMS)
	ordered("LB removal/healthy rejoin", ev.LBRemovalMS, ev.HealthyRejoinMS)
	for name, p := range map[string]*float64{
		"stop_completion_ms":               ev.StopCompletionMS,
		"observed_unavailability_start_ms": ev.ObservedUnavailabilityStartMS,
		"observed_unavailability_end_ms":   ev.ObservedUnavailabilityEndMS,
		"lb_removal_ms":                    ev.LBRemovalMS,
		"restart_request_ms":               ev.RestartRequestMS,
		"healthy_rejoin_ms":                ev.HealthyRejoinMS,
	} {
		if p != nil && (!finiteNumber(*p) || *p <= 0) {
			add(&invalid, "%s must be a positive finite timestamp when supplied", name)
		}
	}

	if ev.StopCompletionMS == nil {
		add(&unavailable, "stop completion is missing; stop outcome is unavailable")
	}
	if ev.ObservedUnavailabilityStartMS == nil {
		add(&unavailable, "observed unavailability start is missing")
	}
	if ev.LBRemovalMS == nil {
		add(&unavailable, "LB removal evidence is missing; removal cannot be attributed")
	}
	if ev.ObservedUnavailabilityEndMS == nil {
		add(&censored, "observed unavailability end is missing; recovery is right-censored")
	}
	if ev.HealthyRejoinMS == nil {
		add(&censored, "healthy rejoin is missing; recovery is right-censored")
	}
	if ev.RestartRequestMS == nil && ev.HealthyRejoinMS != nil {
		add(&unavailable, "restart request is missing; the rejoin mechanism cannot be attributed")
	}
	if ev.ClockDomain == "" || ev.ClockSource == "" {
		add(&unavailable, "explicit clock domain and source are required")
	}
	if ev.EventSource != FaultEventSourceOperatorManual {
		add(&invalid, "event_source must be exactly %q", FaultEventSourceOperatorManual)
	}

	// A structurally valid identity still yields a declaration-based topology
	// reference even when recovery is incomplete.
	if ev.Project != "" && ev.Service != "" && ev.Replicas > 0 && ev.Replica >= 0 && ev.Replica < ev.Replicas {
		classification, remaining := "replica_degradation", ev.Replicas-1
		if ev.Replicas == 1 {
			classification, remaining = "N=1_service_outage", 0
		}
		res.DegradedTopology = &FaultTopologyReference{
			Project: ev.Project, Service: ev.Service, Replicas: ev.Replicas,
			DownReplica: ev.Replica, RemainingReplicas: remaining,
			Classification:      classification,
			Basis:               "declared N and exactly one intended replica; not inferred from the transient stop/rejoin interval",
			DegradationMeasured: false, InferredFromTransientWindow: false,
		}
	}

	if len(invalid) > 0 {
		res.Status = FaultEventInvalid
		res.Reasons = sortedUnique(invalid)
		return res
	}
	if ev.DryRun {
		res.Status = FaultEventDryRun
		res.Reasons = sortedUnique(append(unavailable, censored...))
		if len(res.Reasons) == 0 {
			res.Reasons = []string{"dry-run event is structurally valid but cannot qualify an executed fault"}
		}
		res.Transient = transientObservation(ev)
		return res
	}
	if len(unavailable) > 0 {
		res.Status = FaultUnavailable
		res.Reasons = sortedUnique(unavailable)
		res.Transient = transientObservation(ev)
		return res
	}
	if len(censored) > 0 {
		res.Status = FaultRightCensored
		res.Reasons = sortedUnique(censored)
		res.Transient = transientObservation(ev)
		return res
	}

	res.Status = FaultQualified
	res.Qualified = true
	res.Reasons = []string{"explicit manual event completed one intended replica stop and healthy rejoin with ordered timestamps"}
	res.Transient = transientObservation(ev)
	return res
}

func transientObservation(ev FaultEvent) *FaultTransientObservation {
	o := &FaultTransientObservation{
		StopRequested:         finiteNumber(ev.StopRequestMS) && ev.StopRequestMS > 0,
		StopObserved:          ev.StopCompletionMS != nil,
		StopRequestMS:         ev.StopRequestMS,
		StopCompletionMS:      ev.StopCompletionMS,
		Signal:                ev.Signal,
		GraceMS:               ev.GraceMS,
		Forced:                ev.Forced,
		LBRemovalObserved:     ev.LBRemovalMS != nil,
		LBRemovalMS:           ev.LBRemovalMS,
		UnavailabilityStartMS: ev.ObservedUnavailabilityStartMS,
		UnavailabilityEndMS:   ev.ObservedUnavailabilityEndMS,
		RestartRequested:      ev.RestartRequestMS != nil,
		RestartRequestMS:      ev.RestartRequestMS,
		HealthyRejoinObserved: ev.HealthyRejoinMS != nil,
		HealthyRejoinMS:       ev.HealthyRejoinMS,
	}
	if ev.StopCompletionMS != nil {
		o.StopDurationMS = nonNegativeDelta(ev.StopRequestMS, *ev.StopCompletionMS)
	}
	if ev.ObservedUnavailabilityStartMS != nil && ev.ObservedUnavailabilityEndMS != nil {
		o.UnavailabilityDurationMS = nonNegativeDelta(*ev.ObservedUnavailabilityStartMS, *ev.ObservedUnavailabilityEndMS)
	}
	if ev.RestartRequestMS != nil && ev.HealthyRejoinMS != nil {
		o.RejoinAfterRestartMS = nonNegativeDelta(*ev.RestartRequestMS, *ev.HealthyRejoinMS)
	}
	if ev.ObservedUnavailabilityEndMS != nil && ev.HealthyRejoinMS != nil {
		o.RejoinAfterUnavailabilityMS = nonNegativeDelta(*ev.ObservedUnavailabilityEndMS, *ev.HealthyRejoinMS)
	}
	return o
}

func nonNegativeDelta(a, b float64) *float64 {
	if !finiteNumber(a) || !finiteNumber(b) {
		return nil
	}
	v := b - a
	if v < 0 {
		return nil
	}
	return &v
}

func floatPtr(v float64) *float64 { return &v }

func sortedUnique(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := append([]string{}, in...)
	sort.Strings(out)
	j := 0
	for _, s := range out {
		if j == 0 || out[j-1] != s {
			out[j] = s
			j++
		}
	}
	return out[:j]
}

// DecodeFaultEvent strictly decodes one manual fault event. Duplicate keys,
// unknown fields, trailing JSON and explicit nulls are rejected.
func DecodeFaultEvent(b []byte) (*FaultEvent, error) {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, err
	}
	required := []string{
		"schema_version", "run_id", "protocol_binding_sha256", "platform", "project", "service",
		"container_id", "container_image", "replica", "replicas", "service_replicas",
		"stop_request_ms", "signal", "forced", "unexpected_transitions",
		"offered_rate_messages_per_second", "window_start_ms", "window_end_ms",
		"clock_domain", "clock_source", "event_source", "dry_run",
	}
	if err := requireJSONFields(top, "fault event", required...); err != nil {
		return nil, err
	}
	if err := rejectJSONNulls(top, "fault event"); err != nil {
		return nil, err
	}
	var replicas map[string]json.RawMessage
	if err := json.Unmarshal(top["service_replicas"], &replicas); err != nil {
		return nil, fmt.Errorf("fault event service_replicas must be an explicit object: %w", err)
	}
	for k, v := range replicas {
		if k == "" {
			return nil, fmt.Errorf("fault event service_replicas contains an empty key")
		}
		if string(v) == "null" {
			return nil, fmt.Errorf("fault event service_replicas[%q] must not be null", k)
		}
	}
	var transitions []map[string]json.RawMessage
	if err := json.Unmarshal(top["unexpected_transitions"], &transitions); err != nil {
		return nil, fmt.Errorf("fault event unexpected_transitions must be an explicit array: %w", err)
	}
	for i, tr := range transitions {
		if err := requireJSONFields(tr, fmt.Sprintf("fault event unexpected transition %d", i), "at_ms", "container_id", "kind"); err != nil {
			return nil, err
		}
		if err := rejectJSONNulls(tr, fmt.Sprintf("fault event unexpected transition %d", i)); err != nil {
			return nil, err
		}
	}
	var ev FaultEvent
	if err := decode(b, &ev, true); err != nil {
		return nil, err
	}
	return &ev, nil
}
