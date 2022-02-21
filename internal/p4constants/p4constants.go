// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package p4constants

//noinspection GoSnakeCaseUsage
const (

	// HeaderFields
	HdrPreQosPipeRoutingRoutesV4dstPrefix      uint32 = 1
	HdrPreQosPipeAclAclsinport                 uint32 = 1
	HdrPreQosPipeAclAclssrcIface               uint32 = 2
	HdrPreQosPipeAclAclsethSrc                 uint32 = 3
	HdrPreQosPipeAclAclsethDst                 uint32 = 4
	HdrPreQosPipeAclAclsethType                uint32 = 5
	HdrPreQosPipeAclAclsipv4Src                uint32 = 6
	HdrPreQosPipeAclAclsipv4Dst                uint32 = 7
	HdrPreQosPipeAclAclsipv4Proto              uint32 = 8
	HdrPreQosPipeAclAclsl4Sport                uint32 = 9
	HdrPreQosPipeAclAclsl4Dport                uint32 = 10
	HdrPreQosPipeMyStationdstMac               uint32 = 1
	HdrPreQosPipeInterfacesipv4DstPrefix       uint32 = 1
	HdrPreQosPipeSessionsUplinkn3Address       uint32 = 1
	HdrPreQosPipeSessionsUplinkteid            uint32 = 2
	HdrPreQosPipeSessionsDownlinkueAddress     uint32 = 1
	HdrPreQosPipeTerminationsUplinkueAddress   uint32 = 1
	HdrPreQosPipeTerminationsUplinkappId       uint32 = 2
	HdrPreQosPipeTerminationsDownlinkueAddress uint32 = 1
	HdrPreQosPipeTerminationsDownlinkappId     uint32 = 2
	HdrPreQosPipeApplicationsappIpAddr         uint32 = 1
	HdrPreQosPipeApplicationsappL4Port         uint32 = 2
	HdrPreQosPipeApplicationsappIpProto        uint32 = 3
	HdrPreQosPipeTunnelPeerstunnelPeerId       uint32 = 1
	// Tables
	TablePreQosPipeRoutingRoutesV4      uint32 = 39015874
	TablePreQosPipeAclAcls              uint32 = 47204971
	TablePreQosPipeMyStation            uint32 = 40931612
	TablePreQosPipeInterfaces           uint32 = 33923840
	TablePreQosPipeSessionsUplink       uint32 = 44976597
	TablePreQosPipeSessionsDownlink     uint32 = 34742049
	TablePreQosPipeTerminationsUplink   uint32 = 37595532
	TablePreQosPipeTerminationsDownlink uint32 = 34778590
	TablePreQosPipeApplications         uint32 = 46868458
	TablePreQosPipeTunnelPeers          uint32 = 49497304
	// Actions
	ActionNoAction                         uint32 = 21257015
	ActionPreQosPipeRoutingDrop            uint32 = 31448256
	ActionPreQosPipeRoutingRoute           uint32 = 23965128
	ActionPreQosPipeAclSetPort             uint32 = 30494847
	ActionPreQosPipeAclPunt                uint32 = 26495283
	ActionPreQosPipeAclCloneToCpu          uint32 = 21596798
	ActionPreQosPipeAclDrop                uint32 = 18812293
	ActionPreQosPipeInitializeMetadata     uint32 = 23766285
	ActionPreQosPipeSetSourceIface         uint32 = 26090030
	ActionPreQosPipeDoDrop                 uint32 = 28401267
	ActionPreQosPipeSetSessionUplink       uint32 = 19461580
	ActionPreQosPipeSetSessionUplinkDrop   uint32 = 22196934
	ActionPreQosPipeSetSessionDownlink     uint32 = 21848329
	ActionPreQosPipeSetSessionDownlinkDrop uint32 = 20229579
	ActionPreQosPipeSetSessionDownlinkBuff uint32 = 20249483
	ActionPreQosPipeUplinkTermFwdNoTc      uint32 = 21760615
	ActionPreQosPipeUplinkTermFwd          uint32 = 28305359
	ActionPreQosPipeUplinkTermDrop         uint32 = 20977365
	ActionPreQosPipeDownlinkTermFwdNoTc    uint32 = 26185804
	ActionPreQosPipeDownlinkTermFwd        uint32 = 32699713
	ActionPreQosPipeDownlinkTermDrop       uint32 = 31264233
	ActionPreQosPipeSetAppId               uint32 = 23010411
	ActionPreQosPipeLoadTunnelParam        uint32 = 32742981
	ActionPreQosPipeDoGtpuTunnel           uint32 = 29247910
	ActionPreQosPipeDoGtpuTunnelWithPsc    uint32 = 31713420
	// ActionParams
	ActionParamPreQosPipeRoutingRoutesrcMac                    uint32 = 1
	ActionParamPreQosPipeRoutingRoutedstMac                    uint32 = 2
	ActionParamPreQosPipeRoutingRouteegressPort                uint32 = 3
	ActionParamPreQosPipeAclSetPortport                        uint32 = 1
	ActionParamPreQosPipeSetSourceIfacesrcIface                uint32 = 1
	ActionParamPreQosPipeSetSourceIfacedirection               uint32 = 2
	ActionParamPreQosPipeSetSourceIfacesliceId                 uint32 = 3
	ActionParamPreQosPipeSetSessionUplinksessionMeterIdx       uint32 = 1
	ActionParamPreQosPipeSetSessionDownlinktunnelPeerId        uint32 = 1
	ActionParamPreQosPipeSetSessionDownlinksessionMeterIdx     uint32 = 2
	ActionParamPreQosPipeSetSessionDownlinkBuffsessionMeterIdx uint32 = 1
	ActionParamPreQosPipeUplinkTermFwdNoTcctrIdx               uint32 = 1
	ActionParamPreQosPipeUplinkTermFwdNoTcappMeterIdx          uint32 = 2
	ActionParamPreQosPipeUplinkTermFwdctrIdx                   uint32 = 1
	ActionParamPreQosPipeUplinkTermFwdtc                       uint32 = 2
	ActionParamPreQosPipeUplinkTermFwdappMeterIdx              uint32 = 3
	ActionParamPreQosPipeUplinkTermDropctrIdx                  uint32 = 1
	ActionParamPreQosPipeDownlinkTermFwdNoTcctrIdx             uint32 = 1
	ActionParamPreQosPipeDownlinkTermFwdNoTcteid               uint32 = 2
	ActionParamPreQosPipeDownlinkTermFwdNoTcqfi                uint32 = 3
	ActionParamPreQosPipeDownlinkTermFwdNoTcappMeterIdx        uint32 = 4
	ActionParamPreQosPipeDownlinkTermFwdctrIdx                 uint32 = 1
	ActionParamPreQosPipeDownlinkTermFwdteid                   uint32 = 2
	ActionParamPreQosPipeDownlinkTermFwdqfi                    uint32 = 3
	ActionParamPreQosPipeDownlinkTermFwdtc                     uint32 = 4
	ActionParamPreQosPipeDownlinkTermFwdappMeterIdx            uint32 = 5
	ActionParamPreQosPipeDownlinkTermDropctrIdx                uint32 = 1
	ActionParamPreQosPipeSetAppIdappId                         uint32 = 1
	ActionParamPreQosPipeLoadTunnelParamsrcAddr                uint32 = 1
	ActionParamPreQosPipeLoadTunnelParamdstAddr                uint32 = 2
	ActionParamPreQosPipeLoadTunnelParamsport                  uint32 = 3
	// IndirectCounters
	CounterPreQosPipePreQosCounter   uint32 = 315693181
	CounterPostQosPipePostQosCounter uint32 = 302958180
	// DirectCounters
	DirectCounterAcls uint32 = 325583051
	// ActionProfiles
	ActionProfileHashedSelector uint32 = 297808402
	// PacketMetadata
	PacketMetaPacketOut uint32 = 75327753
	PacketMetaPacketIn  uint32 = 80671331
	// Meters
	MeterPreQosPipeAppMeter     uint32 = 338231090
	MeterPreQosPipeSessionMeter uint32 = 347593234
)
