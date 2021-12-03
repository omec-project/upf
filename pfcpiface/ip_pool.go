// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"errors"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
)

type IPPool struct {
	mu       sync.Mutex
	freePool []net.IP
	// inventory makes note if IP is allocated
	inventory map[string]bool
}

func (i *IPPool) DeallocIP(ip net.IP) {
	i.mu.Lock()
	defer i.mu.Unlock()

	isAllocated, ok := i.inventory[ip.String()]
	if !ok {
		log.Warnln("Attempt to dealloc non-existent IP", ip)
		return
	}

	if !isAllocated {
		log.Warnln("Attempt to dealloc a free IP", ip)
		return
	}

	i.inventory[ip.String()] = false
	i.freePool = append(i.freePool, ip) // Simply append to enqueue.
	log.Traceln("Deallocated IP:", ip)
}

func (i *IPPool) AllocIP() (net.IP, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.freePool) == 0 {
		err := errors.New("ip pool empty")
		return nil, err
	}

	ip := i.freePool[0]
	i.inventory[ip.String()] = true
	i.freePool = i.freePool[1:] // Slice off the element once it is dequeued.

	log.Traceln("Allocated IP:", ip)
	return ip, nil
}

func NewIPPool(cidr string) (*IPPool, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	i := &IPPool{
		inventory: make(map[string]bool),
	}

	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ipVal := make(net.IP, len(ip))
		copy(ipVal, ip)
		i.inventory[ipVal.String()] = false
		i.freePool = append(i.freePool, ipVal)
	}

	// remove network address and broadcast address
	i.freePool = i.freePool[1 : len(i.freePool)-1]

	return i, nil
}
