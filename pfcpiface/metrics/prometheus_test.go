// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"testing"
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
		require.NoError(t, err)

		_, err = NewPrometheusService()
		require.Error(t, err)
	})

	t.Run("can register multiple times with stop", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		var s *Service
		s, err := NewPrometheusService()
		require.NoError(t, err)

		err = s.Stop()
		require.NoError(t, err)

		_, err = NewPrometheusService()
		require.NoError(t, err)
	})
}
