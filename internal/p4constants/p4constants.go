// SPDX-License-Identifier: Apache-2.0
// Copyright 2022-present Open Networking Foundation

package p4constants

const (
	// HeaderFields
	HdrPreQosPipeRoutingRoutesV4DstPrefix      uint32 = 1
	HdrPreQosPipeAclAclsInport                 uint32 = 1
	HdrPreQosPipeAclAclsSrcIface               uint32 = 2
	HdrPreQosPipeAclAclsEthSrc                 uint32 = 3
	HdrPreQosPipeAclAclsEthDst                 uint32 = 4
	HdrPreQosPipeAclAclsEthType                uint32 = 5
	HdrPreQosPipeAclAclsIpv4Src                uint32 = 6
	HdrPreQosPipeAclAclsIpv4Dst                uint32 = 7
	HdrPreQosPipeAclAclsIpv4Proto              uint32 = 8
	HdrPreQosPipeAclAclsL4Sport                uint32 = 9
	HdrPreQosPipeAclAclsL4Dport                uint32 = 10
	HdrPreQosPipeMyStationDstMac               uint32 = 1
	HdrPreQosPipeInterfacesIpv4DstPrefix       uint32 = 1
	HdrPreQosPipeSessionsUplinkN3Address       uint32 = 1
	HdrPreQosPipeSessionsUplinkTeid            uint32 = 2
	HdrPreQosPipeSessionsDownlinkUeAddress     uint32 = 1
	HdrPreQosPipeTerminationsUplinkUeAddress   uint32 = 1
	HdrPreQosPipeTerminationsUplinkAppId       uint32 = 2
	HdrPreQosPipeTerminationsDownlinkUeAddress uint32 = 1
	HdrPreQosPipeTerminationsDownlinkAppId     uint32 = 2
	HdrPreQosPipeApplicationsSliceId           uint32 = 1
	HdrPreQosPipeApplicationsAppIpAddr         uint32 = 2
	HdrPreQosPipeApplicationsAppL4Port         uint32 = 3
	HdrPreQosPipeApplicationsAppIpProto        uint32 = 4
	HdrPreQosPipeTunnelPeersTunnelPeerId       uint32 = 1
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
	ActionPreQosPipeUplinkTermFwd          uint32 = 28305359
	ActionPreQosPipeUplinkTermDrop         uint32 = 20977365
	ActionPreQosPipeDownlinkTermFwd        uint32 = 32699713
	ActionPreQosPipeDownlinkTermDrop       uint32 = 31264233
	ActionPreQosPipeSetAppId               uint32 = 23010411
	ActionPreQosPipeLoadTunnelParam        uint32 = 32742981
	ActionPreQosPipeDoGtpuTunnel           uint32 = 29247910
	ActionPreQosPipeDoGtpuTunnelWithPsc    uint32 = 31713420
	// ActionParams
	ActionParamPreQosPipeRoutingRouteSrcMac                    uint32 = 1
	ActionParamPreQosPipeRoutingRouteDstMac                    uint32 = 2
	ActionParamPreQosPipeRoutingRouteEgressPort                uint32 = 3
	ActionParamPreQosPipeAclSetPortPort                        uint32 = 1
	ActionParamPreQosPipeSetSourceIfaceSrcIface                uint32 = 1
	ActionParamPreQosPipeSetSourceIfaceDirection               uint32 = 2
	ActionParamPreQosPipeSetSourceIfaceSliceId                 uint32 = 3
	ActionParamPreQosPipeSetSessionUplinkSessionMeterIdx       uint32 = 1
	ActionParamPreQosPipeSetSessionDownlinkTunnelPeerId        uint32 = 1
	ActionParamPreQosPipeSetSessionDownlinkSessionMeterIdx     uint32 = 2
	ActionParamPreQosPipeSetSessionDownlinkBuffSessionMeterIdx uint32 = 1
	ActionParamPreQosPipeUplinkTermFwdCtrIdx                   uint32 = 1
	ActionParamPreQosPipeUplinkTermFwdTc                       uint32 = 2
	ActionParamPreQosPipeUplinkTermFwdAppMeterIdx              uint32 = 3
	ActionParamPreQosPipeUplinkTermDropCtrIdx                  uint32 = 1
	ActionParamPreQosPipeDownlinkTermFwdCtrIdx                 uint32 = 1
	ActionParamPreQosPipeDownlinkTermFwdTeid                   uint32 = 2
	ActionParamPreQosPipeDownlinkTermFwdQfi                    uint32 = 3
	ActionParamPreQosPipeDownlinkTermFwdTc                     uint32 = 4
	ActionParamPreQosPipeDownlinkTermFwdAppMeterIdx            uint32 = 5
	ActionParamPreQosPipeDownlinkTermDropCtrIdx                uint32 = 1
	ActionParamPreQosPipeSetAppIdAppId                         uint32 = 1
	ActionParamPreQosPipeLoadTunnelParamSrcAddr                uint32 = 1
	ActionParamPreQosPipeLoadTunnelParamDstAddr                uint32 = 2
	ActionParamPreQosPipeLoadTunnelParamSport                  uint32 = 3
	// IndirectCounters
	CounterPreQosPipePreQosCounter       uint32 = 315693181
	CounterSizePreQosPipePreQosCounter   uint64 = 1024
	CounterPostQosPipePostQosCounter     uint32 = 302958180
	CounterSizePostQosPipePostQosCounter uint64 = 1024
	// DirectCounters
	DirectCounterAcls uint32 = 325583051
	// ActionProfiles
	ActionProfileHashedSelector uint32 = 297808402
	// PacketMetadata
	PacketMetaPacketOut uint32 = 75327753
	PacketMetaPacketIn  uint32 = 80671331
	// Meters
	MeterPreQosPipeAppMeter         uint32 = 338231090
	MeterSizePreQosPipeAppMeter     uint64 = 1024
	MeterPreQosPipeSessionMeter     uint32 = 347593234
	MeterSizePreQosPipeSessionMeter uint64 = 1024
	MeterPreQosPipeSliceTcMeter     uint32 = 336833095
	MeterSizePreQosPipeSliceTcMeter uint64 = 64
	// Enumerators
	EnumDirectionUnknown       uint32 = 0
	EnumDirectionUplink        uint32 = 1
	EnumDirectionDownlink      uint32 = 2
	EnumDirectionOther         uint32 = 3
	EnumInterfaceTypeUnknown   uint32 = 0
	EnumInterfaceTypeAccess    uint32 = 1
	EnumInterfaceTypeCore      uint32 = 2
	EnumTrafficClassBestEffort uint32 = 0
	EnumTrafficClassControl    uint32 = 1
	EnumTrafficClassRealTime   uint32 = 2
	EnumTrafficClassElastic    uint32 = 3
	BitwidthMfAppId            int32  = 8
	BitwidthMfAppIpAddr        int32  = 32
	BitwidthMfAppIpProto       int32  = 8
	BitwidthMfAppL4Port        int32  = 16
	BitwidthMfDstMac           int32  = 48
	BitwidthMfDstPrefix        int32  = 32
	BitwidthMfEthDst           int32  = 48
	BitwidthMfEthSrc           int32  = 48
	BitwidthMfEthType          int32  = 16
	BitwidthMfInport           int32  = 9
	BitwidthMfIpv4Dst          int32  = 32
	BitwidthMfIpv4DstPrefix    int32  = 32
	BitwidthMfIpv4Proto        int32  = 8
	BitwidthMfIpv4Src          int32  = 32
	BitwidthMfL4Dport          int32  = 16
	BitwidthMfL4Sport          int32  = 16
	BitwidthMfN3Address        int32  = 32
	BitwidthMfSliceId          int32  = 4
	BitwidthMfSrcIface         int32  = 8
	BitwidthMfTeid             int32  = 32
	BitwidthMfTunnelPeerId     int32  = 8
	BitwidthMfUeAddress        int32  = 32
	BitwidthApAppId            int32  = 8
	BitwidthApAppMeterIdx      int32  = 32
	BitwidthApCtrIdx           int32  = 32
	BitwidthApDirection        int32  = 8
	BitwidthApDstAddr          int32  = 32
	BitwidthApDstMac           int32  = 48
	BitwidthApEgressPort       int32  = 9
	BitwidthApPort             int32  = 9
	BitwidthApQfi              int32  = 6
	BitwidthApSessionMeterIdx  int32  = 32
	BitwidthApSliceId          int32  = 4
	BitwidthApSport            int32  = 16
	BitwidthApSrcAddr          int32  = 32
	BitwidthApSrcIface         int32  = 8
	BitwidthApSrcMac           int32  = 48
	BitwidthApTc               int32  = 2
	BitwidthApTeid             int32  = 32
	BitwidthApTunnelPeerId     int32  = 8
)

