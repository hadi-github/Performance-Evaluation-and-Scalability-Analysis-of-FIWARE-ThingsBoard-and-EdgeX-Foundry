package diskmetrics

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sample(t float64, commit, walSync int64, syncMs float64) PGSample {
	var s PGSample
	s.TMs = t
	s.DB.XactCommit = commit
	s.WAL.WALSync = walSync
	s.WAL.WALSyncTime = syncMs
	return s
}

func near(t *testing.T, name string, got *float64, want float64) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: nil, want %v", name, want)
	}
	if math.Abs(*got-want) > 1e-9 {
		t.Fatalf("%s: got %v, want %v", name, *got, want)
	}
}

func TestWALKnownAnswer(t *testing.T) {
	// 60 s window: 600 commits, 300 WAL syncs, 1500 ms in fdatasync.
	s := []PGSample{sample(1_000_000, 1000, 500, 1000), sample(1_060_000, 1600, 800, 2500)}
	b := WAL(s, 1_000_000, 1_060_000)
	if b.Status != "ok" || b.WindowS != 60 {
		t.Fatalf("status/window: %+v", b)
	}
	near(t, "commits/s", b.CommitsPerS, 10)
	near(t, "syncs/s", b.WALSyncsPerS, 5)
	near(t, "ms/sync", b.WALSyncMsPerSync, 5)
	near(t, "sync share", b.WALSyncTimeShare, 0.025)
	near(t, "commits/sync", b.CommitsPerSync, 2)
}

func TestWALNoSyncsLeavesRatiosUndefined(t *testing.T) {
	s := []PGSample{sample(0, 0, 10, 0), sample(60_000, 100, 10, 0)}
	s[0].TMs, s[1].TMs = 1_000_000, 1_060_000
	b := WAL(s, 1_000_000, 1_060_000)
	if b.Status != "ok" || b.WALSyncMsPerSync != nil || b.CommitsPerSync != nil {
		t.Fatalf("zero syncs must leave per-sync ratios undefined, not zero: %+v", b)
	}
	near(t, "commits/s", b.CommitsPerS, 100.0/60)
	near(t, "share", b.WALSyncTimeShare, 0)
}

func TestWALWithoutSeriesIsUnavailableNotZero(t *testing.T) {
	b := WAL(nil, 0, 60_000)
	if b.Status != "unavailable" || b.CommitsPerS != nil || !strings.Contains(b.Reason, "no usable pg-counters samples") {
		t.Fatalf("missing series must be explicit: %+v", b)
	}
}

func TestWALEdgesMustBracketWindow(t *testing.T) {
	s := []PGSample{sample(1_000_000, 1, 1, 1), sample(1_000_000, 2, 2, 2)}
	if b := WAL(s, 1_000_000, 1_060_000); b.Status != "unavailable" {
		t.Fatalf("zero-length span must be unavailable: %+v", b)
	}
}

