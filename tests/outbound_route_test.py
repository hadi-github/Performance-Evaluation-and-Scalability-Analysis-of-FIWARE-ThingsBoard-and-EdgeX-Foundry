"""Orion sink setup must not silently reuse a legacy sink route."""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


@unittest.skipUnless(shutil.which('jq'), 'requires jq')
class OutboundRouteTests(unittest.TestCase):
    def invoke(self, urls):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            rows = [{'id': f'sub-{i}', 'description': 'bench sink',
                     'notification': {'http': {'url': url}}}
                    for i, url in enumerate(urls)]
            (root / 'subscriptions.json').write_text(json.dumps(rows))
            curl = root / 'curl'
            curl.write_text('#!/usr/bin/env python3\n'
                            'import json,os,pathlib,sys\n'
                            'root=pathlib.Path(os.environ["FAKE_ROOT"])\n'
                            'with (root/"calls.jsonl").open("a") as f: f.write(json.dumps(sys.argv[1:])+"\\n")\n'
                            'if "-X" not in sys.argv: print((root/"subscriptions.json").read_text())\n')
            curl.chmod(0o755)
            sleep = root / 'sleep'
            sleep.write_text('#!/bin/sh\nexit 0\n')
            sleep.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                       FAKE_ROOT=str(root), SINK_URL='http://sink:18091')
            p = subprocess.run(['bash', 'scripts/outbound.sh', 'fiware'], cwd=ROOT,
                               env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in (root / 'calls.jsonl').read_text().splitlines()]
            return p, calls

    def test_legacy_route_is_patched(self):
        p, calls = self.invoke(['http://sink:18090/fiware'])
        self.assertEqual(p.returncode, 0, p.stderr)
        patches = [c for c in calls if 'PATCH' in c]
        self.assertEqual(len(patches), 1)
        body = json.loads(patches[0][patches[0].index('--data-binary') + 1])
        self.assertEqual(body['notification']['http']['url'], 'http://sink:18091/fiware')

    def test_correct_route_is_unchanged(self):
        p, calls = self.invoke(['http://sink:18091/fiware'])
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(len(calls), 1)

    def test_ambiguous_routes_are_rejected(self):
        p, calls = self.invoke(['http://sink:18090/fiware', 'http://sink:18091/fiware'])
        self.assertNotEqual(p.returncode, 0)
        self.assertEqual(len(calls), 1)
