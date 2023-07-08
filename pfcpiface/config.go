// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"github.com/omec-project/upf-epc/internal/p4constants"
	log "github.com/sirupsen/logrus"

	"net"
	"time"

	"encoding/json"
	"io"
	"os"
)

const (
	// Default values
	maxReqRetriesDefault = 5
	respTimeoutDefault   = 2 * time.Second
	hbIntervalDefault    = 5 * time.Second
	readTimeoutDefault   = 15 * time.Second
)

// Conf : Json conf struct.
type Conf struct {
	Mode              string           `json:"mode"`
	AccessIface       IfaceType        `json:"access"`
	CoreIface         IfaceType        `json:"core"`
	CPIface           CPIfaceInfo      `json:"cpiface"`
	P4rtcIface        P4rtcInfo        `json:"p4rtciface"`
	EnableP4rt        bool             `json:"enable_p4rt"`
	EnableFlowMeasure bool             `json:"measure_flow"`
	SimInfo           SimModeInfo      `json:"sim"`
	ConnTimeout       uint32           `json:"conn_timeout"` // TODO(max): unused, remove
	ReadTimeout       uint32           `json:"read_timeout"` // TODO(max): convert to duration string
	EnableNotifyBess  bool             `json:"enable_notify_bess"`
	EnableEndMarker   bool             `json:"enable_end_marker"`
	NotifySockAddr    string           `json:"notify_sockaddr"`
	EndMarkerSockAddr string           `json:"endmarker_sockaddr"`
	LogLevel          log.Level        `json:"log_level"`
	QciQosConfig      []QciQosConfig   `json:"qci_qos_config"`
	SliceMeterConfig  SliceMeterConfig `json:"slice_rate_limit_config"`
	MaxReqRetries     uint8            `json:"max_req_retries"`
	RespTimeout       string           `json:"resp_timeout"`
	EnableHBTimer     bool             `json:"enable_hbTimer"`
	HeartBeatInterval string           `json:"heart_beat_interval"`
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
	HTTPPort        string   `json:"http_port"`
	Dnn             string   `json:"dnn"`
	EnableUeIPAlloc bool     `json:"enable_ue_ip_alloc"`
	UEIPPool        string   `json:"ue_ip_pool"`
}

// IfaceType : Gateway interface struct.
type IfaceType struct {
	IfName string `json:"ifname"`
}

// P4rtcInfo : P4 runtime interface settings.
type P4rtcInfo struct {
	SliceID             uint8           `json:"slice_id"`
	AccessIP            string          `json:"access_ip"`
	P4rtcServer         string          `json:"p4rtc_server"`
	P4rtcPort           string          `json:"p4rtc_port"`
	QFIToTC             map[uint8]uint8 `json:"qfi_tc_mapping"`
	DefaultTC           uint8           `json:"default_tc"`
	ClearStateOnRestart bool            `json:"clear_state_on_restart"`
}

// validateConf checks that the given config reaches a baseline of correctness.
func validateConf(conf Conf) error {
	if conf.EnableP4rt {
		_, _, err := net.ParseCIDR(conf.P4rtcIface.AccessIP)
		if err != nil {
			return ErrInvalidArgumentWithReason("conf.P4rtcIface.AccessIP", conf.P4rtcIface.AccessIP, err.Error())
		}

		_, _, err = net.ParseCIDR(conf.CPIface.UEIPPool)
		if err != nil {
			return ErrInvalidArgumentWithReason("conf.UEIPPool", conf.CPIface.UEIPPool, err.Error())
		}

		if conf.Mode != "" {
			return ErrInvalidArgumentWithReason("conf.Mode", conf.Mode, "mode must not be set for UP4")
		}
	} else {
		// Mode is only relevant in a BESS deployment.
		validModes := map[string]struct{}{
			"af_xdp":    {},
			"af_packet": {},
			"cndp":      {},
			"dpdk":      {},
			"sim":       {},
		}
		if _, ok := validModes[conf.Mode]; !ok {
			return ErrInvalidArgumentWithReason("conf.Mode", conf.Mode, "invalid mode")
		}
	}

	if conf.CPIface.EnableUeIPAlloc {
		_, _, err := net.ParseCIDR(conf.CPIface.UEIPPool)
		if err != nil {
			return ErrInvalidArgumentWithReason("conf.UEIPPool", conf.CPIface.UEIPPool, err.Error())
		}
	}

	for _, peer := range conf.CPIface.Peers {
		ip := net.ParseIP(peer)
		if ip == nil {
			return ErrInvalidArgumentWithReason("conf.CPIface.Peers", peer, "invalid IP")
		}
	}

	if _, err := time.ParseDuration(conf.RespTimeout); err != nil {
		return ErrInvalidArgumentWithReason("conf.RespTimeout", conf.RespTimeout, "invalid duration")
	}

	if conf.ReadTimeout == 0 {
		return ErrInvalidArgumentWithReason("conf.ReadTimeout", conf.ReadTimeout, "invalid duration")
	}

	if conf.MaxReqRetries == 0 {
		return ErrInvalidArgumentWithReason("conf.MaxReqRetries", conf.MaxReqRetries, "invalid number of retries")
	}

	if conf.EnableHBTimer {
		if _, err := time.ParseDuration(conf.HeartBeatInterval); err != nil {
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
	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		return Conf{}, err
	}

	var conf Conf
	conf.LogLevel = log.InfoLevel
	conf.P4rtcIface.DefaultTC = uint8(p4constants.EnumTrafficClassElastic)

	err = json.Unmarshal(byteValue, &conf)
	if err != nil {
		return Conf{}, err
	}

	// Set defaults, when missing.
	if conf.RespTimeout == "" {
		conf.RespTimeout = respTimeoutDefault.String()
	}

	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = uint32(readTimeoutDefault.Seconds())
	}

	if conf.MaxReqRetries == 0 {
		conf.MaxReqRetries = maxReqRetriesDefault
	}

	if conf.EnableHBTimer {
		if conf.HeartBeatInterval == "" {
			conf.HeartBeatInterval = hbIntervalDefault.String()
		}
	}

	// Perform basic validation.
	err = validateConf(conf)
	if err != nil {
		return Conf{}, err
	}

	return conf, nil
}
