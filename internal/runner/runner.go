// Package runner executes one generator run: resolve the plan, run k6 in Docker (pinned image, pinned cores),
// sample the generator's CPU per core and k6's memory, split the k6 log into message records, and write the
// manifest with the health verdict. A run directory is never reused: results/<run_id>/ must not exist.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"iotbench/internal/plan"
	"iotbench/internal/protocol"
	"iotbench/internal/report"
)

type Options struct {
	CPUSnapshotCommand string // optional trusted shell command returning one SUT snapshot TSV
	PlanPath           string
	RepoRoot           string   // contains workloads/k6
	ResultsDir         string   // run directories are created inside
	RunID              string   // optional; generated if empty
	Image              string   // k6 image reference
	Cpuset             string   // generator cores, e.g. "0-1"
	FullJSON           bool     // also write k6's full JSON output (calibration runs only: 13 samples per request)
	CPUs               string   // docker --cpus limit for k6; tests only (deliberate generator starvation)
	PromRW             string   // optional Prometheus remote-write URL (k6 -o experimental-prometheus-rw); monitoring only
	GrafanaURL         string   // optional: post a run annotation; password from env BENCH_GRAFANA_PASSWORD
	Env                []string // passed to k6 (credentials, never stored in artifacts)
	Stdout             *os.File
}

type Manifest struct {
	CPUSnapshotCommand          string                    `json:"cpu_snapshot_command,omitempty"`
	SchemaVersion               int                       `json:"schema_version"`
	RunID                       string                    `json:"run_id"`
	PlanID                      string                    `json:"plan_id"`
	PilotOnly                   bool                      `json:"pilot_only"`
	PlanSHA256                  string                    `json:"plan_sha256"`
	Deviations                  []string                  `json:"deviations,omitempty"` // from the plan
	Resolved                    *plan.Resolved            `json:"resolved_plan"`
	K6                          map[string]string         `json:"k6"`
	Git                         map[string]string         `json:"git"`
	ScriptsSHA256               string                    `json:"k6_scripts_sha256"` // content of workloads/k6 as executed (independent of git)
	Host                        map[string]string         `json:"generator_host"`
	Cpuset                      string                    `json:"generator_cpuset"`
	CPUsLimit                   string                    `json:"generator_cpus_limit,omitempty"`
	PromRW                      string                    `json:"prometheus_remote_write,omitempty"`
	Annotation                  string                    `json:"grafana_annotation,omitempty"`
	FullJSON                    bool                      `json:"full_k6_json_output"`
	Synthetic                   bool                      `json:"synthetic,omitempty"`
	OfflineFixture              bool                      `json:"offline_engineering_fixture,omitempty"`
	EnvKeys                     []string                  `json:"env_keys_passed"` // names only, values redacted
	Connection                  map[string]any            `json:"connection_model"`
	Payload                     *report.Sample            `json:"sample_payload"`
	StartUTC                    string                    `json:"start_utc"`
	EndUTC                      string                    `json:"end_utc"`
	K6ExitCode                  int                       `json:"k6_exit_code"`
	Health                      *report.Health            `json:"health"`
	Verdict                     string                    `json:"verdict"`
	ProtocolBinding             *protocol.Binding         `json:"protocol_binding,omitempty"`
	ProtocolPilot               *protocol.PilotParameters `json:"protocol_pilot_parameters,omitempty"`
	RuntimeMetricInterpretation string                    `json:"runtime_metric_interpretation"`
}

const LegacyMetricInterpretation = "legacy-acknowledgement-only-v1"

func run(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitInfo(repo string) map[string]string {
	sha := run("git", "-C", repo, "rev-parse", "HEAD")
	if sha != "" {
		dirty := "false"
		if run("git", "-C", repo, "status", "--porcelain") != "" {
			dirty = "true"
		}
		return map[string]string{"sha": sha, "dirty": dirty, "source": "git"}
	}
	// Deployed copies have no .git: deploy writes GIT_REV ("<sha> <dirty>").
	if b, err := os.ReadFile(filepath.Join(repo, "GIT_REV")); err == nil {
		f := strings.Fields(string(b))
		if len(f) >= 2 {
			return map[string]string{"sha": f[0], "dirty": f[1], "source": "GIT_REV file"}
		}
	}
	return map[string]string{"sha": "unknown", "dirty": "unknown", "source": "none"}
}

func hostInfo() map[string]string {
	h := map[string]string{"go_os_arch": runtime.GOOS + "/" + runtime.GOARCH}
	h["hostname"], _ = os.Hostname()
	h["kernel"] = run("uname", "-r")
	h["nproc"] = strconv.Itoa(runtime.NumCPU())
	h["docker"] = run("docker", "version", "--format", "{{.Server.Version}}")
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "model name") {
				h["cpu_model"] = strings.TrimSpace(strings.SplitN(l, ":", 2)[1])
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		h["mem_total"] = strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(b), "\n", 2)[0], "MemTotal:"))
	}
	return h
}

