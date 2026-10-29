// Shared parts of the three platform workloads: an OPEN workload model (a fixed arrival rate of RATE requests/s,
// independent of how fast the platform answers), round-robin device selection, sensor values and the summary.
//
// Scenarios: "warmup" (WARMUP long) then "steady" (DURATION long), both constant-arrival-rate at RATE req/s.
// Every sample carries the k6 "scenario" tag, so analyses and dashboards use scenario="steady" only.
import exec from 'k6/execution';
import { Counter } from 'k6/metrics';
// Local copy of https://jslib.k6.io/k6-summary/0.1.0/index.js (the VMs cannot reach jslib.k6.io).
import { textSummary } from './k6-summary.js';

function env(name, def) {
  const v = __ENV[name];
  return v === undefined || v === '' ? def : v;
}

// "10m", "90s", "1h30m" -> seconds
export function seconds(d) {
  let total = 0;
  const re = /(\d+(?:\.\d+)?)(h|m|s|ms)/g;
  let m;
  let matched = '';
  while ((m = re.exec(d)) !== null) {
    matched += m[0];
    total += parseFloat(m[1]) * { h: 3600, m: 60, s: 1, ms: 0.001 }[m[2]];
  }
  if (matched !== d) throw new Error('invalid duration: ' + d);
  return total;
}

export const cfg = {
  platform: env('PLATFORM', 'unknown'),
  sut: env('SUT_HOST', '87.107.191.107'),
  rate: parseInt(env('RATE', '10'), 10),
  duration: env('DURATION', '10m'),
  warmup: env('WARMUP', '1m'),
  devices: parseInt(env('DEVICES', '100'), 10),
  n: env('N', '1'),
  testid: env('TESTID', 'manual-' + Date.now()),
  resultsDir: env('RESULTS_DIR', 'results'),
  cleanup: env('CLEANUP', '1') === '1',
  verify: env('VERIFY', '1') === '1',
  timeout: env('HTTP_TIMEOUT', '10s'),
};
// VUs only bound concurrency: an arrival-rate executor needs about RATE x latency VUs. If it runs out, k6 drops
// iterations instead of slowing down, and the run is marked INVALID (the target rate was not delivered).
cfg.preVUs = parseInt(env('PRE_VUS', String(Math.max(10, Math.ceil(cfg.rate / 2)))), 10);
cfg.maxVUs = parseInt(env('MAX_VUS', String(Math.max(50, cfg.rate * 2))), 10);

export function makeOptions() {
  const scenario = (startTime, duration) => ({
    executor: 'constant-arrival-rate',
    rate: cfg.rate,
    timeUnit: '1s',
    duration,
    startTime,
    preAllocatedVUs: cfg.preVUs,
    maxVUs: cfg.maxVUs,
    gracefulStop: '30s',
  });
  const scenarios = { steady: scenario(cfg.warmup, cfg.duration) };
  if (seconds(cfg.warmup) > 0) scenarios.warmup = scenario('0s', cfg.warmup);
  else scenarios.steady.startTime = '0s';
  return {
    scenarios,
    setupTimeout: '15m',
    teardownTimeout: '15m',
    summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
    // Default system tags minus "url": every device has its own URL, which would create one Prometheus series per
    // device. Requests are told apart by their "name" tag (setup / send / verify / cleanup) instead.
    systemTags: ['proto', 'subproto', 'status', 'method', 'name', 'group', 'check', 'error', 'error_code',
      'tls_version', 'scenario', 'service', 'expected_response'],
    tags: { platform: cfg.platform, n: String(cfg.n), rate: String(cfg.rate), testid: cfg.testid },
    // Thresholds that always pass: they only make k6 keep these per-scenario sub-metrics for handleSummary.
    thresholds: {
      'http_reqs{scenario:steady}': ['count>=0'],
      'http_req_duration{scenario:steady}': ['max>=0'],
      'http_req_failed{scenario:steady}': ['rate>=0'],
      'checks{scenario:steady}': ['rate>=0'],
      'sent_ok{scenario:steady}': ['count>=0'],
    },
  };
}

// Custom counters. sent_ok: telemetry requests answered with 200 (all phases).
// stored_messages / devices_with_data: what the platform reports back in teardown (verification).
export const sentOk = new Counter('sent_ok');
export const storedMessages = new Counter('stored_messages');
export const devicesWithData = new Counter('devices_with_data');

export function deviceIndex() {
  return exec.scenario.iterationInTest % cfg.devices;
}

function rnd(lo, hi) {
  return lo + Math.floor(Math.random() * (hi - lo + 1));
}

// Same value ranges as the reference JMeter plans.
export function sensor() {
  return { temperature: rnd(15, 45), humidity: rnd(30, 80), pressure: rnd(950, 1150) };
}

