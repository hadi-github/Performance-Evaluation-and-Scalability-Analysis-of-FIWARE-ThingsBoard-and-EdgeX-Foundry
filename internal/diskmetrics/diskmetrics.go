// Package diskmetrics reports per-run disk and PostgreSQL WAL measurements for EdgeX (D29a, 2026-10-26).
//
// Two different sources are kept apart and labelled as such:
//   - host/device disk statistics from node-exporter (all writes on the device, not only PostgreSQL WAL);
//   - PostgreSQL WAL and commit counters from the diagnostic pg-counters series (only when that overlay ran).
//
// Formulas match scripts/diag/analyze.py so the retained diagnostic results can be reproduced. Nothing here changes
// a verdict: the output is informational and is read by no pass/fail rule. A missing source is reported with a
// reason and never converted to zero.
package diskmetrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SchemaVersion of resources.json. Bump on any field change.
const SchemaVersion = 1

// Sources state the measurement scope in the output itself.
const (
	DiskSource = "node-exporter: host block device, all writes (not only PostgreSQL WAL); averages of 30 s rates " +
		"sampled every 5 s over the step window"
	WALSource = "pg-counters: PostgreSQL cluster counters from the diagnostic overlay (pg_stat_database, pg_stat_wal), " +
		"differenced at the samples nearest the window edges"
)

// StepWindow is one measured step of a run, in epoch milliseconds.
type StepWindow struct {
	Name      string  `json:"name"`
	Phase     string  `json:"phase"`
	StartMs   float64 `json:"start_ms"`
	DurationS float64 `json:"duration_s"`
}

// DiskBlock is host/device disk activity. Status is "ok" or "unavailable"; Reason explains the latter.
type DiskBlock struct {
	Source       string             `json:"source"`
	Status       string             `json:"status"`
	Reason       string             `json:"reason,omitempty"`
	StepS        int                `json:"query_step_s"`
	WriteAwaitMs map[string]float64 `json:"disk_write_await_ms,omitempty"`
	WritesPerS   map[string]float64 `json:"disk_writes_per_s,omitempty"`
	Notes        []string           `json:"notes,omitempty"`
}

// WALBlock is PostgreSQL WAL and commit activity over the window. Pointer fields are nil when undefined (no sync or
// no commit in the window) rather than zero.
type WALBlock struct {
	Source           string      `json:"source"`
	Status           string      `json:"status"`
	Reason           string      `json:"reason,omitempty"`
	SampleIntervalS  float64     `json:"sample_interval_s,omitempty"`
	WindowS          float64     `json:"window_s,omitempty"`
	EdgeErrorMs      *[2]float64 `json:"edge_error_ms,omitempty"`
	CommitsPerS      *float64    `json:"commits_per_s,omitempty"`
	WALSyncsPerS     *float64    `json:"wal_syncs_per_s,omitempty"`
	WALSyncMsPerSync *float64    `json:"wal_sync_ms_per_sync,omitempty"`
	WALSyncTimeShare *float64    `json:"wal_sync_time_share_of_window,omitempty"`
	CommitsPerSync   *float64    `json:"commits_per_sync,omitempty"`
}

// StepResources groups both sources for one step.
type StepResources struct {
	Name    string    `json:"name"`
	Phase   string    `json:"phase"`
	StartMs float64   `json:"window_start_ms"`
	EndMs   float64   `json:"window_end_ms"`
	Disk    DiskBlock `json:"host_disk"`
	WAL     WALBlock  `json:"postgres_wal"`
}

// Resources is the per-run document written as resources.json.
type Resources struct {
	SchemaVersion int             `json:"schema_version"`
	RunID         string          `json:"run_id"`
	Platform      string          `json:"platform"`
	Scope         string          `json:"scope"`
	Steps         []StepResources `json:"steps"`
}