func k6Info(image string) (map[string]string, error) {
	if run("docker", "image", "inspect", image) == "" {
		if out, err := exec.Command("docker", "pull", "-q", image).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("docker pull %s: %v: %s", image, err, out)
		}
	}
	digest := run("docker", "image", "inspect", "--format", "{{range .RepoDigests}}{{.}} {{end}}", image)
	return map[string]string{
		"image":    image,
		"digest":   strings.TrimSpace(digest),
		"image_id": run("docker", "image", "inspect", "--format", "{{.Id}}", image),
		"version":  run("docker", "run", "--rm", image, "version"),
	}, nil
}

// scriptsHash hashes every file under dir (relative path + content, sorted), i.e. the k6 code actually executed.
func scriptsHash(dir string) string {
	h := sha256.New()
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "unreadable: " + err.Error()
		}
		rel, _ := filepath.Rel(dir, f)
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ---- generator sampling: per-core busy fraction from /proc/stat, k6 container memory from its cgroup

type cpuTimes struct{ busy, total uint64 }

func readCPU() []cpuTimes {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil
	}
	var out []cpuTimes
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 8 || !strings.HasPrefix(f[0], "cpu") || f[0] == "cpu" {
			continue
		}
		var v [8]uint64
		for i := 0; i < 8; i++ {
			v[i], _ = strconv.ParseUint(f[i+1], 10, 64)
		}
		total := v[0] + v[1] + v[2] + v[3] + v[4] + v[5] + v[6] + v[7]
		out = append(out, cpuTimes{busy: total - v[3] - v[4], total: total})
	}
	return out
}

func containerMem(name string) int64 {
	pid := run("docker", "inspect", "-f", "{{.State.Pid}}", name)
	if pid == "" || pid == "0" {
		return 0
	}
	b, err := os.ReadFile("/proc/" + pid + "/cgroup")
	if err != nil {
		return 0
	}
	path := strings.TrimPrefix(strings.TrimSpace(string(b)), "0::")
	m, err := os.ReadFile(filepath.Join("/sys/fs/cgroup", path, "memory.current"))
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(m)), 10, 64)
	return n
}

func sample(ctx context.Context, container string, out *os.File) {
	w := bufio.NewWriter(out)
	enc := json.NewEncoder(w)
	prev := readCPU()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	defer w.Flush()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cur := readCPU()
			s := report.CPUSample{Ms: float64(now.UnixNano()) / 1e6, K6MemB: containerMem(container)}
			for i := range cur {
				if i < len(prev) && cur[i].total > prev[i].total {
					s.CoreBusy = append(s.CoreBusy, float64(cur[i].busy-prev[i].busy)/float64(cur[i].total-prev[i].total))
				} else {
					s.CoreBusy = append(s.CoreBusy, 0)
				}
			}
			prev = cur
			_ = enc.Encode(s)
		}
	}
}

