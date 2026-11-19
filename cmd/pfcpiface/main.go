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

var configPath = flag.String("config", "upf.jsonc", "path to upf config")

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

	// Log the configuration, but never whether lawful interception is provisioned.
	// The Li field is a pointer, so printing it renders an address on a tasked node
	// and <nil> on any other, telling anyone with access to the general operator log
	// that this network element is an interception point. Interception must be
	// undetectable, so redact the field and leave it indistinguishable.
	redactedConf := conf
	redactedConf.Li = nil
	logger.InitLog.Infof("%+v", redactedConf)

	pfcpi := pfcpiface.NewPFCPIface(conf)

	// blocking
	pfcpi.Run()
}
