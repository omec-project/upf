// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"fmt"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

const (
	SrcIfaceStr        string = "src_iface"     // Src Interface field name
)

// IntfTableEntry ... Interface Table Entry API.
type IntfTableEntry struct {
	IP        []byte
	PrefixLen int
	SrcIntf   string
	Direction string
}

// ActionParam ... Action Param API.
type ActionParam struct {
	Len   uint32
	Name  string
	Value []byte
}

// MatchField .. Match Field API.
type MatchField struct {
	Len       uint32
	PrefixLen uint32
	Name      string
	Value     []byte
	Mask      []byte
}

// IntfCounterEntry .. Counter entry function API.
type IntfCounterEntry struct {
	CounterID uint64
	Index     uint64
	ByteCount []uint64
	PktCount  []uint64
}

// AppTableEntry .. Table entry function API.
type AppTableEntry struct {
	// FieldSize, ParamSize are redundant; I keep them only for backward compatibility.
	// TODO: remove them
	FieldSize  uint32
	ParamSize  uint32
	TableName  string
	ActionName string
	Fields     []MatchField
	Params     []ActionParam
}

func NewP4RtTableEntry() *AppTableEntry {
	return &AppTableEntry{
		Fields:     make([]MatchField, 0),
		Params:     make([]ActionParam, 0),
	}
}

// TODO: generate byte slice based on the interface type
func (te *AppTableEntry) withExactMatchField(name string, value interface{}, size uint8) {

}

// TODO: generate byte slice based on the interface type
func (te *AppTableEntry) withTernaryMatchField(name string, value interface{}, mask interface{}, size uint8) {

}

func (te *AppTableEntry) withActionParam(name string, value interface{}, size uint8) {

}

func BuildSessionsTableEntry(pdr pdr, tunnelPeerID uint8, buffer bool) (*AppTableEntry, error) {
	te := NewP4RtTableEntry()
	te.TableName = "PreQosPipe.sessions"

	te.withExactMatchField(SrcIfaceStr, pdr.srcIface, 1)
	if pdr.srcIface == access {
		te.withExactMatchField("ipv4_dst", pdr.tunnelIP4Dst, 4)
		te.withTernaryMatchField("teid", pdr.tunnelTEID, pdr.tunnelTEIDMask, 4)
		te.ActionName = "set_params_uplink"
	} else if pdr.srcIface == core {
		te.withExactMatchField("ipv4_dst", pdr.dstIP, 4)
		te.ActionName = "set_params_downlink"
		te.withActionParam("tunnel_peer_id", tunnelPeerID, 1)
		te.withActionParam("needs_buffering", buffer, 1)
	}

	return te, nil
}

func BuildP4Entry() *p4.Entity_TableEntry {
	return &p4.Entity_TableEntry{}
}

func BuildTerminationsTableEntry(pdr pdr, relatedFAR far) (*AppTableEntry, error) {
	te := NewP4RtTableEntry()
	te.TableName = "PreQosPipe.terminations"

	// FIXME: how to get slice ID?
	te.withExactMatchField("slice_id", 0, 1)
	te.withExactMatchField("src_iface", pdr.srcIface, 1)
	if pdr.srcIface == access {
		// TODO: how to get UE addr?
		te.withExactMatchField("ue_address", 0, 4)
	} else {
		te.withExactMatchField("ue_address", pdr.dstIP, 4)
	}

	if relatedFAR.Drops() {
		te.ActionName = "term_drop"
	} else if relatedFAR.Forwards() || relatedFAR.Buffers() {
		te.withActionParam("ctr_idx", pdr.ctrID, 4)
		// FIXME: use how QFI to TC mapping
		te.withActionParam("tc", 0, 1)
		if pdr.srcIface == access {
			te.ActionName = "term_uplink"
		} else if pdr.srcIface == core {
			te.ActionName = "term_downlink"
			// FIXME: provide QFI, from QER??
			te.withActionParam("qfi", 0, 1)
			te.withActionParam("teid", relatedFAR.tunnelTEID, 4)
		}
	} else {
		return nil, fmt.Errorf("unknown FAR action")
	}

	return te, nil
}

func BuildGTPTunnelPeerTableEntry(tunnelPeerID uint8, far far) (*AppTableEntry, error) {
	te := NewP4RtTableEntry()
	te.TableName = "PreQosPipe.tunnel_peers"
	te.withExactMatchField("tunnel_peer_id", tunnelPeerID, 1)

	te.ActionName = "load_tunnel_param"
	te.withActionParam("src_addr", far.tunnelIP4Src, 4)
	te.withActionParam("dst_addr", far.tunnelIP4Dst, 4)
	// TODO: verify if we use proper port here
	te.withActionParam("sport", far.tunnelPort, 4)

	return te, nil
}



