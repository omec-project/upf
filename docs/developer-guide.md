<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022 Open Networking Foundation
-->
# Developer guide

## Testing local Go dependencies

The `upf-epc` repository relies on some external Go dependencies, which are not mature yet (e.g. pfcpsim or p4runtime-go-client).
It's often needed to extend those dependencies first, before adding a new feature to the PFCP Agent. However, when using Go modules and Dockerized environment,
it's hard to test WIP changes to local dependencies. Therefore, this repository come up with a way to use Go vendoring, instead of Go modules, for development purposes.

To use a local Go dependency add the `replace` directive to `go.mod`. An example:

```
replace github.com/antoninbas/p4runtime-go-client v0.0.0-20211006214122-ea704d54a7d3 => ../p4runtime-go-client
```

Then, to build the Docker image using the local dependency:

```
DOCKER_BUILD_ARGS="--build-arg GOFLAGS=-mod=vendor" make docker-build
```

To run E2E integration tests with the local dependency:

```
DOCKER_BUILD_ARGS="--build-arg GOFLAGS=-mod=vendor" make test-up4-integration 
```
