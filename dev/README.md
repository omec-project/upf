**BESS Stub & Local Test**

- **Purpose**: quick way to run a lightweight BESS gRPC stub alongside the `pfcpiface` runtime for smoke testing without DPDK/hugepage privileges.

- **Build & Run (Docker Compose)**

Run from the repository root:

```bash
docker compose -f docker-compose.test.yml up --build
```

This will build an image `upf-bess-stub:local` and then start the `bess_stub` service and the `pfcpiface` container using the local `dev/upf_test_local_lo.jsonc` config.

- **Notes**:
- The stub implements a minimal subset of the BESSControl gRPC API (GetVersion and ModuleCommand) and returns success for module commands. It's intended for CI or developer smoke tests where running full BESS with DPDK is not possible.
- To run a full end-to-end test with real BESS you still need a BESS image built/run with host hugepages and appropriate privileges.
