"""Offline tests for the W4 campaign driver's admission and acceptance rules.

The driver decides two things that must never be guesses: which W4 cells the
frozen controls admit, and which finished results count as an accepted attempt.
Both are tested here without touching a host.
"""
import importlib.util
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('run_w4_campaign', ROOT / 'scripts' / 'run-w4-campaign.py')
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)


def good_result(**overrides):
    """A completed W4 attempt that satisfies every mandatory gate."""
    result = {
        'run_id': 'w4dp-tb-r01-20261108',
        'platform': 'thingsboard',
        'run_kind': 'ordinary',
        'workload_policy': 'dispatch-integrity-v1',
        'measurement_valid': True,
        'capture_valid': True,
        'latency_assessed': True,
        'generator_resource_qualified': True,
        'sut_resource_qualified': True,
        'cleanup_verified': True,
        'post_run_check_qualified': True,
        'history_retrieved': True,
        'reanalysis_with_history': True,
        'analysis_retrieved': True,
        'platform_result': 'VALID_PLATFORM_SUCCESS',
        'history_result': 'VALID_HISTORY_SUCCESS',
        'workload_lock_retained': False,
        'post_run_defect': None,
        'fetch_errors': [],
        'finish': 'stopped owned services; volumes preserved',
    }
    result.update(overrides)
    return result


class AdmissionTests(unittest.TestCase):
    def test_every_matrix_row_is_classified_exactly_once(self):
        rows = mod.w4_rows()
        self.assertEqual(len(rows), 15, 'W4 is five repetitions on three platforms')
        runnable, skipped = mod.plan(rows)
        ids = [r['run_id'] for r in runnable] + [s['run_id'] for s in skipped]
        self.assertEqual(sorted(ids), sorted(r['run_id'] for r in rows),
                         'no row may be silently dropped or duplicated')
        self.assertEqual(len(ids), len(set(ids)))

    def test_skips_carry_a_reason_and_are_only_inventory_cells(self):
        _, skipped = mod.plan(mod.w4_rows())
        for entry in skipped:
            self.assertTrue(entry['reason'].strip(), f'{entry["run_id"]} skipped without a reason')
            self.assertEqual(entry['platform'], 'edgex',
                             'only the EdgeX inventory cap may block a W4 cell')
        self.assertEqual(len(skipped), 4)

    def test_edgex_cells_are_admitted_in_order(self):
        """The first EdgeX repetition is admitted; the rest are not.

        A cohort is permanently retained, so admitting rep 1 must not also admit
        rep 2: the gate is cumulative, not per-run.
        """
        rows = [r for r in mod.w4_rows() if r['platform'] == 'edgex']
        runnable, skipped = mod.plan(rows)
        self.assertEqual([r['run_id'] for r in runnable], ['w4dp-ex-r01-20261108'])
        self.assertEqual(len(skipped), 4)

    def test_thingsboard_and_fiware_are_never_blocked(self):
        rows = [r for r in mod.w4_rows() if r['platform'] in ('thingsboard', 'fiware')]
        runnable, skipped = mod.plan(rows)
        self.assertEqual(len(runnable), 10)
        self.assertEqual(skipped, [])

    def test_plan_only_is_ordered_by_ordinal(self):
        rows = mod.w4_rows()
        ordinals = [r['ordinal'] for r in rows]
        self.assertEqual(ordinals, sorted(ordinals), 'cells must run in frozen matrix order')


class AcceptanceTests(unittest.TestCase):
    def test_clean_success_is_accepted(self):
        self.assertIsNone(mod.workload_policy_ok(good_result(), 'w4dp-tb-r01-20261108'))

    def test_platform_backlog_is_a_result_not_a_defect(self):
        """A measured platform failure is the finding; it must not stop the batch."""
        self.assertIsNone(mod.workload_policy_ok(
            good_result(platform_result='VALID_PLATFORM_FAILURE_OR_BACKLOG'),
            'w4dp-tb-r01-20261108'))

    def test_unassessed_result_is_rejected(self):
        problem = mod.workload_policy_ok(good_result(platform_result='NOT_ASSESSED_NO_ANALYSIS'),
                                         'w4dp-tb-r01-20261108')
        self.assertIn('NOT_ASSESSED_NO_ANALYSIS', problem)

    def test_history_failure_is_rejected(self):
        problem = mod.workload_policy_ok(good_result(history_result='VALID_HISTORY_FAILURE'),
                                         'w4dp-tb-r01-20261108')
        self.assertIn('history_result', problem)

    def test_each_mandatory_gate_is_enforced(self):
        for gate in ('measurement_valid', 'capture_valid', 'latency_assessed',
                     'generator_resource_qualified', 'sut_resource_qualified',
                     'cleanup_verified', 'post_run_check_qualified', 'history_retrieved',
                     'reanalysis_with_history', 'analysis_retrieved'):
            with self.subTest(gate=gate):
                problem = mod.workload_policy_ok(good_result(**{gate: False}),
                                                 'w4dp-tb-r01-20261108')
                self.assertIn(gate, problem)

    def test_identity_mismatch_is_rejected(self):
        problem = mod.workload_policy_ok(good_result(), 'w4dp-tb-r02-20261108')
        self.assertIn('identity', problem)

    def test_retained_lock_and_fetch_errors_are_rejected(self):
        self.assertIn('lock', mod.workload_policy_ok(good_result(workload_lock_retained=True),
                                                     'w4dp-tb-r01-20261108'))
        self.assertIn('fetch_errors', mod.workload_policy_ok(good_result(fetch_errors=['x']),
                                                             'w4dp-tb-r01-20261108'))

    def test_unexpected_finish_is_rejected(self):
        problem = mod.workload_policy_ok(good_result(finish='left services running'),
                                         'w4dp-tb-r01-20261108')
        self.assertIn('finish', problem)


if __name__ == '__main__':
    unittest.main()
