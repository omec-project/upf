#!/usr/bin/env python3

# Copyright (c) 2017, Nefeli Networks, Inc.
# SPDX-FileCopyrightText: 2024, Intel Corporation
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


import os
import shlex
import subprocess
import sys
import time

# How many cores we reserve for vSwitches?
# If set to 2, Containers will run on core 2, 3, 4, ..., skipping core 0-1.
VM_START_CPU = int(os.getenv("VM_START_CPU", "1"))
VM_MEM_SOCKET = int(os.getenv("VM_MEM_SOCKET", "0"))

HUGEPAGES_PATH = os.getenv("HUGEPAGES_PATH", "/dev/hugepages")

# "io retry" gives better throughput as the VM will not touch the packet
# payload, but it is somewhat unrealistic...
FWD_MODE = os.getenv("FWD_MODE", "macswap retry")

# per container configuration
# NUM_CPUS cores are used for forwarding
NUM_VCPUS = int(os.getenv("VM_VCPUS", "1"))
NUM_VPORTS = int(os.getenv("BESS_PORTS", "2"))
NUM_QUEUES = int(os.getenv("BESS_QUEUES", "1"))
QSIZE = int(os.getenv("BESS_QSIZE", "1024"))
PKT_SIZE = int(os.getenv("BESS_PKT_SIZE", "60"))

VERBOSE = int(os.getenv("VERBOSE", "0"))

SOCKDIR = "/tmp/bessd"
IMAGE = "registry.aetherproject.org/sdcore/bess_build"
CONTAINER_NAME = "bessd"


def launch(cid):
    print(f"Running container {cid} as a forwarder")
    first_cpu = VM_START_CPU + cid * NUM_VCPUS
    last_cpu = first_cpu + NUM_VCPUS - 1
    eal_opts = f"--in-memory --no-pci -m 256 -l 0,{first_cpu}-{last_cpu}"

    for port_id in range(NUM_VPORTS):
        sockpath = f"{SOCKDIR}/vhost_user{cid}_{port_id}.sock"
        eal_opts += f" --vdev=virtio_user{port_id},path={sockpath},queues={NUM_QUEUES}"

    testpmd_opts = (
        f"-i --txd={QSIZE} --rxd={QSIZE} "
        f"--txq={NUM_QUEUES} --rxq={NUM_QUEUES} --total-num-mbufs=65536"
    )

    if (
        subprocess.check_output(["numactl", "-H"], universal_newlines=True).find(
            " 1 nodes"
        )
        >= 0
    ):
        cmd = ""
    else:
        cmd = f"numactl -m {VM_MEM_SOCKET} "

    cmd += (
        "docker run --privileged -i --rm --name {name} -v {huge}:{huge} "
        "-v {sock}:{sock} {image} {cmd} {eal_options} "
        "-- {testpmd_options}".format(
            name=CONTAINER_NAME + str(cid),
            huge=HUGEPAGES_PATH,
            sock=SOCKDIR,
            image=IMAGE,
            cmd="/usr/local/bin/testpmd",
            eal_options=eal_opts,
            testpmd_options=testpmd_opts,
        )
    )

    if VERBOSE:
        out = None  # to screen
        print(cmd)
    else:
        out = subprocess.DEVNULL

    proc = subprocess.Popen(
        shlex.split(cmd),
        stdin=subprocess.PIPE,
        stdout=out,
        stderr=subprocess.STDOUT,
        universal_newlines=True,
    )
    print(f"set fwd {FWD_MODE}", file=proc.stdin)
    print(f"set txpkts {PKT_SIZE}", file=proc.stdin)
    print(f"start tx_first {QSIZE}", file=proc.stdin, flush=True)
    return proc


def kill(cid):
    print(f"Terminating container {cid} ")

    cmd = f"docker kill {CONTAINER_NAME + str(cid)}"

    if VERBOSE:
        print(cmd)

    try:
        out = None if VERBOSE else subprocess.DEVNULL
        err = None if VERBOSE else subprocess.DEVNULL
        subprocess.check_call(shlex.split(cmd), stdout=out, stderr=err)
    except subprocess.CalledProcessError:
        pass


def main(argv):
    if len(argv) != 2:
        print(f"Usage: {argv[0]} <# of containers to launch>", file=sys.stderr)
        return 2

    num_containers = int(argv[1])

    procs = []

    try:
        for i in range(num_containers):
            procs.append(launch(i))

        print("Press Ctrl+C to terminate all containers")
        while True:
            time.sleep(100)
    except KeyboardInterrupt:
        pass
    finally:
        for cid in range(num_containers):
            kill(cid)
        for proc in procs:
            try:
                proc.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.communicate()

    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
