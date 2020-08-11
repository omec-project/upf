// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewCreateURR creates a new CreateURR IE.
func NewCreateURR(
	urr, method, triggers, period, volth, volqt, evth, evqt,
	timeth, timeqt, qhtime, dropped, vtime, montime, subvth, subtimeth,
	subvqt, subtimeqt, subevth, subevqt, inact, likedURR, mInfo, mechanism,
	aggURRs, far, ethInact, addMontime, reports *IE) *IE {

	return newGroupedIE(
		CreateURR, 0,
		urr, method, triggers, period, volth, volqt, evth, evqt,
		timeth, timeqt, qhtime, dropped, vtime, montime, subvth, subtimeth,
		subvqt, subtimeqt, subevth, subevqt, inact, likedURR, mInfo, mechanism,
		aggURRs, far, ethInact, addMontime, reports,
	)
}

// CreateURR returns the IEs above CreateURR if the type of IE matches.
func (i *IE) CreateURR() ([]*IE, error) {
	if i.Type != CreateURR {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
