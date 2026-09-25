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
- The production PTF Docker image now uses pinned Cisco TRex v3.06 with the
  tracked UPF compatibility patches, copies only required client libraries,
  and uses the PTF venv's Scapy 2.7.0.
- The prior Stratum-derived `2.92-scapy-2.4.5` image replaced the installed
  Scapy distribution with bundled Scapy 2.4.5, creating the mixed-Scapy
  environment captured in the baseline. Issue #1163 records that historical
  Python 3.14 import failure and is explicitly superseded by #1220.

## Definition of done

- [x] PTF/TRex image builds with a fully resolvable, hash-pinned dependency set.
      ([production validation, 2026-09-25](https://github.com/andybavier/upf/actions/runs/36168175496))
- [x] The final runtime environment passes `pip check`.
      ([production validation, 2026-09-25](https://github.com/andybavier/upf/actions/runs/36168175496))
- [x] PTF and TRex import one documented Scapy version from one installed
      distribution; no second Scapy package is exposed through `PYTHONPATH`.
      The production image imports Scapy 2.7.0 from the PTF venv.
- [ ] `scapy-helper`/`pyperclip` does not create an undocumented resolver
      conflict.
- [ ] Image smoke checks, PTF smoke tests, and the two-host PTF/TRex traffic
      validation pass.
- [ ] The source/version choices and dependency-regeneration procedure are
      documented.
- [ ] A current Dependabot-style Python dependency update resolves and builds.

## Work checklist

### 1. Capture a reproducible baseline

- [x] Build the current PTF image from `ptf/` on an x86_64 GitHub runner.
      ([baseline run, 2026-09-24](https://github.com/andybavier/upf/actions/runs/36065717421))
- [x] Record `pip list`, `pip check`, and the installed Scapy distribution
      before and after the Dockerfile's custom-Scapy installation. Pre-override
      Scapy 2.7.0 passes `pip check`; final Scapy 2.4.5 fails because PTF
      requires Scapy >=2.5.0.
- [x] Record import paths and versions for `ptf`, `scapy`, `trex.stl.api`, and
      `trex_stf_lib`. Both stages import packages from the venv; the pre-override
      TRex STL imports fail on `get_if`, while final Scapy cannot import GTP or
      the TRex STL API because `scapy.modules.six.moves` is absent.
- [x] Run `make dependency-smoke` and capture the current result. The
      [2026-09-23 GitHub Actions run](https://github.com/andybavier/upf/actions/runs/35913958681)
      built successfully on x86_64 and failed as expected at final `pip check`:
      PTF 0.12.0 requires Scapy >=2.5.0, while the image installs 2.4.5.
- [x] Confirm the exact resolver failure from Dependabot PR #1306 (or an
      equivalent local resolution) for `scapy-helper`/`pyperclip`.
      `scapy-helper==0.14.8` requires `pyperclip==1.8.2`, conflicting with
      `pyperclip==1.11.0` (`ResolutionImpossible`).

### 2. Select a supported TRex source

- [x] Inventory the UPF code's use of TRex APIs in `ptf/lib/` and
      `ptf/tests/`. UPF uses the stateless `STLClient`, stream/profile types,
      and the legacy daemon-management `CTRexClient`.
- [x] Evaluate the Cisco TRex v3.06 client-only candidate with a narrowly
      maintained UPF patch set. It is newer than the Stratum fork but bundles
      Scapy 2.4.3. The tracked patches in `ptf/patches/trex/` replace the
      reachable `imp` use, select the caller-provided Scapy, and remove Python
      3.14 regex-literal warnings from the imported UPF client paths. The
      [2026-09-24 upstream-candidate job](https://github.com/andybavier/upf/actions/runs/36070149147/job/107868953740)
      passes `pip check` and all required PTF/TRex imports with v3.06 and
      Scapy 2.7.0, with no syntax warnings.
- [ ] Evaluate any maintained compatible fork only if it offers a materially
      smaller or better-supported patch set than the Cisco-v3.06 candidate.
- [x] Assess the Cisco-v3.06 candidate's Python 3.14 compatibility, Scapy
      compatibility, UPF import/API boundary, and packaging needs. It requires
      the three local patches and a client-only image assembly; remote TRex
      daemon/server compatibility remains to be tested on hardware.
- [x] Select Cisco TRex v3.06 for the production client source, with the
      tracked UPF patch series. The tag resolves to commit
      `46be64bedbe9d505dfb55099c38119d6e86267ca`.
- [x] Record the selected source's checksum/revision and license/provenance.
      The Dockerfile verifies the v3.06 archive SHA-256
      `869c9120a427c507e023a12716ffd0332232a2c8c13e86b63d46a3e15d02b509`;
      the upstream project is Apache-2.0 licensed.

### 3. Remove the mixed-Scapy design

- [x] Update `ptf/Dockerfile` to fetch the selected pinned TRex source.
- [x] Copy only the required TRex client modules into the runtime image,
      including the stateless API and legacy daemon-management client.
- [x] Stop copying or exposing any bundled `scapy` package from TRex through
      `PYTHONPATH`.
- [x] Remove the custom Scapy wheel build/install override from the Dockerfile.
- [x] Install exactly one Scapy distribution that satisfies both PTF and the
      selected TRex client.
- [x] Verify all Scapy imports resolve to the same installed distribution.
      The [2026-09-25 production validation](https://github.com/andybavier/upf/actions/runs/36168175496)
      passes `pip check` and the PTF/TRex dependency smoke check.

### 4. Modernize and lock Python dependencies

- [ ] **Deferred:** Upgrade or replace `scapy-helper==0.14.8`. Dependabot
      independently ignores `pyperclip` updates as a temporary guardrail; the
      underlying PTF dependency relationship remains to be addressed.
- [x] Maintain an explicitly pinned, documented UPF patch series for the
      selected upstream source; do not suppress pip resolver errors.
- [ ] **Deferred:** Regenerate `ptf/requirements-ptf.txt` and
      `ptf/requirements-trex.txt` together, including hashes, from a
      source/input manifest rather than updating transitive lock entries
      independently.
- [ ] **Deferred:** Add the source manifests and/or documented regeneration
      command needed to reproduce the locked requirements.
- [ ] **Deferred:** Confirm a clean environment resolves both requirement
      files without `--ignore-installed`, `--no-deps`, or post-install package
      replacement.

### 5. Make the result continuously verifiable

- [x] Run `pip check` after all final runtime packages are installed.
- [x] Add a container smoke check for the documented Scapy version and import
      origins of `ptf`, `scapy`, `trex.stl.api`, and `trex_stf_lib`.
      (`make dependency-smoke`; it now passes in the production CI image.)
- [x] Run baseline and final-image smoke checks in a dedicated pull-request
      workflow. The final image now passes its smoke check.
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
