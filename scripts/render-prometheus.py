#!/usr/bin/env python3
"""Render `monitoring/prometheus.yml` for this deployment.

Prometheus expands environment variables only in `external_labels`, not in
`scrape_configs`, so a `${SUT_IP}` left in a scrape target would be scraped as
the literal string `${SUT_IP}` — a target that is permanently down and looks
like a platform fault. The template is therefore rendered here, before Prometheus
starts, and Prometheus reads the rendered file.

    scripts/render-prometheus.py            # write monitoring/prometheus.rendered.yml
    scripts/render-prometheus.py --check    # fail if the rendered file is stale

The template keeps `${SUT_IP}` and `${LOAD_IP}` placeholders; every other byte is
literal. Rendering is deterministic, so the same configuration always produces
the same bytes and a stale file is detectable.
"""
from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import iotbench_config as cfg  # noqa: E402

REPO = Path(__file__).resolve().parents[1]
TEMPLATE = REPO / 'monitoring' / 'prometheus.yml'
RENDERED = REPO / 'monitoring' / 'prometheus.rendered.yml'

PLACEHOLDERS = {
    'SUT_IP': cfg.SUT_IP,
    'LOAD_IP': cfg.LOAD_IP,
    'SUT_HOST': cfg.SUT,
    'LOAD_HOST': cfg.LOAD,
}


def render(text: str) -> str:
    for name, value in PLACEHOLDERS.items():
        text = text.replace('${' + name + '}', value)
    return text


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--check', action='store_true', help='verify only; write nothing')
    args = ap.parse_args()

    if not TEMPLATE.is_file():
        print(f'template missing: {TEMPLATE}', file=sys.stderr)
        return 2
    wanted = render(TEMPLATE.read_text())
    leftover = [p for p in PLACEHOLDERS if '${' + p + '}' in wanted]
    if leftover:
        print('unsubstituted placeholders: ' + ', '.join(leftover), file=sys.stderr)
        return 2

    if args.check:
        current = RENDERED.read_text() if RENDERED.is_file() else None
        if current != wanted:
            print(f'{RENDERED.name} is stale; run scripts/render-prometheus.py', file=sys.stderr)
            return 1
        print(f'{RENDERED.name} is current')
        return 0

    RENDERED.write_text(wanted)
    # Keep the file private-ish but readable by the container's nobody user.
    os.chmod(RENDERED, 0o644)
    print(f'rendered {RENDERED.relative_to(REPO)} for SUT {cfg.SUT_IP}, load {cfg.LOAD_IP}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
