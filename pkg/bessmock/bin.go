// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package bessmock

import (
	"context"
	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
)

// The methods below are not used by BESS-UPF - we can leave them unimplemented
// They are moved to bin.go to make service.go clean

func (b bessService) GetVersion(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.VersionResponse, error) {
	panic("implement me")
}

func (b bessService) ResetAll(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) KillBess(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ImportPlugin(ctx context.Context, request *bess_pb.ImportPluginRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) UnloadPlugin(ctx context.Context, request *bess_pb.UnloadPluginRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ListPlugins(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListPluginsResponse, error) {
	panic("implement me")
}

func (b bessService) PauseAll(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) PauseWorker(ctx context.Context, request *bess_pb.PauseWorkerRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ResumeWorker(ctx context.Context, request *bess_pb.ResumeWorkerRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ResumeAll(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ResetWorkers(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ListWorkers(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListWorkersResponse, error) {
	panic("implement me")
}

func (b bessService) AddWorker(ctx context.Context, request *bess_pb.AddWorkerRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) DestroyWorker(ctx context.Context, request *bess_pb.DestroyWorkerRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ResetTcs(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ListTcs(ctx context.Context, request *bess_pb.ListTcsRequest) (*bess_pb.ListTcsResponse, error) {
	panic("implement me")
}

func (b bessService) CheckSchedulingConstraints(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.CheckSchedulingConstraintsResponse, error) {
	panic("implement me")
}

func (b bessService) AddTc(ctx context.Context, request *bess_pb.AddTcRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) UpdateTcParams(ctx context.Context, request *bess_pb.UpdateTcParamsRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) UpdateTcParent(ctx context.Context, request *bess_pb.UpdateTcParentRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) GetTcStats(ctx context.Context, request *bess_pb.GetTcStatsRequest) (*bess_pb.GetTcStatsResponse, error) {
	panic("implement me")
}

func (b bessService) ListDrivers(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListDriversResponse, error) {
	panic("implement me")
}

func (b bessService) GetDriverInfo(ctx context.Context, request *bess_pb.GetDriverInfoRequest) (*bess_pb.GetDriverInfoResponse, error) {
	panic("implement me")
}

func (b bessService) ResetPorts(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ListPorts(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListPortsResponse, error) {
	panic("implement me")
}

func (b bessService) CreatePort(ctx context.Context, request *bess_pb.CreatePortRequest) (*bess_pb.CreatePortResponse, error) {
	panic("implement me")
}

func (b bessService) DestroyPort(ctx context.Context, request *bess_pb.DestroyPortRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) SetPortConf(ctx context.Context, request *bess_pb.SetPortConfRequest) (*bess_pb.CommandResponse, error) {
	panic("implement me")
}

func (b bessService) GetPortConf(ctx context.Context, request *bess_pb.GetPortConfRequest) (*bess_pb.GetPortConfResponse, error) {
	panic("implement me")
}

func (b bessService) GetLinkStatus(ctx context.Context, request *bess_pb.GetLinkStatusRequest) (*bess_pb.GetLinkStatusResponse, error) {
	panic("implement me")
}

func (b bessService) ListMclass(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListMclassResponse, error) {
	panic("implement me")
}

func (b bessService) GetMclassInfo(ctx context.Context, request *bess_pb.GetMclassInfoRequest) (*bess_pb.GetMclassInfoResponse, error) {
	panic("implement me")
}

func (b bessService) ResetModules(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) ListModules(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListModulesResponse, error) {
	panic("implement me")
}

func (b bessService) CreateModule(ctx context.Context, request *bess_pb.CreateModuleRequest) (*bess_pb.CreateModuleResponse, error) {
	panic("implement me")
}

func (b bessService) DestroyModule(ctx context.Context, request *bess_pb.DestroyModuleRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) GetModuleInfo(ctx context.Context, request *bess_pb.GetModuleInfoRequest) (*bess_pb.GetModuleInfoResponse, error) {
	panic("implement me")
}

func (b bessService) ConnectModules(ctx context.Context, request *bess_pb.ConnectModulesRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) DisconnectModules(ctx context.Context, request *bess_pb.DisconnectModulesRequest) (*bess_pb.EmptyResponse, error) {
	panic("implement me")
}

func (b bessService) DumpMempool(ctx context.Context, request *bess_pb.DumpMempoolRequest) (*bess_pb.DumpMempoolResponse, error) {
	panic("implement me")
}

func (b bessService) ListGateHookClass(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListGateHookClassResponse, error) {
	panic("implement me")
}

func (b bessService) GetGateHookClassInfo(ctx context.Context, request *bess_pb.GetGateHookClassInfoRequest) (*bess_pb.GetGateHookClassInfoResponse, error) {
	panic("implement me")
}

func (b bessService) ConfigureGateHook(ctx context.Context, request *bess_pb.ConfigureGateHookRequest) (*bess_pb.ConfigureGateHookResponse, error) {
	panic("implement me")
}

func (b bessService) ListGateHooks(ctx context.Context, request *bess_pb.EmptyRequest) (*bess_pb.ListGateHooksResponse, error) {
	panic("implement me")
}

func (b bessService) GateHookCommand(ctx context.Context, request *bess_pb.GateHookCommandRequest) (*bess_pb.CommandResponse, error) {
	panic("implement me")
}

func (b bessService) ConfigureResumeHook(ctx context.Context, request *bess_pb.ConfigureResumeHookRequest) (*bess_pb.CommandResponse, error) {
	panic("implement me")
}
