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

	pb "github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"google.golang.org/grpc"
)

var (
	bess       = flag.String("bess", "localhost:10514", "BESS IP/port combo")
	configPath = flag.String("config", "upf.json", "path to upf config")
)

// Conf : Json conf struct
type Conf struct {
	MaxSessions uint32      `json:"max_sessions"`
	Cpiface     CpifaceType `json:"cpiface"`
}

// CpifaceType : ZMQ-based interface struct
type CpifaceType struct {
	N3IP string `json:"s1u_sgw_ip"`
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
func ParseJSON(filepath *string, conf *Conf) {
	/* Open up file */
	jsonFile, err := os.Open(*filepath)

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

func main() {
	var conf Conf

	// cmdline args
	flag.Parse()

	// read and parse json startup file
	ParseJSON(configPath, &conf)
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
	ctx := context.Background()

	// operation needs pausing workers
	_, err = c.PauseAll(ctx, &pb.EmptyRequest{})
	if err != nil {
		log.Println("unable to pause:", err)
	}

	for i := uint32(0); i < conf.MaxSessions; i++ {

		// create and add pdr
		addPDR(ctx, c, uint32(i))

		// create and add far
		addFAR(ctx, c, s1uSgwIP, uint32(i))

		// create and add counters
		addCounters(ctx, c, uint32(i))
	}

	log.Println("Done!")
	c.ResumeAll(ctx, &pb.EmptyRequest{})
}
