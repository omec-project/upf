// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewFramedIPv6Route creates a new FramedIPv6Route IE.
func NewFramedIPv6Route(name string) *IE {
	return newStringIE(FramedIPv6Route, name)
}

// FramedIPv6Route returns FramedIPv6Route in string if the type of IE matches.
func (i *IE) FramedIPv6Route() (string, error) {
	switch i.Type {
	case FramedIPv6Route:
		return string(i.Payload), nil
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == FramedIPv6Route {
				return x.FramedIPv6Route()
			}
		}
		return "", ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == FramedIPv6Route {
				return x.FramedIPv6Route()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
