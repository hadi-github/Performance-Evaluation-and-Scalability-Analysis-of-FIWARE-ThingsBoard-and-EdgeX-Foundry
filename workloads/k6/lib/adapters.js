// Platform adapters. Each one only wraps the same logical message
//   { run_id, seq, device, scheduled_send_ms, temperature, humidity, pressure }
// in its platform's format, and classifies the response explicitly:
//   ack_success       the adapter's success status (and, for 207 batch answers, every item's statusCode is 2xx)
//   explicit_failure  any other HTTP response
//   unknown           no HTTP response: timeout, connection reset, any transport error (status 0)
// setup() provisions the devices (not measured) and returns plain JSON state shared with every VU.
// Sends discard response bodies (discardResponseBodies) unless the adapter needs them, so every setup/cleanup
// request asks for its body explicitly (responseType: 'text').
import http from 'k6/http';
import exec from 'k6/execution';
import { sleep } from 'k6';

const JSON_HDR = { 'Content-Type': 'application/json' };

function must(res, ok, what) {
  if (!ok.includes(res.status)) {
    exec.test.abort(what + ' failed: HTTP ' + res.status + ' ' + String(res.body).slice(0, 300));
  }
  return res;
}

function deviceName(runId, i) {
  return 'bench-' + runId + '-' + String(i).padStart(5, '0');
}

function classifyStatus(res, okStatus) {
  if (res.status === 0) return { outcome: 'unknown', detail: 'transport' };
  return res.status === okStatus ? { outcome: 'ack_success' } : { outcome: 'explicit_failure' };
}

// A 207 answer is a batch: success only if every item reports a 2xx statusCode.
function classify207(res) {
  let items;
  try {
    items = JSON.parse(res.body);
  } catch (e) {
    return { outcome: 'explicit_failure', detail: 'unparseable_207_body' };
  }
  if (!Array.isArray(items) || items.length === 0) return { outcome: 'explicit_failure', detail: 'empty_207_body' };
  const bad = items.find((x) => !(x.statusCode >= 200 && x.statusCode < 300));
  return bad ? { outcome: 'explicit_failure', detail: 'item_status_' + bad.statusCode } : { outcome: 'ack_success' };
}

// ------------------------------------------------------------------ fixture / reference ingester
const fixture = {
  successStatus: 200,
  needsBody: (cfg) => !!cfg.target.expect_batch_207,
  setup(cfg) {
    const devices = [];
    for (let i = 0; i < cfg.devices; i++) devices.push(deviceName(cfg.run_id, i));
    return { devices };
  },
  request(cfg, st, m) {
    return {
      url: cfg.target.base_url + '/ingest',
      body: JSON.stringify({
        run_id: m.run_id, seq: m.seq, device_id: st.devices[m.device], scheduled_send_ms: m.scheduled_send_ms,
        temperature: m.temperature, humidity: m.humidity, pressure: m.pressure,
      }),
      headers: JSON_HDR,
    };
  },
  classify(res) {
    if (res.status === 207) return classify207(res);
    return classifyStatus(res, 200);
  },
  teardown() {},
};

// ------------------------------------------------------------------ ThingsBoard (HTTP device API)
function tbLogin(cfg) {
  const res = must(http.post(cfg.target.base_url + '/api/auth/login', JSON.stringify({
    username: __ENV.TB_USER || 'tenant@thingsboard.org', password: __ENV.TB_PASS || 'tenant',
  }), { headers: JSON_HDR, tags: { name: 'setup' }, responseType: 'text' }), [200], 'ThingsBoard login');
  return res.json('token');
}

