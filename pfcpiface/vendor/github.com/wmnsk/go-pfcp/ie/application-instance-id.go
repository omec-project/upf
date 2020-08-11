// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewApplicationInstanceID creates a new ApplicationInstanceID IE.
func NewApplicationInstanceID(id string) *IE {
	return newStringIE(ApplicationInstanceID, id)
}

// ApplicationInstanceID returns ApplicationInstanceID in string if the type of IE matches.
func (i *IE) ApplicationInstanceID() (string, error) {
	switch i.Type {
	case ApplicationInstanceID:
		return string(i.Payload), nil
	case ApplicationDetectionInformation:
		ies, err := i.ApplicationDetectionInformation()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == ApplicationInstanceID {
				return x.ApplicationInstanceID()
			}
		}
		return "", ErrIENotFound
	case UsageReportWithinSessionReportRequest:
		ies, err := i.UsageReport()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == ApplicationDetectionInformation {
				return x.ApplicationInstanceID()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
