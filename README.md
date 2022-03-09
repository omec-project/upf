<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2019 Intel Corporation
-->

# upf

[![Go Report Card](https://goreportcard.com/badge/github.com/omec-project/upf-epc)](https://goreportcard.com/report/github.com/omec-project/upf-epc)

[![Build Status](https://jenkins.onosproject.org/buildStatus/icon?job=bess-upf-linerate-tests&subject=Linerate+Tests)](https://jenkins.onosproject.org/job/bess-upf-linerate-tests/)

This project implements a 4G/5G User Plane Function (UPF) compliant with 3GPP TS 23.501. It follows the 3GPP Control and User Plane Separation (CUPS) architecture, making use of the PFCP protocol for the communication between SMF (5G) or SPGW-C (4G) and UPF.
This UPF implementation is actively used as part of [the Aether platform](https://opennetworking.org/aether/) in conjunction with the SD-Core mobile core control plane. 

## Overview

![UPF overview](./docs/images/upf-overview.jpg)

The UPF implementation consists of two layers: 

- **PFCP Agent:** (_pfcpiface_) a Go-based implementation of the PFCP northbound API used to interact with the mobile core control plane.
- **Fastpath:** responsible for the actual data plane packet processing.

The PFCP Agent implements fastpath plugins that translate 
  PFCP messages to fastpath-specific configurations. We currently support two fastpath implementations: 
  - BESS: a software-based fastpath built on top of the [Berkeley Extensible Software Switch](https://github.com/NetSys/bess/) (BESS) framework.
    Please see the ONFConnect 2019 [talk](https://www.youtube.com/watch?v=fqJGWcwcOxE) for more details. You can also see demo videos [here](https://www.youtube.com/watch?v=KxK64jalKHw) and [here](https://youtu.be/rWnZuJeUWi4).
  - [UP4](https://github.com/omec-project/up4): an implementation leveraging ONOS and P4-programmable switches to realize a hardware-based fastpath. 
 
The combination of PFCP Agent and UP4 is usually referred to as P4-UPF. While BESS-UPF denotes the combination of PFCP Agent and the BESS fastpath.
  
PFCP Agent internally abstracts different fastpaths using a common API, while the different plug-ins can use spcific southbound protocols to communicate with the different fastpath instances. Support for new fastpaths can be provided by implementing new plugins.

### Feature List

**PFCP Agent**

* PFCP Association Setup/Release and Heartbeats 
* Session Establishment/Modification with support for PFCP entities such as Packet Detection Rules (PDRs), Forwarding Action Rules (FARs),
QoS Enforcement Rules (QERs).
* UPF-initiated PFCP association  
* UPF-based UE IP address assignment
* Application filtering using SDF filters
* Generation of End Marker Packets
* Downlink Data Notification (DDN) using PFCP Session Report
* Integration with Prometheus for exporting PFCP and data plane-level metrics. 
* Application filtering using application PFDs (_**experimental**_).

**BESS-UPF**

* IPv4 support
* N3, N4, N6, N9 interfacing
* Single & Multi-port support
* Monitoring/Debugging capabilties using
  - tcpdump on individual BESS modules
  - visualization web interface
  - command line shell interface for displaying statistics
* Static IP routing
* Dynamic IP routing
* Support for IPv4 datagrams reassembly
* Support for IPv4 packets fragmentation
* Support for UE IP NAT
* Service Data Flow (SDF) configuration via N4/PFCP.
* I-UPF/A-UPF ULCL/Branching i.e., simultaneous N6/N9 support within PFCP session
* Downlink Data Notification (DDN) - notification only (no buffering)
* Basic QoS support, with per-slice and per-session rate limiting
* Per-flow latency and throughput metrics  
* Network Token Functions (_**experimental**_)

**P4-UPF**

P4-UPF implements a core set of features capable of supporting requirements for a broad range of enterprise use cases.
See [the ONF's blog post](https://opennetworking.org/news-and-events/blog/using-p4-and-programmable-switches-to-implement-a-4g-5g-upf-in-aether/) for an overview of P4-UPF. 
Refer to [the SD-Fabric documentation](https://docs.sd-fabric.org/master/advanced/upf.html) for the detailed feature set.

## Getting started

This repository provides code to build two Docker images: `pfcpiface` (the PFCP Agent) and `bess` (the BESS-based fastpath). 

To build all Docker images run:

```
make docker-build
```

To build a selected image use `DOCKER_TARGETS`:

```
DOCKER_TARGETS=pfcpiface make docker-build
```

The latest Docker images are also published in the OMEC project's DockerHub registry: [upf-epc-bess](https://hub.docker.com/r/omecproject/upf-epc-bess), [upf-epc-pfcpiface](https://hub.docker.com/r/omecproject/upf-epc-pfcpiface).

### Installation

Please see [INSTALL.md](docs/INSTALL.md) for details on how to set up the PFCP Agent with BESS-UPF. 

To install the PFCP Agent with UP4 please follow [the SD-Fabric documentation](https://docs.sd-fabric.org/master/index.html). 

### Configuration

Please see [configuration_guide.md](docs/configuration_guide.md) to learn more about configurations.

### Testing

The UPF project currently implements three types of tests:

**Unit tests** for the PFCP Agent's code. To run unit tests use:

```
make test
```

**E2E integration tests** that verify the inter-working between the PFCP Agent and a fastpath. 
We provide two modes of E2E integration tests: `native` and `docker`. 
The `native` mode invokes Go objects directly from the `go test` framework, thus it makes the test cases easier to debug.
The `docker` mode uses fully Dockerized environment and runs all components (the PFCP Agent and a fastpath mock) as Docker containers. It ensures the correct behavior of the package produced by the UPF project.

To run E2E integration tests for UP4 in the `docker` mode use:

```
make test-up4-integration-docker
```

To run E2E integration tests for BESS-UPF in the `native` mode use:

```
make test-bess-integration-native
```

> NOTE! The `docker` mode for BESS-UPF and the `native` mode for UP4 are not implemented yet.

**PTF tests for BESS-UPF** verify the BESS-based implementation of the UPF fastpath (data plane). 
Follow the included [README](./ptf/README.md) to run PTF tests for BESS-UPF.

## Contributing

The UPF project welcomes new contributors. Feel free to propose a new feature, integrate a new UPF fastpath or fix bugs!

Before contributing, please follow the below guidelines:

* Check out [open issues](https://github.com/omec-project/upf/issues).
* Check out [the developer guide](./docs/developer-guide.md).
* We follow the best practices described in https://google.github.io/eng-practices/review/developer/. Get familiar with them before submitting a PR.
* Both unit and E2E integration tests must pass on CI. Please make sure that tests are passing with your change (see **Testing** section).

## Support

To report any other kind of problem, feel free to open a GitHub Issue or reach out to the project maintainers on the ONF Community Slack.

## License

The project is licensed under the [Apache License, version 2.0](./LICENSES/Apache-2.0.txt). 