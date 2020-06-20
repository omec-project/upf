package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

var (
	bess = flag.String("bess", "localhost:10514", "BESS IP/port combo")
)

// Don't change these values
const (
	// src-iface consts
	Core   = 0x2
	Access = 0x1
	// far-id specific directions
	Uplink   = 0x0
	Downlink = 0x1
	// udp tunneling port
	UDPGtpuPort = 2152
	// far-action specific values
	FARForwardAction = 0x0
	FARTunnelAction  = 0x1
)

type pdr struct {
	srcIface     uint8
	tunnelIP4Dst uint32
	eNBTeid      uint32
	ueAddrIP     uint32
	inetIP       uint32
	uePort       uint16
	inetPort     uint16
	proto        uint8

	srcIfaceMask     uint8
	tunnelIP4DstMask uint32
	eNBTeidMask      uint32
	ueAddrIPMask     uint32
	inetIPMask       uint32
	uePortMask       uint16
	inetPortMask     uint16
	protoMask        uint8

	pdrID     uint32
	fseID     uint32
	ctrID     uint32
	farID     uint32
	needDecap uint8
}

// NewPDR create new instance of PDR
//func NewPDR(srcIface uint8, eNBTeid uint32, ueaddrIP uint32, inetIP uint32, eNBTeidMask uint32, ueaddrIPMask uint32, inetIPMask uint32) Something {
//	something := Something{}
//	something.Text = text
//	something.DefaultText = "default text"
//	return something
// }

// Conf : Json conf struct
type Conf struct {
	//Mode             string      `json:"mode"`
	//UECidr           string      `json:"ue_cidr"`
	//EnbCidr          string      `json:"enb_cidr"`
	MaxSessions uint32 `json:"max_sessions"`
	//MaxIPDefragFlows uint32      `json:"max_ip_defrag_flows"`
	//IPFragWithEthMtu uint32      `json:"ip_frag_with_eth_mtu"`
	//Measure          bool        `json:"measure"`
	//S1UIntf          Interface   `json:"s1u"`
	//SGIIntf          Interface   `json:"sgi"`
	//Workers          uint32      `json:"workers"`
	Cpiface CpifaceType `json:"cpiface"`
}

//type Interface struct {
//	Name string `json:"ifname"`
//	Nat  string `json:"ip_masquerade"`
//}

// CpifaceType : ZMQ-based interface struct
type CpifaceType struct {
	N3IP string `json:"s1u_sgw_ip"`
	//	Northbound_IP string `json:"zmqd_nb_ip"`
	//	Local_IP      string `json:"zmqd_ip"`
	//	Hostname      string `json:"hostname"`
}

func inetAton(ip string) (ipInt uint32) {
	ipByte := net.ParseIP(ip).To4()
	ipInt = ip2int(ipByte)
	return
}

func ip2int(ip net.IP) uint32 {
	if len(ip) == 16 {
		return binary.BigEndian.Uint32(ip[12:16])
	}
	return binary.BigEndian.Uint32(ip)
}

