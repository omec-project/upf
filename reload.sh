#!/bin/bash
docker exec bess /conf/reload.sh
# Uncomment when testing with trafficgen
#docker exec bess /conf/setup_trafficgen_routes.sh
docker exec bess bessctl show pipeline | tee pipeline.txt
