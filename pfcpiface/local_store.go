// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"sync"

	"github.com/omec-project/upf-epc/logger"
)

type InMemoryStore struct {
	// sessions stores all PFCP sessions.
	// sync.Map is optimized for case when multiple goroutines
	// read, write, and overwrite entries for disjoint sets of keys.
	sessions sync.Map
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

func (i *InMemoryStore) GetAllSessions() []PFCPSession {
	sessions := make([]PFCPSession, 0)

	i.sessions.Range(func(key, value interface{}) bool {
		v := value.(PFCPSession)
		sessions = append(sessions, v)
		return true
	})

	logger.PfcpLog.With("sessions", sessions).Debugln("got all PFCP sessions from local store")

	return sessions
}

func (i *InMemoryStore) PutSession(session PFCPSession) error {
	if session.localSEID == 0 {
		return ErrInvalidArgument("session.localSEID", session.localSEID)
	}

	i.sessions.Store(session.localSEID, session)

	logger.PfcpLog.With("session", session).Debugln("saved PFCP sessions to local store")

	return nil
}

func (i *InMemoryStore) DeleteSession(fseid uint64) error {
	i.sessions.Delete(fseid)

	logger.PfcpLog.With("F-SEID", fseid).Debugln("PFCP session removed from local store")

	return nil
}

func (i *InMemoryStore) DeleteAllSessions() bool {
	i.sessions.Range(func(key, value interface{}) bool {
		i.sessions.Delete(key)
		return true
	})

	logger.P4Log.Debugln("all PFCP sessions removed from local store")

	return true
}

func (i *InMemoryStore) GetSession(fseid uint64) (PFCPSession, bool) {
	sess, ok := i.sessions.Load(fseid)
	if !ok {
		return PFCPSession{}, false
	}

	session, ok := sess.(PFCPSession)
	if !ok {
		return PFCPSession{}, false
	}

	logger.PfcpLog.With("session", session).Debugln("Got PFCP session from local store")

	return session, ok
}
