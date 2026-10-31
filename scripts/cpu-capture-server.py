#!/usr/bin/env python3
"""Read-only, source-restricted CPU/clock helper for bounded qualification.

Projects are fixed by the operator, never supplied by HTTP clients. No deployment
or command-execution API is exposed. Run on the SUT, pinned to its monitoring core.
"""
import argparse
import http.server
import json
import pathlib
import signal
import subprocess
import threading
import time


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--port', type=int, default=18085)
    p.add_argument('--project', action='append', required=True)
    p.add_argument('--allow', action='append', default=['127.0.0.1'])
    a = p.parse_args()
    script = pathlib.Path(__file__).resolve().parents[1]/'monitoring/exporters/cpu-snapshot.sh'

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            if self.client_address[0] not in a.allow or self.path not in ('/snapshot', '/clock', '/inventory'):
                self.send_error(403)
                return
            if self.path == '/clock':
                data = json.dumps({'epoch_ns': str(time.time_ns()), 'clock': 'CLOCK_REALTIME, SUT'}).encode()
                self.send_response(200)
                self.end_headers()
                self.wfile.write(data)
                return
            if self.path == '/inventory':
                rows = []
                for project in a.project:
                    ids = subprocess.check_output(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project='+project], text=True).split()
                    if not ids:
                        continue
                    for c in json.loads(subprocess.check_output(['docker', 'inspect', *ids])):
                        rows.append(dict(project=project, service=c['Config']['Labels'].get('com.docker.compose.service'),
                                         name=c['Name'].lstrip('/'), id=c['Id'], image=c['Config']['Image'], image_id=c['Image'],
                                         cpuset=c['HostConfig']['CpusetCpus'], memory_bytes=c['HostConfig']['Memory'],
                                         swap_bytes=c['HostConfig']['MemorySwap'], started_at=c['State']['StartedAt'],
                                         restarts=c['RestartCount'], oom_killed=c['State']['OOMKilled'],
                                         health=c['State'].get('Health', {}).get('Status', 'no Docker healthcheck'),
                                         heap_options=[v for v in c['Config'].get('Env', []) if v.startswith(('JAVA_OPTS=', 'JAVA_TOOL_OPTIONS='))]))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(json.dumps(rows).encode())
                return
            output = []
            try:
                for project in a.project:
                    result = subprocess.run(['bash', str(script), project], capture_output=True, timeout=8, check=True)
                    if not result.stdout:
                        raise ValueError('empty project snapshot: ' + project)
                    output.append(result.stdout)
            except (subprocess.SubprocessError, ValueError) as e:
                self.send_error(503, str(e))
                return
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b''.join(output))

        def log_message(self, *_):
            pass

    server = http.server.ThreadingHTTPServer(('0.0.0.0', a.port), Handler)
    def stop(*_):
        threading.Thread(target=server.shutdown, daemon=True).start()
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    try:
        server.serve_forever()
    finally:
        server.server_close()


if __name__ == '__main__':
    main()
