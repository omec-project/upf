// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"reflect"

	log "github.com/sirupsen/logrus"

	"net"
	"time"

	"encoding/json"
	"io/ioutil"
	"os"
)

const (
	// Default values
	maxReqRetriesDefault = 5
	respTimeoutDefault   = 2 * time.Second
	hbIntervalDefault    = 5 * time.Second
	readTimeoutDefault   = 15 * time.Second

	// NotifySockAddr : Unix Socket path to read bess notification from.
	NotifySockAddr = "/tmp/notifycp"

	// EndMarkerSockAddr : Unix Socket path to send end marker packet.
	EndMarkerSockAddr = "/tmp/pfcpport"

	// possible fastpaths
	FastpathBESS = "bess"
	FastpathUP4  = "up4"
)

// Conf : Json conf struct.
type Conf struct {
	LogLevel log.Level `json:"log_level"`

	// global settings for PFCP Agent
	EnableUeIPAlloc   bool             `json:"enable_ue_ip_alloc"`
	UEIPPool          string           `json:"ue_ip_pool"`
	EnableFlowMeasure bool             `json:"measure_flow"`
	EnableEndMarker   bool             `json:"enable_end_marker"`
	SliceMeterConfig  SliceMeterConfig `json:"slice_rate_limit_config"`
	CPIface           CPIfaceInfo      `json:"cpiface"`

	Fastpath string `json:"fastpath"`

	BESSInfo BESSInfo `json:"bess"`
	UP4Info  UP4Info  `json:"up4"`
}

