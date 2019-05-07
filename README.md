# ngic-bess

## Pre-reqs

To follow instructions, install

* Docker CE >= 17.06
* 2 DPDK compatiable interfaces bound to VFIO and update their iommu_group id in `setup.sh`

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
