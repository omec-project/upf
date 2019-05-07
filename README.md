# ngic-bess

## Pre-reqs

To follow instructions, you need

* Docker CE >= 17.06
* Update the `--devices` line in `setup.sh` with device files of 2 DPDK bound devices

## Init

To run BESS daemon with custom NGIC modules' code

```bash
./setup.sh
```

To init the pipeline or reflect changes to `spgwu.bess`

```bash
./reload.sh
```

## Operate Pipeline

Control program is WIP to dynamically configure

* Routes
* Neighbors
* UE Session Info

## Observe

To drop into BESS shell

```bash
docker exec -it bess ./bessctl
```
