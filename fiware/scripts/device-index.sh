#!/usr/bin/env bash
# D27 (2026-10-26): unique index on iotagentul.devices {service, subservice, apikey, id}, guarded by a duplicate audit.
# The audit runs BEFORE any index creation. A duplicate key or an incomplete key stops the operation and is reported.
# Nothing is deleted, merged or rewritten.
#
#   fiware/scripts/device-index.sh audit [FILE]        audit an NDJSON registry export (default: stdin)
#   fiware/scripts/device-index.sh export              print the registry export from the SUT's mongo (read-only)
#   fiware/scripts/device-index.sh create [--apply]    audit the live registry, then create the unique index;
#                                                      without --apply it only prints the planned command
#
# Exit status: 0 clean, 1 stopped (duplicate or incomplete keys), 2 usage or I/O error.
# Index creation on any registry is not authorized by D27's Text + Impl/checks level; --apply belongs to the later,
# separately authorized step. DEVICE_INDEX_SOURCE=FILE replaces the live export (used by the offline tests).
set -euo pipefail

container="${DEVICE_INDEX_MONGO:-fiware-mongo-1}"
index_name="uniq_service_subservice_apikey_id"
mongo_eval='db.getSiblingDB("iotagentul").devices.find({}, {_id: 0, service: 1, subservice: 1, apikey: 1, id: 1})
  .forEach(d => print(JSON.stringify(d)))'

export_registry() {
  if [[ -n "${DEVICE_INDEX_SOURCE:-}" ]]; then
    cat "$DEVICE_INDEX_SOURCE"
  else
    docker exec -i "$container" mongosh --quiet --eval "$mongo_eval"
  fi
}

audit() {
  local in="${1:-/dev/stdin}" result
  result="$(jq -c -s '
    def k: [.service, .subservice, .apikey, .id];
    . as $all
    | ($all | map(select(k | any(. == null)))) as $inc
    | ($all | map(select(k | all(. != null))) | group_by(k)
        | map(select(length > 1) | {key: (.[0] | k), count: length})) as $dups
    | {total: ($all | length),
       incomplete: ($inc | length),
       incomplete_examples: ($inc[:5] | map(k)),
       duplicate_groups: ($dups | length),
       duplicate_examples: $dups[:20]}
  ' < "$in")" || { echo "audit: could not parse $in" >&2; exit 2; }
  echo "$result"
  if [[ "$(jq -r '(.duplicate_groups + .incomplete) > 0' <<<"$result")" == "true" ]]; then
    echo "STOP (D27): duplicate or incomplete registry keys; no index is created and nothing is changed." >&2
    exit 1
  fi
}

create() {
  local apply=no
  if [[ "${1:-}" == "--apply" ]]; then apply=yes; fi
  export_registry | audit /dev/stdin || exit $?
  local cmd="db.getSiblingDB(\"iotagentul\").devices.createIndex({service: 1, subservice: 1, apikey: 1, id: 1}, {unique: true, name: \"$index_name\"})"
  if [[ "$apply" != yes ]]; then
    echo "dry run (audit clean): would run on $container: $cmd"
    exit 0
  fi
  docker exec -i "$container" mongosh --quiet --eval "$cmd"
}

case "${1:-}" in
  audit)  audit "${2:-}" ;;
  export) export_registry ;;
  create) shift; create "${1:-}" ;;
  *)      sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
