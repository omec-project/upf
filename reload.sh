#!/bin/bash
docker exec bess bessctl daemon reset -- run file /conf/spgwu.bess
docker exec bess bessctl show pipeline | tee pipeline.txt

# Uncomment when testing with trafficgen
# docker exec bess /conf/setup_trafficgen_routes.sh
# docker exec bess bessctl show pipeline | tee pipeline.txt
