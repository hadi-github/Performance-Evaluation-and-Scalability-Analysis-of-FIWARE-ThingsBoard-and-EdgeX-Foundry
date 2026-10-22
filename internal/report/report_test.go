package report

import (
	"encoding/json"
	"math"
	"testing"
)

func TestFromTrend(t *testing.T) {
	q := fromTrend(TrendStats{Count: 100, P50: 1.5, P99: 7.25, Max: 30})
	if q.Count != 100 || q.P50 != 1.5 || q.P99 != 7.25 || q.Max != 30 {
		t.Fatalf("got %+v", q)
	}
	q = fromTrend(TrendStats{})
	if q.Count != 0 || !math.IsNaN(float64(q.P99)) {
		t.Fatalf("empty: %+v", q)
	}
	b, err := json.Marshal(q)
	if err != nil || string(b) != `{"count":0,"p50":null,"p99":null,"max":null}` {
		t.Fatalf("json %s %v", b, err)
	}
}
