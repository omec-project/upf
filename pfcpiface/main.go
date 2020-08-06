// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	// "github.com/badhrinathpa/p4rtc_go"
)

const (
	modeSim = "sim"
)

const (
	Idle             = 0
	Connecting       = 1
	Ready            = 2
	TransientFailure = 3
	Shutdown         = 4
)

var (
	host     string
	deviceId uint64 = 1
	timeout  uint32 = 10
	conf     Conf
	client   *P4rtClient = nil
)

var (
	bess       = flag.String("bess", "localhost:10514", "BESS IP/port combo")
	configPath = flag.String("config", "upf.json", "path to upf config")
	httpAddr   = flag.String("http", "0.0.0.0:8080", "http IP/port combo")
	simulate   = flag.String("simulate", "", "create|delete simulated sessions")
)

// Conf : Json conf struct
type Conf struct {
	Mode        string        `json:"mode"`
	MaxSessions uint32        `json:"max_sessions"`
	N3Iface     IfaceType     `json:"s1u"`
	N6Iface     IfaceType     `json:"sgi"`
	CPIface     CPIfaceInfo   `json:"cpiface"`
	PFCPIface   PFCPIfaceInfo `json:"pfcpiface"`
	SimInfo     SimModeInfo   `json:"sim"`
}

// SimModeInfo : Sim mode attributes
type SimModeInfo struct {
	StartUeIP    string `json:"start_ue_ip"`
	StartEnodeIP string `json:"start_enb_ip"`
	StartTeid    string `json:"start_teid"`
}

// CPIfaceInfo : CPIface interface settings
type CPIfaceInfo struct {
	DestIP string `json:"nb_dst_ip"`
}

// CPIfaceInfo : CPIface interface settings
type PFCPIfaceInfo struct {
	N3IP         string `json:"n3_ip"`
	P4rtc_server string `json:"p4rtc_server"`
	P4rtc_port   uint32 `json:"p4rtc_port"`
	UEIP         string `json:"ue_ip_pool"`
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

// ParseIP : parse IP address from config
func ParseIP(n3name string) (net.IP, net.IPMask) {
	ip, ip_net, err := net.ParseCIDR(n3name)
	if err != nil {
		log.Fatalln("Unable to parse IP: ", err)
	}
	log.Println("IP: ", ip)
	return ip, (ip_net).Mask
}

func SetSwitchInfo(conf Conf) error {
	fmt.Println("Set Switch Info")
	n3IP, n3IPMask := ParseIP(conf.PFCPIface.N3IP)
	fmt.Println("N3IP: ", n3IP, ", N3IPMask: ", n3IPMask)
	ueIP, ueIPMask := ParseIP(conf.PFCPIface.UEIP)
	fmt.Println("UEIP: ", ueIP, ", UEIPMask: ", ueIPMask)
	fmt.Printf("device id %d\n", (*client).DeviceID)
	p4InfoPath := "/bin/p4info.txt"
	deviceConfigPath := "/bin/bmv2.json"

	err := client.SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath)
	if err != nil {
		fmt.Printf("set forwarding pipeling config failed. %v\n", err)
		return err
	}

	prefix_size, _ := n3IPMask.Size()
	intf_entry := Intf_Table_Entry{
		Ip:         n3IP.To4(),
		Prefix_Len: prefix_size,
		Src_Intf:   "ACCESS",
		Direction:  "UPLINK",
	}

	func_type := FUNCTION_TYPE_INSERT

	err = client.WriteInterfaceTable(
		intf_entry, func_type)
	if err != nil {
		fmt.Printf("Write Interface table failed. %v\n", err)
		return err
	}

	prefix_size, _ = ueIPMask.Size()
	intf_entry = Intf_Table_Entry{
		Ip:         ueIP.To4(),
		Prefix_Len: prefix_size,
		Src_Intf:   "CORE",
		Direction:  "DOWNLINK",
	}

	err = client.WriteInterfaceTable(
		intf_entry, func_type)
	return err
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// cmdline args
	flag.Parse()

	// read and parse json startup file
	ParseJSON(configPath, &conf)
	log.Println(conf)
	// parse N3IP
	n3IP, n3IPMask := ParseIP(conf.PFCPIface.N3IP)
	fmt.Println("N3IP: ", n3IP, ", N3IPMask: ", n3IPMask)
	p4rtc_server := conf.PFCPIface.P4rtc_server
	fmt.Println("p4rtc server ip/name", p4rtc_server)
	p4rtc_port := conf.PFCPIface.P4rtc_port

	var simInfo *SimModeInfo = nil
	if conf.Mode == modeSim {
		simInfo = &conf.SimInfo
	}

	upf := &upf{
		n3Iface:     conf.N3Iface.IfName,
		n6Iface:     conf.N6Iface.IfName,
		n3IP:        n3IP,
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

	n4SrcIP := getOutboundIP(conf.CPIface.DestIP)
	log.Println("N4 IP: ", n4SrcIP.String())

	go pfcpifaceMainLoop(upf, n3IP.String(), n4SrcIP.String())

	host = p4rtc_server + ":" + strconv.Itoa(int(p4rtc_port))
	log.Println("server name: ", host)
	deviceId = 1
	timeout = 30
	var err error
	client, err = CreateChannel(host, deviceId, timeout)
	if err != nil {
		fmt.Printf("create channel failed : %v\n", err)
	}
	if client != nil {
		fmt.Printf("device id %d\n", (*client).DeviceID)
		err = SetSwitchInfo(conf)
		if err != nil {
			fmt.Printf("Switch set info failed. %v\n", err)
		}
	} else {
		fmt.Printf("p4runtime client is null.\n")
	}

	setupProm(upf)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
