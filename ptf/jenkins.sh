#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2021 Open Networking Foundation

# This file is executed by the Jenkins job defined at
# https://gerrit.onosproject.org/plugins/gitiles/ci-management/+/refs/heads/master/jjb/pipeline/bess-upf-linerate.groovy
# https://gerrit.onosproject.org/plugins/gitiles/ci-management/+/refs/heads/master/jjb/templates/bess-upf-job.yaml

set -eux -o pipefail

make build
./run_tests -t tests/linerate
