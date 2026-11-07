#!/usr/bin/env python3
"""Controlled receiver for UNCHANGED k6 platform adapters; not a platform emulator.

Implements only the setup/ingress API subset used by adapters.js. Credentials are
synthetic, storage is a receipt ledger, and there is no platform processing,
durability, history, rule engine or notification path. Output is create-exclusive.
"""
import argparse
import hashlib
import http.server
import json
import signal
import socket
import threading
import time
import urllib.parse


class State:
    def __init__(self, output, delay=0, token_length=32):
        self.output = output
        self.delay = delay
        self.token_length = token_length
        self.lock = threading.Lock()
        self.devices = {}
        self.tokens = {}
        self.credentials = {}
        self.groups = set()
        self.received = 0
        self.errors = 0
        self.setup_requests = 0

    def record(self, item):
        with self.lock:
            try:
                self.output.write(json.dumps(item, separators=(',', ':')) + '\n')
                self.output.flush()
            except OSError:
                self.errors += 1
                raise
            self.received += 1


class Server(http.server.ThreadingHTTPServer):
    request_queue_size = 2048
    daemon_threads = False


def handler(state, allow):
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = 'HTTP/1.1'

        def setup(self):
            super().setup()
            self.connection.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
            self.connection.settimeout(15)

        def log_message(self, *_):
            pass

        def reply(self, code, value=None):
            body = b'' if value is None else json.dumps(value).encode()
            self.send_response(code)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            self.dispatch()

        def do_POST(self):
            self.dispatch()

        def dispatch(self):
            if self.client_address[0] not in allow:
                self.close_connection = True
                return self.reply(403)
            recv_ns = time.time_ns()
            u = urllib.parse.urlsplit(self.path)
            path = u.path
            raw = self.rfile.read(min(int(self.headers.get('Content-Length', 0)), 2 << 20)).decode()
            try:
                body = json.loads(raw) if raw and path != '/iot/d' else None
                if path == '/stats':
                    with state.lock:
                        return self.reply(200, dict(received=state.received, recording_errors=state.errors, setup_requests=state.setup_requests))
                if path == '/api/auth/login':
                    return self.reply(200, {'token': 'controlled-auth-token'})
                if path == '/api/device' and self.command == 'POST':
                    if self.headers.get('X-Authorization') != 'Bearer controlled-auth-token':
                        return self.reply(401)
                    name = body['name']
                    ident = hashlib.sha256(name.encode()).hexdigest()[:32]
                    with state.lock:
                        state.devices[ident] = name
                        credential = ident[:state.token_length]
                        state.tokens[credential] = name
                        state.credentials[ident] = credential
                        state.setup_requests += 1
                    return self.reply(200, {'id': {'id': ident}})
                if path.startswith('/api/device/') and path.endswith('/credentials'):
                    ident = path.split('/')[3]
                    return self.reply(200, {'credentialsId': state.credentials.get(ident, ident)})
                if path == '/iot/services':
                    with state.lock:
                        state.groups.update(g['apikey'] for g in body['services'])
                    return self.reply(201, {})
                if path == '/iot/devices':
                    with state.lock:
                        state.devices.update({d['device_id']: d['device_id'] for d in body['devices']})
                        state.setup_requests += len(body['devices'])
                    return self.reply(201, {})
                if path.startswith('/api/v3/deviceprofile/name/'):
                    return self.reply(200, {'name': 'bench-gen-v1'})
                if path.startswith('/api/v3/device/name/'):
                    return self.reply(200 if path.rsplit('/', 1)[-1] in state.devices else 404, {})
                if path == '/api/v3/device':
                    with state.lock:
                        state.devices.update({d['device']['name']: d['device']['name'] for d in body})
                        state.setup_requests += len(body)
                    return self.reply(207, [{'statusCode': 201} for _ in body])
                if path.startswith('/api/v1/') and path.endswith('/telemetry'):
                    device = state.tokens.get(path.split('/')[3])
                    adapter = 'thingsboard'
                    message = body
                elif path == '/iot/d':
                    query = urllib.parse.parse_qs(u.query)
                    if query.get('k', [''])[0] not in state.groups:
                        return self.reply(403)
                    device = state.devices.get(query['i'][0])
                    parts = raw.split('|')
                    fields = dict(zip(parts[::2], parts[1::2]))
                    message = dict(run_id=fields['r'], seq=int(fields['q']), scheduled_send_ms=float(fields['s']), temperature=int(fields['t']), humidity=int(fields['h']), pressure=int(fields['p']))
                    if 'd' in fields:
                        # Explicit M1e payload mode: the padding alias is stored as actual wire text.
                        message['padding'] = '' if fields['d'] == '""' else fields['d']
                    adapter = 'fiware'
                elif path.startswith('/api/v3/resource/') and path.endswith('/reading'):
                    device = state.devices.get(path.split('/')[4])
                    adapter = 'edgex'
                    message = body
                else:
                    return self.reply(404)
                if device is None:
                    return self.reply(404)
                assert isinstance(message['run_id'], str)
                assert isinstance(message['seq'], int)
                for key in ('scheduled_send_ms', 'temperature', 'humidity', 'pressure'):
                    assert isinstance(message[key], (int, float))
                # Payloads can themselves carry device_id. Receiver identity is
                # authoritative, so merge first and set the receipt metadata last.
                record = dict(message)
                record.update(device_id=device, adapter=adapter, recv_ns=str(recv_ns),
                              recv_ms=recv_ns/1e6, request_path=self.path, body=raw,
                              status=200)
                state.record(record)
                if state.delay:
                    time.sleep(state.delay)
                return self.reply(200)
            except (KeyError, ValueError, TypeError, AssertionError):
                return self.reply(400)
            except OSError:
                return self.reply(507)
    return Handler


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--port', type=int, required=True)
    p.add_argument('--out', required=True)
    p.add_argument('--delay-ms', type=int, default=0)
    p.add_argument('--token-length', type=int, default=32,
                   help='synthetic ThingsBoard credential length (default: 32)')
    p.add_argument('--allow', action='append', default=['127.0.0.1'])
    a = p.parse_args()
    with open(a.out, 'x', buffering=1) as f:
        if a.token_length < 1:
            p.error('--token-length must be positive')
        state = State(f, a.delay_ms/1000, a.token_length)
        server = Server(('0.0.0.0', a.port), handler(state, set(a.allow)))
        def stop(*_):
            threading.Thread(target=server.shutdown, daemon=True).start()
        signal.signal(signal.SIGTERM, stop)
        signal.signal(signal.SIGINT, stop)
        try:
            server.serve_forever()
        finally:
            server.server_close()
            print(json.dumps(dict(received=state.received, recording_errors=state.errors)), flush=True)
        return 3 if state.errors else 0


if __name__ == '__main__':
    raise SystemExit(main())
