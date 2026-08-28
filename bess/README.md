<!--
SPDX-FileCopyrightText: 2016-2017, Nefeli Networks, Inc.
SPDX-FileCopyrightText: 2017, The Regents of the University of California.
SPDX-License-Identifier: BSD-3-Clause
-->

![Release](https://img.shields.io/github/v/release/omec-project/bess?sort=semver)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/omec-project/bess/badge)](https://scorecard.dev/viewer/?uri=github.com/omec-project/bess)

## BESS (Berkeley Extensible Software Switch)

BESS (formerly known as [SoftNIC](https://www2.eecs.berkeley.edu/Pubs/TechRpts/2015/EECS-2015-155.html)) is a modular framework for software switches. BESS itself is *not* a virtual switch; it is neither pre-configured nor hard-coded to provide particular functionality, such as Ethernet bridging or OpenFlow-driven switching. Instead, you (or an external controller) can *configure* your own packet processing datapath by composing small "modules". While the basic concept is similar to [Click](http://read.cs.ucla.edu/click/click), BESS does not sacrifice performance for programmability.

BESS was created by Sangjin Han and is developed at the University of California, Berkeley and at Nefeli Networks. [Contributors to BESS](https://github.com/omec-project/bess/blob/master/CONTRIBUTING.md) include students, researchers, and developers who care about networking with high performance and high customizability. BESS is open-source under a BSD license.

If you are new to BESS, we recommend you start here:

1. [BESS Overview](https://github.com/omec-project/bess/wiki/BESS-Overview)
2. [Build and Install BESS](https://github.com/omec-project/bess/wiki/Build-and-Install-BESS)
3. [Write a BESS Configuration Script](https://github.com/omec-project/bess/wiki/Writing-a-BESS-Configuration-Script)
4. [Connect BESS to a Network Interface, VM, or Container](https://github.com/omec-project/bess/wiki/Hooking-up-BESS-Ports)

Documentation can be found [here](https://github.com/omec-project/bess/wiki/). Please consider [contributing](https://github.com/omec-project/bess/wiki/How-to-Contribute) to the project!

## Reach out to us through

1. #sdcore-dev channel in [Aether Community Slack](https://aether5g-project.slack.com)
