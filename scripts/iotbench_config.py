#!/usr/bin/env python3
"""Single source of truth for deployment endpoints. Import this; never hardcode.

A new environment is configured by editing `.env` (see `.env.example`), not by
editing source. This module is the only place that decides a default, so the
qualified layout and a fresh deployment differ in exactly one file.

Precedence, highest first:
  1. an explicit environment variable (so a one-off override needs no file)
  2. a value from the loaded `.env`
  3. the qualified default, which reproduces the pre-registered testbed

Every default below is the value the frozen campaign already used, so importing
this module changes no behaviour when nothing is configured.
"""
from __future__ import annotations

import os
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
ENV_FILE = Path(os.environ.get('IOTBENCH_ENV_FILE', REPO / '.env'))


def _parse_env_file(path: Path) -> dict[str, str]:
    """Parse KEY=VALUE pairs from `.env`.

    Deliberately tiny and dependency-free: a deployment host should not need
    python-dotenv to start a benchmark. Supports comments, blank lines, `export`
    prefixes and single/double quoted values.

    The result is kept private rather than pushed into `os.environ`. Mutating the
    process environment at import time would leak `.env` values into every child
    process (ssh, docker, the launcher's own subprocesses) and would make a test
    that deliberately passes a different environment see the developer's `.env`
    instead. Lookups go through `get()`, which applies the documented precedence.
    """
    values: dict[str, str] = {}
    if not path.is_file():
        return values
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith('#'):
            continue
        if line.startswith('export '):
            line = line[len('export '):].lstrip()
        if '=' not in line:
            continue
        key, _, value = line.partition('=')
        key = key.strip()
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in '"\'':
            value = value[1:-1]
        values[key] = value
    return values


FILE_VALUES = _parse_env_file(ENV_FILE)


def get(name: str, default: str) -> str:
    """Explicit environment first, then `.env`, then the qualified default."""
    if name in os.environ:
        return os.environ[name]
    return FILE_VALUES.get(name, default)


# --- hosts ------------------------------------------------------------------
# SSH aliases (or hostnames) used to reach each machine.
SUT = get('IOTBENCH_SUT_HOST', get('IOTBENCH_SUT', 'ramonavm2'))
LOAD = get('IOTBENCH_LOAD_HOST', get('IOTBENCH_LOAD', 'ramonavm1'))

# Network addresses as the generator and the platforms see them. These are what
# appear inside plans and generated subscriptions, and they are NOT the SSH
# alias: a host can be reached by a jump alias while advertising a different IP.
SUT_IP = get('IOTBENCH_SUT_IP', '87.107.191.107')
LOAD_IP = get('IOTBENCH_LOAD_IP', '87.107.191.32')

# --- ssh --------------------------------------------------------------------
SSH_USER = get('IOTBENCH_SSH_USER', 'root')
SSH_PORT_SUT = get('IOTBENCH_SSH_PORT_SUT', '22')
SSH_PORT_LOAD = get('IOTBENCH_SSH_PORT_LOAD', '22')
SSH_KEY = get('IOTBENCH_SSH_KEY', '')
JUMP = get('IOTBENCH_JUMP', '')
SSH_CONFIG = get('IOTBENCH_SSH_CONFIG', '')

# --- remote layout ----------------------------------------------------------
# The defaults are the paths the qualified campaign actually used. They embed the
# date the directory was created, which is ugly but load-bearing: live state and
# receipts were recorded against them. A fresh deployment sets the date-free
# `/srv/iot-bench/{sut,load}` layout in .env instead.
SUT_ROOT = get('IOTBENCH_SUT_ROOT', '/root/iot-qual-tb-20261105')
LOAD_ROOT = get('IOTBENCH_LOAD_ROOT', '/root/iot-qual-load-20261105')
TB_COMPOSE = get('IOTBENCH_TB_COMPOSE', '/root/iot-qual-tb-20261105/deploy/thingsboard/compose.yaml')
FW_DIR = get('IOTBENCH_FW_DIR', '/root/iot-bench-deploy-cand-auto20261027')
EDGEX_DIR = get('IOTBENCH_EDGEX_DIR', FW_DIR + '/edgex')
REMOTE_RESULTS = get('IOTBENCH_REMOTE_RESULTS',
                     '/root/iot-qual-load-20261105/feasible-ordinary-dispatch-20261106')

# --- ports ------------------------------------------------------------------
# Platform ingress ports are part of the plan's target URLs. They are listed here
# so a rewrite of a plan's host keeps the qualified ports by default.
PORTS = {
    'thingsboard': {'base_url': 8080},
    'fiware': {'base_url': 1026, 'north_url': 4041, 'south_url': 7896},
    'edgex': {'base_url': 59986, 'metadata_url': 59881, 'ingress_url': 59986},
}
SINK_PORT = get('IOTBENCH_SINK_PORT', '18091')


def target_for(platform: str, host: str = None) -> dict:
    """The plan `target` block for a platform, pointed at `host`."""
    host = host or SUT_IP
    return {k: f'http://{host}:{v}' for k, v in PORTS[platform].items()}


def summary() -> dict:
    return {
        'sut': SUT, 'load': LOAD, 'sut_ip': SUT_IP, 'load_ip': LOAD_IP,
        'ssh_user': SSH_USER, 'ssh_port_sut': SSH_PORT_SUT, 'ssh_port_load': SSH_PORT_LOAD,
        'jump': JUMP or None, 'ssh_config': SSH_CONFIG or None,
        'sut_root': SUT_ROOT, 'load_root': LOAD_ROOT, 'remote_results': REMOTE_RESULTS,
        'env_file': str(ENV_FILE) if ENV_FILE.is_file() else None,
    }


if __name__ == '__main__':
    import json
    print(json.dumps(summary(), indent=2))
