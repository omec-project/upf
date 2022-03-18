// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package fake_bess

import (
	"context"
	"fmt"
	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	"github.com/omec-project/upf-epc/pkg/utils"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"math"
	"sync"
)

const (
	pdrLookupModuleName  = "pdrLookup"
	farLookupModuleName  = "farLookup"
	sessionQerModuleName = "sessionQERLookup"
	appQerModuleName     = "appQERLookup"
)

type FakePdr struct {
	srcIface     uint8
	srcIfaceMask uint8

	tunnelIP4Dst     uint32
	tunnelIP4DstMask uint32

	tunnelTEID     uint32
	tunnelTEIDMask uint32

	ueAddress uint32

	srcIP       uint32
	srcIPMask   uint32
	dstIP       uint32
	dstIPMask   uint32
	srcPort     uint16
	srcPortMask uint16
	dstPort     uint16
	dstPortMask uint16
	proto       uint8
	protoMask   uint8

	precedence  uint32
	fseID       uint64
	fseidIP     uint32
	ctrID       uint32
	farID       uint32
	qerID       uint32
	needDecap   uint8
	allocIPFlag bool

	// Public fields
	PdrID       uint32
	SrcPortLow  uint16
	SrcPortHigh uint16
	DstPortLow  uint16
	DstPortHigh uint16
}

func (p FakePdr) IsUplink() bool {
	return p.srcIface == 1
}

func (p FakePdr) IsDownlink() bool {
	return p.srcIface == 2
}

func (p FakePdr) String() string {
	return fmt.Sprintf("PDR(id=%v, F-SEID=%v, srcIface=%v, tunnelIPv4Dst=%v/%x, "+
		"tunnelTEID=%v/%x, ueAddress=%v, applicationFilter=%v, precedence=%v, F-SEID IP=%v, "+
		"counterID=%v, farID=%v, qerIDs=%v, needDecap=%v, allocIPFlag=%v)",
		p.PdrID, p.fseID, p.srcIface, utils.Uint32ToIp4(p.tunnelIP4Dst), p.tunnelIP4DstMask,
		p.tunnelTEID, p.tunnelTEIDMask, utils.Uint32ToIp4(p.ueAddress), "todo", p.precedence,
		p.fseidIP, p.ctrID, p.farID, p.qerID, p.needDecap, p.allocIPFlag)
}

type FakeFar struct {
	FarID   uint32
	fseID   uint64
	fseidIP uint32

	dstIntf       uint8
	sendEndMarker bool
	applyAction   uint8
	tunnelType    uint8
	tunnelIP4Src  uint32
	tunnelIP4Dst  uint32
	tunnelTEID    uint32
	tunnelPort    uint16
}

func (f FakeFar) String() string {
	return fmt.Sprintf("FAR(id=%v, F-SEID=%v, F-SEID IPv4=%v, dstInterface=%v, tunnelType=%v, "+
		"tunnelIPv4Src=%v, tunnelIPv4Dst=%v, tunnelTEID=%v, tunnelSrcPort=%v, "+
		"sendEndMarker=%v, drops=%v, forwards=%v, buffers=%v)", f.FarID, f.fseID, utils.Uint32ToIp4(f.fseidIP), f.dstIntf,
		f.tunnelType, utils.Uint32ToIp4(f.tunnelIP4Src), utils.Uint32ToIp4(f.tunnelIP4Dst), f.tunnelTEID, f.tunnelPort, f.sendEndMarker,
		f.Drops(), f.Forwards(), f.Buffers())
}

func (f *FakeFar) Drops() bool {
	return utils.Uint8Has1stBit(f.applyAction)
}

func (f *FakeFar) Forwards() bool {
	return utils.Uint8Has2ndBit(f.applyAction)
}

func (f *FakeFar) Buffers() bool {
	return utils.Uint8Has3rdBit(f.applyAction)
}

type FakeQer struct {
	QerID    uint32
	qosLevel uint8
	qfi      uint8
	ulStatus uint8
	dlStatus uint8
	ulMbr    uint64 // in kilobits/sec
	dlMbr    uint64 // in kilobits/sec
	ulGbr    uint64 // in kilobits/sec
	dlGbr    uint64 // in kilobits/sec
	fseID    uint64
	fseidIP  uint32
}

