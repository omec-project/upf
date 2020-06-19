package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"time"

	"github.com/golang/protobuf/ptypes"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

var (
	bess = flag.String("bess", "localhost:10514", "BESS IP/port combo")
)

const CORE, ACCESS = 0x2, 0x1
const UPLINK, DOWNLINK = 0x0, 0x1
const UDP_GTPU_PORT = 2152
const FARForwardAction, FARTunnelAction = 0x0, 0x1

type pdr struct {
	eNBIP    uint32
	teid     uint32
	ueAddr   uint32
	inetIP   uint32
	uePort   uint16
	inetPort uint16
	proto    uint8
}

type Conf struct {
	Mode             string      `json:"mode"`
	UECidr           string      `json:"ue_cidr"`
	EnbCidr          string      `json:"enb_cidr"`
	MaxSessions      uint32      `json:"max_sessions"`
	MaxIPDefragFlows uint32      `json:"max_ip_defrag_flows"`
	IPFragWithEthMtu uint32      `json:"ip_frag_with_eth_mtu"`
	Measure          bool        `json:"measure"`
	S1UIntf          Interface   `json:"s1u"`
	SGIIntf          Interface   `json:"sgi"`
	Workers          uint32      `json:"workers"`
	Cpiface          CpifaceType `json:"cpiface"`
}

type Interface struct {
	Name string `json:"ifname"`
	Nat  string `json:"ip_masquerade"`
}

type CpifaceType struct {
	N3_IP         string `json:"s1u_sgw_ip"`
	Northbound_IP string `json:"zmqd_nb_ip"`
	Local_IP      string `json:"zmqd_ip"`
	Hostname      string `json:"hostname"`
}

func inet_aton(ip string) (ip_int uint32) {
	ip_byte := net.ParseIP(ip).To4()
	for i := 0; i < len(ip_byte); i++ {
		ip_int |= uint32(ip_byte[i])
		if i < 3 {
			ip_int <<= 8
		}
	}
	return
}

func ParseJson(conf *Conf) {
	/* Open up file */
	jsonFile, err := os.Open("/tmp/upf.json")

	if err != nil {
		log.Println("Error opening file: ", err)
		return
	}
	defer jsonFile.Close()

	/* read our opened file */
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Println("Error reading file: ", err)
		return
	}

	json.Unmarshal(byteValue, conf)
}

func addPDR(ctx context.Context, c pb.BESSControlClient, session_id uint32) {

	const ueip, teid = 0x10000001, 0xf0000000

	f := &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: CORE}},                      /* src_iface */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* tunnel_ipv4_dst */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* enb_teid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(ueip + session_id)}}, /* ueaddr ip*/
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* inet ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* ue port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* inet port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* proto id */
		},
		Masks: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0xFF}},       /* src_iface-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* tunnel_ipv4_dst-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* enb_teid-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0xFFFFFFFF}}, /* ueaddr ip-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* inet ip-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* ue port-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* inet port-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* proto id-mask */
		},
		Valuesv: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* pdr-id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + session_id)}}, /* fseid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(session_id)}},        /* ctr_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(DOWNLINK)}},          /* far_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* need_decap */
		},
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PDRLookup",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}

	f = &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: ACCESS}},                    /* src_iface */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* tunnel_ipv4_dst */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + session_id)}}, /* enb_teid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0}},                         /* inet ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(ueip + session_id)}}, /* ueaddr ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* inet port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* ue port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* proto-id */
		},
		Masks: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0xFF}},       /* src_iface-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* tunnel_ipv4_dst-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0xFFFFFFFF}}, /* enb_teid-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* inet ip-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0xFFFFFFFF}}, /* ueaddr ip-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* inet port-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* ue port-mask */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},        /* proto-id-mask */
		},
		Valuesv: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                       /* pdr_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + session_id)}}, /* fseid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(session_id)}},        /* ctr_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(UPLINK)}},            /* far_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x1}},                       /* need_decap */
		},
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err = c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PDRLookup",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}
}

