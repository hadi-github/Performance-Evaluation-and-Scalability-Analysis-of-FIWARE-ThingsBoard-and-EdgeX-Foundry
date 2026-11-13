"""The EdgeX inventory gate must reserve the cohort the run actually creates.

The gate's invariant is `retained + cohort <= discovery_limit`. It is a
per-attempt check, so the reserve has to be the number of devices that attempt
will add. It was once a fixed 6,000 -- a whole-batch projection -- which
over-reserved 3x W2's cohort and 6x W3/W4's and blocked runs that fit under the
unchanged 16,384 limit.

These tests pin both halves of the fix: the launcher passes the plan's device
count, and the helper still falls back to the cohort when the variable is unset.
See research/findings/edgex-inventory-reserve-cohort-sized-20261112.md.
"""
import pathlib
import re
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
LAUNCHER = ROOT / 'results/feasible-campaign-20261106/run-feasible-campaign.py'
HELPER = ROOT / 'deploy/edgex/scripts/provision-then-scale.sh'

#: The frozen discovery limit at decision time. This test asserts the gate fits
#: under THIS value; raising it would need its own decision and receipt.
DISCOVERY_LIMIT = 16384
#: Live retained inventory when the decision was taken.
RETAINED = 11192


class InventoryReserveTests(unittest.TestCase):
    def test_launcher_reserves_the_plan_device_count(self):
        """A fixed 6,000 must not be reintroduced: it is a batch, not a run, number."""
        text = LAUNCHER.read_text()
        self.assertIn('EDGEX_INVENTORY_RESERVE={devices}', text)
        self.assertNotIn('EDGEX_INVENTORY_RESERVE=6000', text)

    def test_helper_still_falls_back_to_the_cohort(self):
        """The helper's own default must stay `$D` (the cohort), never a constant."""
        text = HELPER.read_text()
        m = re.search(r'reserve=\$\{EDGEX_INVENTORY_RESERVE:-([^}]+)\}', text)
        self.assertIsNotNone(m, 'reserve fallback disappeared from the gate')
        self.assertEqual(m.group(1), '$D')

    def test_gate_compares_retained_plus_reserve_against_both_limits(self):
        text = HELPER.read_text()
        gate = text[text.index('retained + reserve > metadata_limit'):]
        gate = gate[:gate.index('\n')]
        self.assertIn('metadata_limit', gate)
        self.assertIn('driver_limit', gate)

    def test_every_w_cohort_fits_under_the_unchanged_limit(self):
        """W2/W3/W4 must fit with the cohort-sized reserve, limit untouched.

        Inventory is retained across runs, so the retained total accumulates.
        """
        retained = RETAINED
        for name, cohort in (('W2', 2000), ('W3', 1000), ('W4', 1000)):
            total = retained + cohort
            self.assertLessEqual(
                total, DISCOVERY_LIMIT,
                f'{name}: {retained} retained + {cohort} cohort = {total} '
                f'exceeds {DISCOVERY_LIMIT}')
            retained = total

    def test_the_old_fixed_reserve_would_have_blocked_w2(self):
        """Guard the reason for the change: 11,192 + 6,000 did not fit."""
        self.assertGreater(RETAINED + 6000, DISCOVERY_LIMIT)

    def test_reserve_is_not_an_unlimited_unblock(self):
        """Retained inventory accumulates; a further run past the cap must fail."""
        after_w2_w3_w4 = RETAINED + 2000 + 1000 + 1000
        self.assertLessEqual(after_w2_w3_w4 + 1000, DISCOVERY_LIMIT)  # 5th fits
        self.assertGreater(after_w2_w3_w4 + 2000, DISCOVERY_LIMIT)    # 6th does not


class GatePathTests(unittest.TestCase):
    """The EdgeX gate must be written and read at the same path.

    A W run rebinds REMOTE_RESULTS to the W root, but activate() and
    post_run_check() once hardcoded B0's gates directory. The gate was written
    to B0's path and retrieved from W's, so a passing gate came back as
    `post_run_check_qualified: false`. Both must use the rebound variable.
    """

    def test_no_hardcoded_b0_gates_path(self):
        text = LAUNCHER.read_text()
        self.assertNotIn('feasible-ordinary-dispatch-20261106/gates', text,
                         'EdgeX gate path must follow REMOTE_RESULTS, not a B0 literal')

    def test_prepare_and_check_use_remote_results(self):
        text = LAUNCHER.read_text()
        start = text.index('bash {shlex.quote(helper)} prepare 2')
        prepare = text[start:start + 400]
        self.assertIn('{REMOTE_RESULTS}/gates/', prepare)
        start = text.index('bash {shlex.quote(helper)} check')
        check = text[start:start + 400]
        self.assertIn('{REMOTE_RESULTS}/gates/', check)


if __name__ == '__main__':
    unittest.main()
