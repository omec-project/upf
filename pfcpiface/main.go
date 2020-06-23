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
	simuSess   = flag.String("simuSess", "create", "create or delete simulated sessions")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string    `json:"mode"`
	MaxSessions uint32    `json:"max_sessions"`
	N3Iface     IfaceType `json:"s1u"`
}

// IfaceType : Gateway interface struct
type IfaceType struct {
	N3IfaceName string `json:"ifname"`
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
		log.Fatalln("Unable to get info on N3 interface name:", err)
	}

	addresses, err := byNameInterface.Addrs()

	ip, _, err := net.ParseCIDR(addresses[0].String())
	if err != nil {
		log.Fatalln("Unable to parse N3IP: ", err)
	}
	log.Println("N3 IP: ", ip)
	return ip
}

func sim(upf *upf) {
	// Pause workers before
	upf.pauseAll()

	const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	const ng4tMaxUeRan, ng4tMaxEnbRan = 500000, 80
	s1uip := ip2int(upf.n3IP)

	for i := uint32(0); i < upf.maxSessions; i++ {
		// NG4T-based formula to calculate enodeB IP address against a given UE IP address
		// il_trafficgen also uses the same scheme
		// See SimuCPEnbv4Teid(...) in ngic code for more details
		ueOfRan := i % ng4tMaxUeRan
		ran := i / ng4tMaxUeRan
		enbOfRan := ueOfRan % ng4tMaxEnbRan
		enbIdx := ran*ng4tMaxEnbRan + enbOfRan

		// create/delete downlink pdr
		pdrDown := pdr{
			srcIface:     core,
			srcIP:        ueip + i,
			srcIfaceMask: 0xFF,
			srcIPMask:    0xFFFFFFFF,
			fseID:        teid + i,
			ctrID:        i,
			farID:        downlink,
		}
		if *simuSess == string("create") {
			upf.addPDR(pdrDown)
		} else if *simuSess == string("delete") {
			upf.delPDR(pdrDown)
		}

		// create/delete uplink pdr
		pdrUp := pdr{
			srcIface:     access,
			eNBTeid:      teid + i,
			dstIP:        ueip + i,
			srcIfaceMask: 0xFF,
			eNBTeidMask:  0xFFFFFFFF,
			dstIPMask:    0xFFFFFFFF,
			fseID:        teid + i,
			ctrID:        i,
			farID:        uplink,
		}
		if *simuSess == string("create") {
			upf.addPDR(pdrUp)
		} else if *simuSess == string("delete") {
			upf.delPDR(pdrUp)
		}

		// create/delete downlink far
		farDown := far{
			farID:       downlink,
			fseID:       teid + i,
			action:      farTunnel,
			tunnelType:  0x1,
			s1uIP:       s1uip,
			eNBIP:       enbip + enbIdx,
			eNBTeid:     teid + i,
			UDPGTPUPort: udpGTPUPort,
		}
		if *simuSess == string("create") {
			upf.addFAR(farDown)
		} else if *simuSess == string("delete") {
			upf.delFAR(farDown)
		}

		// create/delete uplink far
		farUp := far{
			farID:  uplink,
			fseID:  teid + i,
			action: farForward,
		}
		if *simuSess == string("create") {
			upf.addFAR(farUp)
		} else if *simuSess == string("delete") {
			upf.delFAR(farUp)
		}

		// create/delete counters
		if *simuSess == string("create") {
			upf.addCounters(pdrDown.ctrID)
		} else if *simuSess == string("delete") {
			upf.delCounters(pdrDown.ctrID)
		}
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
		log.Fatalln("did not connect:", err)
	}
	defer conn.Close()

	upf := &upf{
		n3IP:        ParseN3IP(conf.N3Iface.N3IfaceName),
		client:      pb.NewBESSControlClient(conn),
		ctx:         context.Background(),
		maxSessions: conf.MaxSessions,
	}

	//if conf.Mode == modeSim {
	sim(upf)
	//}
}
