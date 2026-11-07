// Offline setup-contract test with k6 HTTP/execution stubs, never live evidence.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const source = fs.readFileSync('workloads/k6/lib/adapters.js', 'utf8')
  .replace(/^import .*;\n/gm, '')
  .replace('export const ADAPTERS =', 'globalThis.adapters =');
const cfg = { run_id: 'test-run', devices: 2,
  target: { metadata_url: 'http://meta', ingress_url: 'http://ingress' } };

function setup(env, missing = false) {
  const posts = [];
  const ctx = {
    __ENV: env, sleep() {},
    exec: { test: { abort(message) { throw new Error(message); } } },
    http: {
      get(url) { return { status: missing && url.includes('/device/name/') ? 404 : 200 }; },
      post(url, body) { posts.push({ url, body }); return { status: 200 }; },
    },
    classifyStatus(res) { return { outcome: res.status === 200 ? 'ack_success' : 'explicit_failure' }; },
  };
  vm.runInNewContext(source, ctx);
  return { run: () => ctx.adapters.edgex.setup(cfg), posts };
}

let t = setup({ EDGEX_PREPROVISIONED_RUN_ID: 'test-run' });
assert.equal(t.run().devices.length, 2);
assert.equal(t.posts.length, 0, 'a fully gated cohort must not inject setup events into the sink');
t = setup({});
t.run();
assert.equal(t.posts.length, 16, 'legacy setup must retain its existing probes');
t = setup({ EDGEX_PREPROVISIONED_RUN_ID: 'other-run' });
assert.throws(t.run, /gate identity/);
assert.equal(t.posts.length, 0);
t = setup({ EDGEX_PREPROVISIONED_RUN_ID: 'test-run' }, true);
assert.throws(t.run, /missing devices/);
assert.equal(t.posts.length, 0, 'a gated run must not provision at N=2');
console.log('EdgeX preprovision setup contract: 4 checks passed (offline stubs)');
