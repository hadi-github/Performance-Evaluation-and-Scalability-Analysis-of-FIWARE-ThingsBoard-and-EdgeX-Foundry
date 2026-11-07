#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
tmp_parent=${TMPDIR:-/tmp/opencode}
mkdir -p "$tmp_parent"
tmp=$(mktemp -d "$tmp_parent/tb-log-capture-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
cat >"$tmp/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
if [[ $1 == inspect ]]; then
  [[ $4 == replica-a ]] && printf 'host-a\n' || printf 'host-b\n'
elif [[ $1 == cp ]]; then
  src=$2; dest=$3
  [[ $src == */host-a || $src == */host-b ]]
  mkdir -p "$dest/${src##*/}"
  printf 'current\n' >"$dest/${src##*/}/thingsboard.log"
  printf 'rotated\n' >"$dest/${src##*/}/thingsboard.1.log"
else
  exit 2
fi
MOCK
chmod +x "$tmp/bin/docker"
PATH="$tmp/bin:$PATH" "$repo/scripts/capture-thingsboard-file-logs.sh" "$tmp/out" replica-a replica-b
test "$(cat "$tmp/out/replica-a/container-hostname.txt")" = host-a
test "$(cat "$tmp/out/replica-b/host-b/thingsboard.1.log")" = rotated
if PATH="$tmp/bin:$PATH" "$repo/scripts/capture-thingsboard-file-logs.sh" "$tmp/out" replica-a 2>/dev/null; then
  echo 'expected overwrite protection' >&2; exit 1
fi
echo 'capture_thingsboard_file_logs_test: PASS'
