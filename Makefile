# Monitoring commands (monitoring/README.md). Platform stacks have their own Makefiles (fiware/, edgex/, thingsboard/).
PROM_IMAGE ?= docker.arvancloud.ir/prom/prometheus:v3.13.4
P ?=

.PHONY: monitoring-up monitoring-check monitoring-down monitoring-reload exporters-up exporters-down exporters-install dashboards

## ---- load host (ramonavm1)
monitoring-up:
	cd monitoring && test -f .env || { echo "monitoring/.env missing: cp monitoring/.env.example monitoring/.env and set GF_ADMIN_PASSWORD"; exit 2; }
	cd monitoring && docker compose up -d

# Static checks (promtool) + live target health; exit 1 if a target of the running platform or the load host is down.
monitoring-check:
	docker run --rm -v $(CURDIR)/monitoring:/etc/prometheus --entrypoint promtool $(PROM_IMAGE) check config /etc/prometheus/prometheus.yml
	@curl -fsS 127.0.0.1:9090/api/v1/targets | jq -r '.data.activeTargets[] | [.labels.job, .health, (.lastError // "")] | @tsv' | column -t -s "$$(printf '\t')"

monitoring-reload:
	curl -fsS -X POST 127.0.0.1:9090/-/reload && echo reloaded

monitoring-down:
	cd monitoring && docker compose down

## ---- SUT (ramonavm2)
exporters-up:
	cd monitoring/exporters && $(MAKE) up P=$(P)

exporters-down:
	cd monitoring/exporters && $(MAKE) down

# Once per SUT: firewall (monitoring ports only from ramonavm1) + container-info timer (cpuset/restart/OOM facts).
exporters-install:
	cd monitoring/exporters && ./firewall.sh install && ./container-info.sh install

## ---- workstation (needs grafana.com)
dashboards:
	python3 monitoring/grafana/build_dashboards.py
