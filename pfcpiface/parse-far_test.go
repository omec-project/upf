// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"github.com/omec-project/upf-epc/test/integration"
	"net"
	"testing"
)

func TestFARParsing(t *testing.T) {

	f := far{}

	op := create
	var validFARID uint32 = 1
	var validFARAction uint8 = ActionForward

	validFAR := integration.NewUplinkFAR(integration.IEMethod(op), validFARID, validFARAction)
	// using invalid action (0)
	invalidFAR := integration.NewDownlinkFAR(integration.IEMethod(op), 2, 0, 100, "10.0.10.1")

	mockUpf := &upf{
		accessIP: net.ParseIP("192.168.0.1"),
		coreIP:   net.ParseIP("10.0.0.1"),
	}

	err := f.parseFAR(validFAR, 100, mockUpf, op)
	if err != nil {
		t.Errorf("Error parsing FAR: %v", err)
	}

	if f.farID != validFARID {
		t.Errorf("Error parsing FAR: FAR ID does not match")
	}

	if f.applyAction != validFARAction {
		t.Errorf("Error parsing FAR: FAR action does not match")
	}

	err = f.parseFAR(invalidFAR, 101, mockUpf, op)
	if err == nil {
		t.Errorf("Error parsing FAR: FAR is invalid but no error is returned")
	}
}
