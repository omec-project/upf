#!/usr/bin/env python3

# Copyright (c) 2014-2016, The Regents of the University of California.
# Copyright (c) 2016-2017, Nefeli Networks, Inc.
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

from __future__ import print_function
import os
import shlex
import subprocess
import sys
import time

TARGET_REGISTRY = os.getenv("DOCKER_REGISTRY", "ghcr.io/")
TARGET_REPOSITORY = os.getenv("DOCKER_REPOSITORY", "omec-project/")
TARGET = "bess_build"
FULL_TARGET = TARGET_REGISTRY + TARGET_REPOSITORY + TARGET


def print_usage(prog):
    print('Usage - {}'.format(prog))


def run_cmd(cmd, shell=False):
    if shell:
        subprocess.check_call(cmd, shell=True)
    else:
        subprocess.check_call(shlex.split(cmd))


def build():
    tag_suffix = ''
    version = time.strftime('%y%m%d')

    run_cmd('docker build -f env/Dockerfile '
            '-t {target}:latest{suffix} -t {target}:{version}{suffix} '
            '.'.format(target=FULL_TARGET,
                       version=version, suffix=tag_suffix))

    print('Build succeeded: {}:{}{}'.format(FULL_TARGET, version, tag_suffix))
    print('Build succeeded: {}:latest{}'.format(FULL_TARGET, tag_suffix))

    return version, tag_suffix


def push(version, tag_suffix):
    run_cmd('docker push {}:latest{}'.format(FULL_TARGET, tag_suffix))
    run_cmd('docker push {}:{}{}'.format(FULL_TARGET, version, tag_suffix))


def main(argv):
    if len(argv) != 1:
        print_usage(argv[0])
        return 2

    version, tag_suffix = build()

    prompt = input  # Python 3

    if prompt('Do you wish to push the image? [y/N] ').lower() in ['y', 'yes']:
        push(version, tag_suffix)
    else:
        print('The image was not pushed')

    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
