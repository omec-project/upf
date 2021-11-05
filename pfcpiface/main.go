// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
)

var (
	configPath = flag.String("config", "upf.json", "path to upf config")
	httpAddr   = flag.String("http", "0.0.0.0:8080", "http IP/port combo")
	simulate   = flag.String("simulate", "", "create|delete simulated sessions")
	pfcpsim    = flag.Bool("pfcpsim", false, "simulate PFCP")
)

// Conf : Json conf struct.
type Conf struct {
	Mode              string           `json:"mode"`
	AccessIface       IfaceType        `json:"access"`
	CoreIface         IfaceType        `json:"core"`
	CPIface           CPIfaceInfo      `json:"cpiface"`
	P4rtcIface        P4rtcInfo        `json:"p4rtciface"`
	EnableP4rt        bool             `json:"enable_p4rt"`
	SimInfo           SimModeInfo      `json:"sim"`
	ConnTimeout       uint32           `json:"conn_timeout"`
	ReadTimeout       uint32           `json:"read_timeout"`
	EnableNotifyBess  bool             `json:"enable_notify_bess"`
	EnableEndMarker   bool             `json:"enable_end_marker"`
	NotifySockAddr    string           `json:"notify_sockaddr"`
	EndMarkerSockAddr string           `json:"endmarker_sockaddr"`
	LogLevel          string           `json:"log_level"`
	QciQosConfig      []QciQosConfig   `json:"qci_qos_config"`
	SliceMeterConfig  SliceMeterConfig `json:"slice_rate_limit_config"`
}

// QciQosConfig : Qos configured attributes.
type QciQosConfig struct {
	QCI                uint8  `json:"qci"`
	CBS                uint32 `json:"cbs"`
	PBS                uint32 `json:"pbs"`
	EBS                uint32 `json:"ebs"`
	BurstDurationMs    uint32 `json:"burst_duration_ms"`
	SchedulingPriority uint32 `json:"priority"`
}

type SliceMeterConfig struct {
	N6RateBps    uint64 `json:"n6_bps"`
	N6BurstBytes uint64 `json:"n6_burst_bytes"`
	N3RateBps    uint64 `json:"n3_bps"`
	N3BurstBytes uint64 `json:"n3_burst_bytes"`
}

// SimModeInfo : Sim mode attributes.
type SimModeInfo struct {
	MaxSessions uint32 `json:"max_sessions"`
	StartUEIP   net.IP `json:"start_ue_ip"`
	StartENBIP  net.IP `json:"start_enb_ip"`
	StartAUPFIP net.IP `json:"start_aupf_ip"`
	N6AppIP     net.IP `json:"n6_app_ip"`
	N9AppIP     net.IP `json:"n9_app_ip"`
	StartN3TEID string `json:"start_n3_teid"`
	StartN9TEID string `json:"start_n9_teid"`
}

// CPIfaceInfo : CPIface interface settings.
type CPIfaceInfo struct {
	Peers           []string `json:"peers"`
	UseFQDN         bool     `json:"use_fqdn"`
	NodeID          string   `json:"hostname"`
	EnableUeIPAlloc bool     `json:"enable_ue_ip_alloc"`
	UEIPPool        string   `json:"ue_ip_pool"`
	HTTPPort        string   `json:"http_port"`
	Dnn             string   `json:"dnn"`
}

// IfaceType : Gateway interface struct.
type IfaceType struct {
	IfName string `json:"ifname"`
}

// ParseJSON : parse json file and populate corresponding struct.
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

	// Set default log level
	if conf.LogLevel == "" {
		conf.LogLevel = "info"
	}
}

// ParseStrIP : parse IP address from config.
func ParseStrIP(n3name string) (net.IP, net.IPMask) {
	ip, ipNet, err := net.ParseCIDR(n3name)
	if err != nil {
		log.Fatalln("Unable to parse IP: ", err)
	}

	log.Println("IP: ", ip)

	return ip, (ipNet).Mask
}

// ParseIP : parse IP address from the interface name.
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

func init() {
	// Set up logger
	log.SetReportCaller(true)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}

func main() {
	// cmdline args
	flag.Parse()

	var (
		conf Conf
		fp   fastPath
	)

	// read and parse json startup file
	ParseJSON(configPath, &conf)

	if level, err := log.ParseLevel(conf.LogLevel); err != nil {
		log.Fatalln(err)
	} else {
		log.SetLevel(level)
	}

	log.Infoln(conf)

	if conf.EnableP4rt {
		fp = &p4rtc{}
	} else {
		fp = &bess{}
	}

	upf := NewUPF(&conf, fp)

	if *pfcpsim {
		pfcpSim()
		return
	}

	if *simulate != "" {
		if *simulate != "create" && *simulate != "delete" {
			log.Fatalln("Invalid simulate method", simulate)
		}

		upf.sim(*simulate, &conf.SimInfo)

		return
	}

	setupConfigHandler(upf)
	setupProm(upf)

	httpPort := "8080"
	if conf.CPIface.HTTPPort != "" {
		httpPort = conf.CPIface.HTTPPort
	}

	httpSrv := &http.Server{Addr: ":" + httpPort, Handler: nil}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalln("http server failed", err)
		}

		log.Infoln("http server closed")
	}()

	ctx, cancel := context.WithCancel(context.Background())
	node := NewPFCPNode(ctx, upf)
	go node.Serve()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig

	cancel()

	// Wait for node shutdown before http shutdown
	node.Done()

	if err := httpSrv.Shutdown(context.Background()); err != nil {
		log.Errorln("Failed to shutdown http: %v", err)
	}

	upf.exit()
}
