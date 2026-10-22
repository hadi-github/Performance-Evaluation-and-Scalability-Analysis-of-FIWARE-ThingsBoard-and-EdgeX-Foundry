-- one row per backend of the platform DB (sampled every second with \watch 1)
SELECT (extract(epoch FROM clock_timestamp()) * 1000)::bigint, pid, coalesce(host(client_addr), ''),
       application_name, coalesce(state, ''), coalesce(wait_event_type, ''), coalesce(wait_event, ''), backend_type
FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()
\watch 1
