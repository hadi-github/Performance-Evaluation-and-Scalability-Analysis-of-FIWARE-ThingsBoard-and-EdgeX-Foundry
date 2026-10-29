// Integration tests: the generator (real k6 in Docker, pinned image) against the in-process fixture endpoint with
// deliberate faults. They prove that each fault is classified and flagged correctly (docs/methodology/generator.md, section 10).
// They need Docker and take a few minutes:  go test ./tests/integration -v      (skipped with -short)
package integration

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"iotbench/internal/fixture"
	"iotbench/internal/hash"
	"iotbench/internal/plan"
	"iotbench/internal/reconcile"
	"iotbench/internal/report"
	"iotbench/internal/runner"
)

const image = "docker.arvancloud.ir/grafana/k6:1.8.1"

func repo(t *testing.T) string {
	d, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func needDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test (needs Docker)")
	}
	if exec.Command("docker", "version").Run() != nil {
		t.Skip("docker not available")
	}
}

type target struct {
	url   string
	store string
	srv   *http.Server
	fx    *fixture.Server
	f     *os.File
}

func startFixture(t *testing.T, c fixture.Config) *target {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "store.ndjson")
	f, err := os.Create(store)
	if err != nil {
		t.Fatal(err)
	}
	fx := fixture.New(c, f, 1<<20)
	srv := &http.Server{Handler: fx}
	go srv.Serve(ln)
	return &target{url: "http://" + ln.Addr().String(), store: store, srv: srv, fx: fx, f: f}
}

// stop shuts the server down and returns the stored identities.
func (tg *target) stop(t *testing.T) []reconcile.Stored {
	t.Helper()
	_ = tg.srv.Close()
	time.Sleep(200 * time.Millisecond)
	tg.fx.Close()
	tg.f.Close()
	if tg.fx.Overflow() > 0 {
		t.Fatalf("fixture lost %d records", tg.fx.Overflow())
	}
	recs, err := fixture.ReadRecords(tg.store)
	if err != nil {
		t.Fatal(err)
	}
	var out []reconcile.Stored
	for _, r := range recs {
		if r.Stored && r.Seq >= 0 {
			out = append(out, reconcile.Stored{RunID: r.RunID, Seq: r.Seq})
		}
	}
	return out
}

func basePlan(url string, rate float64, seconds int) plan.Plan {
	return plan.Plan{
		SchemaVersion: 1, PlanID: "itest", Purpose: "integration test", PilotOnly: true, Seed: 99,
		Adapter: "fixture", Target: plan.Target{BaseURL: url}, Devices: 10, RequestTimeMs: 1000,
		Connection: plan.Connection{KeepAlive: true}, Arrivals: plan.Arrivals{Model: "constant"},
		VUs:         plan.VUs{PreAllocated: 50, Max: 50},
		Schedule:    plan.Schedule{Type: "steps", Steps: []plan.StepSpec{{Phase: "step", Rate: rate, DurationS: seconds}}},
		Health:      plan.Health{MinAchievedRatio: 0.99, SendLagP99Ms: 10, EarlyToleranceMs: 2},
		HistogramMs: []float64{0, 1, 2, 3, 5, 7, 10, 15, 20, 30, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 2000, 5000},
	}
}

func run(t *testing.T, p plan.Plan, cpus string) (string, *runner.Manifest) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	b, _ := json.Marshal(p)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	d, m, err := runner.Run(runner.Options{PlanPath: path, RepoRoot: repo(t), ResultsDir: filepath.Join(dir, "results"),
		Image: image, Cpuset: "0-1", CPUs: cpus})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return d, m
}

