import assert from 'node:assert/strict';
import { buildPayload, buildPayloadOffset, canonicalPayloadBytes, validateStoredPayload } from '../../workloads/k6/lib/payload.js';

const p = { run_id: 'pilot-1', seq: 17, device_id: 'device-17', scheduled_send_ms: 1234.125,
  temperature: 20, humidity: 40, pressure: 1000, padding: '' };
const canonical = '{"run_id":"pilot-1","seq":17,"device_id":"device-17","scheduled_send_ms":1234.125,"temperature":20,"humidity":40,"pressure":1000,"padding":""}';
const padded = '{"run_id":"pilot-1","seq":17,"device_id":"device-17","scheduled_send_ms":1234.125,"temperature":20,"humidity":40,"pressure":1000,"padding":"a9f0653a6090799138"}';
assert.equal(canonicalPayloadBytes(p), canonical);
assert.equal(buildPayload(p, 7, 160), padded);
assert.equal(buildPayloadOffset(p, 7, 1024).length, canonical.length + 1024);
assert.equal(validateStoredPayload(p, canonical), true);
assert.throws(() => validateStoredPayload(p, canonical.slice(0, -4)), /invalid/);
assert.throws(() => validateStoredPayload(p, canonical.replace('"temperature":20', '"temperature":21')), /differ/);

for (const field of ['run_id', 'device_id']) {
  for (const invalid of [undefined, null, 42, ['pilot-1'], { id: 'pilot-1' }]) {
    const bad = { ...p, [field]: invalid };
    assert.throws(() => canonicalPayloadBytes(bad), /ASCII strings/);
  }
}

for (const time of [0, 0.001, 1048576.001, Number.MAX_SAFE_INTEGER, -0]) {
  assert.doesNotThrow(() => canonicalPayloadBytes({ ...p, scheduled_send_ms: time }));
}
for (const time of [0.0001, 1048576.0011, Number.MAX_SAFE_INTEGER + 1]) {
  assert.throws(() => canonicalPayloadBytes({ ...p, scheduled_send_ms: time }), /scheduled_send_ms/);
}
const minusZero = canonicalPayloadBytes({ ...p, scheduled_send_ms: -0 });
const plusZero = canonicalPayloadBytes({ ...p, scheduled_send_ms: 0 });
assert.equal(minusZero, plusZero);
assert.match(minusZero, /"scheduled_send_ms":0/);
const timestampVector = { ...p, seq: 1, device_id: 'device-1', temperature: 20, humidity: 40, pressure: 1000 };
for (const [time, expected] of [
  [0, '{"run_id":"pilot-1","seq":1,"device_id":"device-1","scheduled_send_ms":0,"temperature":20,"humidity":40,"pressure":1000,"padding":""}'],
  [1048576.001, '{"run_id":"pilot-1","seq":1,"device_id":"device-1","scheduled_send_ms":1048576.001,"temperature":20,"humidity":40,"pressure":1000,"padding":""}'],
  [Number.MAX_SAFE_INTEGER, '{"run_id":"pilot-1","seq":1,"device_id":"device-1","scheduled_send_ms":9007199254740991,"temperature":20,"humidity":40,"pressure":1000,"padding":""}'],
]) assert.equal(canonicalPayloadBytes({ ...timestampVector, scheduled_send_ms: time }), expected);

const timestampTokens = new Set(['0', '1', '1700000000000', '1793664000000', '9007199254740990', '9007199254740991']);
for (let power = 1_000_000n; power <= 1_000_000_000_000_000n; power *= 10n) {
  timestampTokens.add(String(power - 1n)); timestampTokens.add(String(power)); timestampTokens.add(String(power + 1n));
}
for (const token of timestampTokens) {
  const value = Number(token);
  const payload = { ...timestampVector, scheduled_send_ms: value };
  const encoded = canonicalPayloadBytes(payload);
  assert.ok(encoded.includes(`"scheduled_send_ms":${token},`), `canonical bytes preserve ${token}`);
  assert.equal(validateStoredPayload(payload, encoded), true, `stored validation accepts ${token}`);
}
for (const token of ['1000000.001', '1700000000000.001', '1793664000000.001']) {
  const value = Number(token);
  const payload = { ...timestampVector, scheduled_send_ms: value };
  const encoded = canonicalPayloadBytes(payload);
  assert.ok(encoded.includes(`"scheduled_send_ms":${token},`), `canonical bytes preserve ${token}`);
  assert.equal(validateStoredPayload(payload, encoded), true, `stored validation accepts ${token}`);
}

