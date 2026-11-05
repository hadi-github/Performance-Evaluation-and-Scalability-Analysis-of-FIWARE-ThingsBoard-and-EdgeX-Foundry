package protocol

import "sort"

type Outcome string

const (
	OutcomeOK      Outcome = "OK"
	OutcomeError   Outcome = "ERROR"
	OutcomeUnknown Outcome = "UNKNOWN"
	OutcomeNotSent Outcome = "NOT_SENT"
)

// ValidateOutcome refuses NOT_SENT without positive, identified non-submission evidence.
func ValidateOutcome(outcome Outcome, positiveNonSubmissionEvidence bool) bool {
	switch outcome {
	case OutcomeOK, OutcomeError, OutcomeUnknown:
		return true
	case OutcomeNotSent:
		return positiveNonSubmissionEvidence
	default:
		return false
	}
}

// NearestRank is conditional on observed delivery. Callers must pass one latency per unique first receipt;
// missing outcomes have no finite latency. A zero-observation cohort has no percentile.
func NearestRank(values []float64, percentile int) *float64 {
	if len(values) == 0 || percentile < 1 || percentile > 100 {
		return nil
	}
	x := append([]float64(nil), values...)
	sort.Float64s(x)
	index := (percentile*len(x)+99)/100 - 1
	v := x[index]
	return &v
}

type CohortCounts struct {
	Scheduled int `json:"scheduled"`
	Observed  int `json:"observed"`
	Missing   int `json:"missing"`
}

func Counts(scheduled int, uniqueFirstObserved int) CohortCounts {
	return CohortCounts{Scheduled: scheduled, Observed: uniqueFirstObserved, Missing: scheduled - uniqueFirstObserved}
}

// Attainment counts scheduled cohort messages with a unique observed receipt at or below threshold.
// The denominator is scheduled, so missing receipts do not disappear from the companion metric.
func Attainment(scheduled int, uniqueFirstReceiptLatencyMS []float64, thresholdMS float64) (int, float64) {
	if scheduled <= 0 || thresholdMS < 0 {
		return 0, 0
	}
	met := 0
	for _, latency := range uniqueFirstReceiptLatencyMS {
		if latency <= thresholdMS {
			met++
		}
	}
	return met, float64(met) / float64(scheduled)
}
