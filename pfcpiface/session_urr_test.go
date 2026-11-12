// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"errors"
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

var testEpoch = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// A control plane that lost the response to a modification repeats it, Create URR and
// all. The URR it names already exists then, with usage measured for it; replacing it
// must keep that usage, or the bytes are lost, and must not add a second URR, or they
// are reported twice.
func TestCreateURRForAURRTheSessionHoldsKeepsItsUsage(t *testing.T) {
	var s PFCPSession

	s.CreateURR(urr{urrID: testURRID, measureVolume: true}, testEpoch)
	s.CreateURR(urr{urrID: testURRID, measureVolume: true, measureDuration: true}, testEpoch.Add(time.Hour))

	if len(s.urrs) != 1 {
		t.Fatalf("expected one URR, got %d", len(s.urrs))
	}

	if !s.urrs[0].measureDuration {
		t.Fatal("the repeated Create URR's provisioning was not applied")
	}

	if !s.urrs[0].usage.since.Equal(testEpoch) {
		t.Fatalf("the URR's measurement now starts at %v, expected %v: its usage was reset",
			s.urrs[0].usage.since, testEpoch)
	}
}

func TestUpdateURRChangesOnlyTheIEsItCarries(t *testing.T) {
	var s PFCPSession

	s.CreateURR(urr{
		urrID:           testURRID,
		measureVolume:   true,
		triggers:        urrTriggers{volumeThreshold: true},
		volumeThreshold: volumeLimit{total: 1000, hasTotal: true},
		timeThreshold:   time.Minute,
	}, testEpoch)

	err := s.UpdateURR(
		urr{urrID: testURRID, volumeThreshold: volumeLimit{total: 2000, hasTotal: true}},
		urrIEs{volumeThreshold: true}, testEpoch)
	if err != nil {
		t.Fatalf("UpdateURR: %v", err)
	}

	u := s.urrs[0]

	if u.volumeThreshold.total != 2000 {
		t.Fatalf("the Volume Threshold the update carried was not applied: %+v", u.volumeThreshold)
	}

	if !u.measureVolume || !u.triggers.volumeThreshold || u.timeThreshold != time.Minute {
		t.Fatalf("an IE the update did not carry was changed: %v", u)
	}
}

func TestUpdateAndRemoveOfAnUnknownURRFail(t *testing.T) {
	var s PFCPSession

	if err := s.UpdateURR(urr{urrID: testURRID}, urrIEs{}, testEpoch); !errors.Is(err, errNotFound) {
		t.Fatalf("UpdateURR of an unknown URR returned %v, expected a not-found error", err)
	}

	if _, err := s.RemoveURR(testURRID); !errors.Is(err, errNotFound) {
		t.Fatalf("RemoveURR of an unknown URR returned %v, expected a not-found error", err)
	}
}

// An Update PDR carries URR IDs only to change them (Table 7.5.4.2-1). One that carries
// none leaves the PDR measured by the URRs it had.
func TestUpdatePDRKeepsTheURRsOfAPDRWhenItCarriesNone(t *testing.T) {
	var s PFCPSession

	s.CreatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true})

	if err := s.UpdatePDR(pdr{pdrID: downlinkPDRID, precedence: modifiedPrecedence}); err != nil {
		t.Fatalf("UpdatePDR: %v", err)
	}

	if got := s.pdrs[0].urrIDList; len(got) != 1 || got[0] != testURRID {
		t.Fatalf("the PDR's URRs became %v, expected [%d]", got, testURRID)
	}

	err := s.UpdatePDR(pdr{pdrID: downlinkPDRID, urrIDList: []uint32{testURRID + 1}, hasURRIDs: true})
	if err != nil {
		t.Fatalf("UpdatePDR: %v", err)
	}

	if got := s.pdrs[0].urrIDList; len(got) != 1 || got[0] != testURRID+1 {
		t.Fatalf("the PDR's URRs became %v, expected [%d]", got, testURRID+1)
	}
}

// reportingConn is a connection whose UPF reports usage.
func reportingConn(t *testing.T) *PFCPConn {
	t.Helper()

	pConn := establishingConn(t, nil)
	pConn.upf.enableUsageReporting = true
	pConn.upf.urrPollInterval = time.Hour

	return pConn
}

