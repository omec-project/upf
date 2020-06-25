<!--
SPDX-License-Identifier: Apache-2.0
Copyright(c) 2019 Intel Corporation
-->

# UPF-EPC - Installation Instructions

## Pre-reqs

You need the following dependencies.

* Docker CE >= 19.03
* Linux kernel version >= 4.15 for Docker; >= 4.19 for AF_XDP
* Hugepages mounted at `/dev/hugepages` or updated location in [`docker_setup.sh`](docker_setup.sh)
* Update mode for devices: `dpdk`, `af_xdp` or `af_packet` in [`docker_setup.sh`](docker_setup.sh),
    along with device details
* Update [`docker_setup.sh`](docker_setup.sh) and [`conf/up4.bess`](conf/up4.bess) to run iltrafficgen tests

>`docker_setup.sh` is a quick start guide to set up UPF-EPC for evaluation.

## Init

### ZMQ Streamer

UPF-EPC communicates with the CP via ZMQ. Please adjust
[`interface.cfg`](https://github.com/omec-project/ngic-rtc/tree/central-cp-multi-upfs/config/interface.cfg) accordingly.

### CP

Please refer to [INSTALL.md](https://github.com/omec-project/ngic-rtc/tree/central-cp-multi-upfs/INSTALL.MD) to get CP running.

### DP

| VAR            | DEFAULT    | NOTES                                              |
|----------------|------------|----------------------------------------------------|
| MAKEFLAGS      | -j$(nproc) | Customize if build fails due to memory exhaustion  |
| DOCKER_BUIDKIT |          1 | Turn off to try legacy builder on older Docker ver |

To run BESS daemon with NGIC modules' code:

```bash
./docker_setup.sh
```

To update the pipeline, reflect changes to [`conf/up4.bess`](conf/up4.bess)
and/or [`conf/upf.json`](conf/upf.json)

To display the pipeline, do:

```bash
docker exec bess ./bessctl show pipeline > pipeline.txt
```

## Operate DP Pipeline

Control program(s) to dynamically configure BESS modules

| Functionality | Controller |
|---------------|------------|
| Routes | [route_control.py](conf/route_control.py) |
| UE sessions | Static trafficgen only in `pfcpiface/main.go` |
| CP communication | [zmq-cpiface.cc](cpiface/zmq-cpiface.cc) |

## Testing

UPF-EPC has been tested against 2 microbenchmark applications (on an Intel Xeon Platinum 8170 @ 2.10GHz)

### [il_trafficgen](https://github.com/omec-project/il_trafficgen)
<!-- Baseline performance of the dataplane is ~5 Mpps per CPU -->
* Tested with up to 30K subscribers
* 128B Ethernet frame size
* 1 default bearer per session

### Spirent Landslide 17.5.0 GA testcases

* Tested with up to 10K subscribers
* 64B Ethernet frame size
* 1 default bearer per session
* 100 transactions/sec

## Observe DP Pipeline

To view the pipeline, open [http://[hostip]:8000](http://[hostip]:8000)
in a browser

To drop into BESS shell

```bash
docker exec -it bess bessctl
```

## Network Token Functions

OMEC includes a Network Token Function (NTF) which provides preliminary support
for Network Tokens, a new protocol to expose datapath services to end users and
application providers. More details are available at https://networktokens.org

In order to compile NTF support, run the following:

```bash
ENABLE_NTF=1 ./docker_setup.sh
```

Update [`conf/spgwu.json`](conf/spgwu.json) and set the `enable_ntf` option to
`true`.