func addFAR(ctx context.Context, c pb.BESSControlClient, s1uip uint32, session_id uint32) {

	const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	const NG4T_MAX_UE_RAN, NG4T_MAX_ENB_RAN = 500000, 80
	// NG4T-based formula to calculate enodeB IP address against a given UE IP address
	// il_trafficgen also uses the same scheme
	// See SimuCPEnbv4Teid(...) in ngic code for more details
	ue_of_ran := session_id % NG4T_MAX_UE_RAN
	ran := session_id / NG4T_MAX_UE_RAN
	enb_of_ran := ue_of_ran % NG4T_MAX_ENB_RAN
	enb_idx := ran*NG4T_MAX_ENB_RAN + enb_of_ran

	f := &pb.ExactMatchCommandAddArg{
		Gate: 1,
		Fields: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(DOWNLINK)}},          /* far_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + session_id)}}, /* fseid */
		},
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: FARTunnelAction}},           /* action */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x1}},                       /* tunnel_out_type */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(s1uip)}},             /* s1u-ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(enbip + enb_idx)}},   /* enb ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + session_id)}}, /* enb teid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(UDP_GTPU_PORT)}},     /* udp gtpu port */
		},
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "FARLookup",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}

	f = &pb.ExactMatchCommandAddArg{
		Gate: 1,
		Fields: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(UPLINK)}},            /* far id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + session_id)}}, /* fseid */
		},
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: FARForwardAction}}, /* action */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},              /* tunnel_out_type */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},              /* not needed */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},              /* not needed */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},              /* not needed */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},              /* not needed */
		},
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err = c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "FARLookup",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}
}

func addCounters(ctx context.Context, c pb.BESSControlClient, session_id uint32) {

	f := &pb.CounterAddArg{
		CtrId: session_id,
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PreQoSCounter",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}

	f = &pb.CounterAddArg{
		CtrId: session_id,
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err = c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PostDLQoSCounter",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}

	f = &pb.CounterAddArg{
		CtrId: session_id,
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	res, err = c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PostULQoSCounter",
		Cmd:  "add",
		Arg:  any,
	})
	if err != nil {
		log.Println("Error running module command:", err)
		return
	}
	if res.GetError() != nil {
		log.Println("Error updating rule:", res.GetError())
		return
	}
}

func main() {
	var conf Conf
	var s1u_sgw_ip uint32

	// cmdline args
	flag.Parse()

	// read and parse json startup file
	ParseJson(&conf)
	log.Println("Conf Details")
	log.Println("Mode: " + conf.Mode)
	log.Println("UE Cidr: " + conf.UECidr)
	log.Println("Enb Cidr: " + conf.EnbCidr)
	log.Println("Max Sessions: ", conf.MaxSessions)
	log.Println("Max IP Defrag Sessions: ", conf.MaxIPDefragFlows)
	log.Println("IP Frag MTU Size: ", conf.IPFragWithEthMtu)
	log.Println("Measure: ", conf.Measure)
	log.Println("S1U Intf Name: " + conf.S1UIntf.Name)
	log.Println("S1U NAT Settings: " + conf.S1UIntf.Nat)
	log.Println("SGI Intf Name: " + conf.SGIIntf.Name)
	log.Println("SGI NAT Settings: " + conf.SGIIntf.Nat)
	log.Println("Workers: ", conf.Workers)
	log.Println("CPIface N3: " + conf.Cpiface.N3_IP)
	log.Println("CPIface zmqd_nb_ip: " + conf.Cpiface.Northbound_IP)
	log.Println("CPIface zmqd_ip: " + conf.Cpiface.Local_IP)
	log.Println("CPIface hostname: " + conf.Cpiface.Hostname)
	log.Println("Adding ", conf.MaxSessions, " rules...")

	// setting s1u_sgw_ip
	s1u_sgw_ip = inet_aton(conf.Cpiface.N3_IP)

	// get bess grpc client
	conn, err := grpc.Dial(*bess, grpc.WithInsecure())
	if err != nil {
		log.Println("did not connect:", err)
	}
	defer conn.Close()

	c := pb.NewBESSControlClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// operation needs pausing workers
	_, err = c.PauseAll(ctx, &pb.EmptyRequest{})
	if err != nil {
		log.Println("unable to pause:", err)
	}
	defer c.ResumeAll(ctx, &pb.EmptyRequest{})

	for i := uint32(0); i < conf.MaxSessions; i++ {
		// create and add pdr
		addPDR(ctx, c, uint32(i))

		// create and add far
		addFAR(ctx, c, s1u_sgw_ip, uint32(i))

		// create and add counters
		addCounters(ctx, c, uint32(i))
	}

	log.Println("Done!")
}
