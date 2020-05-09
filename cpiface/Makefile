# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

CXX ?= g++
CXXFLAGS ?= -g -O3 -std=c++17 -Werror -Wall -Wcast-align
TARGET ?= zmq-cpiface
PBDIR ?= ./pb
GRPC_LIBS = -l:libgrpc++.a -l:libgrpc_unsecure.a -l:libcares.a

all:
	$(CXX) $(CXXFLAGS) *.cc -I$(PBDIR) $(PBDIR)/*.pb.o -o $(TARGET) -l:libprotobuf.a $(GRPC_LIBS) -lpthread -lz -lzmq -lglog

clean:
	rm -rf $(TARGET)
