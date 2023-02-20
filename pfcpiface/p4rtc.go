// SPDX-License-Identifier: Apache-2.0
// Copyright 2021-present Open Networking Foundation

package pfcpiface

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc/connectivity"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

const invalidID = 0 //nolint:unused

// Table Entry Function Type.
const (
	FunctionTypeInsert uint8 = 1 // Insert table Entry Function
	FunctionTypeUpdate uint8 = 2 // Update table Entry Function
	FunctionTypeDelete uint8 = 3 // Delete table Entry Function
)

// P4rtClient ... P4 Runtime client object.
type P4rtClient struct {
	client     p4.P4RuntimeClient
	conn       *grpc.ClientConn
	stream     p4.P4Runtime_StreamChannelClient
	electionID p4.Uint128
	deviceID   uint64
	digests    chan *p4.DigestList

	// exported fields
	P4Info *p4ConfigV1.P4Info
}

type P4RuntimeError struct {
	errors []*p4.Error
}

func (e *P4RuntimeError) Error() string {
	return fmt.Sprintf("P4RuntimeError: %v", e.errors)
}

func (e *P4RuntimeError) Get() []*p4.Error {
	return e.errors
}

// convertError parses nested P4Runtime errors.
// See https://p4.org/p4-spec/p4runtime/main/P4Runtime-Spec.html#sec-error-reporting-messages.
func convertError(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	if st.Code() != codes.Unknown {
		return err
	}

	p4RtError := &P4RuntimeError{
		errors: make([]*p4.Error, 0),
	}

	for _, detailItem := range st.Details() {
		p4Error, ok := detailItem.(*p4.Error)
		if !ok {
			p4Error = &p4.Error{
				CanonicalCode: int32(codes.Unknown),
				Message:       "failed to unpack P4 error",
			}
		}

		p4RtError.errors = append(p4RtError.errors, p4Error)
	}

	return p4RtError
}

// TimeBasedElectionId Generates an election id that is monotonically increasing with time.
// Specifically, the upper 64 bits are the unix timestamp in seconds, and the
// lower 64 bits are the remaining nanoseconds. This is compatible with
// election-systems that use the same epoch-based election IDs, and in that
// case, this election ID will be guaranteed to be higher than any previous
// election ID. This is useful in tests where repeated connections need to
// acquire mastership reliably.
func TimeBasedElectionId() p4.Uint128 {
	now := time.Now()

	return p4.Uint128{
		High: uint64(now.Unix()),
		Low:  uint64(now.UnixNano() % 1e9),
	}
}

// CheckStatus ... Check client connection status.
func (c *P4rtClient) CheckStatus() connectivity.State {
	return c.conn.GetState()
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
func (c *P4rtClient) Init() (err error) {
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
				c.digests <- dig
			} else {
				log.Println("stream recv: ", res)
			}
		}
	}()

	log.Println("exited from recv thread.")

	return
}

func (c *P4rtClient) GetNextDigestData() []byte {
	// blocking
	nextDigest := <-c.digests

	for _, p4d := range nextDigest.GetData() {
		if bitstring := p4d.GetBitstring(); bitstring != nil {
			log.WithFields(log.Fields{
				"device-id":     c.deviceID,
				"conn":          c.conn.Target(),
				"digest length": len(bitstring),
				"data":          bitstring,
			}).Trace("Received Digest")

			return bitstring
		}
	}

	return nil
}

// ReadCounterEntry .. Read counter Entry.
func (c *P4rtClient) ReadCounterEntry(entry *p4.CounterEntry) (*p4.ReadResponse, error) {
	log.Traceln("Read Counter Entry ", entry.CounterId)

	entity := &p4.Entity{
		Entity: &p4.Entity_CounterEntry{CounterEntry: entry},
	}

	log.Traceln(proto.MarshalTextString(entity))

	return c.ReadReq(entity)
}

