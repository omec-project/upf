<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022-present Open Networking Foundation
-->

# Configuration Guide

## PFCP Agent

This document focuses on frequently used configurations.

Please refer to [upf.jsonc](../conf/upf.jsonc) file for the full list of configurable parameters.

### Common configurations

These are configurations commonly shared between P4-UPF and BESS-UPF.

| Config | Default value | Mandatory | Comments |
| ------ | ------------- | --------- | -------- |
| `log_level` | info | No | |
| `hostname` | - | No | Used to get local IP address and local NodeID in PFCP messages |
| `http_port` | 8080 | No | |
| `max_req_retries` | 5 | No | Max retries for sending PFCP message towards SMF/SPGW-C |
| `resp_timeout` | 2s | No | Period to wait for a response from SMF/SPGW-C |
| `enable_end_marker` | false | No | |
| `enable_gtpu_path_monitoring` | false | No | |
| `cpiface.enable_ue_ip_alloc` | false | No | Whether to enable UPF-based UE IP allocation |
| `cpiface.ue_ip_pool` | - | Yes for P4-UPF or when `enable_ue_ip_alloc` is set | IP pool from which we allocate UE IP address |
| `cpiface.dnn` | - | No | Data Network Name to use during PFCP Association |

### BESS-UPF specific configurations

| Config | Default value | Mandatory | Comments |
| ------ | ------------- | --------- | -------- |
| `measure_upf` | false | No | Enable per port metrics |
| `measure_flow` | false | No | Enable per flow metrics |
| `enable_usage_reporting` | false | No | Measure and report the usage of the URRs the control plane provisions. Needs `measure_flow`. When off, URRs are ignored. When on, a session message with a URR that asks for what the UPF does not do (a reporting trigger other than periodic, volume threshold, time threshold and volume quota, a time quota, event measurement, or any other URR setting than the measurement method, reporting triggers, measurement period, the volume and time thresholds and the volume quota) is refused with "Rule creation/modification Failure", naming the URR; one that queries usage reports is refused with "Service not supported". A URR whose volume quota is exhausted has its traffic dropped at the application QER each of its PDRs is looked up by, the first in the QER list of the PDR, until a new quota is granted or the URR is removed. A PDR whose first QER is the session QER is stopped at the session QER, and the other traffic of the session with it; one whose first QER the session does not hold is left alone, since both pipelines already drop its traffic at the application QER lookup. The QoS modules drop it at their own gate, 7, which `conf/up4.bess` and `conf/closed_loop.bess` sink; the closed-loop pipeline does not pass it on, as it does what the status-drop gate of an application QER drops |
| `urr_poll_interval` | 5s | No | How often URR usage is read from the datapath, at least 1s |
| `access.ifname` | - | Yes | Access-facing network interface name |
| `core.ifname` | - | Yes | Core-facing network interface name |
| `enable_notify_bess` | false | No | Whether to enable Notify feature for DDNs |
