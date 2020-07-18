// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"
)

// NewPFDContents creates a new PFDContents IE.
func NewPFDContents(fd, url, dn, cp, dnp string, afd, aurl, adnp []string) *IE {
	f := NewPFDContentsFields(fd, url, dn, cp, dnp, afd, aurl, adnp)
	b, err := f.Marshal()
	if err != nil {
		return nil
	}

	return New(PFDContents, b)
}

// PFDContents returns PFDContents in structured format if the type of IE matches.
//
// This IE has a complex payload that costs much when parsing.
func (i *IE) PFDContents() (*PFDContentsFields, error) {
	switch i.Type {
	case PFDContents:
		s, err := ParsePFDContentsFields(i.Payload)
		if err != nil {
			return nil, err
		}
		return s, nil
	case PFDContext:
		ies, err := i.PFDContext()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PFDContents {
				return x.PFDContents()
			}
		}
		return nil, ErrIENotFound
	case ApplicationIDsPFDs:
		ies, err := i.ApplicationIDsPFDs()
		if err != nil {
			return nil, err
		}
		for _, x := range ies {
			if x.Type == PFDContext {
				return x.PFDContents()
			}
		}
		return nil, ErrIENotFound
	default:
		return nil, &InvalidTypeError{Type: i.Type}
	}
}

// PFDContentsFields represents a fields contained in PFDContents IE.
type PFDContentsFields struct {
	Flags                           uint8
	FDLength                        uint16
	FlowDescription                 string
	URLLength                       uint16
	URL                             string
	DNLength                        uint16
	DomainName                      string
	CPLength                        uint16
	CustomPFDContent                string
	DNPLength                       uint16
	DomainNameProtocol              string
	AFDLength                       uint16
	AdditionalFlowDescription       []string
	AURLLength                      uint16
	AdditionalURL                   []string
	ADNPLength                      uint16
	AdditionalDomainNameAndProtocol []string
}

// NewPFDContentsFields creates a new NewPFDContentsFields.
func NewPFDContentsFields(fd, url, dn, cp, dnp string, afd, aurl, adnp []string) *PFDContentsFields {
	f := &PFDContentsFields{}
	if fd != "" {
		f.FlowDescription = fd
		f.FDLength = uint16(len([]byte(fd)))
		f.SetFDFlag()
	}

	if url != "" {
		f.URL = url
		f.URLLength = uint16(len([]byte(url)))
		f.SetURLFlag()
	}

	if dn != "" {
		f.DomainName = dn
		f.DNLength = uint16(len([]byte(dn)))
		f.SetDNFlag()
	}

	if cp != "" {
		f.CustomPFDContent = cp
		f.CPLength = uint16(len([]byte(cp)))
		f.SetCPFlag()
	}

	if dnp != "" {
		f.DomainNameProtocol = dnp
		f.DNPLength = uint16(len([]byte(dnp)))
		f.SetDNPFlag()
	}

	if afd != nil {
		l := 0
		for _, a := range afd {
			l += 2 + len([]byte(a))
		}

		f.AdditionalFlowDescription = afd
		f.AFDLength = uint16(l)
		f.SetAFDFlag()
	}

	if aurl != nil {
		l := 0
		for _, a := range aurl {
			l += 2 + len([]byte(a))
		}

		f.AdditionalURL = aurl
		f.AURLLength = uint16(l)
		f.SetAURLFlag()
	}

	if adnp != nil {
		l := 0
		for _, a := range adnp {
			l += 2 + len([]byte(a))
		}

		f.AdditionalDomainNameAndProtocol = adnp
		f.ADNPLength = uint16(l)
		f.SetADNPFlag()
	}

	return f
}

// HasADNP reports whether ADNP flag is set.
func (f *PFDContentsFields) HasADNP() bool {
	return has8thBit(f.Flags)
}

// SetADNPFlag sets ADNP flag in PFDContents.
func (f *PFDContentsFields) SetADNPFlag() {
	f.Flags |= 0x80
}

// HasAURL reports whether AURL flag is set.
func (f *PFDContentsFields) HasAURL() bool {
	return has7thBit(f.Flags)
}

// SetAURLFlag sets AURL flag in PFDContents.
func (f *PFDContentsFields) SetAURLFlag() {
	f.Flags |= 0x40
}

// HasAFD reports whether AFD flag is set.
func (f *PFDContentsFields) HasAFD() bool {
	return has6thBit(f.Flags)
}

// SetAFDFlag sets AFD flag in PFDContents.
func (f *PFDContentsFields) SetAFDFlag() {
	f.Flags |= 0x20
}

