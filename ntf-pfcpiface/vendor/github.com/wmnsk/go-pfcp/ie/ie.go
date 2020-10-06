// Copyright 2019-2020 go-pfcp authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ie

import (
	"encoding/binary"
	"io"

	"github.com/wmnsk/go-pfcp/internal/logger"
)

// IE Type definitions.
const (
	_                                                                uint16 = 0
	CreatePDR                                                        uint16 = 1
	PDI                                                              uint16 = 2
	CreateFAR                                                        uint16 = 3
	ForwardingParameters                                             uint16 = 4
	DuplicatingParameters                                            uint16 = 5
	CreateURR                                                        uint16 = 6
	CreateQER                                                        uint16 = 7
	CreatedPDR                                                       uint16 = 8
	UpdatePDR                                                        uint16 = 9
	UpdateFAR                                                        uint16 = 10
	UpdateForwardingParameters                                       uint16 = 11
	UpdateBARWithinSessionReportResponse                             uint16 = 12
	UpdateURR                                                        uint16 = 13
	UpdateQER                                                        uint16 = 14
	RemovePDR                                                        uint16 = 15
	RemoveFAR                                                        uint16 = 16
	RemoveURR                                                        uint16 = 17
	RemoveQER                                                        uint16 = 18
	Cause                                                            uint16 = 19
	SourceInterface                                                  uint16 = 20
	FTEID                                                            uint16 = 21
	NetworkInstance                                                  uint16 = 22
	SDFFilter                                                        uint16 = 23
	ApplicationID                                                    uint16 = 24
	GateStatus                                                       uint16 = 25
	MBR                                                              uint16 = 26
	GBR                                                              uint16 = 27
	QERCorrelationID                                                 uint16 = 28
	Precedence                                                       uint16 = 29
	TransportLevelMarking                                            uint16 = 30
	VolumeThreshold                                                  uint16 = 31
	TimeThreshold                                                    uint16 = 32
	MonitoringTime                                                   uint16 = 33
	SubsequentVolumeThreshold                                        uint16 = 34
	SubsequentTimeThreshold                                          uint16 = 35
	InactivityDetectionTime                                          uint16 = 36
	ReportingTriggers                                                uint16 = 37
	RedirectInformation                                              uint16 = 38
	ReportType                                                       uint16 = 39
	OffendingIE                                                      uint16 = 40
	ForwardingPolicy                                                 uint16 = 41
	DestinationInterface                                             uint16 = 42
	UPFunctionFeatures                                               uint16 = 43
	ApplyAction                                                      uint16 = 44
	DownlinkDataServiceInformation                                   uint16 = 45
	DownlinkDataNotificationDelay                                    uint16 = 46
	DLBufferingDuration                                              uint16 = 47
	DLBufferingSuggestedPacketCount                                  uint16 = 48
	PFCPSMReqFlags                                                   uint16 = 49
	PFCPSRRspFlags                                                   uint16 = 50
	LoadControlInformation                                           uint16 = 51
	SequenceNumber                                                   uint16 = 52
	Metric                                                           uint16 = 53
	OverloadControlInformation                                       uint16 = 54
	Timer                                                            uint16 = 55
	PDRID                                                            uint16 = 56
	FSEID                                                            uint16 = 57
	ApplicationIDsPFDs                                               uint16 = 58
	PFDContext                                                       uint16 = 59
	NodeID                                                           uint16 = 60
	PFDContents                                                      uint16 = 61
	MeasurementMethod                                                uint16 = 62
	UsageReportTrigger                                               uint16 = 63
	MeasurementPeriod                                                uint16 = 64
	FQCSID                                                           uint16 = 65
	VolumeMeasurement                                                uint16 = 66
	DurationMeasurement                                              uint16 = 67
	ApplicationDetectionInformation                                  uint16 = 68
	TimeOfFirstPacket                                                uint16 = 69
	TimeOfLastPacket                                                 uint16 = 70
	QuotaHoldingTime                                                 uint16 = 71
	DroppedDLTrafficThreshold                                        uint16 = 72
	VolumeQuota                                                      uint16 = 73
	TimeQuota                                                        uint16 = 74
	StartTime                                                        uint16 = 75
	EndTime                                                          uint16 = 76
	QueryURR                                                         uint16 = 77
	UsageReportWithinSessionModificationResponse                     uint16 = 78
	UsageReportWithinSessionDeletionResponse                         uint16 = 79
	UsageReportWithinSessionReportRequest                            uint16 = 80
	URRID                                                            uint16 = 81
	LinkedURRID                                                      uint16 = 82
	DownlinkDataReport                                               uint16 = 83
	OuterHeaderCreation                                              uint16 = 84
	CreateBAR                                                        uint16 = 85
	UpdateBARWithinSessionModificationRequest                        uint16 = 86
	RemoveBAR                                                        uint16 = 87
	BARID                                                            uint16 = 88
	CPFunctionFeatures                                               uint16 = 89
	UsageInformation                                                 uint16 = 90
	ApplicationInstanceID                                            uint16 = 91
	FlowInformation                                                  uint16 = 92
	UEIPAddress                                                      uint16 = 93
	PacketRate                                                       uint16 = 94
	OuterHeaderRemoval                                               uint16 = 95
	RecoveryTimeStamp                                                uint16 = 96
	DLFlowLevelMarking                                               uint16 = 97
	HeaderEnrichment                                                 uint16 = 98
	ErrorIndicationReport                                            uint16 = 99
	MeasurementInformation                                           uint16 = 100
	NodeReportType                                                   uint16 = 101
	UserPlanePathFailureReport                                       uint16 = 102
	RemoteGTPUPeer                                                   uint16 = 103
	URSEQN                                                           uint16 = 104
	UpdateDuplicatingParameters                                      uint16 = 105
	ActivatePredefinedRules                                          uint16 = 106
	DeactivatePredefinedRules                                        uint16 = 107
	FARID                                                            uint16 = 108
	QERID                                                            uint16 = 109
	OCIFlags                                                         uint16 = 110
	PFCPAssociationReleaseRequest                                    uint16 = 111
	GracefulReleasePeriod                                            uint16 = 112
	PDNType                                                          uint16 = 113
	FailedRuleID                                                     uint16 = 114
	TimeQuotaMechanism                                               uint16 = 115
	UserPlaneIPResourceInformation                                   uint16 = 116
	UserPlaneInactivityTimer                                         uint16 = 117
	AggregatedURRs                                                   uint16 = 118
	Multiplier                                                       uint16 = 119
	AggregatedURRID                                                  uint16 = 120
	SubsequentVolumeQuota                                            uint16 = 121
	SubsequentTimeQuota                                              uint16 = 122
	RQI                                                              uint16 = 123
	QFI                                                              uint16 = 124
	QueryURRReference                                                uint16 = 125
	AdditionalUsageReportsInformation                                uint16 = 126
	CreateTrafficEndpoint                                            uint16 = 127
	CreatedTrafficEndpoint                                           uint16 = 128
	UpdateTrafficEndpoint                                            uint16 = 129
	RemoveTrafficEndpoint                                            uint16 = 130
	TrafficEndpointID                                                uint16 = 131
	EthernetPacketFilter                                             uint16 = 132
	MACAddress                                                       uint16 = 133
	CTAG                                                             uint16 = 134
	STAG                                                             uint16 = 135
	Ethertype                                                        uint16 = 136
	Proxying                                                         uint16 = 137
	EthernetFilterID                                                 uint16 = 138
	EthernetFilterProperties                                         uint16 = 139
	SuggestedBufferingPacketsCount                                   uint16 = 140
	UserID                                                           uint16 = 141
	EthernetPDUSessionInformation                                    uint16 = 142
	EthernetTrafficInformation                                       uint16 = 143
	MACAddressesDetected                                             uint16 = 144
	MACAddressesRemoved                                              uint16 = 145
	EthernetInactivityTimer                                          uint16 = 146
	AdditionalMonitoringTime                                         uint16 = 147
	EventQuota                                                       uint16 = 148
	EventThreshold                                                   uint16 = 149
	SubsequentEventQuota                                             uint16 = 150
	SubsequentEventThreshold                                         uint16 = 151
	TraceInformation                                                 uint16 = 152
	FramedRoute                                                      uint16 = 153
	FramedRouting                                                    uint16 = 154
	FramedIPv6Route                                                  uint16 = 155
	EventTimeStamp                                                   uint16 = 156
	AveragingWindow                                                  uint16 = 157
	PagingPolicyIndicator                                            uint16 = 158
	APNDNN                                                           uint16 = 159
	TGPPInterfaceType                                                uint16 = 160
	PFCPSRReqFlags                                                   uint16 = 161
	PFCPAUReqFlags                                                   uint16 = 162
	ActivationTime                                                   uint16 = 163
	DeactivationTime                                                 uint16 = 164
	CreateMAR                                                        uint16 = 165
	TGPPAccessForwardingActionInformation                            uint16 = 166
	NonTGPPAccessForwardingActionInformation                         uint16 = 167
	RemoveMAR                                                        uint16 = 168
	UpdateMAR                                                        uint16 = 169
	MARID                                                            uint16 = 170
	SteeringFunctionality                                            uint16 = 171
	SteeringMode                                                     uint16 = 172
	Weight                                                           uint16 = 173
	Priority                                                         uint16 = 174
	UpdateTGPPAccessForwardingActionInformation                      uint16 = 175
	UpdateNonTGPPAccessForwardingActionInformation                   uint16 = 176
	UEIPAddressPoolIdentity                                          uint16 = 177
	AlternativeSMFIPAddress                                          uint16 = 178
	PacketReplicationAndDetectionCarryOnInformation                  uint16 = 179
	SMFSetID                                                         uint16 = 180
	QuotaValidityTime                                                uint16 = 181
	NumberOfReports                                                  uint16 = 182
	PFCPSessionRetentionInformation                                  uint16 = 183
	PFCPASRspFlags                                                   uint16 = 184
	CPPFCPEntityIPAddress                                            uint16 = 185
	PFCPSEReqFlags                                                   uint16 = 186
	UserPlanePathRecoveryReport                                      uint16 = 187
	IPMulticastAddressingInfo                                        uint16 = 188
	JoinIPMulticastInformationWithinUsageReport                      uint16 = 189
	LeaveIPMulticastInformationWithinUsageReport                     uint16 = 190
	IPMulticastAddress                                               uint16 = 191
	SourceIPAddress                                                  uint16 = 192
	PacketRateStatus                                                 uint16 = 193
	CreateBridgeInfoForTSC                                           uint16 = 194
	CreatedBridgeInfoForTSC                                          uint16 = 195
	DSTTPortNumber                                                   uint16 = 196
	NWTTPortNumber                                                   uint16 = 197
	TSNBridgeID                                                      uint16 = 198
	PortManagementInformationForTSCWithinSessionModificationRequest  uint16 = 199
	PortManagementInformationForTSCWithinSessionModificationResponse uint16 = 200
	PortManagementInformationForTSCWithinSessionReportRequest        uint16 = 201
	PortManagementInformationContainer                               uint16 = 202
	ClockDriftControlInformation                                     uint16 = 203
	RequestedClockDriftInformation                                   uint16 = 204
	ClockDriftReport                                                 uint16 = 205
	TSNTimeDomainNumber                                              uint16 = 206
	TimeOffsetThreshold                                              uint16 = 207
	CumulativeRateRatioThreshold                                     uint16 = 208
	TimeOffsetMeasurement                                            uint16 = 209
	CumulativeRateRatioMeasurement                                   uint16 = 210
	RemoveSRR                                                        uint16 = 211
	CreateSRR                                                        uint16 = 212
	UpdateSRR                                                        uint16 = 213
	SessionReport                                                    uint16 = 214
	SRRID                                                            uint16 = 215
	AccessAvailabilityControlInformation                             uint16 = 216
	RequestedAccessAvailabilityInformation                           uint16 = 217
	AccessAvailabilityReport                                         uint16 = 218
	AccessAvailabilityInformation                                    uint16 = 219
	ProvideATSSSControlInformation                                   uint16 = 220
	ATSSSControlParameters                                           uint16 = 221
	MPTCPControlInformation                                          uint16 = 222
	ATSSSLLControlInformation                                        uint16 = 223
	PMFControlInformation                                            uint16 = 224
	MPTCPParameters                                                  uint16 = 225
	ATSSSLLParameters                                                uint16 = 226
	PMFParameters                                                    uint16 = 227
	MPTCPAddressInformation                                          uint16 = 228
	UELinkSpecificIPAddress                                          uint16 = 229
	PMFAddressInformation                                            uint16 = 230
	ATSSSLLInformation                                               uint16 = 231
	DataNetworkAccessIdentifier                                      uint16 = 232
	UEIPAddressPoolInformation                                       uint16 = 233
	AveragePacketDelay                                               uint16 = 234
	MinimumPacketDelay                                               uint16 = 235
	MaximumPacketDelay                                               uint16 = 236
	QoSReportTrigger                                                 uint16 = 237
	GTPUPathQoSControlInformation                                    uint16 = 238
	GTPUPathQoSReport                                                uint16 = 239
	QoSInformationInGTPUPathQoSReport                                uint16 = 240
	GTPUPathInterfaceType                                            uint16 = 241
	QoSMonitoringPerQoSFlowControlInformation                        uint16 = 242
	RequestedQoSMonitoring                                           uint16 = 243
	ReportingFrequency                                               uint16 = 244
	PacketDelayThresholds                                            uint16 = 245
	MinimumWaitTime                                                  uint16 = 246
	QoSMonitoringReport                                              uint16 = 247
	QoSMonitoringMeasurement                                         uint16 = 248
	MTEDTControlInformation                                          uint16 = 249
	DLDataPacketsSize                                                uint16 = 250
	QERControlIndications                                            uint16 = 251
	PacketRateStatusReport                                           uint16 = 252
	NFInstanceID                                                     uint16 = 253
	EthernetContextInformation                                       uint16 = 254
	RedundantTransmissionParameters                                  uint16 = 255
	UpdatedPDR                                                       uint16 = 256
)