// ParseJSON : parse json file and populate corresponding struct
func ParseJSON(conf *Conf) {
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

func addPDR(ctx context.Context, c pb.BESSControlClient, sessionID uint32) {

	const ueip, teid = 0x10000001, 0xf0000000

	f := &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: Core}},                     /* src_iface */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* tunnel_ipv4_dst */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* enb_teid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(ueip + sessionID)}}, /* ueaddr ip*/
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* inet ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* ue port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* inet port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* proto id */
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
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* pdr-id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + sessionID)}}, /* fseid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(sessionID)}},        /* ctr_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(Downlink)}},         /* far_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* need_decap */
		},
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PDRLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert DOWNLINK PDR rule! Retrying...")
			return
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: Access}},                   /* src_iface */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* tunnel_ipv4_dst */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + sessionID)}}, /* enb_teid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0}},                        /* inet ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(ueip + sessionID)}}, /* ueaddr ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* inet port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* ue port */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* proto-id */
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
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x0}},                      /* pdr_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + sessionID)}}, /* fseid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(sessionID)}},        /* ctr_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(Uplink)}},           /* far_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x1}},                      /* need_decap */
		},
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PDRLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert UPLINK PDR rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func addFAR(ctx context.Context, c pb.BESSControlClient, s1uip uint32, sessionID uint32) {

	const ueip, teid, enbip = 0x10000001, 0xf0000000, 0x0b010181
	const ng4tMaxUeRan, ng4tMaxEnbRan = 500000, 80
	// NG4T-based formula to calculate enodeB IP address against a given UE IP address
	// il_trafficgen also uses the same scheme
	// See SimuCPEnbv4Teid(...) in ngic code for more details
	ueOfRan := sessionID % ng4tMaxUeRan
	ran := sessionID / ng4tMaxUeRan
	enbOfRan := ueOfRan % ng4tMaxEnbRan
	enbIdx := ran*ng4tMaxEnbRan + enbOfRan

	f := &pb.ExactMatchCommandAddArg{
		Gate: 1,
		Fields: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(Downlink)}},         /* far_id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + sessionID)}}, /* fseid */
		},
		Values: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: FARTunnelAction}},          /* action */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: 0x1}},                      /* tunnel_out_type */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(s1uip)}},            /* s1u-ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(enbip + enbIdx)}},   /* enb ip */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + sessionID)}}, /* enb teid */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(UDPGtpuPort)}},      /* udp gtpu port */
		},
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "FARLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert DOWNLINK FAR rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.ExactMatchCommandAddArg{
		Gate: 1,
		Fields: []*pb.FieldData{
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(Uplink)}},           /* far id */
			{Encoding: &pb.FieldData_ValueInt{ValueInt: uint64(teid + sessionID)}}, /* fseid */
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

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "FARLookup",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert UPLINK FAR rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func addCounters(ctx context.Context, c pb.BESSControlClient, sessionID uint32) {

	f := &pb.CounterAddArg{
		CtrId: sessionID,
	}

	any, err := ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PreQoSCounter",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PreQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.CounterAddArg{
		CtrId: sessionID,
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PostDLQoSCounter",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PostDLQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}

	f = &pb.CounterAddArg{
		CtrId: sessionID,
	}

	any, err = ptypes.MarshalAny(f)
	if err != nil {
		log.Println("Error marshalling the rule", f, err)
		return
	}

	for res, err := c.ModuleCommand(ctx, &pb.CommandRequest{
		Name: "PostULQoSCounter",
		Cmd:  "add",
		Arg:  any,
	}); ; {

		if err != nil {
			// codes.DeadlineExceeded.String() has a diff string literal. WHY ?!!
			if !strings.Contains(grpc.ErrorDesc(err), "deadline exceeded") {
				log.Println("Error running module command:", err)
				return
			}
			log.Println("Deadline exceeded when trying to insert PostULQoSCounter rule! Retrying...")
		}

		if res.GetError() != nil {
			log.Println("Error updating rule:", res.GetError())
			return
		}

		/* everything went fine */
		if err == nil {
			break
		}
	}
}

func main() {
	var conf Conf

	// cmdline args
	flag.Parse()

	// read and parse json startup file
	ParseJSON(&conf)
	/*
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
	*/
	log.Println(conf)

	// setting s1u_sgw_ip
	s1uSgwIP := inetAton(conf.Cpiface.N3IP)

	// get bess grpc client
	conn, err := grpc.Dial(*bess, grpc.WithInsecure())
	if err != nil {
		log.Println("did not connect:", err)
	}
	defer conn.Close()

	c := pb.NewBESSControlClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// create context without timeout
	//ctx := context.Background()

	// operation needs pausing workers
	_, err = c.PauseAll(ctx, &pb.EmptyRequest{})
	if err != nil {
		log.Println("unable to pause:", err)
	}
	defer c.ResumeAll(ctx, &pb.EmptyRequest{})

	for i := uint32(0); i < conf.MaxSessions; i++ {
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		// create and add pdr
		addPDR(ctx, c, uint32(i))

		// create and add far
		addFAR(ctx, c, s1uSgwIP, uint32(i))

		// create and add counters
		addCounters(ctx, c, uint32(i))
	}

	log.Println("Done!")
}
