// Pure reference for logical-json-ascii-v1. Deliberately not wired into adapters.
import { mix } from './hash.js';

export const PAYLOAD_PADDING_SALT = 0x50414431;
export const PAYLOAD_MAX_BYTES = 1 << 20;

function validate(p) {
  if (p === null || typeof p !== 'object' || Array.isArray(p)) throw new Error('payload must be an object');
  const ident = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/;
  if (typeof p.run_id !== 'string' || typeof p.device_id !== 'string'
      || !ident.test(p.run_id) || !ident.test(p.device_id)) throw new Error('run_id and device_id must be bounded ASCII strings');
  if (!Number.isSafeInteger(p.seq) || p.seq < 0 || p.seq > 0xffffffff) throw new Error('seq outside uint32');
  if (typeof p.scheduled_send_ms !== 'number' || !Number.isFinite(p.scheduled_send_ms) || p.scheduled_send_ms < 0 || p.scheduled_send_ms > Number.MAX_SAFE_INTEGER
      || !atMostThreeDecimalPlaces(String(p.scheduled_send_ms))) {
    throw new Error('scheduled_send_ms must be safe, finite, nonnegative, and at most 3 decimals; never rounded');
  }
  if (!Number.isInteger(p.temperature) || p.temperature < 15 || p.temperature > 45
      || !Number.isInteger(p.humidity) || p.humidity < 30 || p.humidity > 80
      || !Number.isInteger(p.pressure) || p.pressure < 950 || p.pressure > 1150) throw new Error('numeric values outside contract ranges');
  if (typeof p.padding !== 'string' || p.padding.length > PAYLOAD_MAX_BYTES || !/^[0-9a-f]*$/.test(p.padding)) throw new Error('padding must be bounded lowercase hex');
}

function decimalParts(decimal) {
  const parts = decimal.toLowerCase().split('e');
  if (parts.length > 2) return null;
  const exponentText = parts[1] || '0';
  if (!/^[+-]?\d+$/.test(exponentText)) return null;
  const exponent = Number(exponentText);
  if (!Number.isSafeInteger(exponent) || Math.abs(exponent) > 10000) return null;
  let mantissa = parts[0].replace(/^-/, '');
  const dot = mantissa.indexOf('.');
  const fraction = dot < 0 ? 0 : mantissa.length - dot - 1;
  if (dot >= 0) mantissa = mantissa.slice(0, dot) + mantissa.slice(dot + 1);
  if (!/^\d+$/.test(mantissa)) return null;
  const coefficient = mantissa.replace(/^0+/, '');
  if (!coefficient) return { zero: true, scale: 0 };
  const trailing = (coefficient.match(/0+$/) || [''])[0].length;
  return { zero: false, scale: fraction - exponent - trailing };
}

function atMostThreeDecimalPlaces(decimal) {
  const p = decimalParts(decimal);
  return p !== null && (p.zero || p.scale <= 3);
}

export function canonicalPayloadBytes(payload) {
  validate(payload);
  const timestamp = Object.is(payload.scheduled_send_ms, -0) ? 0 : payload.scheduled_send_ms;
  // Object insertion order is normative and matches the Go struct declaration order.
  const encoded = JSON.stringify({ run_id: payload.run_id, seq: payload.seq, device_id: payload.device_id,
    scheduled_send_ms: timestamp, temperature: payload.temperature, humidity: payload.humidity,
    pressure: payload.pressure, padding: payload.padding });
  if (encoded.length > PAYLOAD_MAX_BYTES) throw new Error('encoded payload exceeds contract maximum');
  return encoded;
}

export function buildPayload(payload, seed, targetBytes) {
  if (!Number.isInteger(targetBytes) || targetBytes < 0 || targetBytes > PAYLOAD_MAX_BYTES) throw new Error('target bytes outside contract bounds');
  if (!Number.isInteger(seed) || seed < 0 || seed > 0xffffffff) throw new Error('seed outside uint32');
  const p = { ...payload, padding: '' };
  const base = canonicalPayloadBytes(p);
  const chars = targetBytes - base.length;
  if (chars < 0 || chars % 2) throw new Error(`target ${targetBytes} cannot be represented with hex padding`);
  let padding = '';
  for (let block = 0; padding.length < chars; block++) {
    const v = mix(seed >>> 0, p.seq >>> 0, (PAYLOAD_PADDING_SALT ^ block) >>> 0);
    padding += (v >>> 0).toString(16).padStart(8, '0');
  }
  p.padding = padding.slice(0, chars);
  const out = canonicalPayloadBytes(p);
  if (out.length !== targetBytes) throw new Error('internal payload sizing mismatch');
  return out;
}

export function buildPayloadOffset(payload, seed, offsetBytes) {
  if (!Number.isInteger(offsetBytes) || offsetBytes < 0 || offsetBytes > PAYLOAD_MAX_BYTES) throw new Error('payload offset outside contract bounds');
  const baseline = canonicalPayloadBytes({ ...payload, padding: '' });
  if (baseline.length + offsetBytes > PAYLOAD_MAX_BYTES) throw new Error('payload target exceeds contract maximum');
  return buildPayload(payload, seed, baseline.length + offsetBytes);
}

