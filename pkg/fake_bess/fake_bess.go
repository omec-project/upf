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
	service    *fakeBessService
}

// NewFakeBESS creates a new fake BESS server with the
func NewFakeBESS() *FakeBESS {
	return &FakeBESS{
		service: newFakeBESSService(),
	}
}

// Run starts and runs the BESS gRPC server on the given address. Blocking until Stop is called.
func (b *FakeBESS) Run(address string) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(address))
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

// Stop the BESS gRPC server.
func (b *FakeBESS) Stop() {
	b.grpcServer.Stop()
}

func (b *FakeBESS) GetPdrTableEntries() (entries []FakePdr) {
	msgs := b.service.GetOrAddModule(pdrLookupModuleName).GetState()
	for _, m := range msgs {
		e, ok := m.(*bess_pb.WildcardMatchCommandAddArg)
		if !ok {
			panic("unexpected message type")
		}
		entries = append(entries, UnmarshalPdr(e))
	}
	entries = AggregateRangeMatchPdr(entries)

	return
}

func (b *FakeBESS) GetFarTableEntries() (entries []FakeFar) {
	msgs := b.service.GetOrAddModule(farLookupModuleName).GetState()
	for _, m := range msgs {
		e, ok := m.(*bess_pb.ExactMatchCommandAddArg)
		if !ok {
			panic("unexpected message type")
		}
		entries = append(entries, UnmarshalFar(e))
	}
	return
}

func (b *FakeBESS) GetSessionQerTableEntries() (entries []FakeQer) {
	msgs := b.service.GetOrAddModule(sessionQerModuleName).GetState()
	for _, m := range msgs {
		e, ok := m.(*bess_pb.QosCommandAddArg)
		if !ok {
			panic("unexpected message type")
		}
		entries = append(entries, UnmarshalSessionQer(e))
	}
	return
}

func (b *FakeBESS) GetAppQerTableEntries() (entries []FakeQer) {
	msgs := b.service.GetOrAddModule(appQerModuleName).GetState()
	for _, m := range msgs {
		e, ok := m.(*bess_pb.QosCommandAddArg)
		if !ok {
			panic("unexpected message type")
		}
		entries = append(entries, UnmarshalAppQer(e))
	}
	return
}
