// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"fmt"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

const (
	DefaultPriority = 0
)

type P4rtTranslator struct {
	p4Info     p4ConfigV1.P4Info
}

func newP4RtTranslator(p4info p4ConfigV1.P4Info) *P4rtTranslator {
	return &P4rtTranslator{
		p4Info: p4info,
	}
}

func (t *P4rtTranslator) tableID(name string) uint32 {
	for _, table := range t.p4Info.Tables {
		if table.Preamble.Name == name {
			return table.Preamble.Id
		}
	}

	return invalidID
}

func (t *P4rtTranslator) counterID(name string) uint32 {
	for _, counter := range t.p4Info.GetCounters() {
		if counter.Preamble.Name == name {
			return counter.Preamble.Id
		}
	}
	return invalidID
}

func (t *P4rtTranslator) actionID(name string) uint32 {
	for _, action := range t.p4Info.Actions {
		if action.Preamble.Name == name {
			return action.Preamble.Id
		}
	}

	return invalidID
}

func (t *P4rtTranslator) getEnumVal(enumName string, valName string) ([]byte, error) {
	enumVal, ok := t.p4Info.TypeInfo.SerializableEnums[enumName]
	if !ok {
		err := fmt.Errorf("enum not found with name %s", enumName)
		return nil, err
	}

	for _, enums := range enumVal.Members {
		if enums.Name == valName {
			return enums.Value, nil
		}
	}

	return nil, fmt.Errorf("EnumVal not found")
}


// TODO: find a way to use *p4.TableEntry as receiver
func (t *P4rtTranslator) withExactMatchField(entry *p4.TableEntry, name string, value interface{}) error {
	matchField := &p4.FieldMatch{
		FieldId: 0,
	}



	entry.Match = append(entry.Match, matchField)

	return nil
}

// TODO: implement it
func (t *P4rtTranslator) BuildSessionsTableEntry(pdr pdr, tunnelPeerID uint8, buffer bool) (*p4.TableEntry, error) {
	tableID := t.tableID("PreQosPipe.sessions")




	entry := &p4.TableEntry{
		TableId:  tableID,
		// FIXME: we might want to configure priority
		Priority: DefaultPriority,
		//Action:   tableAction,
	}

	return entry, nil
}

// TODO: implement it
func (t *P4rtTranslator) BuildTerminationsTableEntry(pdr pdr, relatedFAR far) (*p4.TableEntry, error) {
	return nil, nil
}

// TODO: implement it
func (t *P4rtTranslator) BuildGTPTunnelPeerTableEntry(tunnelPeerID uint8, far far) (*p4.TableEntry, error) {
	return nil, nil
}