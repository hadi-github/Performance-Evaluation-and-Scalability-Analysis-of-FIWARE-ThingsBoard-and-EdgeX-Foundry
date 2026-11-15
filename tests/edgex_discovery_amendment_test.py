"""The EdgeX discovery amendment must be opt-in and must leave the frozen path alone.

The amendment raises `MaxResultCount` from 16,384 to 50,000 and runs device-rest
from an image built with the profile-fetch dedup patch. Both depart from the
configuration B0 was reviewed against, so every one of these tests pins a safety
property that would otherwise erode silently:

  * the frozen override is byte-identical (its hash is bound by B0 reviews);
  * the amendment is a separate file, never a mutation of the frozen one;
  * the flag defaults OFF, and off means the frozen override is staged;
  * on means the amended override AND the amended image are both required --
    raising the cap without the dedup image unblocks nothing, because the
    readiness gate runs first;
  * the W4 skip list still enumerates all four blocked cells when the flag is
    off, so the historical reason stays legible.

See research/findings/edgex-profile-fetch-dedup-amendment-20261115.md.
"""
import hashlib
import importlib.util
import json
import os
import pathlib
import re
import subprocess
import sys
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / 'results/feasible-campaign-20261106/run-feasible-campaign.py'
W4_DRIVER = ROOT / 'scripts/run-w4-campaign.py'
FROZEN_OVERRIDE = ROOT / 'deploy/edgex/compose.feasible-discovery.yaml'
AMENDED_OVERRIDE = ROOT / 'deploy/edgex/compose.feasible-discovery-amended.yaml'
HELPER = ROOT / 'deploy/edgex/scripts/provision-then-scale.sh'
BUILD_SCRIPT = ROOT / 'deploy/edgex/scripts/build-patched-device-rest.sh'
PATCH_FILE = ROOT / 'deploy/edgex/amendment/device-sdk-dedup-profile-fetch.patch'
SDK_TEST = ROOT / 'deploy/edgex/amendment/sdk_dedup_profile_fetch_test.go'

#: Hash of the frozen override. B0 review bindings depend on these exact bytes.
FROZEN_SHA256 = '0893a6b861c800a264ae4612e4d3e45dc13deb2082ac42640125af8993b0aa1a'
FROZEN_CAP = '16384'
AMENDED_CAP = '50000'

#: What the W batch needs: 35,192 retained + 6,000 reserve.
W_BATCH_REQUIRED_CAP = 41192


def clean_env():
    env = dict(os.environ)
    env.pop('IOTBENCH_EDGEX_DISCOVERY_AMENDMENT', None)
    env['IOTBENCH_ENV_FILE'] = '/nonexistent'
    return env


def load_launcher(env):
    """Import the launcher under a given environment (module reads env at import)."""
    saved = dict(os.environ)
    os.environ.clear()
    os.environ.update(env)
    try:
        spec = importlib.util.spec_from_file_location('b0_under_test', LAUNCHER)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod
    finally:
        os.environ.clear()
        os.environ.update(saved)


class FrozenOverrideTests(unittest.TestCase):
    def test_frozen_override_is_byte_identical(self):
        """B0 reviews bind this hash; changing it would invalidate them."""
        actual = hashlib.sha256(FROZEN_OVERRIDE.read_bytes()).hexdigest()
        self.assertEqual(actual, FROZEN_SHA256)

    def test_frozen_override_still_caps_at_16384(self):
        self.assertIn(f'ALL_SERVICES_SERVICE_MAXRESULTCOUNT: "{FROZEN_CAP}"',
                      FROZEN_OVERRIDE.read_text())

    def test_amendment_is_a_separate_file(self):
        self.assertTrue(AMENDED_OVERRIDE.is_file())
        self.assertNotEqual(AMENDED_OVERRIDE, FROZEN_OVERRIDE)


