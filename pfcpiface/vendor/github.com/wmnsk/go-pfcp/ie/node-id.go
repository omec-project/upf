// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"io"
	"net"
	"strings"

	"github.com/wmnsk/go-pfcp/internal/utils"
)

// NodeID definitions.
const (
	NodeIDIPv4Address uint8 = 0
	NodeIDIPv6Address uint8 = 1
	NodeIDFQDN        uint8 = 2
)

// NewNodeID creates a new NodeID IE.
//
// Only one of the parameters should have a non-empty value(!="").
// If multiple parameters are given, the first one is used(ipv4 > ipv6 > fqdn)
func NewNodeID(ipv4, ipv6, fqdn string) *IE {
	var p []byte

	switch {
	case ipv4 != "":
		p = make([]byte, 5)
		p[0] = NodeIDIPv4Address
		copy(p[1:], net.ParseIP(ipv4).To4())
	case ipv6 != "":
		p = make([]byte, 17)
		p[0] = NodeIDIPv6Address
		copy(p[1:], net.ParseIP(ipv6).To16())
	case fqdn != "":
		p = make([]byte, 2+len([]byte(fqdn)))
		p[0] = NodeIDFQDN
		copy(p[1:], utils.EncodeFQDN(fqdn))
	default: // all params are empty
		return nil
	}

	return New(NodeID, p)
}

// NodeID returns NodeID in string if the type of IE matches.
func (i *IE) NodeID() (string, error) {
	if i.Type != NodeID {
		return "", &InvalidTypeError{Type: i.Type}
	}
	if len(i.Payload) < 2 {
		return "", io.ErrUnexpectedEOF
	}

	switch i.Payload[0] {
	case NodeIDIPv4Address:
		return net.IP(i.Payload[1:]).To4().String(), nil
	case NodeIDIPv6Address:
		return net.IP(i.Payload[1:]).To16().String(), nil
	case NodeIDFQDN:
		b := i.Payload[1:]
		var (
			nodeID []string
			offset int
		)
		max := len(b)
		for {
			if offset >= max {
				break
			}
			l := int(b[offset])
			if offset+l+1 > max {
				break
			}
			nodeID = append(nodeID, string(b[offset+1:offset+l+1]))
			offset += l + 1
		}

		return strings.Join(nodeID, "."), nil
	default:
		return "", &InvalidNodeIDError{ID: i.Payload[0]}
	}
}
