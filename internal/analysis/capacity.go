package analysis

// M1f offline recovery/capacity reducer. All windows, the pass predicate and the
// search policy are explicit caller inputs. Nothing in this file adopts a default
// SLO threshold, a hybrid search method, or a campaign-capacity verdict.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
)

const (
	CapacitySchemaVersion    = 1
	CapacityPredicateVersion = "capacity-predicate-v1"
	CapacitySearchVersion    = "manual-window-grid-v1"
)

type CapacityStatus string

const (
	CapacityBracketed      CapacityStatus = "bracketed"
	CapacityLowerBoundOnly CapacityStatus = "lower_bound_only"
	CapacityNoPassingRate  CapacityStatus = "no_passing_rate"
	CapacityUnresolved     CapacityStatus = "unresolved"
)

// CapacityPredicate is a versioned, explicit pass predicate. Every numeric gate is
// supplied; there is no implicit 500 ms, error-rate, backlog or sample default.
type CapacityPredicate struct {
	Version              string   `json:"version"`
	ID                   string   `json:"id"`
	P99MaxMS             float64  `json:"p99_max_ms"`
	AttainmentMinRatio   float64  `json:"attainment_min_ratio"`
	MissingMax           float64  `json:"missing_max"`
	AchievedRateMinRatio float64  `json:"achieved_rate_min_ratio"`
	BacklogMax           *float64 `json:"backlog_max,omitempty"`
	MinSamples           int      `json:"min_samples"`
	Source               string   `json:"source"`
}

// CapacitySearchPolicy states the search assumptions instead of silently choosing
// between archived protocol variants.
type CapacitySearchPolicy struct {
	Version             string `json:"version"`
	MonotonicityAssumed bool   `json:"monotonicity_assumed"`
	MinConfirmations    int    `json:"min_confirmations"`
	Source              string `json:"source"`
}

// CapacityWindow is a predeclared observation window. Counts and metrics are
// supplied evidence, not recomputed from a run and not synthesized.
type CapacityWindow struct {
	ID              string   `json:"id"`
	RunID           string   `json:"run_id"`
	OfferedRate     float64  `json:"offered_rate_messages_per_second"`
	Fault           bool     `json:"fault"`
	Recovery        string   `json:"recovery"` // none | failover | rejoin | not_recovered
	WindowStartMS   float64  `json:"window_start_ms"`
	WindowEndMS     float64  `json:"window_end_ms"`
	Scheduled       float64  `json:"scheduled"`
	Observed        float64  `json:"observed"`
	Missing         float64  `json:"missing"`
	P99MS           float64  `json:"p99_ms"`
	AttainmentRatio float64  `json:"attainment_ratio"`
	AchievedRate    float64  `json:"achieved_rate_messages_per_second"`
	Backlog         float64  `json:"backlog"`
	Samples         float64  `json:"samples"`
	RecoveredAtMS   *float64 `json:"recovered_at_ms,omitempty"`
}

type CapacityInput struct {
	SchemaVersion int                  `json:"schema_version"`
	Predicate     CapacityPredicate    `json:"predicate"`
	Search        CapacitySearchPolicy `json:"search"`
	Windows       []CapacityWindow     `json:"windows"`
}

type CapacityWindowResult struct {
	ID                string   `json:"id"`
	RunID             string   `json:"run_id"`
	OfferedRate       float64  `json:"offered_rate_messages_per_second"`
	Fault             bool     `json:"fault"`
	Recovery          string   `json:"recovery"`
	Validity          string   `json:"validity"` // valid | invalid | right_censored
	Outcome           string   `json:"outcome"`  // pass | fail | failover_observed | rejoin_observed | invalid | right_censored
	ObservedViolation bool     `json:"observed_violation"`
	Violations        []string `json:"violations"`
	Reasons           []string `json:"reasons"`
	UsedForCapacity   bool     `json:"used_for_capacity"`
	Censored          bool     `json:"censored"`
	Exact             bool     `json:"exact"`
	RecoveredAtMS     *float64 `json:"recovered_at_ms,omitempty"`
}