func GetTableIDToNameMap() map[uint32]string {
	return map[uint32]string{
		39015874: "PreQosPipe.Routing.routes_v4",
		47204971: "PreQosPipe.Acl.acls",
		40931612: "PreQosPipe.my_station",
		33923840: "PreQosPipe.interfaces",
		44976597: "PreQosPipe.sessions_uplink",
		34742049: "PreQosPipe.sessions_downlink",
		37595532: "PreQosPipe.terminations_uplink",
		34778590: "PreQosPipe.terminations_downlink",
		46868458: "PreQosPipe.applications",
		49497304: "PreQosPipe.tunnel_peers",
	}
}

func GetTableIDList() []uint32 {
	return []uint32{
		39015874,
		47204971,
		40931612,
		33923840,
		44976597,
		34742049,
		37595532,
		34778590,
		46868458,
		49497304,
	}
}

func GetActionIDToNameMap() map[uint32]string {
	return map[uint32]string{
		21257015: "NoAction",
		31448256: "PreQosPipe.Routing.drop",
		23965128: "PreQosPipe.Routing.route",
		30494847: "PreQosPipe.Acl.set_port",
		26495283: "PreQosPipe.Acl.punt",
		21596798: "PreQosPipe.Acl.clone_to_cpu",
		18812293: "PreQosPipe.Acl.drop",
		23766285: "PreQosPipe._initialize_metadata",
		26090030: "PreQosPipe.set_source_iface",
		28401267: "PreQosPipe.do_drop",
		19461580: "PreQosPipe.set_session_uplink",
		22196934: "PreQosPipe.set_session_uplink_drop",
		21848329: "PreQosPipe.set_session_downlink",
		20229579: "PreQosPipe.set_session_downlink_drop",
		20249483: "PreQosPipe.set_session_downlink_buff",
		28305359: "PreQosPipe.uplink_term_fwd",
		20977365: "PreQosPipe.uplink_term_drop",
		32699713: "PreQosPipe.downlink_term_fwd",
		31264233: "PreQosPipe.downlink_term_drop",
		23010411: "PreQosPipe.set_app_id",
		32742981: "PreQosPipe.load_tunnel_param",
		29247910: "PreQosPipe.do_gtpu_tunnel",
		31713420: "PreQosPipe.do_gtpu_tunnel_with_psc",
	}
}

