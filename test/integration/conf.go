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
)

const (
	UEPoolUPF = "10.250.0.0/16"
	UEPoolCP  = "17.0.0.0/16"
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

func GetConfig(configType uint32) pfcpiface.Conf {
	switch configType {
	case ConfigDefault:
		return BESSConfigDefault()
	case ConfigUPFBasedIPAllocation:
		return BESSConfigUPFBasedIPAllocation()
	}

	panic("wrong datapath or config type provided")
}
