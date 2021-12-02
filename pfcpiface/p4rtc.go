// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"google.golang.org/grpc/credentials/insecure"

	//nolint:staticcheck // Ignore SA1019.
	// Upgrading to google.golang.org/protobuf/proto is not a drop-in replacement,
	// as also P4Runtime stubs are based on the deprecated proto.
	"github.com/golang/protobuf/proto"
	grpcRetry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	p4ConfigV1 "github.com/p4lang/p4runtime/go/p4/config/v1"
	p4 "github.com/p4lang/p4runtime/go/p4/v1"
	log "github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
)

// P4DeviceConfig ... Device config.
type P4DeviceConfig []byte

const invalidID = 0

// TODO: use iota
// Table Entry Function Type.
const (
	FunctionTypeInsert uint8  = 1               // Insert table Entry Function
	FunctionTypeUpdate uint8  = 2               // Update table Entry Function
	FunctionTypeDelete uint8  = 3               // Delete table Entry Function
	InterfaceTypeStr   string = "InterfaceType" // Interface Type field name"
)

// P4rtClient ... P4 Runtime client object.
type P4rtClient struct {
	client     p4.P4RuntimeClient
	conn       *grpc.ClientConn
	stream     p4.P4Runtime_StreamChannelClient
	electionID p4.Uint128

	// exported fields
	P4Info     p4ConfigV1.P4Info
	DeviceID   uint64
}

// CheckStatus ... Check client connection status.
func (c *P4rtClient) CheckStatus() (state int) {
	return int(c.conn.GetState())
}

// SetMastership .. API.
func (c *P4rtClient) SetMastership(electionID p4.Uint128) (err error) {
	c.electionID = electionID
	mastershipReq := &p4.StreamMessageRequest{
		Update: &p4.StreamMessageRequest_Arbitration{
			Arbitration: &p4.MasterArbitrationUpdate{
				DeviceId:   1,
				ElectionId: &electionID,
			},
		},
	}
	err = c.stream.Send(mastershipReq)

	return
}

// SendPacketOut .. send packet out p4 server.
func (c *P4rtClient) SendPacketOut(packet []byte) (err error) {
	pktOutReq := &p4.StreamMessageRequest{
		Update: &p4.StreamMessageRequest_Packet{
			Packet: &p4.PacketOut{
				Payload: packet,
			},
		},
	}
	err = c.stream.Send(pktOutReq)

	return err
}

