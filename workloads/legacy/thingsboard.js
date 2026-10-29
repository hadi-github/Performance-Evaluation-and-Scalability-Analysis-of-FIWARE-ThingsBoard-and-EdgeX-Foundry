// ThingsBoard workload: HTTP device API -> tb-http-transport -> Kafka -> tb-core / tb-rule-engine -> PostgreSQL.
// setup:    tenant login, then per device: POST /api/device + GET /api/device/{id}/credentials (access token)
// iteration: POST :8080/api/v1/<token>/telemetry  {"temperature":<int>,"humidity":<int>,"pressure":<int>}
// teardown: count the stored temperature points per device (agg=COUNT over the run window) -> stored_messages,
//           which must equal sent_ok (exactly once: no loss, no duplicates), then delete the devices.
//
// The payload has no explicit "ts": ThingsBoard stamps it on arrival. ts_kv is keyed by (device, key, ts), so two
// messages of the same device in the same millisecond would overwrite each other. With round-robin devices a device
// is hit every DEVICES/RATE seconds, so keep RATE/DEVICES well below 1000 (e.g. <= 200 req/s per device).
import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { cfg, makeOptions, makeSummary, deviceIndex, deviceName, sensor, sentOk, storedMessages } from './lib/common.js';

export const options = makeOptions();

const BASE = 'http://' + cfg.sut + ':' + (__ENV.TB_HTTP_PORT || '8080');
const JSON_HDR = { 'Content-Type': 'application/json' };

function must(res, want, what) {
  if (!want.includes(res.status)) {
    exec.test.abort(what + ' failed: HTTP ' + res.status + ' ' + String(res.body).slice(0, 300));
  }
  return res;
}

function login() {
  const res = must(http.post(BASE + '/api/auth/login', JSON.stringify({
    username: __ENV.TB_USER || 'tenant@thingsboard.org',
    password: __ENV.TB_PASS || 'tenant',
  }), { headers: JSON_HDR, tags: { name: 'setup' } }), [200], 'tenant login');
  return res.json('token');
}

export function setup() {
  const jwt = login();
  const auth = { headers: Object.assign({ 'X-Authorization': 'Bearer ' + jwt }, JSON_HDR), tags: { name: 'setup' } };
  const startTs = Date.now();
  const devices = [];
  for (let i = 0; i < cfg.devices; i++) {
    const dev = must(http.post(BASE + '/api/device', JSON.stringify({ name: deviceName(i), type: 'bench' }), auth),
      [200], 'create device ' + i);
    const id = dev.json('id.id');
    const cred = must(http.get(BASE + '/api/device/' + id + '/credentials', auth), [200], 'device credentials ' + i);
    devices.push({ id, token: cred.json('credentialsId') });
  }
  console.log('provisioned ' + devices.length + ' devices');
  return { devices, startTs };
}

export default function (data) {
  const d = data.devices[deviceIndex()];
  // Identity attributes on every message: (testid, iterationInTest) is unique across all VUs and scenarios
  // (incl. warmup), so the store-side export (scripts/history-ids-audit.sh thingsboard <testid>, run on the SUT)
  // reconciles identity sets, not only counts — a missing message can no longer be offset by a duplicate.
  // Same-millisecond collisions still overwrite (TB stamps ts on arrival); the export reports them as missing.
  const seq = exec.scenario.iterationInTest;
  const body = sensor();
  body.run_id = cfg.testid;
  body.seq = seq;
  const res = http.post(BASE + '/api/v1/' + d.token + '/telemetry', JSON.stringify(body),
    { headers: JSON_HDR, tags: { name: 'send' }, timeout: cfg.timeout, responseType: 'none' });
  if (check(res, { 'status is 200': (r) => r.status === 200 })) sentOk.add(1);
}

export function teardown(data) {
  const jwt = login(); // the setup token may have expired on long runs
  const auth = { headers: { 'X-Authorization': 'Bearer ' + jwt }, tags: { name: 'verify' } };
  if (cfg.verify) {
    sleep(10); // SQL time-series writes are batched asynchronously
    const startTs = data.startTs - 60000;
    const endTs = Date.now() + 60000;
    let stored = 0;
    let failed = 0;
    for (const d of data.devices) {
      const res = http.get(BASE + '/api/plugins/telemetry/DEVICE/' + d.id + '/values/timeseries?keys=temperature'
        + '&startTs=' + startTs + '&endTs=' + endTs + '&agg=COUNT&interval=' + (endTs - startTs) + '&limit=1000', auth);
      if (res.status !== 200) { failed++; continue; }
      for (const p of (res.json('temperature') || [])) stored += parseInt(p.value, 10);
    }
    storedMessages.add(stored);
    console.log('verify: ' + stored + ' stored telemetry messages (' + failed + ' count queries failed)');
  }
  if (cfg.cleanup) {
    let failed = 0;
    for (const d of data.devices) {
      if (http.del(BASE + '/api/device/' + d.id, null,
        { headers: auth.headers, tags: { name: 'cleanup' } }).status !== 200) failed++;
    }
    console.log('cleanup: ' + data.devices.length + ' devices deleted (' + failed + ' failed)');
  }
}

export function handleSummary(data) {
  return makeSummary(data, 'count');
}
