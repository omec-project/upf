// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
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

// ParseN3IP : parse N3 IP address from the interface name
func ParseN3IP(n3name string) net.IP {
	byNameInterface, err := net.InterfaceByName(n3name)

	if err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}

	addresses, err := byNameInterface.Addrs()

	ip, _, err := net.ParseCIDR(addresses[0].String())
	if err != nil {
		fmt.Println("Unable to parse N3IP: ", err)
	}
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

		// create and add downlink pdr
		pdrDown := pdr{
			srcIface:     core,
			srcIP:        ueip + i,
			srcIfaceMask: 0xFF,
			srcIPMask:    0xFFFFFFFF,
			fseID:        teid + i,
			ctrID:        i,
			farID:        downlink,
		}
		upf.addPDR(pdrDown)

		// create and add uplink pdr
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
		upf.addPDR(pdrUp)

		// create and add downlink far
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
		upf.addFAR(farDown)

		// create and add uplink far
		farUp := far{
			farID:  uplink,
			fseID:  teid + i,
			action: farForward,
		}
		upf.addFAR(farUp)

		// create and add counters
		upf.addCounters(pdrDown.ctrID)
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
		n3IP:        ParseN3IP(conf.N3Iface.N3IfaceName),
		client:      pb.NewBESSControlClient(conn),
		ctx:         context.Background(),
		maxSessions: conf.MaxSessions,
	}

	//if conf.Mode == modeSim {
	sim(upf)
	//}
}
