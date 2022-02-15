package pfcpiface

import (
	"testing"

	//nolint:staticcheck // Ignore SA1019.
	// Upgrading to google.golang.org/protobuf/proto is not a drop-in replacement,
	// as also P4Runtime stubs are based on the deprecated proto.
	"github.com/golang/protobuf/proto"

	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	"github.com/stretchr/testify/require"
)

// FIXME it's better to directly read the p4info in conf/p4/bin
var mockP4INFO = "pkg_info {\n  arch: \"v1model\"\n}\ntables {\n  preamble {\n    id: 39015874\n    name: \"PreQosPipe.Routing.routes_v4\"\n    alias: \"routes_v4\"\n  }\n  match_fields {\n    id: 1\n    name: \"dst_prefix\"\n    bitwidth: 32\n    match_type: LPM\n  }\n  action_refs {\n    id: 23965128\n  }\n  action_refs {\n    id: 21257015\n    annotations: \"@defaultonly\"\n    scope: DEFAULT_ONLY\n  }\n  implementation_id: 297808402\n  size: 1024\n}\ntables {\n  preamble {\n    id: 47204971\n    name: \"PreQosPipe.Acl.acls\"\n    alias: \"Acl.acls\"\n  }\n  match_fields {\n    id: 1\n    name: \"inport\"\n    bitwidth: 9\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 2\n    name: \"src_iface\"\n    bitwidth: 8\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 3\n    name: \"eth_src\"\n    bitwidth: 48\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 4\n    name: \"eth_dst\"\n    bitwidth: 48\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 5\n    name: \"eth_type\"\n    bitwidth: 16\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 6\n    name: \"ipv4_src\"\n    bitwidth: 32\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 7\n    name: \"ipv4_dst\"\n    bitwidth: 32\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 8\n    name: \"ipv4_proto\"\n    bitwidth: 8\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 9\n    name: \"l4_sport\"\n    bitwidth: 16\n    match_type: TERNARY\n  }\n  match_fields {\n    id: 10\n    name: \"l4_dport\"\n    bitwidth: 16\n    match_type: TERNARY\n  }\n  action_refs {\n    id: 30494847\n  }\n  action_refs {\n    id: 26495283\n  }\n  action_refs {\n    id: 21596798\n  }\n  action_refs {\n    id: 18812293\n  }\n  action_refs {\n    id: 21257015\n  }\n  const_default_action_id: 21257015\n  direct_resource_ids: 325583051\n  size: 1024\n}\ntables {\n  preamble {\n    id: 40931612\n    name: \"PreQosPipe.my_station\"\n    alias: \"my_station\"\n  }\n  match_fields {\n    id: 1\n    name: \"dst_mac\"\n    bitwidth: 48\n    match_type: EXACT\n  }\n  action_refs {\n    id: 21257015\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33923840\n    name: \"PreQosPipe.interfaces\"\n    alias: \"interfaces\"\n  }\n  match_fields {\n    id: 1\n    name: \"ipv4_dst_prefix\"\n    bitwidth: 32\n    match_type: LPM\n  }\n  action_refs {\n    id: 26090030\n  }\n  const_default_action_id: 26090030\n  size: 1024\n}\ntables {\n  preamble {\n    id: 44976597\n    name: \"PreQosPipe.sessions_uplink\"\n    alias: \"sessions_uplink\"\n  }\n  match_fields {\n    id: 1\n    name: \"n3_address\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  match_fields {\n    id: 2\n    name: \"teid\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  action_refs {\n    id: 19461580\n  }\n  action_refs {\n    id: 22196934\n  }\n  action_refs {\n    id: 28401267\n    annotations: \"@defaultonly\"\n    scope: DEFAULT_ONLY\n  }\n  const_default_action_id: 28401267\n  size: 1024\n}\ntables {\n  preamble {\n    id: 34742049\n    name: \"PreQosPipe.sessions_downlink\"\n    alias: \"sessions_downlink\"\n  }\n  match_fields {\n    id: 1\n    name: \"ue_address\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  action_refs {\n    id: 21848329\n  }\n  action_refs {\n    id: 20229579\n  }\n  action_refs {\n    id: 20249483\n  }\n  action_refs {\n    id: 28401267\n    annotations: \"@defaultonly\"\n    scope: DEFAULT_ONLY\n  }\n  const_default_action_id: 28401267\n  size: 1024\n}\ntables {\n  preamble {\n    id: 37595532\n    name: \"PreQosPipe.terminations_uplink\"\n    alias: \"terminations_uplink\"\n  }\n  match_fields {\n    id: 1\n    name: \"ue_address\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  match_fields {\n    id: 2\n    name: \"app_id\"\n    bitwidth: 8\n    match_type: EXACT\n  }\n  action_refs {\n    id: 28305359\n  }\n  action_refs {\n    id: 21760615\n  }\n  action_refs {\n    id: 20977365\n  }\n  action_refs {\n    id: 28401267\n    annotations: \"@defaultonly\"\n    scope: DEFAULT_ONLY\n  }\n  const_default_action_id: 28401267\n  size: 1024\n}\ntables {\n  preamble {\n    id: 34778590\n    name: \"PreQosPipe.terminations_downlink\"\n    alias: \"terminations_downlink\"\n  }\n  match_fields {\n    id: 1\n    name: \"ue_address\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  match_fields {\n    id: 2\n    name: \"app_id\"\n    bitwidth: 8\n    match_type: EXACT\n  }\n  action_refs {\n    id: 32699713\n  }\n  action_refs {\n    id: 31264233\n  }\n  action_refs {\n    id: 26185804\n  }\n  action_refs {\n    id: 28401267\n    annotations: \"@defaultonly\"\n    scope: DEFAULT_ONLY\n  }\n  const_default_action_id: 28401267\n  size: 1024\n}\ntables {\n  preamble {\n    id: 46868458\n    name: \"PreQosPipe.applications\"\n    alias: \"applications\"\n  }\n  match_fields {\n    id: 1\n    name: \"app_ip_addr\"\n    bitwidth: 32\n    match_type: LPM\n  }\n  match_fields {\n    id: 2\n    name: \"app_l4_port\"\n    bitwidth: 16\n    match_type: RANGE\n  }\n  match_fields {\n    id: 3\n    name: \"app_ip_proto\"\n    bitwidth: 8\n    match_type: TERNARY\n  }\n  action_refs {\n    id: 23010411\n  }\n  const_default_action_id: 23010411\n  size: 1024\n}\ntables {\n  preamble {\n    id: 49497304\n    name: \"PreQosPipe.tunnel_peers\"\n    alias: \"tunnel_peers\"\n  }\n  match_fields {\n    id: 1\n    name: \"tunnel_peer_id\"\n    bitwidth: 8\n    match_type: EXACT\n  }\n  action_refs {\n    id: 32742981\n  }\n  action_refs {\n    id: 21257015\n    annotations: \"@defaultonly\"\n    scope: DEFAULT_ONLY\n  }\n  size: 1024\n}\nactions {\n  preamble {\n    id: 21257015\n    name: \"NoAction\"\n    alias: \"NoAction\"\n    annotations: \"@noWarn(\\\"unused\\\")\"\n  }\n}\nactions {\n  preamble {\n    id: 31448256\n    name: \"PreQosPipe.Routing.drop\"\n    alias: \"Routing.drop\"\n  }\n}\nactions {\n  preamble {\n    id: 23965128\n    name: \"PreQosPipe.Routing.route\"\n    alias: \"route\"\n  }\n  params {\n    id: 1\n    name: \"src_mac\"\n    bitwidth: 48\n  }\n  params {\n    id: 2\n    name: \"dst_mac\"\n    bitwidth: 48\n  }\n  params {\n    id: 3\n    name: \"egress_port\"\n    bitwidth: 9\n  }\n}\nactions {\n  preamble {\n    id: 30494847\n    name: \"PreQosPipe.Acl.set_port\"\n    alias: \"set_port\"\n  }\n  params {\n    id: 1\n    name: \"port\"\n    bitwidth: 9\n  }\n}\nactions {\n  preamble {\n    id: 26495283\n    name: \"PreQosPipe.Acl.punt\"\n    alias: \"punt\"\n  }\n}\nactions {\n  preamble {\n    id: 21596798\n    name: \"PreQosPipe.Acl.clone_to_cpu\"\n    alias: \"clone_to_cpu\"\n  }\n}\nactions {\n  preamble {\n    id: 18812293\n    name: \"PreQosPipe.Acl.drop\"\n    alias: \"Acl.drop\"\n  }\n}\nactions {\n  preamble {\n    id: 26090030\n    name: \"PreQosPipe.set_source_iface\"\n    alias: \"set_source_iface\"\n  }\n  params {\n    id: 1\n    name: \"src_iface\"\n    bitwidth: 8\n  }\n  params {\n    id: 2\n    name: \"direction\"\n    bitwidth: 8\n  }\n  params {\n    id: 3\n    name: \"slice_id\"\n    bitwidth: 4\n  }\n}\nactions {\n  preamble {\n    id: 28401267\n    name: \"PreQosPipe.do_drop\"\n    alias: \"do_drop\"\n  }\n}\nactions {\n  preamble {\n    id: 19461580\n    name: \"PreQosPipe.set_session_uplink\"\n    alias: \"set_session_uplink\"\n  }\n}\nactions {\n  preamble {\n    id: 22196934\n    name: \"PreQosPipe.set_session_uplink_drop\"\n    alias: \"set_session_uplink_drop\"\n  }\n}\nactions {\n  preamble {\n    id: 21848329\n    name: \"PreQosPipe.set_session_downlink\"\n    alias: \"set_session_downlink\"\n  }\n  params {\n    id: 1\n    name: \"tunnel_peer_id\"\n    bitwidth: 8\n  }\n}\nactions {\n  preamble {\n    id: 20229579\n    name: \"PreQosPipe.set_session_downlink_drop\"\n    alias: \"set_session_downlink_drop\"\n  }\n}\nactions {\n  preamble {\n    id: 20249483\n    name: \"PreQosPipe.set_session_downlink_buff\"\n    alias: \"set_session_downlink_buff\"\n  }\n}\nactions {\n  preamble {\n    id: 28305359\n    name: \"PreQosPipe.uplink_term_fwd\"\n    alias: \"uplink_term_fwd\"\n  }\n  params {\n    id: 1\n    name: \"ctr_idx\"\n    bitwidth: 32\n  }\n  params {\n    id: 2\n    name: \"tc\"\n    bitwidth: 2\n  }\n}\nactions {\n  preamble {\n    id: 21760615\n    name: \"PreQosPipe.uplink_term_fwd_no_tc\"\n    alias: \"uplink_term_fwd_no_tc\"\n  }\n  params {\n    id: 1\n    name: \"ctr_idx\"\n    bitwidth: 32\n  }\n}\nactions {\n  preamble {\n    id: 20977365\n    name: \"PreQosPipe.uplink_term_drop\"\n    alias: \"uplink_term_drop\"\n  }\n  params {\n    id: 1\n    name: \"ctr_idx\"\n    bitwidth: 32\n  }\n}\nactions {\n  preamble {\n    id: 32699713\n    name: \"PreQosPipe.downlink_term_fwd\"\n    alias: \"downlink_term_fwd\"\n  }\n  params {\n    id: 1\n    name: \"ctr_idx\"\n    bitwidth: 32\n  }\n  params {\n    id: 2\n    name: \"teid\"\n    bitwidth: 32\n  }\n  params {\n    id: 3\n    name: \"qfi\"\n    bitwidth: 6\n  }\n  params {\n    id: 4\n    name: \"tc\"\n    bitwidth: 2\n  }\n}\nactions {\n  preamble {\n    id: 26185804\n    name: \"PreQosPipe.downlink_term_fwd_no_tc\"\n    alias: \"downlink_term_fwd_no_tc\"\n  }\n  params {\n    id: 1\n    name: \"ctr_idx\"\n    bitwidth: 32\n  }\n  params {\n    id: 2\n    name: \"teid\"\n    bitwidth: 32\n  }\n  params {\n    id: 3\n    name: \"qfi\"\n    bitwidth: 6\n  }\n}\nactions {\n  preamble {\n    id: 31264233\n    name: \"PreQosPipe.downlink_term_drop\"\n    alias: \"downlink_term_drop\"\n  }\n  params {\n    id: 1\n    name: \"ctr_idx\"\n    bitwidth: 32\n  }\n}\nactions {\n  preamble {\n    id: 23010411\n    name: \"PreQosPipe.set_app_id\"\n    alias: \"set_app_id\"\n  }\n  params {\n    id: 1\n    name: \"app_id\"\n    bitwidth: 8\n  }\n}\nactions {\n  preamble {\n    id: 32742981\n    name: \"PreQosPipe.load_tunnel_param\"\n    alias: \"load_tunnel_param\"\n  }\n  params {\n    id: 1\n    name: \"src_addr\"\n    bitwidth: 32\n  }\n  params {\n    id: 2\n    name: \"dst_addr\"\n    bitwidth: 32\n  }\n  params {\n    id: 3\n    name: \"sport\"\n    bitwidth: 16\n  }\n}\nactions {\n  preamble {\n    id: 29247910\n    name: \"PreQosPipe.do_gtpu_tunnel\"\n    alias: \"do_gtpu_tunnel\"\n  }\n}\nactions {\n  preamble {\n    id: 31713420\n    name: \"PreQosPipe.do_gtpu_tunnel_with_psc\"\n    alias: \"do_gtpu_tunnel_with_psc\"\n  }\n}\naction_profiles {\n  preamble {\n    id: 297808402\n    name: \"hashed_selector\"\n    alias: \"hashed_selector\"\n  }\n  table_ids: 39015874\n  with_selector: true\n  size: 1024\n}\ncounters {\n  preamble {\n    id: 315693181\n    name: \"PreQosPipe.pre_qos_counter\"\n    alias: \"pre_qos_counter\"\n  }\n  spec {\n    unit: BOTH\n  }\n  size: 1024\n}\ncounters {\n  preamble {\n    id: 302958180\n    name: \"PostQosPipe.post_qos_counter\"\n    alias: \"post_qos_counter\"\n  }\n  spec {\n    unit: BOTH\n  }\n  size: 1024\n}\ndirect_counters {\n  preamble {\n    id: 325583051\n    name: \"acls\"\n    alias: \"acls\"\n  }\n  spec {\n    unit: BOTH\n  }\n  direct_table_id: 47204971\n}\ncontroller_packet_metadata {\n  preamble {\n    id: 75327753\n    name: \"packet_out\"\n    alias: \"packet_out\"\n    annotations: \"@controller_header(\\\"packet_out\\\")\"\n  }\n  metadata {\n    id: 1\n    name: \"reserved\"\n    bitwidth: 8\n  }\n}\ncontroller_packet_metadata {\n  preamble {\n    id: 80671331\n    name: \"packet_in\"\n    alias: \"packet_in\"\n    annotations: \"@controller_header(\\\"packet_in\\\")\"\n  }\n  metadata {\n    id: 1\n    name: \"ingress_port\"\n    bitwidth: 9\n  }\n  metadata {\n    id: 2\n    name: \"_pad\"\n    bitwidth: 7\n  }\n}\ndigests {\n  preamble {\n    id: 396224266\n    name: \"ddn_digest_t\"\n    alias: \"ddn_digest_t\"\n  }\n  type_spec {\n    struct {\n      name: \"ddn_digest_t\"\n    }\n  }\n}\ntype_info {\n  structs {\n    key: \"ddn_digest_t\"\n    value {\n      members {\n        name: \"ue_address\"\n        type_spec {\n          bitstring {\n            bit {\n              bitwidth: 32\n            }\n          }\n        }\n      }\n    }\n  }\n  serializable_enums {\n    key: \"Direction\"\n    value {\n      underlying_type {\n        bitwidth: 8\n      }\n      members {\n        name: \"UNKNOWN\"\n        value: \"\\000\"\n      }\n      members {\n        name: \"UPLINK\"\n        value: \"\\001\"\n      }\n      members {\n        name: \"DOWNLINK\"\n        value: \"\\002\"\n      }\n      members {\n        name: \"OTHER\"\n        value: \"\\003\"\n      }\n    }\n  }\n  serializable_enums {\n    key: \"InterfaceType\"\n    value {\n      underlying_type {\n        bitwidth: 8\n      }\n      members {\n        name: \"UNKNOWN\"\n        value: \"\\000\"\n      }\n      members {\n        name: \"ACCESS\"\n        value: \"\\001\"\n      }\n      members {\n        name: \"CORE\"\n        value: \"\\002\"\n      }\n    }\n  }\n}\n"

