// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"math/rand"
	"sync"
	"time"
)

// PFCPSessionMgr manages PFCP sessions
type PFCPSessionMgr struct {
	rng        *rand.Rand
	nodeID     string
	maxRetries int
	appPFDs    map[string]appPFD
	sessions   map[uint64]*PFCPSession
}

// PFD holds the switch level application IDs
type appPFD struct {
	appID     string
	flowDescs []string
}

// NewPFCPSessionMgr initializes a manager struct with RNG and map of id/sessions
func NewPFCPSessionMgr(maxRetries int) *PFCPSessionMgr {
	return &PFCPSessionMgr{
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		maxRetries: maxRetries,
		sessions:   make(map[uint64]*PFCPSession),
	}
}

// RemoveSession removes session using id
func (mgr *PFCPSessionMgr) RemoveSession(id uint64) {
	delete(mgr.sessions, id)
	globalPfcpStats.sessions.WithLabelValues(mgr.nodeID).Set(float64(len(mgr.sessions)))
}

type notifyFlag struct {
	flag bool
	mux  sync.Mutex
}

// PFCPSession implements one PFCP session
type PFCPSession struct {
	localSEID        uint64
	remoteSEID       uint64
	notificationFlag notifyFlag
	pdrs             []pdr
	fars             []far
	qers             []qer
}

// NewPFCPSession allocates an session with ID
func (mgr *PFCPSessionMgr) NewPFCPSession(rseid uint64) uint64 {
	for i := 0; i < mgr.maxRetries; i++ {
		lseid := mgr.rng.Uint64()
		// Check if it already exists
		if _, ok := mgr.sessions[lseid]; ok {
			continue
		}

		s := PFCPSession{
			localSEID:  lseid,
			remoteSEID: rseid,
			pdrs:       make([]pdr, 0, MaxItems),
			fars:       make([]far, 0, MaxItems),
			qers:       make([]qer, 0, MaxItems),
		}
		mgr.sessions[lseid] = &s
		globalPfcpStats.sessions.WithLabelValues(mgr.nodeID).Set(float64(len(mgr.sessions)))
		return lseid
	}
	return 0
}

// ResetAppPFDs resets the map of application PFDs
func (mgr *PFCPSessionMgr) ResetAppPFDs() {
	mgr.appPFDs = make(map[string]appPFD)
}

// NewAppPFD stores app PFD in session mgr
func (mgr *PFCPSessionMgr) NewAppPFD(appID string) {
	mgr.appPFDs[appID] = appPFD{
		appID:     appID,
		flowDescs: make([]string, 0, MaxItems),
	}
}

// RemoveAppPFD removes appPFD using appID
func (mgr *PFCPSessionMgr) RemoveAppPFD(appID string) {
	delete(mgr.appPFDs, appID)
}
