// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package bessmock

import (
	"fmt"
	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
	"net"
)

type BESSMock struct {
	localEndpoint  string
	remoteEndpoint string

	grpcServer *grpc.Server
	service    *bessService
}

func NewBESSMock(lAddr string, rAddr string) *BESSMock {
	return &BESSMock{
		localEndpoint:  lAddr,
		remoteEndpoint: rAddr,
		service:        NewBESSService(),
	}
}

func (b *BESSMock) Run() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(b.localEndpoint))
	if err != nil {
		return err
	}

	b.grpcServer = grpc.NewServer()
	bess_pb.RegisterBESSControlServer(b.grpcServer, b.service)

	// Blocking
	err = b.grpcServer.Serve(listener)
	if err != nil {
		return err
	}

	return nil
}

func (b *BESSMock) Stop() {
	b.grpcServer.Stop()
}
