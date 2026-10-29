#!/usr/bin/env bash
# Diagnostics round 1 orchestration, run from the workstation (docs/methodology/hypotheses.md). One function per step; each
# DIAG run = collectors on the SUT + scripts/run-with-sink.sh on the load host + SUT data copied next to the run.
#
#   scripts/diag/session.sh <function> [args]     e.g. scripts/diag/session.sh fiware_all
set -euo pipefail
KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519_vps}"
J=(-o BatchMode=yes -o ConnectTimeout=40 -o ServerAliveInterval=15 -o IdentitiesOnly=yes -i "$KEY" -J root@ramonavm2)
v1() { ssh "${J[@]}" root@87.107.191.32 "$@"; }
v2() { ssh -o BatchMode=yes -o ServerAliveInterval=15 root@ramonavm2 "$@"; }
R=/root/iot-bench-deploy
log() { printf '[%s] %s\n' "$(date -u +%T)" "$*"; }

# run_diag <platform> <plan> <run_id> [mongo_profile_rate] [explore flags...]
run_diag() {
  local platform=$1 plan=$2 id=$3 prate=${4:-0}; shift 4 || shift $#
  log "$id: collectors"
  v2 "cd $R && scripts/diag/collect.sh start $platform $id $prate"
  log "$id: run ($plan)"
  v1 "cd $R && scripts/run-with-sink.sh workloads/plans/$plan.json $id results/diag; tail -3 results/diag/$id.run.txt"
  v2 "cd $R && scripts/diag/collect.sh stop $id >/dev/null"
  v2 "cd /root/diag && tar cf - $id" | v1 "mkdir -p $R/results/diag/sut && tar xf - -C $R/results/diag/sut"
  log "$id: explore"
  v1 "cd $R/results/diag && ../../bin/benchctl explore -run $id -sink $id.sink.ndjson -cpu sut/$id/cpu.tsv $* | head -12" || true
}

fiware_up() { # cold start N=4, exporters, no subscription
  v2 "cd $R && (cd thingsboard && make clean >/dev/null 2>&1); (cd edgex && docker compose --profile ui --profile export down -v --remove-orphans >/dev/null 2>&1);
      scripts/monitoring.sh exporters-down >/dev/null 2>&1 || true
      cd fiware && docker compose down -v --remove-orphans >/dev/null 2>&1; make up N=4 >/dev/null && cd .. &&
      scripts/monitoring.sh exporters-up fiware >/dev/null &&
      until curl -sf -m5 -o /dev/null 127.0.0.1:1026/version && curl -sf -m5 -o /dev/null 127.0.0.1:4041/iot/about; do sleep 3; done; echo fiware up"
}

fiware_procmodel() { # H-F4 / Step 3 FIWARE item 1: process model per IoT Agent and Orion flags
  v2 'out=/root/diag/DIAG-fiware-procmodel.txt; mkdir -p /root/diag; {
    for c in $(docker ps --format "{{.Names}}" --filter name=fiware-iot-agent | sort); do
      echo "== $c"; docker top $c -eo pid,ppid,rss,vsz,args
      docker inspect -f "{{range .Config.Env}}{{println .}}{{end}}" $c | grep -E "^IOTA_(MULTI_CORE|LOG_LEVEL)="
      echo "nproc: $(docker exec $c nproc)"
      echo "node: $(docker exec $c node -e "const o=require(\"os\");console.log(JSON.stringify({cpus:o.cpus().length,availableParallelism:o.availableParallelism?o.availableParallelism():null,node:process.version}))")"
      echo "cpuset: $(docker inspect -f "{{.HostConfig.CpusetCpus}}" $c)  mem_limit: $(docker inspect -f "{{.HostConfig.Memory}}" $c)"
    done
    for c in $(docker ps --format "{{.Names}}" --filter name=fiware-orion | sort); do
      echo "== $c"; docker top $c -eo pid,rss,args; docker inspect -f "entrypoint={{json .Config.Entrypoint}} cmd={{json .Config.Cmd}}" $c
      ip=$(docker inspect -f "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}" $c)
      echo "version: $(curl -s -m5 http://$ip:1026/version | tr -d "\n ")"
      echo "admin/log: $(curl -s -m5 http://$ip:1026/admin/log)"
    done
    echo "== mongo"; docker inspect -f "cmd={{json .Config.Cmd}}" fiware-mongo-1
  } > $out 2>&1; cat $out'
}

fiware_all() {
  fiware_up
  fiware_procmodel
  run_diag fiware diag-fiware-obs-nosink-r50 DIAG-fiware-obs-nosink-r50 0
  log "creating the Orion subscription (waits 65 s)"
  v2 "cd $R && SUT=127.0.0.1 scripts/outbound.sh fiware"
  run_diag fiware diag-fiware-obs-sink-r50 DIAG-fiware-obs-sink-r50 0
  run_diag fiware diag-fiware-k4-r100 DIAG-fiware-k4-r100 90
  v2 'docker inspect $(docker ps -aq --filter name=fiware-) --format "{{.Name}} restarts={{.RestartCount}} oom={{.State.OOMKilled}}"'
}

"$@"
