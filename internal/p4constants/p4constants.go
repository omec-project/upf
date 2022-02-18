/*
* Copyright 2022-present Open Networking Foundation
*
* SPDX-License-Identifier: Apache-2_0
 */
package p4constants

//noinspection GoSnakeCaseUsage
const (

	//HeaderFields
	Hdr_PreQosPipe_Routing_routes_v4DstPrefix     uint32 = 1
	Hdr_PreQosPipe_Acl_aclsInport                 uint32 = 1
	Hdr_PreQosPipe_Acl_aclsSrcIface               uint32 = 2
	Hdr_PreQosPipe_Acl_aclsEthSrc                 uint32 = 3
	Hdr_PreQosPipe_Acl_aclsEthDst                 uint32 = 4
	Hdr_PreQosPipe_Acl_aclsEthType                uint32 = 5
	Hdr_PreQosPipe_Acl_aclsIpv4Src                uint32 = 6
	Hdr_PreQosPipe_Acl_aclsIpv4Dst                uint32 = 7
	Hdr_PreQosPipe_Acl_aclsIpv4Proto              uint32 = 8
	Hdr_PreQosPipe_Acl_aclsL4Sport                uint32 = 9
	Hdr_PreQosPipe_Acl_aclsL4Dport                uint32 = 10
	Hdr_PreQosPipe_my_stationDstMac               uint32 = 1
	Hdr_PreQosPipe_interfacesIpv4DstPrefix        uint32 = 1
	Hdr_PreQosPipe_sessions_uplinkN3Address       uint32 = 1
	Hdr_PreQosPipe_sessions_uplinkTeid            uint32 = 2
	Hdr_PreQosPipe_sessions_downlinkUeAddress     uint32 = 1
	Hdr_PreQosPipe_terminations_uplinkUeAddress   uint32 = 1
	Hdr_PreQosPipe_terminations_uplinkAppId       uint32 = 2
	Hdr_PreQosPipe_terminations_downlinkUeAddress uint32 = 1
	Hdr_PreQosPipe_terminations_downlinkAppId     uint32 = 2
	Hdr_PreQosPipe_applicationsAppIpAddr          uint32 = 1
	Hdr_PreQosPipe_applicationsAppL4Port          uint32 = 2
	Hdr_PreQosPipe_applicationsAppIpProto         uint32 = 3
	Hdr_PreQosPipe_tunnel_peersTunnelPeerId       uint32 = 1
	//Tables
	Table_PreQosPipeRoutingRoutesV4      uint32 = 39015874
	Table_PreQosPipeAclAcls              uint32 = 47204971
	Table_PreQosPipeMyStation            uint32 = 40931612
	Table_PreQosPipeInterfaces           uint32 = 33923840
	Table_PreQosPipeSessionsUplink       uint32 = 44976597
	Table_PreQosPipeSessionsDownlink     uint32 = 34742049
	Table_PreQosPipeTerminationsUplink   uint32 = 37595532
	Table_PreQosPipeTerminationsDownlink uint32 = 34778590
	Table_PreQosPipeApplications         uint32 = 46868458
	Table_PreQosPipeTunnelPeers          uint32 = 49497304
	//Actions
	Action_NoAction                         uint32 = 21257015
	Action_PreQosPipeRoutingDrop            uint32 = 31448256
	Action_PreQosPipeRoutingRoute           uint32 = 23965128
	Action_PreQosPipeAclSetPort             uint32 = 30494847
	Action_PreQosPipeAclPunt                uint32 = 26495283
	Action_PreQosPipeAclCloneToCpu          uint32 = 21596798
	Action_PreQosPipeAclDrop                uint32 = 18812293
	Action_PreQosPipeInitializeMetadata     uint32 = 23766285
	Action_PreQosPipeSetSourceIface         uint32 = 26090030
	Action_PreQosPipeDoDrop                 uint32 = 28401267
	Action_PreQosPipeSetSessionUplink       uint32 = 19461580
	Action_PreQosPipeSetSessionUplinkDrop   uint32 = 22196934
	Action_PreQosPipeSetSessionDownlink     uint32 = 21848329
	Action_PreQosPipeSetSessionDownlinkDrop uint32 = 20229579
	Action_PreQosPipeSetSessionDownlinkBuff uint32 = 20249483
	Action_PreQosPipeUplinkTermFwdNoTc      uint32 = 21760615
	Action_PreQosPipeUplinkTermFwd          uint32 = 28305359
	Action_PreQosPipeUplinkTermDrop         uint32 = 20977365
	Action_PreQosPipeDownlinkTermFwdNoTc    uint32 = 26185804
	Action_PreQosPipeDownlinkTermFwd        uint32 = 32699713
	Action_PreQosPipeDownlinkTermDrop       uint32 = 31264233
	Action_PreQosPipeSetAppId               uint32 = 23010411
	Action_PreQosPipeLoadTunnelParam        uint32 = 32742981
	Action_PreQosPipeDoGtpuTunnel           uint32 = 29247910
	Action_PreQosPipeDoGtpuTunnelWithPsc    uint32 = 31713420
	//IndirectCounters
	Counter_PreQosPipePreQosCounter   uint32 = 315693181
	Counter_PostQosPipePostQosCounter uint32 = 302958180
	//DirectCounters
	DirectCounter_Acls uint32 = 325583051
	//ActionParams
	ActionParam_PreQosPipe_Routing_routeSrcMac                      uint32 = 1
	ActionParam_PreQosPipe_Routing_routeDstMac                      uint32 = 2
	ActionParam_PreQosPipe_Routing_routeEgressPort                  uint32 = 3
	ActionParam_PreQosPipe_Acl_set_portPort                         uint32 = 1
	ActionParam_PreQosPipe_set_source_ifaceSrcIface                 uint32 = 1
	ActionParam_PreQosPipe_set_source_ifaceDirection                uint32 = 2
	ActionParam_PreQosPipe_set_source_ifaceSliceId                  uint32 = 3
	ActionParam_PreQosPipe_set_session_uplinkSessionMeterIdx        uint32 = 1
	ActionParam_PreQosPipe_set_session_downlinkTunnelPeerId         uint32 = 1
	ActionParam_PreQosPipe_set_session_downlinkSessionMeterIdx      uint32 = 2
	ActionParam_PreQosPipe_set_session_downlink_buffSessionMeterIdx uint32 = 1
	ActionParam_PreQosPipe_uplink_term_fwd_no_tcCtrIdx              uint32 = 1
	ActionParam_PreQosPipe_uplink_term_fwd_no_tcAppMeterIdx         uint32 = 2
	ActionParam_PreQosPipe_uplink_term_fwdCtrIdx                    uint32 = 1
	ActionParam_PreQosPipe_uplink_term_fwdTc                        uint32 = 2
	ActionParam_PreQosPipe_uplink_term_fwdAppMeterIdx               uint32 = 3
	ActionParam_PreQosPipe_uplink_term_dropCtrIdx                   uint32 = 1
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcCtrIdx            uint32 = 1
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcTeid              uint32 = 2
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcQfi               uint32 = 3
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcAppMeterIdx       uint32 = 4
	ActionParam_PreQosPipe_downlink_term_fwdCtrIdx                  uint32 = 1
	ActionParam_PreQosPipe_downlink_term_fwdTeid                    uint32 = 2
	ActionParam_PreQosPipe_downlink_term_fwdQfi                     uint32 = 3
	ActionParam_PreQosPipe_downlink_term_fwdTc                      uint32 = 4
	ActionParam_PreQosPipe_downlink_term_fwdAppMeterIdx             uint32 = 5
	ActionParam_PreQosPipe_downlink_term_dropCtrIdx                 uint32 = 1
	ActionParam_PreQosPipe_set_app_idAppId                          uint32 = 1
	ActionParam_PreQosPipe_load_tunnel_paramSrcAddr                 uint32 = 1
	ActionParam_PreQosPipe_load_tunnel_paramDstAddr                 uint32 = 2
	ActionParam_PreQosPipe_load_tunnel_paramSport                   uint32 = 3
	//ActionProfiles
	ActionProfile_hashed_selector uint32 = 297808402
	//PacketMetadata
	PacketMeta_PacketOut uint32 = 75327753
	PacketMeta_PacketIn  uint32 = 80671331
	//MetersMeter_PreQosPipeAppMeter	uint32 = 338231090
	Meter_PreQosPipeSessionMeter uint32 = 347593234
)
