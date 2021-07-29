// Copyright 2019-2021 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "github.com/wmnsk/go-pfcp/internal/utils"

// NewNetworkInstance creates a new NetworkInstance IE.
func NewNetworkInstance(instance string) *IE {
	return newStringIE(NetworkInstance, instance)
}

// NewNetworkInstanceFQDN creates a new NetworkInstance IE from the given
// FQDN string.
func NewNetworkInstanceFQDN(fqdn string) *IE {
	return newFQDNIE(NetworkInstance, fqdn)
}

// NetworkInstance returns NetworkInstance in string if the type of IE matches.
func (i *IE) NetworkInstance() (string, error) {
	switch i.Type {
	case NetworkInstance:
		return string(i.Payload), nil
	case PDI:
		ies, err := i.PDI()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			switch i.Type {
			case NetworkInstance, RedundantTransmissionParameters:
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == NetworkInstance {
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == NetworkInstance {
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == NetworkInstance {
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == NetworkInstance {
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	case RedundantTransmissionParameters:
		ies, err := i.RedundantTransmissionParameters()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == NetworkInstance {
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	case UEIPAddressPoolInformation:
		ies, err := i.UEIPAddressPoolInformation()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == NetworkInstance {
				return x.NetworkInstance()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}

// NetworkInstanceFQDN returns NetworkInstance in string if the type of IE matches.
// This is for the case that NetworkInstance is encoded as a FQDN.
func (i *IE) NetworkInstanceFQDN() (string, error) {
	// for checking the type
	_, err := i.NetworkInstance()
	if err != nil {
		return "", err
	}

	return utils.DecodeFQDN(i.Payload), nil
}

// NetworkInstanceHeuristic assumes that the payload is encoded in Name Syntax
// and returns the decoded string if it looks meaningful. Otherwise returns a
// string just converted from []byte.
func (i *IE) NetworkInstanceHeuristic() (string, error) {
	v, err := i.NetworkInstanceFQDN()
	if err != nil {
		return "", err
	}

	if v != "" { // can be more strict...
		return v, nil
	}

	return i.NetworkInstance()
}
