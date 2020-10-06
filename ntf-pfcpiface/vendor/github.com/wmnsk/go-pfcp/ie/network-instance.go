// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewNetworkInstance creates a new NetworkInstance IE.
func NewNetworkInstance(instance string) *IE {
	return newStringIE(NetworkInstance, instance)
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
