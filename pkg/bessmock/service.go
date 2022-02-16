// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package bessmock

import (
	"context"
	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
)

type bessService struct {
}

func NewBESSService() *bessService {
	return &bessService{}
}

func (b bessService) GetPortStats(ctx context.Context, request *bess_pb.GetPortStatsRequest) (*bess_pb.GetPortStatsResponse, error) {
	// TODO: implement it
	panic("implement me")
}

func (b bessService) ModuleCommand(ctx context.Context, request *bess_pb.CommandRequest) (*bess_pb.CommandResponse, error) {
	// TODO: implement it
	panic("implement me")
}