const thingsboard = {
  successStatus: 200,
  needsBody: () => false,
  setup(cfg) {
    const auth = { headers: Object.assign({ 'X-Authorization': 'Bearer ' + tbLogin(cfg) }, JSON_HDR), tags: { name: 'setup' }, responseType: 'text' };
    const devices = [];
    for (let i = 0; i < cfg.devices; i++) {
      const d = must(http.post(cfg.target.base_url + '/api/device',
        JSON.stringify({ name: deviceName(cfg.run_id, i), type: 'bench' }), auth), [200], 'create device ' + i);
      const id = d.json('id.id');
      const c = must(http.get(cfg.target.base_url + '/api/device/' + id + '/credentials', auth), [200], 'credentials ' + i);
      devices.push({ id, token: c.json('credentialsId') });
    }
    return { devices };
  },
  // Stored as one ts_kv row per key (6 rows per logical reading, all with the same arrival ts).
  request(cfg, st, m) {
    return {
      url: cfg.target.base_url + '/api/v1/' + st.devices[m.device].token + '/telemetry',
      body: JSON.stringify({
        run_id: m.run_id, seq: m.seq, scheduled_send_ms: m.scheduled_send_ms,
        temperature: m.temperature, humidity: m.humidity, pressure: m.pressure,
      }),
      headers: JSON_HDR,
    };
  },
  classify(res) {
    return classifyStatus(res, 200);
  },
  teardown(cfg, st) {
    const auth = { headers: { 'X-Authorization': 'Bearer ' + tbLogin(cfg) }, tags: { name: 'cleanup' }, responseType: 'text' };
    for (const d of st.devices) http.del(cfg.target.base_url + '/api/device/' + d.id, null, auth);
  },
};

// ------------------------------------------------------------------ FIWARE (IoT Agent UltraLight 2.0 over HTTP)
const FW_HDR = { 'fiware-service': __ENV.FIWARE_SERVICE || 'openiot', 'fiware-servicepath': '/' };
const FW_TYPE = 'Sensor';

const fiware = {
  successStatus: 200,
  needsBody: () => false,
  setup(cfg) {
    const apikey = 'bench' + cfg.run_id.replace(/[^A-Za-z0-9]/g, '');
    const hdr = Object.assign({ 'Content-Type': 'application/json' }, FW_HDR);
    must(http.post(cfg.target.north_url + '/iot/services', JSON.stringify({
      services: [{ apikey, entity_type: FW_TYPE, resource: '/iot/d' }],
    }), { headers: hdr, tags: { name: 'setup' }, responseType: 'text' }), [201], 'create service group');
    const devices = [];
    for (let i = 0; i < cfg.devices; i++) devices.push(deviceName(cfg.run_id, i));
    for (let off = 0; off < devices.length; off += 100) {
      const batch = devices.slice(off, off + 100).map((id) => ({
        device_id: id, apikey, entity_name: 'urn:ngsi-ld:' + FW_TYPE + ':' + id, entity_type: FW_TYPE,
        attributes: [
          { object_id: 't', name: 'temperature', type: 'Integer' },
          { object_id: 'h', name: 'humidity', type: 'Integer' },
          { object_id: 'p', name: 'pressure', type: 'Integer' },
          { object_id: 'q', name: 'seq', type: 'Number' },
          { object_id: 'r', name: 'run_id', type: 'Text' },
          { object_id: 's', name: 'scheduled_send_ms', type: 'Number' },
        ],
      }));
      must(http.post(cfg.target.north_url + '/iot/devices', JSON.stringify({ devices: batch }),
        { headers: hdr, tags: { name: 'setup' }, responseType: 'text', timeout: '60s' }), [201], 'provision devices');
    }
    return { apikey, devices };
  },
  request(cfg, st, m) {
    return {
      url: cfg.target.south_url + '/iot/d?k=' + st.apikey + '&i=' + st.devices[m.device],
      body: 't|' + m.temperature + '|h|' + m.humidity + '|p|' + m.pressure
        + '|q|' + m.seq + '|r|' + m.run_id + '|s|' + m.scheduled_send_ms,
      headers: { 'Content-Type': 'text/plain' },
    };
  },
  classify(res) {
    return classifyStatus(res, 200);
  },
  teardown(cfg, st) {
    for (const id of st.devices) {
      http.del(cfg.target.north_url + '/iot/devices/' + id, null, { headers: FW_HDR, tags: { name: 'cleanup' }, responseType: 'text' });
    }
    http.del(cfg.target.north_url + '/iot/services?resource=/iot/d&apikey=' + st.apikey, null,
      { headers: FW_HDR, tags: { name: 'cleanup' }, responseType: 'text' });
  },
};

// ------------------------------------------------------------------ EdgeX (device-rest, one Object reading)
const EDGEX_PROFILE = 'bench-gen-v1';

