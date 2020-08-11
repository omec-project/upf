// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewRemoveQER creates a new RemoveQER IE.
func NewRemoveQER(qer *IE) *IE {
	return newGroupedIE(RemoveQER, 0, qer)
}

// RemoveQER returns the IEs above RemoveQER if the type of IE matches.
func (i *IE) RemoveQER() ([]*IE, error) {
	if i.Type != RemoveQER {
		return nil, &InvalidTypeError{Type: i.Type}
	}

	return ParseMultiIEs(i.Payload)
}
