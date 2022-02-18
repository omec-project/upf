package pfcpiface

type SessionStore interface {
	UpdateSession(PFCPSession) error
	GetSession(fseid uint64) (PFCPSession, bool)
	GetAllSessions() []PFCPSession
	RemoveSession(fseid uint64) error
}
