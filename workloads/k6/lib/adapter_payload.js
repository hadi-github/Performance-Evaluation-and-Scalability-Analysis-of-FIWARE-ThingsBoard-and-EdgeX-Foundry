// Adapter-native projection of the existing logical-json-ascii-v1 payload.
//
// This module deliberately does not encode, pad, round or truncate anything itself. It delegates the
// complete logical body to payload.js (`canonicalPayloadBytes` / `buildPayload`) and then maps that
// already-encoded object onto each platform's native wire shape. The canonical body remains the
// source of truth for run_id, seq, device_id, scheduled_send_ms, temperature, humidity, pressure and
// padding; the platform mapping only changes container/key spelling.
//
// Payload selection is explicit: `cfg.payload` must carry the encoding id and an integer
// `size_offset_bytes`. Its absence preserves the historical adapter body byte-for-byte. There is no
// implicit baseline/1KiB/4KiB default and no fallback to a "nearest" supported size.
import { buildPayload, canonicalPayloadBytes } from './payload.js';

export const ADAPTER_PAYLOAD_ENCODING = 'logical-json-ascii-v1';

const JSON_HDR = { 'Content-Type': 'application/json' };

function selectedPlan(cfg) {
  if (cfg === null || typeof cfg !== 'object') throw new Error('adapter payload requires a resolved plan object');
  const plan = cfg.payload;
  if (plan === undefined || plan === null) return null;
  if (typeof plan !== 'object' || Array.isArray(plan)) throw new Error('payload must be an explicit object');
  if (plan.encoding_id !== ADAPTER_PAYLOAD_ENCODING) {
    throw new Error(`unsupported payload encoding_id ${JSON.stringify(plan.encoding_id)}`);
  }
  if (!Number.isInteger(plan.size_offset_bytes) || plan.size_offset_bytes < 0) {
    throw new Error('payload.size_offset_bytes must be an explicit non-negative integer; no implicit size point');
  }
  return plan;
}

export function payloadSelected(cfg) {
  return selectedPlan(cfg) !== null;
}

export function logicalPayload(m, deviceID) {
  return {
    run_id: m.run_id,
    seq: m.seq,
    device_id: deviceID,
    scheduled_send_ms: m.scheduled_send_ms,
    temperature: m.temperature,
    humidity: m.humidity,
    pressure: m.pressure,
    padding: '',
  };
}

// buildLogical returns the canonical body and its decoded fields. When payload selection is off it
// still supplies the canonical baseline for diagnostics, but marks payloadSelected=false so callers
// keep emitting the legacy native body.
export function buildLogical(cfg, m, deviceID) {
  const plan = selectedPlan(cfg);
  const logical = logicalPayload(m, deviceID);
  const baseline = canonicalPayloadBytes(logical);
  if (plan === null) {
    return { payloadSelected: false, logical, baselineBytes: baseline, bytes: baseline, padding: '', targetBytes: null, offset: null };
  }
  const target = baseline.length + plan.size_offset_bytes;
  const bytes = buildPayload(logical, cfg.seed, target);
  const stored = JSON.parse(bytes);
  return {
    payloadSelected: true,
    logical: stored,
    baselineBytes: baseline,
    bytes,
    padding: stored.padding,
    targetBytes: target,
    offset: plan.size_offset_bytes,
  };
}

function legacyFixtureBody(m, deviceID) {
  return JSON.stringify({
    run_id: m.run_id, seq: m.seq, device_id: deviceID, scheduled_send_ms: m.scheduled_send_ms,
    temperature: m.temperature, humidity: m.humidity, pressure: m.pressure,
  });
}

function legacyPlatformBody(m) {
  return JSON.stringify({
    run_id: m.run_id, seq: m.seq, scheduled_send_ms: m.scheduled_send_ms,
    temperature: m.temperature, humidity: m.humidity, pressure: m.pressure,
  });
}

function legacyUltraLightBody(m) {
  return 't|' + m.temperature + '|h|' + m.humidity + '|p|' + m.pressure
    + '|q|' + m.seq + '|r|' + m.run_id + '|s|' + m.scheduled_send_ms;
}

function ultraLightBody(stored) {
  return 't|' + stored.temperature + '|h|' + stored.humidity + '|p|' + stored.pressure
    + '|q|' + stored.seq + '|r|' + stored.run_id + '|s|' + stored.scheduled_send_ms
    + '|d|' + stored.padding;
}

// nativePayload is the single adapter-facing constructor. Every platform receives exactly the
// canonical bytes when its native format is JSON; FIWARE receives the same fields as UltraLight
// attributes, with the padding alias provisioned only in explicit payload mode.
export function nativePayload(adapter, cfg, m, deviceID) {
  const built = buildLogical(cfg, m, deviceID);
  let body;
  let headers;
  let mapping;
  switch (adapter) {
    case 'fixture':
      body = built.payloadSelected ? built.bytes : legacyFixtureBody(m, deviceID);
      headers = JSON_HDR;
      mapping = 'fixture-canonical-json-body';
      break;
    case 'thingsboard':
      body = built.payloadSelected ? built.bytes : legacyPlatformBody(m);
      headers = JSON_HDR;
      mapping = 'thingsboard-http-telemetry-json-keys';
      break;
    case 'edgex':
      body = built.payloadSelected ? built.bytes : legacyPlatformBody(m);
      headers = JSON_HDR;
      mapping = 'edgex-device-rest-object-reading';
      break;
    case 'fiware':
      body = built.payloadSelected ? ultraLightBody(built.logical) : legacyUltraLightBody(m);
      headers = { 'Content-Type': 'text/plain' };
      mapping = 'fiware-ultralight-2.0-text-attributes';
      break;
    default:
      throw new Error('unknown adapter ' + String(adapter));
  }
  return {
    body,
    headers,
    mapping,
    payloadSelected: built.payloadSelected,
    logicalBytes: built.bytes,
    logical: built.logical,
    baselineBytes: built.baselineBytes,
    padding: built.padding,
    targetBytes: built.targetBytes,
    offset: built.offset,
  };
}

function httpHead(url, headers) {
  const names = Object.keys(headers || {}).sort();
  let head = 'POST ' + url + ' HTTP/1.1\r\n';
  for (const name of names) head += name + ': ' + headers[name] + '\r\n';
  return head + '\r\n';
}

// wireAccounting reports the three quantities separately: canonical logical bytes, the adapter's
// serialized body bytes, and the HTTP request-head overhead. `adapter_expansion_bytes` is the
// native-format expansion relative to the logical body; it is not a scientific payload-size claim.
export function wireAccounting(url, headers, native) {
  const head = httpHead(url, headers);
  const logical = native.logicalBytes.length;
  return {
    logical_bytes: logical,
    adapter_body_bytes: native.body.length,
    http_overhead_bytes: head.length,
    adapter_expansion_bytes: native.body.length - logical,
    wire_total_bytes: head.length + native.body.length,
  };
}
