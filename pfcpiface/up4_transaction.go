// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
)

type transactionType uint8

const (
	TransactionUnknown transactionType = iota
	TransactionCreate
	TransactionModify
	TransactionDelete
)

// transactionContext stores shared values that are used by more than one operation in the UP4 transaction.
type transactionContext struct {
	// PDR ID -> application ID
	applicationIDs map[uint32]uint8
	// FAR ID -> tunnel peer ID
	tunnelPeerIDs map[uint32]uint8
}

// UP4Transaction represents single, atomic operation performed within the UP4 device context.
// All operations on UP4Transaction are NOT thread-safe.
type UP4Transaction struct {
	txType  transactionType
	success bool

	ctx transactionContext

	updates []*p4.Update

	onRollback []func()
}

func NewUP4Transaction(t transactionType) *UP4Transaction {
	return &UP4Transaction{
		txType:  t,
		updates: make([]*p4.Update, 0),
		ctx: transactionContext{
			applicationIDs: make(map[uint32]uint8),
			tunnelPeerIDs:  make(map[uint32]uint8),
		},
	}
}

func (t *UP4Transaction) WithApplicationID(pdrID uint32, applicationID uint8) {
	t.ctx.applicationIDs[pdrID] = applicationID
}

func (t *UP4Transaction) GetApplicationID(pdrID uint32) (uint8, bool) {
	appID, exists := t.ctx.applicationIDs[pdrID]
	return appID, exists
}

func (t *UP4Transaction) WithTunnelPeerID(farID uint32, tunnelPeerID uint8) {
	t.ctx.tunnelPeerIDs[farID] = tunnelPeerID
}

func (t *UP4Transaction) GetTunnelPeerID(farID uint32) (uint8, bool) {
	tunnelPeerID, exists := t.ctx.tunnelPeerIDs[farID]
	return tunnelPeerID, exists
}

func (t *UP4Transaction) Success() bool {
	return t.success
}

func (t *UP4Transaction) containsP4Update(update *p4.Update) bool {
	for _, u := range t.updates {
		if u == update {
			return true
		}
	}

	return false
}

func (t *UP4Transaction) WithTableEntry(entry *p4.TableEntry) {
	var updateType p4.Update_Type
	switch t.txType {
	case TransactionCreate:
		updateType = p4.Update_INSERT
	case TransactionModify:
		updateType = p4.Update_MODIFY
	case TransactionDelete:
		updateType = p4.Update_DELETE
	}

	p4Update := &p4.Update{
		Type: updateType,
		Entity: &p4.Entity{
			Entity: &p4.Entity_TableEntry{TableEntry: entry},
		},
	}

	if !t.containsP4Update(p4Update) {
		t.updates = append(t.updates, p4Update)
	}
}

func (t *UP4Transaction) WithMeterEntry(entry *p4.MeterEntry) {
	p4Update := &p4.Update{
		Type: p4.Update_MODIFY, // it's always MODIFY for Meters
		Entity: &p4.Entity{
			Entity: &p4.Entity_MeterEntry{MeterEntry: entry},
		},
	}

	if !t.containsP4Update(p4Update) {
		t.updates = append(t.updates, p4Update)
	}
}

func (t *UP4Transaction) WithTableEntryOverwriteType(entry *p4.TableEntry, updateType p4.Update_Type) {
	p4Update := &p4.Update{
		Type: updateType,
		Entity: &p4.Entity{
			Entity: &p4.Entity_TableEntry{TableEntry: entry},
		},
	}

	if !t.containsP4Update(p4Update) {
		t.updates = append(t.updates, p4Update)
	}
}

func (t *UP4Transaction) OnRollback(f func()) {
	t.onRollback = append(t.onRollback, f)
}
