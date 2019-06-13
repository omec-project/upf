# ngic-bess

## Pre-reqs

To follow instructions, you need

* Docker CE >= 17.06
* Update the `--devices` line in `setup.sh` with device files of 2 DPDK bound devices
* Hugepages mounted at `/dev/hugepages` or updated location in `setup.sh`
* Update `conf/setup_trafficgen_routes.sh` and `conf/spgwu.bess` to run iltrafficgen tests

## Init

To run BESS daemon with custom NGIC modules' code

```bash
./docker_setup.sh
```

To init the pipeline or reflect changes to `spgwu.bess`

```bash
docker exec bess /conf/reload.sh
docker exec bess bessctl show pipeline > pipeline.txt
```

## Operate Pipeline

Control program(s) to dynamically configure BESS modules

| Functionality | Controller |
|---------------|------------|
| Routes | [route_control.py](conf/route_control.py) |
| UE sessions | Static trafficgen only in `spgwu.bess` |

## Observe

To view the pipeline, open [http://[hostip]:8000](http://[hostip]:8000)
in a browser

To drop into BESS shell

```bash
docker exec -it bess bessctl
```
