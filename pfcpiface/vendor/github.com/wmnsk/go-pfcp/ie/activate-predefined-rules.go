// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

// NewActivatePredefinedRules creates a new ActivatePredefinedRules IE.
func NewActivatePredefinedRules(name string) *IE {
	return newStringIE(ActivatePredefinedRules, name)
}

// ActivatePredefinedRules returns ActivatePredefinedRules in string if the type of IE matches.
func (i *IE) ActivatePredefinedRules() (string, error) {
	switch i.Type {
	case ActivatePredefinedRules:
		return string(i.Payload), nil
	case CreatePDR:
		ies, err := i.CreatePDR()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == ActivatePredefinedRules {
				return x.ActivatePredefinedRules()
			}
		}
		return "", ErrIENotFound
	case UpdatePDR:
		ies, err := i.UpdatePDR()
		if err != nil {
			return "", err
		}
		for _, x := range ies {
			if x.Type == ActivatePredefinedRules {
				return x.ActivatePredefinedRules()
			}
		}
		return "", ErrIENotFound
	default:
		return "", &InvalidTypeError{Type: i.Type}
	}
}
