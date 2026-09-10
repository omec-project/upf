// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"runtime"

	"github.com/omec-project/upf-epc/pfcpiface"
	"go.uber.org/zap"
)

const (
	ConfigDefault = iota
	ConfigUPFBasedIPAllocation
	// A pool holding exactly two usable addresses, so that whether a rejected
	// session released its address is observable as capacity. With the /16 pool
	// it is not: DeallocIP returns an address to the back of the free queue while
	// allocation takes from the front, so the next session gets the next address
	// either way.
	ConfigUPFBasedIPAllocationTinyPool
)

const (
	UEPoolUPF = "10.250.0.0/16"
	// /30 is four addresses; NewIPPool drops the network and broadcast ones, so
	// this yields exactly .1 and .2.
	UEPoolUPFTiny = "10.251.0.0/30"
	UEPoolCP      = "17.0.0.0/16"
)

var baseConfig = pfcpiface.Conf{
	ReadTimeout: 15,
	RespTimeout: "2s",
	LogLevel:    zap.InfoLevel,
}

func BESSConfigDefault() pfcpiface.Conf {
	var intf string

	switch runtime.GOOS {
	case "darwin":
		intf = "lo0"
	case "linux":
		intf = "lo"
	}

	config := baseConfig
	config.AccessIface = pfcpiface.IfaceType{
		IfName: intf,
	}
	config.CoreIface = pfcpiface.IfaceType{
		IfName: intf,
	}
	return config
}

func BESSConfigUPFBasedIPAllocation() pfcpiface.Conf {
	config := BESSConfigDefault()
	config.CPIface = pfcpiface.CPIfaceInfo{
		EnableUeIPAlloc: true,
		UEIPPool:        UEPoolUPF,
	}

	return config
}

func BESSConfigUPFBasedIPAllocationTinyPool() pfcpiface.Conf {
	config := BESSConfigDefault()
	config.CPIface = pfcpiface.CPIfaceInfo{
		EnableUeIPAlloc: true,
		UEIPPool:        UEPoolUPFTiny,
	}

	return config
}

func GetConfig(configType uint32) pfcpiface.Conf {
	switch configType {
	case ConfigDefault:
		return BESSConfigDefault()
	case ConfigUPFBasedIPAllocation:
		return BESSConfigUPFBasedIPAllocation()
	case ConfigUPFBasedIPAllocationTinyPool:
		return BESSConfigUPFBasedIPAllocationTinyPool()
	}

	panic("wrong datapath or config type provided")
}
