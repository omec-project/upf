// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"google.golang.org/grpc"
)

// A URR whose Volume Quota is exhausted stops its traffic, by closing the gates of the
// QERs that apply to it, until a Volume Quota IE grants it a new quota (TS 29.244 clause
// 5.2.2.3.1).

// quotaURR measures volume against a 10 MB Volume Quota and reports its exhaustion.
func quotaURR() urr {
	return urr{
		urrID:         testURRID,
		measureVolume: true,
		triggers:      urrTriggers{volumeQuota: true},
		volumeQuota:   volumeLimit{total: 10 * mb, hasTotal: true},
	}
}

// quotaSession is a session whose PDR, measured by u, applies an application QER.
func quotaSession(u urr) *PFCPSession {
	s := &PFCPSession{}
	s.CreatePDR(pdr{
		pdrID: downlinkPDRID, urrIDList: []uint32{u.urrID}, hasURRIDs: true,
		qerIDList: []uint32{applicationQERID},
	})
	s.CreateQER(qer{qerID: applicationQERID, qosLevel: ApplicationQos})
	s.CreateURR(u, testEpoch)

	return s
}

// newQuota is an Update URR that grants quota, and when it is sent.
func newQuota(quota uint64) (urr, urrIEs, time.Time) {
	return urr{urrID: testURRID, volumeQuota: volumeLimit{total: quota, hasTotal: true}},
		urrIEs{volumeQuota: true}, testEpoch
}

func TestAnExhaustedQuotaIsReportedAndClosesTheGate(t *testing.T) {
	s := quotaSession(quotaURR())

	reports := poll(s, downlink(10*mb), testEpoch.Add(time.Second))
	if len(reports) != 1 || reports[0].trigger != triggerVOLQU || reports[0].volume.total != 10*mb {
		t.Fatalf("expected a VOLQU report of 10 MB, got %+v", reports)
	}

	changed := s.applyQuotaGates()
	if len(changed) != 1 || !changed[0].quotaClosed || !s.qers[0].quotaClosed {
		t.Fatalf("the exhausted quota did not close the QER's gate: changed %+v", changed)
	}
}

// A CHF that grants fixed chunks sends the same Volume Quota again, and that is a
// new grant.
func TestASameSizeGrantAfterExhaustionReopensTheGate(t *testing.T) {
	s := quotaSession(quotaURR())

	poll(s, downlink(10*mb), testEpoch.Add(time.Second))
	s.applyQuotaGates()

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	changed := s.applyQuotaGates()
	if len(changed) != 1 || changed[0].quotaClosed || s.qers[0].quotaClosed {
		t.Fatalf("a new quota of the same size did not reopen the gate: changed %+v", changed)
	}

	if r := poll(s, downlink(9*mb), testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("the new quota fired before 10 MB: %+v", r)
	}

	if r := poll(s, downlink(1*mb), testEpoch.Add(3*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected the new quota to be exhausted at 10 MB, got %+v", r)
	}
}

// Clause 5.2.2.3.1 NOTE 1: the traffic since the last report is deducted from a new quota.
func TestUsageBeforeTheGrantCountsAgainstIt(t *testing.T) {
	s := quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeQuota: true}})

	s.addUsage(downlinkPDRID, downlink(1*mb), testEpoch.Add(time.Second))

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, downlink(9*mb-1), testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("the quota fired before the further 9 MB: %+v", r)
	}

	if r := poll(s, downlink(1), testEpoch.Add(3*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected the quota to be exhausted after a further 9 MB, got %+v", r)
	}
}

// A quota granted while a report is outstanding, and the report then refused, is
// counted from the last report the control plane counted (NOTE 1): the refused volume
// counts against it.
func TestAQuotaGrantedBeforeARefusalCountsTheRefusedVolume(t *testing.T) {
	u := quotaURR()
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := quotaSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	s.addUsage(downlinkPDRID, downlink(6*mb), testEpoch.Add(time.Minute))
	sent := s.dueReports(testEpoch.Add(time.Minute))

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	s.settleReports(sent, reportRejected)

	if r := poll(s, downlink(4*mb), testEpoch.Add(61*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected the quota to be exhausted at 10 MB since the last counted report, got %+v", r)
	}
}

// A quota granted before a report the control plane counted is not new to a later
// refusal: what it had counted stays counted.
func TestAQuotaGrantedBeforeACountedReportKeepsItsCountOnALaterRefusal(t *testing.T) {
	u := quotaURR()
	u.triggers.periodic = true
	u.measurementPeriod = time.Minute

	s := quotaSession(u)

	poll(s, downlink(0), testEpoch) // arms the period

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, downlink(6*mb), testEpoch.Add(time.Minute)); len(r) != 1 || r[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report, got %+v", r)
	}

	s.addUsage(downlinkPDRID, downlink(2*mb), testEpoch.Add(2*time.Minute))
	s.settleReports(s.dueReports(testEpoch.Add(2*time.Minute)), reportRejected)

	if r := poll(s, downlink(2*mb), testEpoch.Add(121*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected the quota to be exhausted at 10 MB since the grant, got %+v", r)
	}
}

// A zero quota stops the traffic at once. It is reported only if there is usage to
// report, so a control plane that stops a rating group with one is not answered with a
// report it would answer with another zero quota.
func TestAZeroQuotaClosesTheGateAtOnce(t *testing.T) {
	s := quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeQuota: true}})

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	if changed := s.applyQuotaGates(); len(changed) != 1 || !changed[0].quotaClosed {
		t.Fatalf("a zero quota did not close the gate: %+v", changed)
	}

	if r := s.dueReports(testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("a zero quota with no usage to report was reported: %+v", r)
	}

	s = quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeQuota: true}})
	s.addUsage(downlinkPDRID, downlink(5), testEpoch.Add(time.Second))

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	if r := s.dueReports(testEpoch.Add(2 * time.Second)); len(r) != 1 || r[0].volume.total != 5 {
		t.Fatalf("a zero quota did not report the usage since the last report: %+v", r)
	}
}

// An Update URR does not read the datapath's counts, so usage counted before a zero quota
// arrived may be credited only at the next poll. It is reported then, once.
func TestAZeroQuotaReportsTheUsageCreditedAfterIt(t *testing.T) {
	s := quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeQuota: true}})

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("a zero quota with no usage to report was reported: %+v", r)
	}

	if r := poll(s, downlink(5), testEpoch.Add(2*time.Second)); len(r) != 1 ||
		r[0].trigger != triggerVOLQU || r[0].volume.total != 5 {
		t.Fatalf("the usage credited after a zero quota was not reported: %+v", r)
	}

	if r := poll(s, downlink(3), testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("a zero quota was reported twice: %+v", r)
	}

	s = quotaSession(volumeThresholdURR(100 * mb))

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, downlink(5), testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("a zero quota without VOLQU was reported: %+v", r)
	}
}

// Usage a refused report gives back to a URR whose quota of zero arrived while the report
// was outstanding is reported with VOLQU, as usage held when the quota arrived is.
func TestAZeroQuotaReportsTheUsageARefusedReportGivesBack(t *testing.T) {
	for _, outcome := range []reportOutcome{reportRejected, reportDelivered} {
		u := volumeThresholdURR(5)
		u.triggers.volumeQuota = true
		s := quotaSession(u)

		s.addUsage(downlinkPDRID, downlink(7), testEpoch.Add(time.Second))

		sent := s.dueReports(testEpoch.Add(time.Second))
		if len(sent) != 1 || sent[0].trigger != triggerVOLTH {
			t.Fatalf("expected a threshold report, got %+v", sent)
		}

		if err := s.UpdateURR(newQuota(0)); err != nil {
			t.Fatal(err)
		}

		s.settleReports(sent, outcome)

		r := poll(s, volumeCounts{}, testEpoch.Add(2*time.Second))

		switch outcome {
		case reportRejected:
			if len(r) != 1 || r[0].trigger&triggerVOLQU == 0 || r[0].volume.total != 7 {
				t.Fatalf("the usage a refused report gave back was not reported with VOLQU: %+v", r)
			}
		default:
			if len(r) != 0 {
				t.Fatalf("a zero quota with no usage to report was reported: %+v", r)
			}
		}
	}
}

