#!/usr/bin/env python3
"""Tests for the rate-derived generator VU-pool policy (owner amendment 2026-11-10).

The pool is generator concurrency, not a platform capacity claim. These tests
pin the two properties that make the amendment safe to apply mid-project:

  1. At 25/s the rule must yield exactly the value B0 already used (150), so no
     historical cell or verdict is reinterpreted.
  2. At 50/s and above the rule must yield a pool at least `rate x timeout`, so
     the open-loop executor cannot starve itself and report generator drops as
     platform loss.
"""

import importlib.util
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'scripts' / name)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


pool = load('pool_policy.py')
integrity = load('dispatch-integrity.py')

W_ROOT = ROOT / 'workloads' / 'campaigns' / 'fixed-points-w1-w4'


class PoolArithmeticTests(unittest.TestCase):
    def test_matches_frozen_table_for_every_qualified_rate(self):
        """The formula and the frozen table must not drift apart."""
        for rate, expected in pool.FROZEN_POOLS.items():
            self.assertEqual(pool.required_pool(rate), expected, f'{rate}/s')

    def test_b0_rate_is_unchanged(self):
        """25/s must reproduce B0's 150 exactly, or the amendment is not safe."""
        self.assertEqual(pool.FROZEN_POOLS[pool.B0_RATE_PER_S], 150)
        self.assertEqual(pool.pool_for_rate(25), 150)
        self.assertEqual(pool.required_pool(25), 150)

    def test_every_pool_covers_rate_times_timeout(self):
        """A pool below `rate x timeout` starves the executor by construction."""
        for rate in pool.QUALIFIED_RATES:
            need = pool.bound_in_flight(rate, pool.REQUEST_TIMEOUT_MS)
            self.assertGreaterEqual(pool.pool_for_rate(rate), need,
                                    f'{rate}/s: pool does not cover its own bound')
            self.assertGreaterEqual(pool.pool_for_rate(rate), pool.required_pool(rate))

    def test_headroom_exceeds_observed_b0_jitter(self):
        """B0 peaked at 126 VUs against a theoretical 125, so >1.0 headroom is required."""
        self.assertGreater(pool.HEADROOM, 1.0)
        observed_peak, theoretical = 126, 125
        self.assertGreaterEqual(pool.HEADROOM, observed_peak / theoretical)

    def test_50s_pool_is_300(self):
        self.assertEqual(pool.pool_for_rate(50), 300)

    def test_unknown_rate_fails_closed(self):
        for rate in (0, 10, 40, 75):
            with self.assertRaises(ValueError):
                pool.pool_for_rate(rate, pool.POLICY_V2)

    def test_v1_keeps_the_historical_fixed_pool(self):
        """v1 must answer 150 for whatever it is asked, to stay byte-historical."""
        for rate in pool.QUALIFIED_RATES:
            self.assertEqual(pool.pool_for_rate(rate, pool.POLICY_V1), 150)

    def test_policy_selection(self):
        self.assertEqual(pool.policy_for_rate(25), pool.POLICY_V1)
        self.assertEqual(pool.policy_for_rate(50), pool.POLICY_V2)
        self.assertEqual(pool.policy_for_rate(100), pool.POLICY_V2)


class IntegrityPoolEnforcementTests(unittest.TestCase):
    """The evaluator must accept only the frozen pool for the plan's own rate."""

    @staticmethod
    def plan(rate, vu, policy):
        return {'health': {'workload_policy': policy},
                'vus': {'pre_allocated': vu, 'max': vu},
                'steps': [{'name': 'measurement', 'tick_rate': rate, 'vus': vu}]}

    def assertAccepts(self, rate, vu, policy):
        integrity._check_pool(self.plan(rate, vu, policy), policy)

    def assertRejects(self, rate, vu, policy):
        with self.assertRaises(ValueError):
            integrity._check_pool(self.plan(rate, vu, policy), policy)

    def test_accepts_each_frozen_pair(self):
        for rate in pool.QUALIFIED_RATES:
            self.assertAccepts(rate, pool.pool_for_rate(rate), pool.POLICY_V2)

    def test_rejects_the_starved_w1_shape(self):
        """The exact configuration that produced both preserved negatives."""
        self.assertRejects(50, 150, pool.POLICY_V2)

    def test_rejects_pool_from_a_different_rate(self):
        self.assertRejects(25, 300, pool.POLICY_V2)
        self.assertRejects(50, 550, pool.POLICY_V2)

    def test_rejects_pool_below_the_headroom_rule(self):
        self.assertRejects(50, 250, pool.POLICY_V2)

    def test_rejects_unqualified_rate(self):
        self.assertRejects(40, 300, pool.POLICY_V2)

    def test_rejects_per_step_pool_override(self):
        plan = self.plan(50, 300, pool.POLICY_V2)
        plan['steps'][0]['vus'] = 150
        with self.assertRaises(ValueError):
            integrity._check_pool(plan, pool.POLICY_V2)

    def test_v1_still_accepts_historical_b0_shape(self):
        self.assertAccepts(25, 150, pool.POLICY_V1)


