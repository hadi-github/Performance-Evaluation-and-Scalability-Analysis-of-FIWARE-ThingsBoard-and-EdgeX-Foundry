#!/usr/bin/env python3
"""Verify an export's checksums, CSV/raw-response equality and offline image links."""
import csv
import hashlib
import json
import math
import pathlib
import re
import sys


def verify(directory):
    directory = pathlib.Path(directory)
    for line in (directory / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ', 1)
        assert hashlib.sha256((directory / name).read_bytes()).hexdigest() == digest, name
    report = json.loads((directory / 'export.json').read_text())
    lookup = {}
    for query in report['queries']:
        if query['status'] == 'ERROR':
            raise AssertionError('query error: ' + query['id'])
        data = json.loads((directory / (query['id'] + '.json')).read_text())
        finite = 0
        for series in data['data']['result']:
            label = json.dumps(series['metric'], sort_keys=True)
            for t, value in series['values']:
                lookup[query['id'], label, round(float(t), 3)] = float(value)
                finite += math.isfinite(float(value))
        assert (query['status'] == 'OK') == (finite > 0), query
    checked = 0
    with (directory / 'measurements.csv').open() as f:
        for row in csv.DictReader(f):
            value = float(row['value'])
            if math.isfinite(value):
                expected = lookup[row['query_id'], row['labels_json'], float(row['unix_seconds'])]
                assert value == expected, row
                checked += 1
    page = (directory / 'index.html').read_text()
    images = re.findall(r'<img src="([^"]+)"', page)
    assert len(images) == 12, len(images)
    for name in images:
        assert not name.startswith(('http:', 'https:')), name
        png = directory / name
        assert png.read_bytes().startswith(b'\x89PNG\r\n\x1a\n'), name
        assert '<svg' in png.with_suffix('.svg').read_text(), name
    return {'run_id': report['run_id'], 'finite_values_checked': checked,
            'query_errors': 0, 'offline_png_svg_pairs': len(images), 'status': 'VERIFIED'}


if __name__ == '__main__':
    for directory in sys.argv[1:]:
        print(json.dumps(verify(directory)))
