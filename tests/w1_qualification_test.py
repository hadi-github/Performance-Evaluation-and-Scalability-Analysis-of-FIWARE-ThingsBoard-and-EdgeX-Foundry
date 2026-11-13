"""W1 qualification staging and shape-binding tests.

The W1 release gate requires a full-duration 50/s 300/600 s qualification. A
qualification that silently changed shape would "clear" W1 while proving nothing,
so these tests assert both that the staged plans match the gate and that the
validator rejects every load-bearing deviation.
"""
import importlib.util
import json
import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
W_ROOT = ROOT / 'workloads' / 'campaigns' / 'fixed-points-w1-w4'


def load(script):
    spec = importlib.util.spec_from_file_location(script.stem.replace('-', '_'), script)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


stage = load(ROOT / 'scripts' / 'stage-w1-qualification.py')


def resolved_from(plan_path, run_id):
    """Minimal resolved-plan shape mirroring benchctl resolve for these plans."""
    p = json.loads(plan_path.read_text())
    steps = []
    offset = 0
    for i, s in enumerate(p['schedule']['steps']):
        steps.append({'index': i, 'name': f's{i:02d}-{s["phase"]}-r{s["rate"]}', 'phase': s['phase'],
                      'rate': s['rate'], 'tick_rate': s['rate'], 'duration_s': s['duration_s'],
                      'start_s': offset, 'ticks': s['rate'] * s['duration_s'],
                      'seq_offset': None, 'vus': s['vus']})
        offset += s['duration_s']
    return dict(p, run_id=run_id, steps=steps,
                total_ticks=sum(s['ticks'] for s in steps), total_duration_s=offset)


class QualificationStagingTests(unittest.TestCase):
    def test_index_and_plans_exist_and_match_generator(self):
        """The generator reproduces every *live* plan byte-for-byte.

        Preserved generations are historical evidence and keep the bytes they
        actually ran at, so they are checked for existence only.
        """
        files, index = stage.artifacts()
        self.assertEqual(len(index['runs']), 3)
        self.assertFalse(index['execution_authorized'])
        self.assertEqual(index['status'], 'NOT_LAUNCH_CLEARED')
        on_disk = json.loads((W_ROOT / 'qualification' / 'index.json').read_text())
        live_plans = {r['plan'] for r in on_disk['runs']
                      if r.get('execution_status') == 'NOT_LAUNCH_CLEARED'}
        for name, body in files.items():
            staged = W_ROOT / 'qualification' / name
            self.assertTrue(staged.is_file(), name)
            if name in live_plans:
                self.assertEqual(staged.read_bytes(), stage.encoded(body), name)

    def test_each_plan_is_the_full_w1_duration(self):
        files, index = stage.artifacts()
        for row in index['runs']:
            plan = files[row['plan']]
            steps = plan['schedule']['steps']
            self.assertEqual([s['duration_s'] for s in steps], [300, 600])
            self.assertEqual([s['rate'] for s in steps], [50, 50])
            self.assertEqual(sum(s['rate'] * s['duration_s'] for s in steps), 45000)
            self.assertEqual(row['expected_warmup_messages'], 15000)
            self.assertEqual(row['expected_measurement_messages'], 30000)
            self.assertFalse(row['counts_toward_proposed_attempts'])

    def test_qualification_ids_never_collide_with_proposed_attempts(self):
        matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
        proposed = {r['run_id'] for r in matrix['experimental_runs']}
        _, index = stage.artifacts()
        quals = {r['run_id'] for r in index['runs']}
        self.assertFalse(proposed & quals)
        for rid in quals:
            self.assertTrue(rid.startswith('w1-qual-'), rid)

    def test_qualification_seed_is_outside_the_repetition_seed_set(self):
        scope = json.loads((W_ROOT / 'scope.json').read_text())
        self.assertNotIn(stage.QUAL_SEED, scope['seed_by_repetition'])

    def test_qualification_matches_the_canonical_attempt_shape(self):
        """Only identity may differ from the real W1 attempt it is qualifying.

        Holds for *live* rows. A preserved generation is historical evidence: it
        keeps the shape it actually ran at, which may predate the pool amendment.
        """
        scope = json.loads((W_ROOT / 'scope.json').read_text())
        index = json.loads((W_ROOT / 'qualification' / 'index.json').read_text())
        live = [r for r in index['runs'] if r.get('execution_status') == 'NOT_LAUNCH_CLEARED']
        # Zero live rows is a legitimate terminal state: once every platform in the
        # generation has been measured (VALID or INVALID) there is nothing left to
        # launch. This precondition was never an invariant, so its absence must not
        # be reported as a shape regression.
        if not live:
            self.skipTest('no launchable qualification row remains: every platform is measured')
        for row in live:
            qual = json.loads((W_ROOT / 'qualification' / row['plan']).read_text())
            canon = json.loads((W_ROOT / 'plans' /
                                f'w1dp-{stage.PREFIX[row["platform"]]}-r01-20261108.json').read_text())
            for field in ('adapter', 'devices', 'request_timeout_ms', 'vus', 'connection',
                          'arrivals', 'target', 'payload', 'health', 'test_faults', 'cleanup'):
                self.assertEqual(qual[field], canon[field], f'{row["run_id"]}: {field}')
            self.assertEqual([(s['rate'], s['duration_s'], s['vus']) for s in qual['schedule']['steps']],
                             [(s['rate'], s['duration_s'], s['vus']) for s in canon['schedule']['steps']])
            self.assertNotEqual(qual['seed'], canon['seed'])
            self.assertEqual(qual['protocol_pilot_parameters']['drain_seconds_after_measurement'],
                             scope['shared']['maximum_receipt_drain_seconds'])


