# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2019-present Intel Corporation
# Copyright (c) 2024-present The UPF contributors

"""Minimal BESS gRPC stub for testing UPF startup.

This stub implements a tiny subset of the `BESSControl` API used by UPF:
- `GetVersion`
- `ModuleCommand`

It intentionally returns success for `ModuleCommand` and a fixed version string.
"""
import logging
from concurrent import futures

import grpc
from google.protobuf import any_pb2

from pfcpiface.bess_pb import bess_msg_pb2
from pfcpiface.bess_pb import service_pb2_grpc
from pfcpiface.bess_pb import error_pb2

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
        LOG.info("ModuleCommand called: name=%s cmd=%s", request.Name, request.Cmd)
        # For test purposes always return success (empty response)
        return bess_msg_pb2.CommandResponse(
            error=error_pb2.Error(code=0, errmsg=""),
            data=any_pb2.Any(),
        )


def serve(bind_addr: str = "0.0.0.0:10514") -> None:
    logging.basicConfig(level=logging.INFO)
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
