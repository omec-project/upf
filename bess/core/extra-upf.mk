# SPDX-License-Identifier: Apache-2.0
# Copyright 2020 Intel Corporation

# Initial flags
# CXXFLAGS += -Werror=format-truncation -Warray-bounds -fbounds-check \
# 			-fno-strict-overflow -fno-delete-null-pointer-checks -fwrapv

# Updated flags
CXXFLAGS += -Wno-deprecated-copy -Wno-cast-align

# Enable flag for clang++ / This flag is unknown by g++
ifneq "$(shell expr $(CXXCOMPILER) = g++)" "1"
  CXXFLAGS += -Wno-defaulted-function-deleted
endif

# When doing performance analysis
#CXXFLAGS += -fno-omit-frame-pointer

$(info   CXXFLAGS is $(CXXFLAGS))
