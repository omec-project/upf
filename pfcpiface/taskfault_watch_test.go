// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package pfcpiface

import (
	"testing"
	"time"

	"github.com/omec-project/util/li/mtls"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// TestTheTriggerListenerStartsTheTaskFaultWatcher: the task-fault watcher is the only thing
// that reports a task's state faults to the ADMF and retracts them (TS 103 221-1 clause
// 6.5.2.1). A held task selecting no traffic is noTrafficSelected; with the watcher not
// started, that state would be answerable on request and reported to nobody.
func TestTheTriggerListenerStartsTheTaskFaultWatcher(t *testing.T) {
	restoreInterval, restoreStop := taskFaultInterval, taskFaultStop
	stop := make(chan struct{})
	taskFaultInterval, taskFaultStop = 50*time.Millisecond, stop
	t.Cleanup(func() {
		close(stop)
		taskFaultInterval, taskFaultStop = restoreInterval, restoreStop
	})

	dir := t.TempDir()
	caPath, caCert, caKey := liCA(t, dir)
	upfCert, upfKey := liLeaf(t, dir, caCert, caKey, "NE", testNEID)

	upfMat, err := mtls.Load(upfCert, upfKey, caPath)
	if err != nil {
		t.Fatalf("load upf material: %v", err)
	}

	admf := newRecordingADMF(t, nil)
	enabler := newCCEnabler(store.New(), nil, nil)
	t.Cleanup(enabler.stop)

	cfg := &LiConfig{NEID: testNEID, TFID: testTFID, X1Listen: freePort(t)}
	tasks, err := startTriggerListener(cfg, upfMat.ServerTLS(), admf.reporter(), enabler,
		x2x3.NewIdentity(testNEID, upfInterceptionPoint), nil, nil)
	if err != nil {
		t.Fatalf("startTriggerListener: %v", err)
	}

	// A content task naming a session this element does not hold selects no traffic.
	task := types.InterceptTask{
		XID:      testXIDPrimary,
		Products: []types.ProductType{types.ProductCC},
		Targets:  []types.TargetIdentifier{{Type: types.TargetFSEID, Value: "14426627323429955319"}},
	}
	if !tasks.Activate(task) {
		t.Fatal("Activate failed")
	}
	enabler.reparse()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if admf.received(string(task.XID)) && admf.received("noTrafficSelected") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("no noTrafficSelected report for the held task reached the ADMF: the task-fault " +
		"watcher is not running")
}
