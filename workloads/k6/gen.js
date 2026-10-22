// Open-model telemetry generator. Run through `benchctl run`, which resolves the plan into explicit steps
// (RESOLVED = path of the resolved plan JSON) and checks the results.
//
// Every step is one constant-arrival-rate scenario. Within step s, iteration i (exec.scenario.iterationInTest) is
//   seq               = s.seq_offset + i
//   scheduled_send_ms = (scenario start) + i * 1000 / s.tick_rate
// That mapping is exact only while no iteration is dropped (docs/k6-feasibility.md); a run with
// dropped_iterations > 0 is therefore INVALID. One iteration = at most one HTTP request; no retries, no sleeps.
//
// Step boundary guard: the executor runs GUARD_S longer than the step, and only iterations i < ticks are sent;
// later ones return immediately ("beyond plan"). Without the guard k6 ran rate x duration - 1 iterations in 1 of
// 10 one-minute steps: the last tick's timer fired after the scenario deadline and was neither started nor counted
// as dropped (docs/calibration.md). With it, the last planned tick is never near a deadline.
//
// Outputs (no per-message console logging for successes, no IDs in metric tags):
//   - counters per step: attempted, ack_success, explicit_failure, unknown, early sends, beyond-plan, thinned ticks;
//   - send lag (actual - scheduled) per step and ack latency (response - scheduled) per step and per minute of the
//     step (by scheduled tick), as two tagged k6 Trends with exact p50/p99/max per tag combination;
//   - one JSON log record per non-success message or early send (k6 --log-output file, written by benchctl);
//   - handleSummary -> <OUT_DIR>/k6-summary.json (all counters, dropped iterations and VUs per step).
import http from 'k6/http';
import exec from 'k6/execution';
import { Counter, Gauge, Trend } from 'k6/metrics';
import { ADAPTERS } from './lib/adapters.js';
import { selected, threshold, values } from './lib/hash.js';

const cfg = JSON.parse(open(__ENV.RESOLVED));
const OUT_DIR = __ENV.OUT_DIR || '.';
const adapter = ADAPTERS[cfg.adapter];
if (!adapter) throw new Error('unknown adapter ' + cfg.adapter);

const THINNING = cfg.arrivals.model === 'thinning';
const SEL_THRESHOLD = threshold(cfg.send_probability);
const TIMEOUT = cfg.request_timeout_ms + 'ms';
const EARLY_TOL = cfg.health.early_tolerance_ms;
const DUP = new Set(cfg.test_faults.duplicate_seqs || []);
const EARLY_EVERY = cfg.test_faults.early_every || 0;
// Calibration runs with full JSON output: attach seq/scheduled time to the samples (non-indexed metadata), so
// k6's own ns sample times can serve as the authoritative send time (benchctl sendtimes).
const FULL_JSON = __ENV.FULL_JSON === '1';

// ---- metrics, declared in the init context of every VU. Keep the number of metric objects small: k6 creates them
// per VU, and per-(step, bucket) Counters reached 5.2 GB at 200 VUs x 30 steps (docs/calibration.md).
const OUTCOMES = ['ack_success', 'explicit_failure', 'unknown'];
const M = cfg.steps.map((s) => {
  const c = (n) => new Counter('g_' + n + '_s' + s.index);
  const o = {};
  OUTCOMES.forEach((k) => { o[k] = c(k); });
  return {
    attempted: c('attempted'), outcome: o, early: c('early'), beyond: c('beyond'), thinned: c('thinned'),
    dupInjected: c('dup_injected'), t0: new Gauge('g_t0_s' + s.index),
    plannedRate: new Gauge('g_planned_rate_s' + s.index), // for live monitoring only (refreshed every 500 ticks)
  };
});
// Latency Trends, tagged s = step index, m = minute of the step by scheduled tick (0-based). k6 keeps every value of
// a Trend (and of each thresholded sub-metric) for the summary, so p99s are exact.
const LAG = new Trend('g_lag_ms');
const ACK = new Trend('g_ack_ms');
const minutes = (s) => Math.ceil(s.duration_s / 60);