// QfiQosConfig : Qos configured attributes.
type QfiQosConfig struct {
	QFI                uint8  `json:"qfi"`
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

// CPIfaceInfo : CPIface (northbound) interface settings.
type CPIfaceInfo struct {
	HTTPPort          string   `json:"http_port"`
	Peers             []string `json:"peers"`
	UseFQDN           bool     `json:"use_fqdn"`
	NodeID            string   `json:"node_id"`
	Dnn               string   `json:"dnn"`
	EnableHBTimer     bool     `json:"enable_hbTimer"`
	HeartBeatInterval string   `json:"heart_beat_interval"`

	ReadTimeout   uint32 `json:"read_timeout"` // TODO(max): convert to duration string
	MaxReqRetries uint8  `json:"max_req_retries"`
	RespTimeout   string `json:"resp_timeout"`
}

// IfaceType : Gateway interface struct.
type IfaceType struct {
	IfName string `json:"ifname"`
}

// BESSInfo : BESS pipeline settings.
type BESSInfo struct {
	Mode              string         `json:"mode"`
	AccessIface       IfaceType      `json:"access"`
	CoreIface         IfaceType      `json:"core"`
	EnableNotifyBess  bool           `json:"enable_notify_bess"`
	NotifySockAddr    string         `json:"notify_sockaddr"`
	EndMarkerSockAddr string         `json:"endmarker_sockaddr"`
	QfiQosConfig      []QfiQosConfig `json:"qfi_qos_config"`
	SimInfo           SimModeInfo    `json:"sim"`
}

// UP4Info : UP4 interface settings.
type UP4Info struct {
	AccessIP    string          `json:"access_ip"`
	P4rtcServer string          `json:"p4rtc_server"`
	P4rtcPort   string          `json:"p4rtc_port"`
	QFIToTC     map[uint8]uint8 `json:"qfi_tc_mapping"`
}

// validateConf checks that the given config reaches a baseline of correctness.
func validateConf(conf Conf) error {
	if conf.Fastpath == FastpathUP4 {
		_, _, err := net.ParseCIDR(conf.UP4Info.AccessIP)
		if err != nil {
			return ErrInvalidArgumentWithReason("conf.UP4Info.AccessIP", conf.UP4Info.AccessIP, err.Error())
		}

		_, _, err = net.ParseCIDR(conf.UEIPPool)
		if err != nil {
			return ErrInvalidArgumentWithReason("conf.UEIPPool", conf.UEIPPool, err.Error())
		}

		if !reflect.DeepEqual(BESSInfo{}, conf.BESSInfo) {
			return ErrInvalidArgumentWithReason("conf.BESSInfo", conf.BESSInfo, "BESS settings must not be set for UP4")
		}
	} else if conf.Fastpath == FastpathBESS {
		if !reflect.DeepEqual(UP4Info{}, conf.UP4Info) {
			return ErrInvalidArgumentWithReason("conf.UP4Info", conf.UP4Info, "UP4 settings must not be set for BESS")
		}

		// Mode is only relevant in a BESS deployment.
		validModes := map[string]struct{}{
			"af_xdp":    {},
			"af_packet": {},
			"dpdk":      {},
			"sim":       {},
		}
		if _, ok := validModes[conf.BESSInfo.Mode]; !ok {
			return ErrInvalidArgumentWithReason("conf.Mode", conf.BESSInfo.Mode, "invalid mode")
		}

		if conf.BESSInfo.EnableNotifyBess {
			if conf.BESSInfo.NotifySockAddr == "" {
				conf.BESSInfo.NotifySockAddr = NotifySockAddr
			}
		}

		if conf.EnableEndMarker {
			if conf.BESSInfo.EndMarkerSockAddr == "" {
				conf.BESSInfo.EndMarkerSockAddr = EndMarkerSockAddr
			}
		}
	} else {
		return ErrInvalidArgumentWithReason("conf.fastpath", conf.Fastpath, "invalid fastpath type")
	}

	if conf.EnableUeIPAlloc {
		_, _, err := net.ParseCIDR(conf.UEIPPool)
		if err != nil {
			return ErrInvalidArgumentWithReason("conf.UEIPPool", conf.UEIPPool, err.Error())
		}
	}

	for _, peer := range conf.CPIface.Peers {
		ip := net.ParseIP(peer)
		if ip == nil {
			return ErrInvalidArgumentWithReason("conf.CPIface.Peers", peer, "invalid IP")
		}
	}

	if _, err := time.ParseDuration(conf.CPIface.RespTimeout); err != nil {
		return ErrInvalidArgumentWithReason("conf.RespTimeout", conf.CPIface.RespTimeout, "invalid duration")
	}

	if conf.CPIface.ReadTimeout == 0 {
		return ErrInvalidArgumentWithReason("conf.ReadTimeout", conf.CPIface.ReadTimeout, "invalid duration")
	}

	if conf.CPIface.MaxReqRetries == 0 {
		return ErrInvalidArgumentWithReason("conf.MaxReqRetries", conf.CPIface.MaxReqRetries, "invalid number of retries")
	}

	if conf.CPIface.EnableHBTimer {
		if _, err := time.ParseDuration(conf.CPIface.HeartBeatInterval); err != nil {
			return err
		}
	}

	return nil
}

// LoadConfigFile : parse json file and populate corresponding struct.
func LoadConfigFile(filepath string) (Conf, error) {
	// Open up file.
	jsonFile, err := os.Open(filepath)
	if err != nil {
		return Conf{}, err
	}
	defer jsonFile.Close()

	// Read our file into memory.
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return Conf{}, err
	}

	var conf Conf

	err = json.Unmarshal(byteValue, &conf)
	if err != nil {
		return Conf{}, err
	}

	// Set defaults, when missing.
	if conf.CPIface.RespTimeout == "" {
		conf.CPIface.RespTimeout = respTimeoutDefault.String()
	}

	if conf.CPIface.ReadTimeout == 0 {
		conf.CPIface.ReadTimeout = uint32(readTimeoutDefault.Seconds())
	}

	if conf.CPIface.MaxReqRetries == 0 {
		conf.CPIface.MaxReqRetries = maxReqRetriesDefault
	}

	if conf.CPIface.EnableHBTimer {
		if conf.CPIface.HeartBeatInterval == "" {
			conf.CPIface.HeartBeatInterval = hbIntervalDefault.String()
		}
	}

	// Perform basic validation.
	err = validateConf(conf)
	if err != nil {
		return Conf{}, err
	}

	return conf, nil
}
