"""File-only tests for the M1e stored-payload export helper. Docker is replaced, never contacted."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
PADDING = "0123456789abcdef" * 64

MOCK = r'''#!/usr/bin/env python3
import json, os, sys
stdin = sys.stdin.read()
query = " ".join(sys.argv[1:]) + "\n" + stdin
mode = os.environ.get('HISTORY_PAYLOAD_TEST_MODE', 'success')
if mode == 'failure':
    sys.exit(27)
if 'information_schema.tables' in query:
    if mode == 'no_tables':
        pass
    else:
        print('"mtbench"."etReading"')
elif 'SELECT count(*)' in query:
    print('0')
elif 'FROM ts_kv r' in query:
    row = {'run_id': 'SYNTHETIC-m1e-stored', 'seq': 7, 'device_id': 'bench-SYNTHETIC-m1e-stored-00007',
           'scheduled_send_ms': 1700000007000, 'temperature': 20, 'humidity': 40, 'pressure': 1000,
           'padding': os.environ.get('HISTORY_PAYLOAD_PADDING', '0123456789abcdef')}
    if mode == 'missing_padding':
        row['padding'] = None
    print(json.dumps(row))
elif 'core_data.reading' in query:
    print(json.dumps({'run_id': 'SYNTHETIC-m1e-stored', 'seq': 7, 'device_id': 'bench-SYNTHETIC-m1e-stored-00007',
                      'scheduled_send_ms': 1700000007000, 'temperature': 20, 'humidity': 40, 'pressure': 1000,
                      'padding': os.environ.get('HISTORY_PAYLOAD_PADDING', '0123456789abcdef')}))
elif 'SELECT json_build_object' in query:
    print(json.dumps({'run_id': 'SYNTHETIC-m1e-stored', 'seq': 7, 'device_id': 'bench-SYNTHETIC-m1e-stored-00007',
                      'scheduled_send_ms': 1700000007000, 'temperature': 20, 'humidity': 40, 'pressure': 1000,
                      'padding': os.environ.get('HISTORY_PAYLOAD_PADDING', '0123456789abcdef')}))
'''


class HistoryPayloadTest(unittest.TestCase):
    def run_export(self, platform, mode='success', padding=PADDING):
        with tempfile.TemporaryDirectory(prefix='history-payload-', dir='/tmp/opencode') as directory:
            mock = Path(directory) / 'docker'
            mock.write_text(MOCK)
            mock.chmod(0o755)
            env = dict(os.environ, PATH=directory + os.pathsep + os.environ['PATH'],
                       HISTORY_PAYLOAD_TEST_MODE=mode, HISTORY_PAYLOAD_PADDING=padding)
            return subprocess.run(['bash', str(ROOT / 'scripts/history-payload.sh'), platform, 'SYNTHETIC-m1e-stored'],
                                  env=env, capture_output=True, text=True, check=False)

    def test_platform_exports_include_the_actual_padding_field(self):
        for platform in ('thingsboard', 'edgex', 'fiware'):
            with self.subTest(platform=platform):
                result = self.run_export(platform)
                self.assertEqual(result.returncode, 0, result.stderr)
                lines = [line for line in result.stdout.splitlines() if line.strip()]
                self.assertEqual(len(lines), 1, result.stdout)
                row = json.loads(lines[0])
                self.assertEqual(row['padding'], PADDING)
                self.assertEqual(row['run_id'], 'SYNTHETIC-m1e-stored')
                self.assertEqual(row['seq'], 7)
                self.assertEqual(row['device_id'], 'bench-SYNTHETIC-m1e-stored-00007')
                self.assertEqual(row['scheduled_send_ms'], 1700000007000)
                self.assertEqual((row['temperature'], row['humidity'], row['pressure']), (20, 40, 1000))

    def test_missing_padding_is_not_silently_replaced(self):
        result = self.run_export('thingsboard', mode='missing_padding')
        self.assertEqual(result.returncode, 0, result.stderr)
        row = json.loads(result.stdout)
        self.assertIsNone(row['padding'], 'export invented padding instead of surfacing the stored null')

    def test_query_failures_are_not_successful_empty_exports(self):
        for platform in ('thingsboard', 'edgex', 'fiware'):
            with self.subTest(platform=platform):
                result = self.run_export(platform, mode='failure')
                self.assertNotEqual(result.returncode, 0)

    def test_fiware_no_tables_is_unavailable(self):
        result = self.run_export('fiware', mode='no_tables')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('capture unavailable', result.stderr)


if __name__ == '__main__':
    unittest.main()
