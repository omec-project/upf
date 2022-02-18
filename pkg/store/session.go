package store

type PFCPSession struct {
	FSEID uint64

}

type SessionStore interface {
	CreateSession(PFCPSession)



}
