// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"encoding/hex"
	"flag"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// Golden-file contract tests: PFCP requests in two shapes -- a charging control plane's
// URR, and a minimal one -- kept encoded in testdata, driven through establishment, polls
// and the URR's removal, asserting the usage reports the UPF sends for them. The
// fixtures are built by writeGoldenInteropFixtures, so a change in how go-pfcp encodes
// them shows as a diff. Regenerate them with: go test ./pfcpiface/ -run TestGoldenSession -update
var updateGoldenInterop = flag.Bool("update", false, "rewrite golden PFCP fixtures")

const goldenSMFNodeID = "golden-smf"

// goldenEstablishment builds the establishment request of a lane. Both carry one uplink
// PDR and FAR and one URR: "charging" a charging URR (volume and time, VOLTH/TIMTH/VOLQU,
// a volume quota), "minimal" a minimal one (volume, VOLTH).
func goldenEstablishment(lane string, remoteSEID uint64) *message.SessionEstablishmentRequest {
	var urrIE *ie.IE

	switch lane {
	case "charging":
		urrIE = ie.NewCreateURR(
			ie.NewURRID(1),
			ie.NewMeasurementMethod(0, 1, 1),               // VOLUM + DURAT
			ie.NewReportingTriggers(0x02|0x04, 0x01, 0x00), // VOLTH+TIMTH, VOLQU
			ie.NewVolumeThreshold(0x01, 1000, 0, 0),
			ie.NewTimeThreshold(300*time.Second),
			ie.NewVolumeQuota(0x01, 5000, 0, 0),
		)
	case "minimal":
		urrIE = ie.NewCreateURR(
			ie.NewURRID(1),
			ie.NewMeasurementMethod(0, 1, 0),          // VOLUM only
			ie.NewReportingTriggers(0x02, 0x00, 0x00), // VOLTH only
			ie.NewVolumeThreshold(0x01, 1000, 0, 0),
		)
	}

	return message.NewSessionEstablishmentRequest(0, 0, 0, 1, 0,
		ie.NewNodeID("", "", goldenSMFNodeID),
		ie.NewFSEID(remoteSEID, net.ParseIP("192.0.2.1"), nil),
		ie.NewCreatePDR(
			ie.NewPDRID(1),
			ie.NewPrecedence(200),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceAccess),
				ie.NewFTEID(0x01, 0x30000000, net.ParseIP("198.51.100.1"), nil, 0),
			),
			ie.NewOuterHeaderRemoval(0, 0),
			ie.NewFARID(1),
			ie.NewURRID(1),
		),
		ie.NewCreateFAR(
			ie.NewFARID(1),
			ie.NewApplyAction(0x02), // FORW
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceCore),
			),
		),
		urrIE,
	)
}

// goldenRemoval builds the fixture's modification request removing the URR.
func goldenRemoval(seq uint32) *message.SessionModificationRequest {
	return message.NewSessionModificationRequest(0, 0, 0, seq, 0,
		ie.NewRemoveURR(ie.NewURRID(1)),
	)
}

func goldenInteropPath(name string) string {
	return filepath.Join("testdata", name)
}

func writeGoldenInteropFixtures(t *testing.T) {
	t.Helper()

	fixtures := map[string]message.Message{
		"establishment_charging.hex": goldenEstablishment("charging", 0xABCD),
		"establishment_minimal.hex":  goldenEstablishment("minimal", 0xABCE),
		"remove_urr.hex":             goldenRemoval(2),
	}

	for name, msg := range fixtures {
		buf := make([]byte, msg.MarshalLen())
		if err := msg.MarshalTo(buf); err != nil {
			t.Fatalf("failed to marshal fixture %s: %v", name, err)
		}

		if err := os.WriteFile(goldenInteropPath(name), []byte(hex.EncodeToString(buf)+"\n"), 0o600); err != nil {
			t.Fatalf("failed to write fixture %s: %v", name, err)
		}
	}
}

func loadGoldenInterop(t *testing.T, name string) message.Message {
	t.Helper()

	raw, err := os.ReadFile(goldenInteropPath(name))
	if err != nil {
		t.Fatalf("failed to read golden fixture %s (run with -update to generate): %v", name, err)
	}

	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("failed to decode golden fixture %s: %v", name, err)
	}

	msg, err := message.Parse(decoded)
	if err != nil {
		t.Fatalf("failed to parse golden fixture %s: %v", name, err)
	}

	return msg
}

// uplinkCounts is a read in which the session's PDR 1 forwarded uplink bytes.
func uplinkCounts(localSEID, bytes uint64) flowMeasureCounts {
	key := flowMeasureKey{localSEID, 1}
	st := &pb.FlowMeasureReadResponse_Statistic{Fseid: localSEID, Pdr: 1, TotalBytes: bytes, TotalPackets: 1}

	return flowMeasureCounts{
		preQos: flowMeasureTable{key: st},
		postUL: flowMeasureTable{key: st},
		postDL: flowMeasureTable{},
	}
}

