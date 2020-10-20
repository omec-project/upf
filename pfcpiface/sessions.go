// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"math/rand"
	"time"
)

// PFCPSessionMgr manages PFCP sessions
type PFCPSessionMgr struct {
	rng        *rand.Rand
	maxRetries int
	sessions   map[uint64]PFCPSession
}

// NewPFCPSessionMgr initializes a manager struct with RNG and map of id/sessions
func NewPFCPSessionMgr(maxRetries int) *PFCPSessionMgr {
	return &PFCPSessionMgr{
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		maxRetries: maxRetries,
		sessions:   make(map[uint64]PFCPSession),
	}
}

// RemoveSession removes session using id
func (mgr *PFCPSessionMgr) RemoveSession(id uint64) {
	delete(mgr.sessions, id)
}

// PFCPSession implements one PFCP session
type PFCPSession struct {
	localSEID  uint64
	remoteSEID uint64
	pdrs       []pdr
	fars       []far
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
		}
		mgr.sessions[lseid] = s
		return lseid
	}
	return 0
}