// secondMockP4INFO from https://github.com/p4lang/PI/blob/main/proto/demo_grpc/simple_router.p4info.txt
var secondMockP4INFO = "tables {\n  preamble {\n    id: 33586128\n    name: \"decap_cpu_header\"\n    alias: \"decap_cpu_header\"\n  }\n  action_refs {\n    id: 16788917\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33589124\n    name: \"forward\"\n    alias: \"forward\"\n  }\n  match_fields {\n    id: 1\n    name: \"routing_metadata.nhop_ipv4\"\n    bitwidth: 32\n    match_type: EXACT\n  }\n  action_refs {\n    id: 16780303\n  }\n  action_refs {\n    id: 16840314\n  }\n  action_refs {\n    id: 16784184\n  }\n  size: 512\n}\ntables {\n  preamble {\n    id: 33581985\n    name: \"ipv4_lpm\"\n    alias: \"ipv4_lpm\"\n  }\n  match_fields {\n    id: 1\n    name: \"ipv4.dstAddr\"\n    bitwidth: 32\n    match_type: LPM\n  }\n  action_refs {\n    id: 16812204\n  }\n  action_refs {\n    id: 16784184\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33555613\n    name: \"send_arp_to_cpu\"\n    alias: \"send_arp_to_cpu\"\n  }\n  action_refs {\n    id: 16840314\n  }\n  size: 1024\n}\ntables {\n  preamble {\n    id: 33562826\n    name: \"send_frame\"\n    alias: \"send_frame\"\n  }\n  match_fields {\n    id: 1\n    name: \"standard_metadata.egress_port\"\n    bitwidth: 9\n    match_type: EXACT\n  }\n  action_refs {\n    id: 16813016\n  }\n  action_refs {\n    id: 16784184\n  }\n  size: 256\n}\nactions {\n  preamble {\n    id: 16788917\n    name: \"do_decap_cpu_header\"\n    alias: \"do_decap_cpu_header\"\n  }\n}\nactions {\n  preamble {\n    id: 16780303\n    name: \"set_dmac\"\n    alias: \"set_dmac\"\n  }\n  params {\n    id: 1\n    name: \"dmac\"\n    bitwidth: 48\n  }\n}\nactions {\n  preamble {\n    id: 16840314\n    name: \"do_send_to_cpu\"\n    alias: \"do_send_to_cpu\"\n  }\n  params {\n    id: 1\n    name: \"reason\"\n    bitwidth: 16\n  }\n  params {\n    id: 2\n    name: \"cpu_port\"\n    bitwidth: 9\n  }\n}\nactions {\n  preamble {\n    id: 16784184\n    name: \"_drop\"\n    alias: \"_drop\"\n  }\n}\nactions {\n  preamble {\n    id: 16812204\n    name: \"set_nhop\"\n    alias: \"set_nhop\"\n  }\n  params {\n    id: 1\n    name: \"nhop_ipv4\"\n    bitwidth: 32\n  }\n  params {\n    id: 2\n    name: \"port\"\n    bitwidth: 9\n  }\n}\nactions {\n  preamble {\n    id: 16813016\n    name: \"rewrite_mac\"\n    alias: \"rewrite_mac\"\n  }\n  params {\n    id: 1\n    name: \"smac\"\n    bitwidth: 48\n  }\n}"