class StagedPlanPoolTests(unittest.TestCase):
    """Every staged control file must declare the pool its rate requires."""

    def test_all_proposed_plans_use_the_rate_derived_pool(self):
        matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
        scope = json.loads((W_ROOT / 'scope.json').read_text())
        self.assertEqual(len(matrix['experimental_runs']), 60)
        for row in matrix['experimental_runs']:
            plan = json.loads((W_ROOT / row['plan']).read_text())
            rate = scope['variants'][row['workload_id']]['total_messages_per_second']
            want = pool.pool_for_rate(rate)
            self.assertEqual(plan['vus'], {'pre_allocated': want, 'max': want}, row['run_id'])
            for step in plan['schedule']['steps']:
                self.assertEqual(step['vus'], want, row['run_id'])
                self.assertEqual(step['rate'], rate, row['run_id'])
            self.assertEqual(plan['health']['workload_policy'], pool.policy_for_rate(rate),
                             row['run_id'])

    def test_only_w1_plans_use_the_larger_pool(self):
        """The amendment must not move any 25/s cell off the B0-identical pool."""
        matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
        big = [r['run_id'] for r in matrix['experimental_runs']
               if json.loads((W_ROOT / r['plan']).read_text())['vus']['max'] != 150]
        self.assertEqual(len(big), 15)
        self.assertTrue(all(r.startswith('w1dp-') for r in big), big)

    def test_live_qualification_plans_use_the_rate_derived_pool(self):
        index = json.loads((W_ROOT / 'qualification' / 'index.json').read_text())
        for row in index['runs']:
            if row.get('execution_status') != 'NOT_LAUNCH_CLEARED':
                continue
            plan = json.loads((W_ROOT / 'qualification' / row['plan']).read_text())
            self.assertEqual(plan['vus']['max'], pool.pool_for_rate(50), row['run_id'])
            self.assertEqual(plan['health']['workload_policy'], pool.POLICY_V2, row['run_id'])

    def test_scope_records_the_amendment_and_its_comparability(self):
        scope = json.loads((W_ROOT / 'scope.json').read_text())
        shared = scope['shared']
        self.assertIn('generator_pool_policy', shared)
        self.assertEqual(shared['generator_pool_headroom_factor'], pool.HEADROOM)
        self.assertEqual(shared['generator_pool_by_rate'],
                         {str(r): p for r, p in sorted(pool.FROZEN_POOLS.items())})
        matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
        rev = matrix['pool_policy_revision']
        self.assertEqual(rev['revision'], '2026-11-10-generator-pool-v2')
        self.assertIn('none', rev['b0_impact'].lower())

    def test_preserved_v1_generations_keep_their_original_pool(self):
        """Consumed evidence must never be rewritten to the new pool."""
        index = json.loads((W_ROOT / 'qualification' / 'index.json').read_text())
        preserved = [r for r in index['runs'] if r.get('superseded_by')]
        self.assertTrue(preserved)
        for row in preserved:
            if not (W_ROOT / 'qualification' / row['plan']).is_file():
                continue
            plan = json.loads((W_ROOT / 'qualification' / row['plan']).read_text())
            if row.get('execution_status') == 'INVALID_VU_POOL_BOUND':
                self.assertEqual(plan['vus']['max'], 150, row['run_id'])


if __name__ == '__main__':
    unittest.main(verbosity=2)
