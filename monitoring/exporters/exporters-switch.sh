#!/usr/bin/env bash
# Point the SUT's platform-specific exporters at the platform that is currently up.
#
#   exporters-switch.sh thingsboard | fiware | edgex | none
#
# Why this exists: postgres-exporter publishes 9187 for both ThingsBoard and EdgeX,
# so the two profiles cannot be up together. Node-exporter and cAdvisor are host
# level (network_mode: host / host pid) and stay up across switches; only the
# platform profiles are torn down and rebuilt.
#
# It is monitoring only: nothing here changes a platform container, a volume or a
# measurement input. A switch that fails leaves the previous exporters down, which
# costs evidence but never invalidates a run.
set -euo pipefail
cd "$(dirname "$0")"

target="${1:?usage: exporters-switch.sh thingsboard|fiware|edgex|none}"

profiles=(thingsboard fiware edgex)

# Down every platform profile first: `up` on one profile would otherwise collide
# with the other profile's published ports.
for p in "${profiles[@]}"; do
  docker compose -f compose.yaml -f "compose.${p}.yaml" down --remove-orphans >/dev/null 2>&1 || true
done

if [[ "$target" == "none" ]]; then
  echo "exporters: platform profiles stopped, host exporters left running"
  exit 0
fi

if [[ ! -f "compose.${target}.yaml" ]]; then
  echo "exporters-switch: no profile for '$target'" >&2
  exit 2
fi

# A platform profile joins that platform's Compose network, which only exists while
# the platform has been started at least once. Fail loudly rather than silently
# producing an exporter that scrapes nothing.
case "$target" in
  thingsboard) net="${TB_PROJECT:-tbq20261105}_default" ;;
  fiware)      net="fiware_default" ;;
  edgex)       net="edgex_edgex-network" ;;
esac
if ! docker network inspect "$net" >/dev/null 2>&1; then
  echo "exporters-switch: network '$net' does not exist; is $target up?" >&2
  exit 3
fi

docker compose -f compose.yaml -f "compose.${target}.yaml" up -d --remove-orphans >/dev/null
echo "exporters: $target profile up (network $net)"
