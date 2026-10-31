import importlib.util
import io
import json
import pathlib
import threading
import unittest
import urllib.request

spec = importlib.util.spec_from_file_location('receiver', pathlib.Path(__file__).parents[1]/'scripts/adapter-receiver.py')
receiver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(receiver)


class ReceiverTest(unittest.TestCase):
    def test_all_wire_formats_preserve_identity(self):
        out = io.StringIO()
        state = receiver.State(out)
        server = receiver.Server(('127.0.0.1', 0), receiver.handler(state, {'127.0.0.1'}))
        threading.Thread(target=server.serve_forever, daemon=True).start()
        base = f'http://127.0.0.1:{server.server_port}'

        def post(path, obj, headers=None):
            body = obj.encode() if isinstance(obj, str) else json.dumps(obj).encode()
            with urllib.request.urlopen(urllib.request.Request(base+path, data=body, headers=headers or {})) as r:
                data = r.read()
                return r.status, json.loads(data) if data else None

        try:
            _, d = post('/api/device', {'name': 'tb-dev'}, {'X-Authorization': 'Bearer controlled-auth-token'})
            ident = d['id']['id']
            message = dict(run_id='run', seq=0, scheduled_send_ms=100, temperature=20, humidity=40, pressure=1000)
            self.assertEqual(post('/api/v1/'+ident+'/telemetry', message)[0], 200)
            post('/iot/services', {'services': [{'apikey': 'key'}]})
            post('/iot/devices', {'devices': [{'device_id': 'fw-dev'}]})
            self.assertEqual(post('/iot/d?k=key&i=fw-dev', 't|20|h|40|p|1000|q|1|r|run|s|101')[0], 200)
            post('/api/v3/device', [{'device': {'name': 'edge-dev'}}])
            message['seq'] = 2
            self.assertEqual(post('/api/v3/resource/edge-dev/reading', message)[0], 200)
            rows = [json.loads(line) for line in out.getvalue().splitlines()]
            self.assertEqual([r['seq'] for r in rows], [0, 1, 2])
            self.assertEqual([r['adapter'] for r in rows], ['thingsboard', 'fiware', 'edgex'])
            self.assertEqual(len({r['device_id'] for r in rows}), 3)
            self.assertEqual(state.errors, 0)
        finally:
            server.shutdown()
            server.server_close()


if __name__ == '__main__':
    unittest.main()