class AmendedOverrideTests(unittest.TestCase):
    def test_amended_cap_clears_the_w_batch_requirement(self):
        text = AMENDED_OVERRIDE.read_text()
        m = re.search(r'ALL_SERVICES_SERVICE_MAXRESULTCOUNT: "(\d+)"', text)
        self.assertIsNotNone(m, 'amended override lost its cap')
        self.assertGreaterEqual(int(m.group(1)), W_BATCH_REQUIRED_CAP)

    def test_amended_override_raises_the_cap_only(self):
        """It must not touch replicas, cpusets, memory or ports."""
        text = AMENDED_OVERRIDE.read_text()
        for forbidden in ('replicas:', 'cpuset:', 'mem_limit:', 'memswap_limit:'):
            self.assertNotIn(forbidden, text,
                             f'amendment must not set {forbidden}; it is a cap+image change')

    def test_amended_override_pins_the_amended_image(self):
        self.assertIn('edgex-amended/device-rest:',
                      AMENDED_OVERRIDE.read_text())

    def test_amended_override_removes_the_build_stanza(self):
        """A surviving build stanza would let compose rebuild the stock image.

        That would silently discard the dedup patch while the cap stayed raised --
        the exact combination that looks amended and is not.
        """
        text = AMENDED_OVERRIDE.read_text()
        self.assertIn('build: !reset null', text)


class FlagTests(unittest.TestCase):
    def test_flag_defaults_off(self):
        mod = load_launcher(clean_env())
        self.assertFalse(mod.EDGEX_DISCOVERY_AMENDMENT)
        self.assertEqual(mod.edgex_discovery_override_source(), FROZEN_OVERRIDE)

    def test_flag_off_uses_stock_image(self):
        mod = load_launcher(clean_env())
        self.assertEqual(mod.EDGEX_STOCK_IMAGE, 'edgex-nats/device-rest:4.0.2')
        self.assertNotEqual(mod.EDGEX_AMENDMENT_IMAGE, mod.EDGEX_STOCK_IMAGE)

    def test_flag_on_selects_amended_override_and_image(self):
        env = clean_env()
        env['IOTBENCH_EDGEX_DISCOVERY_AMENDMENT'] = '1'
        mod = load_launcher(env)
        self.assertTrue(mod.EDGEX_DISCOVERY_AMENDMENT)
        self.assertEqual(mod.edgex_discovery_override_source(), AMENDED_OVERRIDE)
        self.assertEqual(mod.EDGEX_AMENDMENT_IMAGE, 'edgex-amended/device-rest:4.0.2-p1')

    def test_only_the_exact_value_1_enables_it(self):
        """A typo must not silently enable a frozen-configuration departure."""
        for value in ('true', 'yes', 'on', 'TRUE', '0', ''):
            env = clean_env()
            env['IOTBENCH_EDGEX_DISCOVERY_AMENDMENT'] = value
            mod = load_launcher(env)
            self.assertFalse(mod.EDGEX_DISCOVERY_AMENDMENT,
                             f'{value!r} must not enable the amendment')

    def test_launcher_has_a_single_override_resolver(self):
        """activate/verify_live must not disagree about which override is live."""
        text = LAUNCHER.read_text()
        self.assertNotIn("sha256_file(REPO/'deploy/edgex/compose.feasible-discovery.yaml')", text)
        self.assertGreaterEqual(text.count('edgex_discovery_override_source()'), 3)


class LauncherCapAssertionTests(unittest.TestCase):
    def test_rendered_cap_is_pinned_in_both_directions(self):
        text = LAUNCHER.read_text()
        self.assertIn("wanted_cap = \"50000\" if EDGEX_DISCOVERY_AMENDMENT else \"16384\"", text)
        self.assertIn('EdgeX discovery cap is', text)

    def test_live_render_applies_the_override(self):
        """Without the -f, the live check would validate stock bytes, not the run's.

        The rendered-config check lives in read_only_check(), not verify_live():
        verify_live only pings endpoints.
        """
        text = LAUNCHER.read_text()
        start = text.index('def read_only_check')
        body = text[start:text.index('\ndef ', start + 10)]
        self.assertIn('live_override', body)
        self.assertIn('edgex_discovery_override_source()', body)


