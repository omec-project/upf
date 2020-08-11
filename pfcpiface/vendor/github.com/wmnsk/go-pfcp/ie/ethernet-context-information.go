// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewEthernetContextInformation creates a new EthernetContextInformation IE.
func NewEthernetContextInformation(mac *IE) *IE {
	return newGroupedIE(EthernetContextInformation, 0, mac)
}

// EthernetContextInformation returns the IEs above EthernetContextInformation if the type of IE matches.
func (i *IE) EthernetContextInformation() ([]*IE, error) {
	if i.Type != EthernetContextInformation {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
