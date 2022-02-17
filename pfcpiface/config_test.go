// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

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

func TestLoadConfigFile(t *testing.T) {
	t.Run("sample config is valid", func(t *testing.T) {
		s := `{
			"log_level": "info",
			"cpiface": {
				"dnn": "internet",
				"node_id": "upf",
				"http_port": "8080"
			},
			"measure_flow": true,
			"fastpath": "bess",
			"bess": {
				"mode": "dpdk",
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
				"enable_notify_bess": true,
				"notify_sockaddr": "/pod-share/notifycp",
				"qfi_qos_config": [{
					"qfi": 0,
					"cbs": 50000,
					"ebs": 50000,
					"pbs": 50000,
					"burst_duration_ms": 10,
					"priority": 7
				}]
			}
		}`
		confPath := t.TempDir() + "/conf.json"
		mustWriteStringToDisk(s, confPath)

		_, err := LoadConfigFile(confPath)
		require.NoError(t, err)
	})

	t.Run("all sample configs must be valid", func(t *testing.T) {
		paths := []string{
			"../conf/upf.json",
			"../ptf/config/upf.json",
			"../test/integration/config/default.json",
			"../test/integration/config/ue_ip_alloc.json",
		}

		for _, path := range paths {
			_, err := LoadConfigFile(path)
			assert.NoError(t, err, "config %v is not valid", path)
		}
	})
}