// PGSample is one pg-counters record: the fields used here, as written by scripts/diag/sql/pg-counters.sql.
type PGSample struct {
	TMs float64 `json:"t_ms"`
	DB  struct {
		XactCommit int64 `json:"xact_commit"`
	} `json:"db"`
	WAL struct {
		WALWrite     int64   `json:"wal_write"`
		WALSync      int64   `json:"wal_sync"`
		WALWriteTime float64 `json:"wal_write_time"`
		WALSyncTime  float64 `json:"wal_sync_time"`
		StatsReset   string  `json:"stats_reset"`
	} `json:"wal"`
}

// ReadPGCounters parses a pg-counters.ndjson file. json_agg output spans several lines, so a record starts at a line
// beginning with {"t_ms" (the same rule as scripts/diag/analyze.py). Unparseable chunks are skipped and counted.
func ReadPGCounters(path string) (samples []PGSample, skipped int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var chunks []string
	var cur strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, `{"t_ms"`) && cur.Len() > 0 {
			chunks = append(chunks, cur.String())
			cur.Reset()
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	for _, c := range chunks {
		var s PGSample
		if json.Unmarshal([]byte(strings.TrimSpace(c)), &s) != nil || s.TMs == 0 {
			skipped++
			continue
		}
		samples = append(samples, s)
	}
	sort.SliceStable(samples, func(i, j int) bool { return samples[i].TMs < samples[j].TMs })
	return samples, skipped, nil
}

// nearest returns the sample whose time is closest to t (the first one on ties), as analyze.py's nearest().
func nearest(samples []PGSample, t float64) (PGSample, bool) {
	if len(samples) == 0 {
		return PGSample{}, false
	}
	best := samples[0]
	for _, s := range samples[1:] {
		if math.Abs(s.TMs-t) < math.Abs(best.TMs-t) {
			best = s
		}
	}
	return best, true
}

// WAL computes the PostgreSQL block for the window [t0, t1] (epoch ms). It needs two samples that differ in time.
func WAL(samples []PGSample, t0, t1 float64) WALBlock {
	b := WALBlock{Source: WALSource, SampleIntervalS: 10}
	if len(samples) < 2 {
		b.Status, b.Reason = "unavailable", "no usable pg-counters samples (no file, an empty file, or fewer than two records)"
		return b
	}
	a, _ := nearest(samples, t0)
	z, _ := nearest(samples, t1)
	dt := (z.TMs - a.TMs) / 1000
	if dt <= 0 {
		b.Status, b.Reason = "unavailable", "the two nearest samples do not bracket the window"
		return b
	}
	if z.WAL.StatsReset != a.WAL.StatsReset {
		b.Status, b.Reason = "unavailable", "pg_stat_wal was reset inside the window (stats_reset changed)"
		return b
	}
	commits := z.DB.XactCommit - a.DB.XactCommit
	syncs := z.WAL.WALSync - a.WAL.WALSync
	syncMs := z.WAL.WALSyncTime - a.WAL.WALSyncTime
	if commits < 0 || syncs < 0 || syncMs < 0 {
		b.Status, b.Reason = "unavailable", "a cumulative counter decreased inside the window"
		return b
	}
	b.Status = "ok"
	b.WindowS = dt
	edge := [2]float64{a.TMs - t0, z.TMs - t1}
	b.EdgeErrorMs = &edge
	commitsF, syncsF := float64(commits), float64(syncs)
	cps := commitsF / dt
	sps := syncsF / dt
	share := syncMs / 1000 / dt
	b.CommitsPerS, b.WALSyncsPerS, b.WALSyncTimeShare = &cps, &sps, &share
	if syncs > 0 {
		msps := syncMs / syncsF
		b.WALSyncMsPerSync = &msps
	}
	if syncs > 0 && commits > 0 {
		cpsync := commitsF / syncsF
		b.CommitsPerSync = &cpsync
	}
	return b
}

// PromSeries is one matrix series returned by the Prometheus HTTP API.
type PromSeries struct {
	Metric map[string]string `json:"metric"`
	Values [][]any           `json:"values"`
}

