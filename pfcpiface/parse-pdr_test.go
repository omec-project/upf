package main

import (
	"net"
	"testing"

	pfcpsimLib "github.com/omec-project/pfcpsim/pkg/pfcpsim/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

type pdrTestCase struct {
	input       *ie.IE
	expected    *pdr
	description string
}

func TestParsePDR(t *testing.T) {
	UEAddress := net.ParseIP("10.0.1.1")
	N3Address := net.ParseIP("192.168.0.1")
	FSEID := uint64(100)
	pdrID := uint16(999)
	precedence := uint32(1)
	qerID := uint32(4)
	farID := uint32(2)
	teid := uint32(1234)

	for _, scenario := range []pdrTestCase{
		{
			input: pfcpsimLib.NewPDRBuilder().
				WithID(pdrID).
				WithMethod(pfcpsimLib.IEMethod(create)).
				WithPrecedence(precedence).
				WithFARID(farID).
				AddQERID(qerID).
				WithN3Address(N3Address.String()).
				WithTEID(teid).
				MarkAsUplink().
				BuildPDR(),
			expected: &pdr{
				pdrID:            uint32(pdrID), // go-pfcp uses uint16 to create PDRIDs, while in pfcpiface we use uint32
				precedence:       precedence,
				tunnelIP4Dst:     ip2int(N3Address),
				tunnelIP4DstMask: 0xffffffff, // 32 bit mask
				srcIface:         access,
				srcIfaceMask:     0xff,
				fseID:            FSEID,
				tunnelTEID:       teid,
				tunnelTEIDMask:   0xffffffff,
				farID:            farID,
				qerIDList:        []uint32{qerID},
				needDecap:        0x1, // OuterHeaderRemoval IE is present for uplink PDRs
			},
			description: "Valid Uplink Create PDR input",
		},
		{
			input: pfcpsimLib.NewPDRBuilder().
				WithID(pdrID).
				WithMethod(pfcpsimLib.IEMethod(update)).
				WithFARID(farID).
				AddQERID(qerID).
				WithUEAddress(UEAddress.String()).
				MarkAsDownlink().
				BuildPDR(),
			expected: &pdr{
				pdrID:        uint32(pdrID),
				fseID:        FSEID,
				farID:        farID,
				srcIface:     core,
				srcIfaceMask: 0xff,
				ueAddress:    ip2int(UEAddress),
				qerIDList:    []uint32{qerID},
			},
			description: "Valid downlink Update PDR input",
		},
		{
			input: pfcpsimLib.NewPDRBuilder().
				WithID(pdrID).
				WithMethod(pfcpsimLib.IEMethod(create)).
				WithFARID(farID).
				AddQERID(qerID).
				WithUEAddress(UEAddress.String()).
				MarkAsDownlink().
				BuildPDR(),
			expected: &pdr{
				pdrID:        uint32(pdrID),
				fseID:        FSEID,
				farID:        farID,
				srcIface:     core,
				srcIfaceMask: 0xff,
				ueAddress:    ip2int(UEAddress),
				qerIDList:    []uint32{qerID},
			},
			description: "Valid downlink Create PDR input",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			mockMapPFD := make(map[string]appPFD)
			mockMapPFD["1"] = appPFD{
				appID:     "1",
				flowDescs: nil,
			}
			mockPDR := &pdr{}
			mockIPPool, _ := NewIPPool("10.0.0.0")

			err := mockPDR.parsePDR(scenario.input, FSEID, mockMapPFD, mockIPPool)
			require.NoError(t, err)

			assert.Equal(t, mockPDR, scenario.expected)
		})
	}
}

func TestParsePDRShouldError(t *testing.T) {
	var FSEID uint64 = 100

	for _, scenario := range []pdrTestCase{
		{
			input: ie.NewCreatePDR(
				ie.NewPrecedence(0),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
					ie.NewFTEID(0x00, 0, net.ParseIP(""), nil, 0),
					ie.NewSDFFilter("", "", "", "", 1),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(2),
			),
			expected: &pdr{
				qerIDList: []uint32{},
			},
			description: "Malformed Uplink PDR input without PDR ID",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			mockMapPFD := make(map[string]appPFD)
			mockMapPFD["1"] = appPFD{
				appID:     "1",
				flowDescs: nil,
			}
			mockPDR := &pdr{}
			mockIPPool, _ := NewIPPool("10.0.0.0")

			err := mockPDR.parsePDR(scenario.input, FSEID, mockMapPFD, mockIPPool)
			require.Error(t, err)

			assert.Equal(t, scenario.expected, mockPDR)
		})
	}
}
