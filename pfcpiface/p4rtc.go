// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/golang/protobuf/proto"
	grpcRetry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
)

//P4DeviceConfig ... Device config
type P4DeviceConfig []byte

const invalidID = 0

//Table Entry Function Type
const (
	FunctionTypeInsert uint8 = 1 //Insert table Entry Function
	FunctionTypeUpdate uint8 = 2 //Update table Entry Function
	FunctionTypeDelete uint8 = 3 //Delete table Entry Function
)

// IntfTableEntry ... Interface Table Entry API
type IntfTableEntry struct {
	IP        []byte
	PrefixLen int
	SrcIntf   string
	Direction string
}

// ActionParam ... Action Param API
type ActionParam struct {
	Len   uint32
	Name  string
	Value []byte
}

// MatchField .. Match Field API
type MatchField struct {
	Len       uint32
	PrefixLen uint32
	Name      string
	Value     []byte
	Mask      []byte
}

//IntfCounterEntry .. Counter entry function API
type IntfCounterEntry struct {
	CounterID uint64
	Index     uint64
	ByteCount []uint64
	PktCount  []uint64
}

//AppTableEntry .. Table entry function API
type AppTableEntry struct {
	FieldSize  uint32
	ParamSize  uint32
	TableName  string
	ActionName string
	Fields     []MatchField
	Params     []ActionParam
}

// P4rtClient ... P4 Runtime client object
type P4rtClient struct {
	Client     p4.P4RuntimeClient
	Conn       *grpc.ClientConn
	P4Info     p4ConfigV1.P4Info
	Stream     p4.P4Runtime_StreamChannelClient
	DeviceID   uint64
	ElectionID p4.Uint128
}

func (c *P4rtClient) tableID(name string) uint32 {
	for _, table := range c.P4Info.Tables {
		if table.Preamble.Name == name {
			return table.Preamble.Id
		}
	}
	return invalidID
}

/*func (c *P4rtClient) counterID(name string) uint32 {
	for _, counter := range c.P4Info.GetCounters() {
		if counter.Preamble.Name == name {
			return counter.Preamble.Id
		}
	}
	return invalidID
}
*/
func (c *P4rtClient) actionID(name string) uint32 {
	for _, action := range c.P4Info.Actions {
		if action.Preamble.Name == name {
			return action.Preamble.Id
		}
	}
	return invalidID
}

func (c *P4rtClient) getEnumVal(enumName string,
	valName string) ([]byte, error) {
	enumVal, ok := c.P4Info.TypeInfo.SerializableEnums[enumName]
	if !ok {
		err := fmt.Errorf("Enum Not found with name %s", enumName)
		return nil, err
	}

	for _, enums := range enumVal.Members {
		if enums.Name == valName {
			return enums.Value, nil
		}
	}

	err := fmt.Errorf("EnumVal not found")
	return nil, err
}

// CheckStatus ... Check client connection status
func (c *P4rtClient) CheckStatus() (state int) {
	return int(c.Conn.GetState())
}

// SetMastership .. API
func (c *P4rtClient) SetMastership(electionID p4.Uint128) (err error) {
	c.ElectionID = electionID
	mastershipReq := &p4.StreamMessageRequest{
		Update: &p4.StreamMessageRequest_Arbitration{
			Arbitration: &p4.MasterArbitrationUpdate{
				DeviceId:   1,
				ElectionId: &electionID,
			},
		},
	}
	err = c.Stream.Send(mastershipReq)
	return
}

