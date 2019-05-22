#!/bin/bash
docker exec bess ./bessctl daemon reset -- run file /conf/spgwu2.bess
docker exec bess ./bessctl show pipeline | tee pipeline.txt
