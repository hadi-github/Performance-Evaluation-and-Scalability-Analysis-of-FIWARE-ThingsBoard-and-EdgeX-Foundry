// FIWARE workload: IoT Agent UltraLight 2.0 over HTTP -> Orion -> MongoDB.
// setup:    one service group (API key) + DEVICES devices (batch provisioning on the IoT Agent north port 4041)
// iteration: POST :7896/iot/d?k=<apikey>&i=<device>  body "t|<int>|h|<int>|p|<int>|run_id|<testid>|seq|<n>"
// teardown: verify that every device's entity was updated during the run (Orion keeps only the latest value, so an
//           exact stored==sent count is impossible), then delete devices, entities and the service group.
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';
import { cfg, makeOptions, makeSummary, deviceIndex, deviceName, sensor, sentOk, devicesWithData } from './lib/common.js';

export const options = makeOptions();

const NORTH = 'http://' + cfg.sut + ':' + (__ENV.IOTA_NORTH_PORT || '4041');
const SOUTH = 'http://' + cfg.sut + ':' + (__ENV.IOTA_HTTP_PORT || '7896');
const ORION = 'http://' + cfg.sut + ':' + (__ENV.ORION_PORT || '1026');
const HDR = { 'fiware-service': __ENV.FIWARE_SERVICE || 'openiot', 'fiware-servicepath': '/' };
const JSON_HDR = Object.assign({ 'Content-Type': 'application/json' }, HDR);
const TYPE = 'Sensor';
const entityId = (dev) => 'urn:ngsi-ld:' + TYPE + ':' + dev;

function must(res, want, what) {
  if (!want.includes(res.status)) {
    exec.test.abort(what + ' failed: HTTP ' + res.status + ' ' + String(res.body).slice(0, 300));
  }
  return res;
}

export function setup() {
  const apikey = 'bench' + cfg.testid.replace(/[^A-Za-z0-9]/g, '');
  const start = new Date().toISOString();
  must(http.post(NORTH + '/iot/services', JSON.stringify({
    services: [{ apikey, entity_type: TYPE, resource: '/iot/d' }],
  }), { headers: JSON_HDR, tags: { name: 'setup' } }), [201], 'create service group');

  const ids = [];
  for (let i = 0; i < cfg.devices; i++) ids.push(deviceName(i));
  for (let off = 0; off < ids.length; off += 100) {
    const devices = ids.slice(off, off + 100).map((id) => ({
      device_id: id,
      apikey,
      entity_name: entityId(id),
      entity_type: TYPE,
      attributes: [
        { object_id: 't', name: 'temperature', type: 'Integer' },
        { object_id: 'h', name: 'humidity', type: 'Integer' },
        { object_id: 'p', name: 'pressure', type: 'Integer' },
        // identity attributes: (run_id, seq) reconcile the store via scripts/history-ids-audit.sh fiware
        // (SUT side, QL/Timescale candidate) instead of count-only verification
        { object_id: 'run_id', name: 'run_id', type: 'Text' },
        { object_id: 'seq', name: 'seq', type: 'Number' },
      ],
    }));
    must(http.post(NORTH + '/iot/devices', JSON.stringify({ devices }),
      { headers: JSON_HDR, tags: { name: 'setup' }, timeout: '60s' }), [201], 'provision devices');
  }
  console.log('provisioned ' + ids.length + ' devices, apikey ' + apikey);
  return { apikey, ids, start };
}

export default function (data) {
  const id = data.ids[deviceIndex()];
  const s = sensor();
  // seq is unique across the whole test (shared counter over all VUs/scenarios, incl. warmup)
  const seq = exec.scenario.iterationInTest;
  const res = http.post(SOUTH + '/iot/d?k=' + data.apikey + '&i=' + id,
    't|' + s.temperature + '|h|' + s.humidity + '|p|' + s.pressure + '|run_id|' + cfg.testid + '|seq|' + seq,
    { headers: { 'Content-Type': 'text/plain' }, tags: { name: 'send' }, timeout: cfg.timeout, responseType: 'none' });
  if (check(res, { 'status is 200': (r) => r.status === 200 })) sentOk.add(1);
}

export function teardown(data) {
  if (cfg.verify) {
    // Entities of this run whose TimeInstant (set by the IoT Agent on every measure) is after setup.
    const q = encodeURIComponent('TimeInstant>=' + data.start);
    const res = http.get(ORION + '/v2/entities?type=' + TYPE + '&idPattern=' + encodeURIComponent('^' + entityId('bench-' + cfg.testid + '-'))
      + '&q=' + q + '&options=count&limit=1&attrs=TimeInstant', { headers: HDR, tags: { name: 'verify' } });
    const n = parseInt(res.headers['Fiware-Total-Count'] || '0', 10);
    devicesWithData.add(n);
    console.log('verify: ' + n + ' of ' + data.ids.length + ' entities updated during the run (HTTP ' + res.status + ')');
  }
  if (cfg.cleanup) {
    let failed = 0;
    for (const id of data.ids) {
      const r = http.del(NORTH + '/iot/devices/' + id, null, { headers: HDR, tags: { name: 'cleanup' } });
      if (r.status !== 204) failed++;
    }
    for (let off = 0; off < data.ids.length; off += 100) {
      const entities = data.ids.slice(off, off + 100).map((id) => ({ id: entityId(id), type: TYPE }));
      http.post(ORION + '/v2/op/update', JSON.stringify({ actionType: 'delete', entities }),
        { headers: JSON_HDR, tags: { name: 'cleanup' } });
    }
    http.del(NORTH + '/iot/services?resource=/iot/d&apikey=' + data.apikey, null,
      { headers: HDR, tags: { name: 'cleanup' } });
    console.log('cleanup: devices, entities and service group deleted (' + failed + ' device deletes failed)');
  }
}

export function handleSummary(data) {
  return makeSummary(data, 'devices');
}