// Init .. Initialize Client
func (c *P4rtClient) Init(timeout uint32, reportNotifyChan chan<- uint64) (err error) {
	// Initialize stream for mastership and packet I/O
	//ctx, cancel := context.WithTimeout(context.Background(),
	//                                   time.Duration(timeout) * time.Second)
	//defer cancel()
	c.Stream, err = c.Client.StreamChannel(
		context.Background(),
		grpcRetry.WithMax(3),
		grpcRetry.WithPerRetryTimeout(1*time.Second))
	if err != nil {
		log.Println("stream channel error: ", err)
		return
	}
	go func() {
		for {
			res, err := c.Stream.Recv()
			if err != nil {
				log.Println("stream recv error: ", err)
				return
			} else if arb := res.GetArbitration(); arb != nil {
				if code.Code(arb.Status.Code) == code.Code_OK {
					log.Println("client is master")
				} else {
					log.Println("client is not master")
				}
			} else if dig := res.GetDigest(); dig != nil {
				log.Println("Received Digest")
				for _, p4d := range dig.GetData() {
					if fseidStr := p4d.GetBitstring(); fseidStr != nil {
						log.Println("fseid data in digest")
						fseid := binary.BigEndian.Uint64(fseidStr[4:])
						reportNotifyChan <- fseid
					}
					/*if structVal := p4d.GetStruct(); structVal != nil {
						log.Println("Struct data in digest")
						for _, memberVal := range structVal.GetMembers() {
							fseid := memberVal.GetBitstring()
							c.reportDigest <- fseid
						}
					}*/
				}
			} else {
				log.Println("stream recv: ", res)
			}

		}
	}()

	/*
		    select {
		   	    case <-ctx.Done():
				log.Println(ctx.Err()) // prints "context deadline exceeded"
			}*/

	log.Println("exited from recv thread.")
	return
}

// WriteFarTable .. Write far table entry API
func (c *P4rtClient) WriteFarTable(
	farEntry far, funcType uint8) error {

	log.Println("WriteFarTable.")
	te := AppTableEntry{
		TableName: "PreQosPipe.load_far_attributes",
	}

	te.FieldSize = 2
	te.Fields = make([]MatchField, te.FieldSize)
	te.Fields[0].Name = "far_id"

	te.Fields[0].Value = make([]byte, 4)
	binary.BigEndian.PutUint32(te.Fields[0].Value, uint32(farEntry.farID))

	te.Fields[1].Name = "session_id"
	fseidVal := make([]byte, 12)
	binary.BigEndian.PutUint32(fseidVal[:4], farEntry.fseidIP)
	binary.BigEndian.PutUint64(fseidVal[4:], farEntry.fseID)
	te.Fields[1].Value = make([]byte, 12)
	copy(te.Fields[1].Value, fseidVal)

	var prio int32
	if funcType == FunctionTypeDelete {
		te.ActionName = "NoAction"
		te.ParamSize = 0
		go func() {
			ret := c.InsertTableEntry(te, funcType, prio)
			if ret != nil {
				log.Println("Insert Table entry error : ", ret)
			}
		}()
		return nil
	} else if funcType == FunctionTypeInsert {
		te.ActionName = "PreQosPipe.load_normal_far_attributes"
		te.ParamSize = 2
		te.Params = make([]ActionParam, te.ParamSize)
		te.Params[0].Name = "needs_dropping"
		te.Params[0].Value = make([]byte, 1)
		te.Params[0].Value[0] = byte(farEntry.applyAction & 0x01)
		te.Params[1].Name = "notify_cp"
		te.Params[1].Value = make([]byte, 1)
		te.Params[1].Value[0] = byte(farEntry.applyAction & 0x08)
	} else if funcType == FunctionTypeUpdate {

		te.ActionName = "PreQosPipe.load_tunnel_far_attributes"
		te.ParamSize = 8
		te.Params = make([]ActionParam, te.ParamSize)
		te.Params[0].Name = "needs_dropping"
		te.Params[0].Value = make([]byte, 1)
		te.Params[0].Value[0] = byte(farEntry.applyAction & 0x01)
		te.Params[1].Name = "notify_cp"
		te.Params[1].Value = make([]byte, 1)
		if (farEntry.applyAction & 0x08) != 0 {
			te.Params[1].Value[0] = byte(0x01)
		}
		te.Params[2].Name = "needs_buffering"
		te.Params[2].Value = make([]byte, 1)
		if (farEntry.applyAction & 0x04) != 0 {
			te.Params[2].Value[0] = byte(0x01)
		}
		te.Params[3].Name = "src_addr"
		te.Params[3].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Params[3].Value, farEntry.tunnelIP4Src)
		te.Params[4].Name = "dst_addr"
		te.Params[4].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Params[4].Value, farEntry.tunnelIP4Dst)
		te.Params[5].Name = "teid"
		te.Params[5].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Params[5].Value, farEntry.tunnelTEID)
		te.Params[6].Name = "sport"
		te.Params[6].Value = make([]byte, 2)
		binary.BigEndian.PutUint16(te.Params[6].Value, farEntry.tunnelPort)
		te.Params[7].Name = "tunnel_type"
		enumName := "TunnelType"
		var tunnelStr string
		switch farEntry.tunnelType {
		case 0x01:
			tunnelStr = "GTPU"
		default:
			log.Println("Unknown tunneling not handled in p4rt.")
			return nil
		}

		val, err := c.getEnumVal(enumName, tunnelStr)
		if err != nil {
			log.Println("Could not find enum val ", err)
			return err
		}
		te.Params[7].Value = make([]byte, 1)
		te.Params[7].Value[0] = val[0]
	}

	return c.InsertTableEntry(te, funcType, prio)
}