class QualificationShapeValidationTests(unittest.TestCase):
    def setUp(self):
        self.launcher = load(ROOT / 'scripts' / 'run-workload-campaign.py')
        self.scope = self.launcher.load_scope()
        self.row = self.launcher.qualification_rows()[0]
        self.plan = self.launcher.W_QUAL_ROOT / self.row['plan']
        self.base = resolved_from(self.plan, self.row['run_id'])

    def test_baseline_is_accepted(self):
        out = self.launcher.validate_qualification_shape(self.row, self.base, self.scope)
        self.assertEqual(out['kind'], 'qualification')
        self.assertFalse(out['counts_toward_proposed_attempts'])

    def _rejects(self, mutate, needle):
        import copy
        bad = copy.deepcopy(self.base)
        mutate(bad)
        with self.assertRaises(SystemExit) as ctx:
            self.launcher.validate_qualification_shape(self.row, bad, self.scope)
        self.assertIn(needle, str(ctx.exception))

    def test_rejects_short_measurement(self):
        self._rejects(lambda d: d['steps'][1].update(duration_s=120), 'rate_and_steps')

    def test_rejects_wrong_rate(self):
        self._rejects(lambda d: [s.update(rate=25) for s in d['steps']], 'rate_and_steps')

    def test_rejects_undersized_vu_pool(self):
        # The pool is rate-derived; a pool smaller than the frozen value is a fail.
        self._rejects(lambda d: d['vus'].update(pre_allocated=150, max=150), 'vus')

    def test_rejects_oversized_vu_pool(self):
        self._rejects(lambda d: d['vus'].update(max=400, pre_allocated=400), 'vus')

    def test_rejects_wrong_target(self):
        self._rejects(lambda d: d['target'].update(base_url='http://elsewhere:1'), 'target')

    def test_rejects_injected_fault(self):
        self._rejects(lambda d: d.update(test_faults={'inject': 1}), 'no_faults')

    def test_rejects_wrong_seed(self):
        self._rejects(lambda d: d.update(seed=1), 'seed')

    def test_rejects_padding_drift(self):
        self._rejects(lambda d: d['payload'].update(size_offset_bytes=1024), 'padding')

    def test_rejects_lost_identities(self):
        self._rejects(lambda d: d['steps'][1].update(ticks=29999), 'message_counts')


