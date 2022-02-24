package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const dummy_p4info = "dummy_p4info.txt"

func Test_generate(t *testing.T) {
	p4config := getP4Config(dummy_p4info)

	test := generateConstants(p4config)

	require.True(t, strings.Contains(test, "HdrPreQosPipeSessionsUplinkTeid \t uint32 = 2"))
	require.True(t, strings.Contains(test, "TablePreQosPipeSessionsDownlink \t uint32 = 34742049"))
	require.True(t, strings.Contains(test, "ActionParamPreQosPipeSetSessionUplinkSessionMeterIdx \t uint32 = 1"))
	require.True(t, strings.Contains(test, "DirectCounterAcls \t uint32 = 325583051"))
	require.True(t, strings.Contains(test, "PacketMetaPacketOut \t uint32 = 75327753"))
	require.True(t, strings.Contains(test, "MeterPreQosPipeAppMeter \t uint32 = 338231090"))
}

func Test_generateTables(t *testing.T) {
	p4config := getP4Config(dummy_p4info)

	test := generateTables(p4config)

	require.True(t, strings.Contains(test, "39015874:\"PreQosPipe.Routing.routes_v4\","))
	require.True(t, strings.Contains(test, "34778590:\"PreQosPipe.terminations_downlink\","))
}
