// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewDeactivatePredefinedRules creates a new DeactivatePredefinedRules IE.
func NewDeactivatePredefinedRules(name string) *IE {
	return newStringIE(DeactivatePredefinedRules, name)
}

// DeactivatePredefinedRules returns DeactivatePredefinedRules in string if the type of IE matches.
func (i *IE) DeactivatePredefinedRules() (string, error) {
	switch i.Type {
	case DeactivatePredefinedRules:
		return string(i.Payload), nil
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == DeactivatePredefinedRules {
				return x.DeactivatePredefinedRules()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
