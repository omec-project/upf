<!--
SPDX-License-Identifier: Apache-2.0
Copyright 2022 Open Networking Foundation
-->
# Developer guide

## New Features or Improvements to the BESS pipeline

When implementing new features or making improvements to the `BESS` pipeline,
the easiest way to do so is by:

- Clone the `bess` repository inside the UPF repository
```bash
$ cd <path/to/upf>
$ git clone https://github.com/<your-user>/bess.git
```

- **Temporarily** modify Dockerfile to use the `bess` cloned in the previous
step
```diff
diff --git a/Dockerfile b/Dockerfile
index 052456d..03b7d33 100644
--- a/Dockerfile
+++ b/Dockerfile
@@ -11,9 +11,7 @@ RUN apt-get update && \

 # BESS pre-reqs
 WORKDIR /bess
-ARG BESS_COMMIT=dpdk-2011-focal
-RUN git clone https://github.com/omec-project/bess.git .
-RUN git checkout ${BESS_COMMIT}
+COPY bess/ .
 RUN cp -a protobuf /protobuf

 # Stage bess-build: builds bess with its dependencies
```

- Implement a feature or make modifications

- Test the modifications

- Revert change in Dockerfile
```diff
diff --git a/Dockerfile b/Dockerfile
index 03b7d33..052456d 100644
--- a/Dockerfile
+++ b/Dockerfile
@@ -11,7 +11,9 @@ RUN apt-get update && \

 # BESS pre-reqs
 WORKDIR /bess
-COPY bess/ .
+ARG BESS_COMMIT=dpdk-2011-focal
+RUN git clone https://github.com/omec-project/bess.git .
+RUN git checkout ${BESS_COMMIT}
 RUN cp -a protobuf /protobuf

 # Stage bess-build: builds bess with its dependencies
```

- Commit your changes to `bess` repository and, if needed, `upf` repository
- Open pull request in `bess` repository and, if needed, `upf` repository


## Testing local Go dependencies

The `upf` repository relies on some external Go dependencies, which are not
mature yet (e.g. pfcpsim or p4runtime-go-client).
It's often needed to extend those dependencies first, before adding a new
feature to the PFCP Agent. However, when using Go modules and containerized
environment, it's hard to test work-in-progress (WIP) changes to local
dependencies. Therefore, this repository comes up with a way to use Go
vendoring, instead of Go modules, for development purposes.

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
