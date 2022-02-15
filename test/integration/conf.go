// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"github.com/omec-project/upf-epc/pfcpiface"
)

const (
	defaultUEPool              = "10.250.0.0/16"
	defaultAccessIP            = "198.18.0.1/32"
	defaultP4RuntimeServerPort = "50001"
)

func ConfUP4Default() pfcpiface.Conf {
	return pfcpiface.Conf{
		RespTimeout: "2s",
		ReadTimeout: 15,
		CPIface: pfcpiface.CPIfaceInfo{
			UEIPPool: defaultUEPool,
		},
		P4rtcIface: pfcpiface.P4rtcInfo{
			AccessIP:     defaultAccessIP,
			P4rtcServer:  "127.0.0.1",
			P4rtcPort:    defaultP4RuntimeServerPort,
			P4Info:       "../../conf/p4/bin/p4info.txt",
			DeviceConfig: "../../conf/p4/bin/bmv2.json",
		},
		EnableP4rt: true,
	}
}

func ConfUP4UeIpAlloc() pfcpiface.Conf {
	c := ConfUP4Default()
	c.CPIface.EnableUeIPAlloc = true
	return c
}
