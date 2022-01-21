package main

import (
	"context"
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
var (
	log               *logrus.Logger
	remotePeerAddress net.IP
	localAddress      net.IP
	inputFile         string
	doOnce            sync.Once

	globalMockSmf *MockSMF
)

func GetLoggerInstance() *logrus.Logger {
	// setting global logging instance
	doOnce.Do(func() {
		log = logrus.New()
	})
	return log
}

func SetLogLevel(level logrus.Level) {
	log.SetLevel(level)
}

const (
	HEARTBEAT_PERIOD   = 5 // in seconds
	PFCP_PROTOCOL_PORT = 8805
)

type Session struct {
	ourSeid uint64

	ueAddress net.IP
	peerSeid  uint64
	//sentPdrs  map[int]ie.IE
	//sentFars  map[int]ie.IE
	//sentQers map[int]ie.IE
}

type MockSMF struct {
	seqLock        *sync.Mutex
	sequenceNumber uint32

	associationEstablished bool

	activeSessions map[int]Session

	remotePeerAddress net.IP
	localAddress      net.IP
	UdpConnection     *net.UDPConn
	recvChannel       chan message.Message
	heartbeatChannel  chan message.HeartbeatResponse

	log *logrus.Logger

	ctx              context.Context
	cancelHeartbeats context.CancelFunc
	cancelRecv       context.CancelFunc
}

func NewMockSMF(rAddr net.IP, lAddr net.IP, logger *logrus.Logger) *MockSMF {
	if logger == nil {
		logger = new(logrus.Logger)
	}

	return &MockSMF{
		remotePeerAddress:      rAddr,
		localAddress:           lAddr,
		seqLock:                new(sync.Mutex),
		sequenceNumber:         0,
		associationEstablished: false,
		activeSessions:         make(map[int]Session),
		ctx:                    context.Background(),
		log:                    GetLoggerInstance(),
		recvChannel:            make(chan message.Message),
		heartbeatChannel:       make(chan message.HeartbeatResponse),
	}
}

func (m *MockSMF) DisconnectAndClose() {
	m.cancelHeartbeats()
	m.cancelRecv()
	err := m.UdpConnection.Close()
	if err != nil {
		log.Errorf("Error while closing connection to remote peer: %v", err)
		return
	}
}

func (m *MockSMF) Connect() {
	udpAddr := fmt.Sprintf("%s:%v", m.remotePeerAddress.String(), PFCP_PROTOCOL_PORT)
	m.log.Infof("Start connection to %v", udpAddr)

	rAddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}

	lAddr := &net.UDPAddr{IP: m.localAddress}
	//lAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1")} // FIXME DEBUG
	m.UdpConnection, err = net.DialUDP("udp", lAddr, rAddr)
	if err != nil {
		m.log.Fatal(err)
	}
	m.log.Debugf("Connected to %v", m.UdpConnection.RemoteAddr())

	ctx, cancelFunc := context.WithCancel(m.ctx)
	m.cancelRecv = cancelFunc
	go m.recv(ctx)
}

func (m *MockSMF) getSequenceNumber(reset bool) uint32 {
	m.seqLock.Lock()

	m.sequenceNumber++
	if reset {
		m.sequenceNumber = 0
	}

	m.seqLock.Unlock()

	return m.sequenceNumber
}

func (m *MockSMF) sendData(data []byte) error {
	_, err := m.UdpConnection.Write(data)
	if err != nil {
		m.log.Errorf("Error while sending data to socket: %v", err)
		return err
	}

	return nil
}

func (m *MockSMF) createSession() {
	lastIndex := len(m.activeSessions) - 1
	lastSeid := m.activeSessions[lastIndex].ourSeid // get last ourSeid to generate new one
	newSeid := lastSeid + 1

	sess := Session{
		ourSeid:   newSeid,
		ueAddress: nil, // TODO Where to get UE-Address?
		peerSeid:  0,   // TODO Where to get peer SEID? Association Response?
	}

	m.activeSessions[lastIndex+1] = sess
}

func (m *MockSMF) recv(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			buffer := make([]byte, 1500)

			n, _, err := m.UdpConnection.ReadFromUDP(buffer)
			if err != nil {
				m.log.Errorf("Error on read from connection: %v", err)
				continue
			}
			msg, err := message.Parse(buffer[:n])
			if err != nil {
				m.log.Errorf("Error while parsing pfcp message: %v", err)
				continue
			}

			if hbResponse, ok := msg.(*message.HeartbeatResponse); ok {
				m.heartbeatChannel <- *hbResponse
			} else {
				m.recvChannel <- msg
			}
		}
	}
}