// A zero quota stops the traffic whether or not the URR measures volume.
func TestAZeroQuotaClosesTheGateWithoutVOLUM(t *testing.T) {
	s := quotaSession(urr{urrID: testURRID, measureDuration: true, triggers: urrTriggers{volumeQuota: true}})

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	if changed := s.applyQuotaGates(); len(changed) != 1 || !changed[0].quotaClosed {
		t.Fatalf("a zero quota without VOLUM did not close the gate: %+v", changed)
	}
}

// A non-zero quota counts only the volume the URR measures: without VOLUM it is never
// reached, and an Update URR that turns VOLUM off holds the count where it is.
func TestANonZeroQuotaCountsOnlyMeasuredVolume(t *testing.T) {
	u := quotaURR()
	u.measureVolume, u.measureDuration = false, true
	s := quotaSession(u)

	if r := poll(s, downlink(20*mb), testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("a quota without VOLUM was reported: %+v", r)
	}

	if changed := s.applyQuotaGates(); len(changed) != 0 {
		t.Fatalf("a quota without VOLUM closed the gate: %+v", changed)
	}

	s = quotaSession(quotaURR())
	poll(s, downlink(6*mb), testEpoch.Add(time.Second))

	volumeOff := urr{urrID: testURRID, measureDuration: true}
	if err := s.UpdateURR(volumeOff, urrIEs{measurementMethod: true}, testEpoch.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, downlink(20*mb), testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("volume measured with VOLUM off counted against the quota: %+v", r)
	}

	volumeOn := urr{urrID: testURRID, measureVolume: true}
	if err := s.UpdateURR(volumeOn, urrIEs{measurementMethod: true}, testEpoch.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, downlink(3*mb), testEpoch.Add(5*time.Second)); len(r) != 0 {
		t.Fatalf("the count was not held while VOLUM was off, 9 MB of 10 reported: %+v", r)
	}

	if r := poll(s, downlink(1*mb), testEpoch.Add(6*time.Second)); len(r) != 1 ||
		r[0].trigger&triggerVOLQU == 0 {
		t.Fatalf("the held count did not reach the quota: %+v", r)
	}
}

// With only the threshold trigger set, an exhausted quota stops the traffic without a
// report (clause 5.2.2.2.1).
func TestAQuotaWithoutItsTriggerStopsTheTrafficSilently(t *testing.T) {
	u := quotaURR()
	u.triggers = urrTriggers{volumeThreshold: true}
	u.volumeThreshold = volumeLimit{total: 100 * mb, hasTotal: true}

	s := quotaSession(u)

	if r := poll(s, downlink(10*mb), testEpoch.Add(time.Second)); len(r) != 0 {
		t.Fatalf("the quota was reported without its trigger: %+v", r)
	}

	if changed := s.applyQuotaGates(); len(changed) != 1 || !changed[0].quotaClosed {
		t.Fatal("the exhausted quota did not close the gate")
	}
}

// Only the exhausted rating group stops: the exhausted URR's
// application QER closes; the other rating group's, and the session QER both share, stay
// open.
func TestQuotaExhaustionStopsOnlyItsRatingGroup(t *testing.T) {
	const otherPDRID, otherQERID, otherURRID = downlinkPDRID + 1, applicationQERID + 10, testURRID + 1

	s := &PFCPSession{}
	s.CreatePDR(pdr{
		pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true,
		qerIDList: []uint32{applicationQERID, sessionQERID},
	})
	s.CreatePDR(pdr{
		pdrID: otherPDRID, urrIDList: []uint32{otherURRID}, hasURRIDs: true,
		qerIDList: []uint32{otherQERID, sessionQERID},
	})
	s.CreateQER(qer{qerID: applicationQERID, qosLevel: ApplicationQos})
	s.CreateQER(qer{qerID: otherQERID, qosLevel: ApplicationQos})
	s.CreateQER(qer{qerID: sessionQERID, qosLevel: SessionQos})
	s.CreateURR(quotaURR(), testEpoch)

	other := quotaURR()
	other.urrID = otherURRID
	s.CreateURR(other, testEpoch)

	poll(s, downlink(10*mb), testEpoch.Add(time.Second))

	changed := s.applyQuotaGates()
	if len(changed) != 1 || changed[0].qerID != applicationQERID {
		t.Fatalf("expected only the exhausted rating group's QER to close, got %+v", changed)
	}
}

// BESS looks a PDR's traffic up at the first QER in its list alone, so that is the QER a
// quota closes. Another application QER further down the list is the first of some other
// PDR, whose rating group would stop with it. A PDR whose first QER is the session QER is
// stopped at the session QER, since that QER can keep the application entry it had before
// it was marked. A PDR whose first QER the session does not hold is already sunk at the
// application lookup's default gate, so nothing closes for it.
func TestAQuotaClosesOnlyThePDRsFirstQER(t *testing.T) {
	const otherPDRID, otherQERID, otherURRID = downlinkPDRID + 1, applicationQERID + 10, testURRID + 1

	for _, tc := range []struct {
		name   string
		qerIDs []uint32
		want   []uint32
	}{
		{"application QERs before the session QER", []uint32{applicationQERID, otherQERID, sessionQERID}, []uint32{applicationQERID}},
		{"only the session QER", []uint32{sessionQERID}, []uint32{sessionQERID}},
		{"the session QER first", []uint32{sessionQERID, applicationQERID}, []uint32{sessionQERID}},
		{"a QER the session does not hold first", []uint32{applicationQERID + 99, applicationQERID, sessionQERID}, nil},
		// BESS looks a PDR without a QER up as QER 0.
		{"no QER, with a QER 0 in the session", nil, []uint32{0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &PFCPSession{}
			s.CreatePDR(pdr{
				pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true,
				qerIDList: tc.qerIDs,
			})
			s.CreatePDR(pdr{
				pdrID: otherPDRID, urrIDList: []uint32{otherURRID}, hasURRIDs: true,
				qerIDList: []uint32{otherQERID, sessionQERID},
			})
			s.CreateQER(qer{qerID: applicationQERID, qosLevel: ApplicationQos})
			s.CreateQER(qer{qerID: otherQERID, qosLevel: ApplicationQos})
			s.CreateQER(qer{qerID: sessionQERID, qosLevel: SessionQos})
			s.CreateQER(qer{qerID: 0, qosLevel: ApplicationQos})
			s.CreateURR(quotaURR(), testEpoch)

			other := quotaURR()
			other.urrID = otherURRID
			s.CreateURR(other, testEpoch)

			poll(s, downlink(10*mb), testEpoch.Add(time.Second))

			var closed []uint32

			for _, q := range s.applyQuotaGates() {
				if q.quotaClosed {
					closed = append(closed, q.qerID)
				}
			}

			if !slices.Equal(closed, tc.want) {
				t.Fatalf("the exhausted PDR with QERs %v closed %v, expected %v",
					tc.qerIDs, closed, tc.want)
			}
		})
	}
}

// qerWritesDP records the QERs every add and modify batch writes, and can refuse them,
// leave them unfinished, or fail their RPCs.
type qerWritesDP struct {
	fakeDP

	mu         sync.Mutex
	writes     []qer
	deletes    []qer
	pdrWrites  []pdr
	refuse     bool
	refuseOnce bool
	unfinished bool
	rpcFails   bool
}