func records(t *testing.T, dir string) []report.Record {
	t.Helper()
	r, err := report.ReadNDJSON[report.Record](filepath.Join(dir, "records.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func hasReason(m *runner.Manifest, check string) bool {
	for _, r := range m.Health.Reasons {
		if strings.Contains(r, check) {
			return true
		}
	}
	return false
}

func TestCleanRunIsValidAndReconciles(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{})
	d, m := run(t, basePlan(tg.url, 100, 5), "")
	st := tg.stop(t)
	if m.Verdict != "VALID" {
		t.Fatalf("verdict %s: %v", m.Verdict, m.Health.Reasons)
	}
	res, err := reconcile.Run(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Clean || res.Planned != 500 || res.AckSuccess != 500 || res.StoredUnique != 500 {
		t.Fatalf("reconcile: %+v", res)
	}
}

// Response delayed beyond the client timeout, but the message is stored -> unknown, and found persisted.
func TestTimeoutButStoredIsUnknownAndPersisted(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{SlowEvery: 50, SlowDelay: 3 * time.Second})
	d, m := run(t, basePlan(tg.url, 50, 6), "")
	st := tg.stop(t)
	if m.Verdict != "VALID" {
		t.Fatalf("generator health must stay VALID (the SUT was slow, not the generator): %v", m.Health.Reasons)
	}
	recs := records(t, d)
	if len(recs) != 6 { // seq 0, 50, ..., 250 of 300
		t.Fatalf("want 6 unknown records, got %d", len(recs))
	}
	for _, r := range recs {
		if r.Outcome != "unknown" || r.Status != 0 || r.Seq%50 != 0 {
			t.Fatalf("bad record %+v", r)
		}
	}
	res, err := reconcile.Run(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PersistedDespiteUnknown) != 6 || len(res.AckLost) != 0 {
		t.Fatalf("reconcile: %+v", res)
	}
}

func TestServerErrorIsExplicitFailure(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{FailEvery: 40, FailStatus: 503})
	d, m := run(t, basePlan(tg.url, 100, 4), "")
	st := tg.stop(t)
	if m.Verdict != "VALID" {
		t.Fatalf("verdict: %v", m.Health.Reasons)
	}
	recs := records(t, d)
	if len(recs) != 10 { // seq 0, 40, ..., 360 of 400
		t.Fatalf("want 10 failure records, got %d", len(recs))
	}
	for _, r := range recs {
		if r.Outcome != "explicit_failure" || r.Status != 503 {
			t.Fatalf("bad record %+v", r)
		}
	}
	res, err := reconcile.Run(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExplicitFailure != 10 || len(res.PersistedDespiteFailure) != 0 || len(res.AckLost) != 0 {
		t.Fatalf("reconcile: %+v", res)
	}
}

// A 207 batch answer counts as success only if every item is 2xx (the EdgeX case).
func TestBatch207ItemFailureIsExplicitFailure(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{Mode207: true, Fail207Every: 25})
	p := basePlan(tg.url, 100, 4)
	p.Target.ExpectBatch207 = true
	d, m := run(t, p, "")
	tg.stop(t)
	recs := records(t, d)
	if len(recs) != 16 { // seq 0, 25, ..., 375 of 400
		t.Fatalf("want 16 failure records, got %d (%v)", len(recs), m.Health.Reasons)
	}
	for _, r := range recs {
		if r.Outcome != "explicit_failure" || r.Status != 207 || r.Detail != "item_status_500" {
			t.Fatalf("bad record %+v", r)
		}
	}
	if m.Health.Steps[0].Outcomes["ack_success"] != 384 {
		t.Fatalf("ack: %v", m.Health.Steps[0].Outcomes)
	}
}

// The endpoint slows down until VUs run out: drops must be detected and the run flagged INVALID.
func TestSlowdownUntilVUsRunOutIsInvalid(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{RampTo: 2 * time.Second, RampOver: 3 * time.Second})
	p := basePlan(tg.url, 100, 8)
	p.VUs = plan.VUs{PreAllocated: 10, Max: 10}
	p.RequestTimeMs = 5000
	d, m := run(t, p, "")
	tg.stop(t)
	if m.Verdict != "INVALID" || !hasReason(m, "dropped_iterations") {
		t.Fatalf("want INVALID with dropped_iterations, got %s %v", m.Verdict, m.Health.Reasons)
	}
	if _, err := reconcile.Run(d, nil); err == nil {
		t.Fatal("reconciliation must refuse a run with dropped iterations")
	}
}

func TestDuplicateSeqIsDetectedDownstream(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{})
	p := basePlan(tg.url, 100, 3)
	p.TestFaults.DuplicateSeqs = []int64{10, 77}
	d, m := run(t, p, "")
	st := tg.stop(t)
	if m.Verdict != "VALID" {
		t.Fatalf("verdict: %v", m.Health.Reasons)
	}
	res, err := reconcile.Run(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Duplicates) != 2 || res.Duplicates[0] != 10 || res.Duplicates[1] != 77 || res.Clean {
		t.Fatalf("reconcile: %+v", res)
	}
}

// One deliberately missing delivery (200 to the generator, never stored), offset by one duplicate: stored
// counts stay EQUAL while identities differ. Equal row counts must never pass for identity equality.
func TestMissingDeliveryOffsetByDuplicateKeepsCountsEqualButNotClean(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{DropSeqs: []int64{7}})
	p := basePlan(tg.url, 100, 3)
	p.TestFaults.DuplicateSeqs = []int64{42}
	d, m := run(t, p, "")
	st := tg.stop(t)
	if m.Verdict != "VALID" {
		t.Fatalf("verdict: %v", m.Health.Reasons)
	}
	res, err := reconcile.Run(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if res.StoredUnique+res.DuplicateCopies != res.Planned || res.StoredUnique != res.Planned-1 {
		t.Fatalf("fixture must keep stored ROW counts equal to planned (%d rows vs %d planned, %d unique)",
			res.StoredUnique+res.DuplicateCopies, res.Planned, res.StoredUnique)
	}
	if len(res.AckLost) != 1 || res.AckLost[0] != 7 || len(res.Duplicates) != 1 || res.Duplicates[0] != 42 || res.Clean {
		t.Fatalf("reconcile: %+v", res)
	}
}

