#!/bin/bash
BESSCTL="/opt/bess/bessctl/bessctl"
docker exec bess $BESSCTL daemon reset -- run file /conf/spgwu.bess
docker exec bess $BESSCTL show pipeline | tee pipeline.txt