func (d *qerWritesDP) SendMsgToUPF(method upfMsgType, all, updated PacketForwardingRules) uint8 {
	cause, _ := d.SendMsgToUPFWithCompletion(method, all, updated)
	return cause
}

// SendMsgToUPFWithCompletion answers an unfinished batch as bess does: accepted, and not
// finished.
func (d *qerWritesDP) SendMsgToUPFWithCompletion(
	method upfMsgType, all, updated PacketForwardingRules,
) (uint8, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.refuse || d.refuseOnce {
		d.refuseOnce = false
		return ie.CauseRequestRejected, true
	}

	if d.unfinished {
		return ie.CauseRequestAccepted, false
	}

	// bess answers a batch whose RPCs failed as accepted and finished.
	if d.rpcFails {
		return ie.CauseRequestAccepted, true
	}

	switch method {
	case upfMsgTypeAdd:
		d.writes = append(d.writes, all.qers...)
		d.pdrWrites = append(d.pdrWrites, all.pdrs...)
	case upfMsgTypeMod:
		d.writes = append(d.writes, updated.qers...)
		d.pdrWrites = append(d.pdrWrites, updated.pdrs...)
	case upfMsgTypeDel:
		d.deletes = append(d.deletes, all.qers...)
	}

	return ie.CauseRequestAccepted, true
}

// WriteQERGates takes the gates as SendMsgToUPFWithCompletion takes a batch of them,
// except that, as in bess, a failed RPC is not taken.
func (d *qerWritesDP) WriteQERGates(qers []qer) bool {
	d.mu.Lock()
	rpcFails := d.rpcFails
	d.mu.Unlock()

	if rpcFails {
		return false
	}

	cause, finished := d.SendMsgToUPFWithCompletion(
		upfMsgTypeMod, PacketForwardingRules{}, PacketForwardingRules{qers: qers})

	return cause != ie.CauseRequestRejected && finished
}

// lastWrite is the application QER as it was last written, and whether it was.
func (d *qerWritesDP) lastWrite() (qer, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := len(d.writes) - 1; i >= 0; i-- {
		if d.writes[i].qerID == applicationQERID {
			return d.writes[i], true
		}
	}

	return qer{}, false
}

func createQuotaURR(quota uint64) *ie.IE {
	return ie.NewCreateURR(
		ie.NewURRID(testURRID),
		ie.NewMeasurementMethod(0, 1, 0),
		ie.NewReportingTriggers(0, triggerOctet6VOLQU),
		ie.NewVolumeQuota(volumeTOVOL, quota, 0, 0),
	)
}

// newQuotaSetup is a reportingSetup whose session's PDR applies a QER and is measured by a
// URR with the given quota, on a datapath that records QER writes.
func newQuotaSetup(t *testing.T, quota uint64) (*reportingSetup, *qerWritesDP) {
	t.Helper()

	rs := newIdleReportingSetup(t)
	dp := &qerWritesDP{}
	rs.pConn.upf.datapath = dp

	rs.localSEID = establishStaticSession(t, rs.pConn,
		corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID), ie.NewQERID(applicationQERID)),
		downlinkFAR(),
		ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
		createQuotaURR(quota))

	return rs, dp
}

// testQuota is the quota a setup that exhausts it is given.
const testQuota = 10 * mb

// exhaust measures testQuota and polls.
func (rs *reportingSetup) exhaust(t *testing.T, dp *qerWritesDP) {
	t.Helper()

	rs.reader.set(downlinkCounts(rs.localSEID, testQuota))
	rs.monitor.poll(time.Now())

	if q, ok := dp.lastWrite(); !ok || !q.quotaClosed {
		t.Fatalf("the poll did not write the QER closed: %+v", q)
	}
}

func TestTheGateFollowsTheQuotaThroughTheSessionHandlers(t *testing.T) {
	rs, dp := newQuotaSetup(t, testQuota)

	rs.exhaust(t, dp)

	rs.modify(t, ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeQuota(volumeTOVOL, 10*mb, 0, 0)))

	if q, _ := dp.lastWrite(); q.quotaClosed {
		t.Fatal("the modification that granted a new quota did not write the gate open")
	}

	if storedSession(t, rs.pConn, rs.localSEID).qers[0].quotaClosed {
		t.Fatal("the stored QER is still quota-closed after the new grant")
	}
}

// An Update QER replaces the rule, open gates and all; a quota-closed gate stays
// closed until the URR gets a new quota.
func TestAnUpdateQERDoesNotReopenAQuotaClosedGate(t *testing.T) {
	rs, dp := newQuotaSetup(t, testQuota)

	rs.exhaust(t, dp)

	rs.modify(t, ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(higherMBR, higherMBR)))

	q, _ := dp.lastWrite()
	if !q.quotaClosed || q.ulMbr != higherMBR {
		t.Fatalf("the updated QER was written %+v, expected the new MBR with the gate still closed", q)
	}
}

// A removed URR releases the gates it closed.
func TestRemovingTheURRReopensItsGate(t *testing.T) {
	rs, dp := newQuotaSetup(t, testQuota)

	rs.exhaust(t, dp)
	rs.monitor.lastRead = time.Time{}

	rs.modify(t, ie.NewRemoveURR(ie.NewURRID(testURRID)))

	if q, _ := dp.lastWrite(); q.quotaClosed {
		t.Fatal("removing the URR did not write its QER open")
	}
}

// Clause 5.2.2.3.1 NOTE 3: a URR provisioned with a zero quota stops its traffic before
// any is allowed.
func TestAZeroQuotaAtEstablishmentWritesTheGateClosed(t *testing.T) {
	_, dp := newQuotaSetup(t, 0)

	if q, ok := dp.lastWrite(); !ok || !q.quotaClosed {
		t.Fatalf("the establishment wrote the QER %+v, expected it closed", q)
	}
}

// A gate the datapath refused, whose write did not finish, or whose RPC failed, is
// written again at the next poll. Recorded as written, it would never be, and the
// traffic would run on past its quota.
func TestARefusedGateIsWrittenAgainAtTheNextPoll(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail func(*qerWritesDP, bool)
	}{
		{"refused", func(dp *qerWritesDP, on bool) { dp.refuse = on }},
		{"not finished", func(dp *qerWritesDP, on bool) { dp.unfinished = on }},
		{"RPC failed", func(dp *qerWritesDP, on bool) { dp.rpcFails = on }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs, dp := newQuotaSetup(t, 10*mb)

			tc.fail(dp, true)
			rs.reader.set(downlinkCounts(rs.localSEID, 10*mb))
			rs.monitor.poll(time.Now())

			if storedSession(t, rs.pConn, rs.localSEID).qers[0].quotaClosed {
				t.Fatal("the session records a gate the datapath did not take")
			}

			tc.fail(dp, false)
			rs.monitor.poll(time.Now())

			if q, _ := dp.lastWrite(); !q.quotaClosed {
				t.Fatal("the next poll did not write the gate closed")
			}
		})
	}
}

// qosArgStub records the QoS rules written to bessd.
type qosArgStub struct {
	pb.BESSControlClient

	mu      sync.Mutex
	gates   []uint64
	modules []string
}

func (s *qosArgStub) ModuleCommand(
	_ context.Context, req *pb.CommandRequest, _ ...grpc.CallOption,
) (*pb.CommandResponse, error) {
	var arg pb.QosCommandAddArg
	if err := req.Arg.UnmarshalTo(&arg); err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.gates = append(s.gates, arg.Gate)
	s.modules = append(s.modules, req.Name)
	s.mu.Unlock()

	return &pb.CommandResponse{}, nil
}