func (q FakeQer) String() string {
	return fmt.Sprintf("QER(id=%v, F-SEID=%v, F-SEID IP=%v, QFI=%v, "+
		"uplinkMBR=%v, downlinkMBR=%v, uplinkGBR=%v, downlinkGBR=%v, type=%v, "+
		"uplinkStatus=%v, downlinkStatus=%v)",
		q.QerID, q.fseID, q.fseidIP, q.qfi, q.ulMbr, q.dlMbr, q.ulGbr, q.dlGbr,
		q.qosLevel, q.ulStatus, q.dlStatus)
}

type fakeBessService struct {
	bess_pb.UnimplementedBESSControlServer
	modules map[string]module
	mtx     sync.Mutex
}

func newFakeBESSService() *fakeBessService {
	return &fakeBessService{
		modules: make(map[string]module),
	}
}

func (b *fakeBessService) GetOrAddModule(name string) module {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.unsafeGetOrAddModule(name)
}

func (b *fakeBessService) unsafeGetOrAddModule(name string) module {
	if _, ok := b.modules[name]; !ok {
		if name == pdrLookupModuleName {
			b.modules[name] = &wildcardModule{
				baseModule{name: name},
				nil,
			}
		} else if name == farLookupModuleName {
			b.modules[name] = &exactMatchModule{
				baseModule{name: name},
				nil,
			}
		} else if name == appQerModuleName || name == sessionQerModuleName {
			b.modules[name] = &qosModule{
				baseModule{name: name},
				nil,
			}
		} else {
			log.Fatalf("unknown module name: %v", name)
		}
	}
	return b.modules[name]
}

func (b *fakeBessService) GetPortStats(ctx context.Context, request *bess_pb.GetPortStatsRequest) (*bess_pb.GetPortStatsResponse, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	// TODO: implement it
	return &bess_pb.GetPortStatsResponse{}, nil
}

func (b *fakeBessService) ModuleCommand(ctx context.Context, request *bess_pb.CommandRequest) (*bess_pb.CommandResponse, error) {
	b.mtx.Lock()
	defer b.mtx.Unlock()

	m := b.unsafeGetOrAddModule(request.Name)
	if err := m.HandleRequest(request.Cmd, request.Arg); err != nil {
		return nil, err
	}

	return &bess_pb.CommandResponse{}, nil
}

func fieldsAreEqual(a, b []*bess_pb.FieldData) bool {
	if len(a) != len(b) {
		return false
	}

	for i := 0; i < len(a); i++ {
		if a[i].String() != b[i].String() {
			return false
		}
	}

	return true
}

func UnmarshalPdr(wc *bess_pb.WildcardMatchCommandAddArg) (p FakePdr) {
	p.needDecap = uint8(wc.Gate)
	p.precedence = uint32(math.MaxUint32 - wc.Priority)

	// Values
	p.srcIface = uint8(wc.Values[0].GetValueInt())
	p.tunnelIP4Dst = uint32(wc.Values[1].GetValueInt())
	p.tunnelTEID = uint32(wc.Values[2].GetValueInt())
	p.srcIP = uint32(wc.Values[3].GetValueInt())
	p.dstIP = uint32(wc.Values[4].GetValueInt())
	p.srcPort = uint16(wc.Values[5].GetValueInt())
	p.dstPort = uint16(wc.Values[6].GetValueInt())
	p.proto = uint8(wc.Values[7].GetValueInt())

	// Masks
	p.srcIfaceMask = uint8(wc.Masks[0].GetValueInt())
	p.tunnelIP4DstMask = uint32(wc.Masks[1].GetValueInt())
	p.tunnelTEIDMask = uint32(wc.Masks[2].GetValueInt())
	p.srcIPMask = uint32(wc.Masks[3].GetValueInt())
	p.dstIPMask = uint32(wc.Masks[4].GetValueInt())
	p.srcPortMask = uint16(wc.Masks[5].GetValueInt())
	p.dstPortMask = uint16(wc.Masks[6].GetValueInt())
	p.protoMask = uint8(wc.Masks[7].GetValueInt())

	// Valuesv
	p.PdrID = uint32(wc.Valuesv[0].GetValueInt())
	p.fseID = wc.Valuesv[1].GetValueInt()
	p.ctrID = uint32(wc.Valuesv[2].GetValueInt())
	p.qerID = uint32(wc.Valuesv[3].GetValueInt())
	p.farID = uint32(wc.Valuesv[4].GetValueInt())

	return
}

