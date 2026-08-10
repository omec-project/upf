<!--
SPDX-FileCopyrightText: 2026 Intel Corporation
SPDX-License-Identifier: Apache-2.0
-->
# BESS Integration Plan

## Summary

Absorb `omec-project/bess` into `omec-project/upf` and make UPF the canonical
source for BESS-UPF datapath development. The migration should import BESS as a
squashed subtree with explicit source-SHA provenance, remove the external
`bess_build` image dependency, keep the existing UPF build interface, and
prepare the separate BESS repository for retirement after UPF CI and releases
are proven.

## Build Context and Progress

- Before this branch, UPF's root `Dockerfile` treated BESS as a prebuilt
  supplier stage: `FROM ghcr.io/omec-project/bess_build:... AS bess-build`.
- On the integration branch, the root `Dockerfile` now builds BESS from the
  imported `bess/` source using the builder and `bess-build` stages adapted
  from BESS's `env/Dockerfile`.
- UPF's `bess` image copies artifacts from the in-repo BESS build stage, then
  adds UPF-specific BESS pipeline configuration from `conf/`.
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
- The final `upf-bess` image now pins its overlapping Python runtime packages
  to BESS's tested versions: `grpcio==1.83.0`, `protobuf==7.35.1`, and
  `typing-extensions==4.16.0`. Python protobuf generation remains a separate
  toolchain (`grpcio-tools==1.82.1`, `grpcio==1.82.1`, and
  `protobuf==7.35.1`), and PTF retains its independent test-runtime lock.
- BESS's imported `env/runtime-deps.yml` is now the source of truth for BESS
  runtime apt packages in the final image. UPF-only packages remain explicit
  in the root `Dockerfile`.
- The BESS build stage now stages DPDK runtime libraries and PMD plugins; the
  final image copies those staged artifacts without recreating PMD symlinks.

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
  - Keep root `requirements.txt` as the single requirements file installed in
    the final `upf-bess` image, and align overlapping BESS runtime pins and
    hashes with `bess/env/requirements-run.txt`.
  - Keep `requirements_pb.txt` as a separate, locked Python protobuf-generation
    toolchain because `grpcio-tools` has its own protobuf compatibility
    constraints. Upgrade its `grpcio`, `grpcio-tools`, and `protobuf` pins as
    a tested set rather than forcing them to match the runtime image.
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
- Add `DOCKER_EXTRA_BUILD_ARGS` for optional Docker flags such as `--no-cache`
  without replacing the default `DOCKER_BUILD_ARGS` values that pass `CPU` and
  parallel `MAKEFLAGS`.
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

- Keep `make docker-build`, `DOCKER_TARGETS`, `DOCKER_BUILD_ARGS`,
  `DOCKER_EXTRA_BUILD_ARGS`, and `CPU` as the public build interface.
- Do not require developers to run `git submodule` commands or clone BESS
  separately.
- Treat BESS code as normal UPF source under `bess/`; BESS changes go through
  UPF PRs and UPF CI.
- Sign all integration commits with `git commit -s` so the branch satisfies
  UPF's DCO/sign-off requirements.

## Implementation Milestones

### Milestone 1: Source Import and Build Context Hygiene

Status: complete. BESS was imported as a squashed subtree from
`ac3763d659c5e63ed52c08c1f6311f63b31ac776`; `.dockerignore` excludes the
local `repos/` reference checkout and preserves the canonical `bess/` tree.

- Import BESS under `bess/` as a squashed subtree.
- Record the exact imported BESS source commit SHA so maintainers can trace the
  initial source snapshot back to the old BESS repository.
- Update `.dockerignore` to exclude local reference checkouts such as `repos/`
  without excluding the canonical imported `bess/` directory.
- Exit criteria: `git log -- bess/core` shows the squashed BESS import, UPF
  status is clean except intended changes, and Docker build context no longer
  includes local reference repositories.

### Milestone 2: In-Repo BESS Docker Build Prototype

Status: complete. The root Dockerfile builds BESS from `bess/`; x86_64 builds
validated `CPU=haswell` and `CPU=ivybridge`, along with the expected runtime
artifact paths.

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

Status: complete. Closeout validation passed `make pb`, `make py-pb`, the PTF
image build, and the `upf-bess` Python/runtime smoke checks with the selected
dependency set. The full two-host PTF/TRex traffic run remains deferred
validation, not a milestone blocker. Completed substeps:

- BESS protobuf exports are staged at `/protobuf` and copied to legacy
  destinations only where consumers require them.
- DPDK library and PMD staging is owned by `bess-build`; the final image copies
  the prepared library and PMD directories.
- BESS runtime apt packages are installed from `bess/env/runtime-deps.yml`,
  while UPF-only packages remain explicit in the root Dockerfile.
- Final-image Python runtime pins for `grpcio`, `protobuf`, and
  `typing-extensions` match BESS's runtime lockfile. The refreshed BESS
  snapshot's gRPC 1.83.0 and protobuf 7.35.1 runtime was validated during
  milestone closeout.
- The Python protobuf generator remains intentionally separate pending a
  dedicated generator-toolchain upgrade and validation.
- PTF dependencies now use `/opt/ptf-venv`, removing global Python installs
  and `--ignore-installed`; its x86_64 Docker image build passes.
- The normal locked PTF and TRex dependency installs pass `pip check` before
  TRex's custom Scapy is installed. TRex then intentionally replaces Scapy
  2.7.0 with its bundled 2.4.5 distribution. PTF 0.12.0 declares
  `scapy>=2.5.0`, so a final `pip check` reports a known conflict. A baseline
  build of the original image confirms the same Scapy 2.4.5 and failing
  `pip check`; this is pre-existing behavior, not a result of the integration.
  The Dockerfile documents the exception and validates the PTF and Scapy
  imports plus the installed Scapy distribution version. The built image's
  runtime smoke test passed with PTF 0.12.0 and Scapy 2.4.5. Full two-host
  PTF/TRex validation remains pending.
- Audit and remove `--ignore-installed` from final-image and generated-artifact
  Python installs. Use isolated Python environments or explicit staging paths
  so Ubuntu, BESS build, runtime, protobuf-generation, and PTF dependencies do
  not silently override one another. Any unavoidable exception must document
  the conflicting distribution, why isolation is impractical, and its
  validation coverage.

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

Status: complete. PR #1 CI run `30931083624` passed the UPF and PTF image
builds, existing UPF tests, the path-filtered BESS source build and tests, and
all four BESS clang-format scopes. The root Dockerfile remains covered by
Hadolint; the duplicate standalone BESS Dockerfile is intentionally deferred
to the Milestone 5 retirement work.

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
- Retire the duplicate standalone BESS image pipeline after the root Dockerfile
  source build and UPF CI have proven its replacement:
  - Remove `bess/env/Dockerfile` and `bess/env/rebuild_images.py`; the root
    `Dockerfile` is the only supported BESS image build path.
  - Remove the standalone BESS image publication workflow
    `bess/.github/workflows/push.yml`, plus any now-inert nested BESS workflow
    configuration that is not migrated to a root UPF workflow.
  - Update documentation and test helpers that name `bess_build`, including
    `docs/developer-guide.md` and
    `bess/bessctl/conf/port/vhost/launch_container.py`, to use or configure
    the canonical `upf-bess` image instead.
  - Retain `bess/env/ci.yml`, BESS dependency playbooks and requirements, and
    BESS source tests because the UPF direct-source CI job uses them.
- Before deletion, verify that `make docker-build`, BESS direct-source CI,
  `make pb`, `make py-pb`, and the UPF runtime image validation pass without
  the standalone pipeline. Search the repository for remaining `bess_build`
  references and either remove each one or document its temporary purpose.
- After UPF releases no longer consume or publish `bess_build`, archive/freeze
  the external BESS repository, update its README with the UPF contribution
  location, redirect issue and PR guidance, and stop standalone image releases.
- Exit criteria: developers can follow UPF docs to build and test the integrated
  source tree without external BESS repo steps or a standalone `bess_build`
  image. No live UPF build, CI workflow, documentation, or test helper depends
  on the retired pipeline.

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
  Python requirements: `bess`, `py-pb`, and PTF image build. The final `bess`
  image has been validated with BESS's aligned `grpcio` and `protobuf` pins;
  `py-pb` generated no binding diffs and the PTF image build passes. Complete
  the PTF/TRex functional testbed run before closing this validation item.
- Verify each isolated Python environment with `pip check`, its expected
  package versions, and the imports used by that stage. Do not rely on
  `--ignore-installed` to hide an Ubuntu package-version discrepancy.
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

- Use a squashed `git subtree` import with the source commit SHA recorded in
  the import history, not a full-history import or submodule.
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