func setupNewTranslator(p4info string) *P4rtTranslator {
	var p4Config p4ConfigV1.P4Info

	_ = proto.UnmarshalText(p4info, &p4Config)

	return newP4RtTranslator(p4Config)
}

func Test_actionID(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		translator *P4rtTranslator
		want       uint32
	}{
		{name: "get NoAction",
			args:       "NoAction",
			translator: setupNewTranslator(mockP4INFO),
			want:       uint32(21257015),
		},
		{name: "get rewrite_mac action",
			args:       "rewrite_mac",
			translator: setupNewTranslator(secondMockP4INFO),
			want:       uint32(16813016),
		},
		{name: "non existing action",
			args:       "qwerty",
			translator: setupNewTranslator(mockP4INFO),
			want:       uint32(0),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := tt.translator.actionID(tt.args)
				require.Equal(t, tt.want, got)
			},
		)
	}
}

func Test_tableID(t *testing.T) {
	tests := []struct {
		name       string
		args       string
		translator *P4rtTranslator
		want       uint32
	}{
		{name: "Existing table",
			args:       "PreQosPipe.Routing.routes_v4",
			translator: setupNewTranslator(mockP4INFO),
			want:       uint32(39015874),
		},
		{name: "Existing table in second mock P4 Info",
			args:       "forward",
			translator: setupNewTranslator(secondMockP4INFO),
			want:       uint32(33589124),
		},
		{name: "Non existing table",
			args:       "testtttt",
			translator: setupNewTranslator(secondMockP4INFO),
			want:       uint32(0),
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := tt.translator.tableID(tt.args)
				require.Equal(t, tt.want, got)
			},
		)
	}
}

