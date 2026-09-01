// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Forsway Scandinavia AB

package pfcpiface

import (
	"context"
	"errors"
	"testing"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

// moduleCommandStub answers ModuleCommand with whatever the test needs and leaves the
// rest of BESSControlClient unimplemented, which is enough for processQER.
type moduleCommandStub struct {
	pb.BESSControlClient

	resp *pb.CommandResponse
	err  error
}

func (s moduleCommandStub) ModuleCommand(
	_ context.Context, _ *pb.CommandRequest, _ ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	return s.resp, s.err
}

// A module that refuses a rule answers over a healthy RPC: the gRPC error is nil and
// the refusal is carried in the response. Reporting that as success tells the caller
// the datapath holds a rule it has in fact rejected.
func TestProcessQERReportsAModuleRefusal(t *testing.T) {
	b := &bess{client: moduleCommandStub{
		resp: &pb.CommandResponse{Error: &pb.Error{Code: 1, Errmsg: "table is full"}},
	}}

	err := b.processQER(context.Background(), nil, upfMsgTypeAdd, "appQERLookup")
	if err == nil {
		t.Fatal("processQER() = nil for a refused rule, want an error")
	}
}

func TestProcessQERReportsAFailedRPC(t *testing.T) {
	rpcErr := errors.New("connection refused")
	b := &bess{client: moduleCommandStub{err: rpcErr}}

	err := b.processQER(context.Background(), nil, upfMsgTypeAdd, "appQERLookup")
	if !errors.Is(err, rpcErr) {
		t.Errorf("processQER() = %v, want the RPC error", err)
	}
}

func TestProcessQERAcceptsAnInstalledRule(t *testing.T) {
	b := &bess{client: moduleCommandStub{resp: &pb.CommandResponse{}}}

	if err := b.processQER(context.Background(), nil, upfMsgTypeAdd, "appQERLookup"); err != nil {
		t.Errorf("processQER() = %v, want nil for a rule the module took", err)
	}
}

func TestProcessQERRejectsAnUnknownMethod(t *testing.T) {
	b := &bess{client: moduleCommandStub{resp: &pb.CommandResponse{}}}

	if err := b.processQER(context.Background(), nil, upfMsgTypeMod, "appQERLookup"); err == nil {
		t.Error("processQER() = nil for an unsupported method, want an error")
	}
}
