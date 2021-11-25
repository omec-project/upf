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
	enableHBTimer  bool
	hbMaxRetries   uint8
	hbInterval     time.Duration
	hbRespDuration time.Duration
}

// to be replaced with go-pfcp structs

// Don't change these values.
const (
	tunnelGTPUPort = 2152
	invalidQerID   = 0xFFFFFFFF

	// src-iface consts.
	core   = 0x2
	access = 0x1

	// far-id specific directions.
	n3 = 0x0
	n6 = 0x1
	n9 = 0x2

	// Heart Beat Parameters
	maxHbRetries   = 5
	hbReqInterval  = 5000
	hbRespInterval = 2000
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
		enableUeIPAlloc:  conf.CPIface.EnableUeIPAlloc,
		enableEndMarker:  conf.EnableEndMarker,
		accessIface:      conf.AccessIface.IfName,
		coreIface:        conf.CoreIface.IfName,
		ippoolCidr:       conf.CPIface.UEIPPool,
		nodeID:           nodeID,
		fastPath:         fp,
		dnn:              conf.CPIface.Dnn,
		reportNotifyChan: make(chan uint64, 1024),
		enableHBTimer:    conf.EnableHBTimer,
	}

	u.accessIP = ParseIP(conf.AccessIface.IfName, "Access")
	u.coreIP = ParseIP(conf.CoreIface.IfName, "Core")

	if u.enableHBTimer {
		u.hbMaxRetries = maxHbRetries
		if conf.HbMaxRetries != 0 {
			u.hbMaxRetries = conf.HbMaxRetries
		}

		u.hbInterval = time.Duration(hbReqInterval) * time.Millisecond
		if conf.HeartBeatInterval != 0 {
			u.hbInterval = time.Duration(conf.HeartBeatInterval) * time.Millisecond
		}

		u.hbRespDuration = time.Duration(hbRespInterval) * time.Millisecond
		if conf.HeartBeatRespDuration != 0 {
			u.hbRespDuration = time.Duration(conf.HeartBeatRespDuration) * time.Millisecond
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
