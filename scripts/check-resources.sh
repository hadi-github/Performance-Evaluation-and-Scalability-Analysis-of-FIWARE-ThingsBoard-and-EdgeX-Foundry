#!/usr/bin/env bash
# Refuse to start a stack whose memory limits do not fit the host: with limits summing above physical memory a
# "fixed" allocation is not fixed any more. Reads `docker compose config --format json` on stdin.
# Usage (from a platform Makefile):  docker compose config --format json | ../scripts/check-resources.sh
set -euo pipefail
command -v jq >/dev/null || { echo "jq required" >&2; exit 2; }

HOST_RESERVE_MB="${HOST_RESERVE_MB:-1536}"   # left for the OS, Docker and page cache

cfg="$(cat)"
total_mb="$(awk '/^MemTotal:/ {print int($2/1024)}' /proc/meminfo)"

# Every running service must have a memory limit, a matching swap limit and a cpuset.
missing="$(jq -r '.services | to_entries[]
  | select((.value.mem_limit // null) == null or (.value.memswap_limit // null) == null or (.value.cpuset // "") == "")
  | .key' <<<"$cfg")"
if [[ -n "$missing" ]]; then
  echo "services without mem_limit/memswap_limit/cpuset: $(echo $missing)" >&2
  exit 1
fi

used_mb="$(jq '[.services[] | ((.mem_limit | tonumber) * (.deploy.replicas // 1))] | add / 1048576 | floor' <<<"$cfg")"
budget_mb=$(( total_mb - HOST_RESERVE_MB ))

printf 'memory limits: %s MB of %s MB budget (host %s MB, reserve %s MB)\n' \
  "$used_mb" "$budget_mb" "$total_mb" "$HOST_RESERVE_MB"
jq -r '.services | to_entries[] | "  \(.key) x\(.value.deploy.replicas // 1): cpuset=\(.value.cpuset) mem=\((.value.mem_limit | tonumber) / 1048576 | floor)MB"' <<<"$cfg"

if (( used_mb > budget_mb )); then
  echo "memory limits exceed the budget - lower N or the *_MEM values in .env" >&2
  exit 1
fi
