import os
import pathlib
import subprocess
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


class SinkFirewallOwnershipTests(unittest.TestCase):
    def run_stop(self, check_rc):
        temp = tempfile.TemporaryDirectory()
        root = pathlib.Path(temp.name)
        fakebin = root / "bin"
        fakebin.mkdir()
        iptables = fakebin / "iptables"
        iptables.write_text(
            "#!/usr/bin/env bash\n"
            "case \" $* \" in\n"
            "  *' -D '*) exit 2;;\n"
            f"  *' -C '*) exit {check_rc};;\n"
            "  *) exit 2;;\n"
            "esac\n"
        )
        iptables.chmod(0o755)
        pidfile = root / "sink.pid"
        pathlib.Path(str(pidfile) + ".rule-added").write_text("1\n")
        env = os.environ.copy()
        env.update({
            "PATH": f"{fakebin}:{env['PATH']}",
            "SINK_PID_FILE": str(pidfile),
            "SINK_PORT": "19991",
            "SINK_BIN": str(ROOT / "bin" / "sink"),
        })
        result = subprocess.run(
            ["bash", str(ROOT / "scripts" / "sink.sh"), "stop"],
            env=env, text=True, capture_output=True, check=False,
        )
        return temp, pathlib.Path(str(pidfile) + ".rule-added"), result

    def test_removes_ownership_marker_only_when_check_confirms_absent(self):
        temp, marker, result = self.run_stop(1)
        try:
            self.assertFalse(marker.exists())
            self.assertNotEqual(result.returncode, 0)  # marker-only cleanup is not a clean sink shutdown
        finally:
            temp.cleanup()

    def test_retains_ownership_marker_when_rule_check_errors(self):
        temp, marker, result = self.run_stop(2)
        try:
            self.assertTrue(marker.exists())
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("cannot verify", result.stderr)
        finally:
            temp.cleanup()


if __name__ == "__main__":
    unittest.main()