// Init .. Initialize Client.
func (c *P4rtClient) Init(reportNotifyChan chan<- uint64) (err error) {
	// Initialize stream for mastership and packet I/O
	// ctx, cancel := context.WithTimeout(context.Background(),
	//                                   time.Duration(timeout) * time.Second)
	// defer cancel()
	c.stream, err = c.client.StreamChannel(
		context.Background(),
		grpcRetry.WithMax(3),
		grpcRetry.WithPerRetryTimeout(1*time.Second))
	if err != nil {
		log.Println("stream channel error: ", err)
		return
	}

	go func() {
		for {
			res, err := c.stream.Recv()
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
		}
	*/

	log.Println("exited from recv thread.")

	return
}

//// WriteFarTable .. Write far table entry API.
//func (c *P4rtClient) WriteFarTable(farEntry far, funcType uint8) error {
//	log.Println("WriteFarTable.")
//
//	te := AppTableEntry{
//		TableName: "PreQosPipe.load_far_attributes",
//	}
//
//	te.FieldSize = 2
//	te.Fields = make([]MatchField, te.FieldSize)
//	te.Fields[0].Name = "far_id"
//
//	te.Fields[0].Value = make([]byte, 4)
//	binary.BigEndian.PutUint32(te.Fields[0].Value, farEntry.farID)
//
//	te.Fields[1].Name = "session_id"
//	te.Fields[1].Value = make([]byte, 12)
//
//	fseidVal := make([]byte, 12)
//	binary.BigEndian.PutUint32(fseidVal[:4], farEntry.fseidIP)
//	binary.BigEndian.PutUint64(fseidVal[4:], farEntry.fseID)
//
//	copy(te.Fields[1].Value, fseidVal)
//
//	var prio int32
//
//	if funcType == FunctionTypeDelete {
//		te.ActionName = "NoAction"
//		te.ParamSize = 0
//
//		go func() {
//			ret := c.InsertTableEntry(te, funcType, prio)
//			if ret != nil {
//				log.Println("Insert Table entry error : ", ret)
//			}
//		}()
//
//		return nil
//	} else if funcType == FunctionTypeInsert {
//		te.ActionName = "PreQosPipe.load_normal_far_attributes"
//		te.ParamSize = 2
//		te.Params = make([]ActionParam, te.ParamSize)
//		te.Params[0].Name = "needs_dropping"
//		te.Params[0].Value = make([]byte, 1)
//		te.Params[0].Value[0] = farEntry.applyAction & 0x01
//		te.Params[1].Name = "notify_cp"
//		te.Params[1].Value = make([]byte, 1)
//		te.Params[1].Value[0] = farEntry.applyAction & 0x08
//	} else if funcType == FunctionTypeUpdate {
//		te.ActionName = "PreQosPipe.load_tunnel_far_attributes"
//		te.ParamSize = 8
//		te.Params = make([]ActionParam, te.ParamSize)
//		te.Params[0].Name = "needs_dropping"
//		te.Params[0].Value = make([]byte, 1)
//		te.Params[0].Value[0] = farEntry.applyAction & 0x01
//		te.Params[1].Name = "notify_cp"
//		te.Params[1].Value = make([]byte, 1)
//		if (farEntry.applyAction & 0x08) != 0 {
//			te.Params[1].Value[0] = byte(0x01)
//		}
//		te.Params[2].Name = "needs_buffering"
//		te.Params[2].Value = make([]byte, 1)
//		if (farEntry.applyAction & 0x04) != 0 {
//			te.Params[2].Value[0] = byte(0x01)
//		}
//		te.Params[3].Name = "src_addr"
//		te.Params[3].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Params[3].Value, farEntry.tunnelIP4Src)
//		te.Params[4].Name = "dst_addr"
//		te.Params[4].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Params[4].Value, farEntry.tunnelIP4Dst)
//		te.Params[5].Name = "teid"
//		te.Params[5].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Params[5].Value, farEntry.tunnelTEID)
//		te.Params[6].Name = "sport"
//		te.Params[6].Value = make([]byte, 2)
//		binary.BigEndian.PutUint16(te.Params[6].Value, farEntry.tunnelPort)
//		te.Params[7].Name = "tunnel_type"
//		enumName := "TunnelType"
//		var tunnelStr string
//		switch farEntry.tunnelType {
//		case 0x01:
//			tunnelStr = "GTPU"
//		default:
//			log.Println("Unknown tunneling not handled in p4rt.")
//			return nil
//		}
//
//		val, err := c.getEnumVal(enumName, tunnelStr)
//		if err != nil {
//			log.Println("Could not find enum val ", err)
//			return err
//		}
//		te.Params[7].Value = make([]byte, 1)
//		te.Params[7].Value[0] = val[0]
//	}
//
//	return c.InsertTableEntry(te, funcType, prio)
//}
//
//// WritePdrTable .. Write pdr table entry API.
//func (c *P4rtClient) WritePdrTable(pdrEntry pdr, funcType uint8) error {
//	log.Println("WritePdrTable.")
//
//	te := AppTableEntry{
//		TableName:  "PreQosPipe.pdrs",
//		ActionName: "PreQosPipe.set_pdr_attributes",
//	}
//
//	te.FieldSize = 4
//	te.Fields = make([]MatchField, te.FieldSize)
//	te.FieldSize = 2
//	te.Fields[0].Name = SrcIfaceStr
//	enumName := InterfaceTypeStr
//
//	var (
//		srcIntfStr string
//		decapVal   uint8
//	)
//
//	if pdrEntry.srcIface == access {
//		srcIntfStr = "ACCESS"
//		decapVal = 1
//	} else {
//		srcIntfStr = "CORE"
//	}
//
//	val, _ := c.getEnumVal(enumName, srcIntfStr)
//	te.Fields[0].Value = val
//
//	if pdrEntry.srcIface == access {
//		te.FieldSize = 3
//		te.Fields[1].Name = "teid"
//		te.Fields[1].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Fields[1].Value, pdrEntry.tunnelTEID)
//		te.Fields[1].Mask = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Fields[1].Mask, pdrEntry.tunnelTEIDMask)
//		// te.Fields[2].Mask =  b
//
//		te.Fields[2].Name = "tunnel_ipv4_dst"
//		te.Fields[2].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Fields[2].Value, pdrEntry.tunnelIP4Dst)
//		te.Fields[2].Mask = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Fields[2].Mask, pdrEntry.tunnelIP4DstMask)
//	} else if pdrEntry.srcIface == core {
//		te.Fields[1].Name = "ue_addr"
//		te.Fields[1].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Fields[1].Value, pdrEntry.dstIP)
//		te.Fields[1].Mask = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Fields[1].Mask, pdrEntry.dstIPMask)
//	}
//
//	var prio int32 = 2
//
//	if funcType == FunctionTypeDelete {
//		te.ActionName = "NoAction"
//		te.ParamSize = 0
//
//		go func() {
//			ret := c.InsertTableEntry(te, funcType, prio)
//			if ret != nil {
//				log.Println("Insert Table entry error : ", ret)
//			}
//		}()
//
//		return nil
//	} else if funcType == FunctionTypeInsert {
//		te.ParamSize = 5
//		te.Params = make([]ActionParam, te.ParamSize)
//		te.Params[0].Name = "id"
//		te.Params[0].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Params[0].Value, pdrEntry.pdrID)
//
//		te.Params[1].Name = "fseid"
//		fseidVal := make([]byte, 12)
//		binary.BigEndian.PutUint32(fseidVal[:4], pdrEntry.fseidIP)
//		binary.BigEndian.PutUint64(fseidVal[4:], pdrEntry.fseID)
//		te.Params[1].Value = make([]byte, 12)
//		copy(te.Params[1].Value, fseidVal)
//
//		te.Params[2].Name = "ctr_id"
//		te.Params[2].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Params[2].Value, pdrEntry.ctrID)
//
//		te.Params[3].Name = "far_id"
//		te.Params[3].Value = make([]byte, 4)
//		binary.BigEndian.PutUint32(te.Params[3].Value, pdrEntry.farID)
//
//		te.Params[4].Name = "needs_gtpu_decap"
//		te.Params[4].Value = make([]byte, 1)
//		te.Params[4].Value[0] = decapVal
//	}
//
//	return c.InsertTableEntry(te, funcType, prio)
//}
//
//// WriteInterfaceTable ... Write Interface table Entry.
//func (c *P4rtClient) WriteInterfaceTable(intfEntry IntfTableEntry, funcType uint8) error {
//	log.Println("WriteInterfaceTable.")
//
//	te := AppTableEntry{
//		TableName:  "PreQosPipe.source_iface_lookup",
//		ActionName: "PreQosPipe.set_source_iface",
//	}
//
//	te.FieldSize = 1
//	te.Fields = make([]MatchField, 1)
//	te.Fields[0].Name = "ipv4_dst_prefix"
//	te.Fields[0].Value = intfEntry.IP
//	te.Fields[0].PrefixLen = uint32(intfEntry.PrefixLen)
//
//	te.ParamSize = 2
//	te.Params = make([]ActionParam, 2)
//	te.Params[0].Name = SrcIfaceStr
//	enumName := InterfaceTypeStr
//
//	val, err := c.getEnumVal(enumName, intfEntry.SrcIntf)
//	if err != nil {
//		log.Println("Could not find enum val ", err)
//		return err
//	}
//
//	te.Params[0].Value = val
//	te.Params[1].Name = "direction"
//	enumName = "Direction"
//
//	val, err = c.getEnumVal(enumName, intfEntry.Direction)
//	if err != nil {
//		log.Println("Could not find enum val ", err)
//		return nil
//	}
//
//	te.Params[1].Value = val
//
//	var prio int32
//
//	return c.InsertTableEntry(te, funcType, prio)
//}

func (c *P4rtClient) getCounterValue(entity *p4.Entity, ce *IntfCounterEntry) error {
	entry := entity.GetCounterEntry()
	if entry == nil {
		return ErrOperationFailedWithReason("get counter value", "cannot get counter entry from P4 entity")
	}

	index := uint64(entry.GetIndex().Index)
	byteCount := uint64(entry.GetData().ByteCount)
	pktCount := uint64(entry.GetData().PacketCount)
	ce.ByteCount[index] = byteCount
	ce.PktCount[index] = pktCount
	log.Traceln("index , bytecount, pktcount ", index, byteCount, pktCount)

	return nil
}

func (c *P4rtClient) getFieldValue(entity *p4.Entity, te AppTableEntry) (*MatchField, error) {
	log.Println("get Field Value")

	entry := entity.GetTableEntry()
	tableID := c.tableID(te.TableName)
	actionID := c.actionID(te.ActionName)
	inputField := te.Fields[0]
	inputParam := te.Params[0]

	if entry.TableId != tableID {
		return nil, ErrInvalidArgument("tableID", tableID)
	} else if entry.Action.GetAction().ActionId != actionID {
		return nil, ErrInvalidArgument("ActionID", actionID)
	}

	var (
		matchType p4ConfigV1.MatchField_MatchType
		fieldID   uint32
		paramID   uint32
	)

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
						return nil, ErrUnsupported("MatchType for FieldMatch", matchType)
					}

					log.Println("Field value found.")

					return &inputField, nil
				}
			}
		}
	}

	return nil, ErrOperationFailed("getField Value")
}

