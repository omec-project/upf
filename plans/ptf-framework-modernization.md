# PTF/TRex Python dependency modernization (#1220)

## Objective

Replace the fragile PTF/TRex Python dependency arrangement with a reproducible,
fully resolvable stack.  The completed image must use one tested Scapy version,
allow routine Dependabot Python updates, and pass the PTF/TRex validation
required by [issue #1220](https://github.com/omec-project/upf/issues/1220).

## Background and current state

- The PTF requirements pin `scapy==2.7.0` and `scapy-helper==0.14.8`.
- `scapy-helper==0.14.8` requires `pyperclip==1.8.2`, blocking Dependabot's
  newer `pyperclip` updates.
- The PTF Docker image downloads the Stratum-derived TRex
  `2.92-scapy-2.4.5`, makes its external libraries available on `PYTHONPATH`,
  and then replaces the installed Scapy distribution with its bundled Scapy
  2.4.5. This creates a mixed Scapy environment.
- Issue #1163 records the resulting import failure on Python 3.14 and is
  explicitly superseded by #1220.

## Definition of done

- [ ] PTF/TRex image builds with a fully resolvable, hash-pinned dependency set.
- [ ] The final runtime environment passes `pip check`.
- [ ] PTF and TRex import one documented Scapy version from one installed
      distribution; no second Scapy package is exposed through `PYTHONPATH`.
- [ ] `scapy-helper`/`pyperclip` does not create an undocumented resolver
      conflict.
- [ ] Image smoke checks, PTF smoke tests, and the two-host PTF/TRex traffic
      validation pass.
- [ ] The source/version choices and dependency-regeneration procedure are
      documented.
- [ ] A current Dependabot-style Python dependency update resolves and builds.

## Work checklist

### 1. Capture a reproducible baseline

- [ ] Build the current PTF image from `ptf/`.
- [ ] Record `pip list`, `pip check`, and the installed Scapy distribution
      before and after the Dockerfile's custom-Scapy installation.
- [ ] Record import paths and versions for `ptf`, `scapy`, `trex.stl.api`, and
      `trex_stf_lib`.
- [x] Run `make dependency-smoke` and capture the current result. The
      [2026-09-23 GitHub Actions run](https://github.com/andybavier/upf/actions/runs/35913958681)
      built successfully on x86_64 and failed as expected at final `pip check`:
      PTF 0.12.0 requires Scapy >=2.5.0, while the image installs 2.4.5.
- [ ] Confirm the exact resolver failure from Dependabot PR #1306 (or an
      equivalent local resolution) for `scapy-helper`/`pyperclip`.

### 2. Select a supported TRex source

- [ ] Inventory the UPF code's use of TRex APIs in `ptf/lib/` and
      `ptf/tests/`.
- [ ] Evaluate a current upstream Cisco TRex release, a maintained compatible
      fork, and a narrowly maintained patch of the present source.
- [ ] For each candidate, assess Python 3.14 compatibility, compatible Scapy
      versions, API compatibility, release/provenance pinning, and packaging
      needs.
- [ ] Decide the source and exact revision/version in a short design note or
      PR description; obtain maintainer agreement before the migration.
- [ ] Record the selected source's checksum/revision and license/provenance.

### 3. Remove the mixed-Scapy design

- [ ] Update `ptf/Dockerfile` to fetch the selected pinned TRex source.
- [ ] Copy only the required TRex client modules into the runtime image.
- [ ] Stop copying or exposing any bundled `scapy` package from TRex through
      `PYTHONPATH`.
- [ ] Remove the custom Scapy wheel build/install override from the Dockerfile.
- [ ] Install exactly one Scapy distribution that satisfies both PTF and the
      selected TRex client.
- [ ] Verify all Scapy imports resolve to the same installed distribution.

### 4. Modernize and lock Python dependencies

- [ ] Upgrade or replace `scapy-helper==0.14.8`.
- [ ] If no upstream release is viable, create an explicitly pinned and
      documented maintained patch/fork; do not suppress pip resolver errors.
- [ ] Regenerate `ptf/requirements-ptf.txt` and `ptf/requirements-trex.txt`
      together, including hashes.
- [ ] Add the source manifests and/or documented regeneration command needed
      to reproduce the locked requirements.
- [ ] Confirm a clean environment resolves both requirement files without
      `--ignore-installed`, `--no-deps`, or post-install package replacement.

### 5. Make the result continuously verifiable

- [ ] Run `pip check` after all final runtime packages are installed.
- [x] Add a container smoke check for the documented Scapy version and import
      origins of `ptf`, `scapy`, `trex.stl.api`, and `trex_stf_lib`.
      (`make dependency-smoke`; it will become required CI once the legacy
      conflict is removed.)
- [ ] Ensure the existing PR image-build workflow runs those checks.
- [ ] Add focused automated tests where the selected TRex API requires UPF
      compatibility changes.

### 6. Validate end-to-end behavior

- [ ] Run unary PTF tests in the image.
- [ ] Run a representative linerate/baseline PTF test against a two-host
      UPF/TRex testbed.
- [ ] Run representative uplink/downlink, QER, and MBR traffic coverage.
- [ ] Capture commands, image digest, TRex version, Scapy version, testbed
      details, and results in the PR or linked test report.
- [ ] Recreate or rebase a Dependabot-style Python dependency update and
      verify it resolves and builds.

### 7. Document and close out

- [ ] Update `ptf/README.md` with the selected TRex and Scapy versions,
      supported environment, and validation instructions.
- [ ] Document any intentional dependency exception with its owner and removal
      path.
- [ ] Link the implementation PR(s) and validation evidence to #1220.
- [ ] Close #1163 as resolved by #1220 once the mixed-Scapy failure is fixed.
- [ ] Close #1220 only after every definition-of-done item is complete.

## Proposed delivery sequence

1. Baseline plus TRex-source evaluation/decision.
2. Dockerfile and dependency-stack migration.
3. CI smoke checks and documentation.
4. Two-host validation and Dependabot update verification.

The source-selection step is intentionally a decision gate: it determines the
compatible Scapy version and whether an UPF-side TRex API adaptation is needed.
