#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Intel Corporation

# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

docker rm -f prom grafana || true

docker run -d --name prom \
	--net=host \
	-v $PWD/conf/prometheus.yml:/etc/prometheus/prometheus.yml \
	prom/prometheus

docker run -d --name grafana \
	--net=host \
	-v $PWD/conf/grafana:/etc/grafana/provisioning \
	-v $PWD/conf/grafana/:/var/lib/grafana/dashboards/ \
	grafana/grafana
