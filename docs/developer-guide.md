<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022 Open Networking Foundation
-->
# Developer guide

## New Features or Improvements to the BESS pipeline

When implementing new features or making improvements to the `BESS` pipeline,
the easiest way to do so is by:

- Clone the `bess` repository and make your changes
  ```bash
  $ cd <path/to/upf>/..
  $ git clone https://github.com/<your-user>/bess.git
  $ cd bess
  # make your modifications
  ```

- Rebuild the `bess_build` image locally from the `bess` repository
  ```bash
  $ cd <path/to/bess>
  $ yes N | ./env/rebuild_images.py jammy64
  ```

- Update the `FROM` line in `Dockerfile` to use the locally-built image
  ```diff
  -FROM ghcr.io/omec-project/bess_build:260223@sha256:... AS bess-build
  +FROM ghcr.io/omec-project/bess_build:latest AS bess-build
  ```

- Build the UPF Docker image
  ```bash
  $ cd <path/to/upf>
  $ DOCKER_PULL="" make docker-build
  ```

- Test the modifications

- Commit your changes to `bess` repository and, if needed, `upf` repository
- Open pull request in `bess` repository and, if needed, `upf` repository


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