// WritePdrTable .. Write pdr table entry API
func (c *P4rtClient) WritePdrTable(
	pdrEntry pdr, funcType uint8) error {

	log.Println("WritePdrTable.")
	te := AppTableEntry{
		TableName:  "PreQosPipe.pdrs",
		ActionName: "PreQosPipe.set_pdr_attributes",
	}

	te.FieldSize = 4
	te.Fields = make([]MatchField, te.FieldSize)
	te.FieldSize = 2
	te.Fields[0].Name = "src_iface"
	enumName := "InterfaceType"
	var srcIntfStr string
	var decapVal uint8
	if pdrEntry.srcIface == access {
		srcIntfStr = "ACCESS"
		decapVal = 1
	} else {
		srcIntfStr = "CORE"
	}

	val, _ := c.getEnumVal(enumName, srcIntfStr)
	te.Fields[0].Value = val

	if pdrEntry.srcIface == access {
		te.FieldSize = 3
		te.Fields[1].Name = "teid"
		te.Fields[1].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Fields[1].Value, pdrEntry.tunnelTEID)
		te.Fields[1].Mask = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Fields[1].Mask, pdrEntry.tunnelTEIDMask)
		//te.Fields[2].Mask =  b

		te.Fields[2].Name = "tunnel_ipv4_dst"
		te.Fields[2].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Fields[2].Value, pdrEntry.tunnelIP4Dst)
		te.Fields[2].Mask = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Fields[2].Mask, pdrEntry.tunnelIP4DstMask)
	} else if pdrEntry.srcIface == core {
		te.Fields[1].Name = "ue_addr"
		te.Fields[1].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Fields[1].Value, pdrEntry.dstIP)
		te.Fields[1].Mask = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Fields[1].Mask, pdrEntry.dstIPMask)
	}

	var prio int32 = 2
	if funcType == FunctionTypeDelete {
		te.ActionName = "NoAction"
		te.ParamSize = 0
		go func() {
			ret := c.InsertTableEntry(te, funcType, prio)
			if ret != nil {
				log.Println("Insert Table entry error : ", ret)
			}
		}()
		return nil
	} else if funcType == FunctionTypeInsert {

		te.ParamSize = 5
		te.Params = make([]ActionParam, te.ParamSize)
		te.Params[0].Name = "id"
		te.Params[0].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Params[0].Value, pdrEntry.pdrID)

		te.Params[1].Name = "fseid"
		fseidVal := make([]byte, 12)
		binary.BigEndian.PutUint32(fseidVal[:4], pdrEntry.fseidIP)
		binary.BigEndian.PutUint64(fseidVal[4:], pdrEntry.fseID)
		te.Params[1].Value = make([]byte, 12)
		copy(te.Params[1].Value, fseidVal)

		te.Params[2].Name = "ctr_id"
		te.Params[2].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Params[2].Value, pdrEntry.ctrID)

		te.Params[3].Name = "far_id"
		te.Params[3].Value = make([]byte, 4)
		binary.BigEndian.PutUint32(te.Params[3].Value, uint32(pdrEntry.farID))

		te.Params[4].Name = "needs_gtpu_decap"
		te.Params[4].Value = make([]byte, 1)
		te.Params[4].Value[0] = byte(decapVal)
	}

	return c.InsertTableEntry(te, funcType, prio)
}