// splitLog separates message records (console.log JSON with "rec") from k6's other log lines.
func splitLog(dir string) error {
	in, err := os.Open(filepath.Join(dir, "k6-log.ndjson"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer in.Close()
	recs, err := os.Create(filepath.Join(dir, "records.ndjson"))
	if err != nil {
		return err
	}
	defer recs.Close()
	msgs, err := os.Create(filepath.Join(dir, "k6-messages.log"))
	if err != nil {
		return err
	}
	defer msgs.Close()
	rw, mw := bufio.NewWriter(recs), bufio.NewWriter(msgs)
	defer rw.Flush()
	defer mw.Flush()
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l struct {
			Msg    string `json:"msg"`
			Source string `json:"source"`
		}
		line := sc.Bytes()
		if json.Unmarshal(line, &l) == nil && l.Source == "console" && strings.HasPrefix(l.Msg, `{"rec":`) {
			rw.WriteString(l.Msg)
			rw.WriteByte('\n')
			continue
		}
		mw.Write(line)
		mw.WriteByte('\n')
	}
	return sc.Err()
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

type preparedRun struct {
	dir        string
	plan       *plan.Plan
	resolved   *plan.Resolved
	planSHA256 string
}

// prepareRun performs the non-Docker part of a run: strict plan decode, resolution, protocol
// binding verification/eligibility, and creation of a new run directory containing plan.json
// and resolved.json. Run and PrepareOfflineFixture share it so offline fixtures exercise the
// same plan identity and protocol metadata flow.
func prepareRun(o Options) (*preparedRun, error) {
	raw, err := os.ReadFile(o.PlanPath)
	if err != nil {
		return nil, err
	}
	p, err := plan.Decode(raw)
	if err != nil {
		return nil, err
	}
	runID := o.RunID
	if runID == "" {
		b := make([]byte, 2)
		_, _ = rand.Read(b)
		runID = fmt.Sprintf("%s-%s-%s", p.PlanID, time.Now().UTC().Format("20060102t150405z"), hex.EncodeToString(b))
	}
	r, err := p.Resolve(runID)
	if err != nil {
		return nil, err
	}
	if r.TotalTicks >= 1<<32 {
		return nil, fmt.Errorf("plan has %d ticks; seq must stay below 2^32", r.TotalTicks)
	}
	if r.ProtocolBinding != nil {
		definitionPath := filepath.Join(o.RepoRoot, "research", "protocols", "iotbench-telemetry-v0.1.0-pilot.1.json")
		loaded, loadErr := protocol.LoadVerified(definitionPath, o.RepoRoot)
		if loadErr != nil {
			return nil, fmt.Errorf("protocol binding verification: %w", loadErr)
		}
		if err = protocol.CheckEligibility(loaded, *r.ProtocolBinding, r.ProtocolPilot, protocol.EvidenceReferences{}); err != nil {
			return nil, fmt.Errorf("protocol binding eligibility: %w", err)
		}
	}
	dir := filepath.Join(o.ResultsDir, runID)
	if err := os.MkdirAll(o.ResultsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, fmt.Errorf("run directory: %w (a run directory is never reused)", err)
	}
	sum := sha256.Sum256(raw)
	if err := os.WriteFile(filepath.Join(dir, "plan.json"), raw, 0o644); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(dir, "resolved.json"), r); err != nil {
		return nil, err
	}
	return &preparedRun{dir: dir, plan: p, resolved: r, planSHA256: hex.EncodeToString(sum[:])}, nil
}

func newManifest(o Options, prep *preparedRun, k6, host map[string]string, scripts string) *Manifest {
	m := &Manifest{SchemaVersion: 1, RunID: prep.resolved.RunID, PlanID: prep.plan.PlanID, PilotOnly: prep.plan.PilotOnly, Deviations: prep.plan.Deviations,
		PlanSHA256: prep.planSHA256, Resolved: prep.resolved, K6: k6, Git: gitInfo(o.RepoRoot), Host: host,
		Cpuset: o.Cpuset, CPUsLimit: o.CPUs, FullJSON: o.FullJSON, PromRW: o.PromRW,
		ScriptsSHA256:   scripts,
		ProtocolBinding: prep.plan.ProtocolBinding, ProtocolPilot: prep.plan.ProtocolPilot, RuntimeMetricInterpretation: LegacyMetricInterpretation,
		Connection: map[string]any{"keep_alive": prep.plan.Connection.KeepAlive, "connections": "one per VU (k6 VU = one in-flight request)",
			"http": "HTTP/1.1", "retries": "none (k6 never retries; one iteration = one request)", "request_timeout_ms": prep.plan.RequestTimeMs}}
	m.CPUSnapshotCommand = o.CPUSnapshotCommand
	for _, kv := range o.Env {
		m.EnvKeys = append(m.EnvKeys, strings.SplitN(kv, "=", 2)[0])
	}
	return m
}

// OfflineFixture is a non-live run directory built for offline pipeline tests. It contains
// plan/resolved/manifest metadata produced by the same preparation code as Run. k6 identity
// fields are explicitly synthetic: no generator, Docker or service was invoked.
type OfflineFixture struct {
	Dir        string
	Manifest   *Manifest
	Plan       *plan.Plan
	Resolved   *plan.Resolved
	PlanSHA256 string
}

// PrepareOfflineFixture performs plan decode, resolution, protocol-binding verification and
// runner manifest construction without contacting Docker, k6 or any service. It writes only
// plan.json, resolved.json and manifest.json into a new run directory. The manifest is marked
// synthetic and offline; callers must treat the result as an engineering fixture, never live
// evidence, and must supply any additional fixture artifacts they intend to analyze.
func PrepareOfflineFixture(o Options) (*OfflineFixture, error) {
	prep, err := prepareRun(o)
	if err != nil {
		return nil, err
	}
	k6 := map[string]string{
		"mode":     "offline-engineering-fixture",
		"executed": "false",
		"image":    "synthetic/no-k6",
		"digest":   "",
		"image_id": "",
		"version":  "",
	}
	host := map[string]string{
		"mode":       "offline-engineering-fixture",
		"go_os_arch": runtime.GOOS + "/" + runtime.GOARCH,
	}
	m := newManifest(o, prep, k6, host, scriptsHash(filepath.Join(o.RepoRoot, "workloads", "k6")))
	m.Synthetic = true
	m.OfflineFixture = true
	m.Verdict = "SYNTHETIC_FIXTURE"
	if err := writeJSON(filepath.Join(prep.dir, "manifest.json"), m); err != nil {
		return nil, err
	}
	return &OfflineFixture{Dir: prep.dir, Manifest: m, Plan: prep.plan, Resolved: prep.resolved, PlanSHA256: prep.planSHA256}, nil
}

// Run executes one run and returns its directory and manifest.
func Run(o Options) (string, *Manifest, error) {
	prep, err := prepareRun(o)
	if err != nil {
		return "", nil, err
	}
	dir := prep.dir
	k6, err := k6Info(o.Image)
	if err != nil {
		return dir, nil, err
	}
	m := newManifest(o, prep, k6, hostInfo(), scriptsHash(filepath.Join(o.RepoRoot, "workloads", "k6")))

	abs, _ := filepath.Abs(dir)
	k6dir, _ := filepath.Abs(filepath.Join(o.RepoRoot, "workloads", "k6"))
	container := "k6-" + prep.resolved.RunID
	args := []string{"run", "--rm", "--name", container, "--network", "host", "--cpuset-cpus", o.Cpuset,
		"-u", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"-v", k6dir + ":/k6:ro", "-v", abs + ":/out", "-w", "/k6",
		"-e", "RESOLVED=/out/resolved.json", "-e", "OUT_DIR=/out"}
	if o.CPUs != "" {
		args = append(args, "--cpus", o.CPUs)
	}
	if o.FullJSON {
		args = append(args, "-e", "FULL_JSON=1")
	}
	if o.PromRW != "" {
		// Counters arrive as <name>_total; trends only as precomputed per-push-interval statistics
		// (k6_<trend>_p99 etc.), which are NOT mergeable across intervals or runs.
		args = append(args, "-e", "K6_PROMETHEUS_RW_SERVER_URL="+o.PromRW,
			"-e", "K6_PROMETHEUS_RW_PUSH_INTERVAL=5s",
			"-e", "K6_PROMETHEUS_RW_TREND_STATS=p(50),p(99),max")
	}
	for _, kv := range o.Env {
		args = append(args, "-e", kv)
	}
	args = append(args, o.Image, "run", "--quiet", "--log-format", "json", "--log-output", "file=/out/k6-log.ndjson")
	if o.FullJSON {
		args = append(args, "--out", "json=/out/k6.json.gz")
	}
	if o.PromRW != "" {
		args = append(args, "--out", "experimental-prometheus-rw")
	}
	args = append(args, "gen.js")

	cpuFile, err := os.Create(filepath.Join(dir, "gen-cpu.ndjson"))
	if err != nil {
		return dir, nil, err
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sample(ctx, container, cpuFile); close(done) }()

	m.StartUTC = time.Now().UTC().Format(time.RFC3339Nano)
	var cpuDone chan struct{}
	if o.CPUSnapshotCommand != "" {
		cpuDone = make(chan struct{})
		go func() { captureCPU(ctx, o.CPUSnapshotCommand, dir); close(cpuDone) }()
	}
	var stdout bytes.Buffer
	cmd := exec.Command("docker", args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stdout
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			m.K6ExitCode = ee.ExitCode()
		} else {
			m.K6ExitCode = -1
		}
	}
	m.EndUTC = time.Now().UTC().Format(time.RFC3339Nano)
	stop()
	if cpuDone != nil {
		<-cpuDone
	}
	<-done
	cpuFile.Close()
	_ = os.WriteFile(filepath.Join(dir, "k6-stdout.log"), stdout.Bytes(), 0o644)
	if err := splitLog(dir); err != nil {
		return dir, m, err
	}

	if err := Finish(dir, m); err != nil {
		return dir, m, err
	}
	if cpuDone != nil {
		if err := finishCPU(dir); err != nil {
			_ = writeJSON(filepath.Join(dir, "cpu-boundary.json"), map[string]string{"status": "UNAVAILABLE", "error": err.Error()})
		}
	} else {
		_ = writeJSON(filepath.Join(dir, "cpu-boundary.json"), map[string]string{"status": "UNAVAILABLE", "reason": "No CPU snapshot command configured; no boundary counters captured"})
	}
	if o.GrafanaURL != "" {
		m.Annotation = annotate(o.GrafanaURL, m)
		if err := writeJSON(filepath.Join(dir, "manifest.json"), m); err != nil {
			return dir, m, err
		}
	}
	return dir, m, nil
}