// IE represents an Information Element of PFCP messages.
type IE struct {
	Type         uint16
	Length       uint16
	EnterpriseID uint16
	Payload      []byte
	ChildIEs     []*IE
}

// New creates a new IE.
func New(itype uint16, data []byte) *IE {
	i := &IE{
		Type:    itype,
		Payload: data,
	}
	i.SetLength()

	return i
}

// NewVendorSpecificIE creates a new vendor-specific IE.
func NewVendorSpecificIE(itype, eid uint16, data []byte) *IE {
	i := &IE{
		Type:         itype,
		EnterpriseID: eid,
		Payload:      data,
	}
	i.SetLength()

	return i
}

// NewGroupedIE creates a new grouped IE.
func NewGroupedIE(itype uint16, ies ...*IE) *IE {
	return newGroupedIE(itype, 0, ies...)
}

// NewVendorSpecificGroupedIE creates a new grouped IE.
func NewVendorSpecificGroupedIE(itype, eid uint16, ies ...*IE) *IE {
	return newGroupedIE(itype, eid, ies...)
}

// Parse parses b into IE.
func Parse(b []byte) (*IE, error) {
	i := &IE{}
	if err := i.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return i, nil
}

// UnmarshalBinary parses b into IE.
func (i *IE) UnmarshalBinary(b []byte) error {
	l := len(b)
	if l < 4 {
		return io.ErrUnexpectedEOF
	}

	i.Type = binary.BigEndian.Uint16(b[0:2])
	i.Length = binary.BigEndian.Uint16(b[2:4])

	offset := 4
	if i.IsVendorSpecific() && l >= 6 {
		i.EnterpriseID = binary.BigEndian.Uint16(b[4:6])
		offset += 2
	}

	if l <= offset {
		return nil
	}

	i.Payload = b[offset : offset+int(i.Length)]

	if i.IsGrouped() {
		var err error
		i.ChildIEs, err = ParseMultiIEs(i.Payload)
		if err != nil {
			return err
		}
	}

	return nil
}

