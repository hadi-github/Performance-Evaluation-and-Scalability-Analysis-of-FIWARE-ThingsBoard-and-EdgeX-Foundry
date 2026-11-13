# Fixed resource layout shared by every platform Makefile (see README, "Resource layout").
#
#   core 0              OS, Docker daemon, logging (no containers)
#   LB_CPUSET           HAProxy                                     (.env, default 1)
#   INFRA_CPUSET        databases, brokers, Kafka, ...              (.env, default 2-3)
#   POOL_CPUSET         the platform pool: one core per instance    (computed here from N)
#
# N=1 -> 4, N=2 -> 4-5, N=4 -> 4-7. All replicated services of the platform share the pool, so the platform
# gets exactly N cores at N instances. Memory is a hard per-container limit without swap (compose.yaml + .env).

N ?= 1
POOL_FIRST_CPU ?= 4

NCPU := $(shell nproc)
POOL_LAST_CPU := $(shell echo $$(( $(POOL_FIRST_CPU) + $(N) - 1 )))
# An explicitly supplied pool wins. resources.mk derives one core per instance,
# which is right for the plain `make up N=..` path, but W4 runs N=1 on the same
# two-core pool as N=2 so that replica count is the only variable. `?=` keeps
# every default byte-identical when nothing is supplied, and lets the caller's
# pool be what `make check` actually validates.
POOL_CPUSET ?= $(if $(filter 1,$(N)),$(POOL_FIRST_CPU),$(POOL_FIRST_CPU)-$(POOL_LAST_CPU))

ifneq ($(shell [ $(N) -ge 1 ] 2>/dev/null && [ $(POOL_LAST_CPU) -lt $(NCPU) ] && echo ok),ok)
$(error N=$(N) needs pool cores $(POOL_CPUSET), but this host only has cores 0-$(shell echo $$(( $(NCPU) - 1 ))))
endif

export POOL_CPUSET
