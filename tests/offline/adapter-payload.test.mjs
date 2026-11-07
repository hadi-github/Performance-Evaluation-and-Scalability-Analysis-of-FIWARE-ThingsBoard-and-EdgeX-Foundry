// SYNTHETIC OFFLINE ENGINEERING FIXTURE ONLY.
// Exercises the real k6 adapter-payload projection and response-boundary helpers in Node. It starts
// only a loopback HTTP fixture; no k6, Docker, platform, SUT or deployment is invoked. The canonical
// encoder remains workloads/k6/lib/payload.js; this test never reconstructs padding from a seed to
// satisfy validation except where it explicitly proves deterministic equality of the real encoder.
import assert from 'node:assert/strict';
import http from 'node:http';
import { buildPayload, canonicalPayloadBytes, validateStoredPayload } from '../../workloads/k6/lib/payload.js';
import {
  ADAPTER_PAYLOAD_ENCODING, buildLogical, nativePayload, payloadSelected, wireAccounting,
} from '../../workloads/k6/lib/adapter_payload.js';
import {
  BOUNDARY_OUTCOMES, classify207, classifyStatus, ledgerOutcome,
} from '../../workloads/k6/lib/response_semantics.js';

const ADAPTERS = ['fixture', 'thingsboard', 'fiware', 'edgex'];
const OFFSETS = [0, 1024, 4096];
const SEED = 17;
const RUN_ID = 'SYNTHETIC-m1e-offline-payload';
const MESSAGE = {
  run_id: RUN_ID, seq: 5, scheduled_send_ms: 1700000005000,
  temperature: 20, humidity: 40, pressure: 1000,
};
const DEVICE = `bench-${RUN_ID}-00005`;

function cfgWith(offset) {
  return { seed: SEED, payload: { encoding_id: ADAPTER_PAYLOAD_ENCODING, size_offset_bytes: offset } };
}

function expectedLogical(message, deviceID, padding) {
  return {
    run_id: message.run_id, seq: message.seq, device_id: deviceID,
    scheduled_send_ms: message.scheduled_send_ms, temperature: message.temperature,
    humidity: message.humidity, pressure: message.pressure, padding,
  };
}

function parseUltraLight(body) {
  const parts = body.split('|');
  const fields = {};
  for (let i = 0; i < parts.length; i += 2) fields[parts[i]] = parts[i + 1];
  if (fields.d === '""') fields.d = '';
  return fields;
}

// Canonical projection of the platform's actual stored native fields. For JSON platforms the body
// itself is the stored object. For FIWARE it projects the UltraLight text fields (including the actual
// `d` padding field) into the canonical object; it never derives padding from the seed.
function storedProjection(adapter, body, deviceID, options = {}) {
  if (adapter !== 'fiware') {
    const parsed = JSON.parse(body);
    if (options.omitPadding) delete parsed.padding;
    if (options.truncatePadding) parsed.padding = parsed.padding.slice(0, -1);
    if (options.changePadding) parsed.padding = (parsed.padding[0] === 'a' ? 'b' : 'a') + parsed.padding.slice(1);
    return JSON.stringify(parsed);
  }
  const fields = parseUltraLight(body);
  let padding = fields.d;
  if (options.omitPadding) padding = undefined;
  if (options.truncatePadding) padding = padding.slice(0, -1);
  if (options.changePadding) padding = (padding[0] === 'a' ? 'b' : 'a') + padding.slice(1);
  const paddingField = padding === undefined ? '' : `,"padding":${JSON.stringify(padding)}`;
  return `{"run_id":${JSON.stringify(fields.r)},"seq":${fields.q},"device_id":${JSON.stringify(deviceID)}`
    + `,"scheduled_send_ms":${fields.s},"temperature":${fields.t},"humidity":${fields.h}`
    + `,"pressure":${fields.p}${paddingField}}`;
}

assert.equal(payloadSelected({ seed: SEED }), false, 'legacy absence must stay legacy');
assert.throws(() => payloadSelected({ seed: SEED, payload: { encoding_id: 'other', size_offset_bytes: 0 } }), /encoding/);
assert.throws(() => payloadSelected({ seed: SEED, payload: { encoding_id: ADAPTER_PAYLOAD_ENCODING } }), /size_offset/);
assert.throws(() => payloadSelected({ seed: SEED, payload: { encoding_id: ADAPTER_PAYLOAD_ENCODING, size_offset_bytes: 1.5 } }), /size_offset/);

