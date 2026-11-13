#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Intel Corporation
"""Report PTF/TRex dependency state without failing on known legacy defects."""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
from importlib.metadata import PackageNotFoundError, distribution, version
from pathlib import Path

IMPORT_PROBE = "import importlib, sys; print(getattr(importlib.import_module(sys.argv[1]), '__file__', None))"


def report_module(module_name: str) -> None:
    # A child process turns any import failure into an exit status instead of an exception.
    result = subprocess.run(
        [sys.executable, "-P", "-c", IMPORT_PROBE, module_name],
        capture_output=True,
        text=True,
        check=False,
        env={**os.environ, "PYTHONPATH": os.pathsep.join(sys.path)},
    )
    if result.returncode != 0:
        stderr_lines = result.stderr.strip().splitlines()
        error = stderr_lines[-1] if stderr_lines else f"exit status {result.returncode}"
        print(f"IMPORT {module_name}: FAILED: {error}")
        return

    stdout_lines = result.stdout.strip().splitlines()
    print(f"IMPORT {module_name}: {stdout_lines[-1] if stdout_lines else None}")


def report_scapy_distribution() -> None:
    try:
        scapy_distribution = distribution("scapy")
    except PackageNotFoundError:
        print("SCAPY DISTRIBUTION: NOT INSTALLED")
        return

    print(f"SCAPY DISTRIBUTION VERSION: {version('scapy')}")
    print(
        f"SCAPY DISTRIBUTION ROOT: {Path(scapy_distribution.locate_file('')).resolve()}"
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--stage", required=True)
    args = parser.parse_args()

    print(f"BASELINE STAGE: {args.stage}")
    print(f"PYTHON EXECUTABLE: {sys.executable}")
    print("PYTHON PATH:")
    print("\n".join(sys.path))

    print("PIP FREEZE:")
    subprocess.run([sys.executable, "-m", "pip", "list", "--format=freeze"], check=True)

    print("PIP CHECK:")
    result = subprocess.run([sys.executable, "-m", "pip", "check"], check=False)
    print(f"PIP CHECK EXIT STATUS: {result.returncode}")

    report_scapy_distribution()
    for module in (
        "ptf",
        "scapy",
        "scapy.contrib.gtp",
        "trex.stl.api",
        "trex_stl_lib.api",
        "trex_stf_lib.trex_client",
    ):
        report_module(module)


if __name__ == "__main__":
    main()