// HasDNP reports whether DNP flag is set.
func (f *PFDContentsFields) HasDNP() bool {
	return has5thBit(f.Flags)
}

// SetDNPFlag sets DNP flag in PFDContents.
func (f *PFDContentsFields) SetDNPFlag() {
	f.Flags |= 0x10
}

// HasCP reports whether CP flag is set.
func (f *PFDContentsFields) HasCP() bool {
	return has4thBit(f.Flags)
}

// SetCPFlag sets CP flag in PFDContents.
func (f *PFDContentsFields) SetCPFlag() {
	f.Flags |= 0x08
}

// HasDN reports whether DN flag is set.
func (f *PFDContentsFields) HasDN() bool {
	return has3rdBit(f.Flags)
}

// SetDNFlag sets DN flag in PFDContents.
func (f *PFDContentsFields) SetDNFlag() {
	f.Flags |= 0x04
}

// HasURL reports whether URL flag is set.
func (f *PFDContentsFields) HasURL() bool {
	return has2ndBit(f.Flags)
}

// SetURLFlag sets URL flag in PFDContents.
func (f *PFDContentsFields) SetURLFlag() {
	f.Flags |= 0x02
}

// HasFD reports whether FD flag is set.
func (f *PFDContentsFields) HasFD() bool {
	return has1stBit(f.Flags)
}

// SetFDFlag sets FD flag in PFDContents.
func (f *PFDContentsFields) SetFDFlag() {
	f.Flags |= 0x01
}

// ParsePFDContentsFields parses b into PFDContentsFields.
func ParsePFDContentsFields(b []byte) (*PFDContentsFields, error) {
	f := &PFDContentsFields{}
	if err := f.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return f, nil
}

// UnmarshalBinary parses b into IE.
func (f *PFDContentsFields) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 3 {
		return io.ErrUnexpectedEOF
	}

	f.Flags = b[0]
	offset := 2 // 2nd octet is spare

	if f.HasFD() {
		if len(b[offset:]) < offset+2+int(f.FDLength) {
			return io.ErrUnexpectedEOF
		}
		f.FDLength = binary.BigEndian.Uint16(b[offset : offset+2])
		f.FlowDescription = string(b[offset+2 : offset+2+int(f.FDLength)])
		offset += 2 + int(f.FDLength)
	}

	if f.HasURL() {
		if len(b[offset:]) < offset+2+int(f.URLLength) {
			return io.ErrUnexpectedEOF
		}
		f.URLLength = binary.BigEndian.Uint16(b[offset : offset+2])
		f.URL = string(b[offset+2 : offset+2+int(f.URLLength)])
		offset += 2 + int(f.URLLength)
	}

	if f.HasDN() {
		if len(b[offset:]) < offset+2+int(f.DNLength) {
			return io.ErrUnexpectedEOF
		}
		f.DNLength = binary.BigEndian.Uint16(b[offset : offset+2])
		f.DomainName = string(b[offset+2 : offset+2+int(f.DNLength)])
		offset += 2 + int(f.DNLength)
	}

	if f.HasCP() {
		if len(b[offset:]) < offset+2+int(f.CPLength) {
			return io.ErrUnexpectedEOF
		}
		f.CPLength = binary.BigEndian.Uint16(b[offset : offset+2])
		f.CustomPFDContent = string(b[offset+2 : offset+2+int(f.CPLength)])
		offset += 2 + int(f.CPLength)
	}

	if f.HasDNP() {
		if len(b[offset:]) < offset+2+int(f.DNPLength) {
			return io.ErrUnexpectedEOF
		}
		f.DNPLength = binary.BigEndian.Uint16(b[offset : offset+2])
		f.DomainNameProtocol = string(b[offset+2 : offset+2+int(f.DNPLength)])
		offset += 2 + int(f.DNPLength)
	}

	if f.HasAFD() {
		if len(b[offset:]) < offset+2+int(f.AFDLength) {
			return io.ErrUnexpectedEOF
		}
		f.AFDLength = binary.BigEndian.Uint16(b[offset : offset+2])

		p := b[offset+2 : offset+2+int(f.AFDLength)]
		o := 0
		for {
			if len(p) <= o+2 {
				break
			}
			l := binary.BigEndian.Uint16(p[o : o+2])
			if len(p) < o+2+int(l) {
				break
			}
			f.AdditionalFlowDescription = append(f.AdditionalFlowDescription, string(p[o+2:o+2+int(l)]))
		}
		offset += 2 + int(f.AFDLength)
	}

	if f.HasAURL() {
		if len(b[offset:]) < offset+2+int(f.AURLLength) {
			return io.ErrUnexpectedEOF
		}
		f.AURLLength = binary.BigEndian.Uint16(b[offset : offset+2])

		p := b[offset+2 : offset+2+int(f.AURLLength)]
		o := 0
		for {
			if len(p) <= o+2 {
				break
			}
			l := binary.BigEndian.Uint16(p[o : o+2])
			if len(p) < o+2+int(l) {
				break
			}
			f.AdditionalURL = append(f.AdditionalURL, string(p[o+2:o+2+int(l)]))
		}
		offset += 2 + int(f.AURLLength)
	}

	if f.HasADNP() {
		if len(b[offset:]) < offset+2+int(f.ADNPLength) {
			return io.ErrUnexpectedEOF
		}
		f.ADNPLength = binary.BigEndian.Uint16(b[offset : offset+2])

		p := b[offset+2 : offset+2+int(f.ADNPLength)]
		o := 0
		for {
			if len(p) <= o+2 {
				break
			}
			l := binary.BigEndian.Uint16(p[o : o+2])
			if len(p) < o+2+int(l) {
				break
			}
			f.AdditionalDomainNameAndProtocol = append(f.AdditionalDomainNameAndProtocol, string(p[o+2:o+2+int(l)]))
		}
	}

	return nil
}

