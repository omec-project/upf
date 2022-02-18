/*
* Copyright 2022-present Open Networking Foundation
*
* SPDX-License-Identifier: Apache-2_0
 */
package p4constants

//noinspection GoSnakeCaseUsage
const (

	//HeaderFields
	Hdr_PreQosPipe_Routing_routes_v4DstPrefixuint32     = 1
	Hdr_PreQosPipe_Acl_aclsInportuint32                 = 1
	Hdr_PreQosPipe_Acl_aclsSrcIfaceuint32               = 2
	Hdr_PreQosPipe_Acl_aclsEthSrcuint32                 = 3
	Hdr_PreQosPipe_Acl_aclsEthDstuint32                 = 4
	Hdr_PreQosPipe_Acl_aclsEthTypeuint32                = 5
	Hdr_PreQosPipe_Acl_aclsIpv4Srcuint32                = 6
	Hdr_PreQosPipe_Acl_aclsIpv4Dstuint32                = 7
	Hdr_PreQosPipe_Acl_aclsIpv4Protouint32              = 8
	Hdr_PreQosPipe_Acl_aclsL4Sportuint32                = 9
	Hdr_PreQosPipe_Acl_aclsL4Dportuint32                = 10
	Hdr_PreQosPipe_my_stationDstMacuint32               = 1
	Hdr_PreQosPipe_interfacesIpv4DstPrefixuint32        = 1
	Hdr_PreQosPipe_sessions_uplinkN3Addressuint32       = 1
	Hdr_PreQosPipe_sessions_uplinkTeiduint32            = 2
	Hdr_PreQosPipe_sessions_downlinkUeAddressuint32     = 1
	Hdr_PreQosPipe_terminations_uplinkUeAddressuint32   = 1
	Hdr_PreQosPipe_terminations_uplinkAppIduint32       = 2
	Hdr_PreQosPipe_terminations_downlinkUeAddressuint32 = 1
	Hdr_PreQosPipe_terminations_downlinkAppIduint32     = 2
	Hdr_PreQosPipe_applicationsAppIpAddruint32          = 1
	Hdr_PreQosPipe_applicationsAppL4Portuint32          = 2
	Hdr_PreQosPipe_applicationsAppIpProtouint32         = 3
	Hdr_PreQosPipe_tunnel_peersTunnelPeerIduint32       = 1
	//Tables
	Table_PreQosPipeRoutingRoutesV4uint32      = 39015874
	Table_PreQosPipeAclAclsuint32              = 47204971
	Table_PreQosPipeMyStationuint32            = 40931612
	Table_PreQosPipeInterfacesuint32           = 33923840
	Table_PreQosPipeSessionsUplinkuint32       = 44976597
	Table_PreQosPipeSessionsDownlinkuint32     = 34742049
	Table_PreQosPipeTerminationsUplinkuint32   = 37595532
	Table_PreQosPipeTerminationsDownlinkuint32 = 34778590
	Table_PreQosPipeApplicationsuint32         = 46868458
	Table_PreQosPipeTunnelPeersuint32          = 49497304
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
	Counter_PreQosPipePreQosCounteruint32   = 315693181
	Counter_PostQosPipePostQosCounteruint32 = 302958180
	//DirectCounters
	DirectCounter_Aclsuint32 = 325583051
	//ActionParams
	ActionParam_PreQosPipe_Routing_routeSrcMacuint32                      = 1
	ActionParam_PreQosPipe_Routing_routeDstMacuint32                      = 2
	ActionParam_PreQosPipe_Routing_routeEgressPortuint32                  = 3
	ActionParam_PreQosPipe_Acl_set_portPortuint32                         = 1
	ActionParam_PreQosPipe_set_source_ifaceSrcIfaceuint32                 = 1
	ActionParam_PreQosPipe_set_source_ifaceDirectionuint32                = 2
	ActionParam_PreQosPipe_set_source_ifaceSliceIduint32                  = 3
	ActionParam_PreQosPipe_set_session_uplinkSessionMeterIdxuint32        = 1
	ActionParam_PreQosPipe_set_session_downlinkTunnelPeerIduint32         = 1
	ActionParam_PreQosPipe_set_session_downlinkSessionMeterIdxuint32      = 2
	ActionParam_PreQosPipe_set_session_downlink_buffSessionMeterIdxuint32 = 1
	ActionParam_PreQosPipe_uplink_term_fwd_no_tcCtrIdxuint32              = 1
	ActionParam_PreQosPipe_uplink_term_fwd_no_tcAppMeterIdxuint32         = 2
	ActionParam_PreQosPipe_uplink_term_fwdCtrIdxuint32                    = 1
	ActionParam_PreQosPipe_uplink_term_fwdTcuint32                        = 2
	ActionParam_PreQosPipe_uplink_term_fwdAppMeterIdxuint32               = 3
	ActionParam_PreQosPipe_uplink_term_dropCtrIdxuint32                   = 1
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcCtrIdxuint32            = 1
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcTeiduint32              = 2
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcQfiuint32               = 3
	ActionParam_PreQosPipe_downlink_term_fwd_no_tcAppMeterIdxuint32       = 4
	ActionParam_PreQosPipe_downlink_term_fwdCtrIdxuint32                  = 1
	ActionParam_PreQosPipe_downlink_term_fwdTeiduint32                    = 2
	ActionParam_PreQosPipe_downlink_term_fwdQfiuint32                     = 3
	ActionParam_PreQosPipe_downlink_term_fwdTcuint32                      = 4
	ActionParam_PreQosPipe_downlink_term_fwdAppMeterIdxuint32             = 5
	ActionParam_PreQosPipe_downlink_term_dropCtrIdxuint32                 = 1
	ActionParam_PreQosPipe_set_app_idAppIduint32                          = 1
	ActionParam_PreQosPipe_load_tunnel_paramSrcAddruint32                 = 1
	ActionParam_PreQosPipe_load_tunnel_paramDstAddruint32                 = 2
	ActionParam_PreQosPipe_load_tunnel_paramSportuint32                   = 3
	//ActionProfiles
	ActionProfile_hashed_selectoruint32 = 297808402
	//PacketMetadata
	PacketMeta_PacketOutuint32 = 75327753
	PacketMeta_PacketInuint32  = 80671331
	//MetersMeter_PreQosPipeAppMeteruint32 = 338231090
	Meter_PreQosPipeSessionMeteruint32 = 347593234
)
