// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewUpdateURR creates a new UpdateURR IE.
func NewUpdateURR(
	urr, method, triggers, period, volth, volqt, evth, evqt,
	timeth, timeqt, qhtime, dropped, vtime, montime, subvth, subtimeth,
	subvqt, subtimeqt, subevth, subevqt, inact, likedURR, mInfo, mechanism,
	aggURRs, far, ethInact, addMontime, reports *IE) *IE {

	return newGroupedIE(
		UpdateURR, 0,
		urr, method, triggers, period, volth, volqt, evth, evqt,
		timeth, timeqt, qhtime, dropped, vtime, montime, subvth, subtimeth,
		subvqt, subtimeqt, subevth, subevqt, inact, likedURR, mInfo, mechanism,
		aggURRs, far, ethInact, addMontime, reports,
	)
}

// UpdateURR returns the IEs above UpdateURR if the type of IE matches.
func (i *IE) UpdateURR() ([]*IE, error) {
	if i.Type != UpdateURR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
