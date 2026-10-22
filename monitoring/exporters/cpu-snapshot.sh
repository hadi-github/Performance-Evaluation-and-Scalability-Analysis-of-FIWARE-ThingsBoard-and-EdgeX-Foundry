#!/usr/bin/env bash
# CPU boundary snapshot (cgroup v2): one line per running container of a compose project with its cumulative
# CPU usage read directly from the kernel (cpu.stat usage_usec), plus the snapshot time. Two snapshots give the
# exact CPU-seconds of each container over the interval: (usage_usec_end - usage_usec_start) / 1e6.
# Used to validate the Prometheus/cAdvisor CPU series (docs/metrics-dictionary.md); the future controller's
# boundary-snapshot interface is in docs/tooling-interfaces.md.
#   ./cpu-snapshot.sh <project: tb|fiware|edgex> > snap.tsv
set -euo pipefail
project="${1:?compose project}"
now=$(date +%s.%N)
for id in $(docker ps -q --no-trunc --filter "label=com.docker.compose.project=$project"); do
  name=$(docker inspect --format '{{.Name}}' "$id"); name=${name#/}
  started=$(docker inspect --format '{{.State.StartedAt}}' "$id")
  f=/sys/fs/cgroup/system.slice/docker-$id.scope/cpu.stat
  usage=$(awk '$1 == "usage_usec" {print $2}' "$f")
  printf '%s\t%s\t%s\t%s\t%s\n' "$now" "$name" "$id" "$usage" "$started"
done
