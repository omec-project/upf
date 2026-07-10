# BESS Integration Plan

## Summary

Absorb `omec-project/bess` into `omec-project/upf` and make UPF the canonical
source for BESS-UPF datapath development. The migration should import BESS as a
squashed subtree with explicit source-SHA provenance, remove the external
`bess_build` image dependency, keep the existing UPF build interface, and
prepare the separate BESS repository for retirement after UPF CI and releases
are proven.

## Current Build Context

- UPF's root `Dockerfile` currently treats BESS as a prebuilt supplier stage:
  `FROM ghcr.io/omec-project/bess_build:260603@sha256:... AS bess-build`.
  UPF does not compile BESS source today.
- UPF's `bess` image copies BESS artifacts from that supplier stage, then adds
  UPF-specific BESS pipeline configuration from `conf/`.
- UPF's `Makefile` drives Docker builds through `DOCKER_TARGETS`, defaulting to
  `bess pfcp`, and already passes `MAKEFLAGS` and `CPU` as Docker build args.
- UPF's `pb` and `ptf-pb` Docker targets generate Go and Python protobuf
  bindings from BESS protobuf sources supplied by the current `bess-build`
  stage.
- BESS's own image build is in `env/Dockerfile`. It installs build dependencies
  with Ansible, compiles BESS with `./build.py bess`, stages runtime artifacts,
  and exports a runtime image that matches the artifacts UPF currently consumes.
- BESS's `core/Makefile` applies the architecture choice through
  `-march=$(CPU)`, so UPF's existing `CPU` build arg can become the unified
  architecture-selection interface after BESS is imported.
- BESS CI currently includes source-level validation that UPF does not run:
  BESS C++ build, kmod build, `core/all_test`, Python unittest discovery, module
  tests, clang-format checks, and BESS Dockerfile linting.
- Python dependency maintenance is currently split across both repos and several
  Docker stages. UPF pins `grpcio==1.80.0` and `protobuf==6.33.6` in
  `requirements.txt` and `requirements_pb.txt`, PTF pins `protobuf==7.34.1`,
  while BESS pins `grpcio==1.81.1` and `protobuf==7.35.1` in its runtime and
  development requirement files.
- Runtime apt package ownership is also split. UPF manually installs BESS
  runtime packages in the root `Dockerfile`, while BESS maintains
  `env/runtime-deps.yml` for the runtime image it publishes today.
- DPDK runtime library and PMD plugin setup is duplicated. BESS stages runtime
  libraries and `/opt/bess/lib/dpdk-pmds`; UPF then has additional PMD symlink
  logic in its final `bess` stage.

## Key Changes

- Import BESS into UPF under `bess/` using a squashed subtree:

  ```bash
  git remote add bess https://github.com/omec-project/bess.git
  git fetch bess
  git subtree add --prefix=bess bess main --squash
  ```

  Record the exact imported BESS source commit in the import commit and plan.
  The initial import snapshot is
  `ac3763d659c5e63ed52c08c1f6311f63b31ac776`. The old BESS repository remains
  the historical reference for pre-integration BESS development.

- Refactor the root UPF `Dockerfile` so the `bess-build` stage builds from the
  in-repo `bess/` source instead of
  `ghcr.io/omec-project/bess_build`.
- Adapt the build stages from BESS's `env/Dockerfile` into the root UPF
  `Dockerfile`, changing BESS-relative `COPY` paths to use the imported
  `bess/` directory while keeping UPF as the top-level Docker build context.
- Preserve existing artifact paths consumed by UPF image stages:
  `/bin/bessd`, `/bin/modules`, `/opt/bess`, `/protobuf`,
  `/bess/protobuf`, protobuf includes, DPDK libraries, and the PMD plugin
  directory.
- Keep UPF's protobuf-generation targets wired to the in-repo BESS build stage
  so `make pb`, `make py-pb`, and `ptf/Makefile` keep producing the same
  generated bindings.
- Consolidate Python dependency ownership after BESS is imported:
  - Use one shared runtime requirements file for the final `upf-bess` image
    instead of maintaining separate UPF and BESS runtime pins for overlapping
    packages.
  - Use one shared protobuf-generation requirements file for Python protobuf
    generation, aligned with the runtime `grpcio`/`protobuf` versions unless a
    documented tooling constraint requires a different version.
  - Keep PTF-only traffic-generator dependencies separate, but align overlapping
    `grpcio`, `protobuf`, `scapy`, `psutil`, and `typing-extensions` pins where
    compatible.
  - Preserve `--require-hashes` installs; regenerate hashes once per consolidated
    file instead of independently across UPF and BESS files.
