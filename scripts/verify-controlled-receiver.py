#!/usr/bin/env python3
"""Reconcile controlled receiver raw records with retained constant-arrival benchctl runs."""
import collections
import json
import pathlib
import sys


def analyse(root):
    root = pathlib.Path(root)
    records = []
    for path in sorted((root / 'receiver/out').glob('*.ndjson')):
        records.extend(json.loads(line) for line in path.read_text().splitlines() if line)
    result = {}
    for directory in sorted((root / 'qual400-20261031').iterdir()):
        if not directory.is_dir():
            continue
        run_id = directory.name
        manifest = json.loads((directory / 'manifest.json').read_text())
        plan = json.loads((directory / 'resolved.json').read_text())
        summary = json.loads((directory / 'k6-summary.json').read_text())
        assert plan['arrivals']['model'] == 'constant', 'thinned identities need seed selection'
        received = [r for r in records if r.get('run_id') == run_id]
        counts = collections.Counter(r['seq'] for r in received if isinstance(r.get('seq'), int) and r['seq'] >= 0)
        expected = {seq for s in plan['steps'] for seq in range(s['seq_offset'], s['seq_offset'] + s['ticks'])}
        dropped = sum(s['dropped_iterations'] for s in summary['steps'])
        accounting = all(s['attempted'] == s['ack_success'] + s['explicit_failure'] + s['unknown'] for s in summary['steps'])
        item = {
            'generator_verdict': manifest['verdict'],
            'receiver_rows': len(received), 'receiver_unique': len(counts),
            'duplicate_extra_occurrences': sum(n - 1 for n in counts.values()),
            'malformed': len(received) - sum(counts.values()),
            'missing_planned': len(expected - set(counts)) if not dropped else None,
            'unexpected': len(set(counts) - expected),
            'complete_outcome_accounting': accounting,
            'early_sends': sum(s['early'] for s in summary['steps']),
            'dropped_iterations': dropped,
            'identity_basis': 'exact' if not dropped else 'mapping not exact: no capacity reconciliation',
        }
        assert accounting, run_id
        if manifest['verdict'] == 'VALID':
            assert not any(item[k] for k in ('duplicate_extra_occurrences','malformed','missing_planned','unexpected','early_sends','dropped_iterations')), item
            assert len(received) == sum(s['attempted'] for s in summary['steps']), run_id
        result[run_id] = item
    return result


if __name__ == '__main__':
    print(json.dumps(analyse(sys.argv[1]), indent=2))
