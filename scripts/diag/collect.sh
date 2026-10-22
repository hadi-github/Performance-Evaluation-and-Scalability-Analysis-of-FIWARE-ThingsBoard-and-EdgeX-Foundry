#!/usr/bin/env bash
# Diagnostics collectors on the SUT (ramonavm2), all pinned to core 0 so they never charge CPU to a platform
# container: sidecar containers on the platform network for psql / valkey-cli / mongosh, and a host loop for HTTP
# endpoints. Output in /root/diag/<run_id>/.
#
#   scripts/diag/collect.sh start <edgex|thingsboard|fiware> <run_id> [mongo_profile_rate]
#   scripts/diag/collect.sh stop <run_id>
#
#   cpu.tsv          monitoring/exporters/cpu-snapshot.sh every 1 s (cgroup usage per container)
#   containers.tsv   name, IP, cpuset, memory limit, start time, restart count (at start and at stop)
#   pg-activity.tsv  EdgeX: pg_stat_activity every 1 s (t_ms, pid, client IP, app, state, wait type, wait event, type)
#   pg-counters.ndjson  EdgeX: pg_stat_database / pg_stat_wal / pg_stat_statements every 10 s
#   valkey.txt       ThingsBoard: INFO commandstats every 10 s (server_time_usec as timestamp)
#   mongo.ndjson     FIWARE: opcounters every 5 s; with a profile rate, one "profile" line (scripts/diag/mongo-loop.js)
#   http.ndjson      NATS varz/connz (EdgeX), Orion /admin/metrics and /statistics per replica (FIWARE), every 10 s
set -euo pipefail
cd "$(dirname "$0")/../.."
REPO=$PWD
cmd="${1:?start|stop}"

containers() { # name ip cpuset mem started restarts, for one compose project
  docker ps -a --filter "label=com.docker.compose.project=$1" -q | xargs -r docker inspect -f \
    '{{.Name}}	{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}	{{.HostConfig.CpusetCpus}}	{{.HostConfig.Memory}}	{{.State.StartedAt}}	{{.RestartCount}}' |
    sed 's|^/||' | sort
}

case "$cmd" in
  start)
    platform="${2:?platform}"; run="${3:?run_id}"; prate="${4:-0}"
    case "$platform" in edgex) proj=edgex net=edgex_edgex-network ;; thingsboard) proj=tb net=tb_default ;;
                        fiware) proj=fiware net=fiware_default ;; *) echo "bad platform" >&2; exit 2 ;; esac
    out=/root/diag/$run; mkdir -p "$out"; echo "$platform $proj" > "$out/platform"
    containers "$proj" > "$out/containers.tsv"
    (setsid nohup taskset -c 0 monitoring/exporters/cpu-snapshot.sh "$proj" --loop 1 > "$out/cpu.tsv" 2>/dev/null < /dev/null & echo $! > "$out/cpu.pid")
    side() { docker run -d --name "diag-$1-$run" --cpuset-cpus 0 --memory 256m --network "$net" \
               -v "$REPO/scripts/diag:/diag:ro" "${@:2}" >/dev/null; echo "diag-$1-$run" >> "$out/sidecars"; }
    : > "$out/sidecars"
    case "$platform" in
      edgex)
        pg=(-e PGHOST=database -e PGUSER=postgres -e PGPASSWORD=postgres -e PGDATABASE=edgex_db postgres:16.3-alpine)
        side pgact "${pg[@]}" sh /diag/pg-loop.sh activity
        side pgcnt "${pg[@]}" sh /diag/pg-loop.sh counters ;;
      thingsboard) side valkey valkey/valkey:8.0-alpine sh /diag/valkey-loop.sh ;;
      fiware) side mongo -e PROFILE_RATE="$prate" mongo:8.0 mongosh --quiet mongodb://mongo:27017 /diag/mongo-loop.js ;;
    esac
    # host loop: HTTP endpoints by container IP
    (setsid nohup taskset -c 0 bash -c '
      out=$1; platform=$2
      while :; do
        t=$(date +%s%3N)
        if [ "$platform" = edgex ]; then
          ip=$(docker inspect -f "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}" edgex-nats-1)
          for ep in varz "connz?subs=1"; do
            curl -s -m 5 "http://$ip:8222/$ep" | jq -c --arg t "$t" --arg ep "$ep" "{t_ms: (\$t|tonumber), src: (\"nats_\" + \$ep), data: .}" || true
          done
        elif [ "$platform" = fiware ]; then
          for c in $(docker ps --format "{{.Names}}" --filter name=fiware-orion | sort); do
            ip=$(docker inspect -f "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}" $c)
            curl -s -m 5 "http://$ip:1026/admin/metrics" | jq -Rsc --arg t "$t" --arg c "$c" "{t_ms: (\$t|tonumber), src: \"orion_metrics\", container: \$c, data: .}" || true
            curl -s -m 5 "http://$ip:1026/statistics" | jq -c --arg t "$t" --arg c "$c" "{t_ms: (\$t|tonumber), src: \"orion_statistics\", container: \$c, data: .}" || true
          done
        else exit 0; fi
        sleep 10
      done' _ "$out" "$platform" >> "$out/http.ndjson" 2>/dev/null < /dev/null & echo $! > "$out/http.pid")
    echo "collecting into $out"
    ;;
  stop)
    run="${2:?run_id}"; out=/root/diag/$run
    read -r platform proj < "$out/platform"
    for p in cpu http; do [ -f "$out/$p.pid" ] && kill "$(cat "$out/$p.pid")" 2>/dev/null || true; done
    pkill -f "[c]pu-snapshot.sh $proj --loop" || true
    while read -r s; do
      case "$s" in
        diag-pgact-*) f=pg-activity.tsv ;; diag-pgcnt-*) f=pg-counters.ndjson ;;
        diag-valkey-*) f=valkey.txt ;; diag-mongo-*) f=mongo.ndjson ;;
      esac
      docker logs "$s" > "$out/$f" 2> "$out/$f.err" || true
      docker rm -f "$s" >/dev/null 2>&1 || true
    done < "$out/sidecars"
    containers "$proj" > "$out/containers-end.tsv"
    ls -la "$out"
    ;;
  *) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