func UnmarshalFar(em *bess_pb.ExactMatchCommandAddArg) (f FakeFar) {
	// Fields.
	f.FarID = uint32(em.Fields[0].GetValueInt())
	f.fseID = em.Fields[1].GetValueInt()

	// Values.
	f.applyAction = uint8(em.Values[0].GetValueInt())
	f.tunnelType = uint8(em.Values[1].GetValueInt())
	f.tunnelIP4Src = uint32(em.Values[2].GetValueInt())
	f.tunnelIP4Dst = uint32(em.Values[3].GetValueInt())
	f.tunnelTEID = uint32(em.Values[4].GetValueInt())
	f.tunnelPort = uint16(em.Values[5].GetValueInt())

	return
}

func UnmarshalSessionQer(qc *bess_pb.QosCommandAddArg) (q FakeQer) {
	// Fields.
	// srcIface = uint32(qc.Fields[0].GetValueInt())
	q.fseID = qc.Fields[1].GetValueInt()

	return
}

func UnmarshalAppQer(qc *bess_pb.QosCommandAddArg) (q FakeQer) {
	// Fields.
	// srcIface = uint32(qc.Fields[0].GetValueInt())
	q.QerID = uint32(qc.Fields[1].GetValueInt())
	q.fseID = qc.Fields[2].GetValueInt()

	// Values
	q.qfi = uint8(qc.Values[0].GetValueInt())

	return
}

// Fake BESS module
type module interface {
	Name() string
	HandleRequest(cmd string, arg *anypb.Any) error
	GetState() []proto.Message
}

type baseModule struct {
	name string
}

func (b *baseModule) Name() string {
	return b.name
}

func (b *baseModule) HandleRequest(cmd string, arg *anypb.Any) (err error) {
	if !isValidCommand(cmd) {
		return status.Errorf(codes.InvalidArgument, "invalid command: %v", cmd)
	}

	return
}

type wildcardModule struct {
	baseModule
	entries []*bess_pb.WildcardMatchCommandAddArg
}

func (w *wildcardModule) GetState() (msgs []proto.Message) {
	for _, e := range w.entries {
		msgs = append(msgs, e)
	}
	return msgs
}

func (w *wildcardModule) HandleRequest(cmd string, arg *anypb.Any) (err error) {
	if err = w.baseModule.HandleRequest(cmd, arg); err != nil {
		return
	}

	log := log.WithField("module", w.Name()).WithField("cmd", cmd)

	if cmd == "add" {
		wc := &bess_pb.WildcardMatchCommandAddArg{}
		err = arg.UnmarshalTo(wc)
		if err != nil {
			return err
		}
		var existing *bess_pb.WildcardMatchCommandAddArg
		for _, e := range w.entries {
			if fieldsAreEqual(e.GetValues(), wc.GetValues()) &&
				fieldsAreEqual(e.GetMasks(), wc.GetMasks()) {
				existing = e
			}
		}
		if existing != nil {
			log.Tracef("updated existing entry %v", existing)
			existing.Reset()
			proto.Merge(existing, wc)
		} else {
			log.Tracef("added new entry %v", wc)
			w.entries = append(w.entries, wc)
		}
	} else if cmd == "delete" {
		wc := &bess_pb.WildcardMatchCommandDeleteArg{}
		err = arg.UnmarshalTo(wc)
		if err != nil {
			return err
		}
		idx := -1
		for i, e := range w.entries {
			if fieldsAreEqual(e.GetValues(), wc.GetValues()) &&
				fieldsAreEqual(e.GetMasks(), wc.GetMasks()) {
				idx = i
			}
		}
		if idx == -1 {
			return status.Errorf(codes.NotFound, "entry not found: %v", wc)
		} else {
			log.Tracef("deleted existing entry %v", w.entries[idx])
			w.entries = append(w.entries[:idx], w.entries[idx+1:]...)
		}
	} else if cmd == "clear" {
		wc := &bess_pb.WildcardMatchCommandClearArg{}
		err = arg.UnmarshalTo(wc)
		if err != nil {
			return err
		}
		// clear all rules
		w.entries = nil
	} else {
		panic("should not happen")
	}

	return nil
}

