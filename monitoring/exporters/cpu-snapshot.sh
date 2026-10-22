#!/usr/bin/env bash
# CPU boundary snapshot (cgroup v2): one line per running container of a compose project with its cumulative
# CPU usage read directly from the kernel (cpu.stat usage_usec), plus the snapshot time. Two snapshots give the
# exact CPU-seconds of each container over the interval: (usage_usec_end - usage_usec_start) / 1e6.
# Used to validate the Prometheus/cAdvisor CPU series (docs/metrics-dictionary.md); the future controller's
# boundary-snapshot interface is in docs/tooling-interfaces.md.
# Columns: snapshot epoch s, container, id, usage_usec, StartedAt, cpuset.
#   ./cpu-snapshot.sh <project: tb|fiware|edgex> > snap.tsv
#   ./cpu-snapshot.sh <project> --loop [interval_s]   (append a snapshot every interval until killed)
set -euo pipefail
project="${1:?compose project}"
if [[ "${2:-}" == --loop ]]; then
  while :; do "$0" "$project"; sleep "${3:-1}"; done
fi
now=$(date +%s.%N)
ids=$(docker ps -q --no-trunc --filter "label=com.docker.compose.project=$project")
[[ -n "$ids" ]] || exit 0
# one batched inspect per snapshot (cheap enough to loop every second on the OS core)
docker inspect --format '{{.Id}}|{{.Name}}|{{.State.StartedAt}}|{{.HostConfig.CpusetCpus}}' $ids |
while IFS='|' read -r id name started cpuset; do
  usage=$(awk '$1 == "usage_usec" {print $2}' "/sys/fs/cgroup/system.slice/docker-$id.scope/cpu.stat" 2>/dev/null || echo NA)
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$now" "${name#/}" "$id" "$usage" "$started" "${cpuset:-none}"
done
