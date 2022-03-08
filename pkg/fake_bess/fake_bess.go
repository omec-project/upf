// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package fake_bess

import (
	"fmt"
	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
	"net"
)

type FakeBESS struct {
	grpcServer *grpc.Server
	server     *fakeBessService
}

// NewFakeBESS creates a new fake BESS server with the
func NewFakeBESS() *FakeBESS {
	return &FakeBESS{
		server: newFakeBESSService(),
	}
}

// Run starts and runs the BESS gRPC server on the given address. Blocking until Stop is called.
func (b *FakeBESS) Run(address string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(address))
	if err != nil {
		return err
	}

	b.grpcServer = grpc.NewServer()
	bess_pb.RegisterBESSControlServer(b.grpcServer, b.server)

	// Blocking
	err = b.grpcServer.Serve(listener)
	if err != nil {
		return err
	}

	return nil
}

// Stop the BESS gRPC server.
func (b *FakeBESS) Stop() {
	b.grpcServer.Stop()
}

func (b *FakeBESS) GetPdrTableEntries() (entries []*bess_pb.WildcardMatchCommandAddArg) {
	msgs := b.server.GetOrAddModule(pdrLookupModuleName).GetState()
	for _, m := range msgs {
		e, _ := m.(*bess_pb.WildcardMatchCommandAddArg)
		entries = append(entries, e)
	}
	return
}

func (b *FakeBESS) GetFarTableEntries() (entries []*bess_pb.ExactMatchCommandAddArg) {
	msgs := b.server.GetOrAddModule(farLookupModuleName).GetState()
	for _, m := range msgs {
		e, _ := m.(*bess_pb.ExactMatchCommandAddArg)
		entries = append(entries, e)
	}
	return
}

func (b *FakeBESS) GetSessionQerTableEntries() (entries []*bess_pb.QosCommandAddArg) {
	msgs := b.server.GetOrAddModule(sessionQerModuleName).GetState()
	for _, m := range msgs {
		e, _ := m.(*bess_pb.QosCommandAddArg)
		entries = append(entries, e)
	}
	return
}

func (b *FakeBESS) GetAppQerTableEntries() (entries []*bess_pb.QosCommandAddArg) {
	msgs := b.server.GetOrAddModule(appQerModuleName).GetState()
	for _, m := range msgs {
		e, _ := m.(*bess_pb.QosCommandAddArg)
		entries = append(entries, e)
	}
	return
}
