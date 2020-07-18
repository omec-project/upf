// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Header represents a PFCP header.
type Header struct {
	Flags           uint8 // Version + FO, MP, S flags
	Type            uint8
	Length          uint16
	SEID            uint64
	SequenceNumber  uint32 // 3 octets
	MessagePriority uint8  // half octet
	Payload         []byte
}

// NewHeader creates a new Header.
func NewHeader(ver, fo, mp, s, mtype uint8, seid uint64, seq uint32, pri uint8, payload []byte) *Header {
	h := &Header{
		Flags:           ((ver & 0x7) << 5) | (fo << 2) | (mp << 1) | s,
		Type:            mtype,
		SEID:            seid,
		SequenceNumber:  seq,
		MessagePriority: pri,
		Payload:         payload,
	}
	h.SetLength()

	return h
}

// NewHeaderNodeRelated creates a new Header for Node Related Messages.
func NewHeaderNodeRelated(ver, mtype uint8, seq uint32, payload []byte) *Header {
	return NewHeader(ver, 0, 0, 0, mtype, 0, seq, 0, payload)
}

// NewHeaderSessionRelated creates a new Header for Session Related Messages.
func NewHeaderSessionRelated(ver, fo, mp, s, mtype uint8, seid uint64, seq uint32, pri uint8, payload []byte) *Header {
	// currently this is identical to NewHeader.
	return NewHeader(ver, fo, mp, s, mtype, seid, seq, pri, payload)
}

// HasFO reports whether Header has Follow On message(FO flag is set or not).
func (h *Header) HasFO() bool {
	return has3rdBit(h.Flags)
}

// HasMP reports whether Header has MessagePriority(MP flag is set or not).
func (h *Header) HasMP() bool {
	return has2ndBit(h.Flags)
}

// HasSEID reports whether Header has SEID(S flag is set or not).
func (h *Header) HasSEID() bool {
	return has1stBit(h.Flags)
}

// Marshal returns the byte sequence generated from a Header instance.
func (h *Header) Marshal() ([]byte, error) {
	b := make([]byte, h.MarshalLen())
	if err := h.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (h *Header) MarshalTo(b []byte) error {
	b[0] = h.Flags
	b[1] = h.Type
	binary.BigEndian.PutUint16(b[2:4], h.Length)

	offset := 4
	if h.HasSEID() {
		binary.BigEndian.PutUint64(b[offset:offset+8], h.SEID)
		offset += 8
	}

	copy(b[offset:offset+3], uint32To24(h.SequenceNumber))
	b[offset+3] = h.MessagePriority
	copy(b[offset+4:h.MarshalLen()], h.Payload)

	return nil
}

// ParseHeader decodes given byte sequence as a GTPv2 header.
func ParseHeader(b []byte) (*Header, error) {
	h := &Header{}
	if err := h.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return h, nil
}

// UnmarshalBinary sets the values retrieved from byte sequence in GTPv2 header.
func (h *Header) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 8 {
		return io.ErrUnexpectedEOF
	}
	h.Flags = b[0]
	h.Type = b[1]
	h.Length = binary.BigEndian.Uint16(b[2:4])

	offset := 4
	if h.HasSEID() {
		if l < offset+8 {
			return io.ErrUnexpectedEOF
		}
		h.SEID = binary.BigEndian.Uint64(b[offset : offset+8])
		offset += 8
	}

	if l < offset+4 {
		return io.ErrUnexpectedEOF
	}
	h.SequenceNumber = uint24To32(b[offset : offset+3])
	h.MessagePriority = b[offset+3]
	offset += 4

	if int(h.Length)+offset != l {
		h.Payload = b[offset:]
		return nil
	}

	if l < offset+int(h.Length) {
		return io.ErrUnexpectedEOF
	}
	h.Payload = b[offset : offset+int(h.Length)]
	return nil
}

// MarshalLen returns field length in integer.
func (h *Header) MarshalLen() int {
	l := 8 + len(h.Payload)
	if h.HasSEID() {
		l += 8
	}

	return l
}

// SetLength sets the length in Length field.
func (h *Header) SetLength() {
	h.Length = uint16(4 + len(h.Payload))
	if h.HasSEID() {
		h.Length += 8
	}
}

// Version returns the GTP version.
func (h *Header) Version() int {
	return 1
}

// MessageType returns the type of messagg.
func (h *Header) MessageType() uint8 {
	return h.Type
}

func (h *Header) seid() uint64 {
	if !h.HasSEID() {
		return 0
	}
	return h.SEID
}

// SetSEID sets the S Flag to 1 and puts the SEID given into SEID field.
func (h *Header) SetSEID(seid uint64) {
	h.Flags |= 0x01
	h.SEID = seid
}

// Sequence returns SequenceNumber in uint32.
func (h *Header) Sequence() uint32 {
	return h.SequenceNumber
}

// SetSequenceNumber sets the SequenceNumber in Header.
func (h *Header) SetSequenceNumber(seq uint32) {
	h.SequenceNumber = seq
}

// SetMP sets the M Flag to 1 and puts the MessagePriority
// given into MessagePriority field.
func (h *Header) SetMP(mp uint8) {
	h.Flags |= (1 << 2)
	h.MessagePriority = (mp << 4) & 0xf0
}

// MP returns the value of MessagePriority.
//
// Note that this returns the value set in the field even if the MP Flag
// is not set to 1.
func (h *Header) MP() uint8 {
	return (h.MessagePriority & 0xf0) >> 4
}

// String returns the GTPv2 header values in human readable format.
func (h *Header) String() string {
	return fmt.Sprintf("{Version: %d, Flags: {FO: %t, MP: %t, S: %t}, Type: %d, Length: %d, SEID: %#016x, SequenceNumber: %#x, MessagePriority: %d, Payload: %#x}",
		h.Version(),
		h.HasFO(), h.HasMP(), h.HasSEID(),
		h.Type,
		h.Length,
		h.SEID,
		h.SequenceNumber,
		h.MessagePriority,
		h.Payload,
	)
}
