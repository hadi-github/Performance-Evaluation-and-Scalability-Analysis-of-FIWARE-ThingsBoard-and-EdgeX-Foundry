# Exporters on the SUT.  Usage:  make up P=<thingsboard|fiware|edgex>   |   make down   |   make ps
P ?=
DC = docker compose -f compose.yaml $(if $(P),-f compose.$(P).yaml,)

.PHONY: up down ps firewall kafka
up:
	@test -n "$(P)" || { echo "usage: make up P=<thingsboard|fiware|edgex>"; exit 2; }
	@test -f compose.$(P).yaml || { echo "unknown platform $(P)"; exit 2; }
	$(DC) up -d --remove-orphans

# Add the missing TB observer without recreating the always-on node/cAdvisor
# services or removing other exporters. Requires the running tb_default network.
kafka:
	docker compose -f compose.yaml -f compose.thingsboard.yaml up -d --no-deps kafka-exporter

# Removes every exporter: the platform-specific ones are orphans of the base file (same compose project).
down:
	docker compose -f compose.yaml down --remove-orphans

ps:
	docker compose -f compose.yaml ps -a

firewall:
	./firewall.sh install