func Test_getCounterByName(t *testing.T) {
	type args struct {
		counterName string
		counterID   uint32
	}

	tests := []struct {
		name       string
		translator *P4rtTranslator
		want       *args
		wantErr    bool
	}{
		{name: "Existing counter",
			translator: setupNewTranslator(mockP4INFO),
			want: &args{
				counterName: "PreQosPipe.pre_qos_counter",
				counterID:   uint32(315693181),
			},
			wantErr: false,
		},
		{name: "Existing counter",
			translator: setupNewTranslator(mockP4INFO),
			want: &args{
				counterName: "PostQosPipe.post_qos_counter",
				counterID:   uint32(302958180),
			},
			wantErr: false,
		},
		{name: "Non existing counter",
			want: &args{
				counterName: "testttt",
				counterID:   0,
			},
			translator: setupNewTranslator(mockP4INFO),
			wantErr:    true,
		},
		{name: "Non existing counter",
			want: &args{
				counterName: "testtt",
				counterID:   0,
			},
			translator: setupNewTranslator(secondMockP4INFO),
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.translator.getCounterByName(tt.want.counterName)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.Equal(t, tt.want.counterID, got.GetPreamble().GetId())
					require.Equal(t, tt.want.counterName, got.GetPreamble().Name)
				}
			},
		)
	}
}

