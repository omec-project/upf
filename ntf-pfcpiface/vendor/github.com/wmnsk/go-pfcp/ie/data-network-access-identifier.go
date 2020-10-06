// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewDataNetworkAccessIdentifier creates a new DataNetworkAccessIdentifier IE.
func NewDataNetworkAccessIdentifier(id string) *IE {
	return newStringIE(DataNetworkAccessIdentifier, id)
}

// DataNetworkAccessIdentifier returns DataNetworkAccessIdentifier in string if the type of IE matches.
func (i *IE) DataNetworkAccessIdentifier() (string, error) {
	switch i.Type {
	case DataNetworkAccessIdentifier:
		return string(i.Payload), nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == DataNetworkAccessIdentifier {
				return x.DataNetworkAccessIdentifier()
			}
		}
		return "", ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == DataNetworkAccessIdentifier {
				return x.DataNetworkAccessIdentifier()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