func GetActionIDList() []uint32 {
	return []uint32{
		21257015,
		31448256,
		23965128,
		30494847,
		26495283,
		21596798,
		18812293,
		23766285,
		26090030,
		28401267,
		19461580,
		22196934,
		21848329,
		20229579,
		20249483,
		28305359,
		20977365,
		32699713,
		31264233,
		23010411,
		32742981,
		29247910,
		31713420,
	}
}

func GetActionProfileIDToNameMap() map[uint32]string {
	return map[uint32]string{
		297808402: "hashed_selector",
	}
}

func GetActionProfileIDList() []uint32 {
	return []uint32{
		297808402,
	}
}

func GetCounterIDToNameMap() map[uint32]string {
	return map[uint32]string{
		315693181: "PreQosPipe.pre_qos_counter",
		302958180: "PostQosPipe.post_qos_counter",
	}
}

func GetCounterIDList() []uint32 {
	return []uint32{
		315693181,
		302958180,
	}
}

func GetDirectCounterIDToNameMap() map[uint32]string {
	return map[uint32]string{
		325583051: "acls",
	}
}

func GetDirectCounterIDList() []uint32 {
	return []uint32{
		325583051,
	}
}

func GetMeterIDToNameMap() map[uint32]string {
	return map[uint32]string{
		338231090: "PreQosPipe.app_meter",
		347593234: "PreQosPipe.session_meter",
		336833095: "PreQosPipe.slice_tc_meter",
	}
}

func GetMeterIDList() []uint32 {
	return []uint32{
		338231090,
		347593234,
		336833095,
	}
}

func GetDirectMeterIDToNameMap() map[uint32]string {
	return map[uint32]string{}
}

func GetDirectMeterIDList() []uint32 {
	return []uint32{}
}

func GetControllerPacketMetadataIDToNameMap() map[uint32]string {
	return map[uint32]string{
		75327753: "packet_out",
		80671331: "packet_in",
	}
}

func GetControllerPacketMetadataIDList() []uint32 {
	return []uint32{
		75327753,
		80671331,
	}
}

func GetRegisterIDToNameMap() map[uint32]string {
	return map[uint32]string{}
}

func GetRegisterIDList() []uint32 {
	return []uint32{}
}