func Test_getTableByID(t *testing.T) {
	type args struct {
		tableID   uint32
		tableName string
	}

	tests := []struct {
		name       string
		translator *P4rtTranslator
		want       *args
		wantErr    bool
	}{
		{name: "Existing table",
			translator: setupNewTranslator(mockP4INFO),
			want: &args{
				tableID:   39015874,
				tableName: "PreQosPipe.Routing.routes_v4",
			},
			wantErr: false,
		},
		{name: "Existing table",
			translator: setupNewTranslator(secondMockP4INFO),
			want: &args{
				tableID:   33589124,
				tableName: "forward",
			},
			wantErr: false,
		},
		{name: "non existing table",
			translator: setupNewTranslator(mockP4INFO),
			want: &args{
				tableID: 999999,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.translator.getTableByID(tt.want.tableID)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.Equal(t, tt.want.tableID, got.GetPreamble().GetId())
					require.Equal(t, tt.want.tableName, got.GetPreamble().GetName())
				}
			},
		)
	}
}

func Test_getMatchFieldByName(t *testing.T) {
	ts := setupNewTranslator(mockP4INFO)

	mockTable, err := ts.getTableByID(47204971) //acls table
	require.NoError(t, err)

	type args struct {
		table          *p4ConfigV1.Table
		matchFieldName string
	}

	type want struct {
		name  string
		id    uint32
		match p4ConfigV1.MatchField_MatchType
	}

	tests := []struct {
		name    string
		args    *args
		want    *want
		wantErr bool
	}{
		{name: "Existing match Field",
			args: &args{
				table:          mockTable,
				matchFieldName: "inport",
			},
			want: &want{
				name:  "inport",
				id:    1,
				match: p4ConfigV1.MatchField_TERNARY,
			},
		},
		{name: "Existing match Field",
			args: &args{
				table:          mockTable,
				matchFieldName: "ipv4_dst",
			},
			want: &want{
				name:  "ipv4_dst",
				id:    7,
				match: p4ConfigV1.MatchField_TERNARY,
			},
		},
		{name: "non existing match field",
			args: &args{
				table:          mockTable,
				matchFieldName: "test",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := ts.getMatchFieldByName(tt.args.table, tt.args.matchFieldName)
				if tt.wantErr {
					require.Nil(t, got)
					return
				}
				require.Equal(t, tt.want.name, got.GetName())
				require.Equal(t, tt.want.id, got.GetId())
				require.Equal(t, tt.want.match, got.GetMatchType())
			},
		)
	}
}

