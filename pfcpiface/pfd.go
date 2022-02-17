// SPDX-License-Identifier: Apache-2.0
// Copyright 2021 Intel Corporation

package pfcpiface

// PFD holds the switch level application IDs.
type appPFD struct {
	appID     string
	flowDescs []string
}

// ResetAppPFDs resets the map of application PFDs.
func (pConn *PFCPConn) ResetAppPFDs() {
	pConn.appPFDs = make(map[string]appPFD)
}

// NewAppPFD stores app PFD in session mgr.
func (pConn *PFCPConn) NewAppPFD(appID string) {
	pConn.appPFDs[appID] = appPFD{
		appID:     appID,
		flowDescs: make([]string, 0, MaxItems),
	}
}

// RemoveAppPFD removes appPFD using appID.
func (pConn *PFCPConn) RemoveAppPFD(appID string) {
	delete(pConn.appPFDs, appID)
}
