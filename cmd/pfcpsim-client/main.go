package main

import (
	"fmt"
	p4rtc "github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/util/conversion"
	"github.com/omec-project/upf-epc/pkg/mockSmf/smf"
	"github.com/omec-project/upf-epc/test/integration/providers"
	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"
	"github.com/pborman/getopt/v2"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	log *logrus.Logger

	remotePeerAddress net.IP
	localAddress      net.IP
	upfAddress        net.IP
	NodeBAddress      net.IP
	ueAddressPool     string

	inputFile string
	doOnce    sync.Once

	sessionCount int

	globalMockSmf *smf.MockSMF
)

const (
	// Values for mock-up4 environment
	defaultGNodeBAddress = "198.18.0.10"
	defaultUeAddressPool = "17.0.0.0/24"

	defaultUpfN3Address = "198.18.0.1"

	defaultSliceID = 0

	srcIfaceAccess = 0x1
	srcIfaceCore   = 0x2

	directionUplink   = 0x1
	directionDownlink = 0x2
)

// GetLoggerInstance uses a singleton pattern to return a single logger instance
// that can be used anywhere.
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

// copyOutputToLogfile reads from Stdout and Stderr to save in a persistent file,
// provided through logfile parameter.
func copyOutputToLogfile(logfile string) func() {
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

func initMockUP4() error {
	// used to initialize UP4 only
	var masterElectionID = p4_v1.Uint128{High: 2, Low: 0}

	p4rtClient, err := providers.ConnectP4rt("127.0.0.1:50001", masterElectionID)
	if err != nil {
		return err
	}
	defer providers.DisconnectP4rt()

	_, currentFilePath, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("could not retrieve current file path")
	}

	bmv2Json := path.Join(currentFilePath, "../../../conf/p4/bin/bmv2.json")
	p4Info := path.Join(currentFilePath, "../../../conf/p4/bin/p4info.txt")

	_, err = p4rtClient.SetFwdPipe(bmv2Json, p4Info, 0)
	if err != nil {
		return err
	}

	ipAddr, err := conversion.IpToBinary(defaultUpfN3Address)
	if err != nil {
		return err
	}

	srcIface, err := conversion.UInt32ToBinary(srcIfaceAccess, 1)
	if err != nil {
		return err
	}

	direction, err := conversion.UInt32ToBinary(directionUplink, 1)
	if err != nil {
		return err
	}

	sliceID, err := conversion.UInt32ToBinary(defaultSliceID, 1)
	if err != nil {
		return err
	}

	te := p4rtClient.NewTableEntry("PreQosPipe.interfaces", []p4rtc.MatchInterface{&p4rtc.LpmMatch{
		Value: ipAddr,
		PLen:  32,
	}}, p4rtClient.NewTableActionDirect("PreQosPipe.set_source_iface",
		[][]byte{srcIface, direction, sliceID}), nil)

	if err := p4rtClient.InsertTableEntry(te); err != nil {
		return err
	}

	return nil
}

func init() {
	log = GetLoggerInstance()

	err := initMockUP4()
	if err != nil {
		log.Fatalf("Could not initialize mock UP4: %v", err)
	}

	providers.RunDockerCommand("pfcpiface", "/bin/pfcpiface -config /config.json")

	// wait for PFCP Agent to initialize
	time.Sleep(time.Second * 3)
}

// getInterfaceAddress retrieves the IP of interfaceName.
// Returns error if fail occurs at any stage.
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

	return nil, fmt.Errorf("could not find interface: %v", interfaceName)
}

