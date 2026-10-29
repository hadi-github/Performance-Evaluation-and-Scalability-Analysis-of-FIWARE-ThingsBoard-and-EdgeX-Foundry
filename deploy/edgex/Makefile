# EdgeX benchmark stack.  Usage:  make build && make up N=4 && make smoke N=4
include ../../scripts/resources.mk
DC := EDGEX_REPLICAS=$(N) docker compose

.PHONY: env build check up ui ps logs smoke down clean
env:
	@test -f .env || cp .env.example .env

# Builds the NATS-enabled images from the official v4.0.2 sources (needs github.com + a Go module proxy).
build: env
	docker compose build

check: env
	$(DC) config --format json | ../../scripts/check-resources.sh

up: check
	$(DC) up -d --remove-orphans

ui: check
	$(DC) --profile ui up -d

ps:
	docker compose ps

logs:
	docker compose logs -f --tail=100

smoke:
	EDGEX_REPLICAS=$(N) ./scripts/smoke.sh

down:
	docker compose --profile ui down --remove-orphans

clean:
	docker compose --profile ui down -v --remove-orphans