function strictRawJSON(text) {
  let i = 0; const rootRaw = Object.create(null);
  const ws = () => { while (i < text.length && /\s/.test(text[i])) i++; };
  function stringToken() {
    const start = i++;
    while (i < text.length) {
      if (text[i] === '\\') { i += 2; continue; }
      if (text[i++] === '"') return JSON.parse(text.slice(start, i));
    }
    throw new Error('unterminated JSON string');
  }
  function value(depth) {
    ws(); const c = text[i];
    if (c === '"') { stringToken(); return; }
    if (c === '{') {
      i++; ws(); const seen = new Set();
      if (text[i] === '}') { i++; return; }
      while (i < text.length) {
        ws(); if (text[i] !== '"') throw new Error('object key must be a string');
        const key = stringToken(); if (seen.has(key)) throw new Error(`duplicate JSON key ${key}`); seen.add(key);
        ws(); if (text[i++] !== ':') throw new Error('expected colon'); ws(); const rawStart = i;
        value(depth + 1); if (depth === 0) rootRaw[key] = text.slice(rawStart, i);
        ws(); const sep = text[i++]; if (sep === '}') return; if (sep !== ',') throw new Error('expected comma or object end');
      }
      throw new Error('unterminated JSON object');
    }
    if (c === '[') {
      i++; ws(); if (text[i] === ']') { i++; return; }
      while (i < text.length) { value(depth + 1); ws(); const sep = text[i++]; if (sep === ']') return; if (sep !== ',') throw new Error('expected comma or array end'); }
      throw new Error('unterminated JSON array');
    }
    const number = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/.exec(text.slice(i));
    if (number) { i += number[0].length; return; }
    for (const literal of ['true', 'false', 'null']) if (text.startsWith(literal, i)) { i += literal.length; return; }
    throw new Error(`invalid JSON value at byte ${i}`);
  }
  value(0); ws(); if (i !== text.length) throw new Error('trailing JSON data');
  return { value: JSON.parse(text), rootRaw };
}

function rawDecimalAtMostThreePlaces(token) {
  const p = decimalParts(token || '');
  return p !== null && (p.zero || p.scale <= 3);
}

function rawIntegerToken(token, min, max) {
  if (typeof token !== 'string' || !/^-?(?:0|[1-9]\d*)$/.test(token)) return false;
  const value = Number(token);
  return Number.isSafeInteger(value) && value >= min && value <= max && String(value) === token;
}

// Accept raw JSON only: duplicate keys cannot be detected after JSON.parse has erased them.
export function validateStoredPayload(expected, storedBytes) {
  if (typeof storedBytes !== 'string' && !(storedBytes instanceof Uint8Array)) throw new Error('stored payload must be raw JSON text or bytes');
  let text;
  try { text = typeof storedBytes === 'string' ? storedBytes : new TextDecoder('utf-8', { fatal: true }).decode(storedBytes); }
  catch (e) { throw new Error(`stored payload UTF-8 invalid: ${e.message}`); }
  let parsed;
  try { parsed = strictRawJSON(text); }
  catch (e) { throw new Error(`stored payload JSON invalid: ${e.message}`); }
  const actual = parsed.value;
  if (actual === null || typeof actual !== 'object' || Array.isArray(actual)) throw new Error('stored payload must be a JSON object');
  const keys = ['run_id', 'seq', 'device_id', 'scheduled_send_ms', 'temperature', 'humidity', 'pressure', 'padding'];
  if (Object.keys(actual).some((k) => !keys.includes(k)) || keys.some((k) => !(k in actual))) throw new Error('stored payload fields are incomplete or unknown');
  if (keys.some((k) => actual[k] === null)) throw new Error('stored payload fields must not be null');
  if (!rawDecimalAtMostThreePlaces(parsed.rootRaw.scheduled_send_ms)) throw new Error('stored timestamp has more than three effective decimal places');
  for (const [field, min, max] of [['seq', 0, 0xffffffff], ['temperature', 15, 45], ['humidity', 30, 80], ['pressure', 950, 1150]]) {
    if (!rawIntegerToken(parsed.rootRaw[field]?.trim(), min, max)) throw new Error(`stored ${field} must use an exact integer JSON token in range`);
  }
  const rawTimestamp = parsed.rootRaw.scheduled_send_ms.trim();
  const timestampParts = decimalParts(rawTimestamp);
  if (timestampParts === null || (!timestampParts.zero && actual.scheduled_send_ms === 0)) throw new Error('stored timestamp underflowed or has an unsupported exponent');
  if (rawTimestamp !== JSON.stringify(actual.scheduled_send_ms)
      && !(actual.scheduled_send_ms === 0 && timestampParts.zero)) throw new Error('stored timestamp is not in canonical exact decimal form');
  const got = canonicalPayloadBytes(actual); const want = canonicalPayloadBytes(expected);
  if (got !== want) throw new Error('stored payload fields differ from expected logical payload');
  return true;
}
