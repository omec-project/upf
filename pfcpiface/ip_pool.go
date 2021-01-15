// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"errors"
	"log"
	"net"
)

type ipPool struct {
	freePool []string
}

func (ipp *ipPool) deallocIPV4(element net.IP) {
	ipp.freePool = append(ipp.freePool, element.String()) // Simply append to enqueue.
	log.Println("Enqueued:", element.String())
}

func (ipp *ipPool) allocIPV4() (net.IP, error) {
	if len(ipp.freePool) == 0 {
		err := errors.New("ip pool empty")
		return nil, err
	}
	element := ipp.freePool[0] // The first element is the one to be dequeued.
	log.Println("Dequeued:", element)
	ipp.freePool = ipp.freePool[1:] // Slice off the element once it is dequeued.
	ipVal := net.ParseIP(element).To4()
	return ipVal, nil
}

func (ipp *ipPool) initPool(cidr string) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ipp.freePool = append(ipp.freePool, ip.String())
	}
	// remove network address and broadcast address

	ipp.freePool = ipp.freePool[1 : len(ipp.freePool)-1]
	return nil
}
