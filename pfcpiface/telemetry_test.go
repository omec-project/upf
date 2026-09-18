// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TODO: we currently need to reset the DefaultRegisterer between tests, as some
// leave the registry in a bad state. Use custom registries to avoid global state.
var backupGlobalRegistry prometheus.Registerer

func saveReg() {
	backupGlobalRegistry = prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
}

func restoreReg() {
	prometheus.DefaultRegisterer = backupGlobalRegistry
}

// newTestPFCPNode builds a *PFCPNode the way NewPFCPNode does, but on an ephemeral
// local port instead of the hardcoded production PFCPPort (":8805"). setupProm only
// reads node.upf, so the node needs no real PFCP listener -- and binding the real port
// makes the test collide with any other PFCP endpoint already running on the host (and
// NewPFCPNode calls log.Fatalln on a bind failure, which kills the whole test binary
// rather than just this test). node.metrics is left nil since setupProm never reads it,
// avoiding the extra global Prometheus registrations a real metrics service would add.
func newTestPFCPNode(t *testing.T, u *upf) *PFCPNode {
	t.Helper()

	var lc net.ListenConfig

	conn, err := lc.ListenPacket(context.Background(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open test PFCP socket: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	node := &PFCPNode{
		ctx:        ctx,
		cancel:     cancel,
		PacketConn: conn,
		done:       make(chan struct{}),
		upf:        u,
	}

	t.Cleanup(func() {
		if err := node.Close(); err != nil {
			t.Errorf("failed to close node: %v", err)
		}

		if node.metrics != nil {
			if err := node.metrics.Stop(); err != nil {
				t.Errorf("failed to stop metrics: %v", err)
			}
		}
	})

	return node
}

func Test_setupProm(t *testing.T) {
	t.Run("can setup prom multiple times with clearProm", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		// TODO: use actual mocks
		upf := &upf{}
		node := newTestPFCPNode(t, upf)

		uc, nc, err := setupProm(http.NewServeMux(), upf, node)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		clearProm(uc, nc)

		_, _, err = setupProm(http.NewServeMux(), upf, node)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cannot setup prom multiple times without clearProm", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		// TODO: use actual mocks
		upf := &upf{}
		node := newTestPFCPNode(t, upf)

		_, _, err := setupProm(http.NewServeMux(), upf, node)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, _, err = setupProm(http.NewServeMux(), upf, node)
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
	})
}
