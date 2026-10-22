#!/usr/bin/env bash
# Monitoring commands (monitoring/README.md). Same as `make monitoring-*` for hosts without make (ramonavm1).
#   scripts/monitoring.sh up | check | reload | down        load host (Prometheus, Grafana, node-exporter, cAdvisor)
#   scripts/monitoring.sh exporters-up <platform> | exporters-down | exporters-install     SUT
set -euo pipefail
cd "$(dirname "$0")/.."
PROM_IMAGE="${PROM_IMAGE:-docker.arvancloud.ir/prom/prometheus:v3.13.4}"
case "${1:-}" in
  up)
    [[ -f monitoring/.env ]] || { echo "monitoring/.env missing: cp monitoring/.env.example monitoring/.env, set GF_ADMIN_PASSWORD" >&2; exit 2; }
    (cd monitoring && docker compose up -d) ;;
  check)
    docker run --rm -v "$PWD/monitoring:/etc/prometheus" --entrypoint promtool "$PROM_IMAGE" check config /etc/prometheus/prometheus.yml
    curl -fsS 127.0.0.1:9090/api/v1/targets | jq -r '.data.activeTargets[] | [.labels.job, .health, (.lastError // "")] | @tsv' | column -t -s $'\t' ;;
  reload) curl -fsS -X POST 127.0.0.1:9090/-/reload && echo reloaded ;;
  down) (cd monitoring && docker compose down) ;;
  exporters-up) (cd monitoring/exporters && docker compose -f compose.yaml -f "compose.${2:?platform}.yaml" up -d --remove-orphans) ;;
  exporters-down) (cd monitoring/exporters && docker compose -f compose.yaml down --remove-orphans) ;;
  exporters-install) (cd monitoring/exporters && ./firewall.sh install && ./container-info.sh install) ;;
  *) sed -n '2,5p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
