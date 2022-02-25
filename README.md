<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2019 Intel Corporation
-->

# upf

[![Go Report Card](https://goreportcard.com/badge/github.com/omec-project/upf-epc)](https://goreportcard.com/report/github.com/omec-project/upf-epc)

[![Build Status](https://jenkins.onosproject.org/buildStatus/icon?job=bess-upf-linerate-tests&subject=Linerate+Tests)](https://jenkins.onosproject.org/job/bess-upf-linerate-tests/)

This project implements User Plane Function (UPF) - the 4G/5G mobile user plane compliant with 3GPP TS 23.501. 
It follows the 3GPP CUPS (Control and User Plane Separation) architecture, making use of the PFCP protocol for the communication between SMF (5G) or SPGW-C (4G) and UPF.
The UPF implementation is a part of the Aether platform. 

## Overview

The UPF project consists of two logical sub-components: 

- **the PFCP Agent** (_pfcpiface_) that implements the northbound interface of UPF and exposes the PFCP server.
- a **fastpath** that implements a data plane of UPF. The PFCP Agent implements fastpath plugins that translates the 
  PFCP semantics to the actual, fastpath-specific data plane configuration. We currently support two fastpath implementations: 
  - BESS-UPF - the UPF implementation that is build on top of [Berkeley Extensible Software Switch](https://github.com/NetSys/bess/) (BESS) programmable framework.
    Please see the ONFConnect 2019 [talk](https://www.youtube.com/watch?v=fqJGWcwcOxE) for more details. You can also see demo videos [here](https://www.youtube.com/watch?v=KxK64jalKHw) and [here](https://youtu.be/rWnZuJeUWi4).
  - [UP4](https://github.com/omec-project/up4) - the open-source P4-based UPF implementation, which is a part of the SD-Fabric project. 
  
## Feature List

### Complete

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
* Downlink Data Notification (DDN) - notification only

### In Progress

* Usage Reporting Rules (URR)
* Application Detection and Control (ADC) configuration via N4/PFCP.

### Pending

* PCC (Policy Control and Charging) rules configuration.
* SDF and APN based Qos Metering for MBR.
* Sponsored Domain Name support
* Buffering of downlink data

## Installation

Please see [INSTALL.md](docs/INSTALL.md) for details on how to set up BESS-UPF. 
To install UP4 please follow [the SD-Fabric documentation](https://docs.sd-fabric.org/master/index.html). 

## License

The UPF implementation is licensed under the [Apache License, version 2.0](./LICENSES/Apache-2.0.txt). 