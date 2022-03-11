// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	reservedProto         = uint8(0xff)
	Ipv4WildcardNetString = "0.0.0.0/0"
)

var errBadFilterDesc = errors.New("unsupported Filter Description format")

type endpoint struct {
	IPNet *net.IPNet
	ports portRange
}

func (ep *endpoint) parseNet(ipnet string) error {
	ipNetFields := strings.Split(ipnet, "/")

	switch len(ipNetFields) {
	case 1:
		ipnet = ipNetFields[0] + "/32"
	case 2:
	default:
		return ErrInvalidArgument("network string", len(ipNetFields))
	}

	var err error

	_, ep.IPNet, err = net.ParseCIDR(ipnet)
	if err != nil {
		return ErrOperationFailedWithReason("ParseCIDR", err.Error())
	}

	return nil
}

func (ep *endpoint) parsePort(port string) error {
	ports := strings.Split(port, "-")
	if len(ports) == 0 || len(ports) > 2 {
		return ErrInvalidArgument("port string", port)
	}
	// Pretend this is a port range with one element.
	if len(ports) == 1 {
		ports = append(ports, ports[0])
	}

	low, err := strconv.ParseUint(ports[0], 10, 16)
	if err != nil {
		return err
	}

	high, err := strconv.ParseUint(ports[1], 10, 16)
	if err != nil {
		return err
	}

	if low > high {
		return ErrInvalidArgumentWithReason("port", port, "invalid port range")
	}

	ep.ports = newRangeMatchPortRange(uint16(low), uint16(high))

	return nil
}

type ipFilterRule struct {
	action, direction string
	proto             uint8
	src, dst          endpoint
}

func newIpFilterRule() *ipFilterRule {
	return &ipFilterRule{
		src: endpoint{ports: newWildcardPortRange()},
		dst: endpoint{ports: newWildcardPortRange()},
	}
}

func (ipf *ipFilterRule) String() string {
	return fmt.Sprintf("FlowDescription{action=%v, direction=%v, proto=%v, "+
		"srcIP=%v, srcPort=%v, dstIP=%v, dstPort=%v}",
		ipf.action, ipf.direction, ipf.proto, ipf.src.IPNet, ipf.src.ports, ipf.dst.IPNet, ipf.dst.ports)
}

func parseFlowDesc(flowDesc, ueIP string) (*ipFilterRule, error) {
	parseLog := log.WithFields(log.Fields{
		"flow-description": flowDesc,
		"ue-address":       ueIP,
	})
	parseLog.Debug("Parsing flow description")

	ipf := newIpFilterRule()

	fields := strings.Fields(flowDesc)
	if len(fields) < 3 {
		return nil, errBadFilterDesc
	}

	if err := parseAction(fields[0]); err != nil {
		return nil, err
	}

	ipf.action = fields[0]

	if err := parseDirection(fields[1]); err != nil {
		return nil, err
	}

	ipf.direction = fields[1]
	ipf.proto, _ = parseL4Proto(fields[2])

	// bring to common intermediate representation
	xform := func(i int) {
		switch fields[i] {
		case "any":
			fields[i] = Ipv4WildcardNetString
		case "assigned":
			if ueIP == "0.0.0.0" {
				fields[i] = Ipv4WildcardNetString
			} else if ueIP != "" && ueIP != "<nil>" {
				fields[i] = ueIP
			} else {
				fields[i] = Ipv4WildcardNetString
			}
		}
	}

	for i := 3; i < len(fields); i++ {
		switch fields[i] {
		case "from":
			i++
			xform(i)

			err := ipf.src.parseNet(fields[i])
			if err != nil {
				parseLog.Error(err)
				return nil, err
			}

			if fields[i+1] != "to" {
				i++

				err = ipf.src.parsePort(fields[i])
				if err != nil {
					parseLog.Error("src port parse failed ", err)
					return nil, err
				}
			}
		case "to":
			i++
			xform(i)

			err := ipf.dst.parseNet(fields[i])
			if err != nil {
				parseLog.Error(err)
				return nil, err
			}

			if i < len(fields)-1 {
				i++

				err = ipf.dst.parsePort(fields[i])
				if err != nil {
					parseLog.Error("dst port parse failed ", err)
					return nil, err
				}
			}
		}
	}

	parseLog = parseLog.WithField("ip-filter", ipf)
	parseLog.Debug("Flow description parsed successfully")

	return ipf, nil
}

func parseAction(action string) error {
	switch action {
	case "permit":
	case "deny":
	default:
		return errBadFilterDesc
	}

	return nil
}

func parseDirection(dir string) error {
	switch dir {
	case "in":
	case "out":
	default:
		return errBadFilterDesc
	}

	return nil
}

func parseL4Proto(proto string) (uint8, error) {
	p, err := strconv.ParseUint(proto, 10, 8)
	if err == nil {
		return uint8(p), nil
	}

	switch proto {
	case "udp":
		return 17, nil
	case "tcp":
		return 6, nil
	default:
		return reservedProto, errBadFilterDesc
	}
}
