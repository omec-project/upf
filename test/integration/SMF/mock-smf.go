package main

import (
	"errors"
	"fmt"
	"github.com/pborman/getopt/v2"
	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Global vars
// TODO refactor. Try to make global vars local vars.

var (
	log                    *logrus.Logger
	upfAddress             net.IP
	localAddress           net.IP
	semaphore              chan struct{}
	sequenceNumber         uint32
	associationEstablished bool
	UdpConnection          *net.UDPConn
	inputFile              string
)

// End of global vars

const (
	HEARTBEAT_PERIOD   = 5 // in seconds
	PFCP_PROTOCOL_PORT = 8805
)

// IFace contains the network interface card name and address
type IFace struct {
	Name string
	IP   net.IP
}

func init() {
	// Initializing global vars
	log = logrus.New()
	upfAddress = nil
	localAddress = nil
	semaphore = make(chan struct{}, 1) // binary semaphore guarding sequenceNumber
	sequenceNumber = 0
	associationEstablished = false
	UdpConnection = nil
	inputFile = ""
}

func getSequenceNumber(reset bool) uint32 {
	semaphore <- struct{}{} // Acquire lock
	if reset {
		sequenceNumber = 0
	}
	seq := sequenceNumber

	<-semaphore // Release lock

	return seq
}

func getIfaceAddress(interfaceName string) (*IFace, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Errorf("could not retrieve network interfaces: %v", err)
		return nil, err
	}

	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			log.Errorf("localAddresses: %+v\n", err.Error())
			continue
		}

		for _, addr := range addrs {
			switch iface := addr.(type) {
			case *net.IPNet:
				log.Debugf("Ifaces: %v : %s (%s)\n", i.Name, iface, iface.IP.DefaultMask())
				if strings.Contains(i.Name, interfaceName) {
					return &IFace{
						Name: i.Name,
						IP:   iface.IP,
					}, nil
				}
			}
		}
	}

	return nil, errors.New("could not find a correct interface")
}

// recvHandler method is responsible for receiving pfcp message responses. The
// responses are sent through inbound channel.
func recvHandler(wg *sync.WaitGroup, msgType uint8) {
	defer wg.Done()
	done := false

	for !done {
		buffer := make([]byte, 1500)
		n, _, err := UdpConnection.ReadFromUDP(buffer)
		if err != nil {
			log.Errorf("Error on read: %v", err)
			continue
		}
		msg, err := message.Parse(buffer[:n])
		if err != nil {
			log.Errorf("Error while parsing pfcp message: %v", err)
		}

		switch msg.MessageType() {
		case msgType:
			log.Infof("Received expected msg: %v", msg.MessageTypeName())
		default:
			done = true
			log.Errorf("ERROR Received unexpected message: %v", msg.MessageTypeName())
		}
	}
}

func sendData(data []byte) error {
	_, err := UdpConnection.Write(data)
	if err != nil {
		log.Errorf("Error while sending data to socket: %v", err)
		return err
	}

	return nil
}

func craftPfcpAssociationSetupRequest(address net.IP) *message.AssociationSetupRequest {
	ie1 := ie.NewNodeID(address.String(), "", "")
	ie2 := ie.NewRecoveryTimeStamp(time.Now())

	var seq uint32 = 0 // Should be 0 when initializing new association. TODO reset global sequence_num

	request := message.NewAssociationSetupRequest(seq, ie1, ie2)

	return request
}

func setup_association() {
	wg := new(sync.WaitGroup)
	assoRequest, _ := craftPfcpAssociationSetupRequest(upfAddress).Marshal()
	err := sendData(assoRequest)
	if err != nil {
		log.Errorf("Error while sending association request: %v", err)
		return
	}

	wg.Add(1)
	go recvHandler(wg, message.MsgTypeAssociationSetupResponse) // start response handler on new goroutine

	wg.Wait()
}

