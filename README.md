<!--
SPDX-License-Identifier: Apache-2.0
Copyright(c) 2019 Intel Corporation
-->

# UPF-EPC - Overview

UPF-EPC is a revised version of [ngic-rtc](https://github.com/omec-project/ngic-rtc)'s [dp](https://github.com/omec-project/ngic-rtc/dp).
It works seamlessly with all NFs available in the omec-project's EPC. Like ngic-rtc's dp, it communicates with [cp](https://github.com/omec-project/ngic-rtc/cp) and
conforms to Control User Plane Separated (CUPS) architecture. The prototype is based on the 3GPP TS23501 specifications of EPC and functions as a co-located Service
and Packet Gateway (SPGW-U).

The dataplane is built on top of [Berkeley Extensible Software Switch](https://github.com/NetSys/bess/) (BESS) programmable framework, where each submodule in the SPGW-U
pipeline is represented by a BESS-based module. As a result, the pipeline built using BESS can visually be interpreted as a directed acyclic graph, where
each module represents a graphical node. The revised dataplane is not just more flexible to use but also configurable and operator-friendly.

*Please see the ONFConnect 2019 [talk](https://www.youtube.com/watch?v=fqJGWcwcOxE) for more details.*

BESS tools are available out-of-the-box for debugging and/or monitoring; *e.g.*:

* Run `tcpdump` on arbitrary dataplane pipeline module

```bash
localhost:10514 $ tcpdump s1uFastBPF
  Running: tcpdump -r /tmp/tmpYUlLw8
reading from file /tmp/tmpYUlLw8, link-type EN10MB (Ethernet)
23:51:02.331926 STP 802.1s, Rapid STP, CIST Flags [Learn, Forward], length 102
tcpdump: pcap_loop: error reading dump file: Interrupted system call
localhost:10514 $ tcpdump s1uFastBPF
  Running: tcpdump -r /tmp/tmpUBTGau
reading from file /tmp/tmpUBTGau, link-type EN10MB (Ethernet)
00:03:02.286527 STP 802.1s, Rapid STP, CIST Flags [Learn, Forward], length 102
00:03:04.289155 STP 802.1s, Rapid STP, CIST Flags [Learn, Forward], length 102
00:03:06.282790 IP 0.0.0.0.bootpc > 255.255.255.255.bootps: BOOTP/DHCP, Request from 68:05:ca:37:e2:80 (oui Unknown), length 300
00:03:06.291918 STP 802.1s, Rapid STP, CIST Flags [Learn, Forward], length 102
00:03:07.175420 IP 0.0.0.0.bootpc > 255.255.255.255.bootps: BOOTP/DHCP, Request from 68:05:ca:37:d9:e0 (oui Unknown), length 300
00:03:07.489266 IP 0.0.0.0.bootpc > 255.255.255.255.bootps: BOOTP/DHCP, Request from 68:05:ca:37:d9:e1 (oui Unknown), length 300
00:03:08.130884 IP 0.0.0.0.bootpc > 255.255.255.255.bootps: BOOTP/DHCP, Request from 68:05:ca:37:e1:38 (oui Unknown), length 300
00:03:08.294573 STP 802.1s, Rapid STP, CIST Flags [Learn, Forward], length 102
00:03:10.247193 STP 802.1s, Rapid STP, CIST Flags [Learn, Forward], length 102
```

* Visualize your dataplane pipeline
<img src="https://ibin.co/50MaB2FZdlsz.png">
<!--![](docs/images/bess_snip2.png)-->

## Feature List

### Complete

* IPv4 support
* S1-U, S11, SGi interfacing
* Single & Multi-port support
* Monitoring/Debugging capabilties using *(i)* tcpdump on individual BESS modules, *(ii)* visualization web interface, and *(iii)* command line shell interface for displaying statistics *etc*.
* Static IP routing
* Dynamic IP routing
* Support for IPv4 datagrams reassembly

### In Progress

* Billing and Charging
* Support for IP packets fragmentation

### Pending

* PCC (Policy Control and Charging) rules configuration.
* ADC (Application Detection and control) rules configuration.
* Packet Filters for Service Data Flow (SDF) configuration.
* Packet Selectors/Filters for ADC configuration.
* SDF and APN based Qos Metering for MBR.
* Sponsored Domain Name support
* S5/S8 interfacing

## Installation

Please see [INSTALL.md](INSTALL.md) for details on how to set up CP and UPF-EPC.