export function deviceName(i) {
  return 'bench-' + cfg.testid + '-' + String(i).padStart(4, '0');
}

function metric(data, name, stat) {
  const m = data.metrics[name];
  return m && m.values && m.values[stat] !== undefined ? m.values[stat] : 0;
}

export function makeSummary(data, verifyKind) {
  const steadyS = seconds(cfg.duration);
  const warmupS = seconds(cfg.warmup);
  const steadyReqs = metric(data, 'http_reqs{scenario:steady}', 'count');
  const achieved = steadyS > 0 ? steadyReqs / steadyS : 0;
  const dropped = metric(data, 'dropped_iterations', 'count');
  const sent = metric(data, 'sent_ok', 'count');
  const stored = metric(data, 'stored_messages', 'count');
  const withData = metric(data, 'devices_with_data', 'count');
  const steadyFailRate = metric(data, 'http_req_failed{scenario:steady}', 'rate');

  const invalid = [];
  if (dropped > 0) invalid.push(dropped + ' dropped iterations (not enough VUs: raise MAX_VUS, or the SUT is saturated)');
  if (cfg.rate > 0 && Math.abs(achieved - cfg.rate) / cfg.rate > 0.01) {
    invalid.push('achieved ' + achieved.toFixed(2) + ' req/s in steady phase, target ' + cfg.rate);
  }
  const findings = [];
  if (steadyFailRate > 0) findings.push('steady-phase error rate ' + (steadyFailRate * 100).toFixed(3) + '%');
  let verification = 'skipped';
  if (cfg.verify && verifyKind === 'count') {
    verification = stored === sent ? 'OK: stored == sent (' + stored + ')'
      : (stored < sent ? 'LOSS' : 'DUPLICATES') + ': stored ' + stored + ' vs sent ' + sent;
    if (stored !== sent) findings.push(verification);
  } else if (cfg.verify && verifyKind === 'devices') {
    verification = withData === cfg.devices ? 'OK: all ' + withData + ' devices updated'
      : 'MISSING: ' + withData + ' of ' + cfg.devices + ' devices updated';
    if (withData !== cfg.devices) findings.push(verification);
  }

  const result = {
    testid: cfg.testid,
    platform: cfg.platform,
    n: cfg.n,
    target_rate: cfg.rate,
    steady_duration_s: steadyS,
    warmup: cfg.warmup,
    warmup_s: warmupS,
    devices: cfg.devices,
    // the executor schedules rate x duration arrivals in the steady phase (and the same in warmup);
    // dropped_iterations + the achieved-rate check verify that the planned schedule was delivered
    planned_arrivals: Math.round((steadyS + warmupS) * cfg.rate),
    valid: invalid.length === 0,
    invalid_reasons: invalid,
    findings,
    steady: {
      requests: steadyReqs,
      achieved_rate: Number(achieved.toFixed(3)),
      error_rate: steadyFailRate,
      // k6 http_req_duration trend values are milliseconds; keep the unit explicit at every level
      latency_unit: 'ms',
      latency_ms: data.metrics['http_req_duration{scenario:steady}']
        ? data.metrics['http_req_duration{scenario:steady}'].values : {},
    },
    dropped_iterations: dropped,
    sent_ok_total: sent,
    verification,
    stored_messages: stored,
    devices_with_data: withData,
  };
  const verdict = '\n==== ' + cfg.testid + ': ' + (result.valid ? 'VALID' : 'INVALID') + ' run ====\n'
    + 'steady: ' + steadyReqs + ' requests, ' + achieved.toFixed(2) + ' req/s (target ' + cfg.rate + '), errors '
    + (steadyFailRate * 100).toFixed(3) + '%, p95 '
    + (result.steady.latency_ms['p(95)'] || 0).toFixed(1) + ' ms, p99 '
    + (result.steady.latency_ms['p(99)'] || 0).toFixed(1) + ' ms (latency unit: ms)\n'
    + 'planned arrivals (steady+warmup): ' + Math.round((steadyS + warmupS) * cfg.rate) + '\n'
    + 'verification: ' + verification + '\n'
    + (invalid.length ? 'INVALID because: ' + invalid.join('; ') + '\n' : '')
    + (findings.length ? 'findings: ' + findings.join('; ') + '\n' : '');
  const dir = cfg.resultsDir + '/' + cfg.testid;
  const out = {};
  out[dir + '/summary.json'] = JSON.stringify(result, null, 2);
  out[dir + '/k6-summary.json'] = JSON.stringify(data, null, 2);
  out[dir + '/summary.txt'] = textSummary(data, { indent: ' ', enableColors: false }) + verdict;
  out.stdout = textSummary(data, { indent: ' ', enableColors: true }) + verdict;
  return out;
}