type RateConfirmation struct {
	Rate            float64  `json:"offered_rate_messages_per_second"`
	Pass            int      `json:"pass"`
	Fail            int      `json:"fail"`
	Invalid         int      `json:"invalid"`
	FaultInvalid    int      `json:"fault_invalid"`
	FaultCensored   int      `json:"fault_right_censored"`
	CapacityWindows int      `json:"no_fault_windows"`
	Confirmed       bool     `json:"confirmed"`
	Contradictory   bool     `json:"contradictory"`
	WindowIDs       []string `json:"window_ids"`
}

type CapacityBoundEvidence struct {
	Bound         string   `json:"bound"` // lower | upper
	Rate          *float64 `json:"rate"`
	Confirmations int      `json:"confirmations"`
	Basis         string   `json:"basis"`
	WindowIDs     []string `json:"window_ids"`
}

type RecoveryWindowResult struct {
	ID                string   `json:"id"`
	RunID             string   `json:"run_id"`
	OfferedRate       float64  `json:"offered_rate_messages_per_second"`
	Recovery          string   `json:"recovery"`
	Status            string   `json:"status"` // failover_observed | rejoin_observed | right_censored | invalid
	Censored          bool     `json:"censored"`
	Exact             bool     `json:"exact"`
	ObservedViolation bool     `json:"observed_violation"`
	Reasons           []string `json:"reasons"`
}

type RecoverySummary struct {
	Status             string                 `json:"status"` // no_fault_windows | observed | right_censored | invalid
	FaultWindows       int                    `json:"fault_windows"`
	NoFaultWindows     int                    `json:"no_fault_windows"`
	FailoverObserved   int                    `json:"failover_observed"`
	RejoinObserved     int                    `json:"rejoin_observed"`
	RightCensored      int                    `json:"right_censored"`
	Invalid            int                    `json:"invalid"`
	Windows            []RecoveryWindowResult `json:"windows"`
	CensoredReferences []RecoveryWindowResult `json:"censored_references,omitempty"`
}

type CapacityResult struct {
	SchemaVersion  int                     `json:"schema_version"`
	Status         CapacityStatus          `json:"status"`
	LowerBound     *float64                `json:"lower_bound_messages_per_second"`
	UpperBound     *float64                `json:"upper_bound_messages_per_second"`
	BoundEvidence  []CapacityBoundEvidence `json:"bound_evidence"`
	StopReason     string                  `json:"stop_reason"`
	Confirmations  []RateConfirmation      `json:"confirmation_counts"`
	Recovery       RecoverySummary         `json:"recovery"`
	Windows        []CapacityWindowResult  `json:"windows"`
	Predicate      CapacityPredicate       `json:"predicate"`
	Search         CapacitySearchPolicy    `json:"search"`
	Contradictions []string                `json:"contradictions"`
	Limitations    []string                `json:"limitations"`
}

