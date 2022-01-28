// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"net"
	"testing"

	"github.com/omec-project/upf-epc/test/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

type testCase struct {
	input       *ie.IE
	op          operation
	description string
}

func TestParseFAR(t *testing.T) {
	createOp, updateOp := create, update

	for _, scenario := range []testCase{
		{
			op:          createOp,
			input:       integration.NewUplinkFAR(integration.IEMethod(createOp), 999, ActionDrop),
			description: "Valid Uplink FAR input with create operation",
		},
		{
			op: updateOp,
			input: integration.NewDownlinkFAR(integration.IEMethod(updateOp),
				1,
				ActionForward,
				100,
				"10.0.0.1",
			),
			description: "Valid Downlink FAR input with update operation",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			mockFar := far{}
			mockUpf := &upf{
				accessIP: net.ParseIP("192.168.0.1"),
				coreIP:   net.ParseIP("10.0.0.1"),
			}

			err := mockFar.parseFAR(scenario.input, 100, mockUpf, scenario.op)
			require.NoError(t, err)

			expectedID, err := scenario.input.FARID()
			require.NoError(t, err)
			assert.Equal(t, mockFar.farID, expectedID)
		})
	}
}

func TestParseFARShouldError(t *testing.T) {
	createOp, updateOp := create, update

	for _, scenario := range []testCase{
		{
			op:          createOp,
			input:       integration.NewUplinkFAR(integration.IEMethod(createOp), 1, 0),
			description: "Uplink FAR with invalid action",
		},
		{
			op: updateOp,
			input: integration.NewDownlinkFAR(integration.IEMethod(updateOp),
				1,
				0,
				100,
				"10.0.0.1",
			),
			description: "Downlink FAR with invalid action",
		},
		{
			op: createOp,
			input: ie.NewCreateFAR(
				ie.NewApplyAction(ActionDrop),
				ie.NewUpdateForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceAccess),
					ie.NewOuterHeaderCreation(0x100, 100, "10.0.0.1", "", 0, 0, 0),
				),
			),
			description: "Malformed Downlink FAR with missing FARID",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			mockFar := far{}
			mockUpf := &upf{
				accessIP: net.ParseIP("192.168.0.1"),
				coreIP:   net.ParseIP("10.0.0.1"),
			}

			err := mockFar.parseFAR(scenario.input, 101, mockUpf, scenario.op)
			require.Error(t, err)
		})
	}
}
