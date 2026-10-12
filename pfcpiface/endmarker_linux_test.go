// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Intel Corporation

//go:build linux

package pfcpiface

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// endMarkerSockDir returns a fresh directory for an End Marker socket. It is
// kept in /tmp rather than t.TempDir(): a unix socket path has a hard length
// limit that the default temp directory can exceed.
func endMarkerSockDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "end-marker")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(dir) })

	return dir
}

// TestSetUpfInfoWithEndMarkerSocketAbsentAtStart starts the agent before the
// datapath has created its End Marker socket, which is what happens whenever the
// agent wins the race with bessd or bessd restarts on its own. SetUpfInfo used to
// dial once and return on failure: the rest of its setup was skipped, the send
// loop never started, and nothing ever dialled again.
//
// The dial is the real one; the test only watches it, so that the socket is
// created after a dial has found it missing rather than after a guessed delay.
//
// unixpacket is Linux-only, hence the build tag.
func TestSetUpfInfoWithEndMarkerSocketAbsentAtStart(t *testing.T) {
	sockAddr := filepath.Join(endMarkerSockDir(t), "pfcpport")

	missed := make(chan struct{})

	var missedOnce sync.Once

	var d net.Dialer

	b := &bess{
		endMarkerDial: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := d.DialContext(ctx, network, address)
			if err != nil {
				missedOnce.Do(func() { close(missed) })
			}

			return conn, err
		},
	}

	setUpfInfoWithoutDatapath(t, b, &Conf{
		EnableEndMarker:          true,
		EndMarkerSockAddr:        sockAddr,
		EnableGtpuPathMonitoring: true,
	})

	// The last step of SetUpfInfo stands in for everything after the End Marker
	// setup, slice meters included.
	if !enableGtpuPathMonitoring {
		t.Error("SetUpfInfo skipped the setup after the End Marker dial failed")
	}

	select {
	case <-missed:
	case <-time.After(5 * time.Second):
		t.Fatal("the agent never dialled the End Marker socket")
	}

	// The datapath comes up after the agent has found the socket missing.
	var lc net.ListenConfig

	ln, err := lc.Listen(context.Background(), "unixpacket", sockAddr)
	if err != nil {
		t.Fatalf("listen %v: %v", sockAddr, err)
	}

	defer ln.Close()

	accept := func(what string) net.Conn {
		t.Helper()

		if err := ln.(*net.UnixListener).SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
			t.Fatalf("set deadline: %v", err)
		}

		conn, err := ln.Accept()
		if err != nil {
			t.Fatalf("%s: the agent did not connect to the End Marker socket: %v", what, err)
		}

		return conn
	}

	expectMarker := func(conn net.Conn, want []byte, what string) {
		t.Helper()

		markers := [][]byte{want}
		if err := b.SendEndMarkers(&markers); err != nil {
			t.Fatalf("%s: SendEndMarkers: %v", what, err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}

		buf := make([]byte, 64)

		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("%s: no End Marker reached the datapath: %v", what, err)
		}

		if !bytes.Equal(buf[:n], want) {
			t.Fatalf("%s: got %x, want %x", what, buf[:n], want)
		}
	}

	conn := accept("socket created after start")
	expectMarker(conn, []byte("first"), "socket created after start")

	// The datapath restarts: its end of the connection goes away.
	conn.Close()

	conn = accept("after the datapath dropped the connection")
	defer conn.Close()

	expectMarker(conn, []byte("second"), "after the datapath dropped the connection")
}

// TestSendEndMarkersDoesNotBlockWithoutTheSocket covers what the dial-once code
// did to the PFCP side: with no send loop draining the bounded queue, the 1025th
// End Marker blocked SendEndMarkers, and with it the association's reader, for
// the life of the process.
func TestSendEndMarkersDoesNotBlockWithoutTheSocket(t *testing.T) {
	b := &bess{}
	setUpfInfoWithoutDatapath(t, b, &Conf{
		EnableEndMarker:   true,
		EndMarkerSockAddr: filepath.Join(endMarkerSockDir(t), "pfcpport"),
	})

	markers := make([][]byte, 2048)
	for i := range markers {
		markers[i] = []byte{byte(i)}
	}

	done := make(chan struct{})

	go func() {
		if err := b.SendEndMarkers(&markers); err != nil {
			t.Errorf("SendEndMarkers: %v", err)
		}

		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SendEndMarkers blocked: nothing drains the End Marker queue while the socket is absent")
	}
}

// A datapath that stops reading without closing its end fills the socket, and
// the sender's write then blocks. Stopping has to end that write, or the sender
// and its connection outlive Exit.
func TestEndMarkerLinkStopsWhileAWriteIsBlocked(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}

	// The datapath's end is held open and never read.
	datapath := os.NewFile(uintptr(fds[1]), "datapath")
	defer datapath.Close()

	agentFile := os.NewFile(uintptr(fds[0]), "agent")

	agent, err := net.FileConn(agentFile)
	agentFile.Close()

	if err != nil {
		t.Fatalf("file conn: %v", err)
	}

	dialed := false
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if dialed {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		dialed = true

		return agent, nil
	}

	markers, stopLink := startTestLink(t, dial)

	// Queue markers until the queue stops draining: the sender is then stuck in
	// a write the full socket will not accept.
	marker := make([]byte, 4096)

	for sent := 0; ; sent++ {
		if sent > 100000 {
			t.Fatal("the socket never filled")
		}

		select {
		case markers <- marker:
			continue
		case <-time.After(200 * time.Millisecond):
		}

		break
	}

	select {
	case <-stopLink():
	case <-time.After(5 * time.Second):
		t.Fatal("the sender did not stop while its write was blocked on a full socket")
	}
}
