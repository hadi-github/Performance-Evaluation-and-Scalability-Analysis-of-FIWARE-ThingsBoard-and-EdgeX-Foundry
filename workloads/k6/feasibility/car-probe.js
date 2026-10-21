// Feasibility probe A: what native k6 tells an iteration about its planned arrival (docs/k6-feasibility.md).
// constant-arrival-rate, one request per iteration, no sleep. Every request carries what the iteration knows about
// itself in X-Bench-* headers; the fixture target records them with its own receipt time.
//
// Env: TARGET (http://127.0.0.1:18080), RATE, DURATION, PRE_VUS, MAX_VUS, DELAY_MS (target delay), STALL_EVERY /
// STALL_MS (busy-wait STALL_MS in every STALL_EVERY-th iteration: occupies that VU, a generator-side delay).
import http from 'k6/http';
import exec from 'k6/execution';

const TARGET = __ENV.TARGET || 'http://127.0.0.1:18080';
const RATE = parseInt(__ENV.RATE || '100', 10);
const DELAY_MS = __ENV.DELAY_MS || '0';
const STALL_EVERY = parseInt(__ENV.STALL_EVERY || '0', 10);
const STALL_MS = parseInt(__ENV.STALL_MS || '0', 10);

export const options = {
  scenarios: {
    probe: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: __ENV.DURATION || '10s',
      preAllocatedVUs: parseInt(__ENV.PRE_VUS || '20', 10),
      maxVUs: parseInt(__ENV.MAX_VUS || __ENV.PRE_VUS || '20', 10),
      gracefulStop: '30s',
    },
  },
  systemTags: ['status', 'name', 'scenario', 'expected_response'],
};

export default function () {
  const startMs = Date.now();
  const iter = exec.scenario.iterationInTest;
  if (STALL_EVERY > 0 && iter % STALL_EVERY === 0) {
    const t = Date.now();
    while (Date.now() - t < STALL_MS) { /* deliberate generator-side stall */ }
  }
  exec.vu.metrics.metadata.iter = String(iter);
  const preMs = Date.now();
  http.post(TARGET + '/ingest?delay_ms=' + DELAY_MS, '', {
    headers: {
      'X-Bench-Iter': String(iter),
      'X-Bench-Vu': String(exec.vu.idInTest),
      'X-Bench-Vu-Iter': String(exec.vu.iterationInScenario),
      'X-Bench-Start-Ms': String(startMs),
      'X-Bench-Pre-Ms': String(preMs),
      'X-Bench-Scenario-Start-Ms': String(exec.scenario.startTime),
    },
    tags: { name: 'probe' },
    responseType: 'none',
  });
}
