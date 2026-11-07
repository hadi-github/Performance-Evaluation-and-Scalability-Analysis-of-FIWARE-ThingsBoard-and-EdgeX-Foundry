"""Historical qualification entry point must fail before remote access in a clone."""
import pathlib
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class HistoricalPackagingTests(unittest.TestCase):
    def invoke(self, *args):
        with tempfile.TemporaryDirectory() as d:
            here = pathlib.Path(d) / 'results' / 'feasible-campaign-20261106'
            here.mkdir(parents=True)
            script = here / 'qualify-endpoint.py'
            shutil.copy2(ROOT / 'results' / here.name / script.name, script)
            result = subprocess.run([sys.executable, str(script), *args],
                                    capture_output=True, text=True, timeout=10)
            self.assertFalse((here / 'endpoint-qualification').exists())
            return result

    def test_default_invocation_is_explicitly_historical_and_does_not_launch(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Historical unsupported executor', result.stdout)

    def test_missing_original_companion_fails_before_output_or_remote_access(self):
        result = self.invoke('--execute-reviewed')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('original benchctl-calibrated.sh companion is external to Git', result.stderr)
