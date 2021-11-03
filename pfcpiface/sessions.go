// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"sync"
)

type notifyFlag struct {
	flag bool
	mux  sync.Mutex
}

// PFCPSession implements one PFCP session.
type PFCPSession struct {
	localSEID        uint64
	remoteSEID       uint64
	notificationFlag notifyFlag
	pdrs             []pdr
	fars             []far
	qers             []qer
}

// NewPFCPSession allocates an session with ID.
func (pConn *PFCPConn) NewPFCPSession(rseid uint64) uint64 {
	for i := 0; i < pConn.maxRetries; i++ {
		lseid := pConn.rng.Uint64()
		// Check if it already exists
		if _, ok := pConn.sessions[lseid]; ok {
			continue
		}

		s := PFCPSession{
			localSEID:  lseid,
			remoteSEID: rseid,
			pdrs:       make([]pdr, 0, MaxItems),
			fars:       make([]far, 0, MaxItems),
			qers:       make([]qer, 0, MaxItems),
		}
		pConn.sessions[lseid] = &s
		globalPfcpStats.sessions.WithLabelValues(pConn.nodeID.remote).Set(float64(len(pConn.sessions)))

		return lseid
	}

	return 0
}

// RemoveSession removes session using id.
func (pConn *PFCPConn) RemoveSession(id uint64) {
	delete(pConn.sessions, id)
	globalPfcpStats.sessions.WithLabelValues(pConn.nodeID.remote).Set(float64(len(pConn.sessions)))
}
