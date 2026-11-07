#!/usr/bin/env bash
# Read-only capture of ThingsBoard's per-container file logs (including rotations).
set -euo pipefail

usage() { echo "Usage: $0 OUTPUT_DIR CONTAINER [CONTAINER ...]" >&2; }
if (($# < 2)); then usage; exit 2; fi
out=$1; shift
mkdir -p -- "$out"

for container in "$@"; do
  [[ $container =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || { echo "invalid container name: $container" >&2; exit 2; }
  hostname=$(docker inspect --format '{{.Config.Hostname}}' "$container") || {
    echo "cannot inspect container: $container" >&2; exit 1;
  }
  [[ $hostname =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || { echo "invalid hostname for $container" >&2; exit 1; }
  dest="$out/$container"
  if [[ -e $dest ]]; then echo "refusing to overwrite existing capture: $dest" >&2; exit 1; fi
  mkdir -- "$dest"
  printf '%s\n' "$hostname" >"$dest/container-hostname.txt"
  # ThingsBoard writes to /var/log/thingsboard/<container hostname>; copying the
  # directory preserves the active log and all rotated siblings in-place.
  if ! docker cp "$container:/var/log/thingsboard/$hostname" "$dest/"; then
    rmdir -- "$dest" 2>/dev/null || true
    echo "could not copy file logs for $container ($hostname)" >&2
    exit 1
  fi
done