func startPfcpHeartbeats(wg *sync.WaitGroup, quitCh chan struct{}) {
	defer wg.Done()
	period := 1 * time.Second
	ticker := time.NewTicker(period)
	log.Debugf("Started PFCP heartbeat goroutine. Sending msgs every: %s seconds", period)

	seq := getSequenceNumber(false)

	go recvHandler(wg, message.MsgTypeHeartbeatResponse) // start receiving

	for {
		select {
		case <-ticker.C:
			// TODO send heartbeat messages
			hbreq := message.NewHeartbeatRequest(
				seq,
				ie.NewRecoveryTimeStamp(time.Now()),
				ie.NewSourceIPAddress(upfAddress, nil, 0),
			)

			hbreq.Header.SequenceNumber = seq // FIXME check this

			reqBytes, err := hbreq.Marshal()
			if err != nil {
				log.Fatalf("could not marshal heartbeat request: %v", err)
			}

			err = sendData(reqBytes)
			if err != nil {
				log.Fatalf("could not send data: %v", err)
			}

		case <-quitCh:
			log.Debug("Stopping PFCP heartbeat goroutine")
			ticker.Stop()
			return
		}
	}
}

func parseArgs() {
	//inputFile := getopt.StringLong("input-file", 'i', "", "File to poll for input commands. Default is stdin")
	//outputFile := getopt.StringLong("output-file", 'o', "", "File in which to write output. Default is stdout")
	upfAddr := getopt.StringLong("upfaddr", 'a', "", "Address of the UPF")
	verbosity := getopt.BoolLong("verbose", 'v', "", "Set verbosity level")
	optHelp := getopt.BoolLong("help", 0, "Help")

	getopt.Parse()
	if *optHelp {
		getopt.Usage()
		os.Exit(0)
	}

	// Flag checks
	if *verbosity {
		log.Level = logrus.DebugLevel
		log.Debug("verbosity level set.")
	}

	upfAddress = net.ParseIP(*upfAddr)
	if upfAddress == nil {
		log.Fatalf("could not parse upf address")
	}

}

// Set global connection to PFCP peer provided address
func connect(address net.IP) {
	udpAddr := fmt.Sprintf("%s:%v", address.String(), PFCP_PROTOCOL_PORT)
	log.Debugf("Start connection to %v", udpAddr)

	rAddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}

	var lAddr *net.UDPAddr = nil // FIXME debug purpose. set local address when using remote connections.
	conn, err := net.DialUDP("udp", lAddr, rAddr)
	if err != nil {
		log.Fatal(err)
	}
	log.Debugf("Connected to %v", conn.RemoteAddr())
	associationEstablished = true
	UdpConnection = conn

}

func readInput(input chan<- int) {
	if inputFile != "" {
		// Set inputFile as stdIn

		oldStdin := os.Stdin
		defer func() {
			os.Stdin = oldStdin
		}() // restore old StdIN

		f, err := os.Open(inputFile)
		if err != nil {
			log.Errorf("Error while reading inputFile: %v", err)
		} else {
			defer func(f *os.File) {
				err := f.Close()
				if err != nil {
					log.Errorf("Error while closing file: %v", err)
				}
			}(f)

			os.Stdin = f
		}
	}

	for {
		var u int
		_, err := fmt.Scanf("%d\n", &u)
		if err != nil {
			if err == io.EOF {
				return
			} else {
				panic(err)
			}
		}
		input <- u
	}
}

func handleUserInput() {
	userInput := make(chan int)
	done := false
	go readInput(userInput)

	for !done {
		fmt.Println("Enter answer ")

		select {
		case userAnswer := <-userInput:
			if userAnswer == 1 {
				fmt.Println("Correct answer:", userAnswer)
			} else {
				fmt.Println("Wrong answer")
			}
		case <-time.After(5 * time.Second):
			done = true
			fmt.Println("\n Time is over!")
		}
	}
}