func (m *MockSMF) SetupPfcpAssociation() {
	assoRequest, _ := craftPfcpAssociationSetupRequest(m.remotePeerAddress, m.getSequenceNumber(true)).Marshal()

	err := m.sendData(assoRequest)
	if err != nil {
		m.log.Errorf("Error while sending association request: %v", err)
	}

	response := m.recvWithTimeout(2)

	if response.MessageType() != message.MsgTypeAssociationSetupResponse {
		m.log.Errorf("Received unexpected type of message: %v", response.MessageTypeName())
		return
	}

	log.Infof("Established PFCP association to %v", m.remotePeerAddress)
	m.associationEstablished = true

	ctx, cancelFunc := context.WithCancel(m.ctx)
	m.cancelHeartbeats = cancelFunc
	go m.startPfcpHeartbeats(ctx)
}

func (m *MockSMF) recvWithTimeout(timeout time.Duration) message.Message {
	select {
	case msg := <-m.recvChannel:
		// TODO should I filter here? what if we intercept a message that is not the one we're interested in? e.g. heartbeat
		return msg
	case <-time.After(timeout * time.Second):
		m.log.Errorf("Timeout reached while waiting for response")
		return nil
	}
}

func (m *MockSMF) recvHeartbeatWithTimeout(timeout time.Duration) *message.HeartbeatResponse {
	select {
	case msg := <-m.heartbeatChannel:
		return &msg
	case <-time.After(timeout * time.Second):
		m.log.Errorf("Timeout reached while waiting for response")
		return nil
	}
}

func (m *MockSMF) TeardownPfcpAssociation() {
	data, err := craftPfcpAssociationReleaseRequest(m.remotePeerAddress).Marshal()
	if err != nil {
		m.log.Errorf("Error marshalling association release request: %v", err)
		return
	}

	err = m.sendData(data)
	if err != nil {
		m.log.Errorf("Error while sending data: %v", err)
	}

	response := m.recvWithTimeout(2)
	if response == nil {
		m.log.Errorf("Error while tearing down PFCP association: did not receive any message")
		return
	}
	if response.MessageType() != message.MsgTypeAssociationReleaseResponse {
		m.log.Errorf("Error while tearing down PFCP association. Received unexpected msg. Type: %v",
			response.MessageTypeName())
		return
	}

	m.cancelHeartbeats()

	// clear sessions
	m.activeSessions = make(map[int]Session)
	m.setAssociationEstablished(false)
	log.Infoln("Association removed.")

}

func (m *MockSMF) setAssociationEstablished(value bool) {
	m.associationEstablished = value
}

func (m *MockSMF) startPfcpHeartbeats(ctx context.Context) {

	period := HEARTBEAT_PERIOD * time.Second
	ticker := time.NewTicker(period)

	ctx, cancelFunc := context.WithCancel(m.ctx)
	m.cancelHeartbeats = cancelFunc

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var localAddress net.IP = nil // FIXME recover local address
			seq := m.getSequenceNumber(false)

			hbreq := message.NewHeartbeatRequest(
				seq,
				ie.NewRecoveryTimeStamp(time.Now()),
				ie.NewSourceIPAddress(localAddress, nil, 0),
			)

			reqBytes, err := hbreq.Marshal()
			if err != nil {
				m.log.Fatalf("could not marshal heartbeat request: %v", err)
			}

			err = m.sendData(reqBytes)
			if err != nil {
				m.log.Fatalf("could not send data: %v", err)
			}

			response := m.recvHeartbeatWithTimeout(2 * HEARTBEAT_PERIOD) //FIXME
			if response == nil {
				m.log.Errorf("Did not receive any heartbeat response")
				m.setAssociationEstablished(false)
				m.cancelHeartbeats()
			}

			m.setAssociationEstablished(true)
		}
	}
}

func init() {
	// Initializing global vars
	log = GetLoggerInstance()
	remotePeerAddress = nil
	localAddress = nil
	inputFile = ""
	globalMockSmf = &MockSMF{} // Empty struct
}

// Retrieves the IP associated with interfaceName. returns error if something goes wrong.
func getIfaceAddress(interfaceName string) (net.IP, error) {
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
					return iface.IP, nil
				}
			}
		}
	}

	return nil, errors.New("could not find a correct interface")
}

func craftPfcpAssociationReleaseRequest(address net.IP) *message.AssociationReleaseRequest {
	ie1 := ie.NewNodeID(address.String(), "", "")

	return message.NewAssociationReleaseRequest(0, ie1)
}

func craftPfcpAssociationSetupRequest(address net.IP, seq uint32) *message.AssociationSetupRequest {
	ie1 := ie.NewNodeID(address.String(), "", "")
	ie2 := ie.NewRecoveryTimeStamp(time.Now())

	request := message.NewAssociationSetupRequest(seq, ie1, ie2)

	return request
}

func craftPfcpSessionEstRequest(address net.IP, seq uint32, Seid uint64) *message.SessionEstablishmentRequest {
	IEnodeID := ie.NewNodeID(address.String(), "", "")

	fseidIE := craftFseid(address, Seid)
	return message.NewSessionEstablishmentRequest(
		0,
		0,
		0,
		seq,
		0,
		fseidIE,
		IEnodeID)
}

