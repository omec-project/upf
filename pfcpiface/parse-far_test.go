// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package main

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

type testCase struct {
	input       *ie.IE
	op          operation
	description string
}

// TODO use pfcpsim library to create FARs

func TestParseFAR(t *testing.T) {
	createOp, updateOp := create, update

	for _, scenario := range []testCase{
		{
			op: createOp,
			input: ie.NewCreateFAR(
				ie.NewFARID(999),
				ie.NewApplyAction(ActionDrop),
				ie.NewForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceCore),
				),
			),
			description: "Valid Uplink FAR input with create operation",
		},
		{
			op: updateOp,
			input: ie.NewUpdateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(ActionForward),
				ie.NewUpdateForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceAccess),
					ie.NewOuterHeaderCreation(0x100, 100, "10.0.0.1", "", 0, 0, 0),
				),
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
			op: createOp,
			input: ie.NewCreateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(0),
				ie.NewForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceCore),
				),
			),
			description: "Uplink FAR with invalid action",
		},
		{
			op: updateOp,
			input: ie.NewUpdateFAR(
				ie.NewFARID(1),
				ie.NewApplyAction(0),
				ie.NewUpdateForwardingParameters(
					ie.NewDestinationInterface(ie.DstInterfaceAccess),
					ie.NewOuterHeaderCreation(0x100, 100, "10.0.0.1", "", 0, 0, 0),
				),
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
