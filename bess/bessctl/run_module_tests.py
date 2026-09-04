#!/usr/bin/env python3

# Copyright (c) 2017, The Regents of the University of California.
# All rights reserved.
#
# SPDX-License-Identifier: BSD-3-Clause
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
#
# * Redistributions of source code must retain the above copyright notice, this
# list of conditions and the following disclaimer.
#
# * Redistributions in binary form must reproduce the above copyright notice,
# this list of conditions and the following disclaimer in the documentation
# and/or other materials provided with the distribution.
#
# * Neither the names of the copyright holders nor the names of their
# contributors may be used to endorse or promote products derived from this
# software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.


import argparse
import glob
import os
import shlex
import subprocess
import sys

this_dir = os.path.dirname(os.path.abspath(__file__))
bessctl = os.path.join(this_dir, "bessctl")
default_test_dir = os.path.join(this_dir, "module_tests")

try:
    from .errors import CommandError
except ImportError:  # executed as a script / imported as a top-level module
    if __package__:
        raise
    from errors import CommandError


class DaemonStartError(Exception):
    pass


def run_cmd(cmd):
    args = shlex.split(cmd)
    proc = subprocess.Popen(
        args,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        universal_newlines=True,
    )
    lines = []
    for line in proc.stdout or ():
        print(line, end="")
        lines.append(line)
    proc.wait()
    if proc.returncode != 0:
        raise CommandError(proc.returncode, args, "".join(lines))


def main():

    arg_parser = argparse.ArgumentParser(description="Run per-module unit tests")
    arg_parser.add_argument(
        "--test_name", type=str, default="*", help="Name of a specific test to run."
    )
    arg_parser.add_argument(
        "--test_dir",
        type=str,
        default=default_test_dir,
        help="Path to the directory to serach for tests.",
    )
    args = arg_parser.parse_args()

    any_failure = 0

    daemon_start_cmd = f"{bessctl} daemon start"

    try:
        run_cmd(daemon_start_cmd)
    except CommandError as e:
        raise DaemonStartError("bess daemon could not start") from e

    for file_name in glob.glob(os.path.join(args.test_dir, f"{args.test_name}.py")):
        print(f"Running test {file_name}")

        try:
            run_cmd(f"{bessctl} daemon reset -- run file {file_name}")
        except CommandError as e:
            print(f"Test {file_name} failed (exit code {e.returncode})")
            any_failure = 1
            run_cmd(daemon_start_cmd)

    sys.exit(any_failure)


if __name__ == "__main__":
    main()