// Marshal returns the byte sequence generated from an IE instance.
func (i *IE) Marshal() ([]byte, error) {
	b := make([]byte, i.MarshalLen())
	if err := i.MarshalTo(b); err != nil {
		return nil, err
	}
	return b, nil
}

// MarshalTo puts the byte sequence in the byte array given as b.
func (i *IE) MarshalTo(b []byte) error {
	l := len(b)
	if l < 4 {
		return ErrInvalidLength
	}

	binary.BigEndian.PutUint16(b[:2], i.Type)
	binary.BigEndian.PutUint16(b[2:4], i.Length)

	offset := 4
	if i.IsVendorSpecific() && l >= 6 {
		binary.BigEndian.PutUint16(b[4:6], i.EnterpriseID)
		offset += 2
	}

	if i.IsGrouped() {
		for _, ie := range i.ChildIEs {
			if err := ie.MarshalTo(b[offset:]); err != nil {
				return err
			}
			offset += ie.MarshalLen()
		}
		return nil
	}

	copy(b[4:i.MarshalLen()], i.Payload)
	return nil
}

// MarshalLen returns field length in integer.
func (i *IE) MarshalLen() int {
	l := 4
	if i.IsVendorSpecific() {
		l += 2
	}

	if i.IsGrouped() {
		for _, ie := range i.ChildIEs {
			l += ie.MarshalLen()
		}
		return l
	}

	return l + len(i.Payload)
}