// The datapath drops the traffic of a quota-closed QER in both directions, whatever the
// control plane's gates say, through the quota gate, at an application and a session QER
// alike; a gate the control plane closes keeps the status-drop gate.
func TestBessDropsTheTrafficOfAQuotaClosedQER(t *testing.T) {
	for _, tc := range []struct {
		name        string
		level       QosLevel
		status      uint8
		quotaClosed bool
		want        uint64
	}{
		{"quota closed", ApplicationQos, ie.GateStatusOpen, true, qerGateQuotaDrop},
		{"quota and gate status closed", ApplicationQos, ie.GateStatusClosed, true, qerGateQuotaDrop},
		{"gate status closed", ApplicationQos, ie.GateStatusClosed, false, qerGateStatusDrop},
		{"session QER quota closed", SessionQos, ie.GateStatusOpen, true, qerGateQuotaDrop},
		{"session QER gate status closed", SessionQos, ie.GateStatusClosed, false, qerGateStatusDrop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &qosArgStub{}
			b := &bess{client: stub, qciQosMap: map[uint8]*QosConfigVal{0: {}}}
			q := qer{
				qerID: applicationQERID, qosLevel: tc.level,
				ulStatus: tc.status, dlStatus: tc.status, quotaClosed: tc.quotaClosed,
			}

			if err := b.handleUplinkQER(context.Background(), q); err != nil {
				t.Fatal(err)
			}

			if err := b.handleDownlinkQER(context.Background(), q); err != nil {
				t.Fatal(err)
			}

			if len(stub.gates) != 2 || stub.gates[0] != tc.want || stub.gates[1] != tc.want {
				t.Fatalf("the QER was written with gates %v, expected %d in both directions", stub.gates, tc.want)
			}

			module := AppQerLookup
			if tc.level == SessionQos {
				module = SessQerLookup
			}

			if stub.modules[0] != module || stub.modules[1] != module {
				t.Fatalf("the QER was written to %v, expected %s", stub.modules, module)
			}
		})
	}
}

// Both pipelines sink the quota gate, at the application and the session QER, under the
// number pfcpiface writes. The closed-loop pipeline passes what the status-drop gate drops
// on, so a quota written there would not stop the traffic.
func TestEveryPipelineSinksTheQuotaGate(t *testing.T) {
	for _, file := range []string{"../conf/up4.bess", "../conf/closed_loop.bess"} {
		conf, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}

		for _, line := range []string{
			fmt.Sprintf("qerQuotaDropGate = %d\n", qerGateQuotaDrop),
			"appQERLookup:qerQuotaDropGate -> appQERQuotaDrop::Sink()\n",
			"sessionQERLookup:qerQuotaDropGate -> sessionQERQuotaDrop::Sink()\n",
			// A PDR whose first QER has no application entry is left to these two.
			"appQERLookup:qerFailGate -> appQERLookupFail::Sink()\n",
			"appQERLookup.set_default_gate(gate=qerFailGate)\n",
		} {
			if !strings.Contains(string(conf), "\n"+line) {
				t.Errorf("%s lacks the line %q", file, strings.TrimSpace(line))
			}
		}
	}
}

// Clause 5.2.2.3.1 NOTE 1 counts a new threshold, like a new quota, from the last report.
// The SMF grants 10 MB with an 8 MB threshold, and sends both again after the quota
// report: the threshold fires 8 MB into each grant, not earlier each time.
func TestANewThresholdIsCountedFromTheLastReport(t *testing.T) {
	u := quotaURR()
	u.triggers.volumeThreshold = true
	u.volumeThreshold = volumeLimit{total: 8 * mb, hasTotal: true}

	s := quotaSession(u)

	if r := poll(s, downlink(8*mb), testEpoch.Add(time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
		t.Fatalf("expected a threshold report at 8 MB, got %+v", r)
	}

	if r := poll(s, downlink(2*mb), testEpoch.Add(2*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected a quota report at 10 MB, got %+v", r)
	}

	regrant := urr{
		urrID:           testURRID,
		volumeThreshold: volumeLimit{total: 8 * mb, hasTotal: true},
		volumeQuota:     volumeLimit{total: 10 * mb, hasTotal: true},
	}
	if err := s.UpdateURR(regrant, urrIEs{volumeThreshold: true, volumeQuota: true}, testEpoch); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, downlink(8*mb-1), testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("the new threshold fired before 8 MB: %+v", r)
	}

	if r := poll(s, downlink(1), testEpoch.Add(4*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLTH {
		t.Fatalf("expected the new threshold to fire at 8 MB, got %+v", r)
	}
}

// A refused quota report leaves the quota exhausted and the traffic stopped, so nothing
// else would trigger a report: it is due again.
func TestARejectedQuotaReportIsDueAgain(t *testing.T) {
	s := quotaSession(quotaURR())

	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch.Add(time.Second))
	sent := s.dueReports(testEpoch.Add(time.Second))
	s.settleReports(sent, reportRejected)

	again := s.dueReports(testEpoch.Add(2 * time.Second))
	if len(again) != 1 || again[0].trigger&triggerVOLQU == 0 || again[0].volume.total != 10*mb {
		t.Fatalf("expected the quota report again with its 10 MB, got %+v", again)
	}
}

// A Create URR replacing a URR replaces its provisioning: with no quota, there is none to
// be exhausted, and the gates it closed open.
func TestACreateURRWithoutAQuotaReleasesTheReplacedURRsGate(t *testing.T) {
	s := quotaSession(quotaURR())

	poll(s, downlink(10*mb), testEpoch.Add(time.Second))
	s.applyQuotaGates()

	s.CreateURR(urr{urrID: testURRID, measureVolume: true}, testEpoch.Add(2*time.Second))

	if changed := s.applyQuotaGates(); len(changed) != 1 || changed[0].quotaClosed {
		t.Fatalf("the replacement without a quota did not reopen the gate: %+v", changed)
	}
}

// A replacing Create URR with a quota grants it, whatever its value, counted from the
// URR's last report: the gate reopens, and usage since that report counts against it.
func TestAReplacingCreateURRGrantsItsQuotaFromTheLastReport(t *testing.T) {
	s := quotaSession(quotaURR())

	if r := poll(s, downlink(10*mb), testEpoch.Add(time.Second)); len(r) != 1 {
		t.Fatalf("the quota was not reported: %+v", r)
	}

	s.applyQuotaGates()
	s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch.Add(2*time.Second))
	s.CreateURR(quotaURR(), testEpoch.Add(3*time.Second))

	if changed := s.applyQuotaGates(); len(changed) != 1 || changed[0].quotaClosed {
		t.Fatalf("the replacement with a quota did not reopen the gate: %+v", changed)
	}

	if r := poll(s, downlink(5*mb), testEpoch.Add(4*time.Second)); len(r) != 0 {
		t.Fatalf("9 MB of a 10 MB quota was reported: %+v", r)
	}

	if r := poll(s, downlink(1*mb), testEpoch.Add(5*time.Second)); len(r) != 1 ||
		r[0].trigger&triggerVOLQU == 0 || r[0].volume.total != 10*mb {
		t.Fatalf("the usage before the replacement did not count against its quota: %+v", r)
	}
}

// A QER is stopped for every PDR that applies it: rating groups that share an
// application QER stop together.
func TestASharedApplicationQERStopsEveryRatingGroupOnIt(t *testing.T) {
	const otherPDRID, otherURRID = downlinkPDRID + 1, testURRID + 1

	s := quotaSession(quotaURR())
	s.CreatePDR(pdr{
		pdrID: otherPDRID, urrIDList: []uint32{otherURRID}, hasURRIDs: true,
		qerIDList: []uint32{applicationQERID},
	})

	other := quotaURR()
	other.urrID = otherURRID
	s.CreateURR(other, testEpoch)

	poll(s, downlink(10*mb), testEpoch.Add(time.Second))

	if changed := s.applyQuotaGates(); len(changed) != 1 || !changed[0].quotaClosed ||
		changed[0].qerID != applicationQERID {
		t.Fatalf("the shared QER did not close: %+v", changed)
	}

	if s.findURR(otherURRID).usage.quotaExhausted {
		t.Fatal("the other rating group's quota was exhausted")
	}
}

// A PDR that applies no QER the session holds has no gate: its exhausted quota is
// reported, and nothing closes.
func TestAPDRWithoutAQERIsNotStopped(t *testing.T) {
	for name, qerIDs := range map[string][]uint32{
		"no QER":       nil,
		"dangling QER": {applicationQERID + 99},
	} {
		t.Run(name, func(t *testing.T) {
			s := &PFCPSession{}
			s.CreatePDR(pdr{
				pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true,
				qerIDList: qerIDs,
			})
			s.CreateQER(qer{qerID: applicationQERID, qosLevel: ApplicationQos})
			s.CreateURR(quotaURR(), testEpoch)

			if r := poll(s, downlink(10*mb), testEpoch.Add(time.Second)); len(r) != 1 ||
				r[0].trigger&triggerVOLQU == 0 {
				t.Fatalf("the quota was not reported: %+v", r)
			}

			if changed := s.applyQuotaGates(); len(changed) != 0 {
				t.Fatalf("a gate closed for a PDR that applies none: %+v", changed)
			}
		})
	}
}

// A URR that loses its last PDR forgets what it used of its quota: a PDR that measures it
// again finds the quota as it was provisioned (clause 5.2.2.3.1 NOTE 5).
func TestADissociatedURRForgetsItsUsedQuota(t *testing.T) {
	s := quotaSession(quotaURR())
	before := slices.Clone(s.pdrs)

	if r := poll(s, downlink(10*mb), testEpoch.Add(time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected the quota to be exhausted, got %+v", r)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(2*time.Second))

	s.CreatePDR(pdr{
		pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true,
		qerIDList: []uint32{applicationQERID},
	})

	if r := poll(s, downlink(6*mb), testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("expected 6 MB within the 10 MB quota as provisioned, got %+v", r)
	}

	s.applyQuotaGates()

	if s.qers[0].quotaClosed {
		t.Fatal("the gate is closed by a quota the URR should have forgotten")
	}
}

// A quota of zero is exhausted as provisioned, so a URR that loses its last PDR keeps it
// exhausted.
func TestADissociatedURRWithAZeroQuotaStaysExhausted(t *testing.T) {
	u := quotaURR()
	u.volumeQuota = volumeLimit{hasTotal: true}

	s := quotaSession(u)
	before := slices.Clone(s.pdrs)

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(time.Second))

	if !s.findURR(testURRID).usage.quotaExhausted {
		t.Fatal("a zero quota was taken as not exhausted after the reset")
	}
}

// A quota granted while a report was outstanding is forgotten with the rest when the URR
// loses its last PDR: if that report is then refused, the usage it gives back is not
// counted against the quota.
func TestAReportRefusedAfterADissociationDoesNotCountAgainstTheForgottenQuota(t *testing.T) {
	u := quotaURR()
	u.triggers.periodic, u.measurementPeriod = true, time.Second

	s := quotaSession(u)
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch)
	s.dueReports(testEpoch)                          // the period starts
	sent := s.dueReports(testEpoch.Add(time.Second)) // and ends: 4 MB outstanding

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(2*time.Second))
	s.settleReports(sent, reportRejected)

	if q := s.findURR(testURRID).usage.toQuota; !q.isZero() {
		t.Fatalf("the refused report's %+v was counted against the forgotten quota", q)
	}
}

