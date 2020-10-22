// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
)

const (
	reservedProto = uint8(0xff)
)

var (
	errBadSDF = errors.New("Unsupported SDF format")
)

type endpoint struct {
	IPNet *net.IPNet
	Port  uint16
}

func (ep *endpoint) parseNet(ipnet string) error {
	ipNetFields := strings.Split(ipnet, "/")
	log.Println(ipNetFields)
	switch len(ipNetFields) {
	case 1:
		ipnet = ipNetFields[0] + "/32"
	case 2:
	default:
		return errors.New("Incorrect network string")
	}

	var err error
	_, ep.IPNet, err = net.ParseCIDR(ipnet)
	if err != nil {
		return errors.New("Unable to ParseCIDR")
	}
	return nil
}

func (ep *endpoint) parsePort(port string) error {
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return err
	}
	ep.Port = uint16(p)
	return nil
}

type ipFilterRule struct {
	action, direction string
	proto             uint8
	src, dst          endpoint
}

// "permit out ip from any to assigned"
// "permit out ip from 60.60.0.102 to assigned"
// "permit out ip from 60.60.0.102/32 to assigned"
// "permit out ip from any to 60.60.0.102"
// "permit out ip from 60.60.0.1/26 to 60.60.0.102"
// "permit out ip from 60.60.0.1 8888 to 60.60.0.102/26"
// "permit out ip from 60.60.0.1 to 60.60.0.102 9999"
// "permit out ip from 60.60.0.1 8888 to 60.60.0.102 9999"

func (ipf *ipFilterRule) parseFlowDesc(flowDesc, ueIP string) error {
	fields := strings.Fields(flowDesc)

	if err := parseAction(fields[0]); err != nil {
		return err
	}
	ipf.action = fields[0]

	if err := parseDirection(fields[1]); err != nil {
		return err
	}
	ipf.direction = fields[1]

	ipf.proto = parseProto(fields[2])

	// bring to common intermediate representation
	xform := func(i int) {
		log.Println(fields)
		switch fields[i] {
		case "any":
			fields[i] = "0.0.0.0/0"
		case "assigned":
			if ueIP != "" && ueIP != "<nil>" {
				fields[i] = ueIP
			} else {
				fields[i] = "0.0.0.0/0"
			}
		}
		log.Println(fields)
	}

	for i := 3; i < len(fields); i++ {
		log.Println(fields[i])
		switch fields[i] {
		case "from":
			i++
			xform(i)
			err := ipf.src.parseNet(fields[i])
			if err != nil {
				log.Println(err)
			}

			if fields[i+1] != "to" {
				i++
				ipf.src.parsePort(fields[i])
			}
		case "to":
			i++
			xform(i)
			err := ipf.dst.parseNet(fields[i])
			if err != nil {
				log.Println(err)
			}

			if i < len(fields)-1 {
				i++
				ipf.dst.parsePort(fields[i])
			}
		}
	}

	fmt.Println(ipf)
	return nil
}

func parseAction(action string) error {
	switch action {
	case "permit":
	case "deny":
	default:
		return errBadSDF
	}
	return nil
}

func parseDirection(dir string) error {
	switch dir {
	case "in":
	case "out":
	default:
		return errBadSDF
	}
	return nil
}

func parseProto(proto string) uint8 {
	switch proto {
	case "udp":
		return 17
	case "tcp":
		return 6
	default:
		return reservedProto // IANA reserved
	}
}
