// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import "io"

// NewForwardingPolicy creates a new ForwardingPolicy IE.
func NewForwardingPolicy(id string) *IE {
	l := len([]byte(id))
	i := New(ForwardingPolicy, make([]byte, 1+l))

	i.Payload[0] = uint8(l)
	copy(i.Payload[1:], []byte(id))

	return i
}

// ForwardingPolicy returns ForwardingPolicy in []byte if the type of IE matches.
func (i *IE) ForwardingPolicy() ([]byte, error) {
	switch i.Type {
	case ForwardingPolicy:
		return i.Payload, nil
	case ForwardingParameters:
		ies, err := i.ForwardingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ForwardingPolicy {
				return x.ForwardingPolicy()
			}
		}
		return nil, ErrIENotFound
	case UpdateForwardingParameters:
		ies, err := i.UpdateForwardingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ForwardingPolicy {
				return x.ForwardingPolicy()
			}
		}
		return nil, ErrIENotFound
	case DuplicatingParameters:
		ies, err := i.DuplicatingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ForwardingPolicy {
				return x.ForwardingPolicy()
			}
		}
		return nil, ErrIENotFound
	case UpdateDuplicatingParameters:
		ies, err := i.UpdateDuplicatingParameters()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == ForwardingPolicy {
				return x.ForwardingPolicy()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// ForwardingPolicyIdentifier returns ForwardingPolicyIdentifier in string if the type of IE matches.
func (i *IE) ForwardingPolicyIdentifier() (string, error) {
	v, err := i.ForwardingPolicy()
	if err != nil {
		return "", err
	}

	l := len(v)
	if l < 1 {
		return "", io.ErrUnexpectedEOF
	}

	idlen := int(v[0])
	if l < idlen+1 {
		return "", io.ErrUnexpectedEOF
	}

	return string(v[1 : idlen+1]), nil
}