const built = {};
let vectorCount = 0;
for (const adapter of ADAPTERS) {
  for (const offset of OFFSETS) {
    const cfg = cfgWith(offset);
    const native = nativePayload(adapter, cfg, MESSAGE, DEVICE);
    const base = canonicalPayloadBytes(expectedLogical(MESSAGE, DEVICE, ''));
    const expectedBytes = buildPayload(expectedLogical(MESSAGE, DEVICE, ''), SEED, base.length + offset);
    assert.equal(native.payloadSelected, true);
    assert.equal(native.logicalBytes, expectedBytes, `${adapter}/${offset}: canonical logical bytes`);
    assert.equal(native.logicalBytes.length, base.length + offset, `${adapter}/${offset}: logical length`);
    assert.equal(native.padding.length, offset, `${adapter}/${offset}: padding length`);
    assert.equal(native.logical.run_id, RUN_ID);
    assert.equal(native.logical.seq, MESSAGE.seq);
    assert.equal(native.logical.device_id, DEVICE);
    assert.equal(native.logical.scheduled_send_ms, MESSAGE.scheduled_send_ms);
    assert.equal(native.logical.temperature, MESSAGE.temperature);
    assert.equal(native.logical.humidity, MESSAGE.humidity);
    assert.equal(native.logical.pressure, MESSAGE.pressure);

    const expected = expectedLogical(MESSAGE, DEVICE, native.padding);
    const projection = storedProjection(adapter, native.body, DEVICE);
    assert.equal(validateStoredPayload(expected, projection), true, `${adapter}/${offset}: actual stored projection`);

    const mutations = native.padding.length > 0 ? ['omitPadding', 'truncatePadding', 'changePadding'] : ['omitPadding'];
    for (const mutation of mutations) {
      const bad = storedProjection(adapter, native.body, DEVICE, { [mutation]: true });
      const identity = JSON.parse(bad);
      assert.equal(identity.run_id, RUN_ID, `${adapter}/${offset}/${mutation}: identity preserved`);
      assert.equal(identity.seq, MESSAGE.seq, `${adapter}/${offset}/${mutation}: seq preserved`);
      assert.equal(identity.device_id, DEVICE, `${adapter}/${offset}/${mutation}: device preserved`);
      assert.throws(() => validateStoredPayload(expected, bad), undefined,
        `${adapter}/${offset}/${mutation}: corrupt padding accepted`);
    }

    if (adapter === 'fiware') {
      if (offset === 0) assert.ok(native.body.endsWith('|d|""'), 'empty padding must use valid UltraLight quoting');
      const fields = parseUltraLight(native.body);
      assert.equal(fields.d, native.padding, `${adapter}/${offset}: ultraLight d alias carries actual padding`);
      assert.equal(fields.r, RUN_ID);
      assert.equal(fields.q, String(MESSAGE.seq));
      assert.equal(fields.s, String(MESSAGE.scheduled_send_ms));
    } else {
      assert.equal(native.body, expectedBytes, `${adapter}/${offset}: JSON body is canonical bytes`);
    }

    const acct = wireAccounting('http://127.0.0.1:1/ingest', native.headers, native);
    assert.equal(acct.logical_bytes, native.logicalBytes.length);
    assert.equal(acct.adapter_body_bytes, native.body.length);
    assert.ok(acct.http_overhead_bytes > 0);
    assert.equal(acct.wire_total_bytes, acct.http_overhead_bytes + acct.adapter_body_bytes);
    built[`${adapter}/${offset}`] = {
      logical_bytes: acct.logical_bytes, adapter_body_bytes: acct.adapter_body_bytes,
      http_overhead_bytes: acct.http_overhead_bytes, padding_length: native.padding.length,
    };
    vectorCount += 1;
  }
}

// Explicit opt-in: absence of payload preserves the historical bodies byte-for-byte.
const legacyExpected = {
  fixture: JSON.stringify({
    run_id: MESSAGE.run_id, seq: MESSAGE.seq, device_id: DEVICE, scheduled_send_ms: MESSAGE.scheduled_send_ms,
    temperature: MESSAGE.temperature, humidity: MESSAGE.humidity, pressure: MESSAGE.pressure,
  }),
  thingsboard: JSON.stringify({
    run_id: MESSAGE.run_id, seq: MESSAGE.seq, scheduled_send_ms: MESSAGE.scheduled_send_ms,
    temperature: MESSAGE.temperature, humidity: MESSAGE.humidity, pressure: MESSAGE.pressure,
  }),
  edgex: JSON.stringify({
    run_id: MESSAGE.run_id, seq: MESSAGE.seq, scheduled_send_ms: MESSAGE.scheduled_send_ms,
    temperature: MESSAGE.temperature, humidity: MESSAGE.humidity, pressure: MESSAGE.pressure,
  }),
  fiware: `t|${MESSAGE.temperature}|h|${MESSAGE.humidity}|p|${MESSAGE.pressure}`
    + `|q|${MESSAGE.seq}|r|${MESSAGE.run_id}|s|${MESSAGE.scheduled_send_ms}`,
};
for (const adapter of ADAPTERS) {
  const native = nativePayload(adapter, { seed: SEED }, MESSAGE, DEVICE);
  assert.equal(native.payloadSelected, false, `${adapter}: legacy selection`);
  assert.equal(native.body, legacyExpected[adapter], `${adapter}: legacy body changed`);
  assert.equal(native.body.includes('"padding"') || native.body.includes('|d|'), false, `${adapter}: legacy padding leaked`);
}

