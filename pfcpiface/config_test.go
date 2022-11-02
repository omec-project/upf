// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"io/fs"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustWriteStringToDisk(s string, path string) {
	err := os.WriteFile(path, []byte(s), fs.ModePerm)
	if err != nil {
		panic(err)
	}
}

func TestLoadConfigFile(t *testing.T) {
	t.Run("sample config is valid", func(t *testing.T) {
		s := `{
			"mode": "dpdk",
			"log_level": "info",
			"workers": 1,
			"max_sessions": 50000,
			"table_sizes": {
				"pdrLookup": 50000,
				"appQERLookup": 200000,
				"sessionQERLookup": 100000,
				"farLookup": 150000
			},
			"access": {
				"ifname": "access"
			},
			"core": {
				"ifname": "core"
			},
			"measure_upf": true,
			"measure_flow": true,
			"enable_notify_bess": true,
			"notify_sockaddr": "/pod-share/notifycp",
			"cpiface": {
				"dnn": "internet",
				"hostname": "upf",
				"http_port": "8080"
			},
			"n6_bps": 1000000000,
			"n6_burst_bytes": 12500000,
			"n3_bps": 1000000000,
			"n3_burst_bytes": 12500000,
			"qci_qos_config": [{
				"qci": 0,
				"cbs": 50000,
				"ebs": 50000,
				"pbs": 50000,
				"burst_duration_ms": 10,
				"priority": 7
			}]
		}`
		confPath := t.TempDir() + "/conf.json"
		mustWriteStringToDisk(s, confPath)

		_, err := LoadConfigFile(confPath)
		require.NoError(t, err)
	})

	t.Run("empty config has log level info", func(t *testing.T) {
		s := `{
			"mode": "dpdk"
		}`
		confPath := t.TempDir() + "/conf.json"
		mustWriteStringToDisk(s, confPath)

		conf, err := LoadConfigFile(confPath)
		require.NoError(t, err)
		require.Equal(t, conf.LogLevel, log.InfoLevel)
	})

	t.Run("all sample configs must be valid", func(t *testing.T) {
		paths := []string{
			"../conf/upf.json",
			"../ptf/config/upf.json",
		}

		for _, path := range paths {
			_, err := LoadConfigFile(path)
			assert.NoError(t, err, "config %v is not valid", path)
		}
	})
}
