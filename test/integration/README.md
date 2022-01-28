<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022 Open Networking Foundation
-->
# E2E integration tests

The tests defined in this directory implement the so-called "broad integration tests" 
(they are sometimes called system tests or E2E tests, see [Martin Fowler's blog](https://martinfowler.com/bliki/IntegrationTest.html)).

The purpose of E2E integration tests is to verify the behavior of the PFCP Agent with different flavors of PFCP messages, 
as well as to check PFCP Agent's integration with data plane components (UP4, BESS-UPF). In detail, these tests verify if 
PFCP messages are handled as expected by the PFCP Agent, and if the PFCP Agent installs correct packet forwarding rules onto the fast-path target (UP4/BESS). 

## Structure 

- `infra/`: contains build and deployment files.
- `config/`: contains app-specific config (e.g., `upf.json`).
- the current directory contains `*_test.go` files defining test scenarios.

## Overview

The E2E integration tests are integrated within the Go test framework and can be run by `go test`. 

The tests use `docker-compose` to set up `pfcpiface` and `mock-up4` (the BMv2 container running the UP4 pipeline) images.
Then, a given test case generates PFCP messages towards `pfcpiface` and fetches the runtime forwarding configuration from the
data plane component (e.g., via P4Runtime for UP4) to verify forwarding state configuration. 

## Run tests 

To run all E2E integration tests invoke the command below from the root directory:

```bash
make test-up4-integration
```