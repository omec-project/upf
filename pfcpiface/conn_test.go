package pfcpiface

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestCaseGenerateFseid struct {
	input       *PFCPConn
	notZero     uint64
	description string
}

func TestGenerateFseid(t *testing.T) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano())) // #nosec G404

	pConn := &PFCPConn{
		pConnId: 22,
		rng:     rng,
	}

	for _, scenario := range []TestCaseGenerateFseid{
		{
			input:       pConn,
			notZero:     0,
			description: "Checking if a random number is not zero",
		},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			generatedId, err := scenario.input.GenerateFseid()
			assert.NotEqual(t, scenario.notZero, generatedId)
			require.NoError(t, err)
		})
	}
}