// annotate posts the run window as a Grafana annotation (tags: benchmark, run id, plan id, verdict). It is a
// navigation aid only; the run's evidence is its directory, not the annotation.
func annotate(url string, m *Manifest) string {
	start, err1 := time.Parse(time.RFC3339Nano, m.StartUTC)
	end, err2 := time.Parse(time.RFC3339Nano, m.EndUTC)
	if err1 != nil || err2 != nil {
		return "failed: bad timestamps"
	}
	body, _ := json.Marshal(map[string]any{
		"time": start.UnixMilli(), "timeEnd": end.UnixMilli(),
		"tags": []string{"benchmark", m.RunID, m.PlanID, m.Verdict},
		"text": fmt.Sprintf("%s (%s) generator verdict %s", m.RunID, m.PlanID, m.Verdict),
	})
	req, err := http.NewRequest("POST", strings.TrimRight(url, "/")+"/api/annotations", bytes.NewReader(body))
	if err != nil {
		return "failed: " + err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("admin", os.Getenv("BENCH_GRAFANA_PASSWORD"))
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "failed: " + err.Error()
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Sprintf("failed: HTTP %d", res.StatusCode)
	}
	return "posted"
}

// Finish (re)computes the health verdict from the run directory and writes health.json and manifest.json.
func Finish(dir string, m *Manifest) error {
	if err := validateProtocolManifest(m); err != nil {
		return err
	}
	_, s, h, err := report.Evaluate(dir)
	if err != nil {
		return err
	}
	if m.K6ExitCode != 0 {
		h.Valid = false
		h.Reasons = append(h.Reasons, fmt.Sprintf("k6 exited with code %d", m.K6ExitCode))
	}
	if s != nil {
		m.Payload = s.SampleRequest
	}
	m.Health = h
	switch {
	case !h.Valid:
		m.Verdict = "INVALID"
	case m.OfflineFixture:
		// A health-valid synthetic fixture is still not a live run.
		m.Verdict = "SYNTHETIC_FIXTURE"
	default:
		m.Verdict = "VALID"
	}
	if err := writeJSON(filepath.Join(dir, "health.json"), h); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "manifest.json"), m)
}

func validateProtocolManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest missing")
	}
	if m.Resolved == nil {
		return nil
	}
	if !reflect.DeepEqual(m.ProtocolBinding, m.Resolved.ProtocolBinding) || !reflect.DeepEqual(m.ProtocolPilot, m.Resolved.ProtocolPilot) {
		return fmt.Errorf("manifest protocol binding/pilot metadata contradicts resolved plan")
	}
	if m.RuntimeMetricInterpretation != "" && m.RuntimeMetricInterpretation != LegacyMetricInterpretation {
		return fmt.Errorf("runner cannot claim protocol receipt instrumentation; runtime interpretation must remain %q", LegacyMetricInterpretation)
	}
	if m.Resolved.ProtocolBinding != nil && m.RuntimeMetricInterpretation != LegacyMetricInterpretation {
		return fmt.Errorf("protocol-bound runtime must be explicitly labeled legacy acknowledgement-only")
	}
	return nil
}
