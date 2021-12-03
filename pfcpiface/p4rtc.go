// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package main

import (
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
	deviceID   uint64

	// exported fields
	P4Info   p4ConfigV1.P4Info
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

// FIXME: write functions to read counters
//func (c *P4rtClient) getCounterValue(entity *p4.Entity, ce *IntfCounterEntry) error {
//	entry := entity.GetCounterEntry()
//	index := uint64(entry.GetIndex().Index)
//	byteCount := uint64(entry.GetData().ByteCount)
//	pktCount := uint64(entry.GetData().PacketCount)
//	ce.ByteCount[index] = byteCount
//	ce.PktCount[index] = pktCount
//	log.Traceln("index , bytecount, pktcount ", index, byteCount, pktCount)
//
//	return nil
//}
//
//// ReadCounter ... Read Counter entry.
//func (c *P4rtClient) ReadCounter(ce *IntfCounterEntry) error {
//	log.Println("ReadCounter ID : ", ce.CounterID)
//
//	readRes, err := c.ReadCounterEntry(ce)
//	if err != nil {
//		log.Println("Read Counters failed ", err)
//		return err
//	}
//
//	log.Traceln(proto.MarshalTextString(readRes))
//
//	for _, ent := range readRes.GetEntities() {
//		err := c.getCounterValue(ent, ce)
//		if err != nil {
//			log.Println("getCounterValue failed ", err)
//			continue
//		}
//	}
//
//	return nil
//}
//
//// ReadCounterEntry .. Read counter Entry.
//func (c *P4rtClient) ReadCounterEntry(ce *IntfCounterEntry) (*p4.ReadResponse, error) {
//	log.Traceln("Read Counter Entry")
//
//	var (
//		index    p4.Index
//		entry    p4.CounterEntry
//		entity   p4.Entity
//		ctrEntry p4.Entity_CounterEntry
//	)
//
//	index.Index = int64(ce.Index)
//	entry.CounterId = uint32(ce.CounterID)
//	// entry.Index = &index
//	ctrEntry.CounterEntry = &entry
//	entity.Entity = &ctrEntry
//	/*
//		index := &p4.Index{
//			Index: int64(ce.Index),
//		}
//		entry := &p4.CounterEntry{
//			CounterId: uint32(ce.CounterID),
//			Index:     index,
//		}
//
//		entity := &p4.Entity{
//			Entity: &p4.Entity_CounterEntry{CounterEntry: entry},
//		}*/
//	log.Traceln(proto.MarshalTextString(&entity))
//
//	return c.ReadReq(&entity)
//}

// ReadTableEntry ... Read table Entry.
func (c *P4rtClient) ReadTableEntry(entry *p4.TableEntry, prio int32) (*p4.ReadResponse, error) {
	log.Println("Read Table Entry for Table ", entry.TableId)

	entity := &p4.Entity{
		Entity: &p4.Entity_TableEntry{TableEntry: entry},
	}
	log.Traceln(proto.MarshalTextString(entity))

	return c.ReadReq(entity)
}

// ReadReqEntities ... Read request Entity.
func (c *P4rtClient) ReadReqEntities(entities []*p4.Entity) (*p4.ReadResponse, error) {
	req := &p4.ReadRequest{
		DeviceId: c.deviceID,
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
	req.DeviceId = c.deviceID
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
func (c *P4rtClient) InsertTableEntry(entry *p4.TableEntry, funcType uint8, prio int32) error {
	log.Println("Insert Table Entry for Table ", entry.TableId)

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
		DeviceId:   c.deviceID,
		ElectionId: &c.electionID,
		Updates:    []*p4.Update{update},
	}
	_, err := c.client.Write(context.Background(), req)

	return err
}

// WriteBatchReq ... Write batch Request to up4.
func (c *P4rtClient) WriteBatchReq(updates []*p4.Update) error {
	req := &p4.WriteRequest{
		DeviceId:   c.deviceID,
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

	pipeline, err := GetPipelineConfig(c.client, c.deviceID)
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

	err = SetPipelineConfig(c.client, c.deviceID, &c.electionID, &pipeline)
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
		deviceID: deviceID,
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