func TestReadPGCountersMultiLineAndSkips(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pg-counters.ndjson")
	content := `{"t_ms" : 1000, "db" : {"xact_commit":5}, "wal" : {"wal_sync":2, "wal_sync_time":3.5, "wal_write":1, "wal_write_time":0.1}, "stmts" : [
 {"query":"commit","calls":3}
]}
{"t_ms" : broken
{"t_ms":2000,"db":{"xact_commit":9},"wal":{"wal_sync":4,"wal_sync_time":7.5,"wal_write":2,"wal_write_time":0.2}}
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s, skipped, err := ReadPGCounters(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 || skipped != 1 {
		t.Fatalf("samples %d skipped %d", len(s), skipped)
	}
	if s[0].DB.XactCommit != 5 || s[1].WAL.WALSync != 4 || s[1].WAL.WALSyncTime != 7.5 {
		t.Fatalf("fields: %+v %+v", s[0], s[1])
	}
}

func TestAverageSkipsNonFiniteAndOmitsAllNonFinite(t *testing.T) {
	series := []PromSeries{
		{Metric: map[string]string{"device": "sda"}, Values: [][]any{{1, "0.004"}, {2, "NaN"}, {3, "+Inf"}, {4, "0.006"}}},
		{Metric: map[string]string{"device": "sr0"}, Values: [][]any{{1, "NaN"}, {2, "-Inf"}}},
	}
	got := AverageByDevice(series)
	if math.Abs(got["sda"]-0.005) > 1e-12 {
		t.Fatalf("sda mean %v", got["sda"])
	}
	if _, ok := got["sr0"]; ok {
		t.Fatalf("a device without finite samples must be absent, not zero: %v", got)
	}
}

func TestDiskWithoutSourceIsExplicit(t *testing.T) {
	b := Disk("", "", 5, 0, 60_000, nil)
	if b.Status != "unavailable" || b.WriteAwaitMs != nil || b.WritesPerS != nil {
		t.Fatalf("%+v", b)
	}
}

func fakeProm(t *testing.T, failAwait bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if r.URL.Path != "/api/v1/query_range" || r.URL.Query().Get("step") != "5" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if failAwait && strings.Contains(q, "write_time") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var result []PromSeries
		switch {
		case strings.Contains(q, "write_time"):
			result = []PromSeries{{Metric: map[string]string{"device": "sda"}, Values: [][]any{{1, "0.004"}, {2, "0.005"}}}}
		default:
			result = []PromSeries{
				{Metric: map[string]string{"device": "sda"}, Values: [][]any{{1, "200"}, {2, "200"}}},
				{Metric: map[string]string{"device": "sr0"}, Values: [][]any{{1, "0"}, {2, "0"}}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "matrix", "result": result},
		})
	}))
}

func TestDiskFromPrometheusKnownAnswer(t *testing.T) {
	srv := fakeProm(t, false)
	defer srv.Close()
	b := Disk(srv.URL, "10.0.0.1:9100", 5, 0, 60_000, nil)
	if b.Status != "ok" {
		t.Fatalf("status %+v", b)
	}
	if math.Abs(b.WriteAwaitMs["sda"]-4.5) > 1e-9 {
		t.Fatalf("await sda %v", b.WriteAwaitMs)
	}
	if _, ok := b.WriteAwaitMs["sr0"]; ok {
		t.Fatalf("sr0 has no writes and must have no await: %v", b.WriteAwaitMs)
	}
	if b.WritesPerS["sda"] != 200 || b.WritesPerS["sr0"] != 0 {
		t.Fatalf("writes %v", b.WritesPerS)
	}
	if len(b.Notes) != 1 || !strings.Contains(b.Notes[0], "sr0") {
		t.Fatalf("notes %v", b.Notes)
	}
}

func TestDiskPrometheusErrorIsUnavailable(t *testing.T) {
	srv := fakeProm(t, true)
	defer srv.Close()
	b := Disk(srv.URL, "10.0.0.1:9100", 5, 0, 60_000, nil)
	if b.Status != "unavailable" || !strings.Contains(b.Reason, "await query failed") || b.WriteAwaitMs != nil {
		t.Fatalf("%+v", b)
	}
}

// TestRetainedDiagWALReproduces recomputes the PostgreSQL block of a retained DIAG run from its raw pg-counters file
// and compares it with the committed diagnostic result. It skips when results/ is absent (a fresh clone).
func TestRetainedDiagWALReproduces(t *testing.T) {
	root := filepath.Join("..", "..", "results", "diag")
	const run = "DIAG-edgex-k4-r500b"
	diag := filepath.Join(root, run+".diag.json")
	pg := filepath.Join(root, "sut", run, "pg-counters.ndjson")
	summary := filepath.Join(root, run, "k6-summary.json")
	resolved := filepath.Join(root, run, "resolved.json")
	for _, p := range []string{diag, pg, summary, resolved} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("retained evidence not present: %v", err)
		}
	}
	var want struct {
		Postgres struct {
			WindowS          float64    `json:"window_s"`
			EdgeErrorMs      [2]float64 `json:"edge_error_ms"`
			CommitsPerS      float64    `json:"commits_per_s"`
			WALSyncsPerS     float64    `json:"wal_syncs_per_s"`
			WALSyncMsPerSync float64    `json:"wal_sync_ms_per_sync"`
			ShareOfWindow    float64    `json:"wal_sync_time_share_of_window"`
		} `json:"postgres"`
	}
	raw, err := os.ReadFile(diag)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	var sum struct {
		Steps []struct {
			ScenarioStartMs float64 `json:"scenario_start_ms"`
		} `json:"steps"`
	}
	var res struct {
		Steps []struct {
			DurationS float64 `json:"duration_s"`
		} `json:"steps"`
	}
	rb, _ := os.ReadFile(summary)
	if err := json.Unmarshal(rb, &sum); err != nil {
		t.Fatal(err)
	}
	rr, _ := os.ReadFile(resolved)
	if err := json.Unmarshal(rr, &res); err != nil {
		t.Fatal(err)
	}
	last := len(res.Steps) - 1
	t0 := sum.Steps[last].ScenarioStartMs
	t1 := t0 + res.Steps[last].DurationS*1000

	samples, _, err := ReadPGCounters(pg)
	if err != nil {
		t.Fatal(err)
	}
	b := WAL(samples, t0, t1)
	if b.Status != "ok" {
		t.Fatalf("%+v", b)
	}
	cmp := func(name string, got *float64, want float64) {
		t.Helper()
		if got == nil || math.Abs(*got-want) > 1e-6*math.Max(1, math.Abs(want)) {
			t.Fatalf("%s: got %v, retained %v", name, got, want)
		}
	}
	cmp("commits/s", b.CommitsPerS, want.Postgres.CommitsPerS)
	cmp("wal syncs/s", b.WALSyncsPerS, want.Postgres.WALSyncsPerS)
	cmp("ms/sync", b.WALSyncMsPerSync, want.Postgres.WALSyncMsPerSync)
	cmp("sync share", b.WALSyncTimeShare, want.Postgres.ShareOfWindow)
	if math.Abs(b.WindowS-want.Postgres.WindowS) > 1e-6 {
		t.Fatalf("window %v retained %v", b.WindowS, want.Postgres.WindowS)
	}
	if b.EdgeErrorMs == nil || math.Abs(b.EdgeErrorMs[0]-want.Postgres.EdgeErrorMs[0]) > 1e-6 || math.Abs(b.EdgeErrorMs[1]-want.Postgres.EdgeErrorMs[1]) > 1e-6 {
		t.Fatalf("edge error %v retained %v", b.EdgeErrorMs, want.Postgres.EdgeErrorMs)
	}
}

func TestWALStatsResetInsideWindowIsUnavailable(t *testing.T) {
	a := sample(1_000_000, 1000, 500, 1000)
	z := sample(1_060_000, 1600, 800, 2500)
	a.WAL.StatsReset, z.WAL.StatsReset = "2026-10-22T18:00:00Z", "2026-10-26T09:00:00Z"
	if b := WAL([]PGSample{a, z}, 1_000_000, 1_060_000); b.Status != "unavailable" || b.CommitsPerS != nil {
		t.Fatalf("a reset inside the window must not yield a rate: %+v", b)
	}
}

func TestWALDecreasingCounterIsUnavailable(t *testing.T) {
	a := sample(1_000_000, 1000, 500, 1000)
	z := sample(1_060_000, 900, 800, 2500) // commits went backwards: a reset without a stats_reset change
	if b := WAL([]PGSample{a, z}, 1_000_000, 1_060_000); b.Status != "unavailable" {
		t.Fatalf("a decreasing counter must be unavailable: %+v", b)
	}
}
