// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewDownlinkDataReport creates a new DownlinkDataReport IE.
func NewDownlinkDataReport(pdr, sinfo, psize *IE) *IE {
	return newGroupedIE(DownlinkDataReport, 0, pdr, sinfo, psize)
}

// DownlinkDataReport returns the IEs above DownlinkDataReport if the type of IE matches.
func (i *IE) DownlinkDataReport() ([]*IE, error) {
	if i.Type != DownlinkDataReport {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
