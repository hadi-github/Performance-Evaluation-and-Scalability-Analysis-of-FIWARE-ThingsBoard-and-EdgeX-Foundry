"""Offline tests for the .env configuration layer and plan rebasing.

The point of these two scripts is that moving the testbed to new addresses must
not change what is measured. These tests pin exactly that.
"""
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'scripts'))

import iotbench_config as cfg  # noqa: E402


def load_script(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


rebase = load_script('rebase_plan', ROOT / 'scripts' / 'rebase-plan.py')
overlay = load_script('iotbench_plan_overlay', ROOT / 'scripts' / 'iotbench_plan_overlay.py')


def clean_env(**overrides):
    """The real environment with every IOTBENCH_* variable removed.

    The developer's shell may legitimately have `.env` exported (that is how the
    deployment is driven by hand). Tests must not inherit it, or they would
    assert against whatever machine happens to be configured.
    """
    env = {k: v for k, v in os.environ.items() if not k.startswith('IOTBENCH_')}
    env.update(overrides)
    return env


def config_value(expr, **env):
    """Evaluate a snippet against a freshly imported `iotbench_config`."""
    out = subprocess.run([sys.executable, '-c',
                          'import sys, json; sys.path.insert(0, "scripts");'
                          'import iotbench_config as c; print(json.dumps(' + expr + '))'],
                         cwd=ROOT, capture_output=True, text=True, env=clean_env(**env))
    if out.returncode != 0:
        raise AssertionError(out.stderr)
    return json.loads(out.stdout)


class ConfigDefaultsTest(unittest.TestCase):
    """With no .env, every default must reproduce the qualified testbed."""

    def setUp(self):
        # Force the no-file path even on a machine that has a real .env.
        self.no_file = {'IOTBENCH_ENV_FILE': '/nonexistent'}

    def test_qualified_defaults(self):
        got = config_value('{"sut": c.SUT, "load": c.LOAD, "sut_ip": c.SUT_IP,'
                           ' "load_ip": c.LOAD_IP, "sink_port": c.SINK_PORT}',
                           **self.no_file)
        self.assertEqual(got, {'sut': 'ramonavm2', 'load': 'ramonavm1',
                               'sut_ip': '87.107.191.107',
                               'load_ip': '87.107.191.32',
                               'sink_port': '18091'})

    def test_remote_paths_are_the_historical_ones(self):
        # These embed a date but are load-bearing: live state was recorded there.
        got = config_value('{"results": c.REMOTE_RESULTS, "tb": c.TB_COMPOSE,'
                           ' "edgex": c.EDGEX_DIR, "fw": c.FW_DIR}', **self.no_file)
        self.assertEqual(got['results'],
                         '/root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106')
        self.assertEqual(got['tb'],
                         '/root/iot-qual-tb-20261105/deploy/thingsboard/compose.yaml')
        self.assertEqual(got['fw'], '/root/iot-bench-deploy-cand-auto20261027')
        self.assertEqual(got['edgex'], '/root/iot-bench-deploy-cand-auto20261027/edgex')

    def test_import_does_not_pollute_the_environment(self):
        # A child process must not inherit .env values it did not ask for.
        with tempfile.TemporaryDirectory() as d:
            env = pathlib.Path(d) / '.env'
            env.write_text('IOTBENCH_SUT_IP=10.9.9.9\n')
            out = subprocess.run([sys.executable, '-c',
                                  'import sys, os; sys.path.insert(0, "scripts");'
                                  'import iotbench_config;'
                                  'print(os.environ.get("IOTBENCH_SUT_IP", "<unset>"))'],
                                 cwd=ROOT, capture_output=True, text=True,
                                 env=clean_env(IOTBENCH_ENV_FILE=str(env)))
            self.assertEqual(out.stdout.strip(), '<unset>', out.stderr)

    def test_env_file_does_not_override_real_environment(self):
        # A one-off `IOTBENCH_SUT_IP=x ./cmd` must win over the file.
        with tempfile.TemporaryDirectory() as d:
            env = pathlib.Path(d) / '.env'
            env.write_text('IOTBENCH_SUT_IP=10.9.9.9\n')
            got = config_value('c.SUT_IP', IOTBENCH_ENV_FILE=str(env),
                               IOTBENCH_SUT_IP='10.1.1.1')
            self.assertEqual(got, '10.1.1.1')

    def test_env_file_is_read_when_variable_absent(self):
        with tempfile.TemporaryDirectory() as d:
            env = pathlib.Path(d) / '.env'
            env.write_text('IOTBENCH_SUT_IP=10.9.9.9\n')
            got = config_value('c.SUT_IP', IOTBENCH_ENV_FILE=str(env))
            self.assertEqual(got, '10.9.9.9')

    def test_target_for_platform_keeps_qualified_ports(self):
        self.assertEqual(cfg.target_for('thingsboard', '10.0.0.5'),
                         {'base_url': 'http://10.0.0.5:8080'})
        self.assertEqual(cfg.target_for('fiware', '10.0.0.5'),
                         {'base_url': 'http://10.0.0.5:1026',
                          'north_url': 'http://10.0.0.5:4041',
                          'south_url': 'http://10.0.0.5:7896'})
        self.assertEqual(cfg.target_for('edgex', '10.0.0.5'),
                         {'base_url': 'http://10.0.0.5:59986',
                          'metadata_url': 'http://10.0.0.5:59881',
                          'ingress_url': 'http://10.0.0.5:59986'})


class RebasePlanTest(unittest.TestCase):
    """Rebasing may change `target` and nothing else."""

    PLAN = ROOT / 'workloads/campaigns/fixed-points-w1-w4/plans/w4dp-fw-r01-20261108.json'

    def setUp(self):
        self.plan = json.loads(self.PLAN.read_text())

    def test_only_target_changes(self):
        out = overlay.rewrite_target(self.plan, '10.20.30.40')
        diff = overlay.differing_fields(self.plan, out)
        self.assertTrue(diff)
        self.assertTrue(all(d.startswith('target') for d in diff), diff)

    def test_ports_are_preserved(self):
        out = overlay.rewrite_target(self.plan, '10.20.30.40')
        self.assertEqual(out['target']['base_url'], 'http://10.20.30.40:1026')
        self.assertEqual(out['target']['north_url'], 'http://10.20.30.40:4041')
        self.assertEqual(out['target']['south_url'], 'http://10.20.30.40:7896')

    def test_workload_fields_untouched(self):
        out = overlay.rewrite_target(self.plan, '10.20.30.40')
        for field in ('devices', 'seed', 'adapter', 'payload', 'schedule',
                      'request_timeout_ms', 'connection', 'health'):
            if field in self.plan:
                self.assertEqual(out[field], self.plan[field], field)

    def test_original_is_not_mutated(self):
        before = json.dumps(self.plan, sort_keys=True)
        overlay.rewrite_target(self.plan, '10.20.30.40')
        self.assertEqual(json.dumps(self.plan, sort_keys=True), before)

    def test_tampered_plan_is_detected(self):
        good = overlay.rewrite_target(self.plan, '10.20.30.40')
        tampered = json.loads(json.dumps(good))
        tampered['devices'] = 2000
        diff = overlay.differing_fields(self.plan, tampered)
        outside = [d for d in diff if not d.startswith('target')]
        self.assertIn('devices', outside)

    def test_cli_verify_rejects_tampering(self):
        with tempfile.TemporaryDirectory() as d:
            out = pathlib.Path(d) / 'p.json'
            good = overlay.rewrite_target(self.plan, '10.20.30.40')
            good['devices'] = 2000
            out.write_text(json.dumps(good))
            rc = subprocess.run([sys.executable, str(ROOT / 'scripts' / 'rebase-plan.py'),
                                 str(self.PLAN), str(out), '--verify'],
                                capture_output=True, text=True).returncode
            self.assertEqual(rc, 1)

    def test_cli_verify_accepts_a_clean_rebase(self):
        with tempfile.TemporaryDirectory() as d:
            out = pathlib.Path(d) / 'p.json'
            out.write_text(json.dumps(overlay.rewrite_target(self.plan, '10.20.30.40')))
            p = subprocess.run([sys.executable, str(ROOT / 'scripts' / 'rebase-plan.py'),
                                str(self.PLAN), str(out), '--verify'],
                               capture_output=True, text=True,
                               env=dict(os.environ, IOT_BENCH_PREBUILT_DIR='/nonexistent'))
            self.assertEqual(p.returncode, 0, p.stderr)


class ConfigureScriptTest(unittest.TestCase):
    SCRIPT = ROOT / 'scripts' / 'configure-from-env.sh'

    def run_cfg(self, env_text, *args):
        d = tempfile.mkdtemp()
        env = pathlib.Path(d) / '.env'
        env.write_text(env_text)
        return subprocess.run(['bash', str(self.SCRIPT), *args], cwd=ROOT,
                              capture_output=True, text=True,
                              env=clean_env(IOTBENCH_ENV_FILE=str(env)))

    def test_rejects_bad_port(self):
        p = self.run_cfg('IOTBENCH_SSH_PORT_LOAD=notaport\n', '--check')
        self.assertNotEqual(p.returncode, 0)
        self.assertIn('invalid port', p.stderr)

    def test_rejects_missing_key(self):
        p = self.run_cfg('IOTBENCH_SSH_KEY=/nonexistent/key\n', '--check')
        self.assertNotEqual(p.returncode, 0)
        self.assertIn('not a file', p.stderr)

    def test_accepts_600_key(self):
        with tempfile.NamedTemporaryFile() as k:
            os.chmod(k.name, 0o600)
            p = self.run_cfg(f'IOTBENCH_SSH_KEY={k.name}\n', '--check')
            self.assertEqual(p.returncode, 0, p.stderr)

    def test_rejects_group_readable_key(self):
        with tempfile.NamedTemporaryFile() as k:
            os.chmod(k.name, 0o644)
            p = self.run_cfg(f'IOTBENCH_SSH_KEY={k.name}\n', '--check')
            self.assertNotEqual(p.returncode, 0)
            self.assertIn('too open', p.stderr)

    def test_check_writes_nothing(self):
        with tempfile.TemporaryDirectory() as d:
            out = pathlib.Path(d) / 'cfg'
            p = self.run_cfg(f'IOTBENCH_SSH_CONFIG={out}\n', '--check')
            self.assertEqual(p.returncode, 0, p.stderr)
            self.assertFalse(out.exists())


if __name__ == '__main__':
    unittest.main()