const edgex = {
  successStatus: 200,
  needsBody: () => true, // a 207 answer must be parsed item by item
  setup(cfg) {
    const meta = cfg.target.metadata_url + '/api/v3';
    const get = { tags: { name: 'setup' }, responseType: 'none' };
    // Existing profile/devices are reused, never re-posted: with several device-rest replicas, adding a device makes
    // core-metadata panic (docs/findings/edgex-device-validation-panic.md), so provisioning happens before scaling
    // out (edgex/scripts/provision-then-scale.sh) and this setup then creates nothing.
    if (must(http.get(meta + '/deviceprofile/name/' + EDGEX_PROFILE, get), [200, 404], 'get device profile').status === 404) {
      const p = must(http.post(meta + '/deviceprofile', JSON.stringify([{ apiVersion: 'v3', profile: {
        name: EDGEX_PROFILE, manufacturer: 'bench', model: 'v1', labels: ['bench'],
        deviceResources: [{ name: 'reading', isHidden: false, properties: { valueType: 'Object', readWrite: 'RW', mediaType: 'application/json' } }],
        deviceCommands: [],
      } }]), { headers: JSON_HDR, tags: { name: 'setup' }, responseType: 'text' }), [207], 'create device profile');
      const ps = p.json('0.statusCode');
      if (ps !== 201 && ps !== 409) exec.test.abort('device profile: statusCode ' + ps + ' ' + p.body);
    }
    const devices = [];
    for (let i = 0; i < cfg.devices; i++) devices.push(deviceName(cfg.run_id, i));
    const missing = devices.filter((name) =>
      must(http.get(meta + '/device/name/' + name, get), [200, 404], 'get device ' + name).status === 404);
    for (let off = 0; off < missing.length; off += 100) {
      const batch = missing.slice(off, off + 100).map((name) => ({ apiVersion: 'v3', device: {
        name, serviceName: 'device-rest', profileName: EDGEX_PROFILE, adminState: 'UNLOCKED', operatingState: 'UP',
        protocols: { other: { Address: 'bench' } },
      } }));
      const r = must(http.post(meta + '/device', JSON.stringify(batch), { headers: JSON_HDR, tags: { name: 'setup' }, responseType: 'text', timeout: '60s' }),
        [207], 'create devices');
      const bad = r.json().find((x) => x.statusCode !== 201);
      if (bad) exec.test.abort('create devices: ' + JSON.stringify(bad));
    }
    // device-rest learns new devices from a system event over the message bus. Probe the first and the last device
    // until 8 consecutive readings are accepted (the LB spreads probes over replicas). Probes carry run_id
    // "<run_id>-probe" and seq -1, so reconciliation never counts them as messages of this run.
    for (const dev of [devices[0], devices[devices.length - 1]]) {
      let ok = 0;
      for (let t = 0; t < 120 && ok < 8; t++) {
        const res = http.post(cfg.target.ingress_url + '/api/v3/resource/' + dev + '/reading',
          JSON.stringify({ run_id: cfg.run_id + '-probe', seq: -1, scheduled_send_ms: 0, temperature: 0, humidity: 0, pressure: 0 }),
          { headers: JSON_HDR, tags: { name: 'setup' }, responseType: 'text' });
        if (edgex.classify(res).outcome === 'ack_success') ok++;
        else { ok = 0; sleep(0.5); }
      }
      if (ok < 8) exec.test.abort('device-rest never accepted readings for ' + dev);
    }
    return { devices };
  },
  request(cfg, st, m) {
    return {
      url: cfg.target.ingress_url + '/api/v3/resource/' + st.devices[m.device] + '/reading',
      body: JSON.stringify({
        run_id: m.run_id, seq: m.seq, scheduled_send_ms: m.scheduled_send_ms,
        temperature: m.temperature, humidity: m.humidity, pressure: m.pressure,
      }),
      headers: JSON_HDR,
    };
  },
  classify(res) {
    if (res.status === 207) return classify207(res);
    return classifyStatus(res, 200);
  },
  teardown(cfg, st) {
    for (const d of st.devices) {
      http.del(cfg.target.metadata_url + '/api/v3/device/name/' + d, null, { tags: { name: 'cleanup' }, responseType: 'text' });
    }
  },
};

export const ADAPTERS = { fixture, thingsboard, fiware, edgex };