func craftFseid(address net.IP, seid uint64) *ie.IE {
	return ie.NewFSEID(seid, address, nil)
}

func parseArgs() {
	//inputFile := getopt.StringLong("input-file", 'i', "", "File to poll for input commands. Default is stdin")
	//outputFile := getopt.StringLong("output-file", 'o', "", "File in which to write output. Default is stdout")
	upfAddr := getopt.StringLong("upfaddr", 'a', "", "Address of the remote peer (e.g. UPF)")
	verbosity := getopt.BoolLong("verbose", 'v', "Set verbosity level")
	interfaceName := getopt.StringLong("interface", 'i', "Set interface name to discover local address")
	optHelp := getopt.BoolLong("help", 0, "Help")

	getopt.Parse()
	if *optHelp {
		getopt.Usage()
		os.Exit(0)
	}

	// Flag checks
	if *verbosity {
		SetLogLevel(logrus.DebugLevel)
		log.Info("verbosity level set.")
	}

	remotePeerAddress = net.ParseIP(*upfAddr)
	if remotePeerAddress == nil {
		log.Fatalf("could not parse upf address")
	}

	var err error = nil
	localAddress, err = getIfaceAddress(*interfaceName)
	if err != nil {
		log.Fatalf("Error while retriving interface informations: %v", err)
	}

}

func readInput(input chan<- int) {
	//inputFile = "commands.txt" //FIXME DEBUG

	if inputFile != "" {
		// Set inputFile as stdIn

		oldStdin := os.Stdin
		defer func() {
			// restore StdIN
			os.Stdin = oldStdin
		}()

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
				break
			} else {
				log.Debugf("Skipping bad entry: %v", err)
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
		fmt.Println("1. Teardown Association")
		fmt.Println("2. Setup Association")
		fmt.Println("3. Create Session ")
		fmt.Print("Enter service: ")

		select {
		case userAnswer := <-userInput:
			switch userAnswer {
			case 1:
				log.Infof("Selected Teardown Association: %v", userAnswer)
				globalMockSmf.TeardownPfcpAssociation()
			case 2:
				log.Infoln("Selected Setup Association: %v", userAnswer)
				globalMockSmf.SetupPfcpAssociation()

			default:
				fmt.Println("Not implemented or wrong answer")
			}

			//case <-time.After(10 * time.Second):
			//	done = true
			//	fmt.Println("\n DEBUG: Time is over!")
		}
	}
}

func server(wg *sync.WaitGroup, quitCh chan struct{}) {
	// Emulates User-plane N4
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

			switch msg.MessageType() {
			case message.MsgTypeHeartbeatRequest:
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

			case message.MsgTypeAssociationSetupRequest:
				assoRequest, ok := msg.(*message.AssociationSetupRequest)
				if !ok {
					log.Printf("Server: got unexpected message: %s, from: %s", assoRequest.MessageTypeName(), addr)
					continue
				}
				seq++

				assoResponse, err := message.NewAssociationSetupResponse(seq).Marshal() // FIXME add IEs
				if err != nil {
					log.Errorf("Error while marshaling association response: %v", err)
				}

				if _, err := conn.WriteTo(assoResponse, addr); err != nil {
					log.Fatal(err)
				}
				log.Printf("Server: sent Association Response to: %s", addr)

			case message.MsgTypeAssociationReleaseRequest:
				assoReleaseRequest, ok := msg.(*message.AssociationReleaseRequest)
				if !ok {
					log.Infof("Server: got unexpected message: %s, from: %s", assoReleaseRequest.MessageTypeName(), addr)
					continue
				}
				seq = 0

				cause := ie.NewCause(ie.CauseRequestAccepted)
				ts := ie.NewRecoveryTimeStamp(time.Now())
				releaseResponse, err := message.NewAssociationReleaseResponse(seq, cause, ts).Marshal() // FIXME add IEs
				if err != nil {
					log.Errorf("Error while marshaling association response: %v", err)
				}

				if _, err := conn.WriteTo(releaseResponse, addr); err != nil {
					log.Fatal(err)
				}
				log.Infof("Server: sent Association Release Response to: %s", addr)
				log.Infof("Server: Association removed.")
			} // end of switch

		}
	}
}

func main() {
	log.SetOutput(io.MultiWriter(os.Stdout)) // Debug. if you want to save the log output to file simply add it in here.
	wg := new(sync.WaitGroup)                // main wait group
	quitCh := make(chan struct{})

	wg.Add(1)
	go server(wg, quitCh) // start emulating server for debug.
	time.Sleep(500 * time.Millisecond)

	parseArgs()

	globalMockSmf = NewMockSMF(remotePeerAddress, localAddress, log)
	globalMockSmf.Connect()

	handleUserInput()

	globalMockSmf.DisconnectAndClose()
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