func (c *P4rtClient) addFieldValue(entry *p4.TableEntry, field MatchField, tableID uint32) error {
	log.Traceln("add Match field")

	fieldVal := &p4.FieldMatch{
		FieldId: 0,
	}

	for _, tables := range c.P4Info.Tables {
		if tables.Preamble.Id == tableID {
			for _, fields := range tables.MatchFields {
				if fields.Name == field.Name {
					log.Traceln("field name match found.")

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
						return ErrUnsupported("MatchType for FieldMatch", fields.GetMatchType())
					}

					entry.Match = append(entry.Match, fieldVal)

					return nil
				}
			}
		}
	}

	return ErrOperationFailed("addField Value")
}

func (c *P4rtClient) addActionValue(action *p4.Action, param ActionParam,
	actionID uint32) error {
	log.Traceln("add action param value")

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

	return ErrOperationFailed("addAction Value")
}

// ReadCounter ... Read Counter entry.
func (c *P4rtClient) ReadCounter(ce *IntfCounterEntry) error {
	log.Println("ReadCounter ID : ", ce.CounterID)

	readRes, err := c.ReadCounterEntry(ce)
	if err != nil {
		log.Println("Read Counters failed ", err)
		return err
	}

	log.Traceln(proto.MarshalTextString(readRes))

	for _, ent := range readRes.GetEntities() {
		err := c.getCounterValue(ent, ce)
		if err != nil {
			log.Println("getCounterValue failed ", err)
			continue
		}
	}

	return nil
}

