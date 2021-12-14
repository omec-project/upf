// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"net"
	"time"

	"github.com/Showmax/go-fqdn"
	log "github.com/sirupsen/logrus"
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
	accessIface       string
	coreIface         string
	ippoolCidr        string
	accessIP          net.IP
	coreIP            net.IP
	nodeID            string
	ippool            *IPPool
	dnn               string
	reportNotifyChan  chan uint64
	sliceInfo         *SliceInfo

	fastPath
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

	// Default values for outgoing requests
	maxReqRetriesDefault = 5
	respTimeoutDefault   = 2 * time.Second

	// Default value for Heart Beat Interval
	hbIntervalDefault = 5 * time.Second
)

func (u *upf) isConnected() bool {
	return u.fastPath.isConnected(&u.accessIP)
}

func (u *upf) addSliceInfo(sliceInfo *SliceInfo) error {
	if sliceInfo == nil {
		return errors.New("invalid slice")
	}

	u.sliceInfo = sliceInfo

	return u.fastPath.addSliceInfo(sliceInfo)
}

func NewUPF(conf *Conf, fp fastPath) *upf {
	var (
		err    error
		nodeID string
	)

	nodeID = conf.CPIface.NodeID
	if conf.CPIface.UseFQDN && nodeID == "" {
		nodeID, err = fqdn.FqdnHostname()
		if err != nil {
			log.Fatalln("Unable to get hostname", err)
		}
	}

	// TODO: Delete this once CI config is fixed
	if nodeID != "" {
		hosts, err := net.LookupHost(nodeID)
		if err != nil {
			log.Fatalln("Unable to resolve hostname", nodeID, err)
		}

		nodeID = hosts[0]
	}

	u := &upf{
		enableUeIPAlloc:   conf.CPIface.EnableUeIPAlloc,
		enableEndMarker:   conf.EnableEndMarker,
		enableFlowMeasure: conf.EnableFlowMeasure,
		accessIface:       conf.AccessIface.IfName,
		coreIface:         conf.CoreIface.IfName,
		ippoolCidr:        conf.CPIface.UEIPPool,
		nodeID:            nodeID,
		fastPath:          fp,
		dnn:               conf.CPIface.Dnn,
		reportNotifyChan:  make(chan uint64, 1024),
		maxReqRetries:     maxReqRetriesDefault,
		respTimeout:       respTimeoutDefault,
		enableHBTimer:     conf.EnableHBTimer,
		hbInterval:        hbIntervalDefault,
	}

	if !conf.EnableP4rt {
		u.accessIP = ParseIP(conf.AccessIface.IfName, "Access")
		u.coreIP = ParseIP(conf.CoreIface.IfName, "Core")
	}

	if conf.MaxReqRetries != 0 {
		u.maxReqRetries = conf.MaxReqRetries
	}

	if conf.RespTimeout != "" {
		u.respTimeout, err = time.ParseDuration(conf.RespTimeout)
		if err != nil {
			log.Fatalln("Unable to parse resp_timeout")
		}
	}

	if u.enableHBTimer {
		if conf.HeartBeatInterval != "" {
			u.hbInterval, err = time.ParseDuration(conf.HeartBeatInterval)
			if err != nil {
				log.Fatalln("Unable to parse heart_beat_interval")
			}
		}
	}

	if u.enableUeIPAlloc {
		u.ippool, err = NewIPPool(u.ippoolCidr)
		if err != nil {
			log.Fatalln("ip pool init failed", err)
		}
	}

	u.fastPath.setUpfInfo(u, conf)

	return u
}
