package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/golang/protobuf/ptypes"
	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

var (
	bess = flag.String("bess", "localhost:10514", "BESS IP/port combo")
)

type pdr struct {
	eNBIP    uint32
	teid     uint32
	ueAddr   uint32
	inetIP   uint32
	uePort   uint16
	inetPort uint16
	proto    uint8
}

func addPDR(ctx context.Context, c pb.BESSControlClient, pdr pdr) {
	f := &pb.WildcardMatchCommandAddArg{
		Gate:     1,
		Priority: 1,
		Values:   []*pb.FieldData{},
		Masks:    []*pb.FieldData{},
		Valuesv:  []*pb.FieldData{},
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
}

func main() {
	// cmdline args
	flag.Parse()

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

	// create and add pdr
	p := pdr{}
	addPDR(ctx, c, p)
}