// SetLength sets the length in Length field.
func (i *IE) SetLength() {
	l := 0

	if i.IsVendorSpecific() {
		l += 2
	}

	i.Length = uint16(l + len(i.Payload))
}

// IsVendorSpecific reports whether an IE is vendor-specific or defined by 3gpp.
func (i *IE) IsVendorSpecific() bool {
	// Spef: TS 29.244 8.1.1 Information Element Format
	// If the Bit 8 of Octet 1 is not set, this indicates that the IE is defined by 3GPP
	// and the EnterpriseID is absent. If Bit 8 of Octet 1 is set, this indicates that the
	// IE is defined by a vendor and the Enterprise ID is present identified by the Enterprise ID.
	return i.Type&0x8000 != 0
}

var grouped = []uint16{
	// TODO: fill here with all the type of IEs that may be grouped, using constants above.
	1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
	17, 18, 51, 54, 58, 59, 68, 77, 78, 79, 80, 83, 85, 86, 87, 99, 102, 105, 118, 127,
	128, 129, 130, 132, 143, 147, 165, 166, 167, 168, 169, 175, 176, 183, 187, 188,
	189, 190, 195, 199, 200, 201, 203, 205, 211, 212, 213, 214, 216, 218, 220, 221,
	225, 226, 227, 233, 238, 239, 240, 242, 247, 252, 254, 255, 256,
}

