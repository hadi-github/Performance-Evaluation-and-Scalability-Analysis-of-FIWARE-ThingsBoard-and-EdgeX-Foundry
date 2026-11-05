"""File-only exporter regression tests. Docker is replaced, never contacted."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
MOCK = r'''#!/usr/bin/env python3
import json, os, sys
query = sys.argv[-1]
mode = os.environ['HISTORY_TEST_MODE']
if 'information_schema.tables' in query:
    if mode == 'discovery_failure': sys.exit(23)
    if mode != 'no_tables': print('"mtbench"."etReading"')
elif 'true AS history_export_row_unrecoverable' in query:
    if mode == 'diagnostic_failure': sys.exit(24)
    if mode == 'unrecoverable': print(json.dumps({'history_export_row_unrecoverable': True, 'seq_raw_value': 'broken'}))
elif 'LATERAL' in query:
    if mode == 'recovery_failure': sys.exit(25)
else:
    if mode == 'typed_failure': sys.exit(26)
    if mode != 'empty': print(json.dumps({'run_id': 'SYNTHETIC-test', 'seq': 0, 'device': 'device-0', 'ts': 1700000000000}))
'''


class HistoryExportTest(unittest.TestCase):
    def run_export(self, mode):
        with tempfile.TemporaryDirectory(prefix="history-export-", dir="/tmp/opencode") as directory:
            mock = Path(directory) / "docker"
            mock.write_text(MOCK)
            mock.chmod(0o755)
            # The first PATH entry guarantees no real Docker is invoked.
            env = dict(os.environ, PATH=directory + os.pathsep + os.environ["PATH"], HISTORY_TEST_MODE=mode)
            return subprocess.run(["bash", str(ROOT / "scripts/history-ids.sh"), "fiware", "SYNTHETIC-test"],
                                  env=env, capture_output=True, text=True, check=False)

    def test_query_failures_are_not_successful_empty_captures(self):
        for mode in ("discovery_failure", "typed_failure", "recovery_failure", "diagnostic_failure", "no_tables"):
            with self.subTest(mode=mode):
                result = self.run_export(mode)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("unrecoverable=0", result.stderr)

    def test_success_preserves_identity(self):
        result = self.run_export("success")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["seq"], 0)
        self.assertIn("unrecoverable=0", result.stderr)

    def test_available_empty_table_is_not_unavailable(self):
        result = self.run_export("empty")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertIn("unrecoverable=0", result.stderr)

    def test_unrecoverable_rows_remain_explicit(self):
        result = self.run_export("unrecoverable")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('"history_export_row_unrecoverable": true', result.stderr)
        self.assertIn("unrecoverable=1", result.stderr)


if __name__ == "__main__":
    unittest.main()
