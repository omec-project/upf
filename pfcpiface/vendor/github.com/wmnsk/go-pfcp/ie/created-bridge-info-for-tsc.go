// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreatedBridgeInfoForTSC creates a new CreatedBridgeInfoForTSC IE.
func NewCreatedBridgeInfoForTSC(ies ...*IE) *IE {
	return newGroupedIE(CreatedBridgeInfoForTSC, 0, ies...)
}

// CreatedBridgeInfoForTSC returns the IEs above CreatedBridgeInfoForTSC if the type of IE matches.
func (i *IE) CreatedBridgeInfoForTSC() ([]*IE, error) {
	if i.Type != CreatedBridgeInfoForTSC {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
