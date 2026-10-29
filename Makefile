# Monitoring commands (monitoring/README.md); they wrap scripts/monitoring.sh, which also works without make.
# Platform stacks have their own Makefiles (deploy/fiware/, deploy/edgex/, deploy/thingsboard/).
P ?=

.PHONY: monitoring-up monitoring-check monitoring-reload monitoring-down exporters-up exporters-down exporters-install dashboards

monitoring-up monitoring-check monitoring-reload monitoring-down:   ## load host (ramonavm1)
	scripts/monitoring.sh $(subst monitoring-,,$@)

exporters-up:                                                      ## SUT (ramonavm2): make exporters-up P=edgex
	scripts/monitoring.sh exporters-up $(P)

exporters-down exporters-install:
	scripts/monitoring.sh $@

dashboards:                                                        ## workstation (needs grafana.com)
	python3 monitoring/grafana/build_dashboards.py