- Consolidate runtime apt dependency ownership:
  - Use BESS's imported runtime dependency definition as the source of truth for
    BESS runtime packages.
  - Remove duplicate manual BESS runtime package lists from the UPF runtime
    stage where they overlap with the BESS runtime dependency definition.
  - Keep UPF-only runtime packages explicitly separate so future dependency
    updates have a clear owner.
- Consolidate DPDK runtime staging:
  - Let the in-repo BESS build stage produce the DPDK shared-library staging
    directory and PMD plugin directory.
  - Make the final `upf-bess` image copy those staged artifacts instead of
    reconstructing equivalent PMD symlinks in a second place.
  - Preserve UPF's required PMD coverage for AF_PACKET, AF_XDP, PCI, vdev, and
    supported Intel NIC drivers, and document any intentionally excluded PMDs.
- Simplify protobuf ownership:
  - Treat imported BESS `protobuf/` as the single source for BESS control-plane
    protobuf definitions.
  - Keep separate generated outputs only where consumers require different
    languages or package layouts: Go bindings for `pfcpiface`, Python bindings
    for PTF, and BESS's own `pybess` bindings.
  - Align generation tool versions with the consolidated Python dependency
    policy and document any required exception.
- Keep UPF's existing `CPU ?= native` and `--build-arg CPU=$(CPU)` interface as
  the architecture-selection mechanism for BESS builds.
- Define and document the supported `CPU` values for project-built images,
  including `native`, `haswell`, and `ivybridge`, while still allowing advanced
  users to pass any compiler-supported `-march` value for local experiments.
- Update developer documentation to remove the old separate-repo workflow and
  document the new single-checkout workflow:

  ```bash
  CPU=native make docker-build
  CPU=haswell make docker-build
  CPU=ivybridge make docker-build
  ```

- Merge relevant BESS CI obligations into UPF CI: in-repo BESS source build,
  kmod build where runner support permits, `core/all_test`, Python unittest
  discovery, BESS module tests, BESS Dockerfile/build validation, clang-format
  checks for BESS C++/protobuf files, license checks, FOSSA/SBOM coverage, and
  image build checks.
- Add path filtering for expensive BESS source checks and Docker image builds so
  documentation-only or unrelated UPF changes do not rebuild the full BESS
  toolchain unnecessarily.
- Update `.dockerignore` so local reference checkouts such as `repos/` are not
  accidentally sent to Docker builds, while the canonical imported `bess/`
  directory remains included.
- Retire the old BESS repository after UPF builds and releases no longer depend
  on it: archive/freeze the repo, update its README, redirect issue/PR guidance,
  and stop publishing or consuming standalone `bess_build`.

## Public Interfaces

- Keep `make docker-build`, `DOCKER_TARGETS`, `DOCKER_BUILD_ARGS`, and `CPU` as
  the public build interface.
- Do not require developers to run `git submodule` commands or clone BESS
  separately.
- Treat BESS code as normal UPF source under `bess/`; BESS changes go through
  UPF PRs and UPF CI.
- Sign all integration commits with `git commit -s` so the branch satisfies
  UPF's DCO/sign-off requirements.

## Implementation Milestones

### Milestone 1: Source Import and Build Context Hygiene

- Import BESS under `bess/` as a squashed subtree.
- Record the exact imported BESS source commit SHA so maintainers can trace the
  initial source snapshot back to the old BESS repository.
- Update `.dockerignore` to exclude local reference checkouts such as `repos/`
  without excluding the canonical imported `bess/` directory.
- Exit criteria: `git log -- bess/core` shows the squashed BESS import, UPF
  status is clean except intended changes, and Docker build context no longer
  includes local reference repositories.

### Milestone 2: In-Repo BESS Docker Build Prototype

- Adapt BESS's `env/Dockerfile` builder and `bess-build` stages into the root
  UPF `Dockerfile`.
- Change BESS-relative `COPY` paths to use `bess/` while preserving UPF as the
  top-level Docker build context.
- Keep the existing BESS artifact contract: `/bin/bessd`, `/bin/modules`,
  `/opt/bess`, `/protobuf`, `/bess/protobuf`, protobuf includes, DPDK runtime
  libraries, and `/opt/bess/lib/dpdk-pmds`.
