// Regression test for the generator's complete-ledger switch.
//
// The ledger mode is decided in the k6 init context from the resolved plan. Source-level
// assertion only: no k6 is executed and no run is produced. This pins the defect observed on
// w1-qual-tb-20261110d, where a `dispatch-integrity-v2` plan fell through to the compact
// error-only log (406 rows of 45,000) because the switch named only v1.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(resolve(here, '../../workloads/k6/gen.js'), 'utf8');

// Both revisions must be treated as explicit dispatch-integrity policies. internal/plan
// .IsDispatchIntegrity() accepts exactly these two, and the generator must agree.
const policies = [...src.matchAll(/'dispatch-integrity-v(?:1|2)'/g)].map((m) => m[0]);
assert.ok(policies.includes("'dispatch-integrity-v1'"), 'v1 must enable the complete ledger (historical B0 behaviour)');
assert.ok(policies.includes("'dispatch-integrity-v2'"), 'v2 must enable the complete ledger (w1-qual-tb-20261110d regression)');

// The switch must consult the policy list rather than compare against a single literal,
// otherwise a future revision repeats the same failure silently.
assert.doesNotMatch(
  src,
  /COMPLETE_LEDGER\s*=\s*[^;]*===\s*'dispatch-integrity-v1'/,
  'COMPLETE_LEDGER must not be pinned to a single policy literal',
);
assert.match(src, /const COMPLETE_LEDGER = .*DISPATCH_POLICIES\.includes\(/, 'COMPLETE_LEDGER must include every dispatch-integrity revision');

// Historical unbound plans keep the compact log: absence of a policy must not force a
// full ledger, or B0 comparability and the recorded legacy verdicts change meaning.
assert.match(src, /!!cfg\.protocol_binding/, 'protocol-bound runs must still force the complete ledger');

console.log('complete-ledger policy: ok');
