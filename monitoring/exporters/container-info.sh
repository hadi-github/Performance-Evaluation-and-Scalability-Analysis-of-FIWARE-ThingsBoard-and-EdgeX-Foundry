#!/usr/bin/env bash
# Writes bench_container_* metrics for node-exporter's textfile collector: the per-container facts cAdvisor does not
# export (cpuset affinity, Docker restart count, OOM-killed flag). Run by iot-bench-container-info.timer every 10 s.
#   ./container-info.sh            write once
#   ./container-info.sh install    install the systemd service + timer
set -euo pipefail
DIR=/var/lib/node_exporter/textfile
OUT=$DIR/bench_containers.prom

if [[ "${1:-}" == install ]]; then
  install -m 0755 "$0" /usr/local/sbin/iot-bench-container-info
  cat > /etc/systemd/system/iot-bench-container-info.service <<UNIT
[Unit]
Description=IoT benchmark container facts for node-exporter (cpuset, restarts, OOM)
After=docker.service
[Service]
Type=oneshot
ExecStart=/usr/local/sbin/iot-bench-container-info
UNIT
  cat > /etc/systemd/system/iot-bench-container-info.timer <<UNIT
[Unit]
Description=Refresh IoT benchmark container facts every 10 s
[Timer]
OnBootSec=10s
OnUnitActiveSec=10s
AccuracySec=1s
[Install]
WantedBy=timers.target
UNIT
  systemctl daemon-reload
  systemctl enable --now iot-bench-container-info.timer
  echo "installed iot-bench-container-info.timer"
  exit 0
fi

mkdir -p "$DIR"
tmp=$(mktemp "$DIR/.bench.XXXXXX")
{
  echo "# HELP bench_container_cpuset_cpus Number of CPUs in the container's cpuset (0 = not pinned)."
  echo "# TYPE bench_container_cpuset_cpus gauge"
  echo "# HELP bench_container_info Container facts; value is always 1."
  echo "# TYPE bench_container_info gauge"
  echo "# HELP bench_container_restarts Docker restart count of the container (resets when the container is recreated)."
  echo "# TYPE bench_container_restarts gauge"
  echo "# HELP bench_container_oom_killed 1 if the container's last exit was an OOM kill."
  echo "# TYPE bench_container_oom_killed gauge"
  ids=$(docker ps -aq --filter label=com.docker.compose.project)
  [[ -n "$ids" ]] && docker inspect --format \
    '{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}|{{.Name}}|{{.HostConfig.CpusetCpus}}|{{.RestartCount}}|{{.State.OOMKilled}}|{{.State.Status}}' $ids |
  while IFS='|' read -r project service name cpuset restarts oom status; do
    case "$project" in tb|fiware|edgex|exporters) ;; *) continue ;; esac
    name=${name#/}
    n=0
    if [[ -n "$cpuset" ]]; then
      IFS=, read -ra parts <<< "$cpuset"
      for p in "${parts[@]}"; do
        if [[ "$p" == *-* ]]; then n=$((n + ${p#*-} - ${p%-*} + 1)); else n=$((n + 1)); fi
      done
    fi
    case "$project" in tb) platform=thingsboard ;; exporters) platform="" ;; *) platform=$project ;; esac
    l="project=\"$project\",platform=\"$platform\",service=\"$service\",container=\"$name\""
    echo "bench_container_cpuset_cpus{$l} $n"
    echo "bench_container_info{$l,cpuset=\"${cpuset:-none}\",status=\"$status\"} 1"
    echo "bench_container_restarts{$l} $restarts"
    echo "bench_container_oom_killed{$l} $([[ "$oom" == true ]] && echo 1 || echo 0)"
  done
} > "$tmp"
chmod 0644 "$tmp"
mv "$tmp" "$OUT"