// ReadCounterEntry .. Read counter Entry.
func (c *P4rtClient) ReadCounterEntry(ce *IntfCounterEntry) (*p4.ReadResponse, error) {
	log.Traceln("Read Counter Entry")

	var (
		index    p4.Index
		entry    p4.CounterEntry
		entity   p4.Entity
		ctrEntry p4.Entity_CounterEntry
	)

	index.Index = int64(ce.Index)
	entry.CounterId = uint32(ce.CounterID)
	// entry.Index = &index
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
	log.Traceln(proto.MarshalTextString(&entity))

	return c.ReadReq(&entity)
}

// ClearFarTable ... Clear FAR Table.
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

// ClearPdrTable ... Clear PDR Table.
func (c *P4rtClient) ClearPdrTable() error {
	log.Println("ClearPdrTable")

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

// ReadInterfaceTable ... Read Interface table Entry.
func (c *P4rtClient) ReadInterfaceTable(intfEntry *IntfTableEntry) error {
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
	te.Params[0].Name = SrcIfaceStr
	enumName := InterfaceTypeStr

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

	return ErrOperationFailed("ReadInterfaceTable")
}

// ReadTableEntry ... Read table Entry.
func (c *P4rtClient) ReadTableEntry(tableEntry AppTableEntry, prio int32) (*p4.ReadResponse, error) {
	log.Println("Read Table Entry for Table ", tableEntry.TableName)
	tableID := c.tableID(tableEntry.TableName)

	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: prio,
	}

	entity := &p4.Entity{
		Entity: &p4.Entity_TableEntry{TableEntry: entry},
	}
	log.Traceln(proto.MarshalTextString(entity))

	return c.ReadReq(entity)
}

// ReadReqEntities ... Read request Entity.
func (c *P4rtClient) ReadReqEntities(entities []*p4.Entity) (*p4.ReadResponse, error) {
	req := &p4.ReadRequest{
		DeviceId: c.DeviceID,
		Entities: entities,
	}
	log.Traceln(proto.MarshalTextString(req))

	readClient, err := c.client.Read(context.Background(), req)
	if err == nil {
		readRes, err := readClient.Recv()
		if err == nil {
			log.Traceln(proto.MarshalTextString(readRes))
			return readRes, nil
		}
	}

	return nil, err
}

// ReadReq ... Read Request.
func (c *P4rtClient) ReadReq(entity *p4.Entity) (*p4.ReadResponse, error) {
	var req p4.ReadRequest
	req.DeviceId = c.DeviceID
	req.Entities = []*p4.Entity{entity}

	ctx, cancel := context.WithTimeout(context.Background(),
		2*time.Second)
	defer cancel()

	log.Traceln(proto.MarshalTextString(&req))

	readClient, err := c.client.Read(ctx, &req)
	if err == nil {
		readRes, err := readClient.Recv()
		if err == nil {
			log.Traceln(proto.MarshalTextString(readRes))
			return readRes, nil
		}
	}

	return nil, err
}

