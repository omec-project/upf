// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"math/rand"
	"net"
	"testing"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// A UE address the UPF allocated is recorded in the pool's inventory against the
// session's local SEID, and nowhere else that lasts. The session's own rules do not
// keep it: a modification can remove the PDR that carries it, or replace that PDR with
// one naming the address explicitly, and either leaves the session holding an address
// that no rule of its own points at any more.

const (
	testRemoteNodeID = "test-smf"
	tinyPoolCIDR     = "10.251.0.0/30" // .1 and .2, so capacity is observable
	downlinkPDRID    = 1
	downlinkFARID    = 1
)

// establishingConn is teardownConn plus what the session handlers need: a source of
// local SEIDs, and a NodeID the request can match.
func establishingConn(t *testing.T, pool *IPPool) *PFCPConn {
	t.Helper()

	pConn := teardownConn(t, pool)
	pConn.rng = rand.New(rand.NewSource(1)) // #nosec G404 -- deterministic local SEIDs for a test
	pConn.maxRetries = 100
	pConn.nodeID = nodeID{
		localIE: ie.NewNodeID("127.0.0.1", "", ""),
		local:   "127.0.0.1",
		remote:  testRemoteNodeID,
	}

	return pConn
}

// chooseUEAddress is what a control plane sends when it wants the UPF to pick the
// address: CHV4 set and no address carried, which is the only input that allocates.
func chooseUEAddress() *ie.IE {
	return ie.NewUEIPAddress(0x10, "", "", 0, 0)
}

func downlinkFAR() *ie.IE {
	return ie.NewCreateFAR(
		ie.NewFARID(downlinkFARID),
		ie.NewApplyAction(ActionForward),
		ie.NewForwardingParameters(ie.NewDestinationInterface(ie.DstInterfaceCore)),
	)
}

// establishAllocatingSession drives a real establishment whose downlink PDR asks the
// UPF to choose the UE address, and returns the local SEID the pool is keyed by.
func establishAllocatingSession(t *testing.T, pConn *PFCPConn, remoteSEID uint64) uint64 {
	t.Helper()

	sereq := message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(remoteSEID, net.ParseIP("127.0.0.1"), nil),
		ie.NewCreatePDR(
			ie.NewPDRID(downlinkPDRID),
			ie.NewPrecedence(0),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				chooseUEAddress(),
			),
			ie.NewFARID(downlinkFARID),
		),
		downlinkFAR(),
	)

	rsp, err := pConn.handleSessionEstablishmentRequest(sereq)
	if err != nil {
		t.Fatalf("establishment refused: %v", err)
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	assertAccepted(t, seres.Cause, "establishment")

	if seres.UPFSEID == nil {
		t.Fatal("the establishment response carries no UP F-SEID")
	}

	fseid, err := seres.UPFSEID.FSEID()
	if err != nil {
		t.Fatalf("could not read the local F-SEID: %v", err)
	}

	// The response must carry the address the UPF chose, or the session under test is
	// not the one this file is about.
	if len(seres.CreatedPDR) == 0 {
		t.Fatal("the establishment response carries no Created PDR, so no address was allocated")
	}

	return fseid.SEID
}

func assertAccepted(t *testing.T, causeIE *ie.IE, what string) {
	t.Helper()

	if causeIE == nil {
		t.Fatalf("%s answered without a Cause IE", what)
	}

	cause, err := causeIE.Cause()
	if err != nil {
		t.Fatalf("could not read the Cause of the %s response: %v", what, err)
	}

	if cause != ie.CauseRequestAccepted {
		t.Fatalf("%s was answered with cause %d, expected %d",
			what, cause, ie.CauseRequestAccepted)
	}
}

func modifySession(t *testing.T, pConn *PFCPConn, localSEID uint64, ies ...*ie.IE) {
	t.Helper()

	smreq := message.NewSessionModificationRequest(0, 0, localSEID, 2, 0, ies...)

	rsp, err := pConn.handleSessionModificationRequest(smreq)
	if err != nil {
		t.Fatalf("modification refused: %v", err)
	}

	smres, ok := rsp.(*message.SessionModificationResponse)
	if !ok {
		t.Fatalf("expected a Session Modification Response, got %T", rsp)
	}

	assertAccepted(t, smres.Cause, "modification")
}

