-- one JSON line: commits, WAL I/O (track_wal_io_timing), top statements (pg_stat_statements)
SELECT json_build_object(
  't_ms', (extract(epoch FROM clock_timestamp()) * 1000)::bigint,
  'db', (SELECT row_to_json(d) FROM (SELECT xact_commit, xact_rollback, tup_inserted, blk_write_time
                                      FROM pg_stat_database WHERE datname = current_database()) d),
  'wal', (SELECT row_to_json(w) FROM pg_stat_wal w),
  'stmts', (SELECT json_agg(s) FROM (SELECT queryid, left(regexp_replace(query, '\s+', ' ', 'g'), 160) AS query, calls,
                                            total_exec_time, mean_exec_time, rows
                                     FROM pg_stat_statements
                                     WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
                                     ORDER BY calls DESC LIMIT 20) s));
