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
