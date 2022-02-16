// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package pfcpiface

import (
	"math"
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wmnsk/go-pfcp/ie"
)

type pdrTestCase struct {
	input       *ie.IE
	expected    *pdr
	description string
}

func Test_parsePDR(t *testing.T) {
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
			input: ie.NewCreatePDR(
				ie.NewPDRID(pdrID),
				ie.NewPrecedence(precedence),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
					ie.NewFTEID(0x01, teid, N3Address, nil, 0),
				),
				ie.NewOuterHeaderRemoval(0, 0),
				ie.NewFARID(farID),
				ie.NewQERID(qerID),
			),
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
			input: ie.NewUpdatePDR(
				ie.NewPDRID(pdrID),
				ie.NewPrecedence(0),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, UEAddress.String(), "", 0, 0),
				),
				ie.NewFARID(farID),
				ie.NewQERID(qerID),
			),
			expected: &pdr{
				pdrID:        uint32(pdrID),
				fseID:        FSEID,
				farID:        farID,
				srcIface:     core,
				srcIfaceMask: 0xff,
				ueAddress:    ip2int(UEAddress),
				qerIDList:    []uint32{qerID},
				appFilter: applicationFilter{
					dstIPMask: math.MaxUint32,
					dstIP:     ip2int(UEAddress),
				},
			},
			description: "Valid downlink Update PDR input",
		},
		{
			input: ie.NewCreatePDR(
				ie.NewPDRID(pdrID),
				ie.NewPrecedence(0),
				ie.NewPDI(
					ie.NewSourceInterface(ie.SrcInterfaceCore),
					ie.NewUEIPAddress(0x2, UEAddress.String(), "", 0, 0),
				),
				ie.NewFARID(farID),
				ie.NewQERID(qerID),
			),
			expected: &pdr{
				pdrID:        uint32(pdrID),
				fseID:        FSEID,
				farID:        farID,
				srcIface:     core,
				srcIfaceMask: 0xff,
				ueAddress:    ip2int(UEAddress),
				qerIDList:    []uint32{qerID},
				appFilter: applicationFilter{
					dstIPMask: math.MaxUint32,
					dstIP:     ip2int(UEAddress),
				},
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
				fseID:     FSEID,
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

func TestCreatePortRangeCartesianProduct(t *testing.T) {
	type args struct {
		src portRange
		dst portRange
	}

	tests := []struct {
		name    string
		args    args
		want    []portRangeTernaryCartesianProduct
		wantErr bool
	}{
		{name: "exact ranges",
			args: args{src: newExactMatchPortRange(5000), dst: newExactMatchPortRange(80)},
			want: []portRangeTernaryCartesianProduct{{
				srcPort: 5000,
				srcMask: math.MaxUint16,
				dstPort: 80,
				dstMask: math.MaxUint16,
			}},
			wantErr: false},
		{name: "wildcard dst range",
			args: args{src: newExactMatchPortRange(10), dst: newWildcardPortRange()},
			want: []portRangeTernaryCartesianProduct{{
				srcPort: 10,
				srcMask: math.MaxUint16,
				dstPort: 0,
				dstMask: 0,
			}},
			wantErr: false},
		{name: "true range src range",
			args: args{src: newRangeMatchPortRange(1, 3), dst: newExactMatchPortRange(80)},
			want: []portRangeTernaryCartesianProduct{
				{
					srcPort: 0x1,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				},
				{
					srcPort: 0x2,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				},
				{
					srcPort: 0x3,
					srcMask: 0xffff,
					dstPort: 80,
					dstMask: math.MaxUint16,
				}},
			wantErr: false},
		{name: "invalid double range",
			args:    args{src: newRangeMatchPortRange(10, 20), dst: newRangeMatchPortRange(80, 85)},
			want:    nil,
			wantErr: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := CreatePortRangeCartesianProduct(tt.args.src, tt.args.dst)
				if (err != nil) != tt.wantErr {
					t.Errorf("CreatePortRangeCartesianProduct() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("CreatePortRangeCartesianProduct() got = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_defaultPortRange(t *testing.T) {
	t.Run("default constructed is wildcard", func(t *testing.T) {
		assert.True(t, portRange{}.isWildcardMatch(), "default portRange is wildcard")
	})
}

func Test_newWildcardPortRange(t *testing.T) {
	tests := []struct {
		name string
		want portRange
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := newWildcardPortRange(); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("newWildcardPortRange() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portRange_String(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want string
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.String(); got != tt.want {
					t.Errorf("String() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portRange_isExactMatch(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want bool
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.isExactMatch(); got != tt.want {
					t.Errorf("isExactMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portRange_isRangeMatch(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want bool
	}{
		// TODO: Add test cases.
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.isRangeMatch(); got != tt.want {
					t.Errorf("isRangeMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_portRange_isWildcardMatch(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want bool
	}{
		// TODO: Add test cases.
		{name: "foo", pr: portRange{
			low:  0,
			high: 0,
		}, want: true},
	}

	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.isWildcardMatch(); got != tt.want {
					t.Errorf("isWildcardMatch() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

// Perform a ternary match of value against rules.
func matchesTernary(value uint16, rules []portRangeTernaryRule) bool {
	for _, r := range rules {
		if (value & r.mask) == r.port {
			return true
		}
	}

	return false
}

func Test_portRange_asComplexTernaryMatches(t *testing.T) {
	tests := []struct {
		name     string
		pr       portRange
		strategy RangeConversionStrategy
		wantErr  bool
		want     []portRangeTernaryRule
	}{
		{name: "Exact match port range",
			pr: portRange{
				low:  8888,
				high: 8888,
			},
			want: []portRangeTernaryRule{
				{port: 8888, mask: 0xffff},
			},
			wantErr: false},
		{name: "wildcard port range",
			pr: portRange{
				low:  0,
				high: math.MaxUint16,
			},
			want: []portRangeTernaryRule{
				{port: 0, mask: 0},
			},
			wantErr: false},
		{name: "Simplest port range",
			pr: portRange{
				low:  0b0, // 0
				high: 0b1, // 1
			},
			//want: []portRangeTernaryRule{
			//	{port: 0b0, mask: 0xfffe},
			//},
			wantErr: false},
		{name: "Simplest port range2",
			pr: portRange{
				low:  0b01, // 1
				high: 0b10, // 2
			},
			//want: []portRangeTernaryRule{
			//	{port: 0b01, mask: 0xffff},
			//	{port: 0b10, mask: 0xffff},
			//},
			wantErr: false},
		{name: "Trivial ternary port range",
			pr: portRange{
				low:  0x0100, // 256
				high: 0x01ff, // 511
			},
			strategy: Ternary,
			//want: []portRangeTernaryRule{
			//	{port: 0x0100, mask: 0xff00},
			//},
			wantErr: false},
		{name: "one to three range",
			pr: portRange{
				low:  0b01, // 1
				high: 0b11, // 3
			},
			//want: []portRangeTernaryRule{
			//	{port: 0b01, mask: 0xffff},
			//	{port: 0b10, mask: 0xfffe},
			//},
			wantErr: false},
		{name: "True port range",
			pr: portRange{
				low:  0b00010, //  2
				high: 0b11101, // 29
			},
			wantErr: false},
		{name: "Worst case port range",
			pr: portRange{
				low:  1,
				high: 65534,
			},
			strategy: Ternary,
			wantErr:  false},
		{name: "low port filter",
			pr: portRange{
				low:  0,
				high: 1023,
			},
			strategy: Ternary,
			wantErr:  false},
		{name: "some small app filter",
			pr: portRange{
				low:  8080,
				high: 8084,
			},
			wantErr: false},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.pr.asComplexTernaryMatches(tt.strategy)
				if (err != nil) != tt.wantErr {
					t.Errorf("asComplexTernaryMatches() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if tt.want != nil && !reflect.DeepEqual(got, tt.want) {
					t.Errorf("asComplexTernaryMatches() got = %v, want %v", got, tt.want)
				}
				// Do exhaustive test over entire value range.
				for port := 0; port <= math.MaxUint16; port++ {
					expectMatch := port >= int(tt.pr.low) && port <= int(tt.pr.high)
					if matchesTernary(uint16(port), got) != expectMatch {
						mod := " "
						if !expectMatch {
							mod = " not "
						}
						t.Errorf("Expected port %v to%vmatch against rules %v from range %+v", port, mod, got, tt.pr)
					}
				}
			},
		)
	}
}

func Test_portRange_asTrivialTernaryMatch(t *testing.T) {
	tests := []struct {
		name     string
		pr       portRange
		wantPort uint16
		wantMask uint16
		wantErr  bool
	}{
		{name: "Wildcard range", pr: portRange{
			low:  0,
			high: 0,
		}, wantPort: 0, wantMask: 0, wantErr: false},
		{name: "Exact match range", pr: portRange{
			low:  100,
			high: 100,
		}, wantPort: 100, wantMask: 0xffff, wantErr: false},
		{name: "True range match fail", pr: portRange{
			low:  100,
			high: 200,
		}, wantPort: 0, wantMask: 0, wantErr: true},
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.pr.asTrivialTernaryMatch()
				if (err != nil) != tt.wantErr {
					t.Errorf("asTrivialTernaryMatch() error = %v, wantErr %v", err, tt.wantErr)
					return
				}

				if got.port != tt.wantPort {
					t.Errorf("asTrivialTernaryMatch() got = %v, want %v", got.port, tt.wantPort)
				}
				if got.mask != tt.wantMask {
					t.Errorf("asTrivialTernaryMatch() got = %v, want %v", got.mask, tt.wantMask)
				}
			},
		)
	}
}

func Test_portRange_Width(t *testing.T) {
	tests := []struct {
		name string
		pr   portRange
		want uint16
	}{
		{name: "wildcard", pr: newWildcardPortRange(), want: math.MaxUint16},
		{name: "zero value", pr: portRange{}, want: math.MaxUint16},
		{name: "exact match", pr: newExactMatchPortRange(100), want: 1},
		{name: "range match", pr: newRangeMatchPortRange(10, 12), want: 3},
		{name: "range single match", pr: newRangeMatchPortRange(1000, 1000), want: 1},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				if got := tt.pr.Width(); got != tt.want {
					t.Errorf("Width() = %v, want %v", got, tt.want)
				}
			},
		)
	}
}

func Test_pdr_parseSDFFilter(t *testing.T) {
	ueAddress := "17.0.0.1"

	newFilter := func(flowDesc string) *ie.IE {
		return ie.NewSDFFilter(flowDesc, "", "", "", 1)
	}

	tests := []struct {
		name          string
		direction     uint8
		sdfIE         *ie.IE
		wantAppFilter applicationFilter
		wantErr       bool
	}{
		{
			name:      "downlink SDF filter - app L4 port not spec-compliant",
			sdfIE:     newFilter("permit out udp from 192.168.1.1/32 to assigned 80-400"),
			direction: core,
			wantAppFilter: applicationFilter{
				srcIP:        ip2int(net.ParseIP("192.168.1.1")),
				dstIP:        ip2int(net.ParseIP(ueAddress)),
				srcPortRange: newRangeMatchPortRange(80, 400),
				dstPortRange: newWildcardPortRange(),
				proto:        17,
				srcIPMask:    math.MaxUint32,
				dstIPMask:    math.MaxUint32,
				protoMask:    math.MaxUint8,
			},
			wantErr: false,
		},
		{
			name:      "uplink SDF filter - app L4 port not spec-compliant",
			sdfIE:     newFilter("permit out udp from 192.168.1.1/32 to assigned 80-400"),
			direction: access,
			wantAppFilter: applicationFilter{
				srcIP:        ip2int(net.ParseIP(ueAddress)),
				dstIP:        ip2int(net.ParseIP("192.168.1.1")),
				srcPortRange: newWildcardPortRange(),
				dstPortRange: newRangeMatchPortRange(80, 400),
				proto:        17,
				srcIPMask:    math.MaxUint32,
				dstIPMask:    math.MaxUint32,
				protoMask:    math.MaxUint8,
			},
			wantErr: false,
		},
		{
			name:      "downlink SDF filter - app L4 port spec-compliant",
			sdfIE:     newFilter("permit out udp from 192.168.1.1/32 80-400 to assigned"),
			direction: core,
			wantAppFilter: applicationFilter{
				srcIP:        ip2int(net.ParseIP("192.168.1.1")),
				dstIP:        ip2int(net.ParseIP(ueAddress)),
				srcPortRange: newRangeMatchPortRange(80, 400),
				dstPortRange: newWildcardPortRange(),
				proto:        17,
				srcIPMask:    math.MaxUint32,
				dstIPMask:    math.MaxUint32,
				protoMask:    math.MaxUint8,
			},
			wantErr: false,
		},
		{
			name:      "uplink SDF filter - app L4 port spec-compliant",
			sdfIE:     newFilter("permit out udp from 192.168.1.1/32 80-400 to assigned"),
			direction: access,
			wantAppFilter: applicationFilter{
				srcIP:        ip2int(net.ParseIP(ueAddress)),
				dstIP:        ip2int(net.ParseIP("192.168.1.1")),
				srcPortRange: newWildcardPortRange(),
				dstPortRange: newRangeMatchPortRange(80, 400),
				proto:        17,
				srcIPMask:    math.MaxUint32,
				dstIPMask:    math.MaxUint32,
				protoMask:    math.MaxUint8,
			},
			wantErr: false,
		},
		{
			name:    "wrong IE type passed",
			sdfIE:   ie.NewQERID(0),
			wantErr: true,
		},
		{
			name:    "empty flow description",
			sdfIE:   newFilter(""),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &pdr{
				ueAddress: ip2int(net.ParseIP("17.0.0.1")),
				srcIface:  tt.direction,
			}
			if err := p.parseSDFFilter(tt.sdfIE); (err != nil) != tt.wantErr {
				t.Errorf("parseSDFFilter() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				require.Equal(t, tt.wantAppFilter, p.appFilter)
			}
		})
	}
}

func Test_pdr_parsePDI(t *testing.T) {
	ueAddress := "17.0.0.1"

	type args struct {
		pdiIEs  []*ie.IE
		appPFDs map[string]appPFD
		ippool  *IPPool
	}

	tests := []struct {
		name     string
		inputPDR pdr
		args     args
		wantPDR  pdr
		wantErr  bool
	}{
		{
			name: "uplink PDR - no SDF Filter IE",
			args: args{
				pdiIEs: []*ie.IE{
					ie.NewUEIPAddress(0x2, ueAddress, "", 0, 0),
					ie.NewSourceInterface(ie.SrcInterfaceAccess),
				},
			},
			wantPDR: pdr{
				srcIface:     access,
				srcIfaceMask: math.MaxUint8,
				ueAddress:    ip2int(net.ParseIP(ueAddress)),
				appFilter: applicationFilter{
					srcIP:     ip2int(net.ParseIP(ueAddress)),
					srcIPMask: math.MaxUint32,
				},
			},
			wantErr: false,
		},
		{
			name: "downlink PDR - no SDF Filter IE",
			args: args{
				pdiIEs: []*ie.IE{
					ie.NewUEIPAddress(0x2, ueAddress, "", 0, 0),
					ie.NewSourceInterface(ie.SrcInterfaceCore),
				},
			},
			wantPDR: pdr{
				srcIface:     core,
				srcIfaceMask: math.MaxUint8,
				ueAddress:    ip2int(net.ParseIP(ueAddress)),
				appFilter: applicationFilter{
					dstIP:     ip2int(net.ParseIP(ueAddress)),
					dstIPMask: math.MaxUint32,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pdr{}
			if err := p.parsePDI(tt.args.pdiIEs, tt.args.appPFDs, tt.args.ippool); (err != nil) != tt.wantErr {
				t.Errorf("parsePDI() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				require.Equal(t, tt.wantPDR, p)
			}
		})
	}
}
