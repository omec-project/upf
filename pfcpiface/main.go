// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

const (
	modeSim = "sim"
)

var (
	bess       = flag.String("bess", "localhost:10514", "BESS IP/port combo")
	configPath = flag.String("config", "upf.json", "path to upf config")
	simuDelay  = flag.Duration("simuDelay", 0, "create/delete simulated sessions with simuDelay")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string      `json:"mode"`
	MaxSessions uint32      `json:"max_sessions"`
	N3Iface     IfaceType   `json:"s1u"`
	CPIface     CPIfaceInfo `json:"cpiface"`
}

// CPIfaceInfo : CPIface interface settings
type CPIfaceInfo struct {
	SourceIP string `json:"zmqd_ip"`
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

func sim(upf *upf, method string) {
	start := time.Now()

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

		// create/delete uplink far
		farUp := far{
			farID:  uplink,
			fseID:  teid + i,
			action: farForward,
		}

		switch method {
		case "create":
			doneCh := make(chan bool, 8)
			upf.addPDR(doneCh, pdrDown)
			upf.addPDR(doneCh, pdrUp)

			upf.addFAR(doneCh, farDown)
			upf.addFAR(doneCh, farUp)

			upf.addCounter(doneCh, pdrDown.ctrID, "PreQoSCounter")
			upf.addCounter(doneCh, pdrDown.ctrID, "PostDLQoSCounter")
			upf.addCounter(doneCh, pdrDown.ctrID, "PostULQoSCounter")

			for i := 0; i < 7; i++ {
				<-doneCh
			}
			close(doneCh)

		case "delete":
			doneCh := make(chan bool, 8)

			upf.delPDR(doneCh, pdrDown)
			upf.delPDR(doneCh, pdrUp)

			upf.delFAR(doneCh, farDown)
			upf.delFAR(doneCh, farUp)

			upf.delCounter(doneCh, pdrDown.ctrID, "PreQoSCounter")
			upf.delCounter(doneCh, pdrDown.ctrID, "PostDLQoSCounter")
			upf.delCounter(doneCh, pdrDown.ctrID, "PostULQoSCounter")

			for i := 0; i < 7; i++ {
				<-doneCh
			}
			close(doneCh)

		default:
			log.Fatalln("Unsupported method", method)
		}
	}

	upf.resumeAll()

	log.Println("Sessions/s:", float64(upf.maxSessions)/time.Since(start).Seconds())
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
	n3IP := ParseN3IP(conf.N3Iface.N3IfaceName)

	// get bess grpc client
	conn, err := grpc.Dial(*bess, grpc.WithInsecure())
	if err != nil {
		log.Fatalln("did not connect:", err)
	}
	defer conn.Close()

	upf := &upf{
		n3IP:        n3IP,
		client:      pb.NewBESSControlClient(conn),
		maxSessions: conf.MaxSessions,
	}

	if *simuDelay > 0 {
		log.Println("Adding sessions:", conf.MaxSessions)
		sim(upf, "create")

		time.Sleep(*simuDelay)

		log.Println("Deleting sessions:", conf.MaxSessions)
		sim(upf, "delete")
	} else {
		pfcpifaceMainLoop(n3IP.String(), conf.CPIface.SourceIP)
	}
}
