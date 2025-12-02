# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2019-present Intel Corporation
# Copyright (c) 2024-present The UPF contributors

"""BESS gRPC stub for testing UPF startup.

This stub implements a tiny subset of the `BESSControl` API used by UPF:
- `GetVersion`
- `ModuleCommand`

It intentionally returns success for `ModuleCommand` and a fixed version string.
"""
import logging
from concurrent import futures

import grpc
from google.protobuf import any_pb2

# Ensure generated protobuf modules that use bare imports (e.g. `import bess_msg_pb2`)
# can be resolved by adding the package's `bess_pb` dir to sys.path when installed
# under site-packages.
import site as _site_mod
import os as _os_mod
import sys as _sys_mod
_site_pkgs = _site_mod.getsitepackages()
if _site_pkgs:
    _sp = _site_pkgs[0]
    _bess_dir = _os_mod.path.join(_sp, "pfcpiface", "bess_pb")
    if _os_mod.path.isdir(_bess_dir):
        _sys_mod.path.insert(0, _bess_dir)

# Also add the extracted `/app/pfcpiface/bess_pb` directory to sys.path when
# running with PYTHONPATH=/app so generated modules that use bare imports
# (e.g. `import bess_msg_pb2`) resolve correctly.
_app_base = _os_mod.environ.get("PYTHONPATH", "/app")
_app_bess_dir = _os_mod.path.join(_app_base, "pfcpiface", "bess_pb")
if _os_mod.path.isdir(_app_bess_dir):
    try:
        if _app_bess_dir not in _sys_mod.path:
            _sys_mod.path.insert(0, _app_bess_dir)
    except Exception:
        pass

_loaded_via_files = False
try:
    import importlib.util as _importlib_util
    _app_base = _os_mod.environ.get("PYTHONPATH", "/app")
    _pb_dir = _os_mod.path.join(_app_base, "pfcpiface", "bess_pb")
    if _os_mod.path.isdir(_pb_dir):
        # Make bare imports like `import bess_msg_pb2` resolve by adding the
        # bess_pb directory directly to `sys.path`.
        try:
            import sys as _sys
            if _pb_dir not in _sys.path:
                _sys.path.insert(0, _pb_dir)
        except Exception:
            pass
        def _load(name, fname):
            path = _os_mod.path.join(_pb_dir, fname)
            if _os_mod.path.isfile(path):
                spec = _importlib_util.spec_from_file_location(name, path)
                mod = _importlib_util.module_from_spec(spec)
                # Execute module to register descriptors into the global pool
                spec.loader.exec_module(mod)
                # Register under the full package name and also the short
                # bare module name (e.g. 'bess_msg_pb2') so generated files
                # that use bare imports resolve correctly.
                try:
                    import sys as _reg_sys
                    _reg_sys.modules[name] = mod
                    short = _os_mod.path.splitext(_os_mod.path.basename(fname))[0]
                    _reg_sys.modules[short] = mod
                except Exception:
                    pass
                return mod
            return None

        # Ensure well-known protos are present in the descriptor pool first.
        try:
            # import any_pb2 / other well-known protos so descriptor deps resolve
            import google.protobuf.any_pb2 as _any_pb2  # noqa: F401
        except Exception:
            pass

        # Load in dependency order to populate descriptor pool correctly.
        # Note: bess_msg_pb2 must be loaded before service_pb2 if service
        # depends on messages defined in bess_msg.proto.
        _load("pfcpiface.bess_pb.error_pb2", "error_pb2.py")
        _load("pfcpiface.bess_pb.util_msg_pb2", "util_msg_pb2.py")
        _load("pfcpiface.bess_pb.module_msg_pb2", "module_msg_pb2.py")
        _load("pfcpiface.bess_pb.bess_msg_pb2", "bess_msg_pb2.py")
        _load("pfcpiface.bess_pb.service_pb2", "service_pb2.py")
        _load("pfcpiface.bess_pb.service_pb2_grpc", "service_pb2_grpc.py")
        _loaded_via_files = True