func volumeURR(id uint32) *ie.IE {
	return ie.NewCreateURR(
		ie.NewURRID(id),
		ie.NewMeasurementMethod(0, 1, 0),
		ie.NewReportingTriggers(triggerOctet5VOLTH, 0),
		ie.NewVolumeThreshold(volumeTOVOL, 1000, 0, 0),
	)
}

func TestEstablishmentStoresTheSessionsURRs(t *testing.T) {
	pConn := reportingConn(t)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID)), downlinkFAR(), volumeURR(testURRID))

	s := storedSession(t, pConn, localSEID)

	if len(s.urrs) != 1 || s.urrs[0].urrID != testURRID {
		t.Fatalf("expected URR %d in the session, got %v", testURRID, s.urrs)
	}

	if got := s.pdrs[0].urrIDList; len(got) != 1 || got[0] != testURRID {
		t.Fatalf("expected the PDR to be measured by URR %d, got %v", testURRID, got)
	}
}

// A UPF that does not report usage ignores URRs, as it did before it could report any.
func TestEstablishmentIgnoresURRsWhenUsageReportingIsOff(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(), volumeURR(testURRID))

	if s := storedSession(t, pConn, localSEID); len(s.urrs) != 0 {
		t.Fatalf("expected no URRs, got %v", s.urrs)
	}
}

func TestEstablishmentRefusesACreateURRWithoutAMeasurementMethod(t *testing.T) {
	pConn := reportingConn(t)

	rsp, err := pConn.handleSessionEstablishmentRequest(message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(testRemoteSEID, nil, nil),
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(),
		ie.NewCreateURR(ie.NewURRID(testURRID), ie.NewReportingTriggers(triggerOctet5PERIO, 0)),
	))
	// The establishment wraps its refusals in a HandlePFCPMsgError, which does not unwrap.
	var handlerErr *HandlePFCPMsgError
	if !errors.As(err, &handlerErr) || !errors.Is(handlerErr.Err, errURRMeasurementMethodMissing) {
		t.Fatalf("the establishment returned %v, expected a refusal for the missing Measurement Method", err)
	}

	seres, ok := rsp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", rsp)
	}

	// A malformed URR is no rule that failed to be applied, and names none.
	assertRefusal(t, seres.Cause, seres.FailedRuleID, ie.CauseRequestRejected, 0)

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("the refused establishment left %d session(s) in the store", n)
	}
}

func TestModificationCreatesUpdatesAndRemovesURRs(t *testing.T) {
	pConn := reportingConn(t)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(), volumeURR(testURRID))

	rsp, err := pConn.handleSessionModificationRequest(message.NewSessionModificationRequest(0, 0, localSEID, 2, 0,
		volumeURR(testURRID+1),
		ie.NewUpdateURR(ie.NewURRID(testURRID+1), ie.NewVolumeThreshold(volumeTOVOL, 5000, 0, 0)),
		ie.NewRemoveURR(ie.NewURRID(testURRID)),
	))
	if err != nil {
		t.Fatalf("modification refused: %v", err)
	}

	assertAccepted(t, rsp.(*message.SessionModificationResponse).Cause, "modification")

	s := storedSession(t, pConn, localSEID)

	if len(s.urrs) != 1 || s.urrs[0].urrID != testURRID+1 {
		t.Fatalf("expected only URR %d in the session, got %v", testURRID+1, s.urrs)
	}

	if s.urrs[0].volumeThreshold.total != 5000 {
		t.Fatalf("the Update URR was not applied: %v", s.urrs[0])
	}
}

// The modification works on a copy of the session's rules and publishes it only when the
// message succeeds. URRs are among those rules: an update applied before a later refusal
// must not reach the store.
func TestARefusedModificationKeepsTheURRsItHad(t *testing.T) {
	pConn := reportingConn(t)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(), volumeURR(testURRID))

	refuseModification(t, pConn, localSEID, errNotFound,
		ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeThreshold(volumeTOVOL, 5000, 0, 0)),
		ie.NewRemoveURR(ie.NewURRID(testURRID+1)),
	)

	s := storedSession(t, pConn, localSEID)

	if len(s.urrs) != 1 || s.urrs[0].volumeThreshold.total != 1000 {
		t.Fatalf("the refused modification changed the stored URRs: %v", s.urrs)
	}
}

