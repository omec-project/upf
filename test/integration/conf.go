// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/omec-project/upf-epc/pfcpiface"
	"go.uber.org/zap"
)

const (
	ConfigDefault = iota
	ConfigUPFBasedIPAllocation
	ConfigWipeOutOnUP4Restart
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

func UP4ConfigDefault() pfcpiface.Conf {
	var up4Server string
	switch os.Getenv(EnvMode) {
	case ModeDocker:
		up4Server = "mock-up4"
	case ModeNative:
		up4Server = "127.0.0.1"
	}

	config := baseConfig
	config.EnableP4rt = true
	config.EnableGtpuPathMonitoring = false
	config.P4rtcIface = pfcpiface.P4rtcInfo{
		SliceID:     1,
		AccessIP:    upfN3Address + "/32",
		P4rtcServer: up4Server,
		P4rtcPort:   "50001",
		QFIToTC: map[uint8]uint8{
			8: 2,
		},
		DefaultTC: 3,
	}

	config.CPIface = pfcpiface.CPIfaceInfo{
		UEIPPool: UEPoolCP,
	}

	return config
}

func UP4ConfigUPFBasedIPAllocation() pfcpiface.Conf {
	config := UP4ConfigDefault()
	config.CPIface = pfcpiface.CPIfaceInfo{
		EnableUeIPAlloc: true,
		UEIPPool:        UEPoolUPF,
	}

	return config
}

func UP4ConfigWipeOutOnUP4Restart() pfcpiface.Conf {
	config := UP4ConfigDefault()
	config.P4rtcIface.ClearStateOnRestart = true

	return config
}

func GetConfig(datapath string, configType uint32) pfcpiface.Conf {
	switch datapath {
	case DatapathUP4:
		switch configType {
		case ConfigDefault:
			return UP4ConfigDefault()
		case ConfigUPFBasedIPAllocation:
			return UP4ConfigUPFBasedIPAllocation()
		case ConfigWipeOutOnUP4Restart:
			return UP4ConfigWipeOutOnUP4Restart()
		}
	case DatapathBESS:
		switch configType {
		case ConfigDefault:
			return BESSConfigDefault()
		case ConfigUPFBasedIPAllocation:
			return BESSConfigUPFBasedIPAllocation()
		}
	}

	panic("wrong datapath or config type provided")
}

func PushSliceMeterConfig(sliceConfig pfcpiface.NetworkSlice) error {
	rawSliceConfig, err := json.Marshal(sliceConfig)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.8:8080/v1/config/network-slices", bytes.NewBuffer(rawSliceConfig))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
