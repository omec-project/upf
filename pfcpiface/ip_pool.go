// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package pfcpiface

import (
	"fmt"
	"net"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
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

	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
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
		log.Traceln("Found existing session", seid, "IP", ip)
		return ip, nil
	}

	// Check capacity before new allocations.
	if len(i.freePool) == 0 {
		return nil, ErrOperationFailedWithReason("IP allocation", "ip pool empty")
	}

	ip = i.freePool[0]
	i.freePool = i.freePool[1:] // Slice off the element once it is dequeued.
	i.inventory[seid] = ip
	log.Traceln("Allocated new session", seid, "IP", ip)

	ipVal := make(net.IP, len(ip))
	copy(ipVal, ip)

	return ipVal, nil
}

func (i *IPPool) DeallocIP(seid uint64) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	ip, ok := i.inventory[seid]
	if !ok {
		log.Warnln("Attempt to dealloc non-existent session", seid)
		return ErrInvalidArgumentWithReason("seid", seid, "can't dealloc non-existent session")
	}

	delete(i.inventory, seid)
	i.freePool = append(i.freePool, ip) // Simply append to enqueue.
	log.Traceln("Deallocated session ", seid, "IP", ip)

	return nil
}

func (i *IPPool) String() string {
	i.mu.Lock()
	defer i.mu.Unlock()

	sb := strings.Builder{}
	sb.WriteString("inventory: ")

	for s, e := range i.inventory {
		sb.WriteString(fmt.Sprintf("{F-SEID %v -> %+v} ", s, e))
	}

	sb.WriteString(fmt.Sprintf("Number of free IP addresses left: %d", len(i.freePool)))

	return sb.String()
}
