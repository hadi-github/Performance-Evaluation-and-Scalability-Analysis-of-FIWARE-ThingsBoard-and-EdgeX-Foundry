"""Readiness must observe the initialized ingest route, not an early ping."""
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'deploy/edgex/scripts/provision-then-scale.sh'


class DriverReadyTests(unittest.TestCase):
    def run_wait(self, codes, timeout='10'):
        text = SOURCE.read_text()
        function = text[text.index('wait_driver_ready() {'):text.index('\nmetadata_state()')]
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            (root / 'codes').write_text('\n'.join(codes) + '\n')
            curl = root / 'curl'
            curl.write_text('#!/usr/bin/env python3\n'
                            'import os,pathlib,sys\n'
                            'p=pathlib.Path(os.environ["FIXTURE"]); lines=(p/"codes").read_text().splitlines()\n'
                            'print(lines[0] if lines else "404",end="")\n'
                            '(p/"codes").write_text("\\n".join(lines[1:])+"\\n")\n'
                            'with (p/"calls").open("a") as f:f.write(" ".join(sys.argv[1:])+"\\n")\n')
            curl.chmod(0o755)
            sleep = root / 'sleep'
            sleep.write_text('#!/bin/sh\nexit 0\n')
            sleep.chmod(0o755)
            env = dict(os.environ, FIXTURE=d, EDGEX_DRIVER_READY_TIMEOUT=timeout,
                       PATH=d + os.pathsep + os.environ['PATH'])
            p = subprocess.run(['bash', '-c', function+'\nwait_driver_ready http://replica:59986'],
                               env=env, text=True, capture_output=True, timeout=15)
            calls = (root / 'calls').read_text() if (root / 'calls').exists() else ''
            return p, calls

    def test_waits_through_startup_until_route_is_registered(self):
        p, calls = self.run_wait(['404', '503', '405'])
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(len(calls.splitlines()), 3)
        self.assertNotIn('-X POST', calls)
        self.assertNotIn('/ping', calls)

    def test_deadline_fails_closed_without_provisioning(self):
        p, calls = self.run_wait(['404'], timeout='0')
        self.assertEqual(p.returncode, 4)
        self.assertEqual(calls, '')

    def test_driver_wait_precedes_any_provisioning(self):
        text = SOURCE.read_text()
        prepare = text[text.index('  prepare)'):text.index('  check)')]
        # The readiness gate now runs through ensure_replica_ready (bounded wait +
        # at most one repair restart); the invariant is that it still precedes
        # every device POST.
        self.assertIn('ensure_replica_ready', prepare)
        self.assertLess(prepare.index('ensure_replica_ready'), prepare.index('-X POST $META/deviceprofile'))
        # ...and that the readiness gate itself reaches wait_driver_ready.
        repair = text[text.index('ensure_replica_ready() {'):text.index('case "${1:-}" in')]
        self.assertIn('wait_driver_ready', repair)
        # No unbounded wait loop may be reintroduced into the setup path.
        self.assertNotIn('until ', prepare)