// A quota granted after the reset is counted from the URR's last counted report (NOTE
// 1). A report outstanding at the reset that is then refused was not counted, so the
// volume it gives back counts against the new quota, as it would without the reset.
func TestAQuotaGrantedAfterADissociationCountsAReportRefusedSince(t *testing.T) {
	u := quotaURR()
	u.triggers.periodic, u.measurementPeriod = true, time.Second

	s := quotaSession(u)
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch)
	s.dueReports(testEpoch)                          // the period starts
	sent := s.dueReports(testEpoch.Add(time.Second)) // and ends: 4 MB outstanding

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	if r := s.reportDissociated(before, testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("expected no termination report with nothing measured since, got %+v", r)
	}

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	s.settleReports(sent, reportRejected)

	if q := s.findURR(testURRID).usage.toQuota; q.total != 4*mb {
		t.Fatalf("expected the refused 4 MB counted against the new quota, got %+v", q)
	}
}

// A report the modification response carried again ahead of the termination report is
// taken as counted when its Session Report Request is refused, so a quota granted after
// the reset does not count it.
func TestAQuotaGrantedAfterADissociationDoesNotCountAReplayedReportRefusedSince(t *testing.T) {
	u := quotaURR()
	u.triggers.periodic, u.measurementPeriod = true, time.Second

	s := quotaSession(u)
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(4*mb), testEpoch)
	s.dueReports(testEpoch)                          // the period starts
	sent := s.dueReports(testEpoch.Add(time.Second)) // and ends: 4 MB outstanding
	s.addUsage(downlinkPDRID, downlink(mb), testEpoch.Add(2*time.Second))

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	if r := s.reportDissociated(before, testEpoch.Add(2*time.Second)); len(r) != 2 {
		t.Fatalf("expected the outstanding report, then a termination report, got %+v", r)
	}

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	s.settleReports(sent, reportRejected)

	us := s.findURR(testURRID).usage
	if !us.toQuota.isZero() || !us.unreported.isZero() {
		t.Fatalf("the refused replayed report was given back: toQuota %+v, unreported %+v",
			us.toQuota, us.unreported)
	}
}

