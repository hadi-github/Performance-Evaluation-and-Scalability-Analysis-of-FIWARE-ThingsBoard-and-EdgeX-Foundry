#!/usr/bin/env bash
# Schema-specific exporter: EdgeX's join was live-verified 2026-10-28 (see below).
# Recheck the pinned schema after image/schema changes; query errors must remain failures.
#
# Companion to scripts/history-ids.sh. It additionally emits the device and the store timestamp, which
# `benchctl audit -history` needs for ThingsBoard-style same-millisecond collision analysis (D33 item 4):
#   {"run_id": ..., "seq": ..., "device": ..., "ts": ...}
#
#   scripts/history-ids-audit.sh thingsboard <run_id> > history.ndjson   ts_kv "seq" joined to "run_id"; device name; ts (ms)
#   scripts/history-ids-audit.sh edgex <run_id> > history.ndjson         core-data readings; device name; origin (ns -> ms)
#   scripts/history-ids-audit.sh edgex describe                          print core_data.reading columns (read-only)
#   scripts/history-ids-audit.sh fiware <run_id>                         QL/TimescaleDB, device and store ts
#
# The export is read-only: no statement modifies the database. Run it on the SUT only under a separately authorized
# run; the offline check is `benchctl audit` on an existing export.
set -euo pipefail
platform="${1:?platform}"; run="${2:-}"

check_run() {
  [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9-]*$ ]] || { echo "bad run id: $1" >&2; exit 2; }
}

case "$platform" in
  thingsboard)
    check_run "${run:?run_id}"
    docker exec -i tb-postgres-1 psql -U postgres -d thingsboard -At -v ON_ERROR_STOP=1 <<SQL
SELECT json_build_object('run_id', r.str_v, 'seq', coalesce(s.long_v, s.dbl_v::bigint, s.str_v::bigint),
                         'device', d.name, 'ts', s.ts)
FROM ts_kv s
JOIN ts_kv r ON r.entity_id = s.entity_id AND r.ts = s.ts
 AND r.key = (SELECT key_id FROM key_dictionary WHERE key = 'run_id')
JOIN device d ON d.id = s.entity_id
WHERE s.key = (SELECT key_id FROM key_dictionary WHERE key = 'seq') AND r.str_v = '$run';
SQL
    ;;
  edgex)
    if [[ "$run" == "describe" ]]; then
      docker exec -i edgex-database-1 psql -U postgres -d edgex_db -At -v ON_ERROR_STOP=1 \
        -c "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'core_data' AND table_name = 'reading' ORDER BY ordinal_position;"
      exit 0
    fi
    check_run "${run:?run_id or describe}"
    # Live-verified 2026-10-28 against the pinned EdgeX schema: core_data.reading has NO devicename column;
    # the device name comes from core_data.device_info (id, devicename, …) joined via reading.device_info_id.
    docker exec -i edgex-database-1 psql -U postgres -d edgex_db -At -v ON_ERROR_STOP=1 <<SQL
SELECT json_build_object('run_id', r.objectvalue->>'run_id', 'seq', (r.objectvalue->>'seq')::bigint,
                         'device', di.devicename, 'ts', r.origin / 1000000.0)
FROM core_data.reading r
JOIN core_data.device_info di ON di.id = r.device_info_id
WHERE r.objectvalue->>'run_id' = '$run';
SQL
    ;;
  fiware)
    # FIWARE history export now implemented (QL 1.0.0 + TimescaleDB; live-verified pair-test auto-20261027).
    # describe: per-tenant tables + identity columns (B14 schema check); run_id: NDJSON with device and ts.
    if [[ "$run" == "describe" ]]; then
      docker exec fiwhc-timescale-1 psql -U postgres -d quantumleap -At -v ON_ERROR_STOP=1 \
        -c "SELECT table_schema, table_name, column_name, data_type FROM information_schema.columns
            WHERE table_schema LIKE 'mt%' AND table_name LIKE 'et%'
              AND column_name IN ('run_id','seq','time_index','entity_id','__original_ngsi_entity__')
            ORDER BY table_schema, table_name, ordinal_position;"
      exit 0
    fi
    check_run "${run:?run_id or describe}"
    exec "$(dirname "$0")/history-ids.sh" fiware "$run"
    ;;
  *) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