except Exception:
    _loaded_via_files = False

if _loaded_via_files:
    import sys as _sys
    error_pb2 = _sys.modules.get("pfcpiface.bess_pb.error_pb2")
    util_msg_pb2 = _sys.modules.get("pfcpiface.bess_pb.util_msg_pb2")
    module_msg_pb2 = _sys.modules.get("pfcpiface.bess_pb.module_msg_pb2")
    bess_msg_pb2 = _sys.modules.get("pfcpiface.bess_pb.bess_msg_pb2")
    service_pb2 = _sys.modules.get("pfcpiface.bess_pb.service_pb2")
    service_pb2_grpc = _sys.modules.get("pfcpiface.bess_pb.service_pb2_grpc")
else:
    from pfcpiface.bess_pb import error_pb2
    from pfcpiface.bess_pb import util_msg_pb2
    from pfcpiface.bess_pb import module_msg_pb2
    from pfcpiface.bess_pb import bess_msg_pb2
    from pfcpiface.bess_pb import service_pb2
    from pfcpiface.bess_pb import service_pb2_grpc

LOG = logging.getLogger("bess_stub")


class BESSStub(service_pb2_grpc.BESSControlServicer):
    def GetVersion(self, request, context):
        LOG.info("GetVersion called")
        return bess_msg_pb2.VersionResponse(
            version="bess-stub/0.1",
            error=error_pb2.Error(code=0, errmsg=""),
        )

    def ModuleCommand(self, request, context):
        # request: CommandRequest { name, cmd, arg }
        # Use the generated protobuf field names (lowercase) when accessing
        # request attributes to avoid AttributeError on different generator
        # naming conventions.
        LOG.info("ModuleCommand called: name=%s cmd=%s", request.name, request.cmd)
        # For test purposes always return success (empty response)
        return bess_msg_pb2.CommandResponse(
            error=error_pb2.Error(code=0, errmsg=""),
            data=any_pb2.Any(),
        )


def serve(bind_addr: str = "0.0.0.0:10514") -> None:
    logging.basicConfig(level=logging.INFO)
    # At startup, verify protobuf runtime + generated modules are compatible.
    def _check_proto_compat():
        try:
            # Try to report installed protobuf version if available
            try:
                import pkg_resources
                pb_ver = pkg_resources.get_distribution("protobuf").version
            except Exception:
                pb_ver = None

            # Try importing the generated modules and ensure DESCRIPTOR exists.
            import importlib
            mod = importlib.import_module("pfcpiface.bess_pb.service_pb2")
            if not hasattr(mod, "DESCRIPTOR"):
                LOG.error("Imported pfcpiface.bess_pb.service_pb2 but DESCRIPTOR missing")
                if pb_ver:
                    LOG.error("Installed protobuf version: %s", pb_ver)
                LOG.error("Ensure protobuf==4.25.8 and grpcio==1.56.2 are installed")
                return False
            LOG.info("Protobuf generated modules import OK; DESCRIPTOR found")
            if pb_ver:
                LOG.info("protobuf runtime version: %s", pb_ver)
            return True
        except Exception as e:
            LOG.exception("Failed to import generated protobuf modules: %s", e)
            LOG.error("If this is running inside the BESS image, verify it was built with protobuf==4.25.8 and grpcio==1.56.2")
            return False

    if not _check_proto_compat():
        LOG.error("Proto compatibility check failed; aborting bess_stub startup")
        return
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    service_pb2_grpc.add_BESSControlServicer_to_server(BESSStub(), server)
    server.add_insecure_port(bind_addr)
    LOG.info("Starting bess_stub gRPC server on %s", bind_addr)
    server.start()
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        LOG.info("Shutting down bess_stub")


if __name__ == "__main__":
    serve()