// runGoldenSessionLane drives one SMF shape end to end: establishment, traffic across
// polls, each reporting the bytes since the last, then the URR's removal, reporting the
// rest with TERMR. With a quota, a third poll exhausts it and is reported with VOLQU.
func runGoldenSessionLane(t *testing.T, fixture string, wantDuration, withQuota bool) {
	rs := newReportingSetup(t)
	rs.pConn.nodeID.remote = goldenSMFNodeID

	resp, err := rs.pConn.handleSessionEstablishmentRequest(loadGoldenInterop(t, fixture))
	if err != nil {
		t.Fatalf("establishment failed: %v", err)
	}

	seres, ok := resp.(*message.SessionEstablishmentResponse)
	if !ok {
		t.Fatalf("expected a Session Establishment Response, got %T", resp)
	}

	assertAccepted(t, seres.Cause, "establishment")

	fseid, err := seres.UPFSEID.FSEID()
	if err != nil {
		t.Fatalf("failed to read the UP F-SEID: %v", err)
	}

	rs.localSEID = fseid.SEID

	sess := storedSession(t, rs.pConn, rs.localSEID)
	if len(sess.urrs) != 1 || sess.urrs[0].urrID != 1 || !sess.urrs[0].measureVolume ||
		sess.urrs[0].volumeThreshold.total != 1000 || sess.urrs[0].measureDuration != wantDuration {
		t.Fatalf("the URR was not stored as provisioned: %+v", sess.urrs)
	}

	if len(sess.pdrs) != 1 || len(sess.pdrs[0].urrIDList) != 1 || sess.pdrs[0].urrIDList[0] != 1 {
		t.Fatalf("the PDR is not linked to the URR: %+v", sess.pdrs)
	}

	// 1200 bytes cross the 1000-byte threshold, then 1100 more cross it again; each
	// report carries the bytes since the one before it (TS 29.244 clause 5.2.2.3.1). With
	// the 5000-byte quota, 2700 more exhaust it.
	polls := []uint64{1200, 1100}
	if withQuota {
		polls = append(polls, 2700)
	}

	for i, bytes := range polls {
		rs.reader.set(uplinkCounts(rs.localSEID, bytes))
		rs.monitor.poll(time.Now())

		srreq, _ := rs.receive(t)
		if len(srreq.UsageReport) != 1 {
			t.Fatalf("poll %d: expected one usage report, got %d", i, len(srreq.UsageReport))
		}

		got := readUsageReport(t, srreq.UsageReport[0])
		if got.seq != uint32(i) || got.volume.TotalVolume != bytes || got.volume.UplinkVolume != bytes ||
			!got.volume.HasDLVOL() || got.volume.DownlinkVolume != 0 {
			t.Fatalf("poll %d: report UR-SEQN %d with %+v, expected UR-SEQN %d with %d uplink bytes "+
				"and every counter", i, got.seq, got.volume, i, bytes)
		}

		if got.start.IsZero() || got.end.IsZero() {
			t.Fatalf("poll %d: the report has no Start Time or End Time", i)
		}

		volqu := len(got.trigger) >= 2 && got.trigger[1]&uint8(triggerVOLQU>>8) != 0
		if wantVOLQU := i == 2; volqu != wantVOLQU {
			t.Fatalf("poll %d: report triggers %x, VOLQU expected %v", i, got.trigger, wantVOLQU)
		}

		rs.answer(t, srreq, ie.CauseRequestAccepted)
		rs.settled(t)
	}

	removal, ok := loadGoldenInterop(t, "remove_urr.hex").(*message.SessionModificationRequest)
	if !ok {
		t.Fatal("the removal fixture is not a Session Modification Request")
	}

	removal.Header.SEID = rs.localSEID

	resp, err = rs.pConn.handleSessionModificationRequest(removal)
	if err != nil {
		t.Fatalf("modification failed: %v", err)
	}

	smres, ok := resp.(*message.SessionModificationResponse)
	if !ok {
		t.Fatalf("expected a Session Modification Response, got %T", resp)
	}

	assertAccepted(t, smres.Cause, "modification")

	if len(smres.UsageReport) != 1 {
		t.Fatalf("expected one removal usage report, got %d", len(smres.UsageReport))
	}

	got := readUsageReport(t, smres.UsageReport[0])
	if !isTermination(got) || got.seq != uint32(len(polls)) || got.volume.TotalVolume != 0 {
		t.Fatalf("the removal report is %+v, expected TERMR, UR-SEQN %d, nothing left to report",
			got, len(polls))
	}
}

func TestGoldenSessionChargingURR(t *testing.T) {
	if *updateGoldenInterop {
		writeGoldenInteropFixtures(t)
	}

	runGoldenSessionLane(t, "establishment_charging.hex", true, true)
}

func TestGoldenSessionMinimalURR(t *testing.T) {
	if *updateGoldenInterop {
		writeGoldenInteropFixtures(t)
	}

	runGoldenSessionLane(t, "establishment_minimal.hex", false, false)
}
