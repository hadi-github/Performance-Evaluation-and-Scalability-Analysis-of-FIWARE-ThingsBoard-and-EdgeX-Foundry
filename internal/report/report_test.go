package report

import (
	"math"
	"testing"
)

func TestQuantiles(t *testing.T) {
	edges := []float64{0, 1, 2, 5, 10}
	// 100 samples: 90 in <=1, 9 in <=5, 1 in overflow
	q := quantiles(edges, []float64{0, 90, 0, 9, 0, 1})
	if q.Count != 100 || q.P50 != 1 || q.P99 != 5 || !math.IsInf(float64(q.Max), 1) {
		t.Fatalf("got %+v", q)
	}
	q = quantiles(edges, []float64{0, 0, 0, 0, 0, 0})
	if q.Count != 0 || !math.IsNaN(float64(q.P99)) {
		t.Fatalf("empty: %+v", q)
	}
}