// ReduceCapacity computes descriptive lower/upper observations from explicit
// windows. It does not extrapolate, invent missing samples, or call any result a
// campaign capacity by default.
func ReduceCapacity(in CapacityInput) CapacityResult {
	res := CapacityResult{
		SchemaVersion:  CapacitySchemaVersion,
		Status:         CapacityUnresolved,
		Predicate:      in.Predicate,
		Search:         in.Search,
		Confirmations:  []RateConfirmation{},
		Contradictions: []string{},
		BoundEvidence:  []CapacityBoundEvidence{},
		Limitations: []string{
			"Bounds are empirical window-point observations under the explicitly supplied search policy, not an extrapolated or campaign-qualified C_SLO.",
			"Fault windows are recovery observations only; they never establish capacity bounds.",
			"Invalid, missing-sample or right-censored windows are not exact references and cannot establish either bound.",
		},
	}
	var policyReasons []string
	if in.SchemaVersion != CapacitySchemaVersion {
		policyReasons = append(policyReasons, fmt.Sprintf("unsupported capacity schema_version %d", in.SchemaVersion))
	}
	if in.Predicate.Version != CapacityPredicateVersion || in.Predicate.ID == "" || in.Predicate.Source == "" {
		policyReasons = append(policyReasons, "explicit versioned capacity predicate id/source is required")
	}
	if !finiteNumber(in.Predicate.P99MaxMS) || in.Predicate.P99MaxMS <= 0 {
		policyReasons = append(policyReasons, "predicate p99_max_ms must be explicit and positive")
	}
	if !finiteNumber(in.Predicate.AttainmentMinRatio) || in.Predicate.AttainmentMinRatio <= 0 || in.Predicate.AttainmentMinRatio > 1 {
		policyReasons = append(policyReasons, "predicate attainment_min_ratio must be explicit in (0,1]")
	}
	if !finiteNumber(in.Predicate.MissingMax) || in.Predicate.MissingMax < 0 {
		policyReasons = append(policyReasons, "predicate missing_max must be explicit and non-negative")
	}
	if !finiteNumber(in.Predicate.AchievedRateMinRatio) || in.Predicate.AchievedRateMinRatio <= 0 {
		policyReasons = append(policyReasons, "predicate achieved_rate_min_ratio must be explicit and positive")
	}
	if in.Predicate.MinSamples <= 0 {
		policyReasons = append(policyReasons, "predicate min_samples must be explicit and positive")
	}
	if in.Predicate.BacklogMax != nil && (!finiteNumber(*in.Predicate.BacklogMax) || *in.Predicate.BacklogMax < 0) {
		policyReasons = append(policyReasons, "predicate backlog_max is invalid")
	}
	if in.Search.Version != CapacitySearchVersion || in.Search.Source == "" {
		policyReasons = append(policyReasons, "explicit versioned capacity search policy and source are required")
	}
	if in.Search.MinConfirmations <= 0 {
		policyReasons = append(policyReasons, "search min_confirmations must be explicit and positive")
	}
	if len(policyReasons) > 0 {
		res.StopReason = "explicit predicate/search policy incomplete or unsupported"
		res.Limitations = append(res.Limitations, policyReasons...)
		return res
	}
	if len(in.Windows) == 0 {
		res.StopReason = "no predeclared observation windows supplied"
		return res
	}

	perRate := map[float64]*RateConfirmation{}
	var rates, capacityRates []float64
	capacitySeen := map[float64]bool{}
	var faultResults []RecoveryWindowResult
	for _, w := range in.Windows {
		wr := evaluateCapacityWindow(w, in.Predicate, in.Search)
		res.Windows = append(res.Windows, wr)
		if _, ok := perRate[w.OfferedRate]; !ok {
			perRate[w.OfferedRate] = &RateConfirmation{Rate: w.OfferedRate, WindowIDs: []string{}}
			rates = append(rates, w.OfferedRate)
		}
		rc := perRate[w.OfferedRate]
		rc.WindowIDs = append(rc.WindowIDs, w.ID)
		if w.Fault {
			rr := RecoveryWindowResult{ID: w.ID, RunID: w.RunID, OfferedRate: w.OfferedRate, Recovery: w.Recovery, Censored: wr.Censored, Exact: wr.Exact, ObservedViolation: wr.ObservedViolation, Reasons: append([]string{}, wr.Reasons...)}
			switch {
			case wr.Validity == "invalid":
				rr.Status = "invalid"
				res.Recovery.Invalid++
			case wr.Censored:
				rr.Status = "right_censored"
				res.Recovery.RightCensored++
			case w.Recovery == "failover":
				rr.Status = "failover_observed"
				res.Recovery.FailoverObserved++
			case w.Recovery == "rejoin":
				rr.Status = "rejoin_observed"
				res.Recovery.RejoinObserved++
			default:
				rr.Status = "invalid"
				res.Recovery.Invalid++
			}
			faultResults = append(faultResults, rr)
			if wr.Censored {
				rc.FaultCensored++
			} else if wr.Validity == "invalid" {
				rc.FaultInvalid++
			}
		} else {
			res.Recovery.NoFaultWindows++
			rc.CapacityWindows++
			if !capacitySeen[w.OfferedRate] {
				capacitySeen[w.OfferedRate] = true
				capacityRates = append(capacityRates, w.OfferedRate)
			}
			if wr.Validity == "invalid" {
				rc.Invalid++
			} else if wr.Outcome == "pass" {
				rc.Pass++
			} else if wr.Outcome == "fail" {
				rc.Fail++
			} else {
				rc.Invalid++
			}
		}
	}
	res.Recovery.Windows = faultResults
	res.Recovery.FaultWindows = len(faultResults)
	for _, rr := range faultResults {
		if rr.Censored {
			res.Recovery.CensoredReferences = append(res.Recovery.CensoredReferences, rr)
		}
	}
	switch {
	case len(faultResults) == 0 && res.Recovery.NoFaultWindows == 0:
		res.Recovery.Status = "no_fault_windows"
	case res.Recovery.RightCensored > 0:
		res.Recovery.Status = "right_censored"
	case res.Recovery.Invalid > 0:
		res.Recovery.Status = "invalid"
	case res.Recovery.FailoverObserved > 0 || res.Recovery.RejoinObserved > 0:
		res.Recovery.Status = "observed"
	default:
		res.Recovery.Status = "no_fault_windows"
	}

	sort.Float64s(rates)
	for _, rate := range rates {
		rc := perRate[rate]
		rc.Contradictory = rc.Pass > 0 && rc.Fail > 0
		rc.Confirmed = !rc.Contradictory && rc.Invalid == 0 && (rc.Pass >= in.Search.MinConfirmations || rc.Fail >= in.Search.MinConfirmations)
		sort.Strings(rc.WindowIDs)
		res.Confirmations = append(res.Confirmations, *rc)
		if rc.Contradictory {
			res.Contradictions = append(res.Contradictions, fmt.Sprintf("rate %g has %d passing and %d failing valid repetitions", rate, rc.Pass, rc.Fail))
		}
	}

	// Bound computation uses ascending no-fault rates so the result is invariant to
	// caller window order. Windows themselves are still reported in supplied order.
	sort.Float64s(capacityRates)

	// A lower bound is the highest contiguous confirmed passing prefix from the
	// lowest tested no-fault rate. Any invalid, failing or contradictory point stops
	// the prefix rather than being skipped.
	var lower *float64
	lowerIDs := []string{}
	lowerConfirmations := 0
	for _, rate := range capacityRates {
		rc := perRate[rate]
		if rc.Fail > 0 || rc.Invalid > 0 || rc.Contradictory || rc.Pass < in.Search.MinConfirmations {
			break
		}
		v := rate
		lower = &v
		lowerIDs = append([]string{}, rc.WindowIDs...)
		lowerConfirmations = rc.Pass
	}
	var upper *float64
	upperIDs := []string{}
	upperConfirmations := 0
	if lower != nil {
		for _, rate := range capacityRates {
			if rate <= *lower {
				continue
			}
			rc := perRate[rate]
			if rc.Fail >= in.Search.MinConfirmations && rc.Pass == 0 && rc.Invalid == 0 && !rc.Contradictory {
				v := rate
				upper = &v
				upperIDs = append([]string{}, rc.WindowIDs...)
				upperConfirmations = rc.Fail
				break
			}
		}
	}

	// Any confirmed passing rate above a confirmed failing rate contradicts the
	// declared monotonicity assumption and prevents a clean bracket.
	monotonicityConflict := false
	for _, p := range capacityRates {
		pc := perRate[p]
		if pc.Pass < in.Search.MinConfirmations || pc.Fail > 0 || pc.Invalid > 0 || pc.Contradictory {
			continue
		}
		for _, f := range capacityRates {
			fc := perRate[f]
			if f < p && fc.Fail >= in.Search.MinConfirmations && fc.Pass == 0 && fc.Invalid == 0 && !fc.Contradictory {
				res.Contradictions = append(res.Contradictions, fmt.Sprintf("passing rate %g lies above failing rate %g", p, f))
				monotonicityConflict = true
			}
		}
	}
	if monotonicityConflict {
		// A pass-above-fail contradiction is not a clean monotonic bracket. Discard
		// any candidate upper bound so the result cannot imply one that the ordered
		// evidence does not support. An independently established lower bound is
		// preserved and the conflict stays visible.
		upper = nil
		upperIDs = []string{}
		upperConfirmations = 0
	}
	if len(res.Contradictions) > 0 {
		res.Contradictions = sortedUnique(res.Contradictions)
	}

	switch {
	case lower == nil:
		anyValidNoFault := false
		anyValidPass := false
		for _, rate := range capacityRates {
			rc := perRate[rate]
			if rc.Pass > 0 {
				anyValidPass = true
			}
			if rc.Invalid == 0 && !rc.Contradictory && (rc.Pass > 0 || rc.Fail > 0) {
				anyValidNoFault = true
			}
		}
		if !anyValidNoFault {
			res.Status = CapacityUnresolved
			res.StopReason = "no valid no-fault window can establish a capacity point"
		} else if !anyValidPass {
			res.Status = CapacityNoPassingRate
			res.StopReason = "no valid no-fault window passed the explicit predicate"
		} else {
			res.Status = CapacityUnresolved
			res.StopReason = "no contiguous confirmed passing prefix exists at the explicit confirmation level"
		}
	case monotonicityConflict:
		res.Status = CapacityLowerBoundOnly
		res.LowerBound = lower
		res.StopReason = "a confirmed passing rate lies above a confirmed failing rate, contradicting the declared monotonicity; no upper bound is claimed"
	case upper == nil:
		res.Status = CapacityLowerBoundOnly
		res.LowerBound = lower
		res.StopReason = "all tested windows at or above the lower bound passed or were unavailable; no valid failing point establishes an upper bound"
	case !in.Search.MonotonicityAssumed:
		res.Status = CapacityLowerBoundOnly
		res.LowerBound = lower
		res.StopReason = "a valid failing point was observed above the lower bound, but the search policy does not adopt monotonicity; no upper bound is claimed"
	default:
		res.Status = CapacityBracketed
		res.LowerBound = lower
		res.UpperBound = upper
		res.StopReason = "highest contiguous confirmed passing rate and lowest confirmed failing rate under the explicit monotonicity assumption"
	}

	if lower != nil {
		res.BoundEvidence = append(res.BoundEvidence, CapacityBoundEvidence{Bound: "lower", Rate: lower, Confirmations: lowerConfirmations, Basis: "highest contiguous confirmed passing no-fault rate", WindowIDs: lowerIDs})
	} else {
		res.BoundEvidence = append(res.BoundEvidence, CapacityBoundEvidence{Bound: "lower", Rate: nil, Confirmations: 0, Basis: "not established", WindowIDs: []string{}})
	}
	if res.Status == CapacityBracketed {
		res.BoundEvidence = append(res.BoundEvidence, CapacityBoundEvidence{Bound: "upper", Rate: upper, Confirmations: upperConfirmations, Basis: "lowest confirmed failing no-fault rate above the lower bound", WindowIDs: upperIDs})
	} else {
		reason := "not established"
		if lower != nil && monotonicityConflict {
			reason = "not established: a confirmed passing rate above a confirmed failing rate contradicts the declared monotonicity"
		} else if upper != nil && !in.Search.MonotonicityAssumed {
			reason = "observed failing point not adopted as an upper bound because monotonicity is not declared"
		} else if upper != nil {
			reason = "observed failing point did not meet the explicit confirmation requirement"
		}
		res.BoundEvidence = append(res.BoundEvidence, CapacityBoundEvidence{Bound: "upper", Rate: nil, Confirmations: 0, Basis: reason, WindowIDs: []string{}})
	}
	return res
}

