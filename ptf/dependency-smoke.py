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


def main() -> None:
    subprocess.run([sys.executable, "-m", "pip", "check"], check=True)

    scapy_distribution = distribution("scapy")
    distribution_root = Path(scapy_distribution.locate_file("")).resolve()

    scapy = importlib.import_module("scapy")
    scapy_path = Path(scapy.__file__).resolve()
    try:
        scapy_path.relative_to(distribution_root)
    except ValueError as error:
        raise RuntimeError(
            "Scapy was imported outside its installed distribution: "
            f"{scapy_path} (distribution root: {distribution_root})"
        ) from error

    modules = (
        "ptf",
        "scapy.contrib.gtp",
        "trex.stl.api",
        "trex_stl_lib.api",
        "trex_stf_lib.trex_client",
    )
    for module in modules:
        importlib.import_module(module)

    print(f"Scapy {version('scapy')} imported from {scapy_path}")
    print("PTF/TRex dependency smoke check passed")


if __name__ == "__main__":
    main()