type exactMatchModule struct {
	baseModule
	entries []*bess_pb.ExactMatchCommandAddArg
}

func (e *exactMatchModule) GetState() (msgs []proto.Message) {
	for _, em := range e.entries {
		msgs = append(msgs, em)
	}
	return
}

func (e *exactMatchModule) HandleRequest(cmd string, arg *anypb.Any) (err error) {
	if err = e.baseModule.HandleRequest(cmd, arg); err != nil {
		return
	}

	log := log.WithField("module", e.Name()).WithField("cmd", cmd)

	if cmd == "add" {
		em := &bess_pb.ExactMatchCommandAddArg{}
		err = arg.UnmarshalTo(em)
		if err != nil {
			return err
		}
		var existing *bess_pb.ExactMatchCommandAddArg
		for _, et := range e.entries {
			if fieldsAreEqual(et.GetFields(), em.GetFields()) {
				existing = et
			}
		}
		if existing != nil {
			log.Tracef("updated existing entry %v", em)
			existing.Reset()
			proto.Merge(existing, em)
		} else {
			log.Tracef("added new entry %v", em)
			e.entries = append(e.entries, em)
		}
	} else if cmd == "delete" {
		em := &bess_pb.ExactMatchCommandDeleteArg{}
		err = arg.UnmarshalTo(em)
		if err != nil {
			return err
		}
		idx := -1
		for i, et := range e.entries {
			if fieldsAreEqual(et.GetFields(), em.GetFields()) {
				idx = i
			}
		}
		if idx == -1 {
			return status.Errorf(codes.NotFound, "entry not found: %v", em)
		} else {
			log.Tracef("deleted existing entry %v", e.entries[idx])
			e.entries = append(e.entries[:idx], e.entries[idx+1:]...)
		}
	} else if cmd == "clear" {
		em := &bess_pb.ExactMatchCommandClearArg{}
		err = arg.UnmarshalTo(em)
		if err != nil {
			return err
		}
		// clear all rules
		e.entries = nil
	} else {
		panic("should not happen")
	}

	return nil
}

type qosModule struct {
	baseModule
	entries []*bess_pb.QosCommandAddArg
}

func (q *qosModule) GetState() (msgs []proto.Message) {
	for _, em := range q.entries {
		msgs = append(msgs, em)
	}
	return
}

func (q *qosModule) HandleRequest(cmd string, arg *anypb.Any) (err error) {
	if err = q.baseModule.HandleRequest(cmd, arg); err != nil {
		return
	}

	log := log.WithField("module", q.Name()).WithField("cmd", cmd)

	if cmd == "add" {
		wc := &bess_pb.QosCommandAddArg{}
		err = arg.UnmarshalTo(wc)
		if err != nil {
			return err
		}
		var existing *bess_pb.QosCommandAddArg
		for _, e := range q.entries {
			if fieldsAreEqual(e.GetFields(), wc.GetFields()) {
				existing = e
			}
		}
		if existing != nil {
			log.Tracef("updated existing entry %v", existing)
			existing.Reset()
			proto.Merge(existing, wc)
		} else {
			log.Tracef("added new entry %v", wc)
			q.entries = append(q.entries, wc)
		}
	} else if cmd == "delete" {
		qc := &bess_pb.QosCommandDeleteArg{}
		err = arg.UnmarshalTo(qc)
		if err != nil {
			return err
		}
		idx := -1
		for i, e := range q.entries {
			if fieldsAreEqual(e.GetFields(), qc.GetFields()) {
				idx = i
			}
		}
		if idx == -1 {
			return status.Errorf(codes.NotFound, "entry not found: %v", qc)
		} else {
			log.Tracef("deleted existing entry %v", q.entries[idx])
			q.entries = append(q.entries[:idx], q.entries[idx+1:]...)
		}
	} else if cmd == "clear" {
		qc := &bess_pb.QosCommandClearArg{}
		err = arg.UnmarshalTo(qc)
		if err != nil {
			return err
		}
		// clear all rules
		q.entries = nil
	} else {
		panic("should not happen")
	}

	return nil
}

func isValidCommand(cmd string) bool {
	switch cmd {
	case "add":
		fallthrough
	case "clear":
		fallthrough
	case "delete":
		return true
	default:
		return false
	}
}