func evaluateCapacityWindow(w CapacityWindow, p CapacityPredicate, s CapacitySearchPolicy) CapacityWindowResult {
	wr := CapacityWindowResult{ID: w.ID, RunID: w.RunID, OfferedRate: w.OfferedRate, Fault: w.Fault, Recovery: w.Recovery, Violations: []string{}, Reasons: []string{}, RecoveredAtMS: w.RecoveredAtMS}
	var reasons []string
	add := func(format string, args ...any) { reasons = append(reasons, fmt.Sprintf(format, args...)) }
	if w.ID == "" || w.RunID == "" {
		add("window id and run_id are required")
	}
	if !finiteNumber(w.OfferedRate) || w.OfferedRate <= 0 {
		add("offered rate is missing or non-positive")
	}
	if !finiteNumber(w.WindowStartMS) || !finiteNumber(w.WindowEndMS) || w.WindowStartMS <= 0 || w.WindowEndMS <= w.WindowStartMS {
		add("window start/end is missing or non-monotonic")
	}
	if w.RecoveredAtMS != nil && (!finiteNumber(*w.RecoveredAtMS) || *w.RecoveredAtMS <= 0) {
		add("recovered_at_ms is invalid")
	}
	switch w.Recovery {
	case "none":
		if w.Fault {
			add("fault window must declare failover, rejoin or not_recovered recovery")
		}
	case "failover", "rejoin", "not_recovered":
		if !w.Fault {
			add("no-fault window must declare recovery=none")
		}
	default:
		add("recovery must be one of none, failover, rejoin, not_recovered")
	}

	for name, v := range map[string]float64{
		"scheduled": w.Scheduled, "observed": w.Observed, "missing": w.Missing,
		"attainment_ratio": w.AttainmentRatio, "achieved_rate_messages_per_second": w.AchievedRate,
		"backlog": w.Backlog, "samples": w.Samples,
	} {
		if !finiteNumber(v) || v < 0 {
			add("%s is missing, negative or non-finite", name)
		}
	}
	for name, v := range map[string]float64{
		"scheduled": w.Scheduled, "observed": w.Observed, "missing": w.Missing, "samples": w.Samples,
	} {
		if finiteNumber(v) && math.Trunc(v) != v {
			add("%s must be an integer count", name)
		}
	}
	if w.Scheduled == 0 {
		add("zero scheduled denominator cannot establish a metric")
	}
	if w.Observed+w.Missing != w.Scheduled {
		add("scheduled does not equal observed + missing")
	}
	if w.Samples != w.Observed {
		add("latency samples (%g) do not cover the observed cohort (%g); the reference is not exact", w.Samples, w.Observed)
	}
	if w.Samples < float64(p.MinSamples) {
		add("insufficient samples: %g supplied, predicate requires at least %d", w.Samples, p.MinSamples)
	}
	if w.AttainmentRatio < 0 || w.AttainmentRatio > 1 {
		add("attainment_ratio must be in [0,1]")
	}
	if w.P99MS < 0 || !finiteNumber(w.P99MS) {
		add("p99_ms is invalid")
	}
	if w.P99MS > p.P99MaxMS {
		wr.Violations = append(wr.Violations, fmt.Sprintf("p99 %g ms exceeds %g ms", w.P99MS, p.P99MaxMS))
	}
	if w.AttainmentRatio < p.AttainmentMinRatio {
		wr.Violations = append(wr.Violations, fmt.Sprintf("attainment %g is below %g", w.AttainmentRatio, p.AttainmentMinRatio))
	}
	if w.Missing > p.MissingMax {
		wr.Violations = append(wr.Violations, fmt.Sprintf("missing %g exceeds %g", w.Missing, p.MissingMax))
	}
	if w.AchievedRate < w.OfferedRate*p.AchievedRateMinRatio {
		wr.Violations = append(wr.Violations, fmt.Sprintf("achieved rate %g is below %g x offered %g", w.AchievedRate, p.AchievedRateMinRatio, w.OfferedRate))
	}
	if p.BacklogMax != nil && w.Backlog > *p.BacklogMax {
		wr.Violations = append(wr.Violations, fmt.Sprintf("backlog %g exceeds %g", w.Backlog, *p.BacklogMax))
	}
	wr.ObservedViolation = len(wr.Violations) > 0

	if len(reasons) > 0 {
		wr.Validity = "invalid"
		wr.Outcome = "invalid"
		wr.Reasons = sortedUnique(reasons)
		wr.Exact = false
		return wr
	}
	wr.Validity = "valid"
	wr.Exact = true
	if w.Fault {
		wr.UsedForCapacity = false
		switch w.Recovery {
		case "not_recovered":
			wr.Censored = true
			wr.Exact = false
			wr.Outcome = "right_censored"
			wr.Reasons = []string{"fault window declares non-recovery; recovery is right-censored, not success"}
		case "rejoin":
			if w.RecoveredAtMS == nil {
				wr.Censored = true
				wr.Exact = false
				wr.Outcome = "right_censored"
				wr.Reasons = []string{"rejoin recovery timestamp is missing; recovery is right-censored"}
			} else if *w.RecoveredAtMS < w.WindowStartMS || *w.RecoveredAtMS > w.WindowEndMS {
				wr.Censored = true
				wr.Exact = false
				wr.Outcome = "right_censored"
				wr.Reasons = []string{"recovery lies outside the predeclared observation window; the reference is right-censored and not exact"}
			} else {
				wr.Outcome = "rejoin_observed"
			}
		case "failover":
			if w.Observed == 0 {
				wr.Censored = true
				wr.Exact = false
				wr.Outcome = "right_censored"
				wr.Reasons = []string{"failover declares no observed served messages; failover is right-censored"}
			} else {
				wr.Outcome = "failover_observed"
			}
		}
		return wr
	}
	wr.UsedForCapacity = true
	if wr.ObservedViolation {
		wr.Outcome = "fail"
	} else {
		wr.Outcome = "pass"
	}
	return wr
}

