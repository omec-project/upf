// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"sync"
	"time"
)

type downlinkDataNotifier struct {
	notifyChan chan<- uint64

	notificationInterval time.Duration

	// state keeps track of F-SEIDs and corresponding notification time in future
	state sync.Map
}

func NewDownlinkDataNotifier(notifyChan chan<- uint64, notificationInterval time.Duration) *downlinkDataNotifier {
	return &downlinkDataNotifier{
		notifyChan:           notifyChan,
		notificationInterval: notificationInterval,
	}
}

// Notify checks if DDN should be generated and sends event to notifyChan.
func (n *downlinkDataNotifier) Notify(fseid uint64) {
	if !n.shouldNotify(fseid) {
		return
	}

	n.notifyChan <- fseid
}

// shouldNotify checks if DDN can be generated.
// DDN is generated if:
// 1) notification timer has expired, or
// 2) notification for unknown F-SEID is received
func (n *downlinkDataNotifier) shouldNotify(fseid uint64) bool {
	entry, ok := n.state.Load(fseid)
	if !ok {
		n.state.Store(fseid, time.Now())
		// TODO: add goroutine that will remove stale entries
		return true
	}

	lastTimestamp := entry.(time.Time)

	if time.Since(lastTimestamp) >= n.notificationInterval {
		n.state.Store(fseid, time.Now())
		return true
	}

	return false
}
