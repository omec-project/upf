package main

import (
	"errors"
	"fmt"
	"github.com/omec-project/upf-epc/pkg/mockSmf/smf"
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
	log *logrus.Logger

	remotePeerAddress net.IP
	localAddress      net.IP
	inputFile         string
	doOnce            sync.Once
	sessionCount      int
	baseId            int
	ueAddressPool     string

	globalMockSmf *smf.MockSMF
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

func setStdout(logfile string) func() {
	f, _ := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)

	out := os.Stdout
	multiWriter := io.MultiWriter(out, f)

	// get pipe reader and writer | writes to pipe writer come out pipe reader
	r, w, _ := os.Pipe()

	// replace stdout,stderr with pipe writer | all writes to stdout, stderr will go through pipe instead (fmt.print, log)
	os.Stdout = w
	os.Stderr = w

	// writes with log.Print should also write to multiWriter
	log.SetOutput(multiWriter)

	//create channel to control exit | will block until all copies are finished
	exit := make(chan bool)

	go func() {
		// copy all reads from pipe to multiwriter, which writes to stdout and file
		_, _ = io.Copy(multiWriter, r)
		// when r or w is closed copy will finish and true will be sent to channel
		exit <- true
	}()

	// function to be deferred in main until program exits
	return func() {
		// close writer then block on exit channel. this will let multiWriter finish writing before the program exits
		_ = w.Close()
		<-exit

		_ = f.Close()
	}

}

func init() {
	// Initializing global vars
	log = GetLoggerInstance()
	remotePeerAddress = nil
	localAddress = nil
	inputFile = ""
	globalMockSmf = &smf.MockSMF{}
}

// getInterfaceAddress retrieves the IP of parameter interfaceName. returns error if something goes wrong.
func getInterfaceAddress(interfaceName string) (net.IP, error) {
	// TODO simply this. it retrieves all the interfaces.
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
				if strings.Contains(i.Name, interfaceName) {
					return iface.IP, nil
				}
			}
		}
	}

	return nil, errors.New("could not find a correct interface")
}

func parseArgs() {
	//inputFile := getopt.StringLong("input-file", 'i', "", "File to poll for input commands. Default is stdin")
	outputFile := getopt.StringLong("output-file", 'o', "", "File in which to write output. Default is stdout")
	peerAddr := getopt.StringLong("remoteAddress", 'r', "", "Address of the remote peer (e.g. UPF)")
	verbosity := getopt.BoolLong("verbose", 'v', "Set verbosity level")
	interfaceName := getopt.StringLong("interface", 'i', "Set interface name to discover local address")
	sessionCnt := getopt.IntLong("session-count", 's', 1, "Set the amount of sessions to create, starting from 1 (included)")
	base := getopt.IntLong("base", 'b', 1, "First ID used to generate all other ID fields.")
	ueAddrPool := getopt.StringLong("ue-address-pool", 'u', "17.0.0.0/24", "The IPv4 CIDR prefix from which UE addresses will be drawn, incrementally")
	optHelp := getopt.BoolLong("help", 0, "Help")

	getopt.Parse()
	if *optHelp {
		getopt.Usage()
		os.Exit(0)
	}

	// Flag checks and validations
	if *verbosity {
		SetLogLevel(logrus.DebugLevel)
		log.Info("Verbosity level set.")
	}

	if *outputFile != "" {
		// TODO move this in main function
		fn := setStdout(*outputFile)
		defer fn()
	}

	if *base < 0 {
		log.Fatalf("Base id cannot be a negative number")
	}
	baseId = *base

	if *sessionCnt < 0 {
		log.Fatalf("Session count cannot be a negative number")
	}
	sessionCount = *sessionCnt

	remotePeerAddress = net.ParseIP(*peerAddr)
	if remotePeerAddress == nil {
		address, err := net.LookupHost(*peerAddr)
		if err != nil {
			log.Fatalf("Could not retrieve hostname or address from parameters: %s", *peerAddr)
		}
		remotePeerAddress = net.ParseIP(address[0])
	}

	var err error = nil

	_, _, err = net.ParseCIDR(*ueAddrPool)
	if err != nil {
		log.Fatalf("Could not parse ue address pool: %v", err)
	}
	ueAddressPool = *ueAddrPool

	localAddress, err = getInterfaceAddress(*interfaceName)
	if err != nil {
		log.Fatalf("Error while retriving interface information: %v", err)
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
	go readInput(userInput)

	for {
		// TODO use literals, not int
		fmt.Println("1. Teardown Association")
		fmt.Println("2. Setup Association")
		fmt.Println("3. Create Sessions ")
		fmt.Println("4. Delete Sessions ")
		fmt.Println("9. Exit ")
		fmt.Print("Enter service: ")

		select {
		case userAnswer := <-userInput:
			switch userAnswer {
			case 1:
				log.Infof("Selected teardown association")
				globalMockSmf.TeardownAssociation()
			case 2:
				log.Infof("Selected setup association")
				globalMockSmf.SetupAssociation()
			case 3:
				log.Infof("Selected create sessions")
				globalMockSmf.InitializeSessions(baseId, sessionCount)
			case 4:
				log.Infof("Selected delete sessions")
				globalMockSmf.DeleteAllSessions()
			case 9:
				log.Infoln("Shutting down")
				globalMockSmf.Disconnect()
				os.Exit(0)

			default:
				fmt.Println("Not implemented or bad entry")
			}
		}
	}
}

func server(wg *sync.WaitGroup, quitCh chan struct{}) {
	// Emulates User-plane N4
	defer wg.Done()
	seid := uint64(999) //Mock SEID
	localAddress := net.ParseIP("127.0.0.1")

	laddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%v:8805", localAddress.String()))
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

			case message.MsgTypeSessionEstablishmentRequest:
				log.Infof("received session establishment request")
				cause := ie.NewCause(ie.CauseRequestAccepted)
				fseid := ie.NewFSEID(seid, localAddress, nil)
				seq++
				sessDelResp, err := message.NewSessionEstablishmentResponse(
					0,
					0,
					seid,
					seq,
					0,
					cause,
					fseid,
				).Marshal()

				if err != nil {
					log.Errorf("Error while marshalling session establishment response")
				}

				if _, err := conn.WriteTo(sessDelResp, addr); err != nil {
					log.Fatal(err)
				}

			case message.MsgTypeSessionDeletionRequest:
				log.Infof("Received session deletion request")
				cause := ie.NewCause(ie.CauseRequestAccepted)
				seq++
				sessDelResp, err := message.NewSessionDeletionResponse(
					0,
					0,
					seid,
					seq,
					0,
					cause,
				).Marshal()
				if err != nil {
					log.Errorf("error while marshalling session delete response: %v", err)
				}

				if _, err := conn.WriteTo(sessDelResp, addr); err != nil {
					log.Fatal(err)
				}

			} // end of switch

		}
	}
}

func main() {
	wg := new(sync.WaitGroup)
	quitCh := make(chan struct{})

	wg.Add(1)
	go server(wg, quitCh) // start emulating server for debug.
	time.Sleep(500 * time.Millisecond)

	parseArgs()

	globalMockSmf = smf.NewMockSMF(localAddress.String(), ueAddressPool, GetLoggerInstance())
	err := globalMockSmf.Connect(remotePeerAddress.String())
	if err != nil {
		log.Fatalf("Failed to connect to remote peer: %v", err)
	}

	handleUserInput()

	wg.Wait() // wait for all go-routines before shutting down
}
