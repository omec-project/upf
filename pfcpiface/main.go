// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/json"
	"flag"
	"github.com/wmnsk/go-pfcp/message"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"
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
	pfcpsim         = flag.Bool("pfcpsim", false, "simulate PFCP")
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
	exit()
	parseFunc(conf *Conf)
	addUsageReports(sdRes *message.SessionDeletionResponse, seidKey uint64)
	handleVolQuotaExceed(uint32, uint64)
	setUpfInfo(conf *Conf)
	getUpf() *upf
	readCounterVal()
	sendURRForReporting(recItem *reportRecord)
	setInfo(udpConn *net.UDPConn, updAddr net.Addr, pconn *PFCPConn)
	handleCounterEntry(ce *IntfCounterEntry)
	getAccessIPStr(val *string)
	getAccessIP() net.IP
	getCoreIP(val *string)
	getN4SrcIP(val *string)
	handleChannelStatus() bool
	sendMsgToUPF(method string, pdrs []pdr, fars []far, urrs []urr) uint8
	sendDeleteAllSessionsMsgtoUPF()
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

func schedule(f func(), interval time.Duration, done <-chan bool) *time.Ticker {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				f()
			case <-done:
				return
			}
		}
	}()
	return ticker
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
	intf.setUpfInfo(&conf)

	if *pfcpsim || *simulate != "" {
		log.Println("Simulation mode. Done.")
		return
	}

	var n4srcIP string
	var accessIP string
	var coreIP string
	intf.getN4SrcIP(&n4srcIP)
	intf.getAccessIPStr(&accessIP)
	intf.getCoreIP(&coreIP)
	log.Println("n4srcip ", n4srcIP)
	log.Println("accessip ", accessIP)
	log.Println("coreip ", coreIP)
	go pfcpifaceMainLoop(intf, accessIP, coreIP, n4srcIP, conf.CPIface.DestIP)
	setupProm(intf)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
	intf.exit()
}