// Every n-th stored row loses its run_id: the row exists, the identity does not. It must stay in its own
// unaccountable category and must not be reported as a lost message.
func TestMalformedIdentityIsStoredButUnaccountable(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{MalformedEvery: 50})
	d, m := run(t, basePlan(tg.url, 100, 4), "")
	st := tg.stop(t)
	if m.Verdict != "VALID" {
		t.Fatalf("verdict: %v", m.Health.Reasons)
	}
	res, err := reconcile.Run(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.MalformedIdentities) != 8 { // seqs 0,50,...,350 of 400
		t.Fatalf("want 8 malformed-identity rows, got %+v", res.MalformedIdentities)
	}
	if len(res.AckLost) != 0 || res.Clean {
		t.Fatalf("malformed rows must be reattributed, not counted as ack-lost: %+v", res)
	}
	if res.StoredUnique != 392 { // 400 minus the 8 unidentifiable rows
		t.Fatalf("stored unique %d", res.StoredUnique)
	}
}

func TestEarlySendIsInvalid(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{})
	p := basePlan(tg.url, 100, 3)
	p.TestFaults.EarlyEvery = 100
	d, m := run(t, p, "")
	tg.stop(t)
	if m.Verdict != "INVALID" || !hasReason(m, "no_early_sends") {
		t.Fatalf("want INVALID (early sends), got %s %v", m.Verdict, m.Health.Reasons)
	}
	n := 0
	for _, r := range records(t, d) {
		if r.Early {
			n++
		}
	}
	if n != 3 { // seq 0, 100, 200 of 300
		t.Fatalf("want 3 early records, got %d", n)
	}
}

// A starved generator: no drops (plenty of VUs), but the send-lag gate must catch it.
func TestStarvedGeneratorFailsSendLagGate(t *testing.T) {
	needDocker(t)
	tg := startFixture(t, fixture.Config{})
	p := basePlan(tg.url, 300, 6)
	p.VUs = plan.VUs{PreAllocated: 300, Max: 300}
	_, m := run(t, p, "0.15")
	tg.stop(t)
	if m.Verdict != "INVALID" || !hasReason(m, "send_lag_p99") {
		t.Fatalf("want INVALID (send lag), got %s %v", m.Verdict, m.Health.Reasons)
	}
}

// The k6 hash (workloads/k6/lib/hash.js) must equal internal/hash bit for bit, or thinning sets and payload
// values could not be reproduced offline.
func TestHashParityGoVsK6(t *testing.T) {
	needDocker(t)
	dir := t.TempDir()
	script := `import { mix, values, threshold, selected } from '/k6/lib/hash.js';
export const options = { vus: 1, iterations: 1 };
export default function () {}
export function handleSummary() {
  const out = [];
  for (const seed of [0, 1, 99, 4294967295]) for (let seq = 0; seq < 2000; seq++) {
    const v = values(seed, seq);
    out.push([seed, seq, mix(seed, seq, 0x53454c31), v.temperature, v.humidity, v.pressure, selected(seed, seq, threshold(0.1)) ? 1 : 0]);
  }
  return { '/out/h.json': JSON.stringify(out) };
}`
	if err := os.WriteFile(filepath.Join(dir, "parity.js"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("docker", "run", "--rm", "-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "-v", filepath.Join(repo(t), "workloads/k6")+":/k6:ro",
		"-v", dir+":/out", image, "run", "-q", "/out/parity.js")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("k6: %v %s", err, out)
	}
	var rows [][]float64
	b, err := os.ReadFile(filepath.Join(dir, "h.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	th := hash.Threshold(0.1)
	for _, r := range rows {
		seed, seq := uint32(r[0]), uint32(r[1])
		tt, h, p := hash.Values(seed, seq)
		sel := 0.0
		if hash.Selected(seed, seq, th) {
			sel = 1
		}
		if float64(hash.Mix(seed, seq, hash.SaltSelect)) != r[2] || float64(tt) != r[3] || float64(h) != r[4] || float64(p) != r[5] || sel != r[6] {
			t.Fatalf("mismatch at seed %d seq %d: k6 %v", seed, seq, r)
		}
	}
	if len(rows) != 8000 {
		t.Fatalf("rows %d", len(rows))
	}
}
