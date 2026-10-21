# FIWARE benchmark stack.  Usage:  make up N=4 && make smoke N=4
include ../scripts/resources.mk
DC := FIWARE_REPLICAS=$(N) docker compose

.PHONY: env pull check up ps logs smoke down clean
env:
	@test -f .env || cp .env.example .env

pull: env
	docker compose pull

check: env
	$(DC) config --format json | ../scripts/check-resources.sh

up: check
	$(DC) up -d --remove-orphans

ps:
	docker compose ps

logs:
	docker compose logs -f --tail=100

smoke:
	FIWARE_REPLICAS=$(N) ./scripts/smoke.sh

down:
	docker compose down --remove-orphans

clean:
	docker compose down -v --remove-orphans