// Marshal returns the serialized bytes of PFDContentsFields.
func (f *PFDContentsFields) Marshal() ([]byte, error) {
	b := make([]byte, f.MarshalLen())
	if err := f.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (f *PFDContentsFields) MarshalTo(b []byte) error {
	l := len(b)
	if l < 1 {
		return io.ErrUnexpectedEOF
	}

	b[0] = f.Flags
	offset := 2 // 2nd octet is spare

	if f.HasFD() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.FDLength)
		copy(b[offset+2:offset+2+int(f.FDLength)], []byte(f.FlowDescription))
		offset += 2 + int(f.FDLength)
	}

	if f.HasURL() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.URLLength)
		copy(b[offset+2:offset+2+int(f.URLLength)], []byte(f.URL))
		offset += 2 + int(f.URLLength)
	}

	if f.HasDN() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.DNLength)
		copy(b[offset+2:offset+2+int(f.DNLength)], []byte(f.DomainName))
		offset += 2 + int(f.DNLength)
	}

	if f.HasCP() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.CPLength)
		copy(b[offset+2:offset+2+int(f.CPLength)], []byte(f.CustomPFDContent))
		offset += 2 + int(f.CPLength)
	}

	if f.HasDNP() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.DNPLength)
		copy(b[offset+2:offset+2+int(f.DNPLength)], []byte(f.DomainNameProtocol))
		offset += 2 + int(f.DNPLength)
	}

	if f.HasAFD() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.AFDLength)
		offset += 2
		for _, a := range f.AdditionalFlowDescription {
			l := len([]byte(a))
			binary.BigEndian.PutUint16(b[offset:offset+2], uint16(l))
			copy(b[offset+2:offset+2+l], []byte(a))
			offset += 2 + l
		}
	}

	if f.HasAURL() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.AURLLength)
		offset += 2
		for _, a := range f.AdditionalURL {
			l := len([]byte(a))
			binary.BigEndian.PutUint16(b[offset:offset+2], uint16(l))
			copy(b[offset+2:offset+2+l], []byte(a))
			offset += 2 + l
		}
	}

	if f.HasADNP() {
		binary.BigEndian.PutUint16(b[offset:offset+2], f.ADNPLength)
		offset += 2
		for _, a := range f.AdditionalDomainNameAndProtocol {
			l := len([]byte(a))
			binary.BigEndian.PutUint16(b[offset:offset+2], uint16(l))
			copy(b[offset+2:offset+2+l], []byte(a))
			offset += 2 + l
		}
	}

	return nil
}

// MarshalLen returns field length in integer.
func (f *PFDContentsFields) MarshalLen() int {
	l := 2 // Flag + Spare

	if f.HasFD() {
		l += 2 + int(f.FDLength)
	}

	if f.HasURL() {
		l += 2 + int(f.URLLength)
	}

	if f.HasDN() {
		l += 2 + int(f.DNLength)
	}

	if f.HasCP() {
		l += 2 + int(f.CPLength)
	}

	if f.HasDNP() {
		l += 2 + int(f.DNPLength)
	}

	if f.HasAFD() {
		l += 2 + int(f.AFDLength)
	}

	if f.HasAURL() {
		l += 2 + int(f.AURLLength)
	}

	if f.HasADNP() {
		l += 2 + int(f.ADNPLength)
	}

	return l
}
