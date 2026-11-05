#!/usr/bin/env bash
# Export the message ids a platform stored in its history, as NDJSON {"run_id","seq"} for `benchctl reconcile
# -store` and `benchctl explore -history` (docs/methodology/exploratory-capacity.md, "Reconciliation"). Run on the SUT.
#
#   scripts/history-ids.sh thingsboard <run_id> > history.ndjson   ts_kv rows of key "seq" joined with key "run_id"
#   scripts/history-ids.sh edgex <run_id> > history.ndjson         core-data readings, objectValue seq/run_id
#   scripts/history-ids.sh fiware <run_id> > history.ndjson        QuantumLeap 1.0.0 / TimescaleDB rows
#
# ThingsBoard keys telemetry by (entity, key, ts) with the server receipt time as ts, so two messages of one
# device received in the same millisecond overwrite each other; they show up as missing, not as a lost write.
#
# FIWARE (QuantumLeap 1.0.0 + TimescaleDB; reflects the LIVE pair-test of session auto-20261027):
#   - QuantumLeap stores one table row per notification (plain INSERT; no upsert, no unique key), so duplicate
#     copies and same-timestamp rows are stored independently and are countable individually.
#   - A failed insert's original notification JSON is preserved by QL 1.0.0 in the `__original_ngsi_entity__`
#     column of the same table (verified live). If that JSON yields a parseable (run_id, seq), the row is
#     exported as a normal identity. Rows whose stored JSON cannot yield an integer seq are NOT silently
#     dropped: each is emitted on stderr as one JSON line
#       {"history_export_row_unrecoverable": true, "batch_id": ..., "device": ..., "ts": ...,
#        "seq_raw_value": ..., "pg_error": ...}
#     and a count is printed. The stdout export stays strictly parseable for `benchctl audit -history`.
#   - The read path is psql against the QL database (read-only), not Orion's latest state.
#
# Env overrides (defaults fit this session's isolated candidate container; check `docker ps` otherwise):
#   FWHC_CONTAINER (fiwhc-timescale-1), FWHC_DB (quantumleap)
set -euo pipefail
platform="${1:?platform}"; run="${2:?run_id}"
[[ "$run" =~ ^[A-Za-z0-9][A-Za-z0-9-]*$ ]] || { echo "bad run id: $run" >&2; exit 2; }
FWHC_CONTAINER="${FWHC_CONTAINER:-fiwhc-timescale-1}"
FWHC_DB="${FWHC_DB:-quantumleap}"

fw_psql() { docker exec "$FWHC_CONTAINER" psql -U postgres -d "$FWHC_DB" -At -v ON_ERROR_STOP=1 "$@" 2>/dev/null; }

fw_tables() {
  # Data tables only: per-tenant schema "mt<service>", table "et<entity type>", both identity columns present.
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
  fiware)
    unrecoverable=0
    # A process substitution hides fw_tables' exit status from set -e. Discover
    # synchronously so a failed query cannot look like a successful empty export.
    tables=$(fw_tables)
    [[ -n "$tables" ]] || { echo "history-ids fiware: no identity-bearing history tables; capture unavailable" >&2; exit 1; }
    while IFS= read -r tbl; do
      [[ -n "$tbl" ]] || continue
      # typed rows first
      fw_psql -c "
SELECT row_to_json(r)::text FROM (
  SELECT t.\"run_id\"::text AS run_id,
         (t.\"seq\")::bigint AS seq,
         t.\"entity_id\" AS device,
         (extract(epoch FROM t.\"time_index\")*1000)::bigint AS ts
  FROM $tbl t
  WHERE t.\"run_id\" = '$run' AND t.\"seq\" IS NOT NULL
) r;"
      # error-column rows with a recoverable integer seq
      fw_psql -c "
SELECT row_to_json(r)::text FROM (
  SELECT j.run_id::text AS run_id,
         j.seq_raw::bigint AS seq,
         j.device AS device,
         (extract(epoch FROM t.\"time_index\")*1000)::bigint AS ts
  FROM $tbl t,
       LATERAL (SELECT \"__original_ngsi_entity__\" #>> '{data,run_id,value}' AS run_id,
                       \"__original_ngsi_entity__\" #>> '{data,seq,value}'  AS seq_raw,
                       \"__original_ngsi_entity__\" #>> '{data,id}'         AS device) j
  WHERE t.\"run_id\" IS NULL
    AND t.\"__original_ngsi_entity__\" IS NOT NULL
    AND j.run_id = '$run'
    AND j.seq_raw ~ '^-?[0-9]+\$'
) r;"
      # unrecoverable -> explicit stderr marker, counted (never silently dropped)
      marks=$(fw_psql -c "
SELECT row_to_json(r)::text FROM (
  SELECT true AS history_export_row_unrecoverable,
         t.\"__original_ngsi_entity__\" -> 'failedBatchID' AS batch_id,
         t.\"__original_ngsi_entity__\" #>> '{data,id}' AS device,
         (extract(epoch FROM t.\"time_index\")*1000)::bigint AS ts,
         t.\"__original_ngsi_entity__\" #>> '{data,seq,value}' AS seq_raw_value,
         left(t.\"__original_ngsi_entity__\" ->> 'error', 240) AS pg_error
  FROM $tbl t,
       LATERAL (SELECT \"__original_ngsi_entity__\" #>> '{data,run_id,value}' AS run_id,
                       \"__original_ngsi_entity__\" #>> '{data,seq,value}' AS seq_raw) j
  WHERE t.\"run_id\" IS NULL
    AND t.\"__original_ngsi_entity__\" IS NOT NULL
    AND j.run_id = '$run'
    AND j.seq_raw IS NOT NULL
    AND j.seq_raw !~ '^-?[0-9]+\$'
) r;")
      if [[ -n "${marks:-}" ]]; then
        while IFS= read -r m; do
          [[ -n "$m" ]] || continue
          echo "$m" >&2
          unrecoverable=$((unrecoverable+1))
        done <<< "$marks"
      fi
    done <<< "$tables"
    echo "history-ids fiware: unrecoverable=$unrecoverable (container $FWHC_CONTAINER, run $run)" >&2
    ;;
  *)
    sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
