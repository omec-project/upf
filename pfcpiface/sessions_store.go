// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

type SessionsStore interface {
	// PutSession modifies the PFCP Session data indexed by a given F-SEID or
	// inserts a new PFCP Session record, if it doesn't exist yet.
	PutSession(session PFCPSession) error
	// GetSession returns the PFCP Session data based on F-SEID.
	GetSession(fseid uint64) (PFCPSession, bool)
	// GetAllSessions returns all the PFCP Session records that are currently stored.
	GetAllSessions() []PFCPSession
	// DeleteSession removes a PFCP Session record indexed by F-SEID.
	DeleteSession(fseid uint64) error
	// DeleteAllSessions removes all PFCP sessions from the store.
	// Returns true on success.
	DeleteAllSessions() bool
}