func Test_getActionbyID(t *testing.T) {
	type want struct {
		actionId   uint32
		actionName string
	}

	tests := []struct {
		name       string
		args       uint32
		translator *P4rtTranslator
		want       *want
		wantErr    bool
	}{
		{name: "get existing action",
			args:       32742981,
			translator: setupNewTranslator(mockP4INFO),
			want: &want{actionName: "PreQosPipe.load_tunnel_param",
				actionId: 32742981,
			},
		},
		{name: "non existing action",
			args:       999999,
			translator: setupNewTranslator(mockP4INFO),
			want:       nil,
			wantErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got, err := tt.translator.getActionByID(tt.args)
				if tt.wantErr {
					require.Error(t, err)
					return
				}
				require.Equal(t, tt.want.actionName, got.GetPreamble().GetName())
				require.Equal(t, tt.want.actionId, got.GetPreamble().GetId())
			},
		)
	}
}

func Test_getActionParamByName(t *testing.T) {
	ts := setupNewTranslator(mockP4INFO)

	mockAction, err := ts.getActionByID(32742981) //PreQosPipe.load_tunnel_param
	require.NoError(t, err)

	type args struct {
		action          *p4ConfigV1.Action
		actionParamName string
	}

	type want struct {
		paramName string
		paramId   uint32
		bitwidth  int32
	}

	tests := []struct {
		name    string
		args    *args
		want    *want
		wantErr bool
	}{
		{name: "Existing action param name",
			args: &args{
				action:          mockAction,
				actionParamName: "src_addr",
			},
			want: &want{paramName: "src_addr",
				paramId:  1,
				bitwidth: 32,
			},
		},
		{name: "non existing action param name",
			args: &args{
				action:          mockAction,
				actionParamName: "test",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(
			tt.name, func(t *testing.T) {
				got := ts.getActionParamByName(tt.args.action, tt.args.actionParamName)
				if tt.wantErr {
					require.Nil(t, got)
					return
				}
				require.Equal(t, tt.want.paramName, got.GetName())
				require.Equal(t, tt.want.paramId, got.GetId())
				require.Equal(t, tt.want.bitwidth, got.GetBitwidth())
			},
		)
	}
}

func Test_(t *testing.T) {

}
