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

	fqdn "github.com/Showmax/go-fqdn"
)

const (
	modeSim = "sim"
)

var (
	configPath = flag.String("config", "upf.json", "path to upf config")
	httpAddr   = flag.String("http", "0.0.0.0:8080", "http IP/port combo")
	simulate   = flag.String("simulate", "", "create|delete simulated sessions")
	pfcpsim    = flag.Bool("pfcpsim", false, "simulate PFCP")
	n4SrcIPStr = flag.String("n4SrcIPStr", "", "N4Interface IP")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string      `json:"mode"`
	MaxSessions uint32      `json:"max_sessions"`
	AccessIface IfaceType   `json:"access"`
	CoreIface   IfaceType   `json:"core"`
	CPIface     CPIfaceInfo `json:"cpiface"`
	P4rtcIface  P4rtcInfo   `json:"p4rtciface"`
	EnableP4rt  bool        `json:"enable_p4rt"`
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

// ParseStrIP : parse IP address from config
func ParseStrIP(n3name string) (net.IP, net.IPMask) {
	ip, ipNet, err := net.ParseCIDR(n3name)
	if err != nil {
		log.Fatalln("Unable to parse IP: ", err)
	}
	log.Println("IP: ", ip)
	return ip, (ipNet).Mask
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
	var conf Conf
	var intf fastPath
	// read and parse json startup file
	ParseJSON(configPath, &conf)
	log.Println(conf)

	if conf.EnableP4rt {
		intf = &p4rtc{}
	} else {
		intf = &bess{}
	}

	// fetch fqdn. Prefer json field
	fqdnh := conf.CPIface.FQDNHost
	if fqdnh == "" {
		fqdnh = fqdn.Get()
	}

	upf := &upf{
		accessIface: conf.AccessIface.IfName,
		coreIface:   conf.CoreIface.IfName,
		fqdnHost:    fqdnh,
		maxSessions: conf.MaxSessions,
		intf:        intf,
	}

	upf.setUpfInfo(&conf)

	if *pfcpsim {
		pfcpSim()
		return
	}

	if *simulate != "" {
		if *simulate != "create" && *simulate != "delete" {
			log.Fatalln("Invalid simulate method", simulate)
		}

		log.Println(*simulate, "sessions:", conf.MaxSessions)
		upf.sim(*simulate)
		return
	}
	log.Println("N4 local IP: ", upf.n4SrcIP.String())
	log.Println("Access IP: ", upf.accessIP.String())
	log.Println("Core IP: ", upf.coreIP.String())

	go pfcpifaceMainLoop(upf, upf.accessIP.String(),
		upf.coreIP.String(), upf.n4SrcIP.String(),
		conf.CPIface.DestIP)

	setupProm(upf)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
	upf.exit()
}
