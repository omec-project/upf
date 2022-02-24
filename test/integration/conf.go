// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"github.com/omec-project/upf-epc/pfcpiface"
)

const (
	ConfigDefault = iota
	ConfigUPFBasedIPAllocation
)

var baseConfig = pfcpiface.Conf{
	ReadTimeout:       15,
	RespTimeout:       "2s",
}

func BESSConfigDefault() pfcpiface.Conf {
	config := baseConfig
	config.AccessIface = pfcpiface.IfaceType{
		IfName: "lo0",
	}
	config.CoreIface = pfcpiface.IfaceType{
		IfName: "lo0",
	}
	return config
}

func BESSConfigUPFBasedIPAllocation() pfcpiface.Conf {
	config := BESSConfigDefault()
	config.CPIface = pfcpiface.CPIfaceInfo{
		EnableUeIPAlloc: true,
		UEIPPool:        "10.250.0.0/16",
	}

	return config
}

func UP4ConfigDefault() pfcpiface.Conf {
	config := baseConfig
	config.EnableP4rt = true
	config.P4rtcIface = pfcpiface.P4rtcInfo{
		AccessIP:    upfN3Address + "/32",
		P4rtcServer: "127.0.0.1",
		P4rtcPort:   "50001",
		QFIToTC: map[uint8]uint8{
			8: 3,
		},
	}

	config.CPIface = pfcpiface.CPIfaceInfo{
		UEIPPool:        "10.250.0.0/16",
	}

	return config
}

func UP4ConfigUPFBasedIPAllocation() pfcpiface.Conf {
	config := UP4ConfigDefault()
	config.CPIface = pfcpiface.CPIfaceInfo{
		EnableUeIPAlloc: true,
		UEIPPool:        "10.250.0.0/16",
	}

	return config
}

func GetConfig(fastpath string, configType uint32) pfcpiface.Conf {
	switch fastpath {
	case FastpathUP4:
		switch configType {
		case ConfigDefault:
			return UP4ConfigDefault()
		case ConfigUPFBasedIPAllocation:
			return UP4ConfigUPFBasedIPAllocation()
		}
	case FastpathBESS:
		switch configType {
		case ConfigDefault:
			return BESSConfigDefault()
		case ConfigUPFBasedIPAllocation:
			return BESSConfigUPFBasedIPAllocation()
		}
	}

	panic("Wrong fastpath or config type provided")

	return pfcpiface.Conf{}
}


