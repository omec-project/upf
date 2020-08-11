// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateQER creates a new UpdateQER IE.
func NewUpdateQER(qer, qci, gate, mbr, gbr, pktRate, prStatus, dlflm, qfi, rqi, ppi, aw, qInd *IE) *IE {
	return newGroupedIE(UpdateQER, 0, qer, qci, gate, mbr, gbr, pktRate, prStatus, dlflm, qfi, rqi, ppi, aw, qInd)
}

// UpdateQER returns the IEs above UpdateQER if the type of IE matches.
func (i *IE) UpdateQER() ([]*IE, error) {
	if i.Type != UpdateQER {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
