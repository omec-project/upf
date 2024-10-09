// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022-present Open Networking Foundation

package main

import (
	"flag"

	"github.com/omec-project/upf-epc/logger"
	"github.com/omec-project/upf-epc/pfcpiface"
	"go.uber.org/zap/zapcore"
)

var (
	configPath = flag.String("config", "upf.jsonc", "path to upf config")
)

func main() {
	// cmdline args
	flag.Parse()

	// Read and parse json startup file.
	conf, err := pfcpiface.LoadConfigFile(*configPath)
	if err != nil {
		logger.InitLog.Fatalln("error reading conf file:", err)
	}

	lvl, errLevel := zapcore.ParseLevel(conf.LogLevel.String())
	if errLevel != nil {
		logger.InitLog.Errorln("can not parse input level")
	}
	logger.InitLog.Infoln("setting log level to:", lvl)
	logger.SetLogLevel(lvl)

	logger.InitLog.Infof("%+v", conf)

	pfcpi := pfcpiface.NewPFCPIface(conf)

	// blocking
	pfcpi.Run()
}