// A session kept because the rollback of its refused establishment did not finish is
// kept for its rules alone: it holds no URRs, so no usage is reported for it.
func TestARejectedEstablishmentKeptForItsRollbackHoldsNoURRs(t *testing.T) {
	pConn := reportingConn(t)
	pConn.upf.datapath = &addRefusingDP{delFinishes: false}

	_, err := pConn.handleSessionEstablishmentRequest(message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(testRemoteSEID, nil, nil),
		corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID)), downlinkFAR(), volumeURR(testURRID)))
	if err == nil {
		t.Fatal("an establishment whose batch the datapath refused was accepted")
	}

	sessions := pConn.store.GetAllSessions()
	if len(sessions) != 1 {
		t.Fatalf("%d session(s) stored after a rollback that did not finish, expected 1", len(sessions))
	}

	if len(sessions[0].urrs) != 0 {
		t.Fatalf("the kept session holds URRs %v; the poller would report usage for a "+
			"refused establishment", sessions[0].urrs)
	}
}

// assertRefusal checks a refused session message's Cause, and that it names failedURR as
// its Failed Rule ID, or no rule if failedURR is 0.
func assertRefusal(t *testing.T, cause, failed *ie.IE, wantCause uint8, failedURR uint32) {
	t.Helper()

	if got := causeOf(t, cause); got != wantCause {
		t.Fatalf("the message was answered with cause %d, expected %d", got, wantCause)
	}

	if failedURR == 0 {
		if failed != nil {
			t.Fatalf("the refusal names a Failed Rule ID: %v", failed)
		}

		return
	}

	if failed == nil {
		t.Fatalf("the refusal names no Failed Rule ID, expected URR %d", failedURR)
	}

	// As the control plane reads it.
	b, err := failed.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	if failed, err = ie.Parse(b); err != nil {
		t.Fatal(err)
	}

	typ, err := failed.RuleIDType()
	if err != nil {
		t.Fatal(err)
	}

	id, err := failed.FailedRuleID()
	if err != nil {
		t.Fatal(err)
	}

	if typ != ie.RuleIDTypeURR || id != failedURR {
		t.Fatalf("the Failed Rule ID is rule %d of type %d, expected URR %d", id, typ, failedURR)
	}
}

// A URR that asks for what this UPF does not do cannot be stored and applied as
// provisioned: the establishment is refused with "Rule creation/modification Failure" and
// the URR as the Failed Rule ID (clause 6.3.2.3).
func TestEstablishmentRefusesAURRThisUPFDoesNotActOn(t *testing.T) {
	pConn := reportingConn(t)

	rsp, err := pConn.handleSessionEstablishmentRequest(message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", testRemoteNodeID),
		ie.NewFSEID(testRemoteSEID, nil, nil),
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(),
		ie.NewCreateURR(ie.NewURRID(testURRID), ie.NewMeasurementMethod(0, 1, 0),
			ie.NewReportingTriggers(0, triggerOctet6VOLQU), ie.NewVolumeQuota(volumeTOVOL, 1000, 0, 0)),
	))
	if err == nil {
		t.Fatal("the establishment was accepted")
	}

	seres := rsp.(*message.SessionEstablishmentResponse)
	assertRefusal(t, seres.Cause, seres.FailedRuleID, ie.CauseRuleCreationModificationFailure, testURRID)

	if n := len(pConn.store.GetAllSessions()); n != 0 {
		t.Fatalf("the refused establishment left %d session(s) in the store", n)
	}
}