class HelperEvidenceTests(unittest.TestCase):
    def test_gate_records_which_cap_admitted_the_run(self):
        text = HELPER.read_text()
        self.assertIn('maxresultcount-raised', text)
        self.assertIn('frozen-16384', text)

    def test_gate_records_retained_and_reserve(self):
        text = HELPER.read_text()
        start = text.index("discovery: {retained:")
        self.assertIn('reserve: $reserve', text[start:start + 300])


class BuildEvidenceTests(unittest.TestCase):
    def test_patch_targets_the_sdk_cache_init(self):
        text = PATCH_FILE.read_text()
        self.assertIn('--- a/internal/cache/init.go', text)
        self.assertIn('seenProfile', text)
        self.assertIn('DeviceProfileByName', text)

    def test_patch_deduplicates_rather_than_dropping_the_fetch(self):
        """A patch that removes the fetch entirely would break real multi-profile setups."""
        text = PATCH_FILE.read_text()
        added = [l for l in text.splitlines() if l.startswith('+\t')]
        self.assertTrue(any('continue' in l for l in added))
        self.assertIn('dpc.DeviceProfileByName', text)

    def test_build_uses_vendor_because_readonly_would_ignore_the_patch(self):
        """Pinned because the failure mode is invisible: -mod=readonly compiles fine.

        Only executable lines are checked; the header deliberately *mentions*
        -mod=readonly to explain the hazard.
        """
        dockerfile = (ROOT / 'deploy/edgex/amendment/Dockerfile.device-rest').read_text()
        code = [l for l in dockerfile.splitlines()
                if l.strip() and not l.lstrip().startswith('#')]
        code = '\n'.join(code)
        self.assertIn('-mod=vendor', code)
        self.assertNotIn('-mod=readonly', code)

    def test_build_runs_behavioural_gate_not_just_a_compile(self):
        dockerfile = (ROOT / 'deploy/edgex/amendment/Dockerfile.device-rest').read_text()
        self.assertIn('TestInitCacheDedupProfileFetch', dockerfile)
        self.assertIn('go test', dockerfile)

    def test_behavioural_test_asserts_one_fetch_for_shared_profile(self):
        text = SDK_TEST.read_text()
        self.assertIn('TestInitCacheDedupProfileFetch', text)
        self.assertIn('want 1', text)
        self.assertIn('InitCache', text)

    def test_build_script_refuses_to_double_patch(self):
        text = BUILD_SCRIPT.read_text()
        self.assertIn('already contains', text)

    def test_build_script_is_executable(self):
        self.assertTrue(os.access(BUILD_SCRIPT, os.X_OK))


class W4DriverTests(unittest.TestCase):
    def plan_with(self, amendment):
        env = clean_env()
        if amendment:
            env['IOTBENCH_EDGEX_DISCOVERY_AMENDMENT'] = '1'
        out = subprocess.run([sys.executable, str(W4_DRIVER), '--plan-only'],
                             cwd=ROOT, env=env, capture_output=True, text=True,
                             timeout=120, check=True)
        return json.loads(out.stdout)

    def test_flag_off_keeps_the_four_edgex_cells_blocked(self):
        d = self.plan_with(amendment=False)
        self.assertEqual(d['counts']['runnable'], 11)
        self.assertEqual(d['counts']['skipped'], 4)
        self.assertEqual(
            sorted(s['run_id'] for s in d['skipped']),
            ['w4dp-ex-r02-20261108', 'w4dp-ex-r03-20261108',
             'w4dp-ex-r04-20261108', 'w4dp-ex-r05-20261108'])

    def test_flag_on_unblocks_all_fifteen(self):
        d = self.plan_with(amendment=True)
        self.assertEqual(d['counts']['runnable'], 15)
        self.assertEqual(d['counts']['skipped'], 0)

    def test_blocked_reasons_are_still_recorded_as_data(self):
        """The skip list is history; deleting it would erase why the cells waited."""
        text = W4_DRIVER.read_text()
        self.assertIn('INVENTORY_BLOCKED', text)
        self.assertIn('16,384', text)


if __name__ == '__main__':
    unittest.main()
