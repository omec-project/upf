# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

CXX ?= g++
CPU ?= native
CXXFLAGS ?= -march=$(CPU) -g -O3 -std=c++17 -Werror -Wall -Wcast-align
TARGET ?= zmq-cpiface
PBDIR ?= ./pb
GRPC_LIBS = -l:libprotobuf.a -l:libgrpc++.a -l:libgrpc_unsecure.a -l:libcares.a -lpthread -lz -l:libjsoncpp.a
SRCS := zmq-cpiface.cc

all:
	$(info   CXXFLAGS is $(CXXFLAGS))
	$(CXX) $(CXXFLAGS) $(SRCS) -I$(PBDIR) $(PBDIR)/*.pb.o -o $(TARGET) $(GRPC_LIBS) -lzmq -lglog

clean:
	rm -rf $(TARGET)
