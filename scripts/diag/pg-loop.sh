#!/bin/sh
# sidecar (postgres image): $1 = activity | counters; PGHOST/PGDATABASE/PGUSER/PGPASSWORD from env
until pg_isready -q; do sleep 1; done
case "$1" in
  activity) exec psql -At -F '	' -f /diag/sql/pg-activity.sql ;;
  counters) psql -qAt -c 'CREATE EXTENSION IF NOT EXISTS pg_stat_statements' >/dev/null 2>&1
            while :; do psql -qAt -f /diag/sql/pg-counters.sql; sleep 10; done ;;
esac
