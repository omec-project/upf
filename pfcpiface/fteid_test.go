// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Canonical Ltd.

package pfcpiface_test

import (
	"testing"

	"github.com/omec-project/upf-epc/pfcpiface"
)

func TestFTEIDAllocate(t *testing.T) {
	fteidGenerator := pfcpiface.NewFTEIDGenerator()

	fteid, err := fteidGenerator.Allocate()
	if err != nil {
		t.Errorf("FTEID allocation failed: %v", err)
	}
	if fteid < 1 {
		t.Errorf("FTEID allocation failed, value is too small: %v", fteid)
	}
	if !fteidGenerator.IsAllocated(fteid) {
		t.Errorf("FTEID was not allocated")
	}
}

func TestFTEIDFree(t *testing.T) {
	fteidGenerator := pfcpiface.NewFTEIDGenerator()
	fteid, err := fteidGenerator.Allocate()
	if err != nil {
		t.Errorf("FTEID allocation failed: %v", err)
	}

	fteidGenerator.FreeID(fteid)

	if fteidGenerator.IsAllocated(fteid) {
		t.Errorf("FTEID was not freed")
	}
}
