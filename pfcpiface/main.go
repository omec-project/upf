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
)

const (
	modeSim = "sim"
)

//grpc channel state
const (
	Ready = 2 //grpc channel state Ready
)

var (
	bessIP          = flag.String("bess", "localhost:10514", "BESS IP/port combo")
	configPath      = flag.String("config", "upf.json", "path to upf config")
	httpAddr        = flag.String("http", "0.0.0.0:8080", "http IP/port combo")
	simulate        = flag.String("simulate", "", "create|delete simulated sessions")
	n4SrcIPStr      = flag.String("n4SrcIPStr", "", "N4Interface IP")
	p4RtcServerIP   = flag.String("p4RtcServerIP", "", "P4 Server ip")
	p4RtcServerPort = flag.String("p4RtcServerPort", "", "P4 Server port")
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

// P4rtcInfo : P4 runtime interface settings
type P4rtcInfo struct {
	AccessIP    string `json:"access_ip"`
	P4rtcServer string `json:"p4rtc_server"`
	P4rtcPort   string `json:"p4rtc_port"`
	UEIP        string `json:"ue_ip_pool"`
}

// IfaceType : Gateway interface struct
type IfaceType struct {
	IfName string `json:"ifname"`
}

type common interface {
	parseFunc(conf *Conf)
	getUpfInfo(conf *Conf, u *upf)
	getAccessIP(val *string)
	getCoreIP(val *string)
	getN4SrcIP(val *string)
	handleChannelStatus() bool
	sendMsgToUPF(method string, pdrs []pdr, fars []far, u *upf) uint8
	sendDeleteAllSessionsMsgtoUPF(upf *upf)
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

func setSwitchInfo(p4rtClient *P4rtClient) (net.IP, net.IPMask, error) {
	log.Println("Set Switch Info")
	log.Println("device id ", (*p4rtClient).DeviceID)
	p4InfoPath := "/bin/p4info.txt"
	deviceConfigPath := "/bin/bmv2.json"

	errin := p4rtClient.GetForwardingPipelineConfig()
	if errin != nil {
		errin = p4rtClient.SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath)
		if errin != nil {
			log.Println("set forwarding pipeling config failed. ", errin)
			return nil, nil, errin
		}
	}

	intfEntry := IntfTableEntry{
		SrcIntf:   "ACCESS",
		Direction: "UPLINK",
	}

	errin = p4rtClient.ReadInterfaceTable(&intfEntry)
	if errin != nil {
		log.Println("Read Interface table failed ", errin)
		return nil, nil, errin
	}

	log.Println("accessip after read intf ", intfEntry.IP)
	accessIP := net.IP(intfEntry.IP)
	accessIPMask := net.CIDRMask(intfEntry.PrefixLen, 32)
	log.Println("AccessIP: ", accessIP, ", AccessIPMask: ", accessIPMask)

	return accessIP, accessIPMask, errin
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// cmdline args
	flag.Parse()
	var conf Conf
	var intf common
	// read and parse json startup file
	ParseJSON(configPath, &conf)
	log.Println(conf)

	p4rtcSt := &p4rtc{}
	bessSt := &bess{}
	if conf.EnableP4rt {
		intf = p4rtcSt
	} else {
		intf = bessSt
	}

	intf.parseFunc(&conf)

	log.Println("bessip ", *bessIP)
	/*
		    conn, errin := grpc.Dial(*bessIP, grpc.WithInsecure())
			if errin != nil {
				log.Fatalln("did not connect:", errin)
			}
			defer conn.Close()
		    log.Println("upf ", *upfPt)
		    upfPt.client = pb.NewBESSControlClient(conn)
	*/

	upfPt := &upf{}
	intf.getUpfInfo(&conf, upfPt)

	var simInfo *SimModeInfo
	if conf.Mode == modeSim {
		simInfo = &conf.SimInfo
		upfPt.simInfo = simInfo
	}

	if *simulate != "" {
		if *simulate != "create" && *simulate != "delete" {
			log.Fatalln("Invalid simulate method", simulate)
		}

		log.Println(*simulate, "sessions:", conf.MaxSessions)
		upfPt.sim(*simulate)
		return
	}

	var n4srcIP string
	var accessIP string
	var coreIP string
	intf.getN4SrcIP(&n4srcIP)
	intf.getAccessIP(&accessIP)
	intf.getCoreIP(&coreIP)
	log.Println("n4srcip ", n4srcIP)
	log.Println("accessip ", accessIP)
	log.Println("coreip ", coreIP)
	go pfcpifaceMainLoop(intf, upfPt, accessIP, coreIP, n4srcIP)

	setupProm(upfPt)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