// A VOLQU report the control plane refused is due again, but not while no PDR measures
// the URR: the usage it gave back was not just measured. It goes once a PDR measures the
// URR again.
func TestARefusedQuotaReportWaitsForAPDR(t *testing.T) {
	s := quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeQuota: true}})
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(3*mb), testEpoch)

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	sent := s.dueReports(testEpoch.Add(time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerVOLQU {
		t.Fatalf("expected a VOLQU report of the 3 MB, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(2*time.Second))
	s.settleReports(sent, reportRejected)

	for i := 3; i < 6; i++ {
		if r := poll(s, volumeCounts{}, testEpoch.Add(time.Duration(i)*time.Second)); len(r) != 0 {
			t.Fatalf("poll %d: a URR no PDR measures sent %+v", i, r)
		}
	}

	s.CreatePDR(pdr{
		pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true,
		qerIDList: []uint32{applicationQERID},
	})

	if r := poll(s, volumeCounts{}, testEpoch.Add(6*time.Second)); len(r) != 1 ||
		r[0].trigger != triggerVOLQU || r[0].volume.total != 3*mb {
		t.Fatalf("expected the refused 3 MB in a VOLQU report once a PDR measures the URR, got %+v", r)
	}
}

// A quota granted after the reset counts a refused report's usage (NOTE 1), and if that
// exhausts it, the VOLQU report waits for a PDR to measure the URR.
func TestAQuotaExhaustedByAGivenBackReportIsReportedOnceAPDRMeasuresTheURR(t *testing.T) {
	s := quotaSession(quotaURR())
	before := slices.Clone(s.pdrs)

	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch.Add(time.Second))

	sent := s.dueReports(testEpoch.Add(time.Second))
	if len(sent) != 1 || sent[0].trigger != triggerVOLQU {
		t.Fatalf("expected a VOLQU report, got %+v", sent)
	}

	if _, err := s.RemovePDR(downlinkPDRID); err != nil {
		t.Fatal(err)
	}

	s.reportDissociated(before, testEpoch.Add(2*time.Second))

	if err := s.UpdateURR(newQuota(10 * mb)); err != nil {
		t.Fatal(err)
	}

	s.settleReports(sent, reportRejected)

	if r := poll(s, volumeCounts{}, testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("a URR no PDR measures sent %+v", r)
	}

	s.CreatePDR(pdr{
		pdrID: downlinkPDRID, urrIDList: []uint32{testURRID}, hasURRIDs: true,
		qerIDList: []uint32{applicationQERID},
	})

	if r := poll(s, volumeCounts{}, testEpoch.Add(4*time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected the VOLQU report once a PDR measures the URR, got %+v", r)
	}
}

// A delivered VOLQU report is not sent again: the quota stays exhausted, but its
// report has been made.
func TestADeliveredQuotaReportIsNotRepeated(t *testing.T) {
	s := quotaSession(quotaURR())

	if r := poll(s, downlink(10*mb), testEpoch.Add(time.Second)); len(r) != 1 || r[0].trigger != triggerVOLQU {
		t.Fatalf("expected a VOLQU report, got %+v", r)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("the delivered VOLQU report was repeated: %+v", r)
	}
}

// The quota stops the traffic while another report is in flight, whatever becomes of
// that report.
func TestAQuotaReachedWhileAReportIsInFlightClosesTheGate(t *testing.T) {
	u := quotaURR()
	u.triggers.periodic, u.measurementPeriod = true, time.Second

	s := quotaSession(u)

	s.dueReports(testEpoch) // the period starts
	s.addUsage(downlinkPDRID, downlink(mb), testEpoch.Add(time.Second))

	if sent := s.dueReports(testEpoch.Add(time.Second)); len(sent) != 1 || sent[0].trigger != triggerPERIO {
		t.Fatalf("expected a periodic report in flight, got %+v", sent)
	}

	s.addUsage(downlinkPDRID, downlink(9*mb), testEpoch.Add(2*time.Second))
	s.dueReports(testEpoch.Add(2 * time.Second))

	if changed := s.applyQuotaGates(); len(changed) != 1 || !changed[0].quotaClosed {
		t.Fatalf("the quota reached during the report did not close the gate: %+v", changed)
	}
}

// An Update URR that turns VOLQU off drops a VOLQU report still due: the control plane
// no longer asks for one.
func TestAQuotaReportDueIsDroppedWhenVOLQUIsTurnedOff(t *testing.T) {
	s := quotaSession(quotaURR())

	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch.Add(time.Second))
	s.settleReports(s.dueReports(testEpoch.Add(time.Second)), reportRejected)

	withoutVOLQU := urr{
		urrID:           testURRID,
		triggers:        urrTriggers{volumeThreshold: true},
		volumeThreshold: volumeLimit{total: 100 * mb, hasTotal: true},
	}

	if err := s.UpdateURR(withoutVOLQU, urrIEs{reportingTriggers: true, volumeThreshold: true},
		testEpoch.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}

	if r := poll(s, volumeCounts{}, testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("a VOLQU report went out after VOLQU was turned off: %+v", r)
	}
}

// An Update URR that turns VOLQU off drops the VOLQU report then due, and one that turns
// it on again does not bring back the report of a quota that ran out before: the report
// does not depend on whether a poll ran in between. This holds for a zero quota still
// waiting for usage to report too.
func TestAQuotaReportDroppedWithVOLQUStaysDroppedWhenVOLQUReturns(t *testing.T) {
	setVOLQU := func(s *PFCPSession, on bool, at time.Time) {
		t.Helper()

		u := urr{urrID: testURRID, triggers: urrTriggers{volumeQuota: on, volumeThreshold: true}}
		if err := s.UpdateURR(u, urrIEs{reportingTriggers: true}, at); err != nil {
			t.Fatal(err)
		}
	}

	s := quotaSession(quotaURR())

	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch.Add(time.Second))
	s.settleReports(s.dueReports(testEpoch.Add(time.Second)), reportRejected)

	setVOLQU(s, false, testEpoch.Add(2*time.Second))
	setVOLQU(s, true, testEpoch.Add(2*time.Second))

	if r := poll(s, volumeCounts{}, testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("a VOLQU report dropped with VOLQU came back with it: %+v", r)
	}

	s = quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeQuota: true}})

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	setVOLQU(s, false, testEpoch.Add(time.Second))
	setVOLQU(s, true, testEpoch.Add(time.Second))

	if r := poll(s, downlink(5), testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("a zero quota exhausted before VOLQU was turned off and on was reported: %+v", r)
	}

	// A VOLQU report refused after VOLQU was turned off is not due again.
	s = quotaSession(quotaURR())

	s.addUsage(downlinkPDRID, downlink(10*mb), testEpoch.Add(time.Second))
	sent := s.dueReports(testEpoch.Add(time.Second))

	setVOLQU(s, false, testEpoch.Add(2*time.Second))
	s.settleReports(sent, reportRejected)
	setVOLQU(s, true, testEpoch.Add(2*time.Second))

	if r := poll(s, volumeCounts{}, testEpoch.Add(3*time.Second)); len(r) != 0 {
		t.Fatalf("a VOLQU report refused after VOLQU was turned off was due again: %+v", r)
	}

	// A zero quota that ran out while VOLQU was off is not reported once VOLQU is set.
	s = quotaSession(urr{urrID: testURRID, measureVolume: true, triggers: urrTriggers{volumeThreshold: true}})

	if err := s.UpdateURR(newQuota(0)); err != nil {
		t.Fatal(err)
	}

	setVOLQU(s, true, testEpoch.Add(time.Second))

	if r := poll(s, downlink(5), testEpoch.Add(2*time.Second)); len(r) != 0 {
		t.Fatalf("a zero quota that ran out without VOLQU was reported: %+v", r)
	}
}

// A modification the datapath refuses restores the quota gates with the rest of the
// session's rules, and the stored session keeps its exhausted quota.
func TestARefusedModificationRestoresTheQuotaGate(t *testing.T) {
	rs, dp := newQuotaSetup(t, testQuota)

	rs.exhaust(t, dp)

	dp.mu.Lock()
	dp.refuseOnce = true
	written := len(dp.writes)
	dp.mu.Unlock()

	grant := ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeQuota(volumeTOVOL, 10*mb, 0, 0))

	_, err := rs.pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, rs.localSEID, 2, 0, grant))
	if err == nil {
		t.Fatal("expected the modification refused")
	}

	// The refused batch is not recorded, so what follows it is the rollback's restore.
	dp.mu.Lock()
	restored := append([]qer(nil), dp.writes[written:]...)
	dp.mu.Unlock()

	if len(restored) == 0 {
		t.Fatal("the rollback restored no QER")
	}

	for _, q := range restored {
		if !q.quotaClosed {
			t.Fatalf("the rollback wrote the gate open: %+v", q)
		}
	}

	sess := storedSession(t, rs.pConn, rs.localSEID)
	if !sess.qers[0].quotaClosed || !sess.urrs[0].usage.quotaExhausted {
		t.Fatal("the refused grant reopened the stored gate")
	}
}

// duplicatedQERSetup is a quota setup whose session holds applicationQERID twice: its own
// QER, which the session's marking makes its session QER, and then one an accepted Create
// of the same ID added with a lower MBR, an application QER. Nothing rejects that Create,
// and the two are different datapath entries. The PDR is looked up at the application
// one, the second of its ID.
func duplicatedQERSetup(t *testing.T) (*reportingSetup, *qerWritesDP) {
	t.Helper()

	rs, dp := newQuotaSetup(t, testQuota)

	rs.modify(t, ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR/2, lowerMBR/2)))

	var levels []QosLevel

	for _, q := range storedSession(t, rs.pConn, rs.localSEID).qers {
		if q.qerID == applicationQERID {
			levels = append(levels, q.qosLevel)
		}
	}

	if !slices.Equal(levels, []QosLevel{SessionQos, ApplicationQos}) {
		t.Fatalf("the session holds QER %d at levels %v, expected a session and then an "+
			"application QER", applicationQERID, levels)
	}

	return rs, dp
}

