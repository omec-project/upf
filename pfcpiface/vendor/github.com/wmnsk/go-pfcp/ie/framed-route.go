// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewFramedRoute creates a new FramedRoute IE.
func NewFramedRoute(name string) *IE {
	return newStringIE(FramedRoute, name)
}

// FramedRoute returns FramedRoute in string if the type of IE matches.
func (i *IE) FramedRoute() (string, error) {
	switch i.Type {
	case FramedRoute:
		return string(i.Payload), nil
	case CreateTrafficEndpoint:
		ies, err := i.CreateTrafficEndpoint()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == FramedRoute {
				return x.FramedRoute()
			}
		}
		return "", ErrIENotFound
	case UpdateTrafficEndpoint:
		ies, err := i.UpdateTrafficEndpoint()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == FramedRoute {
				return x.FramedRoute()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