const GUARD_S = 1;
const scenarios = {};
const thresholds = {};
for (const s of cfg.steps) {
  scenarios[s.name] = {
    executor: 'constant-arrival-rate',
    rate: s.tick_rate,
    timeUnit: '1s',
    duration: (s.duration_s + GUARD_S) + 's',
    startTime: s.start_s + 's',
    // Per-step pool (pre-allocated = max, no dynamic allocation); plan default otherwise.
    preAllocatedVUs: s.vus || cfg.vus.pre_allocated,
    maxVUs: s.vus || cfg.vus.max,
    gracefulStop: (cfg.request_timeout_ms + 5000) + 'ms',
    exec: 'gen',
    env: { STEP: String(s.index) },
  };
  // Always-true thresholds: they only make k6 keep these per-step sub-metrics for handleSummary. Never abort.
  thresholds['dropped_iterations{scenario:' + s.name + '}'] = ['count>=0'];
  thresholds['http_reqs{scenario:' + s.name + '}'] = ['count>=0'];
  thresholds['data_sent{scenario:' + s.name + '}'] = ['count>=0'];
  thresholds['g_lag_ms{s:' + s.index + '}'] = ['p(99)>=0'];
  thresholds['g_ack_ms{s:' + s.index + '}'] = ['p(99)>=0'];
  for (let m = 0; m < minutes(s); m++) thresholds['g_ack_ms{s:' + s.index + ',m:' + m + '}'] = ['p(99)>=0'];
}

export const options = {
  scenarios,
  thresholds,
  setupTimeout: '15m',
  teardownTimeout: '15m',
  noConnectionReuse: !cfg.connection.keep_alive,
  discardResponseBodies: !adapter.needsBody(cfg),
  systemTags: ['status', 'scenario', 'name', 'expected_response', 'error_code'],
  // Bounded run-level labels for the optional Prometheus remote-write stream (one value per run; never per message).
  tags: { run_id: cfg.run_id, plan_id: cfg.plan_id, adapter: cfg.adapter },
  summaryTrendStats: ['count', 'avg', 'med', 'p(99)', 'max'],
};

// 207 counts as an expected HTTP status for diagnostics; the outcome is decided by the adapter's classify().
http.setResponseCallback(http.expectedStatuses(adapter.successStatus, 207));

export function setup() {
  const st = adapter.setup(cfg);
  const sample = adapter.request(cfg, st, Object.assign({ run_id: cfg.run_id, seq: 0, device: 0, scheduled_send_ms: Date.now() + 0.125 },
    values(cfg.seed, 0)));
  st.sample = { url_bytes: sample.url.length, body_bytes: sample.body.length, body: sample.body };
  st.setup_end_ms = Date.now();
  return st;
}

function record(r) {
  console.log(JSON.stringify(r));
}

function send(st, step, m, i) {
  const req = adapter.request(cfg, st, m);
  if (FULL_JSON) {
    exec.vu.metrics.metadata.seq = String(m.seq);
    exec.vu.metrics.metadata.sched = String(m.scheduled_send_ms);
  }
  const actual = Date.now();
  const lag = actual - m.scheduled_send_ms;
  const res = http.post(req.url, req.body, { headers: req.headers, timeout: TIMEOUT, tags: { name: 'send' } });
  const response = Date.now();
  const c = adapter.classify(res);
  const mm = M[step.index];
  mm.attempted.add(1);
  mm.outcome[c.outcome].add(1);
  LAG.add(lag, { s: String(step.index) });
  if (c.outcome === 'ack_success') {
    ACK.add(response - m.scheduled_send_ms, { s: String(step.index), m: String(Math.floor(i / (step.tick_rate * 60))) });
  }
  const early = lag < -EARLY_TOL;
  if (early) mm.early.add(1);
  if (c.outcome !== 'ack_success' || early) {
    record({
      rec: 'msg', run_id: m.run_id, seq: m.seq, device_id: m.device, rate_step: step.index, phase: step.phase,
      tick: i, scheduled_send_ms: m.scheduled_send_ms, actual_send_ms: actual, outcome: c.outcome,
      http_status: res.status, error_code: res.error_code || 0, response_ms: response, detail: c.detail || '',
      early,
    });
  }
}

