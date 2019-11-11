# SPDX-License-Identifier: Apache-2.0
# Copyright(c) 2019 Intel Corporation

CXX ?= g++
CXXFLAGS ?= -g -O3 -std=c++17 -Werror -Wall -Wcast-align
TARGET ?= zmq-cpiface
PBDIR ?= ./protobuf
GRPC_SRC ?= /opt/grpc
PROTOBUF_INC ?= $(GRPC_SRC)/third_party/protobuf/src
GRPC_INC ?= $(GRPC_SRC)/include
PROTOBUF_LIB ?= $(GRPC_SRC)/libs/opt/protobuf/libprotobuf.a
GRPC_LIB ?= $(GRPC_SRC)/libs/opt
PROTOC ?= $(GRPC_SRC)/bins/opt/protobuf/protoc

all: proto-objs c-objs
	$(CXX) $(CXXFLAGS) *.pb.o $(PROTOBUF_LIB) -L$(GRPC_LIB) -o $(TARGET) zmq-cpiface.o -lgrpc++ -lzmq -lglog

proto-stuff:
	$(PROTOC) -I$(PBDIR) -I$(PROTOBUF_INC) --cpp_out=. $(PBDIR)/*.proto
	$(PROTOC) -I$(PBDIR) -I$(PROTOBUF_INC) --grpc_out=. --plugin=protoc-gen-grpc=`which grpc_cpp_plugin` $(PBDIR)/*.proto

c-objs: proto-stuff
	$(CXX) $(CXXFLAGS) -c -o zmq-cpiface.o zmq-cpiface.cc -I$(PBDIR) -I$(GRPC_INC) -I$(PROTOBUF_INC)

proto-objs: proto-stuff
	$(CXX) $(CXXFLAGS) *.pb.cc -c -I./ -I$(PBDIR) -I$(PROTOBUF_INC) -I$(GRPC_INC)

clean:
	rm -rf *~ $(TARGET) *.pb.cc *.pb.h core *.o
