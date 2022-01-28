package integration

// this file should contain all the struct defs/constants used among different test cases.

type pfcpSessionData struct {
	nbAddress string
	ueAddress string
	upfN3Address string

	ulTEID uint32
	dlTEID uint32

	sessQFI uint8
	appQFI  uint8
}

