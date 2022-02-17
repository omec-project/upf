/*
 * Copyright 2022-present Open Networking Foundation
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package p4constants

//noinspection GoSnakeCaseUsage
const (
	// Header field IDs
	Hdr_PreQosPipeTunnelPeers_TunnelPeerId       uint32 = 1
	Hdr_PreQosPipeApplications_AppL4Port         uint32 = 2
	Hdr_PreQosPipeApplications_AppIpProto        uint32 = 3
	Hdr_PreQosPipeApplications_AppIpAddr         uint32 = 1
	Hdr_PreQosPipeAclAcls_SrcIface               uint32 = 2
	Hdr_PreQosPipeAclAcls_L4Sport                uint32 = 9
	Hdr_PreQosPipeAclAcls_Ipv4Dst                uint32 = 7
	Hdr_PreQosPipeAclAcls_Ipv4Src                uint32 = 6
	Hdr_PreQosPipeAclAcls_L4Dport                uint32 = 10
	Hdr_PreQosPipeAclAcls_Inport                 uint32 = 1
	Hdr_PreQosPipeAclAcls_Ipv4Proto              uint32 = 8
	Hdr_PreQosPipeAclAcls_EthType                uint32 = 5
	Hdr_PreQosPipeAclAcls_EthSrc                 uint32 = 3
	Hdr_PreQosPipeAclAcls_EthDst                 uint32 = 4
	Hdr_PreQosPipeTerminationsDownlink_AppId     uint32 = 2
	Hdr_PreQosPipeTerminationsDownlink_UeAddress uint32 = 1
	Hdr_PreQosPipeInterfaces_Ipv4DstPrefix       uint32 = 1
	Hdr_PreQosPipeMyStation_DstMac               uint32 = 1
	Hdr_PreQosPipeRoutingRoutesV4_DstPrefix      uint32 = 1
	Hdr_PreQosPipeSessionsDownlink_UeAddress     uint32 = 1
	Hdr_PreQosPipeSessionsUplink_N3Address       uint32 = 1
	Hdr_PreQosPipeSessionsUplink_Teid            uint32 = 2
	Hdr_PreQosPipeTerminationsUplink_AppId       uint32 = 2
	Hdr_PreQosPipeTerminationsUplink_UeAddress   uint32 = 1
	// Table IDs
	Table_PreQosPipeTunnelPeers          uint32 = 49497304
	Table_PreQosPipeAclAcls              uint32 = 47204971
	Table_PreQosPipeTerminationsUplink   uint32 = 37595532
	Table_PreQosPipeRoutingRoutesV4      uint32 = 39015874
	Table_PreQosPipeSessionsUplink       uint32 = 44976597
	Table_PreQosPipeMyStation            uint32 = 40931612
	Table_PreQosPipeSessionsDownlink     uint32 = 34742049
	Table_PreQosPipeApplications         uint32 = 46868458
	Table_PreQosPipeInterfaces           uint32 = 33923840
	Table_PreQosPipeTerminationsDownlink uint32 = 34778590
	// Indirect Counter IDs
	Counter_PreQosPipePreQosCounter   uint32 = 315693181
	Counter_PostQosPipePostQosCounter uint32 = 302958180
	// Direct Counter IDs
	DirectCounter_Acls uint32 = 325583051
	// Action IDs
	Action_PreQosPipeRoutingDrop            uint32 = 31448256
	Action_PreQosPipeSetSessionDownlinkBuff uint32 = 20249483
	Action_PreQosPipeUplinkTermFwd          uint32 = 28305359
	Action_PreQosPipeDownlinkTermFwd        uint32 = 32699713
	Action_PreQosPipeAclPunt                uint32 = 26495283
	Action_NoAction                         uint32 = 21257015
	Action_PreQosPipeSetSessionDownlinkDrop uint32 = 20229579
	Action_PreQosPipeAclSetPort             uint32 = 30494847
	Action_PreQosPipeDoGtpuTunnelWithPsc    uint32 = 31713420
	Action_PreQosPipeUplinkTermDrop         uint32 = 20977365
	Action_PreQosPipeDoDrop                 uint32 = 28401267
	Action_PreQosPipeAclDrop                uint32 = 18812293
	Action_PreQosPipeSetSourceIface         uint32 = 26090030
	Action_PreQosPipeDoGtpuTunnel           uint32 = 29247910
	Action_PreQosPipeSetSessionUplinkDrop   uint32 = 22196934
	Action_PreQosPipeSetSessionDownlink     uint32 = 21848329
	Action_PreQosPipeLoadTunnelParam        uint32 = 32742981
	Action_PreQosPipeInitializeMetadata     uint32 = 23766285
	Action_PreQosPipeUplinkTermFwdNoTc      uint32 = 21760615
	Action_PreQosPipeRoutingRoute           uint32 = 23965128
	Action_PreQosPipeSetAppId               uint32 = 23010411
	Action_PreQosPipeDownlinkTermFwdNoTc    uint32 = 26185804
	Action_PreQosPipeSetSessionUplink       uint32 = 19461580
	Action_PreQosPipeDownlinkTermDrop       uint32 = 31264233
	Action_PreQosPipeAclCloneToCpu          uint32 = 21596798
	// Action Param IDs
	ActionParam_PreQosPipeSetSessionDownlink_TunnelPeerId        uint32 = 1
	ActionParam_PreQosPipeSetSessionDownlink_SessionMeterIdx     uint32 = 2
	ActionParam_PreQosPipeRoutingRoute_SrcMac                    uint32 = 1
	ActionParam_PreQosPipeRoutingRoute_EgressPort                uint32 = 3
	ActionParam_PreQosPipeRoutingRoute_DstMac                    uint32 = 2
	ActionParam_PreQosPipeUplinkTermFwdNoTc_AppMeterIdx          uint32 = 2
	ActionParam_PreQosPipeUplinkTermFwdNoTc_CtrIdx               uint32 = 1
	ActionParam_PreQosPipeSetSessionUplink_SessionMeterIdx       uint32 = 1
	ActionParam_PreQosPipeUplinkTermDrop_CtrIdx                  uint32 = 1
	ActionParam_PreQosPipeSetAppId_AppId                         uint32 = 1
	ActionParam_PreQosPipeAclSetPort_Port                        uint32 = 1
	ActionParam_PreQosPipeSetSessionDownlinkBuff_SessionMeterIdx uint32 = 1
	ActionParam_PreQosPipeDownlinkTermDrop_CtrIdx                uint32 = 1
	ActionParam_PreQosPipeDownlinkTermFwd_Tc                     uint32 = 4
	ActionParam_PreQosPipeDownlinkTermFwd_Teid                   uint32 = 2
	ActionParam_PreQosPipeDownlinkTermFwd_AppMeterIdx            uint32 = 5
	ActionParam_PreQosPipeDownlinkTermFwd_CtrIdx                 uint32 = 1
	ActionParam_PreQosPipeDownlinkTermFwd_Qfi                    uint32 = 3
	ActionParam_PreQosPipeUplinkTermFwd_AppMeterIdx              uint32 = 3
	ActionParam_PreQosPipeUplinkTermFwd_Tc                       uint32 = 2
	ActionParam_PreQosPipeUplinkTermFwd_CtrIdx                   uint32 = 1
	ActionParam_PreQosPipeLoadTunnelParam_DstAddr                uint32 = 2
	ActionParam_PreQosPipeLoadTunnelParam_SrcAddr                uint32 = 1
	ActionParam_PreQosPipeLoadTunnelParam_Sport                  uint32 = 3
	ActionParam_PreQosPipeSetSourceIface_SliceId                 uint32 = 3
	ActionParam_PreQosPipeSetSourceIface_SrcIface                uint32 = 1
	ActionParam_PreQosPipeSetSourceIface_Direction               uint32 = 2
	ActionParam_PreQosPipeDownlinkTermFwdNoTc_AppMeterIdx        uint32 = 4
	ActionParam_PreQosPipeDownlinkTermFwdNoTc_Teid               uint32 = 2
	ActionParam_PreQosPipeDownlinkTermFwdNoTc_CtrIdx             uint32 = 1
	ActionParam_PreQosPipeDownlinkTermFwdNoTc_Qfi                uint32 = 3
	// Action Profile IDs
	ActionProfile_HashedSelector uint32 = 297808402
	// Packet Metadata IDs
	PacketMeta_IngressPort uint32 = 1
	PacketMeta_Reserved    uint32 = 1
	// Meter IDs
	Meter_PreQosPipeAppMeter     uint32 = 338231090
	Meter_PreQosPipeSessionMeter uint32 = 347593234
)
