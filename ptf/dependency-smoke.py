#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Intel Corporation
"""Verify that the final PTF image has one coherent Python dependency stack."""

from __future__ import annotations

import importlib
import subprocess
import sys
from importlib.metadata import distribution, version
from pathlib import Path


def check_write_offsets(stream_json: dict, expected: list[int], fields: str) -> None:
    """Fail unless the stream's compiled VM writes land at the expected offsets."""
    write_offsets = [
        instruction["pkt_offset"]
        for instruction in stream_json["vm"]["instructions"]
        if instruction["type"] == "write_flow_var"
    ]
    if write_offsets != expected:
        raise RuntimeError(
            f"TRex compiled the {fields} rewrites at offsets {write_offsets}, "
            f"expected {expected}"
        )


def validate_trex_packet_construction() -> None:
    """Exercise the STL packet path used by the UPF linerate tests."""
    from pkt_utils import pkt_add_gtpu
    from scapy.layers.inet import IP, UDP
    from scapy.layers.l2 import Ether
    from trex.stl.api import (
        STLVM,
        STLFlowLatencyStats,
        STLPktBuilder,
        STLStream,
        STLTXCont,
    )

    vm = STLVM()
    vm.var(
        name="dst",
        min_value="16.0.0.1",
        max_value="16.0.0.2",
        size=4,
        op="inc",
    )
    vm.write(fv_name="dst", pkt_offset="IP.dst")
    vm.fix_chksum()

    packet = (
        Ether(dst="00:11:22:33:44:55", src="00:aa:bb:cc:dd:ee")
        / IP(src="6.6.6.6", dst="16.0.0.1")
        / UDP(sport=10002, dport=10001, chksum=0)
    )
    stream = STLStream(
        packet=STLPktBuilder(pkt=packet, vm=vm),
        mode=STLTXCont(pps=1_000),
        flow_stats=STLFlowLatencyStats(pg_id=1),
    )
    stream_json = stream.to_json()
    if "packet" not in stream_json:
        raise RuntimeError("TRex STL stream serialization omitted the packet")

    # IP.dst is 16 bytes into the IPv4 header, after a 14-byte Ethernet
    # header.  The IPv4 header begins with bit fields, so this also checks
    # that the offset calculation carries Scapy's bit cursor between fields.
    check_write_offsets(stream_json, [30], "IP.dst")

    gtpu_packet = pkt_add_gtpu(
        packet,
        out_ipv4_src="198.18.0.1",
        out_ipv4_dst="11.1.1.129",
        teid=1,
        ext_psc_type=0,
        ext_psc_qfi=9,
    )
    gtpu_vm = STLVM()
    gtpu_vm.var(name="tos", min_value=0, max_value=1, size=1, op="inc")
    gtpu_vm.var(name="teid", min_value=1, max_value=2, size=4, op="inc")
    gtpu_vm.var(
        name="inner_dst",
        min_value="16.0.0.1",
        max_value="16.0.0.2",
        size=4,
        op="inc",
    )
    gtpu_vm.write(fv_name="tos", pkt_offset="IP.tos")
    gtpu_vm.write(fv_name="teid", pkt_offset="GTP_U_Header.teid")
    gtpu_vm.write(fv_name="inner_dst", pkt_offset="IP:1.dst")
    gtpu_stream = STLStream(
        packet=STLPktBuilder(pkt=gtpu_packet, vm=gtpu_vm),
        mode=STLTXCont(pps=1_000),
    )
    gtpu_stream_json = gtpu_stream.to_json()
    if "packet" not in gtpu_stream_json:
        raise RuntimeError("TRex could not serialize the UPF GTP-U packet")

    # Each field follows a different run of bit fields: IP.tos directly
    # follows IPv4's version/IHL, the TEID follows the GTP-U flags, and the
    # inner IP.dst follows the GTP-U header (12 bytes with E set) and its
    # 4-byte PDU Session Container: 14 + 20 + 8 + 12 + 4 + 16 = 74.
    check_write_offsets(
        gtpu_stream_json,
        [15, 46, 74],
        "IP.tos, GTP_U_Header.teid, and IP:1.dst",
    )


def main() -> None:
    subprocess.run([sys.executable, "-m", "pip", "check"], check=True)

    # Compare against the file the Scapy distribution itself installed, not
    # its site-packages directory, which any stray scapy/ tree would satisfy.
    scapy_distribution = distribution("scapy")
    scapy_init = next(
        (
            file
            for file in scapy_distribution.files or ()
            if file.as_posix() == "scapy/__init__.py"
        ),
        None,
    )
    if scapy_init is None:
        raise RuntimeError("The Scapy distribution does not record scapy/__init__.py")
    expected_path = Path(scapy_init.locate()).resolve()

    scapy = importlib.import_module("scapy")
    scapy_path = Path(scapy.__file__).resolve()
    if scapy_path != expected_path:
        raise RuntimeError(
            "Scapy was imported from outside its installed distribution: "
            f"{scapy_path} (distribution installs {expected_path})"
        )

    modules = (
        "ptf",
        "scapy.contrib.gtp",
        "trex.stl.api",
        "trex_stl_lib.api",
        "trex_stf_lib.trex_client",
    )
    for module in modules:
        importlib.import_module(module)

    validate_trex_packet_construction()

    print(f"Scapy {version('scapy')} imported from {scapy_path}")
    print("PTF/TRex dependency and packet-construction smoke check passed")


if __name__ == "__main__":
    main()
