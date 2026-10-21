# ThingsBoard benchmark stack.  Usage:  make install && make up N=4 && make smoke N=4
include ../scripts/resources.mk
DC := TB_REPLICAS=$(N) docker compose

.PHONY: env pull check install up ps logs smoke down clean
env:
	@test -f .env || cp .env.example .env

pull: env
	docker compose pull

check: env
	$(DC) config --format json | ../scripts/check-resources.sh

# One-shot schema install (skipped if the schema already exists).
install: env
	docker compose up -d --wait postgres valkey
	@if [ "$$(docker compose exec -T postgres psql -U postgres -d thingsboard -tAc "select to_regclass('public.tb_schema_settings') is not null")" = "t" ]; then \
	  echo "ThingsBoard schema already installed - skipping"; \
	else docker compose --profile install run --rm tb-install; fi

up: check
	$(DC) up -d --remove-orphans

ps:
	docker compose ps

logs:
	docker compose logs -f --tail=100

# Three JVMs boot on the same pool cores, so startup takes minutes at small N: wait up to 15 min.
smoke:
	WAIT_TIMEOUT=$${WAIT_TIMEOUT:-900} TB_REPLICAS=$(N) ./scripts/smoke.sh

down:
	docker compose down --remove-orphans

# Removes all data (Postgres, Kafka, Valkey) - required between benchmark cells for a cold start.
clean:
	docker compose --profile install down -v --remove-orphans
