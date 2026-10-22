// Run INSIDE an IoT Agent container (node + its node_modules/ws) after `kill -USR1 1` opened the inspector:
//   node /diag/node-cpuprof.js <seconds> > profile.cpuprofile
// Drives the V8 profiler of the agent process through the inspector protocol and prints the .cpuprofile JSON.
const http = require('http');
const WebSocket = require('/opt/iotaul/node_modules/ws');
const secs = Number(process.argv[2] || 60);
http.get('http://127.0.0.1:9229/json/list', (res) => {
  let b = ''; res.on('data', (c) => (b += c)); res.on('end', () => {
    const ws = new WebSocket(JSON.parse(b)[0].webSocketDebuggerUrl);
    let id = 0; const pending = {};
    const send = (method, params) => new Promise((ok) => { id += 1; pending[id] = ok; ws.send(JSON.stringify({ id, method, params })); });
    ws.on('message', (m) => { const r = JSON.parse(m); if (r.id && pending[r.id]) pending[r.id](r.result); });
    ws.on('open', async () => {
      await send('Profiler.enable');
      await send('Profiler.setSamplingInterval', { interval: 1000 }); // 1 ms
      await send('Profiler.start');
      setTimeout(async () => {
        const r = await send('Profiler.stop');
        ws.close(); process.stdout.write(JSON.stringify(r.profile), () => process.exit(0)); // exit only after the pipe drained
      }, secs * 1000);
    });
  });
});
