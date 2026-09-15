// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package pfcpiface

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/omec-project/upf-epc/logger"
)

type IPPool struct {
	mu       sync.Mutex
	freePool []net.IP
	// inventory keeps track of allocated sessions and their IPs.
	inventory map[uint64]net.IP
}

// NewIPPool creates a new pool of IP addresses with the given subnet.
// The smallest supported size is a /30.
func NewIPPool(poolSubnet string) (*IPPool, error) {
	ip, ipnet, err := net.ParseCIDR(poolSubnet)
	if err != nil {
		return nil, err
	}

	i := &IPPool{
		inventory: make(map[uint64]net.IP),
	}

	for ip = ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ipVal := make(net.IP, len(ip))
		copy(ipVal, ip)
		i.freePool = append(i.freePool, ipVal)
	}

	if len(i.freePool) < 2 {
		return nil, ErrInvalidArgumentWithReason("NewIPPool", poolSubnet, "pool subnet is too small to use as a pool")
	}

	// Remove network address and broadcast address.
	i.freePool = i.freePool[1 : len(i.freePool)-1]

	return i, nil
}

func (i *IPPool) LookupOrAllocIP(seid uint64) (net.IP, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Try to find an exiting session and return the allocated IP.
	ip, found := i.inventory[seid]
	if found {
		logger.PfcpLog.Debugln("found existing session", seid, "IP", ip)
		return ip, nil
	}

	// Check capacity before new allocations.
	if len(i.freePool) == 0 {
		return nil, ErrOperationFailedWithReason("IP allocation", "ip pool empty")
	}

	ip = i.freePool[0]
	i.freePool = i.freePool[1:] // Slice off the element once it is dequeued.
	i.inventory[seid] = ip
	logger.PfcpLog.Debugln("allocated new session", seid, "IP", ip)

	ipVal := make(net.IP, len(ip))
	copy(ipVal, ip)

	return ipVal, nil
}

// Release returns the address held for seid to the pool and reports whether there was
// one to return. The inventory is the only record of an allocation that every session
// carries for as long as it exists, which is why the release is keyed by SEID: a
// session's rules are not. A session for which the UPF allocated nothing -- the control
// plane supplied the UE address -- has nothing to release, and that is the ordinary
// case rather than a failure.
func (i *IPPool) Release(seid uint64) bool {
	// UE IP allocation is optional: initTimersAndIPPool builds a pool only when
	// enable_ue_ip_alloc is set, so a deployment whose control plane assigns the
	// addresses has no pool at all. Every caller is on a teardown path that runs
	// either way, so the absent pool is answered here rather than at each of them.
	if i == nil {
		return false
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	ip, ok := i.inventory[seid]
	if !ok {
		return false
	}

	delete(i.inventory, seid)
	i.freePool = append(i.freePool, ip) // Simply append to enqueue.
	logger.PfcpLog.Debugln("released session", seid, "IP", ip)

	return true
}

func (i *IPPool) String() string {
	i.mu.Lock()
	defer i.mu.Unlock()

	sb := strings.Builder{}
	sb.WriteString("inventory: ")

	for s, e := range i.inventory {
		fmt.Fprintf(&sb, "{F-SEID %v -> %+v} ", s, e)
	}

	fmt.Fprintf(&sb, "Number of free IP addresses left: %d", len(i.freePool))

	return sb.String()
}
