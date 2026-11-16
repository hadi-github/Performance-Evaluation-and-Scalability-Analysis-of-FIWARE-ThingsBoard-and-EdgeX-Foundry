#!/usr/bin/env bash
# Keep the ThingsBoard measurement route present while a TB stack is up.
#
# Ordinal 21 (w4dp-tb-r02) ingested and stored all 22,500 messages but delivered
# zero receipts to the sink: the root rule chain was missing the `bench sink`
# TbRestApiCallNode. The B0 launcher installs this route for FIWARE and relies on
# the EdgeX export profile, but never calls `outbound.sh thingsboard`, and nothing
# in the W flow reinstalls it after a diagnostic observer run has removed it.
#
# This is an additive, idempotent guard. It does not modify the benchmark, the
# sink or any hash-bound artefact. It only asserts documented setup:
# `scripts/outbound.sh thingsboard` exits 0 immediately when the node is present.
# It never touches a platform stack that is not ThingsBoard, so
# `isolated_one_platform_at_a_time` still holds.
#
# Usage: scripts/ensure-tb-sink-route.sh [interval_s]
set -euo pipefail
cd "$(dirname "$0")/.."
interval=${1:-30}
log=/tmp/opencode/tb-sink-route.log
mkdir -p "$(dirname "$log")"

# Deployment addresses come from .env; see scripts/iotbench-env.sh.
# shellcheck source=scripts/iotbench-env.sh
source "$(dirname "$0")/iotbench-env.sh"
# iotbench-env.sh leaves SUT as the SSH *alias*; outbound.sh builds the API URL
# from it, so it must be the address. SINK_URL_BASE already points at the load
# host, so the guard only has to pin the SUT side.
export SUT="$SUT_IP"
export SINK_URL="${SINK_URL:-$SINK_URL_BASE}"

while true; do
  if ssh -F .iotbench-ssh-config ramonavm2 \
       'docker ps --format "{{.Names}}" | grep -qx tbq20261105-tb-core-1' 2>/dev/null; then
    if out=$(bash scripts/outbound.sh thingsboard 2>&1); then
      printf '%s ok %s\n' "$(date -u +%H:%M:%SZ)" "$(tr '\n' ' ' <<<"$out")" >> "$log"
    else
      printf '%s FAILED %s\n' "$(date -u +%H:%M:%SZ)" "$(tr '\n' ' ' <<<"$out")" >> "$log"
    fi
  fi
  sleep "$interval"
done