class W2QualificationStagingTests(unittest.TestCase):
    """W2's gate is provisioning: 2,000 devices recognized by every replica.

    The plan must reproduce the W2 attempt exactly, or a pass would not mean the
    real attempt would work. It must also stay pool-identical to B0, because at
    25/s the rate-derived rule returns exactly 150 VUs.
    """

    def setUp(self):
        self.files, self.index = stage.artifacts(workload='W2')

    def test_three_live_plans_staged_under_their_own_root(self):
        self.assertEqual(len(self.index['runs']), 3)
        self.assertEqual(self.index['workload_id'], 'W2')
        self.assertFalse(self.index['execution_authorized'])
        self.assertEqual(self.index['status'], 'NOT_LAUNCH_CLEARED')
        on_disk = json.loads((W_ROOT / 'qualification' / 'w2' / 'index.json').read_text())
        self.assertEqual(len(on_disk['runs']), 3)
        for name, body in self.files.items():
            staged = W_ROOT / 'qualification' / 'w2' / name
            self.assertTrue(staged.is_file(), name)
            self.assertEqual(staged.read_bytes(), stage.encoded(body), name)

    def test_devices_are_two_thousand_and_rate_is_twentyfive(self):
        for row in self.index['runs']:
            plan = self.files[row['plan']]
            self.assertEqual(plan['devices'], 2000)
            self.assertEqual([s['rate'] for s in plan['schedule']['steps']], [25, 25])
            self.assertEqual([s['duration_s'] for s in plan['schedule']['steps']], [300, 600])
            self.assertEqual(row['expected_warmup_messages'], 7500)
            self.assertEqual(row['expected_measurement_messages'], 15000)
            self.assertEqual(row['workload_id'], 'W2')
            self.assertFalse(row['counts_toward_proposed_attempts'])

    def test_pool_is_identical_to_b0_at_twentyfive(self):
        """25/s must not silently change the generator pool away from B0's 150."""
        for row in self.index['runs']:
            plan = self.files[row['plan']]
            self.assertEqual(plan['vus']['max'], 150)
            self.assertEqual(plan['vus']['pre_allocated'], 150)
            self.assertEqual(plan['health']['workload_policy'], 'dispatch-integrity-v1')

    def test_only_identity_and_device_count_differ_from_the_w2_attempt(self):
        for row in self.index['runs']:
            qual = self.files[row['plan']]
            canon = json.loads((W_ROOT / 'plans' /
                                f'w2dp-{stage.PREFIX[row["platform"]]}-r01-20261108.json').read_text())
            for field in ('adapter', 'devices', 'request_timeout_ms', 'vus', 'connection',
                          'arrivals', 'target', 'payload', 'health', 'test_faults', 'cleanup'):
                self.assertEqual(qual[field], canon[field], f'{row["run_id"]}: {field}')
            self.assertEqual([(s['rate'], s['duration_s'], s['vus']) for s in qual['schedule']['steps']],
                             [(s['rate'], s['duration_s'], s['vus']) for s in canon['schedule']['steps']])

    def test_ids_never_collide_with_w1_or_proposed_attempts(self):
        matrix = json.loads((W_ROOT / 'run-matrix.json').read_text())
        proposed = {r['run_id'] for r in matrix['experimental_runs']}
        w1 = {r['run_id'] for r in stage.artifacts()[1]['runs']}
        w2 = {r['run_id'] for r in self.index['runs']}
        self.assertFalse(proposed & w2)
        self.assertFalse(w1 & w2)
        for rid in w2:
            self.assertTrue(rid.startswith('w2-qual-'), rid)


class QualificationIsolationTests(unittest.TestCase):
    def test_id_lookup_separates_attempts_and_qualifications(self):
        launcher = load(ROOT / 'scripts' / 'run-workload-campaign.py')
        kind, row = launcher.locate('w1dp-tb-r01-20261108')
        self.assertEqual(kind, 'attempt')
        qualified = [r for r in launcher.qualification_rows()
                     if r.get('execution_status') == 'NOT_LAUNCH_CLEARED']
        if not qualified:
            self.skipTest('no launchable qualification row remains: every platform is measured')
        live = qualified[0]['run_id']
        kind, row = launcher.locate(live)
        self.assertEqual(kind, 'qualification')
        # A qualification's plan lives under its own workload root: W1 keeps the
        # historical directory, later workloads get a subdirectory.
        self.assertEqual(launcher.run_plan(kind, row),
                         launcher.qual_root_for(row) / row['plan'])
        with self.assertRaises(SystemExit):
            launcher.locate('w9dp-tb-r01-20261108')


