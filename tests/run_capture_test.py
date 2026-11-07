"""Offline test for scripts/run-capture.sh orchestration.

The wrapper is exercised in a throw-away tree with fake sampler/capture/analysis
scripts. This proves the ordering and environment contract (calibrated GOGC,
prom remote-write, Grafana annotation, sink port, cpuset) and that the offline
analysis still runs when the capture itself exits non-zero.
"""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
RID = "capture-fixture"


class RunCaptureTests(unittest.TestCase):
    def tree(self, d):
        root = pathlib.Path(d)
        (root / "scripts").mkdir()
        (root / "bin").mkdir()
        shutil.copy2(ROOT / "scripts" / "run-capture.sh", root / "scripts" / "run-capture.sh")
        shutil.copy2(ROOT / "scripts" / "benchctl-calibrated.sh", root / "scripts" / "benchctl-calibrated.sh")
        # Fake bin/benchctl: record argv so benchctl-calibrated.sh forwarding is observable.
        (root / "bin" / "benchctl").write_text(
            "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> \"$TEST_DIR/benchctl-args.log\"\n"
        )
        (root / "bin" / "sink").write_text("#!/usr/bin/env bash\nexit 0\n")
        (root / "scripts" / "analyze-dispatch-run.py").write_text("#!/usr/bin/env python3\n")
        # Fake sampler: write a line, then the ready file.
        (root / "scripts" / "generator-diagnostic-sampler.py").write_text(
            "import os,pathlib,sys,time\n"
            "out=pathlib.Path(sys.argv[2])\n"
            "out.write_text('{\"load_realtime_ns\":\"1\"}\\n')\n"
            "ready=sys.argv[sys.argv.index('--ready-file')+1]\n"
            "pathlib.Path(ready).write_text('{}')\n"
            "time.sleep(0.6)\n"
            "raise SystemExit(int(os.environ.get('FAKE_SAMPLER_RC','0')))\n"
        )
        # Fake capture: record env, create the artifacts the wrapper expects.
        (root / "scripts" / "run-protocol-capture.sh").write_text(
            "#!/usr/bin/env python3\n"
            "import json,os,pathlib,sys\n"
            "log=pathlib.Path(os.environ['TEST_DIR'])/'capture-env.json'\n"
            "log.write_text(json.dumps({k:os.environ.get(k) for k in "
            "('SINK_PORT','GENERATOR_CPUSET','PROM_RW','GRAFANA_URL','BENCHCTL')}))\n"
            "run=pathlib.Path(sys.argv[3])/sys.argv[2]\n"
            "run.mkdir(parents=True,exist_ok=True)\n"
            "for name in ('resolved.json','k6-summary.json','manifest.json','records.ndjson'):\n"
            " (run/name).write_text('{}\\n')\n"
            "pathlib.Path(sys.argv[3],sys.argv[2]+'.sink.ndjson').write_text('')\n"
            "pathlib.Path(sys.argv[3],sys.argv[2]+'.capture-window.json').write_text('{}\\n')\n"
            "raise SystemExit(int(os.environ.get('FAKE_CAPTURE_RC','0')))\n"
        )
        # Fake analysis: record invocation and emit the expected outputs.
        (root / "scripts" / "post-run-analysis.sh").write_text(
            "#!/usr/bin/env bash\n"
            "printf '%s\\n' \"$*\" >> \"$TEST_DIR/post-run-args.log\"\n"
            "python3 - \"$2/$1.dispatch-analysis.json\" \"$2/$1.analysis-status.json\" <<'PY'\n"
            "import json,pathlib,sys\n"
            "pathlib.Path(sys.argv[1]).write_text(json.dumps({'measurement_valid':True})+chr(10))\n"
            "pathlib.Path(sys.argv[2]).write_text(json.dumps({'produced':True})+chr(10))\n"
            "PY\n"
        )
        for p in (root / "scripts" / "run-capture.sh", root / "scripts" / "benchctl-calibrated.sh",
                  root / "bin" / "benchctl", root / "bin" / "sink",
                  root / "scripts" / "generator-diagnostic-sampler.py",
                  root / "scripts" / "analyze-dispatch-run.py",
                  root / "scripts" / "run-protocol-capture.sh", root / "scripts" / "post-run-analysis.sh"):
            p.chmod(0o755)
        (root / "plan.json").write_text("{}\n")
        return root

    def run_wrapper(self, root, extra_env=None):
        env = dict(os.environ)
        env.pop("GRAFANA_URL", None)
        env.update({"TEST_DIR": str(root), "PYTHON": "python3"})
        env.update(extra_env or {})
        return subprocess.run(
            ["bash", "scripts/run-capture.sh", "plan.json", RID, "results", "thingsboard"],
            cwd=root, env=env, text=True, capture_output=True,
        )

    def test_forwarding_ordering_and_analysis(self):
        with tempfile.TemporaryDirectory() as d:
            root = self.tree(d)
            p = self.run_wrapper(root, {"GRAFANA_URL": "http://grafana.example/api",
                                        "PROM_RW": "http://prom.example/api/v1/write"})
            self.assertEqual(p.returncode, 0, p.stderr)
            env = json.loads((root / "capture-env.json").read_text())
            self.assertEqual(env["SINK_PORT"], "18091")
            self.assertEqual(env["GENERATOR_CPUSET"], "0-1")
            self.assertEqual(env["PROM_RW"], "http://prom.example/api/v1/write")
            self.assertEqual(env["GRAFANA_URL"], "http://grafana.example/api")
            self.assertEqual(env["BENCHCTL"], "scripts/benchctl-calibrated.sh")
            self.assertTrue((root / "results" / f"{RID}.dispatch-analysis.json").exists())
            self.assertTrue((root / "results" / f"{RID}.analysis-status.json").exists())
            self.assertEqual((root / "results" / f"{RID}.capture-exit-code").read_text().strip(), "0")

    def test_calibrated_benchctl_forwards_gogc_only_for_run(self):
        with tempfile.TemporaryDirectory() as d:
            root = self.tree(d)
            env = dict(os.environ, TEST_DIR=str(root))
            run = subprocess.run(["bash", "scripts/benchctl-calibrated.sh", "run", "-plan", "p"],
                                 cwd=root, env=env, text=True, capture_output=True)
            self.assertEqual(run.returncode, 0, run.stderr)
            other = subprocess.run(["bash", "scripts/benchctl-calibrated.sh", "resolve", "-plan", "p"],
                                   cwd=root, env=env, text=True, capture_output=True)
            self.assertEqual(other.returncode, 0, other.stderr)
            lines = (root / "benchctl-args.log").read_text().splitlines()
            self.assertIn("-env GOGC=400", lines[0])
            self.assertNotIn("GOGC", lines[1])

    def test_failed_capture_still_runs_analysis_and_propagates_exit(self):
        with tempfile.TemporaryDirectory() as d:
            root = self.tree(d)
            p = self.run_wrapper(root, {"FAKE_CAPTURE_RC": "7"})
            self.assertEqual(p.returncode, 7, p.stderr)
            self.assertTrue((root / "results" / f"{RID}.dispatch-analysis.json").exists())
            self.assertTrue((root / "post-run-args.log").exists())

    def test_failed_sampler_still_runs_analysis(self):
        with tempfile.TemporaryDirectory() as d:
            root = self.tree(d)
            p = self.run_wrapper(root, {"FAKE_SAMPLER_RC": "6"})
            self.assertEqual(p.returncode, 6, p.stderr)
            self.assertEqual((root / 'results' / f'{RID}.sampler-exit-code').read_text().strip(), '6')
            self.assertTrue((root / 'post-run-args.log').exists())

    def test_missing_window_skips_analysis(self):
        with tempfile.TemporaryDirectory() as d:
            root = self.tree(d)
            # Remove the artifact the fake capture creates for the window.
            fake = root / "scripts" / "run-protocol-capture.sh"
            text = fake.read_text().replace(
                "pathlib.Path(sys.argv[3],sys.argv[2]+'.capture-window.json').write_text('{}\\n')\n", "")
            fake.write_text(text)
            p = self.run_wrapper(root, {"FAKE_CAPTURE_RC": "3"})
            self.assertEqual(p.returncode, 3, p.stderr)
            self.assertFalse((root / "post-run-args.log").exists())


if __name__ == "__main__":
    unittest.main()