//WriteInterfaceTable ... Write Interface table Entry
func (c *P4rtClient) WriteInterfaceTable(
	intfEntry IntfTableEntry,
	funcType uint8) error {

	log.Println("WriteInterfaceTable.")
	te := AppTableEntry{
		TableName:  "PreQosPipe.source_iface_lookup",
		ActionName: "PreQosPipe.set_source_iface",
	}

	te.FieldSize = 1
	te.Fields = make([]MatchField, 1)
	te.Fields[0].Name = "ipv4_dst_prefix"
	te.Fields[0].Value = intfEntry.IP
	te.Fields[0].PrefixLen = uint32(intfEntry.PrefixLen)

	te.ParamSize = 2
	te.Params = make([]ActionParam, 2)
	te.Params[0].Name = "src_iface"
	enumName := "InterfaceType"
	val, err := c.getEnumVal(enumName, intfEntry.SrcIntf)
	if err != nil {
		log.Println("Could not find enum val ", err)
		return err
	}
	te.Params[0].Value = val

	te.Params[1].Name = "direction"
	enumName = "Direction"
	val, err = c.getEnumVal(enumName, intfEntry.Direction)
	if err != nil {
		log.Println("Could not find enum val ", err)
		return nil
	}
	te.Params[1].Value = val

	var prio int32
	return c.InsertTableEntry(te, funcType, prio)
}

func (c *P4rtClient) getCounterValue(entity *p4.Entity,
	ce *IntfCounterEntry) error {
	entry := entity.GetCounterEntry()
	index := uint64(entry.GetIndex().Index)
	byteCount := uint64(entry.GetData().ByteCount)
	pktCount := uint64(entry.GetData().PacketCount)
	ce.ByteCount[index] = byteCount
	ce.PktCount[index] = pktCount
	//log.Println("index , bytecount, pktcount ", index, byteCount, pktCount)
	return nil
}

func (c *P4rtClient) getFieldValue(entity *p4.Entity,
	te AppTableEntry) (*MatchField, error) {
	log.Println("get Field Value")
	entry := entity.GetTableEntry()
	tableID := c.tableID(te.TableName)
	actionID := c.actionID(te.ActionName)
	inputField := te.Fields[0]
	inputParam := te.Params[0]
	if (entry.TableId != tableID) ||
		(entry.Action.GetAction().ActionId != actionID) {
		err := log.Output(1, "Inavlid tableID / ActionID.")
		return nil, err
	}

	var matchType p4ConfigV1.MatchField_MatchType
	var fieldID uint32
	var paramID uint32
	for _, tables := range c.P4Info.Tables {
		if tables.Preamble.Id == tableID {
			for _, fields := range tables.MatchFields {
				if fields.Name == inputField.Name {
					log.Println("field name match found.")
					matchType = fields.GetMatchType()
					fieldID = fields.Id
					break
				}
			}

			if matchType != 0 {
				break
			}
		}
	}

	for _, actions := range c.P4Info.Actions {
		if actions.Preamble.Id == actionID {
			for _, params := range actions.Params {
				if params.Name == inputParam.Name {
					log.Println("field name match found.")
					paramID = params.Id
					break
				}
			}
		}

		if paramID != 0 {
			break
		}
	}

	log.Println("ParamId FieldID ", paramID, fieldID)
	for _, params := range entry.Action.GetAction().Params {
		log.Println("ParamId recvd ", params.ParamId)
		log.Println("Param value ", params.Value)
		log.Println("inputParam value ", inputParam.Value)
		if params.ParamId == paramID &&
			(bytes.Equal(params.Value, inputParam.Value)) {
			log.Println("Param matched")
			for _, fields := range entry.Match {
				if fields.FieldId == fieldID {
					log.Println("field name match found ", inputField.Name)
					switch matchType {
					case p4ConfigV1.MatchField_EXACT:
						{
							exact := fields.GetExact()
							inputField.Value = make([]byte, len(exact.Value))
							copy(inputField.Value, exact.Value)
						}
					case p4ConfigV1.MatchField_LPM:
						{
							lpm := fields.GetLpm()
							inputField.Value = make([]byte, len(lpm.Value))
							copy(inputField.Value, lpm.Value)
							inputField.PrefixLen = uint32(lpm.PrefixLen)
						}
					case p4ConfigV1.MatchField_TERNARY:
						{
							tern := fields.GetTernary()
							inputField.Value = tern.Value
							inputField.Mask = tern.Mask
						}
					case p4ConfigV1.MatchField_RANGE:
						{
							rangeVal := fields.GetRange()
							inputField.Value = rangeVal.Low
							inputField.Mask = rangeVal.High
						}
					default:
						log.Println("Unknown MatchType.")
						err := fmt.Errorf("Unknown MatchType for FieldMatch")
						return nil, err
					}

					log.Println("Field value found.")
					return &inputField, nil
				}
			}
		}
	}

	err := fmt.Errorf("getField Value failed")
	return nil, err
}

