// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// The control plane tells the RAN one uplink TEID per PDU session, and asks for a shared
// F-TEID by giving the session's uplink PDRs a common CHOOSE ID (TS 29.244 clause 5.5.3).
// Allocating per PDR regardless left every PDR but the one the RAN was told on a TEID that
// no packet arrives on: with two PCC rules, all uplink that rule did not match was dropped.

const (
	fteidCH   = 0x04
	fteidCHID = 0x08
	fteidV4   = 0x01
)

// chooseUplinkPDR is an uplink PDR asking the UPF to choose its F-TEID, with the given CHOOSE
// ID when withID is set. The SDF filter makes each PDR its own datapath entry. With CH set, V4
// names the address family and no address is carried (TS 29.244 clause 8.2.3), which is what
// the SD-Core SMF change builds.
func chooseUplinkPDR(pdrID uint16, chooseID uint8, withID bool, flow string) *ie.IE {
	flags := uint8(fteidCH | fteidV4)
	if withID {
		flags |= fteidCHID
	}

	return ie.NewCreatePDR(
		ie.NewPDRID(pdrID),
		ie.NewPrecedence(uint32(pdrID)),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(flags, 0, nil, nil, chooseID),
			ie.NewUEIPAddress(0x02, "10.250.0.7", "", 0, 0),
			ie.NewSDFFilter(flow, "", "", "", 0),
		),
		ie.NewFARID(uint32(pdrID)),
	)
}

// establishUplink sends an establishment carrying the given PDRs and returns the TEID the
// response assigns to each PDR ID.
func establishUplink(t *testing.T, pdrs ...*ie.IE) map[uint16]uint32 {
	t.Helper()

	pConn := establishingConn(t, nil)
	pConn.upf.fteidGenerator = NewFTEIDGenerator()
	pConn.upf.accessIP = net.ParseIP("10.0.0.1")

	ies := []*ie.IE{
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(0xC0FFEE, net.ParseIP("127.0.0.1"), nil),
	}
	ies = append(ies, pdrs...)

	rsp, err := pConn.handleSessionEstablishmentRequest(
		message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0, ies...))
	if err != nil {
		t.Fatalf("establishment refused: %v", err)
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	assertAccepted(t, seres.Cause, "establishment")

	teids := make(map[uint16]uint32)

	for _, created := range seres.CreatedPDR {
		pdrID, err := created.PDRID()
		if err != nil {
			t.Fatalf("a Created PDR without a PDR ID: %v", err)
		}

		fteid, err := created.FTEID()
		if err != nil {
			continue
		}

		teids[pdrID] = fteid.TEID
	}

	if len(teids) != len(pdrs) {
		t.Fatalf("%d Created PDRs carry an F-TEID, want one for each of the %d PDRs", len(teids), len(pdrs))
	}

	return teids
}

func TestPDRsWithOneChooseIDShareOneFTEID(t *testing.T) {
	teids := establishUplink(t,
		chooseUplinkPDR(5, 1, true, "permit out ip from any to assigned"),
		chooseUplinkPDR(6, 1, true, "permit out ip from 192.0.2.10 to assigned"),
	)

	if teids[5] != teids[6] {
		t.Errorf("PDRs 5 and 6 share CHOOSE ID 1 but were given TEIDs %#x and %#x", teids[5], teids[6])
	}
}

func TestEachChooseIDHasAnFTEIDOfItsOwn(t *testing.T) {
	teids := establishUplink(t,
		chooseUplinkPDR(5, 1, true, "permit out ip from any to assigned"),
		chooseUplinkPDR(6, 2, true, "permit out ip from 192.0.2.10 to assigned"),
		chooseUplinkPDR(7, 1, true, "permit out ip from 192.0.2.11 to assigned"),
	)

	if teids[5] != teids[7] {
		t.Errorf("PDRs 5 and 7 share CHOOSE ID 1 but were given TEIDs %#x and %#x", teids[5], teids[7])
	}

	if teids[6] == teids[5] {
		t.Errorf("PDR 6 has CHOOSE ID 2 and was given CHOOSE ID 1's TEID %#x", teids[6])
	}
}

// A CHOOSE without a CHOOSE ID asks for an F-TEID of the PDR's own, as before. A control
// plane that has not adopted CHOOSE ID is answered exactly as it was.
func TestPDRsWithoutAChooseIDEachHaveTheirOwnFTEID(t *testing.T) {
	teids := establishUplink(t,
		chooseUplinkPDR(5, 0, false, "permit out ip from any to assigned"),
		chooseUplinkPDR(6, 0, false, "permit out ip from 192.0.2.10 to assigned"),
	)

	if teids[5] == teids[6] {
		t.Errorf("PDRs 5 and 6 carry no CHOOSE ID and were both given TEID %#x", teids[5])
	}
}

// A CHOOSE ID of zero is a value like any other once the CHID flag is set: the flag says
// whether there is one, and an implementation that tested the value instead would leave
// PDRs asking for ID 0 on separate TEIDs.
func TestChooseIDZeroIsAChooseID(t *testing.T) {
	teids := establishUplink(t,
		chooseUplinkPDR(5, 0, true, "permit out ip from any to assigned"),
		chooseUplinkPDR(6, 0, true, "permit out ip from 192.0.2.10 to assigned"),
	)

	if teids[5] != teids[6] {
		t.Errorf("PDRs 5 and 6 share CHOOSE ID 0 but were given TEIDs %#x and %#x", teids[5], teids[6])
	}
}

// A PDR without a CHOOSE ID is not in any group, including the one whose ID is zero: in a
// message that mixes the two, it still gets an F-TEID of its own.
func TestAPDRWithoutAChooseIDDoesNotJoinChooseIDZero(t *testing.T) {
	teids := establishUplink(t,
		chooseUplinkPDR(5, 0, true, "permit out ip from any to assigned"),
		chooseUplinkPDR(6, 0, false, "permit out ip from 192.0.2.10 to assigned"),
	)

	if teids[5] == teids[6] {
		t.Errorf("PDR 6 carries no CHOOSE ID and was given CHOOSE ID 0's TEID %#x", teids[6])
	}
}

// The same seen from the other side: a PDR without a CHOOSE ID that comes first does not
// start group 0, so a later pair with CHOOSE ID 0 shares a TEID with each other and not with
// it.
func TestAPDRWithoutAChooseIDDoesNotStartGroupZero(t *testing.T) {
	teids := establishUplink(t,
		chooseUplinkPDR(5, 0, false, "permit out ip from any to assigned"),
		chooseUplinkPDR(6, 0, true, "permit out ip from 192.0.2.10 to assigned"),
		chooseUplinkPDR(7, 0, true, "permit out ip from 192.0.2.11 to assigned"),
	)

	if teids[6] == teids[5] {
		t.Errorf("PDR 6 has CHOOSE ID 0 and was given TEID %#x of PDR 5, which has none", teids[6])
	}

	if teids[6] != teids[7] {
		t.Errorf("PDRs 6 and 7 share CHOOSE ID 0 but were given TEIDs %#x and %#x", teids[6], teids[7])
	}
}