// PromRange runs a range query against a Prometheus-compatible base URL.
func PromRange(base, query string, t0, t1 float64, stepS int, client *http.Client) ([]PromSeries, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	q := url.Values{}
	q.Set("query", query)
	q.Set("start", strconv.FormatFloat(t0/1000, 'f', 3, 64))
	q.Set("end", strconv.FormatFloat(t1/1000, 'f', 3, 64))
	q.Set("step", strconv.Itoa(stepS))
	resp, err := client.Get(strings.TrimRight(base, "/") + "/api/v1/query_range?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus HTTP %d", resp.StatusCode)
	}
	var env struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []PromSeries `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("prometheus response: %w", err)
	}
	if env.Status != "success" {
		return nil, fmt.Errorf("prometheus status %q: %s", env.Status, env.Error)
	}
	return env.Data.Result, nil
}

// AverageByDevice returns, per "device" label, the mean of the finite sample values. Series with no finite samples are
// omitted (so an undefined ratio, such as await on a device with zero writes, is absent rather than zero).
func AverageByDevice(series []PromSeries) map[string]float64 {
	out := map[string]float64{}
	for _, s := range series {
		sum, n := 0.0, 0
		for _, v := range s.Values {
			if len(v) < 2 {
				continue
			}
			str, ok := v[1].(string)
			if !ok {
				continue
			}
			f, err := strconv.ParseFloat(str, 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				continue
			}
			sum += f
			n++
		}
		if n > 0 {
			out[s.Metric["device"]] = sum / float64(n)
		}
	}
	return out
}

// DiskQueries returns the PromQL used for each measurement, matching scripts/diag/analyze.py.
func DiskQueries(instance string) (await, writes string) {
	await = fmt.Sprintf(`rate(node_disk_write_time_seconds_total{instance=%q}[30s]) / rate(node_disk_writes_completed_total{instance=%q}[30s])`, instance, instance)
	writes = fmt.Sprintf(`rate(node_disk_writes_completed_total{instance=%q}[30s])`, instance)
	return
}

// Disk queries Prometheus for one step window. base or instance empty means the source was not configured.
func Disk(base, instance string, stepS int, startMs, endMs float64, client *http.Client) DiskBlock {
	b := DiskBlock{Source: DiskSource, StepS: stepS}
	if base == "" || instance == "" {
		b.Status, b.Reason = "unavailable", "no Prometheus URL or node instance supplied"
		return b
	}
	await, writes := DiskQueries(instance)
	aw, err := PromRange(base, await, startMs, endMs, stepS, client)
	if err != nil {
		b.Status, b.Reason = "unavailable", "await query failed: "+err.Error()
		return b
	}
	wr, err := PromRange(base, writes, startMs, endMs, stepS, client)
	if err != nil {
		b.Status, b.Reason = "unavailable", "writes query failed: "+err.Error()
		return b
	}
	if len(aw) == 0 && len(wr) == 0 {
		b.Status, b.Reason = "unavailable", "no series returned (exporter not running, or instance label does not match)"
		return b
	}
	b.WriteAwaitMs = map[string]float64{}
	for dev, v := range AverageByDevice(aw) {
		b.WriteAwaitMs[dev] = v * 1000
	}
	b.WritesPerS = AverageByDevice(wr)
	if len(b.WriteAwaitMs) == 0 && len(b.WritesPerS) == 0 {
		b.Status, b.Reason = "unavailable", "series returned but no finite sample in the window"
		return b
	}
	b.Status = "ok"
	for dev, w := range b.WritesPerS {
		if _, ok := b.WriteAwaitMs[dev]; !ok && w == 0 {
			b.Notes = append(b.Notes, fmt.Sprintf("%s: no writes in the window, so await is undefined", dev))
		}
	}
	sort.Strings(b.Notes)
	return b
}