func (c *P4rtClient) addFieldValue(entry *p4.TableEntry, field MatchField,
	tableID uint32) error {
	//log.Println("add Match field")
	fieldVal := &p4.FieldMatch{
		FieldId: 0,
	}

	for _, tables := range c.P4Info.Tables {
		if tables.Preamble.Id == tableID {
			for _, fields := range tables.MatchFields {
				if fields.Name == field.Name {
					//log.Println("field name match found.")
					fieldVal.FieldId = fields.Id
					switch fields.GetMatchType() {
					case p4ConfigV1.MatchField_EXACT:
						{
							exact := &p4.FieldMatch_Exact{
								Value: field.Value,
							}
							fieldVal.FieldMatchType = &p4.FieldMatch_Exact_{Exact: exact}
						}
					case p4ConfigV1.MatchField_LPM:
						{
							lpm := &p4.FieldMatch_LPM{
								Value:     field.Value,
								PrefixLen: int32(field.PrefixLen),
							}
							fieldVal.FieldMatchType = &p4.FieldMatch_Lpm{Lpm: lpm}
						}
					case p4ConfigV1.MatchField_TERNARY:
						{
							tern := &p4.FieldMatch_Ternary{
								Value: field.Value,
								Mask:  field.Mask,
							}
							fieldVal.FieldMatchType =
								&p4.FieldMatch_Ternary_{Ternary: tern}
						}
					case p4ConfigV1.MatchField_RANGE:
						{
							rangeVal := &p4.FieldMatch_Range{
								Low:  field.Value,
								High: field.Mask,
							}
							fieldVal.FieldMatchType =
								&p4.FieldMatch_Range_{Range: rangeVal}
						}
					default:
						log.Println("Unknown MatchType.")
						err := fmt.Errorf("Unknown MatchType for FieldMatch")
						return err
					}

					entry.Match = append(entry.Match, fieldVal)
					return nil
				}
			}
		}
	}

	err := fmt.Errorf("addField Value failed")
	return err
}

func (c *P4rtClient) addActionValue(action *p4.Action, param ActionParam,
	actionID uint32) error {
	//log.Println("add action param value")

	for _, actions := range c.P4Info.Actions {
		if actions.Preamble.Id == actionID {
			for _, params := range actions.Params {
				if params.Name == param.Name {
					paramVal := &p4.Action_Param{
						ParamId: params.Id,
						Value:   param.Value,
					}
					action.Params = append(action.Params, paramVal)
					return nil
				}
			}
		}
	}

	err := fmt.Errorf("addAction Value failed")
	return err
}

//ReadCounter ... Read Counter entry
func (c *P4rtClient) ReadCounter(ce *IntfCounterEntry) error {

	log.Println("ReadCounter ID : ", ce.CounterID)

	readRes, err := c.ReadCounterEntry(ce)
	if err != nil {
		log.Println("Read Counters failed ", err)
		return err
	}

	//log.Println(proto.MarshalTextString(readRes))
	for _, ent := range readRes.GetEntities() {
		err := c.getCounterValue(ent, ce)
		if err != nil {
			log.Println("getCounterValue failed ", err)
			continue
		}
	}

	return nil
}

//ReadCounterEntry .. Read counter Entry
func (c *P4rtClient) ReadCounterEntry(ce *IntfCounterEntry) (*p4.ReadResponse, error) {

	//log.Println("Read Counter Entry")

	var index p4.Index
	index.Index = int64(ce.Index)
	var entry p4.CounterEntry
	entry.CounterId = uint32(ce.CounterID)
	//entry.Index = &index
	var entity p4.Entity
	var ctrEntry p4.Entity_CounterEntry
	ctrEntry.CounterEntry = &entry
	entity.Entity = &ctrEntry
	/*
		index := &p4.Index{
			Index: int64(ce.Index),
		}
		entry := &p4.CounterEntry{
			CounterId: uint32(ce.CounterID),
			Index:     index,
		}

		entity := &p4.Entity{
			Entity: &p4.Entity_CounterEntry{CounterEntry: entry},
		}*/
	//log.Println(proto.MarshalTextString(&entity))
	return c.ReadReq(&entity)
}