// A modification that asks for what this UPF does not do is refused and changes nothing
// (clause 6.3.3.3). A URR it cannot act on is a rule that failed, named in the response; a
// query for usage reports in the response, which it does not send, is "Service not
// supported".
func TestModificationRefusesWhatThisUPFDoesNotActOn(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ies       []*ie.IE
		cause     uint8
		failedURR uint32
	}{
		{
			"Update URR with a Time Quota",
			[]*ie.IE{ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewTimeQuota(time.Minute))},
			ie.CauseRuleCreationModificationFailure, testURRID,
		},
		{
			"Create URR with a Time Quota",
			[]*ie.IE{ie.NewCreateURR(ie.NewURRID(testURRID+1), ie.NewMeasurementMethod(0, 1, 0),
				ie.NewReportingTriggers(triggerOctet5VOLTH, 0), ie.NewTimeQuota(time.Minute))},
			ie.CauseRuleCreationModificationFailure, testURRID + 1,
		},
		{"Query URR", []*ie.IE{ie.NewQueryURR(ie.NewURRID(testURRID))}, ie.CauseServiceNotSupported, 0},
		{"QAURR", []*ie.IE{ie.NewPFCPSMReqFlags(0x04)}, ie.CauseServiceNotSupported, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pConn := reportingConn(t)
			localSEID := establishStaticSession(t, pConn,
				corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(), volumeURR(testURRID))

			// An update the message would apply before the refusal, which must not reach the store.
			ies := append([]*ie.IE{ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeThreshold(volumeTOVOL, 5000, 0, 0))},
				tc.ies...)

			rsp, err := pConn.handleSessionModificationRequest(
				message.NewSessionModificationRequest(0, 0, localSEID, 2, 0, ies...))
			if err == nil {
				t.Fatal("the modification was accepted")
			}

			smres := rsp.(*message.SessionModificationResponse)
			assertRefusal(t, smres.Cause, smres.FailedRuleID, tc.cause, tc.failedURR)

			if s := storedSession(t, pConn, localSEID); len(s.urrs) != 1 || s.urrs[0].volumeThreshold.total != 1000 {
				t.Fatalf("the refused modification changed the stored URRs: %v", s.urrs)
			}
		})
	}
}

// A PFCPSMReq-Flags IE that cannot be read is not taken as asking for no query: the
// modification is refused before anything in it is applied.
func TestModificationRefusesPFCPSMReqFlagsItCannotRead(t *testing.T) {
	pConn := reportingConn(t)
	localSEID := establishStaticSession(t, pConn,
		corePDR(downlinkPDRID, firstUEAddress), downlinkFAR(), volumeURR(testURRID))

	// As the control plane would send it, so the empty IE survives parsing.
	b, err := message.NewSessionModificationRequest(0, 0, localSEID, 2, 0,
		ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeThreshold(volumeTOVOL, 5000, 0, 0)),
		ie.New(ie.PFCPSMReqFlags, nil)).Marshal()
	if err != nil {
		t.Fatal(err)
	}

	smreq, err := message.ParseSessionModificationRequest(b)
	if err != nil {
		t.Fatal(err)
	}

	rsp, err := pConn.handleSessionModificationRequest(smreq)
	if err == nil {
		t.Fatal("the modification was accepted")
	}

	smres := rsp.(*message.SessionModificationResponse)
	assertRefusal(t, smres.Cause, smres.FailedRuleID, ie.CauseRequestRejected, 0)

	if s := storedSession(t, pConn, localSEID); len(s.urrs) != 1 || s.urrs[0].volumeThreshold.total != 1000 {
		t.Fatalf("the refused modification changed the stored URRs: %v", s.urrs)
	}
}

// A UPF that does not report usage ignores a query as it ignores URRs.
func TestModificationIgnoresAQueryWhenUsageReportingIsOff(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn, corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	rsp, err := pConn.handleSessionModificationRequest(message.NewSessionModificationRequest(0, 0, localSEID, 2, 0,
		ie.NewQueryURR(ie.NewURRID(testURRID)), ie.NewPFCPSMReqFlags(0x04)))
	if err != nil {
		t.Fatalf("modification refused: %v", err)
	}

	assertAccepted(t, rsp.(*message.SessionModificationResponse).Cause, "modification")
}

// A UPF that does not report usage does not read PFCPSMReq-Flags, so an empty one is
// accepted as it was before usage reporting.
func TestModificationIgnoresEmptyPFCPSMReqFlagsWhenUsageReportingIsOff(t *testing.T) {
	pConn := establishingConn(t, nil)
	localSEID := establishStaticSession(t, pConn, corePDR(downlinkPDRID, firstUEAddress), downlinkFAR())

	rsp, err := pConn.handleSessionModificationRequest(message.NewSessionModificationRequest(0, 0, localSEID, 2, 0,
		ie.New(ie.PFCPSMReqFlags, nil)))
	if err != nil {
		t.Fatalf("modification refused: %v", err)
	}

	assertAccepted(t, rsp.(*message.SessionModificationResponse).Cause, "modification")
}
