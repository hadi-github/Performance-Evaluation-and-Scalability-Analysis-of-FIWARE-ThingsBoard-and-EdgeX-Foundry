#!/usr/bin/env bash
# Source this to get the deployment configuration in shell scripts.
#
#   source scripts/iotbench-env.sh
#   echo "$SUT_IP"
#
# Values come from `.env` (see `.env.example`) through `scripts/iotbench_config.py`,
# the single place a default is decided. A real environment variable always wins,
# so `SUT_IP=1.2.3.4 ./script` behaves as before.
#
# It is a `source`-only file on purpose: the Python tools import their own copy of
# the same defaults, so both sides agree without one calling the other at runtime.

# shellcheck shell=bash

_iotbench_eval() {
  python3 - <<'PY'
import shlex, sys
sys.path.insert(0, 'scripts')
try:
    import iotbench_config as c
except Exception as exc:  # pragma: no cover - a broken tree should say so loudly
    sys.stderr.write(f'iotbench-env.sh: cannot load scripts/iotbench_config.py: {exc}\n')
    raise SystemExit(1)
values = {
    'SUT': c.SUT, 'LOAD': c.LOAD, 'SUT_IP': c.SUT_IP, 'LOAD_IP': c.LOAD_IP,
    'SSH_USER': c.SSH_USER, 'SSH_PORT_SUT': c.SSH_PORT_SUT,
    'SSH_PORT_LOAD': c.SSH_PORT_LOAD, 'SSH_KEY': c.SSH_KEY, 'JUMP': c.JUMP,
    'SSH_CONFIG': c.SSH_CONFIG, 'SUT_ROOT': c.SUT_ROOT, 'LOAD_ROOT': c.LOAD_ROOT,
    'TB_COMPOSE': c.TB_COMPOSE, 'FW_DIR': c.FW_DIR, 'EDGEX_DIR': c.EDGEX_DIR,
    'REMOTE_RESULTS': c.REMOTE_RESULTS, 'SINK_PORT': c.SINK_PORT,
}
# Never clobber a value the caller already set. This matters on the load host,
# where a script is started with an explicit `SUT_IP=`/`SINK_PORT=` for this run
# and there is no `.env` at all; assigning unconditionally would silently revert
# it to the qualified default and send traffic to the wrong machine.
for key, value in values.items():
    print(f': "${{{key}:={shlex.quote(str(value))}}}"')
PY
}

eval "$(_iotbench_eval)" || return 1

# Deliberately namespaced. `SUT` and `LOAD` are already used by older scripts to
# mean the *address*, not the SSH alias (outbound.sh, post-run-analysis.sh and
# the launcher's `SUT=` environment for run-capture.sh all pass an IP). Exporting
# an alias under those names would silently re-point a running capture at a name
# the load host cannot resolve, so the aliases get their own names.
SUT_ALIAS="${SUT}"
LOAD_ALIAS="${LOAD}"
# Same rule as above for the derived URL: an explicit SINK_URL wins.
: "${SINK_URL_BASE:=http://${LOAD_IP}:${SINK_PORT}}"
export SUT_ALIAS LOAD_ALIAS SUT_IP LOAD_IP SSH_USER SSH_PORT_SUT SSH_PORT_LOAD \
       SSH_KEY JUMP SSH_CONFIG SUT_ROOT LOAD_ROOT TB_COMPOSE FW_DIR EDGEX_DIR \
       REMOTE_RESULTS SINK_PORT SINK_URL_BASE

