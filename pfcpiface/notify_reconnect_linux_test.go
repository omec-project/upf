// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pfcpiface

import (
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestNotifyListenReconnectsAfterSocketLoss covers the failure that made downlink
// data notifications stop for the life of a pod: the listener returned on the first
// read error, so the datapath went on writing notifications into a socket with no
// reader while reporting successful transmits. The second round of this test is the
// one that fails without the reconnect.
//
// unixpacket is Linux-only, hence the build tag.
func TestNotifyListenReconnectsAfterSocketLoss(t *testing.T) {
	// Kept in /tmp rather than t.TempDir(): a unix socket path has a hard length
	// limit that the default temp directory can exceed.
	dir, err := os.MkdirTemp("/tmp", "ddn-notify")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	defer os.RemoveAll(dir)

	sockAddr := filepath.Join(dir, "notifycp")

	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "unixpacket", sockAddr)
	if err != nil {
		t.Fatalf("listen %v: %v", sockAddr, err)
	}

	defer ln.Close()

	notifications := make(chan uint64, 8)

	b := &bess{notifyStop: make(chan struct{})}
	defer close(b.notifyStop)

	go b.notifyListen(notifications, sockAddr)

	// sendThenDrop accepts the listener's connection, delivers one notification,
	// and closes the connection underneath it.
	sendThenDrop := func(fseid uint64) {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}

		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, fseid)

		if _, err := conn.Write(buf); err != nil {
			t.Errorf("write: %v", err)
		}

		conn.Close()
	}

	expect := func(want uint64, within time.Duration, what string) {
		t.Helper()

		select {
		case got := <-notifications:
			if got != want {
				t.Fatalf("%s: got F-SEID %#x, want %#x", what, got, want)
			}
		case <-time.After(within):
			t.Fatalf("%s: no notification within %v", what, within)
		}
	}

	// Two distinct F-SEIDs, so the notifier's rate limiter cannot be what carries
	// or suppresses the second notification.
	go sendThenDrop(0x1111)
	expect(0x1111, 5*time.Second, "first connection")

	go sendThenDrop(0x2222)
	expect(0x2222, 4*notifyRedialInterval+5*time.Second, "after socket loss")
}
