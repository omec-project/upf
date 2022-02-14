// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package pfcpiface

import (
	"testing"

	pfcpsimLib "github.com/omec-project/pfcpsim/pkg/pfcpsim/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

type qerTestCase struct {
	input       *ie.IE
	expected    *qer
	description string
}

func TestParseQER(t *testing.T) {
	FSEID := uint64(100)

	for _, scenario := range []qerTestCase{
		{
			input: pfcpsimLib.NewQERBuilder().
				WithID(999).
				WithMethod(pfcpsimLib.IEMethod(create)).
				WithQFI(0x09).Build(),
			expected: &qer{
				qerID: 999,
				qfi:   0x09,
				fseID: FSEID,
			},
			description: "Valid Create QER input",
		},
		{
			input: pfcpsimLib.NewQERBuilder().
				WithID(999).
				WithMethod(pfcpsimLib.IEMethod(update)).
				WithQFI(0x09).Build(),
			expected: &qer{
				qerID: 999,
				qfi:   0x09,
				fseID: FSEID,
			},
			description: "Valid Update QER input",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			mockQER := &qer{}

			err := mockQER.parseQER(scenario.input, FSEID)
			require.NoError(t, err)

			assert.Equal(t, scenario.expected, mockQER)
		})
	}
}

func TestParseQERShouldError(t *testing.T) {
	FSEID := uint64(100)

	for _, scenario := range []qerTestCase{
		{
			input: ie.NewCreateQER(
				ie.NewQFI(64),
				ie.NewGateStatus(0, 0),
				ie.NewMBR(0, 1),
				ie.NewGBR(2, 3),
			),
			expected:    &qer{},
			description: "Invalid QER input: no QER ID provided",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			mockQER := &qer{}

			err := mockQER.parseQER(scenario.input, FSEID)
			require.Error(t, err)

			assert.Equal(t, scenario.expected, mockQER)
		})
	}
}