// Size controls: below baseline, odd increments and beyond-maximum targets fail; no implicit rounding.
const logical = expectedLogical(MESSAGE, DEVICE, '');
const baseline = canonicalPayloadBytes(logical).length;
assert.throws(() => buildPayload(logical, SEED, baseline - 1), /represented/);
assert.throws(() => buildPayload(logical, SEED, baseline - 2), /represented/);
assert.throws(() => buildPayload(logical, SEED, baseline + 1), /represented/);
assert.throws(() => buildPayload(logical, SEED, (1 << 20) + 1), /target bytes/);
assert.throws(() => nativePayload('fixture', cfgWith(-1), MESSAGE, DEVICE), /size_offset/);
assert.throws(() => nativePayload('fixture', cfgWith(1), MESSAGE, DEVICE), /represented/);
assert.throws(() => nativePayload('fixture', cfgWith((1 << 20)), MESSAGE, DEVICE), /cannot be represented|maximum|bounds/);

// Deterministic same seed/seq bytes; different seq changes the deterministic padding.
const first = nativePayload('fixture', cfgWith(1024), MESSAGE, DEVICE);
const again = nativePayload('fixture', cfgWith(1024), MESSAGE, DEVICE);
assert.equal(first.body, again.body, 'same seed/seq is not deterministic');
const other = nativePayload('fixture', cfgWith(1024), { ...MESSAGE, seq: MESSAGE.seq + 1 }, DEVICE);
assert.notEqual(JSON.parse(other.body).padding, JSON.parse(first.body).padding, 'different seq reused padding');

// Timestamp identity and device-id boundaries remain the canonical encoder's decisions.
for (const time of [0, 0.001, 1700000000000.001, 9007199254740991]) {
  const m = { ...MESSAGE, scheduled_send_ms: time };
  const native = nativePayload('fixture', cfgWith(0), m, DEVICE);
  const token = time === 0 ? '0' : String(time);
  assert.ok(native.body.includes(`"scheduled_send_ms":${token},`), `timestamp token ${token} not preserved`);
}
for (const id of ['d', 'a'.repeat(128), 'A_1.b:c-d']) {
  assert.equal(nativePayload('fixture', cfgWith(0), MESSAGE, id).logical.device_id, id);
}
for (const id of ['', 'a'.repeat(129), '-leading', 'has|pipe', 'has space']) {
  assert.throws(() => nativePayload('fixture', cfgWith(0), MESSAGE, id), undefined, `device id accepted: ${id}`);
}

// A fixed offset adds exactly that many padding bytes to each message's own unpadded baseline; the
// aggregate logical bytes are the exact sum, not an assumed uniform grid length.
const aggregate = [];
let aggregateLogical = 0;
let aggregateBaseline = 0;
for (let seq = 0; seq < 50; seq++) {
  const m = { ...MESSAGE, seq };
  const x = nativePayload('fixture', cfgWith(1024), m, DEVICE);
  const b = canonicalPayloadBytes(expectedLogical(m, DEVICE, '')).length;
  assert.equal(x.padding.length, 1024, 'fixed offset changed padding length');
  assert.equal(x.logicalBytes.length, b + 1024, 'fixed offset changed logical byte count');
  aggregate.push(x);
  aggregateBaseline += b;
  aggregateLogical += x.logicalBytes.length;
}
assert.equal(aggregateLogical, aggregateBaseline + 50 * 1024);
assert.equal(new Set(aggregate.map((x) => x.padding)).size, 50, 'padding not deterministic by seq');