const zeroExpected = { ...p, seq: 0, scheduled_send_ms: 0, temperature: 15, humidity: 30, pressure: 950 };
const zeroBody = canonicalPayloadBytes(zeroExpected);
assert.equal(validateStoredPayload(zeroExpected, zeroBody.replace('"scheduled_send_ms":0', '"scheduled_send_ms":-0.0')), true);
for (const bad of [
  zeroBody.replace('"seq":0,', ''), zeroBody.replace('"seq":0', '"seq":null'),
  zeroBody.replace('"seq":0', '"seq":"0"'),
  zeroBody.replace('"scheduled_send_ms":0', '"scheduled_send_ms":null'),
  zeroBody.replace('"scheduled_send_ms":0', '"scheduled_send_ms":"0"'),
  zeroBody.replace('"padding":""', ''), zeroBody.replace('"padding":""', '"padding":null'),
  zeroBody.replace('"padding":""', '"padding":0'),
  zeroBody.replace('"run_id":"pilot-1"', '"run_id":0'),
  zeroBody.replace('"run_id":"pilot-1"', '"run_id":"evil","run_id":"pilot-1"'),
  zeroBody.replace('"seq":0', '"seq":1,"seq":0'),
]) assert.throws(() => validateStoredPayload(zeroExpected, bad));

const exactTime = { ...zeroExpected, scheduled_send_ms: 1048576.001 };
const exactBody = canonicalPayloadBytes(exactTime);
assert.equal(validateStoredPayload(exactTime, exactBody), true);
assert.throws(() => validateStoredPayload(exactTime, exactBody.replace('1048576.001', '1048576.0010000000001')), /decimal places/);

for (const [field, token] of [
  ['seq', '1e-400'], ['temperature', '15.0000000000000001'],
  ['humidity', '30.0000000000000001'], ['pressure', '950.0000000000000001'],
  ['seq', '0.0'], ['temperature', '15e0'], ['humidity', '30.0'], ['pressure', '950.0'],
]) {
  const raw = zeroBody.replace(new RegExp(`"${field}":[^,}]+`), `"${field}":${token}`);
  assert.throws(() => validateStoredPayload(zeroExpected, raw), /integer/);
}
for (const token of ['1e-400', '1e-9223372036854775808', '1e999999999999999999999', '-1e-400', '0e10001', '-0e-10001']) {
  const raw = zeroBody.replace('"scheduled_send_ms":0', `"scheduled_send_ms":${token}`);
  assert.throws(() => validateStoredPayload(zeroExpected, raw));
}
for (const token of ['-0', '-0.0', '-0e3']) {
  const raw = zeroBody.replace('"scheduled_send_ms":0', `"scheduled_send_ms":${token}`);
  assert.equal(validateStoredPayload(zeroExpected, raw), true);
}

const maximum = 1 << 20;
const maxPayload = { ...zeroExpected, seq: 0xffffffff };
const baseBytes = canonicalPayloadBytes(maxPayload).length;
const maxFeasible = maximum - ((maximum - baseBytes) % 2);
const atMax = buildPayload(maxPayload, 0xffffffff, maxFeasible);
assert.equal(atMax.length, maxFeasible);
assert.throws(() => buildPayload(maxPayload, 0, maxFeasible + 1), /target bytes/);
assert.throws(() => buildPayload(maxPayload, 0, maximum + 1), /target bytes/);
assert.throws(() => buildPayload(maxPayload, 0, baseBytes + 1), /represented/);
assert.throws(() => buildPayload(maxPayload, 0x100000000, baseBytes), /seed/);
assert.throws(() => buildPayload({ ...maxPayload, seq: 0x100000000 }, 0, baseBytes), /seq/);
assert.throws(() => buildPayloadOffset(maxPayload, 0, maximum), /exceeds contract maximum/);
assert.throws(() => buildPayloadOffset(maxPayload, 0, maximum + 1), /offset/);
assert.throws(() => buildPayloadOffset(maxPayload, 0, -1), /offset/);
const reviewerBoundary = { ...zeroExpected, run_id: 'r', seq: 0, device_id: 'd' };
assert.equal(canonicalPayloadBytes(reviewerBoundary).length, 119);
assert.equal(buildPayload(reviewerBoundary, 0, 1048575).length, 1048575);
console.log('protocol payload golden vectors: OK');
