// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package metrics

import (
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

func TestNewPrometheusService(t *testing.T) {
	t.Run("cannot register multiple times without stop", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		_, err := NewPrometheusService()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = NewPrometheusService()
		if err == nil {
			t.Fatal("expected an error, but got nil")
		}
	})

	t.Run("can register multiple times with stop", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		var s *Service
		s, err := NewPrometheusService()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = s.Stop()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = NewPrometheusService()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// A failed start used to leave the collectors before the failing one
// registered, so a retry failed even after the conflict was gone.
func TestNewPrometheusServiceRollsBackOnFailure(t *testing.T) {
	saveReg()
	defer restoreReg()

	// Register the third collector it registers ahead of it. The registry
	// remembers a name's help and labels after it's unregistered, so this has to
	// match them.
	conflict := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pfcp_sessions",
		Help: "Number of PFCP sessions currently in the UPF",
	}, []string{labelNodeID})
	if err := prometheus.Register(conflict); err != nil {
		t.Fatalf("registering the conflict: %v", err)
	}

	if _, err := NewPrometheusService(); err == nil {
		t.Fatal("NewPrometheusService succeeded despite the conflict")
	}

	prometheus.Unregister(conflict)

	s, err := NewPrometheusService()
	if err != nil {
		t.Fatalf("NewPrometheusService after the conflict was removed: %v", err)
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
