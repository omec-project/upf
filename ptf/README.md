<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright 2021 Open Networking Foundation -->
# PTF tests for BESS-UPF


## Structure
* `tests/`: contains all PTF test case definitions
    * `unary/`: single-packet test cases
    * `linerate/`: high-speed test cases
* `lib/`: general purpose libraries and classes to be imported in PTF
test definitions
* `config/`: contains YAML config file definition for TRex along with
other personalized config files

## Tools
* [PTF](https://github.com/p4lang/PTF) (Packet Testing Framework): a
data plane testing framework written in Python
* [TRex](https://github.com/cisco-system-traffic-generator/trex-core): a
high-speed traffic generator built on top of DPDK, containing a Python API

## Overview

The aim of implementing a test framework for UPF is to create a
developer-friendly infrastructure for creating either single-packet or
high-speed tests that assess UPF features at a component level.

This "component-level" is achieved by *bypassing* calls to the PFCP
agent, in favor of communicating with BESS directly via gRPC.

![Routes](docs/upf-access.svg)

This figure illustrates two options for communicating with the UPF.  In
this framework, we opt for **BESS gRPC calls** instead of calls to the PFCP
agent because they allow direct communication between the framework and
the BESS instance for both installing rules and reading metrics.

## Tests
This directory maintains all of the test case definitions (written in
Python), as well as scripts for running them in a test environment. We
currently provide two general types of test cases:

* **Unary** tests are *single packet* tests that assess UPF
performance in specific scenarios. Packets are crafted and sent to the
UPF using the `Scapy` packet library.
    > *Example*: a unary test could use Scapy to send a single
    non-encapsulated UDP packet to the core interface of the UPF, and
    assert that a GTP-encapsulated packet was received from the access
    interface.

* **Linerate** tests assess the UPF's performance in certain scenarios
at high speeds.  This allows UPF features to be verified that they
perform as expected in an environment more representative of *production
level*. Traffic is generated using the [TRex Python
API](https://github.com/cisco-system-traffic-generator/trex-core/blob/master/doc/trex_cookbook.asciidoc).
    > *Example*: a line rate test could assert the baseline throughput,
    latency, etc. of the UPF is as expected when handling high-speed
    downlink traffic from 10,000 unique UEs.

## Workflow
Tests require two separate machines to run, since both TRex and UPF
use DPDK. Currently, the test workflow is as such:

![Test](docs/test-run.svg)

In **step 1**, rules are installed onto the UPF instance by the test
framework via BESS gRPC messages.

In **step 2**, TRex or Scapy (depending on the type of test case)
generates traffic to the UPF across NICs and physical links.

In **step 3**, traffic routes through the UPF and back to the machine
hosting TRex, where results are asserted.

## Steps to run tests
The run script assumes that the TRex daemon server and the UPF
instance are already running on their respective machines. It also
assumes that all config files in `config/` are configured correctly to
route traffic to the UPF.

To install TRex onto your server, please refer to the [TRex installation
guide](https://trex-tgn.cisco.com/trex/doc/trex_manual.html#_download_and_installation)

### Steps
1. Generate BESS Python protobuf files for gRPC library and PTF
Dockerfile image build dependencies:
```bash
make build
```
2. Run PTF tests using the `run_tests` script:
```bash
./run_tests -t [test-dir] [optional: filename/filename.test_case]
```
### Examples
To run all test cases in the `unary/` directory:
```bash
./run_tests -t tests/unary
```
To run a specific test case:
```bash
./run_tests -t tests/linerate/ baseline.DownlinkPerformanceBaselineTest
```