// InsertTableEntry .. Insert table Entry.
func (c *P4rtClient) InsertTableEntry(tableEntry AppTableEntry, funcType uint8, prio int32) error {
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
		if count >= len(tableEntry.Fields) {
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

	log.Traceln(proto.MarshalTextString(update))

	return c.WriteReq(update)
}

func (c *P4rtClient) WriteTableEntries(entries ...*p4.TableEntry) error {
	var updates []*p4.Update
	for _, entry := range entries {
		update := &p4.Update{
			Type: p4.Update_INSERT,
			Entity: &p4.Entity{
				Entity: &p4.Entity_TableEntry{TableEntry: entry},
			},
		}
		log.Traceln("Writing table entry: ", proto.MarshalTextString(update))
		updates = append(updates, update)
	}

	return c.WriteBatchReq(updates)
}

// WriteReq ... Write Request.
func (c *P4rtClient) WriteReq(update *p4.Update) error {
	req := &p4.WriteRequest{
		DeviceId:   c.DeviceID,
		ElectionId: &c.electionID,
		Updates:    []*p4.Update{update},
	}
	_, err := c.client.Write(context.Background(), req)

	return err
}

// WriteBatchReq ... Write batch Request to up4.
func (c *P4rtClient) WriteBatchReq(updates []*p4.Update) error {
	req := &p4.WriteRequest{
		DeviceId:   c.DeviceID,
		ElectionId: &c.electionID,
	}

	req.Updates = append(req.Updates, updates...)

	log.Traceln(proto.MarshalTextString(req))
	_, err := c.client.Write(context.Background(), req)

	return err
}

// GetForwardingPipelineConfig ... Get Pipeline config from switch.
func (c *P4rtClient) GetForwardingPipelineConfig() (err error) {
	log.Println("GetForwardingPipelineConfig")

	pipeline, err := GetPipelineConfig(c.client, c.DeviceID)
	if err != nil {
		log.Println("set pipeline config error ", err)
		return
	}

	c.P4Info = *pipeline.Config.P4Info

	return
}

// GetPipelineConfig ... Set pipeline config.
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

// SetForwardingPipelineConfig ..
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

	err = SetPipelineConfig(c.client, c.DeviceID, &c.electionID, &pipeline)
	if err != nil {
		log.Println("set pipeline config error ", err)
		return
	}

	return
}

// SetPipelineConfig ... Set pipeline config.
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

// GetConnection ... Get Grpc connection.
func GetConnection(host string) (conn *grpc.ClientConn, err error) {
	/* get connection */
	log.Println("Get connection.")

	conn, err = grpc.Dial(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Println("grpc dial err: ", err)
		return nil, err
	}

	return
}

// LoadDeviceConfig : Load Device config.
func LoadDeviceConfig(deviceConfigPath string) (P4DeviceConfig, error) {
	log.Println("BMv2 JSON: ", deviceConfigPath)

	deviceConfig, err := os.Open(deviceConfigPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", deviceConfigPath, err)
	}
	defer deviceConfig.Close()

	bmv2Info, err := deviceConfig.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", deviceConfigPath, err)
	}

	bin := make([]byte, int(bmv2Info.Size()))
	if b, err := deviceConfig.Read(bin); err != nil {
		return nil, fmt.Errorf("read %s: %w", deviceConfigPath, err)
	} else if b != int(bmv2Info.Size()) {
		return nil, ErrOperationFailedWithReason("bmv2 bin copy", "invalid size of read config")
	}

	return bin, nil
}

// CreateChannel ... Create p4runtime client channel.
func CreateChannel(host string,
	deviceID uint64,
	reportNotifyChan chan<- uint64) (*P4rtClient, error) {
	log.Println("create channel")
	// Second, check to see if we can reuse the gRPC connection for a new P4RT client
	conn, err := GetConnection(host)
	if err != nil {
		log.Println("grpc connection failed")
		return nil, err
	}

	client := &P4rtClient{
		client:   p4.NewP4RuntimeClient(conn),
		conn:     conn,
		DeviceID: deviceID,
	}

	err = client.Init(reportNotifyChan)
	if err != nil {
		log.Println("client Init error: ", err)
		return nil, err
	}

	err = client.SetMastership(p4.Uint128{High: 0, Low: 1})
	if err != nil {
		log.Println("Set Mastership error: ", err)
		return nil, err
	}

	return client, nil
}
