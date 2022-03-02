package pfcpiface

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestCasesAllocatePFCPConnId struct {
	input          *PFCPNode
	expectedOutput uint32
	description    string
}

type TestCasesCleanUpPFCPConn struct {
	input          string
	expectedOutput bool
	description    string
}

func TestFindPFCPConnByFseid(t *testing.T) {
	pConn := &PFCPConn{
		pConnId: uint32(3),
	}
	node := &PFCPNode{
		pConns: make(map[string]*PFCPConn),
	}

	node.pConns["TestString"] = pConn

	id := uint64(3) << 32
	actualPConn := node.findPFCPConnByFseid(id)

	assert.Equal(t, pConn, actualPConn)
}

func TestInitPFCPConnIdPool(t *testing.T) {
	node := &PFCPNode{}
	node.initPFCPConnIdPool()

	checkPool := make([]uint32, 0, maxPFCPConns)
	for i := 1; i < maxPFCPConns+1; i++ {
		checkPool = append(checkPool, uint32(i))
	}
	assert.Equal(t, node.pfcpConnIdPool, checkPool)
	assert.Equal(t, len(node.pfcpConnIdPool), maxPFCPConns)
}

func TestAllocatePFCPConnId(t *testing.T) {
	node := &PFCPNode{
		pfcpConnIdPool: make([]uint32, 0, maxPFCPConns),
	}
	node.initPFCPConnIdPool()

	secNode := &PFCPNode{
		pfcpConnIdPool: make([]uint32, 0, maxPFCPConns),
	}

	for _, scenario := range []TestCasesAllocatePFCPConnId{
		{
			input:          node,
			expectedOutput: 1,
			description:    "First allocatedId should be 1",
		},
		{
			input:          secNode,
			expectedOutput: 0,
			description:    "Empty Pool returning zero as allocatedId",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			allocatedId, err := scenario.input.allocatePFCPConnId()
			if err == nil {
				assert.Equal(t, scenario.expectedOutput, allocatedId)
			} else {
				assert.Equal(t, scenario.expectedOutput, allocatedId)
				require.Error(t, err)
			}
		})
	}

	secNode.initPFCPConnIdPool()

	var allocateAllId uint32

	var err error
	for j := 0; j < maxPFCPConns+1; j++ {
		allocateAllId, err = secNode.allocatePFCPConnId()
	}
	assert.Equal(t, uint32(0), allocateAllId)
	require.Error(t, err)
}

func TestCleanUpPFCPConn(t *testing.T) {
	var rAddr string = "172.19.0.1:59384"

	node := &PFCPNode{
		pConns:         make(map[string]*PFCPConn),
		pfcpConnIdPool: make([]uint32, 0, maxPFCPConns),
	}
	pConn := &PFCPConn{
		pConnId: 20,
	}

	node.pConns[rAddr] = pConn

	for _, scenario := range []TestCasesCleanUpPFCPConn{
		{
			input:          rAddr,
			expectedOutput: false,
			description:    "Deleting the PFCPConn from pConns map",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			node.cleanUpPFCPConn(scenario.input)
			_, ok := node.pConns[rAddr]
			assert.Equal(t, scenario.expectedOutput, ok)
			assert.Equal(t, 1, len(node.pfcpConnIdPool))
			assert.Equal(t, pConn.pConnId, node.pfcpConnIdPool[0])
		})
	}
}
