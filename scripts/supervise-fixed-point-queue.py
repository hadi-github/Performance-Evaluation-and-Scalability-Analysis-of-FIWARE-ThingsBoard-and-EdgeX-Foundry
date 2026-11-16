#!/usr/bin/env python3
"""Hands-off supervisor for the W1-W4 fixed-point queue.

Runs independently of the driver. It never launches, stops or modifies a run.
It only:
  * reports progress and any hard stop,
  * prunes dangling Docker artifacts when free disk drops below 10 GiB,
  * records an invalid capture so the matching pre-registered spare can be
    backfilled later.

Every action is appended to supervisor-log.jsonl. The driver stays the single
source of truth for ordinals.
"""
import json
import os
import subprocess
import sys
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
RESULTS = REPO / 'results/workload-batch-20261108'
LOG = RESULTS / 'driver-log.jsonl'
SUP_LOG = RESULTS / 'supervisor-log.jsonl'
MATRIX = REPO / 'workloads/campaigns/fixed-points-w1-w4/run-matrix.json'
EVIDENCE = RESULTS / 'evidence'
MIN_FREE_BYTES = 10 * 1024 ** 3          # standing order: prune below 10 GiB
HARD_STOP_BYTES = 3 * 1024 ** 3          # driver's own guard


def append(path, record):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('a') as f:
        f.write(json.dumps(record, sort_keys=True) + '\n')
        f.flush()


def free_bytes():
    st = os.statvfs(str(REPO))
    return st.f_bavail * st.f_frsize


def driver_alive():
    p = subprocess.run(['pgrep', '-f', 'drive-fixed-point-matrix.py --start-ordinal'],
                       capture_output=True, text=True)
    # Exclude the supervisor's own pgrep match and the tmux wrapper.
    pids = [x for x in p.stdout.split() if x]
    for pid in pids:
        try:
            cmd = Path(f'/proc/{pid}/cmdline').read_bytes().replace(b'\0', b' ').decode()
        except OSError:
            continue
        if 'drive-fixed-point-matrix.py' in cmd and 'pgrep' not in cmd:
            return True
    return False


def terminals():
    done = {}
    if not LOG.is_file():
        return done
    for line in LOG.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            rec = json.loads(line)
        except json.JSONDecodeError:
            continue
        if rec.get('event') == 'terminal':
            done[rec['run_id']] = rec
    return done


def prune():
    before = free_bytes()
    subprocess.run(['docker', 'image', 'prune', '-f'], capture_output=True, text=True)
    subprocess.run(['docker', 'container', 'prune', '-f'], capture_output=True, text=True)
    after = free_bytes()
    append(SUP_LOG, {'event': 'prune', 'free_before_gib': round(before / 1024 ** 3, 1),
                     'free_after_gib': round(after / 1024 ** 3, 1)})
    return after


def main():
    interval = int(sys.argv[1]) if len(sys.argv) > 1 else 300
    seen = set()
    append(SUP_LOG, {'event': 'supervisor-start', 'pid': os.getpid(), 'interval_s': interval})
    idle_after_driver = 0
    while True:
        free = free_bytes()
        alive = driver_alive()
        done = terminals()

        for rid, rec in done.items():
            if rid in seen:
                continue
            seen.add(rid)
            append(SUP_LOG, {'event': 'terminal-seen', 'run_id': rid,
                             'ordinal': rec.get('ordinal'),
                             'outcome': rec.get('outcome'),
                             'seconds': rec.get('seconds')})

        # Capture validity is only known after the launcher writes run-result.json.
        for rid in list(done):
            rr = EVIDENCE / rid / 'run-result.json'
            if not rr.is_file():
                continue
            try:
                res = json.loads(rr.read_text())
            except (json.JSONDecodeError, OSError):
                continue
            key = 'cap:' + rid
            if key in seen:
                continue
            seen.add(key)
            cap = res.get('capture_valid')
            pr = res.get('platform_result')
            # A genuine platform failure/backlog is a finding, not a defect.
            genuine = isinstance(pr, str) and pr.startswith('VALID_PLATFORM_FAILURE')
            if cap is False and not genuine:
                append(SUP_LOG, {'event': 'invalid-capture', 'run_id': rid,
                                 'platform_result': pr,
                                 'needs_pre_registered_spare': True})
            else:
                append(SUP_LOG, {'event': 'capture-ok', 'run_id': rid,
                                 'platform_result': pr, 'capture_valid': cap})

        if free < MIN_FREE_BYTES:
            append(SUP_LOG, {'event': 'disk-low', 'free_gib': round(free / 1024 ** 3, 1)})
            if free < HARD_STOP_BYTES:
                append(SUP_LOG, {'event': 'disk-critical', 'free_gib': round(free / 1024 ** 3, 1)})
            else:
                prune()

        if not alive:
            idle_after_driver += 1
            # Two consecutive misses (~2 intervals) means the driver exited.
            if idle_after_driver >= 2:
                append(SUP_LOG, {'event': 'driver-exited',
                                 'terminals': len(done),
                                 'free_gib': round(free_bytes() / 1024 ** 3, 1)})
                return 0
        else:
            idle_after_driver = 0

        time.sleep(interval)


if __name__ == '__main__':
    sys.exit(main())