class ReadinessPollResilienceTests(unittest.TestCase):
    """A transport hiccup must not be mistaken for platform unreadiness.

    Observed 2026-11-10: the ThingsBoard qualification aborted because one SSH
    poll through the jump host exceeded its socket timeout, while the platform was
    in fact healthy (login=200, both HAProxy backends UP). Retrying the poll is
    correct; retrying a genuinely unready platform is not, so both are asserted.
    """

    def setUp(self):
        import importlib.util
        import sys
        sys.path.insert(0, str(ROOT / 'scripts'))
        spec = importlib.util.spec_from_file_location('w_launch', ROOT / 'scripts' / 'run-workload-campaign.py')
        self.w = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.w)
        self.b0 = self.w.load_b0()

    READY = ('haproxy_backend_status{proxy="tb-core",state="UP"} 1\n'
             'haproxy_backend_status{proxy="tb-http-transport",state="UP"} 1\n'
             'haproxy_server_status{proxy="tb-core",server="a",state="UP"} 1\n'
             'haproxy_server_status{proxy="tb-core",server="b",state="UP"} 1\n'
             'haproxy_server_status{proxy="tb-http-transport",server="a",state="UP"} 1\n'
             'haproxy_server_status{proxy="tb-http-transport",server="b",state="UP"} 1\n')

    class _P:
        def __init__(self, rc, out):
            self.returncode, self.stdout = rc, out

    def test_transient_timeout_is_retried_not_fatal(self):
        import subprocess
        calls = {'n': 0}

        def flaky(host, cmd, timeout=180, check=True):
            calls['n'] += 1
            if calls['n'] == 1:
                raise subprocess.TimeoutExpired(cmd, timeout)
            return self._P(0, self.READY)

        self.b0.ssh = flaky
        with self.w.WBound(self.b0):
            out = self.b0.wait_tb_backends(timeout=10)
        self.assertIn('tb-core', out)
        self.assertEqual(calls['n'], 2)

    def test_persistent_transport_failure_still_fails_closed(self):
        import subprocess

        def always_timeout(host, cmd, timeout=180, check=True):
            raise subprocess.TimeoutExpired(cmd, timeout)

        self.b0.ssh = always_timeout
        with self.w.WBound(self.b0):
            with self.assertRaises(RuntimeError) as ctx:
                self.b0.wait_tb_backends(timeout=3)
        self.assertIn('must be UP', str(ctx.exception))

    def test_single_backend_up_is_not_readiness(self):
        """One of two backends UP must not be accepted as ready."""
        half = self.READY.replace('proxy="tb-core",state="UP"} 1', 'proxy="tb-core",state="UP"} 0')
        self.b0.ssh = lambda host, cmd, timeout=180, check=True: self._P(0, half)
        with self.w.WBound(self.b0):
            with self.assertRaises(RuntimeError):
                self.b0.wait_tb_backends(timeout=3)


class ConsumedGenerationTests(unittest.TestCase):
    """A consumed ID is evidence, never a launch candidate."""

    def setUp(self):
        self.launcher = load(ROOT / 'scripts' / 'run-workload-campaign.py')

    def test_consumed_id_is_refused_with_its_successor(self):
        rows = {r['run_id']: r for r in self.launcher.qualification_rows()}
        consumed = [rid for rid, r in rows.items()
                    if r.get('execution_status', 'NOT_LAUNCH_CLEARED') != 'NOT_LAUNCH_CLEARED']
        self.assertTrue(consumed, 'expected at least one preserved consumed generation')
        # Prefer a consumed ID that actually has a successor; a consumed-but-unsuperseded
        # ID is covered by the second assertion below.
        rid = next((r for r in sorted(consumed) if rows[r].get('superseded_by')), sorted(consumed)[0])
        with self.assertRaises(SystemExit) as ctx:
            self.launcher.locate(rid)
        self.assertIn('already consumed', str(ctx.exception))
        successor = rows[rid]['superseded_by']
        if successor:
            self.assertIn(successor, str(ctx.exception))
        else:
            self.assertIn('not superseded', str(ctx.exception))

    def test_consumed_without_successor_is_still_refused_honestly(self):
        # A platform outside the latest generation is carried forward with no
        # successor. It must still be refused, and must not claim a successor.
        rows = {r['run_id']: r for r in self.launcher.qualification_rows()}
        orphans = [rid for rid, r in rows.items()
                   if r.get('execution_status', 'NOT_LAUNCH_CLEARED') != 'NOT_LAUNCH_CLEARED'
                   and not r.get('superseded_by')]
        for rid in orphans:
            with self.assertRaises(SystemExit) as ctx:
                self.launcher.locate(rid)
            self.assertNotIn('None', str(ctx.exception))
            self.assertIn('not superseded', str(ctx.exception))

    def test_live_generation_still_resolves(self):
        rows = [r for r in self.launcher.qualification_rows()
                if r.get('execution_status') == 'NOT_LAUNCH_CLEARED']
        if not rows:
            self.skipTest('no launchable qualification row remains: every platform is measured')
        kind, row = self.launcher.locate(rows[0]['run_id'])
        self.assertEqual(kind, 'qualification')

    def test_index_preserves_every_earlier_generation(self):
        index = json.loads((self.launcher.W_QUAL_ROOT / 'index.json').read_text())
        ids = {r['run_id'] for r in index['runs']}
        # every plan file on disk must appear in the index, or evidence would vanish
        for plan in self.launcher.W_QUAL_ROOT.glob('w1-qual-*.json'):
            self.assertIn(plan.stem, ids)




if __name__ == '__main__':
    unittest.main()
