// EdgeX workload: device-rest (HTTP) -> NATS (queue group) -> core-data -> PostgreSQL.
// setup:    device profile "bench-workload" (409 = already there, then it must have the temperature resource), DEVICES devices on device-rest (batch POST to
//           core-metadata), then wait until device-rest accepts readings for the first and the last device.
// iteration: POST :59986/api/v3/resource/<device>/temperature  body "<int>"
//           device-rest takes one resource per request, so each request carries one reading (TB and FIWARE carry
//           three values per request); the unit compared across platforms is the request.
// teardown: count the stored events per device in core-data -> stored_messages (minus the readiness probes), which
//           must equal sent_ok (exactly once: no loss, no duplicates), then delete the devices.
import http from 'k6/http';
import { check, sleep } from 'k6';
import exec from 'k6/execution';
import { cfg, makeOptions, makeSummary, deviceIndex, deviceName, sensor, sentOk, storedMessages } from './lib/common.js';

export const options = makeOptions();

const META = 'http://' + cfg.sut + ':' + (__ENV.EDGEX_CORE_METADATA_PORT || '59881') + '/api/v3';
const DATA = 'http://' + cfg.sut + ':' + (__ENV.EDGEX_CORE_DATA_PORT || '59880') + '/api/v3';
const REST = 'http://' + cfg.sut + ':' + (__ENV.EDGEX_DEVICE_REST_PORT || '59986') + '/api/v3';
const JSON_HDR = { 'Content-Type': 'application/json' };
// Not "bench-profile": the smoke test owns that name, with a different resource.
const PROFILE = 'bench-workload';

function must(res, want, what) {
  if (!want.includes(res.status)) {
    exec.test.abort(what + ' failed: HTTP ' + res.status + ' ' + String(res.body).slice(0, 300));
  }
  return res;
}

// EdgeX batch calls answer 207; the real result is each item's statusCode.
function items(res, ok, what) {
  must(res, [207], what);
  const bad = res.json().filter((x) => !ok.includes(x.statusCode));
  if (bad.length) exec.test.abort(what + ' failed: ' + JSON.stringify(bad[0]).slice(0, 300));
}

function send(dev, value, tags) {
  // count-based ledger only: device-rest carries one Int64 per request, so no (run_id, seq) identity is
  // transportable here — a duplicate can offset a loss in the stored==sent check. Identity-based reconciliation
  // for EdgeX requires the campaign-grade generator (workloads/k6/gen.js EDGEX_PROFILE), whose objectValue
  // reading carries seq/run_id (scripts/history-ids-audit.sh edgex).
  return http.post(REST + '/resource/' + dev + '/temperature', String(value),
    { headers: { 'Content-Type': 'text/plain' }, tags, timeout: cfg.timeout, responseType: 'none' });
}

export function setup() {
  const resource = (name) => ({ name, isHidden: false, properties: { valueType: 'Int64', readWrite: 'RW' } });
  items(http.post(META + '/deviceprofile', JSON.stringify([{ apiVersion: 'v3', profile: {
    name: PROFILE, manufacturer: 'bench', model: 'v1', labels: ['bench'],
    deviceResources: [resource('temperature'), resource('humidity'), resource('pressure')], deviceCommands: [],
  } }]), { headers: JSON_HDR, tags: { name: 'setup' } }), [201, 409], 'create device profile');
  const prof = must(http.get(META + '/deviceprofile/name/' + PROFILE, { tags: { name: 'setup' } }), [200], 'read device profile');
  if (!prof.json('profile.deviceResources').some((r) => r.name === 'temperature')) {
    exec.test.abort('device profile ' + PROFILE + ' exists without a "temperature" resource; delete it and rerun');
  }

  const names = [];
  for (let i = 0; i < cfg.devices; i++) names.push(deviceName(i));
  for (let off = 0; off < names.length; off += 100) {
    const batch = names.slice(off, off + 100).map((name) => ({ apiVersion: 'v3', device: {
      name, serviceName: 'device-rest', profileName: PROFILE, adminState: 'UNLOCKED', operatingState: 'UP',
      protocols: { other: { Address: 'bench' } },
    } }));
    items(http.post(META + '/device', JSON.stringify(batch), { headers: JSON_HDR, tags: { name: 'setup' }, timeout: '60s' }),
      [201], 'create devices');
  }

  // device-rest learns new devices from a system event over NATS. Every replica must know them, and the LB spreads
  // the probes across replicas, so require several consecutive 200s for the first and the last device.
  const probes = {};
  for (const dev of [names[0], names[names.length - 1]]) {
    let ok = 0;
    for (let t = 0; t < 120 && ok < 8; t++) {
      if (send(dev, 0, { name: 'setup' }).status === 200) {
        ok++;
        probes[dev] = (probes[dev] || 0) + 1;
      } else {
        ok = 0;
        sleep(0.5);
      }
    }
    if (ok < 8) exec.test.abort('device-rest never accepted readings for ' + dev);
  }
  console.log('provisioned ' + names.length + ' devices; device-rest ready');
  return { names, probes };
}

export default function (data) {
  const res = send(data.names[deviceIndex()], sensor().temperature, { name: 'send' });
  if (check(res, { 'status is 200': (r) => r.status === 200 })) sentOk.add(1);
}

export function teardown(data) {
  if (cfg.verify) {
    sleep(5); // device-rest -> NATS -> core-data -> PostgreSQL is asynchronous
    let stored = 0;
    let failed = 0;
    for (const dev of data.names) {
      const res = http.get(DATA + '/event/count/device/name/' + dev, { tags: { name: 'verify' } });
      if (res.status !== 200) { failed++; continue; }
      stored += (res.json('count') || 0) - (data.probes[dev] || 0);
    }
    storedMessages.add(stored);
    console.log('verify: ' + stored + ' stored events (' + failed + ' count queries failed)');
  }
  if (cfg.cleanup) {
    let failed = 0;
    for (const dev of data.names) {
      if (http.del(META + '/device/name/' + dev, null, { tags: { name: 'cleanup' } }).status !== 200) failed++;
    }
    console.log('cleanup: ' + data.names.length + ' devices deleted (' + failed + ' failed)');
  }
}

export function handleSummary(data) {
  return makeSummary(data, 'count');
}
