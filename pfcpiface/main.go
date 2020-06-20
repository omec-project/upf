// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

const (
	modeSim = "sim"
)

var (
	bess       = flag.String("bess", "localhost:10514", "BESS IP/port combo")
	configPath = flag.String("config", "upf.json", "path to upf config")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string      `json:"mode"`
	MaxSessions uint32      `json:"max_sessions"`
	Cpiface     CpifaceType `json:"cpiface"`
}

// CpifaceType : ZMQ-based interface struct
type CpifaceType struct {
	N3IP net.IP `json:"s1u_sgw_ip"`
}

// ParseJSON : parse json file and populate corresponding struct
func ParseJSON(filepath *string, conf *Conf) {
	/* Open up file */
	jsonFile, err := os.Open(*filepath)

	if err != nil {
		log.Println("Error opening file: ", err)
		return
	}
	defer jsonFile.Close()

	/* read our opened file */
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Println("Error reading file: ", err)
		return
	}

	json.Unmarshal(byteValue, conf)
}

func sim(upf *upf) {
	// Pause workers before
	upf.pauseAll()

	for i := uint32(0); i < upf.maxSessions; i++ {

		// create and add pdr
		upf.addPDR(uint32(i))

		// create and add far
		upf.addFAR(uint32(i))

		// create and add counters
		upf.addCounters(uint32(i))
	}

	upf.resumeAll()
	log.Println("Done!")
}

func main() {
	var conf Conf

	// cmdline args
	flag.Parse()

	// read and parse json startup file
	ParseJSON(configPath, &conf)
	log.Println(conf)

	// get bess grpc client
	conn, err := grpc.Dial(*bess, grpc.WithInsecure())
	if err != nil {
		log.Println("did not connect:", err)
	}
	defer conn.Close()

	upf := &upf{
		n3IP:        conf.Cpiface.N3IP,
		client:      pb.NewBESSControlClient(conn),
		ctx:         context.Background(),
		maxSessions: conf.MaxSessions,
	}

	if conf.Mode == modeSim {
		sim(upf)
	}
}
