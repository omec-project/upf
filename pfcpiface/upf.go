// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/omec-project/upf-epc/logger"
)

// QosConfigVal : Qos configured value.
type QosConfigVal struct {
	cbs              uint32
	pbs              uint32
	ebs              uint32
	burstDurationMs  uint32
	schedulePriority uint32
}

type SliceInfo struct {
	name         string
	uplinkMbr    uint64
	downlinkMbr  uint64
	ulBurstBytes uint64
	dlBurstBytes uint64
	ueResList    []UeResource
}

type UeResource struct {
	name string
	dnn  string
}

type upf struct {
	enableUeIPAlloc   bool
	enableEndMarker   bool
	enableFlowMeasure bool
	enableGtpuMonitor bool
	accessIface       string
	coreIface         string
	ippoolCidr        string
	n4addr            string
	accessIP          net.IP
	coreIP            net.IP
	nodeID            string
	ippool            *IPPool
	peers             []string
	dnn               string
	reportNotifyChan  chan uint64
	sliceInfo         *SliceInfo
	readTimeout       time.Duration
	fteidGenerator    *FTEIDGenerator

	datapath
	maxReqRetries uint8
	respTimeout   time.Duration
	enableHBTimer bool
	hbInterval    time.Duration
}

// to be replaced with go-pfcp structs

// Don't change these values.
const (
	tunnelGTPUPort = 2152

	// src-iface consts.
	core   = 0x2
	access = 0x1

	// far-id specific directions.
	n3 = 0x0
	n6 = 0x1
	n9 = 0x2
)

func (u *upf) isConnected() bool {
	return u.IsConnected(&u.accessIP)
}

func (u *upf) addSliceInfo(sliceInfo *SliceInfo) error {
	if sliceInfo == nil {
		return ErrInvalidArgument("sliceInfo", sliceInfo)
	}

	u.sliceInfo = sliceInfo

	return u.AddSliceInfo(sliceInfo)
}

func fqdnHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	// if hostname is already FQDN, return it
	if strings.Contains(hostname, ".") {
		return hostname, nil
	}

	// try to get FQDN via reverse DNS lookup
	addrs, err := net.LookupIP(hostname)
	if err != nil {
		logger.PfcpLog.Warnf("failed to get fqdn for %s: %+v", hostname, err)
		return hostname, nil // fallback to short hostname
	}

	for _, addr := range addrs {
		names, err := net.LookupAddr(addr.String())
		if err != nil || len(names) == 0 {
			continue
		}

		// return the first FQDN found
		fqdn := strings.TrimSuffix(names[0], ".")
		if strings.Contains(fqdn, ".") {
			return fqdn, nil
		}
	}

	return hostname, nil // fallback to short hostname
}

func NewUPF(conf *Conf, fp datapath) *upf {
	var (
		err    error
		nodeID string
		hosts  []string
	)

	nodeID = conf.CPIface.NodeID
	if conf.CPIface.UseFQDN && nodeID == "" {
		nodeID, err = fqdnHostname()
		if err != nil {
			logger.PfcpLog.Fatalln("unable to get hostname", err)
		}
	}

	// TODO: Delete this once CI config is fixed
	if nodeID != "" {
		hosts, err = net.LookupHost(nodeID)
		if err != nil {
			logger.PfcpLog.Fatalln("unable to resolve hostname", nodeID, err)
		}

		nodeID = hosts[0]
	}

	u := &upf{
		enableUeIPAlloc:   conf.CPIface.EnableUeIPAlloc,
		enableEndMarker:   conf.EnableEndMarker,
		enableFlowMeasure: conf.EnableFlowMeasure,
		enableGtpuMonitor: conf.EnableGtpuPathMonitoring,
		accessIface:       conf.AccessIface.IfName,
		coreIface:         conf.CoreIface.IfName,
		ippoolCidr:        conf.CPIface.UEIPPool,
		nodeID:            nodeID,
		datapath:          fp,
		dnn:               conf.CPIface.Dnn,
		peers:             conf.CPIface.Peers,
		reportNotifyChan:  make(chan uint64, 1024),
		maxReqRetries:     conf.MaxReqRetries,
		enableHBTimer:     conf.EnableHBTimer,
		readTimeout:       time.Second * time.Duration(conf.ReadTimeout),
		fteidGenerator:    NewFTEIDGenerator(),
		n4addr:            conf.N4Addr,
	}

	if len(conf.CPIface.Peers) > 0 {
		u.peers = make([]string, len(conf.CPIface.Peers))
		nc := copy(u.peers, conf.CPIface.Peers)

		if nc == 0 {
			logger.PfcpLog.Warnln("failed to parse cpiface peers, PFCP Agent will not initiate connection to N4 peers.")
		}
	}

	u.accessIP, err = GetUnicastAddressFromInterface(conf.AccessIface.IfName)
	if err != nil {
		logger.PfcpLog.Errorln(err)
		return nil
	}

	u.coreIP, err = GetUnicastAddressFromInterface(conf.CoreIface.IfName)
	if err != nil {
		logger.PfcpLog.Errorln(err)
		return nil
	}

	u.respTimeout, err = time.ParseDuration(conf.RespTimeout)
	if err != nil {
		logger.PfcpLog.Fatalln("unable to parse resp_timeout")
	}

	if u.enableHBTimer {
		if conf.HeartBeatInterval != "" {
			u.hbInterval, err = time.ParseDuration(conf.HeartBeatInterval)
			if err != nil {
				logger.PfcpLog.Fatalln("unable to parse heart_beat_interval")
			}
		}
	}

	if u.enableUeIPAlloc {
		u.ippool, err = NewIPPool(u.ippoolCidr)
		if err != nil {
			logger.PfcpLog.Fatalln("ip pool init failed", err)
		}
	}

	u.SetUpfInfo(u, conf)

	return u
}