//ClearFarTable ... Clear FAR Table
func (c *P4rtClient) ClearFarTable() error {

	log.Println("ClearFarTable.")
	te := AppTableEntry{
		TableName: "PreQosPipe.load_far_attributes",
	}

	var prio int32
	readRes, err := c.ReadTableEntry(te, prio)
	if err != nil {
		log.Println("Read FAR table failed ", err)
		return err
	}

	updates := make([]*p4.Update, len(readRes.GetEntities()))
	for _, ent := range readRes.GetEntities() {
		updateType := p4.Update_DELETE
		update := &p4.Update{
			Type:   updateType,
			Entity: ent,
		}

		updates = append(updates, update)
	}

	go func() {
		errin := c.WriteBatchReq(updates)
		if errin != nil {
			log.Println("far delete write failed ", errin)
		}
	}()

	return nil
}

//ClearPdrTable ... Clear PDR Table
func (c *P4rtClient) ClearPdrTable() error {

	log.Println("ClearPdrTable.")
	te := AppTableEntry{
		TableName: "PreQosPipe.pdrs",
	}

	var prio int32
	readRes, err := c.ReadTableEntry(te, prio)
	if err != nil {
		log.Println("Read Pdr table failed ", err)
		return err
	}

	updates := make([]*p4.Update, len(readRes.GetEntities()))
	for _, ent := range readRes.GetEntities() {
		updateType := p4.Update_DELETE
		update := &p4.Update{
			Type:   updateType,
			Entity: ent,
		}

		updates = append(updates, update)
	}

	go func() {
		errin := c.WriteBatchReq(updates)
		if errin != nil {
			log.Println("pdr delete write failed ", errin)
		}
	}()

	return nil
}

//ReadInterfaceTable ... Read Interface table Entry
func (c *P4rtClient) ReadInterfaceTable(
	intfEntry *IntfTableEntry) error {

	log.Println("ReadInterfaceTable.")
	te := AppTableEntry{
		TableName:  "PreQosPipe.source_iface_lookup",
		ActionName: "PreQosPipe.set_source_iface",
	}

	te.FieldSize = 1
	te.Fields = make([]MatchField, 1)
	te.Fields[0].Name = "ipv4_dst_prefix"

	te.ParamSize = 2
	te.Params = make([]ActionParam, 2)
	te.Params[0].Name = "src_iface"
	enumName := "InterfaceType"
	val, err := c.getEnumVal(enumName, intfEntry.SrcIntf)
	if err != nil {
		log.Println("Could not find enum val ", err)
		return err
	}
	te.Params[0].Value = val

	te.Params[1].Name = "direction"
	enumName = "Direction"
	val, err = c.getEnumVal(enumName, intfEntry.Direction)
	if err != nil {
		log.Println("Could not find enum val ", err)
		return err
	}
	te.Params[1].Value = val

	var prio int32
	readRes, err := c.ReadTableEntry(te, prio)
	if err != nil {
		log.Println("Read Interface table failed ", err)
		return err
	}

	for _, ent := range readRes.GetEntities() {
		field, err := c.getFieldValue(ent, te)
		if err != nil {
			log.Println("getFieldValue failed ", err)
			continue
		}

		intfEntry.IP = make([]byte, len(field.Value))
		copy(intfEntry.IP, field.Value)
		log.Println("ip , fieldval ", intfEntry.IP, field.Value)
		intfEntry.PrefixLen = int(field.PrefixLen)
		return nil
	}

	err = fmt.Errorf("ReadInterfaceTable failed")
	return err
}

//ReadTableEntry ... Read table Entry
func (c *P4rtClient) ReadTableEntry(
	tableEntry AppTableEntry, prio int32) (*p4.ReadResponse, error) {

	log.Println("Read Table Entry for Table ", tableEntry.TableName)
	tableID := c.tableID(tableEntry.TableName)

	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: prio,
	}

	entity := &p4.Entity{
		Entity: &p4.Entity_TableEntry{TableEntry: entry},
	}
	//log.Println(proto.MarshalTextString(entity))
	return c.ReadReq(entity)
}

