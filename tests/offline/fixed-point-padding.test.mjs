// Offline semantic test only: no k6, Docker, HTTP requests or platform mutation.
import assert from 'node:assert/strict';
import { nativePayload } from '../../workloads/k6/lib/adapter_payload.js';

const m={run_id:'w3dp-tb-r01-20261108',seq:7,scheduled_send_ms:1794080000000,
  temperature:20,humidity:40,pressure:1000};
const device=`bench-${m.run_id}-00007`;
for (const adapter of ['thingsboard','fiware','edgex']) {
  const config=offset=>({seed:202611061,payload:{encoding_id:'logical-json-ascii-v1',size_offset_bytes:offset}});
  const base=nativePayload(adapter,config(0),m,device);
  const padded=nativePayload(adapter,config(1024),m,device);
  assert.equal(Buffer.byteLength(padded.padding,'utf8'),1024);
  assert.match(padded.padding,/^[0-9a-f]{1024}$/);
  assert.equal(Buffer.byteLength(padded.logicalBytes)-Buffer.byteLength(base.logicalBytes),1024);
  const deliveredPadding=adapter==='fiware' ? padded.body.split('|d|')[1] : JSON.parse(padded.body).padding;
  assert.equal(deliveredPadding,padded.padding);
  // This is serialized BODY size, not measured network/interface wire size.
  assert.equal(Buffer.byteLength(padded.body)-Buffer.byteLength(base.body),adapter==='fiware'?1022:1024);
}
console.log('W3: exactly 1024 extra logical padding bytes on all three adapters; native-body sizes kept distinct from wire size');
