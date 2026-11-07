#!/usr/bin/env bash
# Pass the calibrated generator environment to k6 without changing legacy plans.
# GOGC=400 is the documented calibrated setting (docs/methodology/generator.md);
# the value is recorded in the run manifest by benchctl -env (names only).
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
if [[ ${1:-} == run ]]; then
  gate_args=()
  [[ -z "${EDGEX_PREPROVISIONED_RUN_ID:-}" ]] || gate_args=(-env "EDGEX_PREPROVISIONED_RUN_ID=$EDGEX_PREPROVISIONED_RUN_ID")
  exec "$root/bin/benchctl" "$@" -env "GOGC=${GOGC:-400}" "${gate_args[@]}"
fi
exec "$root/bin/benchctl" "$@"