func deleteSession(t *testing.T, pConn *PFCPConn, localSEID uint64) {
	t.Helper()

	sdreq := message.NewSessionDeletionRequest(0, 0, localSEID, 3, 0)

	rsp, err := pConn.handleSessionDeletionRequest(sdreq)
	if err != nil {
		t.Fatalf("deletion refused: %v", err)
	}

	sdres, ok := rsp.(*message.SessionDeletionResponse)
	if !ok {
		t.Fatalf("expected a Session Deletion Response, got %T", rsp)
	}

	assertAccepted(t, sdres.Cause, "deletion")
}

// assertPoolIsWhole allocates the whole of a two-address pool, which only succeeds if
// the session under test gave its address back. Capacity rather than identity: Release
// enqueues at the back of the free pool while LookupOrAllocIP takes from the front, so
// "the next session gets the same address" is indistinguishable from a leak.
func assertPoolIsWhole(t *testing.T, pool *IPPool, after string) {
	t.Helper()

	for i := uint64(1); i <= 2; i++ {
		if _, err := pool.LookupOrAllocIP(0xF000 + i); err != nil {
			t.Fatalf("allocation %d of 2 after %s failed: %v; the session did not "+
				"release the address the UPF allocated for it", i, after, err)
		}
	}
}

// TestASessionThatLostItsAllocatingPDRStillReleasesItsAddress covers a modification
// that removes the PDR carrying the allocated address. The rule is spliced out of the
// session and nothing returns the address with it.
func TestASessionThatLostItsAllocatingPDRStillReleasesItsAddress(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)
	localSEID := establishAllocatingSession(t, pConn, 0xC0FFEE)

	modifySession(t, pConn, localSEID, ie.NewRemovePDR(ie.NewPDRID(downlinkPDRID)))
	deleteSession(t, pConn, localSEID)

	assertPoolIsWhole(t, pool, "a session whose allocating PDR had been removed was deleted")
}

// TestASessionWhoseAddressBecameExplicitStillReleasesIt covers the other half: the
// control plane learned the address from the establishment response and repeats it in
// an Update PDR. The rule is still there, but it no longer says the UPF allocated it.
func TestASessionWhoseAddressBecameExplicitStillReleasesIt(t *testing.T) {
	pool, err := NewIPPool(tinyPoolCIDR)
	if err != nil {
		t.Fatal(err)
	}

	pConn := establishingConn(t, pool)
	localSEID := establishAllocatingSession(t, pConn, 0xC0FFEE)

	session, ok := pConn.store.GetSession(localSEID)
	if !ok {
		t.Fatal("the established session is not in the store")
	}

	ueAddress := int2ip(session.pdrs[0].ueAddress)

	modifySession(t, pConn, localSEID, ie.NewUpdatePDR(
		ie.NewPDRID(downlinkPDRID),
		ie.NewPrecedence(0),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewUEIPAddress(0x2, ueAddress.String(), "", 0, 0),
		),
		ie.NewFARID(downlinkFARID),
	))

	deleteSession(t, pConn, localSEID)

	assertPoolIsWhole(t, pool, "a session whose UE address had become explicit was deleted")
}

// TestSessionsSurviveADeploymentWithNoPool covers the other half of the configuration
// space: enable_ue_ip_alloc is off, the control plane assigns the UE addresses, and
// initTimersAndIPPool never built a pool -- so upf.ippool is nil while every teardown
// path still asks it to release.
func TestSessionsSurviveADeploymentWithNoPool(t *testing.T) {
	pConn := establishingConn(t, nil)

	sereq := message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(0xC0FFEE, net.ParseIP("127.0.0.1"), nil),
		ie.NewCreatePDR(
			ie.NewPDRID(downlinkPDRID),
			ie.NewPrecedence(0),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewUEIPAddress(0x2, "10.250.0.1", "", 0, 0),
			),
			ie.NewFARID(downlinkFARID),
		),
		downlinkFAR(),
	)

	rsp, err := pConn.handleSessionEstablishmentRequest(sereq)
	if err != nil {
		t.Fatalf("establishment refused: %v", err)
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	assertAccepted(t, seres.Cause, "establishment")

	fseid, err := seres.UPFSEID.FSEID()
	if err != nil {
		t.Fatalf("could not read the UP F-SEID: %v", err)
	}

	deleteSession(t, pConn, fseid.SEID)
}