// A modification writes a quota gate to the QER whose gate changed, not to the first QER
// of its ID, as an Update QER is written: here that is the session QER, so the application
// QER would keep its old gate in the datapath while the session records the new one, and
// no poll would write it again. The entry is written once, since one batch programs its
// rules concurrently -- also when the message updates a QER of the ID as well.
func TestAModificationWritesTheQuotaGateToTheQERItChanged(t *testing.T) {
	quota := func(v uint64) *ie.IE {
		return ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeQuota(volumeTOVOL, v, 0, 0))
	}
	// Each update replaces the first QER of the ID, the session QER. Below the other's
	// MBR, the marking then makes the other the session QER; above it, the updated one
	// stays the session QER and is written beside the application QER's gate.
	updateQER := ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR/10, lowerMBR/10))
	updateSessionQER := ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(higherMBR, higherMBR))

	for _, tc := range []struct {
		name    string
		exhaust bool
		ies     []*ie.IE
		closed  bool
	}{
		{"a grant reopens it", true, []*ie.IE{quota(10 * mb)}, false},
		{"a grant with an Update QER of the session QER reopens it", true, []*ie.IE{quota(10 * mb), updateSessionQER}, false},
		{"a zero quota closes it", false, []*ie.IE{quota(0)}, true},
		{"a zero quota with an Update QER closes it", false, []*ie.IE{quota(0), updateQER}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs, dp := duplicatedQERSetup(t)

			if tc.exhaust {
				rs.exhaust(t, dp)
			}

			dp.mu.Lock()
			written := len(dp.writes)
			dp.mu.Unlock()

			rs.modify(t, tc.ies...)

			dp.mu.Lock()
			application := 0

			for _, q := range dp.writes[written:] {
				if q.qerID != applicationQERID {
					continue
				}

				if q.qosLevel == SessionQos && q.quotaClosed {
					t.Errorf("the session QER was written quota-closed: %+v", q)
				}

				if q.qosLevel == ApplicationQos && q.quotaClosed == tc.closed {
					application++
				}
			}
			dp.mu.Unlock()

			if application != 1 {
				t.Fatalf("the application QER %d was written with quotaClosed=%v %d times, "+
					"expected once", applicationQERID, tc.closed, application)
			}

			for _, q := range storedSession(t, rs.pConn, rs.localSEID).qers {
				if want := tc.closed && q.qosLevel == ApplicationQos; q.quotaClosed != want {
					t.Fatalf("the stored QER %+v does not have quotaClosed=%v", q, want)
				}
			}
		})
	}
}

// The rollback of a refused grant removes an updated QER only if no QER of its ID held
// before the message occupies its entry. Compared with the first QER of the ID alone, the
// session QER, the application QER the grant wrote would read as moved, and be removed
// until the restore puts it back a batch later.
func TestARollbackKeepsEveryQERAQuotaGrantWrote(t *testing.T) {
	rs, dp := duplicatedQERSetup(t)

	rs.exhaust(t, dp)

	dp.mu.Lock()
	dp.refuseOnce = true
	dp.mu.Unlock()

	grant := ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeQuota(volumeTOVOL, 10*mb, 0, 0))

	_, err := rs.pConn.handleSessionModificationRequest(
		message.NewSessionModificationRequest(0, 0, rs.localSEID, 2, 0, grant))
	if err == nil {
		t.Fatal("expected the modification refused")
	}

	dp.mu.Lock()
	defer dp.mu.Unlock()

	for _, q := range dp.deletes {
		t.Errorf("the rollback removed %+v, which the restore writes again", q)
	}
}

// lastWriteAt is QER id as it was last written at the given level, and whether it was.
func (d *qerWritesDP) lastWriteAt(id uint32, level QosLevel) (qer, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := len(d.writes) - 1; i >= 0; i-- {
		if d.writes[i].qerID == id && d.writes[i].qosLevel == level {
			return d.writes[i], true
		}
	}

	return qer{}, false
}

// addAnUnusedQER sends a modification that creates a QER no PDR applies. The marking then
// makes the session's other QER, the one its PDR applies, the session QER, though the
// message names neither that QER nor the PDR.
func (rs *reportingSetup) addAnUnusedQER(t *testing.T) {
	t.Helper()

	rs.modify(t, ie.NewCreateQER(ie.NewQERID(applicationQERID+20), ie.NewMBR(lowerMBR, lowerMBR)))

	if q := storedSession(t, rs.pConn, rs.localSEID).qers[0]; q.qosLevel != SessionQos {
		t.Fatalf("the marking left QER %d at level %v, expected it the session QER", q.qerID, q.qosLevel)
	}
}

// A QER the marking makes the session QER keeps the application entry BESS was given for
// it, and its PDR is still looked up by that entry. So the modification writes the QER at
// its new level, and a quota exhausted afterwards closes it there, where the PDR's
// traffic, passed on by the old entry, meets it.
func TestAQuotaStopsAPDRWhoseQERTheMarkingMadeTheSessionQER(t *testing.T) {
	rs, dp := newQuotaSetup(t, testQuota)

	rs.addAnUnusedQER(t)

	if _, ok := dp.lastWriteAt(applicationQERID, SessionQos); !ok {
		t.Fatal("the modification did not write the QER the marking made the session QER")
	}

	rs.exhaust(t, dp)

	if q, ok := dp.lastWriteAt(applicationQERID, SessionQos); !ok || !q.quotaClosed {
		t.Fatalf("the exhausted quota left the session QER %+v open", q)
	}
}

// A quota-closed QER that the marking makes the session QER leaves its application entry
// behind, closed. The modification writes that entry open, as the gate moves to the
// session level with the QER, so a later grant that reopens the session QER leaves no
// gate closed.
func TestAQuotaClosedQERTheMarkingMovesReopensItsApplicationEntry(t *testing.T) {
	rs, dp := newQuotaSetup(t, testQuota)

	rs.exhaust(t, dp)
	rs.addAnUnusedQER(t)

	if q, ok := dp.lastWriteAt(applicationQERID, ApplicationQos); !ok || q.quotaClosed {
		t.Fatalf("the application entry the QER left was last written %+v, expected open", q)
	}

	if q, ok := dp.lastWriteAt(applicationQERID, SessionQos); !ok || !q.quotaClosed {
		t.Fatalf("the QER was written to the session level as %+v, expected quota-closed", q)
	}

	rs.modify(t, ie.NewUpdateURR(ie.NewURRID(testURRID), ie.NewVolumeQuota(volumeTOVOL, 10*mb, 0, 0)))

	if q, _ := dp.lastWriteAt(applicationQERID, SessionQos); q.quotaClosed {
		t.Fatal("the grant left the session QER closed")
	}
}

