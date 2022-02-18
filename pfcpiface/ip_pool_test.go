// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"math"
	"net"
	"sync"
	"testing"
)

func TestNewIPPool(t *testing.T) {
	tests := []struct {
		name       string
		poolSubnet string
		wantErr    bool
	}{
		{name: "normal pool", poolSubnet: "10.0.0.0/24", wantErr: false},
		{name: "smallest allowed pool", poolSubnet: "10.0.0.0/30", wantErr: false},
		{name: "IPv6 pool", poolSubnet: "2001::/124", wantErr: false},
		{name: "too small pool", poolSubnet: "10.0.0.0/32", wantErr: true},
		{name: "missing subnet", poolSubnet: "", wantErr: true},
		{name: "invalid subnet", poolSubnet: "foobar", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				_, err := NewIPPool(tt.poolSubnet)
				if !tt.wantErr {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			},
		)
	}
}

func TestIPPool_LookupOrAllocIP(t *testing.T) {
	t.Run("allocation in IPv6 subnet", func(t *testing.T) {
		const poolSubnet = "2001::/124"
		const seid = 1234
		pool, err := NewIPPool(poolSubnet)
		require.NoError(t, err)
		ip, err := pool.LookupOrAllocIP(seid)
		require.NoError(t, err)
		require.Len(t, ip, net.IPv6len)
	})

	t.Run("repeated SEID lookups return same IP", func(t *testing.T) {
		const poolSubnet = "10.0.0.0/24"
		const seid = 1234
		pool, err := NewIPPool(poolSubnet)
		require.NoError(t, err)
		ip1, err := pool.LookupOrAllocIP(seid)
		require.NoError(t, err)
		ip2, err := pool.LookupOrAllocIP(seid)
		require.NoError(t, err)
		require.Equal(t, ip2, ip1)
		require.Len(t, ip1, net.IPv4len)
	})

	t.Run("full subnet allocation", func(t *testing.T) {
		const poolSubnet = "10.0.0.0/24"
		const usableAddresses = 256 - 2 // Account for network and broadcast addresses
		const baseSeid = 1000
		_, ipnet, err := net.ParseCIDR(poolSubnet)
		require.NoError(t, err)
		pool, err := NewIPPool(poolSubnet)
		require.NoError(t, err)
		seidToIpMap := map[uint64]net.IP{}
		for i := uint64(0); i < usableAddresses; i++ {
			ip, err := pool.LookupOrAllocIP(baseSeid + i)
			require.NoError(t, err)
			seidToIpMap[baseSeid+i] = ip
		}

		for _, ip := range seidToIpMap {
			require.True(t, ipnet.Contains(ip), "allocated ip %v not in subnet %v", ip, ipnet)
		}

		_, err = pool.LookupOrAllocIP(baseSeid + usableAddresses + 1)
		require.Error(t, err, "ip alloc should fail after subnet has been exhausted")

		for seid, ip := range seidToIpMap {
			lookupIP, err := pool.LookupOrAllocIP(seid)
			require.NoError(t, err, "already allocated IPs must be still be lookup-able")
			require.Equal(t, ip, lookupIP, "looked up IP for SEID %v changed", seid)
		}
	})

	t.Run("concurrent allocation", func(t *testing.T) {
		const workers = 4
		const seidsPerWorker = 5000
		pool, err := NewIPPool("10.0.0.0/16")
		require.NoError(t, err)
		wg := sync.WaitGroup{}
		worker := func(startSeid uint64) {
			for seid := startSeid; seid < startSeid+seidsPerWorker; seid++ {
				_, err := pool.LookupOrAllocIP(seid)
				require.NoError(t, err)
			}
			wg.Done()
		}
		for i := uint64(0); i < workers; i++ {
			wg.Add(1)
			go worker(i * seidsPerWorker)
		}
		wg.Wait()
	})
}

func TestIPPool_DeallocIP(t *testing.T) {
	t.Run("plain alloc into dealloc", func(t *testing.T) {
		const poolSubnet = "10.0.0.0/24"
		const seid = 1234
		pool, err := NewIPPool(poolSubnet)
		require.NoError(t, err)
		_, err = pool.LookupOrAllocIP(seid)
		require.NoError(t, err)
		err = pool.DeallocIP(seid)
		require.NoError(t, err)
	})

	t.Run("dealloc non-existent SEIDs fails", func(t *testing.T) {
		pool, err := NewIPPool("10.0.0.0/24")
		require.NoError(t, err)
		err = pool.DeallocIP(1234)
		assert.Error(t, err)
		err = pool.DeallocIP(0)
		assert.Error(t, err)
		err = pool.DeallocIP(math.MaxUint64)
		assert.Error(t, err)
	})
}