// parseArgs perform flag parsing and validation saving necessary data to global variables.
func parseArgs() {
	inputF := getopt.StringLong("input-file", 'f', "", "File to poll for input commands. Default is stdin")
	outputFile := getopt.StringLong("output-file", 'o', "", "File in which copy from Stdout. Default uses only Stdout")
	remotePeer := getopt.StringLong("remote-peer-address", 'r', "127.0.0.1", "Address or hostname of the remote peer (PFCP Agent)")
	upfAddr := getopt.StringLong("upf-address", 'u', defaultUpfN3Address, "Address of the UPF (UP4)")
	interfaceName := getopt.StringLong("interface", 'i', "", "Set interface name to discover local address")
	sessionCnt := getopt.IntLong("session-count", 'c', 1, "Set the amount of sessions to create, starting from 1 (included)")
	ueAddrPool := getopt.StringLong("ue-address-pool", 'e', defaultUeAddressPool, "The IPv4 CIDR prefix from which UE addresses will be generated, incrementally")
	NodeBAddr := getopt.StringLong("nodeb-address", 'g', defaultGNodeBAddress, "The IPv4 of (g/e)NodeBAddress")
	verbosity := getopt.BoolLong("verbose", 'v', "Set verbosity level to debug")

	optHelp := getopt.BoolLong("help", 0, "Help")

	getopt.Parse()
	if *optHelp {
		getopt.Usage()
		os.Exit(0)
	}

	// Flag checks and validations
	if *verbosity {
		level := logrus.DebugLevel
		SetLogLevel(level)
		log.Infof("Verbosity level set to: %v", level.String())
	}

	if *outputFile != "" {
		// TODO move this in main function
		fn := copyOutputToLogfile(*outputFile)
		defer fn()
	}

	if *inputF != "" {
		inputFile = *inputF
	}

	if *sessionCnt <= 0 {
		log.Fatalf("Session count cannot be 0 or a negative number")
	}
	sessionCount = *sessionCnt

	// IPs checks
	NodeBAddress = net.ParseIP(*NodeBAddr)
	if NodeBAddress == nil {
		log.Fatalf("Could not retrieve IP address of (g/e)NodeB")
	}

	remotePeerAddress = net.ParseIP(*remotePeer)
	if remotePeerAddress == nil {
		address, err := net.LookupHost(*remotePeer)
		if err != nil {
			log.Fatalf("Could not retrieve hostname or address for remote peer: %s", *remotePeer)
		}
		remotePeerAddress = net.ParseIP(address[0])
	}

	upfAddress = net.ParseIP(*upfAddr)
	if upfAddress == nil {
		log.Fatalf("Error while parsing UPF address")
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

// readInput will cycle through user's input. if inputFile was provided as a flag, Stdin redirection is performed.
func readInput(input chan<- string) {
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
		var u string
		_, err := fmt.Scanf("%s\n", &u)
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

// handleUserInput spawn a goroutine cycling through user's input.
func handleUserInput() {
	userInput := make(chan string)
	go readInput(userInput)

	for {
		fmt.Println("'disassociate': Teardown Association")
		fmt.Println("'associate': Setup Association")
		fmt.Println("'create': Create Sessions ")
		fmt.Println("'delete': Delete Sessions ")
		fmt.Println("'exit': Exit ")
		fmt.Print("Enter service: ")

		select {
		case userAnswer := <-userInput:
			switch userAnswer {
			case "disassociate":
				log.Tracef("Selected teardown association")
				globalMockSmf.TeardownAssociation()
			case "associate":
				log.Tracef("Selected setup association")
				globalMockSmf.SetupAssociation()
			case "create":
				log.Tracef("Selected create sessions")
				globalMockSmf.InitializeSessions(sessionCount)
			case "delete":
				log.Tracef("Selected delete sessions")
				globalMockSmf.DeleteAllSessions()
			case "exit":
				log.Tracef("Shutting down")
				globalMockSmf.Disconnect()
				os.Exit(0)

			default:
				fmt.Println("Not implemented or bad entry")
			}
		}
	}
}

func main() {
	parseArgs()

	globalMockSmf = smf.NewMockSMF(localAddress.String(),
		ueAddressPool,
		NodeBAddress.String(),
		upfAddress.String(),
		GetLoggerInstance(),
	)

	err := globalMockSmf.Connect(remotePeerAddress.String())
	if err != nil {
		log.Fatalf("Failed to connect to remote peer: %v", err)
	}

	handleUserInput()
}
