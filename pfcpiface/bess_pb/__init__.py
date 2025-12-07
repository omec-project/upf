"""pfcpiface.bess_pb package marker.

SPDX-FileCopyrightText: 2025 Arnav Kapoor
SPDX-License-Identifier: Apache-2.0

Ensures the generated Python protobufs in this directory are importable
as `pfcpiface.bess_pb.<module>`.
"""

from importlib import import_module
import sys as _sys
import google.protobuf.any_pb2 as _any  # ensure well-known protos are present

__all__ = [
    'bess_msg_pb2',
    'error_pb2',
    'module_msg_pb2',
    'service_pb2',
    'service_pb2_grpc',
    'util_msg_pb2',
]

# Attempt to import generated submodules and also register their bare names
# (e.g. 'bess_msg_pb2') in sys.modules so generated files using bare
# imports resolve correctly when imported as a package.
for _m in __all__:
    full = f"pfcpiface.bess_pb.{_m}"
    try:
        mod = import_module(full)
        try:
            _sys.modules[_m] = mod
        except Exception:
            pass
    except Exception:
        # Defer errors until module is actually imported; keep import lightweight.
        pass