// Local loopback fixture server: the actual bytes received by an HTTP endpoint are validated.
const received = [];
const server = http.createServer((req, res) => {
  const chunks = [];
  req.on('data', (chunk) => chunks.push(chunk));
  req.on('end', () => {
    received.push({ url: req.url, body: Buffer.concat(chunks).toString('utf8'), headers: req.headers });
    res.writeHead(200, { 'Content-Type': 'application/json', 'Content-Length': '2' });
    res.end('{}');
  });
});
await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
const port = server.address().port;
try {
  for (const adapter of ADAPTERS) {
    const native = nativePayload(adapter, cfgWith(1024), MESSAGE, DEVICE);
    const res = await fetch(`http://127.0.0.1:${port}/${adapter}`, {
      method: 'POST', body: native.body, headers: native.headers,
    });
    assert.equal(res.status, 200);
    await res.text();
    const seen = received[received.length - 1];
    assert.equal(seen.body, native.body, `${adapter}: wire body changed in flight`);
    assert.equal(validateStoredPayload(
      expectedLogical(MESSAGE, DEVICE, native.padding),
      storedProjection(adapter, seen.body, DEVICE)), true, `${adapter}: received fixture body`);
  }
} finally {
  await new Promise((resolve) => server.close(resolve));
}

// Adapter-boundary outcomes: incomplete/oversized are distinct failures, never acknowledgements.
const semantics = [];
function statusCase(name, res, want) {
  const got = classifyStatus(res, 200);
  assert.equal(got.outcome, want, `${name}: ${JSON.stringify(got)}`);
  semantics.push({ name, outcome: got.outcome });
}
statusCase('normal 200', { status: 200, body: '', headers: {} }, 'ack_success');
statusCase('explicit 500', { status: 500, body: '', headers: {} }, 'explicit_failure');
statusCase('transport reset', { status: 0, error_code: 'connection_reset' }, 'unknown');
statusCase('declared oversized', { status: 200, body: '', headers: { 'Content-Length': String((4 << 20) + 1) } }, 'oversized_response');
statusCase('short body', { status: 200, body: '123', headers: { 'content-length': '10' } }, 'incomplete_response');
statusCase('body read error', { status: 0, error_code: 'response_body_read' }, 'incomplete_response');
assert.equal(BOUNDARY_OUTCOMES.includes('incomplete_response'), true);
assert.equal(BOUNDARY_OUTCOMES.includes('oversized_response'), true);
// The ledger keeps the legacy outcome vocabulary; boundary subtypes are detail diagnostics.
assert.deepEqual(ledgerOutcome({ outcome: 'ack_success' }), { outcome: 'ack_success', detail: '' });
assert.deepEqual(ledgerOutcome({ outcome: 'explicit_failure', detail: 'item_status_500' }),
  { outcome: 'explicit_failure', detail: 'item_status_500' });
assert.deepEqual(ledgerOutcome({ outcome: 'unknown', detail: 'transport:reset' }),
  { outcome: 'unknown', detail: 'transport:reset' });
assert.deepEqual(ledgerOutcome({ outcome: 'incomplete_response', detail: 'body_3_declared_10' }),
  { outcome: 'explicit_failure', detail: 'incomplete_response:body_3_declared_10' });
assert.deepEqual(ledgerOutcome({ outcome: 'oversized_response', detail: 'content_length_4194305_gt_4194304' }),
  { outcome: 'explicit_failure', detail: 'oversized_response:content_length_4194305_gt_4194304' });
for (const item of semantics.filter((x) => BOUNDARY_OUTCOMES.includes(x.outcome))) {
  assert.equal(ledgerOutcome(item).outcome, 'explicit_failure', `${item.name}: boundary became a ledger category`);
}
function batch(body) {
  return { status: 207, body, headers: { 'Content-Length': String(Buffer.byteLength(body)) } };
}
assert.equal(classify207(batch(JSON.stringify([{ statusCode: 201 }])), 4 << 20).outcome, 'ack_success');
assert.equal(classify207(batch(JSON.stringify([{ statusCode: 500 }])), 4 << 20).outcome, 'explicit_failure');
assert.equal(classify207({ status: 207, body: '[{"statusCode":', headers: { 'Content-Length': '100' } }, 4 << 20).outcome, 'incomplete_response');
assert.equal(classify207(batch('not-json'), 4 << 20).outcome, 'explicit_failure');

process.stdout.write(JSON.stringify({
  status: 'ok',
  label: 'SYNTHETIC OFFLINE ENGINEERING FIXTURE ONLY - not live evidence, not runtime capture, not storage proof',
  vectors: vectorCount,
  adapters: ADAPTERS,
  offsets: OFFSETS,
  built,
  aggregate: { messages: 50, logical_bytes_total: aggregateLogical, offset_bytes_each: 1024 },
  boundary: semantics,
}) + '\n');
