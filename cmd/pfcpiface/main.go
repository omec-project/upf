// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022-present Open Networking Foundation

package main

import (
	"flag"
	"github.com/omec-project/upf-epc/pfcpiface"
	log "github.com/sirupsen/logrus"
)

var (
	configPath = flag.String("config", "upf.json", "path to upf config")
)

func init() {
	// Set up logger
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}

func main() {
	// cmdline args
	flag.Parse()

	// Read and parse json startup file.
	conf, err := pfcpiface.LoadConfigFile(*configPath)
	if err != nil {
		log.Fatalln("Error reading conf file:", err)
	}

	log.SetLevel(conf.LogLevel)

	log.Infof("%+v", conf)

	pfcpi := pfcpiface.NewPFCPIface(conf)

	// blocking
	pfcpi.Run()
}
