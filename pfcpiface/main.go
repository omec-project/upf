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

	fqdn "github.com/Showmax/go-fqdn"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
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
	host        string
	enable_p4rt bool
	deviceId    uint64 = 1
	timeout     uint32 = 10
	conf        Conf
	p4client    *P4rtClient = nil
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
	AccessIface IfaceType     `json:"access"`
	CoreIface   IfaceType     `json:"core"`
	CPIface     CPIfaceInfo   `json:"cpiface"`
	PFCPIface   PFCPIfaceInfo `json:"pfcpiface"`
	EnableP4rt  bool          `json:"enable_p4rt"`
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
	DestIP   string `json:"nb_dst_ip"`
	SrcIP    string `json:"nb_src_ip"`
	FQDNHost string `json:"hostname"`
}

// PFCPIfaceInfo : PFCPIface interface settings
type PFCPIfaceInfo struct {
	AccessIP     string `json:"access_ip"`
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

	err = json.Unmarshal(byteValue, conf)
	if err != nil {
		log.Fatalln("Unable to unmarshal conf attributes:", err)
	}
}

// ParseStrIP : parse IP address from config
func ParseStrIP(n3name string) (net.IP, net.IPMask) {
	ip, ip_net, err := net.ParseCIDR(n3name)
	if err != nil {
		log.Fatalln("Unable to parse IP: ", err)
	}
	log.Println("IP: ", ip)
	return ip, (ip_net).Mask
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

func SetSwitchInfo(conf Conf) error {
	fmt.Println("Set Switch Info")
	accessIP, accessIPMask := ParseStrIP(conf.PFCPIface.AccessIP)
	fmt.Println("AccessIP: ", accessIP, ", AccessIPMask: ", accessIPMask)
	ueIP, ueIPMask := ParseStrIP(conf.PFCPIface.UEIP)
	fmt.Println("UEIP: ", ueIP, ", UEIPMask: ", ueIPMask)
	fmt.Printf("device id %d\n", (*p4client).DeviceID)
	p4InfoPath := "/bin/p4info.txt"
	deviceConfigPath := "/bin/bmv2.json"

	err := p4client.SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath)
	if err != nil {
		fmt.Printf("set forwarding pipeling config failed. %v\n", err)
		return err
	}

	prefix_size, _ := accessIPMask.Size()
	intf_entry := Intf_Table_Entry{
		Ip:         accessIP.To4(),
		Prefix_Len: prefix_size,
		Src_Intf:   "ACCESS",
		Direction:  "UPLINK",
	}

	func_type := FUNCTION_TYPE_INSERT

	err = p4client.WriteInterfaceTable(
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

	err = p4client.WriteInterfaceTable(
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
	enable_p4rt = conf.EnableP4rt
    var accessIP, coreIP, n4SrcIP  net.IP
    var accessIPMask        net.IPMask
    var p4rtc_server, fqdnh string
    var p4rtc_port          uint32
    var conn                *grpc.ClientConn
    var err                 error

	if enable_p4rt == true {
		// parse N3IP
		accessIP, accessIPMask = ParseStrIP(conf.PFCPIface.AccessIP)
		fmt.Println("AccessIP: ", accessIP, ", AccessIPMask: ", accessIPMask)
		p4rtc_server := conf.PFCPIface.P4rtc_server
		fmt.Println("p4rtc server ip/name", p4rtc_server)
		p4rtc_port = conf.PFCPIface.P4rtc_port
	} else {

		accessIP = ParseIP(conf.AccessIface.IfName, "Access")
		coreIP = ParseIP(conf.CoreIface.IfName, "Core")
		n4SrcIP = net.ParseIP("0.0.0.0")

		// fetch fqdn. Prefer json field
		fqdnh = conf.CPIface.FQDNHost
		if fqdnh == "" {
			fqdnh = fqdn.Get()
		}

		// get bess grpc client
		conn, err = grpc.Dial(*bess, grpc.WithInsecure())
		if err != nil {
			log.Fatalln("did not connect:", err)
		}
		defer conn.Close()
	}

	var simInfo *SimModeInfo = nil
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

	n4SrcIP = net.ParseIP("0.0.0.0")

	if enable_p4rt == true {
		host = p4rtc_server + ":" + strconv.Itoa(int(p4rtc_port))
		log.Println("server name: ", host)
		deviceId = 1
		timeout = 30
		var err error
		p4client, err = channel_setup()
		if err != nil {
			fmt.Printf("create channel failed : %v\n", err)
		}
	} else {
		if conf.CPIface.SrcIP == "" {
			if conf.CPIface.DestIP != "" {
				n4SrcIP = getOutboundIP(conf.CPIface.DestIP)
			}
		} else {
			addrs, err := net.LookupHost(conf.CPIface.SrcIP)
			if err == nil {
				n4SrcIP = net.ParseIP(addrs[0])
			}
		}
	}

	log.Println("N4 IP: ", n4SrcIP.String())

	go pfcpifaceMainLoop(upf, accessIP.String(), n4SrcIP.String())

	setupProm(upf)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
