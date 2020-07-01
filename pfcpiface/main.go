// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
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
	httpAddr   = flag.String("http", "0.0.0.0:8080", "http IP/port combo")
	simulate   = flag.String("simulate", "", "create|delete simulated sessions")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string      `json:"mode"`
	MaxSessions uint32      `json:"max_sessions"`
	N3Iface     IfaceType   `json:"s1u"`
	N6Iface     IfaceType   `json:"sgi"`
	CPIface     CPIfaceInfo `json:"cpiface"`
}

// CPIfaceInfo : CPIface interface settings
type CPIfaceInfo struct {
	SourceIP string `json:"zmqd_ip"`
}

// IfaceType : Gateway interface struct
type IfaceType struct {
	IfName string `json:"ifname"`
}

// ParseJSON : parse json file and populate corresponding struct
func ParseJSON(filepath *string, conf *Conf) {
	/* Open up file */
	jsonFile, err := os.Open(*filepath)

	if err != nil {
		log.Fatalln("Error opening file: ", err)
	}
	defer jsonFile.Close()

	/* read our opened file */
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Fatalln("Error reading file: ", err)
	}

	json.Unmarshal(byteValue, conf)
}

// ParseN3IP : parse N3 IP address from the interface name
func ParseN3IP(n3name string) net.IP {
	byNameInterface, err := net.InterfaceByName(n3name)

	if err != nil {
		log.Fatalln("Unable to get info on N3 interface name:", n3name, err)
	}

	addresses, err := byNameInterface.Addrs()

	ip, _, err := net.ParseCIDR(addresses[0].String())
	if err != nil {
		log.Fatalln("Unable to parse N3IP: ", err)
	}
	log.Println("N3 IP: ", ip)
	return ip
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// cmdline args
	flag.Parse()

	// read and parse json startup file
	var conf Conf
	ParseJSON(configPath, &conf)
	log.Println(conf)
	// parse N3IP
	n3IP := ParseN3IP(conf.N3Iface.IfName)

	// get bess grpc client
	conn, err := grpc.Dial(*bess, grpc.WithInsecure())
	if err != nil {
		log.Fatalln("did not connect:", err)
	}
	defer conn.Close()

	upf := &upf{
		n3Iface:     conf.N3Iface.IfName,
		n6Iface:     conf.N6Iface.IfName,
		n3IP:        n3IP,
		client:      pb.NewBESSControlClient(conn),
		maxSessions: conf.MaxSessions,
	}

	if *simulate != "" {
		if *simulate != "create" && *simulate != "delete" {
			log.Fatalln("Invalid simulate method", simulate)
		}

		log.Println(*simulate, "sessions:", conf.MaxSessions)
		upf.sim(*simulate)
		return
	}

	go pfcpifaceMainLoop(upf, n3IP.String(), conf.CPIface.SourceIP)

	setupProm(upf)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
