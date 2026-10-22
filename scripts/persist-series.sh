#!/usr/bin/env bash
# Export a cumulative stored-message counter from Prometheus as the persistence series for `benchctl explore
# -persist` (D23: goodput at persistence). Raw scraped samples (no resampling), from 5 min before the warm-up start
# to DRAIN_S after the last step (default 3600 s, capped at now). Run on ramonavm1.
#
#   scripts/persist-series.sh <run_dir> [selector] > <run_id>.persist.tsv
#   default selector (EdgeX): pg_stat_user_tables_n_tup_ins{relname="reading",schemaname="core_data"}
#
# Output: "# ..." header lines (query, window), then "time_ms<TAB>count". Time resolution = the scrape interval of
# the postgres job (5 s) plus Postgres's statistics flush delay (busy backends flush at most once per second).
set -euo pipefail
dir="${1:?run dir}"
sel="${2:-}"
[[ -n "$sel" ]] || sel='pg_stat_user_tables_n_tup_ins{relname="reading",schemaname="core_data"}'
PROM="${PROM:-http://127.0.0.1:9090}"
read -r start end < <(jq -r '[(.steps[0].scenario_start_ms / 1000 | floor), (.steps[-1].scenario_start_ms / 1000 | floor)] | @tsv' "$dir/k6-summary.json")
last_dur=$(jq -r '.steps[-1].duration_s' "$dir/resolved.json")
from=$((start - 300))
to=$((end + last_dur + ${DRAIN_S:-3600}))
now=$(date +%s); (( to > now )) && to=$now
q="${sel}[$((to - from))s] @ ${to}"
echo "# persistence series for $(basename "$dir"): Prometheus $PROM, query: $q"
echo "# window $(date -u -d @$from +%FT%TZ) .. $(date -u -d @$to +%FT%TZ); columns: time_ms count"
# a counter decrease means a cold start wiped the DB: before the warm-up start it restarts the series (this run's
# own cold start), after it the series ends (the next run's cold start)
curl -fsS "$PROM/api/v1/query" --data-urlencode "query=$q" |
  jq -r 'if (.data.result | length) != 1 then error("expected exactly one series, got \(.data.result | length)")
         else .data.result[0].values[] | "\((.[0] * 1000) | round)\t\(.[1])" end' |
  awk -F'\t' -v w=$((start * 1000)) '
    n > 0 && $2 + 0 < prev && $1 + 0 < w { n = 0; print "# restart at " $1 ": counter decreased (" prev " -> " $2 "), this run'"'"'s cold start" }
    n > 0 && $2 + 0 < prev && $1 + 0 >= w { print "# cut at " $1 ": counter decreased (" prev " -> " $2 "), next cold start"; exit }
    { buf[n++] = $0; prev = $2 + 0 }
    n == 0 { next }
    END { for (i = 0; i < n; i++) print buf[i] }'