// DecodeCapacityInput strictly decodes the explicit predicate/search/window input.
func DecodeCapacityInput(b []byte) (*CapacityInput, error) {
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, err
	}
	if err := requireJSONFields(top, "capacity input", "schema_version", "predicate", "search", "windows"); err != nil {
		return nil, err
	}
	if err := rejectJSONNulls(top, "capacity input"); err != nil {
		return nil, err
	}
	var pred map[string]json.RawMessage
	if err := json.Unmarshal(top["predicate"], &pred); err != nil {
		return nil, fmt.Errorf("capacity predicate must be an explicit object: %w", err)
	}
	if err := requireJSONFields(pred, "capacity predicate", "version", "id", "p99_max_ms", "attainment_min_ratio", "missing_max", "achieved_rate_min_ratio", "min_samples", "source"); err != nil {
		return nil, err
	}
	if err := rejectJSONNulls(pred, "capacity predicate"); err != nil {
		return nil, err
	}
	var search map[string]json.RawMessage
	if err := json.Unmarshal(top["search"], &search); err != nil {
		return nil, fmt.Errorf("capacity search must be an explicit object: %w", err)
	}
	if err := requireJSONFields(search, "capacity search", "version", "monotonicity_assumed", "min_confirmations", "source"); err != nil {
		return nil, err
	}
	if err := rejectJSONNulls(search, "capacity search"); err != nil {
		return nil, err
	}
	var windows []map[string]json.RawMessage
	if err := json.Unmarshal(top["windows"], &windows); err != nil {
		return nil, fmt.Errorf("capacity windows must be an explicit array: %w", err)
	}
	for i, w := range windows {
		if err := requireJSONFields(w, fmt.Sprintf("capacity window %d", i), "id", "run_id", "offered_rate_messages_per_second", "fault", "recovery", "window_start_ms", "window_end_ms", "scheduled", "observed", "missing", "p99_ms", "attainment_ratio", "achieved_rate_messages_per_second", "backlog", "samples"); err != nil {
			return nil, err
		}
		if err := rejectJSONNulls(w, fmt.Sprintf("capacity window %d", i)); err != nil {
			return nil, err
		}
	}
	var in CapacityInput
	if err := decode(b, &in, true); err != nil {
		return nil, err
	}
	return &in, nil
}