//ReadReqEntities ... Read request Entity
func (c *P4rtClient) ReadReqEntities(entities []*p4.Entity) (*p4.ReadResponse, error) {
	req := &p4.ReadRequest{
		DeviceId: c.DeviceID,
		Entities: entities,
	}
	//log.Println(proto.MarshalTextString(req))
	readClient, err := c.Client.Read(context.Background(), req)
	if err == nil {
		readRes, err := readClient.Recv()
		if err == nil {
			//log.Println(proto.MarshalTextString(readRes))
			return readRes, nil
		}
	}
	return nil, err
}

// ReadReq ... Read Request
func (c *P4rtClient) ReadReq(entity *p4.Entity) (*p4.ReadResponse, error) {
	var req p4.ReadRequest
	req.DeviceId = c.DeviceID
	req.Entities = []*p4.Entity{entity}

	ctx, cancel := context.WithTimeout(context.Background(),
		2*time.Second)
	defer cancel()

	//log.Println(proto.MarshalTextString(&req))
	readClient, err := c.Client.Read(ctx, &req)
	if err == nil {
		readRes, err := readClient.Recv()
		if err == nil {
			//log.Println(proto.MarshalTextString(readRes))
			return readRes, nil
		}
	}
	return nil, err
}

//InsertTableEntry .. Insert table Entry
func (c *P4rtClient) InsertTableEntry(
	tableEntry AppTableEntry,
	funcType uint8, prio int32) error {

	log.Println("Insert Table Entry for Table ", tableEntry.TableName)
	tableID := c.tableID(tableEntry.TableName)
	actionID := c.actionID(tableEntry.ActionName)
	directAction := &p4.Action{
		ActionId: actionID,
	}

	log.Println("adding action params.")
	for _, p := range tableEntry.Params {
		err := c.addActionValue(directAction, p, actionID)
		if err != nil {
			log.Println("AddActionValue failed ", err)
			return err
		}
	}

	tableAction := &p4.TableAction{
		Type: &p4.TableAction_Action{Action: directAction},
	}

	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: prio,
		Action:   tableAction,
	}

	for count, mf := range tableEntry.Fields {
		if uint32(count) >= tableEntry.FieldSize {
			break
		}
		err := c.addFieldValue(entry, mf, tableID)
		if err != nil {
			return err
		}
	}

	var updateType p4.Update_Type
	if funcType == FunctionTypeUpdate {
		updateType = p4.Update_MODIFY
	} else if funcType == FunctionTypeInsert {
		updateType = p4.Update_INSERT
	} else if funcType == FunctionTypeDelete {
		updateType = p4.Update_DELETE
	}

	update := &p4.Update{
		Type: updateType,
		Entity: &p4.Entity{
			Entity: &p4.Entity_TableEntry{TableEntry: entry},
		},
	}

	//log.Println(proto.MarshalTextString(update))
	return c.WriteReq(update)
}

// WriteReq ... Write Request
func (c *P4rtClient) WriteReq(update *p4.Update) error {
	req := &p4.WriteRequest{
		DeviceId:   c.DeviceID,
		ElectionId: &c.ElectionID,
		Updates:    []*p4.Update{update},
	}
	_, err := c.Client.Write(context.Background(), req)
	return err
}

// WriteBatchReq ... Write batch Request to up4
func (c *P4rtClient) WriteBatchReq(updates []*p4.Update) error {
	req := &p4.WriteRequest{
		DeviceId:   c.DeviceID,
		ElectionId: &c.ElectionID,
	}

	req.Updates = append(req.Updates, updates...)

	//log.Println(proto.MarshalTextString(req))
	_, err := c.Client.Write(context.Background(), req)
	return err
}

// GetForwardingPipelineConfig ... Get Pipeline config from switch
func (c *P4rtClient) GetForwardingPipelineConfig() (err error) {
	log.Println("GetForwardingPipelineConfig")
	pipeline, err := GetPipelineConfig(c.Client, c.DeviceID)
	if err != nil {
		log.Println("set pipeline config error ", err)
		return
	}

	c.P4Info = *pipeline.Config.P4Info
	return
}

