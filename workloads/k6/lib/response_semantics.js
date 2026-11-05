// Adapter-boundary response classification.
//
// The sink's complete-body reader is the observation point; an adapter must not claim a receipt just
// because an HTTP status was 2xx. This module surfaces two diagnostic outcomes distinctly:
//
//   incomplete_response  the body was shorter than its declared Content-Length, or the transport
//                        reported a body/read error.
//   oversized_response   the declared response body exceeds the explicit adapter bound.
//
// Both are failures. They are not retried; retry policy is unchanged. The ledger keeps the legacy
// ack_success / explicit_failure / unknown outcome vocabulary via ledgerOutcome(); the boundary
// subtype remains a detail diagnostic and both are counted as explicit-failure subsets.
export const DEFAULT_RESPONSE_MAX_BYTES = 4 << 20; // 4 MiB, matching the sink body bound

export const BOUNDARY_OUTCOMES = ['incomplete_response', 'oversized_response'];

function utf8Length(text) {
  let bytes = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text.charCodeAt(i);
    if (c < 0x80) bytes += 1;
    else if (c < 0x800) bytes += 2;
    else if (c >= 0xd800 && c <= 0xdbff) { bytes += 4; i++; }
    else bytes += 3;
  }
  return bytes;
}

function headerValue(headers, wanted) {
  if (!headers || typeof headers !== 'object') return null;
  let found = null;
  for (const key of Object.keys(headers)) {
    if (key.toLowerCase() !== wanted) continue;
    const value = headers[key];
    found = Array.isArray(value) ? value[0] : value;
  }
  return found === undefined || found === null ? null : String(found);
}

function declaredLength(res) {
  const raw = headerValue(res && res.headers, 'content-length');
  if (raw === null || !/^\d+$/.test(raw.trim())) return null;
  const value = Number(raw.trim());
  return Number.isSafeInteger(value) ? value : null;
}

function bodyText(res) {
  return res && typeof res.body === 'string' ? res.body : null;
}

function bodyReadError(code) {
  return /body|content.?length|unexpected.?eof|truncat/i.test(String(code || ''));
}

// boundaryIssue returns a distinct outcome for an oversized/incomplete response, or null. It only
// compares the body length against Content-Length when a body string is actually available; with
// discardResponseBodies the transport error code and the declared size still remain visible.
export function boundaryIssue(res, maxBytes = DEFAULT_RESPONSE_MAX_BYTES) {
  if (!Number.isInteger(maxBytes) || maxBytes < 1) throw new Error('response maxBytes must be a positive integer');
  const declared = declaredLength(res);
  if (declared !== null && declared > maxBytes) {
    return { outcome: 'oversized_response', detail: 'content_length_' + declared + '_gt_' + maxBytes };
  }
  const code = res && res.error_code;
  if (bodyReadError(code)) {
    return { outcome: 'incomplete_response', detail: 'transport_' + String(code) };
  }
  const body = bodyText(res);
  // Only compare when a body string is actually present. With discardResponseBodies an empty/null
  // body is not evidence of truncation; the declared-length and transport-error checks above remain.
  if (body !== null && body.length > 0 && declared !== null && utf8Length(body) !== declared) {
    return { outcome: 'incomplete_response', detail: 'body_' + utf8Length(body) + '_declared_' + declared };
  }
  return null;
}

// ledgerOutcome maps a classifier result to the legacy outcome/detail pair written to records.ndjson.
// Boundary failures stay explicit_failure in the outcome field; their subtype is carried in detail so
// older ledger consumers never see a new category string.
export function ledgerOutcome(classification) {
  const c = classification || {};
  const boundary = BOUNDARY_OUTCOMES.includes(c.outcome);
  return {
    outcome: boundary ? 'explicit_failure' : c.outcome,
    detail: boundary ? c.outcome + ':' + (c.detail || '') : (c.detail || ''),
  };
}

export function classifyStatus(res, okStatus, maxBytes = DEFAULT_RESPONSE_MAX_BYTES) {
  const issue = boundaryIssue(res, maxBytes);
  if (issue) return issue;
  if (!res || res.status === 0) {
    return { outcome: 'unknown', detail: 'transport' + (res && res.error_code ? ':' + res.error_code : '') };
  }
  return res.status === okStatus ? { outcome: 'ack_success' } : { outcome: 'explicit_failure' };
}

// A 207 answer is a batch: success only if every item reports a 2xx statusCode. A truncated or
// oversized batch body is classified at the boundary before JSON parsing.
export function classify207(res, maxBytes = DEFAULT_RESPONSE_MAX_BYTES) {
  const issue = boundaryIssue(res, maxBytes);
  if (issue) return issue;
  let items;
  try {
    items = JSON.parse(res.body);
  } catch (e) {
    return { outcome: 'explicit_failure', detail: 'unparseable_207_body' };
  }
  if (!Array.isArray(items) || items.length === 0) return { outcome: 'explicit_failure', detail: 'empty_207_body' };
  const bad = items.find((x) => !(x && x.statusCode >= 200 && x.statusCode < 300));
  return bad ? { outcome: 'explicit_failure', detail: 'item_status_' + (bad && bad.statusCode) } : { outcome: 'ack_success' };
}
