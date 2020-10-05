#!/bin/bash

docker rm -f prom grafana || true

docker run -d --name prom \
	--net=host \
	-v $PWD/conf/prometheus.yml:/etc/prometheus/prometheus.yml \
	prom/prometheus

docker run -d --name grafana \
	--net=host \
	-v $PWD/conf/grafana:/etc/grafana/provisioning \
	-v $PWD/conf/grafana/upf-custom.json:/var/lib/grafana/dashboards/upf-custom.json \
	grafana/grafana
