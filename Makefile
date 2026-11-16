# Repository development commands. Platform stacks have their own Makefiles
# (deploy/fiware/, deploy/edgex/, deploy/thingsboard/); monitoring commands wrap
# scripts/monitoring.sh, which also works without make.
P ?=
GO_PACKAGES := ./cmd/... ./internal/... ./tests/...

.PHONY: build test test-short test-race vet fmt fmt-check check test-python test-node \
        monitoring-up monitoring-check monitoring-reload monitoring-down exporters-up exporters-down \
        exporters-install dashboards

build:                                                             ## compile the benchmark binaries
	go build ./cmd/...

test:                                                              ## full Go suite (Docker integration skips when unavailable)
	go test -count=1 $(GO_PACKAGES)

test-short:                                                        ## fast Go suite (skips Docker integration)
	go test -short -count=1 $(GO_PACKAGES)

test-race:                                                         ## race detector on the runner and CLI
	go test -race -count=1 ./internal/runner ./cmd/benchctl

vet:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w cmd internal tests

fmt-check:
	@test -z "$$(gofmt -l cmd internal tests)" || { echo "gofmt needed:"; gofmt -l cmd internal tests; exit 1; }

check: build vet fmt-check test                                     ## CI-equivalent Go gate

test-python:                                                       ## offline Python regressions
	python3 -B -m unittest discover -s tests -p '*_test.py'
	python3 -B -m unittest discover -s tests/offline -p 'test_*.py'

test-node:                                                         ## offline adapter regressions
	node --test tests/offline/*.test.mjs

monitoring-up monitoring-check monitoring-reload monitoring-down:   ## load host (ramonavm1)
	scripts/monitoring.sh $(subst monitoring-,,$@)

exporters-up:                                                      ## SUT (ramonavm2): make exporters-up P=edgex
	scripts/monitoring.sh exporters-up $(P)

exporters-down exporters-install:
	scripts/monitoring.sh $@

dashboards:                                                        ## workstation (needs grafana.com)
	python3 monitoring/grafana/build_dashboards.py
