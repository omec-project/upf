// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"net/http"
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

func Test_setupProm(t *testing.T) {
	t.Run("can setup prom multiple times with clearProm", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		// TODO: use actual mocks
		upf := &upf{}
		node := NewPFCPNode(upf)

		uc, nc, err := setupProm(http.NewServeMux(), upf, node)
		require.NoError(t, err)

		clearProm(uc, nc)

		_, _, err = setupProm(http.NewServeMux(), upf, node)
		require.NoError(t, err)
	})

	t.Run("cannot setup prom multiple times without clearProm", func(t *testing.T) {
		saveReg()
		defer restoreReg()

		// TODO: use actual mocks
		upf := &upf{}
		node := NewPFCPNode(upf)

		_, _, err := setupProm(http.NewServeMux(), upf, node)
		require.NoError(t, err)

		_, _, err = setupProm(http.NewServeMux(), upf, node)
		require.Error(t, err)
	})
}
