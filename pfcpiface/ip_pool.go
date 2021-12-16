// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

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
	// inventory keeps track of allocated sessions and their IPs
	inventory map[uint64]net.IP
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

func (i *IPPool) LookupOrAllocIP(seid uint64) (net.IP, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.freePool) == 0 {
		return nil, ErrOperationFailedWithReason("IP allocation", "ip pool empty")
	}

	// Try to find an exiting session and return the allocated IP.
	ip, found := i.inventory[seid]
	if found {
		log.Traceln("Found existing session", seid, "IP", ip)
		return ip, nil
	}

	ip = i.freePool[0]
	i.freePool = i.freePool[1:] // Slice off the element once it is dequeued.
	i.inventory[seid] = ip
	log.Traceln("Allocated new session", seid, "IP", ip)

	ipVal := make(net.IP, len(ip))
	copy(ipVal, ip)

	return ipVal, nil
}

func (i *IPPool) String() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	sb := strings.Builder{}
	sb.WriteString("Inventory:\n")
	for s, e := range i.inventory {
		sb.WriteString(fmt.Sprintf("\tSEID %v -> %+v\n", s, e))
	}

	sb.WriteString("Free Pool:\n")
	for _, ip := range i.freePool {
		sb.WriteString(fmt.Sprintf("\tIP %s\n", ip.String()))
	}

	return sb.String()
}

func NewIPPool(cidr string) (*IPPool, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
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

	// remove network address and broadcast address
	i.freePool = i.freePool[1 : len(i.freePool)-1]

	return i, nil
}