// IsGrouped reports whether an IE is grouped type or not.
func (i *IE) IsGrouped() bool {
	for _, itype := range grouped {
		if i.Type == itype {
			return true
		}
	}
	return false
}

// Add adds variable number of IEs to a IE if the IE is grouped type and update length.
// Otherwise, this does nothing(no errors).
func (i *IE) Add(ies ...*IE) {
	if !i.IsGrouped() {
		return
	}

	i.Payload = nil
	i.ChildIEs = append(i.ChildIEs, ies...)
	for _, ie := range i.ChildIEs {
		serialized, err := ie.Marshal()
		if err != nil {
			continue
		}
		i.Payload = append(i.Payload, serialized...)
	}
	i.SetLength()
}

// Remove removes an IE looked up by type and instance.
func (i *IE) Remove(typ uint16) {
	if !i.IsGrouped() {
		return
	}

	i.Payload = nil
	var newChildren []*IE
	for _, ie := range i.ChildIEs {
		if ie.Type == typ {
			continue
		}
		newChildren = append(newChildren, ie)

		serialized, err := ie.Marshal()
		if err != nil {
			continue
		}
		i.Payload = append(i.Payload, serialized...)
	}
	i.ChildIEs = newChildren
	i.SetLength()
}

// FindByType returns IE looked up by type and instance.
//
// The program may be slower when calling this method multiple times
// because this ranges over a ChildIEs each time it is called.
func (i *IE) FindByType(typ uint16) (*IE, error) {
	if !i.IsGrouped() {
		return nil, ErrInvalidType
	}

	for _, ie := range i.ChildIEs {
		if ie.Type == typ {
			return ie, nil
		}
	}
	return nil, ErrIENotFound
}

// ParseMultiIEs decodes multiple IEs at a time.
// This is easy and useful but slower than decoding one by one.
// When you don't know the number of IEs, this is the only way to decode them.
// See benchmarks in diameter_test.go for the detail.
func ParseMultiIEs(b []byte) ([]*IE, error) {
	var ies []*IE
	for {
		if len(b) == 0 {
			break
		}

		i, err := Parse(b)
		if err != nil {
			return nil, err
		}
		ies = append(ies, i)
		b = b[i.MarshalLen():]
	}
	return ies, nil
}

func newUint8ValIE(t uint16, v uint8) *IE {
	return New(t, []byte{v})
}

func newUint16ValIE(t uint16, v uint16) *IE {
	i := New(t, make([]byte, 2))
	binary.BigEndian.PutUint16(i.Payload, v)
	return i
}

func newUint32ValIE(t uint16, v uint32) *IE {
	i := New(t, make([]byte, 4))
	binary.BigEndian.PutUint32(i.Payload, v)
	return i
}

func newUint64ValIE(t uint16, v uint64) *IE {
	i := New(t, make([]byte, 8))
	binary.BigEndian.PutUint64(i.Payload, v)
	return i
}

func newStringIE(t uint16, v string) *IE {
	return New(t, []byte(v))
}

func newGroupedIE(itype, eid uint16, ies ...*IE) *IE {
	i := NewVendorSpecificIE(itype, eid, make([]byte, 0))

	for _, ie := range ies {
		if ie == nil {
			continue
		}

		i.ChildIEs = append(i.ChildIEs, ie)

		serialized, err := ie.Marshal()
		if err != nil {
			logger.Logf("newGroupedIE() failed to marshal an IE(Type=%d): %v", ie.Type, err)
			return nil
		}
		i.Payload = append(i.Payload, serialized...)
	}

	i.SetLength()

	return i
}
