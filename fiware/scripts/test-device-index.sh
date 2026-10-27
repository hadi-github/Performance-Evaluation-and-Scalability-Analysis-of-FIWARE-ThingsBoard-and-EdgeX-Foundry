#!/usr/bin/env bash
# Offline known-answer tests for device-index.sh (D27). No docker, no SUT.
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
script="$here/device-index.sh"
td="$here/testdata"
fail=0

expect() {  # expect WANT_EXIT NAME COMMAND...
  local want=$1 name=$2 got=0; shift 2
  "$@" >/dev/null 2>&1 || got=$?
  if [[ $got -eq $want ]]; then echo "ok    $name"; else echo "FAIL  $name: exit $got, want $want"; fail=1; fi
}

expect 0 "audit clean registry"           "$script" audit "$td/clean.ndjson"
expect 0 "audit empty registry"           "$script" audit "$td/empty.ndjson"
expect 1 "audit stops on duplicate key"   "$script" audit "$td/duplicate.ndjson"
expect 1 "audit stops on incomplete key"  "$script" audit "$td/incomplete.ndjson"
expect 2 "audit missing file is an error" "$script" audit "$td/nope.ndjson"

dup_report="$("$script" audit "$td/duplicate.ndjson" 2>/dev/null || true)"
if ! grep -q '"dev-1"' <<<"$dup_report"; then
  echo "FAIL  duplicate report names the offending key"; fail=1
else
  echo "ok    duplicate report names the offending key"
fi

expect 0 "create dry run, clean registry" env DEVICE_INDEX_SOURCE="$td/clean.ndjson" "$script" create
expect 1 "create stops, duplicate registry" env DEVICE_INDEX_SOURCE="$td/duplicate.ndjson" "$script" create
expect 1 "create stops, incomplete registry" env DEVICE_INDEX_SOURCE="$td/incomplete.ndjson" "$script" create

out="$(DEVICE_INDEX_SOURCE="$td/duplicate.ndjson" "$script" create 2>&1 || true)"
if grep -q "no index is created" <<<"$out" && ! grep -q "createIndex" <<<"$out"; then
  echo "ok    stopped create prints no index command"
else
  echo "FAIL  stopped create must not print or run createIndex"; fail=1
fi

exit $fail
