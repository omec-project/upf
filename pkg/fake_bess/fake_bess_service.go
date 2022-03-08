// SPDX-License-Identifier: Apache-2.0
// Copyright 2022 Open Networking Foundation

package fake_bess

import (
	"context"
	"github.com/omec-project/upf-epc/pfcpiface/bess_pb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"sync"
)

const (
	pdrLookupModuleName  = "pdrLookup"
	farLookupModuleName  = "farLookup"
	sessionQerModuleName = "sessionQERLookup"
	appQerModuleName     = "appQERLookup"
)

type fakeBessService struct {
	bess_pb.UnimplementedBESSControlServer
	reqs    []*bess_pb.CommandRequest
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
	if b.modules[name] == nil {
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

	log.Warn(request)

	m := b.unsafeGetOrAddModule(request.Name)
	if err := m.HandleRequest(request.Cmd, request.Arg); err != nil {
		return nil, err
	}

	b.reqs = append(b.reqs, request)

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
		log.Warn(wc)
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
		log.Warn(wc)
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
		log.Warn(em)
		var existing *bess_pb.ExactMatchCommandAddArg
		for _, et := range e.entries {
			if fieldsAreEqual(et.GetFields(), em.GetFields()) {
				existing = et
			}
		}
		if existing != nil {
			existing.Reset()
			proto.Merge(existing, em)
		} else {
			e.entries = append(e.entries, em)
		}
	} else if cmd == "delete" {
		em := &bess_pb.ExactMatchCommandDeleteArg{}
		err = arg.UnmarshalTo(em)
		if err != nil {
			return err
		}
		log.Warn(em)
		idx := -1
		for i, et := range e.entries {
			if fieldsAreEqual(et.GetFields(), em.GetFields()) {
				idx = i
			}
		}
		if idx == -1 {
			return status.Errorf(codes.NotFound, "entry not found: %v", em)
		} else {
			e.entries = append(e.entries[:idx], e.entries[idx+1:]...)
		}
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
		log.Warn(wc)
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
		log.Warn(qc)
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
	} else {
		panic("should not happen")
	}

	return nil
}

func isValidCommand(cmd string) bool {
	switch cmd {
	case "add":
		fallthrough
	case "delete":
		return true
	default:
		return false
	}
}