// ReadTableEntry ... Read table Entry.
func (c *P4rtClient) ReadTableEntry(entry *p4.TableEntry) (*p4.ReadResponse, error) {
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

func (c *P4rtClient) ClearTable(tableID uint32) error {
	log.Traceln("Clearing P4 table: ", tableID)

	entry := &p4.TableEntry{
		TableId:  tableID,
		Priority: DefaultPriority,
	}

	readRes, err := c.ReadTableEntry(entry)
	if err != nil {
		return err
	}

	updates := make([]*p4.Update, len(readRes.GetEntities()))

	for _, entity := range readRes.GetEntities() {
		update := &p4.Update{
			Type:   p4.Update_DELETE,
			Entity: entity,
		}

		updates = append(updates, update)
	}

	return c.WriteBatchReq(updates)
}

func (c *P4rtClient) ClearTables(tableIDs []uint32) error {
	log.WithFields(log.Fields{
		"table IDs": tableIDs,
	}).Traceln("Clearing P4 tables")

	updates := []*p4.Update{}

	for _, tableID := range tableIDs {
		entry := &p4.TableEntry{
			TableId:  tableID,
			Priority: DefaultPriority,
		}

		readRes, err := c.ReadTableEntry(entry)
		if err != nil {
			return err
		}

		for _, entity := range readRes.GetEntities() {
			updateType := p4.Update_DELETE
			update := &p4.Update{
				Type:   updateType,
				Entity: entity,
			}

			updates = append(updates, update)
		}
	}

	return c.WriteBatchReq(updates)
}

// InsertTableEntry .. Insert table Entry.
func (c *P4rtClient) InsertTableEntry(entry *p4.TableEntry, funcType uint8) error {
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

func (c *P4rtClient) ApplyTableEntries(methodType p4.Update_Type, entries ...*p4.TableEntry) error {
	var updates []*p4.Update

	for _, entry := range entries {
		update := &p4.Update{
			Type: methodType,
			Entity: &p4.Entity{
				Entity: &p4.Entity_TableEntry{TableEntry: entry},
			},
		}
		log.Traceln("Writing table entry: ", proto.MarshalTextString(update))

		updates = append(updates, update)
	}

	return c.WriteBatchReq(updates)
}

func (c *P4rtClient) ApplyMeterEntries(methodType p4.Update_Type, entries ...*p4.MeterEntry) error {
	var updates []*p4.Update

	for _, entry := range entries {
		update := &p4.Update{
			Type: methodType,
			Entity: &p4.Entity{
				Entity: &p4.Entity_MeterEntry{MeterEntry: entry},
			},
		}
		log.Traceln("Writing meter entry: ", proto.MarshalTextString(update))
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

	return convertError(err)
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

	return convertError(err)
}

// GetForwardingPipelineConfig ... Get Pipeline config from switch.
func (c *P4rtClient) GetForwardingPipelineConfig() (err error) {
	getLog := log.WithFields(log.Fields{
		"device ID": c.deviceID,
		"conn":      c.conn.Target(),
	})
	getLog.Info("Getting ForwardingPipelineConfig from P4Rt device")

	pipeline, err := GetPipelineConfig(c.client, c.deviceID)
	if err != nil {
		getLog.Println("set pipeline config error ", err)
		return
	}

	// P4 spec allows for sending successful response to GetForwardingPipelineConfig
	// without config. We fail in such a case, because the response without config is useless.
	if pipeline.GetConfig() == nil {
		return ErrOperationFailedWithReason("GetForwardingPipelineConfig",
			"Operation successful, but no P4 config provided.")
	}

	c.P4Info = pipeline.Config.P4Info

	getLog.Info("Got ForwardingPipelineConfig from P4Rt device")

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

	p4infoBytes, err := os.ReadFile(p4InfoPath)
	if err != nil {
		log.Println("Read p4info file error ", err)
		return
	}

	p4Info := &p4ConfigV1.P4Info{}

	err = proto.UnmarshalText(string(p4infoBytes), p4Info)
	if err != nil {
		log.Println("Unmarshal test failed for p4info ", err)
		return
	}

	c.P4Info = p4Info

	deviceConfig, err := LoadDeviceConfig(deviceConfigPath)
	if err != nil {
		log.Println("bmv2 json read failed ", err)
		return
	}

	var pipeline p4.ForwardingPipelineConfig
	pipeline.P4Info = p4Info
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
func CreateChannel(host string, deviceID uint64) (*P4rtClient, error) {
	log.Println("create channel")

	conn, err := GetConnection(host)
	if err != nil {
		log.Println("grpc connection failed")
		return nil, err
	}

	client := &P4rtClient{
		digests:  make(chan *p4.DigestList, 1024),
		client:   p4.NewP4RuntimeClient(conn),
		conn:     conn,
		deviceID: deviceID,
	}

	err = client.Init()
	if err != nil {
		log.Println("client Init error: ", err)
		return nil, err
	}

	closeStreamOnError := func() {
		if client.stream != nil {
			err := client.stream.CloseSend()
			if err != nil {
				log.Errorf("Failed to close P4Rt stream with %v: %v", client.conn.Target(), err)
			}
		}
	}

	err = client.SetMastership(TimeBasedElectionId())
	if err != nil {
		log.Error("Set Mastership error: ", err)
		closeStreamOnError()

		return nil, err
	}

	err = client.GetForwardingPipelineConfig()
	if err != nil {
		closeStreamOnError()
		return nil, err
	}

	return client, nil
}