// GetPipelineConfig ... Set pipeline config
func GetPipelineConfig(client p4.P4RuntimeClient, deviceID uint64) (*p4.GetForwardingPipelineConfigResponse, error) {
	req := &p4.GetForwardingPipelineConfigRequest{
		DeviceId:     deviceID,
		ResponseType: p4.GetForwardingPipelineConfigRequest_P4INFO_AND_COOKIE,
	}

	configRes, err := client.GetForwardingPipelineConfig(context.Background(), req)
	if err != nil {
		log.Println("get forwarding pipeline returned error ", err)
		return nil, err
	}
	return configRes, nil
}

//SetForwardingPipelineConfig ..
func (c *P4rtClient) SetForwardingPipelineConfig(p4InfoPath, deviceConfigPath string) (err error) {
	log.Println("P4 Info: ", p4InfoPath)

	p4infoBytes, err := ioutil.ReadFile(p4InfoPath)
	if err != nil {
		log.Println("Read p4info file error ", err)
		return
	}

	var p4info p4ConfigV1.P4Info
	err = proto.UnmarshalText(string(p4infoBytes), &p4info)
	if err != nil {
		log.Println("Unmarshal test failed for p4info ", err)
		return
	}

	c.P4Info = p4info
	deviceConfig, err := LoadDeviceConfig(deviceConfigPath)
	if err != nil {
		log.Println("bmv2 json read failed ", err)
		return
	}

	var pipeline p4.ForwardingPipelineConfig
	pipeline.P4Info = &p4info
	pipeline.P4DeviceConfig = deviceConfig

	err = SetPipelineConfig(c.Client, c.DeviceID, &c.ElectionID, &pipeline)
	if err != nil {
		log.Println("set pipeline config error ", err)
		return
	}
	return
}

// SetPipelineConfig ... Set pipeline config
func SetPipelineConfig(client p4.P4RuntimeClient, deviceID uint64, electionID *p4.Uint128, config *p4.ForwardingPipelineConfig) error {
	req := &p4.SetForwardingPipelineConfigRequest{
		DeviceId:   deviceID,
		RoleId:     0,
		ElectionId: electionID,
		Action:     p4.SetForwardingPipelineConfigRequest_VERIFY_AND_COMMIT,
		Config:     config,
	}
	_, err := client.SetForwardingPipelineConfig(context.Background(), req)
	if err != nil {
		log.Println("set forwarding pipeline returned error ", err)
	}
	return err
}

//GetConnection ... Get Grpc connection
func GetConnection(host string) (conn *grpc.ClientConn, err error) {
	/* get connection */
	log.Println("Get connection.")
	conn, err = grpc.Dial(host, grpc.WithInsecure())
	if err != nil {
		log.Println("grpc dial err: ", err)
		return nil, err
	}
	return
}

// LoadDeviceConfig : Load Device config
func LoadDeviceConfig(deviceConfigPath string) (P4DeviceConfig, error) {
	log.Println("BMv2 JSON: ", deviceConfigPath)

	deviceConfig, err := os.Open(deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %v", deviceConfigPath, err)
	}
	defer deviceConfig.Close()
	bmv2Info, err := deviceConfig.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %v", deviceConfigPath, err)
	}

	bin := make([]byte, int(bmv2Info.Size()))
	if b, err := deviceConfig.Read(bin); err != nil {
		return nil, fmt.Errorf("read %s: %v", deviceConfigPath, err)
	} else if b != int(bmv2Info.Size()) {
		return nil, errors.New("bmv2 bin copy failed")
	}

	return bin, nil
}

//CreateChannel ... Create p4runtime client channel
func CreateChannel(host string,
	deviceID uint64,
	timeout uint32,
	reportNotifyChan chan<- uint64) (*P4rtClient, error) {
	log.Println("create channel")
	// Second, check to see if we can reuse the gRPC connection for a new P4RT client
	conn, err := GetConnection(host)
	if err != nil {
		log.Println("grpc connection failed")
		return nil, err
	}

	client := &P4rtClient{
		Client:   p4.NewP4RuntimeClient(conn),
		Conn:     conn,
		DeviceID: deviceID,
	}

	err = client.Init(timeout, reportNotifyChan)
	if err != nil {
		log.Println("Client Init error: ", err)
		return nil, err
	}

	err = client.SetMastership(p4.Uint128{High: 0, Low: 1})
	if err != nil {
		log.Println("Set Mastership error: ", err)
		return nil, err
	}

	return client, nil
}
