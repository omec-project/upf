// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func Test_downlinkDataNotifier_Notify(t *testing.T) {
	n := &downlinkDataNotifier{
		notifyChan:           make(chan<- uint64, 1024),
		notificationInterval: 5 * time.Second,
	}
	
	testFSEID := uint64(0x1)

	n.Notify(testFSEID)
	require.Len(t, n.notifyChan, 1)
	n.Notify(testFSEID)
	// we haven't picked any event from channel, so length should be the same.
	require.Len(t, n.notifyChan, 1)
}

func Test_downlinkDataNotifier_shouldNotify(t *testing.T) {
	t.Run("single F-SEID check rate limiting", func(t *testing.T) {
		n := &downlinkDataNotifier{
			notifyChan:           make(chan<- uint64, 1024),
			notificationInterval: 5 * time.Second,
		}
		testFSEID := uint64(0x1)

		got := n.shouldNotify(testFSEID)
		require.True(t, got)
		<-time.After(3*time.Second)

		got = n.shouldNotify(testFSEID)
		require.False(t, got)
		<-time.After(1*time.Second)

		got = n.shouldNotify(testFSEID)
		require.False(t, got)
		<-time.After(2*time.Second)

		// after ~6 seconds
		got = n.shouldNotify(testFSEID)
		require.True(t, got)

		got = n.shouldNotify(testFSEID)
		require.False(t, got)
		<-time.After(1*time.Second)
	})

	t.Run("multiple F-SEIDs check rate limiting", func(t *testing.T) {
		n := &downlinkDataNotifier{
			notifyChan:           make(chan<- uint64, 1024),
			notificationInterval: 5 * time.Second,
		}

		testFSEIDs := []uint64{0x1, 0x2, 0x3, 0x4, 0x5}

		for _, fseid := range testFSEIDs {
			got := n.shouldNotify(fseid)
			require.True(t, got)
		}

		<-time.After(3*time.Second)

		for _, fseid := range testFSEIDs {
			got := n.shouldNotify(fseid)
			require.False(t, got)
		}

		<-time.After(3*time.Second)

		for _, fseid := range testFSEIDs {
			got := n.shouldNotify(fseid)
			require.True(t, got)
		}

		<-time.After(1*time.Second)

		for _, fseid := range testFSEIDs {
			got := n.shouldNotify(fseid)
			require.False(t, got)
		}
	})
}
