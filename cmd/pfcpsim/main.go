package main

import (
	"fmt"
	"github.com/omec-project/upf-epc/pkg/pfcpsim-client"
	"github.com/pborman/getopt/v2"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
	"os"
	"strings"
)

var (
	remotePeerAddress net.IP
	localAddress      net.IP
	upfAddress        net.IP
	NodeBAddress      net.IP
	ueAddressPool     string

	inputFile string

	sessionCount int

	globalMockSmf *pfcpsim_client.MockSMF
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

// getInterfaceAddress retrieves the IP of interfaceName.
// Returns error if fail occurs at any stage.
func getInterfaceAddress(interfaceName string) (net.IP, error) {
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
		level := log.DebugLevel
		log.SetLevel(level)
		log.Infof("Verbosity level set to: %v", level.String())
	}

	if *outputFile != "" {
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

	globalMockSmf = pfcpsim_client.NewMockSMF(localAddress.String(),
		ueAddressPool,
		NodeBAddress.String(),
		upfAddress.String(),
		log.StandardLogger(),
	)

	err := globalMockSmf.Connect(remotePeerAddress.String())
	if err != nil {
		log.Fatalf("Failed to connect to remote peer: %v", err)
	}

	handleUserInput()
}
