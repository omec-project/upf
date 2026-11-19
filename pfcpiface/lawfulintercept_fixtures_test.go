// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

// Fixture values shared by the lawful-interception tests.
//
// Named rather than repeated because goconst is part of the lint this repository runs,
// and because a warrant identifier that appears in nine files is worth one place to
// change. Only values the interception tests share live here; a value used by one test
// stays where it is read.
const (
	// The element's own identifiers, as the ADMF addresses it.
	testNEID = "upf-1"
	testTFID = "smf-1"

	// Warrant identifiers. Distinct so a test that mixes two tasks says which is which.
	testXIDPrimary   = "11111111-1111-4111-8111-111111111111"
	testXIDSecondary = "22222222-2222-4222-8222-222222222222"
	testXIDTertiary  = "33333333-3333-4333-8333-333333333333"
	// Derived from the SEID the datapath tests use, so a failure names the session.
	testXIDSEIDShaped = "26328981-45f4-4191-8000-000000000000"

	// Addresses. The delivery one is TEST-NET-1 (RFC 5737), which is what a documented
	// example may use and what nothing will route.
	testDeliveryIP   = "192.0.2.1"
	testLoopbackIP   = "127.0.0.1"
	testX3SockAddr   = "10.0.0.1:42069"
	testX1ListenAddr = ":8443"

	// Subject addresses, one of each family.
	testUEIPv4 = "10.250.0.9"
	testUEIPv6 = "2001:db8::9"

	// Keepalive windows, as the config carries them.
	testKeepaliveP1 = "30s"
	testKeepaliveP2 = "60s"

	// The delivery type every interception test asks for: content only, no IRI.
	testDeliveryX3Only = "X3Only"

	// Datapath mode, and a value that parses as nothing.
	testModeSim     = "sim"
	testUnparseable = "nonsense"
)
