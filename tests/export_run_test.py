"""Synthetic exporter regression: fractional times, gaps, NaN, absent data and query errors."""
import csv
import http.server
import json
import math
import pathlib
import subprocess
import sys
import tempfile
import threading
import unittest
import urllib.parse

try:
    import matplotlib  # noqa: F401
    HAVE_MATPLOTLIB = True
except ImportError:
    HAVE_MATPLOTLIB = False


class ExportTest(unittest.TestCase):
    @unittest.skipUnless(HAVE_MATPLOTLIB, "chart export is optional and needs matplotlib")
    def test_partial_export_keeps_missing_and_errors_unavailable(self):
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                q = urllib.parse.parse_qs(urllib.parse.urlsplit(self.path).query)
                expr = q['query'][0]
                start = float(q['start'][0])
                if expr == 'error':
                    body = {'status': 'error', 'error': 'deliberate query failure'}
                else:
                    result = [] if expr == 'missing' else [{'metric': {'step': '1'}, 'values': [[start, 'NaN' if expr == 'nan' else '12.5']]}]
                    body = {'status': 'success', 'data': {'resultType': 'matrix', 'result': result}}
                data = json.dumps(body).encode()
                self.send_response(200)
                self.end_headers()
                self.wfile.write(data)

            def log_message(self, *_):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            d = pathlib.Path(tmp)
            dashboard = {'panels': [{'id': i, 'title': name, 'fieldConfig': {'defaults': {'unit': 'ms'}}, 'targets': [{'expr': name, 'legendFormat': name + ' {{step}}'}]} for i, name in enumerate(('finite', 'nan', 'missing', 'error'), 1)]}
            (d / 'dashboard.json').write_text(json.dumps(dashboard))
            server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                command = [sys.executable, 'scripts/export-run.py', '--run-id', 'fixture', '--platform', 'edgex', '--start', '2026-10-29T12:00:00.123Z', '--end', '2026-10-29T12:00:10.123Z', '--context', '0', '--dashboard', str(d/'dashboard.json'), '--out', str(d/'out'), '--prometheus', f'http://127.0.0.1:{server.server_port}']
                p = subprocess.run(command, capture_output=True, text=True)
                self.assertEqual(p.returncode, 2, p.stderr)
                export = json.loads((d/'out/export.json').read_text())
                self.assertEqual([q['status'] for q in export['queries']], ['OK','NO_DATA','NO_DATA','ERROR'])
                self.assertIn('QUERY ERROR', (d/'out/panel-04.svg').read_text())
                self.assertIn('NO DATA', (d/'out/panel-02.svg').read_text())
                with (d/'out/measurements.csv').open() as f:
                    rows = list(csv.DictReader(f))
                finite = [r for r in rows if math.isfinite(float(r['value']))]
                self.assertEqual(len(finite), 1)
                self.assertEqual(float(finite[0]['value']), 12.5)
                self.assertTrue((d/'out/SHA256SUMS').exists())
            finally:
                server.shutdown()
                server.server_close()


if __name__ == '__main__':
    unittest.main()
