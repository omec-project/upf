docker exec bess /opt/bessctl/bessctl/bessctl daemon reset
docker exec bess /opt/bessctl/bessctl/bessctl run file /router/router.bess
docker exec bess /opt/bessctl/bessctl/bessctl show pipeline | tee bess.txt