func server(wg *sync.WaitGroup, quitCh chan struct{}) {
	defer wg.Done()
	laddr, err := net.ResolveUDPAddr("udp", "localhost:8805")
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 1500)
	var seq uint32 = 1

	for {
		select {
		case <-quitCh:
			log.Debugf("Received quit signal")
			return
		default:
			log.Printf("Server: waiting for messages to come on: %s", laddr)
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				log.Fatal(err)
			}

			msg, err := message.Parse(buf[:n])
			if err != nil {
				log.Printf("Server: ignored undecodable message: %x, error: %s", buf[:n], err)
				continue
			}

			hbreq, ok := msg.(*message.HeartbeatRequest)
			if !ok {
				log.Printf("Server: got unexpected message: %s, from: %s", msg.MessageTypeName(), addr)
				continue
			}

			ts, err := hbreq.RecoveryTimeStamp.RecoveryTimeStamp()
			if err != nil {
				log.Printf("Server: got Heartbeat Request with invalid TS: %s, from: %s", err, addr)
				continue
			} else {
				log.Printf("Server: got Heartbeat Request with TS: %s, from: %s", ts, addr)
			}

			// Timestamp shouldn't be the time message is sent in the real deployment but anyway :D
			hbres, err := message.NewHeartbeatResponse(seq, ie.NewRecoveryTimeStamp(time.Now())).Marshal()
			if err != nil {
				log.Fatal(err)
			}
			seq++

			if _, err := conn.WriteTo(hbres, addr); err != nil {
				log.Fatal(err)
			}

			log.Printf("Server: sent Heartbeat Response to: %s", addr)
		}
	}
}

func main() {
	log.SetOutput(io.MultiWriter(os.Stdout)) // Debug. if you want to save the log output to file simply add it in here.
	wg := new(sync.WaitGroup)                // main wait group
	quitCh := make(chan struct{})

	wg.Add(1)
	go server(wg, quitCh) // start emulating server for debug.

	parseArgs()

	connect(upfAddress) // set global connection

	wg.Add(1)
	go startPfcpHeartbeats(wg, quitCh)

	time.Sleep(3 * time.Second)
	quitCh <- struct{}{} // stops pfcpheartbeat and server goroutine

	handleUserInput()

	wg.Wait() // wait for all go-routine before shutting down

	//// FIXME how to return default interface connected to internet without passing iface name?
	//ourAddress, err := getIfaceAddress("en0")
	//if err != nil {
	//	panic("Couldn't retrieve interface information")
	//}
	//fmt.Println(fmt.Sprintf("Interface Name: %s, IP: %s", ourAddress.Name, ourAddress.IP))

	// Unimplemented
	//if *inputFile == "" {
	//	*inputFile = "/dev/stdin"
	//} else {
	//	// input file is provided. Read it
	//	log.Debug(fmt.Sprintf("provided input-file: %s", *inputFile))
	//	data, err := ioutil.ReadFile(*inputFile)
	//
	//	if err != nil {
	//		fmt.Println("File reading error", err)
	//		return
	//	}
	//
	//	fmt.Println("Contents of file:", string(data))
	//}
	//if *outputFile == "" {
	//	*outputFile = "/dev/stdout"
	//} else {
	//	// File output is provided.
	//	// TODO redirect output to outputFile
	//}

	//if pcapFile != "" {
	//	// Pcap file flag is provided. Truncate file content and start writing.
	//	pcapFile = "capture.pcap"
	//	pcapFile, err := os.OpenFile(pcapFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//	// DEBUG
	//	if err := pcapFile.Close(); err != nil {
	//		log.Fatal(err)
	//	}
	//}

	//addrs, err := net.LookupHost(*upfAddr) // equivalent of py socket.gethostbyname
	//if err != nil {
	//	panic(fmt.Errorf("couldn't retrieve hostname from Address: %s", *upfAddr))
	//}
	//fmt.Println(addrs)

}
