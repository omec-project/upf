<!--
SPDX-FileCopyrightText: 2016-2017, Nefeli Networks, Inc.
SPDX-FileCopyrightText: 2017, The Regents of the University of California.
SPDX-License-Identifier: BSD-3-Clause
-->

### How to generate BESS code documentation

* Install doxygen (sudo apt-get install doxygen)
* cd bess/doxygen
* doxygen bess.dox
* This will create three files:
  * doxygen.errors (usually a list of all undocumented code, all errors go here.)
  * html/ a directory with BESS documentation in HTML
  * latex/ a directory with BESS documentation formatted in LaTeX