export function gen(st) {
  const step = cfg.steps[parseInt(__ENV.STEP, 10)];
  const i = exec.scenario.iterationInTest;
  const mm = M[step.index];
  if (i === 0) mm.t0.add(exec.scenario.startTime);
  if (i % 500 === 0 && i < step.ticks) mm.plannedRate.add(step.rate);
  if (i >= step.ticks) { mm.beyond.add(1); return; }
  const seq = step.seq_offset + i;
  if (THINNING && !selected(cfg.seed, seq, SEL_THRESHOLD)) { mm.thinned.add(1); return; }
  let sched = exec.scenario.startTime + (i * 1000) / step.tick_rate;
  if (EARLY_EVERY > 0 && seq % EARLY_EVERY === 0) sched += 50; // test fault: pretend it was due 50 ms later
  const m = Object.assign({
    run_id: cfg.run_id, seq, device: seq % cfg.devices, scheduled_send_ms: Math.round(sched * 1000) / 1000,
  }, values(cfg.seed, seq));
  send(st, step, m, i);
  if (DUP.has(seq)) { mm.dupInjected.add(1); send(st, step, m, i); }
}

export function teardown(st) {
  if (cfg.cleanup) adapter.teardown(cfg, st);
}

export function handleSummary(data) {
  const v = (name, stat) => (data.metrics[name] && data.metrics[name].values[stat]) || 0;
  const trend = (name) => ({ count: v(name, 'count'), p50: v(name, 'med'), p99: v(name, 'p(99)'), max: v(name, 'max') });
  const steps = cfg.steps.map((s) => ({
    index: s.index, name: s.name,
    scenario_start_ms: v('g_t0_s' + s.index, 'value'),
    attempted: v('g_attempted_s' + s.index, 'count'),
    ack_success: v('g_ack_success_s' + s.index, 'count'),
    explicit_failure: v('g_explicit_failure_s' + s.index, 'count'),
    unknown: v('g_unknown_s' + s.index, 'count'),
    early: v('g_early_s' + s.index, 'count'),
    beyond_plan: v('g_beyond_s' + s.index, 'count'),
    thinned: v('g_thinned_s' + s.index, 'count'),
    dup_injected: v('g_dup_injected_s' + s.index, 'count'),
    dropped_iterations: v('dropped_iterations{scenario:' + s.name + '}', 'count'),
    http_reqs: v('http_reqs{scenario:' + s.name + '}', 'count'),
    data_sent_bytes: v('data_sent{scenario:' + s.name + '}', 'count'),
    send_lag_ms: trend('g_lag_ms{s:' + s.index + '}'),
    ack_latency_ms: trend('g_ack_ms{s:' + s.index + '}'),
    ack_latency_ms_by_minute: Array.from({ length: minutes(s) }, (_, m) => Object.assign({ minute: m + 1 },
      trend('g_ack_ms{s:' + s.index + ',m:' + m + '}'))),
  }));
  const out = {
    schema_version: 2, run_id: cfg.run_id, steps,
    dropped_iterations_total: v('dropped_iterations', 'count'),
    vus_peak: v('vus', 'max'), vus_max_allocated: v('vus_max', 'max'),
    sample_request: data.setup_data ? data.setup_data.sample : null,
    setup_end_ms: data.setup_data ? data.setup_data.setup_end_ms : null,
    test_run_duration_ms: data.state.testRunDurationMs,
  };
  return { [OUT_DIR + '/k6-summary.json']: JSON.stringify(out, null, 1), stdout: '' };
}
