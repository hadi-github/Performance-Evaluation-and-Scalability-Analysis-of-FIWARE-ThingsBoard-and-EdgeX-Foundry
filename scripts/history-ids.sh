#!/usr/bin/env bash
# Export the message ids a platform stored in its history, as NDJSON {"run_id","seq"} for `benchctl reconcile
# -store` and `benchctl explore -history` (docs/exploratory-capacity.md, "Reconciliation"). Run on the SUT.
#
#   scripts/history-ids.sh thingsboard <run_id> > history.ndjson   ts_kv rows of key "seq" joined with key "run_id"
#   scripts/history-ids.sh edgex <run_id> > history.ndjson         core-data readings, objectValue seq/run_id
#   scripts/history-ids.sh fiware <run_id>                         no history (FIWARE-no-history): exits 3
#
# ThingsBoard keys telemetry by (entity, key, ts) with the server receipt time as ts, so two messages of one
# device received in the same millisecond overwrite each other; they show up as missing, not as a lost write.
set -euo pipefail
platform="${1:?platform}"; run="${2:?run_id}"
[[ "$run" =~ ^[A-Za-z0-9][A-Za-z0-9-]*$ ]] || { echo "bad run id: $run" >&2; exit 2; }

case "$platform" in
  thingsboard)
    docker exec -i tb-postgres-1 psql -U postgres -d thingsboard -At -v ON_ERROR_STOP=1 <<SQL
SELECT json_build_object('run_id', r.str_v, 'seq', coalesce(s.long_v, s.dbl_v::bigint, s.str_v::bigint))
FROM ts_kv s
JOIN ts_kv r ON r.entity_id = s.entity_id AND r.ts = s.ts
 AND r.key = (SELECT key_id FROM key_dictionary WHERE key = 'run_id')
WHERE s.key = (SELECT key_id FROM key_dictionary WHERE key = 'seq') AND r.str_v = '$run';
SQL
    ;;
  edgex)
    docker exec -i edgex-database-1 psql -U postgres -d edgex_db -At -v ON_ERROR_STOP=1 <<SQL
SELECT json_build_object('run_id', objectvalue->>'run_id', 'seq', (objectvalue->>'seq')::bigint)
FROM core_data.reading
WHERE objectvalue->>'run_id' = '$run';
SQL
    ;;
  fiware) echo "FIWARE-no-history keeps no history: not reconcilable" >&2; exit 3 ;;
  *) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
