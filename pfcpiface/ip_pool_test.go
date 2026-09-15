// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"math"
	"net"
	"reflect"
	"sync"
	"testing"
)

const ipSubnetCIDR = "10.0.0.0/24"

func TestNewIPPool(t *testing.T) {
	tests := []struct {
		name       string
		poolSubnet string
		wantErr    bool
	}{
		{name: "normal pool", poolSubnet: ipSubnetCIDR, wantErr: false},
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
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
				} else {
					if err == nil {
						t.Fatal("expected an error, but got nil")
					}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ip, err := pool.LookupOrAllocIP(seid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ip) != net.IPv6len {
			t.Fatalf("expected IP length %d, got %d", net.IPv6len, len(ip))
		}
	})

	t.Run("repeated SEID lookups return same IP", func(t *testing.T) {
		const poolSubnet = ipSubnetCIDR
		const seid = 1234
		pool, err := NewIPPool(poolSubnet)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ip1, err := pool.LookupOrAllocIP(seid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ip2, err := pool.LookupOrAllocIP(seid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(ip2, ip1) {
			t.Fatalf("expected %v, got %v", ip1, ip2)
		}
		if len(ip1) != net.IPv4len {
			t.Fatalf("expected IP length %d, got %d", net.IPv4len, len(ip1))
		}
	})

	t.Run("full subnet allocation", func(t *testing.T) {
		const poolSubnet = ipSubnetCIDR
		const usableAddresses = 256 - 2 // Account for network and broadcast addresses
		const baseSeid = 1000
		_, ipnet, err := net.ParseCIDR(poolSubnet)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		pool, err := NewIPPool(poolSubnet)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		seidToIpMap := map[uint64]net.IP{}
		var ip net.IP
		for i := uint64(0); i < usableAddresses; i++ {
			ip, err = pool.LookupOrAllocIP(baseSeid + i)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			seidToIpMap[baseSeid+i] = ip
		}

		for _, ip := range seidToIpMap {
			if !ipnet.Contains(ip) {
				t.Fatalf("allocated ip %v not in subnet %v", ip, ipnet)
			}
		}

		_, err = pool.LookupOrAllocIP(baseSeid + usableAddresses + 1)
		if err == nil {
			t.Fatal("ip alloc should fail after subnet has been exhausted")
		}

		for seid, ip := range seidToIpMap {
			lookupIP, err := pool.LookupOrAllocIP(seid)
			if err != nil {
				t.Fatalf("already allocated IPs must be still be lookup-able: %v", err)
			}
			if !reflect.DeepEqual(ip, lookupIP) {
				t.Fatalf("looked up IP for SEID %v changed: expected %v, got %v", seid, ip, lookupIP)
			}
		}
	})

	t.Run("concurrent allocation", func(t *testing.T) {
		const workers = 4
		const seidsPerWorker = 5000
		pool, err := NewIPPool("10.0.0.0/16")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wg := sync.WaitGroup{}
		worker := func(startSeid uint64) {
			for seid := startSeid; seid < startSeid+seidsPerWorker; seid++ {
				_, err := pool.LookupOrAllocIP(seid)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
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

func TestIPPool_Release(t *testing.T) {
	t.Run("plain alloc into release", func(t *testing.T) {
		const poolSubnet = ipSubnetCIDR
		const seid = 1234
		pool, err := NewIPPool(poolSubnet)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, err = pool.LookupOrAllocIP(seid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !pool.Release(seid) {
			t.Fatal("expected the allocated address to be released")
		}
	})

	t.Run("releasing a SEID that holds nothing is not a failure", func(t *testing.T) {
		pool, err := NewIPPool(ipSubnetCIDR)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, seid := range []uint64{1234, 0, math.MaxUint64} {
			if pool.Release(seid) {
				t.Errorf("SEID %d held no address, but Release reported one", seid)
			}
		}
	})

	t.Run("a deployment with no pool has nothing to release", func(t *testing.T) {
		// enable_ue_ip_alloc is off, so initTimersAndIPPool never built a pool and
		// upf.ippool is nil. The teardown paths call Release regardless.
		var pool *IPPool
		if pool.Release(1234) {
			t.Error("a nil pool reported that it released an address")
		}
	})

	t.Run("a released address is handed out again", func(t *testing.T) {
		// A pool of exactly two addresses: capacity is what distinguishes a released
		// address from a leaked one, because Release enqueues at the back of the free
		// pool while LookupOrAllocIP takes from the front.
		pool, err := NewIPPool("10.251.0.0/30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for seid := uint64(1); seid <= 2; seid++ {
			if _, err := pool.LookupOrAllocIP(seid); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		if _, err := pool.LookupOrAllocIP(3); err == nil {
			t.Fatal("expected the pool to be exhausted")
		}
		if !pool.Release(1) {
			t.Fatal("expected the allocated address to be released")
		}
		if _, err := pool.LookupOrAllocIP(3); err != nil {
			t.Fatalf("the released address was not handed out again: %v", err)
		}
	})
}
