// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 The Linux Foundation

package pfcpiface

import (
	"net"
	"testing"
)

// setUpfInfoDP is a fakeDP that records whether the agent set it up.
type setUpfInfoDP struct {
	fakeDP
	setUp bool
}

func (d *setUpfInfoDP) SetUpfInfo(u *upf, conf *Conf) { d.setUp = true }

// loopbackName returns the name of this host's loopback interface: lo on
// Linux, lo0 on macOS.
func loopbackName(t *testing.T) string {
	t.Helper()

	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("listing interfaces: %v", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			return iface.Name
		}
	}

	t.Skip("no loopback interface")

	return ""
}

func TestNewPFCPIfaceUsesGivenDatapath(t *testing.T) {
	lo := loopbackName(t)
	conf := Conf{RespTimeout: "2s"}
	conf.AccessIface.IfName = lo
	conf.CoreIface.IfName = lo

	dp := &setUpfInfoDP{}

	p := newPFCPIface(conf, dp)
	if p.upf == nil {
		t.Fatal("newPFCPIface built no upf")
	}

	if p.fp != dp {
		t.Errorf("agent datapath is %T, want the fake", p.fp)
	}

	if p.upf.datapath != dp {
		t.Errorf("upf datapath is %T, want the fake", p.upf.datapath)
	}

	if !dp.setUp {
		t.Error("SetUpfInfo was not called on the fake datapath")
	}
}
