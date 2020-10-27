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
	"time"

	fqdn "github.com/Showmax/go-fqdn"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

const (
	modeSim = "sim"
)

var (
	bess       = flag.String("bess", "localhost:10514", "BESS IP/port combo")
	configPath = flag.String("config", "upf.json", "path to upf config")
	httpAddr   = flag.String("http", "0.0.0.0:8081", "http IP/port combo")
	simulate   = flag.String("simulate", "", "create|delete simulated sessions")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string      `json:"mode"`
	MaxSessions uint32      `json:"max_sessions"`
	AccessIface IfaceType   `json:"access"`
	CoreIface   IfaceType   `json:"core"`
	CPIface     CPIfaceInfo `json:"cpiface"`
	SimInfo     SimModeInfo `json:"sim"`
}

// SimModeInfo : Sim mode attributes
type SimModeInfo struct {
	StartUEIP   net.IP `json:"start_ue_ip"`
	StartENBIP  net.IP `json:"start_enb_ip"`
	StartAUPFIP net.IP `json:"start_aupf_ip"`
	N6AppIP     net.IP `json:"n6_app_ip"`
	N9AppIP     net.IP `json:"n9_app_ip"`
	StartN3TEID string `json:"start_n3_teid"`
	StartN9TEID string `json:"start_n9_teid"`
}

// CPIfaceInfo : CPIface interface settings
type CPIfaceInfo struct {
	DestIP   string `json:"nb_dst_ip"`
	SrcIP    string `json:"nb_src_ip"`
	FQDNHost string `json:"hostname"`
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

	err = json.Unmarshal(byteValue, conf)
	if err != nil {
		log.Fatalln("Unable to unmarshal conf attributes:", err)
	}
}

// ParseIP : parse IP address from the interface name
func ParseIP(name string, iface string) net.IP {
	byNameInterface, err := net.InterfaceByName(name)

	if err != nil {
		log.Fatalln("Unable to get info on interface name:", name, err)
	}

	addresses, err := byNameInterface.Addrs()
	if err != nil {
		log.Fatalln("Unable to retrieve addresses from interface name!", err)
	}

	ip, _, err := net.ParseCIDR(addresses[0].String())
	if err != nil {
		log.Fatalln("Unable to parse", iface, " IP: ", err)
	}
	log.Println(iface, " IP: ", ip)
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

	accessIP := ParseIP(conf.AccessIface.IfName, "Access")
	coreIP := ParseIP(conf.CoreIface.IfName, "Core")
	n4SrcIP := net.ParseIP("0.0.0.0")

	// fetch fqdn. Prefer json field
	fqdnh := conf.CPIface.FQDNHost
	if fqdnh == "" {
		fqdnh = fqdn.Get()
	}

	// get bess grpc client
	conn, err := grpc.Dial(*bess, grpc.WithInsecure())
	if err != nil {
		log.Fatalln("did not connect:", err)
	}
	defer conn.Close()

	var simInfo *SimModeInfo
	if conf.Mode == modeSim {
		simInfo = &conf.SimInfo
	}

	upf := &upf{
		accessIface: conf.AccessIface.IfName,
		coreIface:   conf.CoreIface.IfName,
		accessIP:    accessIP,
		coreIP:      coreIP,
		fqdnHost:    fqdnh,
		client:      pb.NewBESSControlClient(conn),
		maxSessions: conf.MaxSessions,
		simInfo:     simInfo,
	}

	if *simulate != "" {
		if *simulate != "create" && *simulate != "delete" {
			log.Fatalln("Invalid simulate method", simulate)
		}

		log.Println(*simulate, "sessions:", conf.MaxSessions)
		upf.sim(*simulate)
		return
	}

	ntfcHost := "ntfc-headless"
	ntfcIP := getRemoteIP(ntfcHost)
	for ntfcIP.IsUnspecified() {
		log.Println("NTF-C address unspecified - waiting for service...")
		time.Sleep(10 * time.Second)
		ntfcIP = getRemoteIP(ntfcHost)
	}

	log.Println("NTF-C address IP: ", ntfcIP.String())
	n4SrcIP = getLocalIP(ntfcHost)
	log.Println("NTF-U address IP: ", n4SrcIP.String())

	log.Println("N4 local IP: ", n4SrcIP.String())

	go pfcpifaceMainLoop(upf, accessIP.String(), coreIP.String(), n4SrcIP.String(), ntfcIP.String())

	setupProm(upf)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
