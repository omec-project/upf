// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewReportType creates a new ReportType IE.
func NewReportType(upir, erir, usar, dldr int) *IE {
	return newUint8ValIE(ReportType, uint8((upir<<3)|(erir<<2)|(usar<<1)|(dldr)))
}

// ReportType returns ReportType in uint8 if the type of IE matches.
func (i *IE) ReportType() (uint8, error) {
	if i.Type != ReportType {
		return 0, &InvalidTypeError{Type: i.Type}
	}

	return i.Payload[0], nil
}

// HasUPIR reports whether an IE has UPIR bit.
func (i *IE) HasUPIR() bool {
	v, err := i.ReportType()
	if err != nil {
		return false
	}

	return has4thBit(v)
}

// HasERIR reports whether an IE has ERIR bit.
func (i *IE) HasERIR() bool {
	v, err := i.ReportType()
	if err != nil {
		return false
	}

	return has3rdBit(v)
}

// HasUSAR reports whether an IE has USAR bit.
func (i *IE) HasUSAR() bool {
	v, err := i.ReportType()
	if err != nil {
		return false
	}

	return has2ndBit(v)
}

// HasDLDR reports whether an IE has DLDR bit.
func (i *IE) HasDLDR() bool {
	v, err := i.ReportType()
	if err != nil {
		return false
	}

	return has1stBit(v)
}