- Exit criteria: `DOCKER_TARGETS=bess CPU=haswell make docker-build` reaches
  the in-repo BESS build path and either succeeds or leaves a documented,
  isolated blocker.

### Milestone 3: Protobuf and Dependency Consolidation

- Align BESS protobuf ownership around imported `bess/protobuf/`.
- Preserve generated outputs for Go `pfcpiface`, PTF Python tests, and BESS
  `pybess`.
- Consolidate overlapping Python requirement files and runtime apt package
  ownership where compatible.
- Consolidate DPDK runtime library and PMD staging into the BESS-owned build
  stage, with UPF packaging copying staged artifacts.
- Exit criteria: `make pb`, `make py-pb`, PTF image build, and `upf-bess` image
  Python imports work with the selected dependency set.

### Milestone 4: CI and Validation Integration

- Bring BESS source-build checks into UPF CI: BESS C++ build, `core/all_test`,
  Python unittest discovery, module tests, clang-format, and Dockerfile linting.
- Gate expensive BESS checks and image builds with path filters where practical.
- Keep unsupported checks such as kernel-module build gated to suitable Linux
  runners.
- Exit criteria: PR CI validates both existing UPF behavior and imported BESS
  source behavior without unnecessary full rebuilds for unrelated changes.

### Milestone 5: Documentation and Repository Retirement

- Replace old BESS development docs that require cloning BESS separately,
  building `bess_build`, and editing UPF's Dockerfile.
- Document the single-checkout workflow, supported `CPU` values, and how to
  run tests using the new layout.
- Prepare retirement guidance for the old BESS repository and standalone
  `bess_build` publishing path.
- Exit criteria: developers can follow UPF docs to build and test the integrated
  source tree without external BESS repo steps.

## Test Plan

- Verify source import:

  ```bash
  git log -- bess/core
  git status
  ```

- Verify image builds:

  ```bash
  DOCKER_TARGETS=bess CPU=native make docker-build
  DOCKER_TARGETS=pfcp make docker-build
  make docker-build
  ```

- Verify architecture selection:

  ```bash
  DOCKER_TARGETS=bess CPU=haswell make docker-build
  DOCKER_TARGETS=bess CPU=ivybridge make docker-build
  ```

- Verify protobuf targets still work:

  ```bash
  make pb
  make py-pb
  ```

- Verify consolidated Python dependency installs in all Docker stages that use
  Python requirements: `bess`, `py-pb`, and PTF image build.
- Verify consolidated runtime apt dependencies by checking the final `upf-bess`
  image still contains required runtime libraries and tools without reinstalling
  duplicate BESS package lists in multiple stages.
- Verify consolidated DPDK staging by checking `/usr/local/lib/x86_64-linux-gnu`
  and `/opt/bess/lib/dpdk-pmds` in the final image and confirming required PMDs
  are present.
- Verify generated Python protobuf code imports under the installed runtime:

  ```bash
  python3 -c "import bess_msg_pb2, service_pb2_grpc"
  ```

- Verify generated protobuf consumers still build:

  ```bash
  cd ptf && make build
  ```

- Verify imported BESS source-level checks in CI or on a Linux runner:

  ```bash
  cd bess
  ./build.py bess
  ./build.py kmod
  cd core && ./all_test
  ```

- Verify existing Go behavior:

  ```bash
  make test
  make test-integration
  ```

- Verify runtime image compatibility by confirming the final `upf-bess` image
  contains `bessd`, `bessctl`, BESS modules, protobufs, DPDK libraries, and the
  PMD plugin directory.

## Assumptions

- Use a history-preserving `git subtree` import, not a squash import or
  submodule.
- Import BESS under `bess/`.
- Preserve current UPF image names and targets: `upf-bess` and `upf-pfcp`.
- Keep the current root `Dockerfile` as the primary image build entrypoint.
- Keep the old BESS repo available during migration, then archive it once UPF
  CI and releases are proven.
- Treat `repos/bess` as local reference material only; it is not part of the
  final integrated source layout.
- Prefer aligning on BESS's newer `grpcio`/`protobuf` pins unless UPF or PTF
  tests expose a compatibility issue.
- Prefer moving duplicated build logic into BESS-owned imported definitions
  first, then layering UPF-specific packaging on top, rather than reimplementing
  BESS runtime setup in the UPF stages.
