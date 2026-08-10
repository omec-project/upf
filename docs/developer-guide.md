<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022 Open Networking Foundation
-->
# Developer guide

## New Features or Improvements to the BESS pipeline

The BESS source used by UPF is part of this repository under `bess/`. Make BESS
pipeline changes in that directory and submit them through the UPF review and
CI process.

The root `Dockerfile` compiles the source in `bess/` and produces the `upf-bess`
image. It is no longer necessary to clone BESS separately and build a standalone
`bess_build` image.

Build both UPF images with the default architecture selection:

```bash
CPU=native make docker-build
```

To build only the BESS-based datapath image, or to select a deployment CPU
architecture, use `DOCKER_TARGETS` and `CPU`:

```bash
DOCKER_TARGETS=bess CPU=haswell make docker-build
DOCKER_TARGETS=bess CPU=ivybridge make docker-build
```

The default `CPU=native` is intended for development. Select a named CPU only
when the target deployment requires it.

For changes that affect BESS protobuf definitions, regenerate both consumers:

```bash
make pb
make py-pb
```

Run the UPF test suites as appropriate:

```bash
make test
make test-integration
```

UPF CI runs the BESS source build, BESS tests, and BESS formatting checks when
files under `bess/` change.


## Testing local Go dependencies

The `upf` repository relies on some external Go dependencies, which are not
mature yet (e.g. pfcpsim).
It's often needed to extend those dependencies first, before adding a new
feature to the PFCP Agent. However, when using Go modules and containerized
environment, it's hard to test work-in-progress (WIP) changes to local
dependencies. Therefore, this repository comes up with a way to use Go
vendoring, instead of Go modules, for development purposes.

To use a local Go dependency add the `replace` directive to `go.mod`. An example:

```
replace github.com/omec-project/pfcpsim v1.3.1 => ../pfcpsim
```

Then, to build the Docker image using the local dependency:

```
DOCKER_BUILD_ARGS="--build-arg GOFLAGS=-mod=vendor" make docker-build
```

To run E2E integration tests with the local dependency:

```
DOCKER_BUILD_ARGS="--build-arg GOFLAGS=-mod=vendor" make test-integration
```
