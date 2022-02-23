package pfcpiface

import "sync"

type InMemoryStore struct {
	mu sync.Mutex
	sessions   map[uint64]PFCPSession
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions: make(map[uint64]PFCPSession),
	}
}

func (i *InMemoryStore) GetAllSessions() []PFCPSession {
	i.mu.Lock()
	defer i.mu.Unlock()

	sessions := make([]PFCPSession, 0)
	for _, v := range i.sessions {
		sessions = append(sessions, v)
	}

	return sessions
}

func (i *InMemoryStore) UpdateSession(session PFCPSession) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if session.localSEID == 0 {
		return ErrInvalidArgument("session.localSEID", session.localSEID)
	}

	i.sessions[session.localSEID] = session

	return nil
}

func (i *InMemoryStore) RemoveSession(fseid uint64) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	delete(i.sessions, fseid)

	return nil
}

func (i *InMemoryStore) GetSession(fseid uint64) (PFCPSession, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	sess, ok :=  i.sessions[fseid]

	return sess, ok
}
