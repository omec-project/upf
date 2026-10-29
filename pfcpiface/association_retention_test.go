// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"net"
	"testing"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

// TS 29.244 clause 6.2.6.2.2: PSREI says that an association already existed for the node ID and
// that the sessions the request asked to retain were. This UPF never deletes them on a
// re-association, so it is set exactly when there was an earlier association to keep them from and
// the request asked: setting it otherwise tells the CP function it may act on sessions this UPF
// never had.
func TestPSREIIsSetOnlyForARetentionRequestThatReplacesAnAssociation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		existing  bool // an association for the same node ID already exists on the connection
		otherNode bool // the existing association is another node ID's
		retention bool // the request carries PFCP Session Retention Information
		want      bool
	}{
		{"re-association asking for retention", true, false, true, true},
		{"re-association not asking", true, false, false, false},
		{"first association asking for retention", false, false, true, false},
		{"another node ID's association, asking for retention", true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Connected, because the handler names the peer by its remote address.
			smf, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
			if err != nil {
				t.Fatalf("failed to create udp socket: %v", err)
			}
			t.Cleanup(func() { smf.Close() })
			c, err := net.DialUDP("udp", nil, smf.LocalAddr().(*net.UDPAddr))
			if err != nil {
				t.Fatalf("failed to connect udp socket: %v", err)
			}
			t.Cleanup(func() { c.Close() })
			p := &PFCPConn{Conn: c, upf: &upf{datapath: &fakeDP{}}}
			p.nodeID.localIE = ie.NewNodeID("", "", "upf")
			if tc.existing {
				p.ts.remote = time.Now().Add(-time.Hour)
				p.nodeID.remote = "smf"
				if tc.otherNode {
					p.nodeID.remote = "another-smf"
				}
			}

			ies := []*ie.IE{ie.NewNodeID("", "", "smf"), ie.NewRecoveryTimeStamp(time.Now())}
			if tc.retention {
				ies = append(ies, ie.NewPFCPSessionRetentionInformation(nil))
			}
			reply, err := p.handleAssociationSetupRequest(message.NewAssociationSetupRequest(1, ies...))
			if err != nil {
				t.Fatalf("handling the association setup request: %v", err)
			}
			asres, ok := reply.(*message.AssociationSetupResponse)
			if !ok {
				t.Fatalf("reply is %T, want an Association Setup Response", reply)
			}
			cause, err := asres.Cause.Cause()
			if err != nil || cause != ie.CauseRequestAccepted {
				t.Fatalf("the association was not accepted: cause %d, %v", cause, err)
			}
			if got := asres.PFCPASRspFlags != nil && asres.PFCPASRspFlags.HasPSREI(); got != tc.want {
				t.Errorf("PSREI set %v, want %v", got, tc.want)
			}
		})
	}
}