// A modification that makes another QER the session QER moves the old one back to the
// application level, since BESS holds one session QER per session, and changes the first
// QER of a PDR that applies the new one. The modification writes the PDR, once, so BESS
// looks its traffic up by the QER the session now has first, and that is the QER a quota
// then closes.
func TestAModificationRewritesWhatTheMarkingMoved(t *testing.T) {
	const otherQERID = applicationQERID + 10

	qerIDs := []*ie.IE{ie.NewQERID(applicationQERID), ie.NewQERID(otherQERID), ie.NewQERID(sessionQERID)}
	// Raising the first QER above the session QER's MBR makes it the session QER.
	raise := ie.NewUpdateQER(ie.NewQERID(applicationQERID), ie.NewMBR(highestMBR, highestMBR))
	updatePDR := ie.NewUpdatePDR(append([]*ie.IE{
		ie.NewPDRID(downlinkPDRID),
		ie.NewPrecedence(establishedPrecedence),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewUEIPAddress(0x2, firstUEAddress, "", 0, 0),
		),
		ie.NewFARID(downlinkFARID),
		ie.NewURRID(testURRID),
	}, qerIDs...)...)

	for name, ies := range map[string][]*ie.IE{
		"an Update QER":                {raise},
		"an Update QER and Update PDR": {raise, updatePDR},
	} {
		t.Run(name, func(t *testing.T) {
			rs := newIdleReportingSetup(t)
			dp := &qerWritesDP{}
			rs.pConn.upf.datapath = dp

			rs.localSEID = establishStaticSession(t, rs.pConn,
				corePDR(downlinkPDRID, firstUEAddress, append([]*ie.IE{ie.NewURRID(testURRID)}, qerIDs...)...),
				downlinkFAR(),
				ie.NewCreateQER(ie.NewQERID(applicationQERID), ie.NewMBR(lowerMBR, lowerMBR)),
				ie.NewCreateQER(ie.NewQERID(otherQERID), ie.NewMBR(lowerMBR, lowerMBR)),
				ie.NewCreateQER(ie.NewQERID(sessionQERID), ie.NewMBR(higherMBR, higherMBR)),
				createQuotaURR(testQuota))

			dp.mu.Lock()
			dp.pdrWrites = nil
			dp.mu.Unlock()

			rs.modify(t, ies...)

			levels := map[uint32]QosLevel{}
			for _, q := range storedSession(t, rs.pConn, rs.localSEID).qers {
				levels[q.qerID] = q.qosLevel
			}

			if levels[applicationQERID] != SessionQos || levels[sessionQERID] != ApplicationQos {
				t.Fatalf("the QERs are at levels %v, expected only QER %d the session QER",
					levels, applicationQERID)
			}

			if _, ok := dp.lastWriteAt(sessionQERID, ApplicationQos); !ok {
				t.Fatalf("the modification did not write QER %d at the application level", sessionQERID)
			}

			dp.mu.Lock()
			written := append([]pdr(nil), dp.pdrWrites...)
			dp.mu.Unlock()

			if len(written) != 1 || firstQERID(written[0]) != otherQERID {
				t.Fatalf("the modification wrote the PDRs %v, expected the PDR once, with QER %d first",
					written, otherQERID)
			}

			rs.reader.set(downlinkCounts(rs.localSEID, testQuota))
			rs.monitor.poll(time.Now())

			for _, id := range []uint32{applicationQERID, otherQERID, sessionQERID} {
				q, _ := dp.lastWriteAt(id, levels[id])
				if q.quotaClosed != (id == otherQERID) {
					t.Errorf("after the quota ran out QER %d was last written %+v; expected only "+
						"QER %d closed", id, q, otherQERID)
				}
			}
		})
	}
}

// A quota-closed session QER that the marking moves back to the application level leaves
// its session entry closed only for as long as no other QER takes it over. Here the new
// session QER does, written with the gate the quota gives it; the old QER's entry is not
// also written open, which would race it in the same batch. The old QER is the first of
// the two, so the guard does not rest on the order the writes are collected in.
func TestAQuotaClosedSessionQERTheMarkingReplacesIsNotWrittenOpenOverItsSuccessor(t *testing.T) {
	const otherQERID = applicationQERID + 10

	pdrIEs := []*ie.IE{
		ie.NewPDRID(downlinkPDRID),
		ie.NewPrecedence(establishedPrecedence),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewUEIPAddress(0x2, firstUEAddress, "", 0, 0),
		),
		ie.NewFARID(downlinkFARID),
		ie.NewQERID(otherQERID),
	}

	// The quota's PDR applies the other QER alone, which makes it, the QER of the higher
	// MBR, the session QER.
	update := ie.NewUpdatePDR(pdrIEs...)

	for _, tc := range []struct {
		name   string
		ies    []*ie.IE
		closed bool
	}{
		// The quota closes the new session QER, as it closed the old one.
		{"the quota still exhausted", []*ie.IE{update}, true},
		// Without its URR the PDR is stopped by nothing, and the new session QER is open.
		{"the quota's URR removed", []*ie.IE{update, ie.NewRemoveURR(ie.NewURRID(testURRID))}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := newIdleReportingSetup(t)
			dp := &qerWritesDP{}
			rs.pConn.upf.datapath = dp

			// The quota's PDR applies only the session QER, so the quota closes that QER.
			rs.localSEID = establishStaticSession(t, rs.pConn,
				corePDR(downlinkPDRID, firstUEAddress, ie.NewURRID(testURRID), ie.NewQERID(sessionQERID)),
				corePDR(secondPDRID, secondUEAddress, ie.NewQERID(otherQERID), ie.NewQERID(sessionQERID)),
				downlinkFAR(),
				ie.NewCreateQER(ie.NewQERID(sessionQERID), ie.NewMBR(higherMBR, higherMBR)),
				ie.NewCreateQER(ie.NewQERID(otherQERID), ie.NewMBR(highestMBR, highestMBR)),
				createQuotaURR(testQuota))

			rs.reader.set(downlinkCounts(rs.localSEID, testQuota))
			rs.monitor.poll(time.Now())

			if q, ok := dp.lastWriteAt(sessionQERID, SessionQos); !ok || !q.quotaClosed {
				t.Fatalf("the exhausted quota left the session QER %+v open", q)
			}

			dp.mu.Lock()
			written := len(dp.writes)
			dp.mu.Unlock()

			rs.modify(t, tc.ies...)

			for _, q := range storedSession(t, rs.pConn, rs.localSEID).qers {
				if (q.qosLevel == SessionQos) != (q.qerID == otherQERID) {
					t.Fatalf("QER %d is at level %v, expected only QER %d the session QER",
						q.qerID, q.qosLevel, otherQERID)
				}
			}

			dp.mu.Lock()
			defer dp.mu.Unlock()

			sessionWrites := 0

			for _, q := range dp.writes[written:] {
				if q.qosLevel != SessionQos {
					continue
				}

				sessionWrites++

				if q.qerID != otherQERID || q.quotaClosed != tc.closed {
					t.Errorf("the modification wrote %+v to the session entry, expected only "+
						"QER %d, quotaClosed=%v", q, otherQERID, tc.closed)
				}
			}

			if sessionWrites != 1 {
				t.Errorf("the modification wrote the session entry %d times, expected once", sessionWrites)
			}
		})
	}
}

// The session QER is one the PDRs share that has no GBR. When every QER they share has
// one, the marking selects none and leaves the levels as they are.
func TestTheMarkingSelectsNoSessionQERAmongGBRQERs(t *testing.T) {
	const gbrQERID = applicationQERID + 10

	s := &PFCPSession{}
	s.CreatePDR(pdr{pdrID: downlinkPDRID, qerIDList: []uint32{gbrQERID}})
	s.CreatePDR(pdr{pdrID: downlinkPDRID + 1, qerIDList: []uint32{applicationQERID, gbrQERID}})
	s.CreateQER(qer{qerID: applicationQERID, qosLevel: ApplicationQos, ulMbr: lowerMBR})
	s.CreateQER(qer{qerID: gbrQERID, qosLevel: ApplicationQos, ulMbr: higherMBR, ulGbr: lowerMBR})

	s.MarkSessionQer(s.qers)

	for _, q := range s.qers {
		if q.qosLevel != ApplicationQos {
			t.Fatalf("the marking made QER %d the session QER, though the PDRs share only "+
				"a GBR QER", q.qerID)
		}
	}
}
