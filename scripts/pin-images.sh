#!/usr/bin/env bash
# Resolve every pulled image of a platform to its registry digest and write compose.pinned.yaml.
# Run once before a measurement campaign, commit the file, and always deploy with:
#   docker compose -f compose.yaml -f compose.pinned.yaml up -d
# Locally built images (EdgeX NATS builds) are pinned by source tag in compose.yaml instead; their
# local image IDs are recorded here as comments so the run manifest can reference them.
# Usage: scripts/pin-images.sh <thingsboard|edgex|fiware>
set -euo pipefail
cd "$(dirname "$0")/../${1:?platform dir}" || exit 2
command -v jq >/dev/null || { echo "jq required" >&2; exit 2; }

out=compose.pinned.yaml
{
  echo "# Generated $(date -u +%FT%TZ) by scripts/pin-images.sh - do not edit by hand"
  echo "services:"
  docker compose --profile '*' config --format json \
    | jq -r '.services | to_entries[] | "\(.key) \(.value.image) \(.value.build != null)"' \
    | while read -r svc img built; do
        if [[ "$built" == "true" ]]; then
          id="$(docker image inspect --format '{{.Id}}' "$img" 2>/dev/null || echo not-built)"
          echo "  # ${svc}: locally built ${img} (${id})"
          continue
        fi
        digest="$(docker buildx imagetools inspect "$img" --format '{{json .Manifest}}' | jq -r .digest)"
        echo "  ${svc}:"
        echo "    image: ${img%@*}@${digest}"
      done
} > "$out"
echo "wrote $(pwd)/$out"
