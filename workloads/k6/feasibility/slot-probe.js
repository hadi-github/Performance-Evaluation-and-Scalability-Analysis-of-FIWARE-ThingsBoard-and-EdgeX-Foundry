// Feasibility probe B: a deterministic arrival schedule owned by the script (docs/methodology/k6-feasibility.md).
// Planned arrival s (slot) is due at T0 + s/RATE, fixed in setup() before any request is sent. VU v (0-based) owns
// slots v, v+VUS, v+2*VUS, ... (per-vu-iterations: one iteration per owned slot). An iteration waits until its slot's
// absolute due time (a deadline, not a pacing sleep: the next due time never depends on when the last request
// finished), sends exactly one request and carries slot id and due time with it. A late slot is sent late and
// visibly late; a slot never reached (maxDuration) is absent from the planned 0..SLOTS-1 set.
//
// Env: TARGET, RATE, DURATION, VUS, DELAY_MS, STALL_EVERY / STALL_MS (as in car-probe.js), LEAD_MS (default 2000)
import http from 'k6/http';
import exec from 'k6/execution';
import { sleep } from 'k6';

const TARGET = __ENV.TARGET || 'http://127.0.0.1:18080';
const RATE = parseInt(__ENV.RATE || '100', 10);
const DURATION_S = parseInt((__ENV.DURATION || '10s').replace(/s$/, ''), 10);
const VUS = parseInt(__ENV.VUS || '20', 10);
const SLOTS = RATE * DURATION_S;
const PERIOD_MS = 1000 / RATE;
const DELAY_MS = __ENV.DELAY_MS || '0';
const STALL_EVERY = parseInt(__ENV.STALL_EVERY || '0', 10);
const STALL_MS = parseInt(__ENV.STALL_MS || '0', 10);

export const options = {
  scenarios: {
    slots: {
      executor: 'per-vu-iterations',
      vus: VUS,
      iterations: Math.ceil(SLOTS / VUS),
      maxDuration: (DURATION_S + 30) + 's', // slots not reached by then stay visibly missing
      gracefulStop: '0s',
    },
  },
  systemTags: ['status', 'name', 'scenario', 'expected_response'],
  setupTimeout: '30s',
};

export function setup() {
  return { t0: Date.now() + parseInt(__ENV.LEAD_MS || '2000', 10) };
}

export default function (data) {
  const slot = (exec.vu.idInTest - 1) + exec.vu.iterationInScenario * VUS;
  if (slot >= SLOTS) return;
  const sched = data.t0 + slot * PERIOD_MS;
  const wait = sched - Date.now();
  if (wait > 0) sleep(wait / 1000);
  const startMs = Date.now();
  if (STALL_EVERY > 0 && slot % STALL_EVERY === 0) {
    const t = Date.now();
    while (Date.now() - t < STALL_MS) { /* deliberate generator-side stall */ }
  }
  exec.vu.metrics.metadata.slot = String(slot);
  const preMs = Date.now();
  http.post(TARGET + '/ingest?delay_ms=' + DELAY_MS, '', {
    headers: {
      'X-Bench-Slot': String(slot),
      'X-Bench-Sched-Ms': String(Math.round(sched)),
      'X-Bench-T0-Ms': String(data.t0),
      'X-Bench-Vu': String(exec.vu.idInTest),
      'X-Bench-Start-Ms': String(startMs),
      'X-Bench-Pre-Ms': String(preMs),
    },
    tags: { name: 'probe' },
    responseType: 'none',
  });
}
