#!/usr/bin/env bash
# Read-only export of the actual stored adapter fields for explicit M1e payload plans.
#
#   scripts/history-payload.sh thingsboard <run_id> > stored-payload.ndjson
#   scripts/history-payload.sh fiware      <run_id> > stored-payload.ndjson
#   scripts/history-payload.sh edgex       <run_id> > stored-payload.ndjson
#
# Each stdout line is the canonical logical object projected from the platform's *stored* fields:
#   {"run_id":...,"seq":...,"device_id":...,"scheduled_send_ms":...,"temperature":...,
#    "humidity":...,"pressure":...,"padding":...}
# The padding value is selected from the actual platform field/key/object; it is never regenerated
# from the seed. A missing field is emitted as JSON null (or causes the query to fail) so
# `protocol.ValidateStored` fails closed instead of treating identity alone as storage evidence.
#
# This is an offline verification helper. It does not prove live delivery, durable commit, or that a
# platform stored the canonical JSON byte-for-byte. The legacy scripts/history-ids*.sh and their
# identity-only output are deliberately unchanged.
#
# FIWARE defaults fit the isolated candidate container; override with FWHC_CONTAINER/FWHC_DB.
set -euo pipefail
platform="${1:?platform}"; run="${2:?run_id}"
[[ "$run" =~ ^[A-Za-z0-9][A-Za-z0-9-]*$ ]] || { echo "bad run id: $run" >&2; exit 2; }
FWHC_CONTAINER="${FWHC_CONTAINER:-fiwhc-timescale-1}"
FWHC_DB="${FWHC_DB:-quantumleap}"

fw_psql() { docker exec "$FWHC_CONTAINER" psql -U postgres -d "$FWHC_DB" -At -v ON_ERROR_STOP=1 "$@" 2>/dev/null; }

fw_tables() {
  fw_psql -c "SELECT quote_ident(t.table_schema)||'.'||quote_ident(t.table_name)
    FROM information_schema.tables t
    WHERE t.table_name LIKE 'et%'
      AND t.table_schema LIKE 'mt%'
      AND EXISTS (SELECT 1 FROM information_schema.columns c
                   WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name
                     AND c.column_name = 'run_id')
      AND EXISTS (SELECT 1 FROM information_schema.columns c2
                   WHERE c2.table_schema = t.table_schema AND c2.table_name = t.table_name
                     AND c2.column_name = 'seq')
    ORDER BY 1;"
}

case "$platform" in
  thingsboard)
    docker exec -i tb-postgres-1 psql -U postgres -d thingsboard -At -v ON_ERROR_STOP=1 <<SQL
WITH k AS (
  SELECT (SELECT key_id FROM key_dictionary WHERE key = 'run_id')            AS run_id_key,
         (SELECT key_id FROM key_dictionary WHERE key = 'seq')               AS seq_key,
         (SELECT key_id FROM key_dictionary WHERE key = 'scheduled_send_ms') AS sched_key,
         (SELECT key_id FROM key_dictionary WHERE key = 'temperature')       AS temp_key,
         (SELECT key_id FROM key_dictionary WHERE key = 'humidity')          AS hum_key,
         (SELECT key_id FROM key_dictionary WHERE key = 'pressure')          AS press_key,
         (SELECT key_id FROM key_dictionary WHERE key = 'padding')           AS padding_key
)
SELECT json_build_object(
  'run_id', r.str_v,
  'seq', coalesce(sq.long_v, sq.dbl_v::bigint, sq.str_v::bigint),
  'device_id', d.name,
  'scheduled_send_ms', coalesce(sc.long_v::numeric, sc.dbl_v::numeric, sc.str_v::numeric),
  'temperature', coalesce(t.long_v, t.dbl_v::bigint),
  'humidity', coalesce(h.long_v, h.dbl_v::bigint),
  'pressure', coalesce(p.long_v, p.dbl_v::bigint),
  'padding', pad.str_v
)
FROM ts_kv r
JOIN k ON true
JOIN device d ON d.id = r.entity_id
LEFT JOIN ts_kv sq  ON sq.entity_id = r.entity_id  AND sq.ts = r.ts  AND sq.key = k.seq_key
LEFT JOIN ts_kv sc  ON sc.entity_id = r.entity_id  AND sc.ts = r.ts  AND sc.key = k.sched_key
LEFT JOIN ts_kv t   ON t.entity_id = r.entity_id   AND t.ts = r.ts   AND t.key = k.temp_key
LEFT JOIN ts_kv h   ON h.entity_id = r.entity_id   AND h.ts = r.ts   AND h.key = k.hum_key
LEFT JOIN ts_kv p   ON p.entity_id = r.entity_id   AND p.ts = r.ts   AND p.key = k.press_key
LEFT JOIN ts_kv pad ON pad.entity_id = r.entity_id AND pad.ts = r.ts AND pad.key = k.padding_key
WHERE r.key = k.run_id_key AND r.str_v = '$run'
ORDER BY r.entity_id, r.ts;
SQL
    ;;
  edgex)
    docker exec -i edgex-database-1 psql -U postgres -d edgex_db -At -v ON_ERROR_STOP=1 <<SQL
SELECT json_build_object(
  'run_id', r.objectvalue->>'run_id',
  'seq', (r.objectvalue->>'seq')::bigint,
  'device_id', coalesce(r.objectvalue->>'device_id', di.devicename),
  'scheduled_send_ms', r.objectvalue->'scheduled_send_ms',
  'temperature', r.objectvalue->'temperature',
  'humidity', r.objectvalue->'humidity',
  'pressure', r.objectvalue->'pressure',
  'padding', r.objectvalue->>'padding'
)
FROM core_data.reading r
JOIN core_data.device_info di ON di.id = r.device_info_id
WHERE r.objectvalue->>'run_id' = '$run';
SQL
    ;;
  fiware)
    tables=$(fw_tables)
    [[ -n "$tables" ]] || { echo "history-payload fiware: no identity-bearing history tables; capture unavailable" >&2; exit 1; }
    warn=0
    while IFS= read -r tbl; do
      [[ -n "$tbl" ]] || continue
      fw_psql -c "
SELECT json_build_object(
  'run_id', t.\"run_id\"::text,
  'seq', (t.\"seq\")::bigint,
  'device_id', t.\"entity_id\",
  'scheduled_send_ms', t.\"scheduled_send_ms\",
  'temperature', t.\"temperature\",
  'humidity', t.\"humidity\",
  'pressure', t.\"pressure\",
  'padding', t.\"padding\"
)
FROM $tbl t
WHERE t.\"run_id\" = '$run' AND t.\"seq\" IS NOT NULL;"
      n=$(fw_psql -c "SELECT count(*) FROM $tbl t WHERE t.\"run_id\" IS NULL AND t.\"__original_ngsi_entity__\" IS NOT NULL AND (t.\"__original_ngsi_entity__\" #>> '{data,run_id,value}') = '$run';")
      if [[ -n "${n:-}" && "${n:-0}" != "0" ]]; then
        echo "history-payload fiware: $n error-column row(s) have no typed padding columns and are not emitted" >&2
        warn=$((warn+n))
      fi
    done <<< "$tables"
    echo "history-payload fiware: unexported_error_rows=$warn (container $FWHC_CONTAINER, run $run)" >&2
    ;;
  *)
    sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
