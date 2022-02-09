// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"io/fs"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustWriteStringToDisk(s string, path string) {
	err := ioutil.WriteFile(path, []byte(s), fs.ModePerm)
	if err != nil {
		panic(err)
	}
}

func TestParseJSON(t *testing.T) {
	t.Run("config is preserved", func(t *testing.T) {
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

		got, err := ParseJSON(confPath)
		require.NoError(t, err)
		assert.Equal(t, "info", got.LogLevel)
	})
}
