#!/usr/bin/env bash
# Diagnostics round 1, ThingsBoard and EdgeX sessions (docs/hypotheses.md). Run from the workstation.
#   scripts/diag/session-tb-edgex.sh <function> [args]
set -euo pipefail
cd "$(dirname "$0")/../.."
source scripts/diag/lib.sh
TBMEM="TB_CORE_MEM=1536m TB_RULE_ENGINE_MEM=1536m TB_HTTP_TRANSPORT_MEM=768m"   # D11 exploratory deviation
TBCF="COMPOSE_FILE=compose.yaml:diag/compose.diag-kafka-jmx.yaml"

# ---------------------------------------------------------------- ThingsBoard
tb_up() {
  v2 "cd $R && (cd fiware && docker compose down -v --remove-orphans >/dev/null 2>&1);
      (cd edgex && docker compose --profile ui --profile export down -v --remove-orphans >/dev/null 2>&1);
      scripts/monitoring.sh exporters-down >/dev/null 2>&1 || true
      cd thingsboard && export $TBCF $TBMEM && make clean >/dev/null 2>&1 && make install >/dev/null 2>&1 &&
      make up N=1 POOL_CPUSET=4-7 >/dev/null 2>&1 && cd .. && scripts/monitoring.sh exporters-up thingsboard >/dev/null &&
      until curl -sf -m5 -o /dev/null -X POST 127.0.0.1:8080/api/auth/login -H 'Content-Type: application/json' \
        -d '{\"username\":\"tenant@thingsboard.org\",\"password\":\"tenant\"}'; do sleep 5; done
      n=\$(docker ps --filter label=com.docker.compose.project=tb --filter status=running --format '{{.Names}}' | grep -cE 'tb-tb-|haproxy');
      [ \"\$n\" = 4 ] || { echo \"TB NOT UP: \$n of 4 tb services running\"; docker ps -a --filter label=com.docker.compose.project=tb --format '{{.Names}} {{.Status}}'; exit 1; }
      echo tb up \$(date -u +%T)"
}

# Kafka JMX request counters from a core-0 sidecar (no CPU charged to the broker's cgroup)
tb_jmx() {
  v2 "docker run --rm --network tb_default --cpuset-cpus 0 -e KAFKA_HEAP_OPTS=-Xmx128m --entrypoint /opt/kafka/bin/kafka-run-class.sh \
        apache/kafka:4.0.0 org.apache.kafka.tools.JmxTool --jmx-url service:jmx:rmi:///jndi/rmi://kafka:9999/jmxrmi \
        --object-name 'kafka.network:type=RequestMetrics,name=RequestsPerSec,*' --attributes Count --one-time true 2>&1 | tail -2"
}

tb_idle() { # H-T1: 5 min without load after start-up
  local id=${1:-DIAG-tb-idle2}
  log "settling 120 s after start-up"; sleep 120
  v2 "cd $R && scripts/diag/collect.sh start thingsboard $id"
  local a b
  a=$(tb_jmx); v2 "printf '%s\n' \"\$(date +%s%3N)\" > /root/diag/$id/jmx-a.csv"; printf '%s\n' "$a" | v2 "cat >> /root/diag/$id/jmx-a.csv"
  log "idle window 300 s"; sleep 300
  v2 "printf '%s\n' \"\$(date +%s%3N)\" > /root/diag/$id/jmx-b.csv"
  b=$(tb_jmx); printf '%s\n' "$b" | v2 "cat >> /root/diag/$id/jmx-b.csv"
  v2 "cd $R && scripts/diag/collect.sh stop $id >/dev/null"
  v2 "out=/root/diag/$id; K='docker run --rm --network tb_default --cpuset-cpus 0 -e KAFKA_HEAP_OPTS=-Xmx128m --entrypoint /bin/sh apache/kafka:4.0.0 -c'
      \$K '/opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka:9092 --describe' > \$out/kafka-topics.txt 2>&1
      \$K '/opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server kafka:9092 --describe --all-groups --members' > \$out/kafka-groups.txt 2>&1
      docker exec tb-postgres-1 psql -U postgres -d thingsboard -c '\\d queue' -c 'select * from queue' > \$out/tb-queues.txt 2>&1
      for c in tb-tb-core-1 tb-tb-rule-engine-1 tb-tb-http-transport-1; do
        { echo \"== \$c\"; docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' \$c | grep -E '^(JAVA_OPTS|TB_|SQL_|SPRING_|CACHE|REDIS|VALKEY)' | grep -viE 'PASSWORD'
          docker exec \$c sh -c 'tr \"\\0\" \" \" < /proc/1/cmdline; echo; (command -v jcmd >/dev/null && jcmd 1 VM.flags) 2>&1; ls -la /var/log/thingsboard 2>&1'
        } >> \$out/tb-jvm.txt 2>&1
      done; ls \$out"
  v2 "cd /root/diag && tar cf - $id" | v1 "mkdir -p $R/results/diag/sut && tar xf - -C $R/results/diag/sut"
}

tb_gclogs() { # copy the three JVMs' GC logs (cumulative since start) into /root/diag/<id>/gc/
  local id=$1
  v2 "mkdir -p /root/diag/$id/gc && for c in tb-tb-core-1 tb-tb-rule-engine-1 tb-tb-http-transport-1; do
        docker cp \$c:/var/log/thingsboard /root/diag/$id/gc/\$c 2>/dev/null || echo \"no /var/log/thingsboard in \$c\"; done; du -sh /root/diag/$id/gc"
  v2 "cd /root/diag && tar cf - $id/gc" | v1 "tar xf - -C $R/results/diag/sut"
}

tb_stored() { # stored ids of a run once the count is stable
  local id=$1
  v2 "cd $R && prev=-1; n=\$(scripts/history-ids.sh thingsboard $id | wc -l); until [ \"\$n\" = \"\$prev\" ]; do prev=\$n; sleep 20;
      n=\$(scripts/history-ids.sh thingsboard $id | wc -l); done; scripts/history-ids.sh thingsboard $id > /root/diag/$id.history.ndjson; echo \"$id stored \$n\""
  v2 "cat /root/diag/$id.history.ndjson" | v1 "cat > $R/results/diag/$id.history.ndjson"
}

tb_runs() {
  v2 "cd $R && EVIDENCE=/root/diag/tb-rulechain-with-sink.json SUT=127.0.0.1 scripts/outbound.sh thingsboard"
  collect_run thingsboard diag-tb-obs-sink-r100 DIAG-tb-obs-sink-r100
  tb_stored DIAG-tb-obs-sink-r100
  tb_gclogs DIAG-tb-obs-sink-r100
  v2 "cd $R && EVIDENCE=/root/diag/tb-rulechain-without-sink.json SUT=127.0.0.1 scripts/outbound.sh thingsboard-remove"
  collect_run thingsboard diag-tb-obs-nosink-r100 DIAG-tb-obs-nosink-r100
  tb_stored DIAG-tb-obs-nosink-r100
  tb_gclogs DIAG-tb-obs-nosink-r100
  to_v1 /root/diag/tb-rulechain-with-sink.json DIAG-tb-obs-sink-r100
  to_v1 /root/diag/tb-rulechain-without-sink.json DIAG-tb-obs-nosink-r100
  v2 'docker inspect $(docker ps -aq --filter name=tb-) --format "{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}}"; dmesg -T | grep "Killed process" | tail -2'
  explore_run DIAG-tb-obs-sink-r100 -history DIAG-tb-obs-sink-r100.history.ndjson
  explore_run DIAG-tb-obs-nosink-r100 -history DIAG-tb-obs-nosink-r100.history.ndjson
}

tb_all() { tb_up; tb_idle DIAG-tb-idle2; tb_runs; }

# ---------------------------------------------------------------- EdgeX
# edgex_run <N> <plan> <run_id> <compose_files> [nosink]
edgex_run() {
  local n=$1 plan=$2 id=$3 cf=$4 mode=${5:-sink}
  log "$id: provision-then-scale N=$n ($cf)"
  v2 "cd $R && mkdir -p /root/diag && cd edgex && COMPOSE_FILE=$cf scripts/provision-then-scale.sh prepare $n 1000 $id /root/diag/$id.gate.json > /root/diag/$id.prepare.log 2>&1; echo prepare rc=\$?; tail -1 /root/diag/$id.prepare.log"
  # exporters attach to the platform network, which exists only after the first cold start (idempotent)
  v2 "cd $R && scripts/monitoring.sh exporters-up edgex >/dev/null 2>&1; docker ps --format '{{.Names}}' | grep -c exporters- | sed 's/^/exporters running: /'"
  edgex_after_prepare "$plan" "$id" "$cf" "$mode"
}

# edgex_after_prepare <plan> <run_id> <compose_files> [nosink]: everything after provision-then-scale (resumable)
edgex_after_prepare() {
  local plan=$1 id=$2 cf=$3 mode=${4:-sink}
  if [[ $mode == nosink ]]; then v2 "cd $R/edgex && COMPOSE_FILE=$cf docker compose --profile export stop app-http-export 2>&1 | tail -1"; fi
  v2 "docker exec edgex-database-1 psql -U postgres -d edgex_db -Atc \"select name, setting from pg_settings where name in ('synchronous_commit','fsync','wal_sync_method','track_wal_io_timing','track_io_timing','shared_preload_libraries','commit_delay','wal_writer_delay')\" > /root/diag/$id.pgsettings.tsv; cat /root/diag/$id.pgsettings.tsv | tr '\n' ' '; echo"
  collect_run edgex "$plan" "$id"
  v2 "cd $R/edgex && COMPOSE_FILE=$cf scripts/provision-then-scale.sh check $id /root/diag/$id.gate.json /root/diag/$id.check.json || true"
  v2 "cd $R && q() { docker exec edgex-database-1 psql -U postgres -d edgex_db -Atc \"select count(*) from core_data.reading where objectvalue->>'run_id'='$id'\"; };
      prev=-1; c=\$(q); until [ \"\$c\" = \"\$prev\" ]; do prev=\$c; sleep 20; c=\$(q); done; echo \"$id persisted \$c drained \$(date -u +%T)\";
      scripts/history-ids.sh edgex $id > /root/diag/$id.history.ndjson"
  v2 "cd /root/diag && tar cf - $id.gate.json $id.check.json $id.history.ndjson $id.prepare.log $id.pgsettings.tsv" | v1 "cd $R/results/diag && tar xf -"
  v1 "cd $R/results/diag && ../../scripts/persist-series.sh $id > $id.persist.tsv && grep -c -v '^#' $id.persist.tsv"
  explore_run "$id" -history "$id.history.ndjson" -gate "$id.gate.json" -check "$id.check.json" -persist "$id.persist.tsv"
}

edgex_resume() { # after the SSH timeout at DIAG-edgex-k4-r500b's collector start (deployment already prepared)
  local PGS=compose.yaml:diag/compose.diag-pgstat.yaml NOS=compose.yaml:diag/compose.diag-pgstat.yaml:diag/compose.diag-nosync.yaml
  edgex_after_prepare diag-edgex-k4-r500 DIAG-edgex-k4-r500b $PGS
  edgex_run 4 diag-edgex-k4-r500-nosync DIAG-edgex-k4-r500-nosync $NOS
  edgex_run 1 diag-edgex-k1-r200 DIAG-edgex-k1-r200 $PGS
  edgex_run 1 diag-edgex-k1-r200-nosync DIAG-edgex-k1-r200-nosync $NOS
  edgex_run 4 diag-edgex-k4-obs-sink-r200 DIAG-edgex-k4-obs-sink-r200 compose.yaml
  edgex_run 4 diag-edgex-k4-obs-nosink-r200 DIAG-edgex-k4-obs-nosink-r200 compose.yaml nosink
}

edgex_all() {
  v2 "cd $R && scripts/monitoring.sh exporters-down >/dev/null 2>&1 || true; (cd thingsboard && make clean >/dev/null 2>&1);
      (cd fiware && docker compose down -v --remove-orphans >/dev/null 2>&1); echo platforms down"
  local PGS=compose.yaml:diag/compose.diag-pgstat.yaml NOS=compose.yaml:diag/compose.diag-pgstat.yaml:diag/compose.diag-nosync.yaml
  # DIAG-edgex-k4-r500 ran without the exporters (no Prometheus persistence series); rerun under a new id
  edgex_run 4 diag-edgex-k4-r500 DIAG-edgex-k4-r500b $PGS
  edgex_run 4 diag-edgex-k4-r500-nosync DIAG-edgex-k4-r500-nosync $NOS
  edgex_run 1 diag-edgex-k1-r200 DIAG-edgex-k1-r200 $PGS
  edgex_run 1 diag-edgex-k1-r200-nosync DIAG-edgex-k1-r200-nosync $NOS
  edgex_run 4 diag-edgex-k4-obs-sink-r200 DIAG-edgex-k4-obs-sink-r200 compose.yaml
  edgex_run 4 diag-edgex-k4-obs-nosink-r200 DIAG-edgex-k4-obs-nosink-r200 compose.yaml nosink
}

"$@"
