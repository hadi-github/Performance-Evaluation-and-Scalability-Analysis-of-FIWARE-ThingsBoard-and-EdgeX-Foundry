there is two compose files, the ```docker-compose.yml``` is containing grafana and grafana-image-renderer to do the process of image exporting

the ```grafana``` directory contains default configurations that used in process of working, dashboards, data sources

the ```influx.db``` compose file is for influxdb container to collect jmeter datas there

after deploying you should open influxdb panel in browser ```localhost:8086``` and get a admin token and put it on the influx db datasource for grafana
