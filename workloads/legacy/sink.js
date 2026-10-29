// Generator-only workload: drives the external sink directly (no platform in the path).
// Purpose: measure the GENERATOR's own sustainable rate — scheduling (constant-arrival), payload, connection
// and recording path (sink NDJSON) — independent of any SUT. The receiver (bin/sink) is independently
// capacity-checked; this script exercises k6 + the arrival machinery + the sink's HTTP contract.
//
//   k6 run -e PLATFORM=sinkgen -e RATE=<r> -e DURATION=60s -e WARMUP=15s -e SINK_URL=http://127.0.0.1:18090 \
//          -e TESTID=<id> -e PRE_VUS=... -e MAX_VUS=... sink.js
//
// The sink's /metrics counters (received/unparsed/overflow) are the record-completeness gate; k6's
// dropped_iterations and the achieved/offered ratio are the schedule-integrity gates.
import http from 'k6/http';
import { check } from 'k6/metrics';
import exec from 'k6/execution';
import { cfg, makeOptions, makeSummary, sentOk } from './lib/common.js';

const SINK = __ENV.SINK_URL || 'http://127.0.0.1:18090';
const JSON_HDR = { headers: { 'Content-Type': 'application/json' }, tags: { name: 'send' } };

export const options = makeOptions();

export default function () {
  // one labelled message per arrival, round-robin over a synthetic device space.
  // The identity is (run_id, seq) with seq unique across the whole test (shared counter over all VUs and
  // scenarios, incl. warmup), so the sink's duplicate detection keys on real message identities, and stored
  // counts can exclude a missing message being offset by a duplicate.
  const seq = exec.scenario.iterationInTest;
  const res = http.post(SINK + '/sinkgen',
    JSON.stringify({ run_id: cfg.testid, seq: seq, device: 'gen-' + (seq % 100), ts: Date.now() }),
    { headers: JSON_HDR.headers, tags: { name: 'send' }, timeout: cfg.timeout, responseType: 'none' });
  if (res.status === 200) sentOk.add(1);
}

export function handleSummary(data) {
  // reuse the shared summary shape (valid/invalid_reasons incl. dropped iterations + achieved rate);
  // makeSummary writes summary.json/k6-summary.json/summary.txt itself
  return makeSummary(data, "skipped"); // no platform-side verify: the sink counters are the record gate
}
