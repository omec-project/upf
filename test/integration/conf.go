// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"github.com/omec-project/upf-epc/pfcpiface"
	"github.com/sirupsen/logrus"
)

const (
	defaultUEPool              = "10.250.0.0/16"
	defaultAccessIP            = "198.18.0.1/32"
	defaultP4RuntimeServerPort = "50001"
)

func ConfUP4Default() pfcpiface.Conf {
	return pfcpiface.Conf{
		CPIface: pfcpiface.CPIfaceInfo{
			UEIPPool: defaultUEPool,
		},
		P4rtcIface: pfcpiface.P4rtcInfo{
			AccessIP:    defaultAccessIP,
			P4rtcServer: "mock-up4",
			P4rtcPort:   defaultP4RuntimeServerPort,
		},
		EnableP4rt: true,
		LogLevel:   logrus.TraceLevel,
	}
}

func ConfUP4UeIpAlloc() pfcpiface.Conf {
	c := ConfUP4Default()
	c.CPIface.EnableUeIPAlloc = true
	return c
}
