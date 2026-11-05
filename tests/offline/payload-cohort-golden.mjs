// SYNTHETIC OFFLINE ENGINEERING FIXTURE ONLY.
// Independent Node execution of the real k6 reference encoder against the shared M1c golden
// vectors. This is not a live producer, runtime capture, storage proof or clock qualification.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildPayload, canonicalPayloadBytes, validateStoredPayload } from '../../workloads/k6/lib/payload.js';
import { mix } from '../../workloads/k6/lib/hash.js';

const here = dirname(fileURLToPath(import.meta.url));
const argv = process.argv.slice(2);
function opt(name, fallback) {
  const i = argv.indexOf(name);
  return i >= 0 && i + 1 < argv.length ? argv[i + 1] : fallback;
}
const golden = JSON.parse(readFileSync(resolve(here, opt('--golden', 'payload-cohort-golden.json')), 'utf8'));
const storedPath = opt('--stored', null);
const sha = (text) => createHash('sha256').update(text, 'utf8').digest('hex');
const salt = 0x50414431;

function expectedPayload(goldenFile, message) {
  return {
    run_id: goldenFile.run_id, seq: message.seq, device_id: message.device_id,
    scheduled_send_ms: message.scheduled_send_ms, temperature: goldenFile.telemetry.temperature,
    humidity: goldenFile.telemetry.humidity, pressure: goldenFile.telemetry.pressure, padding: '',
  };
}

assert.equal(golden.label, 'SYNTHETIC OFFLINE ENGINEERING FIXTURE ONLY - not live evidence, not runtime capture, not storage proof');
assert.deepEqual(golden.size_offsets_bytes, [0, 1024, 4096]);
const results = [];
for (const message of [...golden.cohort, ...golden.edge_cases]) {
  const expected = expectedPayload(golden, message);
  assert.equal(canonicalPayloadBytes(expected), message.expected_baseline, `${message.name}: canonical baseline`);
  assert.equal(sha(message.expected_baseline), message.expected_baseline_sha256, `${message.name}: baseline sha256`);
  assert.ok(message.expected_baseline.includes(`"scheduled_send_ms":${message.timestamp_token},`), `${message.name}: exact timestamp token`);
  for (const size of message.sizes) {
    const target = message.expected_baseline.length + size.offset_bytes;
    assert.equal(target, size.target_bytes, `${message.name}/${size.offset_bytes}: target bytes`);
    const built = buildPayload(expected, golden.seed, target);
    assert.equal(built.length, target, `${message.name}/${size.offset_bytes}: built length`);
    assert.equal(sha(built), size.sha256, `${message.name}/${size.offset_bytes}: built sha256`);
    const parsed = JSON.parse(built);
    assert.equal(parsed.padding.length, size.padding_length, `${message.name}/${size.offset_bytes}: padding length`);
    assert.equal(sha(parsed.padding), size.padding_sha256, `${message.name}/${size.offset_bytes}: padding sha256`);
    let reconstructed = '';
    for (let block = 0; reconstructed.length < size.padding_length; block++) {
      reconstructed += mix(golden.seed >>> 0, message.seq >>> 0, (salt ^ block) >>> 0).toString(16).padStart(8, '0');
    }
    assert.equal(reconstructed.slice(0, size.padding_length), parsed.padding, `${message.name}/${size.offset_bytes}: padding reconstruction`);
    assert.equal(validateStoredPayload({ ...expected, padding: parsed.padding }, built), true, `${message.name}/${size.offset_bytes}: stored validation`);
    results.push({ name: message.name, seq: message.seq, offset_bytes: size.offset_bytes, length: built.length,
      sha256: sha(built), padding_sha256: sha(parsed.padding) });
  }
}

// Stored content must be validated from bytes; altered/omitted/truncated/wrong/null/duplicate input fails.
const zero = golden.edge_cases.find((m) => m.seq === 0 && m.device_id === 'd');
assert.ok(zero, 'zero edge vector present');
const zeroExpected = expectedPayload(golden, zero);
const zeroBody = zero.expected_baseline;
for (const [name, bad] of [
  ['altered', zeroBody.replace('"temperature":20', '"temperature":21')],
  ['omitted', zeroBody.replace(',"padding":""', '')],
  ['truncated', zeroBody.slice(0, -1)],
  ['wrong type', zeroBody.replace('"seq":0', '"seq":"0"')],
  ['null', zeroBody.replace('"scheduled_send_ms":0', '"scheduled_send_ms":null')],
  ['duplicate', zeroBody.replace('"seq":0', '"seq":0,"seq":0')],
]) {
  assert.throws(() => validateStoredPayload(zeroExpected, bad), undefined, `stored ${name} content accepted`);
}

// Timestamp identity and no arbitrary rounding.
assert.equal(canonicalPayloadBytes({ ...zeroExpected, scheduled_send_ms: -0 }), zeroBody.replace('"scheduled_send_ms":0', '"scheduled_send_ms":0'));
for (const time of [0.0001, 1048576.0011, Number.MAX_SAFE_INTEGER + 1]) {
  assert.throws(() => canonicalPayloadBytes({ ...zeroExpected, scheduled_send_ms: time }), /scheduled_send_ms/);
  assert.throws(() => buildPayload({ ...zeroExpected, scheduled_send_ms: time }, golden.seed, zeroBody.length), /scheduled_send_ms/);
}

// Target bounds use the real contract parity rules; no implicit rounding or default size.
const baseline = zeroBody.length;
assert.throws(() => buildPayload(zeroExpected, golden.seed, baseline - 1), /represented/);
assert.throws(() => buildPayload(zeroExpected, golden.seed, baseline + 1), /represented/);
assert.throws(() => buildPayload(zeroExpected, golden.seed, baseline + 0.5), /target bytes/);
assert.throws(() => buildPayload(zeroExpected, golden.seed, (1 << 20) + 1), /target bytes/);

let stored = null;
if (storedPath) {
  const storedOffset = Number(opt('--stored-offset', '4096'));
  assert.ok(golden.size_offsets_bytes.includes(storedOffset), `stored offset ${storedOffset} is not a declared size point`);
  const lines = readFileSync(resolve(here, storedPath), 'utf8').split('\n').filter((line) => line.length > 0);
  const bySeq = new Map(golden.cohort.map((m) => [m.seq, m]));
  for (const line of lines) {
    const decoded = JSON.parse(line);
    const message = bySeq.get(decoded.seq);
    assert.ok(message, `stored export seq ${decoded.seq} is not a cohort identity`);
    const expected = expectedPayload(golden, message);
    const target = message.expected_baseline.length + storedOffset;
    const expectedPadded = JSON.parse(buildPayload(expected, golden.seed, target));
    assert.equal(validateStoredPayload({ ...expected, padding: expectedPadded.padding }, line), true, `stored export seq ${decoded.seq}`);
  }
  stored = { path: storedPath, offset_bytes: storedOffset, lines: lines.length, validated: lines.length };
}

process.stdout.write(JSON.stringify({ status: 'ok', label: golden.label, messages: results, stored }) + '\n');
