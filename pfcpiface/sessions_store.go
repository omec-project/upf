package pfcpiface

type SessionsStore interface {
	// UpdateSession modifies the PFCP Session data indexed by a given F-SEID or
	// inserts a new PFCP Session record, if it doesn't exist yet.
	UpdateSession(session PFCPSession) error
	// GetSession returns the PFCP Session data based on F-SEID.
	GetSession(fseid uint64) (PFCPSession, bool)
	// GetAllSessions returns all the PFCP Session records that are currently stored.
	GetAllSessions() []PFCPSession
	// RemoveSession deletes a PFCP Session record indexed by F-SEID.
	RemoveSession(fseid uint64) error
}
